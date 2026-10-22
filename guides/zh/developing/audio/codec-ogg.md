# pkgs/audio/codec/ogg

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/codec/ogg)

实现 Ogg container 的 page、packet 和 stream framing，不解释 packet 内部使用 Opus 或其他 codec。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg)

Ogg package 拥有 container framing、checksum 和 page sequencing；Opus header 和 PCM conversion 属于 `codecconv` 与 `codec/opus`。
