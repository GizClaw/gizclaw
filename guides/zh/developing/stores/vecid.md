# pkgs/store/vecid

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/store/vecid)

`pkgs/store/vecid` uses locality-sensitive hashing and bucket clustering to establish stable identities for vectors. It matches similar embeddings to existing identities or creates new identities.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/vecid)

## Ownership 边界

VecID 不负责采集音频、生成 speaker embedding 或解释 identity 的用户含义。它与 `vecstore` 的目标不同：`vecstore` 返回相似 vector matches，`vecid` 维护可持续更新的 identity registry。
