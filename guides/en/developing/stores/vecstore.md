# pkgs/store/vecstore

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/store/vecstore)

`pkgs/store/vecstore` Defines vector similarity index and provides exact memory index and HNSW approximate nearest-neighbor implementation. Agent memory and recall use it to search for similar content by embedding.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/vecstore)

## Ownership Boundary

VecStore does not generate embeddings, nor determine model, chunk or recall policy. Embedding dimension, normalization, resource ID, result rearrangement, object name and save timing belong to the caller.
