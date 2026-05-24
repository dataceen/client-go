# Plan: Dataceen-klient i Go

Tredje paralleliserade port (efter C# → TS). Den auktoritativa
implementationen är fortfarande C#-klienten på `C:\Source\Dataceen\DataceenClient\`
(NuGet v1.1.33). TS-porten på `C:\Source\Dataceen\DataceenClientTs\` är
verifierad mot live-backend och har feature-paritet.

Den här klienten samexisterar med båda — ersätter ingen.

## Förutsättningar (beslutade defaults)

| Fråga | Beslut |
|---|---|
| Målplattform | Go 1.22+ (bygger på alla större OS) |
| Modulpath | `github.com/dataceen/client-go` (provisionellt — bytas vid publish) |
| Test-ramverk | `testing` (stdlib) + `testify` om assertions blir besvärliga |
| gRPC | `google.golang.org/grpc` + `google.golang.org/protobuf` (statisk codegen via `protoc`) |
| HTTP | `net/http` (stdlib) |
| Auth | `github.com/AzureAD/microsoft-authentication-library-for-go` (MSAL Go) |
| .env | `github.com/joho/godotenv` (bara för spikes/exempel; biblioteket självt läser inte .env) |
| Repo-struktur | Standard Go-layout: `cmd/`, `pkg/`, `internal/`, `tools/` |
| Försörjningssemantik | Semver från 0.1.0, första stabil vid 1.0 |
| Generatorns input | GraphQL introspection (samma som TS-porten — `decisions.md` #23) |
| Relation till TS/C# | Parallell, inte ersättande |
| Autentisering | Client-credentials via MSAL, samma som C#/TS |

## Komponentmappning C#/TS → Go

| C#/TS-komponent | Go-motsvarighet | Svårighet |
|---|---|---|
| `DataceenClient` (GraphQL över HTTP) | `dataceen.Client` med `net/http` | Lätt |
| `DataceenClient` subscriptions (gRPC) | `pb.NewDataceenEventClient` + `grpc.NewClient` | Lätt (proto är litet) |
| `TokenProvider` (MSAL) | `confidential.New` + `AcquireTokenByCredential` | Lätt |
| `FilterBuilder<T>`, `FieldsBuilder<T>` | **Plain structs / map[string]any** (ingen proxy-magi) — se fas 2 | **Medel** |
| `ExpressionResolver` | Ej tillämpligt — Go saknar runtime-AST. Generatorn emitterar typer + bygg-helpers | – |
| `FilterDefinition`, `FieldsDefinition` | Plain struct-typer | Lätt |
| `CustomerUpdate` med ändringsspårning | Struct + bitmask eller `map[string]any` av "set"-fält. Inget reflection-baserat diff | Medel |
| `FindResult<T>`, `MutationResult` | Generic struct-typer (Go 1.18+ generics) | Trivialt |
| Genererad `AllClient` | Ny generator som emit:ar `.go` | Stort men rakframt |
| `EventHandlerList` | `map[string]func(ctx, evt) error` per topic | Lätt |
| `PostGraphQLAsync` (retry/auth) | `httpTransport` med `context.Context` | Lätt |
| DI / lifetime | Konstruktorer + functional options. Ingen DI-container | Stildiskussion |

## Den enda riktiga svårigheten: DSL för filter/fields

Go har **ingen** av de mekanismer C#/TS använder:
- Inga LINQ-expressions eller AST-inspektion
- Ingen `Proxy` / `__getattr__` — strukturer är statiska
- Generics finns men har inga "indexed-access types" (`T[K]`)

Tre realistiska strategier för Go:

**A — typade structs + `omitempty`:** generera `CustomerFilter` med valfria pekarefält per scalar och `*StringComparisonExp` etc. Idiomatiskt; verbose men typsäkert.

**B — map[string]any:** användaren skriver `map[string]any{"CustomerId": map[string]any{"eq": "x"}}`. Trivial att serialisera men kastar bort all typsäkerhet.

**C — typade builders:** `dataceen.Filter().Customer().CustomerId().Eq("x").And(...).Build()`. Kan bli läsbart men lång kedja.

**Default-rekommendation:** strategi A för filter/orderBy (Go-idiomatisk struct-zero-value), strategi B med en handskriven helper för fields-selection (där struct-uttryck blir absurt verbose).

Slutligt beslut fattas i fas 2 efter prototypning. Se `dsl-comparison.md`.

## Fasplan

### Fas 0 — Spike (1–2 dagar)
Bevisa att de riskfyllda delarna fungerar.

**Filer:**
```
cmd/spike01-token/main.go         # MSAL → Bearer
cmd/spike02-graphql/main.go       # POST /graphql + FindCustomer
cmd/spike03-subscription/main.go  # gRPC stream, första 10 events
cmd/spike04-introspection/main.go # __schema-probe
internal/spikecfg/cfg.go          # gemensam .env-loader
```

**Verifiera:** Alla fyra skript kör mot live-backend (samma Azure AD-credentials som C# och TS använder).

### Fas 1 — Kärn-runtime (3–5 dagar)
Generisk `dataceen.Client` utan typgenerering.

**Filer:**
```
pkg/dataceen/
  client.go         // DataceenClient
  config.go         // Config-struct + functional options
  errors.go         // DataceenError (ResponseCode, SourceQuery)
  http_transport.go // PostGraphQL: 401-refresh + transient retry
  logger.go         // Logger-interface (default = log.Default)
  token_provider.go // MSAL-wrapper med ForceRefresh
  types.go          // FindResult[T], SearchResult[T], MutationResult, ...
```

**API-form:**
```go
client, err := dataceen.NewClient(dataceen.Config{...},
    dataceen.WithLogger(myLogger),
    dataceen.WithMaxTransientRetries(3))

var res dataceen.FindResult[Customer]
err = client.ExecuteQuery(ctx, dataceen.GraphQL{
    OperationName: "FindCustomer",
    Query:         `query { FindCustomer(...) { Items { _id } Cursor } }`,
}, &res)
```

Porta retry/auth-logiken från C#:s `PostGraphQLAsync`. Mutations: ingen retry per default.

### Fas 2 — DSL-prototyper + beslut (2–3 dagar)

Bygg två-tre prototyper (A/B/C ovan) som producerar samma GraphQL för samma
use-case. Utvärdera på:
1. Typsäkerhet
2. Felmeddelanden
3. Verbosity för komplexa AND/OR + djup field-selection
4. Serialisering till GraphQL
5. Läsbarhet i kallplats

**Leverans:** `docs/dsl-comparison.md` med beslut. Behåll prototypkoden i `internal/prototypes/` som referens (samma policy som TS-porten — `decisions.md` #21).

### Fas 3 — Kodgenerator v1 (1–2 veckor)

Generera typade filer för en enda entitet (Customer).

```
tools/codegen/
  main.go         // CLI
  schema.go       // fetch-schema (cachas i tools/codegen/cache/)
  parse.go        // schema → ModelSchema
  emit_model.go   // -> Customer.go
  emit_filter.go  // -> CustomerFilter.go
  emit_create.go  // -> CustomerCreate.go
  emit_update.go  // -> CustomerUpdate.go
  emit_fields.go  // -> CustomerFields.go (helper för field-selection)
  emit_client.go  // -> CustomerClient.go (typade wrappers)
```

Output: `pkg/generated/<TypeName>.go` (gitignored — regenereras).

**Använd schema-fynden från TS-portens `decisions.md` #26–47** verbatim — de är
serveregenskaper, inte språkberoende.

### Fas 4 — Subscriptions (3–5 dagar)
gRPC-baserad subscription-klient med baseload-semantik.

```
pkg/dataceen/subscriptions/
  subscription.go      // Subscription-handle, registrerade handlers
  manager.go           // SubscriptionManager
  dispatcher.go        // event → typade + low-level handlers
  grpc_client.go       // Wrapper kring pb.NewDataceenEventClient
  types.go             // SubscriptionEvent[T], LowLevelEvent
```

**Måste implementeras:**
- Baseload-semantik: BASELOAD_EVENT → BASELOAD_STEP_1 → BASELOAD_END
- CONNECTION_LOST / CONNECTION_RECONNECTED som syntetiska events
- Reconnect med exponentiell backoff (max 10 försök, samma som TS-porten)
- Slow-handler timing (default 500 ms — porta `SlowHandlerThresholdMs`)
- Central panic-recover så en handler-panik inte kraschar streamen
- Token-refresh på reconnect

**Cancellation:** `context.Context` — varje subscription kopplas till ctx, `cancel()` stänger streamen rent.

### Fas 5 — Full modell (3–5 dagar)
Alla 9 noder + 18 relationer + ~12 sub-entiteter för CandyShopModel.

Samma BFS-strategi som TS-porten: emitter följer alla typreferenser, inklusive
sub-entiteter (typer som har en `<Name>Filter`-syskon men ingen `Find<Name>`-query).

Lägg till compound-create för relationer (`CreateCustomerCustomerPlacedOrderOrder`)
med `_fromId`/`_toId` i input fast introspection inte advertiserar dem
(TS-porten hade samma hack — `progress.md` Fas 5 steg 3).

### Fas 6 — Porta exempel-appen (1 vecka)
Go-motsvarighet till `Thomas_CandyShopModel_All_Example`.

```
examples/candyshop/
  main.go
  find/main.go, create/main.go, update/main.go, delete/main.go
  search/main.go         // search + aggregations
  subscribe/main.go      // subscription med 2 topics
  handlers/customer.go, customer_placed_order.go
```

Kör destructive-exempel bara om `RUN_DESTRUCTIVE=1` (samma policy som TS).

### Fas 7 — Paketering & release (3–5 dagar)
- `go.mod`-tagg `v0.1.0`
- README, migrating-from-csharp.md, dsl-reference.md
- CI: GitHub Actions (build + test + go vet + golangci-lint)
- Go-modulen publiceras genom git-tag

## Tidslinje

```
Vecka 1: Fas 0 + Fas 1
Vecka 2: Fas 2 + start Fas 3
Vecka 3: Fas 3
Vecka 4: Fas 3 + Fas 4
Vecka 5: Fas 4 + Fas 5
Vecka 6: Fas 5 + Fas 6
Vecka 7: Fas 6
Vecka 8: Fas 7
```

**Total:** 8 veckor för en utvecklare, buffra 20 % = ~10 veckor (samma rytm som TS-porten).

## Beroenden

```
Fas 0 → Fas 1
Fas 1 → Fas 2
Fas 2 → Fas 3       (DSL-beslut styr generatorn)
Fas 3 → Fas 5
Fas 1 → Fas 4       (parallellt med fas 2+3)
Fas 4 + Fas 5 → Fas 6
Fas 6 → Fas 7
```

## Kritiska beslutpunkter

| # | Beslut | Deadline | Default |
|---|---|---|---|
| 1 | Parallell eller ersättande klient? | ✅ Fas 0 | Parallell |
| 2 | DSL-strategi (A/B/C) | Fas 2 slut | A för filter, B+helper för fields |
| 3 | Generator-output: `pkg/generated/` eller `pkg/<entity>/`? | Fas 3 | Flat `pkg/generated/` (TS-portens fas 5 visade att flat slår nested) |
| 4 | Hur mappa `Long`/`Decimal`-scalars? | Fas 1 | `string` (precision-säkert), exakt som TS-porten |
| 5 | Hur mappa `DateTimeOffset`? | Fas 1 | `string` (ISO) — wire-fidelity, ingen automatisk `time.Time`-konvertering |
| 6 | Feature-paritet vs MVP | Fas 5 | Full paritet med C#/TS |
| 7 | Exempel i samma module eller eget? | Fas 6 | Samma module |
| 8 | Fler auth-varianter? | Fas 1 | Client-credentials only |

## Stoppkriterier

Pausa/omvärdera vid:
- Fas 0 misslyckas med Azure AD eller gRPC → infrastrukturproblem, fixa innan start
- Fas 2 visar att ingen DSL-strategi har acceptabel UX → överväg bara `map[string]any` + helpers
- Fas 5 avslöjar fel antagande om relationer i fas 2

## Referensfiler i andra portarna

- `C:\Source\Dataceen\DataceenClient\` — auktoritativ C#-klient
- `C:\Source\Dataceen\DataceenClient\DataceenClient.cs` — huvudklient (~2700 rader)
- `C:\Source\Dataceen\DataceenClient\TokenProvider\TokenProvider.cs`
- `C:\Source\Dataceen\DataceenClientTs\` — TS-porten med samma fasplan, alla decisions dokumenterade
- `C:\Source\Dataceen\DataceenClientTs\docs\decisions.md` #26–47 — schema-fakta som gäller också för Go-porten
- `C:\Source\Dataceen\DataceenClientTs\docs\porting-guide.md` — meta-guide för nya språkporter
- `C:\Source\Dataceen\DataceenEvent\Protos\dataceenevent.proto` — gRPC-kontrakt (kopierad till `protos/`)
- `C:\test\CSharp\Thomas_CandyShopModel_All_Example\appsettings.json` — Azure AD-credentials
