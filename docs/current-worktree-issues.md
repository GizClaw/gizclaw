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
- [x] [P7 - 旧 docs 目录应在 Guide 迁移完成后删除](#p7-旧-docs-目录应在-guide-迁移完成后删除)
- [x] [P8 - Project Guide 尚未发布到 GitHub Pages](#p8-project-guide-尚未发布到-github-pages)
- [x] [P9 - 删除未被消费的 depotstore 与 filesystem backend](#p9-删除未被消费的-depotstore-与-filesystem-backend)
- [x] [P10 - Object Store 覆盖写失败会破坏已有对象](#p10-object-store-覆盖写失败会破坏已有对象)
- [x] [P11 - Audio Resampler 在 EOF 时丢失缓冲尾部](#p11-audio-resampler-在-eof-时丢失缓冲尾部)
- [x] [P12 - 删除 Stamped Opus envelope 并直接传输 Opus frame](#p12-删除-stamped-opus-envelope-并直接传输-opus-frame)
- [x] [P13 - 收敛 API source 目录与 schema 文件边界](#p13-收敛-api-source-目录与-schema-文件边界)
- [x] [P14 - 删除面向 Client 的 Pet Definition PIXA RPC](#p14-删除面向-client-的-pet-definition-pixa-rpc)
- [x] [P15 - 将聚合 Pet Presentation RPC 拆为按需 Pet APIs](#p15-将聚合-pet-presentation-rpc-拆为按需-pet-apis)
- [x] [P16 - 统一 Transformer registry 并收敛 ASR/TTS facade 命名](#p16-统一-transformer-registry-并收敛-asrtts-facade-命名)
- [x] [P17 - Doubao Transformer provider I/O 绕过调用 context](#p17-doubao-transformer-provider-io-绕过调用-context)
- [x] [P18 - Doubao Realtime 与 Duplex 重复维护媒体和 Stream lifecycle](#p18-doubao-realtime-与-duplex-重复维护媒体和-stream-lifecycle)
- [x] [P19 - Doubao Push-to-Talk 缺少显式 turn 状态约束](#p19-doubao-push-to-talk-缺少显式-turn-状态约束)
- [x] [P20 - AST Translate interrupt 测试存在 Close 与 Recv 竞态](#p20-ast-translate-interrupt-测试存在-close-与-recv-竞态)
- [x] [P21 - Doubao Transformers 缺少跨 Adapter 回归测试门禁](#p21-doubao-transformers-缺少跨-adapter-回归测试门禁)

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

## P7 - 旧 docs 目录应在 Guide 迁移完成后删除

### 问题描述

仓库当前同时维护 `docs/` 与 `guides/` 两套文档入口。即使暂时把 `docs/` 定义为 normative source、把 `guides/` 定义为阅读层，开发者仍需判断同一主题应更新哪一份内容；文档迁移完成后继续保留两套树会造成重复、漂移和失效链接。

`README.md` 的 Repository Layout 与 Documentation 仍直接指向 `docs/*.md`，`AGENTS.md` 的 Go、JavaScript、C 和 documentation review rules 也仍指向 `docs/review-guide/*.md`。直接删除 `docs/` 会使这些入口和 README hero asset 立即失效。

### 解决方案

以 `guides/` 作为唯一项目文档根目录。逐项把 `docs/` 中仍有效的 contract、configuration、protocol、review guide、design 和 asset 内容迁移到对应 Guide 页面或 `guides/references/`；确认没有代码、workflow 或其他文档继续引用旧路径后，完整删除 `docs/` 目录。

当前迁移审计结果如下。`guides/` 尚未覆盖全部 `docs/` 内容，因此暂时不能执行目录删除：

| 旧文档 | 当前 Guide 覆盖情况 | 迁移要求 |
| --- | --- | --- |
| `docs/service_layout.md` | 基本覆盖 | 已进入 `gizclaw/overview`、Peer、Server、RPC 与 API Design；删除前做最终术语核对。 |
| `docs/agent_genx.md` | 部分覆盖 | GenX interface、Stream/EOS、Agent Host 和 Peer connection 已分散进入对应模块；仍需核对完整 wiring、audio mux 和 control RPC。 |
| `docs/edge-node.md` | 部分覆盖 | `gizedge` 已描述当前目录边界和连接流程；仍需迁移有效的 ingress、route、token、certificate 与端口约束，并排除未实现 proposal。 |
| `docs/event_stream.md` | 部分覆盖 | 已有 Stream Events 和 telemetry 页面，但 Agent、Telemetry、Opus stream 的完整 contract 尚未统一；Stamped Opus 的旧设计不得迁移为最终状态。 |
| `docs/review-guide/*.md` | 部分覆盖 | Review、Coding Styles 已建立新入口；仍需逐项确认 remote reviewer、跨语言 contract、验证和 finding 要求没有丢失。 |
| `docs/acl.md` | 未完整覆盖 | `services/system` 只描述目录职责；需补 ACL subject、resource kind、permission、collection create、runtime check 与 ownership 规则。 |
| `docs/context_config.md` | 未覆盖 | CLI 页面仍为 WIP；需补 context config 字段及 transport behavior。 |
| `docs/gameplay.md` | 未完整覆盖 | Gameplay 页面只有领域边界；需拆分资源模型、Pet、Points、Reward、storage 与 API surface。 |
| `docs/ota.md` | 未完整覆盖 | Device/Firmware 页面只有包边界；需补 Admin flow、Device flow、runtime status、storage 与 ACL。 |
| `docs/rpc_protocol.md` | 未完整覆盖 | RPC 页面已描述模块，但缺少 frame header、frame type、protobuf envelope 与 streaming response contract。 |
| `docs/server_config.md` | 未覆盖 | 需新增 Server 配置文档，包含 transport、logging、physical/logical stores 和 CLI context 关系。 |
| `docs/server_mesh.md` | 未覆盖 | 先区分已实现行为与 design proposal；已实现部分进入架构 Guide，未实现部分转正式 issue/design proposal。 |
| `docs/terms.md` | 未覆盖 | 需建立统一术语页，并让开发、审核和编码规范引用它。 |
| `docs/assets/readme-hero.png` | 未迁移 | 移到 Guide 管理的静态资源位置，并更新 README 引用。 |

迁移不是把旧 Markdown 原样复制到新目录。每一项都必须以当前代码、schema 和实际运行行为重新校验；proposal、旧名称和已经决定删除的机制不能作为现状写入 Guide。

迁移收尾时同步完成：

- 修改 `README.md` 的 Repository Layout 与 Documentation，使其只指向 `guides/` 页面或已发布的 Project Guide URL。
- 将 README hero 等仍需保留的静态资源移动到 Guide 管理的静态资源目录，修正 README 与页面引用。
- 修改 `AGENTS.md`，让各语言和文档 review 要求指向 `guides/` 下的新位置。
- 更新源码注释、测试、workflow、issue template 和其他 Markdown 中的 `docs/` 路径。
- 删除临时的 `current-worktree-issues` 页面；已解决事项由代码、最终 Guide 和 git history 表达，未解决事项转为正式 GitHub issues。
- 使用 `rg` 确认仓库不再存在有效的 `docs/` 路径引用，再删除整个目录。

删除必须发生在内容迁移和引用切换之后，不能先删目录再留下缺失的规范与 review contract。

## P8 - Project Guide 尚未发布到 GitHub Pages

### 问题描述

VitePress 当前只能通过本地 dev server 或 production build 查看。仓库没有 GitHub Pages deployment workflow，远程 reviewer 无法直接打开与 PR/main 对应的项目指引。

GizClaw 是 project site，发布地址默认位于 `https://gizclaw.github.io/gizclaw/`。如果构建时仍使用根路径 `/`，生成页面中的资源和导航链接会在该子路径下失效。

### 解决方案

增加独立的 GitHub Pages workflow：

- 在 `main` 的 `guides/**`、workflow 或 guide dependency 变化时触发，并允许 `workflow_dispatch`。
- 使用 `npm ci --prefix guides` 安装独立 Guide 依赖。
- 设置 `VITEPRESS_BASE=/gizclaw/` 后运行 `npm --prefix guides run build`。
- 上传 `guides/.vitepress/dist` Pages artifact，并由单独 deploy job 发布。
- Actions 必须固定到完整 commit SHA，并为 build/deploy jobs 配置最小权限和 concurrency。
- 在仓库 Settings → Pages 将 Source 设置为 GitHub Actions。

本地开发不设置 `VITEPRESS_BASE`，继续使用 `/`；若以后绑定 custom domain，再把部署环境的 base 调整为 `/`，不需要修改页面内容。

## P9 - 删除未被消费的 depotstore 与 filesystem backend

### 问题描述

`pkgs/store/depotstore` 定义通用文件操作接口 `Store` 和本地实现 `Dir`。当前生产代码对它的唯一引用来自 `cmd/internal/storage/storage.go`，用于注册 `KindFilesystem` backend，并通过 `Storage.FS` / `Storage.Filesystem` 返回 `depotstore.Dir`。

仓库中没有任何生产调用方调用 `Storage.FS` 或 `Storage.Filesystem`，也没有实际配置使用 `kind: filesystem`。现有行为只由 `pkgs/store/depotstore` 自身测试和 `cmd/internal/storage/storage_test.go` 的 registry 测试覆盖。因此它不是正在服务业务的 storage abstraction，而是一整条被构造、注册但从未消费的死功能。

仓库已经有实际使用的 `pkgs/store/objectstore`，其 filesystem-backed `objectstore.Dir` 提供受约束的 object key、读写、列举、prefix 删除和 expiration 语义。继续保留另一套无调用方的通用 filesystem store 会扩大 storage 配置和维护表面，也容易让新代码选择错误 abstraction。

### 解决方案

完整删除未使用的 filesystem storage surface：

- 删除 `pkgs/store/depotstore/` package 及其测试。
- 从 `cmd/internal/storage` 删除 `KindFilesystem`、`fss` registry、`Storage.FS`、`Storage.Filesystem`、`newFilesystem` 和 `newFilesystemDir`。
- 删除只验证 filesystem backend 构造、查找和错误分支的 storage tests。
- 保留 `FSConfig` 及 objectstore 的 `fs` driver，因为 `KindObjectStore` 仍使用它构造 `objectstore.Dir`；不能把 objectstore filesystem driver 一并删除。
- 更新 Project Guide、storage 配置示例和其他引用，确保不再把 `filesystem` 描述为受支持的独立 storage kind。

删除后运行全仓库搜索确认 `depotstore`、`KindFilesystem`、`Storage.FS` 和 `Storage.Filesystem` 均无残留，并执行 `go test ./...` 验证 storage configuration 与现有 objectstore consumers。

## P10 - Object Store 覆盖写失败会破坏已有对象

### 问题描述

`pkgs/store/objectstore/dir.go:70-89` 使用 `os.Create` 直接打开目标文件。`os.Create` 会先截断已经存在的对象；如果输入 reader 在 `io.Copy` 过程中返回错误，方法虽然向调用方返回失败，但原对象已经被替换为不完整内容。

实际使用一个中途失败的 reader 覆盖已有对象后，原对象内容被清空并只剩失败前写入的 `partial` 数据。该行为违反了失败写入不应破坏已提交对象的基本存储语义。HNSW 持久化等消费者也通过 Object Store 写入，因此一次中断的保存可能同时破坏上一份仍然有效的索引。

当前 metadata 也不是与对象内容一起提交：对象写完后 metadata 写入失败会删除目标文件，使原对象和原 deadline 一并丢失。

### 解决方案

将 `Dir.put` 改为同目录临时文件加原子替换：

- 在目标目录创建权限受控的临时文件，不直接截断目标文件。
- 只有完整复制、文件关闭以及必要的 sync 成功后才通过 rename 提交新对象。
- metadata 与对象替换必须形成明确的提交顺序；任何提交前错误都保留原对象及其原 deadline。
- 所有失败路径都关闭并删除临时文件，不留下可被 List 观察到的中间对象。
- 增加失败 reader 的新建与覆盖测试，并覆盖 metadata 写入失败，验证旧对象和旧 deadline 在失败后保持不变。

## P11 - Audio Resampler 在 EOF 时丢失缓冲尾部

### 问题描述

`pkgs/audio/resampler/resampler.go:125-179` 在 source 返回最后一批 samples 和 `io.EOF` 后，只调用 `Process`，从未调用底层 resampler 的 `Flush`。高质量重采样器会为滤波保留尾部样本；如果最后一次 `Process` 尚未产生输出，当前实现立即返回 `io.EOF`，缓冲音频永久丢失。

即使最后一次 `Process` 已经产生数据，只要输出大于调用方 buffer，当前实现也会在保存 `leftover` 的同时返回 `io.EOF`。遵循 `io.Reader` 约定、收到 EOF 后停止读取的调用方不会再来取这些 leftover，因此仍然会截断音频。

使用一个合法地在最后一次读取同时返回数据和 `io.EOF` 的 reader 可以稳定复现：Resampler 第一次读取直接得到 `n=0, err=EOF`，后续也没有任何尾部输出。

### 解决方案

为 `Soxr` 增加明确的 source EOF、flushed 和 draining 状态：

- 最后一批输入仍先交给 `Process`，随后只调用一次底层 `Flush`。
- 合并 Process 与 Flush 的输出，并允许通过多次 `Read` 完整排空 pending bytes。
- 只在所有 resampled output 和 leftover 都交给调用方后返回 `io.EOF`。
- 当一次读取既产生数据又到达 source EOF 时，不把 EOF 提前越过尚未交付的输出。
- 增加 source 返回 `(n, io.EOF)`、EOF 在下一次读取返回、短目标 buffer 和无需 sample-rate conversion 的测试，验证输出完整且 EOF 只在 drain 完成后出现。

## P12 - 删除 Stamped Opus envelope 并直接传输 Opus frame

### 问题描述

`pkgs/audio/stampedopus` 在每个 Opus frame 前增加自定义 timestamp envelope，`ProtocolStampedOpusPacket` 将该 envelope 暴露为 Giznet direct packet contract。但 WebRTC 接收路径 `pkgs/giznet/gizwebrtc/conn.go` 已经从 `TrackRemote.ReadRTP` 取得经过 WebRTC media pipeline 交付的 Opus frame，随后丢弃 RTP metadata，并用 `time.Now().UnixMilli()` 人工生成新的 timestamp。

相反方向写入 WebRTC 时，`writeOpus` 先解析 stamped envelope，随后完全忽略其中的 timestamp，只根据 Opus TOC 推导 duration 并调用 `audioTrack.WriteSample`。因此该 timestamp 既不是原始 RTP timestamp，也不参与 WebRTC jitter、排序或播放调度，只在 Giznet 与上层之间增加一层 framing。

这套 envelope 已扩散到 `pkgs/gizclaw`、Go SDK、C SDK、e2e tests 和协议文档，使所有 Opus frame consumer 都必须 Pack/Unpack 一个没有 transport 作用的字段。部分上层还把这个本地墙钟值写进 `genx.StreamCtrl.Timestamp`，让它看起来像可靠的媒体时间轴。

### 解决方案

让 Giznet Peer connection 的 Opus packet payload 直接等于单个 Opus frame：

- 保留内部 direct packet 的协议值 `0x10`，将常量重命名为 `ProtocolOpusPacket`；payload 直接是单个 raw Opus frame，不再包含自定义 header。
- `gizwebrtc.Conn.Read` 从 remote audio track 返回 frame 本身；`Conn.Write` 直接把 frame 写入 WebRTC audio track，并继续依据 Opus TOC 计算 sample duration。
- 删除 `pkgs/audio/stampedopus`、`lastOpusFrameTimestamp` 以及所有 Pack/Unpack 调用。
- `pkgs/gizclaw`、Go SDK、C SDK 和 e2e harness 统一直接读写 Opus frame；没有真实媒体 timestamp 时不再伪造 `StreamCtrl.Timestamp`。
- WebRTC/RTP 层继续负责 jitter、sequence 与 RTP timestamp；如果未来业务确实需要跨层媒体时间轴，应从 RTP metadata 定义独立、真实的 contract，不能恢复本地 `time.Now()` envelope。
- 同步更新 direct packet protocol 文档、terms、tests 与 Project Guide。该 packet 是 Giznet/WebRTC 适配层的内部协议，不保留 Stamped Opus wire compatibility、deprecated alias 或面向 Client 的兼容分支。

迁移测试应验证 raw frame 的 WebRTC 双向传输、Opus TOC duration、Go/C SDK packet bridge 和无额外 header 的 e2e round trip。

## P13 - 收敛 API source 目录与 schema 文件边界

### 问题描述

根 `api/` 当前将 HTTP OpenAPI、共享 JSON Schema、Admin Resources、RPC Protobuf 和 Telemetry Protobuf 混放在同一级。`type/` 与 `resource/` 使用单数目录名，并将很多只有一个 owner 的 Spec 和 value object 拆成独立文件。

`types.json` 同时聚合共享 DTO 与完整 Resource graph，而 Resources 又反向引用 `type/` 中的 Spec，形成不清晰的双向聚合。Peer HTTP 只需要少量共享 DTO，却通过同一入口连接到 Admin Resources。

RPC 核心协议也被不对称地拆到 `common.proto` 与 `peer.proto`：Response、Error 和 Stream Frame 位于前者，Request 与完整 method registry 位于后者。它们共同组成同一个 RPC envelope contract，并没有独立模块边界；`common.options` 实际只用于 nanopb 生成配置。

### 解决方案

将 API source 按格式与协议用途收敛为：

```text
api/
├── http/
│   ├── admin.json
│   ├── peer.json
│   ├── desktop.json
│   ├── openai-compat/
│   ├── shared.json
│   ├── shared/
│   ├── resources.json
│   └── resources/
└── proto/
    ├── rpc/
    │   ├── rpc.proto
    │   ├── nanopb.options
    │   └── payload/
    └── telemetry/
        └── peer_telemetry.proto
```

- `shared/` 只保存真正跨 surface 或跨领域复用的 contract；`shared.json` 不得反向聚合 Resources。
- `resources/` 保存 Admin 声明式 Resource。只有一个 Resource 使用的 Spec 直接内联；`resources.json` 单独聚合 Resource graph。
- 只有一个领域 owner 的小类型按领域合并，不按每个生成 symbol 拆文件。
- 将 `common.proto` 与 `peer.proto` 合并为 `rpc.proto`，统一保存 request、response、error、stream framing 和 method registry。
- 将 `common.options` 重命名为 `nanopb.options`，只表达 C generator configuration。
- `payload/` 继续按 AI、Edge、Firmware、Gameplay、Social、System、Workspace 等领域拆分；Telemetry 保持独立的 Protobuf protocol 目录。
- 更新全部 `$ref`、go:generate directives、JavaScript generator、C nanopb generator、README、Project Guide 与生成产物。

该重构只改变 source organization。不得顺带改变 JSON property、required/nullable 语义、OpenAPI operation ID、Protobuf field number、enum value、RPC method name 或 wire payload。

## P14 - 删除面向 Client 的 Pet Definition PIXA RPC

### 问题描述

RPC registry 暴露 `server.pet_def.pixa.download`，允许 Client 通过 Pet Definition ID 直接下载 PIXA。对应 request/response、Server stream handler、Go SDK wrapper 和 Peer Resource service 形成了一条独立的 Pet Definition 下载 surface。

Client 的产品边界是已经拥有或正在展示的 Pet，而不是 Admin 管理的 Pet Definition。Server 可以从 Pet 的 `petdef_id` 内部解析素材，不需要向 Client 暴露 Pet Definition 下载入口。继续保留 Pet Definition 下载让 Client 绕过 Pet 实例边界，并重复维护 PIXA contract。

### 解决方案

删除 `server.pet_def.pixa.download`，统一通过 Pet surface 获取展示与素材：

- 从 `rpc.proto` method registry 删除 `server.pet_def.pixa.download`，不复用其 enum value。
- 从 gameplay payload 删除 `PetDefPixaDownloadRequest` 与 `PetDefPixaDownloadResponse`。
- 删除 Server dispatch、`handlePetDefPixaDownload`、`PreparePetDefPixaDownload` service contract 和实现。
- 删除 Go SDK `DownloadPetDefPixa` wrapper、result 类型及相关 tests。
- Client 通过 `server.pet.pixa.get` 按 `pet_id` 获取 PIXA；Server 内部根据 Pet 的 `petdef_id` 解析 metadata 与素材。
- Admin 对 Pet Definition PIXA 的上传和管理仍属于 Admin HTTP API，不受该 RPC 删除影响。

同步重新生成 Go、JavaScript 与 C RPC surfaces，并验证 Pet PIXA 获取和 Admin Pet Definition management。

## P15 - 将聚合 Pet Presentation RPC 拆为按需 Pet APIs

### 问题描述

`server.pet.presentation.get` 按 `pet_id` 查询 Pet 与 PetDef 后，将 PetDef 的 attributes、drive actions、PIXA metadata、i18n catalog、路径和更新时间组装进一个大型 `PetPresentation` response。

Client 为读取 actions 或 PIXA 信息必须接收整份 presentation；PetDef 新增展示字段也会继续扩大该 RPC。该结构实际把大部分 PetDef 投影成 Client contract，与“Client 只围绕 Pet instance 获取所需能力”的边界冲突。

### 解决方案

删除 `server.pet.presentation.get`、`PetPresentation` 聚合 message、handler、SDK wrapper 和相关测试。按 Client 的实际读取场景提供小型 Pet APIs：

- `server.pet.actions.get`：输入 `pet_id`，只返回该 Pet 可用的 action ID、cost、effect 和必要 display values。
- `server.pet.pixa.get`：输入 `pet_id`，返回该 Pet 使用的 PIXA metadata 与素材 stream；Server 内部通过 `pet.petdef_id` 解析，不向 Client 暴露 PetDef lookup。
- `server.pet.get` 继续只返回 Pet instance state，不嵌入完整 PetDef 或 presentation。
- 其他 PetDef 数据只有出现明确 Client consumer 时才增加对应的 focused Pet method，不能恢复 catch-all presentation response。

同步修改 gameplay protobuf payload、RPC registry、Server dispatch、Peer Resource service、Go/JavaScript/C SDK 和 e2e tests。保留 Admin HTTP 的 Pet Definition 管理能力。

## P16 - 统一 Transformer registry 并收敛 ASR/TTS facade 命名

### 问题描述

`pkgs/genx/transformers` 当前同时维护 `DefaultMux`、`ASRMux` 和 `TTSMux` 三套 `pattern -> genx.Transformer` Trie。`ASR.Handle`、`TTS.Handle` 与通用 `Mux.Handle` 重复注册和查找逻辑，Model Loader 也明确将每个 ASR/TTS Adapter 同时注册到专用 mux 与 `DefaultMux` 以保持兼容。

生产代码没有调用 `ASR.Create`、`TTS.Synthesize` 或 `TTS.SynthesizeStream`；除 Model Loader 的重复注册外，专用 mux 只由 package tests 使用。因此当前运行链不需要三套 registry，但这些类型和函数已经属于公开 Go API，不能在未声明 breaking change 的情况下直接删除。

`ASR`、`TTS` 类型名还同时表达“能力类别”“Adapter registry”和“session factory”，容易与 `ASRSession`、`TTSSession` 淹没边界。它们本身不根据 EOS 自动切段：facade 只构造 buffer stream，`ASRSession.Close` 写入固定 `audio/opus` EOS，`TTSSession.Close` 和一次性 synthesis 写入 text EOS；具体 Adapter 消费 EOS 后 flush 并结束输出。

### 解决方案

只保留一套 Transformer registry，以现有通用 `Mux` 作为 `pattern -> genx.Transformer` 的唯一注册与选择边界：

- Model Loader 只调用通用 `Handle`，删除对 `HandleASR`、`HandleTTS` 的双重注册。
- 删除 `ASRMux`、`TTSMux`、`NewASRMux`、`NewTTSMux` 及 `ASR.Handle`、`TTS.Handle` 的独立 Trie。
- 删除 `ASR`、`TTS` facade 以及 `ASRSession`、`TTSSession` 等没有生产消费者的命令式包装；不新增 `NewASRSession`、`TTSSegment` 或其他替代 public API。
- 调用方直接向统一 Mux 选中的 Transformer 提供 `genx.Stream`。连续输入、分段和结束统一由 StreamID、BOS、data 与 EOS 表达。
- 保留 package-private 的公共 TTS stream pipeline，例如 `runTTSTransform`；它由 Doubao SeedV2、Doubao ICLV2 和 MiniMax Adapters 复用，不成为新的 public facade。
- Session close 只表达当前输入 stream 完成。EOS 的 MIME type 应来自 session/input contract，不能在通用 ASR helper 中固定为 `audio/opus`；多段边界继续由 StreamID 与 EOS 明确表达。
- 保留 `TTSAudioNormalizer`，因为它负责 provider-neutral 的 TTS 容器头处理，不承担 registry 或 session 职责。

由于当前专用 mux 是已发布 API，应在允许 breaking Go API 的版本完成删除，或先提供一轮 deprecated forwarding wrappers。迁移测试应覆盖 Model Loader 单次注册、pattern 选择、一次性 TTS、streaming TTS、streaming ASR、输入 MIME type 传播、EOS flush、关闭与错误传播。

## P17 - Doubao Transformer provider I/O 绕过调用 context

### 问题描述

Doubao realtime paths 已经从 `Transform` 接收 lifecycle context，但在 provider I/O 时多次改用 `context.Background()`：

- `doubao_realtime.go` 的 `Interrupt`、`EndASR`、`SendAudio`、`SendText` 和 VAD silence send；
- `doubao_realtime_duplex.go` 的 `CancelResponse`、`SendFunctionCallOutputs` 和 `SendAudio`；
- `DoubaoASRSAUC.Transform` 完全忽略传入 context，并在 goroutine 内从 `context.Background()` 创建独立 cancel context。

因此调用方取消 `Transform` 时，正在阻塞的 provider send、EndASR、interrupt 或 standalone ASR session 不一定退出。Realtime/duplex loop 可能无法执行 session close，output Stream 也可能迟迟无法得到 cancellation error。现有 race test 可以通过，但不能证明这些外部 I/O 在取消后结束。

### 解决方案

将 `Transform` context 作为整个 Adapter session 的唯一父 context：

- 所有 provider Open/Send/End/Interrupt/Cancel/FunctionCall API 使用 session context 或其有界 child context，不使用 `context.Background()`。
- `DoubaoASRSAUC.Transform` 不再丢弃 context；transform loop、session open、audio send、result receive 和 pacing timer 全部由该 context 控制。
- 如果 provider cleanup 需要在父 context 取消后继续短暂执行，使用显式 cleanup timeout，并限制在 `Close`/best-effort cleanup；正常业务请求不能脱离父 context。
- context 取消时关闭 input/output/session，确保阻塞的 `Next`、provider Recv 和 output Push 都能退出。

增加会阻塞 SendAudio、EndASR、Interrupt、CancelResponse 和 standalone ASR open/send 的 fake sessions；取消 context 后必须在有限时间内返回，且 output 暴露 `context.Canceled` 或对应终止状态。执行 `go test -race ./pkgs/genx/transformers -count=1`。

## P18 - Doubao Realtime 与 Duplex 重复维护媒体和 Stream lifecycle

### 问题描述

`doubao_realtime.go` 和 `doubao_realtime_duplex.go` 分别约 1792 行和 1724 行。两者各自复制了几乎同构的实现：

- audio format/sample-rate/channel defaults 与 MIME normalization；
- PCM、MP3、raw Opus 的 decode、encode、transcode 和 frame preparation；
- per-stream audio input map 与 codec cleanup；
- base input、segment、response StreamID 状态；
- assistant active/epoch、interrupt EOS 和 20ms output pacing；
- pending chunk、session restart、input EOS 与 output close/error handling。

这些并不是两个 provider protocol 的差异，而是 GenX media/stream pipeline 的公共职责。当前复制已经产生行为漂移：Realtime 在非 PTT EOS 上生成 VAD silence，而 Duplex 直接关闭 local input stream；两边的 done-aware input、event error propagation、helper 命名和 codec feature 也不同。任何 audio 或 interruption 修复都需要在两份千行实现中同步。

### 解决方案

抽取 package-private 的公共 realtime pipeline，不增加新的产品抽象：

- 统一 `realtimeAudioInput` 与 `realtimeAudioInputs`，保存 MIME validation、codec conversion、frame preparation 和 cleanup。
- 统一 `realtimeStreamIDs`，由 mode/policy 决定 segment ID 规则，不复制 mutex state。
- 统一 assistant response lifecycle，处理 active response、epoch、interrupted text/audio EOS 和 output pacing。
- 统一 pending input、restart、context cancellation 和 Stream close/error helper。
- Realtime 与 Duplex 仅实现 provider config、event mapping 和 session operations adapter。

重构必须保持现有 wire/audio behavior，并用同一组 table-driven contract tests 同时运行 Realtime 与 Duplex 的 PCM、MP3、Opus、BOS/EOS、interrupt、cancel、restart 和 provider error cases。

## P19 - Doubao Push-to-Talk 缺少显式 turn 状态约束

### 问题描述

`DoubaoRealtimeModePushToTalk` 当前能够在 EOS 时调用 `EndASR`，并有单次 `BOS -> audio -> EOS` 测试。但实现没有维护 Idle、Capturing、WaitingResponse 或 Responding 等输入状态：

- 任意 EOS 都会调用 `EndASR`，即使此前没有 BOS、没有 audio，或当前 turn 已经结束。
- EOS 后 loop 继续接受 audio，并可继续向同一个 provider session 发送。
- BOS 主要用于设置 StreamID 和在 assistant active 时触发 interrupt，不验证上一输入 turn 是否完成。
- duplicate EOS、audio before BOS、audio after EOS、BOS while capturing 和 EndASR 阻塞/失败均没有状态转换测试。

这使设备按钮抖动、重复 release event、丢失 press event或跨 turn 乱序 chunk 可以形成重复 `EndASR`、错误归属的 audio 或无法完成的 response。当前 happy-path 单测不能证明 Push-to-Talk contract 完整。

### 解决方案

为 Push-to-Talk 输入增加显式 turn state machine：

- `Idle -> Capturing` 只由 BOS 触发，并记录当前 StreamID。
- 只有 Capturing 接受 audio；Idle/WaitingResponse 收到 audio 返回 contract error。
- `Capturing -> WaitingResponse` 只接受一次 EOS，并只调用一次 `EndASR`。
- duplicate EOS、EOS before BOS 和 BOS while Capturing 返回明确错误，不静默改变 provider session。
- assistant 开始输出后进入 Responding；输出完成回到 Idle。
- WaitingResponse/Responding 收到新 BOS 时，按明确的 barge-in policy cancel/interrupt 上一 response，再进入新的 Capturing turn。
- context cancellation 和 provider EndASR/interrupt failure 必须关闭当前 turn 与输出 stream。

用 table-driven state tests 覆盖每个合法和非法 transition，并保留完整的 fake provider integration test，验证每个有效 turn 恰好调用一次 `EndASR`、输出使用正确 StreamID、barge-in 产生一次 interrupted EOS。

## P20 - AST Translate interrupt 测试存在 Close 与 Recv 竞态

### 问题描述

`go test -race ./pkgs/genx/transformers -count=1` 在 `TestDoubaoASTTranslateInterruptsActiveSessionOnNewInputStream` 中报告数据竞争。`DoubaoASTTranslate.interruptSession` 会在 event-forwarding goroutine 正在执行 `session.Recv()` 时调用 `session.Close()`；测试 fake 的 `Recv` 读取 `closeCh`，而 `Close` 在另一 goroutine 关闭后将同一字段写为 nil，没有使用同一把锁或 once 保护。

竞态直接发生在 test double，但它对应真实的 Adapter lifecycle：interrupt 依赖 `Close` 与正在阻塞的 `Recv` 并发执行，以唤醒 receiver。当前 session interface 和测试没有明确并验证 provider session 是否支持该并发契约，因此 race test 失败，也可能掩盖 double-close、close channel panic 或 receiver 无法退出的问题。

### 解决方案

- 明确 AST Translate session contract：`Close` 必须可与 `Recv` 并发调用，并使 Recv 在有限时间内返回。
- 修复 fake session，使 `closeCh` 在构造后保持不变，使用 `sync.Once` 关闭；不要在 `Recv`/`Close` 中并发修改 channel 字段。
- production interrupt path 只调用一次 Close，并等待对应 receiver goroutine 完成后再丢弃 session。
- 增加 concurrent interrupt、重复 Close、context cancel 与 provider Recv error tests。
- 以 `go test -race ./pkgs/genx/transformers -count=1` 作为该修复的必需验证，不能只运行非 race tests。

## P21 - Doubao Transformers 缺少跨 Adapter 回归测试门禁

### 问题描述

Doubao Realtime、Realtime Duplex、ASR、TTS 和 AST Translate 已包含大量单元测试，但测试主要跟随各实现文件分别维护。Realtime 与 Duplex 重复的 audio conversion、StreamID、interrupt、EOS、restart 和 pacing 没有共用 contract suite，因此修复其中一套实现时，测试无法约束另一套保持相同行为。

当前 package 普通测试通过，但 race test 已在 AST Translate interrupt 路径发现 Close/Recv 竞态。Push-to-Talk 也只有 happy-path `BOS -> audio -> EOS` 覆盖，缺少 duplicate EOS、audio before BOS、audio after EOS、BOS while capturing、context cancellation 和 provider call blocking。这说明测试数量较多，但还没有形成能够保护高风险状态机重构的门禁。

### 解决方案

在修改 Doubao Transformer 实现前建立分层回归测试体系：

- 公共 media contract suite：同一组 table-driven cases 覆盖 Realtime 与 Duplex 的 PCM、MP3、raw Opus、非法 MIME、codec failure、frame boundary 和 cleanup。
- 公共 Stream contract suite：覆盖 BOS/data/EOS、StreamID、role、label、terminal error、provider EOF/error、restart、backpressure 和 cancel。
- Realtime Dialogue suite：覆盖 Push-to-Talk 全部合法/非法状态转换、每 turn 单次 EndASR、Realtime VAD、text mode 和 Interrupt。
- Realtime Duplex suite：覆盖 continuous audio、transcription、response text/audio、function calls 和 CancelResponse；不能套用 Push-to-Talk contract。
- Lifecycle/race suite：用可阻塞 fake provider 验证 Open/Send/Recv/End/Interrupt/Cancel/Close 在 context cancel 后退出，并覆盖 Close 与 Recv 并发。
- 真实 provider integration suite：在有凭据的受控 CI 或手动环境验证 SDK session cancel、event ordering、audio format 和 close behavior。

所有 bug fix 必须先提交能复现问题的失败测试，再提交最小修复。公共逻辑 bug 必须通过共享 contract case 同时约束 Realtime 与 Duplex，不能复制两个近似测试。必需门禁为：

```sh
go test ./pkgs/genx/transformers -count=1
go test -race ./pkgs/genx/transformers -count=1
go test ./pkgs/genx/... -count=1
```

race test 未通过时不得认为 Doubao refactor 完成；真实 provider contract 变化也不得仅凭 fake session tests 合并。

## 验证

```sh
go test ./pkgs/gizedge ./pkgs/gizclaw/... -count=1
go test ./pkgs/audio/... ./pkgs/store/... -count=1
go test -race ./pkgs/audio/pcm ./pkgs/store/metrics ./pkgs/store/kv ./pkgs/store/objectstore ./pkgs/store/vecstore -count=1
go vet ./pkgs/audio/... ./pkgs/store/...
npm ci --prefix guides
npm --prefix guides run build
git diff --check
```

记录这些问题时，上述命令均已通过。
