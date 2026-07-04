package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// ListRules returns the tenant's current rule set. An unknown tenant
// returns an empty slice, not an error.
func (r *Rules) ListRules(ctx context.Context, tenantID uuid.UUID) ([]rules.Rule, error) {
	return listRules(ctx, r.pool, tenantID)
}

// GetRule returns a single rule scoped to tenantID. A rule that exists only
// for a different tenant is reported as not-found.
func (r *Rules) GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (rules.Rule, error) {
	var payload []byte
	err := r.pool.QueryRow(ctx,
		`SELECT rule FROM tenant_rules WHERE tenant_id = $1 AND rule_id = $2`,
		tenantID, ruleID,
	).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return rules.Rule{}, fmt.Errorf("rule %s not found for tenant %s", ruleID, tenantID)
	}
	if err != nil {
		return rules.Rule{}, fmt.Errorf("querying rule %s for tenant %s: %w", ruleID, tenantID, err)
	}

	var rl rules.Rule
	if err := json.Unmarshal(payload, &rl); err != nil {
		return rules.Rule{}, fmt.Errorf("unmarshaling rule %s for tenant %s: %w", ruleID, tenantID, err)
	}
	return rl, nil
}

// MutateRules runs fn with the tenant's current rules and commits the
// result by replacing the tenant's whole rule set (delete + insert) inside
// one transaction. A per-tenant pg_advisory_xact_lock serializes concurrent
// mutations even for tenants with no existing rows. Any error from fn
// aborts the transaction, leaving the store unchanged (ADR-0002: no
// unaudited mutation).
func (r *Rules) MutateRules(ctx context.Context, tenantID uuid.UUID, fn storage.MutateRulesFunc) ([]rules.Rule, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning MutateRules transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	// Per-tenant advisory lock, released automatically at commit/rollback.
	// hashtextextended maps the tenant UUID onto the bigint advisory-lock
	// keyspace.
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, tenantID,
	); err != nil {
		return nil, fmt.Errorf("acquiring advisory lock for tenant %s: %w", tenantID, err)
	}

	current, err := listRules(ctx, tx, tenantID)
	if err != nil {
		return nil, err
	}

	updated, err := fn(current)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM tenant_rules WHERE tenant_id = $1`, tenantID,
	); err != nil {
		return nil, fmt.Errorf("clearing rules for tenant %s: %w", tenantID, err)
	}
	for _, rl := range updated {
		payload, err := json.Marshal(rl)
		if err != nil {
			return nil, fmt.Errorf("marshaling rule %s for tenant %s: %w", rl.ID, tenantID, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO tenant_rules (tenant_id, rule_id, rule) VALUES ($1, $2, $3)`,
			tenantID, rl.ID, payload,
		); err != nil {
			return nil, fmt.Errorf("inserting rule %s for tenant %s: %w", rl.ID, tenantID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing MutateRules transaction: %w", err)
	}
	return updated, nil
}

// querier is the subset of pgx query methods shared by *pgxpool.Pool and
// pgx.Tx, so listRules serves both ListRules and MutateRules.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// listRules loads a tenant's rules. The JSON round-trip through the rule
// column naturally yields fresh values, matching the in-memory backend's
// copy semantics — callers can mutate what they get back without touching
// committed state.
func listRules(ctx context.Context, q querier, tenantID uuid.UUID) ([]rules.Rule, error) {
	rows, err := q.Query(ctx,
		`SELECT rule FROM tenant_rules WHERE tenant_id = $1`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying rules for tenant %s: %w", tenantID, err)
	}
	defer rows.Close()

	out := make([]rules.Rule, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scanning rule row for tenant %s: %w", tenantID, err)
		}
		var rl rules.Rule
		if err := json.Unmarshal(payload, &rl); err != nil {
			return nil, fmt.Errorf("unmarshaling rule for tenant %s: %w", tenantID, err)
		}
		out = append(out, rl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading rules for tenant %s: %w", tenantID, err)
	}
	return out, nil
}
