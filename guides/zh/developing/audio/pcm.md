# pkgs/audio/pcm

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/pcm)

定义 GizClaw audio pipeline 使用的 PCM format、chunk、track、writer 和 mixer abstraction。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/pcm)

PCM package 不负责 codec、设备选择或网络 transport；这些能力通过 codec、portaudio 和 Peer media 层组合。

`Mixer.IdleDuration()` 累计 `Read` 无 track 时的等待时长，新 track 创建或读取退出时更新。已有 track
产生的静音不计入该值；发送 pacing 可比较相邻读取的差值，区分回复之间的空闲和活动音频的
处理延迟。该观测不改变 mixer 的静音、阻塞或 track 生命周期。
