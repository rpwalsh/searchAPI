package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment      string
	SiteID           string
	Address          string
	DatabaseURL      string
	APIKey           string
	ElasticAddresses []string
	ElasticAPIKey    string
	ElasticIndex     string
	IngestMaxBytes   int64
	ShutdownTimeout  time.Duration
	SeedOnStart      bool
}

func LoadConfig() (Config, error) {
	environment := env("APP_ENV", "demo")
	production := isProduction(environment)
	cfg := Config{
		Environment:     environment,
		SiteID:          env("SITE_ID", "lab-01"),
		Address:         env("API_ADDR", ":8080"),
		DatabaseURL:     env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/telemetry?sslmode=disable"),
		APIKey:          env("API_KEY", defaultDemoKey(production)),
		ElasticAPIKey:   strings.TrimSpace(os.Getenv("ELASTICSEARCH_API_KEY")),
		ElasticIndex:    env("ELASTICSEARCH_INDEX", "hardware-telemetry-events"),
		IngestMaxBytes:  envInt64("INGEST_MAX_BYTES", 16<<20),
		ShutdownTimeout: time.Duration(envInt64("SHUTDOWN_TIMEOUT_SECONDS", 20)) * time.Second,
		SeedOnStart:     envBool("SEED_ON_START", !production),
	}

	if _, err := url.Parse(cfg.DatabaseURL); err != nil {
		return Config{}, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	if raw := strings.TrimSpace(os.Getenv("ELASTICSEARCH_URL")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if address := strings.TrimSpace(part); address != "" {
				cfg.ElasticAddresses = append(cfg.ElasticAddresses, address)
			}
		}
	}

	return cfg, nil
}

func defaultDemoKey(production bool) string {
	if production {
		return ""
	}
	return "demo-key"
}

func isProduction(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func logLevel(environment string) slog.Level {
	if isProduction(environment) {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}
