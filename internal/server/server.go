package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/contextpack"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/githubapp"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projectrepo"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/repositorysync"
	"github.com/jorgenuanzs/the-pact/internal/rooms"
	"github.com/jorgenuanzs/the-pact/internal/transport/httpapi"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
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
	projectRepositoryStore := projectrepo.NewPostgresRepository(pool)
	projectRepositoryService := projectrepo.NewService(cfg.LocalOrganization, projectRepositoryStore)
	githubAppRepository := githubapp.NewPostgresRepository(pool)
	var githubAppClient *githubapp.Client
	if cfg.GitHubAppConfigured() {
		githubAppClient, err = githubapp.NewClient(githubapp.ClientConfig{
			AppID: cfg.GitHubAppID, ClientID: cfg.GitHubAppClientID,
			ClientSecret: cfg.GitHubAppSecret, PrivateKeyBase64: cfg.GitHubAppPrivateKey,
			APIURL: cfg.GitHubAPIURL, WebURL: cfg.GitHubWebURL,
			RedirectURL: cfg.PublicURL + "/v1/integrations/github/callback",
			Timeout:     cfg.GitHubTimeout, UserAgent: "the-pact/" + buildinfo.Current().Version,
		})
		if err != nil {
			return fmt.Errorf("configure GitHub App client: %w", err)
		}
	}
	githubAppService := githubapp.NewService(githubapp.ServiceConfig{
		OrganizationID: cfg.LocalOrganization, AppSlug: cfg.GitHubAppSlug,
		WebURL: cfg.GitHubWebURL, WebhookSecret: cfg.GitHubWebhookSecret,
		ClientID:    cfg.GitHubAppClientID,
		RedirectURL: cfg.PublicURL + "/v1/integrations/github/callback",
		OAuthSecret: cfg.GitHubAppSecret,
		Configured:  cfg.GitHubAppConfigured(),
	}, githubAppRepository, githubAppClient)
	var tokenSource repositorysync.TokenSource
	if cfg.GitHubAppConfigured() {
		tokenSource = githubAppTokenSource{service: githubAppService}
	}
	githubProvider, err := repositorysync.NewGitHubClient(repositorysync.GitHubOptions{
		APIURL: cfg.GitHubAPIURL, Token: cfg.GitHubToken, Timeout: cfg.GitHubTimeout,
		UserAgent: "the-pact/" + buildinfo.Current().Version, TokenSource: tokenSource,
	})
	if err != nil {
		return fmt.Errorf("configure GitHub repository provider: %w", err)
	}
	repositorySyncRepository := repositorysync.NewPostgresRepository(pool)
	repositorySyncService := repositorysync.NewService(
		cfg.LocalOrganization, projectService, repositorySyncRepository, githubProvider,
		projectRepositoryService,
	)
	workspaceRepository := workspaces.NewPostgresRepository(pool)
	workspaceService := workspaces.NewService(cfg.LocalOrganization, workspaceRepository)
	knowledgeRepository := knowledge.NewPostgresRepository(pool)
	knowledgeService := knowledge.NewService(cfg.LocalOrganization, knowledgeRepository)
	roomRepository := rooms.NewPostgresRepository(pool)
	roomService := rooms.NewService(cfg.LocalOrganization, roomRepository)
	agentSessionRepository := agentsession.NewPostgresRepository(pool)
	agentSessionService := agentsession.NewService(cfg.LocalOrganization, agentSessionRepository)
	coordinationRepository := coordination.NewPostgresRepository(pool)
	coordinationService := coordination.NewService(cfg.LocalOrganization, coordinationRepository)
	contextPackRepository := contextpack.NewPostgresRepository(pool)
	contextPackService := contextpack.NewService(
		cfg.LocalOrganization, contextPackRepository, projectService,
		workspaceService, coordinationService, knowledgeService, projectRepositoryService,
	)
	eventReader := eventlog.NewPostgresReader(pool)
	backofficeReader := backoffice.NewPostgresReader(pool)
	accessRepository := access.NewPostgresRepository(pool)
	accessService := access.NewService(cfg.LocalOrganization, cfg.LocalAPIToken, accessRepository)
	requestContext, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRequests()
	streamContext, cancelStreams := context.WithCancel(context.Background())
	defer cancelStreams()
	if cfg.GitHubSyncInterval > 0 {
		repositorySyncRunner := repositorysync.NewRunner(repositorySyncService, cfg.GitHubSyncInterval, logger)
		go repositorySyncRunner.Run(requestContext)
	}

	handler := httpapi.New(httpapi.Config{
		Logger:                   logger,
		OrganizationID:           cfg.LocalOrganization,
		Build:                    buildinfo.Current(),
		Readiness:                pool.Ping,
		ProjectService:           projectService,
		RepositorySyncService:    repositorySyncService,
		ProjectRepositoryService: projectRepositoryService,
		GitHubAppService:         githubAppService,
		WorkspaceService:         workspaceService,
		KnowledgeService:         knowledgeService,
		RoomService:              roomService,
		AgentSessionService:      agentSessionService,
		CoordinationService:      coordinationService,
		HandoffService:           coordinationService,
		ContextPackService:       contextPackService,
		AccessService:            accessService,
		BackofficeReader:         backofficeReader,
		EventReader:              eventReader,
		StreamShutdown:           streamContext.Done(),
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

type githubAppTokenSource struct{ service *githubapp.Service }

func (s githubAppTokenSource) Token(ctx context.Context, reference repositorysync.Reference) (string, error) {
	token, err := s.service.TokenForRepository(ctx, reference.FullName)
	if errors.Is(err, githubapp.ErrNotFound) || errors.Is(err, githubapp.ErrNotConfigured) {
		return "", nil
	}
	return token, err
}
