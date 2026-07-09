package gizclaw

//go:generate protoc -I ../../../api/rpc --nanopb_out=generated --nanopb_opt=-I../../../api/rpc google/protobuf/struct.proto ../../../api/rpc/common.proto ../../../api/rpc/peer.proto ../../../api/rpc/payload.proto
