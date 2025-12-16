package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurisu1024/ledgerly/config"
	"github.com/kurisu1024/ledgerly/db"
	"github.com/kurisu1024/ledgerly/internal/handlers"
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

// TODO: define service data structure and interface.
type Service interface {
	Run(ctx context.Context) error
}

func NewService() Service {
	return &service{}
}

type service struct {
	db    *pgxpool.Pool
	repo  *Repository
	queue chan *AuditEvent
}

// TODO: Implement
func (s *service) Run(ctx context.Context) error {
	cfg := config.Default()
	pool, err := db.NewPool(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	s.db = pool

	queue := make(chan *AuditEvent, cfg.QueueSize)
	defer close(queue)
	workers := make([]Worker, cfg.WorkerCount)
	for i := 0; i < cfg.WorkerCount; i++ {
		w := NewBatchInsertWorker(cfg.BatchSize, time.Second, s.RecordEvents, queue).Start(ctx)
		workers[i] = w
	}

	handler := handlers.NewHandler(s.RecordEvent)

	router := gin.Default()
	router.POST("/v1/events", handler.PostEvent)
	router.GET("/v1/events", handler.GetEvents)
	router.POST("/v1/verify", handler.PostVerify)
	router.POST("/v1/exports", handler.PostExport)
	router.Run(":8080")

	// Stop Workers
	for _, w := range workers {
		w.Stop()
	}

	return nil

}
func (s *service) RecordEvent(ctx context.Context, tenantID uuid.UUID, e handlers.CreateEventRequest) (string, string, error) {
	var actor, resource, metadata json.RawMessage
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	if err := encoder.Encode(e.Actor); err != nil {
		return "", "", err
	}
	actor = buffer.Bytes()
	buffer.Reset()

	if err := encoder.Encode(e.Resource); err != nil {
		return "", "", err
	}
	resource = buffer.Bytes()
	buffer.Reset()

	if err := encoder.Encode(e.Metadata); err != nil {
		return "", "", err
	}
	metadata = buffer.Bytes()

	event := newAuditEvent(
		tenantID,
		actor,
		e.Action,
		resource,
		metadata,
	)
	s.queue <- event

	return event.ID.String(), event.OccurredAt.Format(time.RFC3339), nil
}

func (s *service) RecordEvents(ctx context.Context, e []*AuditEvent) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	err = s.repo.InsertEvents(ctx, tx, e)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *service) VerifyTenant(ctx context.Context, tenantID uuid.UUID) error {
	rows, err := s.db.Query(ctx, `
		SELECT occurred_at, actor, action, resource, metadata, prev_hash, event_hash
		FROM audit_events
		WHERE tenant_id = $1
		ORDER BY id
	`, tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()

	prev := GenesisHash[:]

	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(
			&e.OccurredAt,
			&e.Actor,
			&e.Action,
			&e.Resource,
			&e.Metadata,
			&e.PrevHash,
			&e.EventHash,
		); err != nil {
			return err
		}

		if !bytes.Equal(e.PrevHash, prev) {
			return errors.New("chain broken")
		}

		computed := ComputeHash(
			tenantID,
			e.OccurredAt,
			e.Actor,
			e.Resource,
			e.Metadata,
			e.Action,
			prev,
		)

		if !bytes.Equal(computed, e.EventHash) {
			return errors.New("hash mismatch")
		}

		prev = e.EventHash
	}

	return nil
}
