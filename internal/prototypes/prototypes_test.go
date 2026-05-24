package prototypes_test

import (
	"reflect"
	"testing"

	pt "github.com/dataceen/client-go/internal/prototypes"
	"github.com/dataceen/client-go/internal/prototypes/a"
	"github.com/dataceen/client-go/internal/prototypes/b"
	"github.com/dataceen/client-go/internal/prototypes/c"
)

// expectedQueryString locks the canonical wire format. If this changes,
// every prototype must change with it (or the test fails).
const expectedQueryString = `query {FindCustomer(size: 10, cursor: "abc123", where: { and: [{ CustomerId: { like: "cst" } }, { or: [{ Status: { eq: "Active" } }, { Status: { eq: "Premium" } }] }] }, order_by: [{ CustomerId: Ascending }, { ExternalReference: Descending }]) { Items { _id CustomerId ExternalReference CustomerPlacedOrder { Order { OrderId } } } Cursor }}`

func TestSerializerProducesExpectedQuery(t *testing.T) {
	op, q := pt.ExpectedQuery()
	if op != "FindCustomer" {
		t.Fatalf("operationName = %q, want FindCustomer", op)
	}
	if q != expectedQueryString {
		t.Fatalf("query mismatch.\n got: %s\nwant: %s", q, expectedQueryString)
	}
}

// canonical user-facing inputs — three different ergonomic shapes that
// should all produce the same plan.

func aInput() a.FindCustomerInput {
	return a.FindCustomerInput{
		Size:   10,
		Cursor: "abc123",
		Filter: &a.CustomerFilter{
			And: []a.CustomerFilter{
				{CustomerID: &a.StringComparisonExp{Like: a.Strp("cst")}},
				{Or: []a.CustomerFilter{
					{Status: &a.StringComparisonExp{Eq: a.Strp("Active")}},
					{Status: &a.StringComparisonExp{Eq: a.Strp("Premium")}},
				}},
			},
		},
		Fields: a.CustomerFields{
			ID:                true,
			CustomerID:        true,
			ExternalReference: true,
			CustomerPlacedOrder: &a.CustomerPlacedOrderFields{
				Order: &a.OrderFields{OrderID: true},
			},
		},
		OrderBy: []a.CustomerOrderBy{
			{CustomerID: a.Ascending},
			{ExternalReference: a.Descending},
		},
	}
}

func bInput() b.FindCustomerInput {
	return b.FindCustomerInput{
		Entity: "Customer",
		Size:   10,
		Cursor: "abc123",
		Filter: b.M{
			"and": []any{
				b.M{"CustomerId": b.M{"like": "cst"}},
				b.M{"or": []any{
					b.M{"Status": b.M{"eq": "Active"}},
					b.M{"Status": b.M{"eq": "Premium"}},
				}},
			},
		},
		Fields: []string{
			"_id", "CustomerId", "ExternalReference",
			"CustomerPlacedOrder.Order.OrderId",
		},
		OrderBy: []b.M{
			{"CustomerId": "Ascending"},
			{"ExternalReference": "Descending"},
		},
	}
}

func cInput() c.FindInput {
	return c.FindInput{
		Entity: "Customer",
		Size:   10,
		Cursor: "abc123",
		Filter: c.And(
			c.F("CustomerId").Like("cst"),
			c.Or(
				c.F("Status").Eq("Active"),
				c.F("Status").Eq("Premium"),
			),
		),
		Fields: c.Fields().
			Pick("_id", "CustomerId", "ExternalReference").
			Nested("CustomerPlacedOrder", func(r *c.FieldsBuilder) {
				r.Nested("Order", func(o *c.FieldsBuilder) {
					o.Pick("OrderId")
				})
			}),
		OrderBy: c.OrderBy().Asc("CustomerId").Desc("ExternalReference"),
	}
}

func TestPrototypeAMatchesTarget(t *testing.T) {
	plan := a.Build(aInput())
	if !reflect.DeepEqual(plan, pt.TargetPlan) {
		t.Fatalf("A plan mismatch\n got: %#v\nwant: %#v", plan, pt.TargetPlan)
	}
	_, q := pt.PlanToGraphQL(plan)
	if q != expectedQueryString {
		t.Fatalf("A query mismatch\n got: %s\nwant: %s", q, expectedQueryString)
	}
}

func TestPrototypeBMatchesTarget(t *testing.T) {
	plan := b.Build(bInput())
	if !reflect.DeepEqual(plan, pt.TargetPlan) {
		t.Fatalf("B plan mismatch\n got: %#v\nwant: %#v", plan, pt.TargetPlan)
	}
	_, q := pt.PlanToGraphQL(plan)
	if q != expectedQueryString {
		t.Fatalf("B query mismatch\n got: %s\nwant: %s", q, expectedQueryString)
	}
}

func TestPrototypeCMatchesTarget(t *testing.T) {
	plan := c.Build(cInput())
	if !reflect.DeepEqual(plan, pt.TargetPlan) {
		t.Fatalf("C plan mismatch\n got: %#v\nwant: %#v", plan, pt.TargetPlan)
	}
	_, q := pt.PlanToGraphQL(plan)
	if q != expectedQueryString {
		t.Fatalf("C query mismatch\n got: %s\nwant: %s", q, expectedQueryString)
	}
}

// The reflect.DeepEqual comparisons above already prove A==B==C semantically.
// This top-level test asserts wire-equality directly so a single failure
// pinpoints which serializer drifted.
func TestAllThreeProduceIdenticalQuery(t *testing.T) {
	_, qA := pt.PlanToGraphQL(a.Build(aInput()))
	_, qB := pt.PlanToGraphQL(b.Build(bInput()))
	_, qC := pt.PlanToGraphQL(c.Build(cInput()))
	if qA != qB || qB != qC {
		t.Fatalf("queries diverge:\nA: %s\nB: %s\nC: %s", qA, qB, qC)
	}
}
