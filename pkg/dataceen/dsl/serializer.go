package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PlanToGraphQL turns a QueryPlan into a Find{Entity} GraphQL request. The
// Items selection is always wrapped in `Items { ... } Cursor` — the standard
// Find result envelope. Search-shape requests are emitted via a separate
// helper in Phase 5.
func PlanToGraphQL(plan QueryPlan) (operationName, query string) {
	operationName = "Find" + plan.Entity

	var b strings.Builder
	fmt.Fprintf(&b, "query {%s(size: %d, cursor: %q", operationName, plan.Size, plan.Cursor)

	if plan.Filter == nil {
		b.WriteString(", where: null")
	} else {
		b.WriteString(", where: ")
		writeFilter(&b, plan.Filter)
	}

	if len(plan.OrderBy) > 0 {
		b.WriteString(", order_by: [")
		for i, ob := range plan.OrderBy {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "{ %s: %s }", ob.Field, ob.Direction)
		}
		b.WriteString("]")
	}

	b.WriteString(") { Items { ")
	writeFields(&b, plan.Fields)
	b.WriteString(" } Cursor }}")

	return operationName, b.String()
}

// RenderFilter serialises a single FilterNode (or nil → "null") into the
// GraphQL `where:` argument form. Used by generated mutation methods that
// don't go through PlanToGraphQL.
func RenderFilter(node FilterNode) string {
	if node == nil {
		return "null"
	}
	var b strings.Builder
	writeFilter(&b, node)
	return b.String()
}

// RenderFields serialises a []FieldNode into a space-separated GraphQL
// selection set (without the surrounding braces). Used by generated Search
// methods that build their own query string.
func RenderFields(fields []FieldNode) string {
	var b strings.Builder
	writeFields(&b, fields)
	return b.String()
}

// RenderOrderBy serialises a []OrderByNode into the GraphQL `order_by`
// argument value, e.g. `[{ CustomerId: Ascending }]`. Returns "" for empty
// input so callers can skip the arg entirely.
func RenderOrderBy(nodes []OrderByNode) string {
	if len(nodes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[")
	for i, ob := range nodes {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "{ %s: %s }", ob.Field, ob.Direction)
	}
	b.WriteString("]")
	return b.String()
}

func writeFilter(b *strings.Builder, node FilterNode) {
	switch n := node.(type) {
	case CondNode:
		fmt.Fprintf(b, "{ %s: { %s: %s } }", n.Field, n.Op, RenderValue(n.Value))
	case AndNode:
		b.WriteString("{ and: [")
		for i, p := range n.Parts {
			if i > 0 {
				b.WriteString(", ")
			}
			writeFilter(b, p)
		}
		b.WriteString("] }")
	case OrNode:
		b.WriteString("{ or: [")
		for i, p := range n.Parts {
			if i > 0 {
				b.WriteString(", ")
			}
			writeFilter(b, p)
		}
		b.WriteString("] }")
	case NotNode:
		b.WriteString("{ not: ")
		writeFilter(b, n.Inner)
		b.WriteString(" }")
	default:
		panic(fmt.Sprintf("unknown FilterNode: %T", node))
	}
}

// RenderValue converts a Go value to its GraphQL literal form. Exported so
// other emitters can reuse the same quoting rules.
func RenderValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		buf, _ := json.Marshal(x)
		return string(buf)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%g", x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = RenderValue(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		buf, err := json.Marshal(x)
		if err != nil {
			panic(fmt.Sprintf("RenderValue: cannot marshal %T: %v", x, err))
		}
		return string(buf)
	}
}

func writeFields(b *strings.Builder, fields []FieldNode) {
	for i, f := range fields {
		if i > 0 {
			b.WriteString(" ")
		}
		switch n := f.(type) {
		case ScalarField:
			b.WriteString(n.Name)
		case NestedField:
			fmt.Fprintf(b, "%s { ", n.Name)
			writeFields(b, n.Children)
			b.WriteString(" }")
		default:
			panic(fmt.Sprintf("unknown FieldNode: %T", f))
		}
	}
}
