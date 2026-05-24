// Package dataceenevent contains the protoc-generated client and types for the
// Dataceen subscription gRPC service.
//
// The .pb.go and _grpc.pb.go files in this directory are generated from
// protos/dataceenevent.proto. Generate them with:
//
//	make gen
//
// or manually:
//
//	protoc --proto_path=protos \
//	  --go_out=. --go_opt=paths=source_relative \
//	  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
//	  protos/dataceenevent.proto
//
// (This requires protoc, protoc-gen-go and protoc-gen-go-grpc on $PATH.)
//
// Wire format reference: see ../../protos/dataceenevent.proto. The TypeScript
// port loads the proto dynamically via @grpc/proto-loader; in Go we use static
// generation because that's idiomatic.
package dataceenevent
