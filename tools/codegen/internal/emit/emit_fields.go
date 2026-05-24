package emit

import (
	"fmt"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// EmitFields produces `<Entity>Fields.go` — strategy A from the DSL
// comparison. A struct with `bool` fields per scalar/enum and pointer-to-
// child-fields per nested OBJECT or LIST. ToNodes converts the populated
// fields into a []dsl.FieldNode for use in a QueryPlan.
//
// Example output for Customer:
//
//	type CustomerFields struct {
//	    MetaId              bool
//	    CustomerId          bool
//	    IsActive            bool
//	    Name                *CustomerNameFields
//	    CustomerPlacedOrder *CustomerPlacedOrderFields
//	}
//
//	func (f CustomerFields) ToNodes() []dsl.FieldNode { ... }
func EmitFields(model schema.ModelSchema, entity schema.EntityInfo, packageName string, emitting map[string]bool) (string, error) {
	var b strings.Builder
	b.WriteString(FileHeader(entity.Name+"Fields", packageName))
	b.WriteString(`import "github.com/dataceen/client-go/pkg/dataceen/dsl"` + "\n\n")

	fmt.Fprintf(&b, "// %sFields selects which fields of %s to fetch.\n", entity.Name, entity.Name)
	fmt.Fprintf(&b, "type %sFields struct {\n", entity.Name)

	for _, f := range entity.Read.Fields {
		writeFieldsStructField(&b, f, emitting)
	}
	b.WriteString("}\n\n")

	// ToNodes
	fmt.Fprintf(&b, "// ToNodes returns the dsl.FieldNode list for the populated fields.\n")
	fmt.Fprintf(&b, "func (f %sFields) ToNodes() []dsl.FieldNode {\n", entity.Name)
	b.WriteString("\tvar out []dsl.FieldNode\n")
	for _, f := range entity.Read.Fields {
		writeFieldsToNodesEntry(&b, f, emitting)
	}
	b.WriteString("\treturn out\n}\n")

	return FormatGo(b.String())
}

// writeFieldsStructField emits one field of <Entity>Fields. Scalars/enums
// are bool; nested-object/relationship references become *<TypeName>Fields
// when the referenced type is in the emit set, otherwise *struct{} (a
// placeholder a user can populate manually as more entities get emitted).
func writeFieldsStructField(b *strings.Builder, f schema.FieldRef, emitting map[string]bool) {
	goName := GoFieldName(f.Name)
	switch f.BaseKind {
	case "OBJECT", "INPUT_OBJECT":
		if emitting[f.BaseType] {
			fmt.Fprintf(b, "\t%s *%sFields\n", goName, f.BaseType)
		} else {
			// Reference outside the emit set — degrade to bool so user can
			// still ask for the relation's id without picking specific subfields.
			fmt.Fprintf(b, "\t%s bool // nested type %s not emitted; selecting all fields\n", goName, f.BaseType)
		}
	default:
		fmt.Fprintf(b, "\t%s bool\n", goName)
	}
}

// writeFieldsToNodesEntry emits one branch of the ToNodes() body.
func writeFieldsToNodesEntry(b *strings.Builder, f schema.FieldRef, emitting map[string]bool) {
	goName := GoFieldName(f.Name)
	switch f.BaseKind {
	case "OBJECT", "INPUT_OBJECT":
		if emitting[f.BaseType] {
			fmt.Fprintf(b, "\tif f.%s != nil {\n", goName)
			fmt.Fprintf(b, "\t\tout = append(out, dsl.NestedField{Name: %q, Children: f.%s.ToNodes()})\n", f.Name, goName)
			b.WriteString("\t}\n")
		} else {
			fmt.Fprintf(b, "\tif f.%s {\n", goName)
			fmt.Fprintf(b, "\t\tout = append(out, dsl.ScalarField{Name: %q})\n", f.Name)
			b.WriteString("\t}\n")
		}
	default:
		fmt.Fprintf(b, "\tif f.%s {\n", goName)
		fmt.Fprintf(b, "\t\tout = append(out, dsl.ScalarField{Name: %q})\n", f.Name)
		b.WriteString("\t}\n")
	}
}
