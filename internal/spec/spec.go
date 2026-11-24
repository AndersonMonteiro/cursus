// Package spec holds the format of an agent spec: its Go types, its JSON
// Schema, and the decoding that refuses a document outside the format.
package spec

import "encoding/json"

const Format = "cursus.agent/v1"

// A field is a pointer only where the format lets it be absent. The test that
// compares these types with the schema relies on that.
type Spec struct {
	Format          string          `json:"format"`
	Name            string          `json:"name"`
	Task            Task            `json:"task"`
	Instructions    string          `json:"instructions"`
	Run             Run             `json:"run"`
	Model           Model           `json:"model"`
	RiskTier        string          `json:"risk_tier"`
	UntrustedInputs []string        `json:"untrusted_inputs"`
	Budgets         Budgets         `json:"budgets"`
	Tools           map[string]Tool `json:"tools"`
	Allowlists      Allowlists      `json:"allowlists"`
	Policies        Policies        `json:"policies"`
	Guardrails      Guardrails      `json:"guardrails"`
	Memory          Memory          `json:"memory"`
}

type Task struct {
	Description      string `json:"description"`
	SuccessCriterion string `json:"success_criterion"`
}

type Run struct {
	Shape string `json:"shape"`
}

type Model struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	// Kept as written: the content is open, and decoding it into Go values
	// would round its numbers before the version hash reads them.
	Parameters json.RawMessage `json:"parameters"`
}

type Budgets struct {
	MaxSteps int64   `json:"max_steps"`
	MaxCost  MaxCost `json:"max_cost"`
}

type MaxCost struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type Tool struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	// Kept as written: the version hash covers a tool schema with its arrays
	// in the order given.
	InputSchema     json.RawMessage `json:"input_schema"`
	OutputSchema    json.RawMessage `json:"output_schema"`
	Effect          Effect          `json:"effect"`
	PrivateData     bool            `json:"private_data"`
	UntrustedOutput bool            `json:"untrusted_output"`
}

type Effect struct {
	Kind      string     `json:"kind"`
	Sensitive *Sensitive `json:"sensitive,omitempty"`
}

type Sensitive struct {
	Quantity string `json:"quantity"`
	Unit     Unit   `json:"unit"`
}

// Exactly one of the two is set.
type Unit struct {
	Pointer *string `json:"pointer,omitempty"`
	Const   *string `json:"const,omitempty"`
}

type Allowlists struct {
	Domains []string `json:"domains"`
}

type Policies struct {
	Approval            Approval            `json:"approval"`
	SensitiveOperations SensitiveOperations `json:"sensitive_operations"`
}

// Approval, SensitiveOperations, and Memory have no fields until the
// mechanism that reads each one adds them (ADR-0003, extension by addition).
type Approval struct{}

type SensitiveOperations struct{}

type Memory struct{}

type Guardrails struct {
	// The format accepts only empty lists so far, and raw elements leave the
	// shape of a checker entry to the ADR that introduces checkers.
	Input  []json.RawMessage `json:"input"`
	Output []json.RawMessage `json:"output"`
}
