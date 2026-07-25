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
	HTTPAddress       string
	DatabaseURL       string
	LocalAPIToken     string
	LocalOrganization string
	LogLevel          string
	RunMigrations     bool
	ShutdownTimeout   time.Duration
	DatabaseTimeout   time.Duration
	StatementTimeout  time.Duration
	LockTimeout       time.Duration
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

	cfg := Config{
		HTTPAddress:       envOrDefault("PACT_HTTP_ADDRESS", "127.0.0.1:8080"),
		DatabaseURL:       strings.TrimSpace(os.Getenv("PACT_DATABASE_URL")),
		LocalAPIToken:     os.Getenv("PACT_LOCAL_API_TOKEN"),
		LocalOrganization: envOrDefault("PACT_LOCAL_ORGANIZATION_ID", DefaultLocalOrganizationID),
		LogLevel:          strings.ToLower(envOrDefault("PACT_LOG_LEVEL", "info")),
		RunMigrations:     runMigrations,
		ShutdownTimeout:   shutdownTimeout,
		DatabaseTimeout:   databaseTimeout,
		StatementTimeout:  statementTimeout,
		LockTimeout:       lockTimeout,
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

func (c Config) ValidateServer() error {
	var errs []error

	if err := c.ValidateBase(); err != nil {
		errs = append(errs, err)
	}
	if _, _, err := net.SplitHostPort(c.HTTPAddress); err != nil {
		errs = append(errs, fmt.Errorf("PACT_HTTP_ADDRESS must be host:port: %w", err))
	}
	if len(c.LocalAPIToken) < 24 {
		errs = append(errs, errors.New("PACT_LOCAL_API_TOKEN must contain at least 24 characters"))
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
