# pkgs/audio/portaudio

提供 native PortAudio capture/playback backend，并把设备 stream 适配为 `pcm` formats 和 writers。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/portaudio)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Driver` / `NewDriver` | 管理 PortAudio backend lifecycle。 |
| `DeviceInfo` | 描述 capture/playback device。 |
| `StreamConfig` / `StreamConfigFromPCM` | 配置 device、format 与 frames per buffer。 |
| `CaptureStream` / `OpenCapture` | 打开 audio input。 |
| `PlaybackStream` / `OpenPlayback` | 打开 audio output。 |
| `PCMPlaybackWriter` / `OpenPCMPlaybackWriter` | 将 PCM chunks 写入 playback stream。 |
| `ListDevices` / `DefaultInputDevice` / `DefaultOutputDevice` | 查询设备。 |
| `NativeRuntimeSupported` / `BackendName` | 描述当前平台 backend availability。 |

平台和 CGO support 由 backend matrix 决定；unsupported build 必须返回明确能力状态或错误。

Native 阻塞式 read/write 在 stream 状态 mutex 外运行。`Stop` 与 `Close` 先禁止新 I/O、abort 活跃 native call、等待全部使用者退出，再关闭 native handle。`Close` 保持幂等，任何 PortAudio 函数都不能看到已释放 handle。Unit test 使用注入的 native operation；`portaudio_device` 测试为 opt-in，不支持的主机报告 unsupported，不能记作 pass。
