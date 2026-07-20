# DashScope Adapter

DashScope Adapter 通过 `dashscoperealtime.Transformer` 将 DashScope realtime multimodal session 适配为 `genx.Transformer`。

公共构造入口为 `dashscoperealtime.New(dashscoperealtime.Config{Client: client})`。Config 保存已解析的 DashScope client、model、voice、modalities、VAD 和 audio format 等不可变选项；constructor 不建立 WebSocket，每个并发 `Transform` 调用建立自己的 session。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `dashscoperealtime.Config` | 保存 client、realtime model、audio format、voice、instructions 和 turn detection 配置。 |
| `dashscoperealtime.New` | 使用类型化 Config 创建 Transformer；构造时不建立连接。 |
| `dashscoperealtime.Transformer.Transform` | 为每次调用建立独立 realtime session，将输入 Stream 写入 provider，并返回统一输出 Stream。 |
| `dashscoperealtime.Stream` | 包装支持 session update 的 realtime output Stream。 |

Provider session update 和 event name 留在 Adapter 内部；调用方只依赖 GenX Stream 与显式 update contract。
