package http

import (
	"context"
	"net/http"
	"time"

	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage"
	"go.uber.org/zap"
)

type T struct {
	mux     *http.ServeMux
	storage storage.Storage
	queue   chan audit.Event
	worker  audit.Worker
	logger  *zap.Logger
}

func (t *T) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.mux.ServeHTTP(w, r)
}

// Config holds configuration for the HTTP server.
type Config struct {
	QueueSize int
	ChainSize int
	// FlushInterval is how often the worker flushes partial chains to storage
	FlushInterval time.Duration
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() Config {
	return Config{
		QueueSize:     1000,
		ChainSize:     100,
		FlushInterval: 5 * time.Second,
	}
}

// New creates a new HTTP server with the provided storage backend and configuration.
// It starts a worker to process events asynchronously.
func New(ctx context.Context, stor storage.Storage, cfg Config, logger *zap.Logger) *T {
	queue := make(chan audit.Event, cfg.QueueSize)

	// Create chain writer that writes to storage
	chainWriter := storage.NewChainWriter(ctx, stor)

	// Create and start worker
	worker := audit.NewBatchInsertWorker(
		cfg.ChainSize,
		cfg.FlushInterval,
		chainWriter,
		queue,
		logger,
	).Start(ctx)

	t := &T{
		mux:     http.NewServeMux(),
		storage: stor,
		queue:   queue,
		worker:  worker,
		logger:  logger,
	}

	// Register routes
	t.mux.HandleFunc("POST /tenants/{tenantID}/events", t.CreateEvent)
	t.mux.HandleFunc("GET /tenants/{tenantID}/events", t.ExportEvents)

	return t
}

// Close gracefully shuts down the server, stopping the worker and closing the queue.
func (t *T) Close() {
	t.logger.Info("shutting down HTTP server")
	t.worker.Stop()
	close(t.queue)
}

func (t *T) handle(pattern string, handler http.Handler) {
	t.mux.Handle(pattern, handler)
}
