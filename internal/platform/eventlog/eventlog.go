package eventlog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	ID               string          `json:"id"`
	ProjectID        string          `json:"project_id"`
	ProjectSequence  int64           `json:"project_sequence"`
	Type             string          `json:"type"`
	Version          int16           `json:"version"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      string          `json:"aggregate_id"`
	AggregateVersion int64           `json:"aggregate_version"`
	CommandID        string          `json:"command_id"`
	CorrelationID    string          `json:"correlation_id"`
	ActorID          *string         `json:"actor_id,omitempty"`
	SessionID        *string         `json:"session_id,omitempty"`
	IntentID         *string         `json:"intent_id,omitempty"`
	CausationID      *string         `json:"causation_id,omitempty"`
	OccurredAt       time.Time       `json:"occurred_at"`
	RecordedAt       time.Time       `json:"recorded_at"`
	Payload          json.RawMessage `json:"payload"`
}

type Reader interface {
	List(context.Context, string, string, int64, int) ([]Event, error)
}

// HistoryReader adds reverse cursor pagination for human-facing activity
// browsers. Reader remains intentionally small so existing event consumers do
// not need to depend on presentation-oriented history queries.
type HistoryReader interface {
	ListRecent(context.Context, string, string, *int64, int, string) ([]Event, error)
}

type PostgresReader struct {
	pool *pgxpool.Pool
}

func NewPostgresReader(pool *pgxpool.Pool) *PostgresReader {
	return &PostgresReader{pool: pool}
}

func (r *PostgresReader) List(
	ctx context.Context,
	organizationID string,
	projectID string,
	after int64,
	limit int,
) ([]Event, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			project_id,
			project_sequence,
			event_type,
			event_version,
			aggregate_type,
			aggregate_id,
			aggregate_version,
			command_id,
			correlation_id,
			actor_id,
			session_id,
			intent_id,
			causation_id,
			occurred_at,
			recorded_at,
			payload
		FROM platform.events
		WHERE organization_id = $1
		  AND project_id = $2
		  AND project_sequence > $3
		ORDER BY project_sequence
		LIMIT $4
	`, organizationID, projectID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("list project events: %w", err)
	}
	defer rows.Close()

	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		var event Event
		err := row.Scan(
			&event.ID,
			&event.ProjectID,
			&event.ProjectSequence,
			&event.Type,
			&event.Version,
			&event.AggregateType,
			&event.AggregateID,
			&event.AggregateVersion,
			&event.CommandID,
			&event.CorrelationID,
			&event.ActorID,
			&event.SessionID,
			&event.IntentID,
			&event.CausationID,
			&event.OccurredAt,
			&event.RecordedAt,
			&event.Payload,
		)
		return event, err
	})
	if err != nil {
		return nil, fmt.Errorf("read project events: %w", err)
	}
	return events, nil
}

func (r *PostgresReader) ListRecent(
	ctx context.Context,
	organizationID string,
	projectID string,
	before *int64,
	limit int,
	query string,
) ([]Event, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			project_id,
			project_sequence,
			event_type,
			event_version,
			aggregate_type,
			aggregate_id,
			aggregate_version,
			command_id,
			correlation_id,
			actor_id,
			session_id,
			intent_id,
			causation_id,
			occurred_at,
			recorded_at,
			payload
		FROM platform.events
		WHERE organization_id = $1
		  AND project_id = $2
		  AND ($3::bigint IS NULL OR project_sequence < $3)
		  AND (
			$4 = '' OR position(lower($4) in lower(concat_ws(
				' ', id::text, project_sequence::text, event_type, aggregate_type,
				aggregate_id::text, command_id::text, correlation_id::text,
				actor_id::text, session_id::text, intent_id::text, causation_id::text,
				payload::text
			))) > 0
		  )
		ORDER BY project_sequence DESC
		LIMIT $5
	`, organizationID, projectID, before, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent project events: %w", err)
	}
	defer rows.Close()

	events, err := pgx.CollectRows(rows, scanEvent)
	if err != nil {
		return nil, fmt.Errorf("read recent project events: %w", err)
	}
	return events, nil
}

func scanEvent(row pgx.CollectableRow) (Event, error) {
	var event Event
	err := row.Scan(
		&event.ID,
		&event.ProjectID,
		&event.ProjectSequence,
		&event.Type,
		&event.Version,
		&event.AggregateType,
		&event.AggregateID,
		&event.AggregateVersion,
		&event.CommandID,
		&event.CorrelationID,
		&event.ActorID,
		&event.SessionID,
		&event.IntentID,
		&event.CausationID,
		&event.OccurredAt,
		&event.RecordedAt,
		&event.Payload,
	)
	return event, err
}
