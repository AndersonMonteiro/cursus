package spec

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed schema.json
var schemaJSON []byte

// Fault is one place where a document leaves the format. Path is an RFC 6901
// JSON Pointer into the document, and the empty string is the document itself.
type Fault struct {
	Path    string
	Message string
}

// DecodeError lists every fault found in a document.
type DecodeError struct {
	Faults []Fault
}

func (e *DecodeError) Error() string {
	parts := make([]string, len(e.Faults))
	for i, f := range e.Faults {
		path := f.Path
		if path == "" {
			path = "(document)"
		}
		parts[i] = path + ": " + f.Message
	}
	return "spec does not conform to " + Format + ": " + strings.Join(parts, "; ")
}

// The name only identifies the embedded resource to the compiler; nothing is
// read from disk or from the network.
const schemaResource = "schema.json"

var compiledSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	// Set explicitly because the library states that its default may change.
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource(schemaResource, doc); err != nil {
		return nil, err
	}
	return c.Compile(schemaResource)
})

// Decode returns the spec that data holds, or a *DecodeError with every fault
// when data is not a document of the format.
func Decode(data []byte) (*Spec, error) {
	schema, err := compiledSchema()
	if err != nil {
		return nil, fmt.Errorf("compile the spec schema: %w", err)
	}

	// encoding/json replaces an invalid byte sequence with U+FFFD instead of
	// refusing it, which would return a spec that differs from what was sent.
	if !utf8.Valid(data) {
		return nil, &DecodeError{Faults: []Fault{{Message: "not a JSON document: invalid UTF-8"}}}
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, &DecodeError{Faults: []Fault{{Message: "not a JSON document: " + err.Error()}}}
	}

	if err := schema.Validate(doc); err != nil {
		var verr *jsonschema.ValidationError
		if !errors.As(err, &verr) {
			return nil, fmt.Errorf("validate the spec: %w", err)
		}
		return nil, &DecodeError{Faults: faults(verr)}
	}

	// The schema has already refused unknown fields. The option stays on so
	// that a field the schema gains and the types lack fails here instead of
	// being dropped.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return nil, &DecodeError{Faults: []Fault{typeFault(err)}}
	}
	return &s, nil
}

func faults(root *jsonschema.ValidationError) []Fault {
	type located struct {
		location []string
		message  string
	}
	var found []located
	add := func(message string, location []string, more ...string) {
		found = append(found, located{slices.Concat(location, more), message})
	}
	// Created per call: the package documents no guarantee that one printer
	// serves concurrent callers.
	printer := message.NewPrinter(language.English)

	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		// A node with causes only groups them; the faults are the leaves.
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		// The library reports a missing or an unknown property at the object
		// that holds it. The fault points at the property itself, which is
		// the place to edit.
		switch k := e.ErrorKind.(type) {
		case *kind.Required:
			for _, name := range k.Missing {
				add("required field is missing", e.InstanceLocation, name)
			}
		case *kind.AdditionalProperties:
			for _, name := range k.Properties {
				add("unknown field", e.InstanceLocation, name)
			}
		// The library prints both numbers rounded, which for the bounds of
		// an integer shows the same figure twice.
		case *kind.Maximum:
			add("must be at most "+k.Want.RatString(), e.InstanceLocation)
		case *kind.Minimum:
			add("must be at least "+k.Want.RatString(), e.InstanceLocation)
		default:
			add(k.LocalizedString(printer), e.InstanceLocation)
		}
	}
	walk(root)

	slices.SortFunc(found, func(a, b located) int {
		if c := slices.CompareFunc(a.location, b.location, compareTokens); c != 0 {
			return c
		}
		return strings.Compare(a.message, b.message)
	})

	out := make([]Fault, len(found))
	for i, f := range found {
		out[i] = Fault{Path: pointer(f.location), Message: f.message}
	}
	return slices.Compact(out)
}

// compareTokens orders two tokens of a path, with array indexes by value so
// that item 2 comes before item 10.
func compareTokens(a, b string) int {
	x, errA := strconv.ParseUint(a, 10, 64)
	y, errB := strconv.ParseUint(b, 10, 64)
	if errA == nil && errB == nil && x != y {
		if x < y {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func pointer(location []string) string {
	var sb strings.Builder
	for _, tok := range location {
		sb.WriteByte('/')
		sb.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(tok))
	}
	return sb.String()
}

// typeFault covers what the schema accepts and a Go type does not hold: an
// integer written with a decimal point or an exponent, such as 150.0, which
// JSON Schema counts as an integer and cannot be told to refuse.
func typeFault(err error) Fault {
	var terr *json.UnmarshalTypeError
	if errors.As(err, &terr) && terr.Field != "" {
		// encoding/json joins the field names with dots. Only the integers of
		// budgets can fail here, and none of their names holds a dot.
		return Fault{
			Path:    pointer(strings.Split(terr.Field, ".")),
			Message: fmt.Sprintf("%s does not fit the field", terr.Value),
		}
	}
	return Fault{Message: err.Error()}
}
