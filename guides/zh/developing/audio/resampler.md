# pkgs/audio/resampler

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/resampler)

提供 signed PCM16 little-endian 的流式采样率转换与 mono/stereo 转换，使用纯 Go polyphase FIR，不依赖 cgo 或 libsoxr。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/resampler)

Resampler 只转换 PCM representation，不负责 decode compressed audio，也不决定目标设备或网络 format。调用方应在连续音频流中复用 converter；改变 input format 时创建新实例，结束后调用 `Close`。同一个实例不支持并发 `Read`。

## 采样与边界

在连续 PCM 流中复用 converter，输入格式变化时重新创建，消费结束后关闭。采样率转换与声道转换可以组合；调用方仍负责选择目标设备需要的格式，并处理输入截断与输出 buffer 不足。

滤波器、输出帧数计算和支持范围直接查阅 [resampler 源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/resampler)。

## 验证

`go test ./pkgs/audio/resampler` 覆盖 frame 数、任意 chunk 边界、声道隔离、频率响应、空流、EOF 与错误路径。纯 Go 路径可用 `CGO_ENABLED=0 go test ./pkgs/audio/resampler` 验证。

`BenchmarkSpeech` 使用真实语音测量完整流（含创建、60 ms 输出读取与 drain），通过 `GIZCLAW_RESAMPLER_SPEECH_DIR` 指定 fixture 目录。Fixture 为 `RATE-CHANNELS.pcm` 命名的 PCM16LE 文件；各文件应来自同一段语音并保留相同时长。以 benchmark case 列表为准准备所有 rate/channel 组合，使用 `go test ./pkgs/audio/resampler -run '^$' -bench BenchmarkSpeech -benchtime=2s -count=5`。与其他性能测试共享主机时应在计时阶段持有共同的 benchmark lock，并记录 load average 与重复测量差异。
