# MiniMax Adapter

`minimaxtts` package 将 MiniMax 语音合成适配为 GenX Transformer。

```go
transformer, err := minimaxtts.New(minimaxtts.Config{
    Client:  client,
    VoiceID: "female-shaonv",
})
```

`Config` 保存不可变的 client、model、voice、speed、volume、pitch、emotion、format、sample rate 和 bitrate。`New` 校验 client 与 voice，但不建立连接。每次 `Transform` 独占 Stream lifecycle 和 provider request state，因此同一个已配置 Transformer 支持并发调用。

MiniMax TTS 是非 agent 的 Stream-to-Stream Transformer，不提供 Toolkit 配置入口。
