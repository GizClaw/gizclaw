# GizClaw Console design

A fleet monitoring console embedded in Server and Edge binaries at `/monitor/`.
Nodes expose `/monitor/api/node`, and this console is the only
monitoring front end.

## Access model

The operator pastes one JSON configuration listing every Server and Edge node
and that node's own Monitor Token. The browser then talks to each node
directly — there is no console backend, no aggregation service, and no place a
token is uploaded to. `pkgs/monitor` answers `/monitor/api/node` with CORS
headers so a console on a different origin can read snapshots with an explicit
`Authorization: Bearer` header; no browser credentials are shared.

Configuration is stored in origin-local IndexedDB encrypted with a
nonextractable Web Crypto key and cleared on explicit logout. Same-origin
scripts can use that key: this is not an OS keychain. Snapshots, logs and samples
are not persisted.

`servers` accepts either an array or an object keyed by node address. Nodes
require HTTPS; plain HTTP is allowed only for localhost during development.

The optional `admin.privateKey` holds an existing Base58 Giznet identity for device
queries. The SDK derives its public key and signs a fresh target-bound Peer HTTP
Bearer locally for every request; no private key is transmitted. The owning Server
checks the caller's active Admin role or grant. Explicit per-device `signedBearer`
credentials take precedence; otherwise reads use this identity, then public-key
debug access when no identity is configured. Configuration changes cancel active
page reads; assistant tools read current credentials at invocation time, including
for unwatched devices. The private key is encrypted, exported and cleared with the
configuration and excluded from page diagnostics. Model access retains its separate
assistant API key, and identifier discovery remains anonymous.

## Components

shadcn/ui (new-york) primitives in `src/components/ui` carry the layout
constraints: `Card` owns panel padding and internal rhythm, page sections stack
with a single container gap, and `Tabs`, `Table`, `Input`, `Select` and `Badge`
keep controls identical across pages. Design tokens live in `src/index.css` and
reuse the Monitor palette: canvas #faf9f5, card #fffdf9, ink #141413, muted
#6c6a64, hairlines #e6dfd8, terracotta accent #cc785c, warm dark #181715 for
logs. Teal and terracotta separate RX and TX. Errors are red and always carry
text.

## Pages

Cluster overview, per-node detail, a dedicated log search, and a device watch
list. Log search is its own page because it is the troubleshooting surface:
device records are queried from the configured persistent LogStore through the
Peer HTTP API, filtered by free text or `key:value` clauses (`-key:value`
excludes), and any record opens a panel with its full structured fields.
Text and level filters apply server-side; field clauses apply to loaded records.

The device watch list adds a device by SN, IMEI or public key through a chosen
node and removes it again; the list lives in the same encrypted browser storage
as the configuration. Device APIs are served by nodes that expose
`/gizclaw/v1`, so a node with public client APIs disabled reports its refusal on
that row rather than silently showing nothing.

## Location

The device Location tab draws reported GNSS fixes as a track: latitude and
longitude are separate stored series, so a fix exists only where both were
sampled in the same step, and invalid coordinates never become a point. Map
tiles come from OpenStreetMap, which therefore receives the viewport and the
coordinates; a browser that is offline keeps the last reported values and omits
the map. Nothing uses browser geolocation.

## Data

Every node is polled in parallel every 5 seconds; one failing node shows its own
error row and never blanks the others. Rates come from cumulative byte counters,
so a restart reads as zero rather than a spike. Charts contain measured samples
only — an empty window stays visibly empty — and at most 600 samples per node
are retained in memory with 2/10/30 minute display windows. Fleet traffic sums
per-node samples into 5-second buckets. Node snapshots contain build identity,
runtime status and transport counters; log search uses the persistent LogStore.

The cluster overview defaults to the Node overview tab with runtime status,
traffic, and the node table. A second Devices & channels tab contains device
metrics, trends, and a node table focused on those counts. Every node detail page
also places device metrics in a separate tab alongside Node overview and Runtime
snapshot. Tabs share the existing fleet poller and retained samples.
The overview uses one divided summary strip instead of separate metric cards.
Node tables support name/address/region search and Server/Edge filters; these
controls only filter already loaded snapshots. Connection state, traffic
directions, process resources, and build identity have separate visual columns.

Before any device metrics have arrived, the device tab shows one collection-state
panel with coverage and a node-by-node status table. It does not render empty
metric tiles or an empty trend chart. Connection failures remain distinct from
old nodes that need an upgrade. Once measurements arrive, compact summaries and
session trends appear; historical measurements remain available after a failed
poll. The sampling-window and pause controls live in the shared top toolbar.
Its optional `device_metrics` snapshot is read from node-local atomic counts,
with no business database or Prometheus query. Server and Edge online-device
counts are separate, node aliases are deduplicated by public key, and current
totals exclude unreadable nodes. Coverage is always visible. Old nodes without
these fields show an upgrade status; unavailable individual measurements remain
empty, never substituted connection counts or invented zeroes.

Message rates use consecutive successful snapshots of the same process. A node
restart, public-key change, counter reset, cancellation or failed poll resets the
baseline. A per-minute view is explicitly an extrapolated estimate from the
sampled rate. Device and message trends use the same session-only sampling
windows; incomplete fleet buckets remain gaps. Node detail breaks physical
resources down into device, upstream, admin and unknown peer kinds. Long-term
Metrics Store history is not read by this console.

## Layout

Sidebar navigation lists the cluster overview and every node; below 768px it
collapses to a horizontal bar and panels stack. Wide content scrolls inside its
own container.

## Log presentation

Overview alerts, node logs, and log search share structured summaries: operation,
HTTP/RPC status, error code, and duration when available. Operation names remain
verbatim; successful results do not add a redundant success label. Nonzero RPC
status codes include an explanation, while unknown codes remain visible. Details
retain the original message and fields alongside the summary. Compact log views
search both summaries and raw field values, expose full summaries on hover, and
use the same dark scrollbar as log search.

## Diagnostic assistant

A floating chat button opens the diagnostic assistant from `@gizclaw/assistant`.
The panel is lazy loaded: the agent, the OpenAI client, and assistant-ui load on
first open. The configuration's optional `assistant` block holds an existing
device's API key, the model alias (default `llm`), an optional node for
`/openai/v1` and an optional per-turn context budget; it is stored, exported and cleared with the rest of the
configuration. Without it the panel explains how to add it.

`src/assistant/console-runtime.ts` implements every `AssistantRuntime` method
with the console's own state and clients, reading the latest state through refs.
Pages publish structured snapshots with `usePageView`; the assistant never reads
the DOM. The panel uses assistant-ui's external store runtime over the
assistant session and renders each tool action as a collapsible card.

`src/assistant/assistant-store.ts` keeps conversations as encrypted records under the `console-assistant/` prefix of the local store,
which logout clears. Each thread holds the panel's entries and the assistant
history it resumes from; a thread index lists the latest 50. The panel opens
the latest thread, saves after every turn, and keeps the conversation mounted
behind the history and knowledge views so a running turn continues. A context
compaction appears as a system note. The knowledge base is the zh guides:
`src/assistant/guides-content.ts` bundles them with `import.meta.glob` into
their own chunk, and `src/assistant/guides.ts` loads and indexes that chunk on
the first search.
