package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/storage"
)

// InMemory is an in-memory implementation of the Storage interface.
// It stores blocks in a nested map structure: tenantID -> blockID -> block.
// This implementation is thread-safe using a RWMutex.
type InMemory struct {
	mu     sync.RWMutex
	blocks map[uuid.UUID]map[uuid.UUID]storage.Block
}

// New creates a new in-memory storage instance.
func New() *InMemory {
	return &InMemory{
		blocks: make(map[uuid.UUID]map[uuid.UUID]storage.Block),
	}
}

// WriteBlock stores a block for a specific tenant.
func (m *InMemory) WriteBlock(ctx context.Context, tenantID uuid.UUID, block storage.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure the block has the correct tenant ID
	block.TenantID = tenantID

	// Initialize tenant map if it doesn't exist
	if m.blocks[tenantID] == nil {
		m.blocks[tenantID] = make(map[uuid.UUID]storage.Block)
	}

	// Store the block
	m.blocks[tenantID][block.ID] = block
	return nil
}

// FetchBlock retrieves a specific block by tenant ID and block ID.
func (m *InMemory) FetchBlock(ctx context.Context, tenantID uuid.UUID, blockID uuid.UUID) (storage.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenantBlocks, exists := m.blocks[tenantID]
	if !exists {
		return storage.Block{}, fmt.Errorf("no blocks found for tenant %s", tenantID)
	}

	block, exists := tenantBlocks[blockID]
	if !exists {
		return storage.Block{}, fmt.Errorf("block %s not found for tenant %s", blockID, tenantID)
	}

	return block, nil
}

// FetchBlocks retrieves all blocks for a tenant with optional filtering.
func (m *InMemory) FetchBlocks(ctx context.Context, tenantID uuid.UUID, opts storage.FetchOptions) ([]storage.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenantBlocks, exists := m.blocks[tenantID]
	if !exists {
		return []storage.Block{}, nil
	}

	// If specific block IDs are requested, filter by them
	if len(opts.BlockIDs) > 0 {
		return m.fetchFilteredBlocks(tenantBlocks, opts)
	}

	// Return all blocks for the tenant
	blocks := make([]storage.Block, 0, len(tenantBlocks))
	for _, block := range tenantBlocks {
		blocks = append(blocks, block)
		if opts.Limit > 0 && len(blocks) >= opts.Limit {
			break
		}
	}

	return blocks, nil
}

// fetchFilteredBlocks returns blocks matching the provided block IDs.
func (m *InMemory) fetchFilteredBlocks(tenantBlocks map[uuid.UUID]storage.Block, opts storage.FetchOptions) ([]storage.Block, error) {
	blocks := make([]storage.Block, 0, len(opts.BlockIDs))
	for _, blockID := range opts.BlockIDs {
		if block, exists := tenantBlocks[blockID]; exists {
			blocks = append(blocks, block)
			if opts.Limit > 0 && len(blocks) >= opts.Limit {
				break
			}
		}
	}
	return blocks, nil
}
