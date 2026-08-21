package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/agentconfig"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/authn"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/gitobserve"
	"github.com/jorgenuanzs/the-pact/internal/lifecycle"
	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/localserver"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/repositorysync"
	"github.com/jorgenuanzs/the-pact/internal/serverprofile"
	"github.com/jorgenuanzs/the-pact/internal/userconfig"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
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
	case "servers":
		return runServers(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "connect":
		return runConnect(args[1:], stdout, stderr)
	case "workspace":
		return runWorkspace(args[1:], stdout, stderr)
	case "repository":
		return runRepository(args[1:], stdout, stderr)
	case "enable":
		return runEnable(args[1:], stdout, stderr)
	case "invite":
		return runInvite(args[1:], stdout, stderr)
	case "join":
		return runJoin(args[1:], stdin, stdout, stderr)
	case "whoami":
		return runWhoAmI(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "logout":
		return runLogout(args[1:], stdout, stderr)
	case "agent":
		return runAgent(args[1:], stdin, stdout, stderr)
	case "node":
		return runNode(args[1:], stdout, stderr)
	case "server":
		return runServer(args[1:], stdout, stderr)
	case "mcp":
		return runMCP(args[1:], stderr)
	case "version":
		return json.NewEncoder(stdout).Encode(buildinfo.Current())
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runServer(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected pact server install, start, stop, status, logs, backup, restore, upgrade, or uninstall")
	}
	manager, err := localserver.NewDefault(stdout, stderr)
	if err != nil {
		return err
	}
	ctx, stop := lifecycle.NotifyContext(context.Background())
	defer stop()

	switch args[0] {
	case "install":
		flags := flag.NewFlagSet("pact server install", flag.ContinueOnError)
		flags.SetOutput(stderr)
		port := flags.Int("port", 8080, "loopback port for PACT Server")
		image := flags.String("image", "", "versioned PACT Server container image")
		force := flags.Bool("force", false, "replace the local installation configuration")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("pact server install accepts no positional arguments")
		}
		result, err := manager.Install(ctx, localserver.InstallOptions{Port: *port, Image: *image, Force: *force})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "PACT Server is ready at %s\n", result.Status.ServerURL)
		fmt.Fprintf(stdout, "Open %s/admin/ and use this one-time setup code:\n\n%s\n\n", result.Status.ServerURL, result.SetupCode)
		fmt.Fprintln(stdout, "The setup code is stored in the private local server configuration until the first owner account is created.")
		return nil
	case "start":
		if len(args) != 1 {
			return errors.New("pact server start accepts no arguments")
		}
		status, err := manager.Start(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "PACT Server started at %s (ready: %t).\n", status.ServerURL, status.Ready)
		return nil
	case "stop":
		if len(args) != 1 {
			return errors.New("pact server stop accepts no arguments")
		}
		status, err := manager.Stop(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "PACT Server stopped (data preserved in %s).\n", status.DataDirectory)
		return nil
	case "status":
		flags := flag.NewFlagSet("pact server status", flag.ContinueOnError)
		flags.SetOutput(stderr)
		jsonOutput := flags.Bool("json", false, "print machine-readable JSON")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("pact server status accepts no positional arguments")
		}
		status, err := manager.Status(ctx)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(status)
		}
		if !status.Installed {
			fmt.Fprintln(stdout, "PACT Server is not installed on this computer.")
			return nil
		}
		fmt.Fprintf(stdout, "Installed: yes\nRunning: %t\nReady: %t\nURL: %s\nImage: %s\nData: %s\n", status.Running, status.Ready, status.ServerURL, status.Image, status.DataDirectory)
		if status.Error != "" {
			fmt.Fprintf(stdout, "Problem: %s\n", status.Error)
		}
		return nil
	case "logs":
		flags := flag.NewFlagSet("pact server logs", flag.ContinueOnError)
		flags.SetOutput(stderr)
		follow := flags.Bool("follow", false, "keep streaming new logs")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("pact server logs accepts no positional arguments")
		}
		return manager.Logs(ctx, *follow, stdout)
	case "backup":
		flags := flag.NewFlagSet("pact server backup", flag.ContinueOnError)
		flags.SetOutput(stderr)
		output := flags.String("output", "", "destination .dump file")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("pact server backup accepts no positional arguments")
		}
		path, err := manager.Backup(ctx, *output)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "PACT backup created at %s\n", path)
		return nil
	case "restore":
		flags := flag.NewFlagSet("pact server restore", flag.ContinueOnError)
		flags.SetOutput(stderr)
		input := flags.String("input", "", "source .dump file")
		confirm := flags.Bool("confirm", false, "confirm replacement of the current database")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 || strings.TrimSpace(*input) == "" {
			return errors.New("pact server restore requires --input FILE")
		}
		if !*confirm {
			return errors.New("restore replaces current database contents; repeat with --confirm")
		}
		if err := manager.Restore(ctx, *input); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "PACT Server data restored. Restart the server before continuing.")
		return nil
	case "upgrade":
		flags := flag.NewFlagSet("pact server upgrade", flag.ContinueOnError)
		flags.SetOutput(stderr)
		image := flags.String("image", "", "versioned PACT Server container image")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("pact server upgrade accepts no positional arguments")
		}
		status, backup, err := manager.Upgrade(ctx, *image)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "PACT Server upgraded to %s. Backup: %s\n", status.Image, backup)
		return nil
	case "uninstall":
		flags := flag.NewFlagSet("pact server uninstall", flag.ContinueOnError)
		flags.SetOutput(stderr)
		removeData := flags.Bool("remove-data", false, "also permanently remove PostgreSQL data and local configuration")
		confirm := flags.Bool("confirm", false, "confirm permanent data removal")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("pact server uninstall accepts no positional arguments")
		}
		if *removeData && !*confirm {
			return errors.New("--remove-data is permanent; repeat with --confirm")
		}
		if err := manager.Uninstall(ctx, *removeData); err != nil {
			return err
		}
		if *removeData {
			fmt.Fprintln(stdout, "PACT Server and its local data were removed.")
		} else {
			fmt.Fprintln(stdout, "PACT Server containers were removed; local configuration and PostgreSQL data were preserved.")
		}
		return nil
	default:
		return fmt.Errorf("unknown server command %q", args[0])
	}
}

func runRepository(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected pact repository list, status, or sync")
	}
	flags := flag.NewFlagSet("pact repository "+args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	projectPath := flags.String("path", ".", "path inside the connected Pact project")
	repositoryID := flags.String("repository", "", "project repository UUID (defaults to the primary repository)")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("pact repository %s accepts no positional arguments", args[0])
	}
	binding, err := localproject.LoadBinding(*projectPath)
	if err != nil {
		return err
	}
	login, err := loginForServer(binding.ServerURL)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch args[0] {
	case "list":
		result, err := client.ListProjectRepositories(ctx, binding.ProjectID)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(result)
	case "status":
		var state repositorysync.State
		if strings.TrimSpace(*repositoryID) == "" {
			state, err = client.GetRepositorySync(ctx, binding.ProjectID)
		} else {
			state, err = client.GetProjectRepositorySync(ctx, binding.ProjectID, *repositoryID)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(state)
	case "sync":
		key, err := randomCommandKey("pact-repository-sync")
		if err != nil {
			return err
		}
		var result repositorysync.Result
		if strings.TrimSpace(*repositoryID) == "" {
			result, err = client.SyncRepository(ctx, binding.ProjectID, key)
		} else {
			result, err = client.SyncProjectRepository(ctx, binding.ProjectID, *repositoryID, key)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(result)
	default:
		return fmt.Errorf("unknown repository command %q", args[0])
	}
}

type repeatedString []string

func (values *repeatedString) String() string {
	return strings.Join(*values, ",")
}

func (values *repeatedString) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func runWorkspace(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected pact workspace list, show, create, or add-project")
	}
	login, err := loginForProjectOrActive(".", "")
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	switch args[0] {
	case "list":
		if len(args) != 1 {
			return errors.New("pact workspace list accepts no arguments")
		}
		workspaceList, err := client.ListWorkspaces(ctx)
		if err != nil {
			return err
		}
		if len(workspaceList) == 0 {
			fmt.Fprintln(stdout, "No Pact workspaces are visible.")
			return nil
		}
		for _, workspace := range workspaceList {
			fmt.Fprintf(stdout, "%s\t%s\t%d project(s)\t%s\n", workspace.Slug, workspace.ID, len(workspace.Projects), workspace.Name)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return errors.New("expected pact workspace show SLUG_OR_ID")
		}
		workspace, err := client.GetWorkspace(ctx, args[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(workspace)
	case "create":
		flags := flag.NewFlagSet("pact workspace create", flag.ContinueOnError)
		flags.SetOutput(stderr)
		name := flags.String("name", "", "workspace name")
		slug := flags.String("slug", "", "workspace slug")
		description := flags.String("description", "", "workspace description")
		var projectIDs repeatedString
		flags.Var(&projectIDs, "project", "project UUID to move into the workspace (repeatable)")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 || strings.TrimSpace(*name) == "" || strings.TrimSpace(*slug) == "" {
			return errors.New("pact workspace create requires --name and --slug")
		}
		key, err := randomCommandKey("pact-workspace-create")
		if err != nil {
			return err
		}
		workspace, err := client.CreateWorkspace(ctx, key, workspaces.CreateInput{
			Name: *name, Slug: *slug, Description: *description, ProjectIDs: projectIDs,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Created workspace %s (%s) with %d project(s).\n", workspace.Slug, workspace.ID, len(workspace.Projects))
		return nil
	case "add-project":
		if len(args) != 3 {
			return errors.New("expected pact workspace add-project WORKSPACE_ID PROJECT_ID")
		}
		workspace, err := client.AttachWorkspaceProject(ctx, args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Workspace %s now contains %d project(s).\n", workspace.Slug, len(workspace.Projects))
		return nil
	default:
		return fmt.Errorf("unknown workspace command %q", args[0])
	}
}

func randomCommandKey(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate command key: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(value), nil
}

func runEnable(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected pact enable codex or pact enable claude")
	}
	clientType := strings.ToLower(strings.TrimSpace(args[0]))
	if clientType != "codex" && clientType != "claude" {
		return fmt.Errorf("unsupported agent client %q; currently supported: codex, claude", args[0])
	}
	flags := flag.NewFlagSet("pact enable "+clientType, flag.ContinueOnError)
	flags.SetOutput(stderr)
	projectPath := flags.String("path", ".", "path inside the connected Pact project")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("pact enable %s accepts no positional arguments", clientType)
	}
	binding, err := localproject.LoadBinding(*projectPath)
	if err != nil {
		return err
	}
	if _, err := loginForServer(binding.ServerURL); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve Pact executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve Pact executable symlinks: %w", err)
	}
	if clientType == "codex" {
		result, err := agentconfig.EnableCodex(agentconfig.CodexOptions{
			ProjectRoot: binding.Root,
			PactCommand: executable,
		})
		if err != nil {
			return err
		}
		state := "already enabled"
		if result.Changed {
			state = "enabled"
		}
		fmt.Fprintf(stdout, "Codex MCP %s for %s\n", state, binding.Root)
		fmt.Fprintf(stdout, "  project config  %s\n", result.ConfigPath)
		if result.Excluded {
			fmt.Fprintln(stdout, "  Git visibility  machine-local (excluded through .git/info/exclude)")
		}
		fmt.Fprintln(stdout, "Restart Codex or reload the VS Code window before opening a new chat.")
		return nil
	}
	result, err := agentconfig.EnableClaude(agentconfig.ClaudeOptions{
		ProjectRoot: binding.Root,
		PactCommand: executable,
	})
	if err != nil {
		return err
	}
	state := "already enabled"
	if result.Changed {
		state = "enabled"
	}
	fmt.Fprintf(stdout, "Claude MCP %s for %s\n", state, binding.Root)
	fmt.Fprintf(stdout, "  project config  %s\n", result.ConfigPath)
	if result.Excluded {
		fmt.Fprintln(stdout, "  Git visibility  machine-local (excluded through .git/info/exclude)")
	}
	fmt.Fprintln(stdout, "Restart Claude Code before opening a new chat and approve the project MCP server when prompted.")
	return nil
}

func runAgent(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "run" {
		return errors.New("expected pact agent run")
	}
	flags := flag.NewFlagSet("pact agent run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	clientType := flags.String("client", "", "agent client type, such as codex, claude, or kimi")
	agentName := flags.String("name", "", "legacy agent label (the durable identity is derived from --client)")
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
	initialSnapshot, err := gitobserve.Capture(context.Background(), binding.Root)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, stop := lifecycle.NotifyContext(context.Background())
	defer stop()
	startContext, cancelStart := context.WithTimeout(ctx, 15*time.Second)
	session, err := client.StartAgentSession(startContext, binding.ProjectID, agentsession.StartInput{
		NodeKey:    node.Key,
		NodeName:   node.Name,
		AgentName:  strings.TrimSpace(*agentName),
		AgentType:  *clientType,
		ClientType: *clientType,
		ObserveGit: true,
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
	if err := reportObservation(ctx, client, session.ID, initialSnapshot); err != nil {
		return err
	}

	heartbeatErrors := make(chan error, 1)
	go maintainHeartbeat(ctx, client, session.ID, heartbeatErrors)
	observationErrors := make(chan error, 1)
	go maintainGitObservations(ctx, binding.Root, client, session.ID, initialSnapshot, 2*time.Second, observationErrors)
	commandArguments := flags.Args()
	if len(commandArguments) == 0 {
		select {
		case <-ctx.Done():
			return nil
		case heartbeatErr := <-heartbeatErrors:
			return heartbeatErr
		case observationErr := <-observationErrors:
			return observationErr
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
		if observationErr := reportCurrentObservation(binding.Root, client, session.ID); observationErr != nil {
			return observationErr
		}
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
	case observationErr := <-observationErrors:
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		<-commandResult
		return observationErr
	case <-ctx.Done():
		if command.Process != nil {
			_ = lifecycle.InterruptProcess(command.Process)
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

func runNode(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "run" {
		return errors.New("expected pact node run")
	}
	flags := flag.NewFlagSet("pact node run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	projectPath := flags.String("path", ".", "path inside the connected Pact project")
	interval := flags.Duration("interval", 2*time.Second, "Git observation interval")
	once := flags.Bool("once", false, "capture one observation and exit")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("pact node run accepts no positional arguments")
	}
	if *interval < 250*time.Millisecond || *interval > time.Minute {
		return errors.New("--interval must be between 250ms and 1m")
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
	initialSnapshot, err := gitobserve.Capture(context.Background(), binding.Root)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, stop := lifecycle.NotifyContext(context.Background())
	defer stop()
	startContext, cancelStart := context.WithTimeout(ctx, 15*time.Second)
	session, err := client.StartAgentSession(startContext, binding.ProjectID, agentsession.StartInput{
		NodeKey: node.Key, NodeName: node.Name, AgentName: "Pact Node",
		AgentType: "pact-node", ClientType: "pact-node", ObserveGit: true,
	})
	cancelStart()
	if err != nil {
		return fmt.Errorf("start Pact Node session: %w", err)
	}
	defer func() {
		closeContext, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelClose()
		_ = client.CloseAgentSession(closeContext, session.ID)
	}()
	if err := reportObservation(ctx, client, session.ID, initialSnapshot); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "PACT Node observing %s (%s)\n", binding.Root, session.ID)
	fmt.Fprintln(stdout, "Only Git state metadata and a SHA-256 fingerprint are sent; file names and contents remain local.")
	if *once {
		return nil
	}
	heartbeatErrors := make(chan error, 1)
	go maintainHeartbeat(ctx, client, session.ID, heartbeatErrors)
	observationErrors := make(chan error, 1)
	go maintainGitObservations(ctx, binding.Root, client, session.ID, initialSnapshot, *interval, observationErrors)
	select {
	case <-ctx.Done():
		return nil
	case heartbeatErr := <-heartbeatErrors:
		return heartbeatErr
	case observationErr := <-observationErrors:
		return observationErr
	}
}

func maintainGitObservations(
	ctx context.Context,
	root string,
	client *pactclient.Client,
	sessionID string,
	previous gitobserve.Snapshot,
	interval time.Duration,
	errorsChannel chan<- error,
) {
	maintainGitObservationsForWorkspace(ctx, root, client, sessionID, nil, previous, interval, errorsChannel)
}

func maintainGitObservationsForWorkspace(
	ctx context.Context,
	root string,
	client *pactclient.Client,
	sessionID string,
	workspaceID *string,
	previous gitobserve.Snapshot,
	interval time.Duration,
	errorsChannel chan<- error,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			captureContext, cancel := context.WithTimeout(ctx, 10*time.Second)
			current, err := gitobserve.Capture(captureContext, root)
			cancel()
			if err == nil && current != previous {
				_, err = submitObservationForWorkspace(ctx, client, sessionID, workspaceID, current)
				if err == nil {
					previous = current
				}
			}
			if err != nil {
				select {
				case errorsChannel <- fmt.Errorf("observe Git repository: %w", err):
				default:
				}
				return
			}
		}
	}
}

func reportCurrentObservation(root string, client *pactclient.Client, sessionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snapshot, err := gitobserve.Capture(ctx, root)
	if err != nil {
		return fmt.Errorf("capture final Git observation: %w", err)
	}
	return reportObservation(ctx, client, sessionID, snapshot)
}

func reportObservation(
	ctx context.Context,
	client *pactclient.Client,
	sessionID string,
	snapshot gitobserve.Snapshot,
) error {
	_, err := submitObservation(ctx, client, sessionID, snapshot)
	return err
}

func submitObservation(
	ctx context.Context,
	client *pactclient.Client,
	sessionID string,
	snapshot gitobserve.Snapshot,
) (agentsession.ObservationResult, error) {
	return submitObservationForWorkspace(ctx, client, sessionID, nil, snapshot)
}

func submitObservationForWorkspace(
	ctx context.Context,
	client *pactclient.Client,
	sessionID string,
	workspaceID *string,
	snapshot gitobserve.Snapshot,
) (agentsession.ObservationResult, error) {
	idempotencyKey, err := newIdempotencyKey("pact-observe")
	if err != nil {
		return agentsession.ObservationResult{}, err
	}
	reportContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := client.ObserveRepository(
		reportContext,
		sessionID,
		idempotencyKey,
		agentsession.ObservationInput{
			WorkspaceID: workspaceID,
			Dirty:       snapshot.Dirty, DiffFingerprint: snapshot.Fingerprint,
			ChangedPaths: snapshot.ChangedPaths, HeadRevision: snapshot.HeadRevision,
			Branch: snapshot.Branch,
		},
	)
	if err != nil {
		return agentsession.ObservationResult{}, fmt.Errorf("report Git observation: %w", err)
	}
	return result, nil
}

func newIdempotencyKey(prefix string) (string, error) {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("create %s idempotency key: %w", prefix, err)
	}
	return prefix + "-" + hex.EncodeToString(keyBytes), nil
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

type serverProfileOutput struct {
	ID             string             `json:"id"`
	Label          string             `json:"label"`
	ServerURL      string             `json:"server_url"`
	Kind           serverprofile.Kind `json:"kind"`
	PrincipalID    string             `json:"principal_id,omitempty"`
	PrincipalLabel string             `json:"principal_label,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	LastUsedAt     time.Time          `json:"last_used_at"`
	Active         bool               `json:"active"`
}

func runServers(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected pact servers list, use, or remove")
	}
	switch args[0] {
	case "list":
		flags := flag.NewFlagSet("pact servers list", flag.ContinueOnError)
		flags.SetOutput(stderr)
		jsonOutput := flags.Bool("json", false, "print machine-readable JSON")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("pact servers list accepts no positional arguments")
		}
		profiles, err := userconfig.ListProfiles()
		if err != nil {
			return err
		}
		activeID := ""
		if active, activeErr := userconfig.ActiveProfile(); activeErr == nil {
			activeID = active.ID
		} else if !errors.Is(activeErr, serverprofile.ErrNoActiveProfile) {
			return activeErr
		}
		output := make([]serverProfileOutput, 0, len(profiles))
		for _, profile := range profiles {
			output = append(output, serverProfileOutput{
				ID: profile.ID, Label: profile.Label, ServerURL: profile.ServerURL,
				Kind: profile.Kind, PrincipalID: profile.PrincipalID,
				PrincipalLabel: profile.PrincipalLabel, CreatedAt: profile.CreatedAt,
				LastUsedAt: profile.LastUsedAt, Active: profile.ID == activeID,
			})
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(output)
		}
		if len(output) == 0 {
			fmt.Fprintln(stdout, "No PACT Server profiles are authorized. Run pact login --server URL.")
			return nil
		}
		for _, profile := range output {
			marker := " "
			if profile.Active {
				marker = "*"
			}
			fmt.Fprintf(stdout, "%s %s\n", marker, profile.Label)
			fmt.Fprintf(stdout, "    URL        %s\n", profile.ServerURL)
			fmt.Fprintf(stdout, "    profile ID %s\n", profile.ID)
			if profile.PrincipalLabel != "" {
				fmt.Fprintf(stdout, "    identity   %s\n", profile.PrincipalLabel)
			}
			fmt.Fprintf(stdout, "    last used  %s\n", profile.LastUsedAt.Format(time.RFC3339))
		}
		fmt.Fprintln(stdout, "* active preference for commands without a bound folder")
		return nil
	case "use":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return errors.New("expected pact servers use PROFILE_OR_URL")
		}
		if err := userconfig.SetActiveProfile(args[1]); err != nil {
			return err
		}
		profile, err := userconfig.FindProfile(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Using %s (%s) for commands without a bound folder.\n", profile.Label, profile.ServerURL)
		return nil
	case "remove":
		flags := flag.NewFlagSet("pact servers remove", flag.ContinueOnError)
		flags.SetOutput(stderr)
		localOnly := flags.Bool("local-only", false, "remove the local profile without revoking the device")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 1 {
			return errors.New("expected pact servers remove PROFILE_OR_URL")
		}
		profile, err := removeServerProfile(flags.Arg(0), *localOnly)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Removed PACT Server profile %s (%s).\n", profile.Label, profile.ServerURL)
		return nil
	default:
		return fmt.Errorf("unknown servers command %q", args[0])
	}
}

func runLogin(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact login", flag.ContinueOnError)
	flags.SetOutput(stderr)
	serverURL := flags.String("server", "", "Pact Server URL")
	profileName := flags.String("name", "", "local name for this PACT Server profile")
	deviceName := flags.String("device-name", "", "name shown for this computer")
	noBrowser := flags.Bool("no-browser", false, "print the verification URL without opening it")
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

	normalizedServer, err := userconfig.NormalizeServerURL(*serverURL)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(*deviceName)
	if name == "" {
		hostname, hostnameErr := os.Hostname()
		if hostnameErr != nil || strings.TrimSpace(hostname) == "" {
			hostname = "This computer"
		}
		name = hostname + " (Pact CLI)"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	authorization, err := pactclient.BeginDeviceAuthorization(ctx, normalizedServer, authn.BeginDeviceInput{DeviceName: name})
	defer cancel()
	if err != nil {
		return fmt.Errorf("begin device login with %s: %w", normalizedServer, err)
	}
	verificationURL, err := resolveServerURL(normalizedServer, authorization.VerificationURI)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Open this URL and approve the device:\n  %s\n", verificationURL)
	fmt.Fprintf(stdout, "Confirm that Pact shows this code:\n  %s\n", authorization.UserCode)
	if !*noBrowser {
		if err := openBrowser(verificationURL); err != nil {
			fmt.Fprintf(stderr, "Could not open the browser automatically: %v\n", err)
		}
	}

	interval := time.Duration(authorization.IntervalSeconds) * time.Second
	if interval < time.Second {
		interval = 2 * time.Second
	}
	deadline := time.Until(authorization.ExpiresAt)
	if deadline <= 0 {
		return errors.New("device authorization expired before it could be approved")
	}
	pollContext, cancelPoll := context.WithTimeout(context.Background(), deadline+time.Second)
	defer cancelPoll()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var exchange authn.DeviceExchange
	for {
		select {
		case <-pollContext.Done():
			return errors.New("device authorization expired; run pact login again")
		case <-ticker.C:
			exchange, err = pactclient.ExchangeDeviceAuthorization(pollContext, normalizedServer, authorization.DeviceCode)
			if err != nil {
				return fmt.Errorf("complete device login: %w", err)
			}
			if exchange.Status == "pending" {
				continue
			}
			if exchange.Status != "authorized" || exchange.DeviceCredential == "" {
				return fmt.Errorf("unexpected device authorization status %q", exchange.Status)
			}
			goto authorized
		}
	}

authorized:
	client, err := pactclient.New(normalizedServer, exchange.DeviceCredential)
	if err != nil {
		return err
	}
	verifyContext, cancelVerify := context.WithTimeout(context.Background(), 15*time.Second)
	principal, err := client.Me(verifyContext)
	cancelVerify()
	if err != nil {
		return fmt.Errorf("verify device login with %s: %w", normalizedServer, err)
	}
	path, err := userconfig.SaveAuthorizedProfile(normalizedServer, exchange.DeviceCredential, userconfig.AuthorizedMetadata{
		ProfileLabel: strings.TrimSpace(*profileName), PrincipalID: principal.ID, PrincipalLabel: principal.DisplayName,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Logged in to %s\n", normalizedServer)
	fmt.Fprintf(stdout, "  identity            %s (%s)\n", principal.DisplayName, principal.OrganizationRole)
	fmt.Fprintf(stdout, "  device              %s\n", name)
	fmt.Fprintf(stdout, "  user configuration  %s\n", path)
	fmt.Fprintln(stdout, "This computer received its own revocable credential; no password was sent to the CLI.")
	return nil
}

func resolveServerURL(serverURL, reference string) (string, error) {
	base, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("parse Pact Server URL: %w", err)
	}
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return "", fmt.Errorf("parse device verification URL: %w", err)
	}
	resolved := base.ResolveReference(parsed)
	if (resolved.Scheme != "http" && resolved.Scheme != "https") || resolved.Host == "" || resolved.User != nil {
		return "", errors.New("Pact Server returned an invalid device verification URL")
	}
	return resolved.String(), nil
}

func openBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", address)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("open %s: %w", address, err)
	}
	return nil
}

func runInvite(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "create" {
		return errors.New("expected pact invite create")
	}
	flags := flag.NewFlagSet("pact invite create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	email := flags.String("email", "", "email address of the collaborator")
	role := flags.String("role", "contributor", "project role: owner, maintainer, contributor, or viewer")
	expires := flags.Duration("expires", 24*time.Hour, "invitation lifetime between 1h and 168h")
	projectPath := flags.String("path", ".", "path inside the connected Pact project")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || strings.TrimSpace(*email) == "" {
		return errors.New("pact invite create requires --email and accepts no positional arguments")
	}
	if *expires < time.Hour || *expires > 7*24*time.Hour || *expires%time.Hour != 0 {
		return errors.New("--expires must be a whole number of hours between 1h and 168h")
	}
	binding, err := localproject.LoadBinding(*projectPath)
	if err != nil {
		return err
	}
	login, err := loginForServer(binding.ServerURL)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	created, err := client.CreateInvitation(
		ctx, binding.ProjectID, strings.TrimSpace(*email), strings.ToLower(strings.TrimSpace(*role)), int(expires.Hours()),
	)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Invitation created for %s as %s; expires %s\n", created.Invitation.Email, created.Invitation.Role, created.Invitation.ExpiresAt.Format(time.RFC3339))
	fmt.Fprintln(stdout, "Send this private, one-time registration URL:")
	fmt.Fprintf(stdout, "%s/admin/#invite=%s\n", strings.TrimRight(login.ServerURL, "/"), url.QueryEscape(created.Secret))
	return nil
}

func runJoin(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact join", flag.ContinueOnError)
	flags.SetOutput(stderr)
	serverURL := flags.String("server", "", "Pact Server URL")
	inviteStdin := flags.Bool("invite-stdin", false, "read the one-time invitation secret from standard input")
	noBrowser := flags.Bool("no-browser", false, "print the registration URL without opening it")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || strings.TrimSpace(*serverURL) == "" || !*inviteStdin {
		return errors.New("pact join requires --server and --invite-stdin")
	}
	secret, err := readSecret(stdin, "invitation secret")
	if err != nil {
		return err
	}
	normalizedServer, err := userconfig.NormalizeServerURL(*serverURL)
	if err != nil {
		return err
	}
	registrationURL := normalizedServer + "/admin/#invite=" + url.QueryEscape(secret)
	fmt.Fprintf(stdout, "Create your Pact account in the browser:\n  %s\n", registrationURL)
	if !*noBrowser {
		if err := openBrowser(registrationURL); err != nil {
			fmt.Fprintf(stderr, "Could not open the browser automatically: %v\n", err)
		}
	}
	fmt.Fprintln(stdout, "After registration, run pact login --server "+normalizedServer+" to authorize this computer.")
	return nil
}

func runWhoAmI(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact whoami", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("pact whoami accepts no arguments")
	}
	login, err := loginForProjectOrActive(".", "")
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	principal, err := client.Me(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s\n", principal.DisplayName)
	fmt.Fprintf(stdout, "  principal ID       %s\n", principal.ID)
	fmt.Fprintf(stdout, "  organization role  %s\n", principal.OrganizationRole)
	fmt.Fprintf(stdout, "  Pact Server         %s\n", login.ServerURL)
	return nil
}

type folderStatusOutput struct {
	Root       string                  `json:"root"`
	Server     folderStatusServer      `json:"server"`
	Workspace  *folderStatusEntity     `json:"workspace"`
	Project    folderStatusEntity      `json:"project"`
	Repository *folderStatusRepository `json:"repository"`
}

type folderStatusServer struct {
	ProfileID string `json:"profile_id"`
	Label     string `json:"label"`
	URL       string `json:"url"`
}

type folderStatusEntity struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type folderStatusRepository struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RemoteURL string `json:"remote_url,omitempty"`
	Primary   bool   `json:"primary"`
}

func runStatus(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	projectPath := flags.String("path", ".", "path inside the connected Pact project")
	jsonOutput := flags.Bool("json", false, "print machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("pact status accepts no positional arguments")
	}
	binding, err := localproject.LoadBinding(*projectPath)
	if err != nil {
		return err
	}
	profile, err := userconfig.FindProfileByURL(binding.ServerURL)
	if err != nil {
		return fmt.Errorf(
			"PACT Server %s required by this project is not authorized on this computer; run pact login --server %s: %w",
			binding.ServerURL, binding.ServerURL, err,
		)
	}
	login, err := loginForServer(binding.ServerURL)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	projectList, err := client.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("load project status from %s: %w", binding.ServerURL, err)
	}
	var project *projects.Project
	for index := range projectList {
		if projectList[index].ID == binding.ProjectID {
			project = &projectList[index]
			break
		}
	}
	if project == nil {
		return fmt.Errorf("project %s bound to this folder is not visible on %s", binding.ProjectID, binding.ServerURL)
	}
	status := folderStatusOutput{
		Root:    binding.Root,
		Server:  folderStatusServer{ProfileID: profile.ID, Label: profile.Label, URL: profile.ServerURL},
		Project: folderStatusEntity{ID: project.ID, Slug: project.Slug, Name: project.Name},
	}
	descriptor, err := localproject.Describe(binding.Root)
	if err != nil {
		return err
	}
	target, err := resolveRemoteBinding(
		ctx, client, binding.ProjectID, descriptor.RemoteURL, binding.WorkspaceID, binding.RepositoryID,
	)
	if err != nil {
		return err
	}
	migrated, err := localproject.Bind(binding.Root, localproject.BindOptions{
		ServerURL: binding.ServerURL, WorkspaceID: target.WorkspaceID,
		RepositoryID: target.RepositoryID, ProjectID: binding.ProjectID,
	})
	if err != nil {
		return err
	}
	status.Root = migrated.Root
	status.Workspace = &folderStatusEntity{ID: target.WorkspaceID, Slug: target.WorkspaceSlug, Name: target.WorkspaceName}
	status.Repository = &folderStatusRepository{
		ID: target.RepositoryID, Name: target.RepositoryName,
		RemoteURL: target.RepositoryRemote, Primary: target.RepositoryPrimary,
	}
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(status)
	}
	fmt.Fprintf(stdout, "Folder      %s\n", status.Root)
	fmt.Fprintf(stdout, "PACT Server %s (%s)\n", status.Server.Label, status.Server.URL)
	if status.Workspace == nil {
		fmt.Fprintln(stdout, "Workspace   not attached")
	} else {
		fmt.Fprintf(stdout, "Workspace   %s (%s)\n", status.Workspace.Slug, status.Workspace.ID)
	}
	fmt.Fprintf(stdout, "Project     %s (%s)\n", status.Project.Slug, status.Project.ID)
	if status.Repository == nil {
		fmt.Fprintln(stdout, "Repository  not resolved")
	} else {
		fmt.Fprintf(stdout, "Repository  %s (%s)\n", status.Repository.Name, status.Repository.ID)
		if status.Repository.RemoteURL != "" {
			fmt.Fprintf(stdout, "Remote      %s\n", status.Repository.RemoteURL)
		}
	}
	return nil
}

func runLogout(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact logout", flag.ContinueOnError)
	flags.SetOutput(stderr)
	server := flags.String("server", "", "PACT Server profile ID or URL")
	localOnly := flags.Bool("local-only", false, "remove local login without revoking the device on the server")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("pact logout accepts no arguments")
	}
	identifier := strings.TrimSpace(*server)
	if identifier == "" {
		profile, err := profileForProjectOrActive(".")
		if err != nil {
			return err
		}
		identifier = profile.ID
	}
	profile, err := removeServerProfile(identifier, *localOnly)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Logged out of %s (%s) on this computer.\n", profile.Label, profile.ServerURL)
	return nil
}

func removeServerProfile(identifier string, localOnly bool) (serverprofile.Profile, error) {
	profile, err := userconfig.FindProfile(identifier)
	if err != nil {
		return serverprofile.Profile{}, err
	}
	if !localOnly {
		authorized, err := userconfig.AuthorizedProfile(profile.ID)
		if err != nil {
			return serverprofile.Profile{}, err
		}
		client, err := pactclient.New(authorized.ServerURL, authorized.DeviceCredential)
		if err != nil {
			return serverprofile.Profile{}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err = client.RevokeCurrentDevice(ctx)
		cancel()
		if err != nil {
			return serverprofile.Profile{}, fmt.Errorf(
				"revoke device from %s: %w (use --local-only only if the server is unavailable)",
				profile.ServerURL, err,
			)
		}
	}
	if err := userconfig.RemoveProfile(profile.ID); err != nil {
		return serverprofile.Profile{}, err
	}
	return profile, nil
}

func readSecret(reader io.Reader, name string) (string, error) {
	content, err := io.ReadAll(io.LimitReader(reader, 4097))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	if len(content) > 4096 {
		return "", fmt.Errorf("%s is too large", name)
	}
	secret := strings.TrimSpace(string(content))
	if secret == "" {
		return "", fmt.Errorf("%s is empty", name)
	}
	return secret, nil
}

func runInit(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	serverURL := flags.String("server", "", "Pact Server URL (defaults to the logged-in server)")
	name := flags.String("name", "", "project name (defaults to the repository directory)")
	workspaceID := flags.String("workspace", "", "workspace UUID when the project destination is ambiguous")
	repositoryID := flags.String("repository", "", "repository UUID when the Git remote is ambiguous")
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

	login, err := loginForProjectOrActive(projectPath, *serverURL)
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
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	project, created, err := resolveProject(ctx, client, descriptor, "", true)
	if err != nil {
		return err
	}
	target, err := resolveRemoteBinding(
		ctx, client, project.ID, descriptor.RemoteURL, *workspaceID, *repositoryID,
	)
	if err != nil {
		return err
	}
	if _, err := localproject.Bind(result.Root, localproject.BindOptions{
		ServerURL: login.ServerURL, WorkspaceID: target.WorkspaceID,
		RepositoryID: target.RepositoryID, ProjectID: project.ID,
	}); err != nil {
		return err
	}

	state := "Connected"
	if created {
		state = "Created and connected"
	}
	fmt.Fprintf(stdout, "%s Pact project in %s\n", state, result.Root)
	printProjectBinding(stdout, result.ManifestPath, result.LocalDirectory, login.ServerURL, project, target)
	return nil
}

func runConnect(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pact connect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	serverURL := flags.String("server", "", "Pact Server URL (defaults to the logged-in server)")
	projectReference := flags.String("project", "", "existing remote project slug or ID")
	workspaceID := flags.String("workspace", "", "workspace UUID when the project destination is ambiguous")
	repositoryID := flags.String("repository", "", "repository UUID when the Git remote is ambiguous")
	rebind := flags.Bool("rebind", false, "explicitly replace an existing folder binding")
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
	login, err := loginForProjectOrActive(projectPath, *serverURL)
	if err != nil {
		return err
	}
	result, err := localproject.Init(localproject.InitOptions{
		StartPath: projectPath, ServerURL: login.ServerURL, AllowServerChange: *rebind,
	})
	if err != nil {
		return err
	}
	descriptor, err := localproject.Describe(result.Root)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.DeviceCredential)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	project, _, err := resolveProject(ctx, client, descriptor, *projectReference, false)
	if err != nil {
		return err
	}
	target, err := resolveRemoteBinding(
		ctx, client, project.ID, descriptor.RemoteURL, *workspaceID, *repositoryID,
	)
	if err != nil {
		return err
	}
	if _, err := localproject.Bind(result.Root, localproject.BindOptions{
		ServerURL: login.ServerURL, WorkspaceID: target.WorkspaceID,
		RepositoryID: target.RepositoryID, ProjectID: project.ID, Rebind: *rebind,
	}); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Connected existing Pact project in %s\n", result.Root)
	printProjectBinding(stdout, result.ManifestPath, result.LocalDirectory, login.ServerURL, project, target)
	return nil
}

func loginForServer(requestedServer string) (userconfig.Config, error) {
	if strings.TrimSpace(requestedServer) == "" {
		return userconfig.Load()
	}
	normalized, err := userconfig.NormalizeServerURL(requestedServer)
	if err != nil {
		return userconfig.Config{}, err
	}
	authorized, err := userconfig.AuthorizedForServer(normalized)
	if err != nil {
		return userconfig.Config{}, fmt.Errorf(
			"PACT Server %s is not authorized on this computer; run pact login --server %s: %w",
			normalized, normalized, err,
		)
	}
	return userconfig.Config{
		SchemaVersion: serverprofile.SchemaVersion, ServerURL: authorized.ServerURL,
		DeviceCredential: authorized.DeviceCredential,
	}, nil
}

func loginForProjectOrActive(projectPath, requestedServer string) (userconfig.Config, error) {
	if strings.TrimSpace(requestedServer) != "" {
		return loginForServer(requestedServer)
	}
	binding, found, err := localproject.FindBinding(projectPath)
	if err != nil {
		return userconfig.Config{}, err
	}
	if found {
		return loginForServer(binding.ServerURL)
	}
	return loginForServer("")
}

func profileForProjectOrActive(projectPath string) (serverprofile.Profile, error) {
	binding, found, err := localproject.FindBinding(projectPath)
	if err != nil {
		return serverprofile.Profile{}, err
	}
	if found {
		profile, err := userconfig.FindProfileByURL(binding.ServerURL)
		if err != nil {
			return serverprofile.Profile{}, fmt.Errorf(
				"PACT Server %s required by this project is not authorized on this computer; run pact login --server %s: %w",
				binding.ServerURL, binding.ServerURL, err,
			)
		}
		return profile, nil
	}
	profile, err := userconfig.ActiveProfile()
	if errors.Is(err, serverprofile.ErrNoActiveProfile) {
		return serverprofile.Profile{}, errors.New("not logged in; run pact login --server <url>")
	}
	return profile, err
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
	target remoteBindingTarget,
) {
	fmt.Fprintf(stdout, "  shared manifest  %s\n", manifestPath)
	fmt.Fprintf(stdout, "  local runtime    %s\n", localDirectory)
	fmt.Fprintf(stdout, "  Pact Server      %s\n", serverURL)
	fmt.Fprintf(stdout, "  remote project   %s (%s)\n", project.Slug, project.ID)
	fmt.Fprintf(stdout, "  workspace        %s (%s)\n", target.WorkspaceSlug, target.WorkspaceID)
	fmt.Fprintf(stdout, "  repository       %s (%s)\n", target.RepositoryName, target.RepositoryID)
	fmt.Fprintln(stdout, "No database credentials, passwords, or device credentials were written to the project.")
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  pact login --server URL [--name NAME] [--device-name NAME] [--no-browser]")
	fmt.Fprintln(writer, "  pact servers list [--json]")
	fmt.Fprintln(writer, "  pact servers use PROFILE_OR_URL")
	fmt.Fprintln(writer, "  pact servers remove [--local-only] PROFILE_OR_URL")
	fmt.Fprintln(writer, "  pact init [--server URL] [--name NAME] [--workspace UUID] [--repository UUID] [PATH]")
	fmt.Fprintln(writer, "  pact connect [--server URL] [--project SLUG_OR_ID] [--workspace UUID] [--repository UUID] [--rebind] [PATH]")
	fmt.Fprintln(writer, "  pact status [--path PATH] [--json]")
	fmt.Fprintln(writer, "  pact workspace list")
	fmt.Fprintln(writer, "  pact workspace show SLUG_OR_ID")
	fmt.Fprintln(writer, "  pact workspace create --name NAME --slug SLUG [--project UUID ...]")
	fmt.Fprintln(writer, "  pact workspace add-project WORKSPACE_ID PROJECT_ID")
	fmt.Fprintln(writer, "  pact repository list [--path PATH]")
	fmt.Fprintln(writer, "  pact repository status [--repository UUID] [--path PATH]")
	fmt.Fprintln(writer, "  pact repository sync [--repository UUID] [--path PATH]")
	fmt.Fprintln(writer, "  pact enable codex [--path PATH]")
	fmt.Fprintln(writer, "  pact enable claude [--path PATH]")
	fmt.Fprintln(writer, "  pact invite create --email EMAIL [--role ROLE] [--path PATH]")
	fmt.Fprintln(writer, "  pact join --server URL --invite-stdin [--no-browser]")
	fmt.Fprintln(writer, "  pact whoami")
	fmt.Fprintln(writer, "  pact logout [--server PROFILE_OR_URL] [--local-only]")
	fmt.Fprintln(writer, "  pact agent run --client TYPE [--name NAME] [--path PATH] [-- COMMAND ...]")
	fmt.Fprintln(writer, "  pact node run [--path PATH] [--interval 2s] [--once]")
	fmt.Fprintln(writer, "  pact server install [--port 8080] [--image IMAGE]")
	fmt.Fprintln(writer, "  pact server start|stop|status|logs")
	fmt.Fprintln(writer, "  pact server backup [--output FILE]")
	fmt.Fprintln(writer, "  pact server restore --input FILE --confirm")
	fmt.Fprintln(writer, "  pact server upgrade [--image IMAGE]")
	fmt.Fprintln(writer, "  pact server uninstall [--remove-data --confirm]")
	fmt.Fprintln(writer, "  pact mcp serve --client TYPE [--name NAME] [--path PATH]")
	fmt.Fprintln(writer, "  pact version")
}
