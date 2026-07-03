package audit_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
	"go.uber.org/zap"
)

// recordingWriter counts events written, safe for use across goroutines.
type recordingWriter struct {
	mu     sync.Mutex
	events int
}

func (r *recordingWriter) Write(chain audit.EventChain) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events += len(chain.Events)
	return nil
}

func (r *recordingWriter) eventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events
}

func TestFlushAfterStop(t *testing.T) {
	t.Log("\tGiven a started worker that has been stopped")
	q := make(chan audit.Event, 1)
	w := audit.NewBatchInsertWorker(3, time.Hour, audit.NoOpEventChainWriter{}, q, zap.NewNop()).
		Start(context.Background())
	w.Stop()

	{
		t.Log("\tWhen calling Flush after Stop")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := w.Flush(ctx)
		if !errors.Is(err, audit.ErrWorkerStopped) {
			t.Fatalf("\t%s\tExpected ErrWorkerStopped, got: %v", fail, err)
		}
		if ctx.Err() != nil {
			t.Fatalf("\t%s\tFlush did not return promptly; ctx expired", fail)
		}
		t.Logf("\t%s\tFlush returned ErrWorkerStopped without hanging", pass)
	}
}

func TestFlushPersistsQueuedEvents(t *testing.T) {
	t.Log("\tGiven a worker with a long flush interval and queued events")
	q := make(chan audit.Event, 10)
	rec := &recordingWriter{}
	w := audit.NewBatchInsertWorker(100, time.Hour, rec, q, zap.NewNop()).
		Start(context.Background())
	defer w.Stop()

	tenantID := uuid.New()
	for i := 0; i < 3; i++ {
		q <- audit.NewEvent(tenantID, actor, "project.create", resource, metadata)
	}

	{
		t.Log("\tWhen calling Flush")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := w.Flush(ctx); err != nil {
			t.Fatalf("\t%s\tFlush failed: %v", fail, err)
		}
		if got := rec.eventCount(); got != 3 {
			t.Fatalf("\t%s\tExpected 3 events written, got %d", fail, got)
		}
		t.Logf("\t%s\tFlush persisted all queued events deterministically", pass)
	}
}
