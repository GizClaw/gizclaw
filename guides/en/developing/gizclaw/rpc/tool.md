# Device procedures

Device procedures use the closed `ClientTool` registry in `api/proto/rpc/payload/tool.proto`. The Server invokes one installed procedure through `client.tool.v0.invoke`; the request contains the enum value and encoded request message selected by that value. `client.tool.v0.list` reports the subset installed on the device. The protocol family and version appear as numeric `RpcMethod` values in `client.rpc.methods.list`.

`pkgs/gizclaw/peer_service_serve_peer_http_tool.go` implements the owner-scoped HTTP list and invoke routes. It validates typed JSON arguments before opening the RPC stream, serializes commands per owner, decodes the registered response type and maps device errors to the public HTTP contract. RuntimeProfile Admin Tools remain Server-side HTTP resources for AI and Workflow runtimes; they are separate from the predefined device procedures.
