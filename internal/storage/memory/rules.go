package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/rules"
	"github.com/kurisu1024/ledgerly/internal/storage"
)

// Rules is an in-memory storage.RuleStore. A single RWMutex guards the
// whole store — rule mutations are low-frequency admin operations, unlike
// the per-tenant event write path, so store-wide serialization of writes
// is an acceptable simplification here; reads only share the lock.
type Rules struct {
	mu       sync.RWMutex
	byTenant map[uuid.UUID]map[uuid.UUID]rules.Rule
}

// NewRules creates a new in-memory rule store.
func NewRules() *Rules {
	return &Rules{byTenant: make(map[uuid.UUID]map[uuid.UUID]rules.Rule)}
}

var _ storage.RuleStore = (*Rules)(nil)

func (r *Rules) ListRules(ctx context.Context, tenantID uuid.UUID) ([]rules.Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listLocked(tenantID), nil
}

// listLocked returns a deep-enough copy of the tenant's rules: callers
// (including MutateRules fns) can mutate the returned slice, the rules, and
// their Fields without ever touching committed store state.
func (r *Rules) listLocked(tenantID uuid.UUID) []rules.Rule {
	tenantRules := r.byTenant[tenantID]
	out := make([]rules.Rule, 0, len(tenantRules))
	for _, rl := range tenantRules {
		out = append(out, copyRule(rl))
	}
	return out
}

func copyRule(rl rules.Rule) rules.Rule {
	if rl.Fields != nil {
		fields := make([]rules.FieldCond, len(rl.Fields))
		copy(fields, rl.Fields)
		rl.Fields = fields
	}
	return rl
}

func (r *Rules) GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (rules.Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rl, ok := r.byTenant[tenantID][ruleID]
	if !ok {
		return rules.Rule{}, fmt.Errorf("rule %s not found for tenant %s", ruleID, tenantID)
	}
	return copyRule(rl), nil
}

// MutateRules runs fn under the store lock on a copy of the tenant's rule
// set and commits the result copy-on-write — only when fn returns nil. Any
// error from fn (a failed audit enqueue, a not-found rule) aborts the
// mutation and leaves the store unchanged (ADR-0002: no unaudited
// mutation).
func (r *Rules) MutateRules(ctx context.Context, tenantID uuid.UUID, fn storage.MutateRulesFunc) ([]rules.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.listLocked(tenantID)
	updated, err := fn(current)
	if err != nil {
		return nil, err
	}
	r.setLocked(tenantID, updated)
	return updated, nil
}

func (r *Rules) setLocked(tenantID uuid.UUID, rs []rules.Rule) {
	tenantMap := make(map[uuid.UUID]rules.Rule, len(rs))
	for _, rl := range rs {
		tenantMap[rl.ID] = copyRule(rl)
	}
	r.byTenant[tenantID] = tenantMap
}
