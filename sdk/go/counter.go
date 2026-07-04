package ledgerly

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// Persistent counters (the sequence reservation high-water mark in
// seq.reserve and the buffer replay cursor in spill.cursor) are stored as
// two alternating fixed-width slots, each a checksummed record:
//
//	%020d %020d %08x\n   — generation, value, CRC-32 (IEEE) of "gen value"
//
// Writes always target the slot NOT holding the newest valid record, so a
// torn or interrupted write can only garble the slot being written — the
// other slot still holds the last durable value. Reads take the valid slot
// with the highest generation. A non-empty file with no valid slot fails
// loudly rather than silently parsing a garbled (possibly smaller) number;
// for the sequencer, a silent rewind would mean duplicated sequence
// numbers.
const (
	counterFieldWidth = 20 // zero-padded decimal, wide enough for any uint64
	counterCRCOffset  = counterFieldWidth + 1 + counterFieldWidth + 1
	counterSlotSize   = counterCRCOffset + 8 + 1
)

// errCounterCorrupt reports a non-empty counter file in which no slot
// passes its checksum: there is no safe value to resume from.
var errCounterCorrupt = errors.New("counter file corrupt: no slot passes its checksum")

// counterSlot is one parsed slot of a counter file. gen and value are only
// meaningful when valid is true.
type counterSlot struct {
	gen   uint64
	value uint64
	valid bool
}

// readCounterSlots reads and parses both slots of f. A slot beyond the
// current file size, or one failing its checksum, comes back invalid. A
// genuine mid-read I/O error is returned as an error — never conflated
// with a fresh (size 0) or corrupt file.
func readCounterSlots(f *os.File) (slots [2]counterSlot, size int, err error) {
	buf := make([]byte, 2*counterSlotSize)
	n, err := f.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return slots, 0, fmt.Errorf("reading counter file: %w", err)
	}
	for i := range slots {
		start := i * counterSlotSize
		if n >= start+counterSlotSize {
			slots[i] = parseCounterSlot(buf[start : start+counterSlotSize])
		}
	}
	return slots, n, nil
}

// parseCounterSlot decodes one slot, returning an invalid slot for any
// framing, checksum, or numeric failure.
func parseCounterSlot(b []byte) counterSlot {
	var s counterSlot
	if b[counterFieldWidth] != ' ' || b[counterCRCOffset-1] != ' ' || b[counterSlotSize-1] != '\n' {
		return s
	}
	var crc uint32
	if _, err := fmt.Sscanf(string(b[counterCRCOffset:counterSlotSize-1]), "%08x", &crc); err != nil {
		return s
	}
	if crc != crc32.ChecksumIEEE(b[:counterCRCOffset-1]) {
		return s
	}
	if _, err := fmt.Sscanf(string(b[:counterCRCOffset-1]), "%d %d", &s.gen, &s.value); err != nil {
		return s
	}
	s.valid = true
	return s
}

// newestValidSlot returns the index of the valid slot with the highest
// generation, or -1 when neither slot is valid.
func newestValidSlot(slots [2]counterSlot) int {
	best := -1
	for i, s := range slots {
		if s.valid && (best == -1 || s.gen > slots[best].gen) {
			best = i
		}
	}
	return best
}

// readCounter returns the counter's last durably written value. A fresh
// (empty) file reads as zero; a non-empty file with no valid slot returns
// errCounterCorrupt.
func readCounter(f *os.File) (uint64, error) {
	slots, size, err := readCounterSlots(f)
	if err != nil {
		return 0, err
	}
	if size == 0 {
		return 0, nil
	}
	best := newestValidSlot(slots)
	if best == -1 {
		return 0, errCounterCorrupt
	}
	return slots[best].value, nil
}

// writeCounter durably records v as the counter's current value: it writes
// a fresh checksummed record into the slot not holding the newest valid
// one and fsyncs, so the previous value survives intact if this write
// tears.
func writeCounter(f *os.File, v uint64) error {
	slots, _, err := readCounterSlots(f)
	if err != nil {
		return fmt.Errorf("writing counter: %w", err)
	}

	gen := uint64(1)
	if best := newestValidSlot(slots); best != -1 {
		gen = slots[best].gen + 1
	}

	rec := fmt.Appendf(nil, "%0*d %0*d ", counterFieldWidth, gen, counterFieldWidth, v)
	rec = fmt.Appendf(rec, "%08x\n", crc32.ChecksumIEEE(rec[:counterCRCOffset-1]))

	// Alternation invariant: every record at generation g lives in slot
	// g%2, so gen (= newest valid gen + 1) never lands on the newest slot.
	offset := int64(gen%2) * counterSlotSize
	if _, err := f.WriteAt(rec, offset); err != nil {
		return fmt.Errorf("writing counter: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing counter: %w", err)
	}
	return nil
}
