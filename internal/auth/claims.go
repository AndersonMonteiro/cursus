// Package auth verifies the token a request carries and enforces the role a
// route requires. ADR-0002 records the token format and the claims.
package auth

import (
	"context"
	"fmt"
	"slices"
	"time"
)

type Role string

const (
	RoleSpecAuthor    Role = "spec_author"
	RoleOwner         Role = "owner"
	RoleApprover      Role = "approver"
	RoleClientService Role = "client_service"
)

// ParseRole rejects any value outside the four roles, so a token or a command
// line cannot introduce a role the system does not know.
func ParseRole(value string) (Role, error) {
	switch role := Role(value); role {
	case RoleSpecAuthor, RoleOwner, RoleApprover, RoleClientService:
		return role, nil
	default:
		return "", fmt.Errorf("unknown role %q", value)
	}
}

// Claims is what the rest of the system knows about the caller. It carries
// nothing specific to JWT, so a Verifier for another token format fills the
// same struct.
type Claims struct {
	Subject   string
	Tenant    string
	Roles     []Role
	ExpiresAt time.Time
}

func (c Claims) HasAnyRole(roles ...Role) bool {
	return slices.ContainsFunc(roles, func(role Role) bool {
		return slices.Contains(c.Roles, role)
	})
}

type claimsKey struct{}

func withClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

// ClaimsFromContext returns the claims Authenticate stored for the request.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(Claims)
	return claims, ok
}
