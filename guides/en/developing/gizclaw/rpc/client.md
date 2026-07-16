# Client

`Implementation file: rpc_client.go`

Define stateless `rpcClient`, and implement Server actively calling RPC methods of Client Peer. Current capabilities include reading device information and identifiers; request construction, RPC calls, and typed response decoding are all handled by this file.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcClient` | Shared receiver for Client-side RPC calls; does not hold connection status itself. |
| `rpcClient.GetClientInfo` | Request and decode Client device info. |
| `rpcClient.GetClientIdentifiers` | Request and decode Client identifiers. |
