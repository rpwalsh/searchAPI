package main

import "testing"

func TestLoadConfigDefaultsToDemoCredentials(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("API_KEY", "")
	t.Setenv("SEED_ON_START", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.Environment != "demo" {
		t.Fatalf("expected demo environment, got %q", cfg.Environment)
	}
	if cfg.APIKey != "demo-key" {
		t.Fatalf("expected demo-key, got %q", cfg.APIKey)
	}
	if !cfg.SeedOnStart {
		t.Fatal("expected demo seed-on-start default")
	}
}

func TestLoadConfigDisablesDemoDefaultsInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("API_KEY", "")
	t.Setenv("SEED_ON_START", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.APIKey != "" {
		t.Fatalf("expected no production default API key, got %q", cfg.APIKey)
	}
	if cfg.SeedOnStart {
		t.Fatal("expected production seed-on-start disabled")
	}
}
