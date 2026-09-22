# pkgs/audio/resampler

提供 signed PCM16 little-endian 的流式采样率转换与 mono/stereo 转换，使用纯 Go polyphase FIR，不依赖 cgo 或 libsoxr。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/resampler)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Format` | 描述 input/output sample rate 与 mono/stereo。 |
| `Resampler` | 定义流式转换与关闭 contract。 |
| `Soxr` | 纯 Go implementation，类型名保留用于 source compatibility。 |
| `New` | 根据 source reader 与两端 format 创建 converter。 |

Resampler 只转换 PCM representation，不负责 decode compressed audio，也不决定目标设备或网络 format。调用方应在连续音频流中复用 converter；改变 input format 时创建新实例，结束后调用 `Close`。同一个实例不支持并发 `Read`。

## 采样与边界

- 相同采样率直接传递 PCM，仅在需要时转换声道。Stereo 转 mono 对左右声道取平均，mono 转 stereo 复制样本；重采样时各声道独立。
- 不同采样率采用 Kaiser-windowed sinc（beta 9），截止频率为两端较低 Nyquist 的 94%。Filter 长度为 `2 * ceil(48 * max(1, inputRate/outputRate)) + 1`；24→16 kHz 使用两个有效 phase、每个 output sample 145 次乘加。
- Integer phase clock 避免累计时间漂移。小分母使用精确 phase；大分母在相邻预计算 phase 间插值。Phase bank 最多 256 个区间，系数总数限制在 262144 内，避免大分母导致无界内存使用。
- Filter 需要未来半个窗口的样本，24→16 kHz 为 3 ms。流首尾按零扩展，EOF 时输出剩余帧并去掉 filter delay；输入 `N` 帧得到 `ceil(N * outputRate/inputRate)` 帧，不附加静音尾巴。
- 支持非整帧 transport chunk；不完整的最终 PCM frame 返回 `io.ErrUnexpectedEOF`。输出 buffer 必须至少容纳一个完整目标 frame。
- 采样率必须在 `[1, 2147483647]`，输出/输入采样率比例在 `[1/256, 256]`。非整数比率不要求来自固定采样率列表。

## 验证

`go test ./pkgs/audio/resampler` 覆盖 frame 数、任意 chunk 边界、声道隔离、频率响应、空流、EOF 与错误路径。纯 Go 路径可用 `CGO_ENABLED=0 go test ./pkgs/audio/resampler` 验证。

`BenchmarkSpeech` 使用真实语音测量完整流（含创建、60 ms 输出读取与 drain），通过 `GIZCLAW_RESAMPLER_SPEECH_DIR` 指定 fixture 目录。Fixture 为 `RATE-CHANNELS.pcm` 命名的 PCM16LE 文件；各文件应来自同一段语音并保留相同时长。以 benchmark case 列表为准准备所有 rate/channel 组合，使用 `go test ./pkgs/audio/resampler -run '^$' -bench BenchmarkSpeech -benchtime=2s -count=5`。与其他性能测试共享主机时应在计时阶段持有共同的 benchmark lock，并记录 load average 与重复测量差异。
