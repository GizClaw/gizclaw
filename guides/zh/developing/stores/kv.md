# pkgs/store/kv

`pkgs/store/kv` 提供精确 key 的读写、删除，以及明确集合 key 下的成员操作和有界有序范围读取。Key 使用 string segments 表达命名空间；不提供数据库 key 的通用枚举。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/kv)

## 核心结构与实现

| 符号 | 作用 |
| --- | --- |
| `Key` / `Entry` | 表达分段 key 与读取结果。 |
| `Store` | 定义精确 CRUD、集合操作与原子 mutation contract。 |
| `Options` | 配置 key separator 等 store 行为。 |
| `Memory` / `NewMemory` | 进程内 ordered store。 |
| `Badger` / `NewBadger` | Badger-backed persistent implementation。 |
| `SQL` / `NewSQLWithDB` | 借用 SQLite/PostgreSQL pool；把逻辑 prefix 映射为物理表。 |
| `Redis` / `NewRedisWithClient` | 借用单节点 Redis client，并保留完整的有序与原子 Store contract。 |
| `Prefixed` | 为已有 Store 增加固定 key namespace。 |

## Ownership 边界

`kv` 只定义 byte payload 与层级 key 语义，不解释 payload 的领域类型。序列化、resource validation、secondary index 和跨记录一致性由使用它的领域 service 负责。调用方应使用稳定 prefix 隔离数据，不能依赖其他领域的内部 key layout。

## Server 组合

一个 `storage.kind: badger` entry 只打开一个 `*badger.DB`；逻辑 Store 用 `NewBadgerWithDB` 借用它。`storage.kind: memory` 只是 marker，每个逻辑 keyvalue Store 创建独立 `*kv.Memory`。SQLite/PostgreSQL keyvalue Store 只声明一个必填的单段 `prefix`，后端直接把它用作带引号的物理表名，不接受 `table` 字段，也不会在表内 encoded key 上再次添加该 prefix。SQL 实现保留 deadline、batch、conditional create、compare-and-mutate 与集合操作的数据库事务语义；过期 row 在读取或后续成功 mutation 时清理，不运行后台 goroutine。它只保存 opaque bytes，不会自动替代领域 SQL repository。固定 `services` 字段引用这个逻辑 Store。

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

SQLite/PostgreSQL 配置只把同一个逻辑声明改为 `storage: database`；此时 `prefix: peers` 同时是后端物理表名。这个名称不能和同一 connector 上其他 KV prefix 或 Metrics/Log table 重复；关闭逻辑 Store 不会关闭共享数据库 pool。

同一物理 connector 上的 Redis keyvalue Store 必须使用非空、规范且两两不重叠的 prefix。Adapter 使用精确 key 与集合操作，使用绝对 deadline，并在单个 Redis 节点上原子实现 batch、conditional create 与 compare-and-mutate；零 deadline 会移除已有 expiration。由于 Store contract 要求任意 key 原子性，不支持 Redis Cluster 或多 endpoint 分片。

Peer record 与 route 共用 `services.peer.store` 指定的 Store。PeerRun 状态和每台 Server 的 Admin Peer 列表使用本机业务 SQL 表，不枚举中心 Redis。

PostgreSQL mutation 按物理表与 encoded key 获取事务级 advisory lock，并按 lock ID 排序，覆盖不存在的 guard；普通写入、删除和条件 mutation 使用同一协议，不再获取表级写锁。不同 key 可以并发，同 key 与多 key 原子操作保持协调。哈希碰撞只增加串行等待，不改变正确性。PostgreSQL mutation 只清理本次涉及的过期 key，读取仍把过期值视为不存在，并在同一个 key advisory lock 下尝试定点物理清理；获得锁后重新检查过期条件，保留并发刷新值，取消或数据库错误不改变逻辑不存在的结果；SQLite 保留原有事务与过期清理行为。

## 精确集合与原子索引更新

`AddMembers`、`RemoveMembers`、`HasMember`、`ListMembers` 只操作一个完整集合 key；成员是 opaque string，不按 key separator 拆分。添加和删除幂等，缺失集合返回空列表，列表顺序不保证。`ListMembers` 读取整个指定集合，调用方必须按业务资源划分集合，不能建立全站大集合。成员判断使用 `HasMember`，不先读取列表。

`ApplyMutation` 在一次原子操作中处理 `Conditions`、记录写入、key 删除及成员增删。条件中的 `Expected == nil` 表示必须不存在，非 nil 表示记录字节必须相等。条件失败返回 false 且没有写入；操作依次写记录、删 key、加成员、删成员。同一个 mutation 的记录 key 和集合 key 必须分离。集合不设置过期时间，最后一个成员删除后集合消失。

Redis 使用原生 Set 和 Lua 原子操作。Badger 使用独立的记录／成员物理命名空间和事务，成员判断是精确 key 查询。SQL 使用记录类型列及独立成员表，联合主键为 `(encoded_key, member)`；事务锁覆盖记录和集合 key。`Prefixed` 同时限定记录、条件和集合 key，不修改成员 ID。

## 有序集合范围查询

`RangeOrderedMembers` 在一个完整集合 key 内按成员字节升序读取，`After` 和 `Before` 为可选的排他边界，`Limit` 必须为正数。成员仍是 opaque string；时间索引由业务层编码为固定宽度时间与资源 ID。它不会解释 key 前缀，也不会枚举其他集合。

有序集合通过 `ApplyMutation.AddOrderedMembers`／`RemoveOrderedMembers` 与业务记录原子增删，和普通 Set、byte value 是不同类型；混用返回 `ErrWrongType`。Redis 使用所有 score 为零的 Sorted Set，并将字典序范围和 LIMIT 下推到 Redis。Badger 使用集合成员 key 的有界 seek；SQL 使用 `(encoded_key, member)` 索引与范围谓词、ORDER BY、LIMIT。Memory 为进程内实现，会在该集合内排序。集合规模仍需由业务范围或分片控制。
