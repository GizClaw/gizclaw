# pkgs/audio/voiceprint

Provides speaker embedding and voice identity detection abstraction, supports ECAPA, ERes2Net and NCNN-backed model paths, and maintains matching identity through `vecid`.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/voiceprint)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Detector` | Define audio input, speaker detection and lifecycle. |
| `DetectorConfig` | Configure model, threshold and vector identity behavior. |
| `DetectResult` | Returns speaker identity, confidence and embedding result. |
| `DetectCallback` | Receive detection events. |
| `ConfidentGt` | Express confidence threshold callback. |
| `NewECAPA` / `NewERes2Net` | Create the corresponding speaker model detector. |

The Voiceprint package is responsible for signal feature, embedding model and identity detection, and does not own recording permissions, peer identity, user profile or biometric data retention policy; these are explicitly controlled by the calling product layer.
