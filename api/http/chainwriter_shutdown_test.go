package http_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	apihttp "github.com/kurisu1024/ledgerly/api/http"
	"github.com/kurisu1024/ledgerly/internal/storage"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

// ctxRespectingStorage wraps an in-memory-style store but refuses writes
// once its ctx argument is already Done — the way a real network-backed
// Storage (Postgres included) behaves. memory.InMemory ignores ctx
// entirely, which is exactly why this bug is invisible with the dev
// backend.
type ctxRespectingStorage struct {
	mu      sync.Mutex
	written []storage.Block
}

func (s *ctxRespectingStorage) WriteBlock(ctx context.Context, tenantID uuid.UUID, block storage.Block) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.written = append(s.written, block)
	return nil
}

func (s *ctxRespectingStorage) FetchBlock(ctx context.Context, tenantID, blockID uuid.UUID) (storage.Block, error) {
	return storage.Block{}, fmt.Errorf("ctxRespectingStorage: FetchBlock not implemented")
}

func (s *ctxRespectingStorage) FetchBlocks(ctx context.Context, tenantID uuid.UUID, opts storage.FetchOptions) ([]storage.Block, error) {
	return nil, fmt.Errorf("ctxRespectingStorage: FetchBlocks not implemented")
}

func (s *ctxRespectingStorage) blockCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.written)
}

var _ storage.Storage = (*ctxRespectingStorage)(nil)

// TestChainWriter_SurvivesParentContextCancellation pins the latent bug from
// issue #25 step 5: ChainWriter is built from the caller's run ctx
// (api/http/http.go), which service.Run cancels to begin shutdown. A
// context-respecting backend's final drain-and-flush on shutdown then runs
// against an already-cancelled ctx and silently loses the buffered event.
// Invisible with memory.InMemory (it ignores ctx); reproduced here with a
// fake Storage that behaves like a real network-backed one.
//
// Currently FAILS: the buffered event is lost when the parent ctx is
// cancelled before Close.
func TestChainWriter_SurvivesParentContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	store := &ctxRespectingStorage{}
	logger, _ := zap.NewDevelopment()

	cfg := apihttp.Config{
		QueueSize:     100,
		ChainSize:     100,           // large: never hits the ingest-triggered flush
		FlushInterval: 1 * time.Hour, // long: never hits the ticker-triggered flush
		JWTPublicKey:  &testKey.PublicKey,
	}

	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)

	tenantID := uuid.New()
	event := Event{
		OccurredAt: time.Now().UTC(),
		Action:     "project.create",
		Actor:      map[string]string{"id": "user1", "type": "user"},
		Resource:   map[string]string{"type": "project", "id": "proj1"},
	}

	w := doJSON(t, server, http.MethodPost, "/v1/events", tenantID, event)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 queuing the event, got %d: %s", w.Code, w.Body.String())
	}

	// Simulate service.Run's shutdown path: the parent (run) ctx is
	// cancelled first, then the handler is closed — exactly the order in
	// service.Run's `defer handler.Close()` after ctx.Done() fires.
	cancel()
	server.Close()

	if got := store.blockCount(); got == 0 {
		t.Fatalf("expected the buffered event to survive parent-ctx cancellation before Close, but storage received %d blocks", got)
	}
}

// hangingStorage blocks WriteBlock until its ctx expires — the way a hung
// network backend (Postgres included) behaves: the pgx call only returns
// when its ctx is done. Under context.WithoutCancel with nothing bounding
// the write, that is never.
type hangingStorage struct{}

func (s *hangingStorage) WriteBlock(ctx context.Context, tenantID uuid.UUID, block storage.Block) error {
	<-ctx.Done()
	return ctx.Err()
}

func (s *hangingStorage) FetchBlock(ctx context.Context, tenantID, blockID uuid.UUID) (storage.Block, error) {
	return storage.Block{}, fmt.Errorf("hangingStorage: FetchBlock not implemented")
}

func (s *hangingStorage) FetchBlocks(ctx context.Context, tenantID uuid.UUID, opts storage.FetchOptions) ([]storage.Block, error) {
	return nil, fmt.Errorf("hangingStorage: FetchBlocks not implemented")
}

var _ storage.Storage = (*hangingStorage)(nil)

// TestChainWriter_ShutdownFlushIsBounded is the counterpart to the
// survives-cancellation test above: decoupling the ChainWriter from the run
// ctx (context.WithoutCancel) must not leave the shutdown flush unbounded.
// With a hung Postgres at SIGTERM, WriteBlock would otherwise block forever
// and the process never exits. Close must return within the configured
// WriteTimeout, not hang.
func TestChainWriter_ShutdownFlushIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	logger, _ := zap.NewDevelopment()

	cfg := apihttp.Config{
		QueueSize:     100,
		ChainSize:     100,           // large: never hits the ingest-triggered flush
		FlushInterval: 1 * time.Hour, // long: never hits the ticker-triggered flush
		WriteTimeout:  200 * time.Millisecond,
		JWTPublicKey:  &testKey.PublicKey,
	}

	server := apihttp.New(ctx, &hangingStorage{}, memory.NewRules(), cfg, logger)

	tenantID := uuid.New()
	event := Event{
		OccurredAt: time.Now().UTC(),
		Action:     "project.create",
		Actor:      map[string]string{"id": "user1", "type": "user"},
		Resource:   map[string]string{"type": "project", "id": "proj1"},
	}

	w := doJSON(t, server, http.MethodPost, "/v1/events", tenantID, event)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 queuing the event, got %d: %s", w.Code, w.Body.String())
	}

	// Same shutdown order as service.Run: cancel the run ctx, then Close.
	cancel()

	done := make(chan struct{})
	go func() {
		server.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return within 5s: shutdown flush is unbounded against a hung storage backend")
	}
}
