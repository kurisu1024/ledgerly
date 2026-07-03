package audit

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Worker interface {
	Start(ctx context.Context) Worker
	Stop()
	// Flush persists every event accepted before the call, blocking until
	// the write completes or ctx expires. The deterministic alternative to
	// waiting out FlushInterval.
	Flush(ctx context.Context) error
}

type batchInsertFunc func(ctx context.Context, e []*Event) error

func NewBatchInsertWorker(chainSize int, timeout time.Duration, chainWriter EventChainWriter, q chan Event,
	log *zap.Logger) Worker {
	return &batchInsertWorker{
		queue:     q,
		chainSize: chainSize,
		timeout:   timeout,
		logger:    log.With(zap.String("category", "worker-"+uuid.New().String())),
		w:         chainWriter,
		flushCh:   make(chan chan struct{}),
	}
}

type batchInsertWorker struct {
	queue      chan Event
	chainSize  int
	timeout    time.Duration
	cancel     context.CancelFunc
	flushCh    chan chan struct{}
	insertFunc batchInsertFunc
	logger     *zap.Logger
	wg         sync.WaitGroup
	w          EventChainWriter
}

func (w *batchInsertWorker) Stop() {
	w.logger.Info("stopping worker")
	w.cancel()
	w.wg.Wait()
}

func (w *batchInsertWorker) Start(ctx context.Context) Worker {
	// Assign cancel before spawning the goroutine so Stop, called from
	// another goroutine, never observes it nil or unsynchronized.
	ctx, w.cancel = context.WithCancel(ctx)
	w.wg = sync.WaitGroup{}
	w.wg.Add(1)
	go w.start(ctx)
	return w
}

func (w *batchInsertWorker) start(ctx context.Context) {
	defer w.wg.Done()

	// chainMap is a map of `EventChain` keyed by TenantID.
	chainMap := make(map[string]EventChain)
	ticker := time.NewTicker(w.timeout)
	defer ticker.Stop()

	for {
		select {
		case event := <-w.queue:
			w.ingest(chainMap, event)
		case <-ticker.C:
			w.flushAll(chainMap)
		case done := <-w.flushCh:
			w.drain(chainMap)
			w.flushAll(chainMap)
			close(done)
		case <-ctx.Done():
			// Drain events already queued (and 202-acknowledged) before
			// flushing, so shutdown never loses accepted events.
			w.drain(chainMap)
			w.flushAll(chainMap)
			return
		}
	}
}

// Flush asks the worker to persist every event accepted before the call.
func (w *batchInsertWorker) Flush(ctx context.Context) error {
	done := make(chan struct{})
	select {
	case w.flushCh <- done:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// drain ingests everything already buffered in the queue without blocking.
func (w *batchInsertWorker) drain(chainMap map[string]EventChain) {
	for {
		select {
		case event := <-w.queue:
			w.ingest(chainMap, event)
		default:
			return
		}
	}
}

// flushAll writes every accumulated chain and resets the map.
func (w *batchInsertWorker) flushAll(chainMap map[string]EventChain) {
	for _, chain := range chainMap {
		w.write(chain)
	}
	clear(chainMap)
}

// ingest appends the event to its tenant's chain, writing and evicting the
// chain when it reaches chainSize.
func (w *batchInsertWorker) ingest(chainMap map[string]EventChain, event Event) {
	chain, ok := chainMap[event.TenantID.String()]
	if !ok {
		chain = NewEventChain(w.chainSize)
	}

	chain = AppendEvent(chain, event)
	chainMap[event.TenantID.String()] = chain

	if len(chain.Events) == w.chainSize {
		w.write(chain)
		delete(chainMap, event.TenantID.String())
	}
}

// TODO: Move to using somethin like temporal for durable executions.
func (w *batchInsertWorker) write(chain EventChain) {
	if err := w.w.Write(chain); err != nil {
		w.logger.Error("failed to write batch", zap.Error(err))
	}
}

type EventChainWriter interface {
	Write(chain EventChain) error
}

type NoOpEventChainWriter struct{}

func (w NoOpEventChainWriter) Write(chain EventChain) error { return nil }
