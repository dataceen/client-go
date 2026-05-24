package emit

import (
	"fmt"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// EmitAggregation produces `Search<Entity>Aggregation.go` — the typed input
// shape callers pass to <Entity>Client.Search via the Aggregations slice.
//
// Two structs are emitted in one file:
//   - Search<Entity>Aggregation: a mutually-exclusive choice of which field
//     to aggregate (one *Search<Entity>AggregationDetails per scalar field).
//   - Search<Entity>AggregationDetails: name/size/type plus a recursive
//     `Aggregations` slice of nested Search<Entity>Aggregation entries.
//
// Returns ("", nil) when the entity has no Aggregation type. That happens
// for sub-entities and for any entity the schema doesn't expose Search on.
func EmitAggregation(model schema.ModelSchema, entity schema.EntityInfo, packageName string, emitting map[string]bool) (string, error) {
	if entity.Aggregation == nil {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(FileHeader("Search"+entity.Name+"Aggregation", packageName))

	fmt.Fprintf(&b, "// Search%sAggregation specifies one aggregation request for %sClient.Search.\n",
		entity.Name, entity.Name)
	b.WriteString("// Set exactly one field to choose which column to aggregate.\n")
	fmt.Fprintf(&b, "type Search%sAggregation struct {\n", entity.Name)

	for _, f := range entity.Aggregation.Fields {
		writeAggregationField(&b, entity.Name, f, emitting)
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "// Search%sAggregationDetails describes one aggregation: a name, an\n", entity.Name)
	b.WriteString("// optional bucket size, the aggregation type (\"Bucket_Terms\",\n")
	b.WriteString("// \"Metrics_Sum\", ...), and an optional recursive Aggregations slice for\n")
	b.WriteString("// nested aggregations.\n")
	fmt.Fprintf(&b, "type Search%sAggregationDetails struct {\n", entity.Name)
	b.WriteString("\tName         string `json:\"name\"`\n")
	b.WriteString("\tSize         int    `json:\"size,omitempty\"`\n")
	b.WriteString("\t// Type is the aggregation kind. Common values:\n")
	b.WriteString("\t//   \"Bucket_Terms\"   — group rows by distinct values\n")
	b.WriteString("\t//   \"Bucket_Filter\"  — group by predicate match\n")
	b.WriteString("\t//   \"Metrics_Sum\"    — sum of a numeric column\n")
	b.WriteString("\t//   \"Metrics_Min\" / \"Metrics_Max\" / \"Metrics_Average\"\n")
	b.WriteString("\tType         string `json:\"type,omitempty\"`\n")
	fmt.Fprintf(&b, "\tAggregations []Search%sAggregation `json:\"aggregations,omitempty\"`\n", entity.Name)
	b.WriteString("}\n")

	return FormatGo(b.String())
}

func writeAggregationField(b *strings.Builder, entityName string, f schema.FieldRef, emitting map[string]bool) {
	goName := GoFieldName(f.Name)
	tag := fmt.Sprintf("`json:\"%s,omitempty\"`", f.Name)

	// Same-entity aggregation details OR a sibling Search<NestedType>_aggregation.
	if f.BaseType == "Search"+entityName+"AggregationDetails" {
		fmt.Fprintf(b, "\t%s *Search%sAggregationDetails %s\n", goName, entityName, tag)
		return
	}
	if strings.HasPrefix(f.BaseType, "Search") && strings.HasSuffix(f.BaseType, "_aggregation") {
		// Nested aggregation. Strip the Search prefix and _aggregation suffix
		// to get the entity name; if that entity is in the emit set, point
		// at its Aggregation type.
		nested := strings.TrimSuffix(strings.TrimPrefix(f.BaseType, "Search"), "_aggregation")
		if emitting[nested] {
			fmt.Fprintf(b, "\t%s *Search%sAggregation %s\n", goName, nested, tag)
			return
		}
	}
	// Fallback for unknown shape — `any` keeps the file compilable; the
	// caller can build the payload by hand and pass it via dsl.EncodeInput.
	fmt.Fprintf(b, "\t%s any %s\n", goName, tag)
}
