package http

import "net/http"

// Trigger-rules endpoints (ADR-0002). All are behind authMiddleware;
// tenant comes exclusively from the JWT, never the path or body.
//
// STUB: every handler currently returns 501. Real behavior (validation,
// MutateRules + chained rule.created/updated/deleted audit event, ETag /
// If-None-Match, cross-tenant 404) lands in GREEN — see rules_test.go and
// e2e_test.go for the pinned contract.

// ListRules handles GET /v1/rules.
func (t *T) ListRules(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// CreateRule handles POST /v1/rules.
func (t *T) CreateRule(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// GetRule handles GET /v1/rules/{id}.
func (t *T) GetRule(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// UpdateRule handles PUT /v1/rules/{id}.
func (t *T) UpdateRule(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// DeleteRule handles DELETE /v1/rules/{id}.
func (t *T) DeleteRule(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
