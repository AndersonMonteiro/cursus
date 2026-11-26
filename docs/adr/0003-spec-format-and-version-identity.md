# ADR-0003: Spec format and version identity

Status: accepted
Date: 2026-11-24

## Context

ADR-0001 states that the API accepts a spec as data and that everything defining the behaviour of an agent is in it. The format of that data needs one definition that the API, `agentctl`, and a later form or editor can share, and it needs a rule for how it grows, because several of its sections belong to mechanisms that later phases build. A version is identified by a hash over the content of the spec, so the format also has to keep one meaning from having two writings wherever it can.

Three points of variation are involved. The JSON Schema library and the canonicalization library are concrete and confined to one package. The hash algorithm is named in the hash string, so another can be added. The canonical form is a deliberate commitment.

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

`spec.Decode` takes the bytes of a document and returns the spec or the list of its faults. Each fault carries a message and a path, written as an RFC 6901 JSON Pointer into the document. A missing field is reported at the path the field would have, and an unknown field at its own path, so every fault points at the place to edit. All faults the schema finds in a document are reported together, ordered by path.

Four refusals come before the schema, and each is reported without the faults the schema would find. Two are at the root: a document that is not one JSON value, and one that holds invalid UTF-8, which a JSON decoder would otherwise replace silently and so return a spec that differs from what was sent. The other two come from version identity and are described under it: a duplicate key, and a value that has no canonical form.

`budgets.max_steps` and `budgets.max_cost.amount` are integers between -9007199254740991 and 9007199254740991, and the schema states those bounds. They are the integers that a double holds exactly, which is how the canonical form writes every number, so two different budgets never share a hash.

One refusal comes after the schema. JSON Schema counts `150.0` and `1e2` as integers and has no keyword that refuses them, while `Decode` accepts an integer only when it is written without a decimal point or an exponent. A document that passes the published schema can therefore still be refused for this one reason. The fault carries its path, and it is reported only once the document has no other fault, one such field at a time.

The format covers what one field can say by itself. Checks that read more than one field or a registry, such as the naming rules, the tool types, and the structural check, are the subject of ADR-0004.

### Version identity

The hash of a spec is SHA-256 over the canonical form of its normalized content, written `sha256:` followed by the digest in lowercase hexadecimal. The canonical form is the one of RFC 8785, the JSON Canonicalization Scheme. The agent name is part of the content, and so are the model identifier, its parameters, and the schemas and the three declarations of every tool. The tenant, the threat model, the design record, and the metadata of a version are outside it. `Spec.Hash` computes the hash of a spec that `Decode` returned.

Normalization covers what two writings of one spec may differ in:

- the order of the keys of every object, at any depth, and the whitespace between tokens;
- the writing of a string, so an escaped character and the character itself are one writing;
- the writing of a number. A number is the double it names, as in RFC 8785, so `0.2`, `0.20`, and `2e-1` are one writing, and so are two literals too close for a double to tell apart;
- the order and the repetition of entries in `allowlists.domains` and `untrusted_inputs`, the two arrays of the format that are sets. They are sorted in the order RFC 8785 gives to keys, by UTF-16 code unit, and repeated entries are dropped.

Everything else is left as written. Every other array keeps its order, including `required`, `enum`, and any array inside a tool schema or inside `model.parameters`, so two tool schemas that differ only in the order of `required` are two versions. The content of a string is not changed: no case is folded and no Unicode normalization is applied, and the rule on how a domain is written belongs to validation (ADR-0004). A tool schema is compared as data and not by what it accepts, so two schemas that accept the same values under different keywords are two versions.

Two refusals of `Decode` exist for identity. A document in which one object holds the same key twice is refused, with a fault at the path of each repeated key; keys are compared after their escapes are read. A document that RFC 8785 cannot write is refused at the root: one with an escaped surrogate that has no pair, or with a number beyond the range of a double. A spec that `Decode` returns therefore always has a hash.

### Libraries

Validation uses `github.com/santhosh-tekuri/jsonschema/v6`. It implements draft 2020-12, carries the metaschemas inside the module, fetches nothing over the network unless a loader is configured, and reports each fault with the location of the value it concerns.

The canonical form is produced by `github.com/gowebpki/jcs`, which implements RFC 8785 and has no dependency outside the standard library.

Both are imported only by `internal/spec`, and no type of either appears in what the package exports.

## Alternatives

**Validation written by hand, without a schema.** The rules would live only in Go code, and a client or a form would have no definition to read. A published schema lets a client check a spec before sending it and lets an editor be generated from it.

**A schema generated from the Go types by reflection.** One definition instead of two. Generators express poorly what this format depends on, such as closed objects everywhere, a section that accepts only the empty object, and a reference to the metaschema, and the published schema would change whenever the generator did. The schema is written by hand and a test keeps the types in agreement with it.

**Optional sections with defaults.** A shorter spec for an agent that uses no memory or guardrails. An omitted section and an empty one would then be two writings of the same agent, and the default would become part of the meaning of every stored spec without being under its hash.

**A hash over the bytes as sent.** Nothing to normalize and nothing to get wrong. The same spec written by `agentctl` from YAML and by a client from its own serializer would be two versions, each asking for its own evaluation and approval.

**A canonical form of the project's own.** It could keep integers of any size and sort every array. A client could then reproduce a hash only with code written for Cursus, while RFC 8785 has implementations in most languages and is what a sorted `JSON.stringify` already produces.

**Sorting every array.** One rule instead of a list of two arrays. The order of an array carries meaning in JSON Schema, in `prefixItems` for one, and in parameters a provider may read in order, so the hash would call two different contracts one version.

**Other libraries.** `github.com/google/jsonschema-go` stops at the first fault and gives no structured location. `github.com/kaptinlin/jsonschema` brings a large dependency tree and loads remote references by default. `github.com/xeipuuv/gojsonschema` does not implement draft 2020-12.

## Consequences

A spec is longer than it would be with defaults: an agent without guardrails still writes two empty lists. In return, what a version executes is fully stated in its spec, and a later phase can add a mechanism without changing any existing version.

The schema and the Go types are two definitions of one format. The test that compares their field names catches a field added to one alone; it does not compare constraints, which stay in the schema.

A tool schema is checked against the metaschema and is not compiled, so a reference inside it is not resolved and a regular expression in it is not parsed at this point.

Replacing the JSON Schema library means rewriting the part of `internal/spec` that compiles the schema and turns the library's errors into faults. The schema file, the types, the fault type, and every caller of `Decode` stay as they are.

The canonical form, the list of arrays that are sets, and what the hash covers are fixed for `cursus.agent/v1`: changing any of them changes the hash of every stored version. An array added to the format later is declared a set or left ordered by the ADR that adds it. A second hash algorithm enters as a second prefix beside `sha256:`, and a reader of a hash tells them apart by the prefix. Replacing the canonicalization library changes no hash as long as the replacement implements RFC 8785.

The hash is computed from the Go types, written back as JSON. A field that the format lets be absent must therefore be left out when it is absent, and the test that compares the types with the schema checks it.

A budget above 9007199254740991 cannot be written. Numbers inside `model.parameters` and tool schemas have no such bound, and two of them that name the same double are one writing.

Reversing the rule of no defaults or of closed objects would change which documents are accepted and is a new `format` value.
