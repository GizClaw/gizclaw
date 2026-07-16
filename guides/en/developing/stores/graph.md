# pkgs/store/graph

`pkgs/store/graph` defines entity/relation graph abstraction and provides `KVGraph` implementation built on `pkgs/store/kv`. It is used for Agent memory and recall capabilities that require adjacency and relation traversal.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/graph)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Entity` | Stores graph entity identity, type and metadata. |
| `Relation` | Express source, target, relation type and metadata. |
| `Graph` | Define entity/relation write, read, delete and adjacency queries. |
| `KVGraph` | Use namespaced KV keys to save graph data and indexes. |
| `NewKVGraph` | Create graph with KV Store, prefix and optional separator. |

## Ownership Boundary

Graph package does not define Agent memory ontology, nor does it determine the business meaning of relation. Entity type, relation type, metadata schema, and traversal strategies belong to the calling domain. `KVGraph` relies on the caller to provide KV lifecycle and does not open, migrate or close the physical database.
