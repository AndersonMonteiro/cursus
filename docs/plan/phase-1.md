# Phase 1: spec

Milestone: [Phase 1: spec](https://github.com/AndersonMonteiro/cursus/milestone/2)

Phase 1 builds the first transition of the agent lifecycle: a version is created in `draft` through the API, and the API refuses the request unless every condition the baseline attaches to that transition is met. At the end of the phase the control plane accepts a spec as data, validates it, identifies it by a hash, checks its threat model and design record against it, records the creation in the audit log, and can suspend a version. No run exists yet, and no version can be promoted.

The phase closes when a fixture agent with synthetic content is created in `draft` through the API by `agentctl apply`, a request without a threat model is refused with nothing written, and `CONTRIBUTING.md` opens periphery work to contributors. The first agent is registered in phase 2, when its design record is accepted.

## Conditions of creation in `draft`

Each condition comes from README or ADR-0001 and is built by the slice named.

| Condition | Slice |
|---|---|
| The spec arrives as JSON and conforms to the published JSON Schema | 1 |
| The version is identified by a hash over normalized content, so two writings of one spec are one version | 2 |
| Every part validates, and every tool declares its effect, whether it touches private data, and whether what it returns is untrusted | 3 |
| A risk tier is declared, and a spec that combines private data, untrusted input, and an outbound channel is refused below `high` | 3 |
| A threat model and a design record are present, cover what the spec declares, and are bound to the hash by digest | 5, 6 |
| The creation is recorded in the audit log in the same transaction | 4, 6 |
| The request comes from a `spec_author`, the creator is recorded, and every row is scoped by tenant | 6 |
| Revising a document is recorded and leaves the hash unchanged | 7 |

## Decisions of this plan

### Spec fields

```yaml
format: cursus.agent/v1
name: <agent>
task: {description, success_criterion}
instructions: <text>
run: {shape: task | session}
model: {provider, id, parameters}
risk_tier: low | medium | high
untrusted_inputs: [run_input]
budgets: {max_steps, max_cost}
tools:
  <name>:
    type: http | agent
    description: <text>
    input_schema: <JSON Schema>
    output_schema: <JSON Schema>
    effect: {kind: read | write | send, sensitive: {quantity, unit}}
    private_data: <bool>
    untrusted_output: <bool>
allowlists: {domains: [...]}
policies: {approval, sensitive_operations}
guardrails: {input: [...], output: [...]}
memory: {corpora, write, retention, compaction}
```

The API accepts JSON; the YAML above is `agentctl`'s way of writing it. Unknown fields are refused and no field has a default, so a default introduced later cannot change the hash of an existing spec. The `tools` map is the tool allowlist, and a map keyed by name keeps tool order out of the hash. `model.id` is the exact identifier the provider serves, never an alias. The inner fields of `policies`, of a guardrail entry, and of `memory` are named by the slice or the ADR that builds each and proposed in its pull request; phase 1 fixes their place and validates their structure.

### Tool declarations

`effect.kind` is `read` when the call changes nothing, `write` when it changes state inside the client's systems, and `send` when it delivers content outside the client's boundary. The optional `effect.sensitive` block makes the effect sensitive and names, as pointers into the call's arguments, where the quantity and the unit of the operation are read. `private_data` and `untrusted_output` are the other two declarations. All three are under the hash.

### Structural check and risk tier

The three legs are read from the spec as follows.

| Leg | Present when |
|---|---|
| Private data | any tool has `private_data: true` |
| Untrusted input | `untrusted_inputs` is not empty, or any tool has `untrusted_output: true` |
| Outbound channel | any tool has `effect.kind: send` |

A spec with all three legs must declare `risk_tier: high`. A lower declaration is refused, and the refusal names the tools and inputs that form each leg. The tier has no other effect in phase 1; what `high` requires of a run and of evaluation coverage is stated in the plans of phases 3 and 4.

### Registries read by validation

Tool types are checked against `http` and `agent`. Guardrail checker names are checked against a registry of known checkers, which is empty until phase 3, so a phase 1 spec carries empty guardrail lists and one that names a checker is refused. Adding guardrails to an agent later is a new version.

### Policies, allowlist, and bindings

The approval policy and the limits of the sensitive operation policy are spec fields under the hash, so changing a limit is a new version, evaluated and approved like any other. The domain allowlist lists every domain the agent's HTTP tools may reach under any binding set, test or production, and a binding whose address lies outside it is refused. A production binding changes only by a request from a subject with the `owner` role; the request is its approval and is recorded. Bindings and the enforcement of the allowlist are built in phase 3.

### Threat model and design record

Both are data, stored by the control plane where content can be erased without touching the audit chain, and bound to the version by their digests. The threat model lists threats, each naming the tools, untrusted inputs, or legs of the structural check it concerns. Creation is refused when a tool with `untrusted_output`, a tool with a sensitive effect, or a present leg has no threat entry. The design record has five required sections: autonomy, run shape, budgets, allowlists, and flow. The template of the threat model refers to the OWASP Top 10 for LLM applications (2025), the OWASP Top 10 for Agentic Applications for 2026, and MITRE ATLAS.

### Roles

| Action | Role |
|---|---|
| Create a version, validate with a dry run, revise a document | `spec_author` |
| Suspend a version | `owner` |
| Read agents and versions | any authenticated subject of the tenant |

Owner means any subject holding the `owner` role in the tenant; an agent has no owner field of its own. A version records the subject that created it, which later phases compare with the subject that evaluates it.

### States and transitions

| From | To | Who | Conditions built in |
|---|---|---|---|
| none | `draft` | `spec_author` | phase 1 |
| `draft` | `active` | `owner`, with passing evidence | phases 4 and 5 |
| `active` | `suspended` | `owner`, or the control plane | phase 1; phase 6 for the control plane |
| `suspended` | `active` | `owner`, with fresh passing evidence | phase 5 |
| `active`, `suspended` | `deprecated` | `owner` | phase 5 |
| `draft`, `deprecated` | `retired` | `owner` | phase 7 |

An owner suspends at once and gives a reason; there is no other condition. The control plane suspends by itself when red-team evidence fails or expires, without waiting for an owner. Suspension applies to one version. Phase 1 builds the transition function for the whole table and the suspend endpoint; every other transition is refused until the phase that builds its conditions, so a promotion is refused unconditionally before phase 5. Rollback and how a deprecated version relates to its successor are set in the phase 5 plan and may add an edge.

### Names

| Kind | Names |
|---|---|
| Packages | `internal/spec`, `internal/audit`, `internal/registry`, `internal/lifecycle` |
| Tables | `audit.entries`, `control.agents`, `control.versions`, `control.version_documents`, `control.version_transitions` |
| Endpoints | `POST /v1/agents/{agent}/versions` (with `?dry_run=true` to validate only), `GET /v1/agents`, `GET /v1/agents/{agent}`, `GET /v1/agents/{agent}/versions`, `GET /v1/agents/{agent}/versions/{hash}`, `PUT /v1/agents/{agent}/versions/{hash}/threat-model`, `PUT /v1/agents/{agent}/versions/{hash}/design-record`, `POST /v1/agents/{agent}/versions/{hash}/suspend`, `GET /v1/spec/schema` |
| Commands | `agentctl validate -f <file>`, `agentctl apply -f <file> --threat-model <file> --design-record <file>` |
| Environment | `CURSUS_API_URL`, `CURSUS_TOKEN` |
| Version metadata outside the hash | `supersedes`, `change_reason` with `spec_change`, `model_retired`, `tool_contract_changed` |

An agent is created by its first version. A model retired by its provider is recorded by creating the new version with `supersedes` naming the previous hash and `change_reason: model_retired`.

### Run fields

Phase 1 creates no run. The fields a run carries are named here so that the phase 3 plan starts from them: `shape` (`task`, `session`), `kind` (`evaluation`, `production`), `source`, `parent_run`, and `delegation_depth`. The table is created in phase 3 by the first slice that writes a run.

### Proposed, to settle in the review of this plan

- Tool schemas are written inline in the spec, in JSON Schema 2020-12, and the version hash covers them as part of the content.
- The hash is SHA-256 over the canonical form of RFC 8785 (JSON Canonicalization Scheme), written `sha256:<hex>`. The agent name is inside the hash; the tenant, the documents, and the version metadata are outside. RFC 8785 sorts object properties and leaves arrays in the order given, so normalization first sorts and deduplicates the arrays that are sets (`allowlists.domains`, `untrusted_inputs`) and lowercases domains.
- Creating a version whose hash already exists for the agent returns the existing version when the documents have the same digests, and is refused otherwise with a pointer to the revision endpoints.
- `model.provider` is checked as a non-empty string until phase 3 names the provider.
- Threat model fields: `format: cursus.threat-model/v1`, `threats[]` with `id`, `title`, `description`, `concerns` (tool names, untrusted inputs, or `private_data`, `untrusted_input`, `outbound`), `references`, and `mitigations`. Design record fields: `format: cursus.design-record/v1` and the sections `autonomy`, `run_shape`, `budgets`, `allowlists`, `flow`.
- The fixture agent's files live in `examples/fixture-agent/`.
- ADR numbers: 0003 spec format and version identity, 0004 validation and the structural check, 0005 audit log, 0006 registry and creation in draft, 0007 lifecycle states and suspension.

## Slices

Issues are created after this plan is reviewed. Every core slice that introduces a mechanism carries the `adr-required` label.

| # | Slice | Scope | Depends on |
|---|---|---|---|
| 1 | Spec format | core | none |
| 2 | Version identity | core | 1 |
| 3 | Validation and the structural check | core | 1 |
| 4 | Audit log | core | none |
| 5 | Threat model and design record formats | core | 3 |
| 6 | Registry and creation in `draft` | core | 2, 3, 4, 5 |
| 7 | Revision of documents | core | 6 |
| 8 | Lifecycle states and suspension | core | 6 |
| 9 | `agentctl validate` and `agentctl apply` | periphery | 6 |
| 10 | Contribution guide | periphery | none |
| 11 | Closing: the fixture agent registered | periphery | 7, 8, 9 |

Slices 1 to 3 and slice 4 can proceed in parallel, and slice 10 at any time. Slices 7, 8, and 9 can proceed in any order after 6.

### 1. Spec format

- Goal: the spec exists as Go types and as a JSON Schema, and a document that does not conform is refused with the path of each fault.
- Files: `docs/adr/0003-*.md`, `internal/spec` (types, embedded schema, decoding).
- Acceptance: a conforming spec decodes. The failure tests refuse an unknown field, a missing required field, a tool without one of its three declarations, a `sensitive` block without `quantity` or `unit`, and a tool schema that is not valid JSON Schema, each with the path reported.
- Out of scope: the hash, checks across fields, HTTP.
- Points of variation: run shape and memory are fields now and executed later (interface now); the JSON Schema library is concrete, confined to `internal/spec` (noted seam).

### 2. Version identity

- Goal: a spec has one hash whatever way it is written.
- Files: `internal/spec` (normalization, hash), ADR-0003 completed.
- Acceptance: two writings of one spec that differ in key order, whitespace, and the order of set arrays give the same hash. The failure tests show that changing the model identifier, a model parameter, a tool's input or output schema, or any of its three declarations gives a different hash, and that a document with a duplicate key is refused.
- Out of scope: storage, documents.
- Points of variation: the hash algorithm is written into the hash string, so another can be added (noted seam); canonical form is a deliberate commitment.

### 3. Validation and the structural check

- Goal: every part of a spec validates, and the three legs and the tier floor are computed from the declarations.
- Files: `docs/adr/0004-*.md`, `internal/spec` (validation, registries, structural check).
- Acceptance: a valid spec passes and the result reports which legs are present. The failure tests refuse a spec with all three legs declared `medium`, naming the tools of each leg; an unknown tool type; any checker name while the registry is empty; a budget that is not positive; and a sensitive pointer that does not resolve in the tool's input schema.
- Out of scope: checking that a model or a domain exists, the meaning of `policies` fields, what `high` requires at run time.
- Points of variation: the registries of tool types and checkers are interfaces now, filled in phase 3; the three tier values are a deliberate commitment.

### 4. Audit log

- Goal: an append-only log, chained by hash, that the database itself protects and a verifier can recompute.
- Files: `docs/adr/0005-*.md`, `internal/audit`, a migration for `audit.entries` and for separate migration and application roles, `compose.yaml`, `.env.example`, `Makefile`, the CI service container.
- Acceptance: an entry holds references and hashes, never content, and is appended inside a transaction the caller provides. The application role inserts and selects; an update or a delete is rejected by the database. The failure test alters a row with the migration role and the verifier reports the entry where the chain breaks.
- Out of scope: writes from Temporal activities and their idempotency on run and step, the record of sensitive operations, trace correlation.
- Points of variation: the store is concrete on Postgres, with a repository interface for tests only; the chain format carries a version (noted seam).

### 5. Threat model and design record formats

- Goal: the two documents are data the control plane can check against a spec.
- Files: `internal/spec` (document types, schemas, coverage check), templates of both documents, ADR-0004 extended.
- Acceptance: documents that cover the spec pass. The failure tests refuse a threat model with no entry for a tool with `untrusted_output`, for a tool with a sensitive effect, and for a present leg, and a design record with a missing or empty section.
- Out of scope: judging the quality of the content, the control mapping to OWASP and MITRE ATLAS, which is a periphery slice at the end of phase 3.
- Points of variation: both formats carry a `format` field (noted seam).

### 6. Registry and creation in `draft`

- Goal: `POST /v1/agents/{agent}/versions` creates a version in `draft` when every condition holds and writes nothing otherwise.
- Files: `docs/adr/0006-*.md`, `internal/registry`, a migration for `control.agents`, `control.versions`, `control.version_documents`, `control.version_transitions`, `cmd/api` routes, the JSON envelope for 404.
- Acceptance: a valid request from a `spec_author` returns the version with its hash, state, creator, and document digests, and the audit log holds the creation. `?dry_run=true` returns the same validation result and writes nothing. The read endpoints and `GET /v1/spec/schema` answer. The failure tests show that a request without a threat model, without a design record, from another role, or with a spec that fails validation is refused with no row written; that a failed audit insert rolls the creation back; that a repeated creation returns the existing version; and that one tenant cannot read another's agent.
- Out of scope: any transition out of `draft`, bindings, runs.
- Points of variation: repository interface for tests only; document content is stored apart from the audit entry so that it can be erased (deliberate commitment).

### 7. Revision of documents

- Goal: a threat model or a design record is revised without changing what executes.
- Files: `internal/registry`.
- Acceptance: a `PUT` by a `spec_author` stores the new document, keeps the previous digest in the record, appends an audit entry, and leaves the hash and the state unchanged. The failure tests refuse a revision that no longer covers the spec, a revision of a retired version, and a revision by another role.
- Out of scope: erasure of earlier content, which belongs to phase 7.

### 8. Lifecycle states and suspension

- Goal: the transitions of a version are one function, and an owner can suspend.
- Files: `docs/adr/0007-*.md`, `internal/lifecycle`, `internal/registry` (suspend handler), `cmd/api` routes.
- Acceptance: the transition function answers, for a state, a target, and an actor, whether the transition is allowed and what it requires, for the whole table above. `POST .../suspend` by an `owner` with a reason moves an active version to `suspended`, writes the transition record and the audit entry in one transaction. The failure tests refuse every edge outside the table, a suspension without a reason or by another role, a suspension of a `draft`, a promotion in any form, and the second of two concurrent transitions of one version. The test places a version in `active` directly in the database, since no promotion exists.
- Out of scope: promotion, lifting a suspension, deprecation, retirement, suspension by the control plane, runs in flight.
- Points of variation: the actor is a type that admits the control plane besides a subject (interface now); conditions of later edges are named and refused (noted seam).

### 9. `agentctl validate` and `agentctl apply`

- Goal: a spec written as `agent.yaml` is validated and applied from the command line through the API.
- Files: `cmd/agentctl`.
- Acceptance: `validate` converts the file to JSON, calls the dry run, and prints the hash, the legs present, and each fault with its path. `apply` creates the version and prints its hash and state. The failure tests cover a refused spec, a missing document file, an unreachable API, and a refused token, each with a non-zero exit and without printing the token.
- Out of scope: any validation performed by the command itself; the API is the only validator.
- Points of variation: the YAML library is concrete and confined to `cmd/agentctl` (noted seam).

### 10. Contribution guide

- Goal: a contributor knows how to take a periphery slice.
- Files: `CONTRIBUTING.md`, `.github/ISSUE_TEMPLATE/`, `README.md` ("How the work is done" shortened).
- Acceptance: the guide states Conventional Commits, branch naming, issue before pull request, the walkthrough and the reviewer note, an ADR before a mechanism, tests under failure, and that contributions are accepted under Apache 2.0 with nothing to sign.
- Out of scope: a code of conduct beyond a pointer, `CODEOWNERS`.

### 11. Closing: the fixture agent registered

- Goal: the phase's closing condition is shown end to end and written up.
- Files: `examples/fixture-agent/` (spec, threat model, design record), `docs/lifecycle/spec/`.
- Acceptance: against the running stack, `agentctl apply` creates the fixture agent in `draft`, the audit verifier passes, and the same command without the threat model is refused with nothing written. `docs/lifecycle/spec/` holds the account of the stage with the commands and their output.
- Out of scope: the first agent, which is registered in phase 2.

## Continuous integration

No job is added. The tests of slices 4, 6, 7, and 8 run against the Postgres service container of `build-test`, which applies the migrations before the tests.

## Out of scope

Runs and the runs table, the runtime loop, tool dispatch, bindings, agent credentials, guardrail checkers, the enforcement of policies and of the domain allowlist, evaluation evidence, promotion, deprecation, retirement, and the first agent.
