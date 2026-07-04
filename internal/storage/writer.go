package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

// DefaultWriteTimeout bounds a single ChainWriter storage write when the
// caller does not specify one. Generous — a write this slow is already
// pathological — but finite, so a hung backend can never block the worker's
// shutdown flush forever.
const DefaultWriteTimeout = 30 * time.Second

// ChainWriter adapts the Storage interface to implement audit.EventChainWriter.
// This allows the audit worker to write event chains to storage.
type ChainWriter struct {
	storage      Storage
	ctx          context.Context
	writeTimeout time.Duration
}

// NewChainWriter creates a new ChainWriter that writes to the provided
// storage. Each Write is bounded by writeTimeout (DefaultWriteTimeout when
// <= 0); the writer's ctx is often decoupled from run-ctx cancellation so
// the shutdown flush can complete, and the per-write timeout is what keeps
// that flush bounded against a hung backend.
func NewChainWriter(ctx context.Context, storage Storage, writeTimeout time.Duration) *ChainWriter {
	if writeTimeout <= 0 {
		writeTimeout = DefaultWriteTimeout
	}
	return &ChainWriter{
		storage:      storage,
		ctx:          ctx,
		writeTimeout: writeTimeout,
	}
}

// Write implements audit.EventChainWriter by writing the event chain to storage.
func (w *ChainWriter) Write(chain audit.EventChain) error {
	if len(chain.Events) == 0 {
		return nil
	}

	// Use the tenant ID from the first event in the chain
	tenantID := chain.Events[0].TenantID

	block := Block{
		ID:       chain.ID,
		TenantID: tenantID,
		Chain:    chain,
	}

	// Bound the individual write: the writer's ctx may deliberately never
	// cancel (shutdown flush), so this timeout is the only thing standing
	// between a hung backend and a process that never exits.
	ctx, cancel := context.WithTimeout(w.ctx, w.writeTimeout)
	defer cancel()
	return w.storage.WriteBlock(ctx, tenantID, block)
}

// ChainReader retrieves event chains from storage for a specific tenant.
type ChainReader struct {
	storage Storage
	ctx     context.Context
}

// NewChainReader creates a new ChainReader.
func NewChainReader(ctx context.Context, storage Storage) *ChainReader {
	return &ChainReader{
		storage: storage,
		ctx:     ctx,
	}
}

// ReadAll retrieves all event chains for a tenant.
func (r *ChainReader) ReadAll(tenantID uuid.UUID) ([]audit.EventChain, error) {
	blocks, err := r.storage.FetchBlocks(r.ctx, tenantID, FetchOptions{})
	if err != nil {
		return nil, err
	}

	chains := make([]audit.EventChain, len(blocks))
	for i, block := range blocks {
		chains[i] = block.Chain
	}
	return chains, nil
}
