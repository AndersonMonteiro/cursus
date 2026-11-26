package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"unicode/utf16"

	"github.com/gowebpki/jcs"
)

// The algorithm is named in the hash, so a later one is told apart by its
// prefix (ADR-0003).
const hashPrefix = "sha256:"

// Hash returns the identity of a spec that Decode returned: two writings of
// one spec give one hash. The spec is not modified.
func (s *Spec) Hash() (string, error) {
	normalized := *s
	normalized.UntrustedInputs = asSet(s.UntrustedInputs)
	normalized.Allowlists.Domains = asSet(s.Allowlists.Domains)

	doc, err := canonical(&normalized)
	if err != nil {
		return "", fmt.Errorf("canonical form of the spec: %w", err)
	}
	return digest(doc), nil
}

// canonical returns the RFC 8785 form of v as encoding/json writes it.
func canonical(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(data)
}

func digest(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hashPrefix + hex.EncodeToString(sum[:])
}

// asSet sorts by UTF-16 code unit, the order RFC 8785 gives to keys, so a
// client sorts the sets and the keys with one comparison.
func asSet(values []string) []string {
	out := slices.Clone(values)
	slices.SortFunc(out, func(a, b string) int {
		return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
	})
	return slices.Compact(out)
}
