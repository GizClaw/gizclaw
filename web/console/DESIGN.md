# GizClaw Console design

A standalone, statically hosted monitoring console for a whole fleet. Nodes no
longer embed a UI: they expose `/monitor/api/node`, and this console is the only
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
records from every node are merged into one stream, filtered by free text or
`key:value` clauses (`-key:value` excludes), and any record opens a panel with
its full structured fields. Records carry the request's own attributes —
`request_id`, `operation`, `route`, `status`, `duration_ms`, `peer_public_key`,
stream identifiers — so one request can be followed across nodes by filtering on
its `request_id`. The console's own Monitor polling is hidden by default.

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
per-node samples into 5-second buckets. The overview's alert feed is the WARN
and ERROR records of each node's latest snapshot, labelled by node. Log records
are merged by their per-node id across polls, so the console keeps a window that
outlives the node's own 500-record ring; a lower id than the one already held
means the node restarted and the window starts over.

## Layout

Sidebar navigation lists the cluster overview and every node; below 768px it
collapses to a horizontal bar and panels stack. Wide content scrolls inside its
own container.
