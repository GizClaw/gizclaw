# pkgs/store/graph

`pkgs/store/graph` 定义 entity/relation graph abstraction，并提供构建在 `pkgs/store/kv` 上的 `KVGraph` 实现。它用于需要邻接关系和 relation traversal 的 Agent memory 与 recall 能力。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/graph)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Entity` | 保存 graph entity identity、type 与 metadata。 |
| `Relation` | 表达 source、target、relation type 与 metadata。 |
| `Graph` | 定义 entity/relation 写入、读取、删除与邻接查询。 |
| `KVGraph` | 使用 namespaced KV keys 保存 graph 数据和 indexes。 |
| `NewKVGraph` | 以 KV Store、prefix 和可选 separator 创建 graph。 |

## Ownership 边界

Graph package 不定义 Agent memory ontology，也不判断 relation 的业务含义。Entity type、relation type、metadata schema 和 traversal 策略属于调用领域。`KVGraph` 依赖调用方提供 KV lifecycle，不打开、迁移或关闭 physical database。
