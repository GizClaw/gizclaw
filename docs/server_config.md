# GizClaw Server Config

The GizClaw server loads its workspace configuration from `config.yaml`.
The command server parses this file through `cmd/internal/server.ConfigFile`
and wires named storage backends and logical stores from it. Services use
conventional logical store names from `stores`; service sections are only for
business settings such as TTLs and system task generators.

## Example

```yaml
# Local bind address. The same port serves HTTP public APIs and WebRTC
# signaling over TCP, and WebRTC ICE over UDP.
listen: 0.0.0.0:9820

# Client-facing server address. LAN deployments should set this to the address
# clients can actually reach. It is used by client contexts and advertised in
# WebRTC ICE UDP host candidates when the host is a concrete IP.
endpoint: 192.168.1.20:9820

# Optional admin public key. When set, admin HTTP/RPC calls must authenticate as
# this key. Leave empty only for local development or tests that inject runtime
# admin identity another way.
admin-public-key: "AzmxuX8okxz4eLD1s5qfpNfD68B35Kpsagqmn6dRydfS"

# Physical storage backends. These are concrete persistence engines.
storage:
  # In-memory key/value backend. Useful for tests and throwaway local servers.
  memory:
    kind: keyvalue
    memory: {}

  # Persistent Badger key/value backend.
  main-kv:
    kind: keyvalue
    badger:
      # Path is relative to the server workspace when the workspace runner loads
      # the config.
      dir: data/kv

  # SQL database for ACL rows.
  acl-db:
    kind: sql
    sqlite:
      dir: data/acl.sqlite

  # SQL database for peer-owned gameplay state and ledgers.
  gameplay-db:
    kind: sql
    sqlite:
      dir: data/gameplay.sqlite

  # Object storage backend for uploaded binary assets.
  # The local filesystem driver stores object keys under data/files. An OSS/S3
  # style driver should expose the same object-store semantics.
  local-assets:
    kind: objectstore
    fs:
      dir: data/files

  # Example OSS-style object storage shape. Exact field names can follow the
  # provider SDK, but the server should expose it through the same object-store
  # interface as local-assets.
  # oss-assets:
  #   kind: objectstore
  #   oss:
  #     endpoint: oss-cn-hangzhou.aliyuncs.com
  #     bucket: gizclaw-assets
  #     prefix: workspaces/default
  #     access_key_id: ${OSS_ACCESS_KEY_ID}
  #     access_key_secret: ${OSS_ACCESS_KEY_SECRET}

# Logical stores. These reference physical storage backends above and can add
# prefixes or expose a SQL backend under a service-facing name.
stores:
  # Peer records are stored under the "peers" prefix inside main-kv.
  peers:
    kind: keyvalue
    storage: main-kv
    prefix: peers

  credentials:
    kind: keyvalue
    storage: main-kv
    prefix: credentials

  firmwares:
    kind: keyvalue
    storage: main-kv
    prefix: firmwares

  # Logical object store for uploaded firmware bin payloads. Firmware JSON
  # metadata lives in the firmwares KV store; bytes live under this prefix.
  firmware-assets:
    kind: objectstore
    storage: local-assets
    prefix: firmwares

  # Agent runtime workspace storage. AgentHost creates one subdirectory/prefix
  # per GizClaw workspace here; agents such as doubao realtime and flowcraft use
  # it for generated runtime config, history, memory, and cache files.
  agenthost:
    kind: objectstore
    storage: local-assets
    prefix: agenthost

  minimax-tenants:
    kind: keyvalue
    storage: main-kv
    prefix: minimax-tenants

  voices:
    kind: keyvalue
    storage: main-kv
    prefix: voices

  workspaces:
    kind: keyvalue
    storage: main-kv
    prefix: workspaces

  workflows:
    kind: keyvalue
    storage: main-kv
    prefix: workflows

  acl:
    kind: sql
    storage: acl-db

  # Admin-maintained gameplay catalog resources.
  game-rulesets:
    kind: keyvalue
    storage: main-kv
    prefix: game-rulesets

  pet-defs:
    kind: keyvalue
    storage: main-kv
    prefix: pet-defs

  badge-defs:
    kind: keyvalue
    storage: main-kv
    prefix: badge-defs

  game-defs:
    kind: keyvalue
    storage: main-kv
    prefix: game-defs

  # Gameplay PetDef pixa files and BadgeDef PNG icons.
  gameplay-assets:
    kind: objectstore
    storage: local-assets
    prefix: gameplay

  # Peer-owned gameplay runtime state.
  gameplay-db:
    kind: sql
    storage: gameplay-db

  # Contact address-book records for peer-facing contact RPCs.
  contacts:
    kind: keyvalue
    storage: main-kv
    prefix: contacts

  # Pending and historical friend request records.
  friend-requests:
    kind: keyvalue
    storage: main-kv
    prefix: friend-requests

  # Accepted peer friend relationships.
  friends:
    kind: keyvalue
    storage: main-kv
    prefix: friends

  # FriendGroup metadata records.
  friend-groups:
    kind: keyvalue
    storage: main-kv
    prefix: friend-groups

  # FriendGroup membership rows.
  friend-group-members:
    kind: keyvalue
    storage: main-kv
    prefix: friend-group-members

  # FriendGroup message metadata. Audio bytes live in friend-group-message-assets below.
  friend-group-messages:
    kind: keyvalue
    storage: main-kv
    prefix: friend-group-messages

  # Logical object store for friend group message audio files. The physical object
  # store is shared with other file payloads; this prefix keeps friend group message
  # audio under friend-group-messages/.
  friend-group-message-assets:
    kind: objectstore
    storage: local-assets
    prefix: friend-group-messages

friends:
  # Lifetime of the 6-digit friend OTP reported by the target device through
  # peerrun.
  friend_otp_ttl: 10m

friend_groups:
  # Default TTL for a friend group message when the send request omits ttl_seconds.
  message_default_ttl: 24h
  # Maximum allowed message TTL. Requests above this value are rejected or
  # clamped by the service, depending on the implementation decision.
  message_max_ttl: 7d
  # Background cleanup interval for deleting expired message metadata and audio
  # objects.
  message_cleanup_interval: 5m
  # Maximum decoded audio bytes accepted by friend group message send.
  message_max_audio_bytes: 2097152
```

## Transport Config

The command server uses one local bind address and one client-facing endpoint:

```yaml
listen: 0.0.0.0:9820
endpoint: 192.168.1.20:9820
```

Binding and dialing direction:

- `listen/tcp` serves server-public HTTP APIs, including `/server-info`,
  `/login`, and the fixed WebRTC signaling path `/webrtc/v1/offer`.
- `listen/udp` serves WebRTC ICE UDP.
- `endpoint` is the public `host:port` clients use for server HTTP/signaling
  and peer setup. When the host is a concrete IP, it is also advertised in
  WebRTC ICE UDP host candidates.

Defaults:

- `listen`: `0.0.0.0:9820`
- `endpoint`: defaults to `listen` when omitted.

## CLI Context Config

CLI contexts use the server `endpoint` value. See [context_config.md](context_config.md)
for the context file schema and dialing behavior.

## Field Notes

- `storage` contains physical backends. Currently supported `kind` values are
  `keyvalue`, `vecstore`, `objectstore`, and `sql`. Local files are exposed as
  an `objectstore` backend through `kind: objectstore` with an `fs` config.
- `stores` contains logical stores. Logical key/value stores should normally
  share a physical KV backend and isolate records with `prefix`.
- Logical object stores use `storage` plus `prefix` so multiple asset classes
  can share one physical object storage backend.
- Services use conventional logical store names directly from `stores`.
  Configuration should not repeat those names in service sections.
- Core services expect these logical store names when `storage`/`stores` are
  configured: `peers`, `credentials`, `firmwares`, `minimax-tenants`, `voices`,
  `workspaces`, `workflows`, and `acl`.
- Optional resource services are wired when their conventional logical stores
  exist: `firmware-assets`, `agenthost`, `contacts`, `friend-requests`,
  `friends`, `friend-groups`, `friend-group-members`, `friend-group-messages`, and
  `friend-group-message-assets`.
- Gameplay catalog services are wired from `game-rulesets`, `pet-defs`,
  `badge-defs`, and `game-defs`. Pet definition pixa files and badge PNG icons
  use `gameplay-assets`. Peer-owned pet, badge, points, game result,
  transaction, and reward grant state uses the `gameplay-db` SQL store.
- `agenthost` is optional for the server itself, but workspace agents such as
  Flowcraft should configure it as an object store so AgentHost can prepare
  per-workspace runtime prefixes and local runtime directories when supported
  by the object-store backend.
- `contacts` stores current-peer address-book records. Contact objects are
  external contact data such as display name and phone number; they are not peer
  friend relationships.
- `friend-requests` stores friend request records, while `friends` stores
  accepted peer friend relationships.
- `friends.friend_otp_ttl` controls how long the 6-digit friend OTP reported by
  the target device through peerrun can be used for friend request creation.
- `friend-groups`, `friend-group-members`, and `friend-group-messages` store
  friend group metadata, membership rows, and friend group message metadata
  separately so list pagination and cleanup can be implemented independently.
- `friend-group-message-assets` stores friend group message audio objects.
  Message records should store relative `audio_path` values under this logical
  object store, never absolute host filesystem paths.
- `friend_groups.message_default_ttl` is used when a friend group message send request omits
  TTL. `friend_groups.message_max_ttl` bounds user-provided TTL values.
- `friend_groups.message_cleanup_interval` controls the background task that removes
  expired friend group message metadata and deletes the referenced audio objects. The
  cleanup task should tolerate already-missing audio objects.
- `friend_groups.message_max_audio_bytes` limits the decoded audio payload accepted by
  friend group message send before writing bytes to the configured object store.
- `endpoint` is the only command-server transport binding field. It must be a
  `host:port` value and is used for both TCP HTTP/signaling and UDP ICE.
- Asset services should use object-store operations such as get, put, delete,
  delete-prefix, and list. They should not require directory creation or rename
  semantics that are awkward for OSS-style backends.
