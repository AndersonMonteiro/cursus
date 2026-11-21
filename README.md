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
| Spec | `agent.yaml`, threat model, version hash | A spec without a threat model and risk tier does not merge, and one that combines private data, untrusted input, and an outbound channel is held to a higher risk tier |
| Design | Flow diagram and an ADR on how much autonomy the agent gets | Tool and domain allowlists per version |
| Build | The loop, the workflow, tools, guardrails, payment control, `make demo` | Static analysis, vulnerability and secrets scanning, SBOM, image scan, a test per guardrail |
| Evaluate | Dataset, scorers, red-team report | Promotion is blocked on adversarial failures, adaptive multi-turn attacks included |
| Deploy | Promotion job, signed image, canary, one rejected and one approved PR | Only signed and evaluated artifacts are promoted |
| Operate | Traces, cost per run, alerts, human annotations feeding the dataset | Detection of guardrail block rate, anomalous spend, repeated denials; the red-team suite is repeated against active versions and a new failure suspends the agent |
| Retire | State change, credential revocation, archived runs | The retirement job fails while any credential remains active |

## What an agent looks like here

An agent in Cursus has a task with a measurable success criterion, a bounded set of tools, a step and cost budget, declared untrusted inputs, and a risk tier that decides when a human must approve an action. It makes decisions at runtime within those bounds; tasks that can be solved by a fixed sequence of calls are expected to be designed that way instead.

The spec also states the shape of the agent's runs and the memory it reads and writes. A run is either a task, which works toward a result and may wait days for an approval, or a session, which holds a conversation turn by turn.

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

Cursus governs the agents it executes, with its own loop. Agents that run on an external harness are outside the project.

A demo client lives under `examples/` as a separate service that talks to Cursus only through the API, so the boundary is visible in code rather than described.

## Identity and approvals

Clients create spec versions in `draft` on their own. Promotion to `active` requires passing evaluations and an owner's approval, which is where control is exercised.

When a run pauses for human approval, the approval can come from a Cursus user through the built-in queue, or from a client service asserting who approved on its side. The audit log records both the asserted approver and the credential that made the assertion. Dual control across delegated identities is the client's responsibility and is stated as such in the security model.

The minimal token issuer that ships with Cursus knows four roles: spec author, owner, approver, and client service.

## Security principles

The model is treated as compromisable. Injected instructions are expected to succeed some of the time, so the controls that decide what an agent can do are deterministic and sit outside the model: tool and domain allowlists, budgets, the approval policy, and the payment policy. Guardrail checkers reduce how often an attack gets through and are never the only barrier.

Three rules follow from this. A spec that combines access to private data, exposure to untrusted input, and a channel to the outside is flagged at validation and held to a higher risk tier. A run that has read untrusted content is marked, and from that point its high-risk tools require human approval; what a delegated agent returns counts as untrusted content for the run that called it. An agent's credentials are scoped to its version, live as briefly as each tool allows, and never enter the model's context.

## Architecture

The control plane accepts a spec as data through the API, validates it there, and keeps immutable versions identified by a hash over the normalized content. The hash covers the model identifier, its parameters, and the contract of every tool, so a change in any of them is a new version; `agent.yaml` is one way of writing a spec. The state machine runs from `draft` through `active` and `deprecated` to `retired`, and an active version can be moved to `suspended` at once, which takes the agent out of service and is recorded as evidence.

The runtime is a Temporal workflow per run, for both shapes. A task is a workflow that runs to a result. A session is a workflow that stays open, receives each message as a request, and delivers the answer as a stream. Model calls, tool executions, and policy evaluations are activities, and compensation follows the saga pattern. A paused run waits for a human approval, for another run, or for an external event; it is resumed by a signal and every wait has a timeout. The decision function itself, which maps the current state to the next action, is a pure Go function that fits on one screen, and an action carries one or more tool calls. Runs record their parent run and delegation depth so that budgets and policies apply to a whole tree of agents, not to each run in isolation. The first version builds tasks, sequential delegation, and the wait for human approval.

Tools follow one contract with input and output schemas, are registered by type, and are dispatched with an idempotency key derived from the run and step. The HTTP adapter validates targets against an SSRF policy and pins DNS resolution at dispatch.

Guardrails run as a checker pipeline before and after each model call, with built-in checkers and a hook for the client's own. Payment control is a declarative policy evaluated before any simulated transfer, backed by an intent state machine and an append-only ledger.

Observability uses OpenTelemetry spans at three boundaries, model, tool, and policy, exported to Langfuse. The audit log is a separate append-only store, chained by hash; it holds references and hashes, and the content they point to lives where it can be erased on request without breaking the chain. Evaluations run through promptfoo against the HTTP API. The interface is server-rendered and limited to three screens: agents, approval queue, and run view.

Local infrastructure is Postgres, Temporal, and Langfuse under Docker Compose. Langfuse requires ClickHouse, Redis, and S3-compatible storage of its own, so the Compose file starts those three for it and Cursus never connects to them. CI runs build, vet, lint, tests, gosec, govulncheck, gitleaks, SBOM generation, image scanning, and signs images with cosign. Cursus itself takes no dependency on Redis, Kafka, or Kubernetes until an agent needs them.

### Points of variation

| Point | Decision |
|---|---|
| Model provider | Interface, one provider, a fake in tests |
| Token verification | Interface, minimal issuer first |
| Run shape | `task` first; `session` named, built when an agent requires it |
| Tool type | Registry: HTTP, payment, agent as tool; retrieval and MCP when an agent needs them |
| Delegation | A child workflow per delegated agent; sequential first, parallel named |
| Wait | Human approval first; another run and an external event named; a timeout on every wait |
| Guardrail | Interface, built-ins plus client hook |
| Run trigger | A `source` field on the run; API first, webhook and cron named |
| Memory | Declared in the spec; working memory first; retrieval and episodic memory built when an agent requires them |
| Durable engine | Temporal, without an abstraction layer |
| Database | Postgres; repository interfaces exist for testing only |

## How the work is done

Every pull request carries one concept and one acceptance criterion, and is merged only after a walkthrough in which the author explains the change and the reviewer writes a short note, in their own words, on what it does and why it was done this way. Those notes are kept with the PR and are the raw material for the project's write-ups. There is no size limit on a PR; the walkthrough is what decides whether a change was understood.

Each mechanism enters with a one-page ADR and a Go test that shows its behaviour under failure, not only on the happy path. Work is planned per phase in `docs/plan/`, and each phase ends with its artifacts in `docs/lifecycle/` and a post grounded in them.

Slices of work are marked core or periphery. Core covers the loop, policy, identity, audit, and payment control. Periphery covers adapters, checkers, datasets, scorers, documentation, and the second agent, and is open to contributors once phase 1 is complete; the walkthrough note is the contribution format. Comments in code explain decisions that are not obvious from reading it; the project's purpose is stated here and not repeated elsewhere.

All data in the repository is synthetic.

Licensed under Apache 2.0.
