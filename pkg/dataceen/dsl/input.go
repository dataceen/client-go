package dsl

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// EncodeInput renders a Go value as GraphQL input-object syntax. Used by
// generated mutation methods (Create/Update/Delete) to inline payloads
// directly into the query string.
//
// Differences from JSON:
//   - Object keys are unquoted (`{ CustomerId: "x" }`, not `{"CustomerId":"x"}`)
//   - Strings are JSON-escaped exactly like JSON (so quoting is identical)
//   - Numbers, bools, null match JSON
//   - Arrays use `[a, b]`
//
// Field selection rules for structs:
//   - Honor `json:"name"` tags; fall back to the Go field name
//   - Skip unexported fields
//   - `omitempty` skips zero values (nil pointer, empty string, zero number,
//     false, empty slice/map). Tag `-` always skips.
//
// nil pointers serialise as `null`; non-nil pointers as their target. Maps
// emit keys sorted alphabetically (deterministic for tests).
func EncodeInput(v any) string {
	if v == nil {
		return "null"
	}
	return encodeValue(reflect.ValueOf(v))
}

func encodeValue(rv reflect.Value) string {
	if !rv.IsValid() {
		return "null"
	}
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return "null"
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Struct:
		return encodeStruct(rv)
	case reflect.Slice, reflect.Array:
		// Treat []byte as a string-like value? No — Dataceen has no bytes
		// scalar. Encode as a normal array.
		parts := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			parts[i] = encodeValue(rv.Index(i))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case reflect.Map:
		return encodeMap(rv)
	case reflect.String:
		b, _ := json.Marshal(rv.String())
		return string(b)
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'g', -1, 64)
	}
	// Last resort: JSON-encode and hope for the best.
	b, err := json.Marshal(rv.Interface())
	if err != nil {
		return "null"
	}
	return string(b)
}

// encodeStruct walks rv's exported fields (which must be a struct) using
// json-tag-aware naming and omitempty handling.
func encodeStruct(rv reflect.Value) string {
	t := rv.Type()
	var parts []string
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		// Embedded struct flattening (anonymous fields).
		if sf.Anonymous {
			inner := encodeValue(rv.Field(i))
			if strings.HasPrefix(inner, "{ ") && strings.HasSuffix(inner, " }") {
				parts = append(parts, strings.TrimSuffix(strings.TrimPrefix(inner, "{ "), " }"))
			}
			continue
		}
		name, omitempty, skip := parseJSONTag(sf)
		if skip {
			continue
		}
		fv := rv.Field(i)
		if omitempty && isZeroValue(fv) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", name, encodeValue(fv)))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func encodeMap(rv reflect.Value) string {
	keys := rv.MapKeys()
	// Stringify and sort keys for deterministic output.
	type kv struct {
		k string
		v reflect.Value
	}
	pairs := make([]kv, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, kv{k: fmt.Sprint(k.Interface()), v: rv.MapIndex(k)})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })

	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s: %s", p.k, encodeValue(p.v))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// parseJSONTag returns the wire name, omitempty flag, and skip flag for a
// struct field. Tag "-" always skips. Empty/missing tag uses the Go name.
func parseJSONTag(sf reflect.StructField) (name string, omitempty bool, skip bool) {
	tag, ok := sf.Tag.Lookup("json")
	if !ok || tag == "" {
		return sf.Name, false, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", false, true
	}
	if parts[0] == "" {
		name = sf.Name
	} else {
		name = parts[0]
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

func isZeroValue(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return rv.IsZero()
}
