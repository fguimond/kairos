package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const schedulesPath = "/api/v1/schedules"

// Create defines a new schedule. Name is required, and exactly one of CronExpr
// or IntervalSec must be set.
func (c *Client) Create(ctx context.Context, in CreateScheduleInput) (*Schedule, error) {
	var out Schedule
	if err := c.do(ctx, http.MethodPost, schedulesPath, nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get fetches a single schedule by ID.
func (c *Client) Get(ctx context.Context, id string) (*Schedule, error) {
	var out Schedule
	if err := c.do(ctx, http.MethodGet, schedulesPath+"/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns schedules matching opts, with pagination metadata.
func (c *Client) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	var out ListResult
	if err := c.do(ctx, http.MethodGet, schedulesPath, opts.query(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update applies a partial update to a schedule. Only non-nil fields in in are
// changed.
func (c *Client) Update(ctx context.Context, id string, in UpdateScheduleInput) (*Schedule, error) {
	var out Schedule
	if err := c.do(ctx, http.MethodPatch, schedulesPath+"/"+url.PathEscape(id), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete soft-deletes a schedule.
func (c *Client) Delete(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, schedulesPath+"/"+url.PathEscape(id), nil, nil, nil)
}

// Pause stops a schedule from firing until it is resumed. It is a convenience
// wrapper over Update that sets the status to paused.
func (c *Client) Pause(ctx context.Context, id string) (*Schedule, error) {
	return c.Update(ctx, id, UpdateScheduleInput{Status: ptr(StatusPaused)})
}

// Resume reactivates a paused schedule; the server recalculates its next run
// time. It is a convenience wrapper over Update that sets the status to active.
func (c *Client) Resume(ctx context.Context, id string) (*Schedule, error) {
	return c.Update(ctx, id, UpdateScheduleInput{Status: ptr(StatusActive)})
}

// Healthz reports whether the server is reachable and healthy.
func (c *Client) Healthz(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/healthz", nil, nil, nil)
}

// query renders ListOptions as URL query parameters matching the server's
// filter parser.
func (o ListOptions) query() url.Values {
	q := url.Values{}

	if o.Name != nil {
		q.Set("name", *o.Name)
	}
	for _, s := range o.Statuses {
		q.Add("status", string(s))
	}
	if o.MaxExecutions != nil {
		q.Set("max_executions", strconv.FormatInt(*o.MaxExecutions, 10))
	}

	addConditions(q, "execution_count", o.ExecutionCount)
	addConditions(q, "created_at", o.CreatedAt)
	addConditions(q, "updated_at", o.UpdatedAt)
	addConditions(q, "next_run_at", o.NextRunAt)

	if len(o.Sort) > 0 {
		parts := make([]string, 0, len(o.Sort))
		for _, s := range o.Sort {
			parts = append(parts, s.Field+":"+string(s.Dir))
		}
		q.Set("sort", strings.Join(parts, ","))
	}

	if o.Page > 0 {
		q.Set("page", strconv.Itoa(o.Page))
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}

	return q
}

// addConditions appends "op:value" filter params for the given key. Time values
// are encoded as RFC3339; everything else uses its default string form.
func addConditions(q url.Values, key string, conds []Condition) {
	for _, cond := range conds {
		var val string
		if t, ok := cond.Value.(time.Time); ok {
			val = t.Format(time.RFC3339)
		} else {
			val = fmt.Sprintf("%v", cond.Value)
		}
		q.Add(key, string(cond.Op)+":"+val)
	}
}

func ptr[T any](v T) *T { return &v }
