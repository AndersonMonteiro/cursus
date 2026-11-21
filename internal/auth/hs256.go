package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MinKeyBytes is the key size RFC 7518 section 3.2 requires for HS256.
const MinKeyBytes = 32

type tokenClaims struct {
	Tenant string   `json:"tenant"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

func checkKey(key []byte) error {
	if len(key) < MinKeyBytes {
		return fmt.Errorf("signing key has %d bytes, HS256 needs at least %d", len(key), MinKeyBytes)
	}
	return nil
}

// Issuer mints the HS256 tokens that HS256Verifier accepts.
type Issuer struct {
	key []byte
	now func() time.Time
}

// NewIssuer returns an issuer signing with key. now is time.Now outside tests.
func NewIssuer(key []byte, now func() time.Time) (*Issuer, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	return &Issuer{key: key, now: now}, nil
}

// Issue applies the rules the verifier enforces, so the issuer cannot mint a
// token that its own verifier would refuse.
func (i *Issuer) Issue(subject, tenant string, roles []Role, ttl time.Duration) (string, error) {
	if subject == "" {
		return "", errors.New("subject is empty")
	}
	if tenant == "" {
		return "", errors.New("tenant is empty")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("ttl must be positive, got %s", ttl)
	}
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		if _, err := ParseRole(string(role)); err != nil {
			return "", err
		}
		names = append(names, string(role))
	}

	now := i.now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims{
		Tenant: tenant,
		Roles:  names,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})
	return token.SignedString(i.key)
}

// HS256Verifier is the Verifier for tokens minted by Issuer.
type HS256Verifier struct {
	key    []byte
	parser *jwt.Parser
}

// NewHS256Verifier returns a verifier for tokens signed with key. now is
// time.Now outside tests.
func NewHS256Verifier(key []byte, now func() time.Time) (*HS256Verifier, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	return &HS256Verifier{
		key: key,
		// The algorithm is pinned: without it the header of the token would
		// choose how the token is verified, "none" included.
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(),
			jwt.WithTimeFunc(now),
		),
	}, nil
}

func (v *HS256Verifier) Verify(_ context.Context, token string) (Claims, error) {
	var parsed tokenClaims
	if _, err := v.parser.ParseWithClaims(token, &parsed, func(*jwt.Token) (any, error) {
		return v.key, nil
	}); err != nil {
		return Claims{}, err
	}

	if parsed.Subject == "" {
		return Claims{}, errors.New("token has no subject")
	}
	if parsed.Tenant == "" {
		return Claims{}, errors.New("token has no tenant")
	}
	roles := make([]Role, 0, len(parsed.Roles))
	for _, name := range parsed.Roles {
		role, err := ParseRole(name)
		if err != nil {
			return Claims{}, err
		}
		roles = append(roles, role)
	}

	return Claims{
		Subject:   parsed.Subject,
		Tenant:    parsed.Tenant,
		Roles:     roles,
		ExpiresAt: parsed.ExpiresAt.Time,
	}, nil
}
