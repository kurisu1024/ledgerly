package ledgerly

import (
	"errors"
	"fmt"
	"os"
)

// buffer is the append-only local disk buffer records spill to on
// delivery failure: JSONL segments plus an fsync'd cursor, so spilled
// records replay in order on retry AND on restart (the Phase B healing
// hook keeps original stamps intact when this is eventually drained by
// outage-recovery tooling).
type buffer struct {
	dir string
}

// openBuffer opens (or creates) a disk buffer rooted at dir.
//
// PARTIAL STUB: only validates and creates dir. Segment files, the fsync'd
// cursor, and replay (below) are not implemented yet.
func openBuffer(dir string) (*buffer, error) {
	if dir == "" {
		return nil, errors.New("ledgerly: buffer requires a non-empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ledgerly: creating buffer dir: %w", err)
	}
	return &buffer{dir: dir}, nil
}

// append spills rec (an already-encoded event body) to the buffer.
//
// STUB: not implemented.
func (b *buffer) append(rec []byte) error {
	return ErrNotImplemented
}

// replay returns every buffered record in original spill order, oldest
// first, for redelivery.
//
// STUB: not implemented.
func (b *buffer) replay() ([][]byte, error) {
	return nil, ErrNotImplemented
}

// ack advances the fsync'd cursor past n successfully redelivered records,
// so a subsequent replay (including after a restart) does not resend them.
//
// STUB: not implemented.
func (b *buffer) ack(n int) error {
	return ErrNotImplemented
}

// isEmpty reports whether the buffer currently holds any unacked records.
//
// STUB: not implemented.
func (b *buffer) isEmpty() (bool, error) {
	return false, ErrNotImplemented
}

// close flushes and closes the buffer's open segment file.
//
// STUB: not implemented.
func (b *buffer) close() error {
	return ErrNotImplemented
}
