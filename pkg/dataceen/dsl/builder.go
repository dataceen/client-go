package dsl

// FilterExpr is the value any filter builder produces. Build paths collect it
// into the AST via toNode.
type FilterExpr interface{ toNode() FilterNode }

// FieldExpr names a column for comparison. Returned by F(name).
type FieldExpr struct{ name string }

// F starts a comparison: `F("CustomerId").Like("cst")`.
func F(name string) FieldExpr { return FieldExpr{name: name} }

// Eq produces `Field { eq: value }`.
func (f FieldExpr) Eq(v any) FilterExpr  { return condExpr{f.name, "eq", v} }
func (f FieldExpr) Neq(v any) FilterExpr { return condExpr{f.name, "neq", v} }
func (f FieldExpr) Gt(v any) FilterExpr  { return condExpr{f.name, "gt", v} }
func (f FieldExpr) Gte(v any) FilterExpr { return condExpr{f.name, "gte", v} }
func (f FieldExpr) Lt(v any) FilterExpr  { return condExpr{f.name, "lt", v} }
func (f FieldExpr) Lte(v any) FilterExpr { return condExpr{f.name, "lte", v} }

// Like is Elasticsearch token-match (decisions.md #39): never wildcards
// (`%`/`*`), never delimiters (`-`/space/`.`).
func (f FieldExpr) Like(v string) FilterExpr     { return condExpr{f.name, "like", v} }
func (f FieldExpr) NLike(v string) FilterExpr    { return condExpr{f.name, "nlike", v} }
func (f FieldExpr) Contains(v string) FilterExpr { return condExpr{f.name, "contains", v} }
func (f FieldExpr) NContains(v string) FilterExpr {
	return condExpr{f.name, "ncontains", v}
}
func (f FieldExpr) In(vs ...any) FilterExpr  { return condExpr{f.name, "in", append([]any{}, vs...)} }
func (f FieldExpr) NIn(vs ...any) FilterExpr { return condExpr{f.name, "nin", append([]any{}, vs...)} }
func (f FieldExpr) IsNull() FilterExpr       { return condExpr{f.name, "is_null", true} }
func (f FieldExpr) Exists() FilterExpr       { return condExpr{f.name, "exists", true} }

type condExpr struct {
	field string
	op    string
	value any
}

func (c condExpr) toNode() FilterNode {
	return CondNode{Field: c.field, Op: c.op, Value: c.value}
}

// And combines child filters: `{ and: [...] }`. Variadic for natural
// composition.
func And(parts ...FilterExpr) FilterExpr { return groupExpr{"and", parts} }

// Or combines child filters: `{ or: [...] }`.
func Or(parts ...FilterExpr) FilterExpr { return groupExpr{"or", parts} }

// Not negates a child filter: `{ not: ... }`.
func Not(inner FilterExpr) FilterExpr { return notExpr{inner} }

type groupExpr struct {
	kind  string // "and" | "or"
	parts []FilterExpr
}

func (g groupExpr) toNode() FilterNode {
	parts := make([]FilterNode, len(g.parts))
	for i, p := range g.parts {
		parts[i] = p.toNode()
	}
	if g.kind == "and" {
		return AndNode{Parts: parts}
	}
	return OrNode{Parts: parts}
}

type notExpr struct{ inner FilterExpr }

func (n notExpr) toNode() FilterNode { return NotNode{Inner: n.inner.toNode()} }

// FilterToNode converts a FilterExpr (or nil) into a FilterNode for inclusion
// in a QueryPlan. Returns nil if the expression is nil — which the serializer
// renders as `where: null`.
func FilterToNode(expr FilterExpr) FilterNode {
	if expr == nil {
		return nil
	}
	return expr.toNode()
}

// FieldsBuilder collects scalar and nested field selections in insertion order.
type FieldsBuilder struct {
	nodes []FieldNode
}

// Fields starts a new selection.
func Fields() *FieldsBuilder { return &FieldsBuilder{} }

// Pick adds one or more scalar fields by name.
func (b *FieldsBuilder) Pick(names ...string) *FieldsBuilder {
	for _, n := range names {
		b.nodes = append(b.nodes, ScalarField{Name: n})
	}
	return b
}

// Nested adds a nested selection. The closure picks children inside the
// nested scope.
func (b *FieldsBuilder) Nested(name string, build func(*FieldsBuilder)) *FieldsBuilder {
	child := Fields()
	build(child)
	b.nodes = append(b.nodes, NestedField{Name: name, Children: child.nodes})
	return b
}

// Append concatenates pre-built FieldNodes (e.g. those produced by a typed
// <Entity>Fields struct's ToNodes method).
func (b *FieldsBuilder) Append(nodes ...FieldNode) *FieldsBuilder {
	b.nodes = append(b.nodes, nodes...)
	return b
}

// Build returns the accumulated field nodes.
func (b *FieldsBuilder) Build() []FieldNode { return b.nodes }

// OrderByBuilder accumulates {field, direction} pairs.
type OrderByBuilder struct {
	nodes []OrderByNode
}

// OrderBy starts an empty ordering.
func OrderBy() *OrderByBuilder { return &OrderByBuilder{} }

// Asc appends `{ field: Ascending }`.
func (o *OrderByBuilder) Asc(field string) *OrderByBuilder {
	o.nodes = append(o.nodes, OrderByNode{Field: field, Direction: "Ascending"})
	return o
}

// Desc appends `{ field: Descending }`.
func (o *OrderByBuilder) Desc(field string) *OrderByBuilder {
	o.nodes = append(o.nodes, OrderByNode{Field: field, Direction: "Descending"})
	return o
}

// Build returns the accumulated order-by nodes.
func (o *OrderByBuilder) Build() []OrderByNode { return o.nodes }
