# pkgs/audio/timestretch

在不改变音高的前提下改变 little-endian PCM16 音频的时长，用于没有原生语速参数的语音合成 provider。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `New` | 按 sample rate、声道数和速度因子创建 `Stretcher`；速度 0.5 输出两倍时长，2 输出一半时长。 |
| `Stretcher.Write` | 接收任意分块的交错 PCM16，返回目前已经确定的伸缩结果，末尾不足一个 sample 的字节留到下一次。 |
| `Stretcher.Flush` | 返回剩余输出并重置状态；整段输出长度等于输入长度除以速度因子。 |
| `Stretcher.Passthrough` | 速度为 1 时原样返回输入。 |

实现是流式 WSOLA（waveform similarity overlap-add）：30ms Hann 窗、50% 重叠，在 ±8ms 范围内寻找与上一帧自然延续最相似的输入片段后叠加。输出与输入的分块方式无关，同一段输入逐字节写入与一次写入得到相同结果。

Stretcher 只处理 PCM，不解码压缩音频，也不决定语速；语速来自 Workspace `tts_speech_rate_percent`，由需要兜底的 transformer（当前是 DashScope realtime）按回复逐段创建和 flush。已有原生语速的 provider 不经过它。
