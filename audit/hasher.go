package audit

import (
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

var GenesisHash = sha256.Sum256([]byte("GENESIS"))

func ComputeHash(tenantID uuid.UUID, occurredAt time.Time,
	actor, resource, metadata json.RawMessage,
	action string,
	prevHash []byte,
) []byte {
	h := sha256.New()

	h.Write([]byte(tenantID.String()))
	h.Write([]byte(occurredAt.UTC().Format(time.RFC3339Nano)))
	h.Write(actor)
	h.Write([]byte(action))
	h.Write(resource)
	h.Write(metadata)
	h.Write(prevHash)

	return h.Sum(nil)
}
