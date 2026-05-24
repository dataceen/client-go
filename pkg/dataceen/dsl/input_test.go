package dsl_test

import (
	"testing"

	"github.com/dataceen/client-go/pkg/dataceen/dsl"
)

func TestEncodeInput_Scalars(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "null"},
		{"hello", `"hello"`},
		{`with "quotes"`, `"with \"quotes\""`},
		{42, "42"},
		{int64(-7), "-7"},
		{true, "true"},
		{false, "false"},
		{3.14, "3.14"},
	}
	for _, c := range cases {
		got := dsl.EncodeInput(c.in)
		if got != c.want {
			t.Errorf("EncodeInput(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEncodeInput_StructWithJSONTags(t *testing.T) {
	type customer struct {
		ID         string  `json:"_id,omitempty"`
		CustomerID string  `json:"CustomerId"`
		IsActive   bool    `json:"IsActive"`
		ExternalRef *string `json:"ExternalReference,omitempty"`
	}
	got := dsl.EncodeInput(customer{
		CustomerID: "cst-1",
		IsActive:   true,
	})
	want := `{ CustomerId: "cst-1", IsActive: true }`
	if got != want {
		t.Errorf("got: %s\nwant: %s", got, want)
	}
}

func TestEncodeInput_StructWithPointers(t *testing.T) {
	type update struct {
		CustomerID  *string `json:"CustomerId,omitempty"`
		IsActive    *bool   `json:"IsActive,omitempty"`
		Description *string `json:"Description,omitempty"`
	}
	id := "cst-2"
	active := true
	got := dsl.EncodeInput(&update{
		CustomerID: &id,
		IsActive:   &active,
		// Description nil → omitted
	})
	want := `{ CustomerId: "cst-2", IsActive: true }`
	if got != want {
		t.Errorf("got: %s\nwant: %s", got, want)
	}
}

func TestEncodeInput_NilPointerFieldEmitsNull(t *testing.T) {
	type withRequired struct {
		Name *string `json:"Name"` // no omitempty — explicit null wanted
	}
	got := dsl.EncodeInput(withRequired{})
	want := `{ Name: null }`
	if got != want {
		t.Errorf("got: %s\nwant: %s", got, want)
	}
}

func TestEncodeInput_Slice(t *testing.T) {
	got := dsl.EncodeInput([]string{"a", "b", "c"})
	want := `["a", "b", "c"]`
	if got != want {
		t.Errorf("got: %s\nwant: %s", got, want)
	}
}

func TestEncodeInput_NestedStruct(t *testing.T) {
	type inner struct {
		First string `json:"FirstName,omitempty"`
		Last  string `json:"LastName,omitempty"`
	}
	type outer struct {
		ID   string `json:"_id"`
		Name *inner `json:"Name,omitempty"`
	}
	got := dsl.EncodeInput(&outer{
		ID:   "c1",
		Name: &inner{First: "Ada", Last: "Lovelace"},
	})
	want := `{ _id: "c1", Name: { FirstName: "Ada", LastName: "Lovelace" } }`
	if got != want {
		t.Errorf("got: %s\nwant: %s", got, want)
	}
}

func TestEncodeInput_MapDeterministic(t *testing.T) {
	got := dsl.EncodeInput(map[string]any{
		"z": 1,
		"a": "x",
		"m": true,
	})
	// Keys must be sorted alphabetically.
	want := `{ a: "x", m: true, z: 1 }`
	if got != want {
		t.Errorf("got: %s\nwant: %s", got, want)
	}
}

func TestEncodeInput_OmitemptySkipsEmptyString(t *testing.T) {
	type rec struct {
		A string `json:"A,omitempty"`
		B string `json:"B"`
	}
	got := dsl.EncodeInput(rec{A: "", B: ""})
	want := `{ B: "" }`
	if got != want {
		t.Errorf("got: %s\nwant: %s", got, want)
	}
}
