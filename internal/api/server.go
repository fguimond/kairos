package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/francisguimond/kairos/internal/store"
)

type scheduleService interface {
	List(ctx context.Context, f store.ListFilter) (*store.ListResult, error)
	Create(ctx context.Context, in store.CreateScheduleInput) (*store.Schedule, error)
	Get(ctx context.Context, id string) (*store.Schedule, error)
	Update(ctx context.Context, id string, in store.UpdateScheduleInput) (*store.Schedule, error)
	Delete(ctx context.Context, id string) error
}

type listResponse struct {
	Data  []*store.Schedule `json:"data"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
	Total int64             `json:"total"`
}

type Server struct {
	srv *http.Server
	svc scheduleService
}

func NewServer(s scheduleService, addr string, apiKey string) *Server {
	srv := &Server{svc: s}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.handleHealthz)
	mux.HandleFunc("GET /api/v1/openapi.yaml", srv.handleOpenAPISpec)
	mux.HandleFunc("GET /docs", srv.handleDocs)
	mux.HandleFunc("GET /api/v1/schedules", srv.handleList)
	mux.HandleFunc("POST /api/v1/schedules", srv.handleCreate)
	mux.HandleFunc("GET /api/v1/schedules/{id}", srv.handleGet)
	mux.HandleFunc("PATCH /api/v1/schedules/{id}", srv.handleUpdate)
	mux.HandleFunc("DELETE /api/v1/schedules/{id}", srv.handleDelete)

	var h http.Handler = mux
	h = loggingMiddleware(h)
	h = recoveryMiddleware(h)
	if apiKey != "" {
		h = apiKeyMiddleware(apiKey)(h)
	}

	srv.srv = &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv
}

func (s *Server) Start() error {
	if err := s.srv.ListenAndServe(); err != nil {
		return fmt.Errorf("http listen: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}

// --- handlers ---

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	f, err := parseListFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := result.Schedules
	if data == nil {
		data = []*store.Schedule{}
	}
	writeJSON(w, http.StatusOK, listResponse{
		Data:  data,
		Page:  f.Page,
		Limit: f.Limit,
		Total: result.Total,
	})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var in store.CreateScheduleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if in.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "name is required")
		return
	}
	if in.CronExpr == "" && in.IntervalSec == nil {
		writeError(w, http.StatusUnprocessableEntity, "cron_expr or interval_sec is required")
		return
	}
	if in.MaxExecutions != nil && *in.MaxExecutions <= 0 {
		writeError(w, http.StatusUnprocessableEntity, "max_executions must be greater than 0")
		return
	}
	if in.Timezone != "" {
		if _, err := time.LoadLocation(in.Timezone); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid timezone: "+err.Error())
			return
		}
	}

	sch, err := s.svc.Create(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sch)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sch, err := s.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sch)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in store.UpdateScheduleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if in.MaxExecutions != nil && *in.MaxExecutions <= 0 {
		writeError(w, http.StatusUnprocessableEntity, "max_executions must be greater than 0")
		return
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid timezone: "+err.Error())
			return
		}
	}
	sch, err := s.svc.Update(r.Context(), id, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sch)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *statusResponseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func apiKeyMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(token), []byte(key)) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "panic", rec, "stack", string(debug.Stack()))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// --- filter parsing ---

func parseListFilter(r *http.Request) (store.ListFilter, error) {
	q := r.URL.Query()
	var f store.ListFilter

	if name := q.Get("name"); name != "" {
		f.Name = &name
	}

	for _, s := range q["status"] {
		f.Statuses = append(f.Statuses, store.Status(s))
	}

	if raw := q.Get("max_executions"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return f, fmt.Errorf("invalid max_executions: %w", err)
		}
		f.MaxExecutions = &v
	}

	var err error
	if f.ExecutionCount, err = parseConditions(q["execution_count"], parseIntVal); err != nil {
		return f, fmt.Errorf("invalid execution_count: %w", err)
	}
	if f.CreatedAt, err = parseConditions(q["created_at"], parseTimeVal); err != nil {
		return f, fmt.Errorf("invalid created_at: %w", err)
	}
	if f.UpdatedAt, err = parseConditions(q["updated_at"], parseTimeVal); err != nil {
		return f, fmt.Errorf("invalid updated_at: %w", err)
	}
	if f.NextRunAt, err = parseConditions(q["next_run_at"], parseTimeVal); err != nil {
		return f, fmt.Errorf("invalid next_run_at: %w", err)
	}

	if raw := q.Get("sort"); raw != "" {
		sort, err := parseSortParam(raw)
		if err != nil {
			return f, err
		}
		f.Sort = sort
	}

	f.Page = 1
	if raw := q.Get("page"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return f, fmt.Errorf("invalid page: must be an integer >= 1")
		}
		f.Page = v
	}

	f.Limit = 50
	if raw := q.Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return f, fmt.Errorf("invalid limit: must be an integer >= 1")
		}
		if v > 200 {
			return f, fmt.Errorf("invalid limit: must be <= 200")
		}
		f.Limit = v
	}

	return f, nil
}

func parseConditions(vals []string, parseVal func(string) (any, error)) ([]store.Condition, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	out := make([]store.Condition, 0, len(vals))
	for _, v := range vals {
		opStr, raw, ok := strings.Cut(v, ":")
		if !ok {
			return nil, fmt.Errorf("expected op:value format, got %q", v)
		}
		sqlOp, err := parseOp(opStr)
		if err != nil {
			return nil, err
		}
		val, err := parseVal(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q: %w", raw, err)
		}
		out = append(out, store.Condition{Op: sqlOp, Value: val})
	}
	return out, nil
}

func parseOp(op string) (string, error) {
	switch op {
	case "eq":
		return "=", nil
	case "lt":
		return "<", nil
	case "lte":
		return "<=", nil
	case "gt":
		return ">", nil
	case "gte":
		return ">=", nil
	default:
		return "", fmt.Errorf("unknown operator %q, expected one of: eq, lt, lte, gt, gte", op)
	}
}

func parseIntVal(s string) (any, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	return v, nil
}

func parseTimeVal(s string) (any, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	return t, nil
}

var sortableFields = map[string]struct{}{
	"name": {}, "description": {}, "cron_expr": {}, "interval_sec": {},
	"timezone": {}, "status": {}, "max_executions": {}, "execution_count": {},
	"next_run_at": {}, "created_at": {}, "updated_at": {},
}

func parseSortParam(raw string) ([]store.SortOrder, error) {
	fields := strings.Split(raw, ",")
	out := make([]store.SortOrder, 0, len(fields))
	for _, f := range fields {
		col, dir, ok := strings.Cut(f, ":")
		if !ok {
			return nil, fmt.Errorf("invalid sort field %q: expected field:direction", f)
		}
		if _, valid := sortableFields[col]; !valid {
			return nil, fmt.Errorf("unknown sort field %q", col)
		}
		switch strings.ToLower(dir) {
		case "asc":
			out = append(out, store.SortOrder{Field: col, Dir: "ASC"})
		case "desc":
			out = append(out, store.SortOrder{Field: col, Dir: "DESC"})
		default:
			return nil, fmt.Errorf("invalid sort direction %q: expected asc or desc", dir)
		}
	}
	return out, nil
}
