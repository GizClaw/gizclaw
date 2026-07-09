# GizClaw

[![CI](https://github.com/GizClaw/gizclaw/actions/workflows/ci.yml/badge.svg)](https://github.com/GizClaw/gizclaw/actions/workflows/ci.yml)
[![CodeQL](https://github.com/GizClaw/gizclaw/actions/workflows/codeql.yml/badge.svg)](https://github.com/GizClaw/gizclaw/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/GizClaw/gizclaw/badge)](https://scorecard.dev/viewer/?uri=github.com/GizClaw/gizclaw)
[![Go Reference](https://pkg.go.dev/badge/github.com/GizClaw/gizclaw-go.svg)](https://pkg.go.dev/github.com/GizClaw/gizclaw-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/GizClaw/gizclaw?filename=go.mod)](go.mod)
[![Release](https://img.shields.io/github/v/release/GizClaw/gizclaw?include_prereleases&sort=semver)](https://github.com/GizClaw/gizclaw/releases)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
![Transport](https://img.shields.io/badge/transport-WebRTC-0ea5e9)
![SDK](https://img.shields.io/badge/SDK-C%20%7C%20JS%20%7C%20Go-22c55e)
![Status](https://img.shields.io/badge/status-active%20development-f59e0b)

![GizClaw agent runtime and server mesh](docs/assets/readme-hero.png)

GizClaw is an out-of-the-box agent runtime and edge server mesh for GizClaw
devices.

It provides the server, CLI, WebRTC transport, Admin/RPC APIs, workflow-backed
agent runtime, device telemetry, state monitoring, OTA firmware delivery,
digital content distribution, social graph, gameplay services, and shared SDK
contracts for connected devices, desktop clients, browser integrations, and
test harnesses.

GizClaw is designed for both developers and end users. Developers can build
devices, services, and browser integrations on top of the shared API contracts,
while desktop users can run a local server, use the UI directly, and customize
their own agents through workspace runtimes.

## Features

- [x] Out-of-the-box server and CLI for local or deployed GizClaw nodes.
- [x] Workspace-based agent runtime where each workspace is an instantiated agent
  environment backed by a workflow configuration.
- [x] Workflow drivers for runtime behavior such as Flowcraft agents, chatroom
  workflows, Doubao AST translation, and Doubao realtime flows.
- [x] WebRTC transport, signaling, and service streams for GizClaw node and
  client connectivity.
- [x] Device registration, configuration, telemetry intake, runtime state
  monitoring, and policy-controlled access.
- [x] Admin and peer RPC APIs generated from shared OpenAPI/RPC schemas.
- [x] Firmware catalog, channel-based OTA metadata, artifact upload, and
  authorized firmware file download.
- [x] Social workspace resources for contact, friend, and chatroom-style
  interactions.
- [x] Gameplay rulesets, point accounts, reward grants, pet adoption, drive
  actions, pet workspaces, game results, badge progression, and pixa asset
  delivery.
- [x] Generated API packages and SDK surfaces for Go packages, JavaScript
  browser clients, C-facing clients, CLI, and e2e harnesses.

## Roadmap

- [ ] Production-ready Admin UI and Play UI.
- [ ] Desktop app beyond the current debug/skeleton shell, including local
  server management and end-user agent customization.
- Other workflow, agent, and realtime conversation engines:
  - [ ] OpenAI Realtime
  - [ ] Coze
  - [ ] Eino
- [ ] Generalized gameplay reward entry points beyond `pet.drive`, including
  built-in tool exposure for agent-driven reward grants.
- [ ] Pet-readable gameplay event writes into pet workspace memory/history after
  care actions and game results.
- [ ] Self-organizing server mesh where devices attach to one node and requests can
  route through other nodes to the node that owns the target device data.
- [ ] Broader SDK coverage, including Flutter/mobile clients.
- [ ] Third-party digital content federation with joint authorization and access
  from agent runtimes.
- [ ] Expanded digital content delivery for gameplay content beyond current pet
  and badge pixa resources.
- [ ] Refresh repository-local agent skills for current CLI, admin, server,
  firmware, gear, workspace, and Play workflows.

## Repository Layout

- `cmd/gizclaw/`: CLI entrypoint for server, context, admin, and play commands.
- `cmd/internal/`: command-layer server wiring, storage setup, logging, and HTTP
  service helpers.
- `pkgs/gizclaw/`: core GizClaw server, peer services, generated API packages,
  CLI client helpers, and domain services.
- `pkgs/giznet/`: WebRTC transport, HTTP-over-service streams, and transport
  contracts.
- `pkgs/store/`: key-value, object, SQL-backed, metrics, vector, graph, and
  identity store primitives.
- `pkgs/agent/`: agent memory, recall, embedding, and local runtime support.
- `pkgs/genx/`: model and generation abstractions used by workflows and agents.
- `pkgs/audio/`: audio codecs, resampling, stamped Opus, playback, and
  voiceprint helpers.
- `api/`: source OpenAPI and RPC schemas. Generated Go and TypeScript code is
  derived from these files.
- `js/`: generated TypeScript SDK packages and browser/runtime tooling.
- `c/`: C-facing SDK surface and bindings.
- `apps/wails/`: desktop shell and frontend.
- `docs/`: design and operator documentation.
- `examples/`: runnable examples for GenX, WebRTC SFU, audio, songs, and
  voiceprint workflows.
- `tests/`: unit, integration, and Docker-backed e2e test projects.

## Development

Run the default Go validation:

```sh
go test ./...
```

For focused server and API work, these packages are often the fastest first
check:

```sh
go test ./cmd/... ./pkgs/... -count=1
```

Regenerate API code after changing files under `api/`:

```sh
go generate ./pkgs/gizclaw/api/...
npm --prefix sdk/js run gen:sdk
```

Docker-backed GizClaw e2e requires provider credentials in
`tests/gizclaw-e2e/.env`:

```sh
cp tests/gizclaw-e2e/.env.example tests/gizclaw-e2e/.env
bash tests/gizclaw-e2e/run_tests.sh
```

## Documentation

- [API definitions](api/README.md)
- [Server configuration](docs/server_config.md)
- [Context configuration](docs/context_config.md)
- [RPC protocol](docs/rpc_protocol.md)
- [ACL](docs/acl.md)
- [OTA](docs/ota.md)
- [Gameplay](docs/gameplay.md)
- [Agent and GenX](docs/agent_genx.md)
- [Service layout](docs/service_layout.md)
