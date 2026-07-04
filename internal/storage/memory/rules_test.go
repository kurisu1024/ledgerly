package memory_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/rules"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
)

func newRule(tenantID uuid.UUID) rules.Rule {
	return rules.Rule{
		ID:            uuid.New(),
		TenantID:      tenantID,
		SchemaVersion: rules.SchemaVersion,
		EventType:     "project.delete",
		LevelAtLeast:  "error",
	}
}

func TestRules_TenantIsolation(t *testing.T) {
	t.Log("\tGiven an in-memory rule store")
	store := memory.NewRules()
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()
	ruleA := newRule(tenantA)
	ruleB := newRule(tenantB)

	t.Log("\tWhen writing rules for two tenants")
	if _, err := store.MutateRules(ctx, tenantA, func(current []rules.Rule) ([]rules.Rule, error) {
		return append(current, ruleA), nil
	}); err != nil {
		t.Fatalf("\t%s\tMutateRules for tenant A: %v", fail, err)
	}
	if _, err := store.MutateRules(ctx, tenantB, func(current []rules.Rule) ([]rules.Rule, error) {
		return append(current, ruleB), nil
	}); err != nil {
		t.Fatalf("\t%s\tMutateRules for tenant B: %v", fail, err)
	}

	t.Log("\tThen each tenant only sees its own rules")
	gotA, err := store.ListRules(ctx, tenantA)
	if err != nil {
		t.Fatalf("\t%s\tListRules tenant A: %v", fail, err)
	}
	if len(gotA) != 1 || gotA[0].ID != ruleA.ID {
		t.Fatalf("\t%s\tExpected tenant A to see exactly its own rule, got %+v", fail, gotA)
	}

	gotB, err := store.ListRules(ctx, tenantB)
	if err != nil {
		t.Fatalf("\t%s\tListRules tenant B: %v", fail, err)
	}
	if len(gotB) != 1 || gotB[0].ID != ruleB.ID {
		t.Fatalf("\t%s\tExpected tenant B to see exactly its own rule, got %+v", fail, gotB)
	}

	if _, err := store.GetRule(ctx, tenantA, ruleB.ID); err == nil {
		t.Fatalf("\t%s\tExpected tenant A GetRule on tenant B's rule to fail", fail)
	}
	t.Logf("\t%s\tTenants correctly isolated", pass)
}

func TestRules_MutateAbortsOnFnError(t *testing.T) {
	t.Log("\tGiven a tenant with an existing rule")
	store := memory.NewRules()
	ctx := context.Background()
	tenantID := uuid.New()
	original := newRule(tenantID)

	if _, err := store.MutateRules(ctx, tenantID, func(current []rules.Rule) ([]rules.Rule, error) {
		return append(current, original), nil
	}); err != nil {
		t.Fatalf("\t%s\tseeding MutateRules: %v", fail, err)
	}

	t.Log("\tWhen a mutation's fn errors (simulating a full audit queue)")
	wantErr := fmt.Errorf("simulated audit enqueue failure")
	_, err := store.MutateRules(ctx, tenantID, func(current []rules.Rule) ([]rules.Rule, error) {
		return []rules.Rule{newRule(tenantID)}, wantErr
	})
	if err == nil {
		t.Fatalf("\t%s\tExpected MutateRules to propagate fn's error", fail)
	}

	t.Log("\tThen the store must be unchanged")
	got, err := store.ListRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("\t%s\tListRules: %v", fail, err)
	}
	if len(got) != 1 || got[0].ID != original.ID {
		t.Fatalf("\t%s\tExpected store unchanged after fn error, got %+v", fail, got)
	}
	t.Logf("\t%s\tStore left unchanged after aborted mutation", pass)
}

func TestRules_MutateRace(t *testing.T) {
	// Run with -race to prove MutateRules is safe for concurrent callers.
	store := memory.NewRules()
	ctx := context.Background()
	tenantID := uuid.New()

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.MutateRules(ctx, tenantID, func(current []rules.Rule) ([]rules.Rule, error) {
				return append(current, newRule(tenantID)), nil
			})
			if err != nil {
				t.Errorf("MutateRules: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := store.ListRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(got) != n {
		t.Fatalf("expected %d rules after %d concurrent mutations, got %d", n, n, len(got))
	}
}
