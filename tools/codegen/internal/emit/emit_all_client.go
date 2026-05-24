package emit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// EmitAllClient produces `AllClient.go` — a one-stop aggregator that holds
// a *<Entity>Client per top-level entity. Convenience wrapper so callers
// don't have to construct each entity client by hand.
//
// Only entities in the emitting set with IsTopLevel=true are included.
func EmitAllClient(model schema.ModelSchema, entities []schema.EntityInfo, packageName string) (string, error) {
	var topLevel []string
	for _, e := range entities {
		if e.IsTopLevel {
			topLevel = append(topLevel, e.Name)
		}
	}
	sort.Strings(topLevel)
	if len(topLevel) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(FileHeader("AllClient", packageName))
	b.WriteString(`import "github.com/dataceen/client-go/pkg/dataceen"` + "\n\n")

	b.WriteString("// AllClient bundles a typed sub-client per top-level entity.\n")
	b.WriteString("// Construct with NewAllClient(client) and access entity APIs through\n")
	b.WriteString("// fields named after the entity, e.g. api.Customer.Find(ctx, ...).\n")
	b.WriteString("type AllClient struct {\n")
	for _, name := range topLevel {
		fmt.Fprintf(&b, "\t%s *%sClient\n", name, name)
	}
	b.WriteString("}\n\n")

	b.WriteString("// NewAllClient builds an AllClient over the provided dataceen.Client.\n")
	b.WriteString("func NewAllClient(c *dataceen.Client) *AllClient {\n")
	b.WriteString("\treturn &AllClient{\n")
	for _, name := range topLevel {
		fmt.Fprintf(&b, "\t\t%s: New%sClient(c),\n", name, name)
	}
	b.WriteString("\t}\n}\n")

	return FormatGo(b.String())
}
