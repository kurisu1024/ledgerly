package http

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/api/events"
	"github.com/kurisu1024/ledgerly/internal/storage"
)

// CreateEvent handles POST requests to create a new event for a specific tenant.
// URL format: POST /tenants/{tenantID}/events
func (t *T) CreateEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract tenant ID from URL path
	tenantIDStr := r.PathValue("tenantID")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	// Decode the event from request body
	var apiEvent events.Event
	if err := json.NewDecoder(r.Body).Decode(&apiEvent); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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
// URL format: GET /tenants/{tenantID}/events?blockID=uuid (optional)
func (t *T) ExportEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract tenant ID from URL path
	tenantIDStr := r.PathValue("tenantID")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
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

	// Fetch blocks from storage
	blocks, err := t.storage.FetchBlocks(r.Context(), tenantID, opts)
	if err != nil {
		http.Error(w, "Failed to fetch events", http.StatusInternalServerError)
		return
	}

	// Convert blocks to API events
	var allEvents []events.Event
	for _, block := range blocks {
		for _, auditEvent := range block.Chain.Events {
			allEvents = append(allEvents, events.FromAuditEvent(auditEvent))
		}
	}

	// Return events as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allEvents)
}
