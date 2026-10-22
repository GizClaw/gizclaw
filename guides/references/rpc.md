# RPC API Reference

RPC 的 method 名称、数字 ID 和请求/响应类型由 [RPC registry](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/gizclaw_rpc.proto) 定义。参数与枚举见下列领域 Schema；调用方使用生成 SDK 或 typed registry，不手写数字 ID。

[Go RPC API](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi) · [Go SDK](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli) · [调用方向说明](/zh/developing/api/proto/rpc/overview) · [Streams](./streams#rpc-streams)

## 连接诊断与设备信息

用于确认连接通路、读取设备资料和注册设备。消息定义见 [system.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/system.proto)。

## Agent 与运行中的 Workspace

运行选择与持久化 Workspace 是不同层次：停止运行不等于删除 Workspace。请求定义见 [system.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/system.proto)，运行关系见 [Agent Host](/zh/developing/gizclaw/peer/agent-host)。

## Firmware

设备从已绑定的 Firmware 获取发布信息，自行下载并校验包；GizClaw 负责资源绑定与查询。见 [firmware.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/firmware.proto) 与 [Firmware 使用说明](/zh/developing/gizclaw/rpc/firmware)。

## Workspace 与 history

Workspace 管理配置，对话 History 保存已经交付的内容；回放通过独立下载获得音频。消息定义见 [workspace.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/workspace.proto)，使用方式见 [Workspace History](/zh/developing/gizclaw/rpc/workspace-history)。

## Workflow、Model 与 Voice catalog

Peer 使用当前 RuntimeProfile 中的名称访问资源，Admin 管理实际资源。投影消息见 [ai.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/ai.proto)，资源绑定说明见 [RuntimeProfile](/zh/developing/gizclaw/services/runtime-profile)。

## Contact 与 Friend

联系人目录与好友关系分别由所属服务管理。消息定义见 [social.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/social.proto)，权限与交互说明见 [Social](/zh/developing/gizclaw/services/social)。

## Friend Group

群名由当前成员的命名空间解析，群成员资格决定可见性。消息定义见 [social.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/social.proto)。

## Tool

RuntimeProfile 的服务端 Tool catalog 与设备提供的操作是不同能力。定义见 [tool.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/tool.proto)，组合方式见 [Runtime Tools](/zh/developing/gizclaw/services/runtime/toolkit)。

## App Config

App Config 用于向设备下发应用自己的不透明配置；设备按需读取，Server 不解释值的业务含义。消息定义见 [system.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/system.proto)。

## API Key

已认证的 Peer 管理自己的 API Key。消息定义见 [system.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/system.proto)，使用方式见 [API Key](/zh/using/api-keys)。

## 设备硬件状态与预定义操作

硬件状态读写与设备操作分开建模，设备只声明自己实际支持的能力。见 [Client provider 说明](/zh/developing/api/proto/rpc/client-provided-to-server)。

### ClientHwd v0

HWD 类型及其读写消息由 [mhs_v0.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/mhs_v0.proto) 定义；RuntimeProfile manifest 用于发现物理实例。

### ClientTool v0

操作枚举与消息映射由 [tool.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/tool.proto) 定义。调用前发现设备已安装的操作，不把 Server 认识某个枚举当成设备已支持它。

## 独立流式语音

独立语音调用按一次请求传输输入或输出，不使用 Workspace 的实时媒体轨。消息定义见 [ai.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/ai.proto)，传输说明见 [Binary streams](./streams#binary-streams)。

## Edge RPC

Edge 与已配置的 Server 通过内部能力协作路由和通知。消息定义见 [edge.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/edge.proto)，权限与方向见 [Server 与 Edge-node](/zh/developing/api/proto/rpc/server-provided-to-edge-node)。

## 未指定值

枚举的未指定值用于表示缺省状态，不是可调用能力。编码与错误处理以 [RPC codec](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/api/rpcapi) 为准。
