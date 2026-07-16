# pkgs/audio/codecconv

Connect PCM, Ogg and Opus packages, provide Ogg Opus encode/decode and packet/header conversion.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/codecconv)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `PCMToOggOpusEncoder` / `NewPCMToOggOpusEncoder` | Incremental encoding PCM for Ogg Opus. |
| `OggToPCM` | Decode Ogg Opus to PCM with specified sample rate. |
| `OpusPacketsToOgg` / `OggOpusPackets` | Convert between raw Opus packets and Ogg stream. |
| `OpusPacketRTPTicks` | Calculate the RTP ticks corresponding to the packet duration. |
| `OpusHeadPacket` / `ParseOpusHeadPacket` | Construct or parse OpusHead. |
| `OpusTagsPacket` | Construct OpusTags metadata packet. |

This package only performs format conversion and does not determine media saving, network sending or playback strategies.
