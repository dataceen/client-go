// Package prototypes contains Phase 2 DSL prototypes (A, B, C). Each variant
// in a/, b/, c/ produces the same canonical QueryPlan from a different
// user-facing API. The shared serializer turns a QueryPlan into a GraphQL
// request string, so the prototypes are directly comparable on output.
//
// IMPORTANT: operator and direction casing follows the Dataceen server
// (decisions.md #26–30): lowercase operators (eq/like/and/or/not/...) and
// PascalCase enum values (Ascending/Descending). The TS prototypes used
// UPPERCASE and were corrected in fas 3 — Go skips that stumble.
package prototypes

// QueryPlan is the language-agnostic intermediate form. Every prototype
// outputs one of these. The serializer is the single source of truth for
// wire format.
type QueryPlan struct {
	Entity  string
	Size    int
	Cursor  string
	Filter  FilterNode // may be nil → "where: null"
	Fields  []FieldNode
	OrderBy []OrderByNode
}

// FilterNode is the sum type {cond, and, or, not}.
type FilterNode interface{ filterNode() }

// CondNode is a leaf: `Field { op: value }`.
type CondNode struct {
	Field string
	Op    string // lowercase: eq, neq, gt, gte, lt, lte, like, nlike, contains, ncontains, in, nin, is_null, exists
	Value any    // string|int|float64|bool|nil|[]any
}

func (CondNode) filterNode() {}

// AndNode wraps `{ and: [...] }`.
type AndNode struct{ Parts []FilterNode }

func (AndNode) filterNode() {}

// OrNode wraps `{ or: [...] }`.
type OrNode struct{ Parts []FilterNode }

func (OrNode) filterNode() {}

// NotNode wraps `{ not: ... }`.
type NotNode struct{ Inner FilterNode }

func (NotNode) filterNode() {}

// FieldNode is the sum type {scalar, nested}.
type FieldNode interface{ fieldNode() }

// ScalarField is a leaf field selection.
type ScalarField struct{ Name string }

func (ScalarField) fieldNode() {}

// NestedField groups child selections under Name.
type NestedField struct {
	Name     string
	Children []FieldNode
}

func (NestedField) fieldNode() {}

// OrderByNode picks a direction for one field. Direction values match the
// server enum: Ascending | Descending.
type OrderByNode struct {
	Field     string
	Direction string // "Ascending" | "Descending"
}
