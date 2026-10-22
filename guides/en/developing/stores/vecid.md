# pkgs/store/vecid

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/store/vecid)

`pkgs/store/vecid` Use locality-sensitive hashing and bucket clustering to establish a stable identity for vectors. The current main consumer is the audio voiceprint detector, which is used to classify similar speaker embeddings into existing identities or create new identities.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/vecid)

## Ownership Boundary

VecID is not responsible for capturing audio, generating speaker embedding, or interpreting the user meaning of identity. It has different goals than `vecstore`: `vecstore` returns similar vector matches, `vecid` maintains a continuously updated identity registry.
