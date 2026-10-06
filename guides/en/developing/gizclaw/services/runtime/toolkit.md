# Runtime Tools

`toolkit` persists Admin HTTP resources. `toolcatalog` resolves current-Peer RuntimeProfile aliases
for both AgentHost and `server.tool.list/get`. Device tools exist only in memory and call the
existing serialized device RPC controller directly.

A dedicated Tool binding selects exactly one source: HTTP `resource_id`, MHS `{id, operation,
fields}`, or a predefined `client_tool.name`. Each Profile Workflow binding explicitly opts in
through `toolkit.tool_names`. Omission and an empty list inject no Tools. Workspace `tool_names`
only narrows that selection; omission adds no restriction and an explicit empty list disables it.

An Eino Workflow binding can select `toolkit.verification_model`, a chat-model alias in the same
Profile. The primary model still proposes native Tool calls. Before a fixed-target MHS read, a mutating device call or
HTTP POST, an independent model checks that exact fixed target and arguments against the actual
conversation and this turn's Tool results. It only approves or rejects, never rewrites arguments
or substitutes a target. Other read operations bypass this additional check. MHS reads require the intended fixed target before device access. Missing conversation,
provider failure and invalid decisions fail closed; normal authorization is reread afterward.
The alias must bind an `llm` Model; other Workflow drivers reject this configuration.
A final-reply check also rejects missing operations, redundant clarification of completed slots,
unsupported completion claims and fabricated results. The primary model can correct a rejected
draft at most twice. Intermediate Tool-round text and rejected drafts stay buffered; only an
accepted final reply is published. Verification failure or exhausted corrections ends the turn
with an error and no unverified reply text. Each checked proposal and final-reply check adds a
model request, cost and latency; corrections can add more. Semantic decisions still require
actual-model qualification.

The legacy Workflow resource `spec.toolkit.tool_ids` grants no runtime authority. Configure the
Profile Workflow binding explicitly. Legacy Workspace `tool_ids` can only narrow HTTP resources
and cannot be combined with aliases. New Peer Workspace selections persist aliases, including
stale references, without falling back to HTTP invocation names.

Profile aliases contain lowercase kebab-case segments separated by dots and are 1–63 bytes.
Underscores are forbidden. A model function name replaces each dot with an underscore, preserving
all other characters: `screen.brightness` maps to `screen_brightness`. Catalog `name` is the alias;
`invoke_name` is the stable projected function name. The HTTP resource's immutable `invoke_name`
and credentials are private implementation details.

MHS binds one current-Peer manifest instance and resolves its HWD. Models cannot supply id/hwd.
Write parameters come from the existing HWD contract and are restricted to explicitly selected
fields. `client.rpc.methods.list.mhs_v0` advertises actual instances and writable fields through
`DeviceControlHandlers.MhsCapabilities`. Generic HWD fields do not establish implementation support;
missing capability information stays unknown and prevents injection. ClientTool input schemas and
protobuf messages come from the predefined procedure registry, and arguments cannot change its
procedure. `client.tool.v0.list` advertises installed handlers.

Discovery batches queries by protocol family within one resolution. Every invocation rereads the
current owner Profile, Workflow binding, Workspace restriction and target capability. Only the
requested alias is resolved for execution. The device queue checks authorization again before
sending the RPC. Offline, unavailable, revoked, invalid and unsupported calls never use another
Tool, procedure or Peer.

The existing catalog includes schema, localized display text, source, fixed target, supported,
online, available and unavailable_reason. `workflow_name` or `workspace_name` selects an effective
subset; they are mutually exclusive and a Workspace must belong to the caller. Disabled and
unresolved entries remain discoverable. Offline means support cannot currently be observed. HTTP
online denotes a resolvable Server resource; enabled state gates available. It does not probe provider health. Pagination
binds Profile revision and scope. Credentials and HTTP auth configuration never enter the catalog.

HTTP execution retains HTTPS-only GET/JSON POST, bounded responses, fixed parameter mappings,
credential resolution on the Server and existing address/redirect/proxy restrictions. Execution
never automatically retries. ToolCall and ToolResult stay inside model continuation and do not
become public assistant control events.

Both proposal and reply verification use a safe projection of the current catalog. Other targets, descriptions and availability provide business context for configured defaults; private HTTP executors and authentication are excluded. A catalog lookup failure prevents the verification model call and execution.

## HTTP resource persistence and authentication

Admin HTTP resources remain in the SQL `tools` table. Canonical ID is the primary key and
private `invoke_name` has a unique constraint. Type, enabled state, description, version and
timestamps use separate columns; input schema, triggers, metadata and HTTP configuration remain
JSON. Conditional updates check row revision and creation incarnation. Resource enumeration
uses ID-ordered batches of at most 256 rows. Secret rotation or deletion/recreation rereads the
current resource before retaining an omitted secret.

Authentication supports none, bearer, header_api_key, volc_ark, volc_search, volc_openapi,
aliyun_app_code and aliyun_openapi_v3. Direct bearer/API-key values are write-only: omission under
the same method retains the secret; replacement rotates it; changing the method removes it.
Provider methods resolve one Server-owned credential at invocation. Volc OpenAPI and Alibaba
Cloud OpenAPI V3 sign the final request; Marketplace uses AppCode. `giztools` owns bounded mapping
and HTTP execution without selecting a Profile or Peer. Invocation rechecks authorization after
credential resolution and before dispatch. Raw transport and credential errors are bounded
recoverable results.

For the inner `run.workspace.set` catalog, the model selects a required Profile `workflow_name`
alias; existing control-app APIs can still select an owned Workspace name. A procedure reply
acknowledges acceptance, while `server.run.workspace.reload-with-options` commits the switch.
Audio `play` accepts an optional index; omission preserves the device default track selection.
