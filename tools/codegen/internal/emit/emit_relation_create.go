package emit

import (
	"fmt"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// EmitRelationCreate produces `<Rel>RelationCreate.go` — the compound-create
// input for a relationship.
//
// Returns ("", nil) for non-relationship entities.
//
// Fields:
//   - The introspection-advertised fields of the compound type
//     (e.g. _id, _documentVersion, _createdAt, _modifiedAt, PlacedAt).
//   - Plus the edge-metadata fields (_fromId, _toId, _fromLabel, _toLabel,
//     _label) which the server accepts but does NOT advertise (decisions.md
//     #32 / TS port progress.md fas 5 step 3). _fromId and _toId are
//     required; the rest are optional.
func EmitRelationCreate(model schema.ModelSchema, entity schema.EntityInfo, packageName string, emitting map[string]bool) (string, error) {
	if entity.Kind != schema.EntityRelationship || entity.Relationship == nil {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(FileHeader(entity.Name+"RelationCreate", packageName))

	fmt.Fprintf(&b, "// %sRelationCreate is the compound-create input for the\n", entity.Name)
	fmt.Fprintf(&b, "// %s relationship (%s -> %s). Sent to the server's\n",
		entity.Name, entity.Relationship.From, entity.Relationship.To)
	fmt.Fprintf(&b, "// %s mutation.\n", entity.Relationship.CreateMutation)
	b.WriteString("//\n")
	b.WriteString("// _fromId and _toId are required (the existing endpoint node IDs).\n")
	b.WriteString("// _fromLabel/_toLabel/_label default to the entity type names if omitted.\n")
	fmt.Fprintf(&b, "type %sRelationCreate struct {\n", entity.Name)

	// Edge metadata first — required _fromId, _toId; optional labels.
	b.WriteString("\t// Edge metadata. _fromId/_toId are required; the labels are\n")
	b.WriteString("\t// auto-derived server-side if omitted.\n")
	b.WriteString("\tFromID    string `json:\"_fromId\"`\n")
	b.WriteString("\tToID      string `json:\"_toId\"`\n")
	b.WriteString("\tFromLabel string `json:\"_fromLabel,omitempty\"`\n")
	b.WriteString("\tToLabel   string `json:\"_toLabel,omitempty\"`\n")
	b.WriteString("\tLabel     string `json:\"_label,omitempty\"`\n\n")

	// Then introspection-advertised business fields, skipping any that
	// collide with the edge-metadata names we just emitted.
	skip := map[string]bool{
		"_fromId": true, "_toId": true,
		"_fromLabel": true, "_toLabel": true,
		"_label": true,
	}
	for _, f := range entity.Relationship.CompoundFields {
		if skip[f.Name] {
			continue
		}
		writeField(&b, f, model.Scalars, emitting)
	}
	b.WriteString("}\n")

	return FormatGo(b.String())
}
