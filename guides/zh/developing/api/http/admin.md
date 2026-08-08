# Admin API

Admin API 面向获得管理权限的 operator、CLI 和管理 UI。它负责声明式资源管理、Peer 管理、Telemetry 查询和 Server 运维，不供普通 Peer 作为产品数据通道使用。

Source：`api/http/admin.json`
Go 生成输出：`pkgs/gizclaw/api/adminhttp`

准确的 endpoint、参数、request 和 response 见 [Admin API Reference](/api/)。本页只说明 surface ownership 与 Schema 依赖。

## Surface 分组

| 分组 | 主要职责 |
| --- | --- |
| Resource | `apply/show` 及统一 Resource envelope |
| Peer | Peer 查询、批准、阻止、刷新、配置与 runtime |
| Runtime access | RuntimeProfile 与 RegistrationToken 管理 |
| AI | Credential、Model、Voice、Provider Tenant、Workflow、Workspace |
| Gameplay | Game Rule、Pet、Badge、Points、Result 与 Reward |
| Social | Contact、Friend 与 Friend Group 管理 |
| Firmware | 声明式 Firmware resource，以及 stable、beta、develop 的 external package 配置 |
| Observability | Server log stream、Peer telemetry query 与 active pending-deletion 运维 |

Admin OpenAPI 只拥有 HTTP path、request/response 和 wire error。Resource validation、authorization、storage 和领域 lifecycle 由对应 services 与 resource manager 实现。

## Admin Resource identity

每个具体 Admin Resource 都由调用方提供的 `(kind, id)` 唯一定位。声明式 envelope 必须包含非空 `metadata.id`；直接 create/put DTO 必须包含同一个 `id`。Server 会原样持久化已接受的值，不生成、不 trim、不把 `name` 解析为 ID。ID 最多包含 1024 个 Unicode 字符（且不超过 4096 个 UTF-8 字节）。首尾包含空白的 ID 非法；独立 URI dot segment `.` 与 `..` 也是保留值；其余合法 ID 仍按 opaque value 处理。`PUT /resources/{kind}/{id}` 和各领域 put endpoint 要求 path ID 与 body ID 完全相等。

`ResourceList` 是唯一例外：它是只用于批量 apply 的虚拟 envelope，本身没有 metadata 或 ID；其中每个具体 item 仍必须声明自己的 `metadata.id`。Apply result 对具体 Resource 返回同一个 ID；ResourceList 顶层 result 不返回 ID。

下游引用只保存目标 Resource 的 canonical ID。调用方可以在提交前构造完整依赖图，因此 apply 不需要先创建资源、记录 Server 返回值，再改写 foreign key。`name` 只存在于明确拥有 scoped name 的 typed contract，例如 Workspace、Contact、FriendGroup、FriendGroupMember 的 `spec.name`、Tool 的 `spec.invoke_name`，以及 Peer RPC alias。Firmware 没有独立 name：Admin 使用 caller-supplied ID 定位，Peer firmware method 不暴露 Firmware identity。Typed name 不是通用 Admin identity。

领域派生的 Resource ID 必须可由已知输入确定性计算：Friend 使用排序后的 Peer public key 关系 ID，FriendGroupInviteToken 的 ID 等于 `friend_group_id`，FriendGroupMember 的 ID 是 `<escaped friend_group_id>:<escaped peer_public_key>`，其中各 component 使用 URL path percent-encoding，分隔冒号本身不属于任何 component。为了保证这个派生 ID 仍满足统一的 1024 字符上限，FriendGroup ID 最多包含 80 个 Unicode 字符；同步发现的 Voice 使用对 provider kind、tenant ID 与 provider voice ID 做带长度分隔的 SHA-256 派生 ID。

这项约束使 Terraform 等声明式 provider 可以使用稳定 import key `<kind>/<id>`，安全重试相同 create/apply，并在 plan 时确定所有引用。服务端不读取或迁移旧的 `metadata.name` 格式。

## Pending-deletion 操作

`DELETE /peers/{publicKey}`、`DELETE /workspaces/{name}` 和 `DELETE /peers/{publicKey}/pets/{id}` 会原子创建或复用一条领域 pending-deletion handoff，并返回删除时的投影；handoff record 只通过下述运维接口暴露。Workspace marker 会立即拒绝该 Workspace 的选择、运行、history/icon 与 mutation，但 Admin Workspace get/list 仍可用于诊断。Peer marker 会立即关闭在线 connection，并拒绝该身份的 reconnect、login、session、RPC、WebRTC、业务读取与 mutation；仅保留 Admin Peer get/list 和同一 delete。完成后 Peer 只保留永久 tombstone，同一 public key 不能重新注册。Workspace delete 只接受用户 Workspace；system Workspace 返回 `SYSTEM_WORKSPACE_DELETE_FORBIDDEN`。普通 Pet delete 保留其绑定和 system Workspace。

Operator 通过 `GET /pending-deletions` 和 `GET /pending-deletions/{deletionId}?source=...` 查看 active cleanup work。`POST /pending-deletions/{deletionId}/retry?source=...` 只会重新排队 `failed` task；其他 active 状态返回 `409`。List cursor 是 opaque value，绑定除 `limit` 外的全部 filter。Response 只暴露领域批准的 locator 与有界 failure metadata，不暴露 owner identity、descriptor、payload、credential、lease token、marker fingerprint、原始 backend error 或 stack trace。Finalization 成功后立即删除 task，因此 get/retry 随后返回 `404`；系统不保留 completion receipt 或 history。

## Resource 依赖

Admin 引用 `shared.json`；该生成入口继续引用 `resources/*.json`：

```text
shared/ ← resources/ ← shared.json ← admin.json
```

Resource 专属 Spec 与 Resource 放在同一文件；Admin API 不应通过 `shared.json` 间接加载整个 Resource graph。
