package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		panic(err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.Environment)}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	store, err := NewStore(cfg, db, logger)
	if err != nil {
		logger.Error("create store", "error", err)
		os.Exit(1)
	}
	if err := store.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		os.Exit(1)
	}

	if cfg.SeedOnStart {
		if summary, err := store.SeedDemo(ctx, "startup"); err != nil {
			logger.Warn("seed demo data", "error", err)
		} else {
			logger.Info("seeded demo data", "asset_id", summary.AssetID, "run_id", summary.RunID)
		}
	}

	if store.ElasticEnabled() {
		go store.ListenAndIndex(ctx)
		logger.Info("elasticsearch indexing enabled", "index", cfg.ElasticIndex)
	} else {
		logger.Info("elasticsearch disabled; search uses postgres fallback")
	}

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           NewRouter(store, cfg, logger),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("searchAPI listening", "address", cfg.Address, "site_id", cfg.SiteID)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown", "error", err)
		os.Exit(1)
	}
	logger.Info("searchAPI stopped")
}
