# ADR-0001: Scope and architecture baseline

Status: accepted
Date: 2026-11-07

## Context

Cursus starts with a README that describes the lifecycle it implements and the boundaries it keeps. Before any code exists, the technology commitments, the points of variation, and the first version's exclusions need to be recorded in one place so that later ADRs refine them rather than rediscover them.

The project is built and reviewed in sessions, several of them by AI assistants, which makes undocumented decisions easy to lose and easy to re-make differently.

## Decision

### Lifecycle

Seven stages, drawn as a loop: spec, design, build, evaluate, deploy, operate, with operate feeding back into spec, and retire as the exit. Governance spans every stage. An agent is an entity with versions, and it changes only by moving through the cycle as a new version. A production run is addressed to the agent and the control plane chooses the version that serves it; an evaluation run names the version it evaluates. How versions of one agent serve side by side, canary, and rollback are set in the phase 5 plan.

A version is `draft`, `active`, `suspended`, `deprecated`, or `retired`. Suspension takes an active version out of service at once and is recorded as evidence. The transitions and who may suspend are set in the phase 1 plan.

Every control of this lifecycle is a condition of a state transition, verified by the control plane when the API receives the request. A pipeline, a command line, and a service are clients of that API and hold no control of their own, so an agent meets the same conditions whoever provisions it, and the first agent follows the same path as any other. Spec, design, and build are verified together when a version is created: building an agent is assembling its spec, model, tools, guardrails, and memory into one immutable version and validating the whole.

Cursus has a cycle of its own, which belongs to its repository on GitHub, where its pipeline builds, tests, scans, and signs the system. That pipeline governs changes to Cursus and takes no part in the lifecycle of an agent.

### Version identity

The API accepts a spec as data and validates it; `agent.yaml` is one way of writing a spec. Everything that defines the behaviour of an agent is in the spec: the instructions, the model and its parameters, the tools, the guardrails, the memory, the run shape, the budgets, the allowlists, the approval policy, and the limits of the sensitive operation policy. A version is identified by a hash over the normalized content of the spec, so two writings of the same spec are the same version. For every tool the agent may call, the hash covers the input and output schemas and three declarations: the effect of the tool, whether it touches private data, and whether what it returns is untrusted. A model retired by its provider, or a tool whose contract or declarations change, therefore produces a new version, which is evaluated and approved like any other. A tool whose behaviour changes under an unchanged contract is the client's responsibility, and so is the truth of what a tool declares.

The address of a tool and the reference to its secret are a binding, kept outside the version so that one version serves under more than one binding. A deployment holds two sets of bindings, test and production, both configuration of the same deployment. The domain allowlist of a version bounds both sets, and a binding whose address lies outside it is refused. Every change of a binding is recorded, and a change to a production binding requires an owner's approval.

A version is created only with a threat model and a design record. The design record holds the reasoning behind the values the spec states, together with the flow of the agent, and carries no value of its own. Both documents are bound to the version hash as evidence, with their digests, and stay outside the hash, so revising one is recorded and leaves what executes unchanged.

### Evaluation and promotion

A run is an evaluation run or a production run. A draft executes only evaluation runs. An active version accepts both, which is how the red-team suite is repeated against it. An evaluation run always uses the test bindings, and no production trigger starts a draft.

Promotion to `active` requires evaluation evidence bound to the version hash and an owner's approval. Cursus ships the evaluation harness, and the client runs it as a client of the API with a credential of its own. The subject that runs it cannot be the subject that created the version. The control plane checks that the evidence names the version hash, that the runs it cites are evaluation runs of that version, and that it covers what the spec requires; coverage is derived from the spec, from its tools, its declared untrusted inputs, and its risk tier. Whatever the evaluation runs show in the control plane's own records, such as a tool call refused by the allowlist, a guardrail block, a budget held, or an approval required, is derived there. Only what depends on a judge is accepted as asserted by the evaluator.

The dataset and the rubric belong to the client. Evidence holds the hash of the dataset and the results, without the data.

Evidence has a validity. The red-team suite includes adaptive, multi-turn attacks and is repeated against active versions by a client of the API. The control plane holds every active version to it, and evidence that fails or has expired leads to suspension. The period of validity and the scheduling are set in the phase 6 plan.

### Technology commitments

| Concern | Commitment |
|---|---|
| Language | Go for the system. Python only where evaluation tooling requires it. |
| Durable execution | Temporal, used directly, without an abstraction layer over workflows, activities, or signals. Its SDK is imported only by `internal/temporal`, tests included, and the lint rejects an import from any other package. |
| Database | Postgres. |
| Observability | OpenTelemetry spans at the model, tool, and policy boundaries, exported over OTLP to Langfuse. |
| Evaluation | promptfoo, driving the HTTP API as a provider. |
| Authentication | A minimal JWT issuer shipped with the project, with four roles: spec author, owner, approver, client service. Token verification is behind an interface so the issuer can be replaced. |
| Interface | Server-rendered, three screens: agents, approval queue, run view. |
| Local infrastructure | Postgres, Temporal, and Langfuse under Docker Compose, together with the ClickHouse, Redis, and MinIO services that Langfuse requires. Cursus connects only to Postgres, Temporal, and Langfuse. |
| Supply chain | Images signed with cosign; CI runs static analysis, vulnerability scanning, secrets scanning, SBOM generation, and image scanning. |

### API shape

A run has one of two shapes, declared in the spec. A `task` works toward a result: the client starts it and receives an identifier, and completion and approval requests are delivered by webhook and can be polled. A task may wait for a human for days, so it is never tied to an open request. A `session` holds a conversation: it stays open, each message is a request that receives an answer, and the answer is delivered as a stream. Both shapes are Temporal workflows and pass through the same guardrails, budgets, policy, and audit. How output checks apply to an answer that is streamed is decided in the session ADR. The first version builds `task`.

### Identity

Versions are created in `draft` by clients. Promotion to `active` requires evaluation evidence and an owner's approval. Approvals of paused runs come either from a Cursus user through the built-in queue or from a client service asserting the approver's identity; the audit log records both the asserted approver and the asserting credential. Dual control across delegated identities is the client's responsibility.

Two kinds of credential exist. A token identifies a caller of the API and is the subject of ADR-0002. The credentials an agent presents to its tools are a separate mechanism, with an ADR of its own that covers their revocation, and a version is not retired while any of its credentials remains active.

### Runs and delegation

Every run records its parent run and delegation depth. Budgets and policies are evaluated against the tree of runs, so a delegated agent cannot hold more authority than the run that delegated to it. Invoking another agent is a tool whose adapter starts a child workflow.

An action returned by the decision function carries one or more tool calls, which lets several delegations start together; the first version dispatches them one at a time. A paused run waits for one of three things: a human approval, another run, or an external event. It is resumed by a signal, and every wait has a timeout, so two runs waiting on each other both end. The first version builds the wait for human approval.

### Memory

The spec declares the memory an agent uses: the corpora it reads, whether it may write, and how long what it writes is kept. Working memory, the context of one run, is what the first version builds. Retrieval over a corpus is a tool type. Episodic memory, written by the agent in production and read by later runs, changes behaviour without a new version, so it comes with checks on what is written and a record of what each run read.

### Security principles

The model is treated as compromisable. Injected instructions are expected to succeed some of the time, so the controls that decide what an agent can do are deterministic and sit outside the model: tool and domain allowlists, budgets, the approval policy, and the sensitive operation policy. Guardrail checkers reduce how often an attack gets through and are never the only barrier.

Three rules follow from this, and all three read what each tool declares. A spec that combines access to private data, exposure to untrusted input, and a channel to the outside is flagged at validation and held to a higher risk tier. A run records two marks: that it has read untrusted content and that it has read private data. From the first mark on, its tools with a sensitive effect require human approval; what a delegated agent returns counts as untrusted content for the run that called it. What the second mark requires is decided in the ADR on the marks of a run. An agent's credentials are scoped to its version, live as briefly as each tool allows, and never enter the model's context.

The sensitive operation policy is declarative and is evaluated before every tool call whose declared effect is sensitive. The quantity and unit of each such operation are read from the arguments of the call, as the tool declares them, so the amount checked is the amount executed. The control plane records them, and the policy applies limits per call and cumulative limits over a window, to the agent and to the tree of runs. Evaluation runs and production runs are counted apart. Bookkeeping that belongs to a domain stays with the client's tool.

### Audit

The audit log is append-only and chained by hash. Entries hold references and hashes, and the content they refer to is stored where it can be erased on request without breaking the chain. Trace content is masked before export and kept for a stated period.

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

### Part of the design, built when an agent requires it

Session runs, retrieval as a tool type, episodic memory, parallel delegation, and waits on another run or on an external event. The spec format and the run model account for them from the start, and each is built with its own ADR when an agent requires it.

### Excluded from the first version

Multi-provider routing by strategy, speech, MCP tool sources, conditional orchestration beyond child workflows, a full operations frontend, and Redis, Kafka, or Kubernetes as dependencies of Cursus. Redis runs locally only as a backing service of Langfuse. Each may enter in a later phase when an agent requires it, with its own ADR.

### Outside the project

Governing agents that run on an external harness, and conversation among peer agents with no run that delegates and answers for the others.

### Boundary

Cursus owns the mechanism of the lifecycle: spec format, versions, states, run execution, guardrail engine, tool contract, sensitive operation policy, approval mechanics, the evaluation harness, the evidence record and the promotion gate, audit, and telemetry emission. Clients own the content: task, limits, rules, the threat model and the design record, tool implementations with their declarations and bindings, approvers, datasets, the running of evaluations, and business alerting. The demo client is a separate service under `examples/` that uses only the API; it brings its own tools and keeps the spec, threat model, design record, and dataset of each of its agents.

## Alternatives

**A lighter durable-execution library such as DBOS or Restate.** Both reduce operational footprint. Temporal was chosen for maturity, the author's prior experience, and the fit of its workflow, activity, and signal model to agent runs with human approval. The alternatives remain candidates if the operational cost of Temporal proves too high for the demo.

**Abstracting the durable engine behind an interface.** Rejected: workflows and signals are not a surface that can be swapped without rewriting the runtime, so the interface would be a false seam.

**Reusing an external identity provider for the demo.** Rejected for the first version so that the repository runs without a dependency on another project. The verification interface keeps the option open.

**A synchronous API for tasks.** Rejected because a task that waits for approval can last longer than any reasonable request timeout. A session answers each message within a request, since a turn is short.

**Running sessions outside the durable engine.** A session held in the API process would answer faster and be simpler to build. It would also lose its state when the process stops, need a second implementation of guardrails, budgets, and audit, and make a session that delegates a task a special case. With both shapes on Temporal, that delegation is the same child workflow as any other. A path outside the engine is considered only if measured latency requires it.

**Governing agents that run on an external harness.** Rejected: the controls in this document depend on Cursus dispatching every model call and every tool call itself, and a loop it does not execute cannot give that guarantee.

**Enforcing the controls of the agent lifecycle in a pipeline.** Rejected: the API is how a client creates a version, so a control that lives in a pipeline is skipped by any client that calls the API directly. As conditions of transitions, the controls hold for every client, and a pipeline remains a convenient way of calling the API.

**Running evaluations inside Cursus.** The control plane would produce the scores itself and the gate would rest on no assertion. It would also bring the evaluation toolchain into the runtime and make Cursus the owner of the client's dataset. With the evaluator as a client, the control plane still derives what its own records show, and only the judged part is asserted.

**Writing the evaluation harness in Go.** Rejected: the evaluation ecosystem is in Python and TypeScript, and the HTTP boundary makes the language of the harness irrelevant to the system.

## Consequences

The runtime cannot run without Temporal, and a change of durable engine means a rewrite of the runtime package. The import boundary confines that rewrite to `internal/temporal` and to the package that holds the workflows; the phase 3 plan names that package and extends the rule if it lies outside `internal/temporal`. Local development needs Postgres, Temporal, and Langfuse under Compose, and Langfuse brings ClickHouse, Redis, and MinIO with it, which makes seven containers and a memory footprint dominated by Langfuse. Evaluation requires a Python or Node toolchain alongside Go. The identity model allows a client to assert approvers, which moves part of dual-control assurance to the client and must stay visible in the security model. The evaluator is a client, so what depends on a judge reaches the gate as an assertion; the audit log records the credential that made it, and that part of the assurance rests with the client. Bindings sit outside the hash, so an evaluated version can be pointed at a different implementation of a tool, which is why a change to a production binding requires an owner's approval and every change is recorded. The spec carries fields for run shapes and memory that the first version does not execute, which keeps their arrival from changing the version format.

In return, approvals, retries, timeouts, and compensation are provided by the engine rather than written here; every stage of the lifecycle has a defined artifact; and the system stays small enough to read.
