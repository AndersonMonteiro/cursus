package spec

import (
	"bytes"
	"os"
	"regexp"
	"slices"
	"testing"
)

// Computed outside Go from testdata/valid.json, with the two sets sorted and
// the keys sorted before JSON.stringify, which is what RFC 8785 specifies.
const validHash = "sha256:4e3770681c37b30db562d8f88bfb97f99058a1e54740c45ec05471194b3362f5"

func hashOf(t *testing.T, data []byte) string {
	t.Helper()
	s, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	h, err := s.Hash()
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	return h
}

func TestHashIsTheSameForTwoWritingsOfOneSpec(t *testing.T) {
	valid, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := os.ReadFile("testdata/valid_rewritten.json")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(valid, rewritten) {
		t.Fatal("the two fixtures are the same bytes")
	}

	h := hashOf(t, valid)
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(h) {
		t.Errorf("hash = %q, want sha256: and 64 hexadecimal digits", h)
	}
	if h != validHash {
		t.Errorf("hash = %s, want %s as computed outside Go", h, validHash)
	}
	if got := hashOf(t, rewritten); got != h {
		t.Errorf("hash of the rewritten spec = %s, want %s", got, h)
	}
}

func TestHashTreatsTheTwoSetsAsSets(t *testing.T) {
	base := Spec{
		UntrustedInputs: []string{"run_input", "webhook_body", "attachment"},
		Allowlists:      Allowlists{Domains: []string{"b.example.com", "a.example.com", "c.example.com"}},
	}
	want, err := base.Hash()
	if err != nil {
		t.Fatal(err)
	}

	other := base
	other.UntrustedInputs = []string{"attachment", "run_input", "attachment", "webhook_body"}
	other.Allowlists.Domains = []string{"c.example.com", "c.example.com", "a.example.com", "b.example.com"}
	written := slices.Clone(other.Allowlists.Domains)

	got, err := other.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("hash = %s, want %s: order and repetition changed the hash", got, want)
	}
	if !slices.Equal(other.Allowlists.Domains, written) {
		t.Errorf("Hash reordered the spec it was given: %v", other.Allowlists.Domains)
	}

	other.Allowlists.Domains = []string{"a.example.com", "b.example.com"}
	if got, err := other.Hash(); err != nil || got == want {
		t.Errorf("hash without one domain = %s, %v; want a different hash", got, err)
	}
	other.Allowlists.Domains = []string{"A.example.com", "b.example.com", "c.example.com"}
	if got, err := other.Hash(); err != nil || got == want {
		t.Errorf("hash with a domain in another case = %s, %v; want a different hash", got, err)
	}
}

func TestHashChangesWithWhatItCovers(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, doc object)
	}{
		{"the fixture as it is", func(t *testing.T, doc object) {}},
		{"agent name", func(t *testing.T, doc object) {
			doc["name"] = "order-helper-2"
		}},
		{"model identifier", func(t *testing.T, doc object) {
			in(t, doc, "model")["id"] = "example-model-2026-06-01"
		}},
		{"model provider", func(t *testing.T, doc object) {
			in(t, doc, "model")["provider"] = "example-two"
		}},
		{"model parameter changed", func(t *testing.T, doc object) {
			in(t, doc, "model", "parameters")["temperature"] = 0.3
		}},
		{"model parameter added", func(t *testing.T, doc object) {
			in(t, doc, "model", "parameters")["top_p"] = 0.9
		}},
		{"model parameter removed", func(t *testing.T, doc object) {
			delete(in(t, doc, "model", "parameters"), "max_tokens")
		}},
		{"tool input schema", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order", "input_schema", "properties", "order_id")["type"] = "integer"
		}},
		{"tool output schema", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order", "output_schema", "properties", "total")["type"] = "number"
		}},
		{"effect kind", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order", "effect")["kind"] = "send"
		}},
		{"effect made sensitive", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order", "effect")["sensitive"] = object{
				"quantity": "/order_id", "unit": object{"const": "orders"},
			}
		}},
		{"effect no longer sensitive", func(t *testing.T, doc object) {
			delete(in(t, doc, "tools", "refund_order", "effect"), "sensitive")
		}},
		{"sensitive quantity", func(t *testing.T, doc object) {
			in(t, doc, "tools", "refund_order", "effect", "sensitive")["quantity"] = "/order_id"
		}},
		{"sensitive unit", func(t *testing.T, doc object) {
			in(t, doc, "tools", "refund_order", "effect", "sensitive")["unit"] = object{"const": "USD"}
		}},
		{"private_data", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order")["private_data"] = false
		}},
		{"untrusted_output", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order")["untrusted_output"] = true
		}},
		{"required reordered inside a tool schema", func(t *testing.T, doc object) {
			in(t, doc, "tools", "refund_order", "input_schema")["required"] = []any{"currency", "amount", "order_id"}
		}},
		{"enum inside a tool schema", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order", "output_schema", "properties", "status")["enum"] = []any{"open", "shipped"}
		}},
		{"the same enum reordered", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order", "output_schema", "properties", "status")["enum"] = []any{"shipped", "open"}
		}},
		{"array inside model.parameters", func(t *testing.T, doc object) {
			in(t, doc, "model", "parameters")["stop"] = []any{"END", "STOP"}
		}},
		{"the same array reordered", func(t *testing.T, doc object) {
			in(t, doc, "model", "parameters")["stop"] = []any{"STOP", "END"}
		}},
		{"tool renamed", func(t *testing.T, doc object) {
			tools := in(t, doc, "tools")
			tools["read_order"] = tools["lookup_order"]
			delete(tools, "lookup_order")
		}},
		{"domain added", func(t *testing.T, doc object) {
			in(t, doc, "allowlists")["domains"] = []any{"orders.example.com", "loyalty.example.com", "refunds.example.com"}
		}},
		{"no untrusted input", func(t *testing.T, doc object) {
			doc["untrusted_inputs"] = []any{}
		}},
		{"budget", func(t *testing.T, doc object) {
			in(t, doc, "budgets")["max_steps"] = 13
		}},
		{"risk tier", func(t *testing.T, doc object) {
			doc["risk_tier"] = "high"
		}},
	}

	seen := map[string]string{}
	for _, tc := range cases {
		doc := validDocument(t)
		tc.mutate(t, doc)
		h := hashOf(t, encode(t, doc))
		if other, ok := seen[h]; ok {
			t.Errorf("%q and %q give the same hash %s", other, tc.name, h)
		}
		seen[h] = tc.name
	}
	if seen[validHash] != "the fixture as it is" {
		t.Errorf("the unchanged fixture did not give %s", validHash)
	}
}
