# Dataceen Go client

Go-port of the Dataceen client. Parallel to the C# (`DataceenClient/`,
NuGet v1.1.33) and TypeScript (`DataceenClientTs/`) implementations — does
not replace them. Verified end-to-end against the live `Thomas/CandyShopModel/All`
backend.

**Status:** v0.1.0-rc — feature parity for Find/FindByID/Create/Update/Delete
(nodes), compound-create (relationships), gRPC subscriptions with typed
handlers. Search + bulk operations + aggregations are scheduled follow-ups.

## Quickstart

```bash
git clone …
cd DataceenClientGoLang

cp .env.example .env
# fill in DATACEEN_CLIENT_SECRET — the rest defaults to CandyShopModel

go run ./examples/candyshop                       # read-only: Find/Subscribe
RUN_DESTRUCTIVE=1 go run ./examples/candyshop      # full CRUD + relation
```

The `pkg/dataceenevent/*.pb.go` proto-generated files and `pkg/generated/`
typed clients are committed, so a fresh clone runs the example out of the
box. Regenerate only when the schema or proto changes (see "Codegen" below).

## A taste of the API

```go
import (
    "context"
    "time"

    "github.com/dataceen/client-go/pkg/dataceen"
    "github.com/dataceen/client-go/pkg/dataceen/dsl"
    "github.com/dataceen/client-go/pkg/dataceen/subscriptions"
    "github.com/dataceen/client-go/pkg/generated"
)

client, _ := dataceen.NewClient(cfg, dataceen.WithLogger(myLogger))
api := generated.NewAllClient(client)

// Find with filter + ordering + nested fields
res, _ := api.Customer.Find(ctx, generated.FindCustomerParams{
    Filter:  dsl.F("CustomerId").Like("cst"),
    OrderBy: dsl.OrderBy().Desc("CustomerId"),
    Fields: generated.CustomerFields{
        MetaId:     true,
        CustomerId: true,
        Name:       &generated.CustomerNameFields{Firstname: true},
    },
})

// Update — only set the fields you mean to change
api.Customer.Update(ctx,
    dsl.F("_id").Eq(someID),
    &generated.CustomerUpdate{
        IsActive: dataceen.Ptr(false),
    })

// Compound-create a relationship between existing nodes
api.CustomerPlacedOrder.Create(ctx, &generated.CustomerPlacedOrderRelationCreate{
    FromID:   customerID,
    ToID:     orderID,
    PlacedAt: time.Now().UTC().Format(time.RFC3339),
})

// gRPC subscription with a typed handler
sub := client.CreateSubscription(subscriptions.Request{
    Topics:          []string{"Customer"},
    BaseloadTopics:  []string{"Customer"},
    IncludeComplete: true,
})
subscriptions.On(sub, "Customer", func(evt *subscriptions.Event[generated.Customer]) error {
    fmt.Println(evt.Complete.CustomerId)
    return nil
})
client.Subscribe(ctx, sub)  // blocks until ctx cancel
```

## Layout

```
.
├── cmd/spike01..04/             # Phase 0 spike binaries (live verifications)
├── internal/
│   ├── prototypes/              # frozen DSL prototype regression-bed
│   └── spikecfg/                # shared .env loader for spikes
├── pkg/
│   ├── dataceen/                # public client: HTTP + retry + token + types
│   │   ├── dsl/                 # filter/fields/order-by builders, AST, serializer
│   │   └── subscriptions/       # gRPC stream with baseload + reconnect
│   ├── dataceenevent/           # protoc-generated proto bindings
│   └── generated/               # typed entity clients (codegen output)
├── tools/codegen/
│   ├── cmd/                     # `codegen` CLI: schema | emit
│   └── internal/{schema,emit}/  # parser + emitters
├── examples/candyshop/          # end-to-end live demo
├── protos/dataceenevent.proto   # Dataceen subscription wire contract
├── docs/                        # plan / decisions / progress / dsl / porting-guide
├── Makefile
└── .env.example
```

## Make targets

```
make test                  # unit tests (all packages)
make test-integration      # one Find against live (gated by RUN_INTEGRATION=1)

make example               # examples/candyshop, read-only
make example-destructive   # examples/candyshop, full CRUD with cleanup

make codegen-schema        # fetch fresh GraphQL introspection → cache/schema.json
make codegen-emit          # emit typed clients into pkg/generated/
make codegen               # both, in order

make gen                   # regenerate proto bindings (rare — only on .proto edits)
make tools                 # install protoc-gen-go + protoc-gen-go-grpc
```

## Codegen

`pkg/generated/` is regenerated from the GraphQL schema. The committed
files are the canonical "default" emission rooted at Customer; that
reaches every entity in CandyShopModel via BFS (39 entities, ~185 files).

```bash
make codegen-schema       # writes tools/codegen/cache/schema.json (~1.3 MB)
make codegen-emit         # emits pkg/generated/*.go from the cache
```

For a different schema or model, set the `.env` to point at the right
domain/model/scope and rerun. The CLI accepts roots:

```bash
go run ./tools/codegen/cmd emit Order            # alternate root
go run ./tools/codegen/cmd emit Customer Order   # multiple roots
```

If you change `protos/dataceenevent.proto`, regenerate the proto bindings:

```bash
make tools                # one-time install of protoc-gen-go plugins
make gen                  # regenerate pkg/dataceenevent/*.pb.go
```

A portable protoc install (no system include path) needs:

```bash
PROTOC_INCLUDE=$HOME/bin/protoc-include make gen
```

## Documentation

- `docs/plan.md` — 8-phase plan, Go-specific
- `docs/decisions.md` — Go-specific decisions + facts inherited from the TS port
- `docs/progress.md` — live status (start here)
- `docs/dsl-comparison.md` — filter/fields DSL prototype analysis + decision
- `docs/porting-guide.md` — Go-specific addendum to the cross-language porting guide

For the full cross-language reference, see the TS port's
`C:\Source\Dataceen\DataceenClientTs\docs\porting-guide.md` — every
server-imposed fact there transfers verbatim.

## What's not yet implemented

These are TS-port features that haven't been ported:

- `Search<Entity>` (separate from Find — supports aggregations + index lookup mode)
- Aggregations — typed bucket/metric input + result fragment
- `CreateBulk` / `UpdateBulk` / `DeleteBulk`
- `<Entity>Update` change-tracking via accessor methods (currently uses pointer fields with `omitempty` — covers the same use case differently)

If you need any of these, the TS port (`DataceenClientTs`) has them; opening
an issue here unblocks the work.

## Module path

Provisional: `github.com/dataceen/client-go`. Update via `go mod edit -module`
before publishing.
