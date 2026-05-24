package dsl

import (
	"encoding/json"
	"strings"
)

// DefaultAggregationDepth is the recursive expansion used for the
// aggregation result fragment when callers don't specify their own.
// Three levels covers typical multi-level aggregations.
const DefaultAggregationDepth = 3

// AggregationFragment builds the GraphQL selection set for an aggregation
// result, finitely expanded to depth levels. GraphQL forbids recursive
// fragments, so the codegen has to settle for a fixed depth.
//
// At depth=N each Bucket gets a nested `Aggregations { ... }` selection
// containing the same shape, repeated until depth is exhausted.
func AggregationFragment(depth int) string {
	if depth <= 0 {
		depth = DefaultAggregationDepth
	}
	var b strings.Builder
	writeAggregationFragment(&b, depth)
	return b.String()
}

func writeAggregationFragment(b *strings.Builder, depth int) {
	b.WriteString("Aggregations { Name Buckets { Key Count")
	if depth > 1 {
		b.WriteString(" ")
		writeAggregationFragment(b, depth-1)
	}
	b.WriteString(" } }")
}

// Aggregation is the typed result payload for one aggregation entry. The
// server returns these inside a SearchResult.Aggregations slice as raw JSON;
// use Aggregation.UnmarshalJSON or DecodeAggregations to read them.
type Aggregation struct {
	Name    string   `json:"Name"`
	Buckets []Bucket `json:"Buckets"`
}

// Bucket is one aggregation bucket. Key is left as raw JSON so the caller
// can decode it into the right scalar type (string, bool, int, ...) without
// a centralised type switch.
type Bucket struct {
	Key          json.RawMessage `json:"Key"`
	Count        int64           `json:"Count"`
	Aggregations []Aggregation   `json:"Aggregations,omitempty"`
}

// DecodeAggregations parses a SearchResult.Aggregations slice (raw JSON
// payloads returned by the server) into typed Aggregation values.
//
//	var typed []dsl.Aggregation
//	typed, err := dsl.DecodeAggregations(searchResult.Aggregations)
func DecodeAggregations(raw []json.RawMessage) ([]Aggregation, error) {
	out := make([]Aggregation, 0, len(raw))
	for _, r := range raw {
		var a Aggregation
		if err := json.Unmarshal(r, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// KeyAsString returns the Bucket's key as a Go string. Returns "" on
// non-string keys (caller should use json.Unmarshal(b.Key, &T) for typed
// access).
func (b Bucket) KeyAsString() string {
	var s string
	if err := json.Unmarshal(b.Key, &s); err != nil {
		return ""
	}
	return s
}

// KeyAsBool returns the Bucket's key as a bool, or (false, false) if the
// payload is not a boolean.
func (b Bucket) KeyAsBool() (bool, bool) {
	var v bool
	if err := json.Unmarshal(b.Key, &v); err == nil {
		return v, true
	}
	// The server sometimes returns boolean aggregation keys as JSON
	// strings ("true" / "false"). Fall back to string form.
	var s string
	if err := json.Unmarshal(b.Key, &s); err == nil {
		switch s {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}
