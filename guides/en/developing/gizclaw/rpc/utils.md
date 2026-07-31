# Utilities

`Implementation file: rpc_utils.go`

Provides single-request dispatch, stream request processing, Ping, client call, request/result construct, payload validation, API error mapping and type conversion helper common to RPC runtime.

This file is an internal auxiliary implementation of RPC and does not have independent domain capabilities.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `handleRPC` / `handleRPCWithStream` | Run one normal or streaming RPC lifecycle on a request-owned connection. |
| `handleRPCStreamRequest` | Select streaming dispatch or normal dispatch and write back response. |
| `callRPC` | Execute a request/response call on an existing connection. |
| `callRPCPing` / `handleRPCPing` | Universal Ping client/server helper. |
| `newRPCRequest` / `newRPCRequestParams` | Construct typed RPC request and payload. |
| `newRPCResultResponse` / `callRPCResult` | Encoding or decoding typed result. |
| `validateRPCParams` | Use generated payload decoder to verify params. |
| `rpcAPIError` / `rpcInvalidParams` / `rpcUnexpectedResponse` | Construct stable RPC errors. |
| `convertRPCType` | Perform conversions between structurally compatible API types. |
