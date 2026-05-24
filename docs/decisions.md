# Beslutade defaults — Go-porten

Loggar Go-specifika beslut. Server-imposed beslut (operator-case, query-namn,
PascalCase-respons, scalar-mappning, like-semantik, …) ärvs från TS-portens
`decisions.md` #26–47 och dokumenteras inte om igen här.

## 2026-04-30 — Projektstart

| # | Beslut | Motivering |
|---|---|---|
| 1 | Parallell klient (inte ersättande) | C# v1.1.33 + TS-porten har båda etablerade konsumenter. Go-versionen ska få växa utan press att ersätta något. |
| 2 | Modulpath: `github.com/dataceen/client-go` (provisionellt) | Standard Go-konvention; bytas vid publish. |
| 3 | Go 1.22+ som baseline | Generics (1.18) krävs för `FindResult[T]`. 1.22 ger `range` över heltal och stabilare loopvariabler. |
| 4 | gRPC via `google.golang.org/grpc` + statisk `protoc`-codegen | Go saknar TS:s `@grpc/proto-loader`-ekvivalent. `dynamicpb` finns men idiomatisk Go är statisk gen. |
| 5 | HTTP via `net/http` (stdlib) | Räcker. Ingen retry-bibliotek — vi skriver explicit transient-retry själva (samma som TS, beslut #10 där). |
| 6 | Auth via MSAL Go (`github.com/AzureAD/microsoft-authentication-library-for-go`) | Officiellt Microsoft-SDK. `azidentity` är ett alternativ men ger mindre kontroll över cache/refresh. MSAL matchar exakt vad C# och TS-porten använder. |
| 7 | `.env` via `godotenv` — bara för spikes och exempel | Biblioteket självt ska aldrig läsa `.env`. Konsumenter passerar `Config`-struct. |
| 8 | Test: stdlib `testing` | `testify` läggs till bara om assertions blir besvärliga. |
| 9 | Repo-layout: `cmd/` (binaries) + `pkg/` (publik API) + `internal/` (delning mellan binaries) + `tools/codegen/` (generatorn) | Go-konvention. Mirror av TS-portens `src/spike/`, `src/client/`, `tools/codegen/`. |
| 10 | Client-credentials-only för auth | Samma som C#/TS. Fler flows om behov uppstår. |

## Öppna frågor

- DSL-strategi (fas 2): A (typed structs) för filter/orderBy är default; fields-selection är osäkert — typed builder eller raw `[]string`?
- `Long`/`Decimal`-scalars: confirmed `string` per TS-porten, men ska vi exponera ett `dataceen/scalars`-paket med helpers (`scalars.LongFromInt64`)?
- Field-tagga JSON-fält enligt server-PascalCase eller mappa till Go-konventionellt CamelCase via custom `MarshalJSON`?

## 2026-04-30 — Förvalda val som ärvs verbatim från TS-porten

Följande är *inte* nya beslut — de är server-egenskaper. Listas här bara så
en läsare av enbart Go-dokumentationen ser hela bilden.

| # | Ärvd från TS | Sammanfattning |
|---|---|---|
| 11 | TS #11 | GraphQL query-namn är `Find{TypeName}`, INTE `Find{TypeName}Nodes`. |
| 12 | TS #12, #13 | PascalCase i wire-respons (`Items`, `Cursor`, `_id`, `CustomerId`) — Go-typer matchar via `json:"Items"` etc. |
| 13 | TS #14 | `DataceenError` centraliserar fel: `ResponseCode`, `SourceQuery`, wrap:ar HTTP/GraphQL/parse. |
| 14 | TS #16 | `ExecuteQuery` retry=true (idempotent), `ExecuteMutation` retry=false (ej idempotent). |
| 15 | TS #23 | Schemakälla för generatorn: GraphQL introspection. |
| 16 | TS #25 | `DateTimeOffset` → Go `string` (ISO). Wire-fidelity. Konsumenter konverterar själva om de vill ha `time.Time`. |
| 17 | TS #26 | Filter-operatorer är lowercase: `eq, neq, gt, gte, lt, lte, like, nlike, contains, ncontains, in, nin, is_null, exists`. |
| 18 | TS #27 | Filter-composition är lowercase: `and, or, not`. |
| 19 | TS #29 | Scalars: `Boolean→bool, Int/Float/Double→numeric, Long/Decimal→string, Date/DateTime/DateTimeOffset/Time/TimeSpan→string`. |
| 20 | TS #30 | Enum-värden är PascalCase_Snake: `Ascending`, `Descending`, `Bucket_Terms`, `Full`, `WithinIndex`. |
| 21 | TS #31 | Duplicerade typer i introspection är server-quirk. Dedupera på namn. |
| 22 | TS #32 | Relationer detekteras via `_fromId`/`_toId`-fält, inte typ-suffix. |
| 23 | TS #33 | `Filter` exkluderar relationer; `SearchFilter` inkluderar dem via dedicated `<Entity><Rel><Target>SearchFilter`. |
| 24 | TS #34 | Query.`Find*` signatur: `(size, cursor, where, order_by) → <Entity>FindResult`. |
| 25 | TS #36 | Mutationer returnerar generisk `Response { Result: [string], ResponseTime: int }`. |
| 26 | TS #37 | Per entitet: `Create, CreateBulk, Update, UpdateBulk, Delete`. |
| 27 | TS #38 | Mutation-arg är *entitetens namn*, inte "input" (`Customer: CustomerCreate!`, `CustomerList: [CustomerCreate]!`). |
| 28 | TS #39–43 | `like` är Elasticsearch `match` (token-baserad). Aldrig wildcards (`%`, `*`); aldrig delimiters (`-`, ` `, `.`). `contains` är exakt substring. Dokumentera i Go via package-doc + Go-doc-kommentarer på relevanta typer. |
| 29 | TS #44–47 | Aggregations finns men advertiseras inte i `<Entity>SearchResult`-introspection — emittern måste lägga till dem ändå. Default fragment-djup 3. |

## 2026-04-30 — Go-specifika designval (preliminära, fas 0/1)

| # | Beslut | Motivering |
|---|---|---|
| 30 | Functional options för Client-konstruktorn | Idiomatiskt Go: `NewClient(cfg, WithLogger(l), WithMaxTransientRetries(3))`. Lättare att utöka än struct-options utan att bryta API:t. |
| 31 | `context.Context` är förstaargumentet på alla API-anrop | Standard Go-mönster. Cancellation, deadlines, request-scoped logging. |
| 32 | Subscription-cancellation via `ctx.Done()` | TS använder en `cancel()`-metod; Go-idiomatisk är att stoppa via `context.CancelFunc`. |
| 33 | `Logger` är ett interface med `Info`/`Warn`/`Error` | Default = `log.Default()`-wrapper. Konsumenter kan adapta zap, slog, logrus med ~10 raders shim. |
| 34 | Resultat-typer behåller PascalCase i JSON-tagar (`json:"Items"` etc.) | Wire-format-fidelity. Användaren ser PascalCase i Go-strukturer också (`result.Items`) — matchar både C# och TS. |
| 35 | Update-tracking: per-fält `set_<Field>: bool`-struct + accessor-metoder | Generatorn emitterar accessors som sätter både värdet och flaggan. Inget reflection. Alternativ: `map[string]any` + manuella sätt-helpers. Beslut tas slutligt i fas 3. |

## 2026-05-01 — Efter fas 2 (DSL-val)

| # | Beslut | Motivering |
|---|---|---|
| 36 | **Strategi C (fluent builders) för filter + orderBy** | Bästa kompositions-ergonomi och minst genererad kod. `c.And(c.F("X").Like("y"), c.Or(...))`. Fältnamn är strings — accepterad tradeoff. Se `dsl-comparison.md`. |
| 37 | **Strategi A (typed struct) för fields** | Compile-time säkerhet vid djupa fältval (`f.CustomerPlacedOrder.Order.OrderID = true`). Generator emit:ar `<Entity>Fields`-struct + en converter per entitet (~30 rader var). |
| 38 | **Lowercase operatorer + PascalCase enum-värden från dag ett** | TS-portens prototyper hade UPPERCASE och rättades först i fas 3 — Go-porten skippar den vägen. Verifierat via `prototypes_test.go`. |
| 39 | **Prototypkod behålls i `internal/prototypes/`** | Permanent regression-testbädd. `prototypes_test.go` låser den kanoniska wire-outputen byte-exakt. Generatorn (fas 3) ska producera kod som matchar. |
| 40 | **Per-entitet field-konstanter (fas 5+)** | Mildra strategi C:s stringly-typed fältnamn genom `customer.CustomerIDField = "CustomerId"`-konstanter. Användare som vill ha autocomplete kan skriva `c.F(customer.CustomerIDField)`. Beslut bekräftas när användarna provkört API:t. |

## Beslut flyttade framåt

(inga aktiva)
