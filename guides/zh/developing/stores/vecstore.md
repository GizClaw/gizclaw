# pkgs/store/vecstore

`pkgs/store/vecstore` 定义 vector similarity index，并提供精确内存索引和 HNSW approximate nearest-neighbor 实现。Agent memory 与 recall 使用它按 embedding 搜索相近内容。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/vecstore)

## 核心结构与实现

| 符号 | 作用 |
| --- | --- |
| `Index` | 定义 vector add、search、delete 和 index lifecycle。 |
| `Match` | 表达匹配 ID、distance 与 metadata。 |
| `Memory` / `NewMemory` | 提供进程内精确 vector search。 |
| `HNSW` / `NewHNSW` | 提供 HNSW approximate index。 |
| `HNSWConfig` | 配置 dimension、distance 与 graph parameters。 |
| `OpenHNSW` | 从 Object Store 打开或创建持久化 HNSW index。 |
| `LoadHNSW` / `LoadHNSWWithOptions` | 从 serialized stream 恢复 HNSW。 |
| `CosineDistance` | 计算 cosine distance。 |

## Ownership 边界

VecStore 不生成 embedding，也不决定模型、chunk 或 recall policy。Embedding dimension、normalization、resource ID、结果重排、object name 和保存时机属于调用方。
