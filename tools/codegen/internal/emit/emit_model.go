package emit

import (
	"fmt"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// EmitModel produces `<Entity>.go` — the read-model struct. Field names use
// GoFieldName conversion; JSON tags preserve the wire format exactly.
func EmitModel(model schema.ModelSchema, entity schema.EntityInfo, packageName string, emitting map[string]bool) (string, error) {
	var b strings.Builder
	b.WriteString(FileHeader(entity.Name, packageName))

	fmt.Fprintf(&b, "// %s is the read-model for the %q entity.\n", entity.Name, entity.Name)
	if entity.Kind == schema.EntityRelationship && entity.Relationship != nil {
		fmt.Fprintf(&b, "// Relationship: %s -[%s]-> %s.\n",
			entity.Relationship.From, entity.Name, entity.Relationship.To)
	}
	fmt.Fprintf(&b, "type %s struct {\n", entity.Name)

	for _, f := range entity.Read.Fields {
		writeField(&b, f, model.Scalars, emitting)
	}
	b.WriteString("}\n")

	return FormatGo(b.String())
}

// writeField emits one struct field. Examples:
//
//	MetaId            string `json:"_id,omitempty"`
//	CustomerId        string `json:"CustomerId"`
//	Name              *CustomerName `json:"Name,omitempty"`
//	CustomerPlacedOrder []CustomerPlacedOrder `json:"CustomerPlacedOrder,omitempty"`
func writeField(b *strings.Builder, f schema.FieldRef, scalars map[string]string, emitting map[string]bool) {
	goName := GoFieldName(f.Name)
	goType := GoType(f, scalars, emitting)
	tag := JSONTag(f.Name, f.NonNull)
	fmt.Fprintf(b, "\t%s %s %s\n", goName, goType, tag)
}
