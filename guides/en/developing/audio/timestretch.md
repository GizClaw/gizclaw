# pkgs/audio/timestretch

Changes the duration of little-endian PCM16 audio without changing its pitch, for speech synthesis providers that have no native speaking-rate parameter.

## Core structure and main functions

| Symbol | Role |
| --- | --- |
| `New` | Creates a `Stretcher` for a sample rate, channel count and speed factor; speed 0.5 doubles the duration and 2 halves it. |
| `Stretcher.Write` | Accepts interleaved PCM16 in any chunk size and returns the stretched audio that is final so far; a trailing partial sample waits for the next call. |
| `Stretcher.Flush` | Returns the remaining output and resets the state; the total output length is the input length divided by the speed factor. |
| `Stretcher.Passthrough` | Returns input unchanged at speed 1. |

The implementation is streaming WSOLA (waveform similarity overlap-add): a 30ms Hann window with 50% overlap, searching ±8ms for the input segment that best continues the previous frame before overlap-adding it. The output does not depend on chunking; writing the same input byte by byte or at once produces identical output.

A Stretcher handles PCM only. It does not decode compressed audio or choose a rate: the rate comes from the Workspace `tts_speech_rate_percent`, and the transformer that needs the fallback (currently DashScope realtime) creates and flushes one Stretcher per reply. Providers with a native rate never use it.
