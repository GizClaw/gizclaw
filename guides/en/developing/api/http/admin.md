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
| Firmware | Firmware resource, external channel package configuration, release and rollback |
| Observability | Server log stream and Peer telemetry query |

Admin OpenAPI only has HTTP path, request/response and wire error. Resource validation, authorization, storage and domain lifecycle are implemented by corresponding services and resource managers.

## Admin Resource identity

Every concrete Admin Resource is uniquely addressed by caller-supplied `(kind, id)`. A declarative envelope must contain a non-empty `metadata.id`, and direct create/put DTOs must contain the same `id`. The Server persists an accepted value exactly; it does not generate or trim it and does not resolve a `name` into an ID. IDs are limited to 1024 Unicode characters (and 4096 UTF-8 bytes). IDs with leading or trailing whitespace are invalid, as are the standalone URI dot segments `.` and `..`; other valid IDs remain opaque. `PUT /resources/{kind}/{id}` and domain-specific put endpoints require exact equality between the path ID and body ID.

`ResourceList` is the only exception. It is a virtual apply-only batch envelope with no metadata or ID; every concrete item inside it still declares its own `metadata.id`. An apply result returns the same ID for a concrete Resource, while the ResourceList top-level result has no ID.

Foreign-key fields store the target Resource's canonical ID. A caller can therefore construct the complete dependency graph before submitting anything; apply never requires creating a Resource, recording a Server-generated ID, and rewriting later references. `name` exists only in typed contracts that explicitly own a scoped name, such as `spec.name` for Workspace, Contact, FriendGroup, FriendGroupMember, and the peer-visible Firmware release-line name, `spec.invoke_name` for Tool, and Peer RPC aliases. It is not generic Admin identity.

Domain-derived Resource IDs must be deterministic from known inputs. Friend uses the sorted Peer-public-key relation ID, FriendGroupInviteToken uses its `friend_group_id` as its ID, and FriendGroupMember uses `<escaped friend_group_id>:<escaped peer_public_key>`. Each component uses URL path percent-encoding, so the separating colon belongs to neither component. FriendGroup IDs are limited to 80 Unicode characters so the derived membership ID stays within the global 1024-character limit. Synchronized Voice IDs use a length-delimited SHA-256 derivation over provider kind, tenant ID, and provider voice ID.

This contract gives declarative providers such as Terraform a stable `<kind>/<id>` import key, retry-safe create/apply, and references known at plan time. The Server does not accept or migrate the legacy `metadata.name` format.

## Pending-deletion operations

`DELETE /peers/{publicKey}`, `DELETE /workspaces/{name}`, and `DELETE /peers/{publicKey}/pets/{id}` atomically create or reuse one domain pending-deletion handoff while returning the retained active projection. They do not expose the handoff record, and the marker does not change authorization, reads, lists, or mutations. Peer Admin deletion does not force-close an online Peer. Workspace deletion accepts only user-created Workspaces and returns `SYSTEM_WORKSPACE_DELETE_FORBIDDEN` for a system Workspace. Pet deletion retains its bound system Workspace. Physical cleanup and pending inspection/retry APIs are owned by the cleanup service, not these delete operations.

## Resource dependency

Admin quotes `shared.json`; the generation entry continues to quote `resources/*.json`:

```text
shared/ ← resources/ ← shared.json ← admin.json
```

Resource-specific Spec and Resource are placed in the same file; the Admin API should not load the entire Resource graph indirectly through `shared.json`.
