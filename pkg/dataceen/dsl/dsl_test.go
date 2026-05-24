package dsl_test

import (
	"testing"

	"github.com/dataceen/client-go/pkg/dataceen/dsl"
)

// TestCanonicalQuery locks the canonical wire format. If serializer output
// changes, internal/prototypes/prototypes_test.go also fails — both must
// move together.
func TestCanonicalQuery(t *testing.T) {
	plan := dsl.QueryPlan{
		Entity: "Customer",
		Size:   10,
		Cursor: "abc123",
		Filter: dsl.FilterToNode(dsl.And(
			dsl.F("CustomerId").Like("cst"),
			dsl.Or(
				dsl.F("Status").Eq("Active"),
				dsl.F("Status").Eq("Premium"),
			),
		)),
		Fields: dsl.Fields().
			Pick("_id", "CustomerId", "ExternalReference").
			Nested("CustomerPlacedOrder", func(r *dsl.FieldsBuilder) {
				r.Nested("Order", func(o *dsl.FieldsBuilder) {
					o.Pick("OrderId")
				})
			}).
			Build(),
		OrderBy: dsl.OrderBy().Asc("CustomerId").Desc("ExternalReference").Build(),
	}

	op, q := dsl.PlanToGraphQL(plan)
	wantOp := "FindCustomer"
	wantQ := `query {FindCustomer(size: 10, cursor: "abc123", where: { and: [{ CustomerId: { like: "cst" } }, { or: [{ Status: { eq: "Active" } }, { Status: { eq: "Premium" } }] }] }, order_by: [{ CustomerId: Ascending }, { ExternalReference: Descending }]) { Items { _id CustomerId ExternalReference CustomerPlacedOrder { Order { OrderId } } } Cursor }}`
	if op != wantOp {
		t.Errorf("operationName = %q, want %q", op, wantOp)
	}
	if q != wantQ {
		t.Errorf("query mismatch\n got: %s\nwant: %s", q, wantQ)
	}
}

func TestEmptyFilterRendersNull(t *testing.T) {
	plan := dsl.QueryPlan{
		Entity: "Customer",
		Size:   5,
		Cursor: "null",
		Fields: []dsl.FieldNode{dsl.ScalarField{Name: "_id"}},
	}
	_, q := dsl.PlanToGraphQL(plan)
	want := `query {FindCustomer(size: 5, cursor: "null", where: null) { Items { _id } Cursor }}`
	if q != want {
		t.Errorf("got: %s\nwant: %s", q, want)
	}
}

func TestNotComposition(t *testing.T) {
	plan := dsl.QueryPlan{
		Entity: "Customer",
		Size:   1,
		Cursor: "null",
		Filter: dsl.FilterToNode(dsl.Not(dsl.F("IsActive").Eq(true))),
		Fields: []dsl.FieldNode{dsl.ScalarField{Name: "_id"}},
	}
	_, q := dsl.PlanToGraphQL(plan)
	want := `query {FindCustomer(size: 1, cursor: "null", where: { not: { IsActive: { eq: true } } }) { Items { _id } Cursor }}`
	if q != want {
		t.Errorf("got: %s\nwant: %s", q, want)
	}
}
