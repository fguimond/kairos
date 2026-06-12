//go:build integration

// Package clientit holds integration tests that drive the public kairos client
// (client/) against a real store -> service -> API stack served over httptest.
// They require Postgres and are gated behind the `integration` build tag, like
// the store integration tests. Run with:
//
//	go test -tags=integration ./internal/clientit/...
package clientit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/francisguimond/kairos/client"
	"github.com/francisguimond/kairos/internal/api"
	"github.com/francisguimond/kairos/internal/service"
	"github.com/francisguimond/kairos/internal/store"
)

// testDB is initialized once in TestMain; tests skip when it is nil.
var testDB *store.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("KAIROS_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable"
	}

	db, err := store.Connect(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client integration tests: database not available (%v) — tests will be skipped\n", err)
	} else {
		testDB = db
		defer testDB.Close()
		if err := store.Migrate(testDB); err != nil {
			fmt.Fprintf(os.Stderr, "client integration tests: migration failed: %v\n", err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

// startServer builds the real API stack over the shared test database, serves
// it via httptest, and returns the base URL. The server (and its routes) is the
// exact one the binary runs.
func startServer(t *testing.T, apiKey string) string {
	t.Helper()
	if testDB == nil {
		t.Skip("database not available")
	}
	if _, err := testDB.Exec(context.Background(), "DELETE FROM schedules"); err != nil {
		t.Fatalf("clear schedules: %v", err)
	}
	srv := api.NewServer(service.New(store.NewScheduleStore(testDB)), "", apiKey)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}

func ptr[T any](v T) *T { return &v }

// TestClientLifecycle exercises Healthz, Create, Get, Update, Pause, Resume,
// Delete and the IsNotFound error path end-to-end.
func TestClientLifecycle(t *testing.T) {
	c := client.New(startServer(t, ""))
	ctx := context.Background()

	if err := c.Healthz(ctx); err != nil {
		t.Fatalf("Healthz: %v", err)
	}

	// Create (cron-based, with a timezone and an execution cap).
	created, err := c.Create(ctx, client.CreateScheduleInput{
		Name:          "nightly",
		Description:   "nightly job",
		CronExpr:      "0 0 * * *",
		Timezone:      "America/New_York",
		Payload:       json.RawMessage(`{"job":"report"}`),
		MaxExecutions: ptr(int64(5)),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID")
	}
	if created.Status != client.StatusActive {
		t.Errorf("status = %q, want active", created.Status)
	}
	if created.NextRunAt == nil {
		t.Error("NextRunAt should be set on an active schedule")
	}
	if string(created.Payload) != `{"job":"report"}` {
		t.Errorf("payload = %s", created.Payload)
	}

	// Get round-trips the same schedule.
	got, err := c.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID || got.Name != "nightly" || got.Timezone != "America/New_York" {
		t.Errorf("Get returned %+v", got)
	}

	// Update changes mutable fields.
	updated, err := c.Update(ctx, created.ID, client.UpdateScheduleInput{
		Name:        ptr("renamed"),
		Description: ptr("updated description"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "renamed" || updated.Description != "updated description" {
		t.Errorf("Update returned %+v", updated)
	}

	// Pause clears the next run time.
	paused, err := c.Pause(ctx, created.ID)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Status != client.StatusPaused {
		t.Errorf("paused status = %q", paused.Status)
	}
	if paused.NextRunAt != nil {
		t.Errorf("paused NextRunAt = %v, want nil", paused.NextRunAt)
	}

	// Resume reactivates and recomputes the next run time.
	resumed, err := c.Resume(ctx, created.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.Status != client.StatusActive {
		t.Errorf("resumed status = %q", resumed.Status)
	}
	if resumed.NextRunAt == nil {
		t.Error("resumed NextRunAt should be recalculated")
	}

	// Delete is a soft-delete; the schedule drops out of List.
	if err := c.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	res, err := c.List(ctx, client.ListOptions{})
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	for _, s := range res.Data {
		if s.ID == created.ID {
			t.Errorf("deleted schedule %s still listed", created.ID)
		}
	}

	// Unknown ID surfaces as a 404 APIError.
	if _, err := c.Get(ctx, "does-not-exist"); !client.IsNotFound(err) {
		t.Errorf("Get(missing) error = %v, want IsNotFound", err)
	}
}

// TestClientCreateInterval covers the interval-based create branch.
func TestClientCreateInterval(t *testing.T) {
	c := client.New(startServer(t, ""))
	ctx := context.Background()

	s, err := c.Create(ctx, client.CreateScheduleInput{
		Name:        "every-minute",
		IntervalSec: ptr(int64(60)),
		Payload:     json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.IntervalSec == nil || *s.IntervalSec != 60 {
		t.Errorf("IntervalSec = %v, want 60", s.IntervalSec)
	}
}

// TestClientListFiltering exercises List with status/name filters, a timestamp
// condition, sorting, and pagination.
func TestClientListFiltering(t *testing.T) {
	c := client.New(startServer(t, ""))
	ctx := context.Background()
	before := time.Now().Add(-time.Hour)

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := c.Create(ctx, client.CreateScheduleInput{
			Name:     name,
			CronExpr: "0 0 * * *",
			Payload:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	// Name wildcard + status + created_at condition + sort.
	res, err := c.List(ctx, client.ListOptions{
		Name:      ptr("*a"), // matches alpha, beta, gamma all end in 'a'
		Statuses:  []client.Status{client.StatusActive},
		CreatedAt: []client.Condition{{Op: client.OpGte, Value: before}},
		Sort:      []client.SortOrder{{Field: "name", Dir: client.SortAsc}},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("Total = %d, want 3", res.Total)
	}
	if res.Data[0].Name != "alpha" || res.Data[2].Name != "gamma" {
		t.Errorf("sort order wrong: %s ... %s", res.Data[0].Name, res.Data[2].Name)
	}

	// Pagination: limit 2 returns a partial page but the full total.
	page1, err := c.List(ctx, client.ListOptions{
		Sort:  []client.SortOrder{{Field: "name", Dir: client.SortAsc}},
		Page:  1,
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1.Data) != 2 || page1.Total != 3 || page1.Limit != 2 {
		t.Errorf("page1 = len %d total %d limit %d", len(page1.Data), page1.Total, page1.Limit)
	}
}

// TestClientAuth verifies the Authorization header round-trips: requests without
// the key are rejected (401) and requests with it succeed.
func TestClientAuth(t *testing.T) {
	url := startServer(t, "s3cret")
	ctx := context.Background()

	// No key configured -> unauthorized.
	noKey := client.New(url)
	if _, err := noKey.List(ctx, client.ListOptions{}); !client.IsUnauthorized(err) {
		t.Errorf("List without key error = %v, want IsUnauthorized", err)
	}

	// Correct key (and a custom http client) -> authorized.
	withKey := client.New(url,
		client.WithAPIKey("s3cret"),
		client.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
	)
	if _, err := withKey.List(ctx, client.ListOptions{}); err != nil {
		t.Errorf("List with key: %v", err)
	}

	// Healthz is exempt from auth.
	if err := noKey.Healthz(ctx); err != nil {
		t.Errorf("Healthz without key: %v", err)
	}
}
