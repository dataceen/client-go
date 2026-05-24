package prototypes

// TargetPlan is the canonical use-case every prototype (A, B, C) must produce:
//
//	Find 10 Customers whose CustomerId LIKE "cst" AND
//	  (Status = "Active" OR Status = "Premium"),
//	cursor "abc123",
//	fields _id, CustomerId, ExternalReference + nested
//	  CustomerPlacedOrder → Order → OrderId,
//	orderBy CustomerId Ascending, ExternalReference Descending.
//
// Operator/direction casing matches the live Dataceen server (decisions.md
// #26–30): lowercase operators, PascalCase enum values.
var TargetPlan = QueryPlan{
	Entity: "Customer",
	Size:   10,
	Cursor: "abc123",
	Filter: AndNode{Parts: []FilterNode{
		CondNode{Field: "CustomerId", Op: "like", Value: "cst"},
		OrNode{Parts: []FilterNode{
			CondNode{Field: "Status", Op: "eq", Value: "Active"},
			CondNode{Field: "Status", Op: "eq", Value: "Premium"},
		}},
	}},
	Fields: []FieldNode{
		ScalarField{Name: "_id"},
		ScalarField{Name: "CustomerId"},
		ScalarField{Name: "ExternalReference"},
		NestedField{Name: "CustomerPlacedOrder", Children: []FieldNode{
			NestedField{Name: "Order", Children: []FieldNode{
				ScalarField{Name: "OrderId"},
			}},
		}},
	},
	OrderBy: []OrderByNode{
		{Field: "CustomerId", Direction: "Ascending"},
		{Field: "ExternalReference", Direction: "Descending"},
	},
}

// ExpectedQuery returns the GraphQL string the canonical plan serialises to.
// All three prototypes are expected to produce a QueryPlan whose serialised
// form matches this exactly.
func ExpectedQuery() (operationName, query string) {
	return PlanToGraphQL(TargetPlan)
}
