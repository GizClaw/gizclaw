# Runtime MCP

Source: [shared connections](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/mcpclient/registry.go)
and [AgentHost integration](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/agenthost/mcp.go).

GizClaw connects Eino ChatModel nodes to remote Streamable HTTP MCP servers.
RuntimeProfile owns connection settings and explicitly grants resources and
individual remote tools to each Workflow. Endpoint URLs and authentication
headers stay on the Server and are not included in Peer projections or model
tool declarations. The generic [MCP client](../../../mcp) owns the protocol;
product runtime owns authorization, shared connections, and Workflow selection.

## Configuration

This fragment extends a Profile with an existing Eino Workflow binding:

```yaml
spec:
  resources:
    mcp_servers:
      knowledge:
        endpoint: https://knowledge.example.com/mcp
        refresh_interval: 5m
  workflows:
    assistant:
      resource_id: eino-assistant
      mcp:
        knowledge:
          resources: true
          tools: [search]
      i18n:
        en: {display_name: Assistant}
        zh-CN: {display_name: 助手}
```

Optional `headers` configure the server-side authentication context. Keep secrets
in protected Admin deployment configuration. MCP protocol headers cannot be
overridden; redirects are disabled. The server alias follows the Profile alias
grammar. Tool names are the MCP server's original names, preserved exactly;
selection is explicit, without wildcards. `resources: true` enables resource and
URI-template discovery and resource reads. Omitting a server from the Workflow
grants no access; selecting a server with neither permission is rejected.

Profile validation checks structure, references, the Eino driver, and identical
permissions for aliases of the same canonical Workflow. It performs no network
requests. Loading the Workspace initializes only its selected capabilities;
connection, discovery, or initial preload failures make that load fail. Changes
to server configuration participate in Profile revision. Reload acquires the
current binding. Existing generations reject calls after their source,
permissions, owner, or Workflow changes; unrelated Profile changes do not revoke
a generation. Workspace parameters cannot grant MCP access or alter its source.
The ordinary Workspace toolkit selection continues to apply to ordinary Profile
Tool bindings; MCP permissions are controlled by the Workflow's separate `mcp`
selection.

## Shared cache and lifecycle

The Server shares a connection and resource cache between Workflow generations
using the same Profile, source alias, and connection settings. Different Profiles
and changed credentials use separate entries. Generation leases retain the
source; releasing one Workflow does not stop another. Releasing the final lease
or shutting down the Server cancels requests, stops refresh workers, and closes
the connection. Failed Agent construction releases any acquired leases.

The initial resource preload loads catalogs and listed resource bodies.
Background polling refreshes complete local snapshots at the configured
interval. A failed refresh retains the previous version. Resource and serialized
byte limits bound each cache; the registry also bounds active sources. Polling
is explicitly owned by the Server rather than starting per model call.

Resource operations read the shared cache directly. Successful background
refreshes become visible on the next operation. Listed resource bodies are
preloaded; concrete template URIs are fetched on their first read and cached for
other Workflows. An unknown URI can therefore require one initial remote read.
Resource text, binary content, MIME types, and metadata retain the MCP result
structure.

Selected remote tools use native model ToolCalls with their discovered input
schemas. Runtime-generated function names avoid collisions with ordinary
Profile tools. Arguments are validated before execution. Each call rechecks
current authorization and goes to the remote server; results are not cached or
retried by the integration. MCP `isError` remains part of the result, while
transport failures return a bounded generic error without connection details.
When a Workflow uses `toolkit.verification_model`, remote tool calls also pass
that verifier; remote read-only hints do not bypass it. Resource reads are
read-only operations.

Resources and tools enter the existing ChatModel ToolCall continuation. They
are not automatically inserted into prompts and do not execute from structured
output nodes. Resource subscriptions and MCP prompts are not part of this
integration.

## Validation

Local MCP and model HTTP fixtures cover shared caches, refresh visibility,
resource templates, native ToolCall continuation, permission revocation,
credential changes, failed construction, polling failure, and lease cleanup.
They do not require a public MCP server or model credentials.

### Live LiteLink and Seed 2.1 Lite qualification

The opt-in deployed-client check exercises real HTTP and verifies that repeated
snapshot reads make zero additional requests:

```bash
GIZCLAW_MCP_LIVE_ENDPOINT=https://litelink.haivivi.com/mcp/v1/public go test -race ./tests/integration/mcp -run '^TestLiveResourceCache$' -count=1 -v
```

The separate `tests/gizclaw-e2e/testdata/mcp-litelink` fixtures qualify the complete
music flow against a local Server built from the current checkout, the deployed
LiteLink MCP, and the real `doubao-seed-2-1-lite-260915` model. With
`GIZCLAW_MCP_ARK_API_KEY` set, the repository's existing real Server/Edge harness
can provision isolated resources and run the committed Giztest in one command:

```bash
go test ./cmd/internal/server -run '^TestLiteLinkMCPMusicGiztest$' -count=1 -v -timeout=720s
```

The harness runs three independent Giztest stories:

- A directly named song is discovered and sent to the player.
- A vague music request receives a clarification question with zero playlist or
  playback mutations. A song-title-only reply in the same Workspace then plays
  the selected song, using the fixture Workflow's history message input.
- A Three Kingdoms request resolves the complete album and queues every episode
  in catalog order before starting playback. The harness independently fetches
  the deployed album and supplies a read-only assertion endpoint. The model
  never receives its expected playlist. Assertions compare the complete actual
  arguments and device readback with that reference, including every URL, title,
  and source ID; a single episode, omission, reordering, or duplicate fails.

Set `GIZCLAW_MCP_EVIDENCE` to an ignored output directory to retain the redacted
JSON report and runner log. The default is a temporary directory. Live execution
skips when the provider key is absent; ordinary Go tests still validate all three
Giztest documents and the independent album-reference checks. The album story
uses `GIZCLAW_MCP_ALBUM_REFERENCE`, which the harness supplies automatically.
To drive a separately running Server for the single-song story instead,
use an isolated Server and Admin context. Set `GIZCLAW_MCP_ARK_API_KEY` to the intended Ark account,
`GIZCLAW_TEST_ENDPOINT` to that Server, and `GIZCLAW_TEST_REGISTRATION_TOKEN` to a
new test-only token before applying resources:

```bash
gizclaw admin apply -f tests/gizclaw-e2e/testdata/mcp-litelink/resources.yaml --context mcp-live-admin
```

```bash
gizclaw test run tests/gizclaw-e2e/testdata/mcp-litelink/music.giztest.yaml --parallel 1 --report .testbench/mcp-live/music.report.json
```

The model receives only the requested song title. It searches the real catalog,
can read work resources for playback metadata, replaces the device playlist with
the returned URL, title and work ID, and invokes playback. Assertions inspect the actual inbound
`client.tool.v0.invoke` arguments, require playlist replacement before play, and
read back the stored playlist and `playing` state. The Giztest Peer is a stateful
protocol fixture without a physical speaker; this proves device invocation and
acceptance, not audible hardware playback. Each scenario cleans up its Peer and
Workspace. Live stories run explicitly rather than in the default credential-free
suite. Album coverage proves ordered queueing and playback acceptance; it does
not simulate end-of-track auto-advance or verify playback-position resume.
