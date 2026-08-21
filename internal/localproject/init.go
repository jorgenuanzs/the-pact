package localproject

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultServerURL = "http://127.0.0.1:8080"
	manifestName     = "pact.yaml"
	localDirectory   = ".pact"
	localConfigName  = "config.json"
)

type InitOptions struct {
	StartPath         string
	Name              string
	ServerURL         string
	AllowServerChange bool
}

type InitResult struct {
	Root               string
	ManifestPath       string
	LocalDirectory     string
	LocalConfigPath    string
	ServerURL          string
	ManifestCreated    bool
	LocalConfigCreated bool
}

func Init(options InitOptions) (InitResult, error) {
	if strings.TrimSpace(options.ServerURL) != "" {
		if _, err := normalizeServerURL(options.ServerURL); err != nil {
			return InitResult{}, err
		}
	}

	root, err := FindRoot(options.StartPath)
	if err != nil {
		return InitResult{}, err
	}

	manifestPath := filepath.Join(root, manifestName)
	localPath := filepath.Join(root, localDirectory)
	configPath := filepath.Join(localPath, localConfigName)

	if err := validateExistingManifest(manifestPath); err != nil {
		return InitResult{}, err
	}
	if err := validateWritablePath(filepath.Join(root, ".gitignore")); err != nil {
		return InitResult{}, err
	}
	if err := ensurePrivateDirectory(localPath); err != nil {
		return InitResult{}, err
	}

	configuredServer, configCreated, err := ensureLocalConfig(configPath, options.ServerURL, options.AllowServerChange)
	if err != nil {
		return InitResult{}, err
	}

	projectName := strings.TrimSpace(options.Name)
	if projectName == "" {
		projectName = filepath.Base(root)
	}
	manifestCreated, err := ensureManifest(
		manifestPath,
		projectName,
		detectCanonicalRef(root),
	)
	if err != nil {
		return InitResult{}, err
	}
	if err := ensureGitIgnore(filepath.Join(root, ".gitignore")); err != nil {
		return InitResult{}, err
	}

	return InitResult{
		Root:               root,
		ManifestPath:       manifestPath,
		LocalDirectory:     localPath,
		LocalConfigPath:    configPath,
		ServerURL:          configuredServer,
		ManifestCreated:    manifestCreated,
		LocalConfigCreated: configCreated,
	}, nil
}

func FindRoot(startPath string) (string, error) {
	if strings.TrimSpace(startPath) == "" {
		startPath = "."
	}
	absolute, err := filepath.Abs(startPath)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect project path: %w", err)
	}
	if !info.IsDir() {
		absolute = filepath.Dir(absolute)
	}

	for candidate := filepath.Clean(absolute); ; candidate = filepath.Dir(candidate) {
		if _, err := os.Lstat(filepath.Join(candidate, ".git")); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect Git metadata: %w", err)
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}

	return "", fmt.Errorf("no Git repository found from %s", absolute)
}

func validateExistingManifest(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", manifestName, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s exists but is not a regular file", manifestName)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestName, err)
	}
	text := string(content)
	if !strings.Contains(text, "apiVersion: pact.dev/") || !strings.Contains(text, "kind: Project") {
		return fmt.Errorf("%s exists but is not a Pact project manifest", manifestName)
	}
	return nil
}

func validateWritablePath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", filepath.Base(path), err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s exists but is not a regular file", filepath.Base(path))
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", localDirectory, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", localDirectory, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s exists but is not a real directory", localDirectory)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure %s: %w", localDirectory, err)
	}
	return nil
}

func ensureLocalConfig(path, requestedServer string, allowServerChange bool) (string, bool, error) {
	release, err := acquireBindingLock(path)
	if err != nil {
		return "", false, err
	}
	defer release()

	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("%s exists but is not a regular file", path)
		}
		existing, readErr := readLocalConfig(path)
		if readErr != nil {
			return "", false, readErr
		}
		if existing.SchemaVersion != 1 && existing.SchemaVersion != LocalBindingSchemaVersion {
			return "", false, fmt.Errorf("unsupported local Pact schema version %d", existing.SchemaVersion)
		}
		if existing.SchemaVersion == LocalBindingSchemaVersion {
			if _, shapeErr := completeV2Config(existing); shapeErr != nil {
				return "", false, shapeErr
			}
		}
		normalized, normalizeErr := normalizeServerURL(existing.ServerURL)
		if normalizeErr != nil {
			return "", false, fmt.Errorf("invalid server URL in local Pact configuration: %w", normalizeErr)
		}
		if strings.TrimSpace(requestedServer) != "" {
			requested, requestErr := normalizeServerURL(requestedServer)
			if requestErr != nil {
				return "", false, requestErr
			}
			if requested != normalized {
				if !allowServerChange {
					return "", false, fmt.Errorf("project is already linked locally to %s", normalized)
				}
				normalized = requested
			}
		}
		if chmodErr := os.Chmod(path, 0o600); chmodErr != nil {
			return "", false, fmt.Errorf("secure local Pact configuration: %w", chmodErr)
		}
		return normalized, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("inspect local Pact configuration: %w", err)
	}

	server := requestedServer
	if strings.TrimSpace(server) == "" {
		server = DefaultServerURL
	}
	normalized, err := normalizeServerURL(server)
	if err != nil {
		return "", false, err
	}
	if err := writeLocalConfig(path, localConfig{
		SchemaVersion: LocalBindingSchemaVersion,
		ServerURL:     normalized,
	}); err != nil {
		return "", false, fmt.Errorf("write local Pact configuration: %w", err)
	}
	return normalized, true, nil
}

func normalizeServerURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid Pact Server URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("Pact Server URL must be an absolute http or https URL")
	}
	if parsed.User != nil {
		return "", errors.New("Pact Server URL must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Pact Server URL must not contain a query or fragment")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return "", errors.New("remote Pact Server URLs must use https")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func ensureManifest(path, projectName, canonicalRef string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect %s: %w", manifestName, err)
	}

	manifest := fmt.Sprintf(`apiVersion: pact.dev/v1alpha1
kind: Project

metadata:
  name: %s

spec:
  governanceMode: observer

  repositories:
    - name: primary
      provider: generic-git
      canonicalRef: %s
      path: .
`, strconv.Quote(projectName), strconv.Quote(canonicalRef))
	if err := writeExclusive(path, []byte(manifest), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", manifestName, err)
	}
	return true, nil
}

func ensureGitIgnore(path string) error {
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read .gitignore: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		switch strings.TrimSpace(line) {
		case ".pact/", "/.pact/":
			return nil
		}
	}

	prefix := content
	if len(prefix) > 0 && prefix[len(prefix)-1] != '\n' {
		prefix = append(prefix, '\n')
	}
	if len(prefix) > 0 {
		prefix = append(prefix, '\n')
	}
	prefix = append(prefix, []byte("# Pact local runtime\n.pact/\n")...)

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := writeAtomic(path, prefix, mode); err != nil {
		return fmt.Errorf("update .gitignore: %w", err)
	}
	return nil
}

func writeExclusive(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".pact-gitignore-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func detectCanonicalRef(root string) string {
	gitDir, err := resolveGitDirectory(root)
	if err != nil {
		return "refs/heads/main"
	}
	content, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "refs/heads/main"
	}
	value := strings.TrimSpace(string(content))
	if strings.HasPrefix(value, "ref: refs/heads/") {
		return strings.TrimPrefix(value, "ref: ")
	}
	return "refs/heads/main"
}

func resolveGitDirectory(root string) (string, error) {
	metadataPath := filepath.Join(root, ".git")
	info, err := os.Stat(metadataPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return metadataPath, nil
	}
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", errors.New("invalid Git metadata file")
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	return filepath.Clean(gitDir), nil
}
