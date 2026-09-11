# Go SDK <Badge type="warning" text="WIP" />

> 本页目前只说明 SDK 的定位和范围，公开 client、连接、RPC、stream 与 telemetry 模块仍待逐项展开。

`sdk/go/gizcli` 提供 Go 调用方使用的 GizClaw client surface，包括连接、安全策略、资源、Peer stream、RPC、Telemetry 与 WebRTC 接入。

Go SDK 是 client-facing boundary，不拥有 server domain behavior。API 和 RPC method 来自 [API Design](../api/overview)；修改 contract 时必须同步生成 surface、SDK 实现和测试。

调用方删除当前 Peer 时使用 `Client.DeletePeer`。成功 response 会终止当前 Peer connection，调用方必须重新连接后才能继续发起工作。

## Context 连接与 Admin Resource

两个子 package 为 CLI 与 Terraform provider 提供共享的纯 Go 能力，在 `CGO_ENABLED=0` 下可为 darwin/linux 的 amd64/arm64 构建，不依赖 `cmd/...`、嵌入的 Console 或 native model runtime。

- `sdk/go/gizcli/contextconn`：`ConfigDir` 返回 CLI context 根目录；`LoadContext` 读取 `<ConfigDir>/<context>/config.yaml`（名称为空时读取 current context），并用 `Options.Endpoint` 替换 `server.endpoint`，覆盖值必须是 `https://host[:port]`；`Dial` 获取 server-info 并准备 WebRTC client；`Connect` 连接、启动 client-side Peer service 并等待 Peer HTTP 可用；传入的 `context.Context` 可取消 server-info 请求、WebRTC dial 与就绪等待。调用方拥有返回的 client 并负责 `Close`。
- `sdk/go/gizcli/adminresource`：`Client` 封装 Admin HTTP 的 `ApplyResource`、`GetResource` 与 `DeleteResource`；非成功 response 返回 `*ResponseError`，文本保持 `CODE: message`，`IsNotFound` 只识别 `<SCOPE>_NOT_FOUND` 错误码。`GetResources` 在一条连接上最多并发 8 个读取，并按输入顺序返回每个引用的结果。`PrepareManifest`、`DecodeManifest` 与 `FormatForPath` 实现 `gizclaw admin apply|validate` 的 JSON/YAML、`${VAR}` 展开与 `<Kind>Resource` 别名规则。

[contextconn API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn) · [adminresource API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)
