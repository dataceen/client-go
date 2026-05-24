# Candyshop example

End-to-end demo of the Dataceen Go client against the live `Thomas/CandyShopModel/All`
backend. Exercises:

- **Find / FindByID** — typed read paths on `*generated.CustomerClient`
- **DSL** — `dsl.F("CustomerId").Like("cst")`, `dsl.OrderBy().Desc(...)`
- **Pagination** — three pages via the `Cursor` returned by Find
- **Subscription** — typed gRPC stream with parsed `Event[Customer]`
- **Create / Update / Delete** — full CRUD lifecycle (gated)

## Prerequisites

1. Go 1.22+ installed
2. `protoc` installed (only needed for the proto package — already generated and committed)
3. Schema cache generated: `make codegen-schema`
4. Generated client emitted: `make codegen-emit`
5. `.env` at the repo root with real Azure AD credentials (mirror of the TS port's `.env`)

The `Makefile` in the repo root has shortcuts for steps 3 and 4:

```bash
make codegen          # = codegen-schema + codegen-emit
```

## Running

```bash
# Read-only — Find/FindByID/like/order_by/pagination/subscribe
go run ./examples/candyshop

# Full demo — adds Create/Update/Delete on a unique test row that's
# automatically cleaned up at the end (deferred deletion).
RUN_DESTRUCTIVE=1 go run ./examples/candyshop
```

## What gets created

The destructive path generates a unique CustomerId of the form
`cst-go-example-<unix-timestamp>` so multiple runs don't collide. The row is
deleted at the end via a `defer` so cleanup runs even if an intermediate step
fails. If you `Ctrl+C` mid-run, the row is left behind — find and delete it
manually:

```bash
# Discover orphans
go run ./examples/candyshop  # Section 3 (`like` filter) lists all "cst-*" customers
```

## Expected output (read-only mode)

```
Dataceen Go client — CandyShop example
model: Thomas/CandyShopModel/All @ https://api.dataceen.com

--- 1. Find first 5 customers ---
  1. cst-000001  (id=Customer_Example_00000, IsActive=true)
  ...
--- 2. FindByID("Customer_Example_00000") ---
  found: cst-000001  ...
--- 3. Find with `like` filter (Elasticsearch token-match) ---
  matched 14 customers (token "cst")
--- 4. Find with order_by CustomerId Descending ---
--- 5. Pagination: 3 pages of size 5 via cursor ---
--- 6. Subscription: BASELOAD on Customer (cap 5 events) ---
  event 1: type=BASELOAD_EVENT ...
  ...
  clean shutdown after 5 events
--- 7. Destructive scenarios skipped ---
    Set RUN_DESTRUCTIVE=1 to run Create/Update/Delete.

All scenarios passed.
```

## Files

- `main.go` — orchestrator, ctx + setup
- `config.go` — `.env` loader
- `find.go` — read-only scenarios 1–5
- `subscribe.go` — scenario 6 (gRPC stream)
- `destructive.go` — scenario 7 (Create/Update/Delete with deferred cleanup)
