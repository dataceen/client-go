package emit

import (
	"fmt"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// EmitFieldConsts produces `<Entity>FieldConsts.go` — a single struct
// `<Entity>Field` with one string member per scalar/relation field, set to
// the wire-format name. Autocomplete-friendly companion to the
// stringly-typed dsl.F builder:
//
//	// Before — no compile-time check on the column name:
//	dsl.F("CustomerId").Like("cst")
//
//	// After — typo would fail to compile:
//	dsl.F(generated.CustomerField.CustomerId).Like("cst")
//
// Emitted for every entity that has a Read.Fields list (i.e. every entity
// in the emit set).
func EmitFieldConsts(model schema.ModelSchema, entity schema.EntityInfo, packageName string) (string, error) {
	if len(entity.Read.Fields) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(FileHeader(entity.Name+"FieldConsts", packageName))

	fmt.Fprintf(&b, "// %sField holds the wire-format names of every %s field.\n",
		entity.Name, entity.Name)
	b.WriteString("// Use with the DSL for typo-safe filter and order-by clauses:\n")
	b.WriteString("//\n")
	fmt.Fprintf(&b, "//\tdsl.F(%sField.CustomerId).Like(\"cst\")\n", entity.Name)
	fmt.Fprintf(&b, "//\tdsl.OrderBy().Asc(%sField.CustomerId)\n", entity.Name)
	fmt.Fprintf(&b, "var %sField = struct {\n", entity.Name)
	for _, f := range entity.Read.Fields {
		goName := GoFieldName(f.Name)
		fmt.Fprintf(&b, "\t%s string\n", goName)
	}
	b.WriteString("}{\n")
	for _, f := range entity.Read.Fields {
		goName := GoFieldName(f.Name)
		fmt.Fprintf(&b, "\t%s: %q,\n", goName, f.Name)
	}
	b.WriteString("}\n")

	return FormatGo(b.String())
}
