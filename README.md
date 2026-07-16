# GizClaw

[![CI](https://github.com/GizClaw/gizclaw/actions/workflows/ci.yml/badge.svg)](https://github.com/GizClaw/gizclaw/actions/workflows/ci.yml)
[![CodeQL](https://github.com/GizClaw/gizclaw/actions/workflows/codeql.yml/badge.svg)](https://github.com/GizClaw/gizclaw/actions/workflows/codeql.yml)
[![GitHub Pages](https://github.com/GizClaw/gizclaw/actions/workflows/guides-pages.yml/badge.svg)](https://gizclaw.github.io/gizclaw/)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/GizClaw/gizclaw/badge)](https://scorecard.dev/viewer/?uri=github.com/GizClaw/gizclaw)
[![Go Reference](https://pkg.go.dev/badge/github.com/GizClaw/gizclaw-go.svg)](https://pkg.go.dev/github.com/GizClaw/gizclaw-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/GizClaw/gizclaw?filename=go.mod)](go.mod)
[![Release](https://img.shields.io/github/v/release/GizClaw/gizclaw?include_prereleases&sort=semver)](https://github.com/GizClaw/gizclaw/releases)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
![Transport](https://img.shields.io/badge/transport-WebRTC-0ea5e9)
![SDK](https://img.shields.io/badge/SDK-C%20%7C%20Dart%20%7C%20JS%20%7C%20Go-22c55e)
![Status](https://img.shields.io/badge/status-active%20development-f59e0b)

![GizClaw agent runtime and edge-server platform](guides/references/assets/readme-hero.png)

GizClaw is an out-of-the-box agent runtime and edge-server platform for GizClaw
devices. It currently supports authoritative servers and single-upstream edge
ingress; a self-organizing multi-server mesh remains future work.

**Documentation:** [English](https://gizclaw.github.io/gizclaw/en/) ·
[简体中文](https://gizclaw.github.io/gizclaw/zh/)

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
- [x] Edge-node ingress and optional TURN relay with authoritative-server
  forwarding over Giznet/WebRTC service streams.
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
- [x] Generated API packages and SDK surfaces for Go, JavaScript, Dart/Flutter,
  C-facing clients, CLI, and e2e harnesses.

## Roadmap

- [ ] Production-ready Admin UI and Play UI.
- Other workflow, agent, and realtime conversation engines:
  - [ ] OpenAI Realtime
  - [ ] Coze
  - [ ] Eino
- [ ] Generalized gameplay reward entry points beyond `pet.drive`, including
  built-in tool exposure for agent-driven reward grants.
- [ ] Pet-readable gameplay event writes into pet workspace memory/history after
  care actions and game results.
- [ ] Self-organizing server mesh where devices attach to one node and requests
  can route through other nodes to the node that owns the target device data.
- [ ] Stabilize the Flutter SDK and mobile client for broader end-user and
  platform coverage.
- [ ] Third-party digital content federation with joint authorization and access
  from agent runtimes.
- [ ] Expanded digital content delivery for gameplay content beyond current pet
  and badge pixa resources.
- [ ] Additional process-wide `slog` sinks beyond stderr and Volc TLS.
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
- `pkgs/gizedge/`: single-upstream edge ingress, forwarding, and optional TURN
  relay runtime.
- `pkgs/store/`: key-value, object, SQL-backed, metrics, vector, graph, and
  identity store primitives.
- `pkgs/agent/`: agent memory, recall, embedding, and local runtime support.
- `pkgs/genx/`: model and generation abstractions used by workflows and agents.
- `pkgs/audio/`: audio codecs, resampling, raw Opus packet helpers, playback, and
  voiceprint helpers.
- `api/`: source OpenAPI and RPC schemas. Generated Go and TypeScript code is
  derived from these files.
- `sdk/`: Go, JavaScript/TypeScript, Dart/Flutter, and C-facing SDK surfaces.
- `js/`: shared JavaScript packages and browser/runtime tooling.
- `apps/wails/`: desktop shell and frontend.
- `apps/gizclaw-app/`: Flutter mobile client for iOS and Android.
- `guides/`: localized development, review, coding, usage, and Reference
  documentation.
- `skills/`: project-level Agent Skills installable with `npx skills`.
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

The hosted [Project Guide](https://gizclaw.github.io/gizclaw/) selects English
or Simplified Chinese from the browser language. Direct locale entrypoints are
[English](https://gizclaw.github.io/gizclaw/en/) and
[简体中文](https://gizclaw.github.io/gizclaw/zh/).

- [Development Guide](https://gizclaw.github.io/gizclaw/en/developing/):
  architecture, API design, package boundaries, CLI, applications, and SDK
  development.
- [Review Guide](https://gizclaw.github.io/gizclaw/en/reviewing/): review
  workflow, review items, self-review, PR Agent review, and issue review.
- [Coding Conventions](https://gizclaw.github.io/gizclaw/en/coding-styles/): Go,
  JavaScript/TypeScript, Dart/Flutter, C/cgo, and documentation conventions.
- [Usage Guide](https://gizclaw.github.io/gizclaw/en/using/): CLI, Wails,
  Flutter, and SDK usage.
- [References](https://gizclaw.github.io/gizclaw/references/): Go references
  plus local Flutter Dartdoc and TypeScript TypeDoc generation.
- [API Design](https://gizclaw.github.io/gizclaw/en/developing/api/overview):
  HTTP, Protobuf/RPC, schema ownership, and generated-surface boundaries.

Documentation sources remain under [`guides/`](guides/), and source API
contracts remain under [`api/`](api/README.md). Start and validate the Guide
locally with:

```sh
npm ci --prefix guides
npm --prefix guides run dev
npm --prefix guides run build:site
```

`build:site` checks that every Chinese Markdown route has an English counterpart
before building the production site.

## Agent Skills

The repository includes project-level Agent Skills for CLI, server, context,
Admin, firmware, workspace, and Play workflows. Install a skill from the
repository root with:

```sh
npx skills add . --skill gizclaw-cli
```

See the [Agent Skills catalog](skills/README.md) for the full list and global
installation commands.
