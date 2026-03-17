package storage

import (
	"context"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

// ChainWriter adapts the Storage interface to implement audit.EventChainWriter.
// This allows the audit worker to write event chains to storage.
type ChainWriter struct {
	storage Storage
	ctx     context.Context
}

// NewChainWriter creates a new ChainWriter that writes to the provided storage.
func NewChainWriter(ctx context.Context, storage Storage) *ChainWriter {
	return &ChainWriter{
		storage: storage,
		ctx:     ctx,
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

	return w.storage.WriteBlock(w.ctx, tenantID, block)
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
