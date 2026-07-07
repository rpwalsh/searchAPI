package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/lib/pq"
)

type Metrics struct {
	Requests        atomic.Uint64
	AcceptedSamples atomic.Uint64
	RejectedSamples atomic.Uint64
	AlarmsRaised    atomic.Uint64
	AuditEvents     atomic.Uint64
	SearchRequests  atomic.Uint64
}

func (m *Metrics) Snapshot() map[string]uint64 {
	return map[string]uint64{
		"requests":         m.Requests.Load(),
		"accepted_samples": m.AcceptedSamples.Load(),
		"rejected_samples": m.RejectedSamples.Load(),
		"alarms_raised":    m.AlarmsRaised.Load(),
		"audit_events":     m.AuditEvents.Load(),
		"search_requests":  m.SearchRequests.Load(),
	}
}

type Store struct {
	cfg     Config
	db      *sql.DB
	es      *elasticsearch.Client
	logger  *slog.Logger
	metrics *Metrics
}

func NewStore(cfg Config, db *sql.DB, logger *slog.Logger) (*Store, error) {
	var es *elasticsearch.Client
	if len(cfg.ElasticAddresses) > 0 {
		esCfg := elasticsearch.Config{Addresses: cfg.ElasticAddresses}
		if cfg.ElasticAPIKey != "" {
			esCfg.APIKey = cfg.ElasticAPIKey
		}
		client, err := elasticsearch.NewClient(esCfg)
		if err != nil {
			return nil, fmt.Errorf("create elasticsearch client: %w", err)
		}
		es = client
	}

	return &Store{
		cfg:     cfg,
		db:      db,
		es:      es,
		logger:  logger,
		metrics: &Metrics{},
	}, nil
}

func (s *Store) ElasticEnabled() bool {
	return s.es != nil
}

func (s *Store) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS assets (
			id text PRIMARY KEY,
			external_id text,
			name text NOT NULL,
			kind text NOT NULL,
			serial_number text,
			location text,
			tags jsonb NOT NULL DEFAULT '[]'::jsonb,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS procedures (
			id text PRIMARY KEY,
			name text NOT NULL,
			version text NOT NULL,
			steps jsonb NOT NULL DEFAULT '[]'::jsonb,
			requirements jsonb NOT NULL DEFAULT '[]'::jsonb,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS test_runs (
			id text PRIMARY KEY,
			asset_id text NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
			procedure_id text,
			run_number text NOT NULL,
			status text NOT NULL,
			operator text,
			site_id text NOT NULL,
			started_at timestamptz NOT NULL,
			ended_at timestamptz,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS channels (
			id text PRIMARY KEY,
			asset_id text NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
			name text NOT NULL,
			unit text,
			sample_rate_hz double precision,
			data_type text NOT NULL,
			alarm_min double precision,
			alarm_max double precision,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS telemetry_samples (
			id text PRIMARY KEY,
			run_id text NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
			asset_id text NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
			channel_id text NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
			gateway_id text,
			ts timestamptz NOT NULL,
			numeric_value double precision,
			text_value text,
			quality text NOT NULL,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			received_at timestamptz NOT NULL,
			replay_key text UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS alarms (
			id text PRIMARY KEY,
			asset_id text NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
			run_id text NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
			channel_id text NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
			severity text NOT NULL,
			status text NOT NULL,
			message text NOT NULL,
			threshold double precision,
			observed_value double precision,
			raised_at timestamptz NOT NULL,
			cleared_at timestamptz,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb
		)`,
		`CREATE TABLE IF NOT EXISTS operator_annotations (
			id text PRIMARY KEY,
			run_id text NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
			asset_id text REFERENCES assets(id) ON DELETE SET NULL,
			author text NOT NULL,
			body text NOT NULL,
			tags jsonb NOT NULL DEFAULT '[]'::jsonb,
			created_at timestamptz NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id text PRIMARY KEY,
			event_type text NOT NULL,
			subject text NOT NULL,
			action text NOT NULL,
			actor text NOT NULL,
			occurred_at timestamptz NOT NULL,
			replay_key text UNIQUE,
			payload jsonb NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_run_channel_ts ON telemetry_samples(run_id, channel_id, ts DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_gateway_ts ON telemetry_samples(gateway_id, ts DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_alarms_status ON alarms(status, raised_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_type_time ON audit_events(event_type, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_payload_gin ON audit_events USING gin(payload)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateAsset(ctx context.Context, asset Asset, actor string) (Asset, error) {
	if err := asset.Prepare(); err != nil {
		return Asset{}, err
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO assets
			(id, external_id, name, kind, serial_number, location, tags, metadata, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			asset.ID, nullString(asset.ExternalID), asset.Name, asset.Kind, nullString(asset.SerialNumber),
			nullString(asset.Location), mustJSON(asset.Tags), mustJSON(asset.Metadata), asset.CreatedAt, asset.UpdatedAt)
		if err != nil {
			return err
		}
		_, err = s.appendAuditEventTx(ctx, tx, "asset", asset.ID, "created", actor, asset, "")
		return err
	})
	return asset, err
}

func (s *Store) ListAssets(ctx context.Context) ([]Asset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, external_id, name, kind, serial_number, location, tags, metadata, created_at, updated_at FROM assets ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Asset
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, asset)
	}
	return out, rows.Err()
}

func (s *Store) GetAsset(ctx context.Context, id string) (Asset, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, external_id, name, kind, serial_number, location, tags, metadata, created_at, updated_at FROM assets WHERE id=$1`, id)
	return scanAsset(row)
}

func (s *Store) CreateProcedure(ctx context.Context, procedure Procedure, actor string) (Procedure, error) {
	if err := procedure.Prepare(); err != nil {
		return Procedure{}, err
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO procedures
			(id, name, version, steps, requirements, metadata, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			procedure.ID, procedure.Name, procedure.Version, mustJSON(procedure.Steps), mustJSON(procedure.Requirements),
			mustJSON(procedure.Metadata), procedure.CreatedAt, procedure.UpdatedAt)
		if err != nil {
			return err
		}
		_, err = s.appendAuditEventTx(ctx, tx, "procedure", procedure.ID, "created", actor, procedure, "")
		return err
	})
	return procedure, err
}

func (s *Store) ListProcedures(ctx context.Context) ([]Procedure, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, version, steps, requirements, metadata, created_at, updated_at FROM procedures ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Procedure
	for rows.Next() {
		procedure, err := scanProcedure(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, procedure)
	}
	return out, rows.Err()
}

func (s *Store) CreateChannel(ctx context.Context, channel Channel, actor string) (Channel, error) {
	if err := channel.Prepare(); err != nil {
		return Channel{}, err
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO channels
			(id, asset_id, name, unit, sample_rate_hz, data_type, alarm_min, alarm_max, metadata, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			channel.ID, channel.AssetID, channel.Name, nullString(channel.Unit), nullFloat64(channel.SampleRateHz),
			channel.DataType, nullableFloat(channel.AlarmMin), nullableFloat(channel.AlarmMax), mustJSON(channel.Metadata),
			channel.CreatedAt, channel.UpdatedAt)
		if err != nil {
			return err
		}
		_, err = s.appendAuditEventTx(ctx, tx, "channel", channel.ID, "created", actor, channel, "")
		return err
	})
	return channel, err
}

func (s *Store) ListChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, asset_id, name, unit, sample_rate_hz, data_type, alarm_min, alarm_max, metadata, created_at, updated_at FROM channels ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Channel
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, channel)
	}
	return out, rows.Err()
}

func (s *Store) CreateTestRun(ctx context.Context, run TestRun, actor string) (TestRun, error) {
	if err := run.Prepare(s.cfg.SiteID); err != nil {
		return TestRun{}, err
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO test_runs
			(id, asset_id, procedure_id, run_number, status, operator, site_id, started_at, ended_at, metadata, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			run.ID, run.AssetID, nullString(run.ProcedureID), run.RunNumber, run.Status, nullString(run.Operator),
			run.SiteID, run.StartedAt, nullableTime(run.EndedAt), mustJSON(run.Metadata), run.CreatedAt, run.UpdatedAt)
		if err != nil {
			return err
		}
		_, err = s.appendAuditEventTx(ctx, tx, "test_run", run.ID, "created", actor, run, "")
		return err
	})
	return run, err
}

func (s *Store) ListTestRuns(ctx context.Context) ([]TestRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, asset_id, procedure_id, run_number, status, operator, site_id, started_at, ended_at, metadata, created_at, updated_at FROM test_runs ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestRun
	for rows.Next() {
		run, err := scanTestRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *Store) CreateAnnotation(ctx context.Context, annotation OperatorAnnotation, actor string) (OperatorAnnotation, error) {
	if err := annotation.Prepare(); err != nil {
		return OperatorAnnotation{}, err
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO operator_annotations
			(id, run_id, asset_id, author, body, tags, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			annotation.ID, annotation.RunID, nullString(annotation.AssetID), annotation.Author, annotation.Body,
			mustJSON(annotation.Tags), annotation.CreatedAt)
		if err != nil {
			return err
		}
		_, err = s.appendAuditEventTx(ctx, tx, "annotation", annotation.ID, "created", actor, annotation, "")
		return err
	})
	return annotation, err
}

func (s *Store) ListAnnotations(ctx context.Context) ([]OperatorAnnotation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, run_id, asset_id, author, body, tags, created_at FROM operator_annotations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OperatorAnnotation
	for rows.Next() {
		var annotation OperatorAnnotation
		var assetID sql.NullString
		var tagsRaw []byte
		if err := rows.Scan(&annotation.ID, &annotation.RunID, &assetID, &annotation.Author, &annotation.Body, &tagsRaw, &annotation.CreatedAt); err != nil {
			return nil, err
		}
		annotation.AssetID = assetID.String
		_ = json.Unmarshal(tagsRaw, &annotation.Tags)
		out = append(out, annotation)
	}
	return out, rows.Err()
}

func (s *Store) ListAlarms(ctx context.Context, status string) ([]Alarm, error) {
	query := `SELECT id, asset_id, run_id, channel_id, severity, status, message, threshold, observed_value, raised_at, cleared_at, metadata FROM alarms`
	args := []any{}
	if status != "" {
		query += ` WHERE status=$1`
		args = append(args, status)
	}
	query += ` ORDER BY raised_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Alarm
	for rows.Next() {
		alarm, err := scanAlarm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, alarm)
	}
	return out, rows.Err()
}

func (s *Store) IngestTelemetryBatch(ctx context.Context, samples []TelemetrySample, actor string) (IngestResult, error) {
	result := IngestResult{
		Rejected:   map[int]string{},
		GatewayIDs: map[string]int{},
	}
	if len(samples) == 0 {
		return result, errors.New("at least one telemetry sample is required")
	}

	err := s.withTx(ctx, func(tx *sql.Tx) error {
		accepted := make([]TelemetrySample, 0, len(samples))
		for i := range samples {
			sample := samples[i]
			if err := sample.Prepare(); err != nil {
				result.RejectedSamples++
				result.Rejected[i] = err.Error()
				continue
			}

			channel, err := s.getChannelTx(ctx, tx, sample.ChannelID)
			if err != nil {
				result.RejectedSamples++
				result.Rejected[i] = err.Error()
				continue
			}
			if sample.AssetID == "" {
				sample.AssetID = channel.AssetID
			}
			if sample.GatewayID != "" {
				result.GatewayIDs[sample.GatewayID]++
			}

			insertResult, err := tx.ExecContext(ctx, `INSERT INTO telemetry_samples
				(id, run_id, asset_id, channel_id, gateway_id, ts, numeric_value, text_value, quality, metadata, received_at, replay_key)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
				ON CONFLICT (replay_key) DO NOTHING`,
				sample.ID, sample.RunID, sample.AssetID, sample.ChannelID, nullString(sample.GatewayID), sample.Timestamp,
				nullableFloat(sample.NumericValue), nullString(sample.TextValue), sample.Quality, mustJSON(sample.Metadata),
				sample.ReceivedAt, nullString(sample.ReplayKey))
			if err != nil {
				result.RejectedSamples++
				result.Rejected[i] = err.Error()
				continue
			}
			if rows, _ := insertResult.RowsAffected(); rows == 0 {
				result.RejectedSamples++
				result.Rejected[i] = "duplicate replay_key"
				continue
			}

			alarm, raised, err := s.raiseAlarmIfNeeded(ctx, tx, channel, sample)
			if err != nil {
				return err
			}
			if raised {
				sample.Alarm = &alarm
				result.Alarms = append(result.Alarms, alarm)
				result.AlarmsRaised++
			}
			result.AcceptedSamples++
			accepted = append(accepted, sample)
		}

		if result.AcceptedSamples == 0 {
			return errors.New("all telemetry samples rejected")
		}

		payload := map[string]any{
			"site_id":          s.cfg.SiteID,
			"accepted_samples": result.AcceptedSamples,
			"rejected_samples": result.RejectedSamples,
			"gateway_ids":      result.GatewayIDs,
			"samples":          accepted,
			"alarms":           result.Alarms,
		}
		event, err := s.appendAuditEventTx(ctx, tx, "telemetry_batch", s.cfg.SiteID, "ingested", actor, payload, "")
		if err != nil {
			return err
		}
		result.EventID = event.ID
		result.SearchDocument = map[string]string{
			"index":       s.cfg.ElasticIndex,
			"document_id": event.ID,
		}
		return nil
	})

	s.metrics.AcceptedSamples.Add(uint64(result.AcceptedSamples))
	s.metrics.RejectedSamples.Add(uint64(result.RejectedSamples))
	s.metrics.AlarmsRaised.Add(uint64(result.AlarmsRaised))
	return result, err
}

func (s *Store) ListEvents(ctx context.Context, eventType string, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, event_type, subject, action, actor, occurred_at, replay_key, payload FROM audit_events`
	args := []any{}
	if eventType != "" {
		query += ` WHERE event_type=$1`
		args = append(args, eventType)
	}
	query += fmt.Sprintf(` ORDER BY occurred_at DESC LIMIT %d`, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuditEvent
	for rows.Next() {
		event, err := scanAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Store) Search(ctx context.Context, query, kind string, limit int) ([]SearchHit, error) {
	s.metrics.SearchRequests.Add(1)
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	query = strings.TrimSpace(query)
	kind = strings.TrimSpace(kind)
	if query == "" {
		return nil, errors.New("query parameter q is required")
	}
	if s.es != nil {
		hits, err := s.searchElastic(ctx, query, kind, limit)
		if err == nil {
			return hits, nil
		}
		s.logger.Warn("elasticsearch search failed; falling back to postgres", "error", err)
	}
	return s.searchPostgres(ctx, query, kind, limit)
}

func (s *Store) ListenAndIndex(ctx context.Context) {
	if s.es == nil {
		return
	}
	report := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			s.logger.Warn("postgres listener event", "event", ev, "error", err)
		}
	}
	listener := pq.NewListener(s.cfg.DatabaseURL, 10*time.Second, time.Minute, report)
	defer listener.Close()
	if err := listener.Listen("telemetry_events"); err != nil {
		s.logger.Warn("listen telemetry_events", "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case notification := <-listener.Notify:
			if notification == nil {
				continue
			}
			if err := s.indexAuditEventByID(ctx, notification.Extra); err != nil {
				s.logger.Warn("index audit event", "event_id", notification.Extra, "error", err)
			}
		case <-time.After(90 * time.Second):
			if err := listener.Ping(); err != nil {
				s.logger.Warn("postgres listener ping", "error", err)
			}
		}
	}
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) appendAuditEventTx(ctx context.Context, tx *sql.Tx, eventType, subject, action, actor string, payload any, replayKey string) (AuditEvent, error) {
	if actor == "" {
		actor = "system"
	}
	event := AuditEvent{
		ID:         newID("evt"),
		EventType:  eventType,
		Subject:    subject,
		Action:     action,
		Actor:      actor,
		OccurredAt: time.Now().UTC(),
		ReplayKey:  replayKey,
		Payload:    rawJSON(payload),
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events
		(id, event_type, subject, action, actor, occurred_at, replay_key, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (replay_key) DO NOTHING`,
		event.ID, event.EventType, event.Subject, event.Action, event.Actor, event.OccurredAt,
		nullString(event.ReplayKey), []byte(event.Payload))
	if err != nil {
		return AuditEvent{}, err
	}
	_, err = tx.ExecContext(ctx, `SELECT pg_notify('telemetry_events', $1)`, event.ID)
	if err != nil {
		return AuditEvent{}, err
	}
	s.metrics.AuditEvents.Add(1)
	return event, nil
}

func (s *Store) getChannelTx(ctx context.Context, tx *sql.Tx, id string) (Channel, error) {
	row := tx.QueryRowContext(ctx, `SELECT id, asset_id, name, unit, sample_rate_hz, data_type, alarm_min, alarm_max, metadata, created_at, updated_at FROM channels WHERE id=$1`, id)
	return scanChannel(row)
}

func (s *Store) raiseAlarmIfNeeded(ctx context.Context, tx *sql.Tx, channel Channel, sample TelemetrySample) (Alarm, bool, error) {
	if sample.NumericValue == nil {
		return Alarm{}, false, nil
	}
	var threshold *float64
	var message string
	if channel.AlarmMax != nil && *sample.NumericValue > *channel.AlarmMax {
		threshold = channel.AlarmMax
		message = fmt.Sprintf("%s above max %.3f %s", channel.Name, *channel.AlarmMax, channel.Unit)
	}
	if channel.AlarmMin != nil && *sample.NumericValue < *channel.AlarmMin {
		threshold = channel.AlarmMin
		message = fmt.Sprintf("%s below min %.3f %s", channel.Name, *channel.AlarmMin, channel.Unit)
	}
	if threshold == nil {
		return Alarm{}, false, nil
	}
	alarm := Alarm{
		AssetID:       sample.AssetID,
		RunID:         sample.RunID,
		ChannelID:     sample.ChannelID,
		Severity:      "critical",
		Status:        "open",
		Message:       message,
		Threshold:     threshold,
		ObservedValue: sample.NumericValue,
		Metadata: JSONMap{
			"sample_id":  sample.ID,
			"gateway_id": sample.GatewayID,
		},
	}
	if err := alarm.Prepare(); err != nil {
		return Alarm{}, false, err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO alarms
		(id, asset_id, run_id, channel_id, severity, status, message, threshold, observed_value, raised_at, cleared_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		alarm.ID, alarm.AssetID, alarm.RunID, alarm.ChannelID, alarm.Severity, alarm.Status, alarm.Message,
		nullableFloat(alarm.Threshold), nullableFloat(alarm.ObservedValue), alarm.RaisedAt, nullableTime(alarm.ClearedAt),
		mustJSON(alarm.Metadata))
	return alarm, true, err
}

func (s *Store) indexAuditEventByID(ctx context.Context, id string) error {
	row := s.db.QueryRowContext(ctx, `SELECT id, event_type, subject, action, actor, occurred_at, replay_key, payload FROM audit_events WHERE id=$1`, id)
	event, err := scanAuditEvent(row)
	if err != nil {
		return err
	}
	body := bytes.NewReader(mustJSON(event))
	res, err := s.es.Index(s.cfg.ElasticIndex, body, s.es.Index.WithContext(ctx), s.es.Index.WithDocumentID(event.ID))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch index failed: %s", res.Status())
	}
	return nil
}

func (s *Store) searchElastic(ctx context.Context, query, kind string, limit int) ([]SearchHit, error) {
	must := []any{
		map[string]any{"query_string": map[string]any{"query": query}},
	}
	filter := []any{}
	if kind != "" {
		filter = append(filter, map[string]any{"term": map[string]any{"event_type.keyword": kind}})
	}
	body := map[string]any{
		"size": limit,
		"query": map[string]any{
			"bool": map[string]any{
				"must":   must,
				"filter": filter,
			},
		},
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}
	res, err := s.es.Search(
		s.es.Search.WithContext(ctx),
		s.es.Search.WithIndex(s.cfg.ElasticIndex),
		s.es.Search.WithBody(&buf),
		s.es.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch search failed: %s", res.Status())
	}
	var decoded struct {
		Hits struct {
			Hits []struct {
				ID     string          `json:"_id"`
				Score  float64         `json:"_score"`
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	hits := make([]SearchHit, 0, len(decoded.Hits.Hits))
	for _, hit := range decoded.Hits.Hits {
		hits = append(hits, SearchHit{
			ID:     hit.ID,
			Kind:   "audit_event",
			Score:  hit.Score,
			Source: hit.Source,
		})
	}
	return hits, nil
}

func (s *Store) searchPostgres(ctx context.Context, query, kind string, limit int) ([]SearchHit, error) {
	like := "%" + strings.ToLower(query) + "%"
	sqlText := `SELECT id, event_type, occurred_at, payload FROM audit_events
		WHERE (lower(subject) LIKE $1 OR lower(action) LIKE $1 OR lower(actor) LIKE $1 OR lower(payload::text) LIKE $1)`
	args := []any{like}
	if kind != "" {
		sqlText += ` AND event_type=$2`
		args = append(args, kind)
	}
	sqlText += fmt.Sprintf(` ORDER BY occurred_at DESC LIMIT %d`, limit)
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var hit SearchHit
		var source []byte
		if err := rows.Scan(&hit.ID, &hit.Kind, &hit.IndexedAt, &source); err != nil {
			return nil, err
		}
		hit.Source = json.RawMessage(source)
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

func scanAsset(scanner interface {
	Scan(dest ...any) error
}) (Asset, error) {
	var asset Asset
	var externalID, serialNumber, location sql.NullString
	var tagsRaw, metadataRaw []byte
	err := scanner.Scan(&asset.ID, &externalID, &asset.Name, &asset.Kind, &serialNumber, &location, &tagsRaw, &metadataRaw, &asset.CreatedAt, &asset.UpdatedAt)
	if err != nil {
		return Asset{}, err
	}
	asset.ExternalID = externalID.String
	asset.SerialNumber = serialNumber.String
	asset.Location = location.String
	_ = json.Unmarshal(tagsRaw, &asset.Tags)
	_ = json.Unmarshal(metadataRaw, &asset.Metadata)
	asset.Metadata = cleanMap(asset.Metadata)
	return asset, nil
}

func scanProcedure(scanner interface {
	Scan(dest ...any) error
}) (Procedure, error) {
	var procedure Procedure
	var stepsRaw, requirementsRaw, metadataRaw []byte
	err := scanner.Scan(&procedure.ID, &procedure.Name, &procedure.Version, &stepsRaw, &requirementsRaw, &metadataRaw, &procedure.CreatedAt, &procedure.UpdatedAt)
	if err != nil {
		return Procedure{}, err
	}
	_ = json.Unmarshal(stepsRaw, &procedure.Steps)
	_ = json.Unmarshal(requirementsRaw, &procedure.Requirements)
	_ = json.Unmarshal(metadataRaw, &procedure.Metadata)
	procedure.Metadata = cleanMap(procedure.Metadata)
	return procedure, nil
}

func scanChannel(scanner interface {
	Scan(dest ...any) error
}) (Channel, error) {
	var channel Channel
	var unit sql.NullString
	var sampleRate, alarmMin, alarmMax sql.NullFloat64
	var metadataRaw []byte
	err := scanner.Scan(&channel.ID, &channel.AssetID, &channel.Name, &unit, &sampleRate, &channel.DataType, &alarmMin, &alarmMax, &metadataRaw, &channel.CreatedAt, &channel.UpdatedAt)
	if err != nil {
		return Channel{}, err
	}
	channel.Unit = unit.String
	if sampleRate.Valid {
		channel.SampleRateHz = sampleRate.Float64
	}
	if alarmMin.Valid {
		channel.AlarmMin = &alarmMin.Float64
	}
	if alarmMax.Valid {
		channel.AlarmMax = &alarmMax.Float64
	}
	_ = json.Unmarshal(metadataRaw, &channel.Metadata)
	channel.Metadata = cleanMap(channel.Metadata)
	return channel, nil
}

func scanTestRun(scanner interface {
	Scan(dest ...any) error
}) (TestRun, error) {
	var run TestRun
	var procedureID, operator sql.NullString
	var endedAt sql.NullTime
	var metadataRaw []byte
	err := scanner.Scan(&run.ID, &run.AssetID, &procedureID, &run.RunNumber, &run.Status, &operator, &run.SiteID, &run.StartedAt, &endedAt, &metadataRaw, &run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		return TestRun{}, err
	}
	run.ProcedureID = procedureID.String
	run.Operator = operator.String
	if endedAt.Valid {
		run.EndedAt = &endedAt.Time
	}
	_ = json.Unmarshal(metadataRaw, &run.Metadata)
	run.Metadata = cleanMap(run.Metadata)
	return run, nil
}

func scanAlarm(scanner interface {
	Scan(dest ...any) error
}) (Alarm, error) {
	var alarm Alarm
	var threshold, observed sql.NullFloat64
	var cleared sql.NullTime
	var metadataRaw []byte
	err := scanner.Scan(&alarm.ID, &alarm.AssetID, &alarm.RunID, &alarm.ChannelID, &alarm.Severity, &alarm.Status, &alarm.Message, &threshold, &observed, &alarm.RaisedAt, &cleared, &metadataRaw)
	if err != nil {
		return Alarm{}, err
	}
	if threshold.Valid {
		alarm.Threshold = &threshold.Float64
	}
	if observed.Valid {
		alarm.ObservedValue = &observed.Float64
	}
	if cleared.Valid {
		alarm.ClearedAt = &cleared.Time
	}
	_ = json.Unmarshal(metadataRaw, &alarm.Metadata)
	alarm.Metadata = cleanMap(alarm.Metadata)
	return alarm, nil
}

func scanAuditEvent(scanner interface {
	Scan(dest ...any) error
}) (AuditEvent, error) {
	var event AuditEvent
	var replayKey sql.NullString
	var payload []byte
	err := scanner.Scan(&event.ID, &event.EventType, &event.Subject, &event.Action, &event.Actor, &event.OccurredAt, &replayKey, &payload)
	if err != nil {
		return AuditEvent{}, err
	}
	event.ReplayKey = replayKey.String
	event.Payload = json.RawMessage(payload)
	return event, nil
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullFloat64(value float64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return *value
}
