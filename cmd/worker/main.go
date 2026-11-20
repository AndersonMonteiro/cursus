// Command worker hosts the Temporal workers that execute agent runs.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AndersonMonteiro/cursus/internal/config"
	"github.com/AndersonMonteiro/cursus/internal/health"
	"github.com/AndersonMonteiro/cursus/internal/postgres"
	"github.com/AndersonMonteiro/cursus/internal/temporal"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

// run connects to the dependencies and serves the health endpoints. No
// Temporal worker polls yet: that starts with the first workflow, when its
// task queue is named.
func run() error {
	cfg, err := config.LoadWorker(os.Getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).
		With("service", "worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	temporalClient, err := temporal.NewClient(cfg.TemporalAddress, cfg.TemporalNamespace, logger)
	if err != nil {
		return err
	}
	defer temporalClient.Close()

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health.Liveness())
	mux.Handle("GET /readyz", health.Readiness(logger, cfg.ReadinessTimeout,
		postgres.NewChecker(pool),
		temporal.NewChecker(temporalClient),
	))

	server := &http.Server{Addr: cfg.Addr, Handler: mux, ReadHeaderTimeout: readHeaderTimeout}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	logger.Info("listening", "addr", cfg.Addr)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
