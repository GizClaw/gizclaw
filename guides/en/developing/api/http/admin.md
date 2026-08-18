# Admin API

The Admin API targets operators, CLI, and admin UI with administrative privileges. It is responsible for declarative resource management, Peer management, Telemetry query and server operation and maintenance, and is not used by ordinary peers as product data channels.

Source:`api/http/admin.json`
Go generated output: `pkgs/gizclaw/api/adminhttp`

See the [Admin API Reference](/api/) for exact endpoints, parameters, requests, and responses. This page only explains surface ownership and Schema dependencies.

## Surface grouping

| Grouping | Main Responsibilities |
| --- | --- |
| Resource | `apply/show` and unified Resource envelope |
| Peer | Peer query, approval, blocking, refresh, configuration and runtime |
| Runtime access | RuntimeProfile and RegistrationToken management |
| AI | Credential, Model, Voice, Provider Tenant, Workflow, Workspace |
| Gameplay | Game Rule, Pet, Badge, Points, Result and Reward |
| Social | Contact, Friend and Friend Group Management |
| Firmware | Declarative Firmware resources and external stable, beta, and develop package configuration |
| Observability | Server log stream, Peer telemetry query, and active pending-deletion operations |

Admin OpenAPI only has HTTP path, request/response and wire error. Resource validation, authorization, storage and domain lifecycle are implemented by corresponding services and resource managers.

Workspace creation is not an Admin operation. Admin exposes Workspace get/list, update, delete, history, and icon operations, but `PUT` is update-only and generic resource apply excludes Workspace. A Peer creates its own Workspace through the typed Peer RPC/domain capability so ownership is derived from authenticated identity rather than supplied by an operator payload.

## Admin Resource identity

Every concrete Admin Resource is uniquely addressed by caller-supplied `(kind, id)`. A declarative envelope must contain a non-empty `metadata.id`, and direct create/put DTOs must contain the same `id`. The Server persists an accepted value exactly; it does not generate or trim it and does not resolve a `name` into an ID. IDs are limited to 1024 Unicode characters (and 4096 UTF-8 bytes). IDs with leading or trailing whitespace are invalid, as are the standalone URI dot segments `.` and `..`; other valid IDs remain opaque. `PUT /resources/{kind}/{id}` and domain-specific put endpoints require exact equality between the path ID and body ID.

`ResourceList` is the only exception. It is a virtual apply-only batch envelope with no metadata or ID; every concrete item inside it still declares its own `metadata.id`. An apply result returns the same ID for a concrete Resource, while the ResourceList top-level result has no ID.

Foreign-key fields store the target Resource's canonical ID. A caller can therefore construct the complete dependency graph before submitting anything; apply never requires creating a Resource, recording a Server-generated ID, and rewriting later references. `name` exists only in typed contracts that explicitly own a scoped name, such as `spec.name` for Workspace, Contact, FriendGroup, and FriendGroupMember, `spec.invoke_name` for Tool, and Peer RPC aliases. Firmware has no separate name: Admin addresses its caller-supplied ID, and Peer firmware methods do not expose a Firmware identity. A typed name is never generic Admin identity.

Domain-derived Resource IDs must be deterministic from known inputs. Friend uses the sorted Peer-public-key relation ID, FriendGroupInviteToken uses its `friend_group_id` as its ID, and FriendGroupMember uses `<escaped friend_group_id>:<escaped peer_public_key>`. Each component uses URL path percent-encoding, so the separating colon belongs to neither component. FriendGroup IDs are limited to 80 Unicode characters so the derived membership ID stays within the global 1024-character limit. Synchronized Voice IDs use a length-delimited SHA-256 derivation over provider kind, tenant ID, and provider voice ID.

This contract gives declarative providers such as Terraform a stable `<kind>/<id>` import key, retry-safe create/apply, and references known at plan time. The Server does not accept or migrate the legacy `metadata.name` format.

## Pending-deletion operations

`DELETE /peers/{publicKey}`, `DELETE /workspaces/{name}`, and `DELETE /peers/{publicKey}/pets/{id}` atomically create or reuse one domain pending-deletion handoff and return the projection captured at deletion time. The handoff record is exposed only through the operator endpoints below. A Workspace marker immediately rejects selection, runtime, history/icon access, and mutations for that Workspace, while Admin Workspace get/list remain available for diagnostics. A Peer marker immediately closes the online connection and rejects reconnect, login, sessions, RPC, WebRTC, business reads, and mutations for that identity; only Admin Peer get/list and the same delete remain. Completion leaves a permanent Peer tombstone, so the public key cannot register again. Workspace deletion accepts only user-created Workspaces and returns `SYSTEM_WORKSPACE_DELETE_FORBIDDEN` for a system Workspace. Ordinary Pet deletion retains its binding and system Workspace.

Operators inspect active cleanup work through `GET /pending-deletions` and `GET /pending-deletions/{deletionId}?source=...`. `POST /pending-deletions/{deletionId}/retry?source=...` requeues only a `failed` task; retrying any other active state returns `409`. List cursors are opaque and bind every filter except `limit`. Responses expose a domain-approved locator and bounded failure metadata, but never owner identities, descriptors, payloads, credentials, lease tokens, marker fingerprints, raw backend errors, or stack traces. Successful finalization immediately removes the task, so get and retry then return `404`; there is no completion receipt or history endpoint.

## Resource dependency

Admin quotes `shared.json`; the generation entry continues to quote `resources/*.json`:

```text
shared/ ← resources/ ← shared.json ← admin.json
```

Resource-specific Spec and Resource are placed in the same file; the Admin API should not load the entire Resource graph indirectly through `shared.json`.
