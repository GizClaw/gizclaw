# pkgs/store/kv

`pkgs/store/kv` 定义 GizClaw 的通用 ordered key-value abstraction。Key 使用 string segments 表达层级路径，Store 提供 get、set、delete、prefix list 和有序遍历能力。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/kv)

## 核心结构与实现

| 符号 | 作用 |
| --- | --- |
| `Key` / `Entry` | 表达分段 key 与读取结果。 |
| `Store` | 定义 CRUD、prefix listing 和 iterator contract。 |
| `Options` | 配置 key separator 等 store 行为。 |
| `Memory` / `NewMemory` | 进程内 ordered store。 |
| `Badger` / `NewBadger` | Badger-backed persistent implementation。 |
| `SQL` / `NewSQLWithDB` | 借用 SQLite/PostgreSQL pool 的 table-scoped implementation。 |
| `Prefixed` | 为已有 Store 增加固定 key namespace。 |
| `ListAfter` | 在 prefix 下从指定 key 之后分页读取。 |

## Ownership 边界

`kv` 只定义 byte payload 与层级 key 语义，不解释 payload 的领域类型。序列化、resource validation、secondary index 和跨记录一致性由使用它的领域 service 负责。调用方应使用稳定 prefix 隔离数据，不能依赖其他领域的内部 key layout。

## Server 组合

一个 `storage.kind: badger` entry 只打开一个 `*badger.DB`；逻辑 Store 用 `NewBadgerWithDB` 借用它。`storage.kind: memory` 只是 marker，每个逻辑 keyvalue Store 创建独立 `*kv.Memory`。SQLite/PostgreSQL keyvalue Store 必须声明独立 `table`，保留 ordered prefix、native paging、deadline、batch、conditional create 与 compare-and-mutate 的数据库事务语义；过期 row 在读取或后续成功 mutation 时清理，不运行后台 goroutine。它只保存 opaque bytes，不会自动替代领域 SQL repository。每个 Store 还可增加 slash-separated prefix；固定 `services` 字段再引用逻辑 Store。

```yaml
storage:
  state:
    kind: badger
    dir: data/kv
stores:
  peer-records:
    kind: keyvalue
    storage: state
    prefix: peers
services:
  peer:
    store: peer-records
```

SQLite/PostgreSQL 配置把同一个逻辑声明改为 `storage: database` 并增加 `table: gizclaw_peer_records`。表名不能和同一 connector 上的 Metrics 或 Log Store 重复；关闭逻辑 Store 不会关闭共享数据库 pool。

Peer route 与 run state 使用代码内置 prefix，不再由 operator 分别绑定 Store。
