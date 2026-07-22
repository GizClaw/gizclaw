# RPC API Reference

本页由 `api/proto/rpc/rpc.proto` 的当前 registry 核对生成，列出全部 94 个 RPC method 及其用途。Method name 是调用时使用的稳定标识；数字 ID 是 Protobuf wire value，不应在应用代码中手写。TypeScript 使用 `RPC_METHODS`，Go 使用 `gizcli.Client` 的 typed 方法或 `rpcapi` registry。

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
| 91 | `server.register` | 使用 RegistrationToken 为当前 Peer 选择 RuntimeProfile，持久化并返回可选 Firmware ID。 |
| 94 | `server.peer.delete` | 原子删除当前 Peer 并写入 pending-deletion handoff；返回空 acknowledgement 与 EOS 后关闭连接。 |

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
| 21 | `server.run.say` | 请求当前 run 使用 RuntimeProfile `voice_alias` 播报文本。 |

## Firmware

Firmware 不属于 RuntimeProfile catalog。RegistrationToken 可以为 Peer 绑定一个 Firmware release-line；设备不列举或选择 Firmware，只在下载时选择 channel。

| ID | Method | 作用 |
| ---: | --- | --- |
| 22 | `server.firmware.get` | 根据当前 Peer 绑定的 Firmware ID 返回 release-line metadata 与 slots。 |
| 23 | `server.firmware.files.download` | 按 channel 和 path 流式下载当前 Peer 绑定的 Firmware artifact 文件。 |

## Workspace 与 history

| ID | Method | 作用 |
| ---: | --- | --- |
| 24 | `server.workspace.list` | 按必填 Collection 精确筛选并分页列出当前 Peer 的 Workspace。 |
| 25 | `server.workspace.get` | 按 name 读取一个 Workspace。 |
| 26 | `server.workspace.create` | 使用 Collection 与 RuntimeProfile `workflow_alias` 创建当前 Peer 的 Workspace。 |
| 27 | `server.workspace.put` | 更新当前 Peer 拥有的 Workspace 配置。 |
| 28 | `server.workspace.delete` | 原子删除当前 Peer 拥有的用户 Workspace 并写入 pending-deletion handoff；system Workspace 不可删除。 |
| 29 | `server.workspace.history.list` | 分页列出指定 Workspace 的 history。 |
| 30 | `server.workspace.history.get` | 读取指定 Workspace 的一条 history。 |
| 31 | `server.workspace.history.audio.get` | 返回 history 音频 metadata，并通过 binary frames 传输音频 bytes。 |
| 89 | `server.workspace.icon.download` | 按 Workspace name 和格式返回 icon metadata，并通过 binary frames 传输图片 bytes。 |

## Workflow、Model 与 Voice catalog

Workflow、Model 与 Voice 由当前 RuntimeProfile 投影为安全 alias catalog。响应携带 RuntimeProfile name 与 revision，不暴露真实资源 ID、provider、tenant、credential 或 ownership。真实资源统一通过 Admin API 管理。

| ID | Method | 作用 |
| ---: | --- | --- |
| 32 | `server.workflow.list` | 按必填 Collection 分页列出当前 RuntimeProfile 的 Workflow aliases。 |
| 33 | `server.workflow.get` | 按全局唯一 alias 读取 RuntimeProfile Workflow projection。 |
| 34 | `server.model.list` | 分页列出当前 RuntimeProfile 的 Model aliases。 |
| 35 | `server.model.get` | 按 alias 读取 RuntimeProfile Model projection。 |
| 36 | `server.voice.list` | 分页列出当前 RuntimeProfile 的 Voice aliases。 |
| 37 | `server.voice.get` | 按 alias 读取 RuntimeProfile Voice projection。 |

## Contact 与 Friend

| ID | Method | 作用 |
| ---: | --- | --- |
| 38 | `server.contact.list` | 分页列出当前 Peer 的联系人。 |
| 39 | `server.contact.get` | 按 ID 读取当前 Peer 的联系人。 |
| 40 | `server.contact.create` | 为当前 Peer 创建联系人。 |
| 41 | `server.contact.put` | 更新当前 Peer 的联系人。 |
| 42 | `server.contact.delete` | 删除当前 Peer 的联系人。 |
| 43 | `server.friend.invite_token.get` | 读取当前 Peer 仍有效的好友邀请码及过期时间。 |
| 44 | `server.friend.invite_token.create` | 为当前 Peer 创建或轮换好友邀请码。 |
| 45 | `server.friend.invite_token.clear` | 清除当前 Peer 的好友邀请码。 |
| 46 | `server.friend.add` | 使用另一个 Peer 的好友邀请码建立好友关系。 |
| 47 | `server.friend.list` | 分页列出当前 Peer 的好友关系。 |
| 48 | `server.friend.delete` | 删除一条好友关系及其关联资源。 |
| 90 | `server.friend.info.get` | 读取指定好友对当前 Peer 可见的 name 和 emoji。 |

## Friend Group 与消息

| ID | Method | 作用 |
| ---: | --- | --- |
| 49 | `server.friend_group.list` | 分页列出当前 Peer 加入的 Friend Group。 |
| 50 | `server.friend_group.get` | 按 ID 读取 Friend Group 及当前 Peer 的 group role。 |
| 51 | `server.friend_group.create` | 创建 Friend Group。 |
| 52 | `server.friend_group.put` | 更新 Friend Group 的 name 或 description。 |
| 53 | `server.friend_group.delete` | 删除 Friend Group 及其关联资源。 |
| 54 | `server.friend_group.invite_token.get` | 读取指定 Friend Group 的邀请码及过期时间。 |
| 55 | `server.friend_group.invite_token.create` | 为指定 Friend Group 创建或轮换邀请码。 |
| 56 | `server.friend_group.invite_token.clear` | 清除指定 Friend Group 的邀请码。 |
| 57 | `server.friend_group.join` | 使用邀请码加入 Friend Group。 |
| 58 | `server.friend_group.members.list` | 分页列出指定 Friend Group 的成员。 |
| 59 | `server.friend_group.members.add` | 向 Friend Group 添加成员并设置 member/admin role。 |
| 60 | `server.friend_group.members.put` | 修改 Friend Group 成员的 member/admin role。 |
| 61 | `server.friend_group.members.delete` | 从 Friend Group 删除成员。 |
| 62 | `server.friend_group.messages.list` | 分页列出 Friend Group 的音频消息 metadata。 |
| 63 | `server.friend_group.messages.get` | 读取一条 Friend Group 音频消息 metadata。 |
| 64 | `server.friend_group.messages.send` | 向 Friend Group 发送带 content type 和可选 TTL 的音频消息。 |

## Gameplay

| ID | Method | 作用 |
| ---: | --- | --- |
| 65 | `server.badge_def.pixa.download` | 返回 Badge Definition 的 PIXA metadata，并通过 binary frames 传输素材 bytes。 |
| 66 | `server.pet.list` | 分页列出当前 Peer 的 Pet。 |
| 67 | `server.pet.get` | 按 ID 读取当前 Peer 的 Pet。 |
| 68 | `runtime.adopt` | 按当前 connection 的 RuntimeProfile 领养 Pet；可提供 peer-scoped Pet ID，使重复请求返回已有 Pet 和原始 transaction 而不重复扣费。 |
| 69 | `server.pet.put` | 修改当前 Peer 的 Pet display name。 |
| 70 | `server.pet.delete` | 原子删除当前 Peer 的 Pet 并写入 Pet pending-deletion handoff；保留绑定的 system Workspace。 |
| 71 | `server.pet.drive` | 对 Pet 执行 action 或提交 game result，并原子返回 Pet、Points、Badge 与 reward 变化。 |
| 72 | `server.points.get` | 读取当前 Peer 与 RuntimeProfile 的 Points account。 |
| 73 | `server.points.transactions.list` | 分页列出 Points transactions。 |
| 74 | `server.points.transactions.get` | 按 ID 读取一条 Points transaction。 |
| 75 | `server.badge.list` | 分页列出当前 Peer 的 Badge。 |
| 76 | `server.badge.get` | 按 ID 读取当前 Peer 的 Badge。 |
| 77 | `server.game_result.list` | 分页列出当前 Peer 的 Game Result。 |
| 78 | `server.game_result.get` | 按 ID 读取一条 Game Result。 |
| 79 | `server.reward_grant.list` | 分页列出当前 Peer 的 Reward Grant。 |
| 80 | `server.reward_grant.get` | 按 ID 读取一条 Reward Grant。 |
| 87 | `server.pet.actions.get` | 读取指定 Pet 当前可用的 actions、效果、clip 映射和 i18n catalog。 |
| 88 | `server.pet.pixa.download` | 返回指定 Pet 对应的 PIXA metadata，并通过 binary frames 传输素材 bytes。 |

## Tool

Tool 同样由当前 RuntimeProfile 投影为安全 alias catalog；Peer 不能创建、修改或删除真实 Tool。

| ID | Method | 作用 |
| ---: | --- | --- |
| 81 | `server.tool.list` | 分页列出当前 RuntimeProfile 的 Tool aliases。 |
| 82 | `server.tool.get` | 按 alias 读取 RuntimeProfile Tool projection。 |
| 83 | `client.tool.invoke` | Server 请求 Client 执行本地 Tool，并用 `call_id` 关联真实执行结果。 |

## 独立流式语音

这两个 method 不创建或选择 Workspace。Transcribe 通过 request binary frames 增量上传音频；Synthesize 先返回音频 metadata，再通过 response binary frames 增量下载音频。Model 与 Voice 都使用当前 RuntimeProfile alias 解析。

| ID | Method | 作用 |
| ---: | --- | --- |
| 92 | `server.speech.transcribe` | 使用 `model_alias` 将有界音频流转换为最终 transcript。 |
| 93 | `server.speech.synthesize` | 使用 `voice_alias` 将文本合成为客户端接受格式的音频流。 |

## Edge RPC

以下方法由 Server 提供给 Edge-node，必须使用 `createEdgeRPCClient` 或对应的 Edge RPC transport；普通 `createPeerRPCClient` 不接受这些 method。

| ID | Method | 作用 |
| ---: | --- | --- |
| 84 | `server.peer.lookup` | 查询指定 Peer 当前的 Server assignment。 |
| 85 | `server.peer.assign` | 创建或更新 Peer assignment，并用 `expected_version` 检查并发冲突。 |
| 86 | `server.route.resolve` | 为目标 Peer 解析当前可用的 Server route/assignment。 |

## 未指定值

ID `0` 是 unspecified，不能调用。调用方遇到未知 method 时应按 method not found 处理。
