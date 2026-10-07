# Peer Sync

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peersync)

`peersync` 拥有 owner Peer 的状态同步检查点。领域 service 继续拥有资源、权限与读取投影；
Peer HTTP 组装现有读取能力，生成 [Public API](../../../api/http/public#peer-状态同步)
定义的有限 SSE 响应。它不订阅设备事件，也不持有设备连接。

检查点以规范资源路径保存 JSON 投影的 SHA-256 摘要，不保存资源 payload。
增量同步比较客户端上次完成的检查点与当前投影，返回完整 upsert 与消失项目的 delete。
无有效基线时返回 reset 和全量。客户端暂存响应，在 done 时一起提交状态与时间戳。

Server 复用 `services.peer.store`，按 `peer-sync/<owner>/head` 与
`peer-sync/<owner>/checkpoints/<timestamp>` 精确寻址。通过 KV `ApplyMutation`
原子比较 head、提交新检查点和删除淘汰的检查点，不枚举数据库 key。
每个 owner 最多保存 64 个检查点，每个检查点保留最多 24 小时；head 同样设置过期时间。
同毫秒请求或时钟回退时按 owner 单调增加时间戳；时间戳保持 JavaScript safe integer。
并发提交使用有界条件重试，取消和存储失败不会产生成功完成事件。

本服务没有后台 worker、锁表或订阅需要关闭。持久化由配置的 KV backend 负责；
memory store 丢失后，旧时间戳自然触发全量 reset。

## Giztest 验证

`server.peer.sync.giztest.yaml` 通过真实 Edge、Server、WebRTC 与 API Key 验证同步；普通 Go CI 执行 `TestPeerSyncGiztest`，JavaScript/原生 Flutter 运行同一场景。入口与覆盖范围见 [测试与 E2E](../../../testing)。
