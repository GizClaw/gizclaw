# Match

`pkgs/genx/match` Compiles YAML rules into matchers and performs template, variable and optional model-assisted matching on `genx.Message`. It is suitable for declaratively identifying input intent or extracting rule results.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`Rule`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Rule) | Define matching rules, patterns, arguments and examples. |
| [`Pattern`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Pattern) | Describes a single matching pattern. |
| [`Matcher`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Matcher) | Holds the compiled rules and performs matching. |
| [`Result`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Result) | Returns the hit rule and parsing parameters. |
| [`ParseRuleYAML`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#ParseRuleYAML) | Parse a single Rule from YAML. |
| [`Compile`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Compile) | Verify and compile Rules into Matcher. |
| [`Collect`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/match#Collect) | Collect the results or errors of matcher iterator. |

Match is only responsible for rule evaluation and does not own Agent routing, HTTP endpoints or workflow lifecycle. The caller decides subsequent product behavior based on the matching results.
