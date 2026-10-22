# RPC Server

Source：[rpc_server.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_server.go)

Define `rpcServer`, required domain service interfaces, connection handler, total dispatch and all Server RPC handlers. It dispatches normal or streaming requests based on the RPC method and converts between RPC payloads and domain service types.

Server methods cover Peer info, runtime status, run Agent, run workspace, history, memory recall, reload, stop and say. For methods that have been planned but not yet implemented in the contract, this file returns a unified not-implemented response. It has RPC composition and adaptation, but no domain rules for peers, runtime or firmware.

`server.run.say` accepts `text` and a scoped RuntimeProfile `voice_name`. It does not accept canonical Voice, Model, or Credential identifiers.

`handleReloadRunWorkspaceWithOptions` accepts an optional target `workspace_name` and partial `parameters`, reuses Peer resource validation and updates, saves the selection and reloads once. Omitting the target uses the current selection. See [Server Provided to Client](../../api/proto/rpc/server-provided-to-client) for the request and failure semantics.
