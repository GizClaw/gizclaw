# 审查项目

这份文档定义 Issue 审查、开发后自我审查和 PR Agent 审查共用的审查项目。每次审查都必须根据改动路径找到适用文档并逐项检查；某项确实不适用时，应说明对象和原因，不能把整组内容概括为“已检查”。

## 查找适用文档

审查开始时按以下顺序建立规则集合：

1. 阅读仓库根目录 `AGENTS.md`，确认仓库级强制规则。
2. 列出 Issue 计划修改的路径，或实现中的全部 changed directory。
3. 根据下方“路径与开发指引”表，读取每个 ownership root 对应的开发文档。
4. 根据文件类型读取对应[编码规范](../coding-styles/)。
5. 改动 API、RPC、生成代码或 SDK 时，同时读取 source contract、provider、adapter 和全部 committed consumer 的文档与实现。

一个审查对象涉及多个 ownership root 时，必须合并所有适用文档。Issue、文档、Schema 和生产代码互相冲突时，应把冲突作为问题明确指出，不能自行选择更方便的一边作为审查依据。

### 路径与开发指引

| 修改路径或 ownership | 必须查阅的开发文档 | 重点确认 |
| --- | --- | --- |
| `api/http/**` | [HTTP API 总览](/zh/developing/api/http/overview)、对应的 [Admin API](/zh/developing/api/http/admin)、[Public API](/zh/developing/api/http/public) 或 [OpenAI Compatible](/zh/developing/api/http/openai-compatible) | Surface ownership、shared/resource 依赖、生成结果与 server 实现 |
| `api/proto/rpc/**` | [Proto API 总览](/zh/developing/api/proto/overview)、[Peer RPC](/zh/developing/api/proto/rpc/overview) 及对应 provider-direction 页面 | Method provider/consumer、message source of truth、生成 surface |
| `api/proto/telemetry/**` | [Telemetry](/zh/developing/api/proto/telemetry) | Telemetry contract、编码、生成代码与消费端 |
| `pkgs/giznet/**` | [pkgs/giznet](/zh/developing/giznet) | WebRTC、PeerConn、transport 与 connection lifecycle |
| `pkgs/gizedge/**` | [pkgs/gizedge](/zh/developing/gizedge) | Device/Edge/Server signaling、反向代理与 RPC routing |
| `pkgs/gizclaw/**` | [pkgs/gizclaw 总览](/zh/developing/gizclaw/overview) | Service layout、角色可见性、根 package 组装边界 |
| `pkgs/gizclaw/peer_*.go` | [Peer 总览](/zh/developing/gizclaw/peer/overview) 及对应的 Management、Authorization、Connection 或 Services 页面 | Peer identity、在线连接、授权和 service surface |
| `pkgs/gizclaw/server*.go` | [Server 总览](/zh/developing/gizclaw/server/overview) 及对应模块页面 | Server 组装、HTTP surface、security policy 与 lifecycle |
| `pkgs/gizclaw/rpc*.go` | [RPC 总览](/zh/developing/gizclaw/rpc/overview) 及对应 Client、Server 或能力页面 | Go RPC implementation 与 `api/proto/rpc` contract 一致性 |
| `pkgs/gizclaw/services/**` | [Services 总览](/zh/developing/gizclaw/services/overview) 及对应领域页面 | 领域 ownership、持久化边界和跨 service 依赖 |
| `pkgs/gizclaw/api/**` | [Generated Go API](/zh/developing/gizclaw/api) 和对应 `api/**` source contract | 生成文件新鲜度；不得手工维护生成 surface |
| `pkgs/gizclaw/contextstore/**` | [Context Store](/zh/developing/gizclaw/contextstore) | Config context、类型安全和调用边界 |
| `pkgs/gizclaw/customid/**` | [Custom ID](/zh/developing/gizclaw/customid) | ID 编码、解析与兼容性 |
| `pkgs/genx/**` | [GenX 总览](/zh/developing/genx/overview) 及对应 Generators、Transformers、Segmentors、Profilers、Labelers 或 Model Loader 页面 | Stream/EOS、interface、Mux、provider adapter 与公共 pipeline 边界 |
| `pkgs/audio/**` | [Audio 总览](/zh/developing/audio/overview) 及对应 codec、PCM、resampler 或 voiceprint 页面 | Frame/codec contract、sample format、buffer 与实时处理 |
| `pkgs/store/**` | [Store 总览](/zh/developing/stores/overview) 及对应 store 页面 | Interface contract、具体 backend 约束、持久化与并发语义 |
| `sdk/js/**` | 对应 [HTTP API](/zh/developing/api/http/overview) 或 [Proto API](/zh/developing/api/proto/overview)，以及 [JavaScript 与 TypeScript](/zh/coding-styles/js) | SDK surface、生成 client、runtime 差异和错误处理 |
| `sdk/flutter/**` | 对应 API/Proto 文档，以及 [Dart 与 Flutter](/zh/coding-styles/dart-flutter) | Dart SDK contract、WebRTC transport、Stream 与生成 message |
| `sdk/c/**` | 对应 [Peer RPC](/zh/developing/api/proto/rpc/overview)，以及 [C 与 cgo](/zh/coding-styles/c) | C API/ABI、nanopb 生成代码、ownership 与 cgo bridge |
| `apps/gizclaw-app/**` | [Dart 与 Flutter](/zh/coding-styles/dart-flutter) 和 App 使用的 SDK/API 文档 | App 与 SDK 边界、Widget lifecycle、平台行为 |
| `apps/wails/**` | [JavaScript 与 TypeScript](/zh/coding-styles/js)、[Go](/zh/coding-styles/go) 和 App 使用的 API 文档 | Go bridge、frontend runtime、生成 client 与 desktop lifecycle |
| `guides/**`、`README.md`、`AGENTS.md` | [文档编码规范](/zh/coding-styles/docs) | 最终形态、事实来源、链接、导航、命令和重复 source of truth |
| `.github/**` | [文档编码规范](/zh/coding-styles/docs) 和仓库根 `AGENTS.md` | Workflow 权限、SHA pin、secret boundary 与实际执行命令 |
| `tests/**`、`examples/**` | 被测试或演示模块的全部开发文档及对应编码规范 | 测试是否证明 production contract，而非建立第二套行为 |

表中的路径按 ownership 匹配，不要求文件系统必须存在同名子目录。例如 `peer_*.go`、`server*.go` 和 `rpc*.go` 当前位于同一 Go package，但审查时仍应按其实际模块职责读取对应页面。

### 文件类型与编码规范

| 修改文件 | 必须查阅 |
| --- | --- |
| `*.go`、`go.mod`、`go.sum` | [Go](/zh/coding-styles/go) |
| `*.js`、`*.jsx`、`*.ts`、`*.tsx`、JavaScript package/lockfile | [JavaScript 与 TypeScript](/zh/coding-styles/js) |
| `*.dart`、`pubspec.yaml`、Flutter platform wiring | [Dart 与 Flutter](/zh/coding-styles/dart-flutter) |
| `*.c`、`*.h`、nanopb output、cgo bridge | [C 与 cgo](/zh/coding-styles/c)；cgo 同时读取 [Go](/zh/coding-styles/go) |
| `*.md`、VitePress、README、配置示例、workflow 说明 | [文档](/zh/coding-styles/docs) |
| OpenAPI、Protobuf、生成配置 | [编码规范总览中的 Contract 规则](/zh/coding-styles/) 以及全部生成 consumer 的语言规范 |

### 跨 Contract 的附加文档

| 修改内容 | 还必须检查 |
| --- | --- |
| HTTP Schema 或 route | 对应 HTTP API 页面、Go server/peer 实现、生成 Go API、JavaScript/Dart consumer |
| Peer RPC method/message | 对应 provider-direction 页面、`pkgs/gizclaw/rpc`、Go/Dart/C 生成代码与调用方 |
| Telemetry Schema | Telemetry 文档、生成 message、发送方与全部接收方 |
| WebRTC signaling/transport | Giznet、Gizedge、Public API、SDK transport 与连接生命周期测试 |
| GenX Stream/EOS | GenX 总览、具体 Adapter、公共 stream processing 和所有 stream consumer |

## 需求与范围

- 改动是否解决 Issue、设计文档或明确请求中的实际问题。
- 是否遗漏 acceptance criteria、错误路径、兼容要求或非功能约束。
- 是否混入与当前目标无关的重构、依赖、生成文件或临时产物。
- 文档是否描述最终行为，而不是把计划、TODO 或迁移过程写成已完成能力。

## 模块与依赖

- 新代码是否位于拥有该行为的 package 或目录。
- public API 是否保持最小，是否出现只为方便调用而泄漏的内部类型。
- 通用 abstraction 是否意外依赖具体 provider、产品资源或 UI。
- constructor、注册流程和 `init` 是否隐藏网络、goroutine 或全局状态副作用。
- dependency 是否必要，lockfile、submodule 和许可证相关文件是否一致。

## Contract 与生成代码

- OpenAPI、Protobuf 或其他 Schema 是否仍是唯一 source of truth。
- 生成代码是否由当前 Schema 重新生成，而不是手工修改。
- Go、JavaScript、Dart、C SDK 的 method、message、field、enum、error 和 stream shape 是否一致。
- server、client、edge-node 与 device 的 provider/consumer 方向是否和 contract 相符。
- 兼容性变化是否被明确要求并覆盖所有调用方。

## 逻辑与状态

- 正常路径、空值、边界值、重复调用、乱序和失败状态是否正确。
- error 是否传播，是否包含足以定位操作和对象的 context。
- 状态机是否存在不可能退出、跳过校验或重复完成的状态。
- retry、timeout、reconnect 和 fallback 是否有上限且不会掩盖最终失败。
- placeholder、fake output、dead registration 和 TODO-only behavior 不得作为完成实现。

## 生命周期与并发

- goroutine、Future、stream、subscription、timer、connection、native handle 和 buffer 是否有明确 owner。
- success、failure、cancel 和 partial initialization 是否都能清理资源。
- channel closing、callback、lock 和共享状态是否存在 race、deadlock、阻塞或 leak。
- UI 销毁、peer 断开和服务停止后是否仍可能收到 callback 或更新状态。

## 输入、安全与平台边界

- HTTP、RPC、事件流、配置、firmware、workflow input 和 SDK payload 是否按不可信输入处理。
- length、pointer、cast、JSON、enum、path 和 credential 是否在所属边界校验。
- 是否提交 secret、token、日志、缓存、二进制、机器路径或临时文件。
- GitHub Actions dependency 是否继续固定到 commit SHA。

## 语言专项

### Go

- package/API、error、slice/map、receiver、embedding 和 `defer` 是否符合 [Go 编码规范](../coding-styles/go)。
- 手写的跨 package API 是否直接使用定义方的限定类型，是否存在通过 alias、同形 wrapper 或仅改名 DTO 隐藏类型真实 ownership 的情况。
- 仓库自有 generator 是否产生跨 package alias；若有，是否从 generator 修复。第三方 generator 直接产生的 alias 或 helper signature 不按手写代码问题报告，也不得仅为满足规范而手工改写、维护 fork 或增加 output normalizer。文件名和生成注释不能替代生成链证据。
- goroutine、context、channel、timer 和连接生命周期是否闭合。
- 并发风险是否需要 `go test -race`、leak test 或 soak test。
- 是否运行 `modernize ./...` 并审查本次改动涉及的手写代码诊断；范围外既有诊断是否如实记录，仓库自有 generator 输出是否回到 generator 修复，第三方生成文件是否保持未手工修改。

### JavaScript 与 TypeScript

- Promise 是否被处理，AbortSignal、subscription 和 effect 是否清理。
- 外部 payload 是否绕过类型边界，browser/Wails/Node runtime 假设是否成立。
- UI 是否覆盖 loading、empty、error、stale 和 permission denied。

### Dart 与 Flutter

- Future、StreamSubscription、controller 和 peer connection 是否正确关闭。
- `await` 后的 `BuildContext`/`setState` 是否检查 `mounted`。
- SDK 与 App ownership 是否分离，Widget 是否复制 transport 或 contract 实现。

### C 与 cgo

- public header、ABI、struct layout、enum 和 callback signature 是否兼容。
- pointer、buffer、length、allocator 和 ownership 是否明确安全。
- C 是否错误保存 Go pointer，`cgo.Handle` 是否在全部路径释放。

### 文档

- endpoint、method、package、目录、配置和命令是否由当前代码或 Schema 支持。
- 图表、链接、目录树与完成状态是否准确。
- 稳定文档是否混入临时 Issue、未实现方案或机器环境。

## 测试与验证

- 测试是否覆盖改动真正的行为、失败路径和回归风险，而非只验证内部实现。
- 跨 package、网络、存储、Schema 或 runtime 的行为是否需要 integration/E2E。
- 生成代码是否运行 regeneration check。
- validation command 是否属于实际改动 package，结果是否真实通过。
- 跳过的验证是否说明原因和剩余风险。

## Git 内容

- diff 中没有无关修改、merge residue、broken symlink 或意外权限变化。
- LFS、submodule、lockfile 和生成文件变化均有明确来源。
- `git diff --check` 通过，提交边界与模块 ownership 一致。
