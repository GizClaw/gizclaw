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
| `Prefixed` | 为已有 Store 增加固定 key namespace。 |
| `ListAfter` | 在 prefix 下从指定 key 之后分页读取。 |

## Ownership 边界

`kv` 只定义 byte payload 与层级 key 语义，不解释 payload 的领域类型。序列化、resource validation、secondary index 和跨记录一致性由使用它的领域 service 负责。调用方应使用稳定 prefix 隔离数据，不能依赖其他领域的内部 key layout。

## Server 组合

一个物理 `storage` entry 只打开一次 Memory 或 Badger。每个逻辑 `stores.kind: keyvalue` entry 引用该 connector，并可增加 slash-separated prefix。固定 `services` 字段再引用逻辑 Store；Store 名称和 prefix 都不会隐式选择 service。多个 service 可以显式复用同一个 Store。系统不提供 SQL-backed KV adapter 或通用 KV table。

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

Peer route 与 run state 使用代码内置 prefix，不再由 operator 分别绑定 Store。
