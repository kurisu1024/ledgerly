package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurisu1024/ledgerly/internal/rules"
	"github.com/kurisu1024/ledgerly/internal/storage"
)

// Rules is a Postgres-backed implementation of storage.RuleStore, backed by
// the tenant_rules table. MutateRules must run under a per-tenant
// pg_advisory_xact_lock so concurrent mutations for the same tenant
// serialize even when the tenant has no existing rows to lock via
// SELECT ... FOR UPDATE.
type Rules struct {
	pool *pgxpool.Pool
}

var _ storage.RuleStore = (*Rules)(nil)

// NewRules creates a Postgres-backed RuleStore that reads/writes through
// pool. pool must already point at a schema with db/schema.sql applied.
func NewRules(pool *pgxpool.Pool) *Rules {
	return &Rules{pool: pool}
}

// ListRules returns the tenant's current rule set.
func (r *Rules) ListRules(ctx context.Context, tenantID uuid.UUID) ([]rules.Rule, error) {
	return nil, fmt.Errorf("postgres: ListRules not implemented")
}

// GetRule returns a single rule scoped to tenantID.
func (r *Rules) GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (rules.Rule, error) {
	return rules.Rule{}, fmt.Errorf("postgres: GetRule not implemented")
}

// MutateRules runs fn with the tenant's current rules and commits the
// result.
func (r *Rules) MutateRules(ctx context.Context, tenantID uuid.UUID, fn storage.MutateRulesFunc) ([]rules.Rule, error) {
	return nil, fmt.Errorf("postgres: MutateRules not implemented")
}
