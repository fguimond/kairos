package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	if _, err := db.Exec(context.Background(), schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// ScheduleStore is the data-access layer for schedules.
type ScheduleStore struct{ db *DB }

func NewScheduleStore(db *DB) *ScheduleStore { return &ScheduleStore{db: db} }

func (s *ScheduleStore) Insert(ctx context.Context, sch *Schedule) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO schedules (id,name,description,cron_expr,interval_sec,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,$9,$10,$11)`,
		sch.ID, sch.Name, sch.Description, sch.CronExpr, sch.IntervalSec,
		sch.Payload, sch.Status, sch.MaxExecutions, sch.NextRunAt, sch.CreatedAt, sch.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert schedule: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("list schedules: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	return out, nil
}

func (s *ScheduleStore) Save(ctx context.Context, sch *Schedule) error {
	_, err := s.db.Exec(ctx,
		`UPDATE schedules SET name=$2,description=$3,cron_expr=$4,interval_sec=$5,payload=$6,
         status=$7,max_executions=$8,updated_at=$9 WHERE id=$1`,
		sch.ID, sch.Name, sch.Description, sch.CronExpr, sch.IntervalSec,
		sch.Payload, sch.Status, sch.MaxExecutions, sch.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save schedule: %w", err)
	}
	return nil
}

func (s *ScheduleStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE schedules SET status='deleted', updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("due schedules: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("due schedules: %w", err)
	}
	return out, nil
}

func (s *ScheduleStore) Complete(ctx context.Context, id string, count int64) error {
	_, err := s.db.Exec(ctx,
		`UPDATE schedules SET status='completed', execution_count=$2, updated_at=NOW() WHERE id=$1`,
		id, count)
	if err != nil {
		return fmt.Errorf("complete schedule: %w", err)
	}
	return nil
}

func (s *ScheduleStore) SetNextRun(ctx context.Context, id string, next time.Time, count int64) error {
	_, err := s.db.Exec(ctx,
		`UPDATE schedules SET next_run_at=$2, execution_count=$3, updated_at=NOW() WHERE id=$1`,
		id, next, count)
	if err != nil {
		return fmt.Errorf("set next run: %w", err)
	}
	return nil
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
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("schedule not found")
	}
	return &sch, err
}
