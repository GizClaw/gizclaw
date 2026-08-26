# Go

Go 代码应保持 package 边界明确、控制流简单、生命周期可推导。格式和命名遵循 `gofmt`、Effective Go 与 Go 官方惯例。

## Package 与公开 API

- package 名使用简短的小写单词，避免重复上层目录已经表达的含义。
- 使用 MixedCaps 命名，不使用下划线模拟其他语言的命名习惯。
- 只导出外部调用方需要的符号，并为新增或修改的公开 package、type、function 和 method 写清楚 Go doc。
- interface 放在消费方边界，用调用方真正需要的方法描述能力；不要提前建立只包一层实现的抽象。
- constructor 不应隐藏启动 goroutine、发起网络请求或修改全局状态等难以预期的副作用。
- 谨慎使用 embedding，避免意外扩大公开 API 或产生含义不清的 promoted member。
- 手写的跨 package API 直接使用定义方的原始类型和带 package 限定的名称。不要通过 `type Local = otherpkg.Type` 重新导出其他 ownership package 的类型；alias 会隐藏类型来源，使调用方和 reviewer 无法从签名判断真正 owner。仓库自有 generator 生成的 Go API 也必须遵守这条规则，应从 generator 修复违规输出。第三方 generator（例如 `protoc-gen-go`、`oapi-codegen`）直接产生的 alias 可以保留，不得仅为满足本规则而手工修改生成文件、维护 generator fork 或增加 output normalizer。文件名、生成注释或所在目录本身不能证明某段代码是第三方生成输出。

## 函数与数据

- 一个函数处理一个连贯职责；优先使用 early return，避免不必要的嵌套。
- error 必须传播或处理。补充 context 时说明失败的操作和对象，不重复堆叠无意义的 `failed to`。
- `panic` 只用于无法合理恢复的 programmer error；`recover` 只能放在明确的隔离边界。
- 明确 slice、map 和 pointer 的所有权及可变性，处理 nil、空值、capacity、append 和 copy 的差别。
- receiver 的 pointer/value 选择应与可变性、复制成本和 method set 一致，同一类型保持统一。
- `init` 只用于必要且可预测的注册，不能隐藏业务启动流程。

## 生成类型与 Protobuf

- `protoc` 生成的 message 属于生成它的 package。手写的 RPC、SDK、adapter 或 service 需要该 wire type 时，直接使用 `*rpcpb.Message` 这类原始限定类型，不为它建立 alias、同形 wrapper 或仅用于改名的 DTO。
- `protoc-gen-go` 等第三方 generator 自动产生的 alias 和 helper signature 属于第三方生成 surface，可以保留；不要手工修改生成文件。若仓库自有 generator 产生 alias，应修改 generator；若需要改变第三方生成结果，应优先调整 Schema 或官方支持的生成配置，并重新生成全部 committed output。
- 业务层只有在拥有独立于 wire message 的领域语义、生命周期或兼容边界时，才定义自己的类型。该类型不能只是为了隐藏生成 package 而逐字段复制 protobuf message。
- `.proto` 仍是 wire contract 的唯一 source of truth。生成器负责 protobuf output 和必要 codec，不应生成第二套同形公共类型来模糊 ownership。

## 并发与资源

- 每个 goroutine 都应能回答：由谁启动、何时退出、如何取消、错误交给谁。
- channel 由发送方或明确的生命周期 owner 关闭；接收方不要为了结束消费而随意关闭 channel。
- context 必须沿调用链传播，不能用新的 background context 丢失已有取消和 deadline。
- timer、ticker、stream、connection 和 worker 在成功、失败与取消路径都必须释放。
- 涉及共享状态、callback 或长期 worker 时，检查锁粒度、阻塞路径、race 和 goroutine leak。

锁 ownership 必须匹配最小持久化 invariant。独立资源 ID 使用会清理空闲 entry 的 keyed coordination；跨资源操作按 canonical 排序获取 key。不得在内部状态 mutex 下执行调用者 callback 或阻塞式 native/network I/O。生命周期代码必须先取消阻塞 I/O、等待使用者退出，再释放 handle。任何锁边界修改都需要确定性的并发进度测试、同资源串行 control 和 `go test -race`。

## 测试与验证

- 纯逻辑、边界值、错误路径和回归场景使用最小有效 package 的 unit test。
- table-driven test 和 subtest 只在能让输入、期望和失败信息更清楚时使用。
- HTTP、RPC、数据库、文件系统、serialization、timeout 和 retry 行为使用 integration test。
- 并发改动应根据风险运行 `go test -race`；性能敏感路径根据需要增加 benchmark。
- 修改手写 Go 临界区后运行 `go run ./tools/quality mutexscope`。fingerprint 变化必须经过语义 review 并精确更新 reviewed file；静态 inventory 不能证明不存在 race 或 deadlock。
- 涉及生成类型、Protobuf 或跨 package API 时运行 `go vet`。仓库自有 generator 输出中的诊断应回到 generator 处理；第三方生成文件中的诊断应记录来源，不为消除诊断而手工修改输出。不得通过手写 alias、wrapper 或 suppression 隐藏 ownership 问题。
- 修改 Go 代码时必须通过 `go run ./tools/quality modernize -binary "$(go env GOPATH)/bin/modernize"` 运行仓库固定版本的 `modernize`。所有被维护 module 中的手写 Go 诊断都必须修复，不能按“既有债务”保留，也不能通过编辑 `tools/quality/modernize.exemptions` 豁免。
- `tools/quality/modernize.exemptions` 是生成代码和第三方代码的精确诊断豁免列表，不是 baseline、历史快照、路径通配或 package suppression。每条记录必须是当前 analyzer 实际输出的完整诊断，并且 source 必须是仓库内的 Git 跟踪普通文件（不能是 symlink），且满足以下一种 provenance：Go package 之前的 comment preamble 含标准 `// Code generated ... DO NOT EDIT.` 标记、路径命中显式 generator-owned output 规则，或路径命中显式 third-party ownership root（例如 `third_party/`）。文件名、字符串/template 内的标记、测试 fixture、任意 `generated`/`vendor` 目录名和豁免列表中已有记录都不能证明 provenance。
- 只有在所有维护 module 均完成分析、没有手写诊断且没有 tool/package-loading failure 后，才可使用 `--write-exemptions` 原子刷新豁免列表。新增、删除、变更或 stale 的生成/第三方诊断都会使普通检查失败；review 必须逐条核对 source ownership。显式 generated/third-party root 内的未跟踪依赖或本地生成输出（例如 `guides/node_modules/`）不属于维护 source，也不能进入已提交豁免列表；其他未跟踪或未知的仓库内 Go source 一律按手写代码失败。仓库自有 generator 能安全修复的诊断仍应从 generator 修复并重新生成；第三方生成输出不得手工修改。`go vet` 使用独立的 `tools/quality/vet.baseline` contract，不受此豁免列表影响。
- `modernize -fix` 会直接修改文件，只能在确认建议适用后使用；执行后必须审查完整 diff，并运行对应测试，不能把 analyzer 建议等同于行为正确性证明。
- 修改 Go 行为后默认运行：

```sh
gofmt -w <changed-files>
modernize ./...
go test ./...
```
