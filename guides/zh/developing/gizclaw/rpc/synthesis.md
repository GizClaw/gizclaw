# 独立流式语音合成

`server.speech.synthesize` 在不让 Peer 直接播放、也不依赖 Workspace 的情况下返回合成音频数据。

Request 包含 RuntimeProfile `voice_alias`、最多 4096 UTF-8 bytes 的文字，以及一到八个可接受 MIME type。Server 在内部把 alias 解析为真实 Voice、Model、tenant 与 credential。发送 binary audio 前，Server 先返回 `SpeechSynthesizeResponse`，声明选中的 `content_type` 和可选 sample rate/channels。Binary frame 只是 transport chunk，不代表 codec packet 边界；response EOS 结束流。

Backpressure 从 TTS Transformer 一直保持到 RPC writer 和 Client reader。Server 不缓存完整输出、不创建 media track、不调用 `server.run.say`、不写 history，也不创建 Workspace。

运行限制属于 Server config：

```yaml
speech:
  synthesis:
    max_text_bytes: 4096
    max_output_bytes: 4194304
    request_timeout: 120s
```

非法 metadata 返回 `INVALID_PARAMS`；未知或 dangling alias 返回 `NOT_FOUND`；不支持或重复的 MIME type、非法文字返回 `BAD_REQUEST`；metadata 前的 provider failure 返回脱敏后的 `INTERNAL_ERROR`。Metadata 之后发生错误时，stream 异常结束，Client 不能把部分音频当成完整结果。

Go `SynthesizeSpeech`、JavaScript `synthesizeSpeech` 和 C `gzc_rpc_speech_synthesize` 都会增量暴露音频；Flutter 提供生成后的 typed method 与 payload surface。
