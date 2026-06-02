package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/yourorg/kairos/internal/store"
)

// Store is the data-access interface the service depends on.
type Store interface {
	Insert(ctx context.Context, sch *store.Schedule) error
	Get(ctx context.Context, id string) (*store.Schedule, error)
	List(ctx context.Context) ([]*store.Schedule, error)
	Save(ctx context.Context, sch *store.Schedule) error
	Delete(ctx context.Context, id string) error
	DueSchedules(ctx context.Context, limit int) ([]*store.Schedule, error)
	Complete(ctx context.Context, id string, count int64) error
	SetNextRun(ctx context.Context, id string, next time.Time, count int64) error
}

// ScheduleService sits between HTTP handlers / engine and the data store.
type ScheduleService struct {
	store Store
}

func New(s Store) *ScheduleService {
	return &ScheduleService{store: s}
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func loadLocation(timezone string) (*time.Location, error) {
	if timezone == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	return loc, nil
}

func nextRun(cronExpr string, intervalSec *int64, timezone string) (time.Time, error) {
	if cronExpr != "" {
		sched, err := cronParser.Parse(cronExpr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
		}
		loc, err := loadLocation(timezone)
		if err != nil {
			return time.Time{}, err
		}
		return sched.Next(time.Now().In(loc)), nil
	}
	if intervalSec != nil && *intervalSec > 0 {
		return time.Now().UTC().Add(time.Duration(*intervalSec) * time.Second), nil
	}
	return time.Time{}, fmt.Errorf("either cron_expr or interval_sec must be provided")
}

func (s *ScheduleService) Create(ctx context.Context, in store.CreateScheduleInput) (*store.Schedule, error) {
	if in.MaxExecutions != nil && *in.MaxExecutions <= 0 {
		return nil, fmt.Errorf("max_executions must be greater than 0")
	}
	nr, err := nextRun(in.CronExpr, in.IntervalSec, in.Timezone)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sch := &store.Schedule{
		ID:            uuid.NewString(),
		Name:          in.Name,
		Description:   in.Description,
		CronExpr:      in.CronExpr,
		IntervalSec:   in.IntervalSec,
		Timezone:      in.Timezone,
		Payload:       in.Payload,
		Status:        store.StatusActive,
		MaxExecutions: in.MaxExecutions,
		NextRunAt:     nr,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.Insert(ctx, sch); err != nil {
		return nil, err
	}
	return sch, nil
}

func (s *ScheduleService) Get(ctx context.Context, id string) (*store.Schedule, error) {
	return s.store.Get(ctx, id)
}

func (s *ScheduleService) List(ctx context.Context) ([]*store.Schedule, error) {
	return s.store.List(ctx)
}

func (s *ScheduleService) Update(ctx context.Context, id string, in store.UpdateScheduleInput) (*store.Schedule, error) {
	if in.MaxExecutions != nil && *in.MaxExecutions <= 0 {
		return nil, fmt.Errorf("max_executions must be greater than 0")
	}
	sch, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		sch.Name = *in.Name
	}
	if in.Description != nil {
		sch.Description = *in.Description
	}
	if in.CronExpr != nil {
		sch.CronExpr = *in.CronExpr
	}
	if in.IntervalSec != nil {
		sch.IntervalSec = in.IntervalSec
	}
	if in.Timezone != nil {
		if _, err := loadLocation(*in.Timezone); err != nil {
			return nil, err
		}
		sch.Timezone = *in.Timezone
	}
	if in.Payload != nil {
		sch.Payload = *in.Payload
	}
	if in.Status != nil {
		sch.Status = *in.Status
	}
	if in.MaxExecutions != nil {
		sch.MaxExecutions = in.MaxExecutions
	}
	sch.UpdatedAt = time.Now().UTC()
	if err := s.store.Save(ctx, sch); err != nil {
		return nil, err
	}
	return sch, nil
}

func (s *ScheduleService) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

func (s *ScheduleService) DueSchedules(ctx context.Context, limit int) ([]*store.Schedule, error) {
	return s.store.DueSchedules(ctx, limit)
}

func (s *ScheduleService) AdvanceNextRun(ctx context.Context, sch *store.Schedule) error {
	sch.ExecutionCount++
	limitReached := sch.MaxExecutions != nil && sch.ExecutionCount >= *sch.MaxExecutions
	oneShot := sch.CronExpr == "" && (sch.IntervalSec == nil || *sch.IntervalSec <= 0)

	if limitReached || oneShot {
		return s.store.Complete(ctx, sch.ID, sch.ExecutionCount)
	}

	var next time.Time
	if sch.CronExpr != "" {
		sched, err := cronParser.Parse(sch.CronExpr)
		if err != nil {
			return fmt.Errorf("parse cron: %w", err)
		}
		loc, err := loadLocation(sch.Timezone)
		if err != nil {
			return err
		}
		next = sched.Next(time.Now().In(loc))
	} else {
		next = time.Now().UTC().Add(time.Duration(*sch.IntervalSec) * time.Second)
	}

	return s.store.SetNextRun(ctx, sch.ID, next, sch.ExecutionCount)
}
