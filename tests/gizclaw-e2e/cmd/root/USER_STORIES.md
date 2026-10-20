# Root CLI

## User Story

As a developer, I want the root `gizclaw` command to expose the supported command groups and keep compatibility for legacy single-dash long flags.

## Covered Behaviors

- `gizclaw --help` lists the top-level command groups.
- Legacy single-dash long flags are normalized before command dispatch.

- The exact root inventory includes `history-migrate` and `kv-migrate`; their validation owners are the existing `cmd/internal/commands/historymigrate/command_test.go` and `cmd/internal/commands/kvmigrate/command_test.go` files. Other command groups retain their E2E story owners.
