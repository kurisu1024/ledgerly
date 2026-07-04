package ledgerly

import (
	"context"
	"time"
)

// sender drains a bounded queue of encoded records to the server. On
// delivery failure it spills to the disk buffer and retries with a capped
// exponential backoff; on recovery it replays buffered records in order
// before resuming live delivery — never slowing the host app in the
// meantime (ADR-0001).
type sender struct {
	client      *apiClient
	buf         *buffer
	queue       <-chan []byte
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

// newSender constructs a sender. It does not start draining until run is
// called.
func newSender(client *apiClient, buf *buffer, queue <-chan []byte, baseBackoff, maxBackoff time.Duration) *sender {
	return &sender{client: client, buf: buf, queue: queue, baseBackoff: baseBackoff, maxBackoff: maxBackoff}
}

// run drains queue until ctx is done, POSTing each record and spilling to
// buf on failure.
//
// STUB: not implemented.
func (s *sender) run(ctx context.Context) {}

// close drains any in-flight sends and checkpoints the buffer cursor so a
// restart resumes exactly where delivery left off.
//
// STUB: not implemented.
func (s *sender) close(ctx context.Context) error {
	return ErrNotImplemented
}

// backoffDuration returns the delay before retry attempt n (1-indexed),
// doubling from base and capped at max.
//
// STUB: not implemented — always returns zero, which is wrong for every n
// greater than 0 and fails to respect max.
func backoffDuration(attempt int, base, max time.Duration) time.Duration {
	return 0
}
