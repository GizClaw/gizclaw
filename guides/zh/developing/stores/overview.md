# pkgs/store 总览

`pkgs/store` 提供 GizClaw 多个领域共同使用的持久化与索引基础能力。这里定义 storage abstraction 和通用实现，不拥有 Peer、Agent、AI 或其他产品资源的业务规则。

## Package 结构

```text
pkgs/store/
├── kv/           # Ordered hierarchical key-value store
├── logstore/     # 日志追加、查询与日志服务 driver
├── memory/       # Observation extraction、fact recall 与 provider adapters
├── metrics/      # Time-series sample write and query
├── objectstore/  # Prefix-addressable binary object storage
├── storage/      # 类型化物理连接配置、实例与生命周期
├── vecid/        # Vector locality-sensitive identity registry
└── vecstore/     # Vector similarity index
```

| Package | 核心边界 | 主要消费者 |
| --- | --- | --- |
| [kv](./kv) | 有序层级 key、CRUD 与范围遍历 | GizClaw services、其他 stores |
| [logstore](./logstore) | 追加结构化 record、backend-neutral 查询与分页 | 进程日志、event/audit 等生产者 |
| [memory](./memory) | 原始 observation、fact recall/update/delete 与异步 operation | Agent runtime、memory evaluation harness |
| [metrics](./metrics) | Sample 写入、instant/range query 与 aggregation | Peer telemetry、Server metrics |
| [objectstore](./objectstore) | Binary object、prefix list/delete 与 expiration | Firmware、workspace assets、HNSW |
| [vecid](./vecid) | Vector hashing、bucket 与 identity 聚类 | Voiceprint detection |
| [vecstore](./vecstore) | Vector add/search/delete 与 HNSW persistence | Agent recall、memory index |

## 依赖关系

```mermaid
flowchart TB
    Domains["GizClaw services / Agent runtime"] --> KV["kv"]
    Domains --> Metrics["metrics"]
    Domains --> Objects["objectstore"]
    Domains --> Vectors["vecstore"]
    Domains --> Memory["memory"]
    Memory --> Remote["Mem0 / Volc remote"]
    Vectors --> Objects
    VecID["vecid"] --> Voiceprint["audio/voiceprint"]
```

`cmd/internal/server` 独占 YAML DTO、严格解析、环境变量展开和 workspace 相对路径解析，再把普通 Go 配置传给公共包。`pkgs/store/storage` 打开并持有物理资源；根包 `pkgs/store` 校验 Storage/Store 组合并构造逻辑 Store。两个公共包都不读取 YAML 或 JSON，也不决定 service binding。

### Server 组合契约

`storage` 是动态的物理 Data Connector 清单。每个 `kind` 直接标识一个具体 backend，并拥有对应路径、DSN、endpoint、credential、pool/client、readiness 与 lifecycle。`stores` 根据 Store kind 与 Storage kind 的组合构造逻辑接口，并通过 prefix、table、database 或 topic scope 隔离数据。固定类型的 `services` block 把内置消费者绑定到具备兼容能力的命名 Store。Registry 名称精确匹配，不具有任何保留 service 语义。

| Store kind | 接口与 scope |
| --- | --- |
| `keyvalue` | `kv.Store`；Badger/memory 使用可选 key prefix；Redis、Redis 读写分离和 TiKV 使用必填且不重叠的 key prefix；SQLite/PostgreSQL 使用必填 prefix，后端将它作为物理表名 |
| `sql` | 借用物理 `*sqlx.DB` pool；schema 由 service 拥有 |
| `objectstore` | `objectstore.ObjectStore`；可选 object prefix |
| `metrics` | `metrics.Store`；`memory`、Prometheus、ClickHouse，或 SQLite/PostgreSQL table |
| `log` | `logstore.Store`；Volc TLS topic |

| Storage kind | 物理配置 |
| --- | --- |
| `badger` | `dir` |
| `memory` | 无属性 |
| `filesystem.dir` | `dir` |
| `volc-tos` | HTTPS endpoint、region、既有 bucket、key pair 与可选 session token |
| `aliyun-oss` | HTTPS endpoint、既有 bucket、key pair 与可选 security token |
| `gcs` | 既有 bucket 与可选 Application Default Credentials 文件 |
| `azure-blob` | HTTPS account URL 与既有 container；身份来自 Azure Default Credential |
| `sqlite` | `dir` 或 `dsn` 二选一 |
| `postgresql`、`clickhouse` | `dsn` |
| `redis` | 单节点 `redis://` 或 `rediss://` URL；`rediss://` 可通过 `tls_ca_file` 增加 PEM CA 证书；不支持 Redis Cluster |
| `redis.readwrite` | 同一复制组、同一逻辑数据库的 `read_url` 与 `write_url`；可分别配置 `read_tls_ca_file`、`write_tls_ca_file`；只支持 `keyvalue` Store，读取允许复制延迟 |
| `tikv` | `pd_addresses`；可选 `tls_ca_file`、`tls_cert_file`、`tls_key_file`；可选成对的 `stale_read_delay` / `read_labels`，详见 [KV](./kv#tikv) |
| `prometheus` | remote-write/query URL 与可选 bearer token |
| `volc-tls` | endpoint、region 与 credential |

Redis TLS 配置示例：

```yaml
storage:
  redis:
    kind: redis
    url: rediss://user:password@host:port/0
    tls_ca_file: /run/secrets/volc-redis-ca.pem
```

部署只提供一个 Redis 入口时继续使用 `kind: redis`。部署具备主从复制和稳定的主节点写入入口后，可以把同一个 Storage 条目切换为读写分离，`stores` 与 `services` 的绑定保持不变：

```yaml
storage:
  redis:
    kind: redis.readwrite
    read_url: ${GIZCLAW_REDIS_READ_URL}
    write_url: ${GIZCLAW_REDIS_WRITE_URL}
    read_tls_ca_file: /run/secrets/replica-ca.pem
    write_tls_ca_file: /run/secrets/primary-ca.pem
```

两个 CA 字段均为可选项，只能用于对应的 `rediss://` 入口。两个连接都由 Storage 打开、检查 readiness 并关闭。复制拓扑和主节点切换由部署层维护；客户端不会自动发现副本或把读取错误转发给写入入口。读写与一致性边界见 [KV Store](./kv)。

公共 Go API 不使用包含 `Kind` 和所有 backend 字段的通用属性结构。`storage.Config`
是由 package 封闭的配置接口，每个 backend 只暴露自身有效的字段：

```go
physical, err := storage.New(map[string]storage.Config{
	"main": storage.PostgreSQLConfig{DSN: postgresDSN},
	"cache": storage.BadgerConfig{Dir: cacheDir},
	"local": storage.MemoryConfig{},
})
```

具体实现包括 `BadgerConfig`、`MemoryConfig`、`FilesystemDirConfig`、
`SQLiteConfig`、`PostgreSQLConfig`、`ClickHouseConfig`、`RedisConfig`、`RedisReadWriteConfig`、`PrometheusConfig`、
`VolcTLSConfig`、`VolcTOSConfig`、`AliyunOSSConfig`、`GCSConfig` 和
`AzureBlobConfig`。因此 Go 调用方不能为 PostgreSQL 传入 `dir`，也不能为
Badger 传入 DSN 或 provider credential。`cmd/internal/server` 保留扁平 YAML DTO，
根据 `kind` 显式转换为对应的具体 Go 类型；YAML 字段不会进入公共配置类型。

多个 Store 可以借用同一个 connector；调用方必须先关闭逻辑 `Stores`，再关闭物理 `Storage`。`memory` 只是无状态 marker，每个引用它的 keyvalue 或 metrics Store 都创建独立实例。`vecstore` 没有内置 Server 消费者，因此不属于命令层 Store 配置；对应公共 package 与构造函数仍保留。RuntimeProfile 与 MemoryLayout 选择的 Memory connection 仍在此 registry 之外。

`objectstore` 与 `filesystem.dir`、`volc-tos`、`aliyun-oss`、`gcs` 和
`azure-blob` 兼容。物理 Storage 拥有 official SDK client、readiness probe、
transport、retry policy 与关闭；逻辑 ObjectStore 借用 client，只应用一次 prefix，
且不关闭 client。因此多个不重叠的逻辑 prefix 可以共享同一个 cloud connector。

SQLite/PostgreSQL KV 只声明一个单段 `prefix`；后端把它直接作为带引号的物理表名，表内 key 不再重复增加该 prefix。SQL Metrics 声明 `table`。这些物理名称都必须是未限定、最长 63 bytes 的 ASCII 名称；KV prefix 还可包含 `-`。Registry 只独立校验每一份声明，不保留表名，也不比较不同 Store 声明。构造函数直接用幂等的 `CREATE ... IF NOT EXISTS` 保证业务表与索引存在；PostgreSQL 在同一事务内执行这些语句，并用按当前 schema 与物理表名派生的事务级 advisory lock 串行化，因此多个进程或 adapter 同时初始化同一张新表不会因 catalog 冲突失败。随后校验当前 adapter 所要求的列、主键、identity 和索引；它不创建 schema version/history 表，也不导入或重写既有 backend 数据。兼容的既有表会直接复用，不兼容的定义会在该 adapter 构造时失败。逻辑 `Close` 只关闭 adapter 状态，不关闭借用的 `*sqlx.DB`。

`sqlmeta.SQLTable` 拥有 SQLite/PostgreSQL 借用表的 dialect、identifier、quoting、初始化与 schema inspection。KV、Metrics 和 History 直接使用这个独立于物理 connector registry 的能力；SDK metrics 不会因此引入 TiKV、PD 或其他服务端连接实现。

SQL backend 错误在补充具体 Store 操作上下文后保留底层 error identity，受信任的调用方可以通过 Go error chain 检查具体原因。公开错误文本只描述失败的 Store 操作，不包含可能携带绑定值、DSN 或连接信息的 driver 原始文本；HTTP/RPC surface 不得向客户端泄露这些细节。

### PostgreSQL 连接池

每个 PostgreSQL physical Storage 按需打开连接，默认最多 128 条，并保留最多 128 条空闲连接供后续查询复用。连接连续空闲 1 分钟后由 `database/sql` 的维护任务回收；正在使用的连接不受空闲回收影响。请求超过池容量时等待可用连接，等待服从请求 context。Store 只借用池，关闭 physical Storage 才会关闭全部连接。

Go 调用方使用 `PostgreSQLConfig.MaxOpenConns` 和 `ConnMaxIdleTime`；Server YAML 的配置位置如下：

```yaml
storage:
  runtime:
    kind: postgresql
    dsn: ${RUNTIME_POSTGRES_DSN}
    max_open_conns: 128
    conn_max_idle_time: 1m
```

省略或零值选择默认值；负值无效，空闲时间使用 Go duration 格式。空闲连接上限与打开连接上限相同，避免周期性并发查询结束后只保留少量连接、下一轮又重复 TLS 与认证。配置作用于一个 physical Storage，不是整个 Server 或整个数据库；同名 Storage 的多个消费者共享容量，不同 Storage 即使 DSN 相同也各有自己的池。配置容量时应合计所有 Server 和 Storage 的连接预算。

诊断时分别观察 `DB.Stats()` 的 `InUse`、`Idle`、`WaitCount`、`WaitDuration`、`MaxIdleClosed` 和 `MaxIdleTimeClosed`，结合新建连接耗时、PG SQL 耗时、锁与事务证据。连接数包含正在建立的连接；`WaitCount` 只表示容量不足时的池等待，不计 TLS/认证耗时。提高复用率不能消除慢 SQL、锁阻塞或冷启动连接故障，也不能替代请求 deadline。

### 进程 SQLite DSN 契约

`pkgs/store/storage` 接受 modernc SQLite DSN，并继续支持 `_pragma` 等
modernc 参数。连接的 `busy_timeout`、WAL journal mode 和 foreign-key PRAGMA
由 GizClaw 负责。`go-sqlite3` 兼容简写 `_busy_timeout`/`_timeout`、
`_foreign_keys`/`_fk`、`_journal_mode`/`_journal`、
`_synchronous`/`_sync`、`_auto_vacuum`/`_vacuum` 和 `_query_only` 不受支持，
并会在打开数据库前被拒绝。这样依赖升级不会静默改变数据库文件或连接行为；
如需不同策略，应在 storage 所拥有的代码中配置，而不是通过这些 DSN 简写。

## 放置规则

这里保存可跨领域复用的 storage interface、backend adapter 以及通用 key、query、index、expiration 与 persistence 语义。领域 resource schema、HTTP/RPC、authorization、进程配置和只属于单一领域的 repository 不应放入 `pkgs/store`。
