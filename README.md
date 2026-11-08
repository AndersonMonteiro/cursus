# Cursus

Cursus takes an AI agent through its whole lifecycle, from the first written specification to the day it is retired, and leaves a verifiable artifact at every stage. It is a reference implementation: a small system, built mostly from established tools, where every decision has a written rationale and every security control is enforced by the pipeline rather than by convention.

The project exists because the pieces of this lifecycle are available today but scattered. Evaluation, tracing, durable execution, and supply-chain security each have mature tooling. What is missing is a readable example of them working together on one agent, end to end.

## The lifecycle

```
spec -> design -> build -> evaluate -> deploy -> operate
  ^                                                 |
  +------------------- iterate ---------------------+
                                                    |
                                                 retire

govern (every stage): owner, risk tier, version, approved tools,
                      evidence for each state change
```

An agent changes by moving through the cycle again as a new version. Production behaviour is never adjusted in place, which is what makes the audit trail meaningful.

Each stage has an artifact a reader can inspect and a security control the pipeline enforces.

| Stage | Artifact | Control |
|---|---|---|
| Spec | `agent.yaml`, threat model, version hash | A spec without a threat model and risk tier does not merge |
| Design | Flow diagram and an ADR on how much autonomy the agent gets | Tool and domain allowlists per version |
| Build | The loop, the workflow, tools, guardrails, payment control, `make demo` | Static analysis, vulnerability and secrets scanning, SBOM, image scan, a test per guardrail |
| Evaluate | Dataset, scorers, red-team report | Promotion is blocked on adversarial failures |
| Deploy | Promotion job, signed image, canary, one rejected and one approved PR | Only signed and evaluated artifacts are promoted |
| Operate | Traces, cost per run, alerts, human annotations feeding the dataset | Detection of guardrail block rate, anomalous spend, repeated denials |
| Retire | State change, credential revocation, archived runs | The retirement job fails while any credential remains active |

## What an agent looks like here

An agent in Cursus has a task with a measurable success criterion, a bounded set of tools, a step and cost budget, declared untrusted inputs, and a risk tier that decides when a human must approve an action. It makes decisions at runtime within those bounds; tasks that can be solved by a fixed sequence of calls are expected to be designed that way instead.

The first agent to go through the cycle is documented in `docs/lifecycle/` as it progresses. A second agent, in a different domain, follows once the first has completed the cycle, to show that the lifecycle holds beyond the case it was built around.

## Where Cursus ends and the client begins

Cursus provides the mechanism of the lifecycle. The system that needs an agent, the client, provides the content and keeps ownership of its own domain.

| Cursus | Client |
|---|---|
| Spec format, validation, versions, lifecycle states | Task, success criteria, limits, risk tier |
| Agent identity, run attribution, audit log | End-user identity, passed as claims |
| Guardrail engine, built-in checkers, hook for external checkers | Which rules apply, with what thresholds, plus any custom checker |
| Tool contract, dispatch, retries, idempotency, approval mechanics | Tool implementations, who approves, how they are notified |
| Evaluation runner, gate, dataset format | Domain dataset and rubric |
| Trace and event emission | Business alerts and the response to them |

A demo client lives under `examples/` as a separate service that talks to Cursus only through the API, so the boundary is visible in code rather than described.

## Identity and approvals

Clients create spec versions in `draft` on their own. Promotion to `active` requires passing evaluations and an owner's approval, which is where control is exercised.

When a run pauses for human approval, the approval can come from a Cursus user through the built-in queue, or from a client service asserting who approved on its side. The audit log records both the asserted approver and the credential that made the assertion. Dual control across delegated identities is the client's responsibility and is stated as such in the security model.

The minimal token issuer that ships with Cursus knows four roles: spec author, owner, approver, and client service.

## Architecture

The control plane parses and validates specs, keeps immutable versions identified by content hash, and drives the state machine from `draft` through `active` and `deprecated` to `retired`.

The runtime is a Temporal workflow per run. Model calls, tool executions, and policy evaluations are activities; approvals are signals; compensation follows the saga pattern. The decision function itself, which maps the current state to the next action, is a pure Go function that fits on one screen. Runs record their parent run and delegation depth so that budgets and policies apply to a whole tree of agents, not to each run in isolation.

Tools follow one contract with input and output schemas, are registered by type, and are dispatched with an idempotency key derived from the run and step. The HTTP adapter validates targets against an SSRF policy and pins DNS resolution at dispatch.

Guardrails run as a checker pipeline before and after each model call, with built-in checkers and a hook for the client's own. Payment control is a declarative policy evaluated before any simulated transfer, backed by an intent state machine and an append-only ledger.

Observability uses OpenTelemetry spans at three boundaries, model, tool, and policy, exported to Langfuse. The audit log is a separate append-only store. Evaluations run through promptfoo against the HTTP API. The interface is server-rendered and limited to three screens: agents, approval queue, and run view.

Local infrastructure is Postgres, Temporal, and Langfuse under Docker Compose. CI runs build, vet, lint, tests, gosec, govulncheck, gitleaks, SBOM generation, image scanning, and signs images with cosign. Redis, Kafka, and Kubernetes are absent until an agent needs them.

### Points of variation

| Point | Decision |
|---|---|
| Model provider | Interface, one provider, a fake in tests |
| Token verification | Interface, minimal issuer first |
| Tool type | Registry: HTTP, payment, agent as tool; MCP when an agent needs it |
| Guardrail | Interface, built-ins plus client hook |
| Run trigger | A `source` field; API first, webhook and cron named |
| Memory | Working memory only; an injection point exists for episodic memory later |
| Durable engine | Temporal, without an abstraction layer |
| Database | Postgres; repository interfaces exist for testing only |

## How the work is done

Every pull request carries one concept and one acceptance criterion, and is merged only after a walkthrough in which the author explains the change and the reviewer writes a short note, in their own words, on what it does and why it was done this way. Those notes are kept with the PR and are the raw material for the project's write-ups. There is no size limit on a PR; the walkthrough is what decides whether a change was understood.

Each mechanism enters with a one-page ADR and a Go test that shows its behaviour under failure, not only on the happy path. Work is planned per phase in `docs/plan/`, and each phase ends with its artifacts in `docs/lifecycle/` and a post grounded in them.

Slices of work are marked core or periphery. Core covers the loop, policy, identity, audit, and payment control. Periphery covers adapters, checkers, datasets, scorers, documentation, and the second agent, and is open to contributors once phase 1 is complete; the walkthrough note is the contribution format. Comments in code explain decisions that are not obvious from reading it; the project's purpose is stated here and not repeated elsewhere.

All data in the repository is synthetic.

Licensed under Apache 2.0.
