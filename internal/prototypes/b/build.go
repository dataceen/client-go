// Package b is prototype B: nothing typed at all — filters, fields, and
// orderBy are plain map[string]any / []string structures. Build walks them
// into the shared QueryPlan AST.
//
// Trades type-safety for code volume: ~70 lines of converter, no codegen
// per entity. Tradeoff is that typos in field names compile cleanly and
// only fail server-side.
package b

import (
	"fmt"

	pt "github.com/dataceen/client-go/internal/prototypes"
)

// M is just an alias for map[string]any so callers don't have to retype it.
type M = map[string]any

// FindCustomerInput is the user-facing entry. Filter/Fields/OrderBy use the
// loose forms documented below. Entity is needed because there are no per-
// entity types to infer it from.
type FindCustomerInput struct {
	Entity string
	Size   int
	Cursor string
	// Filter shapes:
	//   M{"<Field>": M{"<op>": value}}              — a single comparison
	//   M{"and": []any{...filters...}}              — AND group
	//   M{"or":  []any{...filters...}}              — OR group
	//   M{"not": <filter>}                          — NOT
	// nil means "where: null".
	Filter M
	// Fields uses dot-paths to express nesting:
	//   "_id", "CustomerId", "CustomerPlacedOrder.Order.OrderId"
	Fields []string
	// OrderBy is []M{"<Field>": "Ascending" | "Descending"}.
	OrderBy []M
}

// Build returns the shared AST plan.
func Build(in FindCustomerInput) pt.QueryPlan {
	return pt.QueryPlan{
		Entity:  in.Entity,
		Size:    in.Size,
		Cursor:  in.Cursor,
		Filter:  convertFilter(in.Filter),
		Fields:  convertFields(in.Fields),
		OrderBy: convertOrderBy(in.OrderBy),
	}
}

func convertFilter(f M) pt.FilterNode {
	if f == nil {
		return nil
	}
	for k, v := range f {
		switch k {
		case "and":
			parts := convertCompositeList(v, "and")
			return pt.AndNode{Parts: parts}
		case "or":
			parts := convertCompositeList(v, "or")
			return pt.OrNode{Parts: parts}
		case "not":
			inner, ok := v.(M)
			if !ok {
				panic(fmt.Sprintf("not: expected map[string]any, got %T", v))
			}
			return pt.NotNode{Inner: convertFilter(inner)}
		default:
			ops, ok := v.(M)
			if !ok {
				panic(fmt.Sprintf("filter %q: expected map of ops, got %T", k, v))
			}
			// A single field can have multiple operators in the same map; the
			// converter emits an AND of all of them, mirroring server semantics.
			if len(ops) == 1 {
				for op, val := range ops {
					return pt.CondNode{Field: k, Op: op, Value: val}
				}
			}
			conds := make([]pt.FilterNode, 0, len(ops))
			for op, val := range ops {
				conds = append(conds, pt.CondNode{Field: k, Op: op, Value: val})
			}
			return pt.AndNode{Parts: conds}
		}
	}
	return nil
}

func convertCompositeList(v any, key string) []pt.FilterNode {
	list, ok := v.([]any)
	if !ok {
		panic(fmt.Sprintf("%s: expected []any, got %T", key, v))
	}
	parts := make([]pt.FilterNode, 0, len(list))
	for _, item := range list {
		mi, ok := item.(M)
		if !ok {
			panic(fmt.Sprintf("%s element: expected map[string]any, got %T", key, item))
		}
		parts = append(parts, convertFilter(mi))
	}
	return parts
}

// convertFields parses dot-paths and folds shared prefixes into nested
// FieldNodes. Order is preserved per first-occurrence so output is
// deterministic.
func convertFields(paths []string) []pt.FieldNode {
	type group struct {
		name     string
		scalar   bool
		children []string
	}
	order := []string{}
	groups := map[string]*group{}
	for _, p := range paths {
		head, tail := splitPath(p)
		g, ok := groups[head]
		if !ok {
			g = &group{name: head}
			groups[head] = g
			order = append(order, head)
		}
		if tail == "" {
			g.scalar = true
		} else {
			g.children = append(g.children, tail)
		}
	}
	out := make([]pt.FieldNode, 0, len(order))
	for _, name := range order {
		g := groups[name]
		if len(g.children) == 0 {
			out = append(out, pt.ScalarField{Name: name})
			continue
		}
		out = append(out, pt.NestedField{Name: name, Children: convertFields(g.children)})
	}
	return out
}

func splitPath(p string) (head, tail string) {
	for i := 0; i < len(p); i++ {
		if p[i] == '.' {
			return p[:i], p[i+1:]
		}
	}
	return p, ""
}

func convertOrderBy(in []M) []pt.OrderByNode {
	var out []pt.OrderByNode
	for _, m := range in {
		for k, v := range m {
			s, ok := v.(string)
			if !ok {
				panic(fmt.Sprintf("orderBy[%s]: expected string direction, got %T", k, v))
			}
			out = append(out, pt.OrderByNode{Field: k, Direction: s})
		}
	}
	return out
}
