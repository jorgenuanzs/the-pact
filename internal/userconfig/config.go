package userconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const schemaVersion = 1

type Config struct {
	SchemaVersion int    `json:"schema_version"`
	ServerURL     string `json:"server_url"`
	APIToken      string `json:"api_token"`
}

func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, errors.New("not logged in; run pact login --server <url> --token-stdin")
	}
	if err != nil {
		return Config{}, fmt.Errorf("read Pact user configuration: %w", err)
	}
	var config Config
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode Pact user configuration: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("decode Pact user configuration: unexpected trailing data")
	}
	if config.SchemaVersion != schemaVersion {
		return Config{}, fmt.Errorf("unsupported Pact user configuration version %d", config.SchemaVersion)
	}
	normalized, err := NormalizeServerURL(config.ServerURL)
	if err != nil {
		return Config{}, err
	}
	if len(config.APIToken) < 24 {
		return Config{}, errors.New("stored Pact API token is invalid")
	}
	config.ServerURL = normalized
	if err := os.Chmod(path, 0o600); err != nil {
		return Config{}, fmt.Errorf("secure Pact user configuration: %w", err)
	}
	return config, nil
}

func Save(serverURL, apiToken string) (string, error) {
	normalized, err := NormalizeServerURL(serverURL)
	if err != nil {
		return "", err
	}
	apiToken = strings.TrimSpace(apiToken)
	if len(apiToken) < 24 {
		return "", errors.New("Pact API token must contain at least 24 characters")
	}
	path, err := configPath()
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create Pact configuration directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", fmt.Errorf("secure Pact configuration directory: %w", err)
	}
	payload, err := json.MarshalIndent(Config{
		SchemaVersion: schemaVersion,
		ServerURL:     normalized,
		APIToken:      apiToken,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode Pact user configuration: %w", err)
	}
	payload = append(payload, '\n')
	if err := writeAtomic(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("write Pact user configuration: %w", err)
	}
	return path, nil
}

func NormalizeServerURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid Pact Server URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("Pact Server URL must be an absolute http or https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Pact Server URL must not contain credentials, a query, or a fragment")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return "", errors.New("remote Pact Server URLs must use https")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func configPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PACT_CONFIG_DIR")); override != "" {
		absolute, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve PACT_CONFIG_DIR: %w", err)
		}
		return filepath.Join(absolute, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "pact", "config.json"), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pact-user-config-*")
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
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
