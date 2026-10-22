# 设备过程调用

设备过程使用 `api/proto/rpc/payload/tool.proto` 中封闭的 `ClientTool` 注册表。Server 通过 `client.tool.v0.invoke` 调用一个已安装的过程；请求携带枚举值及该值指定的请求消息编码。`client.tool.v0.list` 返回设备实际安装的子集。`client.rpc.methods.list` 用数字 `RpcMethod` 报告协议族和版本。

`pkgs/gizclaw/peer_service_serve_peer_http_tool.go` 实现按 owner 限定的 HTTP 列表与调用路由。在打开 RPC stream 前验证类型化 JSON 参数，按 owner 串行化命令，按注册表解码响应并映射设备错误。RuntimeProfile Admin Tool 仍是 AI 和 Workflow runtime 使用的 Server 侧 HTTP 资源，与预定义设备过程独立。

## 通用设备管理 Invoke

以下工具均通过既有 `tool/v0/invoke` 调用，类型见 `payload/management.proto`；HTTP 参数及
返回模型见 `api/http/peer.json`。Get 的状态字段为 `state`，Set 的实际生效状态为 `applied`。

| 工具 | 请求参数 | 返回结构 |
| --- | --- | --- |
| `content.age.get` / `content.age.set` | Get `{}`；Set `option_id` | `state` / `applied`：`option_id` 与 `options[{id,display_name,min_age_years?,max_age_years?}]` |
| `device.features.get` | `{}` | `features[{id,display_name,enabled,writable}]` |
| `device.feature.set` | `feature_id`、`enabled` boolean | `applied`：该功能的稳定 id、名称、实际 enabled 和 writable |
| `usage.time.get` / `usage.time.set` | Get `{}`；Set `limit_seconds` uint32 | `state` / `applied`：`limit_seconds`、`used_seconds`、`exhausted` |
| `ai.safety.get` / `ai.safety.set` | Get `{}`；Set `level` | `state` / `applied`：当前 `level` 与 `options[{id,display_name}]` |
| `management.password.get` | `{}` | `is_set` boolean |
| `management.password.set` | `password` string | `is_set: true`，仅在持久化成功后返回 |
| `management.password.reset` | `{}` | `is_set: false`，仅清除管理密码 |

ID 最多 64 UTF-8 bytes，显示名称最多 128；年龄和安全选项各最多 16 个，功能列表最多 32 项。
当前年龄/安全选项必须出现在返回的支持列表中，ID 不得重复。boolean 和限额必须携带 presence，
false 与 0 是有效值。限额 0 表示不限制，设备维护当前使用窗口的计时并实际限制使用；
非零限额下 `used_seconds >= limit_seconds` 时 `exhausted` 必须为 true。

设备声明年龄档位、安全等级及功能 ID，GizClaw 不写死产品选项，不提供任意 preference/KV 写入接口。
未知功能返回 NOT_FOUND，功能不可改返回 FAILED_PRECONDITION，未安装工具返回 UNIMPLEMENTED，
执行失败返回相应 RpcStatus。Set 的结果来自设备执行后的权威状态，可反映实际限制或夹限，
不能把请求参数回显作为成功。Server 和受维护 SDK 会拒绝缺失或矛盾的实际结果。
AI 安全等级仍经 invoke 查询和设置；设备可在 handler 内复用 Workspace `safety_fence_level`，
但需要完成权威写入/加载并读取当前生效值后响应，不要求调用者自行修改 Workspace。

管理密码最多 128 UTF-8 bytes，非空，不含 NUL。请求沿用设备控制授权和加密传输，密码只进入
设备专属密码处理器，不进入日志、trace、KV Telemetry 或响应。Go SDK 的设备请求 observer
只看到清空密码后的请求。密码 reset 与 `device.factory_reset` 独立，不重置设置或网络。

GizOS/H106 等设备实现各自 provider，并只注册已经真正实现的工具。Go 使用
`HandleClientTool` 与生成的 `rpcpb` 请求/响应，JS 使用 `handlers.tools[CLIENT_TOOL_IDS[...]]`，
Dart 使用 `GizClawPeerRPCHandlers.tools`，C 使用 `gzc_tool_handler_t` 与生成的 nanopb 消息。
Forge 等控制端先调用 tool list，再按类型 invoke；JS control SDK 提供 `getContentAge`、
`setContentAge`、`getFeatures`、`setFeature`、`getUsageTime`、`setUsageTime`、`getAISafety`、
`setAISafety` 以及三种管理密码方法。它们复用原 HTTP invoke，并透传真实失败。
