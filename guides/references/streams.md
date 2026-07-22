# Stream Reference

本页定义 Giznet DataChannel 的 service byte stream 写入约束。HTTP、RPC 与 Event 的上层 framing 不因分片而改变；DataChannel message boundary 不是上层 frame boundary。

## Reliable ordered service stream

所有 reliable、ordered `giznet/v1/service/<id>` DataChannel 都遵守同一写入模型：

- 每个 channel 只有一个串行 writer，并发逻辑写入的 bytes 不会交错。
- 每个原生 DataChannel message 最多承载 1400 bytes，接收端按连续 byte stream 重组 HTTP 或 RPC/Event frame。
- writer 在 buffered amount 到达 high-water 时停止入队，只在 buffered-amount-low 通知后确认队列不高于 low-water 才恢复。
- 写入完成只表示全部 bytes 已被本地 WebRTC 发送队列接受，不表示远端已经接收或处理。
- close、error、send failure 以及调用路径已有的 timeout/cancellation 会唤醒并终止 active/queued writes。部分逻辑写入失败后，该 service channel 必须关闭，剩余 bytes 不会换新 channel 重试。

| SDK | High-water | Low-water | Native message max |
| --- | ---: | ---: | ---: |
| Go server | 1 MiB | 256 KiB | 1400 bytes |
| JavaScript | 1 MiB | 256 KiB | 1400 bytes |
| Flutter | 1 MiB | 256 KiB | 1400 bytes |
| C API v2 default | 256 KiB | 64 KiB | 1400 bytes |

C 调用方可以通过 `gzc_client_config_t.service_write_high_water_bytes` 与 `service_write_low_water_bytes` 调大阈值；自定义 high-water 不得小于 1400 bytes，且 low-water 必须小于 high-water。`write_timeout_ms` 使用 platform 的单调 `time_instant_ms` 计算完整同步逻辑写入的 elapsed time。同步 C API 只在调用期间借用 caller buffer。

## Exclusions

Unreliable/unordered direct packet DataChannel、Telemetry packet 与 RTP media 不使用 service writer，也不继承上述 water marks。BOS/EOS 等业务边界仍由各自上层协议定义。
