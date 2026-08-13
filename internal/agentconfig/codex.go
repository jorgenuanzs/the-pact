package agentconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	managedStart = "# >>> PACT managed Codex MCP"
	managedEnd   = "# <<< PACT managed Codex MCP"
	maxConfig    = 1 << 20
)

var pactTablePattern = regexp.MustCompile(`(?m)^[\t ]*\[[\t ]*mcp_servers[\t ]*\.[\t ]*(?:pact|"pact"|'pact')[\t ]*\][\t ]*(?:#.*)?$`)

type CodexOptions struct {
	ProjectRoot string
	PactCommand string
}

type CodexResult struct {
	ConfigPath string
	Created    bool
	Changed    bool
	Excluded   bool
}

// EnableCodex installs a project-scoped MCP definition. The generated file is
// local to the checkout because it contains machine-specific absolute paths.
func EnableCodex(options CodexOptions) (CodexResult, error) {
	if strings.TrimSpace(options.ProjectRoot) == "" {
		return CodexResult{}, errors.New("project root is required")
	}
	if strings.TrimSpace(options.PactCommand) == "" {
		return CodexResult{}, errors.New("Pact executable is required")
	}
	root, err := filepath.Abs(strings.TrimSpace(options.ProjectRoot))
	if err != nil {
		return CodexResult{}, fmt.Errorf("resolve project root: %w", err)
	}
	command, err := filepath.Abs(strings.TrimSpace(options.PactCommand))
	if err != nil {
		return CodexResult{}, fmt.Errorf("resolve Pact executable: %w", err)
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		if statErr != nil {
			return CodexResult{}, fmt.Errorf("inspect project root: %w", statErr)
		}
		return CodexResult{}, errors.New("project root is not a directory")
	}
	if info, statErr := os.Stat(command); statErr != nil || !info.Mode().IsRegular() {
		if statErr != nil {
			return CodexResult{}, fmt.Errorf("inspect Pact executable: %w", statErr)
		}
		return CodexResult{}, errors.New("Pact executable is not a regular file")
	}

	directory := filepath.Join(root, ".codex")
	if err := ensureRealDirectory(directory); err != nil {
		return CodexResult{}, err
	}
	configPath := filepath.Join(directory, "config.toml")
	existing, mode, exists, err := readRegularFile(configPath)
	if err != nil {
		return CodexResult{}, err
	}
	updated, err := updateCodexConfig(existing, command, root)
	if err != nil {
		return CodexResult{}, err
	}
	result := CodexResult{
		ConfigPath: configPath,
		Created:    !exists,
		Changed:    !bytes.Equal(existing, updated),
	}
	if !result.Changed {
		return result, nil
	}
	if !exists {
		excluded, excludeErr := ensureLocalExclude(root, "/.codex/config.toml")
		if excludeErr != nil {
			return CodexResult{}, excludeErr
		}
		result.Excluded = excluded
		mode = 0o600
	}
	if err := writeAtomic(configPath, updated, mode); err != nil {
		return CodexResult{}, fmt.Errorf("write Codex project configuration: %w", err)
	}
	return result, nil
}

func ensureRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("create Codex project configuration directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Codex project configuration directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New(".codex exists but is not a real directory")
	}
	return nil
}

func readRegularFile(path string) ([]byte, os.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0o600, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("inspect Codex project configuration: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, errors.New(".codex/config.toml exists but is not a regular file")
	}
	if info.Size() > maxConfig {
		return nil, 0, false, errors.New(".codex/config.toml is too large")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, false, fmt.Errorf("open Codex project configuration: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxConfig+1))
	if err != nil {
		return nil, 0, false, fmt.Errorf("read Codex project configuration: %w", err)
	}
	if len(content) > maxConfig {
		return nil, 0, false, errors.New(".codex/config.toml is too large")
	}
	return content, info.Mode().Perm(), true, nil
}

func updateCodexConfig(existing []byte, command, root string) ([]byte, error) {
	text := string(existing)
	withoutManaged, start, end, found, err := managedRange(text)
	if err != nil {
		return nil, err
	}
	if pactTablePattern.MatchString(withoutManaged) {
		return nil, errors.New(".codex/config.toml already defines mcp_servers.pact outside the PACT-managed block")
	}
	block := renderManagedBlock(command, root)
	if found {
		return []byte(text[:start] + block + text[end:]), nil
	}
	var builder strings.Builder
	builder.WriteString(text)
	if builder.Len() > 0 && !strings.HasSuffix(text, "\n") {
		builder.WriteByte('\n')
	}
	if builder.Len() > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString(block)
	return []byte(builder.String()), nil
}

func managedRange(text string) (string, int, int, bool, error) {
	startCount := strings.Count(text, managedStart)
	endCount := strings.Count(text, managedEnd)
	if startCount == 0 && endCount == 0 {
		return text, 0, 0, false, nil
	}
	if startCount != 1 || endCount != 1 {
		return "", 0, 0, false, errors.New(".codex/config.toml contains a malformed PACT-managed block")
	}
	start := strings.Index(text, managedStart)
	endMarker := strings.Index(text, managedEnd)
	if endMarker < start {
		return "", 0, 0, false, errors.New(".codex/config.toml contains a malformed PACT-managed block")
	}
	end := endMarker + len(managedEnd)
	if end < len(text) && text[end] == '\r' {
		end++
	}
	if end < len(text) && text[end] == '\n' {
		end++
	}
	return text[:start] + text[end:], start, end, true, nil
}

func renderManagedBlock(command, root string) string {
	return managedStart + "\n" +
		"# Machine-local configuration generated by `pact enable codex`.\n" +
		"[mcp_servers.pact]\n" +
		"command = " + strconv.Quote(command) + "\n" +
		"args = [\"mcp\", \"serve\", \"--client\", \"codex\", \"--name\", \"Codex\", \"--path\", \".\"]\n" +
		"cwd = " + strconv.Quote(root) + "\n" +
		managedEnd + "\n"
}

func ensureLocalExclude(root, entry string) (bool, error) {
	command := exec.Command("git", "-C", root, "rev-parse", "--git-path", "info/exclude")
	output, err := command.Output()
	if err != nil {
		return false, fmt.Errorf("resolve Git local exclude file: %w", err)
	}
	excludePath := strings.TrimSpace(string(output))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(root, excludePath)
	}
	content, mode, exists, err := readRegularFile(excludePath)
	if err != nil {
		return false, fmt.Errorf("read Git local exclude file: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == entry {
			return false, nil
		}
	}
	updated := append([]byte(nil), content...)
	if len(updated) > 0 && updated[len(updated)-1] != '\n' {
		updated = append(updated, '\n')
	}
	updated = append(updated, []byte("\n# Pact machine-local agent configuration\n"+entry+"\n")...)
	if !exists {
		mode = 0o600
	}
	if err := writeAtomic(excludePath, updated, mode); err != nil {
		return false, fmt.Errorf("write Git local exclude file: %w", err)
	}
	return true, nil
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pact-agent-config-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
