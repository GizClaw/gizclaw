# API 生成与变更

API 变更必须从根 `api/` 的 source schema 开始。禁止直接修改由第三方工具生成的 `*.pb.go`、OpenAPI Go output、JavaScript generated client 或 C nanopb output。`rpcapi/generated.go` 是历史遗留的手工维护 wrapper，不属于第三方生成输出；修改时必须同时核对 source proto、`rpcproto` 和 codec tests。

## 生成链路

| Source | 主要输出 | 命令 |
| --- | --- | --- |
| HTTP OpenAPI + shared schemas | Go HTTP server/client/models | `go generate ./pkgs/gizclaw/api/adminhttp ./pkgs/gizclaw/api/apitypes ./pkgs/gizclaw/api/peerhttp` |
| `api/proto/rpc/**/*.proto` | Go Protobuf | `go generate ./pkgs/gizclaw/api/rpcproto` |
| RPC descriptors/wrappers | 手工维护的 `rpcapi` committed surface | `go test ./pkgs/gizclaw/api/rpcapi`（当前 `go generate` 也只执行该验证，不会重新生成文件） |
| HTTP + RPC schemas | JavaScript SDK | `npm --prefix sdk/js run gen:sdk` |
| RPC Protobuf | C nanopb SDK | `go generate ./sdk/c/gizclaw` |
| Telemetry Protobuf | Go/JavaScript telemetry | `go generate ./pkgs/gizclaw/api/telemetry` 与 `npm --prefix sdk/js run gen:telemetry` |

独立 C SDK Release 源码包会在完成该生成检查后复制已提交的 nanopb output。打包过程不会运行第二套 generator，也不会把源码包作为另一份协议来源；源码包同时复制所选 GizClaw commit 的 submodule gitlink 指向的精确 nanopb runtime。

全量 Go API 可以使用：

```sh
go generate ./pkgs/gizclaw/api/...
```

`api` Go package 会嵌入仓库拥有的完整 `api/http` 与 `api/proto` source tree。离线 Resource 校验等运行时 contract consumer 直接从 embedded filesystem 读取这些原始定义，不再提交另一份 resolved schema 副本。`pkgs/gizclaw/api/apitypes/types_resolved.json` 继续作为仅用于生成 `apitypes/generated.go` 的 ignored intermediate。使用 `go generate ./pkgs/gizclaw/api/apitypes` 刷新 Go 生成结果；生成后执行 `git diff --exit-code -- pkgs/gizclaw/api/apitypes/generated.go` 可以确认结果保持新鲜。

仓库自有 OpenAPI generator 使用 root Go module 声明的 `oapi-codegen` tool。tool 版本必须与 module 的 `kin-openapi` 版本兼容；升级任一依赖时，必须重新生成并审查所有受影响的已提交 Go output。

## 一次完整变更

```mermaid
flowchart LR
    Edit["修改 api source"] --> Generate["重新生成所有受影响语言"]
    Generate --> Diff["审查 schema 与 generated diff"]
    Diff --> Implement["更新 adapters/services/callers"]
    Implement --> Test["跨语言与 e2e 验证"]
```

审查不能只看生成文件。应先确认 source contract 是否正确，再确认每个生成 surface 新鲜一致，最后验证调用点和业务实现。

## 最低验证

按变更范围选择，但至少包括：

```sh
go test ./pkgs/gizclaw/api/... ./pkgs/gizclaw/... ./sdk/go/... -count=1
npm --prefix sdk/js test
git diff --check
```

RPC/C surface 变化时增加 C generation/build tests；管理资源变化时增加 resource manager 与 CLI e2e；HTTP endpoint 变化时覆盖 strict adapter 的成功和用户可见错误路径。

生成后如果出现大量无关 diff，应先检查工具版本、排序和 template，而不是提交噪声。Generated output 必须与同一提交中的 source schema 一致。

## Generator ownership

- `rpcproto/*.pb.go` 由第三方 `protoc-gen-go` 直接生成。
- `adminhttp`、`apitypes` 和 `peerhttp` 的 Go output 由第三方 `oapi-codegen` 生成；仓库内工具可以准备输入，但不因此拥有生成模板。OpenAI-compatible protocol 由 AI Server Shell 提供，不属于本仓库生成链。
- 第三方生成器产生的 alias、helper signature 或 import qualifier 保持生成结果，不为本地风格规则手改、fork generator 或追加 AST normalizer。
- 仓库手写代码和仓库自有 generator 直接使用类型所属 package，不新增仅用于重命名或 re-export 的跨 package alias。

Monitor OpenAPI（`api/http/monitor.json`）通过 `go generate ./pkgs/monitor/api` 生成 `pkgs/monitor/api/generated.go` 中的 strict Go server/client/models，配置位于 `pkgs/monitor/api/codegen_config.yaml`。`npm --prefix sdk/js run gen:sdk` 依据 `sdk/js/openapi-ts.config.ts` 生成 `web/monitor/src/generated/monitor/` 中的控制台 client。这些已提交输出归 Monitor surface 所有，由 `pkgs/monitor/monitor.go` 和 `web/monitor/src/api.ts` 使用，不通过 Public/Admin SDK package 导出。
