# Security Policy

`Implementation file: server_security_policy.go`

Implement the transport security policy of Giznet Server: determine whether the public key allows the establishment of a Peer connection, and whether the Peer is allowed to open the specified Giznet service.

It owns connection and service admission; RuntimeProfile, ownership, and domain relationships decide product-resource access.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`ServerSecurityPolicy`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy) | Adapt the complete Server configuration to the Giznet security policy. |
| [`ServerSecurityPolicy.AllowPeer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy.AllowPeer) | Determine whether the public key allows connection establishment. |
| [`ServerSecurityPolicy.AllowService`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy.AllowService) | Determine service access based on Peer identity and service ID. |

## Configuration and composition

The official Server selects handshake admission with YAML `peer-admission`, independently of
Admin service authorization through `admin-public-key`:

```yaml
peer-admission: registration-token
```

Omitting the setting or selecting `open` preserves open admission; other values fail before
startup. `ServerSecurityPolicy.AllowPeer` only checks for a Server and delegates to its injected
policy, admitting by default when none is installed. `RegistrationTokenSecurityPolicy` owns the
decision. The official host installs the selected admission component even without an admin key.
An admin key does not bypass admission; its Admin service grant remains independent, alongside
the existing Manager role-based service authorization.

Existing available, non-blocked Peers can reconnect without credentials, including existing
`auto_registered` records. Unknown legacy devices must be provisioned first or use an updated
SDK with a valid credential. A new bootstrap admin likewise needs a Peer record or valid token;
`admin-public-key` alone is not a Peer record. Configured `edge-nodes` are provisioned at startup,
so their upstream connections can continue without credentials.

This setting governs only this Server's WebRTC signaling. Edge terminates client handshakes
itself; subsequent logical tunnels do not enter this policy and are not protected by the Server
setting. See [Gizedge](../../gizedge) for that deployment boundary.

## Blocked enforcement in every admission mode

`open` omits handshake credential preflight; it does not disable Peer blocking.
Peer activation returns `ErrPeerBlocked` for an existing blocked record and closes
the connection without creating/replacing the Peer record or writing new
PeerRoutes assignments or LocalRuns. This also applies to registration-token
mode and logical Peers forwarded through Edge to this Server. Edge's own
handshake remains outside the Server admission setting.

Admin block persists the status and disconnects this Server's current Peer
connections, including an activating replacement. Admin approve restores active
status and permits reconnect. Existing streams close with their connection.
Between the committed block and transport close, new RPC, HTTP, OpenAI, Event,
Admin, and Edge service streams reread durable status. Additional host-policy
grants cannot override blocked denial.

DataChannel callbacks have no request context, so Manager limits each status
lookup to 250 ms. Storage errors, timeout, and cancellation deny access; successful
results are not cached across a block. Unknown normal Peers may still open the
mandatory Event stream before activation. Role services still require an active
role or a host grant. Queries hold no Manager lock and start no background
goroutine. This adds a read per new service and rejects new streams during store
failure, instead of retaining stale authorization. Existing streams are not
queried per message.

## Built-in registration-token policy

- An empty credential requires an existing Peer with `Status != blocked`, and `EnsureAvailable`
  excludes pending deletion and permanent tombstones. Storage errors deny admission.
- A nonempty structured credential must have `version == 1 && type == "gizclaw.com/registration_token"`.
  Unknown versions or types are rejected before any Peer/token storage access or lookup-budget
  reservation. Only the trimmed `value` is passed to `PreflightRegistration` as a RegistrationToken.
  It must be enabled, unexpired, below its activation limit, and resolve its Profile/Firmware successfully. Known blocked, deleting, or tombstoned identities cannot bypass those
  checks with a token. A known key with an invalid credential is also denied, without falling
  back to the empty-credential path.

The predicate creates no Peer, binds no RuntimeProfile owner or firmware, and activates no
runtime. `server.register` rechecks limits, records activation, and binds the owner and firmware
in one SQL transaction. Only one new device can claim the last slot; the loser receives
`PermissionDenied` without a partial binding.
Handshake acceptance grants no AI resource access; an unregistered connection still has no
RuntimeProfile. Errors and logs never include tokens.

Before any nonempty-credential lookup, each Server policy instance enforces a shared budget:
at most 64 failures per 60-second window and 8 simultaneous lookups, reserving possible failures
before I/O. These limits span public keys, so rotating keys or tokens cannot bypass them.
Concurrent duplicate tokens are denied. Failed tokens are indexed by SHA-256 of
`type + "\0" + strings.TrimSpace(value)`, with a one-second negative TTL and a 1024-entry cap. Raw tokens and
successful results are never cached. Bounded eviction does not reset the failure budget;
expired entries are removed at the next lookup.

Queries inherit request cancellation/deadlines and have an additional two-second limit. Locks
cover only memory bookkeeping. Storage operations are read-only; cache and counters are
in-memory. Exhausted budgets or capacity can temporarily deny valid new tokens too; callers
should back off and retry after recovery. Known Peers reconnecting without credentials do not
consume the token budget. Multiple processes have independent limits. Default `open` admission
performs none of these handshake token queries or limits, but still enforces the activation and service checks above.

After an administrator reenables, extends, or raises a token limit, a previous failure remains cached for at most one second; the global failure budget still recovers on its existing 60-second window. Known Peers reconnecting without credentials are unaffected by later token restrictions. These changes prevent new activations without revoking existing devices; presenting a restricted token still fails handshake preflight. See [RuntimeProfile and registration](../services/runtime-profile#registrationtoken) for activation and editing semantics.

Built-in credential types use the `gizclaw.com/` domain prefix; custom policies should use their own domain prefix to avoid collisions. The bare `registration_token` type is rejected. A value is limited to 512 UTF-8 bytes, checked by client construction helpers and encoders and after server decoding. The independent GZOF encoded-message limit remains 4096 bytes.
