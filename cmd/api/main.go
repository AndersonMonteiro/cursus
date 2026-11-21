// Command api serves the Cursus HTTP API.
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

	"github.com/AndersonMonteiro/cursus/internal/auth"
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
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAPI(os.Getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).
		With("service", "api")

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

	// Replacing the issuer means constructing another auth.Verifier here.
	verifier, err := auth.NewHS256Verifier(cfg.TokenSigningKey, time.Now)
	if err != nil {
		return fmt.Errorf("%s: %w", config.EnvTokenSigningKey, err)
	}

	handler := routes(logger, verifier, health.Readiness(logger, cfg.ReadinessTimeout,
		postgres.NewChecker(pool),
		temporal.NewChecker(temporalClient),
	))

	server := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: readHeaderTimeout}
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

// routes mounts the health endpoints in the open and everything else behind
// authentication. API endpoints are registered on the inner mux, so a new
// route is authenticated unless it is deliberately added to the outer one.
func routes(logger *slog.Logger, verifier auth.Verifier, readiness http.Handler) http.Handler {
	authenticated := http.NewServeMux()

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health.Liveness())
	mux.Handle("GET /readyz", readiness)
	mux.Handle("/", auth.Authenticate(verifier, logger)(authenticated))
	return mux
}
