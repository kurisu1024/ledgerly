package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AuditEvent struct {
	TenantID   uuid.UUID
	OccurredAt time.Time

	Actor    json.RawMessage
	Action   string
	Resource json.RawMessage
	Metadata json.RawMessage

	PrevHash  []byte
	EventHash []byte
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
