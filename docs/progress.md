# Progress

Löpande status. Uppdatera när du går vidare.

## Nuläge

**Aktuell fas:** **v0.2.1 + känt wire-format-skew väntar på server-uppdatering (2026-05-04).** v0.2.1 (typo-safe field constants) klar och taggad. Proto-filen uppdaterades 2026-05-04 med ett nytt `domain`-fält på tag 8 i `EventData`, vilket flyttat ner alla efterföljande fält ett steg. Servern är inte uppdaterad än — se "Känt problem" nedan.

**Verifierat på maskin:** Go 1.26.2 + protoc 29.3 (portable install i `~/bin/`).

## Känt problem: wire-format-skew (2026-05-04)

**Symptom:** Live-subscription-tester får 0 events. Sektion 6 i `examples/candyshop/` rapporterar `clean shutdown after 0 events` istället för 5.

**Root cause:** `protos/dataceenevent.proto` uppdaterades med:
```protobuf
message EventData {
  ...
  string clientid = 7;
  string domain = 8;       // NYTT — sköt ner alla efterföljande fält
  string model = 9;        // tidigare 8
  string topic = 10;       // tidigare 9
  string id = 11;          // tidigare 10
  ...
}
```

Det är en wire-incompatible-ändring (proto-tag-numreringen ÄR wire-formatet). Servern sänder fortfarande på de gamla tag-numren, så vår client decodear allt skiftat ett steg upp:

| Servern sänder | Server-tag | Vår client tolkar som |
|---|---|---|
| `model="Customer"` | 8 | `Domain="Customer"` |
| `topic="Customer"` | 9 | `Model="Customer"` |
| `id="Customer_Example_00000"` | 10 | `Topic="Customer_Example_00000"` |
| `fromid` | 11 | `ID` |
| ... | ... | (allt skiftat ett steg) |

Typed-handlers som filtrerar på `Topic == "Customer"` matchar därför aldrig.

**Vad som har gjorts (2026-05-04):**

- `pkg/dataceenevent/*.pb.go` regenererad mot nya proton via `protoc --proto_path=protos --proto_path=$HOME/bin/protoc-include --go_out=. --go_opt=module=github.com/dataceen/client-go --go-grpc_out=. --go-grpc_opt=module=github.com/dataceen/client-go protos/dataceenevent.proto`
- `LowLevelEvent` och `Event[T]` har nytt `Domain string`-fält
- `toLowLevel()`, `syntheticEvent()` och `On[T]`-wrappern kopierar Domain genom
- Build, vet, och unit-tester gröna
- Live-subscription-test markerad med "KNOWN ISSUE"-docstring som förklarar skew

**Vad som händer när servern uppdateras till nya proton:**

- Inga ytterligare kod-ändringar behövs på Go-sidan
- Subscription-test börjar automatiskt passera
- `evt.Domain` populeras från servern istället för att vara tomt

**Vad fungerar fortfarande (HTTP-baserat, opåverkat):**

- Find / FindByID / Like / OrderBy / Pagination — sektion 1-5 i candyshop
- Search + aggregations — sektion 8
- Create / Update / Delete / Bulk / CompoundCreate — sektion 7
- Allt utom gRPC-subscriptions (sektion 6)

Verifierat 2026-05-04: `go run ./examples/candyshop` ger "All scenarios passed" med 0 events i sektion 6 (väntat) och alla andra sektioner gröna.

## Avklarade steg

- [x] Plan skapad (`docs/plan.md`) — 8 faser, ~10 veckor med buffer
- [x] Defaults beslutade (`docs/decisions.md`) — Go-specifika + ärvda från TS
- [x] Projektmapp skapad: `C:\Source\Dataceen\DataceenClientGoLang\`
- [x] `go.mod` initialiserad (modulpath `github.com/dataceen/client-go`, Go 1.22+)
- [x] `.env.example` kopierad från TS-porten
- [x] `.gitignore` på plats
- [x] `protos/dataceenevent.proto` kopierad och kompletterad med `option go_package`
- [x] `Makefile` med `gen`, `tools`, `spike01..04`, `build`, `test`, `tidy`
- [x] `pkg/dataceenevent/` doc.go + gen.go (genererade *.pb.go skapas via `make gen`)
- [x] `internal/spikecfg/cfg.go` — gemensam .env-loader
- [x] Spike 01 — `cmd/spike01-token/main.go` (MSAL → Bearer)
- [x] Spike 02 — `cmd/spike02-graphql/main.go` (FindCustomer mot apiUrl)
- [x] Spike 03 — `cmd/spike03-subscription/main.go` (gRPC stream, första 10 events)
- [x] Spike 04 — `cmd/spike04-introspection/main.go` (`__schema`-probe)

## Fas 0 — Spike ✅ KLAR (2026-04-30)

Verifierat mot live-backend (Thomas/CandyShopModel/All), samma `.env` som TS-porten:

| Spike | Resultat | Detalj |
|---|---|---|
| spike01-token | ✅ OK | JWT 3 segments, length 1308, expires +1h |
| spike02-graphql | ✅ OK | HTTP 200, 5 customers (cst-000001 .. cst-000005) |
| spike03-subscription | ✅ OK | 1 KEEPALIVE + 9 BASELOAD_EVENT på Customer-topic |
| spike04-introspection | ✅ OK | 498 typer (5 ENUM, 363 INPUT_OBJECT, 118 OBJECT, 12 SCALAR), Customer har 13 fält — exakt samma som TS-porten observerade |

### Fynd från fas 0 (Go-specifika)

- **`codes.Canceled` (single L)** — Go gRPC använder amerikansk stavning, inte `Cancelled`. Liten gotcha jämfört med TS:s `grpc.status.CANCELLED`.
- **`paths=source_relative` lägger filer fel** med vår repo-struktur (proto i `protos/`, output önskas i `pkg/dataceenevent/`). Använd `--go_opt=module=github.com/dataceen/client-go` istället så följer protoc:s plugin `option go_package` och strippar modulprefixet.
- **Portable protoc-install kräver explicit `--proto_path` till include-katalogen** med well-known types (annars `google/protobuf/timestamp.proto: File not found`). Makefile har `PROTOC_INCLUDE`-variabel för detta.
- **Spike 03 öppnar med en `KEEPALIVE`-event innan baseload börjar** — TS-koden rapporterade aldrig dessa eftersom proto-loader-defaultsen skippar dem; Go:s grpc-stream levererar dem råa. Inget problem, bara observerat.
- **MSAL Go: `confidential.NewCredFromSecret(secret)` returnerar både cred och error** — annorlunda från TS:s konstruktor som tar secret som plain string. Hanterat i alla spikes.

### Hur du återskapar (för en ny utvecklare)

```bash
# Förutsättningar:
#   - Go 1.22+
#   - protoc (paket-manager ELLER portable-zip till ~/bin/)
#   - .env med DATACEEN_CLIENT_SECRET (kopierad från TS-porten eller annan källa)

make tools                                    # protoc-gen-go + protoc-gen-go-grpc
make tidy                                     # go mod download

# Vid portable protoc-install: ange explicit include-path
PROTOC_INCLUDE=$HOME/bin/protoc-include make gen

# (Vid system-installerad protoc räcker `make gen` utan PROTOC_INCLUDE.)

make spike01 spike02 spike04 spike03
```

## Fas 1 — Kärn-runtime ✅ KLAR (2026-04-30)

### Filer

- [x] `pkg/dataceen/config.go` — `Config`-struct + functional options (`WithLogger`, `WithHTTPClient`, `WithHTTPTimeout`, `WithMaxTransientRetries`, `WithTransientRetryBase`, `WithSlowHandlerThreshold`, `WithTokenProvider`)
- [x] `pkg/dataceen/logger.go` — `Logger`-interface, `ConsoleLogger`, `SilentLogger`, `LogLevel` (debug/info/warn/error)
- [x] `pkg/dataceen/errors.go` — `*Error` med `ResponseCode`, `SourceQuery`, `Cause`. Implementerar `error` + `Unwrap` så `errors.As`/`Is` fungerar.
- [x] `pkg/dataceen/types.go` — `FindResult[T]`, `SearchResult[T]`, `QueryResult[T]`, `RequestResult[T]`, `MutationResult`, `SingleMutationResult`, `FindByIdResult[T]`, `GraphQL`. Alla med PascalCase JSON-tags.
- [x] `pkg/dataceen/token_provider.go` — `TokenProvider`-interface + MSAL-implementation. Force-refresh = rebuild av underliggande `confidential.Client` (MSAL Go saknar publik `ForceRefresh`-flagga).
- [x] `pkg/dataceen/http_transport.go` — `httpTransport.postGraphQL` med 401-refresh, transient-retry (408/429/5xx + nätverksfel), exponentiell backoff (base*2^n), context-cancellation propagering.
- [x] `pkg/dataceen/client.go` — `Client.ExecuteQuery` / `ExecuteMutation` med `parseGraphQLResponse` (envelope → `data[op]` → `ResponseCode!=0`-check → typad decode).

### Tester

- [x] `pkg/dataceen/http_transport_test.go` — 9 tester: first-try success, 401→refresh, double-401, 5xx→success, 5xx-give-up, 5xx no-retry-on-mutation, 4xx no-retry, auth-header, token-acquire-fail
- [x] `pkg/dataceen/client_test.go` — 5 tester: success, GraphQL-errors, ResponseCode≠0, missing operation, mutation-no-retry-default
- [x] `pkg/dataceen/integration_test.go` — `TestIntegration_FindCustomer` mot live, gated av `RUN_INTEGRATION=1`. Verifierat: 5 customers, första `cst-000001`, cursor matchar tidigare spike-resultat.

```
make test                  # 14 unit, 1 skip                             → 1.95s
make test-integration      # FindCustomer(size=5) → 5 items mot live     → 0.45s
```

### API-yta

```go
client, err := dataceen.NewClient(cfg,
    dataceen.WithLogger(myLogger),
    dataceen.WithMaxTransientRetries(3),
    dataceen.WithHTTPTimeout(30*time.Second))

var res dataceen.FindResult[Customer]
err = client.ExecuteQuery(ctx, dataceen.GraphQL{
    OperationName: "FindCustomer",
    Query:         `query { FindCustomer(size: 5, cursor: "null", where: null) { Items { _id CustomerId } Cursor } }`,
}, &res)

// Mutationer: ingen retry per default (icke-idempotenta)
err = client.ExecuteMutation(ctx, mutReq, &mutRes)

// Tvinga retry på/av per anrop
err = client.ExecuteQuery(ctx, req, &res, dataceen.WithoutRetry())
```

### Go-specifika fynd från fas 1

- **MSAL Go saknar publik `ForceRefresh`-flagga** (till skillnad från MSAL .NET och MSAL Node). `confidential.AcquireByCredentialOption`-listan har inga relevanta options. Lösning: token-providern håller `clientSecret` + `authority` och bygger om hela `confidential.Client` på `forceRefresh=true` — ny client = tom in-memory cache = ny STS-roundtrip. Tradeoff: små allokeringar, men 401 är sällsynt så det är okej.
- **`http.NewRequestWithContext` + per-request timeout** kräver att child-context cancellas när responsen är konsumerad, annars läcker timer-allocs. Lösning: en `cancelOnClose`-wrapper på `Body` som anropar `cancel()` i `Close()`.
- **`json.RawMessage`-sniff för `ResponseCode`** innan typad decode: vi kan inte unmarshal:a in i `out`-typen först (då tappar vi kontroll över envelope-checks), och vi kan inte heller dubbel-unmarshal:a hela payloaden in i `map[string]any` (slow, lossy för numeric precision). `json.RawMessage` är bytes-utdrag, perfekt för pre-validering.
- **`errors.As(err, &de)` fungerar för `*dataceen.Error`** eftersom `Error()` är pointer-receiver och `*Error` implementerar `error`. Tester använder `errors.As` istället för type-assertion direkt.

## Fas 2 — DSL-prototyper ✅ KLAR (2026-05-01)

### Filer

- [x] `internal/prototypes/ast.go` — språkagnostisk AST (`QueryPlan`, `FilterNode` (sum: Cond/And/Or/Not), `FieldNode` (sum: Scalar/Nested), `OrderByNode`)
- [x] `internal/prototypes/serializer.go` — `PlanToGraphQL` (single source of truth för wire format)
- [x] `internal/prototypes/model.go` — handritad Customer/Order/CustomerPlacedOrder för prototyperna
- [x] `internal/prototypes/testcase.go` — `TargetPlan` + `ExpectedQuery()`
- [x] `internal/prototypes/a/build.go` — typade structs + per-entitet converter (~250 rader)
- [x] `internal/prototypes/b/build.go` — `map[string]any` + dot-path fields (~120 rader)
- [x] `internal/prototypes/c/build.go` — fluent builders + variadic And/Or (~150 rader)
- [x] `internal/prototypes/prototypes_test.go` — 5 tester, alla gröna

```
go test ./internal/prototypes/...      → 5 PASS, 0.80s
```

### Beslut

**Strategi C (fluent builders) för filter + orderBy, strategi A (typed struct) för fields.**

Se `docs/dsl-comparison.md` för full motivering. Sammanfattning: C vinner på
verbosity, generator-storlek, och kompositions-ergonomi; A vinner på
compile-time säkerhet vid djupa fältval. Hybriden tar det bästa av båda.

### Konsekvenser för fas 3

Generatorn emit:ar:

1. **Per entitet:** `<E>Fields`-struct (strategi A:s shape) + en converter-funktion
2. **Per entitet:** `<E>` (read), `<E>Create` (write-create), `<E>Update` (klass med ändringsspårning — design beslut tas i fas 3)
3. **Delas mellan alla entiteter** (redan i `internal/prototypes/c/`): `FilterExpr`-interface, `F()/And()/Or()/Not()`, `OrderByBuilder`, `PlanToGraphQL`-serializer

Promotering: `internal/prototypes/c/build.go` → `pkg/dataceen/dsl/` (eller liknande publikt namn) i fas 3.

## Fas 3 — Kodgenerator (påbörjad 2026-05-01)

Mirror av TS-portens fas 3-uppdelning (TS `progress.md` "Fas 3" sektion).

### Steg 1 — Schema-dump ✅ KLAR

- [x] `tools/codegen/internal/schema/introspection.go` — `IntrospectionQuery` (verbatim TS, ofType-djup 7) + GraphQL-typdefinitioner (`IntrospectionResponse`, `Type`, `Field`, `InputValue`, `TypeRef`, `EnumValue`)
- [x] `tools/codegen/internal/schema/fetch.go` — `Fetch(ctx, cfg)` returnerar `CachedSchema` med metadata + data; `WriteCache`/`ReadCache`-helpers
- [x] `tools/codegen/cmd/codegen.go` — CLI med `schema`-subkommando (`--print` skippar fetch)
- [x] **Verifierat mot live:** 498 typer (5 ENUM + 363 INPUT_OBJECT + 118 OBJECT + 12 SCALAR) — exakt matchning med spike-04 och TS-porten

```
make codegen-schema           → fetchar, skriver tools/codegen/cache/schema.json (1.3 MB pretty-printed)
make codegen-schema-print     → re-printar stats utan att hämta
```

#### Bug funnen + fixad i runtime under steg 1

Introspection-respons keyas av GraphQL-fältnamn (`__schema`), inte av
operation-namn. Detta bröt `Client.ExecuteQuery`s strikta envelope-pickup
(`data[OperationName]`). **Lösning:** ny `Client.ExecuteRaw`-metod som
dekoderar hela `data`-objektet utan operation-name-pickup. Användbar för
introspection och andra "naked"-queries där fältnamn inte matchar
operation-namnet. Phase 1-tester orörda, ingen API-bryt.

### Steg 2 — Parser ✅ KLAR

- [x] `tools/codegen/internal/schema/parse.go` — `Parse(cached)` → `ModelSchema`
  - Detekterar entiteter via Find*-queryroten (top-level)
  - Sub-entitets-detektion via heuristik: OBJECT med `<Name>Filter`-syskon men ingen `Find<Name>`
  - Relationships detekteras via `_fromId`/`_toId`-fältnamn (decisions.md #32)
  - Compound-create-info parsas från `Create{From}{Rel}{To}`-mutationsmönster (#33)
  - NON_NULL/LIST-wrappers flattnas till bools på `FieldRef`
  - Scalar-mappning: String/ID→string, Boolean→bool, Int→int, Float/Double→float64, Long/Decimal/Date*/Time*/DateTimeOffset→string (decisions.md #29)
  - Duplicerade typer dedupas på namn (decisions.md #31)
- [x] `tools/codegen/internal/schema/parse_test.go` — 7 tester mot live-cachen
  - top-level Customer-detektion + Create/Update/SearchFilter
  - relationship-detektion: `CustomerPlacedOrder` → from=Customer, to=Order, compound=`CustomerCustomerPlacedOrderOrderCreate`
  - sub-entiteter: `CustomerName`, `CustomerContact`, `CustomerAddress` upptäckta
  - scalar-mappning bekräftad
  - `order_by` enum innehåller exakt `[Ascending, Descending]`
  - filter-fält finns och inkluderar `and`/`or`/`not`-composers
  - sanity-counts: ≥5 nodes + ≥5 relationships
- [x] **Resultat:** 42 entiteter (24 nodes + 18 relationships) i CandyShopModel, 3 enums (`order_by`, `aggregationtype`, `resultlookup`), 4 scalars i bruk

### Steg 3 — Emit-moduler ✅ MVP KLAR (2026-05-01)

- [x] **DSL-runtime promoverad** till `pkg/dataceen/dsl/` (publik): `ast.go`, `serializer.go`, `builder.go`, `doc.go`. Egna 3 tester (`TestCanonicalQuery`, `TestEmptyFilterRendersNull`, `TestNotComposition`).
- [x] **`internal/prototypes/`** kvar som frusen regression-bädd; deras AST/serializer är duplikat av dsl, vilket är acceptabelt (de är låsta till en exakt wire-output).
- [x] `tools/codegen/internal/emit/emit.go` — gemensamma helpers (`FileHeader`, `GoFieldName`, `GoType`, `JSONTag`, `FormatGo`)
- [x] `tools/codegen/internal/emit/emit_model.go` — `<E>.go` (read-modell-struct)
- [x] `tools/codegen/internal/emit/emit_create.go` — `<E>Create.go` (input för Create-mutation; skip för relationer)
- [x] `tools/codegen/internal/emit/emit_fields.go` — `<E>Fields.go` (strategi A: bool-flaggor + pekar-på-nested) + `ToNodes()`-metod
- [x] `tools/codegen/internal/emit/emit_client.go` — `<E>Client.go` (Find + FindByID; skip för icke-top-level)
- [x] `tools/codegen/internal/emit/plan.go` — BFS från roots genom alla OBJECT/INPUT_OBJECT-fältreferenser
- [x] `tools/codegen/internal/emit/run.go` — orkestrering, wipe, file write
- [x] CLI emit-subcommand: `go run ./tools/codegen/cmd emit Customer`
- [x] **Resultat:** 39 entiteter upptäckta, 127 filer skrivna, 30 skippade (relations utan create, sub-entiteter utan client). Hela `pkg/generated/` kompilerar utan ändringar.

#### Designval i emit-pipen

- **`Meta`-prefix för underscore-fält:** `_id` → `MetaId`, `_createdAt` → `MetaCreatedAt`. Löser kollision med business-fält som `CreatedAt`. JSON-tag bevarar exakt server-namn (`json:"_id,omitempty"` etc.).
- **Sub-entiteter utanför emit-set degraderas till `any`:** rare i praktiken eftersom BFS når hela grafen från Customer, men gör det säkert att emit:a delar.
- **`*<E>Fields` på pekarefält** för att kunna skilja "select inga subfält" från "select alla subfält". `bool`-fallback för referenser utanför emit-set.
- **Strängbaserad template med `{{ENTITY}}`-substitution** för `emit_client.go` istället för giant `fmt.Fprintf` med 25 args (första försöket producerade `%!(EXTRA string=...)`-skräp och misslyckades gofmt-validering). Single-token replacement är trivialt att läsa och underhålla.

### Steg 4 — Validera mot live-backend ✅ KLAR (2026-05-01)

- [x] `pkg/generated/customer_integration_test.go` — 3 tester gated av `RUN_INTEGRATION=1`:
  - `Find(size: 5)` → 5 customers, första `cst-000001` (id=`Customer_Example_00000`)
  - `FindByID("Customer_Example_00000")` → `cst-000001`
  - `Find(filter: dsl.F("CustomerId").Like("cst"))` → 14 customers (matchar token "cst")

```
make test                   → alla unit-tester (cached) gröna
RUN_INTEGRATION=1 make test-integration  → 1 fas-1-test grön (samma som tidigare)
RUN_INTEGRATION=1 go test ./pkg/generated -run Integration  → 3 nya tester gröna mot live
```

### Återstående i Fas 3/Fas 5 (inte MVP)

- [ ] `<E>Update.go` med change-tracking — beslut om struct-design (set-flaggor vs map-inputs)
- [ ] `<E>Client.Search/Create/CreateBulk/Update/Delete/DeleteBulk`
- [ ] Compound-create för relationer (`Create{From}{Rel}{To}` med `_fromId`/`_toId`-injection)
- [ ] Aggregations-stöd (Phase 5 step 5 i TS-porten)
- [ ] `AllClient` som aggregerar alla top-level `<E>Client` (fas 5 step 2 i TS)

## Fas 4 — Subscriptions ✅ KLAR (2026-05-01)

### Filer

- [x] `pkg/dataceen/subscriptions/types.go` — proto-enum-strings (`StartMode`, `MessageType`, `EventType`, `OperationType`, `ObjectType`), `Request`, `LowLevelEvent`, `Event[T]`, `RawHandler`
- [x] `pkg/dataceen/subscriptions/subscription.go` — `Subscription` struct (id, request, mutex-skyddad state) + free-function `On[T]` (typed handler) + `OnRaw` (low-level)
- [x] `pkg/dataceen/subscriptions/grpc.go` — `dialSubscription` (TLS, host:port, token-metadata) + `streamClient`/`eventStream`-interfaces för testbarhet, `toProtoRequest`-konvertering
- [x] `pkg/dataceen/subscriptions/manager.go` — `Run(ctx, sub, opts) error`: open stream → readUntilDone → reconnect-loop med exponentiell backoff (cap 60s) → max 10 försök
- [x] `pkg/dataceen/subscribe.go` — `Client.CreateSubscription(req)` + `Client.Subscribe(ctx, sub)` med `loggerAdapter` mellan dataceen.Logger och subscriptions.Logger

### Tester

- [x] `pkg/dataceen/subscriptions/manager_test.go` — 6 unit-tester med fake stream:
  - typed handler dispatchas och får parsed `Complete[T]`
  - raw handler ser alla events oavsett topic
  - `KEEPALIVE`-meddelanden filtreras
  - baseload-state spåras (`isBaseLoading`/`baseloadLabel`/`lastPosition`)
  - handler-fel kraschar inte streamen
  - context-cancel avslutar rent mitt i strömmen
- [x] `pkg/dataceen/subscriptions/integration_test.go` — live-test gated av `RUN_INTEGRATION=1`:
  - 5 BASELOAD-events mottagna med `Complete.CustomerId="cst-000001"` … `cst-000005`
  - `CompletePresent: true` (json-payload korrekt parsad)
  - `lastPosition` matchar TS-portens observation (`01DCCE73F80DE6CF000000040001003A`)
  - clean shutdown via `cancel()` inifrån handlern

```
go test ./pkg/dataceen/subscriptions       → 6 PASS
RUN_INTEGRATION=1 go test ./pkg/dataceen/subscriptions -run Integration -v  → 1 PASS, 5 events i 0.94s
```

### API-yta

```go
client, _ := dataceen.NewClient(cfg)

sub := client.CreateSubscription(subscriptions.Request{
    Topics:          []string{"Customer", "Order"},
    BaseloadTopics:  []string{"Customer"},
    StartMode:       subscriptions.StartPositionBeginning,
    IncludeComplete: true,
})

// Typed handler — Complete är pre-parsed Customer
subscriptions.On(sub, "Customer", func(evt *subscriptions.Event[Customer]) error {
    switch evt.EventType {
    case subscriptions.EventBaseload:        // baseload-fas
    case subscriptions.EventSubscription:    // live event
    case subscriptions.EventBaseloadEnd:     // baseload klar
    case subscriptions.EventConnectionLost:  // syntetisk vid reconnect
    case subscriptions.EventConnectionReconnected:
    }
    return nil
})

// Low-level handler — råa fält + JSON-strängar
sub.OnRaw(func(evt *subscriptions.LowLevelEvent) error { ... })

// Blockerar tills ctx cancel eller fatal stream-fel
err := client.Subscribe(ctx, sub)
```

### Go-specifika fynd från fas 4

- **Generic methods finns inte i Go.** TS:s `sub.on<T>(topic, handler)` blir Go:s `subscriptions.On[T](sub, topic, handler)` (free function). Ergonomiskt acceptabelt; bara ett extra paketnamn-prefix.
- **Stream-cancellation via ctx propagation:** `context.WithCancel(ctx)` runt `client.Subscribe()` — när parent ctx cancellas, gRPC-streamen avbryts från `Recv()`. Ingen separat `Cancel()`-metod behövs.
- **Defensive handler-isolation:** varje handler-anrop wrappas i `safeCall` som recoverar paniks och konverterar till error. Ett handler-fel loggas men dödar inte streamen — matchar TS-portens beteende (`runWithTiming` med try/catch).
- **`streamClient`/`eventStream`-interfaces** möjliggör fake-stream i unit-tester utan att starta en echo-server. Real-implementationen är 50 rader, fake-implementationen 20 rader.
- **Baseload-state-uppdatering före dispatch** så handlers kan inspektera `sub.IsBaseLoading()` och se en konsistent vy. Mutex på `Subscription` håller läs/skriv-konkurrens säker.

## Fas 5 — Codegen-utbyggnad ✅ MVP KLAR (2026-05-01)

### Filer

- [x] `pkg/dataceen/dsl/input.go` — `EncodeInput(v) string` (reflection-baserad GraphQL input-syntax: unquoted keys, json-tag-medveten, omitempty, deterministisk map-iteration via key-sort)
- [x] `pkg/dataceen/dsl/input_test.go` — 9 tester (scalars, structs, pointers, nil-handling, nested struct, map, slice, omitempty)
- [x] `pkg/dataceen/dsl/serializer.go` — `RenderFilter(node) string` (publik filter-only renderer för mutation `where`-arg)
- [x] `pkg/dataceen/helpers.go` — `Ptr[T any](v T) *T` (generic pointer-helper för Update-fält)
- [x] `tools/codegen/internal/emit/emit_update.go` — `<E>Update.go` med pointer-only fält + omitempty
- [x] `tools/codegen/internal/emit/emit_client.go` utökad — `Create`, `Delete` (för noder med `<E>Create`-input), `Update` (när `<E>Update` finns)
- [x] `tools/codegen/internal/emit/emit_all_client.go` — `AllClient.go` med en `*<E>Client` per top-level entitet (27 stycken för CandyShopModel)
- [x] `tools/codegen/internal/emit/run.go` utökad — kör Update + AllClient-emitter

### Resultat

```
go run ./tools/codegen/cmd emit Customer
  → 39 entiteter, 167 filer (var av en AllClient.go)
go build ./pkg/generated   → kompilerar
go vet ./...               → ren
```

### Återstår (icke-blocking — kan lösas i fas 5b eller senare)

- [ ] `Search` på `<E>Client` (med aggregations + resultLookup)
- [ ] `CreateBulk` / `UpdateBulk`
- [ ] Compound-create för relationer (`Create{From}{Rel}{To}` + `_fromId`/`_toId`-injection)
- [ ] Aggregations-fragmenter (Phase 5 step 5 i TS-porten)

## Fas 6 — Exempel-app ✅ KLAR (2026-05-01)

### Filer

- [x] `examples/candyshop/main.go` — orkestrator med `--scenario`-flagga och 2 min övergripande timeout
- [x] `examples/candyshop/config.go` — `.env`-loader (söker i flera nivåer)
- [x] `examples/candyshop/find.go` — 5 read-scenarier (Find / FindByID / Like / OrderBy / Pagination)
- [x] `examples/candyshop/subscribe.go` — gRPC-subscription, max 5 events, ren cancel
- [x] `examples/candyshop/destructive.go` — Create → verify → Update → verify → Delete (deferred cleanup)
- [x] `examples/candyshop/README.md` — körinstruktioner

### Verifierat mot live-backend (2026-05-01)

```
go run ./examples/candyshop
  → 6 read-scenarier gröna
RUN_DESTRUCTIVE=1 go run ./examples/candyshop
  → CREATE: ResponseCode=0, Result=[ca17b951-...]
  → UPDATE: IsActive flippad från true → false, ExternalReference uppdaterad
  → DELETE: ResponseCode=0 (cleanup)
  → "All scenarios passed."
```

### Make-targets

```
make example                  # read-only
make example-destructive      # full CRUD med cleanup
```

## Fas 5b — Compound-create för relationer ✅ KLAR (2026-05-01)

- [x] `tools/codegen/internal/emit/emit_relation_create.go` — emit `<Rel>RelationCreate.go` med edge-metadata (`_fromId/_toId/_fromLabel/_toLabel/_label`) plus introspection-advertised business-fält
- [x] `emit_client.go` utökad: relationships får `Create(input)` + `Delete(filter)`; nodes oförändrade
- [x] `Create`-metoden defaultar `_fromLabel/_toLabel/_label` till entitetsnamn om callern inte sätter dem (server kräver dem trots att introspection inte advertiserar)
- [x] **18 RelationCreate.go-filer** + 18 relations-clients regenererade
- [x] **Verifierat mot live:** `examples/candyshop/destructive.go` skapar en `CustomerPlacedOrder`-relation mellan ett nyskapat Customer och ett befintligt Order, ResponseCode=0, cleanup OK

```
api.CustomerPlacedOrder.Create(ctx, &generated.CustomerPlacedOrderRelationCreate{
    FromID:   customerID,
    ToID:     orderID,
    PlacedAt: time.Now().UTC().Format(time.RFC3339),
})
```

## Fas 7 — Paketering & release ✅ KLAR (2026-05-01)

- [x] **Top-level README** omskrivet — quickstart, full layout, API-exempel, Make-targets, codegen-instruktioner, "vad återstår"-sektion
- [x] **GitHub Actions** workflow (`.github/workflows/ci.yml`):
  - build + vet + test på Linux/macOS/Windows
  - golangci-lint job
  - integration-tester på master när `DATACEEN_CLIENT_SECRET` finns som secret (skip:as snyggt om den saknas)
- [x] **golangci-lint config** (`.golangci.yml`) — high-signal linters, ignorerar protoc-genererat och relevanta `revive`-regler för wire-format-trogna fältnamn
- [x] **`*.exe` tillagt i .gitignore** (städade leftover binär)
- [x] **`git init` + initial commit** av 77 filer (exklusive `.env`, `pkg/generated/`, `tools/codegen/cache/`)
- [x] **`git tag -a v0.1.0`** med release notes; commit `899d6ed`

## v0.2.0 — Full feature-paritet ✅ KLAR (2026-05-01)

### Filer

- [x] `tools/codegen/internal/emit/emit_client.go` utökad — `bulkNodeTemplate` (CreateBulk + UpdateBulk för noder), `bulkRelationTemplate` (CreateBulk för relationer med default-label-injection), `searchTemplate` (Search-metod med ResultLookup, Aggregations, AggregationDepth)
- [x] `tools/codegen/internal/emit/emit_aggregation.go` — emitterar `Search<E>Aggregation` (med en pekare per scalar-fält till `Search<E>AggregationDetails` + nested-aggregation-pekare för sub-entities) + `Search<E>AggregationDetails` (Name/Size/Type/Aggregations)
- [x] `tools/codegen/internal/schema/parse.go` utökad — populerar `EntityInfo.Aggregation` och `EntityInfo.AggregationDetails` från typeMap
- [x] `pkg/dataceen/dsl/aggregations.go` (ny) — `AggregationFragment(depth)`, `Aggregation`/`Bucket` result-typer, `DecodeAggregations(rawList)`, `Bucket.KeyAsString()`/`KeyAsBool()` (med fallback för server-strängformade booleans)
- [x] `pkg/dataceen/dsl/serializer.go` — `RenderFields`, `RenderOrderBy` exporterade så searchTemplate kan bygga query-strängen utan PlanToGraphQL
- [x] `examples/candyshop/search.go` — Section 8 med Search + Bucket_Terms-aggregation
- [x] `examples/candyshop/bulk.go` — Section 7f/7g med CreateBulk + UpdateBulk + cleanup via Delete-by-filter

### Verifierat mot live (2026-05-01)

```
$env:RUN_DESTRUCTIVE=1; go run ./examples/candyshop
  → 7a Create + 7b verify + 7c Update + 7d cleanup    ResponseCode=0 alla
  → 7e CompoundCreate Customer↔Order + cleanup        ResponseCode=0
  → 7f CreateBulk(3) + UpdateBulk(2)                  3+2 rader påverkade
  → 7g Bulk cleanup via Delete-by-filter              3 rader raderade
  → 8  Search(filter=like "cst") med Bucket_Terms IsActive
       → 5 Items + 1 Aggregation, IsActive=true:9, false:5 (totalt 14)
```

### Generated kod-storlek

```
197 filer i pkg/generated/  (39 entiteter × ~5 filer/entitet)
- 39× <E>.go              read-modell
- 27× <E>Client.go        typade wrappers  (top-level entiteter)
- 18× <E>RelationCreate.go relationship compound input
- 39× <E>Fields.go         strategi-A field-selection
- 39× <E>Update.go         pointer-only updates (med MetaId för UpdateBulk-routing)
- ~27× Search<E>Aggregation.go  typed aggregation input (där HasSearch)
- AllClient.go             aggregator
```

### Återstår (verkligen icke-blocking nu)

- [ ] `<Entity>Update` med change-tracking via accessor-metoder (eventuellt — pointer-fält täcker samma use-case)
- [ ] DeleteBulk om servern någonsin exponerar det (skemat har inget `Delete<E>Bulk` idag)

Hela TS/C#-portarnas core-feature-set är nu klar i Go.

## Anteckningar mellan sessioner

### Insikter från andra portarna som måste bevakas i Go

- **Query-namn:** `FindCustomer`, INTE `FindCustomerNodes`. C#-metodnamnet är vilseledande.
- **Operator-case:** lowercase (`eq`, `like`, `and`, `or`). TS-portens prototyper hade UPPERCASE och var fel — Go-generatorn ska aldrig ha den buggen.
- **OrderBy-enum:** `Ascending` / `Descending`, INTE `"ASC"/"DESC"`. (TS-portens prototyp var fel.)
- **`like`-semantik:** ES `match`-token-baserad, inte SQL-LIKE. Aldrig wildcards eller delimiters. Dokumentera i Go-doc på filter-typerna när generatorn emittar dem.
- **`_fromId/_toId/_fromLabel/_toLabel`:** introspection advertiserar dem inte på relations-create-typer, men servern accepterar dem. Inkludera i emitter-output.
- **Aggregations:** introspection advertiserar dem inte på `SearchResult` heller. Emittern bygger dem ändå.
- **Sub-entiteter (CustomerName, CustomerContact, …):** har `<Name>Filter`-syskon men ingen `Find<Name>`-query. BFS-emitter måste hitta dem.

### Go-specifika observationer från fas 0

(Fyll i efter att spikar körts.)
