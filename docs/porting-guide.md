# Porting guide — Go-specific addendum

This file copies the structure of `C:\Source\Dataceen\DataceenClientTs\docs\porting-guide.md`
and adds **Go-specific gotchas** at each section. Read both files together.

## Reference implementations

- **C# (authoritative):** `C:\Source\Dataceen\DataceenClient\` — shipped, NuGet v1.1.33
- **TypeScript (verified against live):** `C:\Source\Dataceen\DataceenClientTs\` — feature parity, ~10 weeks of work
- **Go (this port, in progress):** `C:\Source\Dataceen\DataceenClientGoLang\`
- **Proto file:** `protos/dataceenevent.proto` (copy of `C:\Source\Dataceen\DataceenEvent\Protos\dataceenevent.proto` with `option go_package` added)
- **Config template:** `.env.example` (mirror of TS `.env.example` and C# `appsettings.json`)
- **Example data model:** `Thomas/CandyShopModel/All` — 9 nodes, 18 relationships, ~12 sub-entities

---

## What transfers verbatim (server-imposed)

Read `C:\Source\Dataceen\DataceenClientTs\docs\porting-guide.md` "What transfers
verbatim" section in full — every fact there applies to Go too. Highlights:

- Azure AD client-credentials flow; api-scope ≠ subscription-scope
- 401 → force-refresh and retry once
- Query name is `Find{TypeName}`, NOT `Find{TypeName}Nodes`
- Operators lowercase (`eq, neq, gt, gte, lt, lte, like, nlike, contains, ncontains, in, nin, is_null, exists`)
- Composition lowercase (`and, or, not`)
- Response shape PascalCase (`Items, Cursor, ResponseTime, _id`)
- Enum values in input are unquoted GraphQL enum syntax (`Ascending`, `Descending`, `Bucket_Terms`)
- `like` is Elasticsearch `match` — NO wildcards, NO delimiters
- Scalars: `Boolean, Int, Float, Double → numeric`; `Long, Decimal → string`; `Date, DateTime, DateTimeOffset, Time, TimeSpan → ISO string`
- Server-introspection gaps: `_fromId/_toId` on relations, `Aggregations` on search results, duplicate types
- Mutations are NOT retryable; queries are (5xx/408/429 + network errors)

The **CandyShopModel smoke test** (6 cases at the bottom of the TS porting guide)
is the canonical regression for any port. Reproduce it in `examples/candyshop/`
once the runtime exists.

---

## Go-specific design choices

### 1. DSL strategy

Go's static-typed, no-runtime-AST nature pushes the design away from both the
C# (LINQ) and TS (proxy) strategies. See `dsl-comparison.md` for the full
analysis. **Default plan:**

- Filter / orderBy: typed structs with `omitempty` + pointer fields per scalar
- Fields-selection: `[]string` with dot-path syntax (`"CustomerPlacedOrder.Order._id"`) — handwritten helper parses and emits nested GraphQL

Avoid generic `Filter[T]()`-builders; Go generics in 1.22 don't support
"indexed access types" cleanly, and the ergonomics fall off a cliff at depth.

### 2. Async / cancellation

Map TS `AbortSignal` and C# `CancellationToken` to **`context.Context`**.

```go
ctx, cancel := context.WithTimeout(parent, 30*time.Second)
defer cancel()
res, err := client.ExecuteQuery(ctx, ...)
```

Subscriptions: cancel by canceling the ctx that was passed to `Subscribe()`.
Don't add a separate `Cancel()` method — that's TS-flavored and idiomatic Go
projects use ctx everywhere.

### 3. Typing strategy

Generated entity types are plain Go structs with PascalCase JSON tags:

```go
type Customer struct {
    ID           string  `json:"_id,omitempty"`
    CustomerID   string  `json:"CustomerId"`
    IsActive     bool    `json:"IsActive"`
    Name         *CustomerName `json:"Name,omitempty"`
}
```

Operation-specific types (`Customer`, `CustomerCreate`, `CustomerUpdate`,
`CustomerFilter`) MUST stay distinct — server accepts different field subsets
for each. Don't share via embedding.

`Long`/`Decimal` → `string` (precision-safe). Provide `dataceen/scalars`
helpers if users want `int64` / `decimal.Decimal` round-trips.

### 4. Change-tracking on Update

C#: backing fields + `SetValue()`.
TS: class with `_changed: Set<Key>` + getters/setters.
Go: no proxy. Two viable approaches:

- **A — set-flag struct:** generator emits `CustomerUpdate` with a `_set` `map[string]bool` and accessor methods (`SetExternalReference(v string)`) that mark the field dirty. `MarshalJSON` emits only dirty fields.
- **B — explicit map:** `update := dataceen.Update[Customer]{"ExternalReference": "y"}`. Trivial but no type-checking.

Default plan: **A**, because the C# and TS ports both went that way and the
generator can emit accessors mechanically.

### 5. Codegen tooling

Idiomatic Go: a binary in `tools/codegen/` that fetches the schema, parses it,
and emits files into `pkg/generated/`. Use `go/format` to gofmt the output;
use `text/template` for templates. No build-script integration unless that
becomes ergonomic.

`tools/codegen/cache/schema.json` caches the introspection dump (~500 KB).

### 6. Aggregation result recursion

Same as every port: GraphQL doesn't support recursive fragments. Pick a finite
expansion depth (default 3, override via `WithAggregationDepth(int)` option).
The TS expansion code in `src/runtime/aggregations.ts` is the reference.

### 7. gRPC: static codegen, not dynamic loading

TS uses `@grpc/proto-loader` to load the proto at runtime. Go's idiomatic
equivalent is **static codegen** via `protoc` + `protoc-gen-go` +
`protoc-gen-go-grpc`. The Makefile at the repo root runs the generation;
generated files live in `pkg/dataceenevent/`. Commit the generated files
(unlike the TS port where they don't exist) so consumers don't need protoc to
build.

(`google.golang.org/protobuf/types/dynamicpb` exists but is rarely worth it.)

### 8. Logging

Go has multiple competing log libraries (stdlib `log/slog` since 1.21, `zap`,
`zerolog`, `logrus`). Don't pick one.

Define a tiny `Logger` interface with `Info(msg string, kv ...any)` /
`Warn(...)` / `Error(...)` and provide a default that wraps `slog.Default()`.
Consumers wire their own adapter in ~10 lines.

---

## Recommended phase order (same as TS plan)

1. **Spike (1–2 days).** Token / GraphQL / gRPC / introspection.
2. **Runtime (3–5 days).** HTTP transport with 401-refresh + retry. Token provider. Result types.
3. **DSL prototypes (2–3 days).** A vs B vs C. Write up `dsl-comparison.md`.
4. **Codegen v1 (1 week).** Schema → Customer.
5. **Subscriptions (3–5 days).** gRPC client + baseload + reconnect. Can run parallel with codegen.
6. **Full model (3–5 days).** Sub-entities, compound creates, aggregations.
7. **Example app (3–5 days).** Port `Thomas_CandyShopModel_All_Example`.
8. **Packaging.** `git tag v0.1.0`. Document the import path.

**Tempo check:** if codegen takes more than ~10 days, you're overdesigning.

---

## Smoke test that proves the port works

Verbatim from the TS porting guide — port to Go in `examples/candyshop/`:

```pseudo
1. findById("Customer_Example_00000") returns Customer with CustomerId="cst-000001"
2. find({ like: "cst", size: 5 }) returns 5 items
3. search({ aggregations: [{ IsActive: Bucket_Terms "by-active" }] }) returns buckets with Count>0
4. subscribe(topics: ["Customer"], baseload: true) yields ≥5 BASELOAD_EVENT events before BASELOAD_STEP_1
5. create then update then delete a test Customer, all return Result with one id
6. create a CustomerPlacedOrder relationship (compound mutation), verify it shows up via find
```

If all 6 pass, feature parity is achieved.

---

## Things the TS port already proved — don't re-derive

- The schema has 498 types, 12 scalars, 5 enums, ~143 mutations, ~32 queries for CandyShopModel
- Introspection is enabled; use it as the codegen source
- Introspection lies in three known places (`_fromId/_toId`, compound-create extra fields, `Aggregations` on search results)
- `like` + `-` characters don't work the way SQL users expect
- `cursor: "null"` (literal string) means "start"
- Mutations are NOT retryable
- MSAL caches tokens transparently — only force refresh on 401
- Sub-entities (CustomerName, CustomerContact, CustomerAddress, …) have no `FindX` query; they're reached via parent fields only

A Go port that handles these on day one is significantly ahead of where the
TS port was at end of fas 3.
