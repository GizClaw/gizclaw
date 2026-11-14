# pkgs/store/kv

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/store/kv)

`pkgs/store/kv` 提供精确 key 的读写、删除，以及明确集合 key 下的成员操作和有界有序范围读取。Key 使用 string segments 表达命名空间；不提供数据库 key 的通用枚举。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/kv)

## Ownership 边界

`kv` 只定义 byte payload 与层级 key 语义，不解释 payload 的领域类型。序列化、resource validation、secondary index 和跨记录一致性由使用它的领域 service 负责。调用方应使用稳定 prefix 隔离数据，不能依赖其他领域的内部 key layout。

## 批量点查

调用方通过 `store.BatchGet(ctx, keys)` 读取一组明确的 byte record key，不枚举数据库。返回的 `GetResult` 与输入逐项对应，保留顺序、重复 key 和空 byte value；成功值的内存互相独立。缺失、过期或指向集合的 key 均返回 `ErrNotFound`，其他读取与解码错误逐项返回。单键 `Get` 遇到集合 key 仍返回 `ErrWrongType`。`Prefixed` 和 `WithStrongReads` 直接转发批量读取，保留 namespace 与读取 authority。

`BatchGet` 是 `Store` 的必需方法。TiKV 为每次批量调用获取一次时间戳并创建一个快照，将物理 record key 去重后一次交给 SDK `BatchGet`，由 SDK 按 Region 和请求大小拆分并行读取；返回结果仍与原输入逐项对应。事务内 mutation 预取也按物理 key 去重，条件检查与 guard 写回保持不变，避免重复读取同一残留锁导致重复解锁。Redis 在选定读连接上发出一条原生 `MGET`，非 string key 与不存在的 key 都作为 `ErrNotFound` 返回。Memory、Badger 和 SQL 实现在 backend 内最多八路并发调用各自的单键读取，不新建连接；调用层不再探测可选批量接口。

所有实现都继承 caller context，不为内部请求重置超时；backend 的有界单键读取在取消后不再启动排队 key 的存储读取。Read/write-separated backends merge committed local postimages into ordinary results; a backend read failure is propagated for keys not satisfied by those postimages. Cancellation remains an error for every affected key. Redis `MGET` 是单命令原子读取，TiKV 使用单次历史或强读快照；批量 contract 不承诺所有 backend 都使用原子快照，也不把多次 `BatchGet` 合成同一个快照。产品层负责设置整页预算，KV 层不内置业务超时。

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

SQLite/PostgreSQL 配置只把同一个逻辑声明改为 `storage: database`；此时 `prefix: peers` 同时是后端物理表名。这个名称不能和同一 connector 上其他 KV prefix 或 Metrics table 重复；关闭逻辑 Store 不会关闭共享数据库 pool。

同一物理 connector 上的 Redis keyvalue Store 必须使用非空、规范且两两不重叠的 prefix。Adapter 使用精确 key 与集合操作，使用绝对 deadline，并在单个 Redis 节点上原子实现 batch、conditional create 与 compare-and-mutate；零 deadline 会移除已有 expiration。由于 Store contract 要求任意 key 原子性，不支持 Redis Cluster 或多 endpoint 分片。

### Redis 读写分离

`storage.kind: redis.readwrite` 通过 `read_url` 和 `write_url` 选择同一复制组的两个入口，逻辑 Store 仍为 `kind: keyvalue`，业务继续使用 `kv.Store`。普通 `storage.kind: redis` 保持一个连接处理全部操作；没有副本的部署使用普通 backend，无需更改业务绑定。配置示例见 [Store 总览](./overview)。

`Get`、`BatchGet`、`HasMember`、`ListMembers`、`RangeOrderedMembers` 默认使用读连接。`Set`、`Delete`、`BatchSet`、`BatchDelete`、`BatchMutate`、集合修改、conditional create、compare-and-mutate 和 `ApplyMutation` 全部使用写连接；Lua 内的条件读取与写入在主节点原子执行。同一个读写分离 connector 下的逻辑 Store 共用原子 root 和 prefix 隔离规则。逻辑 Store 的 `Close` 不关闭连接。

`kv.WithPrimaryReads(ctx)` selects the write authority for bounded update recovery after a rejected condition or an observation inconsistent with a known successful write. A missing target may be refreshed within a mutation such as API Key revocation. Ordinary queries, including authentication negatives and PendingDeletion administration/discovery, retain the configured reader with backend-local write visibility; writes from other backend instances may still produce stale absence or errors. The fallback preserves cancellation, deadlines and prefixes, does not pin unrelated requests, and does not combine reads into one snapshot. Final writes still compare their conditions atomically.

Read/write separation permits stale observations of writes made by other backend instances. This instance retains committed local record and collection postimages, including deletions and absolute expiration deadlines, until its reader proves that it covers them. Mutation conditions remain atomic on the writer, and failed conditions or uncertain commits do not publish attempted values. Ordinary Redis stores that read and write one endpoint do not enable this layer. The public `kv.Store` interface and stored business values are unchanged; separate reads still do not form one snapshot.

Read/write-separated Redis requires Redis 7 or newer for exact absolute expiration metadata; unsupported expiration commands are rejected before business mutations run. Redis retirement uses replication identity and offset metadata from the writer and a same-attempt reader query. Deployment must allow `INFO replication` for progress tracking. Missing progress proof retains pending local postimages rather than assuming a fixed replication delay. After covered postimages are reclaimed, the backend retains a compact required replication position; a lagging, replaced or unverifiable reader returns an error for uncovered keys instead of returning state older than retired local writes. Reads never fall back to the writer automatically. Both URLs must address the same logical database and replication group, and deployment owns endpoint topology. Ordinary reads opportunistically reclaim all covered pending postimages; write-only workloads retain them until an ordinary read runs. No reader network request is added to the write path and no hidden worker is started. Process restart creates a new local consistency scope; the layer does not provide cross-process or cross-Server read-your-writes.

Peer record 与 route 共用 `services.peer.store` 指定的 Store。PeerRun 状态和每台 Server 的 Admin Peer 列表使用本机业务 SQL 表，不枚举中心 Redis。

### TiKV

`storage.kind: tikv` 通过 `pd_addresses` 中的 `host:port` 列表发现集群，使用 Transactional KV API。逻辑 `keyvalue` Store 使用必填、规范且互不重叠的 `prefix`，同一 connector 下共享一个跨 Region 原子 root。所有写入、条件读取和 collection counter 更新都使用新的中心事务；guard 也参与写冲突检测，避免 Snapshot Isolation 的跨 key write skew。只有明确的 write conflict 会有限重试，不重试提交结果不确定的错误。

```yaml
storage:
  shared:
    kind: tikv
    pd_addresses: [pd-1.example.com:2379, pd-2.example.com:2379, pd-3.example.com:2379]
    tls_ca_file: /run/secrets/tikv-ca.pem
    tls_cert_file: /run/secrets/tikv-client.pem
    tls_key_file: /run/secrets/tikv-client-key.pem
    read_labels:
      region: local
stores:
  peers:
    kind: keyvalue
    storage: shared
    prefix: peers
services:
  peer:
    store: peers
```

Configure `read_labels` alone to enable regional reads; omit it or use an empty map for strong reads throughout. Ordinary data reads go directly to label-matching Learners using native TiKV replica-read synchronization. Snapshots use the cached PD timestamp without subtracting a delay; only a missing cache requires a fresh TSO, and the data read still targets a matching Learner. The application does not query or select safe-ts. Native synchronization may require ReadIndex and coordination with the Leader. A strict RPC guard rejects Leader and nonmatching peers for ordinary data reads; failures propagate instead of switching the data read to another authority. There is no configured maximum staleness or fixed default snapshot delay. The deprecated `stale_read_delay` YAML option and Go `TiKVConfig.StaleReadDelay` field are accepted but ignored; YAML duration text is not parsed. Writes, mutation conditions and explicit primary views remain authoritative and obtain fresh timestamps. Deployment owns replica placement, networking and MVCC GC retention.

Regional TiKV readers merge this backend instance's committed physical postimages according to the actual read timestamp. This covers records, collection members, resets, deletions and absolute expiration without changing persistent payload formats. Concurrent transactions publish by commit order, and each read pins its relevant postimages before timestamp selection. Postimages can be reclaimed only when timestamp coverage preserves local write visibility; cached PD time alone does not prove regional replica coverage. Explicit primary reads bypass this layer. Internal expiry-index discovery retains its existing bounded maintenance path. Strong-only TiKV instances do not allocate the local write layer. Read-your-writes is scoped to one backend instance; it is not guaranteed across instances or process restarts.

TLS CA 与可选 client certificate/key 同时用于 PD、safe-point etcd、TiKV RPC 和故障恢复健康检查，不修改 client-go 的全局配置。恢复探测通过 `tikv.WithStoreLivenessSecurity` 接收同一 connector 的证书策略；不同 connector 即使访问同一地址，也不共享探测结果。显式无 TLS 的 connector 不继承其他连接的 TLS 配置，TLS 失败不回退到明文。Storage 负责关闭物理 client；逻辑 Store 和强读 view 不关闭该连接。

`tests/integration/store/run_tikv_tls_recovery_tests.sh` 使用两个独立 CA 的临时 PD/TiKV 集群验证 mTLS 与真实事务恢复。它在测试 TCP 代理上断开一个连接的传输，恢复后要求原 Storage client 继续读写，另一个 CA 的连接保持可用。该 gate 不重建 client、不访问线上集群，临时证书、数据、日志和二进制由测试目录及其 Compose project 拥有。

测试 Compose 为两个 TiKV 容器显式设置 `nofile` soft/hard limit 为 `1048576`，满足 TiKV 的启动检查，不依赖 Docker daemon 的默认文件描述符上限。

测试代理先建立监听，runner 启动全部容器后再通过临时启动标记开启集群 bootstrap 计时。启动等待与事务恢复分别有界；原 client 仍须在 20 秒内恢复。失败时 CI 只上传明确列出的诊断日志和 source/Go 版本信息，证书、私钥、二进制及数据目录不进入 artifact。

Entry 的绝对 deadline 与记录、过期索引在同一事务写入；零 deadline 会移除已有过期索引。读取立即把过期值视为不存在。Store Registry 为每个 TiKV root 启动一个可取消的清理 worker，只扫描内部 deadline 索引，每批最多 128 条、每轮最多 32 批且限制 15 秒；清空后间隔一分钟，仍有积压时一秒后继续。删除前在新事务重查记录的 deadline，并通过写冲突重试保护并发刷新。 If that transaction proves a historical candidate was already consumed, the uncommitted batch is discarded and discovery is repeated once on primary; the returned count includes only committed index removals. `Stores.Close` 取消并等待 worker，物理 Storage 随后关闭。直接使用 `NewTiKVWithClient` 的调用方可通过 `PurgeExpired` 执行同样的有界清理；这不推进集群 MVCC GC safe point。

### Redis → TiKV 离线迁移 {#redis-tikv-migration}

`gizclaw kv-migrate <dir> --source <Redis connector> --target <TiKV connector>` 读取该 workspace 的 `config.yaml`，只打开这两个物理连接，不启动 Server、不生成身份、不修改配置，也不打开其他 SQL 或 provider 连接。`redis.readwrite` 只打开 `write_url`，并检查 Redis 的 `ROLE` 为 primary、`INFO cluster` 的 `cluster_enabled` 为 0；不接受原生 Redis Cluster，迁移需要 Redis 7 或更新版本的 `PEXPIRETIME`。TiKV 导入和校验始终使用强读，不使用配置的地区历史快照。

迁移配置可以放在独立目录，只声明源 Redis、目标 TiKV 和所选逻辑 Store；TLS 文件使用绝对路径。也可以临时在原配置中增加 TiKV connector。迁移期间保留原逻辑 Store 名称、prefix、Server 身份和 endpoint。`--stores peers,social` 可只选择指定逻辑 KV Store；默认选择绑定到 source 或 target 的全部 `keyvalue` Store。每个选择项必须有规范、非空且互不重叠的 prefix；其他 namespace 不读取、不写入。

迁移流程：

1. 备份 Redis 数据和 workspace 配置，停止所有使用这些 namespace 的 Server、后台删除/清理 worker 及其他写入者；目标 namespace 同样保持停写。整个全量复制和校验窗口内维持这个条件；海量数据的窗口时长取决于数据量，不保证短暂停写。
2. 执行 dry-run，解决所有类型、大小及目标冲突后再导入。
3. 执行导入；中断后可用同一配置重新执行。工具重新扫描，校验已完成的相同记录，并幂等补齐尚未完成的集合。
4. 执行 `--verify-only`。它检查源→目标和目标→源两个方向；缺失、不同的内容/类型/deadline、未完成集合及目标多余的活跃 key 都使命令非零退出。
5. 校验成功后，把选中 `stores.*.storage` 改为 TiKV connector，保持原 prefix 和 service binding。Before restoring service, have deployment confirm that matching Learners can serve native synchronized reads at timestamps later than the final imported commit. 控制面查询同样遵循配置的读取策略。保留独立迁移配置可在切换后再次执行 `--verify-only`。最终运行配置移除不再使用的 Redis connector，避免 Server 启动仍连接旧 Redis；不要更改逻辑 Store prefix。

```sh
gizclaw kv-migrate /srv/gizclaw --source redis-old --target tikv --dry-run
gizclaw kv-migrate /srv/gizclaw --source redis-old --target tikv
gizclaw kv-migrate /srv/gizclaw --source redis-old --target tikv --verify-only
```

工具保留二进制 string、空值、string 的绝对毫秒过期时间、持久 Set 和所有 score 均为零的 Sorted Set（GizClaw 的字典序索引）。过期候选不会变成永久数据。Hash、List、Stream、带 TTL 的集合、非零 score 和超限数据会失败，不跳过或截断。string 默认上限 1 MiB，`--max-value-bytes` 可设为 1 byte–4 MiB；编码后的 key/member key 上限为 4096 bytes。

`--scan-count` 为 Redis COUNT 提示（默认 128，上限 1024），`--workers` 控制每页并发 key 数（默认 8，上限 32）。Redis 紧凑集合可能一次返回全部成员，工具仍把 TiKV member 写入拆成最多 128 个成员的事务，不在内存中累计全库数据。大集合导入期间用 claim 保护元数据，普通 KV 读写返回 `ErrMigrationInProgress`；全部成员校验完成后才发布集合。该协议不提供跨整个 namespace 的一次性原子切换，因此不能在导入过程中启动目标写入者。

已有不同的活跃 TiKV 记录不会被覆盖，源 Redis 数据不会被修改或删除。SDK 诊断写入 stderr，stdout 只输出 JSON 报告。报告给出扫描访问、实际导入、dry-run 预计导入及验证计数；Redis SCAN 可重复返回 key，因此访问计数不是去重后的数据库 key 总量。迁移执行过程中的错误也会输出已完成部分的报告并非零退出；失败后的目标不能用于切换。

保持停写时可以恢复原 Redis 配置。恢复 TiKV 写入后，Redis 不再包含后续写入，不能直接回切；此工具没有反向同步或在线双写能力。迁移只处理选中的 KV namespace，不迁移业务 SQL、History、对象、日志或 Redis 其他用途的数据。

### 控制面与权限读取

Peer records and profiles, assignments, deletion fences and tasks, API key authentication, Social relationships, synchronization checkpoints and infrastructure feeds all follow the configured read policy. With regional TiKV reads enabled, successful ordinary reads may retain an old assignment, active Peer, credential or membership written by another backend instance until the read timestamp advances. This instance's committed local writes are visible through its internal write overlay. Revocation and deletion visibility follows that read timestamp; a successful read does not itself trigger a primary lookup.

Writes and their conditions execute on the write authority. A definite conditional conflict, failed identity lookup or observation incompatible with a known successful claim can trigger a bounded primary refresh. Retries preserve explicit versions, fixed ownership, lease tokens and lifecycle identities; uncertain commits are not replayed. Primary fallback applies only to the affected operation. `WithStrongReads` remains available for explicit authority views and offline verification, and does not create a snapshot across separate calls.

PostgreSQL mutation 按物理表与 encoded key 获取事务级 advisory lock，并按 lock ID 排序，覆盖不存在的 guard；普通写入、删除和条件 mutation 使用同一协议，不再获取表级写锁。不同 key 可以并发，同 key 与多 key 原子操作保持协调。哈希碰撞只增加串行等待，不改变正确性。PostgreSQL mutation 只清理本次涉及的过期 key，读取仍把过期值视为不存在，并在同一个 key advisory lock 下尝试定点物理清理；获得锁后重新检查过期条件，保留并发刷新值，取消或数据库错误不改变逻辑不存在的结果；SQLite 保留原有事务与过期清理行为。

## 精确集合与原子索引更新

`AddMembers`、`RemoveMembers`、`HasMember`、`ListMembers` 只操作一个完整集合 key；成员是 opaque string，不按 key separator 拆分。添加和删除幂等，缺失集合返回空列表，列表顺序不保证。`ListMembers` 读取整个指定集合，调用方必须按业务资源划分集合，不能建立全站大集合。成员判断使用 `HasMember`，不先读取列表。

`ApplyMutation` 在一次原子操作中处理 `Conditions`、记录写入、key 删除及成员增删。条件中的 `Expected == nil` 表示必须不存在，非 nil 表示记录字节必须相等。条件失败返回 false 且没有写入；操作依次写记录、删 key、加成员、删成员。同一个 mutation 的记录 key 和集合 key 必须分离。集合不设置过期时间，最后一个成员删除后集合消失。

Redis 使用原生 Set 和 Lua 原子操作。Badger 使用独立的记录／成员物理命名空间和事务，成员判断是精确 key 查询。SQL 使用记录类型列及独立成员表，联合主键为 `(encoded_key, member)`；事务锁覆盖记录和集合 key。`Prefixed` 同时限定记录、条件和集合 key，不修改成员 ID。

`AddMembers` 和 `AddOrderedMembers` 的 `SetMembers.MaxMembers` 可限制本次添加后的集合大小，零表示不限制。只有增加了不同成员且超过上限才返回 `ErrMemberLimit`，整个 mutation 不产生任何写入；重复成员仍幂等，删除可释放容量。容量校验与记录、成员写入在同一 Lua／事务／内存锁内执行，多个客户端不能并发超额。每个添加分组单独校验，后面的删除不会为前面的添加预留容量。

## 有序集合范围查询

`RangeOrderedMembers` 在一个完整集合 key 内按成员字节升序读取，`After` 和 `Before` 为可选的排他边界，`Limit` 必须为正数。成员仍是 opaque string；时间索引由业务层编码为固定宽度时间与资源 ID。它不会解释 key 前缀，也不会枚举其他集合。

有序集合通过 `ApplyMutation.AddOrderedMembers`／`RemoveOrderedMembers` 与业务记录原子增删，和普通 Set、byte value 是不同类型；混用返回 `ErrWrongType`。Redis 使用所有 score 为零的 Sorted Set，并将字典序范围和 LIMIT 下推到 Redis。Badger 使用集合成员 key 的有界 seek；SQL 使用 `(encoded_key, member)` 索引与范围谓词、ORDER BY、LIMIT。Memory 为进程内实现，会在该集合内排序。集合规模仍需由业务范围或分片控制。
