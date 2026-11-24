package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

type object = map[string]any

func validDocument(t *testing.T) object {
	t.Helper()
	data, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// in walks doc through the named objects and returns the last one.
func in(t *testing.T, doc object, keys ...string) object {
	t.Helper()
	for _, k := range keys {
		next, ok := doc[k].(object)
		if !ok {
			t.Fatalf("fixture has no object at %q", k)
		}
		doc = next
	}
	return doc
}

func encode(t *testing.T, doc object) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func faultsOf(t *testing.T, data []byte) []Fault {
	t.Helper()
	s, err := Decode(data)
	if err == nil {
		t.Fatal("Decode accepted the document")
	}
	if s != nil {
		t.Error("Decode returned a spec together with an error")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("Decode returned %T, want *DecodeError: %v", err, err)
	}
	if len(derr.Faults) == 0 {
		t.Fatal("DecodeError holds no fault")
	}
	return derr.Faults
}

func requireFaultAt(t *testing.T, faults []Fault, path string) {
	t.Helper()
	for _, f := range faults {
		if f.Path == path {
			t.Logf("refused at %q: %s", f.Path, f.Message)
			return
		}
	}
	t.Errorf("no fault at %q, got %+v", path, faults)
}

func TestDecodeConformingSpec(t *testing.T) {
	data, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if s.Format != Format || s.Name != "order-helper" || s.Run.Shape != "task" || s.RiskTier != "medium" {
		t.Errorf("top-level fields read back wrong: %+v", s)
	}
	if s.Model.Provider != "example" || s.Model.ID != "example-model-2026-01-15" {
		t.Errorf("model = %+v", s.Model)
	}
	if !strings.Contains(string(s.Model.Parameters), "0.2") {
		t.Errorf("model.parameters = %s, want the content as written", s.Model.Parameters)
	}
	if !slices.Equal(s.UntrustedInputs, []string{"run_input"}) {
		t.Errorf("untrusted_inputs = %v", s.UntrustedInputs)
	}
	if s.Budgets.MaxSteps != 12 || s.Budgets.MaxCost.Amount != 150 || s.Budgets.MaxCost.Currency != "USD" {
		t.Errorf("budgets = %+v", s.Budgets)
	}
	if !slices.Equal(s.Allowlists.Domains, []string{"orders.example.com"}) {
		t.Errorf("allowlists.domains = %v", s.Allowlists.Domains)
	}
	if len(s.Guardrails.Input) != 0 || len(s.Guardrails.Output) != 0 {
		t.Errorf("guardrails = %+v", s.Guardrails)
	}
	if len(s.Tools) != 3 {
		t.Fatalf("tools = %d, want 3", len(s.Tools))
	}

	lookup := s.Tools["lookup_order"]
	if lookup.Type != "http" || lookup.Effect.Kind != "read" || lookup.Effect.Sensitive != nil ||
		!lookup.PrivateData || lookup.UntrustedOutput {
		t.Errorf("lookup_order = %+v", lookup)
	}
	if !strings.Contains(string(lookup.InputSchema), `"order_id"`) {
		t.Errorf("lookup_order.input_schema = %s", lookup.InputSchema)
	}

	refund := s.Tools["refund_order"].Effect.Sensitive
	if refund == nil || refund.Quantity != "/amount" || refund.Unit.Pointer == nil ||
		*refund.Unit.Pointer != "/currency" || refund.Unit.Const != nil {
		t.Errorf("refund_order.effect.sensitive = %+v", refund)
	}

	points := s.Tools["grant_points"]
	if points.Effect.Sensitive == nil || points.Effect.Sensitive.Unit.Const == nil ||
		*points.Effect.Sensitive.Unit.Const != "points" || points.Effect.Sensitive.Unit.Pointer != nil {
		t.Errorf("grant_points.effect.sensitive = %+v", points.Effect.Sensitive)
	}
	if string(points.OutputSchema) != "true" {
		t.Errorf("grant_points.output_schema = %s, want the boolean schema as written", points.OutputSchema)
	}
}

func TestDecodeAcceptsWhatThePlanListsForLaterSections(t *testing.T) {
	cases := map[string]func(t *testing.T, doc object){
		"model.parameters with nested content": func(t *testing.T, doc object) {
			in(t, doc, "model")["parameters"] = object{"temperature": 0.7, "stop": []any{"END"}, "extra": object{"a": nil}}
		},
		"model.parameters empty": func(t *testing.T, doc object) {
			in(t, doc, "model")["parameters"] = object{}
		},
		"no tools": func(t *testing.T, doc object) {
			doc["tools"] = object{}
		},
		"no untrusted inputs and no domains": func(t *testing.T, doc object) {
			doc["untrusted_inputs"] = []any{}
			in(t, doc, "allowlists")["domains"] = []any{}
		},
		"session shape": func(t *testing.T, doc object) {
			in(t, doc, "run")["shape"] = "session"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			doc := validDocument(t)
			mutate(t, doc)
			if _, err := Decode(encode(t, doc)); err != nil {
				t.Errorf("Decode: %v", err)
			}
		})
	}
}

func TestDecodeRefuses(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, doc object)
		path   string
	}{
		// Unknown fields.
		{"unknown field at the top level", func(t *testing.T, doc object) {
			doc["owner"] = "someone"
		}, "/owner"},
		{"unknown field inside a tool", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order")["url"] = "https://orders.example.com"
		}, "/tools/lookup_order/url"},
		{"unknown field inside an effect", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order", "effect")["reversible"] = true
		}, "/tools/lookup_order/effect/reversible"},
		{"unknown field inside max_cost", func(t *testing.T, doc object) {
			in(t, doc, "budgets", "max_cost")["per"] = "run"
		}, "/budgets/max_cost/per"},

		// Missing required fields.
		{"missing risk_tier", func(t *testing.T, doc object) {
			delete(doc, "risk_tier")
		}, "/risk_tier"},
		{"missing memory", func(t *testing.T, doc object) {
			delete(doc, "memory")
		}, "/memory"},
		{"missing task.success_criterion", func(t *testing.T, doc object) {
			delete(in(t, doc, "task"), "success_criterion")
		}, "/task/success_criterion"},
		{"missing model.parameters", func(t *testing.T, doc object) {
			delete(in(t, doc, "model"), "parameters")
		}, "/model/parameters"},
		{"missing guardrails.output", func(t *testing.T, doc object) {
			delete(in(t, doc, "guardrails"), "output")
		}, "/guardrails/output"},
		{"missing policies.sensitive_operations", func(t *testing.T, doc object) {
			delete(in(t, doc, "policies"), "sensitive_operations")
		}, "/policies/sensitive_operations"},
		{"missing tool input_schema", func(t *testing.T, doc object) {
			delete(in(t, doc, "tools", "lookup_order"), "input_schema")
		}, "/tools/lookup_order/input_schema"},

		// The three declarations of a tool.
		{"tool without effect", func(t *testing.T, doc object) {
			delete(in(t, doc, "tools", "lookup_order"), "effect")
		}, "/tools/lookup_order/effect"},
		{"tool without private_data", func(t *testing.T, doc object) {
			delete(in(t, doc, "tools", "lookup_order"), "private_data")
		}, "/tools/lookup_order/private_data"},
		{"tool without untrusted_output", func(t *testing.T, doc object) {
			delete(in(t, doc, "tools", "lookup_order"), "untrusted_output")
		}, "/tools/lookup_order/untrusted_output"},
		{"effect without kind", func(t *testing.T, doc object) {
			delete(in(t, doc, "tools", "lookup_order", "effect"), "kind")
		}, "/tools/lookup_order/effect/kind"},
		{"effect kind outside the three", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order", "effect")["kind"] = "delete"
		}, "/tools/lookup_order/effect/kind"},
		{"private_data that is not a boolean", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order")["private_data"] = "yes"
		}, "/tools/lookup_order/private_data"},

		// The sensitive block.
		{"sensitive without quantity", func(t *testing.T, doc object) {
			delete(in(t, doc, "tools", "refund_order", "effect", "sensitive"), "quantity")
		}, "/tools/refund_order/effect/sensitive/quantity"},
		{"sensitive without unit", func(t *testing.T, doc object) {
			delete(in(t, doc, "tools", "refund_order", "effect", "sensitive"), "unit")
		}, "/tools/refund_order/effect/sensitive/unit"},
		{"unit with neither pointer nor const", func(t *testing.T, doc object) {
			in(t, doc, "tools", "refund_order", "effect", "sensitive")["unit"] = object{}
		}, "/tools/refund_order/effect/sensitive/unit"},
		{"unit with both pointer and const", func(t *testing.T, doc object) {
			in(t, doc, "tools", "refund_order", "effect", "sensitive")["unit"] = object{"pointer": "/currency", "const": "USD"}
		}, "/tools/refund_order/effect/sensitive/unit"},
		{"quantity that is not a string", func(t *testing.T, doc object) {
			in(t, doc, "tools", "refund_order", "effect", "sensitive")["quantity"] = 100
		}, "/tools/refund_order/effect/sensitive/quantity"},

		// Tool schemas.
		{"input_schema that is not valid JSON Schema", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order")["input_schema"] = object{"type": 12}
		}, "/tools/lookup_order/input_schema/type"},
		{"output_schema that is not valid JSON Schema, nested", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order")["output_schema"] = object{
				"type":       "object",
				"properties": object{"status": object{"minLength": -1}},
			}
		}, "/tools/lookup_order/output_schema/properties/status/minLength"},
		{"input_schema that is neither an object nor a boolean", func(t *testing.T, doc object) {
			in(t, doc, "tools", "lookup_order")["input_schema"] = "object"
		}, "/tools/lookup_order/input_schema"},

		// Sections of later mechanisms.
		{"model.parameters that is not an object", func(t *testing.T, doc object) {
			in(t, doc, "model")["parameters"] = []any{}
		}, "/model/parameters"},
		{"non-empty policies.approval", func(t *testing.T, doc object) {
			in(t, doc, "policies")["approval"] = object{"required_for": "refund_order"}
		}, "/policies/approval/required_for"},
		{"non-empty policies.sensitive_operations", func(t *testing.T, doc object) {
			in(t, doc, "policies")["sensitive_operations"] = object{"limits": []any{}}
		}, "/policies/sensitive_operations/limits"},
		{"non-empty guardrails.input", func(t *testing.T, doc object) {
			in(t, doc, "guardrails")["input"] = []any{"pii"}
		}, "/guardrails/input"},
		{"non-empty guardrails.output", func(t *testing.T, doc object) {
			in(t, doc, "guardrails")["output"] = []any{object{"checker": "pii"}}
		}, "/guardrails/output"},
		{"memory with corpora", func(t *testing.T, doc object) {
			doc["memory"] = object{"corpora": []any{}}
		}, "/memory/corpora"},
		{"memory with write", func(t *testing.T, doc object) {
			doc["memory"] = object{"write": false}
		}, "/memory/write"},
		{"memory with retention", func(t *testing.T, doc object) {
			doc["memory"] = object{"retention": "30d"}
		}, "/memory/retention"},
		{"memory with compaction", func(t *testing.T, doc object) {
			doc["memory"] = object{"compaction": "summary"}
		}, "/memory/compaction"},

		// Single-field rules of the format.
		{"untrusted_inputs value other than run_input", func(t *testing.T, doc object) {
			doc["untrusted_inputs"] = []any{"run_input", "tool_output"}
		}, "/untrusted_inputs/1"},
		{"currency in lowercase", func(t *testing.T, doc object) {
			in(t, doc, "budgets", "max_cost")["currency"] = "usd"
		}, "/budgets/max_cost/currency"},
		{"currency with four letters", func(t *testing.T, doc object) {
			in(t, doc, "budgets", "max_cost")["currency"] = "USDX"
		}, "/budgets/max_cost/currency"},
		{"currency with two letters", func(t *testing.T, doc object) {
			in(t, doc, "budgets", "max_cost")["currency"] = "US"
		}, "/budgets/max_cost/currency"},
		{"currency with a digit", func(t *testing.T, doc object) {
			in(t, doc, "budgets", "max_cost")["currency"] = "US1"
		}, "/budgets/max_cost/currency"},
		{"format of another version", func(t *testing.T, doc object) {
			doc["format"] = "cursus.agent/v2"
		}, "/format"},
		{"run shape outside the two", func(t *testing.T, doc object) {
			in(t, doc, "run")["shape"] = "batch"
		}, "/run/shape"},
		{"risk tier outside the three", func(t *testing.T, doc object) {
			doc["risk_tier"] = "critical"
		}, "/risk_tier"},
		{"empty model.provider", func(t *testing.T, doc object) {
			in(t, doc, "model")["provider"] = ""
		}, "/model/provider"},
		{"amount with a fraction", func(t *testing.T, doc object) {
			in(t, doc, "budgets", "max_cost")["amount"] = 1.5
		}, "/budgets/max_cost/amount"},
		{"amount written with a decimal point", func(t *testing.T, doc object) {
			in(t, doc, "budgets", "max_cost")["amount"] = json.RawMessage("150.0")
		}, "/budgets/max_cost/amount"},
		{"max_steps written with an exponent", func(t *testing.T, doc object) {
			in(t, doc, "budgets")["max_steps"] = json.RawMessage("1e2")
		}, "/budgets/max_steps"},
		{"max_steps beyond int64", func(t *testing.T, doc object) {
			in(t, doc, "budgets")["max_steps"] = json.RawMessage("9223372036854775808")
		}, "/budgets/max_steps"},
		{"amount below int64", func(t *testing.T, doc object) {
			in(t, doc, "budgets", "max_cost")["amount"] = json.RawMessage("-9223372036854775809")
		}, "/budgets/max_cost/amount"},
		{"tools that is a list", func(t *testing.T, doc object) {
			doc["tools"] = []any{}
		}, "/tools"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := validDocument(t)
			tc.mutate(t, doc)
			requireFaultAt(t, faultsOf(t, encode(t, doc)), tc.path)
		})
	}
}

func TestDecodeReportsEveryFault(t *testing.T) {
	doc := validDocument(t)
	doc["owner"] = "someone"
	delete(doc, "risk_tier")
	delete(in(t, doc, "tools", "lookup_order"), "private_data")
	in(t, doc, "budgets", "max_cost")["currency"] = "usd"
	in(t, doc, "tools", "refund_order")["input_schema"] = object{"type": 12}

	faults := faultsOf(t, encode(t, doc))
	for _, path := range []string{
		"/owner",
		"/risk_tier",
		"/tools/lookup_order/private_data",
		"/budgets/max_cost/currency",
		"/tools/refund_order/input_schema/type",
	} {
		requireFaultAt(t, faults, path)
	}
	if !slices.IsSortedFunc(faults, func(a, b Fault) int { return strings.Compare(a.Path, b.Path) }) {
		t.Errorf("faults are not sorted by path: %+v", faults)
	}
}

func TestDecodeReportsAnIntegerOutOfRangeWithTheOtherFaults(t *testing.T) {
	doc := validDocument(t)
	doc["owner"] = "someone"
	in(t, doc, "budgets")["max_steps"] = json.RawMessage("9223372036854775808")

	faults := faultsOf(t, encode(t, doc))
	requireFaultAt(t, faults, "/owner")
	requireFaultAt(t, faults, "/budgets/max_steps")
}

func TestDecodeAcceptsTheBoundsOfInt64(t *testing.T) {
	doc := validDocument(t)
	in(t, doc, "budgets")["max_steps"] = json.RawMessage("9223372036854775807")
	in(t, doc, "budgets", "max_cost")["amount"] = json.RawMessage("-9223372036854775808")

	s, err := Decode(encode(t, doc))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if s.Budgets.MaxSteps != math.MaxInt64 || s.Budgets.MaxCost.Amount != math.MinInt64 {
		t.Errorf("budgets = %+v", s.Budgets)
	}
}

func TestDecodeSortsArrayIndexesByValue(t *testing.T) {
	doc := validDocument(t)
	inputs := make([]any, 11)
	for i := range inputs {
		inputs[i] = "run_input"
	}
	inputs[2], inputs[10] = "tool_output", "tool_output"
	doc["untrusted_inputs"] = inputs

	var paths []string
	for _, f := range faultsOf(t, encode(t, doc)) {
		paths = append(paths, f.Path)
	}
	if want := []string{"/untrusted_inputs/2", "/untrusted_inputs/10"}; !slices.Equal(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestDecodeRefusesInvalidUTF8(t *testing.T) {
	valid, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"in a decoded string":  bytes.Replace(valid, []byte("order-helper"), []byte("order-\xff"), 1),
		"inside a tool schema": bytes.Replace(valid, []byte(`"order_id"`), []byte("\"order_\xff\""), 1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if bytes.Equal(data, valid) {
				t.Fatal("the fixture was not changed")
			}
			requireFaultAt(t, faultsOf(t, data), "")
		})
	}
}

func TestDecodeFromConcurrentCallers(t *testing.T) {
	doc := validDocument(t)
	good := encode(t, doc)
	doc["owner"] = "someone"
	in(t, doc, "budgets", "max_cost")["currency"] = "usd"
	bad := encode(t, doc)

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 20 {
				if _, err := Decode(good); err != nil {
					t.Errorf("Decode of a conforming spec: %v", err)
				}
				var derr *DecodeError
				if _, err := Decode(bad); !errors.As(err, &derr) || len(derr.Faults) != 2 {
					t.Errorf("Decode of a spec with two faults: %v", err)
				}
			}
		})
	}
	wg.Wait()
}

func TestDecodeEscapesThePath(t *testing.T) {
	doc := validDocument(t)
	tools := in(t, doc, "tools")
	tool := in(t, doc, "tools", "lookup_order")
	delete(tool, "effect")
	tools["a/b~c"] = tool

	requireFaultAt(t, faultsOf(t, encode(t, doc)), "/tools/a~1b~0c/effect")
}

func TestDecodeRefusesWhatIsNotOneJSONDocument(t *testing.T) {
	valid, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"empty input":       nil,
		"truncated":         valid[:len(valid)/2],
		"trailing document": append(slices.Clone(valid), []byte(`{"format":"cursus.agent/v1"}`)...),
		"not JSON":          []byte("format: cursus.agent/v1\n"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			requireFaultAt(t, faultsOf(t, data), "")
		})
	}

	t.Run("a document that is not an object", func(t *testing.T) {
		requireFaultAt(t, faultsOf(t, []byte(`["cursus.agent/v1"]`)), "")
	})
}

func TestDecodeErrorNamesEachPath(t *testing.T) {
	err := &DecodeError{Faults: []Fault{
		{Path: "", Message: "not a JSON document"},
		{Path: "/risk_tier", Message: "required field is missing"},
	}}
	got := err.Error()
	for _, want := range []string{"(document): not a JSON document", "/risk_tier: required field is missing"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, want it to contain %q", got, want)
		}
	}
}

// The schema and the Go types are two definitions of one format. This test
// fails when a field exists in one and not in the other, or when the schema
// lets a field be absent that the types cannot represent as absent.
func TestTypesAgreeWithSchema(t *testing.T) {
	var root object
	if err := json.Unmarshal(schemaJSON, &root); err != nil {
		t.Fatal(err)
	}
	compareWithSchema(t, root, root, reflect.TypeFor[Spec](), "")
}

func compareWithSchema(t *testing.T, root, node object, typ reflect.Type, path string) {
	t.Helper()
	if ref, ok := node["$ref"].(string); ok {
		name, local := strings.CutPrefix(ref, "#/$defs/")
		if !local {
			return
		}
		node = root["$defs"].(object)[name].(object)
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.Map:
		values, ok := node["additionalProperties"].(object)
		if !ok {
			t.Errorf("%s: the type is a map and the schema has no schema for its values", path)
			return
		}
		compareWithSchema(t, root, values, typ.Elem(), path+"/*")

	case reflect.Struct:
		if open, ok := node["additionalProperties"].(bool); !ok || open {
			t.Errorf("%s: the schema leaves the object open", path)
		}
		properties, _ := node["properties"].(object)
		var required []string
		for _, r := range anySlice(node["required"]) {
			required = append(required, r.(string))
		}

		seen := map[string]bool{}
		for i := range typ.NumField() {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				t.Errorf("%s: field %s has no JSON name", path, field.Name)
				continue
			}
			seen[name] = true
			fieldPath := path + "/" + name

			property, ok := properties[name].(object)
			if !ok {
				t.Errorf("%s: in the types and not in the schema", fieldPath)
				continue
			}
			optional := field.Type.Kind() == reflect.Pointer
			if optional == slices.Contains(required, name) {
				t.Errorf("%s: required in the schema = %t, pointer in the types = %t", fieldPath, !optional, optional)
			}
			compareWithSchema(t, root, property, field.Type, fieldPath)
		}
		for name := range properties {
			if !seen[name] {
				t.Errorf("%s/%s: in the schema and not in the types", path, name)
			}
		}
	}
}

func anySlice(v any) []any {
	s, _ := v.([]any)
	return s
}
