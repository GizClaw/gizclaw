# TypeScript SDK <Badge type="warning" text="WIP" />

> 本页目前只说明 SDK 的目录和 contract 边界，公开 surface、生成流程和 runtime 行为仍待逐项展开。

`sdk/js/gizclaw` 提供 TypeScript client surface，覆盖 Admin HTTP、Public HTTP、RPC、signaling 和 Telemetry。`sdk/js/scripts` 保存由 OpenAPI、Protobuf 与 method registry 生成 SDK surface 所需的工具。

```text
sdk/js/
├── gizclaw/     # SDK package 与 generated client
└── scripts/     # Contract generation 与生成结果修整
```

生成内容的 source of truth 位于 [API Design](../api/overview)，不能直接把 generated output 当作手写实现维护。

Browser/Desktop 通过 encrypted `/webrtc/v1/offer` signaling 建立连接，并在 ordered
`giznet/v1/service/0` DataChannel 上传输 protobuf RPC envelope、body frames 和 EOS。
`connectGiznetWebRTC` 在 offer 前创建 packet DataChannel 和 Opus-capable audio
transceiver；调用方注入 identity、crypto、fetch 等 runtime-specific primitives。

`createWebRTCFetch` 是 generated client 的 fetch adapter boundary。当前 WebRTC bridge
按 GizClaw RPC method 映射 HTTP request，并不是任意 HTTP proxy。SDK 修改至少运行：

```sh
npm --prefix sdk/js test
npm --prefix sdk/js run gen:sdk
```
