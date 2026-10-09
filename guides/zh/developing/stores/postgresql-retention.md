# PostgreSQL retention

PostgreSQL 时间分区由 pg_partman 5.5.0 管理。LogStore、Memory Store 等公共 interface 不依赖该扩展；PostgreSQL 适配层负责登记和校验，SQLite 后端保持独立。

## 部署

`build/postgres/Dockerfile` 提供 PostgreSQL 17、pgvector 与固定版本 pg_partman。在仓库根目录构建：

~~~sh
docker build -f build/postgres/Dockerfile -t gizclaw-postgres .
~~~

PostgreSQL 启动参数需要包含 `shared_preload_libraries=pg_partman_bgw`，并配置 `pg_partman_bgw.dbname`、有维护权限的 `pg_partman_bgw.role` 和 `pg_partman_bgw.interval=3600`。后台进程每小时预建分区和回收旧分区；没有业务写入时仍执行。时间分区使用 UTC。

Compose 使用一个可重复执行的初始化服务。它基于同一 PostgreSQL 镜像，entrypoint 为 `bootstrap-partman`；环境变量 `PGHOST`、`PGUSER`、`PGPASSWORD` 选择管理员连接，`PG_PARTMAN_DATABASES` 是逗号分隔的既有应用数据库清单。管理员凭据由部署环境注入，不写入配置或镜像。初始化服务在 PostgreSQL 健康后运行，应用通过 `condition: service_completed_successfully` 等待它。

初始化服务以管理员身份为每个数据库安装 pgvector 与 pg_partman，并给该数据库 owner 登记所需权限；已有 volume 也会执行，因此不依赖只运行一次的 docker-entrypoint-initdb.d。已有数据库和业务 role 的创建仍由部署层负责。数据库 owner 必须对应应用连接身份。示例见 `tests/gizclaw-e2e/docker/compose.memory.yaml`，它保持 Server 与 Mem0 数据库、role 相互隔离。

## 保留边界

| 数据 | 分区键 | 策略 |
| --- | --- | --- |
| Workspace History | 写入时计算的 expires_at_unix_nano | Store 与 History asset 的 ttl 都设为 2160h；pg_partman 额外 retention 为零 |
| Peer/model 用量 | hour_unix_nano | pg_partman retention 为 90 days |
| Mem0 PGVector | 物理 expires_at | 固定写入时间加 90 天；pg_partman 额外 retention 为零 |

History 与 Mem0 使用独立表。查询会立即排除到期记录，物理回收以完整 UTC 日分区为单位，由每小时维护处理，所以回收可能比单条记录到期晚一个分区周期加维护间隔。不得把 pg_partman 默认的 detach 当作删除：登记明确设置 retention_keep_table=false。

Server 的 History 维护和 Mem0 服务的附属数据维护也每小时运行，清理 identity、reservation、SQLite journal/cache 和无有效事实的实体索引。应用停止期间 PostgreSQL 后台进程仍回收分区；应用附属数据在下次启动时补清理。二进制 History asset 继续由 ObjectStore 的独立 90 天 TTL 管理。

## 启动和既有数据

已分区的 History 与用量表自动登记，保持父表查询和现有到期语义；pg_partman 接管建删分区。既有子表必要时自动调整名称，不复制业务记录。

Mem0 普通 collection 在首次启动的 PostgreSQL transaction 中转换；原 collection 名保留为 SDK 可更新 view，UUID、payload 和向量原样复制。事务校验数量后才移除旧物理表，任何失败都会回滚该 collection。首次转换需要表锁及构建索引，部署应安排维护窗口；重复启动无需再次复制。

旧 Mem0 记录使用有效创建时间加 90 天；无创建时间的记录从转换时开始保留，未来时间按转换时限界。无效日期会阻止启动并保留旧表，必须先修正源数据。Cloud/Volc 与本地 Qdrant 不使用这个 PostgreSQL 分区策略。
