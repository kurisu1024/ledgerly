// Package postgres is a Postgres-backed implementation of the storage.Storage
// and storage.RuleStore interfaces, using db/schema.sql. See CONTEXT.md and
// issue #25 for the design rationale (authoritative JSONB events, keyed by
// tenant_id/chain_id/position).
package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurisu1024/ledgerly/internal/storage"
)

// Storage is a Postgres-backed implementation of storage.Storage.
type Storage struct {
	pool *pgxpool.Pool
}

var _ storage.Storage = (*Storage)(nil)

// New creates a Postgres-backed Storage that reads/writes through pool.
// pool must already point at a schema with db/schema.sql applied.
func New(pool *pgxpool.Pool) *Storage {
	return &Storage{pool: pool}
}

// WriteBlock persists an event block for a specific tenant.
func (s *Storage) WriteBlock(ctx context.Context, tenantID uuid.UUID, block storage.Block) error {
	return fmt.Errorf("postgres: WriteBlock not implemented")
}

// FetchBlock retrieves a specific block by tenant ID and block ID.
func (s *Storage) FetchBlock(ctx context.Context, tenantID uuid.UUID, blockID uuid.UUID) (storage.Block, error) {
	return storage.Block{}, fmt.Errorf("postgres: FetchBlock not implemented")
}

// FetchBlocks retrieves all blocks for a tenant, with optional filtering.
func (s *Storage) FetchBlocks(ctx context.Context, tenantID uuid.UUID, opts storage.FetchOptions) ([]storage.Block, error) {
	return nil, fmt.Errorf("postgres: FetchBlocks not implemented")
}
