# Kairos — Scheduler Micro-service

Kairos is a lightweight Go microservice that lets you **define schedules via a REST API** and **publishes a message to a queue** (RabbitMQ by default) every time a schedule fires.

## Architecture

```
API clients
    │  POST/PATCH/DELETE/GET /schedules
    ▼
REST API  (net/http)
    │  CRUD
    ▼
Schedule Store  (Postgres)
    │  SELECT … FOR UPDATE SKIP LOCKED
    ▼
Trigger Engine  (background goroutine)
    │  publish JSON message
    ▼
Message Queue  (RabbitMQ)
    │
    ├─► Consumer A
    ├─► Consumer B
    └─► Consumer C
```

## Quick start

```bash
docker compose up -d postgres rabbitmq
go run ./cmd/kairos
```

Or run everything in Docker:

```bash
docker compose up --build
```

## Testing

### Unit tests

No infrastructure required. Run the full suite with:

```bash
go test ./...
```

### Integration tests

Integration tests are gated behind the `integration` build tag and require live infrastructure. Start the dependencies first:

```bash
docker compose up -d postgres rabbitmq
```

| Package              | Dependency | Command                                              |
|----------------------|------------|------------------------------------------------------|
| `internal/store`     | Postgres   | `go test -tags=integration ./internal/store/...`    |
| `internal/queue`     | RabbitMQ   | `go test -tags=integration ./internal/queue/...`    |
| All integration      | Both       | `go test -tags=integration ./...`                   |

Tests skip gracefully when their dependency is not reachable, so running with `-tags=integration` is safe even if only one service is up.

To run a specific test by name:

```bash
go test -tags=integration ./internal/store/... -run TestInsert_RoundTripsAllFields
```

## Configuration

Every option can be set via a command-line flag or an environment variable. **Flags take priority over environment variables.**

| Flag                   | Environment variable      | Default                                                          | Description                                                    |
|------------------------|---------------------------|------------------------------------------------------------------|----------------------------------------------------------------|
| `--listen-addr`        | `KAIROS_LISTEN_ADDR`      | `:8080`                                                          | HTTP listen address                                            |
| `--database-url`       | `KAIROS_DATABASE_URL`     | `postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable` | Postgres DSN                                                   |
| `--amqp-url`           | `KAIROS_AMQP_URL`         | `amqp://guest:guest@localhost:5672/`                             | RabbitMQ connection string                                     |
| `--queue-name`         | `KAIROS_QUEUE_NAME`       | `kairos.triggers`                                                | Queue to publish trigger messages                              |
| `--poll-interval-ms`   | `KAIROS_POLL_INTERVAL_MS` | `5000`                                                           | How often the engine polls (ms)                                |
| `--api-key`            | `KAIROS_API_KEY`          | *(unset — auth disabled)*                                        | Bearer token required on all requests (except `GET /healthz`) |

Example using flags:

```bash
go run ./cmd/kairos \
  --listen-addr=:9090 \
  --database-url=postgres://user:pass@db:5432/kairos?sslmode=disable \
  --api-key=mysecretkey
```

## Authentication

When `KAIROS_API_KEY` is set, every request (except `GET /healthz`) must include the key as a Bearer token:

```
Authorization: Bearer <your-api-key>
```

Generate a key:

```bash
openssl rand -base64 32
```

Requests without the header, or with a wrong key, receive `401 Unauthorized`. When `KAIROS_API_KEY` is unset the service accepts all requests — useful during local development.

## REST API

### Create a schedule

```http
POST /api/v1/schedules
Content-Type: application/json
Authorization: Bearer <your-api-key>

{
  "name": "daily-report",
  "description": "Triggers the daily report job",
  "cron_expr": "0 8 * * *",
  "payload": { "report_type": "daily", "recipients": ["team@example.com"] }
}
```

Cron schedules fire in UTC by default. Pass an optional `timezone` field (IANA name) to evaluate the expression in a different timezone:

```http
POST /api/v1/schedules
Content-Type: application/json

{
  "name": "daily-report-nyc",
  "cron_expr": "0 8 * * *",
  "timezone": "America/New_York",
  "payload": { "report_type": "daily" }
}
```

The schedule above fires at 08:00 New York time (observing EST/EDT automatically). `timezone` is ignored for `interval_sec` schedules — those are always relative durations. An invalid IANA timezone name returns `422 Unprocessable Entity`.

Or with a fixed interval (every 30 seconds):

```http
POST /api/v1/schedules
Content-Type: application/json

{
  "name": "heartbeat",
  "interval_sec": 30,
  "payload": { "type": "ping" }
}
```

Use `max_executions` to cap how many times a schedule fires. Once the limit is reached the schedule transitions to `completed` automatically:

```http
POST /api/v1/schedules
Content-Type: application/json

{
  "name": "onboarding-email",
  "interval_sec": 86400,
  "max_executions": 3,
  "payload": { "template": "onboarding" }
}
```

Response `201 Created`:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "daily-report",
  "cron_expr": "0 8 * * *",
  "status": "active",
  "max_executions": 3,
  "execution_count": 0,
  "next_run_at": "2024-05-02T08:00:00Z",
  "...": "..."
}
```

### List schedules

```http
GET /api/v1/schedules
```

### Get a schedule

```http
GET /api/v1/schedules/{id}
```

### Update a schedule

```http
PATCH /api/v1/schedules/{id}
Content-Type: application/json

{ "status": "paused" }
```

### Delete a schedule

```http
DELETE /api/v1/schedules/{id}
```

Returns `204 No Content`. Soft-deletes (sets `status = deleted`).

## Sample consumer

`cmd/consumer` is a standalone program that connects to the same queue and prints each trigger message to stdout. It's useful for verifying that schedules are firing without writing any consumer code yourself.

```bash
# In one terminal — start the consumer
go run ./cmd/consumer

# In another — start the service and create a schedule
go run ./cmd/kairos
```

Output per fired job:
```
--- trigger fired ---
  schedule_id:   550e8400-e29b-41d4-a716-446655440000
  schedule_name: daily-report
  fired_at:      2024-05-02T08:00:00Z
  payload:       {"report_type":"daily"}
```

The consumer reads `--amqp-url` / `KAIROS_AMQP_URL` and `--queue-name` / `KAIROS_QUEUE_NAME`, using the same defaults as the main service.

## Queue message format

When a schedule fires, the following JSON is published to the queue:

```json
{
  "schedule_id":   "550e8400-e29b-41d4-a716-446655440000",
  "schedule_name": "daily-report",
  "fired_at":      "2024-05-02T08:00:00.123456Z",
  "payload":       { "report_type": "daily", "recipients": ["team@example.com"] }
}
```

Consumers should be idempotent — use `schedule_id` + `fired_at` as a deduplication key.

## Scaling

Multiple Kairos instances can run concurrently. The engine uses `SELECT … FOR UPDATE SKIP LOCKED` so each schedule fires exactly once even with N replicas.

## Releases

Releases are automated via [release-please](https://github.com/googleapis/release-please). On every push to `main`, the GitHub Actions workflow (`.github/workflows/release-please.yml`) inspects commit messages and, when there is releasable content, opens or updates a release PR that:

- bumps the version in `.release-please-manifest.json`
- prepends an entry to `CHANGELOG.md`

Merging that PR creates a GitHub Release and tags the commit.

**Commit message conventions** (follows [Conventional Commits](https://www.conventionalcommits.org/)):

| Prefix | Effect |
|---|---|
| `feat:` | minor version bump |
| `fix:` | patch version bump |
| `feat!:` / `fix!:` / `BREAKING CHANGE:` | major version bump |
| `chore:`, `docs:`, `ci:`, `refactor:`, `test:` | no version bump (not included in changelog) |

Commits that do not follow this format are ignored by release-please.

## Project layout

```
kairos/
├── cmd/
│   ├── kairos/         # main entrypoint
│   └── consumer/       # sample RabbitMQ consumer
├── internal/
│   ├── api/            # HTTP handlers (chi)
│   ├── config/         # env-based config
│   ├── engine/         # trigger polling loop
│   ├── queue/          # AMQP publisher
│   └── store/          # Postgres data layer
├── docker-compose.yml
└── Dockerfile
```
