# pkgs/store/objectstore

`pkgs/store/objectstore` Definition prefix-addressable binary object storage. Object name is a slash-separated key; the caller can read and write a single object, enumerate or delete by prefix, and set a deadline or TTL for the object.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/objectstore)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `ObjectStore` | Define Get, Put, expiration, Delete, DeletePrefix and List. |
| `ObjectInfo` | Returns object name, size and deadline. |
| `LocalDirProvider` | Allows callers to identify the local filesystem backend. |
| `Dir` | Path-based convenience entrypoint; transiently opens an `os.Root` and delegates to `Root`; a `Get` reader closes that Root when the reader closes. |
| `Root` / `NewRoot` | The single filesystem ObjectStore implementation; borrows a physical `*os.Root` for rooted operations without closing it. |

## Main purpose

Workspace history, Agent binary memory data, Gameplay pixa, and HNSW vector index persistence use the Object Store. Firmware OTA packages are external HTTPS resources and are not stored here.

## Ownership Boundary

The Object Store treats directories as an implementation detail and does not provide any filesystem operations. Resource metadata, content type, authorization, and version rules belong to the calling domain; objectstore only owns the binary object lifecycle.

## Server composition

`storage` opens and owns one physical `*os.Root`. Logical ObjectStores borrow the same rooted handle and select an object prefix; `os.Root` rejects absolute paths, `..`, and escaping symlinks. When several logical Stores share one connector, every prefix must be non-empty, clean, and non-overlapping. Asset and AgentHost consumers are then bound explicitly through `services`; closing a scoped Store does not close or invalidate the physical connector.

```yaml
storage:
  files:
    kind: filesystem.dir
    dir: data/files
stores:
  workspace-assets:
    kind: objectstore
    storage: files
    prefix: workspaces
services:
  workspace:
    store: workspaces
    assets_store: workspace-assets
```
