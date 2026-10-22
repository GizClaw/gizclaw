# Client

Source：[rpc_client.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_client.go)

Define stateless `rpcClient`, and implement Server actively calling RPC methods of Client Peer. Current capabilities include reading device information and identifiers; request construction, RPC calls, and typed response decoding are all handled by this file.
