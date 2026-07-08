# GizClaw Peer RPC Protocol

This document describes the stream-level Peer RPC framing protocol.

## Stream Model

One `ServicePeerRPC` giznet service stream carries one RPC exchange.

```text
stream
├── request protobuf frame
├── EOS frame
├── response protobuf frame
├── optional binary body frame
└── EOS frame
```

Unary RPC calls use one request frame, one request EOS frame, one response
frame, and one response EOS frame. Streaming and download RPC calls use the
same protobuf response envelope first, followed by method-specific binary body
frames when the method contract defines them.

EOS is the protocol-level end of one frame sequence. Transport stream EOF means
the stream was closed. EOF before the expected EOS frame is a truncated RPC
exchange.

## Frame Header

Each frame starts with a 4-byte little-endian header:

```text
uint16_le size
uint16_le type
payload[size]
```

`size` is the payload byte length and does not include the header. The maximum
single-frame payload size is 65535 bytes. Larger method bodies must be split
into multiple binary body frames by the method implementation.

## Frame Types

```text
0 EOS
1 JSON
2 Binary
3 Text
```

Peer RPC request and response envelopes use `Binary` frames containing protobuf
messages from `api/rpc/common.proto` and `api/rpc/peer.proto`.
Method-specific payload messages are generated in `api/rpc/payload.proto`.

`JSON` and `Text` frame types remain reserved for non-RPC stream families that
need them. They are not valid Peer RPC request or response envelope frames.

EOS frames must have size `0`, so an EOS frame is four zero bytes:

```text
00 00 00 00
```

## Protobuf Envelopes

`api/rpc/common.proto` and `api/rpc/peer.proto` are the canonical Peer RPC wire schemas.

Requests use `gizclaw.rpc.v1.RpcRequest`:

```proto
message RpcRequest {
  string id = 1;
  RpcMethod method = 2;
  bytes payload = 3;
}
```

Responses use `gizclaw.rpc.v1.RpcResponse`:

```proto
message RpcResponse {
  string id = 1;
  oneof body {
    bytes payload = 2;
    RpcError error = 3;
  }
}
```

`RpcMethod` is the stable numeric method registry. Method numbers are
append-only and must not be reused. SDKs may expose dotted method names for
developer ergonomics, but the wire envelope uses `RpcMethod`.

`payload` carries the method-specific protobuf request or response message for
the selected method. RPC errors use protobuf `RpcError` with stable numeric
error codes and a human-readable message.

## Streaming Responses

Streaming and download methods send a protobuf response envelope first. Any
following body frames are method-specific binary chunks.

```text
request RpcRequest Binary frame
EOS frame
response RpcResponse Binary frame
chunk Binary frame
chunk Binary frame
EOS frame
```

Malformed protobuf payloads, unknown method IDs, invalid frame types, duplicate
response envelopes, and truncated streams are protocol errors.
