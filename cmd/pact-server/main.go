package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/lifecycle"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/server"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("Pact stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "version":
		return json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
	case "serve", "migrate":
	default:
		return usageError(command)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := lifecycle.NotifyContext(context.Background())
	defer stop()

	if command == "serve" {
		return server.Run(ctx, cfg, logger)
	}

	action := "up"
	if len(args) > 1 {
		action = args[1]
	}
	return runMigrations(ctx, cfg, action)
}

func runMigrations(ctx context.Context, cfg config.Config, action string) error {
	databaseCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseTimeout)
	pool, err := postgres.Open(databaseCtx, cfg.DatabaseURL, postgres.Config{
		ApplicationName: "pact-migrate",
	})
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	switch action {
	case "up":
		return migrations.Up(ctx, pool)
	case "status":
		applied, err := migrations.Status(ctx, pool)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(applied)
	default:
		return errors.New("migration action must be \"up\" or \"status\"")
	}
}

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))
}

func usageError(command string) error {
	return fmt.Errorf("unknown command %q; expected serve, migrate [up|status], or version", command)
}
