# pkgs/audio/voiceprint

提供 speaker embedding 与 voice identity detection abstraction，支持 ECAPA、ERes2Net 和 NCNN-backed model paths，并通过 `vecid` 维护匹配 identity。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/voiceprint)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Detector` | 定义 audio input、speaker detection 与 lifecycle。 |
| `DetectorConfig` | 配置 model、threshold 和 vector identity behavior。 |
| `DetectResult` | 返回 speaker identity、confidence 与 embedding result。 |
| `DetectCallback` | 接收 detection events。 |
| `ConfidentGt` | 表达 confidence threshold callback。 |
| `NewECAPA` / `NewERes2Net` | 创建对应 speaker model detector。 |

Voiceprint package 负责 signal feature、embedding model 和 identity detection，不拥有录音权限、Peer identity、用户 profile 或生物特征数据 retention policy；这些由调用产品层明确控制。
