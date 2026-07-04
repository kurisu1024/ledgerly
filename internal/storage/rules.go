package storage

import (
	"context"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/rules"
)

// MutateRulesFunc computes the next rule set for a tenant from the current
// one. Returning an error aborts the mutation: RuleStore.MutateRules must
// leave the store untouched. The handler builds and enqueues the audit
// event for the mutation *inside* fn, so a full event queue makes fn error
// and the rule change never commits (ADR-0002: no unaudited mutation).
type MutateRulesFunc func(current []rules.Rule) (updated []rules.Rule, err error)

// RuleStore persists a tenant's trigger rules. Implementations must run
// MutateRules under a per-tenant critical section so concurrent mutations
// for the same tenant serialize.
type RuleStore interface {
	// ListRules returns the tenant's current rule set. An unknown tenant
	// returns an empty slice, not an error.
	ListRules(ctx context.Context, tenantID uuid.UUID) ([]rules.Rule, error)

	// GetRule returns a single rule scoped to tenantID. A rule that exists
	// only for a different tenant must be reported as not-found.
	GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (rules.Rule, error)

	// MutateRules runs fn with the tenant's current rules and commits the
	// result. If fn returns an error, MutateRules returns that error and
	// must not persist any change.
	MutateRules(ctx context.Context, tenantID uuid.UUID, fn MutateRulesFunc) ([]rules.Rule, error)
}
