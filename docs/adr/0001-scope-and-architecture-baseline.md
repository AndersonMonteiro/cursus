# ADR-0001: Scope and architecture baseline

Status: accepted
Date: 2026-11-07

## Context

Cursus starts with a README that describes the lifecycle it implements and the boundaries it keeps. Before any code exists, the technology commitments, the points of variation, and the first version's exclusions need to be recorded in one place so that later ADRs refine them rather than rediscover them.

The project is built and reviewed in sessions, several of them by AI assistants, which makes undocumented decisions easy to lose and easy to re-make differently.

## Decision

### Lifecycle

Seven stages, drawn as a loop: spec, design, build, evaluate, deploy, operate, with operate feeding back into spec, and retire as the exit. Governance spans every stage. An agent changes only by moving through the cycle as a new version.

### Technology commitments

| Concern | Commitment |
|---|---|
| Language | Go for the system. Python only where evaluation tooling requires it. |
| Durable execution | Temporal, used directly, without an abstraction layer over workflows, activities, or signals. |
| Database | Postgres. |
| Observability | OpenTelemetry spans at the model, tool, and policy boundaries, exported over OTLP to Langfuse. |
| Evaluation | promptfoo, driving the HTTP API as a provider. |
| Authentication | A minimal JWT issuer shipped with the project, with four roles: spec author, owner, approver, client service. Token verification is behind an interface so the issuer can be replaced. |
| Interface | Server-rendered, three screens: agents, approval queue, run view. |
| Local infrastructure | Postgres, Temporal, and Langfuse under Docker Compose. |
| Supply chain | Images signed with cosign; CI runs static analysis, vulnerability scanning, secrets scanning, SBOM generation, and image scanning. |

### API shape

Runs are asynchronous. A client starts a run and receives an identifier; completion and approval requests are delivered by webhook and can be polled. A run may wait for a human for days, so a synchronous API would be wrong for the common case.

### Identity

Spec versions are created in `draft` by clients. Promotion to `active` requires passing evaluations and an owner's approval. Approvals of paused runs come either from a Cursus user through the built-in queue or from a client service asserting the approver's identity; the audit log records both the asserted approver and the asserting credential. Dual control across delegated identities is the client's responsibility.

### Runs and delegation

Every run records its parent run and delegation depth. Budgets and policies are evaluated against the tree of runs, so a delegated agent cannot hold more authority than the run that delegated to it. Invoking another agent is a tool whose adapter starts a child workflow.

### Points of variation

| Point | Decision |
|---|---|
| Model provider | Interface, one provider, a fake in tests |
| Token verification | Interface, minimal issuer first |
| Tool type | Registry: HTTP, payment, agent as tool; MCP when an agent needs it |
| Guardrail | Interface, built-ins plus client hook |
| Run trigger | A `source` field on the run; API first, webhook and cron named |
| Memory | Working memory only; an injection point exists for episodic memory later |
| Durable engine | Temporal, without an abstraction layer |
| Database | Postgres; repository interfaces exist for testing only |

### Excluded from the first version

Multi-provider routing by strategy, long-term and episodic memory, speech, MCP tool sources, parallel and conditional orchestration beyond child workflows, a full operations frontend, Redis, Kafka, Kubernetes. Each may enter in a later phase when an agent requires it, with its own ADR.

### Boundary

Cursus owns the mechanism of the lifecycle: spec format, versions, states, run execution, guardrail engine, tool contract, approval mechanics, evaluation runner, audit, and telemetry emission. Clients own the content: task, limits, rules, tool implementations, approvers, datasets, and business alerting. The demo client is a separate service under `examples/` that uses only the API.

## Alternatives

**A lighter durable-execution library such as DBOS or Restate.** Both reduce operational footprint. Temporal was chosen for maturity, the author's prior experience, and the fit of its workflow, activity, and signal model to agent runs with human approval. The alternatives remain candidates if the operational cost of Temporal proves too high for the demo.

**Abstracting the durable engine behind an interface.** Rejected: workflows and signals are not a surface that can be swapped without rewriting the runtime, so the interface would be a false seam.

**Reusing an external identity provider for the demo.** Rejected for the first version so that the repository runs without a dependency on another project. The verification interface keeps the option open.

**A synchronous run API.** Rejected because runs that wait for approval can last longer than any reasonable request timeout.

**Writing the evaluation harness in Go.** Rejected: the evaluation ecosystem is in Python and TypeScript, and the HTTP boundary makes the language of the harness irrelevant to the system.

## Consequences

The runtime cannot run without Temporal, and a change of durable engine means a rewrite of the runtime package. Local development needs three services under Compose. Evaluation requires a Python or Node toolchain alongside Go. The identity model allows a client to assert approvers, which moves part of dual-control assurance to the client and must stay visible in the security model.

In return, approvals, retries, timeouts, and compensation are provided by the engine rather than written here; every stage of the lifecycle has a defined artifact; and the system stays small enough to read.
