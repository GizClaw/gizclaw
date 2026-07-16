# pkgs/audio/portaudio

Provide native PortAudio capture/playback backend, and adapt the device stream to `pcm` formats and writers.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/portaudio)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Driver` / `NewDriver` | Manage PortAudio backend lifecycle. |
| `DeviceInfo` | Description capture/playback device. |
| `StreamConfig` / `StreamConfigFromPCM` | Configure device, format and frames per buffer. |
| `CaptureStream` / `OpenCapture` | Open audio input. |
| `PlaybackStream` / `OpenPlayback` | Open audio output. |
| `PCMPlaybackWriter` / `OpenPCMPlaybackWriter` | Write PCM chunks to the playback stream. |
| `ListDevices` / `DefaultInputDevice` / `DefaultOutputDevice` | Query the device. |
| `NativeRuntimeSupported` / `BackendName` | Describe the current platform backend availability. |

Platform and CGO support are determined by the backend matrix; unsupported builds must return explicit capability status or errors.
