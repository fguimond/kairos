package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is a thin alias so callers don't import pgxpool directly.
type DB = pgxpool.Pool

// dbtx is the minimal query interface satisfied by both *pgxpool.Pool and pgx.Tx,
// allowing store methods to run against either a pool connection or a transaction.
type dbtx interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

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
    timezone        TEXT        NOT NULL DEFAULT 'UTC',
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
type ScheduleStore struct{ pool *pgxpool.Pool }

func NewScheduleStore(db *DB) *ScheduleStore { return &ScheduleStore{pool: db} }

// conn returns the active transaction from ctx if one was started via BeginTx,
// otherwise falls back to the pool (auto-commit).
func (s *ScheduleStore) conn(ctx context.Context) dbtx {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return s.pool
}

// BeginTx starts a database transaction and returns a derived context that
// carries it. All store methods called with txCtx will run inside the same
// transaction. The caller must call commit or rollback exactly once; calling
// rollback after a successful commit is safe and returns nil.
//
// If ctx already carries a transaction (i.e. the caller is nested inside an
// outer BeginTx), BeginTx joins it: commit and rollback are no-ops and the
// outer transaction owns the boundary.
func (s *ScheduleStore) BeginTx(ctx context.Context) (txCtx context.Context, commit func() error, rollback func() error, err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		noop := func() error { return nil }
		return ctx, noop, noop, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		noop := func() error { return nil }
		return ctx, noop, noop, fmt.Errorf("begin tx: %w", err)
	}
	txCtx = context.WithValue(ctx, txKey{}, tx)
	commit = func() error {
		if err := tx.Commit(txCtx); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		return nil
	}
	rollback = func() error {
		err := tx.Rollback(txCtx)
		if errors.Is(err, pgx.ErrTxClosed) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("rollback: %w", err)
		}
		return nil
	}
	return txCtx, commit, rollback, nil
}

func (s *ScheduleStore) Insert(ctx context.Context, sch *Schedule) error {
	_, err := s.conn(ctx).Exec(ctx,
		`INSERT INTO schedules (id,name,description,cron_expr,interval_sec,timezone,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,0,$10,$11,$12)`,
		sch.ID, sch.Name, sch.Description, sch.CronExpr, sch.IntervalSec, sch.Timezone,
		sch.Payload, sch.Status, sch.MaxExecutions, sch.NextRunAt, sch.CreatedAt, sch.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert schedule: %w", err)
	}
	return nil
}

func (s *ScheduleStore) Get(ctx context.Context, id string) (*Schedule, error) {
	row := s.conn(ctx).QueryRow(ctx,
		`SELECT id,name,description,cron_expr,interval_sec,timezone,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at
         FROM schedules WHERE id=$1`, id)
	return scanSchedule(row)
}

func (s *ScheduleStore) List(ctx context.Context, f ListFilter) ([]*Schedule, error) {
	var (
		clauses []string
		args    []any
		n       = 1
	)

	if len(f.Statuses) == 0 {
		clauses = append(clauses, "status != 'deleted'")
	} else {
		clauses = append(clauses, fmt.Sprintf("status = ANY($%d)", n))
		args = append(args, f.Statuses)
		n++
	}

	if f.Name != nil {
		if strings.Contains(*f.Name, "*") {
			clauses = append(clauses, fmt.Sprintf("name ILIKE $%d", n))
			args = append(args, strings.ReplaceAll(*f.Name, "*", "%"))
		} else {
			clauses = append(clauses, fmt.Sprintf("name = $%d", n))
			args = append(args, *f.Name)
		}
		n++
	}

	if f.MaxExecutions != nil {
		clauses = append(clauses, fmt.Sprintf("max_executions = $%d", n))
		args = append(args, *f.MaxExecutions)
		n++
	}

	for _, c := range f.ExecutionCount {
		clauses = append(clauses, fmt.Sprintf("execution_count %s $%d", c.Op, n))
		args = append(args, c.Value)
		n++
	}

	for _, c := range f.CreatedAt {
		clauses = append(clauses, fmt.Sprintf("created_at %s $%d", c.Op, n))
		args = append(args, c.Value)
		n++
	}

	for _, c := range f.UpdatedAt {
		clauses = append(clauses, fmt.Sprintf("updated_at %s $%d", c.Op, n))
		args = append(args, c.Value)
		n++
	}

	for _, c := range f.NextRunAt {
		clauses = append(clauses, fmt.Sprintf("next_run_at %s $%d", c.Op, n))
		args = append(args, c.Value)
		n++
	}

	q := `SELECT id,name,description,cron_expr,interval_sec,timezone,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at` +
		` FROM schedules WHERE ` + strings.Join(clauses, " AND ") + ` ORDER BY next_run_at ASC`

	rows, err := s.conn(ctx).Query(ctx, q, args...)
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
	_, err := s.conn(ctx).Exec(ctx,
		`UPDATE schedules SET name=$2,description=$3,cron_expr=$4,interval_sec=$5,timezone=$6,
         payload=$7,status=$8,max_executions=$9,updated_at=$10 WHERE id=$1`,
		sch.ID, sch.Name, sch.Description, sch.CronExpr, sch.IntervalSec, sch.Timezone,
		sch.Payload, sch.Status, sch.MaxExecutions, sch.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save schedule: %w", err)
	}
	return nil
}

func (s *ScheduleStore) Delete(ctx context.Context, id string) error {
	_, err := s.conn(ctx).Exec(ctx,
		`UPDATE schedules SET status='deleted', updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	return nil
}

// DueSchedules returns up to limit active schedules whose next_run_at is in the past,
// locking them with SKIP LOCKED so multiple engine instances don't double-fire.
// Must be called with a context carrying a transaction (via BeginTx) so the row
// locks are held until AdvanceNextRun commits.
func (s *ScheduleStore) DueSchedules(ctx context.Context, limit int) ([]*Schedule, error) {
	rows, err := s.conn(ctx).Query(ctx,
		`SELECT id,name,description,cron_expr,interval_sec,timezone,payload,status,max_executions,execution_count,next_run_at,created_at,updated_at
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
	_, err := s.conn(ctx).Exec(ctx,
		`UPDATE schedules SET status='completed', execution_count=$2, updated_at=NOW() WHERE id=$1`,
		id, count)
	if err != nil {
		return fmt.Errorf("complete schedule: %w", err)
	}
	return nil
}

func (s *ScheduleStore) SetNextRun(ctx context.Context, id string, next time.Time, count int64) error {
	_, err := s.conn(ctx).Exec(ctx,
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
		&sch.CronExpr, &sch.IntervalSec, &sch.Timezone, &sch.Payload,
		&sch.Status, &sch.MaxExecutions, &sch.ExecutionCount,
		&sch.NextRunAt, &sch.CreatedAt, &sch.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("schedule not found")
	}
	return &sch, err
}
