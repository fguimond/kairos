package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

// DB is a thin alias so callers don't import pgxpool directly.
type DB = pgxpool.Pool

func Connect(dsn string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS schedules (
    id              TEXT        PRIMARY KEY,
    name            TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    cron_expr       TEXT        NOT NULL DEFAULT '',
    interval_sec    BIGINT,
    payload         JSONB       NOT NULL DEFAULT '{}',
    status          TEXT        NOT NULL DEFAULT 'active',
    max_executions  BIGINT,
    execution_count BIGINT      NOT NULL DEFAULT 0,
    next_run_at     TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_schedules_next_run ON schedules (status, next_run_at);
`

func Migrate(db *DB) error {
	_, err := db.Exec(context.Background(), schema)
	return err
}

// ScheduleStore is the data-access layer for schedules.
type ScheduleStore struct{ db *DB }

func NewScheduleStore(db *DB) *ScheduleStore { return &ScheduleStore{db: db} }

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// nextRun computes the first trigger time for a new schedule.
func nextRun(in CreateScheduleInput) (time.Time, error) {
	if in.CronExpr != "" {
		sched, err := cronParser.Parse(in.CronExpr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
		}
		return sched.Next(time.Now().UTC()), nil
	}
	if in.IntervalSec != nil && *in.IntervalSec > 0 {
		return time.Now().UTC().Add(time.Duration(*in.IntervalSec) * time.Second), nil
	}
	return time.Time{}, fmt.Errorf("either cron_expr or interval_sec must be provided")
}

func (s *ScheduleStore) Create(ctx context.Context, in CreateScheduleInput) (*Schedule, error) {
	if in.MaxExecutions != nil && *in.MaxExecutions <= 0 {
		return nil, fmt.Errorf("max_executions must be greater than 0")
	}
	nr, err := nextRun(in)
	if err != nil {
		return nil, err
	}

	sch := &Schedule{
		ID:            uuid.NewString(),
		Name:          in.Name,
		Description:   in.Description,
		CronExpr:      in.CronExpr,
		IntervalSec:   in.IntervalSec,
		Payload:       in.Payload,
		Status:        StatusActive,
		MaxExecutions: in.MaxExecutions,
		NextRunAt:     nr,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO schedules (id,name,description,cron_expr,interval_sec,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,$9,$10,$11)`,
		sch.ID, sch.Name, sch.Description, sch.CronExpr, sch.IntervalSec,
		sch.Payload, sch.Status, sch.MaxExecutions, sch.NextRunAt, sch.CreatedAt, sch.UpdatedAt,
	)
	return sch, err
}

func (s *ScheduleStore) Get(ctx context.Context, id string) (*Schedule, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id,name,description,cron_expr,interval_sec,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at
         FROM schedules WHERE id=$1`, id)
	return scanSchedule(row)
}

func (s *ScheduleStore) List(ctx context.Context) ([]*Schedule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id,name,description,cron_expr,interval_sec,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at
         FROM schedules WHERE status != 'deleted' ORDER BY next_run_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Schedule
	for rows.Next() {
		sch, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

func (s *ScheduleStore) Update(ctx context.Context, id string, in UpdateScheduleInput) (*Schedule, error) {
	if in.MaxExecutions != nil && *in.MaxExecutions <= 0 {
		return nil, fmt.Errorf("max_executions must be greater than 0")
	}
	sch, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		sch.Name = *in.Name
	}
	if in.Description != nil {
		sch.Description = *in.Description
	}
	if in.CronExpr != nil {
		sch.CronExpr = *in.CronExpr
	}
	if in.IntervalSec != nil {
		sch.IntervalSec = in.IntervalSec
	}
	if in.Payload != nil {
		sch.Payload = *in.Payload
	}
	if in.Status != nil {
		sch.Status = *in.Status
	}
	if in.MaxExecutions != nil {
		sch.MaxExecutions = in.MaxExecutions
	}
	sch.UpdatedAt = time.Now().UTC()

	_, err = s.db.Exec(ctx,
		`UPDATE schedules SET name=$2,description=$3,cron_expr=$4,interval_sec=$5,payload=$6,
         status=$7,max_executions=$8,updated_at=$9 WHERE id=$1`,
		sch.ID, sch.Name, sch.Description, sch.CronExpr, sch.IntervalSec,
		sch.Payload, sch.Status, sch.MaxExecutions, sch.UpdatedAt,
	)
	return sch, err
}

func (s *ScheduleStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE schedules SET status='deleted', updated_at=NOW() WHERE id=$1`, id)
	return err
}

// DueSchedules returns up to limit active schedules whose next_run_at is in the past,
// locking them with SKIP LOCKED so multiple engine instances don't double-fire.
func (s *ScheduleStore) DueSchedules(ctx context.Context, limit int) ([]*Schedule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id,name,description,cron_expr,interval_sec,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at
         FROM schedules
         WHERE status='active' AND next_run_at <= NOW()
         ORDER BY next_run_at ASC
         LIMIT $1
         FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Schedule
	for rows.Next() {
		sch, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

// AdvanceNextRun updates next_run_at after a successful trigger, or marks the
// schedule completed for one-shot jobs (no cron and no interval) or when
// max_executions has been reached.
func (s *ScheduleStore) AdvanceNextRun(ctx context.Context, sch *Schedule) error {
	sch.ExecutionCount++

	limitReached := sch.MaxExecutions != nil && sch.ExecutionCount >= *sch.MaxExecutions
	oneShot := sch.CronExpr == "" && (sch.IntervalSec == nil || *sch.IntervalSec <= 0)

	if limitReached || oneShot {
		_, err := s.db.Exec(ctx,
			`UPDATE schedules SET status='completed', execution_count=$2, updated_at=NOW() WHERE id=$1`,
			sch.ID, sch.ExecutionCount)
		return err
	}

	var next time.Time
	if sch.CronExpr != "" {
		sched, err := cronParser.Parse(sch.CronExpr)
		if err != nil {
			return err
		}
		next = sched.Next(time.Now().UTC())
	} else {
		next = time.Now().UTC().Add(time.Duration(*sch.IntervalSec) * time.Second)
	}

	_, err := s.db.Exec(ctx,
		`UPDATE schedules SET next_run_at=$2, execution_count=$3, updated_at=NOW() WHERE id=$1`,
		sch.ID, next, sch.ExecutionCount)
	return err
}

// scanSchedule works for both pgx.Row and pgx.Rows.
type scannable interface {
	Scan(dest ...any) error
}

func scanSchedule(row scannable) (*Schedule, error) {
	var sch Schedule
	err := row.Scan(
		&sch.ID, &sch.Name, &sch.Description,
		&sch.CronExpr, &sch.IntervalSec, &sch.Payload,
		&sch.Status, &sch.MaxExecutions, &sch.ExecutionCount,
		&sch.NextRunAt, &sch.CreatedAt, &sch.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("schedule not found")
	}
	return &sch, err
}
