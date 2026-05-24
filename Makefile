.PHONY: gen tools spike01 spike02 spike03 spike04 build test test-integration tidy codegen-schema codegen-schema-print

# Code generation: produces pkg/dataceenevent/dataceenevent.pb.go + _grpc.pb.go
# from protos/dataceenevent.proto.
#
# Uses `--go_opt=module=...` so the generated file lands at the path implied by
# the proto's `option go_package`, with the module prefix stripped — i.e.
# pkg/dataceenevent/ relative to repo root.
#
# Set PROTOC_INCLUDE if your protoc install does NOT bundle the well-known
# proto files (google/protobuf/*.proto) on its default include path. Example
# for a Windows portable install:
#   make gen PROTOC_INCLUDE=$(USERPROFILE)/bin/protoc-include
PROTOC_INCLUDE ?=

gen:
	protoc \
	  --proto_path=protos \
	  $(if $(PROTOC_INCLUDE),--proto_path=$(PROTOC_INCLUDE),) \
	  --go_out=. --go_opt=module=github.com/dataceen/client-go \
	  --go-grpc_out=. --go-grpc_opt=module=github.com/dataceen/client-go \
	  protos/dataceenevent.proto

# Install the protoc-gen-go and protoc-gen-go-grpc plugins. (You must already
# have `protoc` itself installed via your OS package manager.)
tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

spike01:
	go run ./cmd/spike01-token

spike02:
	go run ./cmd/spike02-graphql

spike03:
	go run ./cmd/spike03-subscription

spike04:
	go run ./cmd/spike04-introspection

build:
	go build ./...

test:
	go test ./...

test-integration:
	RUN_INTEGRATION=1 go test ./pkg/dataceen -run Integration -v

tidy:
	go mod tidy

# ----- Phase 3 codegen -----------------------------------------------------

# Fetch the full GraphQL introspection from the live backend and write it to
# tools/codegen/cache/schema.json (gitignored). Re-run when the model changes.
codegen-schema:
	go run ./tools/codegen/cmd schema

# Print stats from the existing cached schema without re-fetching.
codegen-schema-print:
	go run ./tools/codegen/cmd schema --print

# Emit all entity files from the cached schema, BFS-rooted at one or more
# root entities. Default root is Customer (the canonical example).
#
#   make codegen-emit                  # roots=Customer
#   make codegen-emit ROOTS="Order"    # alternate root
#   make codegen-emit ROOTS="Customer Product"
ROOTS ?= Customer
codegen-emit:
	go run ./tools/codegen/cmd emit $(ROOTS)

# Convenience target: refresh schema + regenerate.
codegen: codegen-schema codegen-emit
.PHONY: codegen

# ----- Examples ------------------------------------------------------------

# Read-only end-to-end demo against the live CandyShopModel backend.
example:
	go run ./examples/candyshop

# Full CRUD demo. Creates + updates + deletes a unique test row. Idempotent.
example-destructive:
	RUN_DESTRUCTIVE=1 go run ./examples/candyshop
.PHONY: example example-destructive
