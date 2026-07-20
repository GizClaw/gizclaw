# 独立流式语音识别

`server.speech.transcribe` 在不创建、不选择 Workspace 的情况下，把一段有界音频转换成最终 transcript。

Request 包含 RuntimeProfile `model_alias`、`content_type` 和可选 `language`。初始 wire format 是 `audio/L16;rate=16000;channels=1`：16 kHz、单声道、signed 16-bit little-endian PCM。Client 写完 typed request envelope 后持续发送 binary frame，最后发送 request EOS。Server 以 backpressure 把 chunk 交给 alias 对应的 ASR Transformer，再返回 `SpeechTranscribeResponse` 与 response EOS。

一次调用独占一个可靠 Peer RPC service stream。它不创建 audio track、Media Channel、Peer connection、Workspace、history entry，也不保存音频。关闭 stream 或取消 context 会取消 provider 工作。

运行限制属于 Server config：

```yaml
speech:
  transcription:
    max_audio_bytes: 2097152
    max_audio_duration: 60s
    request_timeout: 75s
```

Transcript wire 上限是 8192 UTF-8 bytes。非法 metadata 返回 `INVALID_PARAMS`；未知或 dangling alias 返回 `NOT_FOUND`；空音频、格式错误、不支持或超过限制返回 `BAD_REQUEST`；provider failure 返回脱敏后的 `INTERNAL_ERROR`。

Go `TranscribeSpeech`、JavaScript `transcribeSpeech` 与 C `gzc_rpc_speech_transcribe_open/write/finish` 提供增量上传；Flutter 提供生成后的 typed method 与 payload surface。
