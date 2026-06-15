package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// capture records the request a handler received for later assertions.
type capture struct {
	method string
	path   string
	query  string
	auth   string
	body   []byte
}

// newTestClient starts an httptest server whose handler records the request
// into c and then runs respond. It returns a Client pointed at the server.
func newTestClient(t *testing.T, c *capture, respond func(w http.ResponseWriter, r *http.Request), opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.method = r.Method
		c.path = r.URL.Path
		c.query = r.URL.RawQuery
		c.auth = r.Header.Get("Authorization")
		c.body = body
		respond(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, opts...)
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func TestCreate(t *testing.T) {
	var cap capture
	want := &Schedule{ID: "abc", Name: "nightly", Status: StatusActive}
	c := newTestClient(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, want)
	}, WithAPIKey("secret"))

	got, err := c.Create(context.Background(), CreateScheduleInput{
		Name:     "nightly",
		CronExpr: "0 0 * * *",
		Payload:  json.RawMessage(`{"foo":"bar"}`),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if cap.method != http.MethodPost {
		t.Errorf("method = %q, want POST", cap.method)
	}
	if cap.path != "/api/v1/schedules" {
		t.Errorf("path = %q", cap.path)
	}
	if cap.auth != "Bearer secret" {
		t.Errorf("auth = %q, want Bearer secret", cap.auth)
	}
	var sent CreateScheduleInput
	if err := json.Unmarshal(cap.body, &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sent.Name != "nightly" || sent.CronExpr != "0 0 * * *" {
		t.Errorf("sent body = %+v", sent)
	}
	if got.ID != want.ID || got.Status != StatusActive {
		t.Errorf("got = %+v", got)
	}
}

func TestGet(t *testing.T) {
	var cap capture
	c := newTestClient(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &Schedule{ID: "id-1", Name: "x"})
	})

	got, err := c.Get(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cap.method != http.MethodGet || cap.path != "/api/v1/schedules/id-1" {
		t.Errorf("method=%q path=%q", cap.method, cap.path)
	}
	if cap.auth != "" {
		t.Errorf("auth = %q, want empty (no api key)", cap.auth)
	}
	if got.ID != "id-1" {
		t.Errorf("got = %+v", got)
	}
}

func TestUpdate(t *testing.T) {
	var cap capture
	c := newTestClient(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &Schedule{ID: "id-1", Name: "renamed"})
	})

	_, err := c.Update(context.Background(), "id-1", UpdateScheduleInput{Name: ptr("renamed")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if cap.method != http.MethodPatch || cap.path != "/api/v1/schedules/id-1" {
		t.Errorf("method=%q path=%q", cap.method, cap.path)
	}
	var sent map[string]any
	if err := json.Unmarshal(cap.body, &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sent["name"] != "renamed" {
		t.Errorf("sent name = %v", sent["name"])
	}
	// Unset pointer fields serialize as null.
	if v, ok := sent["status"]; !ok || v != nil {
		t.Errorf("sent status = %v (ok=%v), want null", v, ok)
	}
}

func TestDelete(t *testing.T) {
	var cap capture
	c := newTestClient(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.Delete(context.Background(), "id-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if cap.method != http.MethodDelete || cap.path != "/api/v1/schedules/id-1" {
		t.Errorf("method=%q path=%q", cap.method, cap.path)
	}
}

func TestPauseResume(t *testing.T) {
	tests := []struct {
		name       string
		call       func(c *Client) (*Schedule, error)
		wantStatus string
	}{
		{"pause", func(c *Client) (*Schedule, error) { return c.Pause(context.Background(), "id-1") }, "paused"},
		{"resume", func(c *Client) (*Schedule, error) { return c.Resume(context.Background(), "id-1") }, "active"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cap capture
			c := newTestClient(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, http.StatusOK, &Schedule{ID: "id-1", Status: Status(tt.wantStatus)})
			})

			got, err := tt.call(c)
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if cap.method != http.MethodPatch || cap.path != "/api/v1/schedules/id-1" {
				t.Errorf("method=%q path=%q", cap.method, cap.path)
			}
			var sent map[string]any
			if err := json.Unmarshal(cap.body, &sent); err != nil {
				t.Fatalf("unmarshal sent body: %v", err)
			}
			if sent["status"] != tt.wantStatus {
				t.Errorf("sent status = %v, want %s", sent["status"], tt.wantStatus)
			}
			if string(got.Status) != tt.wantStatus {
				t.Errorf("got status = %s", got.Status)
			}
		})
	}
}

func TestListQuery(t *testing.T) {
	var cap capture
	c := newTestClient(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, ListResult{
			Data:  []*Schedule{{ID: "a"}},
			Page:  2,
			Limit: 10,
			Total: 1,
		})
	})

	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err := c.List(context.Background(), ListOptions{
		Name:           ptr("nightly*"),
		Statuses:       []Status{StatusActive, StatusPaused},
		ExecutionCount: []Condition{{Op: OpGt, Value: int64(5)}},
		CreatedAt:      []Condition{{Op: OpGte, Value: created}},
		Sort:           []SortOrder{{Field: "updated_at", Dir: SortDesc}, {Field: "name", Dir: SortAsc}},
		Page:           2,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if cap.method != http.MethodGet || cap.path != "/api/v1/schedules" {
		t.Errorf("method=%q path=%q", cap.method, cap.path)
	}

	q, err := url.ParseQuery(cap.query)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if q.Get("name") != "nightly*" {
		t.Errorf("name = %q", q.Get("name"))
	}
	if got := q["status"]; len(got) != 2 || got[0] != "active" || got[1] != "paused" {
		t.Errorf("status = %v", got)
	}
	if q.Get("execution_count") != "gt:5" {
		t.Errorf("execution_count = %q", q.Get("execution_count"))
	}
	if q.Get("created_at") != "gte:2024-01-01T00:00:00Z" {
		t.Errorf("created_at = %q", q.Get("created_at"))
	}
	if q.Get("sort") != "updated_at:desc,name:asc" {
		t.Errorf("sort = %q", q.Get("sort"))
	}
	if q.Get("page") != "2" || q.Get("limit") != "10" {
		t.Errorf("page=%q limit=%q", q.Get("page"), q.Get("limit"))
	}

	if res.Total != 1 || len(res.Data) != 1 || res.Data[0].ID != "a" {
		t.Errorf("res = %+v", res)
	}
}

func TestListEmptyOptions(t *testing.T) {
	var cap capture
	c := newTestClient(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, ListResult{Data: []*Schedule{}})
	})

	if _, err := c.List(context.Background(), ListOptions{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if cap.query != "" {
		t.Errorf("query = %q, want empty", cap.query)
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, &capture{}, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]string{"error": "not found"})
	})

	_, err := c.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err is not *APIError: %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound || apiErr.Message != "not found" {
		t.Errorf("apiErr = %+v", apiErr)
	}
	if !IsNotFound(err) {
		t.Error("IsNotFound = false, want true")
	}
}
