# Match

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx/match)

`pkgs/genx/match` Compiles YAML rules into matchers and performs template, variable and optional model-assisted matching on `genx.Message`. It is suitable for declaratively identifying input intent or extracting rule results.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/match)

Match is only responsible for rule evaluation and does not own Agent routing, HTTP endpoints or workflow lifecycle. The caller decides subsequent product behavior based on the matching results.

## Rule and stream contract

`Compile` requires a non-empty ordered rule list. Rule names must be non-empty, trimmed, unique, and safe for the line-based `name: arguments` format. Variable names use Go-style identifiers; non-empty labels must be trimmed; and supported types are `string`, `int`, `float`, and `bool`. An omitted type retains string parsing, while an omitted label retains the existing unexpanded-placeholder behavior. Every pattern placeholder must reference a declared variable. Invalid patterns, examples, references, or typed declarations fail compilation.

The compiled Matcher owns copies of the variable metadata used at runtime. It can therefore be shared by concurrent calls, and later caller mutation does not alter parsing.

`Matcher.Parse` accepts text split at arbitrary chunk boundaries. It preserves line order across UTF-8 text, CRLF, multiple lines in one chunk, split lines, and a final unterminated line. Empty lines produce no result. A known rule produces typed arguments, including missing declarations with `value: null` and `has_value: false`. Unknown output becomes a result with an empty `rule` and the trimmed line in `raw_text`.

`Project` returns `[]any` containing this exact JSON-compatible item shape:

```json
{
  "rule": "play_music",
  "args": {
    "title": {
      "value": "Canon",
      "var": {
        "label": "Song title",
        "type": "string"
      },
      "has_value": true
    }
  },
  "raw_text": ""
}
```

A stream can yield a parsed prefix before a later error. Callers that require atomic state updates must collect successfully before publishing the projected list.
