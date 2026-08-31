# Streaming

`Implementation file: rpc_stream.go`

Define reading and writing of `rpcStream` and RPC request/response envelope: frame sequence, protobuf envelope continuation, EOS, typed method response decoding, iterator and connection I/O error normalization.

This is the RPC framing layer; the underlying connection and service stream belong to `pkgs/giznet`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcStream` | Packaging connection, context and RPC frame codec. |
| `newRPCStream` | Create stream and bind connection lifecycle context. |
| `ReadFrame` / `WriteFrame` | Read and write a single typed RPC frame. |
| `ReadRequest` / `WriteRequest` | Read and write RPC request. |
| `ReadResponse` / `WriteResponse` | Read and write RPC response. |
| `ReadRequestEnvelope` / `ReadResponseEnvelope` | Read a protobuf envelope that may span multiple frames. |
| `WriteRequestEnvelope` / `WriteResponseEnvelope` | Write protobuf envelope and continuation frames. |
| `Frames` / `WriteFrames` / `Responses` | Provides streaming iterator reading and writing. |
| `ReadEOS` / `WriteEOS` | Process stream end marker. |
| `normalizeIOError` | Normalizes underlying I/O errors to RPC stream errors. |

## C request contract

The C SDK represents concurrent unary calls with `gzc_rpc_request_start`,
`gzc_rpc_request_result`, `gzc_rpc_request_cancel`, and
`gzc_rpc_request_destroy`. Each request exclusively owns one RPC service
DataChannel plus bounded frame, continuation, and response buffers. The client
keeps only a non-owning channel-to-request association. The caller serially
invokes `gzc_client_poll` to advance every request; result lookup does not poll
and returns `GZC_ERR_WOULD_BLOCK` while pending.

A response envelope followed by EOS completes with `GZC_OK`; a remote RPC error
is still represented by `gzc_rpc_response_t.has_error`. A deadline returns
`GZC_ERR_TIMEOUT`, while cancellation, an incomplete remote close, or client
close returns `GZC_ERR_CLOSED`. A terminal transition detaches and closes the
channel exactly once, and request-owned response views remain valid until
destroy.

Streaming calls use the same request handle. The caller starts them with
`gzc_rpc_request_start_stream`, queues binary request frames with
`gzc_rpc_request_write`, and queues request EOS with
`gzc_rpc_request_finish_write`. Response envelope, binary data, and response EOS
are delivered in order by the frame callback during `gzc_client_poll`. A write
returns `GZC_ERR_WOULD_BLOCK` while the previous frame is still pending; the
caller polls and retries without introducing a second stream lifecycle.
