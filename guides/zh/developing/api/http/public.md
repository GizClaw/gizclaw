# Public API

Public/Peer HTTP 提供连接入口、设备管理和面向设备所属用户的能力。路径、参数、请求、响应及错误码由 [peer.json](https://github.com/GizClaw/gizclaw/blob/main/api/http/peer.json) 定义，可通过 [API Reference](/api/) 浏览。

WebRTC signaling 在 Peer connection 建立前运行；连接后的能力由 HTTP service stream 或 RPC 承载。选择入口时关注调用者身份与资源所属范围，不为相同能力另建一套契约。认证、handler 与领域服务的组装见 [Peer service](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_peer_http.go)。

## Peer 状态同步

同步用于让控制 App 收敛到设备所属范围内的当前状态，不是历史事件日志。使用服务端返回的检查点继续同步，不根据客户端时钟构造检查点。

客户端暂存一轮变更，收到完成事件后再一起提交状态与检查点；断线或取消时丢弃未完成的一轮。Server 要求重置时重新读取全量。具体事件结构见 Schema，存储说明见 [Peer Sync](../../gizclaw/services/runtime/peersync)。

## 设备与 Contact surface

设备与联系人接口按已认证调用者的权限访问目标 Peer。API Key 的 owner 由绑定决定；签名 Bearer 按当前 Admin/Client 权限授权目标。调用方与目标身份不能混为一体。

普通状态查询读取 Server 保存的投影，不代表刚刚向物理设备重新采样。需要实时硬件结果时，选择对应设备操作；联系人等持久化资源则由领域服务处理，设备离线不等于这些资源不可读。

## 好友与群组 surface

HTTP 与 Peer RPC 复用同一组 Social 服务。群名属于当前成员自己的命名空间，权限取决于真实成员关系；App 不应根据显示名或缓存列表推导权限。

列表里的在线与在房状态来自共享快照，可能受复制延迟影响。它们适合展示；实际通知是否送达仍以设备确认结果为准。交互与存储边界见 [Social](../../gizclaw/services/social)。

## 设备控制流程

控制请求先经过 Server 的身份、资源和参数校验，再转发到设备。硬件状态读写与预定义操作分别通过 MHS 和设备过程处理，具体请求由 Schema 与生成 SDK 定义。

操作接受与操作完成是不同阶段。重启、网络切换、恢复出厂设置和升级可能让设备断线；不要因超时自动重放具有副作用的操作，应重新读取状态确认结果。Server 校验不能代替设备自己的安全限制。

设备能力如何发现与调用，见 [Client provider](../proto/rpc/client-provided-to-server)；协议入口见 [RPC Reference](/references/rpc)。

## 设备调试访问与匿名标识查询

调试访问由设备所属 Server 的当前权限控制。公钥用于选择设备，本身不等于读写授权；控制 App 应区分调试访问、API Key 和签名 Bearer 的身份来源。

匿名标识查询只帮助发现匹配公钥。SN 与 IMEI 是设备声明的标识，不能保证唯一，也不能替代后续认证。具体入口与权限声明见 Peer HTTP Schema。

## 音乐点播

播放器操作通过设备过程入口执行，播放状态通过状态查询与遥测观察。App 应将播放列表编辑、播放指令接受和实际播放状态分开处理；支持的过程和参数见 [播放器说明](../proto/rpc/client-provided-to-server#音乐播放器)。

## MHS v0 HWD

先从 RuntimeProfile manifest 发现硬件实例，再对选中的实例读写。manifest 描述实例身份，HWD Schema 定义状态结构；设备驱动负责实际硬件约束。

写入成功使用设备返回的实际值更新界面。超时不证明写入没有发生，重试前先读取确认。说明见 [RuntimeProfile 硬件清单](../../gizclaw/services/runtime-profile#mhs-v0-硬件清单)。

## Lua 应用上传

应用安装可以流式上传二进制包，或由设备从 URL 下载。选择方式时遵守 Schema 中的包大小与传输约束，不在 App 中拆成另一套安装协议。Server 与 Edge 转发数据并保留背压和取消，设备负责校验与安装结果。

包格式与设备侧接入说明见 [Lua 应用 provider](../proto/rpc/client-provided-to-server#lua-应用)。

## 签名 Bearer

签名 Bearer 证明调用方持有对应私钥，资源权限仍由 Server 按当前角色和目标关系判断。Edge 路由不能扩大调用者权限。Wire 定义见 `peer.json` 的扩展，实际接入见 [API 使用说明](/zh/using/api#签名-bearer-认证)。
