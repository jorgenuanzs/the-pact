package repositorysync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/projects"
)

type ScheduledSyncService interface {
	ListProjects(context.Context) ([]projects.Project, error)
	SyncScheduledProject(context.Context, string) ([]Result, error)
}

type Runner struct {
	service  ScheduledSyncService
	interval time.Duration
	logger   *slog.Logger
}

func NewRunner(service ScheduledSyncService, interval time.Duration, logger *slog.Logger) *Runner {
	return &Runner{service: service, interval: interval, logger: logger}
}

func (r *Runner) Run(ctx context.Context) {
	if r == nil || r.service == nil || r.interval <= 0 {
		return
	}
	r.sync(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sync(ctx)
		}
	}
}

func (r *Runner) sync(ctx context.Context) {
	projectList, err := r.service.ListProjects(ctx)
	if err != nil {
		r.logger.Warn("scheduled repository sync could not list projects", "error", err)
		return
	}
	for _, project := range projectList {
		if project.Status != "active" || project.RootRepository == nil {
			continue
		}
		results, err := r.service.SyncScheduledProject(ctx, project.ID)
		if err != nil {
			var providerErr *ProviderError
			if errors.As(err, &providerErr) {
				r.logger.Warn("scheduled repository sync failed", "project_id", project.ID, "code", providerErr.Code)
			} else {
				r.logger.Warn("scheduled repository sync failed", "project_id", project.ID, "error", err)
			}
		}
		for _, result := range results {
			if result.Changed {
				r.logger.Info("repository state updated", "project_id", project.ID,
					"repository_id", result.State.RepositoryID, "revision", result.State.CanonicalRevision)
			}
		}
	}
}
