package emit

import (
	"fmt"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// EmitCreate produces `<Entity>Create.go` — the input struct for
// Create<Entity> mutations. Relationship entities have no Create input;
// EmitCreate returns ("", nil) for them — the caller should skip writing
// the file. Compound creates (Create{From}{Rel}{To}) are emitted by a
// separate emitter (Phase 5).
func EmitCreate(model schema.ModelSchema, entity schema.EntityInfo, packageName string, emitting map[string]bool) (string, error) {
	if entity.Create == nil {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(FileHeader(entity.Name+"Create", packageName))

	fmt.Fprintf(&b, "// %sCreate is the input shape for the Create%s mutation.\n",
		entity.Name, entity.Name)
	fmt.Fprintf(&b, "type %sCreate struct {\n", entity.Name)

	for _, f := range entity.Create.Fields {
		writeField(&b, f, model.Scalars, emitting)
	}
	b.WriteString("}\n")

	return FormatGo(b.String())
}
