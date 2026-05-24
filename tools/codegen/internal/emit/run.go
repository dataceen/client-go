package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

// Result reports what an emit run produced.
type Result struct {
	OutDir       string
	PackageName  string
	Entities     []string
	FilesWritten []string
	FilesSkipped []string
}

// Run computes the BFS emission set rooted at `roots`, emits all four file
// kinds (model, create, fields, client) for every entity in the set, and
// writes them under outDir. outDir is wiped of *.go files first so removed
// entities don't leave stale generated files.
//
// If gofmt rejects emitted source the file is still written so the failure
// is inspectable; Run returns the first format error encountered.
func Run(model schema.ModelSchema, roots []string, outDir, packageName string) (*Result, error) {
	emitting, entities := PlanEmissionSet(model, roots)
	if len(entities) == 0 {
		return nil, fmt.Errorf("emit: no entities found for roots %v (check spelling — names are case-sensitive)", roots)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("emit: mkdir %s: %w", outDir, err)
	}
	if err := wipeGeneratedFiles(outDir); err != nil {
		return nil, fmt.Errorf("emit: wipe %s: %w", outDir, err)
	}

	res := &Result{OutDir: outDir, PackageName: packageName}
	for _, e := range entities {
		res.Entities = append(res.Entities, e.Name)
	}

	// Always emit a package marker file so `go build ./pkg/generated` works
	// even before any entity is added.
	if err := writeFile(filepath.Join(outDir, "doc.go"), fmt.Sprintf(
		"// Package %s holds Dataceen client code generated from the GraphQL\n"+
			"// introspection schema. Regenerate via:\n//\n"+
			"//\tgo run ./tools/codegen/cmd emit %s\npackage %s\n",
		packageName, strings.Join(roots, " "), packageName)); err != nil {
		return res, err
	}
	res.FilesWritten = append(res.FilesWritten, "doc.go")

	var firstErr error

	for _, e := range entities {
		// Model
		src, err := EmitModel(model, e, packageName, emitting)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("EmitModel(%s): %w", e.Name, err)
		}
		if src != "" {
			fname := e.Name + ".go"
			if err := writeFile(filepath.Join(outDir, fname), src); err != nil {
				return res, err
			}
			res.FilesWritten = append(res.FilesWritten, fname)
		}

		// Create
		src, err = EmitCreate(model, e, packageName, emitting)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("EmitCreate(%s): %w", e.Name, err)
		}
		if src == "" {
			res.FilesSkipped = append(res.FilesSkipped, e.Name+"Create.go")
		} else {
			fname := e.Name + "Create.go"
			if err := writeFile(filepath.Join(outDir, fname), src); err != nil {
				return res, err
			}
			res.FilesWritten = append(res.FilesWritten, fname)
		}

		// FieldConsts — typo-safe field-name constants used with dsl.F()
		src, err = EmitFieldConsts(model, e, packageName)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("EmitFieldConsts(%s): %w", e.Name, err)
		}
		if src != "" {
			fname := e.Name + "FieldConsts.go"
			if err := writeFile(filepath.Join(outDir, fname), src); err != nil {
				return res, err
			}
			res.FilesWritten = append(res.FilesWritten, fname)
		}

		// Aggregation input types (only for entities with HasSearch)
		src, err = EmitAggregation(model, e, packageName, emitting)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("EmitAggregation(%s): %w", e.Name, err)
		}
		if src == "" {
			res.FilesSkipped = append(res.FilesSkipped, "Search"+e.Name+"Aggregation.go")
		} else {
			fname := "Search" + e.Name + "Aggregation.go"
			if err := writeFile(filepath.Join(outDir, fname), src); err != nil {
				return res, err
			}
			res.FilesWritten = append(res.FilesWritten, fname)
		}

		// RelationCreate (relationships only)
		src, err = EmitRelationCreate(model, e, packageName, emitting)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("EmitRelationCreate(%s): %w", e.Name, err)
		}
		if src == "" {
			res.FilesSkipped = append(res.FilesSkipped, e.Name+"RelationCreate.go")
		} else {
			fname := e.Name + "RelationCreate.go"
			if err := writeFile(filepath.Join(outDir, fname), src); err != nil {
				return res, err
			}
			res.FilesWritten = append(res.FilesWritten, fname)
		}

		// Update
		src, err = EmitUpdate(model, e, packageName, emitting)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("EmitUpdate(%s): %w", e.Name, err)
		}
		if src == "" {
			res.FilesSkipped = append(res.FilesSkipped, e.Name+"Update.go")
		} else {
			fname := e.Name + "Update.go"
			if err := writeFile(filepath.Join(outDir, fname), src); err != nil {
				return res, err
			}
			res.FilesWritten = append(res.FilesWritten, fname)
		}

		// Fields
		src, err = EmitFields(model, e, packageName, emitting)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("EmitFields(%s): %w", e.Name, err)
		}
		if src != "" {
			fname := e.Name + "Fields.go"
			if err := writeFile(filepath.Join(outDir, fname), src); err != nil {
				return res, err
			}
			res.FilesWritten = append(res.FilesWritten, fname)
		}

		// Client (top-level entities only — emits Find/FindByID + optional
		// Create/Delete (for nodes) + Update)
		src, err = EmitClient(model, e, packageName)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("EmitClient(%s): %w", e.Name, err)
		}
		if src == "" {
			res.FilesSkipped = append(res.FilesSkipped, e.Name+"Client.go")
		} else {
			fname := e.Name + "Client.go"
			if err := writeFile(filepath.Join(outDir, fname), src); err != nil {
				return res, err
			}
			res.FilesWritten = append(res.FilesWritten, fname)
		}
	}

	// AllClient — one aggregator over every top-level entity in the emit set.
	src, err := EmitAllClient(model, entities, packageName)
	if err != nil && firstErr == nil {
		firstErr = fmt.Errorf("EmitAllClient: %w", err)
	}
	if src != "" {
		if err := writeFile(filepath.Join(outDir, "AllClient.go"), src); err != nil {
			return res, err
		}
		res.FilesWritten = append(res.FilesWritten, "AllClient.go")
	}

	return res, firstErr
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// wipeGeneratedFiles removes *.go files in dir (non-recursive). Preserves
// *_test.go (hand-written) and non-Go files (e.g. a README).
func wipeGeneratedFiles(dir string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}
