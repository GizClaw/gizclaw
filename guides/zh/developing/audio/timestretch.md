# pkgs/audio/timestretch

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/timestretch)

在不改变音高的前提下改变 little-endian PCM16 音频的时长，用于没有原生语速参数的语音合成 provider。

实现是流式 WSOLA（waveform similarity overlap-add）：30ms Hann 窗、50% 重叠，在 ±8ms 范围内寻找与上一帧自然延续最相似的输入片段后叠加。输出与输入的分块方式无关，同一段输入逐字节写入与一次写入得到相同结果。

Stretcher 只处理 PCM，不解码压缩音频，也不决定语速；语速来自 Workspace `tts_speech_rate_percent`，由需要兜底的 transformer（当前是 DashScope realtime）按回复逐段创建和 flush。已有原生语速的 provider 不经过它。
