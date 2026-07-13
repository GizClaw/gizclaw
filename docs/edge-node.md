# Edge Node Design Proposal

## Background

GizClaw needs a public edge layer so devices and browsers can use a stable
HTTPS/WebRTC ingress without connecting directly to authoritative server
nodes. Server nodes should own peer runtime, storage, route registry, token
issuance, and final authorization. Edge nodes should own public ingress,
token-aware routing, certificate consumption, and forwarding.

The current peer role schema uses `admin`, `server`, `client`, and
`unspecified`. This proposal adds an `edge-node` role for peers that are
allowed to terminate public ingress and call edge-facing server services. The
role is intentionally separate from `admin`: an admin peer can manage the
system, but should not automatically receive wildcard private keys or route all
peer traffic. A normal device/client peer should not need admin privileges to
use peer public APIs through an edge node.

The long-term public topology is:

```text
device/browser
  -> HTTPS/WebRTC edge-node
  -> giznet/WebRTC DataChannel
  -> authoritative server
```

The server is not expected to expose public HTTPS/TCP ports in this model.
Devices should reach the edge, and the edge should reach the server through the
node fabric.

## Goal

Define the first design target for edge-node support:

- Keep one `gizclaw` binary, with an edge command/profile such as
  `gizclaw edge serve <dir>`.
- Add `edge-node` as a peer role for public ingress nodes.
- Route browser/device public API traffic through edge nodes instead of
  redirecting clients to authoritative servers.
- Use giznet/WebRTC DataChannel between edge nodes and servers for the
  internal node fabric.
- Provide an edge-facing HTTP transport over giznet for public API forwarding.
- Provide edge-facing RPCs for control-plane operations such as cert download
  and peer route lookup.
- Keep authorization based on caller peer identity and token claims, not on the
  edge node acting as an admin peer.
- Make certificate management part of GizClaw server capabilities, with edge
  nodes downloading allowed cert bundles from server-side cert services.

Non-goals for this proposal:

- No browser-visible HTTPS redirect to an origin server.
- No edge-to-edge data-plane proxy.
- No multi-hop data-plane routing.
- No requirement that normal peer public API callers have the admin role.
- No independent `gizclaw-cert` binary.

## Design

### Roles

The peer role model should include:

```text
admin       # management peer
server      # authoritative storage, peer runtime, route registry, cert service
edge-node   # public ingress, proxy, cert consumer
client      # normal device/client peer; future naming may revisit gear
```

`edge-node` peers are allowed to call edge-facing server services. They are not
treated as the caller for peer public API authorization. The external caller is
still the peer represented by the incoming token.

### Public Ingress

Edge nodes expose the browser/device-facing API:

```text
/server-info GET
/login POST
/webrtc/v1/offer POST
peer public API routes
OpenAI-compatible routes where supported
```

Clients keep using the edge address. The edge must not return a redirect that
makes the browser or device connect directly to the authoritative server.

### Internal Node Fabric

Edge-to-server traffic should use giznet/WebRTC DataChannel. The server does
not need a public HTTPS endpoint for edge traffic. This keeps node identity on
the giznet public key model and allows server nodes to stay behind the edge
layer.

The internal fabric should expose two edge-facing service surfaces:

```text
ServiceEdgeHTTP  # HTTP-over-giznet for public API forwarding
ServiceEdgeRPC   # edge control-plane RPCs
```

`ServiceEdgeHTTP` carries the external HTTP request to the authoritative
server. The edge preserves the relevant headers and body. The server performs
the final token validation, peer ownership checks, and business operation.

`ServiceEdgeRPC` carries edge control-plane calls. Initial methods should
include:

```text
server.cert.get
server.cert.watch
server.route.resolve
server.route.watch
server.peer.lookup
```

### Token Model

Opaque local session IDs do not fit DNS load-balanced edge ingress well. A peer
may log in through one edge and then reach another edge on a later request.
The preferred long-term token is a signed token such as JWT or PASETO.

The token should include enough information for edge routing and server-side
authorization:

```text
iss  issuing server or issuer key id
sub  caller peer public key
aud  GizClaw public API or cluster id
exp  expiry
iat  issued-at time
jti  token id for revocation/replay controls
scope/capabilities
```

The edge may verify the token to reject obviously invalid requests and to find
the caller peer for route lookup. The authoritative server still performs the
final token and authorization checks.

The first implementation can forward `/login` through the edge to the server.
The server issues the token. Subsequent requests through any edge preserve the
token and caller public key when forwarded over `ServiceEdgeHTTP`.

### Route Registry

Servers own peer route records. Edge nodes query or subscribe to these records
through `ServiceEdgeRPC`.

```text
peer_public_key -> authoritative_server_public_key / server_node_id
```

Route records are only routing facts. They do not authorize access to peer
data. Authorization remains a server-side decision based on token claims,
caller peer, target peer/data owner, and policy.

The initial route implementation may be static and map all peers to a
configured upstream server. The interface should still be shaped as a
peer-aware resolver so a later route db can replace the static mapping.

### Certificate Service

Certificate issuance and distribution should be part of the server role. A
server-side cert service can own the ACME account and DNS-01 credentials, renew
wildcard or domain certificates, and distribute cert bundles to authorized
edge-node peers.

Edge nodes should not need DNS provider credentials or ACME account keys. They
should call:

```text
server.cert.get
server.cert.watch
```

over `ServiceEdgeRPC`, verify the server identity, store the returned cert
bundle, and hot-reload their HTTPS listener.

The cert bundle response should include:

```text
cert_name
version
not_before
not_after
fullchain_pem
private_key_pem or encrypted_private_key
issuer_public_key
signature
```

Wildcard certificates are convenient for edge ingress, but distributing one
wildcard private key to many edge nodes increases the impact of an edge
compromise. A later phase can issue per-edge hostname certificates to reduce
that blast radius.

### TLS And Ports

Certificates validate domain names or IP addresses, not TCP ports. A domain
certificate can serve HTTPS on a non-standard port such as `:9821`.

Let's Encrypt validation choices are:

```text
HTTP-01      fixed port 80
TLS-ALPN-01  fixed port 443
DNS-01       DNS TXT, no node port
```

Wildcard certificates require DNS-01. DNS-01 renewal should be centralized on
the server-side cert service rather than performed independently by every edge.

## Alternative

### HTTPS Redirect To Server

An edge could return a redirect to the authoritative server:

```text
browser -> edge -> 302 https://server-x/...
```

This is not preferred. It exposes server endpoints, breaks the unified public
API address, complicates CORS and token audience, and requires servers to
remain public HTTPS endpoints.

### HTTPS Reverse Proxy From Edge To Server

An edge could reverse proxy to an upstream server over HTTPS:

```text
browser -> HTTPS edge -> HTTPS server
```

This is simpler for a short-term single-upstream deployment and can reuse
existing HTTP handlers. It is not the preferred long-term node fabric because
it requires server HTTPS exposure and duplicates identity/TLS concerns already
handled by giznet peer identity.

### Generic Giznet Service Stream Bridge

An edge could bridge arbitrary giznet service streams byte-for-byte. This is
too broad for the peer public API path. It risks bypassing caller-token
semantics and turning the edge into an implicit admin or transport tunnel.

The preferred shape is explicit:

```text
public HTTPS -> ServiceEdgeHTTP -> server auth/business handler
control RPC  -> ServiceEdgeRPC
```

### Independent Cert Node Binary

A dedicated cert node binary could own ACME and distribute certificates. This
is not necessary for the first design. Cert issuance and cert distribution fit
as server capabilities exposed through `ServiceEdgeRPC`, while edge nodes
consume certs through their normal giznet connection.
