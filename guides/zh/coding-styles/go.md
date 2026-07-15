# Go

Go 代码应保持 package 边界明确、控制流简单、生命周期可推导。格式和命名遵循 `gofmt`、Effective Go 与 Go 官方惯例。

## Package 与公开 API

- package 名使用简短的小写单词，避免重复上层目录已经表达的含义。
- 使用 MixedCaps 命名，不使用下划线模拟其他语言的命名习惯。
- 只导出外部调用方需要的符号，并为新增或修改的公开 package、type、function 和 method 写清楚 Go doc。
- interface 放在消费方边界，用调用方真正需要的方法描述能力；不要提前建立只包一层实现的抽象。
- constructor 不应隐藏启动 goroutine、发起网络请求或修改全局状态等难以预期的副作用。
- 谨慎使用 embedding，避免意外扩大公开 API 或产生含义不清的 promoted member。
- 跨 package API 直接使用定义方的原始类型和带 package 限定的名称。不要通过 `type Local = otherpkg.Type` 重新导出外部、生成或其他 ownership package 的类型；alias 会隐藏类型来源，使调用方和 reviewer 无法从签名判断真正 owner。

## 函数与数据

- 一个函数处理一个连贯职责；优先使用 early return，避免不必要的嵌套。
- error 必须传播或处理。补充 context 时说明失败的操作和对象，不重复堆叠无意义的 `failed to`。
- `panic` 只用于无法合理恢复的 programmer error；`recover` 只能放在明确的隔离边界。
- 明确 slice、map 和 pointer 的所有权及可变性，处理 nil、空值、capacity、append 和 copy 的差别。
- receiver 的 pointer/value 选择应与可变性、复制成本和 method set 一致，同一类型保持统一。
- `init` 只用于必要且可预测的注册，不能隐藏业务启动流程。

## 生成类型与 Protobuf

- `protoc` 生成的 message 属于生成它的 package。RPC、SDK、adapter 或 service 需要该 wire type 时，直接使用 `*rpcproto.Message` 这类原始限定类型，不在 `rpcapi`、SDK 或业务 package 中为它建立 alias、同形 wrapper 或仅用于改名的 DTO。
- Protobuf message 包含运行时状态，不是可安全复制的普通 value object。跨函数传递、存储、marshal、reflection 和格式化时使用指针，不按值接收、返回、赋值或放入会复制值的 interface。
- 业务层只有在拥有独立于 wire message 的领域语义、生命周期或兼容边界时，才定义自己的类型。该类型不能只是为了隐藏生成 package、维持一个值参数签名或绕过 `copylocks` 而逐字段复制 protobuf message。
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
- 涉及生成类型、Protobuf 或跨 package API 时运行 `go vet`，不得通过 alias、wrapper 或 suppression 隐藏 `copylocks` 和 ownership 问题。
- 修改 Go 行为后默认运行：

```sh
gofmt -w <changed-files>
go test ./...
```
