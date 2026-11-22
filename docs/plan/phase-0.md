# Phase 0: foundation

Milestone: [Phase 0: foundation](https://github.com/AndersonMonteiro/cursus/milestone/1)

Phase 0 puts in place what every later phase assumes: the documents that define the project, a local environment that starts with one command, a pipeline that refuses insecure changes, and a way to authenticate requests. No agent exists at the end of this phase. What exists is a repository where the next change is tested, scanned, and reviewed before it lands.

The phase closes when a pull request carrying a planted fake secret is blocked by CI, the `main` ruleset requires every CI job to pass, and CI rejects an import of the Temporal SDK outside its package.

## Slices

| # | Slice | Issue |
|---|---|---|
| 1 | Repository skeleton: README, ADR-0001, security policy, PR template | [#1](https://github.com/AndersonMonteiro/cursus/issues/1), merged |
| 2 | Local environment: Compose with Postgres, Temporal, and Langfuse with its backing services (ClickHouse, Redis, MinIO); migrations; API and worker entry points with health endpoints | [#3](https://github.com/AndersonMonteiro/cursus/issues/3) |
| 3 | CI: build, vet, lint, tests against Postgres, static analysis, vulnerability and secrets scanning, SBOM, image scan, linting of the workflow files, dependency updates | [#4](https://github.com/AndersonMonteiro/cursus/issues/4) |
| 4 | Minimal token issuer and verification middleware with tenant and the four roles | [#5](https://github.com/AndersonMonteiro/cursus/issues/5) |
| 5 | Design baseline in README and ADR-0001: run shapes, version identity, security principles, memory, delegation and waiting | [#16](https://github.com/AndersonMonteiro/cursus/issues/16) |
| 6 | Import boundary for the durable engine: the Temporal SDK is imported by one package and the lint rejects any other import | [#17](https://github.com/AndersonMonteiro/cursus/issues/17) |
| 7 | Lifecycle baseline in README and ADR-0001: the cycle of the system and the cycle of the agent, controls as conditions of state transitions, evaluation evidence, and the sensitive operation policy | [#20](https://github.com/AndersonMonteiro/cursus/issues/20) |

Slices 2 and 3 can proceed in either order. Slice 4 depends on 2 for the service skeleton and on 3 for the CI that checks it. Slice 6 follows 5 because both edit ADR-0001, and slice 7 follows 6 for the same reason.

## Out of scope

Agent specs, the runtime loop, tools, guardrails, evaluation, deployment. Any package under `internal/` beyond what slices 2 and 4 need to compile, start, and verify a token.
