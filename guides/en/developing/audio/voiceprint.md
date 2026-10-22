# pkgs/audio/voiceprint

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/voiceprint)

Provides speaker embedding and voice identity detection abstraction, supports ECAPA, ERes2Net and NCNN-backed model paths, and maintains matching identity through `vecid`.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/voiceprint)

The Voiceprint package is responsible for signal feature, embedding model and identity detection, and does not own recording permissions, peer identity, user profile or biometric data retention policy; these are explicitly controlled by the calling product layer.
