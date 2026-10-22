# pkgs/audio/timestretch

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/timestretch)

Changes the duration of little-endian PCM16 audio without changing its pitch, for speech synthesis providers that have no native speaking-rate parameter.

The implementation is streaming WSOLA (waveform similarity overlap-add): a 30ms Hann window with 50% overlap, searching ±8ms for the input segment that best continues the previous frame before overlap-adding it. The output does not depend on chunking; writing the same input byte by byte or at once produces identical output.

A Stretcher handles PCM only. It does not decode compressed audio or choose a rate: the rate comes from the Workspace `tts_speech_rate_percent`, and the transformer that needs the fallback (currently DashScope realtime) creates and flushes one Stretcher per reply. Providers with a native rate never use it.
