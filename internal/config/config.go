package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const DefaultLocalOrganizationID = "00000000-0000-4000-8000-000000000001"

type Config struct {
	HTTPAddress         string
	DatabaseURL         string
	SetupToken          string
	LocalOrganization   string
	LogLevel            string
	RunMigrations       bool
	ShutdownTimeout     time.Duration
	DatabaseTimeout     time.Duration
	StatementTimeout    time.Duration
	LockTimeout         time.Duration
	PublicURL           string
	GitHubAPIURL        string
	GitHubWebURL        string
	GitHubToken         string
	GitHubAppID         int64
	GitHubAppSlug       string
	GitHubAppClientID   string
	GitHubAppSecret     string
	GitHubAppPrivateKey string
	GitHubWebhookSecret string
	GitHubTimeout       time.Duration
	GitHubSyncInterval  time.Duration
}

func Load() (Config, error) {
	runMigrations, err := boolEnv("PACT_RUN_MIGRATIONS", false)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := durationEnv("PACT_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	databaseTimeout, err := durationEnv("PACT_DATABASE_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	statementTimeout, err := durationEnv("PACT_DATABASE_STATEMENT_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	lockTimeout, err := durationEnv("PACT_DATABASE_LOCK_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	githubTimeout, err := durationEnv("PACT_GITHUB_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	githubSyncInterval, err := durationEnv("PACT_GITHUB_SYNC_INTERVAL", 0)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddress:         envOrDefault("PACT_HTTP_ADDRESS", "127.0.0.1:8080"),
		DatabaseURL:         strings.TrimSpace(os.Getenv("PACT_DATABASE_URL")),
		SetupToken:          strings.TrimSpace(os.Getenv("PACT_SETUP_TOKEN")),
		LocalOrganization:   envOrDefault("PACT_LOCAL_ORGANIZATION_ID", DefaultLocalOrganizationID),
		LogLevel:            strings.ToLower(envOrDefault("PACT_LOG_LEVEL", "info")),
		RunMigrations:       runMigrations,
		ShutdownTimeout:     shutdownTimeout,
		DatabaseTimeout:     databaseTimeout,
		StatementTimeout:    statementTimeout,
		LockTimeout:         lockTimeout,
		PublicURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("PACT_PUBLIC_URL")), "/"),
		GitHubAPIURL:        envOrDefault("PACT_GITHUB_API_URL", "https://api.github.com"),
		GitHubWebURL:        envOrDefault("PACT_GITHUB_WEB_URL", "https://github.com"),
		GitHubToken:         strings.TrimSpace(os.Getenv("PACT_GITHUB_TOKEN")),
		GitHubAppSlug:       strings.TrimSpace(os.Getenv("PACT_GITHUB_APP_SLUG")),
		GitHubAppClientID:   strings.TrimSpace(os.Getenv("PACT_GITHUB_APP_CLIENT_ID")),
		GitHubAppSecret:     strings.TrimSpace(os.Getenv("PACT_GITHUB_APP_CLIENT_SECRET")),
		GitHubAppPrivateKey: strings.TrimSpace(os.Getenv("PACT_GITHUB_APP_PRIVATE_KEY_BASE64")),
		GitHubWebhookSecret: strings.TrimSpace(os.Getenv("PACT_GITHUB_APP_WEBHOOK_SECRET")),
		GitHubTimeout:       githubTimeout,
		GitHubSyncInterval:  githubSyncInterval,
	}
	if raw := strings.TrimSpace(os.Getenv("PACT_GITHUB_APP_ID")); raw != "" {
		cfg.GitHubAppID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cfg.GitHubAppID <= 0 {
			return Config{}, errors.New("PACT_GITHUB_APP_ID must be a positive integer")
		}
	}

	if err := cfg.ValidateBase(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) ValidateBase() error {
	var errs []error

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("PACT_DATABASE_URL is required"))
	}
	if c.DatabaseTimeout <= 0 {
		errs = append(errs, errors.New("PACT_DATABASE_TIMEOUT must be positive"))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("PACT_SHUTDOWN_TIMEOUT must be positive"))
	}
	if c.StatementTimeout <= 0 {
		errs = append(errs, errors.New("PACT_DATABASE_STATEMENT_TIMEOUT must be positive"))
	}
	if c.LockTimeout <= 0 {
		errs = append(errs, errors.New("PACT_DATABASE_LOCK_TIMEOUT must be positive"))
	}
	if c.GitHubTimeout <= 0 {
		errs = append(errs, errors.New("PACT_GITHUB_TIMEOUT must be positive"))
	}
	if c.GitHubSyncInterval < 0 {
		errs = append(errs, errors.New("PACT_GITHUB_SYNC_INTERVAL must not be negative"))
	}
	if c.GitHubSyncInterval > 0 && c.GitHubSyncInterval < time.Minute {
		errs = append(errs, errors.New("PACT_GITHUB_SYNC_INTERVAL must be zero or at least 1m"))
	}
	appValues := []bool{
		c.GitHubAppID > 0,
		c.GitHubAppSlug != "",
		c.GitHubAppClientID != "",
		c.GitHubAppSecret != "",
		c.GitHubAppPrivateKey != "",
		c.GitHubWebhookSecret != "",
	}
	configuredValues := 0
	for _, configured := range appValues {
		if configured {
			configuredValues++
		}
	}
	if configuredValues != 0 && (configuredValues != len(appValues) || c.PublicURL == "") {
		errs = append(errs, errors.New("GitHub App configuration is incomplete; PACT_PUBLIC_URL and every PACT_GITHUB_APP_* variable are required together"))
	}
	if !validUUID(c.LocalOrganization) {
		errs = append(errs, errors.New("PACT_LOCAL_ORGANIZATION_ID must be a UUID"))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("PACT_LOG_LEVEL must be debug, info, warn, or error, got %q", c.LogLevel))
	}

	return errors.Join(errs...)
}

func (c Config) GitHubAppConfigured() bool {
	return c.GitHubAppID > 0 && c.GitHubAppSlug != "" && c.GitHubAppClientID != "" &&
		c.GitHubAppSecret != "" && c.GitHubAppPrivateKey != "" &&
		c.GitHubWebhookSecret != "" && c.PublicURL != ""
}

func (c Config) ValidateServer() error {
	var errs []error

	if err := c.ValidateBase(); err != nil {
		errs = append(errs, err)
	}
	if _, _, err := net.SplitHostPort(c.HTTPAddress); err != nil {
		errs = append(errs, fmt.Errorf("PACT_HTTP_ADDRESS must be host:port: %w", err))
	}
	if c.SetupToken != "" && len(c.SetupToken) < 24 {
		errs = append(errs, errors.New("PACT_SETUP_TOKEN must contain at least 24 characters when configured"))
	}

	return errors.Join(errs...)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return value, nil
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	return value, nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, char := range value {
		switch i {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return false
			}
		}
	}
	return true
}
