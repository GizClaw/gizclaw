# pkgs/store/vecid

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/store/vecid)

`pkgs/store/vecid` 使用 locality-sensitive hashing 和 bucket clustering 为 vectors 建立稳定 identity。当前主要消费者是 audio voiceprint detector，用于把相近 speaker embeddings 归入已有 identity 或创建新 identity。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/vecid)

## Ownership 边界

VecID 不负责采集音频、生成 speaker embedding 或解释 identity 的用户含义。它与 `vecstore` 的目标不同：`vecstore` 返回相似 vector matches，`vecid` 维护可持续更新的 identity registry。
