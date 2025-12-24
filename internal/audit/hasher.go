package audit

import (
	"crypto/sha256"
	"time"
)

var genesisHash = sha256.Sum256([]byte("GENESIS"))

func computeHash(e Event) []byte {
	h := sha256.New()

	h.Write([]byte(e.ChainID.String()))
	h.Write([]byte(e.TenantID.String()))
	h.Write([]byte(e.OccurredAt.UTC().Format(time.RFC3339Nano)))
	h.Write(e.Actor)
	h.Write([]byte(e.Action))
	h.Write(e.Resource)
	h.Write(e.Metadata)
	h.Write(e.PrevHash)

	return h.Sum(nil)
}
