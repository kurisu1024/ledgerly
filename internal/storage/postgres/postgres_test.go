package postgres_test

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage"
	"github.com/kurisu1024/ledgerly/internal/storage/postgres"
)

// singleEventChain builds a real, hash-linked one-event chain for
// tenantID via audit.AppendEvent — never hand-rolled, so every test in this
// file exercises the same hashing/linking rules the storage layer must
// preserve on round trip.
func singleEventChain(tenantID uuid.UUID) audit.EventChain {
	chain := audit.NewEventChain(10)
	e := audit.NewEvent(
		tenantID,
		map[string]string{"id": "user1", "type": "user"},
		"project.create",
		map[string]string{"type": "project", "id": "proj1"},
		map[string]string{"reason": "test"},
	)
	return audit.AppendEvent(chain, e)
}

func multiEventChain(tenantID uuid.UUID, n int) audit.EventChain {
	chain := audit.NewEventChain(n + 1)
	for i := 0; i < n; i++ {
		e := audit.NewEvent(
			tenantID,
			map[string]string{"id": fmt.Sprintf("user%d", i), "type": "user"},
			fmt.Sprintf("project.action.%d", i),
			map[string]string{"type": "project", "id": "proj1"},
			map[string]string{"reason": "test"},
		)
		chain = audit.AppendEvent(chain, e)
	}
	return chain
}

func TestWriteFetchBlock_RoundTrip(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.New(pool)
	ctx := context.Background()

	tenantID := uuid.New()
	chain := multiEventChain(tenantID, 3)
	block := storage.Block{ID: chain.ID, TenantID: tenantID, Chain: chain}

	if err := store.WriteBlock(ctx, tenantID, block); err != nil {
		t.Fatalf("WriteBlock: %v", err)
	}

	got, err := store.FetchBlock(ctx, tenantID, chain.ID)
	if err != nil {
		t.Fatalf("FetchBlock: %v", err)
	}

	if !audit.VerifyChain(got.Chain) {
		t.Fatalf("round-tripped chain failed VerifyChain")
	}
	if !reflect.DeepEqual(got.Chain, block.Chain) {
		t.Fatalf("round-tripped chain does not deep-equal original:\n got:  %+v\n want: %+v", got.Chain, block.Chain)
	}
	for i := range got.Chain.Events {
		if !bytes.Equal(got.Chain.Events[i].EventHash, block.Chain.Events[i].EventHash) {
			t.Fatalf("event %d: EventHash not byte-equal after round trip", i)
		}
		if !bytes.Equal(got.Chain.Events[i].PrevHash, block.Chain.Events[i].PrevHash) {
			t.Fatalf("event %d: PrevHash not byte-equal after round trip", i)
		}
		if !got.Chain.Events[i].OccurredAt.Equal(block.Chain.Events[i].OccurredAt) {
			t.Fatalf("event %d: OccurredAt not preserved at ns precision: got %v, want %v",
				i, got.Chain.Events[i].OccurredAt, block.Chain.Events[i].OccurredAt)
		}
	}
}

func TestFetchBlock_OtherTenant_NotFound(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.New(pool)
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()
	chain := singleEventChain(tenantA)

	if err := store.WriteBlock(ctx, tenantA, storage.Block{ID: chain.ID, TenantID: tenantA, Chain: chain}); err != nil {
		t.Fatalf("WriteBlock: %v", err)
	}

	if _, err := store.FetchBlock(ctx, tenantB, chain.ID); err == nil {
		t.Fatalf("expected FetchBlock to fail for tenant B fetching tenant A's block, got nil error")
	}
}

func TestFetchBlocks_TenantScoped(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.New(pool)
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()

	for i := 0; i < 2; i++ {
		chain := singleEventChain(tenantA)
		if err := store.WriteBlock(ctx, tenantA, storage.Block{ID: chain.ID, TenantID: tenantA, Chain: chain}); err != nil {
			t.Fatalf("WriteBlock tenant A #%d: %v", i, err)
		}
	}
	chainB := singleEventChain(tenantB)
	if err := store.WriteBlock(ctx, tenantB, storage.Block{ID: chainB.ID, TenantID: tenantB, Chain: chainB}); err != nil {
		t.Fatalf("WriteBlock tenant B: %v", err)
	}

	blocksA, err := store.FetchBlocks(ctx, tenantA, storage.FetchOptions{})
	if err != nil {
		t.Fatalf("FetchBlocks tenant A: %v", err)
	}
	if len(blocksA) != 2 {
		t.Fatalf("expected 2 blocks for tenant A, got %d", len(blocksA))
	}
	for _, b := range blocksA {
		if b.TenantID != tenantA {
			t.Fatalf("tenant A's FetchBlocks returned a block for tenant %s", b.TenantID)
		}
	}

	blocksB, err := store.FetchBlocks(ctx, tenantB, storage.FetchOptions{})
	if err != nil {
		t.Fatalf("FetchBlocks tenant B: %v", err)
	}
	if len(blocksB) != 1 {
		t.Fatalf("expected 1 block for tenant B, got %d", len(blocksB))
	}
}

func TestFetchBlocks_ByIDsAndLimit(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.New(pool)
	ctx := context.Background()
	tenantID := uuid.New()

	var chainIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		chain := singleEventChain(tenantID)
		chainIDs = append(chainIDs, chain.ID)
		if err := store.WriteBlock(ctx, tenantID, storage.Block{ID: chain.ID, TenantID: tenantID, Chain: chain}); err != nil {
			t.Fatalf("WriteBlock #%d: %v", i, err)
		}
	}

	filtered, err := store.FetchBlocks(ctx, tenantID, storage.FetchOptions{BlockIDs: chainIDs[:2]})
	if err != nil {
		t.Fatalf("FetchBlocks with BlockIDs: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 blocks for the requested IDs, got %d", len(filtered))
	}
	gotIDs := make(map[uuid.UUID]bool, len(filtered))
	for _, b := range filtered {
		gotIDs[b.ID] = true
	}
	for _, id := range chainIDs[:2] {
		if !gotIDs[id] {
			t.Fatalf("expected block %s in BlockIDs-filtered result, got %v", id, filtered)
		}
	}

	limited, err := store.FetchBlocks(ctx, tenantID, storage.FetchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("FetchBlocks with Limit: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 block with Limit=1, got %d", len(limited))
	}
}

func TestWriteBlock_DuplicateChainRejected(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.New(pool)
	ctx := context.Background()
	tenantID := uuid.New()

	chain := singleEventChain(tenantID)
	block := storage.Block{ID: chain.ID, TenantID: tenantID, Chain: chain}

	if err := store.WriteBlock(ctx, tenantID, block); err != nil {
		t.Fatalf("first WriteBlock: %v", err)
	}
	if err := store.WriteBlock(ctx, tenantID, block); err == nil {
		t.Fatalf("expected a second WriteBlock for the same chain ID to be rejected, got nil error")
	}
}

// TestImmutability_UpdateDeleteRaise exercises db/schema.sql's forbid_mutation
// triggers directly against a seeded row — it does not go through Storage,
// so it validates the schema itself independent of the Go implementation's
// state.
func TestImmutability_UpdateDeleteRaise(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	tenantID := uuid.New()
	chainID := uuid.New()

	if _, err := pool.Exec(ctx,
		`INSERT INTO audit_events (tenant_id, chain_id, position, event) VALUES ($1, $2, 0, '{}'::jsonb)`,
		tenantID, chainID,
	); err != nil {
		t.Fatalf("seeding audit_events row: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE audit_events SET event = '{"tampered":true}'::jsonb WHERE tenant_id = $1 AND chain_id = $2 AND position = 0`,
		tenantID, chainID,
	); err == nil {
		t.Fatalf("expected UPDATE on audit_events to be rejected by forbid_mutation, got nil error")
	}

	if _, err := pool.Exec(ctx,
		`DELETE FROM audit_events WHERE tenant_id = $1 AND chain_id = $2 AND position = 0`,
		tenantID, chainID,
	); err == nil {
		t.Fatalf("expected DELETE on audit_events to be rejected by forbid_mutation, got nil error")
	}
}

// TestRestartSurvival is the point of issue #25: write chains, close the
// pool (simulating a process restart), reopen a fresh pool against the same
// schema, and verify every chain still verifies. This is what pins the
// nanosecond-timestamp precision landmine described in the plan — a
// TIMESTAMPTZ column would truncate OccurredAt and silently fail
// VerifyChain here.
func TestRestartSurvival(t *testing.T) {
	dsn, schemaName := setupTestSchema(t)
	pool := openTestPool(t, dsn, schemaName)

	ctx := context.Background()
	tenantID := uuid.New()

	const numChains = 3
	wantChainIDs := make(map[uuid.UUID]bool, numChains)

	store := postgres.New(pool)
	for i := 0; i < numChains; i++ {
		chain := multiEventChain(tenantID, 4)
		wantChainIDs[chain.ID] = true
		if err := store.WriteBlock(ctx, tenantID, storage.Block{ID: chain.ID, TenantID: tenantID, Chain: chain}); err != nil {
			t.Fatalf("WriteBlock chain #%d: %v", i, err)
		}
	}

	pool.Close()

	freshPool := openTestPool(t, dsn, schemaName)
	t.Cleanup(freshPool.Close)

	freshStore := postgres.New(freshPool)
	blocks, err := freshStore.FetchBlocks(ctx, tenantID, storage.FetchOptions{})
	if err != nil {
		t.Fatalf("FetchBlocks after restart: %v", err)
	}
	if len(blocks) != numChains {
		t.Fatalf("expected %d chains to survive restart, got %d", numChains, len(blocks))
	}
	for _, b := range blocks {
		if !wantChainIDs[b.Chain.ID] {
			t.Fatalf("unexpected chain %s after restart", b.Chain.ID)
		}
		if !audit.VerifyChain(b.Chain) {
			t.Fatalf("chain %s failed VerifyChain after restart", b.Chain.ID)
		}
	}
}
