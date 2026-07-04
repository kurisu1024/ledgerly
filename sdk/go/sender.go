package ledgerly

import (
	"context"
	"math/rand/v2"
	"net/http"
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

	// done is closed when run returns, so Close can wait for the drain.
	done chan struct{}
}

// newSender constructs a sender. It does not start draining until run is
// called.
func newSender(client *apiClient, buf *buffer, queue <-chan []byte, baseBackoff, maxBackoff time.Duration) *sender {
	return &sender{
		client:      client,
		buf:         buf,
		queue:       queue,
		baseBackoff: baseBackoff,
		maxBackoff:  maxBackoff,
		done:        make(chan struct{}),
	}
}

// run drains queue until ctx is done or the queue is closed and empty,
// POSTing each record and spilling to buf on failure. It starts by
// replaying anything a previous process left in the buffer (restart
// replay, original stamps intact).
func (s *sender) run(ctx context.Context) {
	defer close(s.done)

	s.drainBuffer(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case body, ok := <-s.queue:
			if !ok {
				return
			}
			s.deliver(ctx, body)
		}
	}
}

// deliver attempts one live send; on failure it spills body to the buffer
// and blocks in the retry loop until the buffer drains (preserving spill
// order ahead of newer queue items) or ctx is done.
func (s *sender) deliver(ctx context.Context, body []byte) {
	if s.send(ctx, body) {
		return
	}
	if err := s.buf.append(body); err != nil {
		// Nowhere left to put the record: drop it. Its burned sequence
		// number leaves a chain-evidenced gap.
		return
	}
	s.drainBuffer(ctx)
}

// drainBuffer replays unacked buffered records in original order until the
// buffer is empty, retrying with capped, jittered exponential backoff
// between rounds. Partial progress is acked incrementally so a crash never
// resends what the server already accepted.
func (s *sender) drainBuffer(ctx context.Context) {
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(jittered(backoffDuration(attempt, s.baseBackoff, s.maxBackoff))):
			}
		}

		records, err := s.buf.replay()
		if err != nil || len(records) == 0 {
			return
		}

		sent := 0
		for _, rec := range records {
			if !s.send(ctx, rec) {
				break
			}
			sent++
		}
		if sent > 0 {
			if err := s.buf.ack(sent); err != nil {
				return
			}
		}
		if sent == len(records) {
			return
		}
	}
}

// send POSTs body and reports whether the server accepted it (any 2xx —
// the canonical reply is 202, meaning accepted into the async ingest
// queue, never persisted).
func (s *sender) send(ctx context.Context, body []byte) bool {
	status, err := s.client.postEvent(ctx, body)
	return err == nil && status >= http.StatusOK && status < http.StatusMultipleChoices
}

// backoffDuration returns the delay before retry attempt n (1-indexed),
// doubling from base and capped at max.
func backoffDuration(attempt int, base, max time.Duration) time.Duration {
	if base > max {
		base = max
	}
	d := base
	for i := 1; i < attempt; i++ {
		if d >= max/2 {
			return max
		}
		d *= 2
	}
	return d
}

// jittered spreads a backoff delay uniformly over [d/2, d) so a fleet of
// SDK instances recovering from the same outage doesn't retry in
// lockstep.
func jittered(d time.Duration) time.Duration {
	if d <= 1 {
		return d
	}
	half := d / 2
	return half + rand.N(d-half)
}
