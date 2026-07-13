# pkgs/audio/songs

保存内置 song、note、tempo、voice 与 metronome definitions，并把乐谱渲染为 PCM chunks。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/songs)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Song` / `Note` / `Voice` | 表达 song metadata 与多声部 notes。 |
| `Tempo` / `TimeSignature` / `Metronome` | 描述节拍和时间结构。 |
| `BeatNote` / `BeatVoice` / `N` | 以 beats 构造旋律。 |
| `All` / `ByID` / `ByName` | 索引内置 songs。 |
| `RenderOptions` / `DefaultRenderOptions` | 配置 PCM render。 |
| `VoiceToChunk` | 将 voice 渲染为指定 PCM format。 |

Songs package 拥有内置乐谱和合成逻辑，不拥有播放设备、用户 playlist 或产品资源 storage。
