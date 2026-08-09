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

Server configuration has three layers:

- `storage` is a dynamic registry of physical connectors and owns DSNs, endpoints, credentials, pools/clients, readiness, and close.
- `stores` is a dynamic registry of logical data-access interfaces and owns prefix, table, database, or topic scope.
- `services` is a fixed typed structure that explicitly binds every built-in consumer to compatible named Stores.

Use `guides/snippets/server-storage-stores-services.yaml` as the complete configuration reference. A reduced fragment looks like this, but a runnable Server still requires every core `services` block from the complete reference:

```yaml
listen: 0.0.0.0:9820
endpoint: gizclaw.example.com:9820

storage:
  main-kv:
    kind: badger
    dir: data/kv

stores:
  peer-records:
    kind: keyvalue
    storage: main-kv
    prefix: peers

services:
  peer:
    store: peer-records
```

Config rules:

- Registry keys are exact, case-sensitive operator-defined names; they have no reserved service meaning.
- Required service bindings never default from a Store name, kind, prefix, or another Store.
- Concrete physical kinds are `badger`, `memory`, `filesystem.dir`, `sqlite`, `postgresql`, `clickhouse`, `prometheus`, and `volc-tls`; no nested driver selector is accepted.
- Prometheus and Volc TLS are physical connector kinds. Their endpoints and credentials belong under `storage`.
- The six logical Store kinds are `keyvalue`, `sql`, `objectstore`, `metrics`, `log.immutable`, and `log.mutable`.
- Keyvalue Stores may use Badger or memory. SQLite/PostgreSQL keyvalue Stores require one single-segment `prefix`; the backend uses it as the physical table name, `table` is invalid, and encoded keys do not repeat the prefix. Metrics Stores may additionally use Prometheus, ClickHouse, SQLite, or PostgreSQL.
- `log.immutable` may use Volc TLS, ClickHouse, SQLite, or PostgreSQL; `log.mutable` may use ClickHouse, SQLite, or PostgreSQL. SQL-backed Metrics and Log Stores require `table`.
- SQLite/PostgreSQL KV declarations claim the physical table derived from `prefix`; Metrics and Log declarations claim their explicit `table`. Physical names are unqualified ASCII values of at most 63 bytes and must be distinct on one connector (ASCII case-insensitive on SQLite, exact on PostgreSQL); the complete claim set is validated before any DDL.
- SQL-backed logical Stores directly ensure their table and indexes with idempotent DDL, then validate the exact schema. They create no version or history table and borrow the physical pool. Close logical Stores before Storage; they never close the pool themselves.
- Public vecstore and graph packages remain available but are not Server Store kinds.
- `memory` is a stateless marker; every logical Store entry creates its own in-process backend.
- `cmd/internal/server` owns YAML DTO decoding and explicit conversion. `pkgs/store/storage` owns typed physical resources, while root `pkgs/store` builds logical Stores and never closes the physical registry.
- `memory.Store` is selected through RuntimeProfile and MemoryLayout and is not a Server Store kind.
- Relative physical paths are resolved from the workspace. Logical Store values are not rewritten.
- `services.agent_host`, `services.metrics`, and `services.system_log` are optional; all other built-in service blocks are required.
- `services.system_log` defaults to info-level stderr when omitted.
- Service-internal collections use code-owned prefixes; expanded bindings such as `route_store`, `invite_token_store`, and `member_store` are unsupported.
- Old top-level pseudo-service blocks, nested Storage drivers, one-layer Stores, generic `kind: log`, and `gizclaw migrate` are unsupported. Stop and recreate incompatible development services; do not import or rewrite existing data automatically.

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
