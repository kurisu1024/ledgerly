package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/rules"
	"github.com/kurisu1024/ledgerly/internal/storage"
)

// Rules is an in-memory storage.RuleStore. A single mutex guards the whole
// store — rule mutations are low-frequency admin operations, unlike the
// per-tenant event write path, so store-wide serialization is an
// acceptable simplification here.
type Rules struct {
	mu       sync.Mutex
	byTenant map[uuid.UUID]map[uuid.UUID]rules.Rule
}

// NewRules creates a new in-memory rule store.
func NewRules() *Rules {
	return &Rules{byTenant: make(map[uuid.UUID]map[uuid.UUID]rules.Rule)}
}

var _ storage.RuleStore = (*Rules)(nil)

func (r *Rules) ListRules(ctx context.Context, tenantID uuid.UUID) ([]rules.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listLocked(tenantID), nil
}

func (r *Rules) listLocked(tenantID uuid.UUID) []rules.Rule {
	tenantRules := r.byTenant[tenantID]
	out := make([]rules.Rule, 0, len(tenantRules))
	for _, rl := range tenantRules {
		out = append(out, rl)
	}
	return out
}

func (r *Rules) GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (rules.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rl, ok := r.byTenant[tenantID][ruleID]
	if !ok {
		return rules.Rule{}, fmt.Errorf("rule %s not found for tenant %s", ruleID, tenantID)
	}
	return rl, nil
}

// MutateRules runs fn under the store lock and commits its result.
//
// STUB BUG (tracked for GREEN): this always commits updated, even when fn
// returns an error, instead of aborting the mutation. It compiles and
// satisfies storage.RuleStore, but violates the ADR-0002 invariant that a
// failed audit-enqueue (inside fn) must leave the store unchanged.
// TestRuleMutation_QueueFull_503_StoreUnchanged (api/http) and
// TestRules_MutateAbortsOnFnError (this package) pin the correct behavior.
func (r *Rules) MutateRules(ctx context.Context, tenantID uuid.UUID, fn storage.MutateRulesFunc) ([]rules.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.listLocked(tenantID)
	updated, err := fn(current)
	r.setLocked(tenantID, updated)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *Rules) setLocked(tenantID uuid.UUID, rs []rules.Rule) {
	tenantMap := make(map[uuid.UUID]rules.Rule, len(rs))
	for _, rl := range rs {
		tenantMap[rl.ID] = rl
	}
	r.byTenant[tenantID] = tenantMap
}
