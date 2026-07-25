package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	ApplicationName  string
	StatementTimeout time.Duration
	LockTimeout      time.Duration
}

func Open(ctx context.Context, databaseURL string, options Config) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}

	cfg.ConnConfig.RuntimeParams["application_name"] = options.ApplicationName
	if options.StatementTimeout > 0 {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = options.StatementTimeout.String()
	}
	if options.LockTimeout > 0 {
		cfg.ConnConfig.RuntimeParams["lock_timeout"] = options.LockTimeout.String()
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	return pool, nil
}
