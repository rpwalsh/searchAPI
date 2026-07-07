package main

import (
	"strings"
	"testing"
)

func TestReadTelemetrySamplesAcceptsNDJSON(t *testing.T) {
	body := strings.NewReader(`{"run_id":"run_1","channel_id":"chan_1","timestamp":"2026-07-07T19:00:01Z","numeric_value":2.4,"quality":"good","replay_key":"r1"}
{"run_id":"run_1","channel_id":"chan_1","timestamp":"2026-07-07T19:00:02Z","numeric_value":8.1,"quality":"good","replay_key":"r2"}`)

	samples, err := readTelemetrySamples(body)
	if err != nil {
		t.Fatalf("readTelemetrySamples returned error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	if samples[1].ReplayKey != "r2" {
		t.Fatalf("expected second replay key r2, got %q", samples[1].ReplayKey)
	}
}

func TestReadTelemetrySamplesAcceptsArray(t *testing.T) {
	body := strings.NewReader(`[{"run_id":"run_1","channel_id":"chan_1","numeric_value":2.4}]`)

	samples, err := readTelemetrySamples(body)
	if err != nil {
		t.Fatalf("readTelemetrySamples returned error: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
}

func TestReadTelemetrySamplesRejectsEmptyStream(t *testing.T) {
	_, err := readTelemetrySamples(strings.NewReader("  "))
	if err == nil {
		t.Fatal("expected empty stream error")
	}
}
