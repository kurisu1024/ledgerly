package http

import (
	"context"
	"crypto/rsa"
	"net/http"
	"time"

	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage"
	"go.uber.org/zap"
)

type T struct {
	mux       *http.ServeMux
	handler   http.Handler // Wrapped handler with middleware
	storage   storage.Storage
	queue     chan audit.Event
	worker    audit.Worker
	logger    *zap.Logger
	jwtSecret *rsa.PublicKey // Public key for JWT verification
}

func (t *T) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.handler.ServeHTTP(w, r)
}

// Config holds configuration for the HTTP server.
type Config struct {
	QueueSize int
	ChainSize int
	// FlushInterval is how often the worker flushes partial chains to storage
	FlushInterval time.Duration
	// JWTPublicKey is the RSA public key for verifying JWT tokens (optional for now)
	JWTPublicKey *rsa.PublicKey
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

	mux := http.NewServeMux()

	t := &T{
		mux:       mux,
		storage:   stor,
		queue:     queue,
		worker:    worker,
		logger:    logger,
		jwtSecret: cfg.JWTPublicKey,
	}

	// Register routes with JWT auth middleware
	mux.HandleFunc("POST /v1/events", t.authMiddleware(t.CreateEvent))
	mux.HandleFunc("GET /v1/export", t.authMiddleware(t.ExportEvents))

	// Wrap mux with logging middleware
	t.handler = loggingMiddleware(logger)(mux)

	return t
}

// Flush persists every event accepted before the call. It blocks until the
// worker has written the events or ctx expires.
func (t *T) Flush(ctx context.Context) error {
	return t.worker.Flush(ctx)
}

// Close gracefully shuts down the server, stopping the worker and closing the queue.
func (t *T) Close() error {
	t.logger.Info("shutting down HTTP server")
	t.worker.Stop()
	close(t.queue)
	return nil
}

func (t *T) handle(pattern string, handler http.Handler) {
	t.mux.Handle(pattern, handler)
}
