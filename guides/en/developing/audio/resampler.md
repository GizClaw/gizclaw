# pkgs/audio/resampler

Provides PCM sample-rate, channel and sample format conversion, current native implementation uses SoXR.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/resampler)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Format` | Description input/output sample rate, channels and encoding. |
| `Resampler` | Define streaming conversion and closing contracts. |
| `Soxr` | SoXR-backed implementation. |
| `New` | Create converter based on source reader and format at both ends. |

Resampler only converts PCM representation and is not responsible for decoding compressed audio, nor does it determine the target device or network format.
