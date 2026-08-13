package coordination

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const scopeLease = 60 * time.Second

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CheckScopes(
	ctx context.Context,
	organizationID string,
	projectID string,
	scopes []ScopeInput,
) (ScopeCheckResult, error) {
	var repositoryID *string
	err := r.pool.QueryRow(ctx, `
		SELECT root_repository_id
		FROM identity.projects
		WHERE organization_id = $1 AND id = $2 AND status <> 'archived'
	`, organizationID, projectID).Scan(&repositoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ScopeCheckResult{}, ErrNotFound
	}
	if err != nil {
		return ScopeCheckResult{}, fmt.Errorf("load project repository for scope check: %w", err)
	}
	if repositoryID == nil {
		return ScopeCheckResult{}, ErrRepositoryUnavailable
	}
	overlaps, err := loadScopeOverlaps(ctx, r.pool, organizationID, projectID, *repositoryID, scopes)
	if err != nil {
		return ScopeCheckResult{}, err
	}
	return ScopeCheckResult{Scopes: scopes, Overlaps: overlaps, Blocked: hasBlockingOverlap(overlaps)}, nil
}

func (r *PostgresRepository) Start(
	ctx context.Context,
	organizationID string,
	principalID string,
	allowAll bool,
	projectID string,
	idempotencyKey string,
	requestHash [sha256.Size]byte,
	input StartInput,
) (StartResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return StartResult{}, fmt.Errorf("begin work.start: %w", err)
	}
	defer rollback(tx)

	actorID, err := authorizeSession(ctx, tx, organizationID, principalID, allowAll, projectID, input.SessionID)
	if err != nil {
		return StartResult{}, err
	}
	if err := lockProjectCoordination(ctx, tx, organizationID, projectID); err != nil {
		return StartResult{}, err
	}
	commandID, stored, replayed, err := reserveCommand(
		ctx, tx, organizationID, projectID, "work.start", idempotencyKey, requestHash,
	)
	if err != nil {
		return StartResult{}, err
	}
	if replayed {
		var result StartResult
		if err := json.Unmarshal(stored, &result); err != nil {
			return StartResult{}, fmt.Errorf("decode stored work.start result: %w", err)
		}
		result.Replayed = true
		return result, nil
	}

	var repositoryID *string
	err = tx.QueryRow(ctx, `
		SELECT root_repository_id
		FROM identity.projects
		WHERE organization_id = $1 AND id = $2 AND status <> 'archived'
		FOR SHARE
	`, organizationID, projectID).Scan(&repositoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return StartResult{}, ErrNotFound
	}
	if err != nil {
		return StartResult{}, fmt.Errorf("load project repository for work.start: %w", err)
	}
	if repositoryID == nil {
		return StartResult{}, ErrRepositoryUnavailable
	}
	overlaps, err := loadScopeOverlaps(ctx, tx, organizationID, projectID, *repositoryID, input.Scopes)
	if err != nil {
		return StartResult{}, err
	}
	if hasBlockingOverlap(overlaps) && !input.AllowOverlap {
		return StartResult{}, &ScopeConflictError{Overlaps: overlaps}
	}

	criteria, err := json.Marshal(input.SuccessCriteria)
	if err != nil {
		return StartResult{}, fmt.Errorf("encode success criteria: %w", err)
	}
	intent, err := scanIntent(tx.QueryRow(ctx, `
		INSERT INTO coordination.intents (
			organization_id, project_id, title, goal, success_criteria, status,
			base_revision, responsible_agent_id, created_by_actor_id
		)
		VALUES ($1, $2, $3, $4, $5, 'active', $6, $7, $7)
		RETURNING id, project_id, title, goal, success_criteria, status, summary,
		          status_detail, base_revision, responsible_agent_id, created_by_actor_id,
		          version, created_at, updated_at, status_changed_at, completed_at
	`, organizationID, projectID, input.Title, input.Goal, criteria, input.BaseRevision, actorID))
	if err != nil {
		return StartResult{}, fmt.Errorf("create intent: %w", err)
	}

	claims := make([]ScopeClaim, 0, len(input.Scopes))
	for _, requested := range input.Scopes {
		resource, err := upsertResourceRef(ctx, tx, organizationID, projectID, *repositoryID, requested)
		if err != nil {
			return StartResult{}, err
		}
		claim, err := scanScopeClaim(tx.QueryRow(ctx, `
			INSERT INTO coordination.scope_claims (
				organization_id, project_id, intent_id, session_id, resource_ref_id,
				origin, confidence, evidence, status, claim_mode,
				last_renewed_at, expires_at
			)
			VALUES (
				$1, $2, $3, $4, $5, 'declared', 1.0000, '{}'::jsonb, 'active', $6,
				transaction_timestamp(), transaction_timestamp() + ($7 * interval '1 second')
			)
			RETURNING id, intent_id, session_id, origin, claim_mode, status, version,
			          created_at, updated_at, last_renewed_at, expires_at
		`, organizationID, projectID, intent.ID, input.SessionID, resource.ID,
			requested.Mode, int64(scopeLease/time.Second)), resource)
		if err != nil {
			return StartResult{}, fmt.Errorf("create scope claim: %w", err)
		}
		claims = append(claims, claim)
	}

	payload := map[string]any{
		"intent": intent, "claims": claims, "overlaps": overlaps,
		"overlap_override": input.AllowOverlap && hasBlockingOverlap(overlaps),
	}
	eventID, err := appendEvent(
		ctx, tx, organizationID, projectID, commandID, actorID, input.SessionID,
		intent.ID, "pact.intent.started.v1", "intent", intent.ID, intent.Version,
		input.BaseRevision, payload,
	)
	if err != nil {
		return StartResult{}, err
	}
	result := StartResult{Intent: intent, Claims: claims, Overlaps: overlaps, EventID: eventID}
	if err := completeCommand(ctx, tx, organizationID, projectID, "work.start", idempotencyKey, commandID, 201, eventID, intent.ID, result); err != nil {
		return StartResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StartResult{}, fmt.Errorf("commit work.start: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) AttachWorkspace(
	ctx context.Context,
	organizationID string,
	principalID string,
	allowAll bool,
	intentID string,
	idempotencyKey string,
	requestHash [sha256.Size]byte,
	input WorkspaceInput,
) (WorkspaceResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return WorkspaceResult{}, fmt.Errorf("begin workspace.attach: %w", err)
	}
	defer rollback(tx)

	var projectID, responsibleAgentID, repositoryID string
	err = tx.QueryRow(ctx, `
		SELECT intent.project_id, intent.responsible_agent_id, project.root_repository_id
		FROM coordination.intents AS intent
		JOIN identity.projects AS project
		  ON project.organization_id = intent.organization_id AND project.id = intent.project_id
		WHERE intent.organization_id = $1 AND intent.id = $2
		  AND intent.status NOT IN ('completed', 'cancelled', 'abandoned')
		FOR UPDATE OF intent
	`, organizationID, intentID).Scan(&projectID, &responsibleAgentID, &repositoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkspaceResult{}, ErrNotFound
	}
	if err != nil {
		return WorkspaceResult{}, fmt.Errorf("load intent for workspace: %w", err)
	}
	actorID, err := authorizeSession(ctx, tx, organizationID, principalID, allowAll, projectID, input.SessionID)
	if err != nil {
		return WorkspaceResult{}, err
	}
	if actorID != responsibleAgentID && !allowAll {
		return WorkspaceResult{}, ErrForbidden
	}
	commandID, stored, replayed, err := reserveCommand(
		ctx, tx, organizationID, projectID, "workspace.attach", idempotencyKey, requestHash,
	)
	if err != nil {
		return WorkspaceResult{}, err
	}
	if replayed {
		var result WorkspaceResult
		if err := json.Unmarshal(stored, &result); err != nil {
			return WorkspaceResult{}, fmt.Errorf("decode stored workspace.attach result: %w", err)
		}
		result.Replayed = true
		return result, nil
	}

	workspace, err := scanWorkspace(tx.QueryRow(ctx, `
		INSERT INTO coordination.workspaces (
			organization_id, project_id, repository_id, intent_id, session_id,
			base_revision, path_ref, git_branch, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'ready')
		RETURNING id, project_id, repository_id, intent_id, session_id, base_revision,
		          path_ref, git_branch, status, status_detail, version, created_at,
		          updated_at, frozen_at, archived_at
	`, organizationID, projectID, repositoryID, intentID, input.SessionID,
		input.BaseRevision, input.PathRef, input.GitBranch))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return WorkspaceResult{}, ErrWorkspaceExists
		}
		return WorkspaceResult{}, fmt.Errorf("create workspace: %w", err)
	}
	payload := map[string]any{
		"workspace_id": workspace.ID, "intent_id": intentID,
		"base_revision": workspace.BaseRevision, "git_branch": workspace.GitBranch,
		"path_ref": workspace.PathRef,
	}
	eventID, err := appendEvent(
		ctx, tx, organizationID, projectID, commandID, actorID, input.SessionID,
		intentID, "pact.workspace.ready.v1", "workspace", workspace.ID,
		workspace.Version, workspace.BaseRevision, payload,
	)
	if err != nil {
		return WorkspaceResult{}, err
	}
	result := WorkspaceResult{Workspace: workspace, EventID: eventID}
	if err := completeCommand(ctx, tx, organizationID, projectID, "workspace.attach", idempotencyKey, commandID, 201, eventID, workspace.ID, result); err != nil {
		return WorkspaceResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WorkspaceResult{}, fmt.Errorf("commit workspace.attach: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) UpdateStatus(
	ctx context.Context,
	organizationID string,
	principalID string,
	allowAll bool,
	intentID string,
	idempotencyKey string,
	requestHash [sha256.Size]byte,
	input StatusInput,
) (StatusResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return StatusResult{}, fmt.Errorf("begin intent.status: %w", err)
	}
	defer rollback(tx)

	var projectID, responsibleAgentID, currentStatus string
	var currentVersion int64
	err = tx.QueryRow(ctx, `
		SELECT project_id, responsible_agent_id, status, version
		FROM coordination.intents
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE
	`, organizationID, intentID).Scan(&projectID, &responsibleAgentID, &currentStatus, &currentVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return StatusResult{}, ErrNotFound
	}
	if err != nil {
		return StatusResult{}, fmt.Errorf("load intent for status update: %w", err)
	}
	actorID, err := authorizeSession(ctx, tx, organizationID, principalID, allowAll, projectID, input.SessionID)
	if err != nil {
		return StatusResult{}, err
	}
	if actorID != responsibleAgentID && !allowAll {
		return StatusResult{}, ErrForbidden
	}
	commandID, stored, replayed, err := reserveCommand(
		ctx, tx, organizationID, projectID, "intent.status", idempotencyKey, requestHash,
	)
	if err != nil {
		return StatusResult{}, err
	}
	if replayed {
		var result StatusResult
		if err := json.Unmarshal(stored, &result); err != nil {
			return StatusResult{}, fmt.Errorf("decode stored intent.status result: %w", err)
		}
		result.Replayed = true
		return result, nil
	}
	if currentVersion != input.ExpectedVersion {
		return StatusResult{}, ErrVersionConflict
	}
	if !validTransition(currentStatus, input.Status) {
		return StatusResult{}, ErrInvalidTransition
	}

	detail, err := json.Marshal(map[string]any{"reason": input.Reason})
	if err != nil {
		return StatusResult{}, fmt.Errorf("encode intent status detail: %w", err)
	}
	intent, err := scanIntent(tx.QueryRow(ctx, `
		UPDATE coordination.intents
		SET status = $3,
		    summary = COALESCE(NULLIF($4, ''), summary),
		    status_detail = $5,
		    version = version + 1,
		    updated_at = transaction_timestamp(),
		    status_changed_at = transaction_timestamp(),
		    completed_at = CASE WHEN $3 = 'completed' THEN transaction_timestamp() ELSE NULL END
		WHERE organization_id = $1 AND id = $2 AND version = $6
		RETURNING id, project_id, title, goal, success_criteria, status, summary,
		          status_detail, base_revision, responsible_agent_id, created_by_actor_id,
		          version, created_at, updated_at, status_changed_at, completed_at
	`, organizationID, intentID, input.Status, input.Summary, detail, input.ExpectedVersion))
	if err != nil {
		return StatusResult{}, fmt.Errorf("update intent status: %w", err)
	}

	if input.Status == "submitted" {
		if _, err := tx.Exec(ctx, `
			UPDATE coordination.workspaces
			SET status = 'frozen', frozen_at = transaction_timestamp(),
			    updated_at = transaction_timestamp(), version = version + 1
			WHERE organization_id = $1 AND project_id = $2 AND intent_id = $3
			  AND status IN ('provisioning', 'ready', 'active')
		`, organizationID, projectID, intentID); err != nil {
			return StatusResult{}, fmt.Errorf("freeze intent workspace: %w", err)
		}
	}
	if isTerminalStatus(input.Status) {
		if _, err := tx.Exec(ctx, `
			UPDATE coordination.scope_claims
			SET status = 'released', released_at = transaction_timestamp(),
			    updated_at = transaction_timestamp(), version = version + 1
			WHERE organization_id = $1 AND project_id = $2 AND intent_id = $3 AND status = 'active'
		`, organizationID, projectID, intentID); err != nil {
			return StatusResult{}, fmt.Errorf("release intent scopes: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE coordination.workspaces
			SET status = 'archived', archived_at = transaction_timestamp(),
			    updated_at = transaction_timestamp(), version = version + 1
			WHERE organization_id = $1 AND project_id = $2 AND intent_id = $3
			  AND status <> 'archived'
		`, organizationID, projectID, intentID); err != nil {
			return StatusResult{}, fmt.Errorf("archive intent workspace: %w", err)
		}
	}

	eventType := "pact.intent." + input.Status + ".v1"
	payload := map[string]any{
		"intent_id": intent.ID, "title": intent.Title, "status": intent.Status,
		"summary": intent.Summary, "reason": input.Reason,
	}
	eventID, err := appendEvent(
		ctx, tx, organizationID, projectID, commandID, actorID, input.SessionID,
		intentID, eventType, "intent", intentID, intent.Version,
		intent.BaseRevision, payload,
	)
	if err != nil {
		return StatusResult{}, err
	}
	result := StatusResult{Intent: intent, EventID: eventID}
	if err := completeCommand(ctx, tx, organizationID, projectID, "intent.status", idempotencyKey, commandID, 200, eventID, intentID, result); err != nil {
		return StatusResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StatusResult{}, fmt.Errorf("commit intent.status: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) List(
	ctx context.Context,
	organizationID string,
	projectID string,
) ([]WorkItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT intent.id, intent.project_id, intent.title, intent.goal,
		       intent.success_criteria, intent.status, intent.summary, intent.status_detail,
		       intent.base_revision, intent.responsible_agent_id, intent.created_by_actor_id,
		       intent.version, intent.created_at, intent.updated_at,
		       intent.status_changed_at, intent.completed_at, actor.display_name
		FROM coordination.intents AS intent
		JOIN identity.actors AS actor
		  ON actor.organization_id = intent.organization_id AND actor.id = intent.responsible_agent_id
		WHERE intent.organization_id = $1 AND intent.project_id = $2
		ORDER BY
		  CASE WHEN intent.status IN ('completed', 'cancelled', 'abandoned') THEN 1 ELSE 0 END,
		  intent.updated_at DESC
		LIMIT 100
	`, organizationID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list work intents: %w", err)
	}
	defer rows.Close()

	items := make([]WorkItem, 0)
	itemIndexByIntent := make(map[string]int)
	intentIDs := make([]string, 0)
	for rows.Next() {
		intent, name, err := scanIntentWithName(rows)
		if err != nil {
			return nil, fmt.Errorf("list work intents: %w", err)
		}
		items = append(items, WorkItem{Intent: intent, ResponsibleName: name, Scopes: make([]ScopeClaim, 0)})
		itemIndexByIntent[intent.ID] = len(items) - 1
		intentIDs = append(intentIDs, intent.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list work intents: %w", err)
	}
	if len(intentIDs) == 0 {
		return items, nil
	}

	workspaceRows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (workspace.intent_id)
		       workspace.intent_id,
		       workspace.id, workspace.project_id, workspace.repository_id,
		       workspace.intent_id, workspace.session_id, workspace.base_revision,
		       workspace.path_ref, workspace.git_branch, workspace.status,
		       workspace.status_detail, workspace.version, workspace.created_at,
		       workspace.updated_at, workspace.frozen_at, workspace.archived_at,
		       session.status = 'active'
		         AND session.expires_at > transaction_timestamp()
		         AND session.last_seen_at >= transaction_timestamp() - interval '30 seconds',
		       session.last_seen_at
		FROM coordination.workspaces AS workspace
		LEFT JOIN identity.sessions AS session
		  ON session.organization_id = workspace.organization_id
		 AND session.project_id = workspace.project_id
		 AND session.id = workspace.session_id
		WHERE workspace.organization_id = $1 AND workspace.project_id = $2
		  AND workspace.intent_id = ANY($3::uuid[])
		ORDER BY workspace.intent_id, workspace.created_at DESC
	`, organizationID, projectID, intentIDs)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	for workspaceRows.Next() {
		var intentID string
		workspace, live, lastSeen, err := scanWorkspaceWithPresence(workspaceRows, &intentID)
		if err != nil {
			workspaceRows.Close()
			return nil, fmt.Errorf("list workspaces: %w", err)
		}
		if index, ok := itemIndexByIntent[intentID]; ok {
			items[index].Workspace = &workspace
			items[index].SessionLive = live
			items[index].SessionLastSeen = lastSeen
		}
	}
	if err := workspaceRows.Err(); err != nil {
		workspaceRows.Close()
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	workspaceRows.Close()

	claimRows, err := r.pool.Query(ctx, `
		SELECT claim.intent_id, claim.id, claim.intent_id, claim.session_id,
		       claim.origin, claim.claim_mode, claim.status, claim.version,
		       claim.created_at, claim.updated_at, claim.last_renewed_at, claim.expires_at,
		       resource.id, resource.repository_id, resource.kind, resource.locator
		FROM coordination.scope_claims AS claim
		JOIN coordination.resource_refs AS resource
		  ON resource.organization_id = claim.organization_id
		 AND resource.project_id = claim.project_id
		 AND resource.id = claim.resource_ref_id
		WHERE claim.organization_id = $1 AND claim.project_id = $2
		  AND claim.intent_id = ANY($3::uuid[])
		ORDER BY claim.created_at, claim.id
	`, organizationID, projectID, intentIDs)
	if err != nil {
		return nil, fmt.Errorf("list work scopes: %w", err)
	}
	defer claimRows.Close()
	for claimRows.Next() {
		var intentID string
		var claim ScopeClaim
		if err := claimRows.Scan(
			&intentID, &claim.ID, &claim.IntentID, &claim.SessionID,
			&claim.Origin, &claim.Mode, &claim.Status, &claim.Version,
			&claim.CreatedAt, &claim.UpdatedAt, &claim.LastRenewedAt, &claim.ExpiresAt,
			&claim.Resource.ID, &claim.Resource.RepositoryID, &claim.Resource.Kind, &claim.Resource.Locator,
		); err != nil {
			return nil, fmt.Errorf("list work scopes: %w", err)
		}
		if index, ok := itemIndexByIntent[intentID]; ok {
			items[index].Scopes = append(items[index].Scopes, claim)
		}
	}
	if err := claimRows.Err(); err != nil {
		return nil, fmt.Errorf("list work scopes: %w", err)
	}
	return items, nil
}

type existingClaim struct {
	ID, IntentID, IntentTitle, IntentStatus, ActorID, ActorName string
	Scope                                                       ScopeInput
	ExpiresAt                                                   time.Time
}

func loadScopeOverlaps(
	ctx context.Context,
	database interface {
		Query(context.Context, string, ...any) (pgx.Rows, error)
	},
	organizationID string,
	projectID string,
	repositoryID string,
	requested []ScopeInput,
) ([]ScopeOverlap, error) {
	rows, err := database.Query(ctx, `
		SELECT claim.id, claim.intent_id, intent.title, intent.status,
		       intent.responsible_agent_id, actor.display_name,
		       resource.kind, resource.locator, claim.claim_mode, claim.expires_at
		FROM coordination.scope_claims AS claim
		JOIN coordination.resource_refs AS resource
		  ON resource.organization_id = claim.organization_id
		 AND resource.project_id = claim.project_id
		 AND resource.id = claim.resource_ref_id
		JOIN coordination.intents AS intent
		  ON intent.organization_id = claim.organization_id
		 AND intent.project_id = claim.project_id
		 AND intent.id = claim.intent_id
		JOIN identity.actors AS actor
		  ON actor.organization_id = intent.organization_id
		 AND actor.id = intent.responsible_agent_id
		WHERE claim.organization_id = $1 AND claim.project_id = $2
		  AND claim.status = 'active' AND claim.expires_at > transaction_timestamp()
		  AND resource.repository_id = $3
		  AND intent.status NOT IN ('completed', 'cancelled', 'abandoned')
		ORDER BY claim.created_at, claim.id
	`, organizationID, projectID, repositoryID)
	if err != nil {
		return nil, fmt.Errorf("load active scope claims: %w", err)
	}
	defer rows.Close()
	existing := make([]existingClaim, 0)
	for rows.Next() {
		var claim existingClaim
		if err := rows.Scan(
			&claim.ID, &claim.IntentID, &claim.IntentTitle, &claim.IntentStatus,
			&claim.ActorID, &claim.ActorName, &claim.Scope.Kind, &claim.Scope.Locator,
			&claim.Scope.Mode, &claim.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("load active scope claims: %w", err)
		}
		existing = append(existing, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load active scope claims: %w", err)
	}

	overlaps := make([]ScopeOverlap, 0)
	for _, candidate := range requested {
		for _, active := range existing {
			if !scopesOverlap(candidate, active.Scope) {
				continue
			}
			blocking := candidate.Mode == ClaimModeExclusive || active.Scope.Mode == ClaimModeExclusive
			overlaps = append(overlaps, ScopeOverlap{
				Requested: candidate, ExistingClaimID: active.ID,
				ExistingIntentID: active.IntentID, ExistingTitle: active.IntentTitle,
				ExistingStatus: active.IntentStatus, ExistingActorID: active.ActorID,
				ExistingActor: active.ActorName, ExistingScope: active.Scope,
				Blocking: blocking, Reason: overlapReason(candidate, active.Scope),
				ExpiresAt: active.ExpiresAt,
			})
		}
	}
	return overlaps, nil
}

func scopesOverlap(left, right ScopeInput) bool {
	if left.Kind == "repository" || right.Kind == "repository" {
		return true
	}
	if left.Kind == "file" && right.Kind == "file" {
		return left.Locator == right.Locator
	}
	if left.Kind == "path" && right.Kind == "path" {
		return pathContains(left.Locator, right.Locator) || pathContains(right.Locator, left.Locator)
	}
	if left.Kind == "path" {
		return pathContains(left.Locator, right.Locator)
	}
	return pathContains(right.Locator, left.Locator)
}

func pathContains(parent, child string) bool {
	return parent == "." || child == parent || strings.HasPrefix(child, parent+"/")
}

func overlapReason(left, right ScopeInput) string {
	if left.Kind == "repository" || right.Kind == "repository" {
		return "repository_scope"
	}
	if left.Locator == right.Locator {
		return "exact_scope"
	}
	return "path_hierarchy"
}

func hasBlockingOverlap(overlaps []ScopeOverlap) bool {
	for _, overlap := range overlaps {
		if overlap.Blocking {
			return true
		}
	}
	return false
}

func validTransition(from, to string) bool {
	if from == to {
		return false
	}
	allowed := map[string]map[string]bool{
		"draft":     {"active": true, "cancelled": true, "abandoned": true},
		"active":    {"blocked": true, "submitted": true, "cancelled": true, "abandoned": true},
		"blocked":   {"active": true, "submitted": true, "cancelled": true, "abandoned": true},
		"submitted": {"completed": true, "blocked": true, "cancelled": true},
	}
	return allowed[from][to]
}

func isTerminalStatus(status string) bool {
	return status == "completed" || status == "cancelled" || status == "abandoned"
}

func authorizeSession(
	ctx context.Context,
	tx pgx.Tx,
	organizationID, principalID string,
	allowAll bool,
	projectID, sessionID string,
) (string, error) {
	var actorID string
	err := tx.QueryRow(ctx, `
		SELECT session.actor_id
		FROM identity.sessions AS session
		JOIN identity.agents AS agent
		  ON agent.organization_id = session.organization_id AND agent.id = session.actor_id
		WHERE session.organization_id = $1 AND session.project_id = $2 AND session.id = $3
		  AND session.status = 'active' AND session.expires_at > transaction_timestamp()
		  AND ($5 OR agent.sponsor_principal_id = $4)
		FOR UPDATE OF session
	`, organizationID, projectID, sessionID, principalID, allowAll).Scan(&actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("authorize coordination session: %w", err)
	}
	return actorID, nil
}

func lockProjectCoordination(ctx context.Context, tx pgx.Tx, organizationID, projectID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, organizationID+":"+projectID); err != nil {
		return fmt.Errorf("lock project coordination: %w", err)
	}
	return nil
}

func upsertResourceRef(
	ctx context.Context,
	tx pgx.Tx,
	organizationID, projectID, repositoryID string,
	scope ScopeInput,
) (ResourceRef, error) {
	var resource ResourceRef
	err := tx.QueryRow(ctx, `
		INSERT INTO coordination.resource_refs (
			organization_id, project_id, repository_id, kind, locator
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT ON CONSTRAINT resource_refs_identity_uq
		DO UPDATE SET locator = EXCLUDED.locator
		RETURNING id, repository_id, kind, locator
	`, organizationID, projectID, repositoryID, scope.Kind, scope.Locator).Scan(
		&resource.ID, &resource.RepositoryID, &resource.Kind, &resource.Locator,
	)
	if err != nil {
		return ResourceRef{}, fmt.Errorf("upsert scope resource: %w", err)
	}
	return resource, nil
}

func reserveCommand(
	ctx context.Context,
	tx pgx.Tx,
	organizationID, projectID, commandType, idempotencyKey string,
	requestHash [sha256.Size]byte,
) (string, json.RawMessage, bool, error) {
	var commandID string
	err := tx.QueryRow(ctx, `
		INSERT INTO platform.idempotency_records (
			organization_id, project_id, command_type, idempotency_key, request_hash
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT ON CONSTRAINT idempotency_records_scope_key_uq DO NOTHING
		RETURNING command_id
	`, organizationID, projectID, commandType, idempotencyKey, requestHash[:]).Scan(&commandID)
	if err == nil {
		return commandID, nil, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, false, fmt.Errorf("reserve %s idempotency key: %w", commandType, err)
	}
	var storedHash []byte
	var outcome *string
	var body json.RawMessage
	err = tx.QueryRow(ctx, `
		SELECT request_hash, outcome, response_body
		FROM platform.idempotency_records
		WHERE organization_id = $1 AND project_id = $2
		  AND command_type = $3 AND idempotency_key = $4
	`, organizationID, projectID, commandType, idempotencyKey).Scan(&storedHash, &outcome, &body)
	if err != nil {
		return "", nil, false, fmt.Errorf("load stored %s result: %w", commandType, err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return "", nil, false, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(body) == 0 {
		return "", nil, false, ErrCommandIncomplete
	}
	return "", body, true, nil
}

func completeCommand(
	ctx context.Context,
	tx pgx.Tx,
	organizationID, projectID, commandType, idempotencyKey, commandID string,
	status int,
	eventID, aggregateID string,
	result any,
) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode %s response: %w", commandType, err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET outcome = 'succeeded', response_status = $6, response_body = $7,
		    event_id = $8, aggregate_id = $9, completed_at = transaction_timestamp()
		WHERE organization_id = $1 AND project_id = $2 AND command_type = $3
		  AND idempotency_key = $4 AND command_id = $5
	`, organizationID, projectID, commandType, idempotencyKey, commandID,
		status, body, eventID, aggregateID)
	if err != nil {
		return fmt.Errorf("store %s result: %w", commandType, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("store %s result: idempotency reservation disappeared", commandType)
	}
	return nil
}

func appendEvent(
	ctx context.Context,
	tx pgx.Tx,
	organizationID, projectID, commandID, actorID, sessionID, intentID string,
	eventType, aggregateType, aggregateID string,
	aggregateVersion int64,
	gitRevision string,
	payload any,
) (string, error) {
	var sequence int64
	err := tx.QueryRow(ctx, `
		WITH next_sequence AS (
			UPDATE platform.project_event_counters
			SET last_sequence = last_sequence + 1, updated_at = transaction_timestamp()
			WHERE organization_id = $1 AND project_id = $2
			RETURNING last_sequence
		)
		UPDATE identity.projects AS project
		SET event_sequence = next_sequence.last_sequence
		FROM next_sequence
		WHERE project.organization_id = $1 AND project.id = $2
		RETURNING next_sequence.last_sequence
	`, organizationID, projectID).Scan(&sequence)
	if err != nil {
		return "", fmt.Errorf("allocate coordination event sequence: %w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode coordination event: %w", err)
	}
	var eventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.events (
			organization_id, project_id, project_sequence, event_type, event_version,
			aggregate_type, aggregate_id, aggregate_version, actor_id, session_id,
			intent_id, command_id, correlation_id, git_revision, payload, payload_hash
		)
		VALUES (
			$1, $2, $3, $4, 1, $5, $6, $7, $8, $9, $10, $11, $11,
			NULLIF($12, ''), $13, sha256(convert_to(($13::jsonb)::text, 'UTF8'))
		)
		RETURNING id
	`, organizationID, projectID, sequence, eventType, aggregateType, aggregateID,
		aggregateVersion, actorID, sessionID, intentID, commandID, gitRevision, body).Scan(&eventID)
	if err != nil {
		return "", fmt.Errorf("append %s event: %w", eventType, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform.outbox (organization_id, project_id, event_id, channel)
		VALUES ($1, $2, $3, 'project-events')
	`, organizationID, projectID, eventID); err != nil {
		return "", fmt.Errorf("enqueue %s event: %w", eventType, err)
	}
	return eventID, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanIntent(row rowScanner) (Intent, error) {
	var intent Intent
	var criteria, detail json.RawMessage
	err := row.Scan(
		&intent.ID, &intent.ProjectID, &intent.Title, &intent.Goal, &criteria,
		&intent.Status, &intent.Summary, &detail, &intent.BaseRevision,
		&intent.ResponsibleAgentID, &intent.CreatedByActorID, &intent.Version,
		&intent.CreatedAt, &intent.UpdatedAt, &intent.StatusChangedAt, &intent.CompletedAt,
	)
	if err != nil {
		return Intent{}, err
	}
	if err := json.Unmarshal(criteria, &intent.SuccessCriteria); err != nil {
		return Intent{}, err
	}
	if err := json.Unmarshal(detail, &intent.StatusDetail); err != nil {
		return Intent{}, err
	}
	if intent.SuccessCriteria == nil {
		intent.SuccessCriteria = make([]string, 0)
	}
	if intent.StatusDetail == nil {
		intent.StatusDetail = make(map[string]any)
	}
	return intent, nil
}

func scanIntentWithName(row rowScanner) (Intent, string, error) {
	var intent Intent
	var criteria, detail json.RawMessage
	var name string
	err := row.Scan(
		&intent.ID, &intent.ProjectID, &intent.Title, &intent.Goal, &criteria,
		&intent.Status, &intent.Summary, &detail, &intent.BaseRevision,
		&intent.ResponsibleAgentID, &intent.CreatedByActorID, &intent.Version,
		&intent.CreatedAt, &intent.UpdatedAt, &intent.StatusChangedAt,
		&intent.CompletedAt, &name,
	)
	if err != nil {
		return Intent{}, "", err
	}
	if err := json.Unmarshal(criteria, &intent.SuccessCriteria); err != nil {
		return Intent{}, "", err
	}
	if err := json.Unmarshal(detail, &intent.StatusDetail); err != nil {
		return Intent{}, "", err
	}
	if intent.SuccessCriteria == nil {
		intent.SuccessCriteria = make([]string, 0)
	}
	if intent.StatusDetail == nil {
		intent.StatusDetail = make(map[string]any)
	}
	return intent, name, nil
}

func scanScopeClaim(row rowScanner, resource ResourceRef) (ScopeClaim, error) {
	var claim ScopeClaim
	err := row.Scan(
		&claim.ID, &claim.IntentID, &claim.SessionID, &claim.Origin,
		&claim.Mode, &claim.Status, &claim.Version, &claim.CreatedAt,
		&claim.UpdatedAt, &claim.LastRenewedAt, &claim.ExpiresAt,
	)
	claim.Resource = resource
	return claim, err
}

func scanWorkspace(row rowScanner) (Workspace, error) {
	var workspace Workspace
	var detail json.RawMessage
	err := row.Scan(
		&workspace.ID, &workspace.ProjectID, &workspace.RepositoryID,
		&workspace.IntentID, &workspace.SessionID, &workspace.BaseRevision,
		&workspace.PathRef, &workspace.GitBranch, &workspace.Status, &detail,
		&workspace.Version, &workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.FrozenAt, &workspace.ArchivedAt,
	)
	if err != nil {
		return Workspace{}, err
	}
	if err := json.Unmarshal(detail, &workspace.StatusDetail); err != nil {
		return Workspace{}, err
	}
	if workspace.StatusDetail == nil {
		workspace.StatusDetail = make(map[string]any)
	}
	return workspace, nil
}

func scanWorkspaceWithPresence(row rowScanner, intentID *string) (Workspace, bool, *time.Time, error) {
	var workspace Workspace
	var detail json.RawMessage
	var live bool
	var lastSeen *time.Time
	err := row.Scan(
		intentID, &workspace.ID, &workspace.ProjectID, &workspace.RepositoryID,
		&workspace.IntentID, &workspace.SessionID, &workspace.BaseRevision,
		&workspace.PathRef, &workspace.GitBranch, &workspace.Status, &detail,
		&workspace.Version, &workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.FrozenAt, &workspace.ArchivedAt, &live, &lastSeen,
	)
	if err != nil {
		return Workspace{}, false, nil, err
	}
	if err := json.Unmarshal(detail, &workspace.StatusDetail); err != nil {
		return Workspace{}, false, nil, err
	}
	if workspace.StatusDetail == nil {
		workspace.StatusDetail = make(map[string]any)
	}
	return workspace, live, lastSeen, nil
}

func rollback(tx pgx.Tx) {
	_ = tx.Rollback(context.Background())
}
