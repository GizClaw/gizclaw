# 当前 Worktree 问题

本文记录编写项目指引和审查当前代码时确认的问题。

复选框表示是否已经知道明确的解决方案：

- `[x]`：解决方案已经明确，尚不表示代码已经修改。
- `[ ]`：尚无确定方案，仍需设计决策或进一步调查。

## TOC

- [x] [P1 - README 对 Edge Server Mesh 的当前状态描述冲突](#p1-readme-对-edge-server-mesh-的当前状态描述冲突)
- [x] [P2 - 删除未使用的 label set observability 机制](#p2-删除未使用的-label-set-observability-机制)
- [x] [P3 - Offer 应统一为一种认证方式](#p3-offer-应统一为一种认证方式)
- [x] [P4 - Edge RPC 文件名没有遵循 rpc 前缀](#p4-edge-rpc-文件名没有遵循-rpc-前缀)
- [x] [P5 - PeerConn Ping 实现放错文件](#p5-peerconn-ping-实现放错文件)
- [x] [P6 - RPC Client 与 Server 实现文件拆分过碎](#p6-rpc-client-与-server-实现文件拆分过碎)

## P1 - README 对 Edge Server Mesh 的当前状态描述冲突

### 问题描述

`README.md:16-17` 将 GizClaw 描述为已经可用的 “edge server mesh”，但 `README.md:65-71` 又将 self-organizing server mesh 明确列为未来工作。

Roadmap 的 `README.md:67-69` 仍把 Edge Node ingress 标记为未实现；当前代码中已经存在 `pkgs/gizedge`、`edge` command、`ServiceEdgeHTTP`、Server Edge Node authorization 和对应测试。

这使开发者无法区分三种边界：已经实现的单 upstream Edge ingress、尚未实现的分布式 Server Mesh，以及仍然需要完成的 Edge 能力。用户侧功能声明也与代码现状不一致。

### 解决方案

将当前产品描述为 Agent Runtime 与 Edge Server，并明确已经实现的是 single-upstream Edge ingress。

Roadmap 只保留尚未实现的 distributed membership、global routing、cross-Server synchronization、certificate control plane 等 Mesh 能力。将 Edge ingress 条目标记完成，或者替换为剩余的具体工作。

需要同步检查 `README.md`、`docs/edge-node.md`、`docs/server_mesh.md` 和项目指引，确保它们使用同一条当前/未来边界。

## P2 - 删除未使用的 label set observability 机制

### 问题描述

`pkgs/gizclaw/http_utils.go:64-78` 会将 `HTTPLabelSet` 写入 request context，但没有生产代码读取这些 label。

`LogAttr`、内部 `labelSet` reader 和 `GenxLabelSet` 只被 `label_set_test.go` 使用。`http_utils.go:77` 在 inner handler 返回后写入 response status，但返回的新 context 立即被丢弃。

因此这套抽象增加了 request path 上的 allocation 和维护成本，却不会产生 log、trace、metrics 或其他可观察行为。代码还会让开发者误以为 HTTP/GenX label 已经被记录。

### 解决方案

删除以下代码：

- `pkgs/gizclaw/label_set.go`
- `pkgs/gizclaw/label_set_test.go`
- `httpLabelSetHandler`
- Server composition 中安装 `httpLabelSetHandler` 的调用点

如果未来重新需要 request observability，应从明确的 log、trace 或 metrics consumer 出发设计，不恢复当前只写不读的 context 机制。

## P3 - Offer 应统一为一种认证方式

### 问题描述

`cmd/internal/server/server.go:57-60` 在 `serve-to-clients=false` 时要求 `/webrtc/v1/offer` 具有有效的 private-ingress session，只有 `/login` 绕过外层检查。

`pkgs/gizedge/edge.go:98-179` 因此需要先登录、缓存 Bearer session，再提交 Edge-to-Server WebRTC offer。与此同时，底层 Giznet signaling 已使用 caller public key、timestamp、nonce、replay protection 和 Server security policy 验证加密 offer。

Device signaling 直接使用 Giznet signaling authentication，而 Edge upstream signaling 额外依赖 public login/session。相同的 `/webrtc/v1/offer` 因入口不同形成了两套认证前置条件。

这意味着 login authorization、session storage 或 private-ingress policy 的变化，都可能阻止一个配置有效的 Edge Node 建立 upstream connection。

### 解决方案

`/webrtc/v1/offer` 统一只使用 Giznet signaling authentication。Edge 必须与 Device 使用完全相同的加密 Offer 签名格式、签名字段和 Server 校验流程，由 Offer 中的 caller public key、timestamp、nonce、signature 和 replay protection 完成身份认证。

- Device 与 Edge 必须调用同一个 Offer 构造和签名实现，不能分别维护两套逻辑。
- Server 必须使用同一个 `/webrtc/v1/offer` handler 完成解码、验签、时钟窗口检查、nonce/replay 检查和 Answer 生成，不能增加 Edge 专用验签分支。
- Server private ingress 应将 `/webrtc/v1/offer` 视为由 signaling handler 自行认证的入口，不要求 Bearer session。
- Edge 建立 upstream connection 时直接提交加密 offer，删除预登录、session cache、Bearer header 和 401 后刷新 session 的流程。
- Device 与 Edge 使用同一套共享代码、Offer wire contract、签名生成方式和签名验证方式，不能为 Edge 增加第二套实现或 Bearer/session 认证。
- 签名认证通过后，Device 与 Edge 的差异只体现在 role 和 service authorization，由 Server security policy 判断。
- Public login/session 仍用于需要 HTTP session 的其他 public/private API，不参与 WebRTC offer 认证。

测试应让 Device 与 Edge 使用同一个签名 helper，并分别证明有效签名成功、无效签名失败、重放失败；不能通过复制两组等价测试掩盖实现分叉。

需要同步修改 `cmd/internal/server`、`pkgs/gizedge`、共享 Giznet/WebRTC signaling 代码、private-ingress tests、Server 配置文档和 Edge 开发指引。

## P4 - Edge RPC 文件名没有遵循 rpc 前缀

### 问题描述

`pkgs/gizclaw/edge_service_rpc.go` 定义 `edgeRPCServer`、RPC dispatch、RPC result 编码和 RPC error mapping，但文件名使用 `edge_service_rpc` 前缀。

同一根 package 中其他 RPC 实现统一使用 `rpc_*`，例如 `rpc_server.go`、`rpc_server_service.go`、`rpc_firmware.go` 和 `rpc_utils.go`。当前命名让 Edge RPC 看起来像独立于 RPC subsystem 的 Edge service 文件，也导致开发指引需要额外建立一个入口。

### 解决方案

将实现文件重命名为 `rpc_edge.go`，对应测试文件重命名为 `rpc_edge_test.go`。不改变 package、类型、RPC method 或运行行为。

重命名后将其与其他 `rpc_*` 文件共同维护；Edge transport 的打开位置仍可位于 Peer connection/service 接线中，但 RPC dispatch 与 codec/error helper 统一归入 RPC 文件组。

## P5 - PeerConn Ping 实现放错文件

### 问题描述

`PeerConn.Ping` 和只供它调用的 `rpcConn` 当前位于 `peer_conn.go`。这两个函数只负责打开 `ServicePeerRPC` stream、执行通用 Ping RPC 并关闭 stream，不参与 Peer connection 的初始化、service serving、packet processing 或 lifecycle cleanup。

实现因此被放在 Connection 文件中，但职责属于 `rpc_all.go` 的通用 Ping RPC。文档如果按当前文件位置归类，也会把 RPC API 错误解释为 Connection 能力。

### 解决方案

保持公开 API `PeerConn.Ping` 的 receiver、签名和行为不变，将 `PeerConn.Ping` 与私有 `rpcConn` helper 从 `peer_conn.go` 移动到 `rpc_all.go`。

同步移动或调整对应测试，使 Connection 测试关注 connection lifecycle，RPC Ping 测试关注 stream 打开、请求响应、错误和关闭行为。该重组不修改 wire contract，也不影响现有调用方。

## P6 - RPC Client 与 Server 实现文件拆分过碎

### 问题描述

RPC Client 当前拆为 `rpc_client.go` 和 `rpc_client_service.go`。前者只定义无状态的 `rpcClient` 空结构，后者只实现该类型的 `GetClientInfo` 与 `GetClientIdentifiers` 方法，两者并不形成独立模块边界。

RPC Server 当前拆为 `rpc_server.go`、`rpc_server_foundation.go` 和 `rpc_server_service.go`。后两个文件分别保存未实现 method 的辅助判断，以及 `rpcServer` 的 Peer、runtime、run 和 workspace handlers；它们都直接服务于 `rpc_server.go` 中同一个 `rpcServer` dispatch，不是可独立使用的 subsystem。

当前拆分让代码和开发指引都出现 `Client` / `Client Service`、`Server` / `Server Foundation` / `Server Service` 等细碎入口。文件名表达的是实现切片，而不是开发者需要理解的模块边界。

### 解决方案

将 RPC Client 合并为一个 `rpc_client.go`：保留 `rpcClient`，并移入 `rpc_client_service.go` 中的 Client info 与 identifiers methods，删除 `rpc_client_service.go`。

将 RPC Server 合并为一个 `rpc_server.go`：保留 composition、dependency interfaces、request loop 和 dispatch，并移入 `rpc_server_foundation.go` 与 `rpc_server_service.go` 中的未实现 method 处理和全部 Server handlers，随后删除这两个拆分文件。

最终以两个文件表达两个模块：

```text
rpc_client.go  # RPC Client 类型及全部 Client methods
rpc_server.go  # RPC Server composition、dispatch 及全部 Server methods
```

该重组不改变 RPC method、wire contract、公开 Go API 或运行行为。测试与开发指引同步收敛为 `Client` 和 `Server` 两个入口；其他具有独立协议或数据流边界的文件，例如 firmware download、gameplay assets、streaming、speed test 和 Edge RPC，继续独立维护。

## 验证

```sh
go test ./pkgs/gizedge ./pkgs/gizclaw/... -count=1
npm run guides:build
git diff --check
```

记录这些问题时，上述命令均已通过。
