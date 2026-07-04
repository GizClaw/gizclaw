---
name: gizclaw-server
version: 1.0.0
description: "Start and manage GizClaw server workspaces. Use for gizclaw serve, gizclaw service install/status/start/stop/restart/uninstall, workspace config.yaml editing, and Admin UI serve entrypoint."
metadata:
  requires:
    bins: ["gizclaw"]
---

# GizClaw Server

Use this skill for server process management and server workspace configuration.

## When To Use

- User asks to start a GizClaw server manually.
- User asks to install, start, stop, restart, status, or uninstall the system service.
- User asks to edit or explain a server workspace `config.yaml`.
- User asks to open the Admin web UI with `admin --listen`.

## How To Start

1. Identify whether the workspace is manually served or service-managed.
2. For foreground/manual operation, use `serve <workspace>`.
3. For service-managed workspaces, use `service ...`; do not use `serve --force`.
4. Before editing a service-managed workspace config, stop the service.
5. Run long-lived `serve` or UI commands in the background and monitor startup output.

## Foreground Server

```bash
<gizclaw> serve <workspace>
<gizclaw> serve --force <workspace>
```

- `serve` always runs in the foreground.
- `--force` means stop a previous foreground server for the same workspace before starting.
- `--force` does not mean foreground.
- `serve` and `serve --force` reject service-managed workspaces.

## System Service

```bash
<gizclaw> service install <workspace>
<gizclaw> service status
<gizclaw> service start
<gizclaw> service stop
<gizclaw> service restart
<gizclaw> service uninstall
```

- `install <workspace>` installs the service definition.
- `status`, `start`, `stop`, `restart`, and `uninstall` do not take a workspace argument.
- Repeating `install` should fail until the old service is uninstalled.
- `uninstall` stops the service before removing the service definition.

## Workspace Layout

`serve` uses `<workspace>` as the working directory.

```text
<workspace>/
├── config.yaml
├── serve.pid
└── firmware/
```

- `config.yaml`: server configuration, including `identity.private-key`.
- `serve.pid`: process mutual exclusion for foreground and service-managed starts.
- `firmware/`: default firmware file storage if configured.

## Workspace Config

The server reads `<workspace>/config.yaml`. Relative paths inside the config are resolved from the workspace directory.

Minimal persistent config:

```yaml
listen: ":9820"
stores:
  gears:
    kind: keyvalue
    backend: badger
    dir: gears
  firmware:
    kind: filestore
    backend: filesystem
    dir: firmware
gears:
  store: gears
depots:
  store: firmware
```

Config rules:

- `listen` is the UDP listen address. Default is `:9820`.
- `stores` defines named storage backends.
- `gears.store` references a `kind: keyvalue` store, commonly `memory` for tests or `badger` for persistence.
- `depots.store` references a `kind: filestore` store, usually `backend: filesystem`.
- `dir` is used by `badger` and `filesystem`; relative paths are under the workspace.
- SQL stores can use `backend: sqlite` with `dir`, or `backend: postgres` with `dsn`.
- Graph stores with `backend: kv` use `store` to reference a keyvalue store.
- Memory stores lose data after restart.

Service-managed edit flow:

```bash
<gizclaw> service stop
<edit <workspace>/config.yaml>
<gizclaw> service start
```

## Admin UI

```bash
<gizclaw> admin --listen 127.0.0.1:8080
```

Run this in the background and monitor startup output.
