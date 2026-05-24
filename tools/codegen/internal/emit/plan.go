package emit

import (
	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// PlanEmissionSet computes the transitive set of entity names reachable from
// the given roots by following non-scalar/non-enum field types. Sub-entities
// without a Find<X> query are still discovered because Customer's read-model
// references CustomerName etc. directly.
//
// Returns the set as a name→true map (suitable for GoType's `emitting`
// argument) and the list of EntityInfos in BFS order so the orchestrator
// can iterate deterministically.
func PlanEmissionSet(model schema.ModelSchema, roots []string) (map[string]bool, []schema.EntityInfo) {
	emitting := make(map[string]bool)
	var ordered []schema.EntityInfo

	queue := append([]string{}, roots...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if emitting[name] {
			continue
		}
		entity := model.FindEntity(name)
		if entity == nil {
			continue
		}
		emitting[name] = true
		ordered = append(ordered, *entity)

		// Enqueue every entity-typed field reference.
		for _, f := range entity.Read.Fields {
			if f.BaseKind == "OBJECT" || f.BaseKind == "INPUT_OBJECT" {
				queue = append(queue, f.BaseType)
			}
		}
		// Don't traverse Create/Update/Filter input types here — their
		// payloads usually overlap with read-model references and we want
		// the read-model graph to drive emission. Refine in Phase 5 if a
		// type leaks through.
	}

	return emitting, ordered
}
