package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	apirules "github.com/kurisu1024/ledgerly/api/rules"
	"github.com/kurisu1024/ledgerly/internal/audit"
	domain "github.com/kurisu1024/ledgerly/internal/rules"
)

// Trigger-rules endpoints (ADR-0002). All are behind authMiddleware;
// tenant comes exclusively from the JWT, never the path or body.
//
// Every mutation is chained unconditionally: the audit event is built and
// enqueued INSIDE the store's MutateRules fn, so a full queue aborts the
// mutation (503, store unchanged) — a rule change is never acknowledged
// without its audit event.

var (
	errAuditQueueFull = errors.New("audit queue is full")
	errRuleNotFound   = errors.New("rule not found")
)

// maxRuleBodyBytes caps CreateRule/UpdateRule request bodies (issue #24).
// An accepted rule is embedded twice into the permanent audit chain, so an
// unbounded body is an unbounded chain-growth lever. 64 KiB is orders of
// magnitude above the largest valid rule (bounded by domain.Validate).
// Repo-wide transport limits are tracked separately (issue #33).
const maxRuleBodyBytes = 64 << 10

// decodeRuleBody decodes a rule mutation body, enforcing maxRuleBodyBytes.
// On failure it writes the error response — 413 for an oversized body, 400
// otherwise — and returns ok == false.
func decodeRuleBody(w http.ResponseWriter, r *http.Request) (wire apirules.Rule, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRuleBodyBytes)

	if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
		}
		return apirules.Rule{}, false
	}
	return wire, true
}

// ListRules handles GET /v1/rules.
func (t *T) ListRules(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rs, err := t.ruleStore.ListRules(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "Failed to list rules", http.StatusInternalServerError)
		return
	}

	etag, err := domain.ETag(rs)
	if err != nil {
		http.Error(w, "Failed to compute rule set ETag", http.StatusInternalServerError)
		return
	}
	w.Header().Set("ETag", etag)

	if inm := r.Header.Get("If-None-Match"); inm != "" && ifNoneMatchSatisfied(inm, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	wire := make([]apirules.Rule, 0, len(rs))
	for _, rl := range rs {
		wire = append(wire, apirules.FromDomain(rl))
	}
	sort.Slice(wire, func(i, j int) bool { return wire[i].ID < wire[j].ID })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apirules.RuleList{
		SchemaVersion: domain.SchemaVersion,
		Rules:         wire,
	})
}

// CreateRule handles POST /v1/rules. The rule ID is server-assigned; any
// client-supplied ID is ignored.
func (t *T) CreateRule(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	subject, _ := getSubject(r.Context())

	wire, ok := decodeRuleBody(w, r)
	if !ok {
		return
	}
	wire.ID = "" // server-assigned

	rule, err := apirules.ToDomain(tenantID, wire)
	if err != nil {
		http.Error(w, "Invalid rule: "+err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	rule.ID = uuid.New()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	_, err = t.ruleStore.MutateRules(r.Context(), tenantID, func(current []domain.Rule) ([]domain.Rule, error) {
		if len(current) >= domain.MaxRulesPerTenant {
			return nil, domain.ErrTooManyRules
		}
		next := append(current, rule)
		if err := t.enqueueRuleAudit(tenantID, subject, domain.ActionRuleCreated, rule.ID, nil, &rule, next); err != nil {
			return nil, err
		}
		return next, nil
	})
	if err != nil {
		writeRuleMutationError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(apirules.FromDomain(rule))
}

// GetRule handles GET /v1/rules/{id}. Absent and cross-tenant rules are
// indistinguishable: both 404.
func (t *T) GetRule(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ruleID, err := apirules.ParseRuleID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	rule, err := t.ruleStore.GetRule(r.Context(), tenantID, ruleID)
	if err != nil {
		http.Error(w, "Rule not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apirules.FromDomain(rule))
}

// UpdateRule handles PUT /v1/rules/{id}. The path ID is authoritative; any
// ID in the body is ignored.
func (t *T) UpdateRule(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	subject, _ := getSubject(r.Context())

	ruleID, err := apirules.ParseRuleID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	wire, ok := decodeRuleBody(w, r)
	if !ok {
		return
	}
	wire.ID = "" // the path ID is authoritative

	rule, err := apirules.ToDomain(tenantID, wire)
	if err != nil {
		http.Error(w, "Invalid rule: "+err.Error(), http.StatusBadRequest)
		return
	}
	rule.ID = ruleID

	_, err = t.ruleStore.MutateRules(r.Context(), tenantID, func(current []domain.Rule) ([]domain.Rule, error) {
		idx := indexOfRule(current, ruleID)
		if idx < 0 {
			return nil, errRuleNotFound
		}
		old := current[idx]
		rule.CreatedAt = old.CreatedAt
		rule.UpdatedAt = time.Now().UTC()

		next := make([]domain.Rule, len(current))
		copy(next, current)
		next[idx] = rule

		if err := t.enqueueRuleAudit(tenantID, subject, domain.ActionRuleUpdated, ruleID, &old, &rule, next); err != nil {
			return nil, err
		}
		return next, nil
	})
	if err != nil {
		writeRuleMutationError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apirules.FromDomain(rule))
}

// DeleteRule handles DELETE /v1/rules/{id}.
func (t *T) DeleteRule(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	subject, _ := getSubject(r.Context())

	ruleID, err := apirules.ParseRuleID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	_, err = t.ruleStore.MutateRules(r.Context(), tenantID, func(current []domain.Rule) ([]domain.Rule, error) {
		idx := indexOfRule(current, ruleID)
		if idx < 0 {
			return nil, errRuleNotFound
		}
		old := current[idx]

		next := make([]domain.Rule, 0, len(current)-1)
		next = append(next, current[:idx]...)
		next = append(next, current[idx+1:]...)

		if err := t.enqueueRuleAudit(tenantID, subject, domain.ActionRuleDeleted, ruleID, &old, nil, next); err != nil {
			return nil, err
		}
		return next, nil
	})
	if err != nil {
		writeRuleMutationError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// enqueueRuleAudit builds the chained audit event for a rule mutation and
// enqueues it non-blocking. It is called INSIDE MutateRules' fn: a full
// queue errors, which aborts the whole mutation. Metadata carries the
// canonical JSON of the old and/or new rule and the post-mutation rule-set
// ETag, exactly as SDKs and the export contract expect.
func (t *T) enqueueRuleAudit(tenantID uuid.UUID, subject, action string, ruleID uuid.UUID, oldRule, newRule *domain.Rule, next []domain.Rule) error {
	etag, err := domain.ETag(next)
	if err != nil {
		return fmt.Errorf("computing post-mutation etag: %w", err)
	}

	metadata := map[string]string{"etag": etag}
	if oldRule != nil {
		b, err := domain.CanonicalJSON([]domain.Rule{*oldRule})
		if err != nil {
			return fmt.Errorf("encoding old rule: %w", err)
		}
		metadata["old-rule"] = string(b)
	}
	if newRule != nil {
		b, err := domain.CanonicalJSON([]domain.Rule{*newRule})
		if err != nil {
			return fmt.Errorf("encoding new rule: %w", err)
		}
		metadata["new-rule"] = string(b)
	}

	event := audit.NewEvent(
		tenantID,
		map[string]string{"sub": subject},
		action,
		map[string]string{"type": "trigger-rule", "id": ruleID.String()},
		metadata,
	)

	select {
	case t.queue <- event:
		return nil
	default:
		return errAuditQueueFull
	}
}

// writeRuleMutationError maps a MutateRules failure to its HTTP status:
// full audit queue → 503 (retryable, store unchanged), missing rule → 404,
// tenant at the rule cap → 409 Conflict (the create conflicts with the
// current state of the tenant's rule collection; retryable after deleting
// a rule), anything else → 500.
func writeRuleMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAuditQueueFull):
		http.Error(w, "Event queue is full", http.StatusServiceUnavailable)
	case errors.Is(err, errRuleNotFound):
		http.Error(w, "Rule not found", http.StatusNotFound)
	case errors.Is(err, domain.ErrTooManyRules):
		http.Error(w, fmt.Sprintf("Rule limit reached: a tenant may hold at most %d rules; delete an existing rule first", domain.MaxRulesPerTenant), http.StatusConflict)
	default:
		http.Error(w, "Failed to mutate rules", http.StatusInternalServerError)
	}
}

func indexOfRule(rs []domain.Rule, id uuid.UUID) int {
	for i := range rs {
		if rs[i].ID == id {
			return i
		}
	}
	return -1
}

// ifNoneMatchSatisfied reports whether the If-None-Match header matches
// etag. Weak validators (W/ prefix) are compared by opaque value — fine for
// a content-derived strong ETag — and "*" matches any current
// representation.
func ifNoneMatchSatisfied(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}
