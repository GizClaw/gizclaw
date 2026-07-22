# AgentKit

`pkgs/genx/agentkit` 保存可复用的 Agent stream 组合能力。它只依赖 GenX interface，不读取 Workspace、Workflow、RuntimeProfile 或 provider credential。

## Audio Dock

[`audiodock`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock) 把一个文本 `genx.Transformer` 与可选 ASR、TTS 组合成新的 `genx.Transformer`：

```go
dock, err := audiodock.New(audiodock.Config{
    Agent: textAgent,
    ASR:   asrTransformer,
    TTS:   ttsMux,
    ResolveVoice: func(ctx context.Context, request audiodock.VoiceRequest) (string, error) {
        return voicePattern(request.Name), nil
    },
})
```

文本输入直接进入 Agent。音频输入以原有 StreamID 增量送入 ASR；ASR 完成的 transcript 作为一轮文本输入送入 Agent。Agent 的文本输出立即可 pull，同时已交付文本会复制给 TTS。TTS 音频和文本共用 response StreamID，但各 MIME channel 独立发送 EOS。

`ResolveVoice` 接收 response StreamID、输出 node/name 与 chunk metadata，返回交给 TTS mux 的 pattern。同一个 response 内的每个具名 publisher 都会独立解析，因此并行 Flowcraft publisher 可以共用 response StreamID、但使用不同 voice。返回空 pattern 时只保留该 publisher 的文本，不合成音频。RuntimeProfile alias 的解析属于产品 factory，不属于 Audio Dock。

一个 Dock 可以并发处理多个 `Transform`。ASR session、Agent run、voice、TTS session、buffer、取消和错误都属于单次调用及其 StreamID；一个 route 失败不会终止其他调用。输出使用可增长内部队列，因此 producer 不依赖消费者及时 pull 才能继续读取 provider stream。

关闭输出会取消对应的 ASR、Agent 和 TTS 工作。被打断的 route 删除未 pull 的后缀，并为已声明的 MIME channel 发送带错误的 EOS。Audio Dock 不执行 ToolCall，也不拥有 provider 协议。
