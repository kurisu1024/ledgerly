package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	httpapi "github.com/kurisu1024/ledgerly/api/http"
	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

var log *zap.Logger

func init() {
	initLogger("dev")
}

func initLogger(env string) {
	logger, err := newLogger(env)
	if err != nil {
		panic(err)
	}
	log = logger
}

func newLogger(env string) (*zap.Logger, error) {
	var cfg zap.Config

	if env == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}

	return cfg.Build()
}

// Service TODO: define service data structure and interface.
type Service interface {
	Run(ctx context.Context) error
}

func New() Service {
	return &T{}
}

type T struct {
	queue chan audit.Event
}

// Run starts the HTTP server and listens for requests until context is cancelled
func (s *T) Run(ctx context.Context) error {
	// Create HTTP API handler. This is the dev composition (in-memory
	// storage, hardcoded port), so it explicitly opts in to unverified JWTs
	// to keep `make load-events` working without a signing key.
	cfg := httpapi.DefaultConfig()
	cfg.AllowUnverifiedJWT = true
	handler := httpapi.New(ctx, memory.New(), cfg, log)
	defer handler.Close()

	// Create HTTP server
	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	// Channel to capture server errors
	errChan := make(chan error, 1)

	// Start server in goroutine
	go func() {
		log.Info("Starting HTTP server", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Info("Shutting down HTTP server")
		// Graceful shutdown with 5 second timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}
