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

Firmware artifacts, workspace history, Agent binary memory data, Gameplay pixa, and HNSW vector index persistence all use the Object Store.

## Ownership Boundary

The Object Store treats directories as an implementation detail and does not provide any filesystem operations. Resource metadata, content type, authorization, and version rules belong to the calling domain; objectstore only owns the binary object lifecycle.
