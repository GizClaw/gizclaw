# pkgs/store/vecid

`pkgs/store/vecid` 使用 locality-sensitive hashing 和 bucket clustering 为 vectors 建立稳定 identity。当前主要消费者是 audio voiceprint detector，用于把相近 speaker embeddings 归入已有 identity 或创建新 identity。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/vecid)

## 核心结构与实现

| 符号 | 作用 |
| --- | --- |
| `Config` | 配置 dimension、hash bits、distance 和 clustering behavior。 |
| `Registry` / `New` | 注册、匹配并维护 vector identities。 |
| `Bucket` | 保存 hash bucket 与 identity candidates。 |
| `Hasher` | 使用 deterministic random hyperplanes 生成 vector hash。 |
| `PlanesFile` | 表达可保存和恢复的 hash planes。 |
| `NewHasher` / `NewHasherFromPlanes` / `NewHasherFromJSON` | 创建或恢复 Hasher。 |
| `Store` | 定义 identity、bucket 和 compact state persistence。 |
| `MemoryStore` | 提供进程内 Registry store。 |

## Ownership 边界

VecID 不负责采集音频、生成 speaker embedding 或解释 identity 的用户含义。它与 `vecstore` 的目标不同：`vecstore` 返回相似 vector matches，`vecid` 维护可持续更新的 identity registry。
