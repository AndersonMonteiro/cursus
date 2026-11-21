package auth

import "context"

// Verifier is the seam for replacing the issuer (ADR-0001, ADR-0002). An
// implementation returns an error for any token it does not accept; callers
// do not distinguish between reasons.
type Verifier interface {
	Verify(ctx context.Context, token string) (Claims, error)
}
