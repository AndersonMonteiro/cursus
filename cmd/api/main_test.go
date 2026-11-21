package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AndersonMonteiro/cursus/internal/auth"
	"github.com/AndersonMonteiro/cursus/internal/health"
)

type staticVerifier struct{ accepted string }

func (s staticVerifier) Verify(_ context.Context, token string) (auth.Claims, error) {
	if token != s.accepted {
		return auth.Claims{}, errors.New("not the accepted token")
	}
	return auth.Claims{Subject: "dev-user", Tenant: "demo"}, nil
}

func testRoutes() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return routes(logger, staticVerifier{accepted: "good"}, health.Readiness(logger, time.Second))
}

func get(handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestHealthEndpointsNeedNoToken(t *testing.T) {
	handler := testRoutes()
	for _, path := range []string{"/healthz", "/readyz"} {
		if recorder := get(handler, http.MethodGet, path, ""); recorder.Code != http.StatusOK {
			t.Errorf("GET %s without a token: status = %d, want 200", path, recorder.Code)
		}
	}
}

func TestEveryOtherPathNeedsAToken(t *testing.T) {
	handler := testRoutes()
	requests := []struct{ method, path string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/anything"},
		{http.MethodGet, "/healthz/extra"},
		{http.MethodPost, "/healthz"},
	}
	for _, request := range requests {
		for _, token := range []string{"", "bad"} {
			recorder := get(handler, request.method, request.path, token)

			if recorder.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with token %q: status = %d, want 401", request.method, request.path, token, recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), `"code":"unauthenticated"`) {
				t.Errorf("%s %s: body %q is not the error envelope", request.method, request.path, recorder.Body.String())
			}
		}
	}
}

func TestAValidTokenPassesAuthentication(t *testing.T) {
	// No authenticated route exists yet, so reaching the inner mux shows as
	// its 404 instead of the middleware's 401.
	recorder := get(testRoutes(), http.MethodGet, "/anything", "good")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
}
