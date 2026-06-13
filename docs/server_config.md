# GizClaw Server Config

The GizClaw server loads its workspace configuration from `config.yaml`.
The command server parses this file through `cmd/internal/server.ConfigFile`
and wires named storage backends, logical stores, and service-specific store
references from it.

## Example

```yaml
# Address the GizClaw server listens on.
# Use ":9820" to listen on all interfaces, or "127.0.0.1:9820" for local-only.
listen: ":9820"

# Optional transport cipher mode for giznet connections.
# Supported values are:
#   chacha_poly   - default giznet encrypted transport when configured
#   aes_256_gcm   - AES-256-GCM encrypted transport
#   plaintext     - no transport encryption, for local tests only
# Omit or leave empty to use the command/runtime default.
cipher-mode: chacha_poly

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

  # SQL database for wallet balances and wallet transactions.
  wallet-db:
    kind: sql
    sqlite:
      dir: data/wallet.sqlite

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

  # PetSpecies JSON metadata lives in main-kv under this prefix. The .pixa
  # bytes live in pet_species.assets_store below.
  pet-species:
    kind: keyvalue
    storage: main-kv
    prefix: pet-species

  # Badge JSON metadata lives in main-kv under this prefix. The icon bytes live
  # in badges.assets_store below.
  badges:
    kind: keyvalue
    storage: main-kv
    prefix: badges

  # Adopted pet records for peer-facing pet RPCs.
  pets:
    kind: keyvalue
    storage: main-kv
    prefix: pets

  # Reward history records for peer-facing reward RPCs.
  rewards:
    kind: keyvalue
    storage: main-kv
    prefix: rewards

  # Wallet balances and transactions use SQL so balance changes and transaction
  # inserts can commit atomically.
  wallets:
    kind: sql
    storage: wallet-db

  # Logical object store for pet species .pixa files only. The physical object
  # store is shared with other file payloads; this prefix keeps pet species
  # assets under pet-species/. The complete PetSpecies service binding is
  # composed in pet_species below together with the pet-species KV store.
  pet-species-assets:
    kind: objectstore
    storage: local-assets
    prefix: pet-species

  # Logical object store for badge icon files only. This keeps badge assets
  # under badges/. The complete Badge service binding is composed in badges
  # below together with the badges KV store.
  badge-assets:
    kind: objectstore
    storage: local-assets
    prefix: badges

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

# Service store bindings. These names must resolve to entries under "stores".
peers:
  store: peers

credentials:
  store: credentials

firmwares:
  store: firmwares

minimax:
  # Admin MiniMax tenant catalog store.
  tenants-store: minimax-tenants
  # Admin voice catalog store.
  voices-store: voices
  # Provider credential store used by MiniMax integrations.
  credentials-store: credentials

workspaces:
  store: workspaces

workflows:
  store: workflows

acl:
  store: acl

# Composite business bindings. The service config combines the metadata KV
# store with the asset object store used by that same resource.
pet_species:
  # Logical KV store for PetSpeciesObject JSON metadata.
  store: pet-species
  # Logical object store for PetSpeciesObject.pixa_path file bytes.
  assets_store: pet-species-assets

badges:
  # Logical KV store for BadgeObject JSON metadata.
  store: badges
  # Logical object store for BadgeObject.icon_path file bytes.
  assets_store: badge-assets

pets:
  # Logical KV store for adopted PetObject records.
  store: pets

rewards:
  # Logical KV store for RewardObject records.
  store: rewards

wallets:
  # Logical SQL store for WalletObject and WalletTransactionObject rows.
  store: wallets

# Peer social resource bindings.
contacts:
  # Logical KV store for ContactObject address-book records.
  store: contacts

friends:
  # Logical KV store for FriendRequestObject records.
  requests_store: friend-requests
  # Logical KV store for accepted FriendObject relationship records.
  store: friends
  # Lifetime of the 6-digit friend OTP reported by the target device through
  # peerrun.
  friend_otp_ttl: 10m

friend_groups:
  # Logical KV store for FriendGroupObject metadata.
  store: friend-groups
  # Logical KV store for FriendGroupMemberObject rows.
  members_store: friend-group-members
  # Logical KV store for FriendGroupMessageObject metadata.
  messages_store: friend-group-messages
  # Logical object store for friend group message audio bytes.
  message_assets_store: friend-group-message-assets
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

# Server-side system task configuration.
system_tasks:
  reward_claim:
    # GenX generator pattern used by reward.claim. "model/<id>" is a GenX
    # pattern, not a filesystem path. The id after "model/" is a Model admin
    # resource id, for example "model/qwen-flash".
    generator: model/qwen-flash
    # Minimum time between two reward.claim calls from the same peer.
    cooldown: 30m
  pet_action:
    # GenX generator pattern used by pet.feed, pet.wash, and pet.play.
    # The common setup uses the same model as reward_claim.
    generator: model/qwen-flash
```

## Field Notes

- `storage` contains physical backends. Currently supported `kind` values are
  `keyvalue`, `vecstore`, `objectstore`, `filesystem`, and `sql`.
  `filesystem` remains only for legacy file-store configs; new asset storage
  should use `kind: objectstore`.
- `stores` contains logical stores. Logical key/value stores should normally
  share a physical KV backend and isolate records with `prefix`.
- Logical object stores use `storage` plus `prefix` so multiple asset classes
  can share one physical object storage backend.
- Service sections such as `peers`, `credentials`, `firmwares`, `workspaces`,
  `workflows`, and `acl` bind a service to a logical store name.
- `minimax` uses separate logical stores for tenants, voices, and credentials.
- `system_tasks.*.generator` values must use `model/<model-id>`. The model id
  must match an admin `Model` resource, such as `qwen-flash`.
- `pet_species` and `badges` are composite service configs: `store` points to
  the logical KV metadata store, while `assets_store` points to the logical
  object asset store.
- `pets.store` and `rewards.store` hold peer-facing JSON records in logical KV
  stores.
- `wallets.store` is SQL-backed because wallet balance updates and transaction
  inserts must commit atomically.
- `contacts.store` stores current-peer address-book records. Contact objects
  are external contact data such as display name and phone number; they are not
  peer friend relationships.
- `friends.requests_store` stores friend request records, while `friends.store`
  stores accepted peer friend relationships.
- `friends.friend_otp_ttl` controls how long the 6-digit friend OTP reported by
  the target device through peerrun can be used for friend request creation.
- `friend_groups.store`, `friend_groups.members_store`, and `friend_groups.messages_store` store
  friend group metadata, membership rows, and friend group message metadata separately so
  list pagination and cleanup can be implemented independently.
- `friend_groups.message_assets_store` stores friend group message audio objects. Message
  records should store relative `audio_path` values under this logical object
  store, never absolute host filesystem paths.
- `friend_groups.message_default_ttl` is used when a friend group message send request omits
  TTL. `friend_groups.message_max_ttl` bounds user-provided TTL values.
- `friend_groups.message_cleanup_interval` controls the background task that removes
  expired friend group message metadata and deletes the referenced audio objects. The
  cleanup task should tolerate already-missing audio objects.
- `friend_groups.message_max_audio_bytes` limits the decoded audio payload accepted by
  friend group message send before writing bytes to the configured object store.
- `cipher-mode` accepts `chacha_poly`, `aes_256_gcm`, `plaintext`, or empty.
- Asset services should use object-store operations such as get, put, delete,
  delete-prefix, and list. They should not require directory creation or rename
  semantics that are awkward for OSS-style backends.
