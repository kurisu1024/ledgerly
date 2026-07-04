package ledgerly

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
)

// Sequencer state files within the buffer dir. instance.json holds the
// per-SDK-instance ID plus the clean-close checkpoint and is only written
// on open and close (atomically, via rename + fsync). The reservation
// high-water mark changes constantly, so it lives in its own fixed-width
// slot file rewritten in place through a persistent handle — checksummed
// alternating slots (counter.go), so a torn write can never surface as a
// silently rewound counter.
const (
	seqStateFile   = "instance.json"
	seqReserveFile = "seq.reserve"
)

// seqState is the on-disk form of instance.json. When Clean is true,
// Checkpoint holds the exact last-issued sequence number (written by a
// clean close); otherwise the fsync'd high-water mark in seqReserveFile
// bounds what may have been issued, and resuming from it may show a false
// gap of at most one reservation block.
type seqState struct {
	InstanceID string `json:"instance-id"`
	Checkpoint uint64 `json:"checkpoint"`
	Clean      bool   `json:"clean"`
}

// sequencer assigns a monotonic, crash-safe sequence number per SDK
// instance, persisted alongside the disk buffer (ADR-0001 amendments: gap
// detectability). Numbers are reserved in blocks — an async refill plus an
// fsync'd high-water mark — so next() rarely blocks on disk I/O. A clean
// close() checkpoints the exact last-issued value; an unclean shutdown may
// leave a false gap bounded by one reservation block on the next open.
type sequencer struct {
	dir       string
	blockSize uint64

	// mu guards the in-memory counter state below. It is never held
	// across file I/O — persistence is serialized by fileMu instead.
	mu         sync.Mutex
	instanceID string
	last       uint64 // last issued sequence number
	highWater  uint64 // top of the current fsync'd reservation
	refilling  bool
	closed     bool

	// fileMu serializes reservation writes. persistedHW tracks the
	// high-water mark on disk so a slow refill can never regress a newer
	// reservation.
	fileMu      sync.Mutex
	reserveF    *os.File
	persistedHW uint64

	refills sync.WaitGroup

	// diag, when set (once, before any next() call — read without
	// synchronization), receives async refill errors as an operator
	// diagnostic; the sync fallback in next() still surfaces them.
	diag func(error)
}

// openSequencer opens (or creates, for a fresh dir) a sequence counter
// persisted under dir, reserving blockSize numbers at a time.
func openSequencer(dir string, blockSize uint64) (*sequencer, error) {
	if dir == "" {
		return nil, errors.New("ledgerly: sequencer requires a non-empty dir")
	}
	if blockSize == 0 {
		return nil, errors.New("ledgerly: sequencer requires a non-zero block size")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ledgerly: creating sequencer dir: %w", err)
	}

	s := &sequencer{dir: dir, blockSize: blockSize}

	reserveF, err := os.OpenFile(filepath.Join(dir, seqReserveFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ledgerly: opening sequence reservation file: %w", err)
	}
	s.reserveF = reserveF
	reservedHW, err := readCounter(reserveF)
	if err != nil {
		_ = reserveF.Close()
		return nil, fmt.Errorf("ledgerly: reading sequence reservation: %w", err)
	}

	raw, err := os.ReadFile(s.statePath())
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.instanceID = uuid.NewString()
		s.last = 0
	case err != nil:
		_ = reserveF.Close()
		return nil, fmt.Errorf("ledgerly: reading sequencer state: %w", err)
	default:
		var st seqState
		if err := json.Unmarshal(raw, &st); err != nil {
			_ = reserveF.Close()
			return nil, fmt.Errorf("ledgerly: decoding sequencer state: %w", err)
		}
		s.instanceID = st.InstanceID
		if st.Clean {
			// Clean shutdown: resume exactly after the last issued number.
			s.last = st.Checkpoint
		} else {
			// Crash: everything up to the fsync'd high-water mark may have
			// been issued. Skip past it — never reuse a number.
			s.last = reservedHW
		}
	}

	// Invalidate any stale checkpoint before issuing anything: from here
	// until close, a crash must resume from the high-water mark.
	st := seqState{InstanceID: s.instanceID}
	if err := writeFileSync(s.statePath(), st); err != nil {
		_ = reserveF.Close()
		return nil, fmt.Errorf("ledgerly: writing sequencer state: %w", err)
	}

	// Burn the first reservation before issuing anything, so a crash at
	// any point never reuses a sequence number. Written unconditionally —
	// after a clean close it may lower a leftover reservation, which is
	// safe (those numbers were never issued) and keeps any false crash
	// gap bounded by one block.
	target := s.last + s.blockSize
	if err := writeCounter(reserveF, target); err != nil {
		_ = reserveF.Close()
		return nil, fmt.Errorf("ledgerly: reserving initial sequence block: %w", err)
	}
	s.persistedHW = target
	s.highWater = target
	return s, nil
}

func (s *sequencer) statePath() string {
	return filepath.Join(s.dir, seqStateFile)
}

// next allocates and returns the next sequence number. Safe for concurrent
// use — a dropped-on-full-queue record still calls next() before the drop,
// so the drop itself is chain-evidenced by the resulting gap.
//
// The common path is memory-only: a background refill extends the fsync'd
// reservation before it runs out. The synchronous fallback (reservation
// fully exhausted) is rare and bounded to a single fixed-width counter
// write.
func (s *sequencer) next() (uint64, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, errors.New("ledgerly: sequencer is closed")
	}

	for s.last >= s.highWater {
		target := s.last + s.blockSize
		s.mu.Unlock()
		if err := s.reserve(target); err != nil {
			return 0, err
		}
		s.mu.Lock()
		if target > s.highWater {
			s.highWater = target
		}
	}

	s.last++
	n := s.last

	// Kick an async refill once half the block is consumed, so the
	// synchronous path above stays rare.
	if s.highWater-s.last <= s.blockSize/2 && !s.refilling {
		s.refilling = true
		s.refills.Add(1)
		go s.refillAsync(s.last + s.blockSize)
	}
	s.mu.Unlock()
	return n, nil
}

// refillAsync extends the fsync'd reservation to target in the background.
// The disk write happens before the in-memory high-water mark is raised,
// so next() only ever issues numbers covered by a persisted reservation.
func (s *sequencer) refillAsync(target uint64) {
	defer s.refills.Done()
	err := s.reserve(target)
	if err != nil && s.diag != nil {
		s.diag(err)
	}

	s.mu.Lock()
	if err == nil && target > s.highWater {
		s.highWater = target
	}
	s.refilling = false
	s.mu.Unlock()
}

// reserve persists target as the new high-water mark, unless a newer
// reservation already reached disk. Never called with s.mu held.
func (s *sequencer) reserve(target uint64) error {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	if target <= s.persistedHW {
		return nil
	}
	if err := writeCounter(s.reserveF, target); err != nil {
		return fmt.Errorf("ledgerly: reserving sequence block: %w", err)
	}
	s.persistedHW = target
	return nil
}

// close checkpoints the exact last-issued sequence number so a subsequent
// openSequencer on the same dir resumes with no gap.
func (s *sequencer) close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	last := s.last
	s.mu.Unlock()

	// Let any in-flight refill land before the checkpoint, so it cannot
	// interleave with the shutdown writes below.
	s.refills.Wait()

	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	st := seqState{InstanceID: s.instanceID, Checkpoint: last, Clean: true}
	if err := writeFileSync(s.statePath(), st); err != nil {
		_ = s.reserveF.Close()
		return fmt.Errorf("ledgerly: checkpointing sequence counter: %w", err)
	}
	if err := s.reserveF.Close(); err != nil {
		return fmt.Errorf("ledgerly: closing sequence reservation file: %w", err)
	}
	return nil
}

// writeFileSync atomically replaces path with the JSON encoding of v:
// write to a temp file, fsync it, then rename over path.
func writeFileSync(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", filepath.Base(path), err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", filepath.Base(path), err)
	}

	// fsync the directory so the rename itself survives a crash — without
	// it the new file's directory entry may not be durable.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("opening dir of %s: %w", filepath.Base(path), err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("syncing dir of %s: %w", filepath.Base(path), err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("closing dir of %s: %w", filepath.Base(path), err)
	}
	return nil
}
