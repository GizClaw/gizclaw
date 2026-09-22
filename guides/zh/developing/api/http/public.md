# Public API

Public API 是 Server 在 WebRTC connection 建立前后向 Public/Peer caller 暴露的 HTTP contract。它是入口边界，不代表 Peer 领域 service 的全部能力。

Source：`api/http/peer.json`
Go 生成输出：`pkgs/gizclaw/api/peerhttp`

准确的 endpoint、参数、request 和 response 见 [API Reference](/api/)。本页只说明 Public/Peer surface 的设计边界。

`/webrtc/v1/offer` 发生在 Peer connection 建立之前，必须保留 HTTP signaling。建立连接后的 Peer 能力可以使用 reliable HTTP-over-service-stream 或 Peer RPC；选择 transport 时应避免为相同能力维护两套 contract。

Offer 使用 ECDH/AEAD 验证公钥持有者并保护 SDP 和可选的 `AdmissionCredential` protobuf 凭证，不依赖 API Key。密文认证不等于运营方准入；[Giznet](../../giznet#signaling-准入凭证) 定义兼容信封，[Security Policy](../../gizclaw/server/security-policy) 定义可选的 registration-token 判定。Public API 可以复用 `ErrorResponse`、`DeviceInfo` 和 `Runtime` 等真正 shared 类型，但不引用 Admin Resources。

API Key 的鉴权和管理契约见 [Peer HTTP · API Key](../../gizclaw/peer/service/api-keys)。设备首次入网仍由本地 BLE 通道负责；设备在线后可经 API Key 和 `/gizclaw/v1/device*` 扫描或更换 Wi‑Fi。

## 设备与 Contact surface

`/gizclaw/v1/device*`、`/gizclaw/v1/contacts*`、`/gizclaw/v1/friends*` 与 `/gizclaw/v1/friend-groups*` 支持 `Authorization: Bearer <api-key>`，也支持下述设备调试访问。Server 从 Key record 取得不可变的 owner Peer；Bearer API Key 的 owner 始终由 Key 决定，manager Key 与普通 Key 对这些 route 拥有相同的 owner-scoped 能力。Key 无效或已撤销返回 `401 INVALID_API_KEY`，owner 不是 active Client 或没有 RuntimeProfile binding 返回 `403 API_KEY_OWNER_UNAVAILABLE`，owner pending deletion 返回 `409 PEER_PENDING_DELETION`，validation 与分页错误返回 `400 INVALID_REQUEST`，store 或 service 故障统一返回脱敏的 `500 INTERNAL_ERROR`。

读路径直接投影 authoritative service，不向设备发送 RPC：

- `GET /device` 返回 `DeviceInfo`（name、emoji、`HardwareInfo`、`DeviceIdentifiers`），与 `server.info.get` 同源。
- `GET /device/runtime` 返回 `Runtime`（online、last seen、address、RX/TX），读取不刷新在线状态。`active_workspace_name` 是设备通过 `server.run.workspace.reload-with-options` 最近一次提交的 Workspace，`pending_workspace_name` 是已选中但设备尚未提交的 Workspace（例如 `PUT /device/run/workspace` 切换进行中）；两者读自 Server 的 `peer_runs` 记录，设备离线时同样可读，未设置时省略。
- `GET /device/status` 返回最近一次 authoritative `PeerStatus` snapshot；不提供 `fresh` 参数，`device.status.get` 工具用于实时查询并回写。
- `GET /device/telemetry/{field}/latest`、`/device/telemetry`、`/device/telemetry/aggregate` 保留 Admin telemetry 的字段枚举、采样时间、查询边界、排序与 aggregate 语义，只把 Peer 固定为 owner。
- `GET /device/firmware` 返回 owner 绑定的 Firmware 配置的全部 channel（`stable`、`beta`、`develop`），每个 channel 携带可选的 `description` 与 `package`（`version`、`url`、`sha256`、`size`）（已有包没有版本时省略 `version`，其余信息仍正常返回），与 `server.firmware.get` 同源。Channel 选择归调用方：Server 不保存设备当前使用的 channel，本 route 一次返回全部 channel，由调用方自行选择。未绑定 `firmware_id` 或绑定的配置已不存在返回 `404 FIRMWARE_NOT_FOUND`；某个 channel 未配置包时该 slot 省略 `package`，不报错。
- `GET /device/runtime-profile` 返回 owner 当前绑定的 RuntimeProfile 的 `name`、`revision`，以及 `collections[].workflows[].name`，直接投影 `workflows.collections`，collection 与 workflow 均按 name 排序。`name`/`revision` 与 Peer RPC 响应的 `runtime_profile_name`/`runtime_profile_revision` 同源，workflow name 即 `server.workflow.*` 使用的 alias。`resource_id`、i18n、driver、models、voices、memory、pet 定义与 `app_config` 都不返回；binding 指向的 Workflow 资源是否仍存在不在读取时校验。绑定在鉴权后消失同样返回 `403 API_KEY_OWNER_UNAVAILABLE`。
- `GET /device/workspaces` 返回 owner 明确拥有的 Workspace（含系统 Workspace），不含共享、无 owner 与已进入 pending deletion 的 Workspace。每项是 `DeviceWorkspace`：`id`、`name`、`collection`、`workflow_name`、`available`、`system`、`created_at`、`updated_at`、`last_active_at`。Workflow 只以 `collection` label 与它在 owner 当前 RuntimeProfile 中解析出的 alias（即 `/device/runtime-profile` 列出的 workflow name）表示，与 Peer RPC `Workspace.workflow_name` 同一解析规则；Admin Workflow ID 不返回。alias 不再解析时省略 `workflow_name` 且 `available=false`；没有 collection label 的 Workspace（如系统 Workspace）省略 `collection`。可选 query `collection`、`workflow_name` 精确过滤，`workflow_name` 过滤永远不匹配 alias 已失效的 Workspace；空过滤值返回 `400 INVALID_REQUEST`。
- `DELETE /device/workspaces/{workspaceId}` 走与 `server.workspace.delete` 相同的 Workspace 删除：立即写入 pending deletion 并返回 `202`（无 body），历史、音频、运行状态与图标由后台清理，不联系设备。删除期间设备对该 Workspace 的读取返回 `WORKSPACE_PENDING_DELETION`，同名 `server.workspace.create` 返回 `ALREADY_EXISTS`，直到清理完成。跨 owner、不存在或已在删除中的 ID 统一返回 `404 WORKSPACE_NOT_FOUND`；系统 Workspace 返回 `409 SYSTEM_WORKSPACE_DELETE_FORBIDDEN`，owner 自身 pending deletion 返回 `409 PEER_PENDING_DELETION`。长期记忆不在当前 Workspace 清理范围内。
- `/contacts` 的 list/create/get/put/delete 使用 `services/social/contact` 的同一 owner-scoped 数据；`{contactName}` 是 owner 作用域内不可变的 `name`，跨 owner 与不存在统一返回 `404 CONTACT_NOT_FOUND`，name 或 phone 冲突返回 `409 CONTACT_ALREADY_EXISTS`，owner 已有 8 个联系人时创建返回 `409 CONTACT_LIMIT_REACHED`。

## 好友与群组 surface

`/gizclaw/v1/friends*` 与 `/gizclaw/v1/friend-groups*` 以 Key owner 的身份调用 `services/social/friend` 与 `services/social/friendgroup`，业务规则、权限和持久化与 `server.friend.*`、`server.friend_group.*` RPC 相同。它们只读写共享 Social KV，不联系设备，设备离线时同样可用。没有单独的社交 scope：能访问 `/device*` 的 Key 就能管理好友与群组。

| Route | 复用 | 成功 |
| --- | --- | --- |
| `GET /friends/invite-token` | `GetFriendInviteToken` | `200 InviteToken`；没有有效邀请码 `404 INVITE_TOKEN_NOT_FOUND` |
| `POST /friends/invite-token` `{ttl_seconds?}` | `CreateFriendInviteTokenWithTTL` | `200 InviteToken` |
| `DELETE /friends/invite-token` | `ClearFriendInviteToken` | `204`，幂等 |
| `POST /friends` `{invite_token}` | `AddFriendReportingExisting` | `201 Friend` |
| `GET /friends` | `ListFriends` | `200 FriendList` |
| `GET /friends/{friendName}` | `GetFriendRelation` | `200 Friend` |
| `DELETE /friends/{friendName}` | `DeleteFriend` | `204`；删除已完成后重复删除同样返回 `204` |
| `GET /friend-groups` | `ListFriendGroups` | `200 FriendGroupList`，每项带 `my_role` |
| `POST /friend-groups` `{name, display_name?, description?}` | `CreateFriendGroup` | `201 FriendGroup` |
| `POST /friend-groups/@join` `{invite_token, name}` | `JoinFriendGroup` | `200 {group, member}`；已用同名加入时幂等 |
| `GET /friend-groups/{friendGroupName}` | `GetFriendGroup` | `200 FriendGroup`，任意成员 |
| `PUT /friend-groups/{friendGroupName}` | `PutFriendGroup` | `200 FriendGroup`，仅 owner |
| `DELETE /friend-groups/{friendGroupName}` | `DeleteFriendGroup` | `204` 解散，仅 owner |
| `GET`/`POST`/`DELETE /friend-groups/{friendGroupName}/invite-token` | `Get`/`CreateWithTTL`/`ClearFriendGroupInviteToken` | 仅 owner |
| `POST /friend-groups/{friendGroupName}/@leave` | `LeaveFriendGroup` | `204`，member 与 admin |
| `GET /friend-groups/{friendGroupName}/members` | `ListFriendGroupMembers` | `200 FriendGroupMemberList`，任意成员 |
| `POST /friend-groups/{friendGroupName}/members` `{peer_public_key, member_name, role}` | `AddFriendGroupMember` | `201`；admin 可加 member，owner 可加 admin |
| `PUT /friend-groups/{friendGroupName}/members/{memberName}` `{role}` | `PutFriendGroupMember` | `200`，仅 owner |
| `DELETE /friend-groups/{friendGroupName}/members/{memberName}` | `DeleteFriendGroupMember` | `204`；删 member 需 admin/owner 或本人，删 admin 需 owner |

- `{friendName}` 与 `{memberName}` 是对方的 canonical public key（也是 `Friend.name` / `FriendGroupMember.name`）。`{friendGroupName}` 是调用方自己命名空间里的群名，非成员解析不到任何名字，因此非成员统一得到 `404 FRIEND_GROUP_NOT_FOUND`。路径与 body 中的名字带首尾空白时返回 `400 INVALID_REQUEST`，分页 `cursor`/`limit` 规则同 contacts（limit 1–200，cursor 为不透明值）。
- `Friend` 与 `FriendGroupMember` 带 `info {display_name, emoji}`，来源与 `server.friend.info.get` 相同（`Profiles.GetSelfInfo`）；对方 Peer 已不存在或已删除时省略 `info`，其他读取失败返回 500。好友与群成员各有 10 个上限，列表逐项读取 profile。`FriendGroupMember` 不返回成员自己命名空间里的群名。
- 成员列表（`GET .../members`）的每项带可选的 `online`（与 `Runtime.online` 相同的 Server 本地连接状态）与 `last_seen_at`（RFC 3339 UTC；Server 从未见过该成员或读取失败时省略），与 `server.friend_group.members.list` 相同；add、put、join 返回的成员对象不带这两个字段。
- `ttl_seconds` 范围 60–604800（7 天），越界 `400 INVALID_REQUEST`，body 可省略。不带时与 RPC 完全一致：已有有效邀请码原样返回，否则新建 5 分钟的码。带时新建码按该 TTL 过期；已有有效码保持码值不变，只把 `expires_at` 延长到 `now + ttl_seconds`，绝不缩短，因此设备正在展示的码继续有效。设备 RPC 的默认 TTL 不变。
- `POST /friends` 在关系已存在时返回 `409 FRIEND_ALREADY_EXISTS`，而 `server.friend.add` 继续幂等地返回已有关系。
- `@leave` 删除调用方自己的成员记录。owner 得到 `409 FRIEND_GROUP_OWNER_CANNOT_LEAVE`，应改为解散；admin 也可以退出（`members.delete` 删除 admin 自己仍要求 owner 角色）。

错误码：

| HTTP | code | 场景 |
| --- | --- | --- |
| 400 | `INVALID_REQUEST` | body 缺失、名字为空或带首尾空白、分页参数、`ttl_seconds` 越界、非法角色 |
| 400 | `FRIEND_SELF_INVITE` | 使用自己的好友邀请码 |
| 403 | `FRIEND_GROUP_PERMISSION_DENIED` | 调用方角色不允许该操作 |
| 404 | `INVITE_TOKEN_NOT_FOUND` | 读取邀请码时没有有效邀请码 |
| 404 | `INVITE_TOKEN_INVALID` | 加好友或入群时邀请码不存在或已过期 |
| 404 | `FRIEND_NOT_FOUND` | 好友关系不存在 |
| 404 | `FRIEND_GROUP_NOT_FOUND` | 调用方没有该名字的群（含非成员） |
| 404 | `FRIEND_GROUP_MEMBER_NOT_FOUND` | 目标 Peer 不是成员 |
| 409 | `FRIEND_ALREADY_EXISTS` | 已是好友 |
| 409 | `FRIEND_LIMIT_REACHED` | 任一方已有 10 个好友 |
| 409 | `FRIEND_GROUP_NAME_CONFLICT` | 该名字已指向另一个群 |
| 409 | `FRIEND_GROUP_ALREADY_JOINED` | 已用其他名字加入同一个群 |
| 409 | `FRIEND_GROUP_FULL` | 群已满 10 人 |
| 409 | `FRIEND_GROUP_LIMIT_REACHED` | Peer 已加入 10 个群 |
| 409 | `FRIEND_GROUP_OWNER_CANNOT_LEAVE` | owner 调用 `@leave` |
| 409 | `FRIEND_GROUP_OWNER_CANNOT_BE_REMOVED` | 删除 owner 成员 |
| 409 | `FRIEND_GROUP_OWNER_ROLE_IMMUTABLE` | 修改 owner 角色 |
| 409 | `FRIEND_GROUP_CHANGED` | 并发修改，可重试 |
| 409 | `FRIEND_GROUP_PENDING_DELETION` | 群正在删除 |
| 409 | `PEER_PENDING_DELETION` / `PEER_DELETED` | owner 或对方 Peer 正在删除 / 已不存在 |
| 500 | `INTERNAL_ERROR` | store 或配置故障，脱敏 |

## 设备控制流程

Server 为 API Key owner 提供两类设备接口：`mhs/v0` 处理硬件状态，`tool/v0` 处理预定义过程。下表路径均加 `/gizclaw/v1` 前缀。`GET /device/status` 读取存储的快照；实时调用需要设备在线。Server 在打开 RPC stream 前验证完整的类型化请求，并按 owner 串行转发。见 [设备 provider](../proto/rpc/client-provided-to-server) 和 [RPC 参考](/references/rpc)。

| 路由 | 设备 RPC | 结果 |
| --- | --- | --- |
| `GET /device/mhs/v0/manifest` | 无 | 已绑定 RuntimeProfile 的硬件清单，离线可读 |
| `POST /device/mhs/v0/read` | `client.mhs.v0.read` | 请求的硬件状态 |
| `PATCH /device/mhs/v0/states` | `client.mhs.v0.write` | 整批实际写入的值 |
| `GET /device/tool/v0/tools` | `client.tool.v0.list` | 设备已安装的预定义工具名称 |
| `POST /device/tool/v0/invoke` | `client.tool.v0.invoke` | 一个预定义工具的 `{ "result": ... }` |

调用体例如 `{ "tool": "device.find", "args": { "duration_ms": 8000 } }`。OpenAPI 对 `tool` 使用 `oneOf` 和 discriminator，各过程保持类型化的参数 Schema。工具包括 `device.status.get`、`sound.play`、`device.find`、`device.reboot`、`device.factory_reset`、Wi-Fi 过程、`firmware.update`、七个 `audioplayer.*` 过程及 `run.workspace.set`。`tool/v0` 使用 GizClaw 定义的封闭枚举。RuntimeProfile Tool 目录独立服务于 Server 侧 HTTP Tool；v0 不提供 Agent 调用产品自定义设备工具的能力。

HTTP `result` 使用所选响应消息的 SDK JSON 投影。若 Protobuf 响应只含一个名为 `value` 的消息字段，该字段会被展开：`audioplayer.playlist.set` 的 `playlist_length` 位于 `result.playlist_length`，不再嵌套 `value`。`audioplayer.playlist.get` 等包含自身字段的响应则将这些字段保留在 `result` 下。

非法工具参数、超过 32 UTF-8 bytes 的 sound 或 SSID、非法 Wi-Fi 密码、超过播放列表容量、错误的固件摘要和非法 MHS 写入，均在 Server 侧以 `400 INVALID_REQUEST` 拒绝，不发送设备 RPC。MHS key 必须在已绑定 manifest 中声明，写入还要求 `read_write`。找不到 Workspace 目标时，在设备调用前返回 `404 WORKSPACE_NOT_FOUND`。设备仍独立执行自身的安全限制。

设备离线映射 `409 DEVICE_OFFLINE`；未安装的工具或 MHS handler 映射 `501 DEVICE_UNSUPPORTED`；超时映射 `504 DEVICE_TIMEOUT`；设备 `INVALID_PARAMS` 映射 `400 DEVICE_REJECTED`；其他设备错误脱敏后映射 `502 DEVICE_ERROR`。不存在的已保存 Wi-Fi 网络映射 `404 WIFI_NETWORK_NOT_FOUND`。重启、Wi-Fi 连接、恢复出厂设置或固件更新得到确认后可能断线，同一连接上的后续命令会返回离线，直到设备重连。异步过程的成功应答仅表示设备已接受操作。

`run.workspace.set` 在 `args` 中接受 `workspace_name`，或 `collection` 加 `workflow_name`，并可附带 `kickoff`。Server 在分发前解析为一个可用 Workspace；设备随后通过 `server.run.workspace.reload-with-options` 切换。已提交状态通过 `GET /device/runtime` 观察。`firmware.update` 接受可选的 `channel` 和 64 位小写十六进制 `sha256`；设备在 OTA 前核对自身解析出的包摘要。

连接前 `/server-info` 返回 authoritative Server 的 `public_key`、软件 `version`、`build_commit` 和传输能力。经过 Edge 时构建字段仍属于 authoritative Server，`transport` 描述 Edge 路由。

## 设备调试访问与匿名标识查询

设备通过自身已认证连接调用 `server.runtime.put`，设置 `{ "debug_mode": "readonly" }`。
允许 `off`（默认）、`readonly`、`fullcontrol`，其他值或缺失字段被拒绝。
该设置由设备所属 authoritative Server 持久化到 本机 PeerRun SQL 表的 `debug_mode` 列（按公钥定位），通过 Runtime 的 `debug_mode` 字段读取；不属于 DeviceInfo，
也不通过 `server.info.put` 修改。断线重连保持设置，缺失记录按 off 处理。

设备、联系人、好友与群组 HTTP 接口使用 `Authorization: Bearer gizclaw_pk_<Base58公钥>` 选择调试设备。
公钥必须是 canonical Base58，裸公钥和 `public_key` query 不提供调试授权。
`gizclaw_sk_v1_` API Key 继续走原有鉴权，不会回退为公钥。
Edge 从公钥查询已有 Peer assignment 并代理到配置中的所属 Server；Edge 不读取 DeviceInfo 或调试模式。
所属 Server 每次从 PeerRun 读取当前权限：readonly 只允许 GET，fullcontrol 允许设备/联系人/好友/群组接口的读写和控制。
设备仍须为可用的 active Client 且具有 RuntimeProfile binding。
API key 管理、Admin 和 OpenAI 接口不接受公钥调试授权。
关闭模式后拒绝新请求，已开始的请求不被撤销；存储失败时拒绝访问，响应不暴露底层错误。
调试响应使用 `Cache-Control: no-store`。

以下 GET 接口无需任何 Authorization，返回 `{ "public_keys": [...] }`，包括调试关闭的全部匹配设备：

- `/gizclaw/v1/peers/@findBySn/{sn}`
- `/gizclaw/v1/peers/@findByImei/{tac}/{serial}`

无匹配返回空数组，只公开公钥。SN 和 IMEI 均为设备声明的非唯一标识。
IMEI 索引为 `by-imei:<tac>:<serial>:<pubkey>`，按前缀列举并回读设备记录核对，更新和删除只影响对应公钥。
Admin IMEI 查询为 `/peers/@findPubKeysByImei/{tac}/{serial}`，CLI `admin peers resolve-imei` 同样返回公钥列表。

## 音乐点播

七个播放器过程统一使用 `POST /gizclaw/v1/device/tool/v0/invoke`，工具名称为 `audioplayer.get`、`audioplayer.playlist.get`、`audioplayer.playlist.set`、`audioplayer.playlist.append`、`audioplayer.play`、`audioplayer.stop` 和 `audioplayer.mode.set`。`args` 保持各过程的类型化字段：set/append 接收 `items`，play 要求从零开始的 `index`，mode 接收 `repeat`。列表最多 32 项。set 原子替换，append 保留顺序和重复项且不会自动重试。通过遥测和 `GET /device/status` 观察播放状态；见 [播放器 provider](../proto/rpc/client-provided-to-server#音乐播放器)。

## MHS v0 硬件状态

以下 API Key owner-scoped 路由提供 GizClaw 自有的 MHS-inspired 预标准 v0，不声称官方 MHS 兼容。清单定义见 [RuntimeProfile](/zh/developing/gizclaw/services/runtime-profile#mhs-v0-硬件清单)。

| 路由 | 结果 |
| --- | --- |
| `GET /gizclaw/v1/device/mhs/v0/manifest` | 当前绑定 Profile 的 `{devices:[...]}`，离线可读，未配置时为空数组 |
| `POST /gizclaw/v1/device/mhs/v0/read` | 请求 `{states:[{device_id,state}]}`，返回 `{states:[{device_id,state,value}]}` |
| `PATCH /gizclaw/v1/device/mhs/v0/states` | 请求和返回均为 `{states:[{device_id,state,value}]}`，返回实际生效值 |

HTTP value 是普通 JSON bool/整数/number/string，enum 使用 string。每批 1–32 个唯一 key；Server 在转发前校验所有 key、写权限、类型、整数精度、范围、step 网格、enum 成员和字符串字节上限。失败返回 `400 INVALID_REQUEST`，不联系设备。请求使用同一份 bound manifest 快照验证响应；设备响应必须恰好覆盖请求的全部 key，不能遗漏、重复或添加 key，值的类型必须与清单一致、enum 值必须在 enum_values 中；不合规返回 `502 DEVICE_ERROR`。min/max/step 只约束写入，设备上报的值反映真实硬件状态，超出范围或不在 step 网格上时照常返回。

读写沿用 5 秒 device-control 路径和 owner 串行化：离线 `409 DEVICE_OFFLINE`，未安装 handler `501 DEVICE_UNSUPPORTED`，超时 `504 DEVICE_TIMEOUT`，设备 INVALID_ARGUMENT/OUT_OF_RANGE 为 `400 DEVICE_REJECTED`。设备可以因硬件版本缺少某个部件返回 NOT_FOUND，映射为 `404 MHS_STATE_NOT_FOUND`；FAILED_PRECONDITION 和其他设备错误映射为脱敏的 `502 DEVICE_ERROR`。设备必须先验证整批再修改，不能部分成功；驱动自行执行安全限制，可以 clamp/round，并返回实际值。超时不证明写入未发生，调用方应重新读取确认。
