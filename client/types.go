package client

import (
	"encoding/json"
	"time"
)

// Status is the lifecycle state of a schedule.
type Status string

const (
	StatusActive    Status = "active"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusDeleted   Status = "deleted"
)

// Schedule represents a single recurring or one-shot job definition. Its JSON
// tags mirror the Kairos API response exactly.
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
	NextRunAt      *time.Time      `json:"next_run_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// CreateScheduleInput is the request body for Create. Either CronExpr or
// IntervalSec must be set, and Name is required.
type CreateScheduleInput struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	CronExpr      string          `json:"cron_expr"`
	IntervalSec   *int64          `json:"interval_sec"`
	Timezone      string          `json:"timezone"`
	Payload       json.RawMessage `json:"payload"`
	MaxExecutions *int64          `json:"max_executions"`
}

// UpdateScheduleInput is the request body for Update. All fields are optional;
// a nil field is omitted (serialized as null) and left unchanged by the server.
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

// Operator is a comparison operator for conditional list filters.
type Operator string

const (
	OpEq  Operator = "eq"
	OpLt  Operator = "lt"
	OpLte Operator = "lte"
	OpGt  Operator = "gt"
	OpGte Operator = "gte"
)

// Condition is a single comparison against a numeric or timestamp field. For
// numeric fields (ExecutionCount, MaxExecutions) Value should be an integer;
// for timestamp fields (CreatedAt, UpdatedAt, NextRunAt) Value should be a
// time.Time.
type Condition struct {
	Op    Operator
	Value any
}

// SortDir is a sort direction.
type SortDir string

const (
	SortAsc  SortDir = "asc"
	SortDesc SortDir = "desc"
)

// SortOrder specifies a field and direction for ordering list results.
// Field must be one of the server's sortable columns, e.g. "name",
// "updated_at", "next_run_at".
type SortOrder struct {
	Field string
	Dir   SortDir
}

// ListOptions specifies optional filters, sorting, and pagination for List.
// The zero value lists the first page with the server's default limit.
type ListOptions struct {
	Name           *string
	Statuses       []Status
	MaxExecutions  *int64
	ExecutionCount []Condition
	CreatedAt      []Condition
	UpdatedAt      []Condition
	NextRunAt      []Condition
	Sort           []SortOrder
	Page           int
	Limit          int
}

// ListResult is the response from List.
type ListResult struct {
	Data  []*Schedule `json:"data"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
	Total int64       `json:"total"`
}
