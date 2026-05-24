package schema_test

import (
	"os"
	"sort"
	"testing"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// loadCachedSchema looks for the cached schema relative to the package dir.
// Tests run from the package, so the cache (which is at repo root) is at
// "../../cache/schema.json".
func loadCachedSchema(t *testing.T) *schema.CachedSchema {
	t.Helper()
	for _, p := range []string{
		"../../cache/schema.json",
		"../../../tools/codegen/cache/schema.json",
	} {
		if _, err := os.Stat(p); err == nil {
			c, err := schema.ReadCache(p)
			if err != nil {
				t.Fatalf("ReadCache(%s): %v", p, err)
			}
			return c
		}
	}
	t.Skip("no cached schema found — run `go run ./tools/codegen/cmd schema` first")
	return nil
}

func TestParse_TopLevelEntities(t *testing.T) {
	cached := loadCachedSchema(t)
	model := schema.Parse(cached)

	if model.Domain != "Thomas" || model.Model != "CandyShopModel" || model.Scope != "All" {
		t.Errorf("metadata = %+v, want Thomas/CandyShopModel/All", model)
	}

	// Sanity: a known top-level entity exists and is classified as a node.
	cust := model.FindEntity("Customer")
	if cust == nil {
		t.Fatal("Customer entity not found")
	}
	if cust.Kind != schema.EntityNode {
		t.Errorf("Customer.Kind = %s, want node", cust.Kind)
	}
	if !cust.IsTopLevel {
		t.Error("Customer.IsTopLevel = false, want true")
	}
	if cust.Create == nil {
		t.Error("Customer.Create is nil")
	}
	if cust.Update == nil {
		t.Error("Customer.Update is nil")
	}
	if cust.SearchFilter == nil {
		t.Error("Customer.SearchFilter is nil")
	}
}

func TestParse_RelationshipDetection(t *testing.T) {
	cached := loadCachedSchema(t)
	model := schema.Parse(cached)

	rel := model.FindEntity("CustomerPlacedOrder")
	if rel == nil {
		t.Fatal("CustomerPlacedOrder not found")
	}
	if rel.Kind != schema.EntityRelationship {
		t.Fatalf("CustomerPlacedOrder.Kind = %s, want relationship", rel.Kind)
	}
	if rel.Create != nil {
		t.Error("relationship.Create should be nil — created via compound mutation")
	}
	if rel.Relationship == nil {
		t.Fatal("Relationship info missing")
	}
	if rel.Relationship.From != "Customer" || rel.Relationship.To != "Order" {
		t.Errorf("relationship from/to = %s/%s, want Customer/Order",
			rel.Relationship.From, rel.Relationship.To)
	}
	if rel.Relationship.CompoundCreateTypeName != "CustomerCustomerPlacedOrderOrderCreate" {
		t.Errorf("compoundCreateTypeName = %s", rel.Relationship.CompoundCreateTypeName)
	}
	if rel.Relationship.CreateMutation != "CreateCustomerCustomerPlacedOrderOrder" {
		t.Errorf("createMutation = %s", rel.Relationship.CreateMutation)
	}
}

func TestParse_SubEntityDiscovery(t *testing.T) {
	cached := loadCachedSchema(t)
	model := schema.Parse(cached)

	for _, sub := range []string{"CustomerName", "CustomerContact", "CustomerAddress"} {
		e := model.FindEntity(sub)
		if e == nil {
			t.Errorf("sub-entity %s not discovered", sub)
			continue
		}
		if e.IsTopLevel {
			t.Errorf("%s.IsTopLevel = true, want false (sub-entity)", sub)
		}
	}
}

func TestParse_ScalarMapping(t *testing.T) {
	cached := loadCachedSchema(t)
	model := schema.Parse(cached)

	cases := map[string]string{
		"String":         "string",
		"Boolean":        "bool",
		"Int":            "int",
		"DateTimeOffset": "string", // wire-fidelity, decisions.md #16/25
		"Long":           "string", // precision-safe, decisions.md #29
		"Decimal":        "string",
	}
	for s, want := range cases {
		if got, ok := model.Scalars[s]; ok && got != want {
			t.Errorf("Scalars[%q] = %q, want %q", s, got, want)
		}
	}
}

func TestParse_EnumValues(t *testing.T) {
	cached := loadCachedSchema(t)
	model := schema.Parse(cached)

	// order_by enum values must be Ascending/Descending (decisions.md #30).
	for _, e := range model.Enums {
		if e.Name == "order_by" {
			vals := append([]string{}, e.Values...)
			sort.Strings(vals)
			if len(vals) != 2 || vals[0] != "Ascending" || vals[1] != "Descending" {
				t.Errorf("order_by values = %v, want [Ascending, Descending]", vals)
			}
			return
		}
	}
	t.Error("order_by enum not found")
}

func TestParse_FilterFieldsExist(t *testing.T) {
	cached := loadCachedSchema(t)
	model := schema.Parse(cached)

	cust := model.FindEntity("Customer")
	if cust == nil {
		t.Fatal("Customer not found")
	}
	have := map[string]bool{}
	for _, f := range cust.Filter.Fields {
		have[f.Name] = true
	}
	for _, name := range []string{"CustomerId", "ExternalReference", "IsActive", "and", "or", "not"} {
		if !have[name] {
			t.Errorf("CustomerFilter missing %q", name)
		}
	}
}

func TestParse_StatsMatchSpike(t *testing.T) {
	cached := loadCachedSchema(t)
	model := schema.Parse(cached)

	// Spike 04 + TS port both reported 498 types, including 9 nodes + 18
	// relationships + ~12 sub-entities. We don't assert exact counts (model
	// can grow); we sanity-check that we found a reasonable population.
	var nodes, rels int
	for _, e := range model.Entities {
		switch e.Kind {
		case schema.EntityNode:
			nodes++
		case schema.EntityRelationship:
			rels++
		}
	}
	if nodes < 5 {
		t.Errorf("only %d node entities — schema parser may have lost types", nodes)
	}
	if rels < 5 {
		t.Errorf("only %d relationship entities — relationship detection may be wrong", rels)
	}
	t.Logf("model: %d entities (%d nodes + %d relationships), %d enums, %d scalars",
		len(model.Entities), nodes, rels, len(model.Enums), len(model.Scalars))
}
