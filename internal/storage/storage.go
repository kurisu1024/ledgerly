package storage

import (
	"context"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

// Storage defines the interface for persisting and retrieving event blocks.
// Implementations can be backed by in-memory storage, MongoDB, PostgreSQL, etc.
type Storage interface {
	// WriteBlock persists an event block for a specific tenant.
	WriteBlock(ctx context.Context, tenantID uuid.UUID, block Block) error

	// FetchBlock retrieves a specific block by tenant ID and block ID.
	FetchBlock(ctx context.Context, tenantID uuid.UUID, blockID uuid.UUID) (Block, error)

	// FetchBlocks retrieves all blocks for a tenant, with optional filtering.
	FetchBlocks(ctx context.Context, tenantID uuid.UUID, opts FetchOptions) ([]Block, error)
}

// Block represents a collection of audit events, typically an event chain.
type Block struct {
	ID       uuid.UUID        `json:"id"`
	TenantID uuid.UUID        `json:"tenant-id"`
	Chain    audit.EventChain `json:"chain"`
}

// FetchOptions provides filtering options when fetching blocks.
type FetchOptions struct {
	// BlockIDs filters results to only include these block IDs.
	// If empty, all blocks for the tenant are returned.
	BlockIDs []uuid.UUID
	// Limit restricts the number of blocks returned.
	// If 0, no limit is applied.
	Limit int
}
