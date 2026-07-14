# 编码规范

GizClaw 是 Go-first、但并非 Go-only 的仓库。代码、Schema、生成 SDK、C binding、Wails 前端和文档共同组成一个产品 contract；编码时必须同时维护其实际跨越的边界。

## 选择对应规范

| 改动范围 | 编码规范 |
| --- | --- |
| Go package、服务、并发与生命周期 | [Go](./go) |
| JavaScript、TypeScript、SDK 与前端 | [JavaScript 与 TypeScript](./js) |
| Dart SDK 与 Flutter App | [Dart 与 Flutter](./dart-flutter) |
| C SDK、C binding 与 cgo bridge | [C 与 cgo](./c) |
| Guide、README、配置说明与架构文档 | [文档](./docs) |

## 通用规则

### 先确定所有权

新代码必须放进拥有该行为的 package 或目录。不要因为调用方便，把 provider-specific、产品资源、传输细节或持久化逻辑扩散到通用 abstraction。

公开 API 应保持最小。只在 package 外部调用方确实需要时导出类型或函数；独立 package 的公开符号由 Go doc 或生成 Reference 说明，开发指引负责解释模块职责和边界。

### Contract 只有一个源头

OpenAPI、Protobuf 和其他 Schema 是生成 surface 的 source of truth。修改 contract 时，应修改源 Schema、重新生成提交到仓库的产物，并同步验证 Go、JavaScript、C 和实际调用方。不得直接把生成文件当作源代码维护。

### 外部输入不可信

HTTP、RPC、事件流、配置、固件、SDK payload、workflow input 和跨语言 buffer 都必须在所属边界校验。解析失败、取消、超时、部分初始化和连接关闭应有明确行为。

### 生命周期必须闭合

创建 goroutine、stream、subscription、timer、文件、网络连接、native handle 或 buffer 的代码，也必须定义取消、关闭和失败清理路径。资源的创建者不一定是关闭者，但关闭所有权必须唯一且清楚。

### 测试跟随风险

测试验证可观察行为，而不是机械追求每个文件一个测试。纯逻辑优先使用 unit test；跨 package、Schema、网络、存储和运行时边界使用 integration 或 E2E；并发与长期运行组件需要覆盖取消、泄漏和竞态风险。

## 提交前最低检查

- 格式化所有修改过的源文件。
- 运行改动所属 package 已定义的 build、test 或生成命令。
- Go 行为改动默认运行 `go test ./...`；只有改动确实局部时才使用更窄范围，并说明原因。
- Contract 改动重新生成并验证所有受影响语言 surface。
- 文档与配置改动至少运行 `git diff --check`，并验证新增链接和命令。
- 不提交 secret、credential、日志、缓存、临时文件、构建产物或无关改动。
