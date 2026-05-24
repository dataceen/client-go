# Instruktion: autogenerera ett nytt Dataceen-exempel

Den här filen är en **prompt-mall** att klistra in i Claude (eller en
liknande AI-assistent) när du vill generera ett nytt körbart exempel mot
en annan Dataceen-modell. Fyll i `{{...}}`-platshållarna och kör.

`examples/candyshop/` är referensimplementationen — alla beslut nedan
matchar hur den är byggd. Om du behöver göra något fundamentalt
annorlunda, ändra först candyshop-exemplet och sedan denna mall, så
båda håller sig synkade.

---

## Prompt att klistra in

```text
Du ska skapa ett komplett Dataceen-exempel under
`examples/{{example-name}}/` som motsvarar `examples/candyshop/` men för
en annan modell. Använd den existerande generated-klienten i
`pkg/generated/` (om den redan är genererad mot rätt modell — annars börja
med `make codegen-schema codegen-emit` enligt nedan).

### Indata

- Repo-rot: `C:\Source\Dataceen\DataceenClientGoLang`
- Exempel-namn: `{{example-name}}` (kebab-case, t.ex. `inventory-demo`)
- Domän/Modell/Scope: `{{Domain}} / {{Model}} / {{Scope}}`
- Huvudentitet (top-level node):  `{{MainEntity}}` (t.ex. `Customer`)
- Sekundär entitet för relations-create: `{{RelatedEntity}}` (t.ex. `Order`)
- Relations-typ: `{{RelationshipName}}` (t.ex. `CustomerPlacedOrder`)
- Aggregations-fält (scalar eller bool): `{{AggField}}` (t.ex. `IsActive`)
- Default-customer-id-prefix att söka på i Like-filtret: `{{LikePrefix}}` (t.ex. `cst`)
- Eventuell känd entitet för FindByID: `{{KnownID}}` (t.ex. `Customer_Example_00000`) — eller "skip" om okänt
- Vilka destructive-scenarier ska aktiveras under RUN_DESTRUCTIVE: full / minimal
  - **full**: Create + Update + Delete + CompoundCreate + Bulk
  - **minimal**: Create + Delete (ingen relation, ingen bulk)

### Förkrav (kör en gång innan du börjar)

1. `.env` i repo-rot har giltiga DATACEEN_*-credentials för
   `{{Domain}}/{{Model}}/{{Scope}}`. Om scope skiljer sig från
   CandyShopModel, kopiera om `.env` från lämplig källa eller redigera
   manuellt. Spara aldrig riktiga secrets i koden — de stannar i `.env`.
2. Schema cached mot rätt modell:
   `make codegen-schema` (skapar `tools/codegen/cache/schema.json`).
3. Generated-klienten matchar modellen:
   `make codegen-emit ROOTS={{MainEntity}}`. Detta producerar
   `pkg/generated/*.go` BFS:at från MainEntity.
4. `go build ./pkg/generated` är ren innan du börjar skriva exempel-koden.

### Filstruktur som ska produceras

Skapa exakt dessa filer under `examples/{{example-name}}/`:

```
examples/{{example-name}}/
├── README.md          # körinstruktioner — mall nedan
├── main.go            # orkestrator + ctx-setup
├── config.go          # .env-loader (kopia av candyshop:s)
├── find.go            # 5 read-only Find-scenarier
├── subscribe.go       # gRPC-subscription, capped events
├── search.go          # Section 8: Search + aggregations
├── destructive.go     # gated av RUN_DESTRUCTIVE=1 (Create/Update/Delete + compound)
└── bulk.go            # gated av RUN_DESTRUCTIVE=1 (CreateBulk/UpdateBulk + cleanup)
```

För `minimal`-läget: hoppa över `bulk.go` och relations-delen i
`destructive.go`.

### Per-fil-mall

Använd exakt samma struktur som `examples/candyshop/<file>.go` men
substituera dessa för `{{MainEntity}}`:

| candyshop | Generera mot |
|---|---|
| `Customer` | `{{MainEntity}}` |
| `CustomerField.CustomerId` | välj ett identifying string-fält från `pkg/generated/{{MainEntity}}FieldConsts.go` |
| `cst` (like-prefix) | `{{LikePrefix}}` |
| `Customer_Example_00000` | `{{KnownID}}` (skippa FindByID-scenariot om "skip") |
| `Order` (i compound-create) | `{{RelatedEntity}}` |
| `CustomerPlacedOrder` | `{{RelationshipName}}` |
| `IsActive` (i aggregation) | `{{AggField}}` |

### Konventioner att följa exakt

1. **Field-namn:** använd alltid `generated.{{MainEntity}}Field.<Field>`
   istället för stränglitteraler. Det är typsäkert och autocomplete-vänligt.
2. **Pekare för Update-fält:** `dataceen.Ptr(value)` för varje fält du vill
   sätta. nil-pekare = "ändra inte denna kolumn".
3. **`like`-värden:** aldrig wildcards (`%`/`*`) eller delimiters
   (`-`/space/`.`). Servern är Elasticsearch — passa rena tokens.
4. **Cursor:** börja paginering med `Cursor: "null"` eller utelämna (default).
5. **Cleanup:** alla destructive-scenarier ska ha `defer`-cleanup som
   raderar det skapade. Använd unika ID:n med `time.Now().Unix()`-suffix
   så parallella körningar inte krockar.
6. **Aggregation-keys:** använd `bucket.KeyAsBool()` för boolean-fält och
   `bucket.KeyAsString()` för string-fält. Båda har string-fallback för
   fall då servern returnerar `"true"`/`"false"` som strängar.
7. **CompoundCreate:** glöm inte att `_fromId` och `_toId` är obligatoriska
   och måste peka på existerande noder. Generated-koden defaultar
   `_fromLabel`/`_toLabel`/`_label` om du utelämnar dem.
8. **RUN_DESTRUCTIVE-gating:** de destructive-anropen ska aldrig köra
   utan `os.Getenv("RUN_DESTRUCTIVE") == "1"`. Wrap dem i `main.go`.
9. **Ren ctx-cancel för subscriptions:** använd `context.WithCancel` runt
   `client.Subscribe(...)` och `cancel()` i handlern när N events är
   uppnådda.
10. **Logger:** alltid `dataceen.WithLogger(dataceen.SilentLogger)` så
    exempel-output blir prydligt — inga `[dataceen-client]`-prefix.

### main.go-flödet

```go
func run() error {
    cfg, _ := loadConfig()
    client, _ := dataceen.NewClient(cfg, dataceen.WithLogger(dataceen.SilentLogger))
    api := generated.NewAllClient(client)

    rootCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    runFinds(rootCtx, api)            // sektion 1-5
    runSubscribe(rootCtx, client)     // sektion 6
    if os.Getenv("RUN_DESTRUCTIVE") == "1" {
        runDestructive(rootCtx, api)  // sektion 7a-e
        runBulkDestructive(rootCtx, api)  // sektion 7f-g — skippa i minimal
    }
    runSearch(rootCtx, api)           // sektion 8
    return nil
}
```

### README.md-mall

```markdown
# {{example-name}} example

End-to-end demo against {{Domain}}/{{Model}}/{{Scope}}.

## Run

  go run ./examples/{{example-name}}                       # read-only
  RUN_DESTRUCTIVE=1 go run ./examples/{{example-name}}      # full surface

## What it does

[Lista sektionerna 1-8 här med kort beskrivning per sektion.]
```

### Verifieringssteg (kör innan du säger "klart")

1. `go build ./examples/{{example-name}}` — måste vara ren
2. `go vet ./examples/{{example-name}}` — måste vara ren
3. `go run ./examples/{{example-name}}` mot live — alla sektioner ska
   sluta med "All scenarios passed."
4. `RUN_DESTRUCTIVE=1 go run ./examples/{{example-name}}` mot live —
   destructive-sektionerna ska köra och städa upp efter sig (ingen
   kvarlämnad data).
5. Re-kör read-only varianten direkt efter destructive — den ska
   fortfarande fungera (bekräftar att cleanup gick rätt).

Om något steg fallerar: rapportera det specifika fel-meddelandet och
vad du försökte. Hoppa inte över verifieringen.

### Anti-patterns att undvika

- ❌ Skriv inte litterala fältnamn som strängar; använd alltid
  `generated.<E>Field.<X>`-konstanten.
- ❌ Använd inte `make codegen` på en redan emitterad klient om
  schema-modellen inte ändrats — det är slöseri.
- ❌ Catch:a inte fel tyst (förutom subscription-handler-fel som ska
  loggas men inte krascha streamen — det hanteras redan av runtime).
- ❌ Skapa inte nya hjälp-paket i `pkg/dataceen/` för exempel-specifika
  saker. Allt exempel-unikt stannar i `examples/{{example-name}}/`.
- ❌ Stoppa inte in `print`/`fmt.Println`-debug i runtime-paketen. Använd
  `dataceen.Logger`-interface om du vill se interna loggar.
- ❌ Glöm inte att uppdatera `Makefile` med ett make-target om exemplet
  ska kunna köras enkelt:

  ```makefile
  example-{{example-name}}:
  	go run ./examples/{{example-name}}

  example-{{example-name}}-destructive:
  	RUN_DESTRUCTIVE=1 go run ./examples/{{example-name}}
  ```

  och uppdatera `.PHONY`-listan.

### Avgränsningar

- Skapa inte ett nytt git-tag för enbart ett nytt exempel — det är
  applikationskod, inte biblioteksversion.
- Skapa inte en ny package i `pkg/` om du inte verkligen lägger till en
  ny *primitiv*. Allt nytt exempel-relaterat ska ligga under
  `examples/{{example-name}}/` med `package main`.

### När du är klar

Rapportera:
1. Filer skapade (med antal rader var).
2. Resultat från live-körningen — sektion-för-sektion.
3. Eventuella avvikelser från denna instruktion och varför.
```

---

## Hur du använder mallen

1. **Kopiera prompt-blocket ovan** (allt mellan `\`\`\`text` och nästa `\`\`\``).
2. **Fyll i `{{...}}`-platshållarna** med dina specifika värden.
3. **Klistra in i Claude** (eller en annan AI-assistent som har shell- och
   filtillgång till repo:t).
4. AI:n bör producera filerna och köra verifieringsstegen själv.
5. Granska resultatet med `git diff` innan du gör commit.

## Tips på vanliga indata

- **Du har inget `{{KnownID}}`?** Kör först
  `go run ./examples/candyshop` mot rätt modell och plocka ett MetaId
  från sektion 1-utskriften. Eller skippa FindByID-sektionen helt med
  `{{KnownID}} = "skip"`.
- **Modellen har ingen relation att compound-create:a?** Sätt
  `{{RelationshipName}}` till `none` och be AI:n att skippa sektion 7e.
- **Modellen har inget bool-fält att aggregera på?** Välj ett string-fält
  och använd `KeyAsString()` istället för `KeyAsBool()` — be AI:n att
  använda string-keys i aggregations-sektionen.

## När mallen behöver uppdateras

Om du själv ändrar `examples/candyshop/` (lägger till nya scenarier eller
ändrar fil-strukturen) — uppdatera den här filen så framtida exempel
genereras enligt den nya konventionen. Kort regel: candyshop är källan,
allt annat är kopior.
