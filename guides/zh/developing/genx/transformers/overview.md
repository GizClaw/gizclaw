# Transformers 总览

`pkgs/genx/transformers` 将一个 `genx.Stream` 转换为另一个 Stream。Provider Adapters 负责外部 speech/realtime 协议；Stream Processing 负责 provider-neutral 的生命周期、buffer、audio byte stream filtering、TTS segmentation 和组合。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers)

## Adapter 结构

| Adapter | 能力 |
| --- | --- |
| [Doubao Speech](./doubao) | ASR、TTS、Realtime、Realtime Duplex 与 speech translation。 |
| [DashScope](./dashscope) | Realtime multimodal conversation。 |
| [MiniMax](./minimax) | Streaming TTS。 |
| [Flowcraft](./flowcraft) | 文本 Stream 驱动的 Flowcraft Graph runtime。 |
| [Stream Processing](./stream-processing) | Provider-neutral 的 mux、Stream lifecycle、audio byte stream filtering 和文本分段。 |

Provider 实现与共享的内部 Stream lifecycle 使用独立 package：

```text
pkgs/genx/transformers/
├── audiostream/
├── internal/streamkit/
├── flowcraft/
├── doubaoasr/
├── doubaotts/
├── minimaxtts/
├── doubaoast/
├── doubaorealtime/
├── doubaorealtimeduplex/
└── dashscoperealtime/
```

每个 provider package 都提供 typed constructor，例如 `doubaoasr.New`、`doubaotts.NewSeedV2` 和 `minimaxtts.New`。Constructor 只解析不可变配置，不建立连接；每次 `Transform(ctx, input)` 单独建立并管理 provider session。Provider adapter 不再通过 flat `transformers.New*` constructor 暴露。

ASR、TTS、AST 和 Doubao Realtime Dialogue 都只是 Stream-to-Stream Transformer，不属于 agent-capable runtime。当前 provider 协议能够支持 Toolkit continuation 的 package 是 Doubao Realtime Duplex 与 DashScope Realtime。StreamKit 与该分类无关，也不拥有 Tool 或 Toolkit。

```mermaid
flowchart LR
    Input["Input Stream"] --> Mux["Transformer Mux"]
    Mux --> Doubao["Doubao Adapter"]
    Mux --> DashScope["DashScope Adapter"]
    Mux --> MiniMax["MiniMax Adapter"]
    Mux --> Processing["Stream Processing"]
    Mux --> Flowcraft["Flowcraft Graph"]
    Doubao --> Output["Output Stream"]
    DashScope --> Output
    MiniMax --> Output
    Processing --> Output
    Flowcraft --> Output
```

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`Mux`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Mux) | 通用 Transformer registry。 |
| [`Transform`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Transform) | 通过默认 mux 选择并执行 Transformer。 |
| [`Handle`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Handle) | 注册通用 Transformer。 |

ASR、TTS、Realtime 和其他能力都实现同一个 `genx.Transformer`，并通过同一个 `Mux` 注册。`Mux` 实现 `genx.TransformerMux`，pattern 不会传入具体 Transformer。Guide 不为能力类别定义额外的 facade、session factory 或 registry API。
