package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func newAuditEvent(
	tenantID uuid.UUID,
	actor json.RawMessage,
	action string,
	resource json.RawMessage,
	metadata json.RawMessage,
) *AuditEvent {
	return &AuditEvent{
		ID:         uuid.New(),
		TenantID:   tenantID,
		OccurredAt: time.Now().UTC(),
		Actor:      actor,
		Action:     action,
		Resource:   resource,
		Metadata:   metadata,
	}
}

type AuditEvent struct {
	ID         uuid.UUID `json:"id"`
	TenantID   uuid.UUID `json:"tenant-id"`
	OccurredAt time.Time `json:"occurred-at"`

	Actor    json.RawMessage `json:"actor"`
	Action   string          `jsonL:"action"`
	Resource json.RawMessage `json:"resource"`
	Metadata json.RawMessage `json:"metadata"`

	PrevHash  []byte `json:"prev-hash"`
	EventHash []byte `json:"event-hash"`
}

type Repository struct{}

func (r *Repository) InsertEvent(
	ctx context.Context,
	tx pgx.Tx,
	event *AuditEvent,
) error {

	var prevHash []byte

	err := tx.QueryRow(ctx, `
		SELECT event_hash
		FROM audit_events
		WHERE tenant_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, event.TenantID).Scan(&prevHash)

	// TODO: Create an abstraction layer so that
	// we are not so tightly coupled to postgresql.
	if err == pgx.ErrNoRows {
		prevHash = GenesisHash[:]
	} else if err != nil {
		return err
	}

	event.PrevHash = prevHash
	event.EventHash = ComputeHash(
		event.TenantID,
		event.OccurredAt,
		event.Actor,
		event.Resource,
		event.Metadata,
		event.Action,
		prevHash,
	)

	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (
			tenant_id,
			occurred_at,
			actor,
			action,
			resource,
			metadata,
			prev_hash,
			event_hash
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`,
		event.TenantID,
		event.OccurredAt,
		event.Actor,
		event.Action,
		event.Resource,
		event.Metadata,
		event.PrevHash,
		event.EventHash,
	)

	return err
}

func (r *Repository) InsertEvents(ctx context.Context, tx pgx.Tx, events []*AuditEvent,
) error {
	if len(events) == 0 {
		return nil
	}

	// Get the last event hash for the tenant from the first event
	// All events in the batch should be for the same tenant
	tenantID := events[0].TenantID
	var prevHash []byte

	err := tx.QueryRow(ctx, `
		SELECT event_hash
		FROM audit_events
		WHERE tenant_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, tenantID).Scan(&prevHash)

	if err == pgx.ErrNoRows {
		prevHash = GenesisHash[:]
	} else if err != nil {
		return err
	}

	// Process events sequentially to compute hashes
	for _, event := range events {
		event.PrevHash = prevHash
		event.EventHash = ComputeHash(
			event.TenantID,
			event.OccurredAt,
			event.Actor,
			event.Resource,
			event.Metadata,
			event.Action,
			prevHash,
		)
		prevHash = event.EventHash
	}

	// Batch insert all events
	batch := &pgx.Batch{}
	for _, event := range events {
		batch.Queue(`
			INSERT INTO audit_events (
				tenant_id,
				occurred_at,
				actor,
				action,
				resource,
				metadata,
				prev_hash,
				event_hash
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`,
			event.TenantID,
			event.OccurredAt,
			event.Actor,
			event.Action,
			event.Resource,
			event.Metadata,
			event.PrevHash,
			event.EventHash,
		)
	}

	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	// Check for errors in batch results
	for i := 0; i < batch.Len(); i++ {
		_, err := br.Exec()
		if err != nil {
			return err
		}
	}

	return nil
}
