# gizclaw-go

[![CI](https://github.com/GizClaw/gizclaw-go/actions/workflows/ci.yml/badge.svg)](https://github.com/GizClaw/gizclaw-go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/GizClaw/gizclaw-go/actions/workflows/codeql.yml/badge.svg)](https://github.com/GizClaw/gizclaw-go/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GizClaw/gizclaw-go)](https://goreportcard.com/report/github.com/GizClaw/gizclaw-go)

`gizclaw-go` is the Go implementation of the GizClaw server, CLI, store layer, and agent/runtime packages.

## Layout

- `cmd/`: CLI entrypoint and command implementations
- `pkgs/store/`: storage primitives such as KV, graph, filesystem, and vector stores
- `pkgs/agent/`: agent-side runtime packages such as `embed`, `memory`, `ncnn`, and `recall`
- `pkgs/genx/`: model/generation abstractions and integrations
- `examples/`: runnable examples; each `main.go` example directory is its own Go module
- `tests/`: end-to-end and scenario-driven tests

## Development

```bash
go test ./...
```

The GitHub Actions workflow in `.github/workflows/ci.yml` currently runs `go test -count=1 ./...` on pushes to `main`, pull requests, and manual dispatch across Linux, macOS, and Windows.

## Docs And Skills

- GizClaw CLI skill: `skills/gizclaw-cli/SKILL.md`
- GenX example: `examples/genx/README.md`
