package auth

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	testKey  = []byte("test-signing-key-of-thirty-two-bytes!")
	otherKey = []byte("another-signing-key-of-enough-bytes!!")
	testNow  = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func newTestIssuer(t *testing.T, key []byte, now time.Time) *Issuer {
	t.Helper()
	issuer, err := NewIssuer(key, fixedClock(now))
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	return issuer
}

func newTestVerifier(t *testing.T, now time.Time) *HS256Verifier {
	t.Helper()
	verifier, err := NewHS256Verifier(testKey, fixedClock(now))
	if err != nil {
		t.Fatalf("NewHS256Verifier: %v", err)
	}
	return verifier
}

func issue(t *testing.T, issuer *Issuer, roles ...Role) string {
	t.Helper()
	token, err := issuer.Issue("dev-user", "demo", roles, time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return token
}

// sign builds a token the Issuer would refuse to mint, to show the verifier
// does not rely on the issuer's own checks.
func sign(t *testing.T, method jwt.SigningMethod, key any, claims jwt.Claims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return token
}

func TestVerifyAcceptsAValidToken(t *testing.T) {
	token := issue(t, newTestIssuer(t, testKey, testNow), RoleOwner, RoleApprover)

	claims, err := newTestVerifier(t, testNow.Add(59*time.Minute)).Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "dev-user" || claims.Tenant != "demo" {
		t.Errorf("subject and tenant = %q and %q", claims.Subject, claims.Tenant)
	}
	if !slices.Equal(claims.Roles, []Role{RoleOwner, RoleApprover}) {
		t.Errorf("roles = %v", claims.Roles)
	}
	if want := testNow.Add(time.Hour); !claims.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", claims.ExpiresAt, want)
	}
}

func TestVerifyRejectsAnExpiredToken(t *testing.T) {
	token := issue(t, newTestIssuer(t, testKey, testNow), RoleOwner)

	_, err := newTestVerifier(t, testNow.Add(time.Hour+time.Second)).Verify(context.Background(), token)
	if err == nil {
		t.Fatal("Verify accepted a token one second past its expiry")
	}
}

func TestVerifyRejectsAWrongSignature(t *testing.T) {
	token := issue(t, newTestIssuer(t, otherKey, testNow), RoleOwner)

	if _, err := newTestVerifier(t, testNow).Verify(context.Background(), token); err == nil {
		t.Fatal("Verify accepted a token signed with another key")
	}
}

func TestVerifyRejectsATamperedPayload(t *testing.T) {
	issuer := newTestIssuer(t, testKey, testNow)
	asAuthor := strings.Split(issue(t, issuer, RoleSpecAuthor), ".")
	asOwner := strings.Split(issue(t, issuer, RoleOwner), ".")
	// The payload of one token with the signature of another.
	forged := strings.Join([]string{asAuthor[0], asOwner[1], asAuthor[2]}, ".")

	if _, err := newTestVerifier(t, testNow).Verify(context.Background(), forged); err == nil {
		t.Fatal("Verify accepted a payload that does not match its signature")
	}
}

func TestVerifyRejectsOtherAlgorithms(t *testing.T) {
	claims := tokenClaims{
		Tenant: "demo",
		Roles:  []string{string(RoleOwner)},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "dev-user",
			ExpiresAt: jwt.NewNumericDate(testNow.Add(time.Hour)),
		},
	}
	tokens := map[string]string{
		"none":  sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims),
		"HS384": sign(t, jwt.SigningMethodHS384, testKey, claims),
	}

	verifier := newTestVerifier(t, testNow)
	for name, token := range tokens {
		if _, err := verifier.Verify(context.Background(), token); err == nil {
			t.Errorf("Verify accepted a token with algorithm %s", name)
		}
	}
}

func TestVerifyRejectsIncompleteClaims(t *testing.T) {
	complete := func() tokenClaims {
		return tokenClaims{
			Tenant: "demo",
			Roles:  []string{string(RoleOwner)},
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "dev-user",
				ExpiresAt: jwt.NewNumericDate(testNow.Add(time.Hour)),
			},
		}
	}
	cases := map[string]func(*tokenClaims){
		"missing tenant":  func(c *tokenClaims) { c.Tenant = "" },
		"missing subject": func(c *tokenClaims) { c.Subject = "" },
		"missing expiry":  func(c *tokenClaims) { c.ExpiresAt = nil },
		"unknown role":    func(c *tokenClaims) { c.Roles = append(c.Roles, "admin") },
	}

	verifier := newTestVerifier(t, testNow)
	if _, err := verifier.Verify(context.Background(), sign(t, jwt.SigningMethodHS256, testKey, complete())); err != nil {
		t.Fatalf("Verify rejected the complete claims the cases start from: %v", err)
	}
	for name, remove := range cases {
		t.Run(name, func(t *testing.T) {
			claims := complete()
			remove(&claims)
			token := sign(t, jwt.SigningMethodHS256, testKey, claims)

			if _, err := verifier.Verify(context.Background(), token); err == nil {
				t.Fatal("Verify accepted the token")
			}
		})
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	verifier := newTestVerifier(t, testNow)
	for _, token := range []string{"", "not-a-token", "a.b.c"} {
		if _, err := verifier.Verify(context.Background(), token); err == nil {
			t.Errorf("Verify accepted %q", token)
		}
	}
}

func TestShortKeysAreRejected(t *testing.T) {
	short := []byte(strings.Repeat("k", MinKeyBytes-1))

	if _, err := NewIssuer(short, time.Now); err == nil {
		t.Error("NewIssuer accepted a key under the minimum length")
	}
	if _, err := NewHS256Verifier(short, time.Now); err == nil {
		t.Error("NewHS256Verifier accepted a key under the minimum length")
	}
}

func TestIssueRefusesWhatTheVerifierWouldReject(t *testing.T) {
	issuer := newTestIssuer(t, testKey, testNow)
	cases := map[string]func() (string, error){
		"empty subject": func() (string, error) { return issuer.Issue("", "demo", []Role{RoleOwner}, time.Hour) },
		"empty tenant":  func() (string, error) { return issuer.Issue("dev-user", "", []Role{RoleOwner}, time.Hour) },
		"unknown role":  func() (string, error) { return issuer.Issue("dev-user", "demo", []Role{"admin"}, time.Hour) },
		"zero ttl":      func() (string, error) { return issuer.Issue("dev-user", "demo", []Role{RoleOwner}, 0) },
	}
	for name, mint := range cases {
		if token, err := mint(); err == nil {
			t.Errorf("%s: Issue returned %q", name, token)
		}
	}
}
