//go:build integration

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// testDB is initialized once in TestMain. Tests call checkDB(t) to skip when
// the database is not available.
var testDB *DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("KAIROS_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable"
	}

	db, err := Connect(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "store tests: database not available (%v) — tests will be skipped\n", err)
	} else {
		testDB = db
		defer testDB.Close()
		if err := Migrate(testDB); err != nil {
			fmt.Fprintf(os.Stderr, "store tests: migration failed: %v\n", err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

// checkDB skips the test when the database is not reachable.
func checkDB(t *testing.T) {
	t.Helper()
	if testDB == nil {
		t.Skip("database not available")
	}
}

// clear removes all rows from the schedules table before each test.
func clear(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(context.Background(), "DELETE FROM schedules")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
}

func newStore() *ScheduleStore { return NewScheduleStore(testDB) }

// future/past helpers for next_run_at values.
func future() time.Time { return time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond) }
func past() time.Time   { return time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond) }
func now() time.Time    { return time.Now().UTC().Truncate(time.Microsecond) }

// makeSchedule builds a minimal valid schedule for inserting in tests.
func makeSchedule(id, name string, nextRun time.Time) *Schedule {
	t := now()
	return &Schedule{
		ID:        id,
		Name:      name,
		CronExpr:  "* * * * *",
		Payload:   json.RawMessage(`{}`),
		Status:    StatusActive,
		NextRunAt: &nextRun,
		CreatedAt: t,
		UpdatedAt: t,
	}
}

// insert is a test helper that calls Insert and fatals on error.
func insert(t *testing.T, s *ScheduleStore, sch *Schedule) {
	t.Helper()
	if err := s.Insert(context.Background(), sch); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

// approxNow asserts that ts is within 5 seconds of the current time.
func approxNow(t *testing.T, label string, ts time.Time) {
	t.Helper()
	delta := time.Since(ts)
	if delta < 0 {
		delta = -delta
	}
	if delta > 5*time.Second {
		t.Errorf("%s: expected approximately now, got %v (delta %v)", label, ts, delta)
	}
}

// --- BeginTx ---

func TestBeginTx_CommitPersistsChanges(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	txCtx, commit, rollback, err := s.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer rollback() //nolint:errcheck

	if err := s.Insert(txCtx, makeSchedule("tx-commit", "test", future())); err != nil {
		t.Fatalf("Insert in tx: %v", err)
	}
	if err := commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := s.Get(context.Background(), "tx-commit")
	if err != nil {
		t.Fatalf("Get after commit: %v", err)
	}
	if got.ID != "tx-commit" {
		t.Errorf("expected id tx-commit, got %q", got.ID)
	}
}

func TestBeginTx_RollbackDiscardsChanges(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	txCtx, _, rollback, err := s.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	if err := s.Insert(txCtx, makeSchedule("tx-rollback", "test", future())); err != nil {
		t.Fatalf("Insert in tx: %v", err)
	}
	if err := rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	_, err = s.Get(context.Background(), "tx-rollback")
	if err == nil {
		t.Error("expected schedule not found after rollback, got nil error")
	}
	if err.Error() != "schedule not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBeginTx_NestedJoinsOuterTransaction(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	outerCtx, _, rollback, err := s.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("outer BeginTx: %v", err)
	}
	defer rollback() //nolint:errcheck

	if err := s.Insert(outerCtx, makeSchedule("outer-id", "test", future())); err != nil {
		t.Fatalf("Insert in outer tx: %v", err)
	}

	// Inner BeginTx should join the outer transaction, not start a new one.
	innerCtx, innerCommit, innerRollback, err := s.BeginTx(outerCtx)
	if err != nil {
		t.Fatalf("inner BeginTx: %v", err)
	}
	if innerCtx != outerCtx {
		t.Error("expected inner BeginTx to return the same context when joining outer tx")
	}
	if err := s.Insert(innerCtx, makeSchedule("inner-id", "test", future())); err != nil {
		t.Fatalf("Insert in inner tx: %v", err)
	}
	// Inner commit/rollback are no-ops; outer transaction still controls the boundary.
	if err := innerCommit(); err != nil {
		t.Fatalf("inner commit: %v", err)
	}
	if err := innerRollback(); err != nil {
		t.Fatalf("inner rollback: %v", err)
	}

	// Roll back the outer transaction — both inserts must be discarded.
	if err := rollback(); err != nil {
		t.Fatalf("outer rollback: %v", err)
	}

	for _, id := range []string{"outer-id", "inner-id"} {
		_, err := s.Get(context.Background(), id)
		if err == nil {
			t.Errorf("expected %q not found after outer rollback", id)
		}
	}
}

func TestBeginTx_RollbackAfterCommitIsNoop(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	_, commit, rollback, err := s.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := rollback(); err != nil {
		t.Errorf("rollback after commit should be a no-op, got: %v", err)
	}
}

// --- Migrate ---

func TestMigrate_Idempotent(t *testing.T) {
	checkDB(t)
	// Running Migrate twice must not error (CREATE TABLE IF NOT EXISTS).
	if err := Migrate(testDB); err != nil {
		t.Fatalf("second Migrate call failed: %v", err)
	}
}

// --- Insert ---

func TestInsert_RoundTripsAllFields(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	payload := json.RawMessage(`{"key":"value","n":42}`)
	maxEx := int64(10)
	iv := int64(120)
	nr := future()
	created := now()

	sch := &Schedule{
		ID:             "full-id",
		Name:           "full name",
		Description:    "a description",
		CronExpr:       "0 12 * * *",
		IntervalSec:    &iv,
		Payload:        payload,
		Status:         StatusActive,
		MaxExecutions:  &maxEx,
		ExecutionCount: 7, // ignored by Insert — always stored as 0
		NextRunAt:      &nr,
		CreatedAt:      created,
		UpdatedAt:      created,
	}
	insert(t, s, sch)

	got, err := s.Get(context.Background(), "full-id")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != "full-id" {
		t.Errorf("ID: want %q, got %q", "full-id", got.ID)
	}
	if got.Name != "full name" {
		t.Errorf("Name: want %q, got %q", "full name", got.Name)
	}
	if got.Description != "a description" {
		t.Errorf("Description: want %q, got %q", "a description", got.Description)
	}
	if got.CronExpr != "0 12 * * *" {
		t.Errorf("CronExpr: want %q, got %q", "0 12 * * *", got.CronExpr)
	}
	if got.IntervalSec == nil || *got.IntervalSec != 120 {
		t.Errorf("IntervalSec: want 120, got %v", got.IntervalSec)
	}
	equalJSON(t, "Payload", `{"key":"value","n":42}`, string(got.Payload))
	if got.Status != StatusActive {
		t.Errorf("Status: want %q, got %q", StatusActive, got.Status)
	}
	if got.MaxExecutions == nil || *got.MaxExecutions != 10 {
		t.Errorf("MaxExecutions: want 10, got %v", got.MaxExecutions)
	}
	if got.ExecutionCount != 0 {
		t.Errorf("ExecutionCount: Insert always stores 0, got %d", got.ExecutionCount)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(nr) {
		t.Errorf("NextRunAt: want %v, got %v", nr, got.NextRunAt)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt: want %v, got %v", created, got.CreatedAt)
	}
	if !got.UpdatedAt.Equal(created) {
		t.Errorf("UpdatedAt: want %v, got %v", created, got.UpdatedAt)
	}
}

func TestInsert_NullableFieldsAreNil(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	sch := makeSchedule("nil-fields", "no optionals", future())
	// IntervalSec and MaxExecutions left nil
	insert(t, s, sch)

	got, err := s.Get(context.Background(), "nil-fields")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.IntervalSec != nil {
		t.Errorf("IntervalSec: want nil, got %v", got.IntervalSec)
	}
	if got.MaxExecutions != nil {
		t.Errorf("MaxExecutions: want nil, got %v", got.MaxExecutions)
	}
}

func TestInsert_ExecutionCountAlwaysZero(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	sch := makeSchedule("count-test", "test", future())
	sch.ExecutionCount = 99
	insert(t, s, sch)

	got, _ := s.Get(context.Background(), "count-test")
	if got.ExecutionCount != 0 {
		t.Errorf("expected execution_count=0 after Insert regardless of input, got %d", got.ExecutionCount)
	}
}

func TestInsert_DuplicateIDFails(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	sch := makeSchedule("dup-id", "first", future())
	insert(t, s, sch)

	err := s.Insert(context.Background(), makeSchedule("dup-id", "second", future()))
	if err == nil {
		t.Error("expected duplicate primary key error, got nil")
	}
}

func TestInsert_EmptyPayloadDefaultsToObject(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	sch := makeSchedule("empty-payload", "test", future())
	sch.Payload = json.RawMessage(`{}`)
	insert(t, s, sch)

	got, _ := s.Get(context.Background(), "empty-payload")
	if string(got.Payload) != `{}` {
		t.Errorf("Payload: want {}, got %s", got.Payload)
	}
}

// --- Get ---

func TestGet_NotFound(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	_, err := s.Get(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing schedule, got nil")
	}
	if err.Error() != "schedule not found" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGet_ReturnsDeletedSchedule(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	sch := makeSchedule("soft-del", "test", future())
	insert(t, s, sch)
	s.Delete(context.Background(), "soft-del")

	// Get must not filter by status (CLAUDE.md: "Get does not filter by status")
	got, err := s.Get(context.Background(), "soft-del")
	if err != nil {
		t.Fatalf("Get on deleted schedule: %v", err)
	}
	if got.Status != StatusDeleted {
		t.Errorf("expected deleted status, got %q", got.Status)
	}
}

// --- List ---

func TestList_Empty(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	result, err := s.List(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 0 {
		t.Errorf("expected empty list, got %d items", len(result.Schedules))
	}
	if result.Total != 0 {
		t.Errorf("expected total=0, got %d", result.Total)
	}
}

func TestList_ExcludesDeletedOnly(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	// Insert schedules with every status.
	active := makeSchedule("a", "active", future())
	paused := makeSchedule("b", "paused", future())
	paused.Status = StatusPaused
	completed := makeSchedule("c", "completed", future())
	completed.Status = StatusCompleted
	deleted := makeSchedule("d", "deleted", future())
	deleted.Status = StatusDeleted

	for _, sch := range []*Schedule{active, paused, completed, deleted} {
		insert(t, s, sch)
	}

	result, err := s.List(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 3 {
		t.Errorf("expected 3 non-deleted schedules, got %d", len(result.Schedules))
	}
	for _, g := range result.Schedules {
		if g.Status == StatusDeleted {
			t.Errorf("List returned deleted schedule %q", g.ID)
		}
	}
}

func TestList_OrderedByNextRunAtAsc(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	base := now()
	for i := 3; i >= 1; i-- {
		sch := makeSchedule(fmt.Sprintf("ord-%d", i), fmt.Sprintf("schedule %d", i), base.Add(time.Duration(i)*time.Hour))
		insert(t, s, sch)
	}

	result, err := s.List(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := result.Schedules
	if len(got) != 3 {
		t.Fatalf("expected 3 schedules, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].NextRunAt == nil || got[i-1].NextRunAt == nil || got[i].NextRunAt.Before(*got[i-1].NextRunAt) {
			t.Errorf("List is not ordered by next_run_at: index %d (%v) is before index %d (%v)",
				i, got[i].NextRunAt, i-1, got[i-1].NextRunAt)
		}
	}
}

// --- List (filters) ---

func TestList_FilterByNameExact(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	insert(t, s, makeSchedule("a", "nightly-backup", future()))
	insert(t, s, makeSchedule("b", "daily-report", future()))

	name := "nightly-backup"
	result, err := s.List(context.Background(), ListFilter{Name: &name})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 1 || result.Schedules[0].ID != "a" {
		t.Errorf("expected only 'a', got %v", ids(result.Schedules))
	}
}

func TestList_FilterByNameWildcard(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	insert(t, s, makeSchedule("a", "nightly-backup", future()))
	insert(t, s, makeSchedule("b", "nightly-report", future()))
	insert(t, s, makeSchedule("c", "daily-sync", future()))

	name := "nightly*"
	result, err := s.List(context.Background(), ListFilter{Name: &name})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 2 {
		t.Errorf("expected 2 results for nightly*, got %d: %v", len(result.Schedules), ids(result.Schedules))
	}
}

func TestList_FilterByStatus(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	active := makeSchedule("a", "active", future())
	paused := makeSchedule("b", "paused", future())
	paused.Status = StatusPaused
	completed := makeSchedule("c", "completed", future())
	completed.Status = StatusCompleted

	for _, sch := range []*Schedule{active, paused, completed} {
		insert(t, s, sch)
	}

	result, err := s.List(context.Background(), ListFilter{Statuses: []Status{StatusActive, StatusPaused}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 2 {
		t.Errorf("expected 2 results, got %d: %v", len(result.Schedules), ids(result.Schedules))
	}
	for _, g := range result.Schedules {
		if g.Status != StatusActive && g.Status != StatusPaused {
			t.Errorf("unexpected status %q in result", g.Status)
		}
	}
}

func TestList_FilterByStatusIncludesDeleted(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	del := makeSchedule("d", "deleted", future())
	del.Status = StatusDeleted
	insert(t, s, del)

	result, err := s.List(context.Background(), ListFilter{Statuses: []Status{StatusDeleted}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 1 || result.Schedules[0].ID != "d" {
		t.Errorf("expected deleted row when status filter includes deleted, got %v", ids(result.Schedules))
	}
}

func TestList_FilterByExecutionCount(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	low := makeSchedule("low", "low", future())
	mid := makeSchedule("mid", "mid", future())
	high := makeSchedule("high", "high", future())

	for _, sch := range []*Schedule{low, mid, high} {
		insert(t, s, sch)
	}
	// advance counts: low=1, mid=5, high=10
	s.SetNextRun(context.Background(), "low", future(), 1)
	s.SetNextRun(context.Background(), "mid", future(), 5)
	s.SetNextRun(context.Background(), "high", future(), 10)

	result, err := s.List(context.Background(), ListFilter{
		ExecutionCount: []Condition{
			{Op: ">", Value: int64(1)},
			{Op: "<", Value: int64(10)},
		},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 1 || result.Schedules[0].ID != "mid" {
		t.Errorf("expected only 'mid', got %v", ids(result.Schedules))
	}
}

func TestList_FilterByNextRunAt(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	base := time.Now().UTC()
	near := makeSchedule("near", "near", base.Add(time.Hour).Truncate(time.Microsecond))
	far := makeSchedule("far", "far", base.Add(48*time.Hour).Truncate(time.Microsecond))

	for _, sch := range []*Schedule{near, far} {
		insert(t, s, sch)
	}

	cutoff := base.Add(24 * time.Hour)
	result, err := s.List(context.Background(), ListFilter{
		NextRunAt: []Condition{{Op: "<", Value: cutoff}},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 1 || result.Schedules[0].ID != "near" {
		t.Errorf("expected only 'near', got %v", ids(result.Schedules))
	}
}

func TestList_FilterByMaxExecutions(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	maxEx := int64(5)
	capped := makeSchedule("capped", "capped", future())
	capped.MaxExecutions = &maxEx
	unlimited := makeSchedule("unlimited", "unlimited", future())

	insert(t, s, capped)
	insert(t, s, unlimited)

	result, err := s.List(context.Background(), ListFilter{MaxExecutions: &maxEx})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Schedules) != 1 || result.Schedules[0].ID != "capped" {
		t.Errorf("expected only 'capped', got %v", ids(result.Schedules))
	}
}

func TestList_Pagination(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	for i := 1; i <= 5; i++ {
		insert(t, s, makeSchedule(fmt.Sprintf("p-%d", i), fmt.Sprintf("schedule %d", i), future()))
	}

	// Page 1 of 2 with limit 3 — should return 3 rows, total=5.
	result, err := s.List(context.Background(), ListFilter{Page: 1, Limit: 3})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(result.Schedules) != 3 {
		t.Errorf("page 1: expected 3 schedules, got %d", len(result.Schedules))
	}
	if result.Total != 5 {
		t.Errorf("page 1: expected total=5, got %d", result.Total)
	}

	// Page 2 — should return 2 rows, total=5.
	result2, err := s.List(context.Background(), ListFilter{Page: 2, Limit: 3})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(result2.Schedules) != 2 {
		t.Errorf("page 2: expected 2 schedules, got %d", len(result2.Schedules))
	}
	if result2.Total != 5 {
		t.Errorf("page 2: expected total=5, got %d", result2.Total)
	}

	// Page beyond results — should return 0 rows, total=0 (no rows to read count from).
	result3, err := s.List(context.Background(), ListFilter{Page: 10, Limit: 3})
	if err != nil {
		t.Fatalf("List page 10: %v", err)
	}
	if len(result3.Schedules) != 0 {
		t.Errorf("page 10: expected 0 schedules, got %d", len(result3.Schedules))
	}
}

// --- Save ---

func TestSave_UpdatesAllMutableFields(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	sch := makeSchedule("save-id", "original", future())
	insert(t, s, sch)

	maxEx := int64(5)
	iv := int64(300)
	newNext := future().Add(time.Hour)
	updated := &Schedule{
		ID:            "save-id",
		Name:          "updated name",
		Description:   "updated desc",
		CronExpr:      "0 6 * * *",
		IntervalSec:   &iv,
		Payload:       json.RawMessage(`{"updated":true}`),
		Status:        StatusPaused,
		MaxExecutions: &maxEx,
		NextRunAt:     &newNext,
		UpdatedAt:     now(),
	}

	if err := s.Save(context.Background(), updated); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, _ := s.Get(context.Background(), "save-id")
	if got.Name != "updated name" {
		t.Errorf("Name: want %q, got %q", "updated name", got.Name)
	}
	if got.Description != "updated desc" {
		t.Errorf("Description: want %q, got %q", "updated desc", got.Description)
	}
	if got.CronExpr != "0 6 * * *" {
		t.Errorf("CronExpr: want %q, got %q", "0 6 * * *", got.CronExpr)
	}
	if got.IntervalSec == nil || *got.IntervalSec != 300 {
		t.Errorf("IntervalSec: want 300, got %v", got.IntervalSec)
	}
	equalJSON(t, "Payload", `{"updated":true}`, string(got.Payload))
	if got.Status != StatusPaused {
		t.Errorf("Status: want paused, got %q", got.Status)
	}
	if got.MaxExecutions == nil || *got.MaxExecutions != 5 {
		t.Errorf("MaxExecutions: want 5, got %v", got.MaxExecutions)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(newNext) {
		t.Errorf("NextRunAt: want %v, got %v", newNext, got.NextRunAt)
	}
	if !got.UpdatedAt.Equal(updated.UpdatedAt) {
		t.Errorf("UpdatedAt: want %v, got %v", updated.UpdatedAt, got.UpdatedAt)
	}
}

func TestSave_ClearsNullableFieldsWhenNil(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	iv := int64(60)
	maxEx := int64(3)
	sch := makeSchedule("nullable-id", "test", future())
	sch.IntervalSec = &iv
	sch.MaxExecutions = &maxEx
	insert(t, s, sch)

	// Save with nil optionals — should clear them.
	sch.IntervalSec = nil
	sch.MaxExecutions = nil
	sch.UpdatedAt = now()
	if err := s.Save(context.Background(), sch); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, _ := s.Get(context.Background(), "nullable-id")
	if got.IntervalSec != nil {
		t.Errorf("IntervalSec: expected nil after Save, got %v", got.IntervalSec)
	}
	if got.MaxExecutions != nil {
		t.Errorf("MaxExecutions: expected nil after Save, got %v", got.MaxExecutions)
	}
}

func TestSave_NonExistentIDIsNoOp(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	err := s.Save(context.Background(), &Schedule{
		ID:        "ghost",
		Name:      "ghost",
		UpdatedAt: now(),
	})
	if err != nil {
		t.Errorf("Save on non-existent ID should not error, got %v", err)
	}
}

// --- Delete ---

func TestDelete_SetsStatusDeletedAndUpdatedAt(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	insert(t, s, makeSchedule("del-id", "test", future()))

	beforeDelete := time.Now().UTC()
	if err := s.Delete(context.Background(), "del-id"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := s.Get(context.Background(), "del-id")
	if got.Status != StatusDeleted {
		t.Errorf("expected status deleted, got %q", got.Status)
	}
	if got.NextRunAt != nil {
		t.Errorf("expected next_run_at=NULL after Delete, got %v", got.NextRunAt)
	}
	if got.UpdatedAt.Before(beforeDelete.Add(-time.Second)) {
		t.Errorf("UpdatedAt not refreshed: got %v, delete happened after %v", got.UpdatedAt, beforeDelete)
	}
}

func TestDelete_NonExistentIDIsNoOp(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	if err := s.Delete(context.Background(), "ghost"); err != nil {
		t.Errorf("Delete on non-existent ID should not error, got %v", err)
	}
}

func TestDelete_DoesNotAffectOtherRows(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	insert(t, s, makeSchedule("keep", "keep me", future()))
	insert(t, s, makeSchedule("remove", "remove me", future()))

	s.Delete(context.Background(), "remove")

	got, err := s.Get(context.Background(), "keep")
	if err != nil {
		t.Fatalf("Get after Delete: %v", err)
	}
	if got.Status == StatusDeleted {
		t.Error("Delete affected the wrong row")
	}
}

// --- DueSchedules ---

func TestDueSchedules_ReturnsPastActiveSchedules(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	insert(t, s, makeSchedule("due", "past", past()))
	insert(t, s, makeSchedule("not-due", "future", future()))

	got, err := s.DueSchedules(context.Background(), 100)
	if err != nil {
		t.Fatalf("DueSchedules: %v", err)
	}
	if len(got) != 1 || got[0].ID != "due" {
		t.Errorf("expected only 'due', got %v", ids(got))
	}
}

func TestDueSchedules_ExcludesNonActiveStatuses(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	paused := makeSchedule("paused", "paused", past())
	paused.Status = StatusPaused
	completed := makeSchedule("completed", "completed", past())
	completed.Status = StatusCompleted
	deleted := makeSchedule("deleted", "deleted", past())
	deleted.Status = StatusDeleted

	for _, sch := range []*Schedule{paused, completed, deleted} {
		insert(t, s, sch)
	}

	got, err := s.DueSchedules(context.Background(), 100)
	if err != nil {
		t.Fatalf("DueSchedules: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no due schedules for non-active statuses, got %v", ids(got))
	}
}

func TestDueSchedules_RespectsLimit(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	for i := 0; i < 5; i++ {
		insert(t, s, makeSchedule(fmt.Sprintf("due-%d", i), fmt.Sprintf("s%d", i), past()))
	}

	got, err := s.DueSchedules(context.Background(), 3)
	if err != nil {
		t.Fatalf("DueSchedules: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(got))
	}
}

func TestDueSchedules_OrderedByNextRunAtAsc(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	base := now().Add(-10 * time.Minute)
	for i := 5; i >= 1; i-- {
		sch := makeSchedule(fmt.Sprintf("due-ord-%d", i), fmt.Sprintf("s%d", i), base.Add(time.Duration(i)*time.Minute))
		insert(t, s, sch)
	}

	got, err := s.DueSchedules(context.Background(), 100)
	if err != nil {
		t.Fatalf("DueSchedules: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i].NextRunAt == nil || got[i-1].NextRunAt == nil || got[i].NextRunAt.Before(*got[i-1].NextRunAt) {
			t.Errorf("DueSchedules not ordered: index %d (%v) before index %d (%v)",
				i, got[i].NextRunAt, i-1, got[i-1].NextRunAt)
		}
	}
}

func TestDueSchedules_Empty(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	got, err := s.DueSchedules(context.Background(), 100)
	if err != nil {
		t.Fatalf("DueSchedules: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

func TestDueSchedules_ExcludesFutureSchedules(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	// Only future schedules.
	for i := 0; i < 3; i++ {
		insert(t, s, makeSchedule(fmt.Sprintf("fut-%d", i), "future", future()))
	}

	got, err := s.DueSchedules(context.Background(), 100)
	if err != nil {
		t.Fatalf("DueSchedules: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no due schedules for future next_run_at, got %d", len(got))
	}
}

// --- Complete ---

func TestComplete_SetsStatusAndCount(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	insert(t, s, makeSchedule("comp-id", "test", past()))

	beforeComplete := time.Now().UTC()
	if err := s.Complete(context.Background(), "comp-id", 7); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, _ := s.Get(context.Background(), "comp-id")
	if got.Status != StatusCompleted {
		t.Errorf("expected status completed, got %q", got.Status)
	}
	if got.NextRunAt != nil {
		t.Errorf("expected next_run_at=NULL after Complete, got %v", got.NextRunAt)
	}
	if got.ExecutionCount != 7 {
		t.Errorf("expected execution_count 7, got %d", got.ExecutionCount)
	}
	approxNow(t, "UpdatedAt", got.UpdatedAt)
	if got.UpdatedAt.Before(beforeComplete.Add(-time.Second)) {
		t.Errorf("UpdatedAt not refreshed after Complete")
	}
}

func TestComplete_NonExistentIDIsNoOp(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	if err := s.Complete(context.Background(), "ghost", 1); err != nil {
		t.Errorf("Complete on non-existent ID should not error, got %v", err)
	}
}

func TestComplete_DoesNotAffectOtherRows(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	insert(t, s, makeSchedule("keep", "keep me", past()))
	insert(t, s, makeSchedule("complete-me", "complete me", past()))

	s.Complete(context.Background(), "complete-me", 1)

	got, _ := s.Get(context.Background(), "keep")
	if got.Status == StatusCompleted {
		t.Error("Complete affected the wrong row")
	}
}

// --- SetNextRun ---

func TestSetNextRun_UpdatesNextRunAtAndCount(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	insert(t, s, makeSchedule("next-id", "test", past()))

	newNext := future()
	beforeSet := time.Now().UTC()
	if err := s.SetNextRun(context.Background(), "next-id", newNext, 3); err != nil {
		t.Fatalf("SetNextRun: %v", err)
	}

	got, _ := s.Get(context.Background(), "next-id")
	if got.NextRunAt == nil || !got.NextRunAt.Equal(newNext) {
		t.Errorf("NextRunAt: want %v, got %v", newNext, got.NextRunAt)
	}
	if got.ExecutionCount != 3 {
		t.Errorf("ExecutionCount: want 3, got %d", got.ExecutionCount)
	}
	if got.UpdatedAt.Before(beforeSet.Add(-time.Second)) {
		t.Errorf("UpdatedAt not refreshed after SetNextRun")
	}
}

func TestSetNextRun_NonExistentIDIsNoOp(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	if err := s.SetNextRun(context.Background(), "ghost", future(), 1); err != nil {
		t.Errorf("SetNextRun on non-existent ID should not error, got %v", err)
	}
}

func TestSetNextRun_DoesNotAffectOtherRows(t *testing.T) {
	checkDB(t)
	clear(t)
	s := newStore()

	origNext := future()
	insert(t, s, makeSchedule("keep", "keep me", origNext))
	insert(t, s, makeSchedule("advance-me", "advance me", past()))

	s.SetNextRun(context.Background(), "advance-me", future(), 1)

	got, _ := s.Get(context.Background(), "keep")
	if !got.NextRunAt.Equal(origNext) {
		t.Errorf("SetNextRun affected the wrong row: NextRunAt changed from %v to %v", origNext, got.NextRunAt)
	}
}

// equalJSON asserts that two JSON strings are semantically equal. Postgres JSONB
// normalises key order and whitespace, so raw-string comparison is unreliable.
func equalJSON(t *testing.T, label, want, got string) {
	t.Helper()
	var wantV, gotV any
	if err := json.Unmarshal([]byte(want), &wantV); err != nil {
		t.Fatalf("%s: unmarshal want: %v", label, err)
	}
	if err := json.Unmarshal([]byte(got), &gotV); err != nil {
		t.Fatalf("%s: unmarshal got: %v", label, err)
	}
	wb, _ := json.Marshal(wantV)
	gb, _ := json.Marshal(gotV)
	if string(wb) != string(gb) {
		t.Errorf("%s: want %s, got %s", label, want, got)
	}
}

// --- ids is a debug helper for test error messages ---

func ids(schedules []*Schedule) []string {
	out := make([]string, len(schedules))
	for i, s := range schedules {
		out[i] = s.ID
	}
	return out
}
