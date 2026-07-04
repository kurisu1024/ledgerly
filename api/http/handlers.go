package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/api/events"
	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage"
)

// maxEventBodyBytes caps POST /v1/events request bodies (issue #33). An
// accepted event is embedded into the permanent audit chain, so an unbounded
// body is an unbounded chain-growth lever. 1 MiB is an order of magnitude
// above any legitimate event payload. Mirrors maxRuleBodyBytes in rules.go.
const maxEventBodyBytes = 1 << 20

// CreateEvent handles POST requests to create a new event for a specific tenant.
// URL format: POST /v1/events
// Tenant ID is extracted from JWT token via auth middleware
func (t *T) CreateEvent(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID from context (set by auth middleware)
	tenantID, err := getTenantID(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Decode the event from request body, enforcing the transport cap
	r.Body = http.MaxBytesReader(w, r.Body, maxEventBodyBytes)

	var apiEvent events.Event
	if err := json.NewDecoder(r.Body).Decode(&apiEvent); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
		}
		return
	}

	// Ensure tenant ID matches
	apiEvent.TenantID = tenantID.String()

	// Convert to audit event
	auditEvent, err := events.ToAuditEvent(apiEvent)
	if err != nil {
		http.Error(w, "Failed to process event: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Send event to worker queue for async processing
	// The worker will handle chaining and storage
	select {
	case t.queue <- auditEvent:
		// Event queued successfully
	default:
		http.Error(w, "Event queue is full", http.StatusServiceUnavailable)
		return
	}

	// Return the created event (it will be processed asynchronously)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(events.FromAuditEvent(auditEvent))
}

// ExportEvents handles GET requests to export all events for a specific tenant.
// URL format: GET /v1/export?blockID=uuid&as-of=RFC3339Nano (both optional)
// Tenant ID is extracted from JWT token via auth middleware.
//
// as-of cuts each stored chain in append order at the given timestamp
// (inclusive to the nanosecond): the walk stops at the first event whose
// occurred-at is strictly after the cut, so every returned chain is a
// verifiable genesis-anchored prefix — never a timestamp-filtered subset.
// Chains whose cut is empty are omitted entirely. Only flushed blocks are
// visible: an event accepted with 202 but not yet flushed never appears,
// regardless of as-of.
func (t *T) ExportEvents(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID from context (set by auth middleware)
	tenantID, err := getTenantID(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse optional filter parameters
	opts := storage.FetchOptions{}
	if blockIDStr := r.URL.Query().Get("blockID"); blockIDStr != "" {
		blockID, err := uuid.Parse(blockIDStr)
		if err != nil {
			http.Error(w, "Invalid block ID", http.StatusBadRequest)
			return
		}
		opts.BlockIDs = []uuid.UUID{blockID}
	}

	var asOf time.Time
	hasAsOf := false
	if asOfStr := r.URL.Query().Get("as-of"); asOfStr != "" {
		asOf, err = time.Parse(time.RFC3339Nano, asOfStr)
		if err != nil {
			http.Error(w, "Invalid as-of parameter", http.StatusBadRequest)
			return
		}
		hasAsOf = true
	}

	// Fetch blocks from storage
	blocks, err := t.storage.FetchBlocks(r.Context(), tenantID, opts)
	if err != nil {
		http.Error(w, "Failed to fetch events", http.StatusInternalServerError)
		return
	}

	// Convert blocks to API events, applying the as-of cut per chain
	var allEvents []events.Event
	for _, block := range blocks {
		chain := block.Chain
		if hasAsOf {
			chain = audit.CutAtTime(chain, asOf)
			if len(chain.Events) == 0 {
				continue
			}
		}
		for _, auditEvent := range chain.Events {
			allEvents = append(allEvents, events.FromAuditEvent(auditEvent))
		}
	}

	// Return events as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allEvents)
}
