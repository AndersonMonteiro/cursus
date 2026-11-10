# Phase 0: foundation

Milestone: [Phase 0: foundation](https://github.com/AndersonMonteiro/cursus/milestone/1)

Phase 0 puts in place what every later phase assumes: the documents that define the project, a local environment that starts with one command, a pipeline that refuses insecure changes, and a way to authenticate requests. No agent exists at the end of this phase. What exists is a repository where the next change is tested, scanned, and reviewed before it lands.

The phase closes when a pull request carrying a planted fake secret is blocked by CI, and the `main` ruleset requires every CI job to pass.

## Slices

| # | Slice | Issue |
|---|---|---|
| 1 | Repository skeleton: README, ADR-0001, security policy, PR template | [#1](https://github.com/AndersonMonteiro/cursus/issues/1), merged |
| 2 | Local environment: Compose with Postgres, Temporal, and Langfuse; migrations; API and worker entry points with health endpoints | [#3](https://github.com/AndersonMonteiro/cursus/issues/3) |
| 3 | CI: build, vet, lint, tests against Postgres, static analysis, vulnerability and secrets scanning, SBOM, image scan, dependency updates | [#4](https://github.com/AndersonMonteiro/cursus/issues/4) |
| 4 | Minimal token issuer and verification middleware with tenant and the four roles | [#5](https://github.com/AndersonMonteiro/cursus/issues/5) |

Slices 2 and 3 can proceed in either order. Slice 4 depends on 2 for the service skeleton and on 3 for the CI that checks it.

## Out of scope

Agent specs, the runtime loop, tools, guardrails, evaluation, deployment. Any package under `internal/` beyond what slices 2 and 4 need to compile, start, and verify a token.
