package ledgerly

import (
	"errors"
	"fmt"
	"os"
)

// ErrNotImplemented is returned by every unimplemented internal stub in
// this package. RED stage for issue #26: the build order in the approved
// deep-plan lands real implementations behind these signatures next.
var ErrNotImplemented = errors.New("ledgerly: not implemented")

// sequencer assigns a monotonic, crash-safe sequence number per SDK
// instance, persisted alongside the disk buffer (ADR-0001 amendments: gap
// detectability). Numbers are reserved in blocks — an async refill plus an
// fsync'd high-water mark — so next() rarely blocks on disk I/O. A clean
// close() checkpoints the exact last-issued value; an unclean shutdown may
// leave a false gap bounded by one reservation block on the next open.
type sequencer struct {
	dir       string
	blockSize uint64
}

// openSequencer opens (or creates, for a fresh dir) a sequence counter
// persisted under dir, reserving blockSize numbers at a time.
//
// PARTIAL STUB: only validates and creates dir. The persisted counter,
// block reservation, and high-water-mark checkpoint (next/close below) are
// not implemented yet.
func openSequencer(dir string, blockSize uint64) (*sequencer, error) {
	if dir == "" {
		return nil, errors.New("ledgerly: sequencer requires a non-empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ledgerly: creating sequencer dir: %w", err)
	}
	return &sequencer{dir: dir, blockSize: blockSize}, nil
}

// next allocates and returns the next sequence number. Safe for concurrent
// use — a dropped-on-full-queue record still calls next() before the drop,
// so the drop itself is chain-evidenced by the resulting gap.
//
// STUB: not implemented.
func (s *sequencer) next() (uint64, error) {
	return 0, ErrNotImplemented
}

// close checkpoints the exact last-issued sequence number so a subsequent
// openSequencer on the same dir resumes with no gap.
//
// STUB: not implemented.
func (s *sequencer) close() error {
	return ErrNotImplemented
}
