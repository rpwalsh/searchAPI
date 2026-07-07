package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type JSONMap map[string]any

type Asset struct {
	ID           string    `json:"id"`
	ExternalID   string    `json:"external_id,omitempty"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	SerialNumber string    `json:"serial_number,omitempty"`
	Location     string    `json:"location,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	Metadata     JSONMap   `json:"metadata,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (a *Asset) Prepare() error {
	a.Name = strings.TrimSpace(a.Name)
	a.Kind = strings.TrimSpace(a.Kind)
	a.ExternalID = strings.TrimSpace(a.ExternalID)
	a.SerialNumber = strings.TrimSpace(a.SerialNumber)
	a.Location = strings.TrimSpace(a.Location)
	a.Tags = cleanTags(a.Tags)
	a.Metadata = cleanMap(a.Metadata)
	if a.ID == "" {
		a.ID = newID("asset")
	}
	if a.Name == "" {
		return errors.New("asset name is required")
	}
	if a.Kind == "" {
		return errors.New("asset kind is required")
	}
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	return nil
}

type ProcedureStep struct {
	Order       int     `json:"order"`
	Name        string  `json:"name"`
	Instruction string  `json:"instruction"`
	Expected    string  `json:"expected,omitempty"`
	TimeoutSec  float64 `json:"timeout_sec,omitempty"`
}

type Procedure struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Steps        []ProcedureStep `json:"steps,omitempty"`
	Requirements []string        `json:"requirements,omitempty"`
	Metadata     JSONMap         `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func (p *Procedure) Prepare() error {
	p.Name = strings.TrimSpace(p.Name)
	p.Version = strings.TrimSpace(p.Version)
	p.Requirements = cleanTags(p.Requirements)
	p.Metadata = cleanMap(p.Metadata)
	if p.ID == "" {
		p.ID = newID("proc")
	}
	if p.Name == "" {
		return errors.New("procedure name is required")
	}
	if p.Version == "" {
		p.Version = "1.0.0"
	}
	for i := range p.Steps {
		p.Steps[i].Name = strings.TrimSpace(p.Steps[i].Name)
		p.Steps[i].Instruction = strings.TrimSpace(p.Steps[i].Instruction)
		if p.Steps[i].Order == 0 {
			p.Steps[i].Order = i + 1
		}
		if p.Steps[i].Name == "" {
			return fmt.Errorf("procedure step %d name is required", i+1)
		}
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	return nil
}

type TestRun struct {
	ID          string     `json:"id"`
	AssetID     string     `json:"asset_id"`
	ProcedureID string     `json:"procedure_id,omitempty"`
	RunNumber   string     `json:"run_number"`
	Status      string     `json:"status"`
	Operator    string     `json:"operator,omitempty"`
	SiteID      string     `json:"site_id"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	Metadata    JSONMap    `json:"metadata,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (r *TestRun) Prepare(defaultSite string) error {
	r.AssetID = strings.TrimSpace(r.AssetID)
	r.ProcedureID = strings.TrimSpace(r.ProcedureID)
	r.RunNumber = strings.TrimSpace(r.RunNumber)
	r.Status = strings.ToLower(strings.TrimSpace(r.Status))
	r.Operator = strings.TrimSpace(r.Operator)
	r.SiteID = strings.TrimSpace(r.SiteID)
	r.Metadata = cleanMap(r.Metadata)
	if r.ID == "" {
		r.ID = newID("run")
	}
	if r.AssetID == "" {
		return errors.New("test run asset_id is required")
	}
	if r.RunNumber == "" {
		r.RunNumber = "run-" + time.Now().UTC().Format("20060102-150405")
	}
	if r.Status == "" {
		r.Status = "active"
	}
	if !oneOf(r.Status, "planned", "active", "passed", "failed", "aborted") {
		return errors.New("test run status must be planned, active, passed, failed, or aborted")
	}
	if r.SiteID == "" {
		r.SiteID = defaultSite
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	return nil
}

type Channel struct {
	ID           string    `json:"id"`
	AssetID      string    `json:"asset_id"`
	Name         string    `json:"name"`
	Unit         string    `json:"unit,omitempty"`
	SampleRateHz float64   `json:"sample_rate_hz,omitempty"`
	DataType     string    `json:"data_type"`
	AlarmMin     *float64  `json:"alarm_min,omitempty"`
	AlarmMax     *float64  `json:"alarm_max,omitempty"`
	Metadata     JSONMap   `json:"metadata,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (c *Channel) Prepare() error {
	c.AssetID = strings.TrimSpace(c.AssetID)
	c.Name = strings.TrimSpace(c.Name)
	c.Unit = strings.TrimSpace(c.Unit)
	c.DataType = strings.ToLower(strings.TrimSpace(c.DataType))
	c.Metadata = cleanMap(c.Metadata)
	if c.ID == "" {
		c.ID = newID("chan")
	}
	if c.AssetID == "" {
		return errors.New("channel asset_id is required")
	}
	if c.Name == "" {
		return errors.New("channel name is required")
	}
	if c.DataType == "" {
		c.DataType = "float64"
	}
	if !oneOf(c.DataType, "float64", "integer", "boolean", "string") {
		return errors.New("channel data_type must be float64, integer, boolean, or string")
	}
	if c.SampleRateHz < 0 {
		return errors.New("channel sample_rate_hz cannot be negative")
	}
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	return nil
}

type TelemetrySample struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	AssetID      string    `json:"asset_id,omitempty"`
	ChannelID    string    `json:"channel_id"`
	GatewayID    string    `json:"gateway_id,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	NumericValue *float64  `json:"numeric_value,omitempty"`
	TextValue    string    `json:"text_value,omitempty"`
	Quality      string    `json:"quality,omitempty"`
	Metadata     JSONMap   `json:"metadata,omitempty"`
	ReceivedAt   time.Time `json:"received_at"`
	Alarm        *Alarm    `json:"alarm,omitempty"`
	ReplayKey    string    `json:"replay_key,omitempty"`
}

func (s *TelemetrySample) Prepare() error {
	s.RunID = strings.TrimSpace(s.RunID)
	s.AssetID = strings.TrimSpace(s.AssetID)
	s.ChannelID = strings.TrimSpace(s.ChannelID)
	s.GatewayID = strings.TrimSpace(s.GatewayID)
	s.Quality = strings.ToLower(strings.TrimSpace(s.Quality))
	s.TextValue = strings.TrimSpace(s.TextValue)
	s.Metadata = cleanMap(s.Metadata)
	s.ReplayKey = strings.TrimSpace(s.ReplayKey)
	if s.ID == "" {
		s.ID = newID("sample")
	}
	if s.RunID == "" {
		return errors.New("sample run_id is required")
	}
	if s.ChannelID == "" {
		return errors.New("sample channel_id is required")
	}
	if s.Timestamp.IsZero() {
		s.Timestamp = time.Now().UTC()
	}
	if s.NumericValue == nil && s.TextValue == "" {
		return errors.New("sample requires numeric_value or text_value")
	}
	if s.Quality == "" {
		s.Quality = "good"
	}
	if !oneOf(s.Quality, "good", "suspect", "bad", "derived") {
		return errors.New("sample quality must be good, suspect, bad, or derived")
	}
	s.ReceivedAt = time.Now().UTC()
	return nil
}

type Alarm struct {
	ID            string     `json:"id"`
	AssetID       string     `json:"asset_id"`
	RunID         string     `json:"run_id"`
	ChannelID     string     `json:"channel_id"`
	Severity      string     `json:"severity"`
	Status        string     `json:"status"`
	Message       string     `json:"message"`
	Threshold     *float64   `json:"threshold,omitempty"`
	ObservedValue *float64   `json:"observed_value,omitempty"`
	RaisedAt      time.Time  `json:"raised_at"`
	ClearedAt     *time.Time `json:"cleared_at,omitempty"`
	Metadata      JSONMap    `json:"metadata,omitempty"`
}

func (a *Alarm) Prepare() error {
	a.AssetID = strings.TrimSpace(a.AssetID)
	a.RunID = strings.TrimSpace(a.RunID)
	a.ChannelID = strings.TrimSpace(a.ChannelID)
	a.Severity = strings.ToLower(strings.TrimSpace(a.Severity))
	a.Status = strings.ToLower(strings.TrimSpace(a.Status))
	a.Message = strings.TrimSpace(a.Message)
	a.Metadata = cleanMap(a.Metadata)
	if a.ID == "" {
		a.ID = newID("alarm")
	}
	if a.AssetID == "" || a.RunID == "" || a.ChannelID == "" {
		return errors.New("alarm asset_id, run_id, and channel_id are required")
	}
	if a.Severity == "" {
		a.Severity = "warning"
	}
	if !oneOf(a.Severity, "info", "warning", "critical") {
		return errors.New("alarm severity must be info, warning, or critical")
	}
	if a.Status == "" {
		a.Status = "open"
	}
	if !oneOf(a.Status, "open", "acknowledged", "closed") {
		return errors.New("alarm status must be open, acknowledged, or closed")
	}
	if a.Message == "" {
		a.Message = "threshold violation"
	}
	if a.RaisedAt.IsZero() {
		a.RaisedAt = time.Now().UTC()
	}
	return nil
}

type OperatorAnnotation struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	AssetID   string    `json:"asset_id,omitempty"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (a *OperatorAnnotation) Prepare() error {
	a.RunID = strings.TrimSpace(a.RunID)
	a.AssetID = strings.TrimSpace(a.AssetID)
	a.Author = strings.TrimSpace(a.Author)
	a.Body = strings.TrimSpace(a.Body)
	a.Tags = cleanTags(a.Tags)
	if a.ID == "" {
		a.ID = newID("note")
	}
	if a.RunID == "" {
		return errors.New("annotation run_id is required")
	}
	if a.Author == "" {
		a.Author = "operator"
	}
	if a.Body == "" {
		return errors.New("annotation body is required")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	return nil
}

type AuditEvent struct {
	ID         string          `json:"id"`
	EventType  string          `json:"event_type"`
	Subject    string          `json:"subject"`
	Action     string          `json:"action"`
	Actor      string          `json:"actor"`
	OccurredAt time.Time       `json:"occurred_at"`
	ReplayKey  string          `json:"replay_key,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

type SearchHit struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Score     float64         `json:"score,omitempty"`
	Source    json.RawMessage `json:"source"`
	IndexedAt time.Time       `json:"indexed_at,omitempty"`
}

type IngestResult struct {
	AcceptedSamples int               `json:"accepted_samples"`
	RejectedSamples int               `json:"rejected_samples"`
	AlarmsRaised    int               `json:"alarms_raised"`
	EventID         string            `json:"event_id,omitempty"`
	Rejected        map[int]string    `json:"rejected,omitempty"`
	Alarms          []Alarm           `json:"alarms,omitempty"`
	GatewayIDs      map[string]int    `json:"gateway_ids,omitempty"`
	SearchDocument  map[string]string `json:"search_document,omitempty"`
}

func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(buf[:]))
}

func mustJSON(value any) []byte {
	if value == nil {
		return []byte("null")
	}
	out, err := json.Marshal(value)
	if err != nil {
		return []byte("null")
	}
	return out
}

func rawJSON(value any) json.RawMessage {
	return json.RawMessage(mustJSON(value))
}

func cleanMap(value JSONMap) JSONMap {
	if value == nil {
		return JSONMap{}
	}
	return value
}

func cleanTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	clean := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		clean = append(clean, tag)
	}
	return clean
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
