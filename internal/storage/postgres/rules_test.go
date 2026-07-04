package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	domainrules "github.com/kurisu1024/ledgerly/internal/rules"
	"github.com/kurisu1024/ledgerly/internal/storage/postgres"
)

func newTestRule(tenantID uuid.UUID) domainrules.Rule {
	return domainrules.Rule{
		ID:            uuid.New(),
		TenantID:      tenantID,
		SchemaVersion: domainrules.SchemaVersion,
		EventType:     "project.delete",
		LevelAtLeast:  "error",
	}
}

func TestRules_MutateCommitRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.NewRules(pool)
	ctx := context.Background()
	tenantID := uuid.New()

	rule := newTestRule(tenantID)
	committed, err := store.MutateRules(ctx, tenantID, func(current []domainrules.Rule) ([]domainrules.Rule, error) {
		return append(current, rule), nil
	})
	if err != nil {
		t.Fatalf("MutateRules: %v", err)
	}
	if len(committed) != 1 || committed[0].ID != rule.ID {
		t.Fatalf("expected MutateRules to return the committed rule set, got %+v", committed)
	}

	got, err := store.ListRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(got) != 1 || got[0].ID != rule.ID {
		t.Fatalf("expected ListRules to reflect the committed mutation, got %+v", got)
	}

	fetched, err := store.GetRule(ctx, tenantID, rule.ID)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if fetched.ID != rule.ID || fetched.EventType != rule.EventType {
		t.Fatalf("GetRule returned unexpected rule: %+v", fetched)
	}
}

func TestRules_MutateAbortsOnFnError(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.NewRules(pool)
	ctx := context.Background()
	tenantID := uuid.New()

	original := newTestRule(tenantID)
	if _, err := store.MutateRules(ctx, tenantID, func(current []domainrules.Rule) ([]domainrules.Rule, error) {
		return append(current, original), nil
	}); err != nil {
		t.Fatalf("seeding MutateRules: %v", err)
	}

	wantErr := fmt.Errorf("simulated audit enqueue failure")
	_, err := store.MutateRules(ctx, tenantID, func(current []domainrules.Rule) ([]domainrules.Rule, error) {
		return []domainrules.Rule{newTestRule(tenantID)}, wantErr
	})
	if err == nil {
		t.Fatalf("expected MutateRules to propagate fn's error")
	}

	got, err := store.ListRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(got) != 1 || got[0].ID != original.ID {
		t.Fatalf("expected store unchanged after aborted mutation, got %+v", got)
	}
}

func TestRules_CrossTenantNotFound(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.NewRules(pool)
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()
	ruleA := newTestRule(tenantA)

	if _, err := store.MutateRules(ctx, tenantA, func(current []domainrules.Rule) ([]domainrules.Rule, error) {
		return append(current, ruleA), nil
	}); err != nil {
		t.Fatalf("MutateRules tenant A: %v", err)
	}

	if _, err := store.GetRule(ctx, tenantB, ruleA.ID); err == nil {
		t.Fatalf("expected GetRule to report not-found for tenant B fetching tenant A's rule")
	}

	gotB, err := store.ListRules(ctx, tenantB)
	if err != nil {
		t.Fatalf("ListRules tenant B: %v", err)
	}
	if len(gotB) != 0 {
		t.Fatalf("expected tenant B to see no rules, got %+v", gotB)
	}
}

// TestRules_ConcurrentFreshTenantCreatesSerialize pins the per-tenant
// pg_advisory_xact_lock: for a brand-new tenant with zero existing rows,
// `SELECT ... FOR UPDATE` locks nothing, so without an explicit advisory
// lock, concurrent MutateRules calls would race read-modify-write and lose
// updates. Every one of n concurrent creates must land.
func TestRules_ConcurrentFreshTenantCreatesSerialize(t *testing.T) {
	pool := newTestPool(t)
	store := postgres.NewRules(pool)
	ctx := context.Background()
	tenantID := uuid.New()

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.MutateRules(ctx, tenantID, func(current []domainrules.Rule) ([]domainrules.Rule, error) {
				return append(current, newTestRule(tenantID)), nil
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("MutateRules: %v", err)
		}
	}

	got, err := store.ListRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(got) != n {
		t.Fatalf("expected %d rules after %d concurrent fresh-tenant creates, got %d (lost update — advisory lock missing?)", n, n, len(got))
	}
}
