package audit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurisu1024/ledgerly/config"
	"github.com/kurisu1024/ledgerly/db"
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
	db   *pgxpool.Pool
	repo *Repository
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
	workers := make([]Worker, cfg.WorkerCount)
	for i := 0; i < cfg.WorkerCount; i++ {
		w := NewBatchInsertWorker(cfg.BatchSize, time.Second, s.RecordEvents, queue).Start(ctx)
		workers[i] = w
	}

	// Stop Workers
	for _, w := range workers {
		w.Stop()
	}

	return nil

}
func (s *service) RecordEvent(ctx context.Context, e *AuditEvent) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	err = s.repo.InsertEvent(ctx, tx, e)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
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
