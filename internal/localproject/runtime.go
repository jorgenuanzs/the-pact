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

	"github.com/jorgenuanzs/the-pact/internal/atomicfile"
	"github.com/jorgenuanzs/the-pact/internal/filelock"
)

const nodeIdentitySchemaVersion = 2

type NodeIdentity struct {
	SchemaVersion int    `json:"schema_version"`
	Key           string `json:"node_key"`
	Name          string `json:"name"`
	ServerURL     string `json:"server_url"`
}

func EnsureNodeIdentity(startPath string) (NodeIdentity, error) {
	binding, err := LoadBinding(startPath)
	if err != nil {
		return NodeIdentity{}, err
	}
	path := filepath.Join(binding.Root, localDirectory, "node.json")
	release, err := filelock.Acquire(path + ".lock")
	if err != nil {
		return NodeIdentity{}, fmt.Errorf("lock Pact node identity: %w", err)
	}
	defer release()
	return ensureNodeIdentity(path, binding.ServerURL, false)
}

func reconcileExistingNodeIdentity(root, serverURL string, rotate bool) error {
	path := filepath.Join(root, localDirectory, "node.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect Pact node identity: %w", err)
	}
	release, err := filelock.Acquire(path + ".lock")
	if err != nil {
		return fmt.Errorf("lock Pact node identity: %w", err)
	}
	defer release()
	_, err = ensureNodeIdentity(path, serverURL, rotate)
	return err
}

func ensureNodeIdentity(path, serverURL string, rotate bool) (NodeIdentity, error) {
	serverURL, err := normalizeServerURL(serverURL)
	if err != nil {
		return NodeIdentity{}, err
	}
	content, err := os.ReadFile(path)
	if err == nil {
		var identity NodeIdentity
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&identity); err != nil {
			return NodeIdentity{}, fmt.Errorf("decode Pact node identity: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return NodeIdentity{}, errors.New("decode Pact node identity: unexpected trailing data")
		}
		if !validUUID(identity.Key) || strings.TrimSpace(identity.Name) == "" {
			return NodeIdentity{}, errors.New("stored Pact node identity is invalid")
		}
		if identity.SchemaVersion == nodeIdentitySchemaVersion {
			configuredServer, normalizeErr := normalizeServerURL(identity.ServerURL)
			if normalizeErr != nil {
				return NodeIdentity{}, errors.New("stored Pact node identity has an invalid server URL")
			}
			if configuredServer == serverURL && !rotate {
				if err := os.Chmod(path, 0o600); err != nil {
					return NodeIdentity{}, fmt.Errorf("secure Pact node identity: %w", err)
				}
				identity.ServerURL = configuredServer
				return identity, nil
			}
		} else if identity.SchemaVersion == 1 && !rotate {
			identity.SchemaVersion = nodeIdentitySchemaVersion
			identity.ServerURL = serverURL
			if err := writeNodeIdentity(path, identity); err != nil {
				return NodeIdentity{}, err
			}
			return identity, nil
		} else if identity.SchemaVersion != 1 {
			return NodeIdentity{}, fmt.Errorf("unsupported Pact node identity schema version %d", identity.SchemaVersion)
		}
		return writeNewNodeIdentity(path, identity.Name, serverURL)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return NodeIdentity{}, fmt.Errorf("read Pact node identity: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "local-machine"
	}
	return writeNewNodeIdentity(path, hostname, serverURL)
}

func writeNewNodeIdentity(path, name, serverURL string) (NodeIdentity, error) {
	nodeKey, err := newUUID()
	if err != nil {
		return NodeIdentity{}, err
	}
	identity := NodeIdentity{
		SchemaVersion: nodeIdentitySchemaVersion,
		Key:           nodeKey,
		Name:          strings.TrimSpace(name),
		ServerURL:     serverURL,
	}
	if err := writeNodeIdentity(path, identity); err != nil {
		return NodeIdentity{}, err
	}
	return identity, nil
}

func writeNodeIdentity(path string, identity NodeIdentity) error {
	payload, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Pact node identity: %w", err)
	}
	payload = append(payload, '\n')
	if err := atomicfile.Write(path, payload, 0o600); err != nil {
		return fmt.Errorf("write Pact node identity: %w", err)
	}
	return nil
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
