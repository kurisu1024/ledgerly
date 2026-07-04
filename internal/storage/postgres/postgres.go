// Package postgres is a Postgres-backed implementation of the storage.Storage
// and storage.RuleStore interfaces, using db/schema.sql. See CONTEXT.md and
// issue #25 for the design rationale (authoritative JSONB events, keyed by
// tenant_id/chain_id/position).
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurisu1024/ledgerly/internal/audit"
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

// WriteBlock persists an event block for a specific tenant. Every event in
// the block's chain is stored as one JSONB row keyed by
// (tenant_id, chain_id, position), all inside a single transaction — a block
// is either fully persisted or not at all. The stored JSONB is the
// authoritative marshaling of audit.Event, preserving the
// nanosecond-precision OccurredAt that chain verification hashes over.
func (s *Storage) WriteBlock(ctx context.Context, tenantID uuid.UUID, block storage.Block) error {
	// Ensure the block belongs to the tenant it is written for, matching
	// the in-memory backend: the tenantID argument is authoritative.
	block.TenantID = tenantID

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning WriteBlock transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	for i, e := range block.Chain.Events {
		payload, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshaling event %d of chain %s: %w", i, block.ID, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO audit_events (tenant_id, chain_id, position, event) VALUES ($1, $2, $3, $4)`,
			tenantID, block.ID, i, payload,
		); err != nil {
			return fmt.Errorf("inserting event %d of chain %s: %w", i, block.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing WriteBlock transaction: %w", err)
	}
	return nil
}

// FetchBlock retrieves a specific block by tenant ID and block ID.
func (s *Storage) FetchBlock(ctx context.Context, tenantID uuid.UUID, blockID uuid.UUID) (storage.Block, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT event FROM audit_events WHERE tenant_id = $1 AND chain_id = $2 ORDER BY position`,
		tenantID, blockID,
	)
	if err != nil {
		return storage.Block{}, fmt.Errorf("querying block %s for tenant %s: %w", blockID, tenantID, err)
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return storage.Block{}, fmt.Errorf("reading block %s for tenant %s: %w", blockID, tenantID, err)
	}
	if len(events) == 0 {
		return storage.Block{}, fmt.Errorf("block %s not found for tenant %s", blockID, tenantID)
	}

	return assembleBlock(tenantID, blockID, events), nil
}

// FetchBlocks retrieves all blocks for a tenant, with optional filtering.
// Rows are read in seq (insertion) order and grouped into blocks in Go;
// FetchOptions.BlockIDs and Limit are applied with the same semantics as the
// in-memory backend. The query is always tenant-scoped — no block is ever
// fetched by chain ID alone.
func (s *Storage) FetchBlocks(ctx context.Context, tenantID uuid.UUID, opts storage.FetchOptions) ([]storage.Block, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT chain_id, event FROM audit_events WHERE tenant_id = $1 ORDER BY seq`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying blocks for tenant %s: %w", tenantID, err)
	}
	defer rows.Close()

	// Group rows into per-chain event slices, remembering first-seen order.
	eventsByChain := make(map[uuid.UUID][]audit.Event)
	var chainOrder []uuid.UUID
	for rows.Next() {
		var chainID uuid.UUID
		var payload []byte
		if err := rows.Scan(&chainID, &payload); err != nil {
			return nil, fmt.Errorf("scanning event row for tenant %s: %w", tenantID, err)
		}
		var e audit.Event
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("unmarshaling event in chain %s: %w", chainID, err)
		}
		if _, seen := eventsByChain[chainID]; !seen {
			chainOrder = append(chainOrder, chainID)
		}
		eventsByChain[chainID] = append(eventsByChain[chainID], e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading blocks for tenant %s: %w", tenantID, err)
	}

	// Apply FetchOptions with the in-memory backend's semantics: when
	// BlockIDs is set, return requested blocks in the requested order,
	// skipping unknown IDs; Limit caps the number of blocks either way.
	ids := chainOrder
	if len(opts.BlockIDs) > 0 {
		ids = opts.BlockIDs
	}

	blocks := make([]storage.Block, 0, len(ids))
	for _, id := range ids {
		events, ok := eventsByChain[id]
		if !ok {
			continue
		}
		blocks = append(blocks, assembleBlock(tenantID, id, events))
		if opts.Limit > 0 && len(blocks) >= opts.Limit {
			break
		}
	}
	return blocks, nil
}

// scanEvents unmarshals every JSONB row into an audit.Event.
func scanEvents(rows pgx.Rows) ([]audit.Event, error) {
	var events []audit.Event
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		var e audit.Event
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("unmarshaling event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// assembleBlock rebuilds a storage.Block from a chain's stored events. The
// chain's own ID comes from the events themselves (AppendEvent stamps every
// event with its ChainID), so a block whose ID differs from its chain's ID
// round-trips faithfully.
func assembleBlock(tenantID, blockID uuid.UUID, events []audit.Event) storage.Block {
	chainID := blockID
	if len(events) > 0 {
		chainID = events[0].ChainID
	}
	return storage.Block{
		ID:       blockID,
		TenantID: tenantID,
		Chain:    audit.EventChain{ID: chainID, Events: events},
	}
}
