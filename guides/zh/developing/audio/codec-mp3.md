# pkgs/audio/codec/mp3

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/codec/mp3)

提供 MP3 stream decode，以及在受支持平台上的 PCM-to-MP3 encode。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3)

Encoder availability 受 build target 与 native dependency 约束；不支持的平台返回明确错误，不能静默生成伪输出。
