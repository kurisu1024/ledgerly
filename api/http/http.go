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
	mux          *http.ServeMux
	handler      http.Handler // Wrapped handler with middleware
	storage      storage.Storage
	ruleStore    storage.RuleStore
	queue        chan audit.Event
	worker       audit.Worker
	logger       *zap.Logger
	jwtPublicKey *rsa.PublicKey // Public key for JWT verification
	// allowUnverifiedJWT accepts decode-only tokens when no public key is
	// configured. Dev mode only; never the default.
	allowUnverifiedJWT bool
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
	// JWTPublicKey is the RSA public key for verifying JWT tokens. When set,
	// every token's RS256 signature and expiry are verified.
	JWTPublicKey *rsa.PublicKey
	// AllowUnverifiedJWT accepts tokens without signature verification when
	// no JWTPublicKey is configured. For local development only — with no
	// key and this flag false, every request is rejected with 401.
	AllowUnverifiedJWT bool
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
func New(ctx context.Context, stor storage.Storage, ruleStore storage.RuleStore, cfg Config, logger *zap.Logger) *T {
	queue := make(chan audit.Event, cfg.QueueSize)

	// Create chain writer that writes to storage. The writer's ctx must
	// survive cancellation of the run ctx: service.Run cancels ctx to begin
	// shutdown, and the worker's final drain-and-flush happens *after* that
	// — a cancellation-respecting backend (Postgres) would otherwise refuse
	// the flush and silently drop 202-acknowledged events.
	chainWriter := storage.NewChainWriter(context.WithoutCancel(ctx), stor)

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
		mux:                mux,
		storage:            stor,
		ruleStore:          ruleStore,
		queue:              queue,
		worker:             worker,
		logger:             logger,
		jwtPublicKey:       cfg.JWTPublicKey,
		allowUnverifiedJWT: cfg.AllowUnverifiedJWT,
	}

	if cfg.JWTPublicKey == nil {
		if cfg.AllowUnverifiedJWT {
			logger.Warn("JWT signature verification DISABLED (AllowUnverifiedJWT) — dev mode only, never run this in production")
		} else {
			logger.Error("no JWT public key configured and AllowUnverifiedJWT is false — every request will be rejected with 401")
		}
	}

	// Register routes with JWT auth middleware
	mux.HandleFunc("POST /v1/events", t.authMiddleware(t.CreateEvent))
	mux.HandleFunc("GET /v1/export", t.authMiddleware(t.ExportEvents))

	mux.HandleFunc("GET /v1/rules", t.authMiddleware(t.ListRules))
	mux.HandleFunc("POST /v1/rules", t.authMiddleware(t.CreateRule))
	mux.HandleFunc("GET /v1/rules/{id}", t.authMiddleware(t.GetRule))
	mux.HandleFunc("PUT /v1/rules/{id}", t.authMiddleware(t.UpdateRule))
	mux.HandleFunc("DELETE /v1/rules/{id}", t.authMiddleware(t.DeleteRule))

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
