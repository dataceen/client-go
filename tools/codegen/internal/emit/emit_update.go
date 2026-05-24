package emit

import (
	"fmt"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// EmitUpdate produces `<Entity>Update.go` — the input shape for the
// Update<Entity> mutation.
//
// Every field is a pointer so callers can express "leave this column
// alone" by leaving the pointer nil. Use dataceen.Ptr(v) to set a value.
// Per dsl.EncodeInput, nil pointers with `,omitempty` are dropped from
// the request — the server then keeps existing values.
//
// Returns ("", nil) when the entity has no Update input type.
func EmitUpdate(model schema.ModelSchema, entity schema.EntityInfo, packageName string, emitting map[string]bool) (string, error) {
	if entity.Update == nil {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(FileHeader(entity.Name+"Update", packageName))

	fmt.Fprintf(&b, "// %sUpdate carries the optional fields to set when updating a %s.\n",
		entity.Name, entity.Name)
	b.WriteString("// Nil pointers are omitted from the request; non-nil pointers are sent\n")
	b.WriteString("// as the new value (use dataceen.Ptr to construct).\n")
	fmt.Fprintf(&b, "type %sUpdate struct {\n", entity.Name)

	for _, f := range entity.Update.Fields {
		writeUpdateField(&b, f, model.Scalars, emitting)
	}
	b.WriteString("}\n")

	return FormatGo(b.String())
}

// writeUpdateField emits one Update field. Differs from writeField in that:
//   - All fields use pointer types (so nil = "no change")
//   - All fields use `,omitempty` so dsl.EncodeInput drops them
func writeUpdateField(b *strings.Builder, f schema.FieldRef, scalars map[string]string, emitting map[string]bool) {
	goName := GoFieldName(f.Name)
	base := goBase(f, scalars, emitting)

	var goType string
	switch {
	case f.IsList:
		// Lists get sent verbatim (server replaces the whole list).
		goType = "[]" + base
	case base == "any":
		// `*any` is awkward; an interface already carries the pointer.
		goType = base
	default:
		goType = "*" + base
	}

	tag := fmt.Sprintf("`json:\"%s,omitempty\"`", f.Name)
	fmt.Fprintf(b, "\t%s %s %s\n", goName, goType, tag)
}
