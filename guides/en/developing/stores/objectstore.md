# pkgs/store/objectstore

`pkgs/store/objectstore` Definition prefix-addressable binary object storage. Object name is a slash-separated key; the caller can read and write a single object, enumerate or delete by prefix, and set a deadline or TTL for the object.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/objectstore)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `ObjectStore` | Define Get, Put, expiration, Delete, DeletePrefix and List. |
| `ObjectInfo` | Returns object name, size and deadline. |
| `LocalDirProvider` | Allows callers to identify the local filesystem backend. |
| `Dir` | Securely map object keys to specified directories and maintain expiration metadata. |

## Main purpose

Workspace history, Agent binary memory data, Gameplay pixa, and HNSW vector index persistence use the Object Store. Firmware OTA packages are external HTTPS resources and are not stored here.

## Ownership Boundary

The Object Store treats directories as an implementation detail and does not provide any filesystem operations. Resource metadata, content type, authorization, and version rules belong to the calling domain; objectstore only owns the binary object lifecycle.

## Server composition

`storage` owns the physical filesystem directory. Logical ObjectStores borrow it and select an object prefix. When several logical Stores share one connector, every prefix must be non-empty, clean, and non-overlapping. Asset and AgentHost consumers are then bound explicitly through `services`; closing a scoped Store does not close or invalidate the physical connector.

```yaml
storage:
  files:
    kind: objectstore
    fs:
      dir: data/files
stores:
  workspace-assets:
    kind: objectstore
    storage: files
    prefix: workspaces
services:
  workspace:
    store: workspaces
    workflow_store: workflows
    assets_store: workspace-assets
```
