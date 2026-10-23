# Device procedures

Device procedures use the closed `ClientTool` registry in `api/proto/rpc/payload/tool.proto`. The Server invokes one installed procedure through `client.tool.v0.invoke`; the request contains the enum value and encoded request message selected by that value. `client.tool.v0.list` reports the subset installed on the device. The protocol family and version appear as numeric `RpcMethod` values in `client.rpc.methods.list`.

`pkgs/gizclaw/peer_service_serve_peer_http_tool.go` implements the owner-scoped HTTP list and invoke routes. It validates typed JSON arguments before opening the RPC stream, serializes commands per owner, decodes the registered response type and maps device errors to the public HTTP contract. RuntimeProfile Admin Tools remain Server-side HTTP resources for AI and Workflow runtimes; they are separate from the predefined device procedures.

## Generic management invoke tools

These procedures use the existing tool/v0 invoke/list registry and `payload/management.proto`; typed HTTP
arguments/results are in `api/http/peer.json`. Get responses contain `state`, setters contain `applied`.

| Tool | Arguments | Result |
| --- | --- | --- |
| content.age.get / content.age.set | Get {}; Set option_id | option_id and options[{id,display_name,min_age_years?,max_age_years?}] |
| device.features.get | {} | features[{id,display_name,enabled,writable}] |
| device.feature.set | feature_id, enabled boolean | applied feature with actual enabled value |
| usage.time.get / usage.time.set | Get {}; Set limit_seconds uint32 | limit_seconds, used_seconds, exhausted |
| ai.safety.get / ai.safety.set | Get {}; Set level | level and options[{id,display_name}] |
| management.password.get | {} | is_set boolean |
| management.password.set | password string | is_set:true after persistence succeeds |
| management.password.reset | {} | is_set:false; clears only this password |

IDs are at most 64 UTF-8 bytes, display names 128; age/safety options have at most 16 entries and features 32.
Options have unique IDs and include the effective selection. Booleans and limits require presence; false
and zero remain valid. A zero time limit means unlimited. The device owns its current accounting window
and enforcement; exhausted is true when a nonzero limit is reached. Product choices and feature IDs come
from the device, not a GizClaw preference/KV setter. Unknown features use NOT_FOUND, read-only features
FAILED_PRECONDITION, missing tools UNIMPLEMENTED, and failed effects an appropriate RpcStatus.
Set responses report authoritative applied state, including device limits/clamping. Missing or contradictory
results fail validation. AI safety is configured through invoke; its handler may reuse Workspace
safety_fence_level but must apply/load the authority and read the effective state before replying.

Passwords are nonempty, at most 128 UTF-8 bytes and contain no NUL. Reuse control authorization and encrypted
transport. Passwords never enter logs, traces, KV telemetry or responses; Go's device request observer sees
a redacted request. Password reset is independent of factory reset and does not erase settings or networks.

Downstream devices register only real implementations: Go HandleClientTool/rpcpb messages, JS handlers.tools,
Dart GizClawPeerRPCHandlers.tools, or C gzc_tool_handler_t/nanopb messages. Control apps discover tools first.
JS control exposes get/set ContentAge, Features, UsageTime, AISafety and the three management-password methods,
using the same invoke route and actual failures. Hardware and application enforcement require downstream
GizOS/product implementations; this repository does not install firmware or change Forge.
