package memory_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
)

var pass = "\u2705"
var fail = "\u274C"

func TestWriteAndFetchBlock(t *testing.T) {
	t.Log("\tGiven an in-memory storage")
	store := memory.New()
	ctx := context.Background()

	tenantID := uuid.New()
	blockID := uuid.New()

	chain := audit.NewEventChain(10)
	event := audit.NewEvent(
		tenantID,
		map[string]string{"id": "user1", "type": "user"},
		"project.create",
		map[string]string{"type": "project", "id": "proj1"},
		map[string]string{"reason": "test"},
	)
	chain = audit.AppendEvent(chain, event)

	block := storage.Block{
		ID:       blockID,
		TenantID: tenantID,
		Chain:    chain,
	}

	t.Log("\tWhen writing a block")
	if err := store.WriteBlock(ctx, tenantID, block); err != nil {
		t.Fatalf("\t%s\tFailed to write block: %v", fail, err)
	}
	t.Logf("\t%s\tSuccessfully wrote block", pass)

	t.Log("\tWhen fetching the block")
	fetchedBlock, err := store.FetchBlock(ctx, tenantID, blockID)
	if err != nil {
		t.Fatalf("\t%s\tFailed to fetch block: %v", fail, err)
	}
	t.Logf("\t%s\tSuccessfully fetched block", pass)

	t.Log("\tThen the fetched block should match the original")
	if fetchedBlock.ID != block.ID {
		t.Errorf("\t%s\tBlock ID mismatch: got %s, want %s", fail, fetchedBlock.ID, block.ID)
	}
	if fetchedBlock.TenantID != block.TenantID {
		t.Errorf("\t%s\tTenant ID mismatch: got %s, want %s", fail, fetchedBlock.TenantID, block.TenantID)
	}
	if len(fetchedBlock.Chain.Events) != len(block.Chain.Events) {
		t.Errorf("\t%s\tEvent count mismatch: got %d, want %d", fail, len(fetchedBlock.Chain.Events), len(block.Chain.Events))
	}
	t.Logf("\t%s\tFetched block matches original", pass)
}

func TestFetchBlocks(t *testing.T) {
	t.Log("\tGiven an in-memory storage with multiple blocks")
	store := memory.New()
	ctx := context.Background()

	tenantID := uuid.New()

	// Create and store multiple blocks
	blockIDs := make([]uuid.UUID, 3)
	for i := 0; i < 3; i++ {
		blockID := uuid.New()
		blockIDs[i] = blockID

		chain := audit.NewEventChain(10)
		event := audit.NewEvent(
			tenantID,
			map[string]string{"id": "user1", "type": "user"},
			"project.create",
			map[string]string{"type": "project", "id": "proj1"},
			map[string]string{"reason": "test"},
		)
		chain = audit.AppendEvent(chain, event)

		block := storage.Block{
			ID:       blockID,
			TenantID: tenantID,
			Chain:    chain,
		}

		if err := store.WriteBlock(ctx, tenantID, block); err != nil {
			t.Fatalf("\t%s\tFailed to write block %d: %v", fail, i, err)
		}
	}

	t.Log("\tWhen fetching all blocks for a tenant")
	blocks, err := store.FetchBlocks(ctx, tenantID, storage.FetchOptions{})
	if err != nil {
		t.Fatalf("\t%s\tFailed to fetch blocks: %v", fail, err)
	}
	if len(blocks) != 3 {
		t.Fatalf("\t%s\tExpected 3 blocks, got %d", fail, len(blocks))
	}
	t.Logf("\t%s\tSuccessfully fetched all blocks", pass)

	t.Log("\tWhen fetching blocks with a filter")
	blocks, err = store.FetchBlocks(ctx, tenantID, storage.FetchOptions{
		BlockIDs: []uuid.UUID{blockIDs[0]},
	})
	if err != nil {
		t.Fatalf("\t%s\tFailed to fetch filtered blocks: %v", fail, err)
	}
	if len(blocks) != 1 {
		t.Fatalf("\t%s\tExpected 1 block, got %d", fail, len(blocks))
	}
	if blocks[0].ID != blockIDs[0] {
		t.Errorf("\t%s\tExpected block ID %s, got %s", fail, blockIDs[0], blocks[0].ID)
	}
	t.Logf("\t%s\tSuccessfully fetched filtered blocks", pass)
}

func TestFetchNonExistentBlock(t *testing.T) {
	t.Log("\tGiven an empty in-memory storage")
	store := memory.New()
	ctx := context.Background()

	tenantID := uuid.New()
	blockID := uuid.New()

	t.Log("\tWhen fetching a non-existent block")
	_, err := store.FetchBlock(ctx, tenantID, blockID)
	if err == nil {
		t.Fatalf("\t%s\tExpected error for non-existent block, got nil", fail)
	}
	t.Logf("\t%s\tCorrectly returned error for non-existent block", pass)
}
