package localproject

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/jorgenuanzs/the-pact/internal/gitremote"
	"gopkg.in/yaml.v3"
)

type Descriptor struct {
	Root              string
	Name              string
	Slug              string
	RemoteURL         string
	DefaultBranch     string
	CanonicalRevision string
	ObjectFormat      string
}

// Checkout describes the Git facts PACT needs before a local manifest or
// binding exists. Desktop onboarding uses it to resolve a checkout against a
// selected PACT Server without requiring users to run the CLI first.
type Checkout struct {
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

func InspectCheckout(startPath string) (Checkout, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return Checkout{}, err
	}
	remote, err := gitOutput(root, "config", "--get", "remote.origin.url")
	if err != nil || strings.TrimSpace(remote) == "" {
		return Checkout{}, errors.New("el repositorio Git necesita un remote origin antes de conectarse a PACT")
	}
	remote, err = NormalizeGitRemote(remote)
	if err != nil {
		return Checkout{}, err
	}
	revision, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return Checkout{}, errors.New("el repositorio Git necesita al menos un commit antes de conectarse a PACT")
	}
	objectFormat, err := gitOutput(root, "rev-parse", "--show-object-format")
	if err != nil || (objectFormat != "sha1" && objectFormat != "sha256") {
		objectFormat = "sha1"
	}
	branch, err := gitOutput(root, "branch", "--show-current")
	if err != nil || strings.TrimSpace(branch) == "" {
		branch = "main"
	}
	name := filepath.Base(root)
	return Checkout{
		Root: root, Name: name, Slug: Slugify(name), RemoteURL: remote,
		DefaultBranch: branch, CanonicalRevision: strings.ToLower(strings.TrimSpace(revision)),
		ObjectFormat: objectFormat,
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

func NormalizeGitRemote(raw string) (string, error) {
	return gitremote.Normalize(raw)
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
