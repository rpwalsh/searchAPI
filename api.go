package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type API struct {
	cfg    Config
	store  *Store
	logger *slog.Logger
}

func NewRouter(store *Store, cfg Config, logger *slog.Logger) http.Handler {
	api := &API{cfg: cfg, store: store, logger: logger}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("GET /readyz", api.ready)
	mux.HandleFunc("GET /metrics", api.metrics)

	mux.HandleFunc("GET /api/v1/assets", api.listAssets)
	mux.HandleFunc("POST /api/v1/assets", api.createAsset)
	mux.HandleFunc("GET /api/v1/assets/{id}", api.getAsset)

	mux.HandleFunc("GET /api/v1/procedures", api.listProcedures)
	mux.HandleFunc("POST /api/v1/procedures", api.createProcedure)

	mux.HandleFunc("GET /api/v1/test-runs", api.listTestRuns)
	mux.HandleFunc("POST /api/v1/test-runs", api.createTestRun)

	mux.HandleFunc("GET /api/v1/channels", api.listChannels)
	mux.HandleFunc("POST /api/v1/channels", api.createChannel)

	mux.HandleFunc("POST /api/v1/ingest/stream", api.ingestStream)
	mux.HandleFunc("GET /api/v1/alarms", api.listAlarms)

	mux.HandleFunc("GET /api/v1/annotations", api.listAnnotations)
	mux.HandleFunc("POST /api/v1/annotations", api.createAnnotation)

	mux.HandleFunc("GET /api/v1/events", api.listEvents)
	mux.HandleFunc("GET /api/v1/search", api.search)
	mux.HandleFunc("POST /api/v1/onboarding/seed", api.seed)
	registerUI(mux, api)

	return api.recoverPanic(api.observe(api.authenticate(securityHeaders(mux))))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (a *API) authenticate(next http.Handler) http.Handler {
	if a.cfg.APIKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if token == "" {
			token = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		}
		if token != a.cfg.APIKey {
			writeError(w, http.StatusUnauthorized, "valid API key required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPublicPath(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.URL.Path == "/" || r.URL.Path == "/ui" || strings.HasPrefix(r.URL.Path, "/ui/") {
		return true
	}
	if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/static/")
}

func (a *API) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		a.store.metrics.Requests.Add(1)
		next.ServeHTTP(recorder, r)
		a.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func (a *API) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.Error("panic recovered", "panic", recovered)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := a.store.db.PingContext(r.Context()); err != nil {
		dbStatus = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"environment":   a.cfg.Environment,
		"site_id":       a.cfg.SiteID,
		"database":      dbStatus,
		"elasticsearch": a.store.ElasticEnabled(),
	})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.store.db.PingContext(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) metrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.store.metrics.Snapshot())
}

func (a *API) createAsset(w http.ResponseWriter, r *http.Request) {
	asset, ok := decodeJSON[Asset](w, r, a.cfg.IngestMaxBytes)
	if !ok {
		return
	}
	created, err := a.store.CreateAsset(r.Context(), asset, actor(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) listAssets(w http.ResponseWriter, r *http.Request) {
	assets, err := a.store.ListAssets(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, assets)
}

func (a *API) getAsset(w http.ResponseWriter, r *http.Request) {
	asset, err := a.store.GetAsset(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

func (a *API) createProcedure(w http.ResponseWriter, r *http.Request) {
	procedure, ok := decodeJSON[Procedure](w, r, a.cfg.IngestMaxBytes)
	if !ok {
		return
	}
	created, err := a.store.CreateProcedure(r.Context(), procedure, actor(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) listProcedures(w http.ResponseWriter, r *http.Request) {
	procedures, err := a.store.ListProcedures(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, procedures)
}

func (a *API) createTestRun(w http.ResponseWriter, r *http.Request) {
	run, ok := decodeJSON[TestRun](w, r, a.cfg.IngestMaxBytes)
	if !ok {
		return
	}
	created, err := a.store.CreateTestRun(r.Context(), run, actor(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) listTestRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := a.store.ListTestRuns(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (a *API) createChannel(w http.ResponseWriter, r *http.Request) {
	channel, ok := decodeJSON[Channel](w, r, a.cfg.IngestMaxBytes)
	if !ok {
		return
	}
	created, err := a.store.CreateChannel(r.Context(), channel, actor(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) listChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := a.store.ListChannels(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (a *API) ingestStream(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.IngestMaxBytes)
	samples, err := readTelemetrySamples(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.store.IngestTelemetryBatch(r.Context(), samples, actor(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (a *API) listAlarms(w http.ResponseWriter, r *http.Request) {
	alarms, err := a.store.ListAlarms(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alarms)
}

func (a *API) createAnnotation(w http.ResponseWriter, r *http.Request) {
	annotation, ok := decodeJSON[OperatorAnnotation](w, r, a.cfg.IngestMaxBytes)
	if !ok {
		return
	}
	created, err := a.store.CreateAnnotation(r.Context(), annotation, actor(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) listAnnotations(w http.ResponseWriter, r *http.Request) {
	annotations, err := a.store.ListAnnotations(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, annotations)
}

func (a *API) listEvents(w http.ResponseWriter, r *http.Request) {
	events, err := a.store.ListEvents(r.Context(), r.URL.Query().Get("type"), limitParam(r, 100))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (a *API) search(w http.ResponseWriter, r *http.Request) {
	hits, err := a.store.Search(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("kind"), limitParam(r, 25))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hits)
}

func (a *API) seed(w http.ResponseWriter, r *http.Request) {
	summary, err := a.store.SeedDemo(r.Context(), actor(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, summary)
}

func decodeJSON[T any](w http.ResponseWriter, r *http.Request, maxBytes int64) (T, bool) {
	var value T
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return value, false
	}
	return value, true
}

func readTelemetrySamples(body io.Reader) ([]TelemetrySample, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, errors.New("empty telemetry stream")
	}

	var samples []TelemetrySample
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &samples); err != nil {
			return nil, fmt.Errorf("decode telemetry array: %w", err)
		}
	} else if raw[0] == '{' {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		for {
			var sample TelemetrySample
			if err := decoder.Decode(&sample); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, fmt.Errorf("decode telemetry object stream: %w", err)
			}
			samples = append(samples, sample)
		}
	} else {
		return readTelemetryNDJSON(raw)
	}
	if len(samples) == 0 {
		return nil, errors.New("telemetry stream contained no samples")
	}
	return samples, nil
}

func readTelemetryNDJSON(raw []byte) ([]TelemetrySample, error) {
	var samples []TelemetrySample
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024), 32<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var sample TelemetrySample
		if err := json.Unmarshal([]byte(text), &sample); err != nil {
			return nil, fmt.Errorf("decode ndjson line %d: %w", line, err)
		}
		samples = append(samples, sample)
	}
	return samples, scanner.Err()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error":  message,
		"status": status,
	})
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "not found")
	case strings.Contains(strings.ToLower(err.Error()), "duplicate"):
		writeError(w, http.StatusConflict, err.Error())
	case strings.Contains(strings.ToLower(err.Error()), "violates foreign key"):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func actor(r *http.Request) string {
	for _, header := range []string{"X-Operator-ID", "X-Client-ID", "X-Actor"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	return "api"
}

func limitParam(r *http.Request, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
