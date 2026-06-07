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

// Condition represents a comparison against a numeric or timestamp column.
type Condition struct {
	Op    string // one of: "=", "<", "<=", ">", ">="
	Value any    // int64 for numeric fields; time.Time for timestamp fields
}

// ListFilter specifies optional constraints for the List query.
type ListFilter struct {
	Name           *string
	Statuses       []Status
	MaxExecutions  *int64
	ExecutionCount []Condition
	CreatedAt      []Condition
	UpdatedAt      []Condition
	NextRunAt      []Condition
}

// Schedule represents a single recurring or one-shot job definition.
type Schedule struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	CronExpr       string          `json:"cron_expr,omitempty"`
	IntervalSec    *int64          `json:"interval_sec,omitempty"`
	Timezone       string          `json:"timezone,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	Status         Status          `json:"status"`
	MaxExecutions  *int64          `json:"max_executions,omitempty"`
	ExecutionCount int64           `json:"execution_count"`
	NextRunAt      time.Time       `json:"next_run_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// CreateScheduleInput is the request body for POST /schedules.
type CreateScheduleInput struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	CronExpr      string          `json:"cron_expr"`
	IntervalSec   *int64          `json:"interval_sec"`
	Timezone      string          `json:"timezone"`
	Payload       json.RawMessage `json:"payload"`
	MaxExecutions *int64          `json:"max_executions"`
}

// UpdateScheduleInput is the request body for PATCH /schedules/{id}.
type UpdateScheduleInput struct {
	Name          *string          `json:"name"`
	Description   *string          `json:"description"`
	CronExpr      *string          `json:"cron_expr"`
	IntervalSec   *int64           `json:"interval_sec"`
	Timezone      *string          `json:"timezone"`
	Payload       *json.RawMessage `json:"payload"`
	Status        *Status          `json:"status"`
	MaxExecutions *int64           `json:"max_executions"`
}
