# ADR-0003: Spec format and version identity

Status: accepted
Date: 2026-11-24

## Context

ADR-0001 states that the API accepts a spec as data and that everything defining the behaviour of an agent is in it. The format of that data needs one definition that the API, `agentctl`, and a later form or editor can share, and it needs a rule for how it grows, because several of its sections belong to mechanisms that later phases build. A version is identified by a hash over the content of the spec, so the format also has to keep one meaning from having two writings wherever it can.

The point of variation involved is the JSON Schema library, which is concrete and confined to one package.

## Decision

### The format

A spec is a JSON document in the format `cursus.agent/v1`. `agent.yaml` is a way of writing it that `agentctl` converts to JSON.

```yaml
format: cursus.agent/v1
name: <agent>
task: {description, success_criterion}
instructions: <text>
run: {shape: task | session}
model: {provider, id, parameters}
risk_tier: low | medium | high
untrusted_inputs: [run_input]
budgets: {max_steps, max_cost: {amount, currency}}
tools:
  <name>:
    type: http
    description: <text>
    input_schema: <JSON Schema>
    output_schema: <JSON Schema>
    effect: {kind: read | write | send, sensitive: {quantity, unit}}
    private_data: <bool>
    untrusted_output: <bool>
allowlists: {domains: [...]}
policies: {approval, sensitive_operations}
guardrails: {input: [...], output: [...]}
memory: {}
```

The definition of the format is a JSON Schema in draft 2020-12, kept in `internal/spec/schema.json` and embedded in the binary. The Go types of `internal/spec` mirror it, and a test fails when the two name different fields.

Every object is closed, so an unknown field is refused. No field has a default, and every field above is required, with one exception: `effect.sensitive`, whose presence is what makes an effect sensitive. A section that could be left out would have an implied default, and a default introduced or changed later would alter the meaning of a spec whose hash stays the same. For the same reason one spec has one writing of each section, which the hash depends on.

`tools` is a map keyed by tool name. It is the tool allowlist of the agent, and a map keeps the order of tools out of the content. The three declarations of a tool, `effect`, `private_data`, and `untrusted_output`, are required on every tool. `effect.sensitive` holds `quantity`, a JSON Pointer into the arguments of the call, and `unit`, which is either `{pointer: <JSON Pointer>}` or `{const: <text>}`. `budgets.max_cost.amount` is an integer in the minor unit of `currency`, and `currency` is three uppercase letters. `untrusted_inputs` accepts the value `run_input`.

`input_schema` and `output_schema` are written inline, in JSON Schema 2020-12. The schema of the spec refers to the 2020-12 metaschema at those two places, so a tool schema is checked as part of the same validation as the rest of the document.

### Sections of later mechanisms

| Section | Accepted in `cursus.agent/v1` today |
|---|---|
| `model.parameters` | An open object. The provider checks its content. |
| `policies.approval`, `policies.sensitive_operations` | The empty object only. |
| `guardrails.input`, `guardrails.output` | The empty list only. |
| `memory` | The empty object only. `corpora`, `write`, `retention`, and `compaction` are reserved names. |

### Extension by addition

`cursus.agent/v1` is extended by addition. A mechanism that reads one of these sections adds optional fields inside it, with its own ADR, and a field that is absent means the mechanism is not used. A spec accepted before the addition stays valid, means what it meant, and keeps its hash. A change that cannot be made by addition, such as removing a field, changing the meaning of one, or giving a default to one, is a new `format` value.

### Decoding

`spec.Decode` takes the bytes of a document and returns the spec or the list of its faults. Each fault carries a message and a path, written as an RFC 6901 JSON Pointer into the document. A missing field is reported at the path the field would have, and an unknown field at its own path, so every fault points at the place to edit. All faults of a document are reported together.

The format covers what one field can say by itself. Checks that read more than one field or a registry, such as the naming rules, the tool types, and the structural check, are the subject of ADR-0004.

### Library

Validation uses `github.com/santhosh-tekuri/jsonschema/v6`. It implements draft 2020-12, carries the metaschemas inside the module, fetches nothing over the network unless a loader is configured, and reports each fault with the location of the value it concerns. It is imported only by `internal/spec`, and no type of the library appears in what the package exports.

## Alternatives

**Validation written by hand, without a schema.** The rules would live only in Go code, and a client or a form would have no definition to read. A published schema lets a client check a spec before sending it and lets an editor be generated from it.

**A schema generated from the Go types by reflection.** One definition instead of two. Generators express poorly what this format depends on, such as closed objects everywhere, a section that accepts only the empty object, and a reference to the metaschema, and the published schema would change whenever the generator did. The schema is written by hand and a test keeps the types in agreement with it.

**Optional sections with defaults.** A shorter spec for an agent that uses no memory or guardrails. An omitted section and an empty one would then be two writings of the same agent, and the default would become part of the meaning of every stored spec without being under its hash.

**Other libraries.** `github.com/google/jsonschema-go` stops at the first fault and gives no structured location. `github.com/kaptinlin/jsonschema` brings a large dependency tree and loads remote references by default. `github.com/xeipuuv/gojsonschema` does not implement draft 2020-12.

## Consequences

A spec is longer than it would be with defaults: an agent without guardrails still writes two empty lists. In return, what a version executes is fully stated in its spec, and a later phase can add a mechanism without changing any existing version.

The schema and the Go types are two definitions of one format. The test that compares their field names catches a field added to one alone; it does not compare constraints, which stay in the schema.

A tool schema is checked against the metaschema and is not compiled, so a reference inside it is not resolved and a regular expression in it is not parsed at this point.

Replacing the library means rewriting the part of `internal/spec` that compiles the schema and turns the library's errors into faults. The schema file, the types, the fault type, and every caller of `Decode` stay as they are.

Reversing the rule of no defaults or of closed objects would change which documents are accepted and is a new `format` value.
