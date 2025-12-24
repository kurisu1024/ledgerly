package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/config"
	"github.com/kurisu1024/ledgerly/internal/audit"
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

	cfg.Encoding = "json"
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	return cfg.Build()
}

// Service TODO: define service data structure and interface.
type Service interface {
	Run(ctx context.Context) error
}

func New() Service {
	return &service{}
}

type service struct {
	queue chan audit.Event
}

// Run TODO: Implement
func (s *service) Run(ctx context.Context) error {
	cfg := config.Default()
	//pool, err := db.NewPool(ctx)
	//if err != nil {
	//	return fmt.Errorf("failed to connect to database: %w", err)
	//}
	//s.db = pool

	queue := make(chan audit.Event, cfg.QueueSize)
	defer close(queue)
	workers := make([]audit.Worker, cfg.WorkerCount)
	for i := 0; i < cfg.WorkerCount; i++ {
		w := audit.NewBatchInsertWorker(cfg.ChainSize, time.Second, audit.NoOpEventChainWriter{}, queue, log).Start(ctx)
		workers[i] = w
	}

	//handler := handlers.NewHandler(s.RecordEvent)
	//
	//router := gin.Default()
	//router.POST("/v1/events", handler.PostEvent)
	//router.GET("/v1/events", handler.GetEvents)
	//router.POST("/v1/verify", handler.PostVerify)
	//router.POST("/v1/exports", handler.PostExport)
	//router.Run(":8080")

	// Stop Workers
	for _, w := range workers {
		w.Stop()
	}

	return nil

}
func (s *service) RecordEvent(ctx context.Context, tenantID uuid.UUID,
	actor, resource, metadata json.RawMessage, action string) (string, string, error) {

	event := audit.NewEvent(
		tenantID,
		actor,
		action,
		resource,
		metadata,
	)
	s.queue <- event

	return event.ID.String(), event.OccurredAt.Format(time.RFC3339), nil
}
