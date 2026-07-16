# Transformers Overview

`pkgs/genx/transformers` Converts a `genx.Stream` to another Stream. Provider Adapters are responsible for external speech/realtime protocols; Stream Processing is responsible for provider-neutral TTS normalization, segmentation and combination.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers)

## Adapter structure

| Adapter | Capabilities |
| --- | --- |
| [Doubao Speech](./doubao) | ASR, TTS, Realtime, Realtime Duplex and speech translation. |
| [DashScope](./dashscope) | Realtime multimodal conversation. |
| [MiniMax](./minimax) | Streaming TTS. |
| [Stream Processing](./stream-processing) | Provider-neutral mux, TTS normalization, text segmentation, and stream wrapper. |

```mermaid
flowchart LR
    Input["Input Stream"] --> Mux["Transformer Mux"]
    Mux --> Doubao["Doubao Adapter"]
    Mux --> DashScope["DashScope Adapter"]
    Mux --> MiniMax["MiniMax Adapter"]
    Mux --> Processing["Stream Processing"]
    Doubao --> Output["Output Stream"]
    DashScope --> Output
    MiniMax --> Output
    Processing --> Output
```

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`Mux`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Mux) | Universal Transformer registry. |
| [`Transform`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Transform) | Select and execute the Transformer via the default mux. |
| [`Handle`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Handle) | Register a universal Transformer. |

ASR, TTS, Realtime and other capabilities all implement the same `genx.Transformer` and are registered through the same `Mux`. Guide does not define additional facade, session factory, or registry APIs for capability classes.
