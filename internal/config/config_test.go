package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadUsesSafeDefaults(t *testing.T) {
	t.Setenv("PACT_DATABASE_URL", "postgres://example")
	t.Setenv("PACT_SETUP_TOKEN", strings.Repeat("x", 24))
	t.Setenv("PACT_HTTP_ADDRESS", "")
	t.Setenv("PACT_LOCAL_ORGANIZATION_ID", "")
	t.Setenv("PACT_LOG_LEVEL", "")
	t.Setenv("PACT_RUN_MIGRATIONS", "")
	t.Setenv("PACT_SHUTDOWN_TIMEOUT", "")
	t.Setenv("PACT_DATABASE_TIMEOUT", "")
	t.Setenv("PACT_DATABASE_STATEMENT_TIMEOUT", "")
	t.Setenv("PACT_DATABASE_LOCK_TIMEOUT", "")
	t.Setenv("PACT_GITHUB_API_URL", "")
	t.Setenv("PACT_GITHUB_TOKEN", "")
	t.Setenv("PACT_GITHUB_TIMEOUT", "")
	t.Setenv("PACT_GITHUB_SYNC_INTERVAL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddress != "127.0.0.1:8080" {
		t.Fatalf("HTTPAddress = %q", cfg.HTTPAddress)
	}
	if cfg.LocalOrganization != DefaultLocalOrganizationID {
		t.Fatalf("LocalOrganization = %q", cfg.LocalOrganization)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %v", cfg.ShutdownTimeout)
	}
	if cfg.StatementTimeout != 15*time.Second || cfg.LockTimeout != 5*time.Second {
		t.Fatalf("database timeouts = %v, %v", cfg.StatementTimeout, cfg.LockTimeout)
	}
	if cfg.GitHubAPIURL != "https://api.github.com" || cfg.GitHubTimeout != 10*time.Second || cfg.GitHubSyncInterval != 0 {
		t.Fatalf("GitHub defaults = %q, %v, %v", cfg.GitHubAPIURL, cfg.GitHubTimeout, cfg.GitHubSyncInterval)
	}
}

func TestValidateServerRejectsShortToken(t *testing.T) {
	cfg := Config{
		HTTPAddress:       "127.0.0.1:8080",
		DatabaseURL:       "postgres://example",
		SetupToken:        "too-short",
		LocalOrganization: DefaultLocalOrganizationID,
		LogLevel:          "info",
		ShutdownTimeout:   time.Second,
		DatabaseTimeout:   time.Second,
		StatementTimeout:  time.Second,
		LockTimeout:       time.Second,
	}

	err := cfg.ValidateServer()
	if err == nil || !strings.Contains(err.Error(), "PACT_SETUP_TOKEN") {
		t.Fatalf("ValidateServer() error = %v", err)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("PACT_DATABASE_URL", "postgres://example")
	t.Setenv("PACT_SHUTDOWN_TIMEOUT", "eventually")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "PACT_SHUTDOWN_TIMEOUT") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsOverlyFrequentGitHubPolling(t *testing.T) {
	t.Setenv("PACT_DATABASE_URL", "postgres://example")
	t.Setenv("PACT_GITHUB_SYNC_INTERVAL", "30s")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "PACT_GITHUB_SYNC_INTERVAL") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsPartialGitHubAppConfiguration(t *testing.T) {
	t.Setenv("PACT_DATABASE_URL", "postgres://example")
	t.Setenv("PACT_GITHUB_APP_ID", "123")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "GitHub App configuration is incomplete") {
		t.Fatalf("Load() error = %v", err)
	}
}
