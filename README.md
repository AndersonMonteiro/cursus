# Cursus

Cursus takes an AI agent through its whole lifecycle, from the first written specification to the day it is retired, and leaves a verifiable artifact at every stage. It is a reference implementation: a small system, built mostly from established tools, where every decision has a written rationale and every security control is enforced by a machine rather than by convention. The control plane refuses a state transition of an agent whose condition is not met, and the pipeline refuses a change to the system itself.

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

Each stage has an artifact a reader can inspect and a control that is the condition of a state transition. The control plane verifies it when the API receives the request, so an agent meets the same controls whoever provisions it.

| Stage | Artifact | Control |
|---|---|---|
| Spec | The spec, its threat model, the version hash | A version is not created without a threat model and a risk tier; one that combines private data, untrusted input, and an outbound channel is held to a higher risk tier |
| Design | Design record: the reasoning behind the autonomy, run shape, budgets, and allowlists the spec states, and the flow | A version is not created without it; tool and domain allowlists are fixed in the version |
| Build | An immutable version that assembles spec, model, tools, guardrails, and memory | A version is created only when every part validates and every tool declares its effect, whether it touches private data, and whether what it returns is untrusted |
| Evaluate | Dataset, scorers, evaluation evidence, red-team report | Promotion is refused without passing evidence bound to the version hash, with coverage derived from the spec and adaptive multi-turn attacks included |
| Deploy | Promotion record, canary, one refused and one accepted promotion | Only an evaluated version approved by an owner becomes active |
| Operate | Traces, cost per run, alerts, human annotations feeding the dataset | Detection of guardrail block rate, anomalous spend, repeated denials; red-team evidence that fails or passes its validity leads to suspension |
| Retire | State change, credential revocation, archived runs | Retirement is refused while any credential of the agent remains active |

Spec, design, and build are verified together, when a version is created in `draft`. Building an agent means assembling its spec, model, tools, guardrails, and memory into one immutable version.

Cursus has a cycle of its own, which belongs to its repository. Its pipeline builds the system, runs static analysis, vulnerability and secrets scanning, generates the SBOM, scans the image, and signs it. A pipeline that provisions an agent is a client of the API like any other and holds no control that the API does not verify.

## What an agent looks like here

An agent in Cursus has a task with a measurable success criterion, a bounded set of tools, a step and cost budget, declared untrusted inputs, and a risk tier that decides when a human must approve an action. It makes decisions at runtime within those bounds; tasks that can be solved by a fixed sequence of calls are expected to be designed that way instead.

An agent is an entity with versions. A run is addressed to the agent, and the control plane chooses the version that serves it.

The spec states everything that defines the agent's behaviour, including the shape of its runs and the memory it reads and writes. A run is either a task, which works toward a result and may wait days for an approval, or a session, which holds a conversation turn by turn. Every tool in the spec declares its effect, whether it touches private data, and whether what it returns is untrusted.

The first agent to go through the cycle belongs to the demo client, which keeps its spec, threat model, design record, and dataset and provisions it through the API. `docs/lifecycle/` holds the account of each stage as the agent progresses. A second agent, in a different domain, follows once the first has completed the cycle, to show that the lifecycle holds beyond the case it was built around.

## Where Cursus ends and the client begins

Cursus provides the mechanism of the lifecycle. The system that needs an agent, the client, provides the content and keeps ownership of its own domain.

| Cursus | Client |
|---|---|
| Spec format, validation, versions, lifecycle states | Task, success criteria, limits, risk tier, threat model, design record |
| Agent identity, run attribution, audit log | End-user identity, passed as claims |
| Guardrail engine, built-in checkers, hook for external checkers | Which rules apply, with what thresholds, plus any custom checker |
| Tool contract, dispatch, retries, idempotency, approval mechanics | Tool implementations and what each declares, test and production bindings, who approves, how they are notified |
| Sensitive operation policy and the record of each sensitive operation | Which operations are limited and by how much |
| Evaluation harness, evidence record, promotion gate, dataset format | Domain dataset and rubric, the running of evaluations |
| Trace and event emission | Business alerts and the response to them |

Cursus governs the agents it executes, with its own loop. Agents that run on an external harness are outside the project.

A demo client lives under `examples/` as a separate service that talks to Cursus only through the API, so the boundary is visible in code rather than described. It brings its own tools and the files of its agents.

## Identity and approvals

Clients create versions in `draft` on their own. A draft executes only evaluation runs, which use test bindings, and no production trigger starts one. Promotion to `active` requires evaluation evidence bound to the version hash and an owner's approval, which is where control is exercised. The evaluation is run by a client of the API with a credential of its own, and that subject cannot be the one that created the version.

When a run pauses for human approval, the approval can come from a Cursus user through the built-in queue, or from a client service asserting who approved on its side. The audit log records both the asserted approver and the credential that made the assertion. Dual control across delegated identities is the client's responsibility and is stated as such in the security model.

The minimal token issuer that ships with Cursus knows four roles: spec author, owner, approver, and client service.

## Security principles

The model is treated as compromisable. Injected instructions are expected to succeed some of the time, so the controls that decide what an agent can do are deterministic and sit outside the model: tool and domain allowlists, budgets, the approval policy, and the sensitive operation policy. Guardrail checkers reduce how often an attack gets through and are never the only barrier.

Three rules follow from this, and all three read what each tool declares. A spec that combines access to private data, exposure to untrusted input, and a channel to the outside is flagged at validation and held to a higher risk tier. A run records two marks: that it has read untrusted content and that it has read private data. From the first mark on, its tools with a sensitive effect require human approval; what a delegated agent returns counts as untrusted content for the run that called it. An agent's credentials are scoped to its version, live as briefly as each tool allows, never enter the model's context, and are distinct from the tokens that identify callers of the API.

Evidence has a validity. The red-team suite is repeated against active versions by a client of the API, as evaluation runs with test bindings, and the control plane holds every active version to it: evidence that fails or passes its validity leads to suspension.

## Architecture

The control plane accepts a spec as data through the API, validates it there, and keeps the immutable versions of each agent, identified by a hash over the normalized content. The hash covers the model identifier, its parameters, and the contract and declarations of every tool, so a change in any of them is a new version; `agent.yaml` is one way of writing a spec. The threat model and the design record are required when a version is created and are bound to its hash as evidence. They stay outside the hash, so revising them is recorded and leaves what executes unchanged. The state machine runs from `draft` through `active` and `deprecated` to `retired`, and an active version can be moved to `suspended` at once, which takes the agent out of service and is recorded as evidence. Every transition has conditions, and the API refuses one whose conditions are not met.

The runtime is a Temporal workflow per run, for both shapes. A task is a workflow that runs to a result. A session is a workflow that stays open, receives each message as a request, and delivers the answer as a stream. Model calls, tool executions, and policy evaluations are activities, and compensation follows the saga pattern. A paused run waits for a human approval, for another run, or for an external event; it is resumed by a signal and every wait has a timeout. The decision function itself, which maps the current state to the next action, is a pure Go function that fits on one screen, and an action carries one or more tool calls. Runs record their parent run and delegation depth so that budgets and policies apply to a whole tree of agents, not to each run in isolation. A run is an evaluation run or a production run; a draft executes only the first, and an active version accepts both. The first version builds tasks, sequential delegation, and the wait for human approval.

Tools follow one contract with input and output schemas, are registered by type, and are dispatched with an idempotency key derived from the run and step. The address of a tool and the reference to its secret are a binding kept outside the version. A deployment holds two sets of bindings, test and production, and every change of a binding is recorded. The HTTP adapter validates targets against an SSRF policy and pins DNS resolution at dispatch.

Guardrails run as a checker pipeline before and after each model call, with built-in checkers and a hook for the client's own. The sensitive operation policy is declarative and is evaluated before every tool call whose declared effect is sensitive. Each such operation states a quantity and a unit, which Cursus records, and the policy applies limits per call and cumulative limits over a window, to the agent and to the tree of runs.

The evaluation harness is a client of the API. It drives evaluation runs through promptfoo and records the results as evidence against the version hash. The control plane derives from its own records whatever those runs show, such as a tool call refused by the allowlist or a guardrail block, and accepts as asserted only what depends on a judge. The dataset stays with the client; the evidence holds its hash and the results.

Observability uses OpenTelemetry spans at three boundaries, model, tool, and policy, exported to Langfuse. Trace content is masked before export and kept for a stated period. The audit log is a separate append-only store, chained by hash; it holds references and hashes, and the content they point to lives where it can be erased on request without breaking the chain. The interface is server-rendered and limited to three screens: agents, approval queue, and run view.

Local infrastructure is Postgres, Temporal, and Langfuse under Docker Compose. Langfuse requires ClickHouse, Redis, and S3-compatible storage of its own, so the Compose file starts those three for it and Cursus never connects to them. CI runs build, vet, lint, tests, gosec, govulncheck, gitleaks, SBOM generation, image scanning, and signs images with cosign. Cursus itself takes no dependency on Redis, Kafka, or Kubernetes until an agent needs them.

### Points of variation

| Point | Decision |
|---|---|
| Model provider | Interface, one provider, a fake in tests |
| Token verification | Interface, minimal issuer first |
| Run shape | `task` first; `session` named, built when an agent requires it |
| Tool type | Registry: HTTP, agent as tool; retrieval and MCP when an agent needs them |
| Delegation | A child workflow per delegated agent; sequential first, parallel named |
| Wait | Human approval first; another run and an external event named; a timeout on every wait |
| Guardrail | Interface, built-ins plus client hook |
| Run trigger | A `source` field on the run; API first, webhook and cron named |
| Memory | Declared in the spec; working memory first; retrieval and episodic memory built when an agent requires them |
| Durable engine | Temporal, without an abstraction layer |
| Database | Postgres; repository interfaces exist for testing only |

## How the work is done

Every pull request carries one concept and one acceptance criterion, and is merged only after a walkthrough in which the author explains the change and the reviewer writes a short note, in their own words, on what it does and why it was done this way. Those notes are kept with the PR and are the raw material for the project's write-ups. There is no size limit on a PR; the walkthrough is what decides whether a change was understood.

Each mechanism enters with a one-page ADR and a Go test that shows its behaviour under failure, not only on the happy path. Work is planned per phase in `docs/plan/`, and each phase ends with the account of its stage in `docs/lifecycle/` and a post grounded in it.

Slices of work are marked core or periphery. Core covers the loop, policy, identity, and audit. Periphery covers adapters, checkers, datasets, scorers, documentation, and the second agent, and is open to contributors once phase 1 is complete; the walkthrough note is the contribution format. Comments in code explain decisions that are not obvious from reading it; the project's purpose is stated here and not repeated elsewhere.

All data in the repository is synthetic.

Licensed under Apache 2.0.
