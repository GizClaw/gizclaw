package rpcpb

//go:generate sh -c "set -eu; wkt_include=$DOLLAR{PROTOBUF_INCLUDE:-}; if [ -z \"$DOLLAR{wkt_include}\" ]; then wkt_include=$DOLLAR(pkg-config --variable=includedir protobuf 2>/dev/null || true); fi; set -- --proto_path=../../../../api/rpc; if [ -n \"$DOLLAR{wkt_include}\" ]; then set -- \"$DOLLAR@\" --proto_path=\"$DOLLAR{wkt_include}\"; fi; protoc --go_out=. --go_opt=paths=source_relative \"$DOLLAR@\" ../../../../api/rpc/common.proto ../../../../api/rpc/peer.proto ../../../../api/rpc/payload.proto"
