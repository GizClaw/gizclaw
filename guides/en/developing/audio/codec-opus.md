# pkgs/audio/codec/opus

Provides Opus encoder/decoder and supported sample-rate contracts for voice, WebRTC media and Ogg Opus conversion.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/codec/opus)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Application` | Distinguish between VoIP, audio and low-delay encoder mode. |
| `OpusSampleRate` | Express sample rates supported by Opus. |
| `Encoder` / `NewEncoder` | Encode PCM frames into Opus packets. |
| `Decoder` / `NewDecoder` | Decode Opus packets to PCM. |
| `Version` | Returns native Opus runtime version. |
| `IsRuntimeSupported` | Determine whether the current build/runtime supports codec. |

This package does not own an Ogg container, RTP timestamp or WebRTC track lifecycle.
