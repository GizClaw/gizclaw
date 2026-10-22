# pkgs/audio/voiceprint

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/voiceprint)

提供 speaker embedding 与 voice identity detection abstraction，支持 ECAPA、ERes2Net 和 NCNN-backed model paths，并通过 `vecid` 维护匹配 identity。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/voiceprint)

Voiceprint package 负责 signal feature、embedding model 和 identity detection，不拥有录音权限、Peer identity、用户 profile 或生物特征数据 retention policy；这些由调用产品层明确控制。
