package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourorg/kairos/internal/store"
)

// mockService implements scheduleService for testing.
type mockService struct {
	list   func(ctx context.Context) ([]*store.Schedule, error)
	create func(ctx context.Context, in store.CreateScheduleInput) (*store.Schedule, error)
	get    func(ctx context.Context, id string) (*store.Schedule, error)
	update func(ctx context.Context, id string, in store.UpdateScheduleInput) (*store.Schedule, error)
	del    func(ctx context.Context, id string) error
}

func (m *mockService) List(ctx context.Context) ([]*store.Schedule, error) { return m.list(ctx) }
func (m *mockService) Create(ctx context.Context, in store.CreateScheduleInput) (*store.Schedule, error) {
	return m.create(ctx, in)
}
func (m *mockService) Get(ctx context.Context, id string) (*store.Schedule, error) {
	return m.get(ctx, id)
}
func (m *mockService) Update(ctx context.Context, id string, in store.UpdateScheduleInput) (*store.Schedule, error) {
	return m.update(ctx, id, in)
}
func (m *mockService) Delete(ctx context.Context, id string) error { return m.del(ctx, id) }

func newTestServer(ms scheduleService) *Server {
	return &Server{svc: ms}
}

// --- /healthz ---

func TestHandleHealthz(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.handleHealthz(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", body["status"])
	}
}

// --- GET /api/v1/schedules ---

func TestHandleList_Happy(t *testing.T) {
	schedules := []*store.Schedule{{ID: "1", Name: "nightly"}}
	srv := newTestServer(&mockService{
		list: func(ctx context.Context) ([]*store.Schedule, error) { return schedules, nil },
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var got []*store.Schedule
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "1" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestHandleList_EmptyList(t *testing.T) {
	srv := newTestServer(&mockService{
		list: func(ctx context.Context) ([]*store.Schedule, error) { return nil, nil },
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleList_StoreError(t *testing.T) {
	srv := newTestServer(&mockService{
		list: func(ctx context.Context) ([]*store.Schedule, error) {
			return nil, errors.New("db error")
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- POST /api/v1/schedules ---

func TestHandleCreate_HappyCron(t *testing.T) {
	created := &store.Schedule{ID: "abc", Name: "daily"}
	srv := newTestServer(&mockService{
		create: func(ctx context.Context, in store.CreateScheduleInput) (*store.Schedule, error) {
			return created, nil
		},
	})
	body, _ := json.Marshal(map[string]any{"name": "daily", "cron_expr": "0 0 * * *"})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreate(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var got store.Schedule
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "abc" {
		t.Errorf("unexpected schedule ID: %s", got.ID)
	}
}

func TestHandleCreate_HappyInterval(t *testing.T) {
	created := &store.Schedule{ID: "xyz", Name: "every-5m"}
	srv := newTestServer(&mockService{
		create: func(ctx context.Context, in store.CreateScheduleInput) (*store.Schedule, error) {
			return created, nil
		},
	})
	body, _ := json.Marshal(map[string]any{"name": "every-5m", "interval_sec": 300})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreate(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreate_InvalidJSON(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.handleCreate(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreate_MissingName(t *testing.T) {
	srv := newTestServer(&mockService{})
	body, _ := json.Marshal(map[string]any{"cron_expr": "* * * * *"})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreate(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleCreate_MissingSchedule(t *testing.T) {
	srv := newTestServer(&mockService{})
	body, _ := json.Marshal(map[string]any{"name": "test"})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreate(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleCreate_MaxExecutionsZero(t *testing.T) {
	srv := newTestServer(&mockService{})
	body, _ := json.Marshal(map[string]any{"name": "test", "cron_expr": "* * * * *", "max_executions": 0})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreate(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleCreate_StoreError(t *testing.T) {
	srv := newTestServer(&mockService{
		create: func(ctx context.Context, in store.CreateScheduleInput) (*store.Schedule, error) {
			return nil, errors.New("invalid cron expression")
		},
	})
	body, _ := json.Marshal(map[string]any{"name": "bad", "cron_expr": "bad-cron"})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreate(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- GET /api/v1/schedules/{id} ---

func TestHandleGet_Happy(t *testing.T) {
	sch := &store.Schedule{ID: "abc-123", Name: "test"}
	srv := newTestServer(&mockService{
		get: func(ctx context.Context, id string) (*store.Schedule, error) {
			if id != "abc-123" {
				t.Errorf("unexpected id: %s", id)
			}
			return sch, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/abc-123", nil)
	r.SetPathValue("id", "abc-123")
	w := httptest.NewRecorder()
	srv.handleGet(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var got store.Schedule
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "abc-123" {
		t.Errorf("unexpected ID: %s", got.ID)
	}
}

func TestHandleGet_NotFound(t *testing.T) {
	srv := newTestServer(&mockService{
		get: func(ctx context.Context, id string) (*store.Schedule, error) {
			return nil, errors.New("schedule not found")
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/missing", nil)
	r.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	srv.handleGet(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- PATCH /api/v1/schedules/{id} ---

func TestHandleUpdate_Happy(t *testing.T) {
	updated := &store.Schedule{ID: "abc", Name: "updated-name"}
	srv := newTestServer(&mockService{
		update: func(ctx context.Context, id string, in store.UpdateScheduleInput) (*store.Schedule, error) {
			return updated, nil
		},
	})
	name := "updated-name"
	body, _ := json.Marshal(store.UpdateScheduleInput{Name: &name})
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/abc", bytes.NewReader(body))
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	srv.handleUpdate(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got store.Schedule
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "updated-name" {
		t.Errorf("unexpected name: %s", got.Name)
	}
}

func TestHandleUpdate_InvalidJSON(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/abc", bytes.NewReader([]byte("{")))
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	srv.handleUpdate(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdate_MaxExecutionsNegative(t *testing.T) {
	srv := newTestServer(&mockService{})
	maxExecutions := int64(-1)
	body, _ := json.Marshal(store.UpdateScheduleInput{MaxExecutions: &maxExecutions})
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/abc", bytes.NewReader(body))
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	srv.handleUpdate(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleUpdate_StoreError(t *testing.T) {
	srv := newTestServer(&mockService{
		update: func(ctx context.Context, id string, in store.UpdateScheduleInput) (*store.Schedule, error) {
			return nil, errors.New("db error")
		},
	})
	body, _ := json.Marshal(store.UpdateScheduleInput{})
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/abc", bytes.NewReader(body))
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	srv.handleUpdate(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- DELETE /api/v1/schedules/{id} ---

func TestHandleDelete_Happy(t *testing.T) {
	srv := newTestServer(&mockService{
		del: func(ctx context.Context, id string) error { return nil },
	})
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/abc", nil)
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	srv.handleDelete(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleDelete_StoreError(t *testing.T) {
	srv := newTestServer(&mockService{
		del: func(ctx context.Context, id string) error { return errors.New("db error") },
	})
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/abc", nil)
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	srv.handleDelete(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- middleware ---

func TestRecoveryMiddleware_CatchesPanic(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})
	h := recoveryMiddleware(panicHandler)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", w.Code)
	}
}
