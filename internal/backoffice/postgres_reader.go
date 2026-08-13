package backoffice

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
)

const (
	recentEventLimit        = 20
	observerFreshnessWindow = 30 * time.Second
	activeActivityWindow    = 15 * time.Second
	recentActivityWindow    = 15 * time.Minute
)

type Reader interface {
	Get(context.Context, string, string) (Overview, error)
}

type PostgresReader struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewPostgresReader(pool *pgxpool.Pool) *PostgresReader {
	return &PostgresReader{
		pool: pool,
		now:  time.Now,
	}
}

func (r *PostgresReader) Get(
	ctx context.Context,
	organizationID string,
	projectID string,
) (Overview, error) {
	asOf := r.now().UTC()
	freshAfter := asOf.Add(-observerFreshnessWindow)

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return Overview{}, fmt.Errorf("begin backoffice snapshot: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	overview := Overview{
		CodeActivity: CodeActivity{
			State:             CodeActivityUnobserved,
			Reason:            ReasonNoConnectedObserver,
			ObserverFreshSecs: int64(observerFreshnessWindow / time.Second),
			ActiveWindowSecs:  int64(activeActivityWindow / time.Second),
			RecentWindowSecs:  int64(recentActivityWindow / time.Second),
		},
		ActiveWork:   make([]ActiveWork, 0),
		RecentEvents: make([]RecentEvent, 0),
		WorkItems:    make([]coordination.WorkItem, 0),
		GeneratedAt:  asOf,
	}

	if err := r.loadCounts(
		ctx,
		tx,
		organizationID,
		projectID,
		asOf,
		freshAfter,
		&overview.Counts,
	); err != nil {
		return Overview{}, err
	}
	activityType, activityAt, err := r.loadLatestCodeActivity(ctx, tx, organizationID, projectID)
	if err != nil {
		return Overview{}, err
	}
	overview.CodeActivity = deriveCodeActivity(
		overview.GeneratedAt,
		overview.Counts.ConnectedObservers,
		activityType,
		activityAt,
	)

	activeWork, err := r.loadActiveWork(
		ctx,
		tx,
		organizationID,
		projectID,
		asOf,
		freshAfter,
	)
	if err != nil {
		return Overview{}, err
	}
	overview.ActiveWork = activeWork

	recentEvents, err := r.loadRecentEvents(ctx, tx, organizationID, projectID)
	if err != nil {
		return Overview{}, err
	}
	overview.RecentEvents = recentEvents

	if err := tx.Commit(ctx); err != nil {
		return Overview{}, fmt.Errorf("commit backoffice snapshot: %w", err)
	}
	return overview, nil
}

func (r *PostgresReader) loadCounts(
	ctx context.Context,
	database queryer,
	organizationID string,
	projectID string,
	asOf time.Time,
	freshAfter time.Time,
	counts *Counts,
) error {
	err := database.QueryRow(ctx, `
		SELECT
			(
				SELECT count(*)
				FROM coordination.repositories
				WHERE organization_id = $1
				  AND project_id = $2
				  AND status <> 'archived'
			),
			(
				SELECT count(*)
				FROM identity.sessions
				WHERE organization_id = $1
				  AND project_id = $2
				  AND status = 'active'
				  AND expires_at > $3
				  AND last_seen_at >= $4
			),
			(
				SELECT count(DISTINCT session.node_id)
				FROM identity.sessions AS session
				JOIN identity.nodes AS node
				  ON node.organization_id = session.organization_id
				 AND node.id = session.node_id
				WHERE session.organization_id = $1
				  AND session.project_id = $2
				  AND session.status = 'active'
				  AND session.expires_at > $3
				  AND session.last_seen_at >= $4
				  AND node.lifecycle_status = 'active'
				  AND node.last_seen_at >= $4
			),
			(
				SELECT count(*)
				FROM identity.sessions AS session
				JOIN identity.nodes AS node
				  ON node.organization_id = session.organization_id
				 AND node.id = session.node_id
				WHERE session.organization_id = $1
				  AND session.project_id = $2
				  AND session.status = 'active'
				  AND session.expires_at > $3
				  AND session.last_seen_at >= $4
				  AND node.lifecycle_status = 'active'
				  AND node.last_seen_at >= $4
				  AND session.announced_capabilities
				      @> '{"workspace.diff.observe.v1": true}'::jsonb
			),
			(
				SELECT count(*)
				FROM coordination.intents
				WHERE organization_id = $1
				  AND project_id = $2
				  AND status = 'active'
			),
			(
				SELECT count(*)
				FROM coordination.intents
				WHERE organization_id = $1
				  AND project_id = $2
				  AND status = 'blocked'
			),
			(
				SELECT count(*)
				FROM coordination.workspaces
				WHERE organization_id = $1
				  AND project_id = $2
				  AND status IN ('provisioning', 'ready', 'active', 'frozen')
			),
			(
				SELECT count(*)
				FROM coordination.scope_claims
				WHERE organization_id = $1
				  AND project_id = $2
				  AND status = 'active'
				  AND expires_at > $3
			),
			(
				SELECT count(*)
				FROM coordination.changesets
				WHERE organization_id = $1
				  AND project_id = $2
				  AND status IN ('created', 'validating', 'validated', 'integrating')
			),
			(
				SELECT count(*)
				FROM platform.events
				WHERE organization_id = $1
				  AND project_id = $2
			)
	`, organizationID, projectID, asOf, freshAfter).Scan(
		&counts.Repositories,
		&counts.LiveSessions,
		&counts.ConnectedNodes,
		&counts.ConnectedObservers,
		&counts.ActiveIntents,
		&counts.BlockedIntents,
		&counts.LiveWorkspaces,
		&counts.ActiveScopeClaims,
		&counts.PendingChangesets,
		&counts.Events,
	)
	if err != nil {
		return fmt.Errorf("load backoffice counts: %w", err)
	}
	return nil
}

func (r *PostgresReader) loadLatestCodeActivity(
	ctx context.Context,
	database queryer,
	organizationID string,
	projectID string,
) (*string, *time.Time, error) {
	var (
		eventType  string
		recordedAt time.Time
	)
	err := database.QueryRow(ctx, `
		SELECT event_type, recorded_at
		FROM platform.events
		WHERE organization_id = $1
		  AND project_id = $2
		  AND event_type IN (
		      'pact.workspace.diff_updated.v1',
		      'pact.git.external_change_detected.v1',
		      'pact.changeset.created.v1'
		  )
		ORDER BY project_sequence DESC
		LIMIT 1
	`, organizationID, projectID).Scan(&eventType, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load latest code activity: %w", err)
	}
	return &eventType, &recordedAt, nil
}

func deriveCodeActivity(
	now time.Time,
	observerCount int64,
	eventType *string,
	observedAt *time.Time,
) CodeActivity {
	activity := CodeActivity{
		State:             CodeActivityUnobserved,
		Reason:            ReasonNoConnectedObserver,
		Source:            eventType,
		ObservedAt:        observedAt,
		ObserverConnected: observerCount > 0,
		ObserverCount:     observerCount,
		ObserverFreshSecs: int64(observerFreshnessWindow / time.Second),
		ActiveWindowSecs:  int64(activeActivityWindow / time.Second),
		RecentWindowSecs:  int64(recentActivityWindow / time.Second),
	}

	if eventType != nil && observedAt != nil {
		age := now.Sub(*observedAt)
		if age < 0 {
			age = 0
		}
		kind := codeActivityKind(*eventType)
		if observerCount > 0 && kind != "changeset" && age <= activeActivityWindow {
			activity.State = CodeActivityEditing
			if kind == "external_change" {
				activity.Reason = ReasonFreshExternalChange
			} else {
				activity.Reason = ReasonFreshWorkspaceDiff
			}
			return activity
		}
		if age <= recentActivityWindow {
			activity.State = CodeActivityRecent
			switch kind {
			case "external_change":
				activity.Reason = ReasonRecentExternalChange
			case "changeset":
				activity.Reason = ReasonRecentChangeset
			default:
				activity.Reason = ReasonRecentWorkspaceDiff
			}
			return activity
		}
	}

	if observerCount > 0 {
		activity.State = CodeActivityIdle
		activity.Reason = ReasonObserverWithoutChange
	}
	return activity
}

func codeActivityKind(eventType string) string {
	switch eventType {
	case "pact.git.external_change_detected.v1":
		return "external_change"
	case "pact.changeset.created.v1":
		return "changeset"
	default:
		return "workspace_diff"
	}
}

func (r *PostgresReader) loadActiveWork(
	ctx context.Context,
	database queryer,
	organizationID string,
	projectID string,
	asOf time.Time,
	freshAfter time.Time,
) ([]ActiveWork, error) {
	rows, err := database.Query(ctx, `
		SELECT
			session.id,
			actor.id,
			actor.display_name,
			actor.kind,
			session.client_type,
			session.status,
			session.last_seen_at,
			session.expires_at,
			node.id,
			node.name,
			node.lifecycle_status,
			intent.id,
			intent.title,
			intent.status,
			workspace.id,
			workspace.status,
			workspace.git_branch,
			workspace.path_ref
		FROM identity.sessions AS session
		JOIN identity.actors AS actor
		  ON actor.organization_id = session.organization_id
		 AND actor.id = session.actor_id
		LEFT JOIN identity.nodes AS node
		  ON node.organization_id = session.organization_id
		 AND node.id = session.node_id
		LEFT JOIN coordination.workspaces AS workspace
		  ON workspace.organization_id = session.organization_id
		 AND workspace.project_id = session.project_id
		 AND workspace.session_id = session.id
		 AND workspace.status IN ('provisioning', 'ready', 'active', 'frozen')
		LEFT JOIN coordination.intents AS intent
		  ON intent.organization_id = workspace.organization_id
		 AND intent.project_id = workspace.project_id
		 AND intent.id = workspace.intent_id
		WHERE session.organization_id = $1
		  AND session.project_id = $2
		  AND session.status = 'active'
		  AND session.expires_at > $3
		  AND session.last_seen_at >= $4
		ORDER BY session.last_seen_at DESC, session.id, workspace.id
	`, organizationID, projectID, asOf, freshAfter)
	if err != nil {
		return nil, fmt.Errorf("load active work: %w", err)
	}
	defer rows.Close()

	work := make([]ActiveWork, 0)
	for rows.Next() {
		var item ActiveWork
		if err := rows.Scan(
			&item.SessionID,
			&item.ActorID,
			&item.ActorName,
			&item.ActorKind,
			&item.ClientType,
			&item.SessionStatus,
			&item.LastSeenAt,
			&item.ExpiresAt,
			&item.NodeID,
			&item.NodeName,
			&item.NodeStatus,
			&item.IntentID,
			&item.IntentTitle,
			&item.IntentStatus,
			&item.WorkspaceID,
			&item.WorkspaceStatus,
			&item.WorkspaceBranch,
			&item.WorkspacePathRef,
		); err != nil {
			return nil, fmt.Errorf("load active work: %w", err)
		}
		work = append(work, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load active work: %w", err)
	}
	return work, nil
}

func (r *PostgresReader) loadRecentEvents(
	ctx context.Context,
	database queryer,
	organizationID string,
	projectID string,
) ([]RecentEvent, error) {
	rows, err := database.Query(ctx, `
		SELECT
			event.id,
			event.project_sequence,
			event.event_type,
			event.actor_id,
			actor.display_name,
			event.session_id,
			event.intent_id,
			event.occurred_at,
			event.payload
		FROM platform.events AS event
		LEFT JOIN identity.actors AS actor
		  ON actor.organization_id = event.organization_id
		 AND actor.id = event.actor_id
		WHERE event.organization_id = $1
		  AND event.project_id = $2
		ORDER BY event.project_sequence DESC
		LIMIT $3
	`, organizationID, projectID, recentEventLimit)
	if err != nil {
		return nil, fmt.Errorf("load recent events: %w", err)
	}
	defer rows.Close()

	events := make([]RecentEvent, 0)
	for rows.Next() {
		var (
			event    RecentEvent
			sequence int64
		)
		if err := rows.Scan(
			&event.ID,
			&sequence,
			&event.Type,
			&event.ActorID,
			&event.ActorName,
			&event.SessionID,
			&event.IntentID,
			&event.OccurredAt,
			&event.Data,
		); err != nil {
			return nil, fmt.Errorf("load recent events: %w", err)
		}
		event.Sequence = strconv.FormatInt(sequence, 10)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load recent events: %w", err)
	}
	return events, nil
}
