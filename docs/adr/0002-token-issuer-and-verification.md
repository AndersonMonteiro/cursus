# ADR-0002: Token issuer and verification

Status: accepted
Date: 2026-11-20

## Context

ADR-0001 commits the project to a minimal JWT issuer of its own, with four roles, and names token verification as a point of variation behind an interface. The API so far serves only its health endpoints. Before the first endpoint that changes state exists, a request needs an identity the system can verify without calling another service, carrying the tenant and the roles that later decisions depend on.

This ADR fixes the token format, the claims, how the key is provided, how the API answers a request it does not accept, and what replacing the issuer would take.

## Decision

### Tokens

Tokens are JWTs signed with HS256. The issuer and the verifier share one key, read from the environment variable `CURSUS_TOKEN_SIGNING_KEY` as a raw string of at least 32 bytes, the minimum RFC 7518 sets for HS256. The variable has no default, and the API does not start without it or with a shorter key.

| Claim | Meaning |
|---|---|
| `sub` | Subject: the user or service the token was issued to |
| `tenant` | Tenant the subject acts for |
| `roles` | Array with any of `spec_author`, `owner`, `approver`, `client_service` |
| `exp` | Expiry |
| `iat` | Time of issue, informational |

A token is accepted when its signature verifies with the key, its algorithm is HS256, `exp` is present and in the future, `sub` and `tenant` are not empty, and every entry of `roles` is one of the four known values. No clock leeway is applied. A token that fails any of these is treated the same as no token.

### Verification seam

Verification is the interface `auth.Verifier` in `internal/auth`, with one method that takes the token string and returns `auth.Claims` (subject, tenant, roles, expiry) or an error. The HS256 verifier is its first implementation. The middleware and every handler depend on the interface and on `auth.Claims`, never on the JWT library.

### Middleware and errors

`auth.Authenticate` reads `Authorization: Bearer <token>`, verifies it, and places the claims in the request context. `auth.RequireRole` admits a request whose claims hold at least one of the roles a route names. A missing or invalid token is answered with 401 and `WWW-Authenticate: Bearer`; a valid token without a required role is answered with 403. Both use the envelope that every API error uses from here on, written by `internal/apierror`:

```json
{"error": {"code": "unauthenticated", "message": "missing or invalid token"}}
```

The 401 body never states why a token was refused; the reason goes to the log. `/healthz` and `/readyz` stay outside the middleware. Every other path of the API is behind it, so a route added later is authenticated unless it is deliberately mounted outside.

### Issuing

`agentctl token --subject <s> --tenant <t> --role <r> [--role <r>] [--ttl 1h]` mints a token with the same key and prints it. It is the only issuer. There is no user store, no login endpoint, no refresh token, and no revocation.

### Tenancy and deployments

Tenant is a claim from the first token, and every later table and query is scoped by it. Isolation between organizations that must not share infrastructure is achieved by running one deployment for each, with its own database and its own signing key. A token minted for one deployment fails signature verification in another, which is why tokens carry no `iss` or `aud` claim.

## Alternatives

**Asymmetric signatures (RS256 or EdDSA) with a published key set.** The verifier would hold only a public key, so a leaked API configuration could not mint tokens. With the issuer and the verifier in the same repository, operated by the same person, the split protects nothing yet and adds key distribution. It is the natural first replacement and fits behind the interface.

**An external identity provider.** Already rejected for the first version in ADR-0001 so that the repository runs on its own. The interface keeps it open.

**Opaque tokens looked up in Postgres.** They give revocation for free, at the cost of a table, a query per request, and an issuing endpoint, which is user management by another name and out of scope for this phase.

**One shared deployment with isolation by tenant claim alone.** Every query would be the only barrier between organizations, and one missing predicate would cross it. Keeping the claim and also separating deployments means a scoping bug stays inside one organization's data.

## Consequences

Any request can be attributed to a subject, a tenant, and a set of roles with no network call, and the audit log has an identity to record from its first entry.

Whoever holds the signing key can mint a token for any tenant and any role, so the key is as sensitive as the database credentials and is provided the same way. Tokens cannot be revoked before they expire, which is why the development issuer defaults to a lifetime of one hour. Rotating the key requires a restart and invalidates every token in circulation at once.

Replacing the issuer requires a new implementation of `auth.Verifier` that validates the provider's tokens and maps its claims to `auth.Claims`, including where tenant and the four roles come from, and one changed line in `cmd/api` where the verifier is constructed. The middleware, the handlers, and the error envelope do not change. `agentctl token` and the signing key variable are removed when the last HS256 token is no longer accepted.
