package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/francisguimond/kairos/internal/store"
)

// mockService implements scheduleService for testing.
type mockService struct {
	list   func(ctx context.Context, f store.ListFilter) (*store.ListResult, error)
	create func(ctx context.Context, in store.CreateScheduleInput) (*store.Schedule, error)
	get    func(ctx context.Context, id string) (*store.Schedule, error)
	update func(ctx context.Context, id string, in store.UpdateScheduleInput) (*store.Schedule, error)
	del    func(ctx context.Context, id string) error
}

func (m *mockService) List(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
	return m.list(ctx, f)
}
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
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			return &store.ListResult{Schedules: schedules, Total: 1}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var got listResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Data) != 1 || got.Data[0].ID != "1" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
	if got.Page != 1 || got.Limit != 50 || got.Total != 1 {
		t.Errorf("unexpected pagination metadata: page=%d limit=%d total=%d", got.Page, got.Limit, got.Total)
	}
}

func TestHandleList_EmptyList(t *testing.T) {
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			return &store.ListResult{}, nil
		},
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
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
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

func TestHandleList_FilterByName(t *testing.T) {
	var gotFilter store.ListFilter
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			gotFilter = f
			return &store.ListResult{}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?name=nightly*", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if gotFilter.Name == nil || *gotFilter.Name != "nightly*" {
		t.Errorf("expected Name=nightly*, got %v", gotFilter.Name)
	}
}

func TestHandleList_FilterByStatus(t *testing.T) {
	var gotFilter store.ListFilter
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			gotFilter = f
			return &store.ListResult{}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?status=active&status=paused", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(gotFilter.Statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(gotFilter.Statuses))
	}
	if gotFilter.Statuses[0] != store.StatusActive || gotFilter.Statuses[1] != store.StatusPaused {
		t.Errorf("unexpected statuses: %v", gotFilter.Statuses)
	}
}

func TestHandleList_FilterByExecutionCount(t *testing.T) {
	var gotFilter store.ListFilter
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			gotFilter = f
			return &store.ListResult{}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?execution_count=gt:5&execution_count=lt:10", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(gotFilter.ExecutionCount) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(gotFilter.ExecutionCount))
	}
	if gotFilter.ExecutionCount[0].Op != ">" || gotFilter.ExecutionCount[0].Value != int64(5) {
		t.Errorf("unexpected first condition: %+v", gotFilter.ExecutionCount[0])
	}
	if gotFilter.ExecutionCount[1].Op != "<" || gotFilter.ExecutionCount[1].Value != int64(10) {
		t.Errorf("unexpected second condition: %+v", gotFilter.ExecutionCount[1])
	}
}

func TestHandleList_FilterByNextRunAt(t *testing.T) {
	var gotFilter store.ListFilter
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			gotFilter = f
			return &store.ListResult{}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?next_run_at=gte:2024-01-01T00:00:00Z", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(gotFilter.NextRunAt) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(gotFilter.NextRunAt))
	}
	if gotFilter.NextRunAt[0].Op != ">=" {
		t.Errorf("unexpected op: %q", gotFilter.NextRunAt[0].Op)
	}
}

func TestHandleList_InvalidOperator(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?execution_count=badop:5", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList_InvalidTimestamp(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?created_at=gt:not-a-time", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList_InvalidMaxExecutions(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?max_executions=abc", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList_MissingOpSeparator(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?execution_count=10", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList_SortSingleField(t *testing.T) {
	var gotFilter store.ListFilter
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			gotFilter = f
			return &store.ListResult{}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?sort=name:asc", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(gotFilter.Sort) != 1 {
		t.Fatalf("expected 1 sort order, got %d", len(gotFilter.Sort))
	}
	if gotFilter.Sort[0].Field != "name" || gotFilter.Sort[0].Dir != "ASC" {
		t.Errorf("unexpected sort: %+v", gotFilter.Sort[0])
	}
}

func TestHandleList_SortMultipleFields(t *testing.T) {
	var gotFilter store.ListFilter
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			gotFilter = f
			return &store.ListResult{}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?sort=status:asc,name:desc", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(gotFilter.Sort) != 2 {
		t.Fatalf("expected 2 sort orders, got %d", len(gotFilter.Sort))
	}
	if gotFilter.Sort[0].Field != "status" || gotFilter.Sort[0].Dir != "ASC" {
		t.Errorf("unexpected first sort: %+v", gotFilter.Sort[0])
	}
	if gotFilter.Sort[1].Field != "name" || gotFilter.Sort[1].Dir != "DESC" {
		t.Errorf("unexpected second sort: %+v", gotFilter.Sort[1])
	}
}

func TestHandleList_SortInvalidField(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?sort=id:asc", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList_SortInvalidDirection(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?sort=name:up", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList_SortMissingDirection(t *testing.T) {
	srv := newTestServer(&mockService{})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?sort=name", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList_DefaultPagination(t *testing.T) {
	var gotFilter store.ListFilter
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			gotFilter = f
			return &store.ListResult{}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if gotFilter.Page != 1 || gotFilter.Limit != 50 {
		t.Errorf("expected default page=1 limit=50, got page=%d limit=%d", gotFilter.Page, gotFilter.Limit)
	}
	var got listResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Page != 1 || got.Limit != 50 {
		t.Errorf("expected page=1 limit=50 in response, got page=%d limit=%d", got.Page, got.Limit)
	}
}

func TestHandleList_ExplicitPageAndLimit(t *testing.T) {
	var gotFilter store.ListFilter
	srv := newTestServer(&mockService{
		list: func(ctx context.Context, f store.ListFilter) (*store.ListResult, error) {
			gotFilter = f
			return &store.ListResult{Total: 100}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?page=3&limit=10", nil)
	w := httptest.NewRecorder()
	srv.handleList(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if gotFilter.Page != 3 || gotFilter.Limit != 10 {
		t.Errorf("expected page=3 limit=10, got page=%d limit=%d", gotFilter.Page, gotFilter.Limit)
	}
	var got listResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Page != 3 || got.Limit != 10 || got.Total != 100 {
		t.Errorf("unexpected response: page=%d limit=%d total=%d", got.Page, got.Limit, got.Total)
	}
}

func TestHandleList_InvalidPage(t *testing.T) {
	srv := newTestServer(&mockService{})
	for _, q := range []string{"page=0", "page=-1", "page=abc"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?"+q, nil)
		w := httptest.NewRecorder()
		srv.handleList(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", q, w.Code)
		}
	}
}

func TestHandleList_InvalidLimit(t *testing.T) {
	srv := newTestServer(&mockService{})
	for _, q := range []string{"limit=0", "limit=-1", "limit=abc", "limit=201"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules?"+q, nil)
		w := httptest.NewRecorder()
		srv.handleList(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", q, w.Code)
		}
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

func TestHandleCreate_InvalidTimezone(t *testing.T) {
	srv := newTestServer(&mockService{})
	body, _ := json.Marshal(map[string]any{"name": "tz-job", "cron_expr": "* * * * *", "timezone": "Not/ATimezone"})
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

func TestHandleUpdate_InvalidTimezone(t *testing.T) {
	srv := newTestServer(&mockService{})
	tz := "Not/ATimezone"
	body, _ := json.Marshal(store.UpdateScheduleInput{Timezone: &tz})
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

func TestAPIKeyMiddleware_ValidKey(t *testing.T) {
	h := apiKeyMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPIKeyMiddleware_InvalidKey(t *testing.T) {
	h := apiKeyMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyMiddleware_MissingHeader(t *testing.T) {
	h := apiKeyMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyMiddleware_HealthzExempt(t *testing.T) {
	h := apiKeyMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /healthz without key, got %d", w.Code)
	}
}

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
