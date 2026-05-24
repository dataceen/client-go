//go:build generate

package dataceenevent

//go:generate protoc --proto_path=../../protos --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative ../../protos/dataceenevent.proto
