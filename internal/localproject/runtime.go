package localproject

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Binding struct {
	Root      string
	ServerURL string
	ProjectID string
}

type NodeIdentity struct {
	SchemaVersion int    `json:"schema_version"`
	Key           string `json:"node_key"`
	Name          string `json:"name"`
}

// FindBinding reports whether a Git checkout contains local PACT binding
// state. A malformed or incomplete binding is returned as an error instead of
// being treated as absent, so callers never fall back to another server.
func FindBinding(startPath string) (Binding, bool, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return Binding{}, false, nil
	}
	configPath := filepath.Join(root, localDirectory, localConfigName)
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		return Binding{}, false, nil
	} else if err != nil {
		return Binding{}, true, fmt.Errorf("inspect local Pact configuration: %w", err)
	}
	binding, err := LoadBinding(root)
	return binding, true, err
}

func LoadBinding(startPath string) (Binding, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return Binding{}, err
	}
	configPath := filepath.Join(root, localDirectory, localConfigName)
	content, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return Binding{}, errors.New("project is not connected; run pact init or pact connect")
	}
	if err != nil {
		return Binding{}, fmt.Errorf("read local Pact configuration: %w", err)
	}
	var config localConfig
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Binding{}, fmt.Errorf("decode local Pact configuration: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Binding{}, errors.New("decode local Pact configuration: unexpected trailing data")
	}
	serverURL, err := normalizeServerURL(config.ServerURL)
	if err != nil {
		return Binding{}, err
	}
	if !validUUID(config.ProjectID) {
		return Binding{}, errors.New("project is not connected to a valid remote project")
	}
	if err := ensurePlatformGitConfig(root); err != nil {
		return Binding{}, err
	}
	if err := os.Chmod(configPath, 0o600); err != nil {
		return Binding{}, fmt.Errorf("secure local Pact configuration: %w", err)
	}
	return Binding{Root: root, ServerURL: serverURL, ProjectID: config.ProjectID}, nil
}

func EnsureNodeIdentity(startPath string) (NodeIdentity, error) {
	binding, err := LoadBinding(startPath)
	if err != nil {
		return NodeIdentity{}, err
	}
	path := filepath.Join(binding.Root, localDirectory, "node.json")
	content, err := os.ReadFile(path)
	if err == nil {
		var identity NodeIdentity
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&identity); err != nil {
			return NodeIdentity{}, fmt.Errorf("decode Pact node identity: %w", err)
		}
		if identity.SchemaVersion != 1 || !validUUID(identity.Key) || strings.TrimSpace(identity.Name) == "" {
			return NodeIdentity{}, errors.New("stored Pact node identity is invalid")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return NodeIdentity{}, fmt.Errorf("secure Pact node identity: %w", err)
		}
		return identity, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return NodeIdentity{}, fmt.Errorf("read Pact node identity: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "local-machine"
	}
	nodeKey, err := newUUID()
	if err != nil {
		return NodeIdentity{}, err
	}
	identity := NodeIdentity{
		SchemaVersion: 1,
		Key:           nodeKey,
		Name:          strings.TrimSpace(hostname),
	}
	payload, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return NodeIdentity{}, fmt.Errorf("encode Pact node identity: %w", err)
	}
	payload = append(payload, '\n')
	if err := writeAtomic(path, payload, 0o600); err != nil {
		return NodeIdentity{}, fmt.Errorf("write Pact node identity: %w", err)
	}
	return identity, nil
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate Pact node identity: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
