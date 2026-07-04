// Package db opens pgx connection pools for the Postgres storage backend.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pingTimeout bounds NewPool's startup ping so an unreachable host fails
// fast even when the caller passes an unbounded ctx.
const pingTimeout = 10 * time.Second

// NewPool opens a pgx connection pool against dsn. The caller owns the pool
// and must Close it; ctx bounds the initial connection attempt, and the
// startup ping is additionally capped at pingTimeout.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}

	config.MaxConns = 10 // local dev: enough for concurrent requests
	config.MinConns = 2  // pre-open 2 connections
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	// pgxpool connects lazily; ping so an unreachable database or bad
	// credentials fail fast at startup instead of on the first query. The
	// ping carries its own timeout so "fail fast" holds regardless of the
	// caller's ctx.
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	return pool, nil
}
