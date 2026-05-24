package dsl

// QueryPlan is the language-agnostic intermediate form. Every builder/codegen
// path produces one of these; the serializer is the only authoritative path
// to wire format.
type QueryPlan struct {
	Entity  string
	Size    int
	Cursor  string
	Filter  FilterNode // may be nil → "where: null"
	Fields  []FieldNode
	OrderBy []OrderByNode
}

// FilterNode is the sum type {Cond, And, Or, Not}.
type FilterNode interface{ filterNode() }

// CondNode is a leaf: `Field { op: value }`.
type CondNode struct {
	Field string
	Op    string // lowercase: eq/neq/gt/gte/lt/lte/like/nlike/contains/ncontains/in/nin/is_null/exists
	Value any
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

// FieldNode is the sum type {Scalar, Nested}.
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
	Direction string
}
