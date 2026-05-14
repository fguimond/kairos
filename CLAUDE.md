# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Start dependencies only (for local development)
docker compose up -d postgres rabbitmq

# Run the service
go run ./cmd/kairos

# Build binary
go build -o kairos ./cmd/kairos

# Run all tests
go test ./...

# Run a single package's tests
go test ./internal/store/...

# Run a single test by name
go test ./internal/store/... -run TestFunctionName
```

No linter is configured yet. The standard `go vet ./...` applies.

## Architecture

Kairos is a scheduler micro-service: HTTP clients define schedules, a background engine fires them by publishing JSON messages to RabbitMQ.

**Request path (CRUD):**
`api.Server` (stdlib `net/http` + method-pattern routing) → `store.ScheduleStore` (pgx/v5 pool) → Postgres

**Trigger path (background):**
`engine.Engine` ticks on `POLL_INTERVAL_MS`, calls `store.DueSchedules` with `FOR UPDATE SKIP LOCKED` (safe for multi-replica deploys), then calls `queue.AMQPPublisher.Publish`. On success, `store.AdvanceNextRun` recalculates `next_run_at` (cron via `robfig/cron`, or `+interval_sec`). One-shot schedules (no cron, no interval) are marked `completed` after firing.

**Schema migration** is embedded in `store.Migrate` as a `CREATE TABLE IF NOT EXISTS` string — no migration framework, run on every startup.

**Key design notes:**
- `engine.Publisher` is an interface — swap in a mock for tests without touching `queue`.
- The `txWrapper` in `engine/engine.go` is a stub; the `FOR UPDATE SKIP LOCKED` lock is acquired at the SQL level per query, not wrapped in an explicit Go transaction. If you add real transaction support, wire a `pgxpool.Tx` through `DueSchedules` and `AdvanceNextRun`.
- Delete is a soft-delete (`status = 'deleted'`); `List` excludes deleted rows but `Get` does not filter by status.
- `payload` is stored as Postgres `JSONB` and passed through as `json.RawMessage` — never re-encoded.
- Logging uses `log/slog` (stdlib); the `api` package wraps every request through `loggingMiddleware` and `recoveryMiddleware`.
