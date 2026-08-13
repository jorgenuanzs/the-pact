package localproject

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

var scpRemotePattern = regexp.MustCompile(`^[^/@:]+@([^/:]+):(.+)$`)

type Descriptor struct {
	Root              string
	Name              string
	Slug              string
	RemoteURL         string
	DefaultBranch     string
	CanonicalRevision string
	ObjectFormat      string
}

type manifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Repositories []struct {
			Name         string `yaml:"name"`
			CanonicalRef string `yaml:"canonicalRef"`
			Path         string `yaml:"path"`
		} `yaml:"repositories"`
	} `yaml:"spec"`
}

func Describe(startPath string) (Descriptor, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return Descriptor{}, err
	}
	projectManifest, err := readManifest(filepath.Join(root, manifestName))
	if err != nil {
		return Descriptor{}, err
	}

	remote, err := gitOutput(root, "config", "--get", "remote.origin.url")
	if err != nil || strings.TrimSpace(remote) == "" {
		return Descriptor{}, errors.New("Git remote origin is required before connecting the project to Pact")
	}
	remote, err = NormalizeGitRemote(remote)
	if err != nil {
		return Descriptor{}, err
	}
	revision, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return Descriptor{}, errors.New("the Git repository must contain at least one commit before connecting to Pact")
	}
	objectFormat, err := gitOutput(root, "rev-parse", "--show-object-format")
	if err != nil || (objectFormat != "sha1" && objectFormat != "sha256") {
		objectFormat = "sha1"
	}

	defaultBranch := "main"
	for _, repository := range projectManifest.Spec.Repositories {
		if repository.Path != "." && repository.Name != "primary" {
			continue
		}
		if branch := strings.TrimPrefix(repository.CanonicalRef, "refs/heads/"); branch != repository.CanonicalRef && branch != "" {
			defaultBranch = branch
		}
		break
	}

	name := strings.TrimSpace(projectManifest.Metadata.Name)
	return Descriptor{
		Root:              root,
		Name:              name,
		Slug:              Slugify(name),
		RemoteURL:         remote,
		DefaultBranch:     defaultBranch,
		CanonicalRevision: strings.ToLower(strings.TrimSpace(revision)),
		ObjectFormat:      objectFormat,
	}, nil
}

func HasManifest(startPath string) (bool, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(filepath.Join(root, manifestName))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", manifestName, err)
	}
	return true, nil
}

func Bind(startPath, serverURL, projectID string) error {
	root, err := FindRoot(startPath)
	if err != nil {
		return err
	}
	if !validUUID(strings.TrimSpace(projectID)) {
		return errors.New("remote project ID must be a UUID")
	}
	configPath := filepath.Join(root, localDirectory, localConfigName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read local Pact configuration: %w", err)
	}
	var config localConfig
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("decode local Pact configuration: %w", err)
	}
	configuredServer, err := normalizeServerURL(config.ServerURL)
	if err != nil {
		return err
	}
	requestedServer, err := normalizeServerURL(serverURL)
	if err != nil {
		return err
	}
	if configuredServer != requestedServer {
		return fmt.Errorf("project is configured for %s, not %s", configuredServer, requestedServer)
	}
	if config.ProjectID != "" && config.ProjectID != projectID {
		return fmt.Errorf("this checkout is already connected to project %s", config.ProjectID)
	}
	config.ProjectID = projectID
	payload, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode local Pact binding: %w", err)
	}
	payload = append(payload, '\n')
	if err := writeAtomic(configPath, payload, 0o600); err != nil {
		return fmt.Errorf("write local Pact binding: %w", err)
	}
	return nil
}

func NormalizeGitRemote(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if matches := scpRemotePattern.FindStringSubmatch(raw); len(matches) == 3 {
		raw = "https://" + matches[1] + "/" + matches[2]
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse Git remote origin: %w", err)
	}
	if parsed.Host == "" {
		return "", errors.New("Git remote origin must be a network URL")
	}
	switch parsed.Scheme {
	case "https", "http", "ssh", "git":
	default:
		return "", fmt.Errorf("unsupported Git remote scheme %q", parsed.Scheme)
	}
	if parsed.Scheme == "ssh" || parsed.Scheme == "git" {
		parsed.Scheme = "https"
	}
	parsed.User = nil
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(strings.TrimRight(parsed.Path, "/"), ".git")
	if parsed.Path == "" || parsed.Path == "/" {
		return "", errors.New("Git remote origin must identify a repository")
	}
	return parsed.String(), nil
}

func Slugify(value string) string {
	var builder strings.Builder
	lastHyphen := false
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
			lastHyphen = false
			continue
		}
		if unicode.IsSpace(character) || character == '-' || character == '_' || unicode.IsPunct(character) {
			if builder.Len() > 0 && !lastHyphen {
				builder.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.TrimRight(builder.String(), "-")
}

func readManifest(path string) (manifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, fmt.Errorf("read %s: %w", manifestName, err)
	}
	var result manifest
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(false)
	if err := decoder.Decode(&result); err != nil {
		return manifest{}, fmt.Errorf("decode %s: %w", manifestName, err)
	}
	if !strings.HasPrefix(result.APIVersion, "pact.dev/") || result.Kind != "Project" {
		return manifest{}, errors.New("pact.yaml is not a Pact project manifest")
	}
	if strings.TrimSpace(result.Metadata.Name) == "" {
		return manifest{}, errors.New("pact.yaml metadata.name is required")
	}
	return result, nil
}

func gitOutput(root string, arguments ...string) (string, error) {
	commandArguments := append([]string{"-C", root}, arguments...)
	command := exec.Command("git", commandArguments...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !((character >= '0' && character <= '9') ||
				(character >= 'a' && character <= 'f') ||
				(character >= 'A' && character <= 'F')) {
				return false
			}
		}
	}
	return true
}
