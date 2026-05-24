package prototypes

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PlanToGraphQL turns a QueryPlan into a GraphQL request suitable for
// dataceen.Client.ExecuteQuery. The real generator (Phase 3) will replace
// this with one that emits typed wrapper methods, but the wire output is
// the same.
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

func writeFilter(b *strings.Builder, node FilterNode) {
	switch n := node.(type) {
	case CondNode:
		fmt.Fprintf(b, "{ %s: { %s: %s } }", n.Field, n.Op, renderValue(n.Value))
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

func renderValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		// Use JSON encoding for proper quoting/escaping.
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
			parts[i] = renderValue(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		// JSON-encode anything else (structs, maps).
		buf, err := json.Marshal(x)
		if err != nil {
			panic(fmt.Sprintf("renderValue: cannot marshal %T: %v", x, err))
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
