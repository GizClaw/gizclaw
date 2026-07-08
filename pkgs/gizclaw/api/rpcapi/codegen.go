package rpcapi

//go:generate go run ../../../../tools/gzc-rpcapi-gen -peer ../../../../api/rpc/peer.proto -common ../../../../api/rpc/common.proto -payload ../../../../api/rpc/payload.proto -out generated.go
//go:generate go run ../../../../tools/gzc-rpcproto-gen -proto ../../../../api/rpc/peer.proto -go-rpcapi-out payload_proto_gen.go
