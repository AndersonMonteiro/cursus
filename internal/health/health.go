// Package health serves the liveness and readiness endpoints shared by the
// service processes.
package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Checker reports whether one dependency of the process is reachable.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

const (
	statusOK          = "ok"
	statusUnavailable = "unavailable"
)

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Liveness answers 200 for as long as the process can serve HTTP. It does not
// look at dependencies: a restart does not fix an unreachable database.
func Liveness() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": statusOK})
	})
}

// Readiness answers 200 when every checker passes within timeout and 503
// otherwise. The endpoint is unauthenticated, so the response carries only the
// name and state of each dependency; the cause of a failure goes to the log.
func Readiness(logger *slog.Logger, timeout time.Duration, checkers ...Checker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		// Checks run concurrently so the probe is bounded by one timeout
		// rather than by the sum over all dependencies.
		results := make([]error, len(checkers))
		var wg sync.WaitGroup
		for i, checker := range checkers {
			wg.Go(func() { results[i] = checker.Check(ctx) })
		}
		wg.Wait()

		response := readinessResponse{Status: statusOK, Checks: make(map[string]string, len(checkers))}
		code := http.StatusOK
		for i, checker := range checkers {
			if err := results[i]; err != nil {
				logger.WarnContext(r.Context(), "readiness check failed", "dependency", checker.Name(), "error", err)
				response.Checks[checker.Name()] = statusUnavailable
				response.Status = statusUnavailable
				code = http.StatusServiceUnavailable
				continue
			}
			response.Checks[checker.Name()] = statusOK
		}
		writeJSON(w, code, response)
	})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
