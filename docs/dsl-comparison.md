# DSL-jämförelse — fas 2 (Go-porten)

**Datum:** 2026-05-01
**Syfte:** Välj DSL-strategi för filter, fields, orderBy. Valet styr generatorn (fas 3).

Alla tre prototyper ligger under `internal/prototypes/{a,b,c}` och producerar
**identisk** GraphQL-output för samma use-case (verifierat i
`prototypes_test.go` — 5 tester, varav `TestAllThreeProduceIdenticalQuery`
är den hårda regressionsspärren).

Skiljer sig från TS-portens prototyper: vi använder **lowercase
operatorer** (`eq`, `like`, `and`, `or`) och **PascalCase enum-värden**
(`Ascending`, `Descending`) från dag ett — TS-prototyperna hade UPPERCASE
och rättades först i fas 3 (TS decisions.md #26–30). Go-porten skippar
den vägen.

---

## Use-case

Hämta 10 kunder vars `CustomerId like "cst"` OCH vars status är antingen
`Active` eller `Premium`. Returnera `_id`, `CustomerId`,
`ExternalReference` plus nästlad relation `CustomerPlacedOrder → Order →
OrderId`. Sortera på `CustomerId Ascending, ExternalReference Descending`.

Kanonisk wire-output (byte-exakt, låst i testet):

```
query {FindCustomer(size: 10, cursor: "abc123", where: { and: [{ CustomerId: { like: "cst" } }, { or: [{ Status: { eq: "Active" } }, { Status: { eq: "Premium" } }] }] }, order_by: [{ CustomerId: Ascending }, { ExternalReference: Descending }]) { Items { _id CustomerId ExternalReference CustomerPlacedOrder { Order { OrderId } } } Cursor }}
```

---

## Samma kod i tre varianter

### A — Typade structs (`omitempty` + pointer fields)

```go
a.FindCustomerInput{
    Size:   10,
    Cursor: "abc123",
    Filter: &a.CustomerFilter{
        And: []a.CustomerFilter{
            {CustomerID: &a.StringComparisonExp{Like: a.Strp("cst")}},
            {Or: []a.CustomerFilter{
                {Status: &a.StringComparisonExp{Eq: a.Strp("Active")}},
                {Status: &a.StringComparisonExp{Eq: a.Strp("Premium")}},
            }},
        },
    },
    Fields: a.CustomerFields{
        ID: true, CustomerID: true, ExternalReference: true,
        CustomerPlacedOrder: &a.CustomerPlacedOrderFields{
            Order: &a.OrderFields{OrderID: true},
        },
    },
    OrderBy: []a.CustomerOrderBy{
        {CustomerID: a.Ascending},
        {ExternalReference: a.Descending},
    },
}
```

### B — `map[string]any` + dot-path fields

```go
b.FindCustomerInput{
    Entity: "Customer",
    Size:   10,
    Cursor: "abc123",
    Filter: b.M{
        "and": []any{
            b.M{"CustomerId": b.M{"like": "cst"}},
            b.M{"or": []any{
                b.M{"Status": b.M{"eq": "Active"}},
                b.M{"Status": b.M{"eq": "Premium"}},
            }},
        },
    },
    Fields: []string{
        "_id", "CustomerId", "ExternalReference",
        "CustomerPlacedOrder.Order.OrderId",
    },
    OrderBy: []b.M{
        {"CustomerId": "Ascending"},
        {"ExternalReference": "Descending"},
    },
}
```

### C — Fluent builders

```go
c.FindInput{
    Entity: "Customer",
    Size:   10,
    Cursor: "abc123",
    Filter: c.And(
        c.F("CustomerId").Like("cst"),
        c.Or(
            c.F("Status").Eq("Active"),
            c.F("Status").Eq("Premium"),
        ),
    ),
    Fields: c.Fields().
        Pick("_id", "CustomerId", "ExternalReference").
        Nested("CustomerPlacedOrder", func(r *c.FieldsBuilder) {
            r.Nested("Order", func(o *c.FieldsBuilder) {
                o.Pick("OrderId")
            })
        }),
    OrderBy: c.OrderBy().Asc("CustomerId").Desc("ExternalReference"),
}
```

---

## Utvärdering

### 1. Typsäkerhet

| | A | B | C |
|---|---|---|---|
| Filter — fältnamn | ✅ Compile-error på tryckfel | ❌ Tryckfel kompilerar; faller på server | ⚠️ Stringly-typed (`F("CustomrId")`) men kan kompletteras med konstanter |
| Filter — operator | ✅ Per-typ-wrapper (`StringComparisonExp` har inte `In: []bool`) | ❌ Inget | ✅ Metoder är fasta (`F(x).Like(...)` är `string`-only) |
| Fields | ✅ Bara faktiska fält (`f.CustomerPlacedOrder.Order.OrderID`) | ❌ Strängar | ⚠️ Strängar (men metoder schemalägger när nesting korrekt) |
| OrderBy | ✅ Bara `Ascending`/`Descending` | ⚠️ Strängar | ✅ Bara `Asc()`/`Desc()` |

**A vinner stort.** B är "all stringly-typed". C ligger emellan — stark
operator-säkerhet, svag fältnamns-säkerhet.

### 2. Verbosity

Filter-uttrycket (LIKE AND (EQ OR EQ)):

| | A | B | C |
|---|---|---|---|
| Rader | 9 | 9 | 7 |
| Tecken | 351 | 245 | 156 |
| Pekare-syntax (`*Strp("...")`) | ja | nej | nej |

**C vinner i verbosity.** A:s pointer-everywhere-mönster är priset för
typsäkerhet i Go. Hjälpfunktioner (`Strp`, `Boolp`) är nödvändiga.

### 3. Genererad kod-storlek (fas 3)

| | A | B | C |
|---|---|---|---|
| Per entitet | `<E>Filter`, `<E>Fields`, `<E>OrderBy`-structs + converters (~200 rader för Customer) | inget | inget |
| Delas | ingen | builder-runtime (~100 rader, redan klar) | builder-runtime (~150 rader, redan klar) |
| Total för 9 noder + 18 relationer + 12 sub-entiteter (~39 entiteter) | ~7800 rader | ~100 rader | ~150 rader |

**B och C vinner kraftigt** på generator-storlek. A:s emit-pass behöver
emit-converter per entitet.

### 4. Felmeddelanden vid tryckfel

**A:** `&a.CustomerFilter{CustomrID: ...}` →

```
unknown field CustomrID in struct literal of type a.CustomerFilter
```

Klockren.

**B:** Ingen kompilatorhjälp. Server returnerar GraphQL-error vid runtime.

**C:** `c.F("CustomrId").Like(...)` kompilerar utan klagomål. Server
returnerar GraphQL-error vid runtime — samma situation som B i praktiken.

**A vinner ensam.** B och C hittar fel först server-side.

### 5. Läsbarhet (subjektivt)

- **A:** Plain data, lätt att läsa och diffa, men `&a.X{Y: a.Strp("z")}` skapar visuell "pointer-soup" särskilt när varje scalar-comparison kräver helper-anrop.
- **B:** Mest kompakt för komplexa villkor, men ser inte ut som typad Go — användarna får läsa en separat dokumentation av schema-konventioner.
- **C:** Läser som ett villkor (`F("X").Like("y")` är meningsfullt). Kedjor är väldigt naturliga för fields-selection och orderBy.

**C bäst för läsbarhet, A bäst för diffbarhet.**

### 6. Kompositionsergonomi (kombinera filter dynamiskt vid runtime)

Tänk: bygg en lista av `FilterExpr` baserat på vilka fält användaren fyllt i.

- **A:** Måste appenda till `[]CustomerFilter` och wrappa i AND-fältet. Funkar men klumpigt.
- **B:** Trivialt — bara `parts := []any{...}; if x { parts = append(parts, ...) }`.
- **C:** Trivialt — `parts := []c.FilterExpr{...}; ... ; c.And(parts...)`.

**B och C vinner.**

---

## Sammanfattning

| Kriterium | A | B | C |
|---|---|---|---|
| Typsäkerhet | ✅✅ | ❌ | ✅ |
| Verbosity | ❌ | ✅ | ✅✅ |
| Generator-bördan | ❌❌ | ✅✅ | ✅✅ |
| Felmeddelanden | ✅✅ | ❌ | ❌ |
| Läsbarhet | ⚠️ | ⚠️ | ✅ |
| Dynamisk komposition | ⚠️ | ✅ | ✅ |

---

## Beslut

**Välj strategi C (fluent builders) som primär API, kompletterad med strategi A för fields.**

Närmare bestämt:

- **Filter:** strategi C — `c.And(c.F("CustomerId").Like("cst"), c.Or(...))`. Fluent builder-API. Stringly-typed fältnamn accepteras som rimligt pris för avsevärt mindre genererad kod.
- **OrderBy:** strategi C — `c.OrderBy().Asc("CustomerId").Desc("ExternalReference")`. Trivialt.
- **Fields:** strategi A — *generera* en `CustomerFields`-struct per entitet med `bool`-fält och pekar-på-nested-strukturer. Ger compile-time-safe nested fält-selection (t.ex. `f.CustomerPlacedOrder.Order.OrderID = true`) — där typsäkerhet faktiskt har betydelse, eftersom djupa fältval är där generera-fel oftast smyger sig in.

### Motivering

1. **Filter och orderBy gynnas mest av kompakthet.** Användare skriver dessa varje dag, ofta dynamiskt sammansatta. C:s API är klart kortast och har bäst kompositions-störy.
2. **Fields är där typsäkerhet betalar sig.** Att lista fältnamn i strängar skalar dåligt — refactoring av en server-modell måste annars manuellt spåras genom alla `.Pick("...")`-anrop.
3. **Generator-bördan är liten** för fields-A (struct + en converter-funktion per entitet, ~30 rader var), men spar oss helt från att emittera filter-typer per entitet.
4. **Stringly-typed filter är acceptabelt** så länge servern är auktoritativ källa — det stora paradigmskiftet (TS-portens fas 3 fynd #26–30) var att operatorer och enum-värden måste matcha *exakt*, vilket varken A, B eller C garanterade utan att skrivas om.

### Kända avvägningar vi accepterar

- **Filtra med fel fältnamn upptäcks server-side.** Vi mildrar det genom att senare (fas 5) emit:a *konstanter* per entitet: `customer.CustomerIDField = "CustomerId"`. Användare som vill ha autocomplete kan skriva `c.F(customer.CustomerIDField).Like("...")`.
- **Operator-mismatch** (t.ex. `Like` på en `bool`-kolumn) upptäcks också server-side. Acceptabelt — felet är direkt och pekar på rätt fält.
- **Builder-allokeringar** är fler än A:s (varje `.Pick().Nested(...)` allokerar en builder + slice). Inte mätbart i praktiken; om det blir det kan vi byta till `[]FieldNode` direkt.

### Utvärderat och förkastat

- **A (pure typed-structs):** för stor genererad kod-volym (~7800 rader för CandyShopModel) jämfört med vinsten i typsäkerhet för filter. För filter och orderBy specifikt är pekar-soppan en daglig irritation.
- **B (pure map[string]any):** ingen typsäkerhet alls, ingen IDE-autocomplete, ingen dokumentation av API:t i koden. Användare som inte redan kan Dataceen-schemat har inget att läsa.

---

## Konsekvenser för fas 3 (generator)

Per entitet ska generatorn emit:a:

1. **`<Entity>Fields`-struct** (strategi A:s field-shape) — inkludera `bool`-fält per scalar och pekar-fält per relation/sub-entitet.
2. **`<Entity>FieldsToNodes`-converter** — wrap eller method på structen (~30 rader).
3. **`<Entity>` interface/struct** — read-modellen.
4. **`<Entity>Create`-input-struct** — write-create-modellen.
5. **`<Entity>Update`-typ** — separat ändringsspårnings-design (struct med `_set: map[string]bool` per fält + accessor-metoder. Beslut tas i fas 3 baserat på hur emittern ser ut.).

Delas mellan alla entiteter (kommer från `internal/prototypes/c/`):

6. **`FilterExpr`-interface, `F()`/`And()`/`Or()`/`Not()`** — runtime-byggare.
7. **`FieldsBuilder`** — generisk fältbyggare (alternativ för användare som inte vill använda typad fields-struct).
8. **`OrderByBuilder`** — generisk ordering-byggare.
9. **`PlanToGraphQL`-serializer** — single source of truth för wire format.

**Återanvändbarhet:** av de ~150 rader runtime-kod i prototyperna stannar
filter+orderBy+serializer som är, fields-runtime byts ut mot generator-emitterad
typad version. Inga prototyper raderas — `internal/prototypes/` blir
permanent regression-testbädd för att verifiera att refactor inte bryter
wire-output.

---

## Vad kastas inte bort

Samtliga prototypfiler (8 stycken) stannar i `internal/prototypes/` som:

- **Fas 3-referens** (generatorn ska emittera kod kompatibel med
  `pt.PlanToGraphQL`).
- **Regressionstest** (`prototypes_test.go` låser kanonisk wire-output
  byte-exakt).
- **Dokumentation av strategi A/B** (även om de inte vinner — om någon
  framtida läsare undrar varför så är prototypen körbar).
