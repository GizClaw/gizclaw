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

Match 只负责规则求值，不拥有 Agent routing、HTTP endpoint 或 workflow lifecycle。调用方根据匹配结果决定后续产品行为。
