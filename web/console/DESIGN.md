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
scripts can use that key: this is not an OS keychain. Nothing else — snapshots,
logs, samples — is persisted.

`servers` accepts either an array or an object keyed by node address. Nodes
require HTTPS; plain HTTP is allowed only for localhost during development.

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
device's API key, the model alias (default `llm`) and an optional node for
`/openai/v1`; it is stored, exported and cleared with the rest of the
configuration. Without it the panel explains how to add it.

`src/assistant/console-runtime.ts` implements every `AssistantRuntime` method
with the console's own state and clients, reading the latest state through refs.
Pages publish structured snapshots with `usePageView`; the assistant never reads
the DOM. The panel uses assistant-ui's external store runtime over the
assistant session and renders each tool action as a collapsible card.
