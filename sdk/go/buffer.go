package ledgerly

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Buffer file names within the buffer dir: the append-only JSONL segment
// and the fsync'd replay cursor (a fixed-width counter of records from
// the segment head already redelivered, rewritten in place).
const (
	segmentFile = "spill.jsonl"
	cursorFile  = "spill.cursor"
)

// buffer is the append-only local disk buffer records spill to on
// delivery failure: JSONL segments plus an fsync'd cursor, so spilled
// records replay in order on retry AND on restart (the Phase B healing
// hook keeps original stamps intact when this is eventually drained by
// outage-recovery tooling).
type buffer struct {
	dir string

	mu      sync.Mutex
	f       *os.File // append handle to the segment
	cursorF *os.File // in-place rewrite handle to the cursor
	acked   int      // records from the segment head already redelivered
	closed  bool
}

// openBuffer opens (or creates) a disk buffer rooted at dir, truncating
// any corrupt tail (a partial line from a crash mid-append) back to the
// last newline boundary.
func openBuffer(dir string) (*buffer, error) {
	if dir == "" {
		return nil, errors.New("ledgerly: buffer requires a non-empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ledgerly: creating buffer dir: %w", err)
	}

	b := &buffer{dir: dir}
	if err := b.truncateCorruptTail(); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(b.segmentPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ledgerly: opening buffer segment: %w", err)
	}
	b.f = f

	cursorF, err := os.OpenFile(b.cursorPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("ledgerly: opening buffer cursor: %w", err)
	}
	b.cursorF = cursorF

	acked, err := readCounter(cursorF)
	if err != nil {
		_ = f.Close()
		_ = cursorF.Close()
		return nil, fmt.Errorf("ledgerly: reading buffer cursor: %w", err)
	}
	b.acked = int(acked)

	// A crash between compaction's Truncate(0) and its cursor rewrite (see
	// ack) leaves the cursor pointing past the now-shorter segment. That
	// state is only reachable post-compaction — every line present was
	// spilled after the truncate and is unacked — so clamp to zero and
	// persist, or those records would silently never replay. Replaying from
	// the start is safe: delivery is at-least-once and duplicates share
	// instance-id+seq.
	lines, err := b.linesLocked()
	if err != nil {
		_ = f.Close()
		_ = cursorF.Close()
		return nil, err
	}
	if b.acked > len(lines) {
		b.acked = 0
		if err := writeCounter(cursorF, 0); err != nil {
			_ = f.Close()
			_ = cursorF.Close()
			return nil, fmt.Errorf("ledgerly: resetting stale buffer cursor: %w", err)
		}
	}
	return b, nil
}

func (b *buffer) segmentPath() string { return filepath.Join(b.dir, segmentFile) }
func (b *buffer) cursorPath() string  { return filepath.Join(b.dir, cursorFile) }

// truncateCorruptTail chops any bytes after the segment's last newline —
// a crash mid-append leaves a partial record that would otherwise poison
// every subsequent replay.
func (b *buffer) truncateCorruptTail() error {
	raw, err := os.ReadFile(b.segmentPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ledgerly: reading buffer segment: %w", err)
	}

	end := bytes.LastIndexByte(raw, '\n') + 1
	if end == len(raw) {
		return nil
	}
	if err := os.Truncate(b.segmentPath(), int64(end)); err != nil {
		return fmt.Errorf("ledgerly: truncating corrupt buffer tail: %w", err)
	}
	return nil
}

// append spills rec (an already-encoded event body) to the buffer,
// fsync'd before returning so a crash cannot lose an acknowledged spill.
func (b *buffer) append(rec []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errors.New("ledgerly: buffer is closed")
	}

	line := make([]byte, 0, len(rec)+1)
	line = append(line, rec...)
	line = append(line, '\n')
	if _, err := b.f.Write(line); err != nil {
		return fmt.Errorf("ledgerly: appending to buffer segment: %w", err)
	}
	if err := b.f.Sync(); err != nil {
		return fmt.Errorf("ledgerly: syncing buffer segment: %w", err)
	}
	return nil
}

// replay returns every unacked buffered record in original spill order,
// oldest first, for redelivery.
func (b *buffer) replay() ([][]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines, err := b.linesLocked()
	if err != nil {
		return nil, err
	}
	if b.acked >= len(lines) {
		return [][]byte{}, nil
	}
	return lines[b.acked:], nil
}

// linesLocked reads the segment and splits it into one record per line.
// Callers must hold b.mu.
func (b *buffer) linesLocked() ([][]byte, error) {
	raw, err := os.ReadFile(b.segmentPath())
	if errors.Is(err, os.ErrNotExist) {
		return [][]byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ledgerly: reading buffer segment: %w", err)
	}

	lines := [][]byte{}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// ack advances the fsync'd cursor past n successfully redelivered records,
// so a subsequent replay (including after a restart) does not resend them.
// When every record is acked the segment is compacted back to empty.
func (b *buffer) ack(n int) error {
	if n <= 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	lines, err := b.linesLocked()
	if err != nil {
		return err
	}
	b.acked += n

	if b.acked >= len(lines) && !b.closed {
		if err := b.f.Truncate(0); err != nil {
			return fmt.Errorf("ledgerly: compacting buffer segment: %w", err)
		}
		b.acked = 0
	}
	if err := writeCounter(b.cursorF, uint64(b.acked)); err != nil {
		return fmt.Errorf("ledgerly: writing buffer cursor: %w", err)
	}
	return nil
}

// isEmpty reports whether the buffer currently holds any unacked records.
func (b *buffer) isEmpty() (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines, err := b.linesLocked()
	if err != nil {
		return false, err
	}
	return b.acked >= len(lines), nil
}

// close flushes and closes the buffer's open segment file.
func (b *buffer) close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true

	if err := b.f.Sync(); err != nil {
		_ = b.f.Close()
		_ = b.cursorF.Close()
		return fmt.Errorf("ledgerly: syncing buffer segment on close: %w", err)
	}
	if err := b.f.Close(); err != nil {
		_ = b.cursorF.Close()
		return fmt.Errorf("ledgerly: closing buffer segment: %w", err)
	}
	if err := b.cursorF.Close(); err != nil {
		return fmt.Errorf("ledgerly: closing buffer cursor: %w", err)
	}
	return nil
}
