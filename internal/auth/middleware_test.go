package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeVerifier shares nothing with the HS256 implementation. The middleware
// working against it is what shows the issuer can be replaced.
type fakeVerifier struct {
	tokens map[string]Claims
}

func (f fakeVerifier) Verify(_ context.Context, token string) (Claims, error) {
	claims, ok := f.tokens[token]
	if !ok {
		return Claims{}, errors.New("fake: unknown token")
	}
	return claims, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// protected is a route that requires the owner role and reports who called.
func protected(verifier Verifier, logger *slog.Logger) http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, _ := ClaimsFromContext(r.Context())
		_, _ = io.WriteString(w, claims.Subject+"@"+claims.Tenant)
	})
	return Authenticate(verifier, logger)(RequireRole(RoleOwner)(handler))
}

func call(handler http.Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/anything", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertEnvelope(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Errorf("status = %d, want %d", recorder.Code, status)
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body %q is not the envelope: %v", recorder.Body.String(), err)
	}
	if envelope.Error.Code != code || envelope.Error.Message == "" {
		t.Errorf("envelope = %+v, want code %q and a message", envelope.Error, code)
	}
}

func TestMiddlewareWithTheHS256Verifier(t *testing.T) {
	issuer := newTestIssuer(t, testKey, testNow)
	expired := issue(t, newTestIssuer(t, testKey, testNow.Add(-2*time.Hour)), RoleOwner)
	handler := protected(newTestVerifier(t, testNow), discardLogger())

	refused := map[string]string{
		"no header":         "",
		"no scheme":         issue(t, issuer, RoleOwner),
		"basic scheme":      "Basic ZGV2OmRldg==",
		"empty bearer":      "Bearer ",
		"not a token":       "Bearer not-a-token",
		"expired token":     "Bearer " + expired,
		"wrong signature":   "Bearer " + issue(t, newTestIssuer(t, otherKey, testNow), RoleOwner),
		"token then a word": "Bearer " + issue(t, issuer, RoleOwner) + " extra",
	}
	for name, authorization := range refused {
		t.Run(name, func(t *testing.T) {
			recorder := call(handler, authorization)

			assertEnvelope(t, recorder, http.StatusUnauthorized, "unauthenticated")
			if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
		})
	}

	t.Run("missing role", func(t *testing.T) {
		recorder := call(handler, "Bearer "+issue(t, issuer, RoleSpecAuthor, RoleApprover))

		assertEnvelope(t, recorder, http.StatusForbidden, "forbidden")
	})

	t.Run("no roles at all", func(t *testing.T) {
		recorder := call(handler, "Bearer "+issue(t, issuer))

		assertEnvelope(t, recorder, http.StatusForbidden, "forbidden")
	})

	t.Run("valid token with the role", func(t *testing.T) {
		recorder := call(handler, "Bearer "+issue(t, issuer, RoleApprover, RoleOwner))

		if recorder.Code != http.StatusOK || recorder.Body.String() != "dev-user@demo" {
			t.Errorf("status = %d, body = %q", recorder.Code, recorder.Body.String())
		}
	})
}

func TestMiddlewareWithAFakeVerifier(t *testing.T) {
	handler := protected(fakeVerifier{tokens: map[string]Claims{
		"opaque-owner":  {Subject: "ana", Tenant: "acme", Roles: []Role{RoleOwner}},
		"opaque-author": {Subject: "rui", Tenant: "acme", Roles: []Role{RoleSpecAuthor}},
	}}, discardLogger())

	if recorder := call(handler, "Bearer opaque-owner"); recorder.Code != http.StatusOK || recorder.Body.String() != "ana@acme" {
		t.Errorf("owner: status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	assertEnvelope(t, call(handler, "Bearer opaque-author"), http.StatusForbidden, "forbidden")
	assertEnvelope(t, call(handler, "Bearer unknown"), http.StatusUnauthorized, "unauthenticated")
}

func TestRefusalDoesNotSayWhy(t *testing.T) {
	var log strings.Builder
	logger := slog.New(slog.NewTextHandler(&log, nil))
	handler := protected(fakeVerifier{}, logger)

	recorder := call(handler, "Bearer unknown")

	if strings.Contains(recorder.Body.String(), "fake: unknown token") {
		t.Errorf("body %q carries the verifier's reason", recorder.Body.String())
	}
	if !strings.Contains(log.String(), "fake: unknown token") {
		t.Errorf("log %q does not carry the verifier's reason", log.String())
	}
}

func TestRequireRoleWithoutAuthenticateFailsClosed(t *testing.T) {
	reached := false
	handler := RequireRole(RoleOwner)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))

	recorder := call(handler, "")

	assertEnvelope(t, recorder, http.StatusUnauthorized, "unauthenticated")
	if reached {
		t.Error("the handler ran without claims in the context")
	}
}
