# Public API

Public/Peer HTTP provides connection entrypoints, device management, and owner-scoped capabilities. [peer.json](https://github.com/GizClaw/gizclaw/blob/main/api/http/peer.json) defines routes, parameters, requests, responses, and errors; browse it through [API Reference](/api/).

WebRTC signaling runs before a Peer connection exists; connected capabilities use an HTTP service stream or RPC. Choose an entrypoint by caller identity and resource ownership rather than defining another contract for the same capability. See [Peer service](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_peer_http.go) for authentication and handler composition.

## Peer state synchronization

Synchronization converges a controller app to current owner-scoped state; it is not a historical event log. Continue from a Server-issued checkpoint rather than constructing one from the client clock.

Stage a round's changes and commit state and checkpoint together only after its completion event. Discard incomplete rounds after disconnect or cancellation, and reload fully when Server requests a reset. Event structures belong to the Schema; see [Peer Sync](../../gizclaw/services/runtime/peersync) for storage context.

## Device and contact surface

Device and contact operations access a target Peer under the authenticated caller's permissions. An API Key has a bound owner; signed Bearer access uses current Admin/Client permissions. Keep caller and target identities distinct.

Ordinary reads return Server projections rather than a fresh physical-device sample. Use a device operation when a live hardware result is needed. Domain services manage persistent resources such as contacts, whose availability differs from device connectivity.

## Friend and friend group surface

HTTP and Peer RPC share Social services. Group names belong to each member's namespace, while actual membership determines permission. Apps must not infer authorization from display names or cached lists.

Online and room-presence displays use shared snapshots and can lag replication. Actual notification delivery still requires device confirmation. See [Social](../../gizclaw/services/social) for interaction and storage boundaries.

## Device control flow

Server validates identity, resource access, and parameters before forwarding control to a device. MHS handles hardware state; device procedures handle predefined operations. Schemas and generated SDKs define the requests.

Acceptance and completion are different stages. Reboot, network changes, factory reset, and upgrade may disconnect a device. Do not automatically replay side effects after a timeout; read state to establish the result. Server validation does not replace device safety limits.

See [Client provider](../proto/rpc/client-provided-to-server) for discovery and invocation, and [RPC Reference](/references/rpc) for protocol entrypoints.

## Device debug access and anonymous lookup

The device's authoritative Server controls debug access under its current permissions. A public key selects a device without granting read or write access. Controllers distinguish debug access, API Keys, and signed Bearer identities.

Anonymous identifier lookup only discovers matching public keys. Device-declared SN and IMEI values need not be unique and never replace authentication. Consult Peer HTTP Schema for entrypoints and access declarations.

## Music playback

Player operations use device procedures, while status queries and telemetry report playback state. Separate playlist edits, accepted commands, and actual playback in the UI. See [player guidance](../proto/rpc/client-provided-to-server#music-player) for supported operations.

## MHS v0 HWDs

Discover physical instances through the RuntimeProfile manifest, then read or write the selected instance. The manifest names instances, HWD Schema defines state, and device drivers enforce hardware limits.

Use the actual device response to update the UI after a write. A timeout does not prove the write was absent; read before retrying. See the [RuntimeProfile hardware manifest](../../gizclaw/services/runtime-profile#mhs-v0-hardware-manifest).

## Lua app upload

Install an app through a binary package stream or a device-side URL download. Follow Schema size and transport constraints rather than inventing a separate installation protocol. Server and Edge preserve backpressure and cancellation; the device validates and installs the package.

See the [Lua app provider](../proto/rpc/client-provided-to-server#lua-applications) for package and device integration guidance.

## Signed Bearer

Signed Bearer proves possession of the caller's private key; Server still authorizes the current role and target relationship. Edge routing does not expand those permissions. The wire extension is in `peer.json`; see [API usage](/en/using/api#signed-bearer-authentication) for integration.
