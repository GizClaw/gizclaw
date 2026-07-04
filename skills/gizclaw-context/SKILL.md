---
name: gizclaw-context
version: 1.0.0
description: "Manage GizClaw CLI contexts and connectivity checks. Use for gizclaw context create/use/list/info/show, gizclaw peer ping, and gizclaw peer server-info."
metadata:
  requires:
    bins: ["gizclaw"]
---

# GizClaw Context

Use this skill when the user needs to connect the CLI to a GizClaw server,
inspect saved contexts, check connectivity, or read public server metadata.

## When To Use

- User asks to create, switch, list, or inspect a CLI context.
- User asks why an admin/client command cannot connect.
- User asks to verify server connectivity or latency.
- User asks for server public metadata.

## How To Start

1. Choose command prefix: `gizclaw`, or `go run ./cmd/gizclaw` inside this repo if needed.
2. If the user named a context, pass `--context <name>` to server-facing commands.
3. If the current context is unclear, run `context info` or `context list`.
4. Use `ping` for peer connectivity and `server-info` for public server metadata.

## Commands

```bash
<gizclaw> context create <name> --server <host:port> --public-key <server-public-key-base58btc>
<gizclaw> context use <name>
<gizclaw> context list
<gizclaw> context info
<gizclaw> context show <name>
<gizclaw> ping --context <name>
<gizclaw> server-info --context <name>
```

## Behavior Notes

- Contexts are local client profiles.
- Each context stores one `config.yaml` containing `identity.private-key`, server address, and server public key.
- Default context storage:
  - Linux/macOS: `$XDG_CONFIG_HOME/gizclaw` or `~/.config/gizclaw`
  - Windows: `%AppData%/gizclaw`
- The first created context becomes current when no current context exists.
- `context list` marks the active context with `*`.
- `context info` shows the current context.
- `context show <name>` shows a named context without switching to it.
- `ping` prints server time, RTT, and clock diff.
- `server-info` prints public server metadata.

## Failure Handling

- If there is no active context, create one or run `context use <name>`.
- If ping fails, verify `context show <name>` has the expected server address and public key.
- If a server-facing command fails with wrong public key or connection errors, check `peer server-info --context <name>` and then the server process.
