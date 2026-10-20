# 共享在线状态

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence)

`peerpresence` 在 `services.peer.store` 的 `presence` namespace 保存跨 Server 的连接状态、最后观察时间和正在运行的 Workspace。好友与群成员列表在关系授权后读取这些快照。它不使用 `services.peer_run.store` SQL，也不改变本地连接索引、运行选择和运行历史的 ownership。

## 发布与查询

Manager 在连接上线、下线、替换、退役和运行 Workspace 变化时发布。Agent Host 在 starting、running、stopping、stopped 或 error 转换后通知 Manager；只有 running 状态携带 Workspace。查询、音频活动和 telemetry 不发布状态，`last_seen_at` 因此是最后一次转换观察到的连接活动，不是持续刷新时钟。

发布只把快照交给进程内 worker，不在 Manager 或 Agent Host 状态锁中执行存储 I/O。每个 Peer 独立串行写入，合并尚未提交的中间状态；每次操作带两秒 context deadline，失败写入一秒后重试最新状态，直到成功或 service 关闭。不同 Peer 的写入互不等待。Storage 打开的 Redis 读写端均启用 context 的 socket I/O deadline；直接借用客户端的 Go 调用方同样应启用该选项。查询使用普通 KV view；Redis 读写分离走读端，写入和原子条件走写端，不对查询错误做主库或 SQL fallback。读取失败视为离线并省略最后观察时间。

Manager 为每个接受的连接创建独立 `Publisher`。首次上线在写权威端读取 `owners/<peer>`，并通过原子条件同时领取 token 与写入快照；后续 Workspace 和下线更新必须匹配该 token。另一台 Server 或同机新连接领取后，旧连接的迟到更新不再覆盖它，其他 Peer 的发布继续运行。`Publisher` 在整个连接生命周期内保留领取条件与 fencing 状态；下线 worker 退出后，删除失败回滚也不能重新领取。首次领取的条件在错误重试间保留，提交响应丢失也不能重新抢回其他连接的 ownership。未曾尝试上线便被合并成下线的连接不领取 token，也不覆盖现有历史；已提交但响应丢失的上线仅在 token 仍归本连接时撤回。ownership token 不逐 Peer 续期，查询仍只读快照与 Server 租约。

运行时由 Server 从现有 Peer KV 自动组装。多台 Server 必须共享该 Store 的物理 namespace；只共享 Social Store 不足以共享 presence。单节点 `redis`、`redis.readwrite` 和其他已有 KV backend 都沿用现有配置，不新增 service binding。

列表使用 `ReadBatch` 先批量读取当前页的 Peer 快照，再按 Server 去重读取租约。两轮共享最多两秒的 presence 预算；调用方更短的 deadline 优先。群的可选 Workspace binding 查询也包含在这份预算内，前置关系授权、分页与好友资料查询沿用请求自身的 context。返回顺序与分页不变，单项失败或排队项超时保守显示离线并省略最后观察时间，已经读到的离线历史不因其他项超时而丢失。

批量 I/O 通过 [KV 层](/zh/developing/stores/kv#批量点查) 的必需方法 `Store.BatchGet` 执行。TiKV 每批复用一个历史快照，Redis 在读端执行原生 `MGET`，其他 backend 在各自实现内最多八路并发；Peer 与租约的两轮批量读取不承诺共用一个快照。只提供旧单项 presence 接口的 Go 调用方保留最多八路并发的兼容读取。

## Server 租约

每次 Server 启动创建独立 generation，并每十秒续期一条 Server 租约，不逐个续期 Peer。默认有效期三十秒；TiKV 配置历史读时额外加上 `kv.ReadDelay` 返回的历史窗口，避免健康租约在历史快照中已经过期。地区历史读仍会延迟首次上线及后续变化的可见性。

查询同时校验快照的 generation、租约和绝对到期时间。异常退出、无法持续续约或时钟到期会让该 Server 的 Peer 离线；正常关闭主动撤销租约。新 generation 会拒绝旧进程的状态写入、续期与关闭撤销。所有 Server 应保持时钟同步；Redis 副本延迟超过租约窗口时会保守显示离线。

Server 进程关闭会取消并等待全部 worker，不关闭借用的 KV Store。presence 只提供列表展示，不作为关系权限、路由 ownership 或消息送达的依据；消息送达仍需要目标设备 ACK。

## 验证

`go test -race ./pkgs/gizclaw/services/runtime/peerpresence ./pkgs/gizclaw` 覆盖租约过期、进程 generation 替换、跨 Server 交接中的迟到 Workspace／下线更新、领取响应丢失、断线、运行转换与 SQL 不可用时的跨 Manager 查询。批量测试覆盖租约去重、部分损坏、未知 Peer，以及两轮查询共享的 deadline。真实 Redis 主从验证由 `tests/store-e2e/run_redis_readwrite_tests.sh` 运行；真实 TiKV Learner 的批量读取、复制滞后和 TCP 响应阻塞验证由 `tests/store-e2e/run_tikv_tests.sh <evidence-dir> --presence-only` 运行，范围见[测试与 E2E](/zh/developing/testing#tikv-docker-e2e)。
