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
	}
}

type batchInsertWorker struct {
	queue      chan Event
	chainSize  int
	timeout    time.Duration
	cancel     context.CancelFunc
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
	w.wg = sync.WaitGroup{}
	w.wg.Add(1)
	go w.start(ctx)
	return w
}

func (w *batchInsertWorker) start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	defer w.wg.Done()

	// chainMap is a map of `EventChain` keyed by TenantID.
	chainMap := make(map[string]EventChain)
	ticker := time.NewTicker(w.timeout)
	defer ticker.Stop()

	for {
		select {
		case event := <-w.queue:
			chain, ok := chainMap[event.TenantID.String()]
			if !ok {
				chain = NewEventChain(w.chainSize)
			}

			chain = AppendEvent(chain, event)
			if len(chain.Events) == w.chainSize {
				w.write(chain)
				delete(chainMap, event.TenantID.String())

			}
		case <-ticker.C:
			for _, chain := range chainMap {
				w.write(chain)
			}
			chainMap = make(map[string]EventChain)

		case <-ctx.Done():
			for _, chain := range chainMap {
				w.write(chain)
				return
			}
		}
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
