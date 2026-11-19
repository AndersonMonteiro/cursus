package health_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AndersonMonteiro/cursus/internal/health"
	"github.com/AndersonMonteiro/cursus/internal/postgres"
	"github.com/AndersonMonteiro/cursus/internal/temporal"
)

const probeTimeout = 500 * time.Millisecond

type response struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

type stubChecker struct {
	name string
	err  error
}

func (s stubChecker) Name() string                { return s.name }
func (s stubChecker) Check(context.Context) error { return s.err }

// closedAddr returns a loopback address that refuses connections: the port is
// reserved by a listener that is closed before the address is used.
func closedAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	return addr
}

func probe(t *testing.T, handler http.Handler) (int, response, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	raw := recorder.Body.String()
	var body response
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return recorder.Code, body, raw
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestReadinessFailsWhenPostgresIsDown(t *testing.T) {
	url := fmt.Sprintf("postgres://cursus:synthetic@%s/cursus?sslmode=disable", closedAddr(t))
	pool, err := postgres.NewPool(t.Context(), url)
	if err != nil {
		t.Fatalf("NewPool must not need a reachable database: %v", err)
	}
	t.Cleanup(pool.Close)

	handler := health.Readiness(discardLogger(), probeTimeout,
		postgres.NewChecker(pool),
		stubChecker{name: "temporal"},
	)
	code, body, raw := probe(t, handler)

	if code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", code)
	}
	if body.Checks["postgres"] != "unavailable" {
		t.Errorf("postgres check = %q, want unavailable", body.Checks["postgres"])
	}
	if body.Checks["temporal"] != "ok" {
		t.Errorf("temporal check = %q, want ok: one failure must not mark the others", body.Checks["temporal"])
	}
	if strings.Contains(raw, "refused") || strings.Contains(raw, "synthetic") {
		t.Errorf("response leaks the failure cause or the connection string: %s", raw)
	}
}

func TestReadinessFailsWhenTemporalIsDown(t *testing.T) {
	c, err := temporal.NewClient(closedAddr(t), "default", discardLogger())
	if err != nil {
		t.Fatalf("NewClient must not need a reachable server: %v", err)
	}
	t.Cleanup(c.Close)

	handler := health.Readiness(discardLogger(), probeTimeout,
		stubChecker{name: "postgres"},
		temporal.NewChecker(c),
	)
	code, body, _ := probe(t, handler)

	if code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", code)
	}
	if body.Checks["temporal"] != "unavailable" {
		t.Errorf("temporal check = %q, want unavailable", body.Checks["temporal"])
	}
	if body.Checks["postgres"] != "ok" {
		t.Errorf("postgres check = %q, want ok", body.Checks["postgres"])
	}
}

type hangingChecker struct{}

func (hangingChecker) Name() string { return "slow" }
func (hangingChecker) Check(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestReadinessFailsWhenADependencyDoesNotAnswerInTime(t *testing.T) {
	handler := health.Readiness(discardLogger(), 50*time.Millisecond, hangingChecker{})

	start := time.Now()
	code, body, _ := probe(t, handler)

	if code != http.StatusServiceUnavailable || body.Checks["slow"] != "unavailable" {
		t.Errorf("status = %d, check = %q; want 503 and unavailable", code, body.Checks["slow"])
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("probe took %v, the timeout did not bound it", elapsed)
	}
}

func TestReadinessSucceedsWhenEveryDependencyAnswers(t *testing.T) {
	handler := health.Readiness(discardLogger(), probeTimeout,
		stubChecker{name: "postgres"},
		stubChecker{name: "temporal"},
	)
	code, body, _ := probe(t, handler)

	if code != http.StatusOK || body.Status != "ok" {
		t.Errorf("status = %d %q, want 200 ok", code, body.Status)
	}
}

func TestLivenessIgnoresDependencies(t *testing.T) {
	recorder := httptest.NewRecorder()
	health.Liveness().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", recorder.Code)
	}
}
