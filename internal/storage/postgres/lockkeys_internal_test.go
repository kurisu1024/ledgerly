package postgres

import (
	"testing"

	"github.com/google/uuid"
)

// TestAdvisoryLockKeys pins the derivation of the two-arg
// pg_advisory_xact_lock(int, int) keys from the tenant UUID: deterministic,
// and sensitive to both halves of the UUID, so all 128 bits of the tenant ID
// feed the advisory-lock keyspace.
func TestAdvisoryLockKeys(t *testing.T) {
	id := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")

	k1a, k2a := advisoryLockKeys(id)
	k1b, k2b := advisoryLockKeys(id)
	if k1a != k1b || k2a != k2b {
		t.Fatalf("advisoryLockKeys not deterministic: (%d,%d) vs (%d,%d)", k1a, k2a, k1b, k2b)
	}

	// Flip a bit in the high 64 bits only: first key must change, second
	// must not (it is derived exclusively from the low half).
	hiFlip := id
	hiFlip[0] ^= 0x01
	h1, h2 := advisoryLockKeys(hiFlip)
	if h1 == k1a {
		t.Fatalf("flipping the high half did not change key1: %d", h1)
	}
	if h2 != k2a {
		t.Fatalf("flipping the high half changed key2: got %d, want %d", h2, k2a)
	}

	// Flip a bit in the low 64 bits only: second key must change, first
	// must not.
	loFlip := id
	loFlip[15] ^= 0x01
	l1, l2 := advisoryLockKeys(loFlip)
	if l2 == k2a {
		t.Fatalf("flipping the low half did not change key2: %d", l2)
	}
	if l1 != k1a {
		t.Fatalf("flipping the low half changed key1: got %d, want %d", l1, k1a)
	}
}
