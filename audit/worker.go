package audit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Worker interface {
	Start(ctx context.Context) Worker
	Stop()
}

type batchInsertFunc func(ctx context.Context, e []*AuditEvent) error

func NewBatchInsertWorker(batchSize int, timeout time.Duration, insertFunc batchInsertFunc, q chan *AuditEvent) Worker {
	return &batchInsertWorker{
		queue:      q,
		batchSize:  batchSize,
		timeout:    timeout,
		insertFunc: insertFunc,
		logger:     log.With(zap.String("category", "worker")),
	}
}

type batchInsertWorker struct {
	queue      chan *AuditEvent
	batchSize  int
	timeout    time.Duration
	cancel     context.CancelFunc
	insertFunc batchInsertFunc
	logger     *zap.Logger
	wg         *sync.WaitGroup
}

func (w *batchInsertWorker) Stop() {
	w.logger.Info("stopping worker")
	w.cancel()
	w.wg.Wait()
}

func (w *batchInsertWorker) Start(ctx context.Context) Worker {
	go w.start(ctx)
	return w
}

func (w *batchInsertWorker) start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	w.wg = &sync.WaitGroup{}
	w.wg.Add(1)
	defer w.wg.Done()
	tenantBatchMap := make(map[string][]*AuditEvent)
	ticker := time.NewTicker(w.timeout)

	for {
		select {
		case event := <-w.queue:
			batch, ok := tenantBatchMap[event.TenantID.String()]
			if !ok {
				batch = make([]*AuditEvent, 0, w.batchSize)
			}
			batch = append(batch, event)
			if len(batch) >= w.batchSize {
				w.flushBatch(ctx, batch)
				batch = batch[:0]
			}
			tenantBatchMap[event.TenantID.String()] = batch
		case <-ticker.C:
			for id, batch := range tenantBatchMap {
				if len(batch) > 0 {
					w.flushBatch(ctx, batch)
					batch = batch[:0]
					tenantBatchMap[id] = batch
				}
			}
		case <-ctx.Done():
			for id, batch := range tenantBatchMap {
				if len(batch) > 0 {
					w.flushBatch(ctx, batch)
					batch = batch[:0]
					tenantBatchMap[id] = batch
				}
				return
			}
		}
	}
}

func (w *batchInsertWorker) flushBatch(ctx context.Context, batch []*AuditEvent) {
	if len(batch) == 0 {
		return
	}
	// TODO: make this a Temporal workflow activity for durable execution.
	if err := w.insertFunc(ctx, batch); err != nil {
		w.logger.Error(fmt.Errorf("failed to insert batch: %w", err).Error(),
			zap.String("tenant-id", batch[0].TenantID.String()),
		)
	}
}
