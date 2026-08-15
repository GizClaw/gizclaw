# pkgs/store/objectstore

`pkgs/store/objectstore` defines provider-neutral, prefix-addressable binary
object storage. An object name is a relative slash-separated key. Callers can
stream one object, list or delete a prefix, and attach a deadline or TTL.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/objectstore)

## Contract and ownership

| Symbol | Function |
| --- | --- |
| `ObjectStore` | Get, streaming overwrite, expiration, idempotent Delete, DeletePrefix, and List |
| `ObjectInfo` | Object name, size, and deadline |
| `Root` / `NewRoot` | Borrows a rooted local filesystem handle |
| `NewVolcTOS`, `NewAliyunOSS`, `NewGCS`, `NewAzureBlob` | Borrow official cloud SDK clients owned by physical Storage |
| `LocalDirProvider` | Identifies only local filesystem-backed stores |

Names are relative normalized slash keys. Empty object names, absolute paths,
parent traversal, and the reserved `.objectstore-meta` namespace are rejected.
Put replaces one exact object and returns only after the backend confirms the
write. Get on a missing or expired object matches `fs.ErrNotExist`; Delete is
idempotent. List consumes all provider pages, filters on a complete path
segment, and returns deterministic lexical order.

Expiration is GizClaw-owned metadata named `gizclaw-deadline` on cloud objects.
A zero deadline removes it. Get and List hide expired objects and attempt to
delete them. The filesystem backend keeps equivalent private sidecar metadata.
Callers must not depend on provider lifecycle rules for this contract.

Resource content type, authorization, naming policy, and version rules remain
with the calling domain. ObjectStore never creates buckets or containers,
serves public URLs, or exposes provider SDK types to services.

## Server composition

`storage` opens and owns the physical root or official SDK client, performs a
30-second read-only readiness probe, and closes transports after logical Stores.
The logical `objectstore` borrows it and applies its configured prefix once.
Non-overlapping logical prefixes may share one connector; the registry rejects
equal or parent/child overlap before listeners open.

```yaml
storage:
  profile-files:
    kind: volc-tos
    endpoint: https://tos-cn-beijing.volces.com
    region: cn-beijing
    bucket: example-profiles
    access_key_id: ${VOLC_TOS_ACCESS_KEY_ID}
    access_key_secret: ${VOLC_TOS_ACCESS_KEY_SECRET}
    session_token: ${VOLC_TOS_SESSION_TOKEN}
stores:
  runtime-profiles:
    kind: objectstore
    storage: profile-files
    prefix: pprof
```

Supported physical configurations are:

| Kind | Required fields | Authentication |
| --- | --- | --- |
| `filesystem.dir` | `dir` | process filesystem permissions |
| `volc-tos` | `endpoint`, `region`, `bucket`, `access_key_id`, `access_key_secret` | optional `session_token` |
| `aliyun-oss` | `endpoint`, `bucket`, `access_key_id`, `access_key_secret` | optional `security_token` |
| `gcs` | `bucket` | ADC, or optional `credentials_file` |
| `azure-blob` | `account_url`, `container` | `DefaultAzureCredential` (managed/workload identity, environment, or developer credentials) |

Buckets and containers must already exist. Production endpoints and account
URLs must use HTTPS. `${VAR}` expansion and workspace-relative resolution of a
GCS credentials file happen in `cmd/internal/server`; credential contents are
never logged. A configured GCS credentials path must name a readable regular
file. Azure credentials are not accepted in YAML.

Every cloud operation has a fixed 30-second bound and bounded SDK retries.
Alibaba OSS List performs a metadata request for each returned object because
its list response does not carry custom metadata; this has request and billing
cost. Other adapters consume metadata returned by their list APIs. DeletePrefix
also lists every page and deletes objects individually, so operators should use
a dedicated prefix and account for provider request costs.

Provider errors exposed in normal error text omit credentials, authorization,
signed query parameters, and raw response bodies. Do not log wrapped provider
errors or enable SDK wire logging in production. Cleanup and retention failures
must be treated as incomplete operations and retried by the owning feature.

## Main uses

Workspace and Gameplay assets, Agent Host runtime data, HNSW persistence, and
Server process profiles use ObjectStore. Firmware OTA packages remain external
HTTPS resources and are not stored here.
