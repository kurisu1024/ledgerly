package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context) (*pgxpool.Pool, error) {

	config, _ := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/ledgerly")

	config.MaxConns = 10 // local dev: enough for concurrent requests
	config.MinConns = 2  // pre-open 2 connections
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 1 * time.Minute

	return pgxpool.NewWithConfig(context.Background(), config)

}
