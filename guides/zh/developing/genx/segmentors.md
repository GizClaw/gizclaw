# Segmentors

`pkgs/genx/segmentors` 将 conversation 内容整理为 segment、entity 和 relation，为 memory 写入与后续 recall 提供结构化结果。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`Segmentor`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Segmentor) | Conversation segmentation contract。 |
| [`Input`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Input) | 承载待分析的 conversation 输入。 |
| [`Result`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Result) | 返回 segments、entities 与 relations。 |
| [`Schema`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Schema) | 约束可抽取的实体和关系结构。 |
| [`GenX`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#GenX) | 使用 Generator 完成结构化 segmentation。 |
| [`Process`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Process) | 通过默认 mux 选择 Segmentor 并处理输入。 |

Segmentors 只负责内容结构化，不保存 conversation、entity graph 或 vector index；这些持久化职责属于 Agent memory 与 stores。
