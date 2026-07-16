# pkgs/store/vecstore

`pkgs/store/vecstore` Defines vector similarity index and provides exact memory index and HNSW approximate nearest-neighbor implementation. Agent memory and recall use it to search for similar content by embedding.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/vecstore)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `Index` | Define vector add, search, delete and index lifecycle. |
| `Match` | The expression matches ID, distance and metadata. |
| `Memory` / `NewMemory` | Provides in-process accurate vector search. |
| `HNSW` / `NewHNSW` | Provide HNSW approximate index. |
| `HNSWConfig` | Configure dimension, distance and graph parameters. |
| `OpenHNSW` | Open or create a persistent HNSW index from the Object Store. |
| `LoadHNSW` / `LoadHNSWWithOptions` | Restore HNSW from serialized stream. |
| `CosineDistance` | Calculate cosine distance. |

## Ownership Boundary

VecStore does not generate embeddings, nor determine model, chunk or recall policy. Embedding dimension, normalization, resource ID, result rearrangement, object name and save timing belong to the caller.
