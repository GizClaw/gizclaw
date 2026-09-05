# RPC API Reference

本页由 `api/proto/rpc/rpc.proto` 的当前 registry 核对生成，列出全部 108 个 RPC method 及其用途。Method name 是调用时使用的稳定标识；数字 ID 是 Protobuf wire value，不应在应用代码中手写。TypeScript 使用 `RPC_METHODS`，Go 使用 `gizcli.Client` 的 typed 方法或 `rpcapi` registry。

`all.*` 由连接两端提供，`client.*` 由 Client/Device 提供，普通 `server.*` 与 `runtime.*` 由 Server 提供。最后一组 Edge RPC 使用独立 service `0x31`，只对 Edge-node 开放；其余方法使用 Peer RPC service `0x00`。

## 连接诊断与设备信息

| ID | Method | 作用 |
| ---: | --- | --- |
| 1 | `all.ping` | 验证 request/response 通路，并交换调用端发送时间与提供端当前时间。 |
| 2 | `all.speed_test.run` | 按指定上下行长度在 RPC stream 上执行吞吐测试。 |
| 3 | `client.info.get` | Server 从 Client 读取 manufacturer、model、hardware revision 等硬件信息。 |
| 4 | `client.identifiers.get` | Server 从 Client 读取 SN、IMEI 和设备 labels。 |
| 5 | `server.info.get` | 读取当前 Peer 在 Server 上的设备资料与标识信息。 |
| 6 | `server.info.put` | 更新当前 Peer 的 name、emoji 等可编辑设备资料，并返回完整资料。 |
| 7 | `server.runtime.get` | 读取当前 Peer 的在线状态、最后地址、最后在线时间和传输字节统计。 |
| 8 | `server.status.get` | 读取当前 Peer 最近上报的电量、充电、GNSS、音量、静音等状态。 |
| 90 | `server.register` | 使用 RegistrationToken 为当前 Peer 选择 RuntimeProfile，持久化并返回 RuntimeProfile name；可选 Firmware binding 仅保存在服务端。 |
| 93 | `server.peer.delete` | 原子创建或复用当前 Peer 的 pending-deletion handoff，同时保留 Peer；立即拒绝当前连接的新工作，尝试返回空 acknowledgement 与 EOS，随后无条件关闭完整连接。 |

## Agent 与运行中的 Workspace

| ID | Method | 作用 |
| ---: | --- | --- |
| 9 | `server.run.agent.get` | 读取当前选中的运行 Agent。 |
| 10 | `server.run.agent.set` | 选择运行 Agent，并返回选择后的 Agent 状态。 |
| 11 | `server.run.workspace.get` | 读取当前、待切换和已选 Workspace 及其运行状态。 |
| 12 | `server.run.workspace.set` | 选择要运行的 Workspace，并返回切换后的状态。 |
| 13 | `server.run.workspace.reload` | 重新加载当前 Workspace 的运行实例。 |
| 14 | `server.run.workspace.history` | 分页读取当前运行 Workspace 的 history。 |
| 15 | `server.run.workspace.history.play` | 请求播放一条当前 Workspace history 的音频。 |
| 16 | `server.run.workspace.memory.stats` | 读取当前 Workspace memory/recall backend 的统计信息。 |
| 17 | `server.run.workspace.recall` | 按 query 和 filters 从当前 Workspace memory 中召回内容。 |
| 18 | `server.run.reload` | 重新加载当前完整 run。 |
| 19 | `server.run.status` | 读取 run 的 state、时间、Workspace 和错误/状态消息。 |
| 20 | `server.run.stop` | 停止当前 run，并返回停止后的状态。 |
| 21 | `server.run.say` | 请求当前 run 使用 RuntimeProfile `voice_name` 播报文本。 |

## Firmware

Firmware 不属于 RuntimeProfile catalog。RegistrationToken 可以为 Peer 绑定一个 Firmware；设备不列举或持久化选择 Firmware channel，只请求具体 channel 的 external package 配置。

| ID | Method | 作用 |
| ---: | --- | --- |
| 22 | `server.firmware.get` | 根据当前 Peer 绑定的 Firmware 和 request channel，返回 HTTPS `.tar.zlib` URL、SHA-256 与 archive size。 |

## Workspace 与 history

| ID | Method | 作用 |
| ---: | --- | --- |
| 24 | `server.workspace.list` | 按必填 Collection 精确筛选并分页列出当前 Peer 的 Workspace。 |
| 25 | `server.workspace.get` | 按 name 读取一个 Workspace。 |
| 26 | `server.workspace.create` | 使用 Collection 与 RuntimeProfile `workflow_name` 创建当前 Peer 的 Workspace。 |
| 27 | `server.workspace.put` | 更新当前 Peer 拥有的 Workspace 配置。 |
| 107 | `server.workspace.input.put` | 只更新指定 Workspace 的 input mode，保留其余 parameters 与 toolkit；Workspace 继承 Workflow parameters 时由 Server 解析继承配置。 |
| 110 | `server.workspace.parameters.set` | 按当前 Workflow driver 更新 Workspace 的受支持参数，不修改 `agent_type`。 |
| 28 | `server.workspace.delete` | 为当前 Peer 拥有的用户 Workspace 原子创建或复用 pending-deletion handoff，同时保留 Workspace；system Workspace 不可删除。 |
| 29 | `server.workspace.history.list` | 分页列出指定 Workspace 的 history。 |
| 30 | `server.workspace.history.get` | 读取指定 Workspace 的一条 history。 |
| 31 | `server.workspace.history.audio.download` | 返回 history 音频 metadata，并通过 binary frames 传输音频 bytes。 |
| 88 | `server.workspace.icon.download` | 按 Workspace name 和格式返回 icon metadata，并通过 binary frames 传输图片 bytes。 |

`server.workspace.parameters.set` 的 `parameters` 是局部更新：当前支持 `input` 以及 `conversation.initiative`、`conversation.agent_initiative_policy`，未提供的字段保持不变。请求不接受 `agent_type`；Server 根据 Workspace 绑定的 Workflow driver 选择参数类型。字段不受该 driver 支持、枚举值无效或 patch 为空时返回 `BAD_REQUEST`。

## Workflow、Model 与 Voice catalog

Workflow、Model 与 Voice 由当前 RuntimeProfile 投影为 Peer name catalog。RuntimeProfile 内部 binding key 可以是 alias，但 Peer RPC 的对象 identity 统一使用 `name`，引用统一使用 `<kind>_name`。没有独立 Peer alias 的对象会把 canonical Admin ID 或内部 record ID 原样投影成 `name`。`display_name` 只表示展示文本，`actor_name` 只表示行为主体；业务 DTO 不使用 `id`，RPC envelope 的 correlation `id` 除外。响应携带 RuntimeProfile name 与 revision，不暴露 provider、tenant、credential 或 ownership。真实资源统一通过 Admin API 管理。

| ID | Method | 作用 |
| ---: | --- | --- |
| 32 | `server.workflow.list` | 按必填 Collection 分页列出当前 RuntimeProfile 的 Workflow names。 |
| 33 | `server.workflow.get` | 按 name 读取 RuntimeProfile Workflow projection。 |
| 34 | `server.model.list` | 分页列出当前 RuntimeProfile 的 Model names。 |
| 35 | `server.model.get` | 按 name 读取 RuntimeProfile Model projection。 |
| 36 | `server.voice.list` | 分页列出当前 RuntimeProfile 的 Voice names。 |
| 37 | `server.voice.get` | 按 name 读取 RuntimeProfile Voice projection。 |

## Contact 与 Friend

| ID | Method | 作用 |
| ---: | --- | --- |
| 38 | `server.contact.list` | 分页列出当前 Peer 的联系人。 |
| 39 | `server.contact.get` | 按当前 Peer 作用域内的 name 读取联系人。 |
| 40 | `server.contact.create` | 使用 caller-supplied name 为当前 Peer 创建联系人。 |
| 41 | `server.contact.put` | 按 name 更新当前 Peer 的联系人；name 不可变。 |
| 42 | `server.contact.delete` | 按 name 删除当前 Peer 的联系人。 |
| 43 | `server.friend.invite_token.get` | 读取当前 Peer 仍有效的好友邀请码及过期时间。 |
| 44 | `server.friend.invite_token.create` | 为当前 Peer 创建或轮换好友邀请码。 |
| 45 | `server.friend.invite_token.clear` | 清除当前 Peer 的好友邀请码。 |
| 46 | `server.friend.add` | 使用另一个 Peer 的好友邀请码建立好友关系。 |
| 47 | `server.friend.list` | 分页列出当前 Peer 的好友关系。 |
| 48 | `server.friend.delete` | 删除一条好友关系及其关联资源。 |
| 89 | `server.friend.info.get` | 读取指定好友对当前 Peer 可见的 name 和 emoji。 |

## Friend Group

| ID | Method | 作用 |
| ---: | --- | --- |
| 49 | `server.friend_group.list` | 分页列出当前 Peer 加入的 Friend Group。 |
| 50 | `server.friend_group.get` | 按当前成员自己的本地 name 读取 Friend Group 及当前 Peer 的 group role。 |
| 51 | `server.friend_group.create` | 使用 owner-local name 创建 Friend Group。 |
| 52 | `server.friend_group.put` | 按本地 name 更新 Friend Group 的 `display_name` 或 description；本地 name 不可变。 |
| 53 | `server.friend_group.delete` | 删除 Friend Group 及其关联资源。 |
| 54 | `server.friend_group.invite_token.get` | 读取指定 Friend Group 的邀请码及过期时间。 |
| 55 | `server.friend_group.invite_token.create` | 为指定 Friend Group 创建或轮换邀请码。 |
| 56 | `server.friend_group.invite_token.clear` | 清除指定 Friend Group 的邀请码。 |
| 57 | `server.friend_group.join` | 使用邀请码和 joining Peer 选择的本地 name 加入 Friend Group。 |
| 58 | `server.friend_group.members.list` | 分页列出指定 Friend Group 的成员。 |
| 59 | `server.friend_group.members.add` | 向 Friend Group 添加成员并设置 member/admin role。 |
| 60 | `server.friend_group.members.put` | 修改 Friend Group 成员的 member/admin role。 |
| 61 | `server.friend_group.members.delete` | 从 Friend Group 删除成员。 |

## Gameplay

| ID | Method | 作用 |
| ---: | --- | --- |
| 64 | `server.badge_def.pixa.download` | 按 BadgeDef name 返回 PIXA metadata，并通过 binary frames 传输素材 bytes。 |
| 65 | `server.pet.list` | 分页列出当前 Peer 的 Pet names。 |
| 66 | `server.pet.get` | 按当前 Peer 作用域内的 name 读取 Pet。 |
| 67 | `runtime.adopt` | 使用 caller-supplied peer-scoped Pet name 按当前 RuntimeProfile 领养 Pet。 |
| 68 | `server.pet.put` | 按 Pet name 修改 display name。 |
| 69 | `server.pet.delete` | 按 Pet name 原子创建或复用 pending-deletion handoff，同时保留 Pet 与绑定的 system Workspace。 |
| 70 | `server.pet.drive` | 按 Pet name 执行 action 或提交按 Game name 选择的 game result，并原子返回 Pet、Points、Badge 与 reward 变化。 |
| 71 | `server.points.get` | 读取当前 Peer 与 RuntimeProfile 的 Points account。 |
| 72 | `server.points.transactions.list` | 分页列出 Points transactions。 |
| 73 | `server.points.transactions.get` | 按 ID 读取一条 Points transaction。 |
| 74 | `server.badge.list` | 分页列出当前 Peer 的 Badge。 |
| 75 | `server.badge.get` | 按 ID 读取当前 Peer 的 Badge。 |
| 76 | `server.game_result.list` | 分页列出当前 Peer 的 Game Result。 |
| 77 | `server.game_result.get` | 按 ID 读取一条 Game Result。 |
| 78 | `server.reward_grant.list` | 分页列出当前 Peer 的 Reward Grant。 |
| 79 | `server.reward_grant.get` | 按 ID 读取一条 Reward Grant。 |
| 86 | `server.pet.actions.get` | 按 Pet name 读取当前可用的 actions、效果和 clip 映射。 |
| 87 | `server.pet.pixa.download` | 按 Pet name 返回对应 PIXA metadata，并通过 binary frames 传输素材 bytes。 |

## Tool

Tool 同样由当前 RuntimeProfile 投影为 Peer name catalog；Peer 不能创建、修改或删除真实 Tool。

| ID | Method | 作用 |
| ---: | --- | --- |
| 80 | `server.tool.list` | 分页列出当前 RuntimeProfile 的 Tool names。 |
| 81 | `server.tool.get` | 按 name 读取 RuntimeProfile Tool projection。 |
| 82 | `client.tool.invoke` | Server 请求 Client 执行本地 Tool，并用 `call_id` 关联真实执行结果。 |

## API Key

已认证的 Peer connection 是 API Key 的根管理入口；这些方法不需要 API Key，也不检查 `manage_api_keys`。

| ID | Method | 作用 |
| ---: | --- | --- |
| 96 | `server.api_key.create` | 为当前 Peer 创建绑定设备的长期 API Key，返回完整可恢复 credential。 |
| 97 | `server.api_key.list` | 按 cursor 分页列出当前 Peer 拥有的 API Key。 |
| 98 | `server.api_key.revoke` | 按 opaque name 撤销当前 Peer 拥有的一个 API Key。 |

## 设备控制与 Wi‑Fi

这一组 `client.*` 方法由设备的 `rpc_provider` 实现，Server 在处理 Public HTTP `/gizclaw/v1/device*` 控制请求时经在线 Peer connection 调用。设备只返回自身可执行的结果：参数非法返回 `INVALID_PARAMS`，未实现返回 `METHOD_NOT_FOUND`，`saved.forget` 找不到 ssid 返回 `NOT_FOUND`。`sound` 与 `ssid` 按 UTF‑8 bytes 限制 32。

| ID | Method | 作用 |
| ---: | --- | --- |
| 100 | `client.device.status.get` | Server 从设备读取实时 `PeerStatus`；用于控制响应回写，不由 `/device/status` 读取触发。 |
| 101 | `client.device.volume.set` | 设置绝对音量 `level`（0–100）与 `muted`，返回设备应用后的 `PeerStatus`。 |
| 102 | `client.device.sound.play` | 播放设备自定义的提示音 `sound`，可选 `duration_ms`。 |
| 103 | `client.device.reboot` | 设备在发出响应后重启，可选 `delay_ms`。 |
| 104 | `client.wifi.status.get` | 读取设备当前 Wi‑Fi 连接状态（`connected`、`ssid`、`rssi_dbm`、`ip`、`bssid`）。 |
| 105 | `client.wifi.saved.list` | 列出设备已保存的 Wi‑Fi 网络 `ssid`。 |
| 106 | `client.wifi.saved.forget` | 按 `ssid` 删除设备已保存的 Wi‑Fi 网络。 |
| 108 | `client.wifi.scan` | 在设备侧扫描周边 Wi‑Fi，按请求的有界 `timeout_ms` 返回接入点列表。 |
| 109 | `client.wifi.connect` | 接受 Wi‑Fi 凭据并在应答 RPC 后切换网络。 |
| 111 | `client.firmware.update` | 通知设备执行一次 OTA。可选 `channel` 指定要安装的 channel，省略时沿用设备自身的 channel；可选 `sha256` 声明调用方看到的目标包，与设备解析出的包不一致时设备拒绝。设备在应答后自行下载、校验、写入并重启。 |

## 独立流式语音

这些 method 不创建或选择 Workspace。Transcribe 与 Extract 通过 request binary frames 增量上传音频；Extract 在转写后按调用方提供的 JSON Schema 返回结构化 JSON；Synthesize 先返回音频 metadata，再通过 response binary frames 增量下载音频。Model 与 Voice 都使用当前 RuntimeProfile 中的 Peer name 解析。

| ID | Method | 作用 |
| ---: | --- | --- |
| 91 | `server.speech.transcribe` | 使用 `model_name` 将有界音频流转换为最终 transcript。 |
| 92 | `server.speech.synthesize` | 使用 `voice_name` 将文本合成为客户端接受格式的音频流。 |
| 94 | `server.speech.extract` | 使用 `asr_model_name` 与 `extract_model_name` 将有界音频转换为符合调用方 JSON Schema 的结果。 |

## Edge RPC

以下方法由 Server 提供给 Edge-node，必须使用 `createEdgeRPCClient` 或对应的 Edge RPC transport；普通 `createPeerRPCClient` 不接受这些 method。

| ID | Method | 作用 |
| ---: | --- | --- |
| 83 | `server.peer.lookup` | 只读查询指定 Peer 当前的固定 Server assignment。 |
| 84 | `server.peer.assign` | 原子 claim 缺少 assignment 的 Peer，或刷新同 owner metadata；其他 owner 返回 conflict，`expected_version` 不能转移归属。 |
| 85 | `server.route.resolve` | 只读解析目标 Peer 当前的固定 Server route/assignment。 |

## 未指定值

ID `0` 是 unspecified，不能调用。调用方遇到未知 method 时应按 method not found 处理。
