package rpcapi

//go:generate go run ./internal/rpcgen -resolve
//go:generate go tool oapi-codegen -config=codegen_config.yaml -o generated.go rpc_resolved.json
//go:generate go run ../../../../tools/gzc-rpcproto-gen -proto ../../../../api/rpc/peer.proto -go-rpcapi-out payload_proto_gen.go
