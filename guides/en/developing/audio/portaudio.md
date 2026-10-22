# pkgs/audio/portaudio

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/portaudio)

Provide native PortAudio capture/playback backend, and adapt the device stream to `pcm` formats and writers.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/portaudio)

Platform and CGO support are determined by the backend matrix; unsupported builds must return explicit capability status or errors.

Native reads and writes run outside the stream state mutex and query PortAudio availability before issuing a bounded 20 ms transfer quantum, so the blocking API is never asked to wait for a caller-sized buffer. `Stop` and `Close` first prevent new I/O, abort the active stream, wait for polling/transfers to drain, and only then close the native handle. `Close` is idempotent and no PortAudio function may observe a freed handle. Unit tests use injected native operations; `portaudio_device` helper subprocesses bound real capture/playback cancellation and report an unsupported host as SKIP rather than PASS.
