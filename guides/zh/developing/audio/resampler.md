# pkgs/audio/resampler

提供 PCM sample-rate、channel 和 sample format conversion，当前 native implementation 使用 SoXR。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/resampler)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Format` | 描述 input/output sample rate、channels 与 encoding。 |
| `Resampler` | 定义流式转换与关闭 contract。 |
| `Soxr` | SoXR-backed implementation。 |
| `New` | 根据 source reader 与两端 format 创建 converter。 |

Resampler 只转换 PCM representation，不负责 decode compressed audio，也不决定目标设备或网络 format。
