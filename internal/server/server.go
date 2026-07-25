package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/transport/httpapi"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateServer(); err != nil {
		return err
	}

	databaseCtx, cancelDatabase := context.WithTimeout(ctx, cfg.DatabaseTimeout)
	pool, err := postgres.Open(databaseCtx, cfg.DatabaseURL, postgres.Config{
		ApplicationName:  "pact-server",
		StatementTimeout: cfg.StatementTimeout,
		LockTimeout:      cfg.LockTimeout,
	})
	cancelDatabase()
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.RunMigrations {
		if err := migrations.Up(ctx, pool); err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
	}
	if err := migrations.Verify(ctx, pool); err != nil {
		return fmt.Errorf("verify database schema: %w", err)
	}
	var organizationExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM identity.organizations
			WHERE id = $1
			  AND status = 'active'
		)
	`, cfg.LocalOrganization).Scan(&organizationExists); err != nil {
		return fmt.Errorf("verify local organization: %w", err)
	}
	if !organizationExists {
		return fmt.Errorf("local organization %s does not exist or is not active", cfg.LocalOrganization)
	}

	projectRepository := projects.NewPostgresRepository(pool)
	projectService := projects.NewService(cfg.LocalOrganization, projectRepository)
	eventReader := eventlog.NewPostgresReader(pool)
	requestContext, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRequests()
	streamContext, cancelStreams := context.WithCancel(context.Background())
	defer cancelStreams()

	handler := httpapi.New(httpapi.Config{
		Logger:         logger,
		APIToken:       cfg.LocalAPIToken,
		OrganizationID: cfg.LocalOrganization,
		Build:          buildinfo.Current(),
		Readiness:      pool.Ping,
		ProjectService: projectService,
		EventReader:    eventReader,
		StreamShutdown: streamContext.Done(),
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext: func(net.Listener) context.Context {
			return requestContext
		},
	}

	listener, err := net.Listen("tcp", cfg.HTTPAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddress, err)
	}

	logger.Info("Pact server listening", "address", listener.Addr().String(), "version", buildinfo.Version)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- httpServer.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		logger.Info("Pact server shutting down")
		cancelStreams()
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		cancelRequests()
		_ = httpServer.Close()
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	cancelRequests()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
	default:
	}

	return nil
}
