# Server Mesh Design Proposal

## Background

GizClaw separates public ingress from authoritative server ownership.
`edge-node` means an edge node: it terminates browser/device-facing
HTTPS/WebRTC ingress, consumes certificates, resolves routes, and forwards
requests into the node fabric.

`server mesh` means the distributed server layer behind edge nodes. Server
nodes own authoritative peer runtime, storage, token issuance, route records,
certificate issuance/distribution, and final authorization decisions.

The long-term topology is:

```text
browser/device
  -> edge-node
  -> server mesh
  -> authoritative peer/data owner
```

The server mesh is not the public edge. It is the set of trusted server nodes
that edge nodes contact through giznet/WebRTC DataChannel.

## Goal

Define the server mesh as a distributed authoritative server layer:

- Multiple server nodes can participate in one GizClaw deployment.
- Each peer or peer-owned data set has an authoritative server owner.
- Server nodes fully synchronize a compact peer directory so every server can
  resolve a peer public key to its host server.
- Server nodes publish resource route records that let edge nodes find the
  owner for shared resources such as chatroom workspaces.
- Edge nodes use route records to send each request to the correct server.
- Server-to-edge and edge-to-server traffic uses the giznet/WebRTC node fabric.
- Server nodes perform final token validation, ACL/ownership checks, and
  business logic execution.
- Server nodes can provide cert services to edge nodes.

The server mesh is not intended to provide multi-hop public data routing. The
public data plane remains one hop from edge node to authoritative server.

## Design

### Node Responsibilities

`edge-node` responsibilities:

```text
public HTTPS/WebRTC ingress
TLS certificate consumption and hot reload
token-aware request routing
route cache
HTTP-over-giznet forwarding
edge control RPC client
```

`server` responsibilities:

```text
authoritative peer runtime
authoritative storage
token issuance and final validation
peer/data ownership checks
route registry
certificate issuance and distribution
edge-facing HTTP/RPC services
server-to-server peer directory sync
shared workspace event delivery and dump/import
```

### Server Mesh Membership

Server nodes should have stable giznet identities. A server node joins the mesh
as a trusted `server` peer. Membership should be explicit and should not be
granted by the `admin` role alone.

The mesh can start with static server membership:

```yaml
servers:
  - public_key: <server-a-public-key>
    endpoint: <server-a-node-endpoint>
  - public_key: <server-b-public-key>
    endpoint: <server-b-node-endpoint>
```

Later phases can add server discovery, health exchange, and membership
rotation.

### Peer Directory

Every server should keep a full local copy of a compact peer directory in a
Badger-backed key/value store. This is similar to a wallet-style full index:
records are small, stored on disk, and synchronized through snapshot/diff/watch
protocols.

The peer directory contains only routing metadata:

```text
peer_public_key
host_server_public_key
role
status
generation
updated_at
expires_at
signature
```

It must not contain chatroom messages, workspace content, assets, or other
large mutable resource data.

The key/value shape can be:

```text
peer-directory/by-pk/<peer_public_key> -> PeerDirectoryRecord
peer-directory/by-generation/<shard>/<generation>/<peer_public_key> -> record_hash
peer-directory/shard-state/<shard> -> latest_generation, root_hash
tombstone/<peer_public_key> -> generation, deleted_at
```

Server-to-server sync should support:

```text
mesh.peer_directory.snapshot.get
mesh.peer_directory.diff.get
mesh.peer_directory.watch
```

New servers can load a snapshot, apply diffs from the snapshot generation, and
then watch future changes. Diff application must be idempotent: an older
generation must not overwrite a newer record.

### Resource Route Records

Peer directory records are not enough for shared resources. The mesh also needs
resource route records for resources whose authoritative owner is not simply
the caller's host server.

The server mesh publishes resource route records:

```text
resource_kind
resource_id
authoritative_server_public_key
server_node_id
generation
expires_at
capabilities
signature
```

Route records are location facts, not authorization grants. An edge node may
use them to choose a server connection, but the chosen server must still
validate the caller token and enforce ownership/ACL policy.

Initial routing can be static:

```text
any peer -> configured upstream server
```

The route API should still be shaped for the long-term case:

```text
server.route.resolve(peer_public_key)
server.route.watch(since_generation)
server.peer.lookup(peer_public_key)
```

### Data Classes

Server mesh data should be split by ownership and sync requirements:

```text
Peer-local data
  examples: peer runtime, device status, self info
  owner: peer host server from the peer directory
  sync: peer directory metadata only

Single-owner business data
  examples: private workspace, personal configuration, peer-owned gameplay
  owner: creator/owner server
  sync: route metadata only

Shared workspace data
  examples: chatroom, shared workspace transcript, shared assets, messages
  owner: workspace authoritative server
  sync: route and membership metadata everywhere; content only on owner
```

Only shared workspaces require server-to-server data dump/sync in this
proposal. Peer-local and single-owner data remain routed to their owner server.

### Shared Workspace And Chatroom Ownership

Chatroom-style workspaces can have members hosted on different servers:

```text
peer A -> server S1
peer B -> server S2
chatroom W owner -> server S1
```

Writes to shared workspace content go to the workspace owner:

```text
B sends message to W
  -> edge resolves W owner as S1
  -> S1 validates B membership
  -> S1 appends the message
  -> S1 delivers events to member current servers
```

The member's host server does not become the message owner. It can deliver
events to local devices and maintain local metadata/cache, but the owner server
keeps the authoritative transcript and asset state.

### ACL And Access Directory

The current ACL implementation uses SQLite. That remains appropriate for the
authoritative server's final permission checks because it provides local
transactions, indexes, and SQL queries over ACL roles and policy bindings.

The server mesh should not use raw SQLite ACL tables as the global sync
protocol. Instead, owner servers should publish a compact, Badger-backed access
directory derived from authoritative ACL/social membership state:

```text
access-directory/by-peer/<peer_public_key>/<resource_kind>/<resource_id>
access-directory/by-resource/<resource_kind>/<resource_id>/<peer_public_key>
access-directory/by-generation/<shard>/<generation>/<entry_id>
```

The access directory contains only metadata such as membership summary,
resource owner server, role/permission summary, generation, and tombstones. It
is used for route lookup, early rejection, UI summaries, and event delivery.
It is not the final authorization source.

Final authorization stays on the authoritative server:

```text
edge/server local Badger access directory
  -> route request to owner server
owner server SQLite ACL
  -> final read/write/use/admin decision
```

This keeps global sync small and append/diff friendly while preserving SQLite
as the authoritative local ACL engine.

### Edge To Server Data Plane

The data plane from edge to server should remain one hop:

```text
edge-node -> authoritative server
```

If the edge cannot reach the authoritative server, it should fail the request
with an explicit unavailable result instead of trying edge-to-edge proxying or
multi-hop server relay.

### Server To Server Event Delivery And Dump

Server-to-server traffic is still needed for shared workspaces, but it should
not be the public request path. It has two narrower responsibilities.

First, event delivery:

```text
workspace owner server
  -> member current server
  -> member device
```

This is used for chatroom messages, workspace events, and notifications after
the owner server has accepted and stored the write.

Second, shared workspace dump/import:

```text
workspace owner server
  -> target server
```

This is used for workspace migration, owner transfer, recovery, cache seeding,
or server decommissioning. It should be scoped to shared workspace resources,
not the whole server database.

Initial server-to-server methods can be:

```text
mesh.workspace.event.deliver
mesh.workspace.dump
mesh.workspace.import
mesh.workspace.log.tail
```

A workspace dump should include:

```text
workspace_id
owner_server_public_key
generation
snapshot_version
exported_at
objects
blobs
append_log_cursor
checksum
signature
```

Imports must verify generation, checksum, signature, and ownership/lease state
before applying data. Dump/import must not let two servers both believe they
are the authoritative owner for the same workspace.

### Edge-Facing Services

Server nodes expose edge-facing services over giznet/WebRTC DataChannel:

```text
ServiceEdgeHTTP
ServiceEdgeRPC
```

`ServiceEdgeHTTP` carries browser/device public API requests from edge nodes to
the authoritative server. The request includes the original token and caller
headers, and the server performs final authorization.

`ServiceEdgeRPC` carries control-plane operations:

```text
server.cert.get
server.cert.watch
server.route.resolve
server.route.watch
server.peer.lookup
```

### Token And Authorization

Signed tokens such as JWT or PASETO fit the server mesh better than opaque
edge-local sessions. A token can carry:

```text
iss  issuing server or issuer key id
sub  caller peer public key
aud  GizClaw public API or cluster id
exp  expiry
iat  issued-at time
jti  token id
scope/capabilities
```

Edge nodes may validate tokens for routing and early rejection. The
authoritative server remains the source of final permission checks.

### Certificate Distribution

Certificate issuance belongs to server mesh capabilities. A server can own the
ACME account and DNS-01 credentials, renew domain or wildcard certificates, and
serve cert bundles to authorized edge nodes through `server.cert.get`.

Edge nodes should not need DNS provider credentials or ACME account keys.

### Non-Goals

- No browser-visible redirect from edge to server.
- No edge-to-edge public data-plane proxy.
- No multi-hop public request routing.
- No generic admin tunnel for peer public API requests.
- No requirement that normal peer callers have the admin role.
- No standalone cert binary requirement.
- No full replication of chatroom messages or workspace content to every
  server.
- No use of SQLite ACL tables as the server mesh diff/snapshot protocol.

## Alternative

### Public Server Endpoints

Each server node could expose its own public HTTPS endpoint and let clients or
edges call it directly. This keeps HTTP simple, but it exposes the server mesh
to the public network and duplicates TLS, DNS, and ingress concerns that should
belong to edge nodes.

### Edge HTTPS Reverse Proxy To Servers

Edge nodes could reverse proxy to server nodes over HTTPS:

```text
browser/device -> HTTPS edge-node -> HTTPS server
```

This can work for a short-term single-upstream deployment. It is not the
preferred long-term mesh because server nodes would still need public or
edge-reachable HTTPS endpoints and certificates. The preferred internal fabric
is giznet/WebRTC DataChannel.

### Full Mesh Of All Nodes

All nodes could connect to all other nodes. This is unnecessary for the public
data plane and scales poorly. The target shape is edge-to-server connectivity
plus server-owned route records, with each public request routed directly from
the edge to the authoritative server.

### Server-To-Server Relay

Server nodes could relay public API requests to other server nodes. This is not
preferred for the data plane. Edge nodes should resolve the authoritative
server and send the request directly. Server-to-server communication can still
exist for control-plane replication, storage migration, or route publication,
but not as a public request relay path.

### Replicating All Workspace Content Everywhere

Every server could keep all chatroom messages and shared workspace content.
This would make reads local, but it turns the mesh into a full distributed
database and greatly increases storage, conflict, and privacy complexity.

The preferred model synchronizes peer directory and access metadata globally,
while keeping shared workspace content on its authoritative owner server unless
there is an explicit dump/import, migration, or replica policy.

### Using SQLite As The Mesh Sync Store

The existing SQLite ACL store could be copied between servers. This is not
preferred. SQLite is suitable for authoritative local ACL checks, but Badger KV
is a better fit for compact full-directory sync, generation indexes,
tombstones, snapshots, and diffs.
