# GizClaw Peer RPC Protocol

This document describes the stream-level Peer RPC framing protocol.

## Stream Model

One `ServicePeerRPC` giznet service stream carries one RPC exchange.

```text
unary stream
├── request protobuf frame
├── EOS frame
├── response protobuf frame
└── EOS frame

download stream
├── request protobuf frame
├── EOS frame
├── response protobuf frame
├── zero or more response binary body frames
└── EOS frame

binary stream
├── request protobuf frame
├── zero or more request binary body frames
├── EOS frame
├── response protobuf frame
├── zero or more response binary body frames
└── EOS frame
```

Unary RPC calls use one request frame, one request EOS frame, one response
frame, and one response EOS frame. Download RPC calls use the same request
sequence, then return the protobuf response envelope before method-specific
binary body frames. Binary streaming RPC calls may send method-specific binary
body frames after the request envelope and before the request EOS; the peer may
read that upload while producing the response envelope and response body frames.

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
into multiple binary body frames by the method implementation. Larger protobuf
request or response envelopes are split into `Text` continuation frames and
terminated by EOS before the next logical frame sequence continues.

## Frame Types

```text
0 EOS
1 JSON
2 Binary
3 Text
```

Peer RPC request and response envelopes normally use one `Binary` frame
containing a protobuf message from `api/rpc/common.proto` and
`api/rpc/peer.proto`. If the encoded envelope is larger than 65535 bytes, the
same envelope bytes are split across one or more `Text` frames. The receiver
reassembles those continuation chunks until the following EOS frame and then
decodes the protobuf envelope. Method-specific payload messages are generated in
`api/rpc/payload.proto`.

`JSON` remains reserved for non-RPC stream families that need it. `Text` is only
valid in Peer RPC as a protobuf envelope continuation frame before the first
request or response envelope has been decoded.

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
  optional bytes payload = 3;
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

Each dispatchable `RpcMethod` enum value carries an `(rpc_method)` option with
the dotted debug name and the request/response payload message names. Go,
JavaScript, and C helper surfaces must derive method-to-payload mappings from
that protobuf metadata instead of a hand-written descriptor or a separate string
registry.

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

An oversized response envelope uses continuation frames before any body frames:

```text
request RpcRequest Binary frame
EOS frame
response RpcResponse Text frame
response RpcResponse Text frame
EOS frame
chunk Binary frame
EOS frame
```

Malformed protobuf payloads, unknown method IDs, invalid frame types, duplicate
response envelopes, and truncated streams are protocol errors.
