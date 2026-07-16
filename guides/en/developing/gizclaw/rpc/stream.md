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
