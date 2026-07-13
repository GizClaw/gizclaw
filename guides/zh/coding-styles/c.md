# C 与 cgo

本规范适用于 `sdk/c/gizclaw`、生成的 C RPC 代码、C-facing platform interface，以及连接 Go 与 C 的 cgo bridge。

## API 与 ABI

- 未明确要求 breaking change 时，保持 public header 的 ABI 兼容。
- struct layout、enum value、typedef、callback signature 和 exported function name 都属于 contract。
- header 与 source 必须同步，包括 declaration、include、ownership、错误返回和 nullability。
- 生成的 RPC method、message 与 codec 必须来自 `api/rpc/**/*.proto` 及生成配置，不能手工修补生成结果。
- platform vtable 应明确 required callback、userdata 传递和 fallback 行为。

## Memory ownership

- 每个 pointer、buffer 和 callback 参数都必须明确是 borrowed、owned 还是 transferred。
- allocation 必须与同一 allocator family 的 free 配对；部分初始化失败也要释放已经取得的资源。
- public API 与 callback 边界先检查 null，再进行 dereference。
- pointer arithmetic、allocation、copy、encode 和 decode 前校验 length，并检查 signed/unsigned、`size_t`、Go length 与 wire width 的转换。
- buffer 只有在 contract 明确保证 lifetime 时才能跨 callback 保存；不得保存 stack memory、临时 Go memory 或已经归还给调用方的 buffer。
- reset/free 在调用方可能重复清理时应设计为 idempotent。

## cgo bridge

- C 不得长期保存普通 Go pointer；需要跨调用保存 Go 对象时使用 `cgo.Handle` 或其他合法 owner。
- 每个 `cgo.Handle` 在成功、失败和取消路径都必须删除。
- C buffer 与 Go slice 的转换要处理 nil、zero length、maximum length 和有效期。
- backend、sink、peer 或 channel 关闭后，不能再回调 Go。
- C callback ID、channel label 和 Go 侧语义必须保持同步。

## 测试与验证

- encode/decode、frame、buffer、key、JSON 和 signaling 的纯逻辑使用 unit test。
- public C API、初始化和 platform vtable 变化需要 compile 或 smoke test。
- 生成代码变化需要 regeneration check；cgo bridge 可使用 Go test 作为可靠验证边界。
- 覆盖 malformed input、boundary length、allocation failure、null pointer 和 partial cleanup。
- 不得仅凭无关的 Go test 通过，就宣称 C surface 已验证。
