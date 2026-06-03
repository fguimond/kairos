package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/yourorg/kairos/internal/store"
)

// --- mock store ---

type mockStore struct {
	beginTxFn      func(ctx context.Context) (context.Context, func() error, func() error, error)
	insertFn       func(ctx context.Context, sch *store.Schedule) error
	getFn          func(ctx context.Context, id string) (*store.Schedule, error)
	listFn         func(ctx context.Context) ([]*store.Schedule, error)
	saveFn         func(ctx context.Context, sch *store.Schedule) error
	deleteFn       func(ctx context.Context, id string) error
	dueSchedulesFn func(ctx context.Context, limit int) ([]*store.Schedule, error)
	completeFn     func(ctx context.Context, id string, count int64) error
	setNextRunFn   func(ctx context.Context, id string, next time.Time, count int64) error
}

func (m *mockStore) BeginTx(ctx context.Context) (context.Context, func() error, func() error, error) {
	if m.beginTxFn != nil {
		return m.beginTxFn(ctx)
	}
	noop := func() error { return nil }
	return ctx, noop, noop, nil
}

func (m *mockStore) Insert(ctx context.Context, sch *store.Schedule) error {
	return m.insertFn(ctx, sch)
}
func (m *mockStore) Get(ctx context.Context, id string) (*store.Schedule, error) {
	return m.getFn(ctx, id)
}
func (m *mockStore) List(ctx context.Context) ([]*store.Schedule, error) {
	return m.listFn(ctx)
}
func (m *mockStore) Save(ctx context.Context, sch *store.Schedule) error {
	return m.saveFn(ctx, sch)
}
func (m *mockStore) Delete(ctx context.Context, id string) error {
	return m.deleteFn(ctx, id)
}
func (m *mockStore) DueSchedules(ctx context.Context, limit int) ([]*store.Schedule, error) {
	return m.dueSchedulesFn(ctx, limit)
}
func (m *mockStore) Complete(ctx context.Context, id string, count int64) error {
	return m.completeFn(ctx, id, count)
}
func (m *mockStore) SetNextRun(ctx context.Context, id string, next time.Time, count int64) error {
	return m.setNextRunFn(ctx, id, next, count)
}

func newSvc(ms *mockStore) *ScheduleService { return New(ms) }

func ptr[T any](v T) *T { return &v }

func baseSchedule() *store.Schedule {
	return &store.Schedule{
		ID:          "existing-id",
		Name:        "original name",
		Description: "original desc",
		CronExpr:    "0 * * * *",
		Payload:     json.RawMessage(`{}`),
		Status:      store.StatusActive,
	}
}

// --- nextRun ---

func TestNextRun_ValidCron(t *testing.T) {
	got, err := nextRun("* * * * *", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.After(time.Now().UTC()) {
		t.Error("expected next run to be in the future")
	}
}

func TestNextRun_ValidInterval(t *testing.T) {
	sec := int64(60)
	before := time.Now().UTC()
	got, err := nextRun("", &sec, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lower := before.Add(59 * time.Second)
	upper := before.Add(61 * time.Second)
	if got.Before(lower) || got.After(upper) {
		t.Errorf("next run %v outside expected range [%v, %v]", got, lower, upper)
	}
}

func TestNextRun_NoCronNoInterval(t *testing.T) {
	_, err := nextRun("", nil, "")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestNextRun_InvalidCronExpr(t *testing.T) {
	_, err := nextRun("not-a-cron", nil, "")
	if err == nil {
		t.Error("expected error for invalid cron expression, got nil")
	}
}

func TestNextRun_ZeroInterval(t *testing.T) {
	sec := int64(0)
	_, err := nextRun("", &sec, "")
	if err == nil {
		t.Error("expected error for zero interval, got nil")
	}
}

func TestNextRun_NegativeInterval(t *testing.T) {
	sec := int64(-10)
	_, err := nextRun("", &sec, "")
	if err == nil {
		t.Error("expected error for negative interval, got nil")
	}
}

func TestNextRun_CronTakesPrecedenceOverInterval(t *testing.T) {
	sec := int64(60)
	got, err := nextRun("* * * * *", &sec, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.After(time.Now().UTC()) {
		t.Error("expected next run to be in the future")
	}
}

func TestNextRun_ValidTimezone(t *testing.T) {
	got, err := nextRun("* * * * *", nil, "America/New_York")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.After(time.Now().UTC()) {
		t.Error("expected next run to be in the future")
	}
}

func TestNextRun_InvalidTimezone(t *testing.T) {
	_, err := nextRun("* * * * *", nil, "Not/ATimezone")
	if err == nil {
		t.Error("expected error for invalid timezone, got nil")
	}
}

func TestNextRun_IntervalIgnoresTimezone(t *testing.T) {
	sec := int64(60)
	before := time.Now().UTC()
	got, err := nextRun("", &sec, "America/New_York")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lower := before.Add(59 * time.Second)
	upper := before.Add(61 * time.Second)
	if got.Before(lower) || got.After(upper) {
		t.Errorf("next run %v outside expected range for interval with timezone", got)
	}
}

// --- Create ---

func TestCreate_CronSetsFieldsCorrectly(t *testing.T) {
	var inserted *store.Schedule
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, sch *store.Schedule) error {
			inserted = sch
			return nil
		},
	})

	before := time.Now().UTC()
	in := store.CreateScheduleInput{
		Name:        "daily job",
		Description: "runs daily",
		CronExpr:    "0 0 * * *",
		Payload:     json.RawMessage(`{"k":"v"}`),
	}
	got, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != inserted {
		t.Error("returned schedule is not the inserted one")
	}
	if got.ID == "" {
		t.Error("expected non-empty ID")
	}
	if got.Name != "daily job" {
		t.Errorf("unexpected name: %q", got.Name)
	}
	if got.Description != "runs daily" {
		t.Errorf("unexpected description: %q", got.Description)
	}
	if got.CronExpr != "0 0 * * *" {
		t.Errorf("unexpected cron_expr: %q", got.CronExpr)
	}
	if string(got.Payload) != `{"k":"v"}` {
		t.Errorf("unexpected payload: %s", got.Payload)
	}
	if got.Status != store.StatusActive {
		t.Errorf("expected status active, got %q", got.Status)
	}
	if got.ExecutionCount != 0 {
		t.Errorf("expected execution_count 0, got %d", got.ExecutionCount)
	}
	if !got.NextRunAt.After(before) {
		t.Error("expected NextRunAt to be in the future")
	}
	if got.CreatedAt.Before(before) || got.UpdatedAt.Before(before) {
		t.Error("expected CreatedAt and UpdatedAt to be set to approximately now")
	}
}

func TestCreate_IntervalSetsNextRun(t *testing.T) {
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})

	before := time.Now().UTC()
	got, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:        "every 5m",
		IntervalSec: ptr(int64(300)),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lower := before.Add(299 * time.Second)
	upper := before.Add(301 * time.Second)
	if got.NextRunAt.Before(lower) || got.NextRunAt.After(upper) {
		t.Errorf("NextRunAt %v outside expected range for 300s interval", got.NextRunAt)
	}
}

func TestCreate_UniqueIDs(t *testing.T) {
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})
	in := store.CreateScheduleInput{Name: "x", CronExpr: "* * * * *"}

	a, _ := svc.Create(context.Background(), in)
	b, _ := svc.Create(context.Background(), in)
	if a.ID == b.ID {
		t.Error("expected distinct IDs across calls")
	}
}

func TestCreate_MaxExecutionsZero_RejectsBeforeInsert(t *testing.T) {
	insertCalled := false
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, _ *store.Schedule) error { insertCalled = true; return nil },
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:          "x",
		CronExpr:      "* * * * *",
		MaxExecutions: ptr(int64(0)),
	})
	if err == nil {
		t.Error("expected error for max_executions=0")
	}
	if insertCalled {
		t.Error("Insert must not be called when validation fails")
	}
}

func TestCreate_MaxExecutionsNegative(t *testing.T) {
	insertCalled := false
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, _ *store.Schedule) error { insertCalled = true; return nil },
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:          "x",
		CronExpr:      "* * * * *",
		MaxExecutions: ptr(int64(-5)),
	})
	if err == nil {
		t.Error("expected error for negative max_executions")
	}
	if insertCalled {
		t.Error("Insert must not be called when validation fails")
	}
}

func TestCreate_TimezonePropagated(t *testing.T) {
	var inserted *store.Schedule
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, sch *store.Schedule) error { inserted = sch; return nil },
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:     "tz job",
		CronExpr: "0 9 * * *",
		Timezone: "America/New_York",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inserted.Timezone != "America/New_York" {
		t.Errorf("expected timezone America/New_York, got %q", inserted.Timezone)
	}
}

func TestCreate_InvalidTimezone(t *testing.T) {
	insertCalled := false
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, _ *store.Schedule) error { insertCalled = true; return nil },
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:     "x",
		CronExpr: "* * * * *",
		Timezone: "Not/ATimezone",
	})
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
	if insertCalled {
		t.Error("Insert must not be called when timezone is invalid")
	}
}

func TestCreate_InvalidCron(t *testing.T) {
	insertCalled := false
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, _ *store.Schedule) error { insertCalled = true; return nil },
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:     "x",
		CronExpr: "bad-cron",
	})
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
	if insertCalled {
		t.Error("Insert must not be called when cron is invalid")
	}
}

func TestCreate_NoCronNoInterval(t *testing.T) {
	insertCalled := false
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, _ *store.Schedule) error { insertCalled = true; return nil },
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{Name: "x"})
	if err == nil {
		t.Error("expected error when neither cron nor interval is provided")
	}
	if insertCalled {
		t.Error("Insert must not be called when schedule trigger is missing")
	}
}

func TestCreate_BeginTxError(t *testing.T) {
	insertCalled := false
	svc := newSvc(&mockStore{
		beginTxFn: func(ctx context.Context) (context.Context, func() error, func() error, error) {
			noop := func() error { return nil }
			return ctx, noop, noop, errors.New("db unavailable")
		},
		insertFn: func(_ context.Context, _ *store.Schedule) error { insertCalled = true; return nil },
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:     "x",
		CronExpr: "* * * * *",
	})
	if err == nil {
		t.Error("expected error when BeginTx fails")
	}
	if insertCalled {
		t.Error("Insert must not be called when BeginTx fails")
	}
}

func TestCreate_InsertError(t *testing.T) {
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, _ *store.Schedule) error {
			return errors.New("db down")
		},
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:     "x",
		CronExpr: "* * * * *",
	})
	if err == nil || err.Error() != "db down" {
		t.Errorf("expected Insert error to be propagated, got %v", err)
	}
}

func TestCreate_MaxExecutionsPropagated(t *testing.T) {
	var inserted *store.Schedule
	svc := newSvc(&mockStore{
		insertFn: func(_ context.Context, sch *store.Schedule) error { inserted = sch; return nil },
	})
	_, err := svc.Create(context.Background(), store.CreateScheduleInput{
		Name:          "once",
		CronExpr:      "* * * * *",
		MaxExecutions: ptr(int64(1)),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inserted.MaxExecutions == nil || *inserted.MaxExecutions != 1 {
		t.Error("expected max_executions=1 to be propagated to Insert")
	}
}

// --- Get ---

func TestGet_HappyPath(t *testing.T) {
	expected := &store.Schedule{ID: "abc", Name: "test"}
	svc := newSvc(&mockStore{
		getFn: func(_ context.Context, id string) (*store.Schedule, error) {
			if id != "abc" {
				t.Errorf("unexpected id passed to Get: %q", id)
			}
			return expected, nil
		},
	})
	got, err := svc.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Error("returned schedule does not match expected")
	}
}

func TestGet_StoreError(t *testing.T) {
	svc := newSvc(&mockStore{
		getFn: func(_ context.Context, _ string) (*store.Schedule, error) {
			return nil, errors.New("schedule not found")
		},
	})
	_, err := svc.Get(context.Background(), "missing")
	if err == nil {
		t.Error("expected error from store to be propagated")
	}
}

// --- List ---

func TestList_HappyPath(t *testing.T) {
	expected := []*store.Schedule{{ID: "1"}, {ID: "2"}}
	svc := newSvc(&mockStore{
		listFn: func(_ context.Context) ([]*store.Schedule, error) { return expected, nil },
	})
	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 schedules, got %d", len(got))
	}
}

func TestList_Empty(t *testing.T) {
	svc := newSvc(&mockStore{
		listFn: func(_ context.Context) ([]*store.Schedule, error) { return nil, nil },
	})
	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %d items", len(got))
	}
}

func TestList_StoreError(t *testing.T) {
	svc := newSvc(&mockStore{
		listFn: func(_ context.Context) ([]*store.Schedule, error) {
			return nil, errors.New("db error")
		},
	})
	_, err := svc.List(context.Background())
	if err == nil {
		t.Error("expected error from store to be propagated")
	}
}

// --- Update ---

func TestUpdate_Name(t *testing.T) {
	var saved *store.Schedule
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, sch *store.Schedule) error { saved = sch; return nil },
	})
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		Name: ptr("new name"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "new name" {
		t.Errorf("expected name %q, got %q", "new name", got.Name)
	}
	if saved.Name != "new name" {
		t.Error("Save was called with wrong name")
	}
	if got.Description != "original desc" {
		t.Errorf("unset field Description should be unchanged, got %q", got.Description)
	}
}

func TestUpdate_Description(t *testing.T) {
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		Description: ptr("new desc"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Description != "new desc" {
		t.Errorf("expected description %q, got %q", "new desc", got.Description)
	}
}

func TestUpdate_CronExpr(t *testing.T) {
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		CronExpr: ptr("0 12 * * *"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CronExpr != "0 12 * * *" {
		t.Errorf("expected cron_expr %q, got %q", "0 12 * * *", got.CronExpr)
	}
}

func TestUpdate_IntervalSec(t *testing.T) {
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		IntervalSec: ptr(int64(120)),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IntervalSec == nil || *got.IntervalSec != 120 {
		t.Errorf("expected interval_sec 120, got %v", got.IntervalSec)
	}
}

func TestUpdate_Payload(t *testing.T) {
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})
	newPayload := json.RawMessage(`{"new":"data"}`)
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		Payload: &newPayload,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got.Payload) != `{"new":"data"}` {
		t.Errorf("unexpected payload: %s", got.Payload)
	}
}

func TestUpdate_Status(t *testing.T) {
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		Status: ptr(store.StatusPaused),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != store.StatusPaused {
		t.Errorf("expected status paused, got %q", got.Status)
	}
}

func TestUpdate_MaxExecutions(t *testing.T) {
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		MaxExecutions: ptr(int64(10)),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaxExecutions == nil || *got.MaxExecutions != 10 {
		t.Errorf("expected max_executions 10, got %v", got.MaxExecutions)
	}
}

func TestUpdate_EmptyPatch_OnlySetsUpdatedAt(t *testing.T) {
	base := baseSchedule()
	before := time.Now().UTC()
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return base, nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { return nil },
	})
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != base.Name {
		t.Error("Name should not change on empty patch")
	}
	if got.UpdatedAt.Before(before) {
		t.Error("UpdatedAt should be set to approximately now")
	}
}

func TestUpdate_MaxExecutionsZero_RejectsBeforeGet(t *testing.T) {
	getCalled := false
	svc := newSvc(&mockStore{
		getFn: func(_ context.Context, _ string) (*store.Schedule, error) {
			getCalled = true
			return baseSchedule(), nil
		},
	})
	_, err := svc.Update(context.Background(), "x", store.UpdateScheduleInput{
		MaxExecutions: ptr(int64(0)),
	})
	if err == nil {
		t.Error("expected error for max_executions=0")
	}
	if getCalled {
		t.Error("Get must not be called when validation fails")
	}
}

func TestUpdate_MaxExecutionsNegative(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.Update(context.Background(), "x", store.UpdateScheduleInput{
		MaxExecutions: ptr(int64(-1)),
	})
	if err == nil {
		t.Error("expected error for negative max_executions")
	}
}

func TestUpdate_InvalidTimezone(t *testing.T) {
	saveCalled := false
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { saveCalled = true; return nil },
	})
	_, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		Timezone: ptr("Not/ATimezone"),
	})
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
	if saveCalled {
		t.Error("Save must not be called when timezone is invalid")
	}
}

func TestUpdate_ValidTimezone(t *testing.T) {
	var saved *store.Schedule
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, sch *store.Schedule) error { saved = sch; return nil },
	})
	got, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{
		Timezone: ptr("America/New_York"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Timezone != "America/New_York" {
		t.Errorf("expected timezone America/New_York, got %q", got.Timezone)
	}
	if saved.Timezone != "America/New_York" {
		t.Error("Save was called with wrong timezone")
	}
}

func TestUpdate_BeginTxError(t *testing.T) {
	getCalled := false
	svc := newSvc(&mockStore{
		beginTxFn: func(ctx context.Context) (context.Context, func() error, func() error, error) {
			noop := func() error { return nil }
			return ctx, noop, noop, errors.New("db unavailable")
		},
		getFn: func(_ context.Context, _ string) (*store.Schedule, error) {
			getCalled = true
			return baseSchedule(), nil
		},
	})
	_, err := svc.Update(context.Background(), "x", store.UpdateScheduleInput{})
	if err == nil {
		t.Error("expected error when BeginTx fails")
	}
	if getCalled {
		t.Error("Get must not be called when BeginTx fails")
	}
}

func TestUpdate_GetError(t *testing.T) {
	saveCalled := false
	svc := newSvc(&mockStore{
		getFn: func(_ context.Context, _ string) (*store.Schedule, error) {
			return nil, errors.New("not found")
		},
		saveFn: func(_ context.Context, _ *store.Schedule) error { saveCalled = true; return nil },
	})
	_, err := svc.Update(context.Background(), "missing", store.UpdateScheduleInput{})
	if err == nil {
		t.Error("expected Get error to be propagated")
	}
	if saveCalled {
		t.Error("Save must not be called when Get fails")
	}
}

func TestUpdate_SaveError(t *testing.T) {
	svc := newSvc(&mockStore{
		getFn:  func(_ context.Context, _ string) (*store.Schedule, error) { return baseSchedule(), nil },
		saveFn: func(_ context.Context, _ *store.Schedule) error { return errors.New("db error") },
	})
	_, err := svc.Update(context.Background(), "existing-id", store.UpdateScheduleInput{})
	if err == nil || err.Error() != "db error" {
		t.Errorf("expected Save error to be propagated, got %v", err)
	}
}

// --- Delete ---

func TestDelete_HappyPath(t *testing.T) {
	svc := newSvc(&mockStore{
		deleteFn: func(_ context.Context, id string) error {
			if id != "target" {
				t.Errorf("unexpected id: %q", id)
			}
			return nil
		},
	})
	if err := svc.Delete(context.Background(), "target"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_BeginTxError(t *testing.T) {
	deleteCalled := false
	svc := newSvc(&mockStore{
		beginTxFn: func(ctx context.Context) (context.Context, func() error, func() error, error) {
			noop := func() error { return nil }
			return ctx, noop, noop, errors.New("db unavailable")
		},
		deleteFn: func(_ context.Context, _ string) error { deleteCalled = true; return nil },
	})
	if err := svc.Delete(context.Background(), "x"); err == nil {
		t.Error("expected error when BeginTx fails")
	}
	if deleteCalled {
		t.Error("Delete must not be called when BeginTx fails")
	}
}

func TestDelete_StoreError(t *testing.T) {
	svc := newSvc(&mockStore{
		deleteFn: func(_ context.Context, _ string) error { return errors.New("db error") },
	})
	if err := svc.Delete(context.Background(), "x"); err == nil {
		t.Error("expected error from store to be propagated")
	}
}

// --- DueSchedules ---

func TestDueSchedules_BeginTxError(t *testing.T) {
	svc := newSvc(&mockStore{
		beginTxFn: func(ctx context.Context) (context.Context, func() error, func() error, error) {
			noop := func() error { return nil }
			return ctx, noop, noop, errors.New("db unavailable")
		},
	})
	_, err := svc.DueSchedules(context.Background(), 10)
	if err == nil {
		t.Error("expected error when BeginTx fails")
	}
}

func TestDueSchedules_HappyPath(t *testing.T) {
	expected := []*store.Schedule{{ID: "s1"}, {ID: "s2"}}
	svc := newSvc(&mockStore{
		dueSchedulesFn: func(_ context.Context, limit int) ([]*store.Schedule, error) {
			if limit != 50 {
				t.Errorf("unexpected limit: %d", limit)
			}
			return expected, nil
		},
	})
	got, err := svc.DueSchedules(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 schedules, got %d", len(got))
	}
}

func TestDueSchedules_StoreError(t *testing.T) {
	svc := newSvc(&mockStore{
		dueSchedulesFn: func(_ context.Context, _ int) ([]*store.Schedule, error) {
			return nil, errors.New("db error")
		},
	})
	_, err := svc.DueSchedules(context.Background(), 10)
	if err == nil {
		t.Error("expected error from store to be propagated")
	}
}

// --- AdvanceNextRun ---

func TestAdvanceNextRun_BeginTxError(t *testing.T) {
	svc := newSvc(&mockStore{
		beginTxFn: func(ctx context.Context) (context.Context, func() error, func() error, error) {
			noop := func() error { return nil }
			return ctx, noop, noop, errors.New("db unavailable")
		},
	})
	sch := &store.Schedule{ID: "s1", CronExpr: "* * * * *"}
	if err := svc.AdvanceNextRun(context.Background(), sch); err == nil {
		t.Error("expected error when BeginTx fails")
	}
}

func TestAdvanceNextRun_OneShot_NoInterval(t *testing.T) {
	completedID := ""
	completedCount := int64(-1)
	svc := newSvc(&mockStore{
		completeFn: func(_ context.Context, id string, count int64) error {
			completedID = id
			completedCount = count
			return nil
		},
	})
	sch := &store.Schedule{ID: "s1", CronExpr: "", IntervalSec: nil, ExecutionCount: 0}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if completedID != "s1" {
		t.Errorf("expected Complete called with id s1, got %q", completedID)
	}
	if completedCount != 1 {
		t.Errorf("expected count 1 passed to Complete, got %d", completedCount)
	}
	if sch.ExecutionCount != 1 {
		t.Errorf("expected sch.ExecutionCount incremented to 1, got %d", sch.ExecutionCount)
	}
}

func TestAdvanceNextRun_OneShot_ZeroInterval(t *testing.T) {
	completeCalled := false
	svc := newSvc(&mockStore{
		completeFn: func(_ context.Context, _ string, _ int64) error { completeCalled = true; return nil },
	})
	sch := &store.Schedule{ID: "s1", CronExpr: "", IntervalSec: ptr(int64(0))}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completeCalled {
		t.Error("expected Complete to be called for zero-interval one-shot")
	}
}

func TestAdvanceNextRun_OneShot_NegativeInterval(t *testing.T) {
	completeCalled := false
	svc := newSvc(&mockStore{
		completeFn: func(_ context.Context, _ string, _ int64) error { completeCalled = true; return nil },
	})
	sch := &store.Schedule{ID: "s1", CronExpr: "", IntervalSec: ptr(int64(-1))}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completeCalled {
		t.Error("expected Complete to be called for negative-interval one-shot")
	}
}

func TestAdvanceNextRun_Interval_BelowLimit(t *testing.T) {
	setNextRunCalled := false
	completeCalled := false
	before := time.Now().UTC()
	svc := newSvc(&mockStore{
		setNextRunFn: func(_ context.Context, id string, next time.Time, count int64) error {
			setNextRunCalled = true
			if id != "s1" {
				t.Errorf("unexpected id: %q", id)
			}
			if !next.After(before) {
				t.Error("expected next run to be in the future")
			}
			if count != 1 {
				t.Errorf("expected count 1, got %d", count)
			}
			return nil
		},
		completeFn: func(_ context.Context, _ string, _ int64) error { completeCalled = true; return nil },
	})
	sch := &store.Schedule{
		ID:             "s1",
		IntervalSec:    ptr(int64(60)),
		MaxExecutions:  ptr(int64(5)),
		ExecutionCount: 0,
	}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !setNextRunCalled {
		t.Error("expected SetNextRun to be called")
	}
	if completeCalled {
		t.Error("Complete must not be called below limit")
	}
}

func TestAdvanceNextRun_Interval_NextTimeInFuture(t *testing.T) {
	before := time.Now().UTC()
	svc := newSvc(&mockStore{
		setNextRunFn: func(_ context.Context, _ string, next time.Time, _ int64) error {
			lower := before.Add(59 * time.Second)
			upper := before.Add(61 * time.Second)
			if next.Before(lower) || next.After(upper) {
				t.Errorf("next run %v outside expected range for 60s interval", next)
			}
			return nil
		},
	})
	sch := &store.Schedule{ID: "s1", IntervalSec: ptr(int64(60))}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdvanceNextRun_Cron_BelowLimit(t *testing.T) {
	setNextRunCalled := false
	completeCalled := false
	before := time.Now().UTC()
	svc := newSvc(&mockStore{
		setNextRunFn: func(_ context.Context, _ string, next time.Time, _ int64) error {
			setNextRunCalled = true
			if !next.After(before) {
				t.Error("expected next run to be in the future for cron schedule")
			}
			return nil
		},
		completeFn: func(_ context.Context, _ string, _ int64) error { completeCalled = true; return nil },
	})
	sch := &store.Schedule{
		ID:             "s1",
		CronExpr:       "* * * * *",
		MaxExecutions:  ptr(int64(10)),
		ExecutionCount: 0,
	}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !setNextRunCalled {
		t.Error("expected SetNextRun to be called for cron schedule below limit")
	}
	if completeCalled {
		t.Error("Complete must not be called below limit")
	}
}

func TestAdvanceNextRun_Interval_ReachesLimit(t *testing.T) {
	completeCalled := false
	setNextRunCalled := false
	svc := newSvc(&mockStore{
		completeFn: func(_ context.Context, _ string, count int64) error {
			completeCalled = true
			if count != 3 {
				t.Errorf("expected count 3, got %d", count)
			}
			return nil
		},
		setNextRunFn: func(_ context.Context, _ string, _ time.Time, _ int64) error {
			setNextRunCalled = true
			return nil
		},
	})
	// ExecutionCount 2, MaxExecutions 3: after increment count=3 == max → complete.
	sch := &store.Schedule{
		ID:             "s1",
		IntervalSec:    ptr(int64(60)),
		MaxExecutions:  ptr(int64(3)),
		ExecutionCount: 2,
	}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completeCalled {
		t.Error("expected Complete to be called when limit is reached")
	}
	if setNextRunCalled {
		t.Error("SetNextRun must not be called when limit is reached")
	}
}

func TestAdvanceNextRun_Cron_ReachesLimit(t *testing.T) {
	completeCalled := false
	svc := newSvc(&mockStore{
		completeFn: func(_ context.Context, _ string, _ int64) error { completeCalled = true; return nil },
	})
	// ExecutionCount 0, MaxExecutions 1: after increment count=1 == max → complete.
	sch := &store.Schedule{
		ID:             "s1",
		CronExpr:       "* * * * *",
		MaxExecutions:  ptr(int64(1)),
		ExecutionCount: 0,
	}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completeCalled {
		t.Error("expected Complete to be called when limit is reached")
	}
}

func TestAdvanceNextRun_ExceedsLimit(t *testing.T) {
	completeCalled := false
	svc := newSvc(&mockStore{
		completeFn: func(_ context.Context, _ string, _ int64) error { completeCalled = true; return nil },
	})
	// ExecutionCount already past max (shouldn't happen in practice, but must be handled).
	sch := &store.Schedule{
		ID:             "s1",
		CronExpr:       "* * * * *",
		MaxExecutions:  ptr(int64(3)),
		ExecutionCount: 5,
	}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completeCalled {
		t.Error("expected Complete to be called when count exceeds max")
	}
}

func TestAdvanceNextRun_NoMaxExecutions_NeverCompletes(t *testing.T) {
	completeCalled := false
	svc := newSvc(&mockStore{
		setNextRunFn: func(_ context.Context, _ string, _ time.Time, _ int64) error { return nil },
		completeFn:   func(_ context.Context, _ string, _ int64) error { completeCalled = true; return nil },
	})
	sch := &store.Schedule{
		ID:             "s1",
		CronExpr:       "* * * * *",
		MaxExecutions:  nil,
		ExecutionCount: 999999,
	}
	if err := svc.AdvanceNextRun(context.Background(), sch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if completeCalled {
		t.Error("Complete must not be called when MaxExecutions is nil")
	}
}

func TestAdvanceNextRun_InvalidCron(t *testing.T) {
	svc := newSvc(&mockStore{})
	// A corrupted cron that somehow bypassed Create validation.
	sch := &store.Schedule{ID: "s1", CronExpr: "not-valid-cron"}
	err := svc.AdvanceNextRun(context.Background(), sch)
	if err == nil {
		t.Error("expected error for unparseable cron expression")
	}
}

func TestAdvanceNextRun_CompleteError(t *testing.T) {
	svc := newSvc(&mockStore{
		completeFn: func(_ context.Context, _ string, _ int64) error { return errors.New("db error") },
	})
	sch := &store.Schedule{ID: "s1"} // one-shot
	if err := svc.AdvanceNextRun(context.Background(), sch); err == nil || err.Error() != "db error" {
		t.Errorf("expected Complete error to be propagated, got %v", err)
	}
}

func TestAdvanceNextRun_SetNextRunError(t *testing.T) {
	svc := newSvc(&mockStore{
		setNextRunFn: func(_ context.Context, _ string, _ time.Time, _ int64) error {
			return errors.New("db error")
		},
	})
	sch := &store.Schedule{ID: "s1", CronExpr: "* * * * *"}
	if err := svc.AdvanceNextRun(context.Background(), sch); err == nil || err.Error() != "db error" {
		t.Errorf("expected SetNextRun error to be propagated, got %v", err)
	}
}
