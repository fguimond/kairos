# Kairos — Scheduler Micro-service

Kairos is a lightweight Go microservice that lets you **define schedules via a REST API** and **publishes a message to a queue** (RabbitMQ by default) every time a schedule fires.

## Architecture

```
API clients
    │  POST/PATCH/DELETE/GET /schedules
    ▼
REST API  (chi)
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

## Environment variables

| Variable           | Default                                              | Description                        |
|--------------------|------------------------------------------------------|------------------------------------|
| `LISTEN_ADDR`      | `:8080`                                              | HTTP listen address                |
| `DATABASE_URL`     | `postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable` | Postgres DSN      |
| `AMQP_URL`         | `amqp://guest:guest@localhost:5672/`                 | RabbitMQ connection string         |
| `QUEUE_NAME`       | `kairos.triggers`                                    | Queue to publish trigger messages  |
| `POLL_INTERVAL_MS` | `5000`                                               | How often the engine polls (ms)    |

## REST API

### Create a schedule

```http
POST /schedules
Content-Type: application/json

{
  "name": "daily-report",
  "description": "Triggers the daily report job",
  "cron_expr": "0 8 * * *",
  "payload": { "report_type": "daily", "recipients": ["team@example.com"] }
}
```

Or with a fixed interval (every 30 seconds):

```http
POST /schedules
Content-Type: application/json

{
  "name": "heartbeat",
  "interval_sec": 30,
  "payload": { "type": "ping" }
}
```

Use `max_executions` to cap how many times a schedule fires. Once the limit is reached the schedule transitions to `completed` automatically:

```http
POST /schedules
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
  ...
}
```

### List schedules

```http
GET /schedules
```

### Get a schedule

```http
GET /schedules/{id}
```

### Update a schedule

```http
PATCH /schedules/{id}
Content-Type: application/json

{ "status": "paused" }
```

### Delete a schedule

```http
DELETE /schedules/{id}
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

The consumer reads `AMQP_URL` and `QUEUE_NAME` from the environment, using the same defaults as the main service.

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
