// Package dsl is the public DSL runtime that generated client code depends on.
//
// It provides three concerns:
//
//  1. The QueryPlan AST (FilterNode, FieldNode, OrderByNode, QueryPlan) —
//     the language-agnostic intermediate form.
//  2. PlanToGraphQL — single source of truth for serialising a plan to the
//     wire format the Dataceen server expects.
//  3. Fluent builders — F(), And(), Or(), Not(), Fields(), OrderBy() — so
//     handwritten and generated code share the same API.
//
// Operator and direction casing matches the live server (decisions.md
// #26–30): lowercase operators (eq, like, and, or, ...) and PascalCase
// enum values (Ascending, Descending).
//
// `internal/prototypes/` keeps a parallel copy of the AST + serializer as a
// regression baseline pinned to the canonical wire format. If you change
// anything here, run `go test ./internal/prototypes/...` — that test locks
// the byte-exact output.
package dsl
