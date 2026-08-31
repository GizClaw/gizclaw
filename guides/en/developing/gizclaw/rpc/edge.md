# Edge Routing

`Implementation file: rpc_edge.go`

Define `edgeRPCServer`, process Peer lookup, assignment and route resolve on the Edge Giznet service; uniformly encode RPC results, and map `peerroute`, Peer and KV errors to RPC error codes.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `edgeRPCServer` | Holds authoritative Peer route service. |
| `Handle` | Handle one RPC lifecycle on the Edge service connection, then close it. |
| `dispatch` | Distributes Edge route RPC methods. |
| `handleLookup` | Query the current assignment of the Peer. |
| `handleAssign` | Create or update Peer assignment. |
| `handleResolve` | Parse the effective route of the target Peer. |
| `edgeRequiredParams` | Decode and verify required params. |
| `edgeRPCResult` / `edgeRPCError` | Encoding typed result or mapping field error. |

`server.peer.lookup` and `server.route.resolve` are read-only. `server.peer.assign` atomically claims a missing Client Peer for the current Server, returns the existing assignment for the same owner, and may refresh only that owner's endpoint/role metadata. A different Server owner returns conflict and is never overwritten; `expected_version` detects stale updates but never authorizes ownership transfer.
