# DashScope Adapter

DashScope Adapter 通过 `DashScopeRealtime` 将 DashScope realtime multimodal session 适配为 `genx.Transformer`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`DashScopeRealtime`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#DashScopeRealtime) | 保存 realtime model、audio format、voice、instructions 和 turn detection 配置。 |
| [`NewDashScopeRealtime`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#NewDashScopeRealtime) | 使用 DashScope client 创建 Transformer。 |
| `DashScopeRealtime.Transform` | 建立 realtime session，将输入 Stream 写入 provider，并返回统一输出 Stream。 |
| [`DashScopeStream`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#DashScopeStream) | 包装支持 session update 的 realtime output Stream。 |

Provider session update 和 event name 留在 Adapter 内部；调用方只依赖 GenX Stream 与显式 update contract。
