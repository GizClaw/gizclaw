# pkgs/store/vecid

`pkgs/store/vecid` Use locality-sensitive hashing and bucket clustering to establish a stable identity for vectors. The current main consumer is the audio voiceprint detector, which is used to classify similar speaker embeddings into existing identities or create new identities.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/vecid)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `Config` | Configure dimension, hash bits, distance and clustering behavior. |
| `Registry` / `New` | Register, match and maintain vector identities. |
| `Bucket` | Stores hash bucket and identity candidates. |
| `Hasher` | Use deterministic random hyperplanes to generate vector hash. |
| `PlanesFile` | Expresses hash planes that can be saved and restored. |
| `NewHasher` / `NewHasherFromPlanes` / `NewHasherFromJSON` | Create or restore Hasher. |
| `Store` | Define identity, bucket and compact state persistence. |
| `MemoryStore` | Provides in-process Registry store. |

## Ownership Boundary

VecID is not responsible for capturing audio, generating speaker embedding, or interpreting the user meaning of identity. It has different goals than `vecstore`: `vecstore` returns similar vector matches, `vecid` maintains a continuously updated identity registry.
