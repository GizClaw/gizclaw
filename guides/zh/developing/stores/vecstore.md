# pkgs/store/vecstore

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/store/vecstore)

`pkgs/store/vecstore` 定义 vector similarity index，并提供精确内存索引和 HNSW approximate nearest-neighbor 实现。Agent memory 与 recall 使用它按 embedding 搜索相近内容。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/vecstore)

## Ownership 边界

VecStore 不生成 embedding，也不决定模型、chunk 或 recall policy。Embedding dimension、normalization、resource ID、结果重排、object name 和保存时机属于调用方。
