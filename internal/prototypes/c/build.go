// Package c is prototype C: typed fluent builders. Each comparison is a
// stand-alone call (`Field("X").Like("y")`); composition uses And/Or/Not
// constructors that take variadic FilterExpr children.
//
// Strategy is generic over the entity (no per-entity codegen needed) at
// the cost of stringly-typed field names. Callers can wrap with per-entity
// helpers if they want stronger guarantees.
package c

import (
	pt "github.com/dataceen/client-go/internal/prototypes"
)

// FilterExpr is the value any builder produces. The build process collects
// it into the shared AST.
type FilterExpr interface{ toNode() pt.FilterNode }

// FieldExpr names a column you can compare against. Returned by F(name).
type FieldExpr struct{ name string }

// F starts a comparison: `F("CustomerId").Like("cst")`.
func F(name string) FieldExpr { return FieldExpr{name: name} }

// Eq produces `Field { eq: value }`.
func (f FieldExpr) Eq(v any) FilterExpr { return condExpr{f.name, "eq", v} }
func (f FieldExpr) Neq(v any) FilterExpr { return condExpr{f.name, "neq", v} }
func (f FieldExpr) Gt(v any) FilterExpr  { return condExpr{f.name, "gt", v} }
func (f FieldExpr) Gte(v any) FilterExpr { return condExpr{f.name, "gte", v} }
func (f FieldExpr) Lt(v any) FilterExpr  { return condExpr{f.name, "lt", v} }
func (f FieldExpr) Lte(v any) FilterExpr { return condExpr{f.name, "lte", v} }

// Like is Elasticsearch token-match per decisions.md #39 — never wildcards
// (`%`/`*`), never delimiters (`-`/space/`.`).
func (f FieldExpr) Like(v string) FilterExpr     { return condExpr{f.name, "like", v} }
func (f FieldExpr) NLike(v string) FilterExpr    { return condExpr{f.name, "nlike", v} }
func (f FieldExpr) Contains(v string) FilterExpr { return condExpr{f.name, "contains", v} }
func (f FieldExpr) NContains(v string) FilterExpr {
	return condExpr{f.name, "ncontains", v}
}
func (f FieldExpr) In(vs ...any) FilterExpr  { return condExpr{f.name, "in", anySlice(vs)} }
func (f FieldExpr) NIn(vs ...any) FilterExpr { return condExpr{f.name, "nin", anySlice(vs)} }
func (f FieldExpr) IsNull() FilterExpr       { return condExpr{f.name, "is_null", true} }
func (f FieldExpr) Exists() FilterExpr       { return condExpr{f.name, "exists", true} }

func anySlice(vs []any) []any { return vs }

type condExpr struct {
	field string
	op    string
	value any
}

func (c condExpr) toNode() pt.FilterNode {
	return pt.CondNode{Field: c.field, Op: c.op, Value: c.value}
}

// And combines child filters into `{ and: [...] }`.
func And(parts ...FilterExpr) FilterExpr { return groupExpr{"and", parts} }

// Or combines child filters into `{ or: [...] }`.
func Or(parts ...FilterExpr) FilterExpr { return groupExpr{"or", parts} }

// Not wraps a child into `{ not: ... }`.
func Not(inner FilterExpr) FilterExpr { return notExpr{inner} }

type groupExpr struct {
	kind  string // "and" | "or"
	parts []FilterExpr
}

func (g groupExpr) toNode() pt.FilterNode {
	parts := make([]pt.FilterNode, len(g.parts))
	for i, p := range g.parts {
		parts[i] = p.toNode()
	}
	if g.kind == "and" {
		return pt.AndNode{Parts: parts}
	}
	return pt.OrNode{Parts: parts}
}

type notExpr struct{ inner FilterExpr }

func (n notExpr) toNode() pt.FilterNode {
	return pt.NotNode{Inner: n.inner.toNode()}
}

// FieldsBuilder collects scalar and nested field selections into an ordered
// slice. Returned by Fields().
type FieldsBuilder struct {
	nodes []pt.FieldNode
}

// Fields starts a new selection.
func Fields() *FieldsBuilder { return &FieldsBuilder{} }

// Pick adds one or more scalar fields by name.
func (b *FieldsBuilder) Pick(names ...string) *FieldsBuilder {
	for _, n := range names {
		b.nodes = append(b.nodes, pt.ScalarField{Name: n})
	}
	return b
}

// Nested adds a nested selection. The closure receives a fresh builder for
// the child scope; whatever it picks becomes the nested children.
func (b *FieldsBuilder) Nested(name string, build func(*FieldsBuilder)) *FieldsBuilder {
	child := Fields()
	build(child)
	b.nodes = append(b.nodes, pt.NestedField{Name: name, Children: child.nodes})
	return b
}

// build returns the shared FieldNode slice.
func (b *FieldsBuilder) build() []pt.FieldNode { return b.nodes }

// OrderByBuilder accumulates {field, direction} pairs. Asc/Desc append.
type OrderByBuilder struct {
	nodes []pt.OrderByNode
}

// OrderBy starts an empty ordering.
func OrderBy() *OrderByBuilder { return &OrderByBuilder{} }

// Asc appends `{ field: Ascending }`.
func (o *OrderByBuilder) Asc(field string) *OrderByBuilder {
	o.nodes = append(o.nodes, pt.OrderByNode{Field: field, Direction: "Ascending"})
	return o
}

// Desc appends `{ field: Descending }`.
func (o *OrderByBuilder) Desc(field string) *OrderByBuilder {
	o.nodes = append(o.nodes, pt.OrderByNode{Field: field, Direction: "Descending"})
	return o
}

func (o *OrderByBuilder) build() []pt.OrderByNode { return o.nodes }

// FindInput is the user-facing entry. Generic over entity name (no per-
// entity types in this prototype).
type FindInput struct {
	Entity  string
	Size    int
	Cursor  string
	Filter  FilterExpr
	Fields  *FieldsBuilder
	OrderBy *OrderByBuilder
}

// Build returns the shared AST plan.
func Build(in FindInput) pt.QueryPlan {
	plan := pt.QueryPlan{
		Entity: in.Entity,
		Size:   in.Size,
		Cursor: in.Cursor,
	}
	if in.Filter != nil {
		plan.Filter = in.Filter.toNode()
	}
	if in.Fields != nil {
		plan.Fields = in.Fields.build()
	}
	if in.OrderBy != nil {
		plan.OrderBy = in.OrderBy.build()
	}
	return plan
}
