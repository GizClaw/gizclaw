# Edge Routing

`Implementation file: rpc_edge.go`

Define `edgeRPCServer`, process Peer lookup, assignment, route resolve, and API-key
owner route resolve on the Edge Giznet service; uniformly encode RPC results,
and map API-key, `peerroute`, Peer, and KV errors to RPC error codes.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `edgeRPCServer` | Holds authoritative Peer route service. |
| `Handle` | Handle one RPC lifecycle on the Edge service connection, then close it. |
| `dispatch` | Distributes Edge route RPC methods. |
| `handleLookup` | Query the current assignment of the Peer. |
| `handleAssign` | Create or update Peer assignment. |
| `handleResolve` | Parse the effective route of the target Peer. |
| `handleAPIKeyResolve` | Authenticate an API key and resolve its owner Peer's effective assignment. |
| `edgeRequiredParams` | Decode and verify required params. |
| `edgeRPCResult` / `edgeRPCError` | Encoding typed result or mapping field error. |

`server.peer.lookup` and `server.route.resolve` are read-only. `server.peer.assign` atomically claims a missing Client Peer for the current Server, returns the existing assignment for the same owner, and may refresh only that owner's endpoint/role metadata. A different Server owner returns conflict and is never overwritten; `expected_version` detects stale updates but never authorizes ownership transfer.

`server.api_key.resolve` (method ID 99) is exposed only through the authenticated
Edge RPC service. The request carries the Bearer credential. The Server uses its
own `services.api_key.store` to authenticate it, obtains the owner Peer from the
API-key record, and returns that Peer's existing `PeerAssignment`. It never
creates, moves, or refreshes an assignment. Invalid or revoked credentials map
to forbidden; a missing or inactive owner route maps to not found. Errors never
echo the credential.
