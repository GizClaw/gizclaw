# GizClaw RPC Protocol

This document describes the stream-level RPC framing protocol.

## Stream Model

One giznet service stream carries one RPC exchange.

```text
stream
├── request frame
├── EOS frame
├── response frame
├── response frame
└── EOS frame
```

Unary RPC calls use one request frame, one request EOS frame, one response
frame, and one response EOS frame. Streaming RPC calls use one request frame,
zero or more request body frames, a request EOS frame, zero or more response
frames, and a response EOS frame.

EOS is the protocol-level end of one frame sequence. Transport stream EOF means
the stream was closed. If a peer sees EOF before the expected EOS frame, the RPC
exchange is truncated.

## Frame Header

Each frame starts with a 4-byte little-endian header:

```text
uint16_le size
uint16_le type
payload[size]
```

`size` is the payload byte length and does not include the header. The maximum
single-frame payload size is 65535 bytes. Larger data must be split into
multiple frames by the caller.

## Frame Types

```text
0 EOS
1 JSON
2 Binary
3 Text
```

Unknown frame types are protocol errors. EOS frames must have size `0`, so an
EOS frame is four zero bytes:

```text
00 00 00 00
```

JSON frames contain compact JSON without indentation. Text frames contain UTF-8
text. Binary frames contain raw bytes and are not base64 encoded.

## JSON RPC Frames

RPC request and response envelopes are JSON frames.

Request:

```json
{"v":1,"id":"req-1","method":"all.ping","params":{}}
```

Unary response:

```json
{"v":1,"id":"req-1","result":{}}
```

Error response:

```json
{"v":1,"id":"req-1","error":{"code":404,"message":"not found"}}
```

## Streaming Responses

List, log, download, and similar methods may return multiple frames on the same
stream.

```text
request JSON frame
EOS frame
response JSON frame
response JSON frame
EOS frame
```

For list methods, each response frame can carry one item or one page. The method
schema defines the response payload shape.

For downloads, the stream can return a JSON metadata frame followed by binary
frames.

```text
request JSON frame
EOS frame
metadata JSON frame
chunk Binary frame
chunk Binary frame
EOS frame
```

## Uploads

Upload methods can use one JSON request frame followed by binary or text frames.

```text
request JSON frame
chunk Binary frame
chunk Binary frame
EOS frame
response JSON frame
EOS frame
```

The method schema defines whether upload chunks are allowed and what final
response is returned.

## Client Behavior

Unary clients write the JSON request frame, write request EOS, read the first
JSON response frame, read response EOS, and return.

Streaming clients read frames until EOS. EOS after zero or more successful
response frames means the RPC exchange is complete.

If the transport stream ends before the method-specific minimum response or
expected EOS is received, the client should report a truncated RPC stream.

## Compatibility

The protocol keeps the existing compact JSON request and response envelope. The
main extension is allowing more than one frame on the same stream and adding a
frame type field so JSON, binary, and text payloads can share the same framing.
