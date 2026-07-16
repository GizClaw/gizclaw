# Edge Routing

`Implementation file: rpc_edge.go`

Define `edgeRPCServer`, process Peer lookup, assignment and route resolve on the Edge Giznet service; uniformly encode RPC results, and map `peerroute`, Peer and KV errors to RPC error codes.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `edgeRPCServer` | Holds authoritative Peer route service. |
| `Handle` | Run an RPC loop on the Edge service connection. |
| `dispatch` | Distributes Edge route RPC methods. |
| `handleLookup` | Query the current assignment of the Peer. |
| `handleAssign` | Create or update Peer assignment. |
| `handleResolve` | Parse the effective route of the target Peer. |
| `edgeRequiredParams` | Decode and verify required params. |
| `edgeRPCResult` / `edgeRPCError` | Encoding typed result or mapping field error. |
