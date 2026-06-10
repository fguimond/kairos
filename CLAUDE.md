# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Git

Never push directly to the `main` branch. Always create a feature branch and open a pull request.

## Commands

```bash
# Start dependencies only (for local development)
docker compose up -d postgres rabbitmq

# Run the service
go run ./cmd/kairos run

# Build binary
go build -o kairos ./cmd/kairos

# Print version (use -ldflags "-X github.com/francisguimond/kairos/internal/version.Version=<tag>" to inject a release version)
go run ./cmd/kairos version

# Run all unit tests
go test ./...

# Run integration tests (requires Postgres via docker compose up -d postgres)
go test -tags=integration ./internal/store/...

# Run a single test by name
go test ./internal/store/... -run TestFunctionName
go test -tags=integration ./internal/store/... -run TestFunctionName

# Run linter
golangci-lint run ./...
```

## Architecture

Kairos is a scheduler micro-service: HTTP clients define schedules, a background engine fires them by publishing JSON messages to RabbitMQ.

**Request path (CRUD):**
`api.Server` (stdlib `net/http` + method-pattern routing) → `store.ScheduleStore` (pgx/v5 pool) → Postgres

**Trigger path (background):**
`engine.Engine` ticks on `POLL_INTERVAL_MS`, calls `store.DueSchedules` with `FOR UPDATE SKIP LOCKED` (safe for multi-replica deploys), then calls `queue.AMQPPublisher.Publish`. On success, `store.AdvanceNextRun` increments `execution_count` and recalculates `next_run_at` (cron via `robfig/cron`, or `+interval_sec`). Schedules are marked `completed` when they are one-shot (no cron, no interval) or when `execution_count` reaches `max_executions`.

**Schema migration** is embedded in `store.Migrate` as a `CREATE TABLE IF NOT EXISTS` string — no migration framework, run on every startup.

**CLI layout (cobra + viper):**
`cmd/kairos/main.go` only calls `cli.Execute()`. The command tree lives in `internal/cli` (`root.go`, `run.go`, `version.go`); `help` and `completion` come from cobra defaults. Config precedence is **flag > env (`KAIROS_*`) > YAML (`./kairos.yaml`, `$HOME/.kairos.yaml`, or `--config <path>`) > built-in default**, materialised by `config.FromViper`. Flag names are kebab-case (`--listen-addr`) with single-letter shorthands derived from the first letter of the long form (`-l`, `-d`, `-a`, `-q`, `-p`, `-k`).

**Key design notes:**
- `engine.Publisher` is an interface — swap in a mock for tests without touching `queue`.
- The `txWrapper` in `engine/engine.go` is a stub; the `FOR UPDATE SKIP LOCKED` lock is acquired at the SQL level per query, not wrapped in an explicit Go transaction. If you add real transaction support, wire a `pgxpool.Tx` through `DueSchedules` and `AdvanceNextRun`.
- Delete is a soft-delete (`status = 'deleted'`); `List` excludes deleted rows but `Get` does not filter by status.
- `payload` is stored as Postgres `JSONB` and passed through as `json.RawMessage` — never re-encoded.
- Logging uses `log/slog` (stdlib); the `api` package wraps every request through `loggingMiddleware` and `recoveryMiddleware`.
