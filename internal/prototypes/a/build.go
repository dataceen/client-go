// Package a is prototype A: typed structs with `omitempty` + pointer fields.
// Filter, OrderBy and Fields are all plain Go structs; the `Build` function
// converts them to the shared QueryPlan AST.
//
// Idiomatic Go: zero-value friendly, json.Marshal-compatible, no reflection.
// Verbose at depth — every comparison value needs a pointer (because
// Go has no null-vs-zero distinction for non-pointer scalars).
package a

import (
	pt "github.com/dataceen/client-go/internal/prototypes"
)

// CustomerFilter is the typed filter root for Customer. Only one of And/Or/Not
// or the per-field comparison expressions is meaningful at a time, but the
// converter walks all populated fields.
type CustomerFilter struct {
	CustomerID        *StringComparisonExp
	ExternalReference *StringComparisonExp
	Status            *StringComparisonExp
	IsActive          *BoolComparisonExp

	And []CustomerFilter
	Or  []CustomerFilter
	Not *CustomerFilter
}

// StringComparisonExp wraps the operator → value mapping for string fields.
// Each operator is a pointer so the zero value means "not set".
type StringComparisonExp struct {
	Eq        *string
	Neq       *string
	Like      *string
	NLike     *string
	Contains  *string
	NContains *string
	In        []string
	NIn       []string
	IsNull    *bool
	Exists    *bool
}

// BoolComparisonExp covers the smaller boolean operator set.
type BoolComparisonExp struct {
	Eq     *bool
	Neq    *bool
	IsNull *bool
	Exists *bool
}

// Strp is a tiny helper since Go has no inline string-pointer literal.
func Strp(s string) *string { return &s }

// CustomerFields is the per-entity field-selection struct. The codegen would
// produce one per entity. Booleans toggle scalars; pointers to nested
// structs include the relation/sub-entity in the selection.
type CustomerFields struct {
	ID                  bool
	CustomerID          bool
	ExternalReference   bool
	Status              bool
	IsActive            bool
	CustomerPlacedOrder *CustomerPlacedOrderFields
}

// CustomerPlacedOrderFields is the relation's sub-selection.
type CustomerPlacedOrderFields struct {
	PlacedAt bool
	Order    *OrderFields
}

// OrderFields is the target node's sub-selection.
type OrderFields struct {
	ID      bool
	OrderID bool
	Total   bool
}

// Direction is the orderBy direction. Literal values match the server enum.
type Direction string

const (
	Ascending  Direction = "Ascending"
	Descending Direction = "Descending"
)

// CustomerOrderBy picks at most one direction per column. Multiple struct
// instances in a slice express a multi-column sort.
type CustomerOrderBy struct {
	CustomerID        Direction
	ExternalReference Direction
	Status            Direction
}

// FindCustomerInput is the user-facing entry point. Mirrors the structure
// of the future generated FindCustomerInput.
type FindCustomerInput struct {
	Size    int
	Cursor  string
	Filter  *CustomerFilter
	Fields  CustomerFields
	OrderBy []CustomerOrderBy
}

// Build returns the shared AST plan. Phase 3's generator will replace this
// with one that calls dataceen.Client.ExecuteQuery directly.
func Build(in FindCustomerInput) pt.QueryPlan {
	return pt.QueryPlan{
		Entity:  "Customer",
		Size:    in.Size,
		Cursor:  in.Cursor,
		Filter:  convertFilter(in.Filter),
		Fields:  convertFields(in.Fields),
		OrderBy: convertOrderBy(in.OrderBy),
	}
}

func convertFilter(f *CustomerFilter) pt.FilterNode {
	if f == nil {
		return nil
	}
	var nodes []pt.FilterNode
	if f.CustomerID != nil {
		nodes = appendStringConds(nodes, "CustomerId", f.CustomerID)
	}
	if f.ExternalReference != nil {
		nodes = appendStringConds(nodes, "ExternalReference", f.ExternalReference)
	}
	if f.Status != nil {
		nodes = appendStringConds(nodes, "Status", f.Status)
	}
	if f.IsActive != nil {
		nodes = appendBoolConds(nodes, "IsActive", f.IsActive)
	}
	if len(f.And) > 0 {
		parts := make([]pt.FilterNode, 0, len(f.And))
		for i := range f.And {
			if n := convertFilter(&f.And[i]); n != nil {
				parts = append(parts, n)
			}
		}
		if len(parts) > 0 {
			nodes = append(nodes, pt.AndNode{Parts: parts})
		}
	}
	if len(f.Or) > 0 {
		parts := make([]pt.FilterNode, 0, len(f.Or))
		for i := range f.Or {
			if n := convertFilter(&f.Or[i]); n != nil {
				parts = append(parts, n)
			}
		}
		if len(parts) > 0 {
			nodes = append(nodes, pt.OrNode{Parts: parts})
		}
	}
	if f.Not != nil {
		if n := convertFilter(f.Not); n != nil {
			nodes = append(nodes, pt.NotNode{Inner: n})
		}
	}
	switch len(nodes) {
	case 0:
		return nil
	case 1:
		return nodes[0]
	default:
		return pt.AndNode{Parts: nodes}
	}
}

func appendStringConds(out []pt.FilterNode, field string, e *StringComparisonExp) []pt.FilterNode {
	if e.Eq != nil {
		out = append(out, pt.CondNode{Field: field, Op: "eq", Value: *e.Eq})
	}
	if e.Neq != nil {
		out = append(out, pt.CondNode{Field: field, Op: "neq", Value: *e.Neq})
	}
	if e.Like != nil {
		out = append(out, pt.CondNode{Field: field, Op: "like", Value: *e.Like})
	}
	if e.NLike != nil {
		out = append(out, pt.CondNode{Field: field, Op: "nlike", Value: *e.NLike})
	}
	if e.Contains != nil {
		out = append(out, pt.CondNode{Field: field, Op: "contains", Value: *e.Contains})
	}
	if e.NContains != nil {
		out = append(out, pt.CondNode{Field: field, Op: "ncontains", Value: *e.NContains})
	}
	if len(e.In) > 0 {
		vals := make([]any, len(e.In))
		for i, v := range e.In {
			vals[i] = v
		}
		out = append(out, pt.CondNode{Field: field, Op: "in", Value: vals})
	}
	if len(e.NIn) > 0 {
		vals := make([]any, len(e.NIn))
		for i, v := range e.NIn {
			vals[i] = v
		}
		out = append(out, pt.CondNode{Field: field, Op: "nin", Value: vals})
	}
	if e.IsNull != nil {
		out = append(out, pt.CondNode{Field: field, Op: "is_null", Value: *e.IsNull})
	}
	if e.Exists != nil {
		out = append(out, pt.CondNode{Field: field, Op: "exists", Value: *e.Exists})
	}
	return out
}

func appendBoolConds(out []pt.FilterNode, field string, e *BoolComparisonExp) []pt.FilterNode {
	if e.Eq != nil {
		out = append(out, pt.CondNode{Field: field, Op: "eq", Value: *e.Eq})
	}
	if e.Neq != nil {
		out = append(out, pt.CondNode{Field: field, Op: "neq", Value: *e.Neq})
	}
	if e.IsNull != nil {
		out = append(out, pt.CondNode{Field: field, Op: "is_null", Value: *e.IsNull})
	}
	if e.Exists != nil {
		out = append(out, pt.CondNode{Field: field, Op: "exists", Value: *e.Exists})
	}
	return out
}

func convertFields(f CustomerFields) []pt.FieldNode {
	var out []pt.FieldNode
	if f.ID {
		out = append(out, pt.ScalarField{Name: "_id"})
	}
	if f.CustomerID {
		out = append(out, pt.ScalarField{Name: "CustomerId"})
	}
	if f.ExternalReference {
		out = append(out, pt.ScalarField{Name: "ExternalReference"})
	}
	if f.Status {
		out = append(out, pt.ScalarField{Name: "Status"})
	}
	if f.IsActive {
		out = append(out, pt.ScalarField{Name: "IsActive"})
	}
	if f.CustomerPlacedOrder != nil {
		out = append(out, pt.NestedField{
			Name:     "CustomerPlacedOrder",
			Children: convertCustomerPlacedOrderFields(*f.CustomerPlacedOrder),
		})
	}
	return out
}

func convertCustomerPlacedOrderFields(f CustomerPlacedOrderFields) []pt.FieldNode {
	var out []pt.FieldNode
	if f.PlacedAt {
		out = append(out, pt.ScalarField{Name: "PlacedAt"})
	}
	if f.Order != nil {
		out = append(out, pt.NestedField{
			Name:     "Order",
			Children: convertOrderFields(*f.Order),
		})
	}
	return out
}

func convertOrderFields(f OrderFields) []pt.FieldNode {
	var out []pt.FieldNode
	if f.ID {
		out = append(out, pt.ScalarField{Name: "_id"})
	}
	if f.OrderID {
		out = append(out, pt.ScalarField{Name: "OrderId"})
	}
	if f.Total {
		out = append(out, pt.ScalarField{Name: "Total"})
	}
	return out
}

func convertOrderBy(in []CustomerOrderBy) []pt.OrderByNode {
	var out []pt.OrderByNode
	for _, ob := range in {
		if ob.CustomerID != "" {
			out = append(out, pt.OrderByNode{Field: "CustomerId", Direction: string(ob.CustomerID)})
		}
		if ob.ExternalReference != "" {
			out = append(out, pt.OrderByNode{Field: "ExternalReference", Direction: string(ob.ExternalReference)})
		}
		if ob.Status != "" {
			out = append(out, pt.OrderByNode{Field: "Status", Direction: string(ob.Status)})
		}
	}
	return out
}
