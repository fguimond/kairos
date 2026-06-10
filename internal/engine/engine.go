package engine

import (
	"context"
	"encoding/json"
	"time"

	"log/slog"

	"github.com/francisguimond/kairos/internal/store"
)

// Publisher is the interface the engine uses to fire a trigger event.
type Publisher interface {
	Publish(ctx context.Context, msg []byte) error
}

// Service is the interface the engine uses to query and advance schedules.
type Service interface {
	BeginTx(ctx context.Context) (context.Context, func() error, func() error, error)
	DueSchedules(ctx context.Context, limit int) ([]*store.Schedule, error)
	AdvanceNextRun(ctx context.Context, sch *store.Schedule) error
}

// TriggerMessage is what gets sent to the queue when a schedule fires.
type TriggerMessage struct {
	ScheduleID   string          `json:"schedule_id"`
	ScheduleName string          `json:"schedule_name"`
	FiredAt      time.Time       `json:"fired_at"`
	Payload      json.RawMessage `json:"payload"`
}

// Engine polls the schedule store and fires due schedules.
type Engine struct {
	svc          Service
	publisher    Publisher
	pollInterval time.Duration
	batchSize    int
}

func New(s Service, p Publisher, pollInterval time.Duration) *Engine {
	return &Engine{
		svc:          s,
		publisher:    p,
		pollInterval: pollInterval,
		batchSize:    100,
	}
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	txCtx, commit, rollback, err := e.svc.BeginTx(ctx)
	if err != nil {
		slog.Error("engine: begin tx", "error", err)
		return
	}
	defer rollback() //nolint:errcheck

	schedules, err := e.svc.DueSchedules(txCtx, e.batchSize)
	if err != nil {
		slog.Error("engine: query due schedules", "error", err)
		return
	}

	for _, sch := range schedules {
		msg := TriggerMessage{
			ScheduleID:   sch.ID,
			ScheduleName: sch.Name,
			FiredAt:      time.Now().UTC(),
			Payload:      sch.Payload,
		}
		body, _ := json.Marshal(msg)

		if err := e.publisher.Publish(ctx, body); err != nil {
			slog.Error("engine: publish failed", "error", err, "schedule_id", sch.ID)
			continue // leave next_run_at unchanged; will retry next tick
		}

		if err := e.svc.AdvanceNextRun(txCtx, sch); err != nil {
			slog.Error("engine: advance next run", "error", err, "schedule_id", sch.ID)
		} else {
			slog.Info("trigger fired", "schedule_id", sch.ID, "name", sch.Name)
		}
	}

	if err := commit(); err != nil {
		slog.Error("engine: commit tx", "error", err)
	}
}
