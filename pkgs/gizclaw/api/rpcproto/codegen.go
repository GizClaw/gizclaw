package rpcpb

//go:generate go run ../../../../tools/gzc-rpcproto-gen -schema ../rpcapi/rpc_resolved.json -out ../../../../api/rpc/payload.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative --proto_path=../../../../api/rpc ../../../../api/rpc/common.proto ../../../../api/rpc/peer.proto ../../../../api/rpc/payload.proto
