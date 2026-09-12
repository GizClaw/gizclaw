# Public API

Public API is an HTTP contract that Server exposes to Public/Peer caller before and after WebRTC connection is established. It is the entry boundary and does not represent the full capabilities of the Peer domain service.

Source:`api/http/peer.json`
Go generated output: `pkgs/gizclaw/api/peerhttp`

See the [API Reference](/api/) for exact endpoints, parameters, requests, and responses. This page only explains the Public/Peer surface design boundary.

`/webrtc/v1/offer` Occurs before the Peer connection is established, HTTP signaling must be preserved. The Peer capability after establishing a connection can use reliable HTTP-over-service-stream or Peer RPC; when choosing a transport, avoid maintaining two sets of contracts for the same capability.

The Offer is authenticated by the signed signaling contract itself and does not depend on an API key. Public API can reuse real shared types such as `ErrorResponse`, `DeviceInfo` and `Runtime`, but does not reference Admin Resources.

See [Peer HTTP · API keys](../../gizclaw/peer/service/api-keys) for the authentication and management contract. First-time provisioning stays on the device-local BLE channel; once the device is online, an API key can scan or change Wi-Fi through `/gizclaw/v1/device*`.

## Device and contact surface

`/gizclaw/v1/device*`, `/gizclaw/v1/contacts*`, `/gizclaw/v1/friends*`, and `/gizclaw/v1/friend-groups*` accept `Authorization: Bearer <api-key>` or device debug access as described below. The Server takes the immutable owner Peer from the key record; API key bearer credentials always select their own immutable owner, and manager keys and ordinary keys have the same owner-scoped capability on these routes. An invalid or revoked key answers `401 INVALID_API_KEY`, an owner that is not an active Client with a RuntimeProfile binding answers `403 API_KEY_OWNER_UNAVAILABLE`, an owner pending deletion answers `409 PEER_PENDING_DELETION`, validation and pagination failures answer `400 INVALID_REQUEST`, and store or service failures collapse into a redacted `500 INTERNAL_ERROR`.

Read routes project the authoritative services and never send an RPC to the device:

- `GET /device` returns `DeviceInfo` (name, emoji, `HardwareInfo`, `DeviceIdentifiers`), the same source as `server.info.get`.
- `GET /device/runtime` returns `Runtime` (online, last seen, address, RX/TX) without refreshing online state.
- `GET /device/status` returns the latest authoritative `PeerStatus` snapshot. There is no `fresh` parameter; `client.device.status.get` exists only for control-response write-back.
- `GET /device/telemetry/{field}/latest`, `/device/telemetry`, and `/device/telemetry/aggregate` keep the Admin telemetry field enum, observation times, query limits, ordering, and aggregate semantics and pin the Peer to the owner.
- `GET /device/firmware` returns every channel (`stable`, `beta`, `develop`) of the Firmware configuration bound to the owner, each with an optional `description` and `package` (`version`, `url`, `sha256`, `size`) (stored packages without a version omit `version` and remain available), the same source as `server.firmware.get`. Channel selection belongs to the caller: the Server does not store the channel the device uses, so this route returns every channel at once and the caller picks one. An unbound `firmware_id`, or a binding whose configuration is gone, answers `404 FIRMWARE_NOT_FOUND`; a channel with no configured package simply omits `package` instead of failing.
- `GET /device/runtime-profile` returns the `name` and `revision` of the RuntimeProfile currently bound to the owner, plus `collections[].workflows[].name`, projected directly from `workflows.collections` with collections and workflows sorted by name. `name`/`revision` share their source with `runtime_profile_name`/`runtime_profile_revision` in Peer RPC responses, and each workflow name is the alias `server.workflow.*` uses. `resource_id`, i18n, driver, models, voices, memory, pet definitions, and `app_config` are never returned, and the read does not check whether the Workflow resource behind a binding still exists. A binding that disappears after authentication also answers `403 API_KEY_OWNER_UNAVAILABLE`.
- `GET /device/workspaces` returns the Workspaces the owner explicitly owns, including system Workspaces; shared, ownerless, and pending-deletion Workspaces are excluded. Each item is a `DeviceWorkspace`: `id`, `name`, `collection`, `workflow_name`, `available`, `system`, `created_at`, `updated_at`, `last_active_at`. The Workflow is identified only by the `collection` label and the alias it resolves to in the owner's current RuntimeProfile (the workflow name `/device/runtime-profile` lists), with the same resolution as Peer RPC `Workspace.workflow_name`; the Admin Workflow ID is never returned. When the alias no longer resolves, `workflow_name` is omitted and `available` is false; a Workspace without a collection label, such as a system Workspace, omits `collection`. Optional `collection` and `workflow_name` query parameters filter exactly, and a `workflow_name` filter never matches a Workspace whose alias no longer resolves; an empty filter value returns `400 INVALID_REQUEST`.
- `DELETE /device/workspaces/{workspaceId}` uses the same Workspace deletion as `server.workspace.delete`: it records the pending deletion and answers `202` with no body, while history, audio, runtime state, and the icon are removed in the background without contacting the device. Until the cleanup finishes, the device reading the Workspace gets `WORKSPACE_PENDING_DELETION` and a same-named `server.workspace.create` gets `ALREADY_EXISTS`. Foreign, absent, and already pending-deletion IDs return `404 WORKSPACE_NOT_FOUND`; a system Workspace returns `409 SYSTEM_WORKSPACE_DELETE_FORBIDDEN`, and an owner whose own deletion is pending returns `409 PEER_PENDING_DELETION`. Long-term memory is outside the current Workspace cleanup.
- `/contacts` list/create/get/put/delete use the same owner-scoped data as `services/social/contact`; `{contactName}` is the owner-scoped immutable `name`, cross-owner and missing names both answer `404 CONTACT_NOT_FOUND`, and name or phone conflicts answer `409 CONTACT_ALREADY_EXISTS`, and creating a contact for an owner that already has 8 answers `409 CONTACT_LIMIT_REACHED`.

## Friend and friend group surface

`/gizclaw/v1/friends*` and `/gizclaw/v1/friend-groups*` call `services/social/friend` and `services/social/friendgroup` as the key owner, with the same business rules, permissions, and persistence as the `server.friend.*` and `server.friend_group.*` RPCs. They only read and write the shared Social KV, never contact the device, and keep working while it is offline. There is no separate social scope: a key that can reach `/device*` can manage friends and groups.

| Route | Reuses | Success |
| --- | --- | --- |
| `GET /friends/invite-token` | `GetFriendInviteToken` | `200 InviteToken`; no active token answers `404 INVITE_TOKEN_NOT_FOUND` |
| `POST /friends/invite-token` `{ttl_seconds?}` | `CreateFriendInviteTokenWithTTL` | `200 InviteToken` |
| `DELETE /friends/invite-token` | `ClearFriendInviteToken` | `204`, idempotent |
| `POST /friends` `{invite_token}` | `AddFriendReportingExisting` | `201 Friend` |
| `GET /friends` | `ListFriends` | `200 FriendList` |
| `GET /friends/{friendName}` | `GetFriendRelation` | `200 Friend` |
| `DELETE /friends/{friendName}` | `DeleteFriend` | `204`; repeating a completed delete also answers `204` |
| `GET /friend-groups` | `ListFriendGroups` | `200 FriendGroupList`, each with `my_role` |
| `POST /friend-groups` `{name, display_name?, description?}` | `CreateFriendGroup` | `201 FriendGroup` |
| `POST /friend-groups/@join` `{invite_token, name}` | `JoinFriendGroup` | `200 {group, member}`; idempotent when already joined under the same name |
| `GET /friend-groups/{friendGroupName}` | `GetFriendGroup` | `200 FriendGroup`, any member |
| `PUT /friend-groups/{friendGroupName}` | `PutFriendGroup` | `200 FriendGroup`, owner only |
| `DELETE /friend-groups/{friendGroupName}` | `DeleteFriendGroup` | `204` dissolve, owner only |
| `GET`/`POST`/`DELETE /friend-groups/{friendGroupName}/invite-token` | `Get`/`CreateWithTTL`/`ClearFriendGroupInviteToken` | Owner only |
| `POST /friend-groups/{friendGroupName}/@leave` | `LeaveFriendGroup` | `204`, members and admins |
| `GET /friend-groups/{friendGroupName}/members` | `ListFriendGroupMembers` | `200 FriendGroupMemberList`, any member |
| `POST /friend-groups/{friendGroupName}/members` `{peer_public_key, member_name, role}` | `AddFriendGroupMember` | `201`; admins add members, the owner adds admins |
| `PUT /friend-groups/{friendGroupName}/members/{memberName}` `{role}` | `PutFriendGroupMember` | `200`, owner only |
| `DELETE /friend-groups/{friendGroupName}/members/{memberName}` | `DeleteFriendGroupMember` | `204`; removing a member needs admin/owner or the member itself, removing an admin needs the owner |

- `{friendName}` and `{memberName}` are the other Peer's canonical public key (also `Friend.name` / `FriendGroupMember.name`). `{friendGroupName}` is the caller's own name for the Group; a non-member resolves no name, so non-members always get `404 FRIEND_GROUP_NOT_FOUND`. Names with surrounding whitespace in the path or body answer `400 INVALID_REQUEST`, and `cursor`/`limit` follow contacts (limit 1–200, opaque cursor).
- `Friend` and `FriendGroupMember` carry `info {display_name, emoji}` from the same source as `server.friend.info.get` (`Profiles.GetSelfInfo`); `info` is omitted when the other Peer no longer exists or is deleted, and any other read failure answers 500. Friends and Group members are each capped at 10, and lists read profiles one item at a time. `FriendGroupMember` does not return the member's own name for the Group.
- `ttl_seconds` ranges over 60–604800 (7 days); out-of-range values answer `400 INVALID_REQUEST`, and the body may be omitted. Without it the route matches the RPC exactly: an active token is returned unchanged, otherwise a new token lives 5 minutes. With it a new token expires after that TTL, and an active token keeps its value while `expires_at` is extended to `now + ttl_seconds`, never shortened, so a code the device is showing stays valid. The device RPC default TTL is unchanged.
- `POST /friends` answers `409 FRIEND_ALREADY_EXISTS` for an existing relationship, while `server.friend.add` still returns the existing relationship idempotently.
- `@leave` removes the caller's own membership. The owner gets `409 FRIEND_GROUP_OWNER_CANNOT_LEAVE` and dissolves the Group instead; admins may leave too (removing an admin's own membership through `members.delete` still requires the owner role).

Error codes:

| HTTP | code | Case |
| --- | --- | --- |
| 400 | `INVALID_REQUEST` | Missing body, empty or whitespace-padded name, paging parameters, `ttl_seconds` out of range, invalid role |
| 400 | `FRIEND_SELF_INVITE` | The friend invite token belongs to the caller |
| 403 | `FRIEND_GROUP_PERMISSION_DENIED` | The caller's role does not permit the operation |
| 404 | `INVITE_TOKEN_NOT_FOUND` | No active invite token to read |
| 404 | `INVITE_TOKEN_INVALID` | The token used to befriend or join does not exist or has expired |
| 404 | `FRIEND_NOT_FOUND` | No such Friend relationship |
| 404 | `FRIEND_GROUP_NOT_FOUND` | The caller has no Group with this name (including non-members) |
| 404 | `FRIEND_GROUP_MEMBER_NOT_FOUND` | The target Peer is not a member |
| 409 | `FRIEND_ALREADY_EXISTS` | Already Friends |
| 409 | `FRIEND_LIMIT_REACHED` | Either Peer already has 10 Friends |
| 409 | `FRIEND_GROUP_NAME_CONFLICT` | The name already points at another Group |
| 409 | `FRIEND_GROUP_ALREADY_JOINED` | Already a member of the Group under another name |
| 409 | `FRIEND_GROUP_FULL` | The Group already has 10 members |
| 409 | `FRIEND_GROUP_LIMIT_REACHED` | The Peer already belongs to 10 Groups |
| 409 | `FRIEND_GROUP_OWNER_CANNOT_LEAVE` | The owner called `@leave` |
| 409 | `FRIEND_GROUP_OWNER_CANNOT_BE_REMOVED` | Removing the owner membership |
| 409 | `FRIEND_GROUP_OWNER_ROLE_IMMUTABLE` | Changing the owner role |
| 409 | `FRIEND_GROUP_CHANGED` | Concurrent change; retry |
| 409 | `FRIEND_GROUP_PENDING_DELETION` | The Group is being deleted |
| 409 | `PEER_PENDING_DELETION` / `PEER_DELETED` | The owner or the other Peer is being retired / is gone |
| 500 | `INTERNAL_ERROR` | Store or configuration failure, redacted |

## Device control flow

Control routes are forwarded as Server-to-device RPCs (see [Client Provided to Server](../proto/rpc/client-provided-to-server)):

```text
PUT /gizclaw/v1/device/volume { level: 0..100, muted }
  → resolve the API key owner
  → find the owner's active connection; none → 409 DEVICE_OFFLINE
  → client.device.volume.set, 5s timeout → 504 DEVICE_TIMEOUT
  → device reports PeerStatus → stored as the owner's PeerStatus (reported_at from the device)
  → 200 { status: PeerStatus }
```

| Route | RPC | Success |
| --- | --- | --- |
| `PUT /device/volume` | `client.device.volume.set` | `200 { status }` |
| `POST /device/actions/play-sound` `{ sound, duration_ms? }` | `client.device.sound.play` | `204` |
| `POST /device/actions/find` `{ duration_ms? }` | `client.device.find` | `204` |
| `POST /device/actions/reboot` `{ delay_ms? }` | `client.device.reboot` | `204` |
| `POST /device/actions/firmware-update` `{ channel?, sha256? }` | `client.firmware.update` | `204` |
| `GET /device/wifi` | `client.wifi.status.get` | `200 DeviceWifiStatus` |
| `GET /device/wifi/saved` | `client.wifi.saved.list` | `200 DeviceWifiSavedList` |
| `DELETE /device/wifi/saved/{ssid}` | `client.wifi.saved.forget` | `204`; unknown ssid → `404 WIFI_NETWORK_NOT_FOUND` |
| `POST /device/wifi/scan` `{ timeout_ms? }` | `client.wifi.scan` | `200 { networks }` |
| `PUT /device/wifi` `{ ssid, passphrase? }` | `client.wifi.connect` | `202` |

`firmware-update` notifies the device to run one OTA; the device acknowledges first and then downloads, verifies, writes, and restarts on its own. `channel` names a channel from `GET /device/firmware` and defaults to the channel the device already uses. `sha256` is the digest the caller saw: the Server only checks that it is a 64-character lowercase hex string, and the device decides whether it matches the package it resolves, answering `INVALID_PARAMS` (mapped to `400 DEVICE_REJECTED`) when it does not. The package the device currently runs is reported as `PeerStatus.firmware_sha256`, so a caller compares it with the target channel's `package.sha256` to tell whether an update is needed.

`find` is "find my device": the device plays its own built-in find-me sound with a rising volume ramp, with no audio URL, catalog track, or `sound` value involved. The body is optional; `duration_ms` must be non-negative, and the device picks the ring time when it is omitted. An app's find-my-device action calls `find` rather than borrowing `play-sound` to play a track.

`sound` is a device-defined string: the Server only checks that it is non-empty and at most 32 UTF‑8 bytes, and the device provider validates the value; `ssid` has the same 32-byte bound. Wi-Fi scan defaults `timeout_ms` to 8000 and clamps it to 1000–15000 instead of using the normal 5-second control timeout. Omit `passphrase` for an open network; a PSK is 8–63 bytes. A `202` connect response only means the device accepted the credentials: it answers RPC before switching, then necessarily goes offline. Control routes answer `409 DEVICE_OFFLINE` during the outage. After reconnect, clients poll `GET /device/wifi` and compare `ssid` with the target to distinguish success from fallback. The passphrase is only forwarded and is never persisted, logged, or echoed. Scan results come from the device, so the Server revalidates them before answering: at most 32 entries, a non-empty `ssid` of at most 32 bytes, a `bssid` of at most 17, and a `security` of at most 5. An answer outside those bounds is rejected whole as `502 DEVICE_ERROR` without echoing the offending value.

A device `INVALID_PARAMS` maps to `400 DEVICE_REJECTED`, `METHOD_NOT_FOUND` (no provider implemented) maps to `501 DEVICE_UNSUPPORTED`, and every other RPC error maps to a redacted `502 DEVICE_ERROR`; bodies carry only a stable `code` and a redacted `message`. Concurrent control commands for one owner are forwarded serially in arrival order and are never merged or replayed. After a device acknowledges `reboot`, `firmware.update`, or `wifi.connect`, later control commands on that same connection answer `409 DEVICE_OFFLINE` until the device reconnects. Control commands never change PeerRun, Workspace, or Agent state.

Before connection, `/server-info` reports the authoritative Server's `public_key`, software `version`, `build_commit`, and transport capabilities. Server identity remains the cryptographic `public_key`. Through an Edge, the build fields remain those of the authoritative Server, while the `transport` object alone selects the Edge route.

## Device debug access and anonymous lookup

An authenticated device calls `server.runtime.put` with `{"debug_mode":"readonly"}`.
Allowed modes are `off` (default), `readonly`, and `fullcontrol`; missing or invalid modes fail.
The authoritative Server persists this setting in the local PeerRun SQL table, in the `debug_mode` column keyed by public key and exposes it in Runtime.debug_mode. It is not part of
DeviceInfo and cannot be set through `server.info.put`. Reconnecting preserves the
setting; an absent record means off.

Device, contact, friend, and friend group HTTP routes accept `Authorization: Bearer gizclaw_pk_<Base58 public key>`.
The key must use canonical Base58; bare keys and public_key query parameters do not
provide debug authorization. API keys beginning gizclaw_sk_v1_ retain their existing
authentication path and never fall back to public-key access. Edge resolves the
existing Peer assignment and proxies to the configured authoritative Server without
reading DeviceInfo or debug mode. That Server reads the current PeerRun mode on
every request: readonly permits GET, fullcontrol permits device/contact/friend/friend group reads, writes,
and controls. Available active Client status and RuntimeProfile binding remain required.
API-key management, Admin, and OpenAI APIs do not accept public-key debug authorization.
Disabling debug rejects new requests without canceling work already in progress.
Storage errors fail closed and are redacted. Debug responses use Cache-Control: no-store.

Anonymous GET `/gizclaw/v1/peers/@findBySn/{sn}` and
`/gizclaw/v1/peers/@findByImei/{tac}/{serial}` require no Authorization and return
`{"public_keys": [...]}` for every match, including debug-off devices. No match returns
an empty array, without device metadata. SN and IMEI are non-unique declarations.
IMEI indexes use `by-imei:<tac>:<serial>:<pubkey>` and verify prefix candidates against
current records. Updates and deletes affect only that public key. Admin lookup is
`/peers/@findPubKeysByImei/{tac}/{serial}`; CLI `admin peers resolve-imei` also returns a list.

## Music playback

These paths use the `/gizclaw/v1` prefix and existing device authorization. Set/append accept `{ "items": [...] }`, play accepts `{ "index": 0 }`, and mode accepts `{ "repeat": "all" }`. Success returns HTTP 200 with `{ "status": ... }`, except playlist.get returns `{ "items": [...], "playlist_revision": 1 }`. Existing device control errors apply; append is never retried automatically. To play one URL, set a one-item list and play index 0. See [player providers](../proto/rpc/client-provided-to-server#music-player) for the contract.

| HTTP | RPC |
| --- | --- |
| `GET /device/audioplayer` | `client.device.audioplayer.get` |
| `GET /device/audioplayer/playlist` | `client.device.audioplayer.playlist.get` |
| `PUT /device/audioplayer/playlist` | `client.device.audioplayer.playlist.set` |
| `POST /device/audioplayer/playlist/append` | `client.device.audioplayer.playlist.append` |
| `POST /device/audioplayer/actions/play` | `client.device.audioplayer.play` |
| `POST /device/audioplayer/actions/stop` | `client.device.audioplayer.stop` |
| `PUT /device/audioplayer/mode` | `client.device.audioplayer.mode.set` |
