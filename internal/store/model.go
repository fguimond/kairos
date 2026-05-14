package store

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusDeleted   Status = "deleted"
)

// Schedule represents a single recurring or one-shot job definition.
type Schedule struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	CronExpr    string          `json:"cron_expr,omitempty"`   // e.g. "*/5 * * * *"
	IntervalSec *int64          `json:"interval_sec,omitempty"` // fixed interval (seconds), alternative to cron
	Payload     json.RawMessage `json:"payload"`               // forwarded verbatim to the queue
	Status      Status          `json:"status"`
	NextRunAt   time.Time       `json:"next_run_at"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CreateScheduleInput is the request body for POST /schedules.
type CreateScheduleInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	CronExpr    string          `json:"cron_expr"`
	IntervalSec *int64          `json:"interval_sec"`
	Payload     json.RawMessage `json:"payload"`
}

// UpdateScheduleInput is the request body for PATCH /schedules/{id}.
type UpdateScheduleInput struct {
	Name        *string          `json:"name"`
	Description *string          `json:"description"`
	CronExpr    *string          `json:"cron_expr"`
	IntervalSec *int64           `json:"interval_sec"`
	Payload     *json.RawMessage `json:"payload"`
	Status      *Status          `json:"status"`
}
