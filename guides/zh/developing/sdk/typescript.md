# TypeScript SDK <Badge type="warning" text="WIP" />

> 本页目前只说明 SDK 的目录和 contract 边界，公开 surface、生成流程和 runtime 行为仍待逐项展开。

`sdk/js/gizclaw` 提供 TypeScript client surface，覆盖 Admin HTTP、Public HTTP、RPC、signaling 和 Telemetry。`sdk/js/scripts` 保存由 OpenAPI、Protobuf 与 method registry 生成 SDK surface 所需的工具。

```text
sdk/js/
├── gizclaw/     # SDK package 与 generated client
└── scripts/     # Contract generation 与生成结果修整
```

生成内容的 source of truth 位于 [API Design](../api/overview)，不能直接把 generated output 当作手写实现维护。
