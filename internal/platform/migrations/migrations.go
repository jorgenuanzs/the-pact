package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const advisoryLockID int64 = 0x50414354

//go:embed sql/*.sql
var migrationFiles embed.FS

type Applied struct {
	Version   string    `json:"version"`
	Checksum  string    `json:"checksum"`
	AppliedAt time.Time `json:"applied_at"`
}

type definition struct {
	Filename string
	Version  string
	Checksum string
	Body     []byte
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

const createLedgerSQL = `
	CREATE TABLE IF NOT EXISTS public.pact_schema_migrations (
		version text PRIMARY KEY,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT transaction_timestamp()
	)
`

func Up(ctx context.Context, pool *pgxpool.Pool) (returnErr error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	releaseConnection := true
	defer func() {
		if releaseConnection {
			conn.Release()
		}
	}()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		var unlocked bool
		unlockErr := conn.QueryRow(
			unlockCtx,
			`SELECT pg_advisory_unlock($1)`,
			advisoryLockID,
		).Scan(&unlocked)
		if unlockErr == nil && unlocked {
			return
		}

		raw := conn.Hijack()
		releaseConnection = false
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer closeCancel()
		closeErr := raw.Close(closeCtx)
		if unlockErr == nil {
			unlockErr = errors.New("database reported that the migration lock was not held")
		}
		returnErr = errors.Join(
			returnErr,
			fmt.Errorf("unlock migrations: %w", unlockErr),
			closeErr,
		)
	}()

	if _, err := conn.Exec(ctx, createLedgerSQL); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	definitions, err := embeddedDefinitions()
	if err != nil {
		return err
	}
	applied, err := readApplied(ctx, conn)
	if err != nil {
		return err
	}
	if err := validateHistory(definitions, applied, false); err != nil {
		return err
	}

	for _, migration := range definitions[len(applied):] {
		if err := applyDefinition(ctx, conn.Conn(), migration); err != nil {
			return err
		}
	}

	return nil
}

func Status(ctx context.Context, pool *pgxpool.Pool) ([]Applied, error) {
	if _, err := pool.Exec(ctx, createLedgerSQL); err != nil {
		return nil, fmt.Errorf("create migration ledger: %w", err)
	}
	return readApplied(ctx, pool)
}

func Verify(ctx context.Context, pool *pgxpool.Pool) error {
	definitions, err := embeddedDefinitions()
	if err != nil {
		return err
	}
	applied, err := readApplied(ctx, pool)
	if err != nil {
		return err
	}
	return validateHistory(definitions, applied, true)
}

func embeddedDefinitions() ([]definition, error) {
	files, err := fs.Glob(migrationFiles, "sql/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list embedded migrations: %w", err)
	}
	sort.Strings(files)

	definitions := make([]definition, 0, len(files))
	for _, filename := range files {
		body, err := migrationFiles.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", filename, err)
		}
		version := strings.TrimSuffix(path.Base(filename), path.Ext(filename))
		sum := sha256.Sum256(body)
		definitions = append(definitions, definition{
			Filename: filename,
			Version:  version,
			Checksum: hex.EncodeToString(sum[:]),
			Body:     body,
		})
	}
	return definitions, nil
}

func readApplied(ctx context.Context, queryer querier) ([]Applied, error) {
	rows, err := queryer.Query(ctx, `
		SELECT version, checksum, applied_at
		FROM public.pact_schema_migrations
		ORDER BY version
	`)
	if err != nil {
		return nil, fmt.Errorf("query migration status: %w", err)
	}
	defer rows.Close()

	applied, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Applied])
	if err != nil {
		return nil, fmt.Errorf("read migration status: %w", err)
	}
	return applied, nil
}

func validateHistory(definitions []definition, applied []Applied, exact bool) error {
	if len(applied) > len(definitions) {
		return fmt.Errorf(
			"database has %d migrations but this binary knows only %d; refusing to run an older binary against a newer schema",
			len(applied),
			len(definitions),
		)
	}

	for index, current := range applied {
		expected := definitions[index]
		if current.Version != expected.Version {
			return fmt.Errorf(
				"database migration history is not a prefix of this binary: position %d has %s, expected %s",
				index+1,
				current.Version,
				expected.Version,
			)
		}
		if current.Checksum != expected.Checksum {
			return fmt.Errorf(
				"database migration %s has checksum %s, expected %s",
				current.Version,
				current.Checksum,
				expected.Checksum,
			)
		}
	}

	if exact && len(applied) != len(definitions) {
		return fmt.Errorf(
			"database has %d of %d required migrations applied",
			len(applied),
			len(definitions),
		)
	}
	return nil
}

func applyDefinition(ctx context.Context, conn *pgx.Conn, migration definition) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.Version, err)
	}
	defer rollback(tx)

	if _, err := tx.Exec(ctx, string(migration.Body)); err != nil {
		return fmt.Errorf("execute migration %s: %w", migration.Version, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO public.pact_schema_migrations (version, checksum)
		VALUES ($1, $2)
	`, migration.Version, migration.Checksum); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.Version, err)
	}

	return nil
}

func rollback(tx pgx.Tx) {
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackCtx)
}
