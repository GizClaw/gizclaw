# pkgs/store/vecid

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/store/vecid)

`pkgs/store/vecid` uses locality-sensitive hashing and bucket clustering to establish stable identities for vectors. It matches similar embeddings to existing identities or creates new identities.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/vecid)

## Ownership Boundary

VecID is not responsible for capturing audio, generating speaker embedding, or interpreting the user meaning of identity. It has different goals than `vecstore`: `vecstore` returns similar vector matches, `vecid` maintains a continuously updated identity registry.
