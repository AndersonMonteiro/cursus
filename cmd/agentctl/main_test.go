package main

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AndersonMonteiro/cursus/internal/auth"
	"github.com/AndersonMonteiro/cursus/internal/config"
)

const testKey = "test-signing-key-of-thirty-two-bytes!"

func env(key string) func(string) string {
	return func(name string) string {
		if name == config.EnvTokenSigningKey {
			return key
		}
		return ""
	}
}

func TestTokenMintsWhatTheVerifierAccepts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"token", "--subject", "dev-user", "--tenant", "demo", "--role", "owner", "--role", "approver", "--ttl", "5m"}

	if code := run(args, env(testKey), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}

	verifier, err := auth.NewHS256Verifier([]byte(testKey), time.Now)
	if err != nil {
		t.Fatalf("NewHS256Verifier: %v", err)
	}
	claims, err := verifier.Verify(context.Background(), strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("the minted token does not verify: %v", err)
	}
	if claims.Subject != "dev-user" || claims.Tenant != "demo" {
		t.Errorf("subject and tenant = %q and %q", claims.Subject, claims.Tenant)
	}
	if !slices.Equal(claims.Roles, []auth.Role{auth.RoleOwner, auth.RoleApprover}) {
		t.Errorf("roles = %v", claims.Roles)
	}
	if remaining := time.Until(claims.ExpiresAt); remaining <= 0 || remaining > 5*time.Minute {
		t.Errorf("token expires in %s, want at most 5m", remaining)
	}
}

func TestTokenDefaultsToOneHour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"token", "--subject", "dev-user", "--tenant", "demo", "--role", "owner"}

	if code := run(args, env(testKey), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}

	verifier, _ := auth.NewHS256Verifier([]byte(testKey), time.Now)
	claims, err := verifier.Verify(context.Background(), strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("the minted token does not verify: %v", err)
	}
	if remaining := time.Until(claims.ExpiresAt); remaining < 59*time.Minute || remaining > time.Hour {
		t.Errorf("token expires in %s, want about 1h", remaining)
	}
}

func TestTokenRefusesIncompleteOrInvalidInput(t *testing.T) {
	complete := []string{"token", "--subject", "dev-user", "--tenant", "demo", "--role", "owner"}
	cases := map[string]struct {
		args []string
		key  string
		want string
	}{
		"key not set":     {complete, "", config.EnvTokenSigningKey},
		"key too short":   {complete, "short-key-value", config.EnvTokenSigningKey},
		"missing subject": {[]string{"token", "--tenant", "demo", "--role", "owner"}, testKey, "--subject"},
		"missing tenant":  {[]string{"token", "--subject", "dev-user", "--role", "owner"}, testKey, "--tenant"},
		"missing role":    {[]string{"token", "--subject", "dev-user", "--tenant", "demo"}, testKey, "--role"},
		"unknown role":    {append(slices.Clone(complete), "--role", "admin"), testKey, "admin"},
		"malformed ttl":   {append(slices.Clone(complete), "--ttl", "soon"), testKey, "ttl"},
		"negative ttl":    {append(slices.Clone(complete), "--ttl", "-1h"), testKey, "ttl"},
		"stray argument":  {append(slices.Clone(complete), "extra"), testKey, "extra"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run(tc.args, env(tc.key), &stdout, &stderr)

			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr %q does not mention %q", stderr.String(), tc.want)
			}
			if tc.key != "" && strings.Contains(stderr.String(), tc.key) {
				t.Errorf("stderr %q repeats the signing key", stderr.String())
			}
		})
	}
}

func TestUnknownCommandAndNoCommandExitWithUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"mint"}, {"version", "extra"}} {
		var stdout, stderr bytes.Buffer

		if code := run(args, env(testKey), &stdout, &stderr); code != 2 {
			t.Errorf("args %v: exit code = %d, want 2", args, code)
		}
		if !strings.Contains(stderr.String(), "usage: agentctl") {
			t.Errorf("args %v: stderr %q has no usage", args, stderr.String())
		}
	}
}
