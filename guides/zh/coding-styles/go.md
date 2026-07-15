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

## 测试与验证

- 纯逻辑、边界值、错误路径和回归场景使用最小有效 package 的 unit test。
- table-driven test 和 subtest 只在能让输入、期望和失败信息更清楚时使用。
- HTTP、RPC、数据库、文件系统、serialization、timeout 和 retry 行为使用 integration test。
- 并发改动应根据风险运行 `go test -race`；性能敏感路径根据需要增加 benchmark。
- 涉及生成类型、Protobuf 或跨 package API 时运行 `go vet`。仓库自有 generator 输出中的诊断应回到 generator 处理；第三方生成文件中的诊断应记录来源，不为消除诊断而手工修改输出。不得通过手写 alias、wrapper 或 suppression 隐藏 ownership 问题。
- 修改 Go 代码时必须运行 `modernize ./...`，检查当前 Go 版本提供的现代化建议。review 至少处理本次改动涉及的手写代码诊断；已有的范围外诊断应在验证结果中说明，不要求混入当前改动。仓库自有 generator 输出中的诊断回到 generator 处理；第三方生成文件中的诊断不应手工修复。
- `modernize -fix` 会直接修改文件，只能在确认建议适用后使用；执行后必须审查完整 diff，并运行对应测试，不能把 analyzer 建议等同于行为正确性证明。
- 修改 Go 行为后默认运行：

```sh
gofmt -w <changed-files>
modernize ./...
go test ./...
```
