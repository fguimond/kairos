package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/yourorg/kairos/internal/store"
)

type mockStore struct {
	dueSchedules   func(ctx context.Context, limit int) ([]*store.Schedule, error)
	advanceNextRun func(ctx context.Context, sch *store.Schedule) error
}

func (m *mockStore) DueSchedules(ctx context.Context, limit int) ([]*store.Schedule, error) {
	return m.dueSchedules(ctx, limit)
}

func (m *mockStore) AdvanceNextRun(ctx context.Context, sch *store.Schedule) error {
	return m.advanceNextRun(ctx, sch)
}

type mockPublisher struct {
	publish func(ctx context.Context, msg []byte) error
}

func (m *mockPublisher) Publish(ctx context.Context, msg []byte) error {
	return m.publish(ctx, msg)
}

func newTestEngine(s Store, p Publisher) *Engine {
	return &Engine{store: s, publisher: p, pollInterval: time.Second, batchSize: 100}
}

// --- tick happy paths ---

func TestTick_NoDueSchedules(t *testing.T) {
	advanced := 0
	e := newTestEngine(
		&mockStore{
			dueSchedules:   func(ctx context.Context, limit int) ([]*store.Schedule, error) { return nil, nil },
			advanceNextRun: func(ctx context.Context, sch *store.Schedule) error { advanced++; return nil },
		},
		&mockPublisher{publish: func(ctx context.Context, msg []byte) error { return nil }},
	)
	e.tick(context.Background())
	if advanced != 0 {
		t.Errorf("expected no advances with no due schedules, got %d", advanced)
	}
}

func TestTick_PublishesAndAdvancesSchedule(t *testing.T) {
	published := 0
	advanced := 0
	sch := &store.Schedule{ID: "s1", Name: "hourly", Payload: json.RawMessage(`{}`)}

	e := newTestEngine(
		&mockStore{
			dueSchedules:   func(ctx context.Context, limit int) ([]*store.Schedule, error) { return []*store.Schedule{sch}, nil },
			advanceNextRun: func(ctx context.Context, s *store.Schedule) error { advanced++; return nil },
		},
		&mockPublisher{publish: func(ctx context.Context, msg []byte) error { published++; return nil }},
	)
	e.tick(context.Background())
	if published != 1 {
		t.Errorf("expected 1 publish, got %d", published)
	}
	if advanced != 1 {
		t.Errorf("expected 1 advance, got %d", advanced)
	}
}

func TestTick_MultipleSchedules_AllProcessed(t *testing.T) {
	published := 0
	advanced := 0
	schedules := []*store.Schedule{
		{ID: "s1", Name: "a", Payload: json.RawMessage(`{}`)},
		{ID: "s2", Name: "b", Payload: json.RawMessage(`{}`)},
		{ID: "s3", Name: "c", Payload: json.RawMessage(`{}`)},
	}

	e := newTestEngine(
		&mockStore{
			dueSchedules:   func(ctx context.Context, limit int) ([]*store.Schedule, error) { return schedules, nil },
			advanceNextRun: func(ctx context.Context, s *store.Schedule) error { advanced++; return nil },
		},
		&mockPublisher{publish: func(ctx context.Context, msg []byte) error { published++; return nil }},
	)
	e.tick(context.Background())
	if published != 3 {
		t.Errorf("expected 3 publishes, got %d", published)
	}
	if advanced != 3 {
		t.Errorf("expected 3 advances, got %d", advanced)
	}
}

func TestTick_MessageContainsCorrectFields(t *testing.T) {
	sch := &store.Schedule{
		ID:      "test-id",
		Name:    "test-schedule",
		Payload: json.RawMessage(`{"key":"value"}`),
	}
	var captured []byte

	e := newTestEngine(
		&mockStore{
			dueSchedules:   func(ctx context.Context, limit int) ([]*store.Schedule, error) { return []*store.Schedule{sch}, nil },
			advanceNextRun: func(ctx context.Context, s *store.Schedule) error { return nil },
		},
		&mockPublisher{publish: func(ctx context.Context, msg []byte) error { captured = msg; return nil }},
	)
	e.tick(context.Background())

	var tm TriggerMessage
	if err := json.Unmarshal(captured, &tm); err != nil {
		t.Fatalf("failed to unmarshal trigger message: %v", err)
	}
	if tm.ScheduleID != "test-id" {
		t.Errorf("expected schedule_id=test-id, got %s", tm.ScheduleID)
	}
	if tm.ScheduleName != "test-schedule" {
		t.Errorf("expected schedule_name=test-schedule, got %s", tm.ScheduleName)
	}
	if string(tm.Payload) != `{"key":"value"}` {
		t.Errorf("unexpected payload: %s", tm.Payload)
	}
	if tm.FiredAt.IsZero() {
		t.Error("expected non-zero fired_at")
	}
}

// --- tick error paths ---

func TestTick_DueSchedulesError_NoPublishOrAdvance(t *testing.T) {
	published := 0
	advanced := 0

	e := newTestEngine(
		&mockStore{
			dueSchedules:   func(ctx context.Context, limit int) ([]*store.Schedule, error) { return nil, errors.New("db error") },
			advanceNextRun: func(ctx context.Context, s *store.Schedule) error { advanced++; return nil },
		},
		&mockPublisher{publish: func(ctx context.Context, msg []byte) error { published++; return nil }},
	)
	e.tick(context.Background())
	if published != 0 || advanced != 0 {
		t.Errorf("expected no publish/advance when DueSchedules fails, got published=%d advanced=%d", published, advanced)
	}
}

func TestTick_PublishError_SkipsAdvance(t *testing.T) {
	advanced := 0
	sch := &store.Schedule{ID: "s1", Name: "test", Payload: json.RawMessage(`{}`)}

	e := newTestEngine(
		&mockStore{
			dueSchedules:   func(ctx context.Context, limit int) ([]*store.Schedule, error) { return []*store.Schedule{sch}, nil },
			advanceNextRun: func(ctx context.Context, s *store.Schedule) error { advanced++; return nil },
		},
		&mockPublisher{publish: func(ctx context.Context, msg []byte) error { return errors.New("amqp down") }},
	)
	e.tick(context.Background())
	if advanced != 0 {
		t.Errorf("expected 0 advances when publish fails, got %d", advanced)
	}
}

func TestTick_PublishError_ContinuesToNextSchedule(t *testing.T) {
	// First publish fails, second succeeds — second schedule should still be advanced.
	published := 0
	advanced := 0
	schedules := []*store.Schedule{
		{ID: "s1", Name: "fail", Payload: json.RawMessage(`{}`)},
		{ID: "s2", Name: "ok", Payload: json.RawMessage(`{}`)},
	}

	e := newTestEngine(
		&mockStore{
			dueSchedules:   func(ctx context.Context, limit int) ([]*store.Schedule, error) { return schedules, nil },
			advanceNextRun: func(ctx context.Context, s *store.Schedule) error { advanced++; return nil },
		},
		&mockPublisher{publish: func(ctx context.Context, msg []byte) error {
			published++
			if published == 1 {
				return errors.New("first publish failed")
			}
			return nil
		}},
	)
	e.tick(context.Background())
	if published != 2 {
		t.Errorf("expected 2 publish attempts, got %d", published)
	}
	if advanced != 1 {
		t.Errorf("expected 1 advance (only the successful publish), got %d", advanced)
	}
}

func TestTick_AdvanceError_DoesNotStopLoop(t *testing.T) {
	// Even when AdvanceNextRun errors, all schedules are still published.
	published := 0
	schedules := []*store.Schedule{
		{ID: "s1", Name: "a", Payload: json.RawMessage(`{}`)},
		{ID: "s2", Name: "b", Payload: json.RawMessage(`{}`)},
	}

	e := newTestEngine(
		&mockStore{
			dueSchedules:   func(ctx context.Context, limit int) ([]*store.Schedule, error) { return schedules, nil },
			advanceNextRun: func(ctx context.Context, s *store.Schedule) error { return errors.New("db error") },
		},
		&mockPublisher{publish: func(ctx context.Context, msg []byte) error { published++; return nil }},
	)
	e.tick(context.Background())
	if published != 2 {
		t.Errorf("expected 2 publishes even with advance errors, got %d", published)
	}
}
