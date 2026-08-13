package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/userconfig"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "pact:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("a command is required")
	}

	switch args[0] {
	case "login":
		return runLogin(args[1:], stdin, stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "connect":
		return runConnect(args[1:], stdout, stderr)
	case "agent":
		return runAgent(args[1:], stdin, stdout, stderr)
	case "version":
		return json.NewEncoder(stdout).Encode(buildinfo.Current())
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAgent(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "run" {
		return errors.New("expected pact agent run")
	}
	flags := flag.NewFlagSet("pact agent run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	clientType := flags.String("client", "", "agent client type, such as codex, claude, or kimi")
	agentName := flags.String("name", "", "agent display name (defaults to the client type)")
	projectPath := flags.String("path", ".", "path inside the connected Pact project")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	*clientType = strings.ToLower(strings.TrimSpace(*clientType))
	if *clientType == "" {
		return errors.New("pact agent run requires --client")
	}
	if strings.TrimSpace(*agentName) == "" {
		*agentName = strings.ToUpper((*clientType)[:1]) + (*clientType)[1:]
	}
	binding, err := localproject.LoadBinding(*projectPath)
	if err != nil {
		return err
	}
	login, err := loginForServer(binding.ServerURL)
	if err != nil {
		return err
	}
	node, err := localproject.EnsureNodeIdentity(binding.Root)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.APIToken)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startContext, cancelStart := context.WithTimeout(ctx, 15*time.Second)
	session, err := client.StartAgentSession(startContext, binding.ProjectID, agentsession.StartInput{
		NodeKey:    node.Key,
		NodeName:   node.Name,
		AgentName:  strings.TrimSpace(*agentName),
		AgentType:  *clientType,
		ClientType: *clientType,
		ObserveGit: false,
	})
	cancelStart()
	if err != nil {
		return fmt.Errorf("start agent session: %w", err)
	}
	defer func() {
		closeContext, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelClose()
		_ = client.CloseAgentSession(closeContext, session.ID)
	}()
	fmt.Fprintf(stdout, "PACT agent session active: %s (%s)\n", session.ActorName, session.ID)

	heartbeatErrors := make(chan error, 1)
	go maintainHeartbeat(ctx, client, session.ID, heartbeatErrors)
	commandArguments := flags.Args()
	if len(commandArguments) == 0 {
		select {
		case <-ctx.Done():
			return nil
		case heartbeatErr := <-heartbeatErrors:
			return heartbeatErr
		}
	}

	command := exec.Command(commandArguments[0], commandArguments[1:]...)
	command.Dir = binding.Root
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = append(os.Environ(),
		"PACT_SESSION_ID="+session.ID,
		"PACT_PROJECT_ID="+binding.ProjectID,
		"PACT_SERVER_URL="+binding.ServerURL,
	)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start %s: %w", commandArguments[0], err)
	}
	commandResult := make(chan error, 1)
	go func() { commandResult <- command.Wait() }()
	select {
	case commandErr := <-commandResult:
		if commandErr != nil {
			return fmt.Errorf("agent command stopped: %w", commandErr)
		}
		return nil
	case heartbeatErr := <-heartbeatErrors:
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		<-commandResult
		return heartbeatErr
	case <-ctx.Done():
		if command.Process != nil {
			_ = command.Process.Signal(os.Interrupt)
		}
		select {
		case <-commandResult:
		case <-time.After(3 * time.Second):
			if command.Process != nil {
				_ = command.Process.Kill()
			}
			<-commandResult
		}
		return nil
	}
}

func maintainHeartbeat(
	ctx context.Context,
	client *pactclient.Client,
	sessionID string,
	errorsChannel chan<- error,
) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeatContext, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := client.HeartbeatAgentSession(heartbeatContext, sessionID)
			cancel()
			if err != nil {
				select {
				case errorsChannel <- fmt.Errorf("heartbeat agent session: %w", err):
				default:
				}
				return
			}
		}
	}
}

func runLogin(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact login", flag.ContinueOnError)
	flags.SetOutput(stderr)
	serverURL := flags.String("server", "", "Pact Server URL")
	tokenStdin := flags.Bool("token-stdin", false, "read the API token from standard input")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("pact login does not accept a project path")
	}
	if strings.TrimSpace(*serverURL) == "" {
		return errors.New("pact login requires --server")
	}

	token := strings.TrimSpace(os.Getenv("PACT_API_TOKEN"))
	if *tokenStdin {
		content, err := io.ReadAll(io.LimitReader(stdin, 4097))
		if err != nil {
			return fmt.Errorf("read API token: %w", err)
		}
		if len(content) > 4096 {
			return errors.New("API token is too large")
		}
		token = strings.TrimSpace(string(content))
	}
	if token == "" {
		return errors.New("set PACT_API_TOKEN or use --token-stdin")
	}
	normalizedServer, err := userconfig.NormalizeServerURL(*serverURL)
	if err != nil {
		return err
	}
	client, err := pactclient.New(normalizedServer, token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := client.ListProjects(ctx); err != nil {
		return fmt.Errorf("authenticate with %s: %w", normalizedServer, err)
	}
	path, err := userconfig.Save(normalizedServer, token)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Logged in to %s\n", normalizedServer)
	fmt.Fprintf(stdout, "  user configuration  %s\n", path)
	fmt.Fprintln(stdout, "The token was stored outside project repositories with mode 0600.")
	return nil
}

func runInit(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	serverURL := flags.String("server", "", "Pact Server URL (defaults to the logged-in server)")
	name := flags.String("name", "", "project name (defaults to the repository directory)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("pact init accepts at most one project path")
	}
	projectPath := "."
	if flags.NArg() == 1 {
		projectPath = flags.Arg(0)
	}

	login, err := loginForServer(*serverURL)
	if err != nil {
		return err
	}
	result, err := localproject.Init(localproject.InitOptions{
		StartPath: projectPath,
		Name:      *name,
		ServerURL: login.ServerURL,
	})
	if err != nil {
		return err
	}
	descriptor, err := localproject.Describe(result.Root)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.APIToken)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	project, created, err := resolveProject(ctx, client, descriptor, "", true)
	if err != nil {
		return err
	}
	if err := localproject.Bind(result.Root, login.ServerURL, project.ID); err != nil {
		return err
	}

	state := "Connected"
	if created {
		state = "Created and connected"
	}
	fmt.Fprintf(stdout, "%s Pact project in %s\n", state, result.Root)
	printProjectBinding(stdout, result.ManifestPath, result.LocalDirectory, login.ServerURL, project)
	return nil
}

func runConnect(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact connect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	serverURL := flags.String("server", "", "Pact Server URL (defaults to the logged-in server)")
	projectReference := flags.String("project", "", "existing remote project slug or ID")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("pact connect accepts at most one project path")
	}
	projectPath := "."
	if flags.NArg() == 1 {
		projectPath = flags.Arg(0)
	}
	hasManifest, err := localproject.HasManifest(projectPath)
	if err != nil {
		return err
	}
	if !hasManifest {
		return errors.New("pact.yaml is missing; the project owner must run pact init first")
	}
	login, err := loginForServer(*serverURL)
	if err != nil {
		return err
	}
	result, err := localproject.Init(localproject.InitOptions{
		StartPath: projectPath,
		ServerURL: login.ServerURL,
	})
	if err != nil {
		return err
	}
	descriptor, err := localproject.Describe(result.Root)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.APIToken)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	project, _, err := resolveProject(ctx, client, descriptor, *projectReference, false)
	if err != nil {
		return err
	}
	if err := localproject.Bind(result.Root, login.ServerURL, project.ID); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Connected existing Pact project in %s\n", result.Root)
	printProjectBinding(stdout, result.ManifestPath, result.LocalDirectory, login.ServerURL, project)
	return nil
}

func loginForServer(requestedServer string) (userconfig.Config, error) {
	login, err := userconfig.Load()
	if err != nil {
		return userconfig.Config{}, err
	}
	if strings.TrimSpace(requestedServer) == "" {
		return login, nil
	}
	normalized, err := userconfig.NormalizeServerURL(requestedServer)
	if err != nil {
		return userconfig.Config{}, err
	}
	if normalized != login.ServerURL {
		return userconfig.Config{}, fmt.Errorf(
			"logged in to %s, not %s; run pact login for the requested server",
			login.ServerURL,
			normalized,
		)
	}
	return login, nil
}

func resolveProject(
	ctx context.Context,
	client *pactclient.Client,
	descriptor localproject.Descriptor,
	projectReference string,
	allowCreate bool,
) (projects.Project, bool, error) {
	projectList, err := client.ListProjects(ctx)
	if err != nil {
		return projects.Project{}, false, err
	}
	if project, found, err := matchProject(projectList, descriptor.RemoteURL, projectReference); err != nil {
		return projects.Project{}, false, err
	} else if found {
		return project, false, nil
	}
	if !allowCreate {
		return projects.Project{}, false, fmt.Errorf(
			"repository %s is not connected to a remote Pact project; ask its owner to run pact init",
			descriptor.RemoteURL,
		)
	}

	revision := descriptor.CanonicalRevision
	input := projects.CreateInput{
		Name:              descriptor.Name,
		Slug:              descriptor.Slug,
		CanonicalRevision: &revision,
		RootRepository: &projects.SourceRepositoryInput{
			Slug:          "primary",
			Name:          "Primary",
			RemoteURL:     descriptor.RemoteURL,
			DefaultBranch: descriptor.DefaultBranch,
			ObjectFormat:  descriptor.ObjectFormat,
		},
	}
	idempotencyHash := sha256.Sum256([]byte("project.init\x00" + strings.ToLower(descriptor.RemoteURL)))
	project, err := client.CreateProject(ctx, "pact-init-"+hex.EncodeToString(idempotencyHash[:]), input)
	if err == nil {
		return project, true, nil
	}
	var problem *pactclient.Problem
	if !errors.As(err, &problem) || problem.Status != 409 {
		return projects.Project{}, false, err
	}
	projectList, listErr := client.ListProjects(ctx)
	if listErr != nil {
		return projects.Project{}, false, errors.Join(err, listErr)
	}
	project, found, matchErr := matchProject(projectList, descriptor.RemoteURL, projectReference)
	if matchErr != nil {
		return projects.Project{}, false, matchErr
	}
	if found {
		return project, false, nil
	}
	return projects.Project{}, false, err
}

func matchProject(
	projectList []projects.Project,
	remoteURL string,
	projectReference string,
) (projects.Project, bool, error) {
	projectReference = strings.TrimSpace(projectReference)
	for _, project := range projectList {
		if projectReference != "" && project.ID != projectReference && project.Slug != projectReference {
			continue
		}
		if project.RootRepository == nil || project.RootRepository.RemoteURL == nil {
			if projectReference != "" {
				return projects.Project{}, false, errors.New("selected Pact project has no root Git repository")
			}
			continue
		}
		registeredRemote, err := localproject.NormalizeGitRemote(*project.RootRepository.RemoteURL)
		if err != nil {
			return projects.Project{}, false, fmt.Errorf("remote project %s has an invalid repository URL: %w", project.Slug, err)
		}
		if strings.EqualFold(registeredRemote, remoteURL) {
			return project, true, nil
		}
		if projectReference != "" {
			return projects.Project{}, false, fmt.Errorf(
				"project %s belongs to %s, not %s",
				projectReference,
				registeredRemote,
				remoteURL,
			)
		}
	}
	if projectReference != "" {
		return projects.Project{}, false, fmt.Errorf("Pact project %q was not found", projectReference)
	}
	return projects.Project{}, false, nil
}

func printProjectBinding(
	stdout io.Writer,
	manifestPath string,
	localDirectory string,
	serverURL string,
	project projects.Project,
) {
	fmt.Fprintf(stdout, "  shared manifest  %s\n", manifestPath)
	fmt.Fprintf(stdout, "  local runtime    %s\n", localDirectory)
	fmt.Fprintf(stdout, "  Pact Server      %s\n", serverURL)
	fmt.Fprintf(stdout, "  remote project   %s (%s)\n", project.Slug, project.ID)
	fmt.Fprintln(stdout, "No database credentials or API tokens were written to the project.")
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  pact login --server URL [--token-stdin]")
	fmt.Fprintln(writer, "  pact init [--server URL] [--name NAME] [PATH]")
	fmt.Fprintln(writer, "  pact connect [--server URL] [--project SLUG_OR_ID] [PATH]")
	fmt.Fprintln(writer, "  pact agent run --client TYPE [--name NAME] [--path PATH] [-- COMMAND ...]")
	fmt.Fprintln(writer, "  pact version")
}
