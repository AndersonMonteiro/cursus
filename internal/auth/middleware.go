package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/AndersonMonteiro/cursus/internal/apierror"
)

const (
	messageUnauthenticated = "missing or invalid token"
	messageForbidden       = "role not permitted"
)

// Authenticate admits a request that carries a token the verifier accepts and
// stores its claims in the request context. Every refusal gets the same
// response, so the body does not tell a caller how close a forged token came;
// the reason is logged.
func Authenticate(verifier Verifier, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				logger.InfoContext(r.Context(), "request refused", "reason", "no bearer token", "path", r.URL.Path)
				unauthenticated(w)
				return
			}
			claims, err := verifier.Verify(r.Context(), token)
			if err != nil {
				logger.InfoContext(r.Context(), "request refused", "reason", err.Error(), "path", r.URL.Path)
				unauthenticated(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(withClaims(r.Context(), claims)))
		})
	}
}

// RequireRole admits a request whose claims hold at least one of roles. It
// must sit inside Authenticate; without claims in the context it answers 401
// instead of letting the request through.
func RequireRole(roles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				unauthenticated(w)
				return
			}
			if !claims.HasAnyRole(roles...) {
				apierror.Write(w, http.StatusForbidden, apierror.CodeForbidden, messageForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

func unauthenticated(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	apierror.Write(w, http.StatusUnauthorized, apierror.CodeUnauthenticated, messageUnauthenticated)
}
