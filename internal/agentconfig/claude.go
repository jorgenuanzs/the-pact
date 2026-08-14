package agentconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const claudeManagedMarker = "pact.enable.claude/v1"

type ClaudeOptions struct {
	ProjectRoot string
	PactCommand string
}

type ClaudeResult struct {
	ConfigPath string
	Created    bool
	Changed    bool
	Excluded   bool
}

type claudeServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// EnableClaude installs a project-scoped stdio MCP definition in .mcp.json.
// A newly created file stays machine-local because the executable and project
// paths are absolute. Existing .mcp.json files keep their current Git policy.
func EnableClaude(options ClaudeOptions) (ClaudeResult, error) {
	if strings.TrimSpace(options.ProjectRoot) == "" {
		return ClaudeResult{}, errors.New("project root is required")
	}
	if strings.TrimSpace(options.PactCommand) == "" {
		return ClaudeResult{}, errors.New("Pact executable is required")
	}
	root, err := filepath.Abs(strings.TrimSpace(options.ProjectRoot))
	if err != nil {
		return ClaudeResult{}, fmt.Errorf("resolve project root: %w", err)
	}
	command, err := filepath.Abs(strings.TrimSpace(options.PactCommand))
	if err != nil {
		return ClaudeResult{}, fmt.Errorf("resolve Pact executable: %w", err)
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		if statErr != nil {
			return ClaudeResult{}, fmt.Errorf("inspect project root: %w", statErr)
		}
		return ClaudeResult{}, errors.New("project root is not a directory")
	}
	if info, statErr := os.Stat(command); statErr != nil || !info.Mode().IsRegular() {
		if statErr != nil {
			return ClaudeResult{}, fmt.Errorf("inspect Pact executable: %w", statErr)
		}
		return ClaudeResult{}, errors.New("Pact executable is not a regular file")
	}

	configPath := filepath.Join(root, ".mcp.json")
	existing, mode, exists, err := readClaudeConfig(configPath)
	if err != nil {
		return ClaudeResult{}, err
	}
	updated, err := updateClaudeConfig(existing, command, root)
	if err != nil {
		return ClaudeResult{}, err
	}
	result := ClaudeResult{
		ConfigPath: configPath,
		Created:    !exists,
		Changed:    !bytes.Equal(existing, updated),
	}
	if !result.Changed {
		return result, nil
	}
	if !exists {
		excluded, excludeErr := ensureLocalExclude(root, "/.mcp.json")
		if excludeErr != nil {
			return ClaudeResult{}, excludeErr
		}
		result.Excluded = excluded
		mode = 0o600
	}
	if err := writeAtomic(configPath, updated, mode); err != nil {
		return ClaudeResult{}, fmt.Errorf("write Claude project configuration: %w", err)
	}
	return result, nil
}

func readClaudeConfig(path string) ([]byte, os.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0o600, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("inspect Claude project configuration: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, errors.New(".mcp.json exists but is not a regular file")
	}
	if info.Size() > maxConfig {
		return nil, 0, false, errors.New(".mcp.json is too large")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, false, fmt.Errorf("open Claude project configuration: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxConfig+1))
	if err != nil {
		return nil, 0, false, fmt.Errorf("read Claude project configuration: %w", err)
	}
	if len(content) > maxConfig {
		return nil, 0, false, errors.New(".mcp.json is too large")
	}
	return content, info.Mode().Perm(), true, nil
}

func updateClaudeConfig(existing []byte, command, root string) ([]byte, error) {
	document := make(map[string]json.RawMessage)
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &document); err != nil {
			return nil, fmt.Errorf("parse .mcp.json: %w", err)
		}
		if document == nil {
			return nil, errors.New(".mcp.json must contain a JSON object")
		}
	}
	servers := make(map[string]json.RawMessage)
	if raw, ok := document["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil || servers == nil {
			return nil, errors.New(".mcp.json mcpServers must be a JSON object")
		}
	}
	if raw, ok := servers["pact"]; ok {
		var current claudeServer
		if err := json.Unmarshal(raw, &current); err != nil || current.Env["PACT_MANAGED_CONFIG"] != claudeManagedMarker {
			return nil, errors.New(".mcp.json already defines mcpServers.pact outside PACT management")
		}
	}
	desired := claudeServer{
		Type: "stdio", Command: command,
		Args: []string{"mcp", "serve", "--client", "claude", "--name", "Claude", "--path", root},
		Env:  map[string]string{"PACT_MANAGED_CONFIG": claudeManagedMarker},
	}
	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return nil, fmt.Errorf("encode Claude Pact server: %w", err)
	}
	servers["pact"] = desiredJSON
	serversJSON, err := json.Marshal(servers)
	if err != nil {
		return nil, fmt.Errorf("encode Claude MCP servers: %w", err)
	}
	document["mcpServers"] = serversJSON
	updated, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode .mcp.json: %w", err)
	}
	return append(updated, '\n'), nil
}
