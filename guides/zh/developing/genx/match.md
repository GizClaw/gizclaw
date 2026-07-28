# Match

`pkgs/genx/match` 将 YAML rule 编译为 matcher，并对 `genx.Message` 执行 template、variable 和可选模型辅助匹配。它适合声明式识别输入意图或提取规则结果。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`Rule`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Rule) | 定义匹配规则、patterns、arguments 和 examples。 |
| [`Pattern`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Pattern) | 描述单个匹配 pattern。 |
| [`Matcher`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Matcher) | 持有编译后的规则并执行匹配。 |
| [`Result`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Result) | 返回命中的规则及解析参数。 |
| [`ParseRuleYAML`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#ParseRuleYAML) | 从 YAML 解析单条 Rule。 |
| [`Compile`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Compile) | 校验并编译 Rules 为 Matcher。 |
| [`Collect`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Collect) | 收集 matcher iterator 的结果或错误。 |
| `Matcher.Parse` | 把任意边界的文本 chunk 分帧为有序的逐行结果。 |
| `Project` | 把有序结果转换为防御性持有的 JSON-compatible value。 |

Match 只负责规则求值，不拥有 Agent routing、HTTP endpoint 或 workflow lifecycle。调用方根据匹配结果决定后续产品行为。

## Rule 与 stream contract

`Compile` 要求有序且非空的 rule list。Rule name 必须非空、无首尾空格、唯一，并且能安全用于逐行的 `name: arguments` 格式。Variable name 使用 Go 风格 identifier；非空 label 必须没有首尾空格；type 只支持 `string`、`int`、`float` 和 `bool`。省略 type 时继续按 string 解析，省略 label 时保留既有的 placeholder 不展开行为。每个 pattern placeholder 必须引用已声明的 variable。非法 pattern、example、reference 或 typed declaration 会让编译失败。

编译后的 Matcher 持有运行时 variable metadata 的副本，因此可以被并发调用共享；调用方在构造后修改原始 rule 不会改变解析结果。

`Matcher.Parse` 接受任意 chunk 边界的文本，并在 UTF-8、CRLF、单 chunk 多行、跨 chunk 行和末尾无换行场景保持逐行顺序。空行不产生结果。已知 rule 产生 typed arguments；未提供的声明仍会出现，并带有 `value: null` 和 `has_value: false`。未知输出的 `rule` 为空，trim 后的原始行放在 `raw_text`。

`Project` 返回 `[]any`，其中每项都是以下准确的 JSON-compatible shape：

```json
{
  "rule": "play_music",
  "args": {
    "title": {
      "value": "卡农",
      "var": {
        "label": "歌曲名",
        "type": "string"
      },
      "has_value": true
    }
  },
  "raw_text": ""
}
```

Stream 可能在后续错误前先产生一个已解析 prefix。需要原子更新状态的调用方必须在完整收集成功后再发布投影列表。
