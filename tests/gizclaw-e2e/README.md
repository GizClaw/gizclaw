# GizClaw E2E

This directory contains the manual/setup-driven GizClaw e2e suites. These tests
depend on a prepared local e2e server, a shared business resource set, and
committed WebRTC identities. Go test files in this tree require the
`gizclaw_e2e` build tag so they are not pulled into ordinary `go test ./...`
runs.

## Modules

- `testdata/`: committed identity/resource data plus ignored generated runtime files.
- `docker/`: Docker Compose e2e stack for the setup server and desktop surface.
- `setup/`: local fallback scripts and container-internal setup steps for
  building the CLI, resetting shared data, granting the default client view,
  and stopping local fallback services.
- `cmd/`: user-facing `gizclaw` CLI command e2e tests.
- `go/`: Go Admin API, RPC, chat, and social e2e tests.
- `js/`: reserved JavaScript/TypeScript package e2e suites.
- `desktop/`: Wails desktop shell e2e suites, with Admin and Play coverage added
  as those views are rewritten.

## Standard Flow

1. Copy `tests/gizclaw-e2e/.env.example` to `tests/gizclaw-e2e/.env`, then fill
   provider credential values. The same file may also override context roles
   when running against existing local or remote dev contexts. Runtime
   addresses, resource names, resource IDs, model IDs, voice IDs, and e2e
   identity keys are committed fixtures, not env values.

2. Run the default ordered WebRTC e2e gate:

```sh
./tests/gizclaw-e2e/run_tests.sh
```

The script builds the host e2e CLI, starts an isolated Docker Compose stack,
waits for the Docker server and Docker desktop surface, writes generated
host-side config under `testdata/docker/<project>/`, then runs JS WebRTC,
desktop shell, Go Admin API, chat, RPC, social, and selected CLI suites one at
a time. It excludes human-review cases, which require separate interactive
audio review. It stops the Compose stack on success or failure.

For debugging, the same flow can be run manually.

Human-review cases are not part of the default e2e gate. Run them explicitly:

```sh
./tests/gizclaw-e2e/run_human_review_tests.sh
```

3. Start a Docker e2e stack manually:

```sh
docker build -f build/Dockerfile.cn.base \
  -t gizclaw-go:linux-amd64-cn-base \
  build

project=gizclaw-e2e-manual
compose_file=tests/gizclaw-e2e/docker/docker-compose.yaml
docker compose -p "$project" -f "$compose_file" up -d --build

server_port=$(docker compose -p "$project" -f "$compose_file" port server 9820 | awk -F: '{print $NF}')
desktop_port=$(docker compose -p "$project" -f "$compose_file" port desktop 4191 | awk -F: '{print $NF}')

export GIZCLAW_E2E_SERVER_ENDPOINT="127.0.0.1:${server_port}"
export GIZCLAW_E2E_DESKTOP_URL="http://127.0.0.1:${desktop_port}"
```

The Compose file owns Docker lifecycle. Use normal Compose commands for logs,
status, and shutdown:

```sh
docker compose -p "$project" -f "$compose_file" logs -f
docker compose -p "$project" -f "$compose_file" down -v
```

4. Build the host e2e CLI binary:

```sh
./tests/gizclaw-e2e/setup/build.sh
```

5. Local fallback only: start the e2e server on the committed fixed endpoint:

```sh
./tests/gizclaw-e2e/setup/start-server.sh
```

This path is not the default e2e lifecycle. It is useful for debugging a single
local checkout when port collisions are not a concern.

6. Local fallback only: clear and initialize server resources:

```sh
./tests/gizclaw-e2e/setup/reset_data.sh
```

To let another peer public key use the default shared client view, apply a
`PeerConfig` for that key:

```sh
./tests/gizclaw-e2e/setup/apply_client_view.sh <peer-public-key>
```

`reset_data.sh` only rebuilds resource state: provider tenants, models,
workflows, workspaces, firmware metadata, ACL rows, and social graph
resources. It does not call provider sync operations. It must not seed runtime
history, message records, replay audio, or other non-resource state.

7. Run Go API/RPC tests that create runtime state:

```sh
go test -tags gizclaw_e2e -count=1 -skip '^(TestHumanReview|TestServerSocialRPCHumanReview)$' ./tests/gizclaw-e2e/go/admin
go test -tags gizclaw_e2e -count=1 -skip '^(TestHumanReview|TestServerSocialRPCHumanReview)$' ./tests/gizclaw-e2e/go/chat
go test -tags gizclaw_e2e -count=1 -skip '^(TestHumanReview|TestServerSocialRPCHumanReview)$' ./tests/gizclaw-e2e/go/rpc
go test -tags gizclaw_e2e -count=1 -skip '^(TestHumanReview|TestServerSocialRPCHumanReview)$' ./tests/gizclaw-e2e/go/social
```

8. Run desktop shell tests against the same setup context model:

```sh
go test -tags gizclaw_e2e -count=1 ./tests/gizclaw-e2e/desktop/...
```

When `GIZCLAW_E2E_DESKTOP_URL` is set, Playwright uses that existing desktop
surface and does not start a local Vite process. For manual frontend inspection
outside the Docker e2e lifecycle, start the Vite app directly:

```sh
npm --workspace @gizclaw/desktop run dev
```

9. Run CLI story tests against the same setup-created server and resource
   catalog:

```sh
go test -tags gizclaw_e2e -count=1 ./tests/gizclaw-e2e/cmd/connect
```

10. Stop local fallback services when finished:

```sh
./tests/gizclaw-e2e/setup/stop.sh
```

The full e2e run is intentionally ordered. The Docker server initializes
resource state through the setup scripts, and Go/JS/Desktop tests exercise the
server and create runtime state through public API/RPC paths. Do not make tests
depend on setup-created runtime records.

## Test Data

`testdata/resources` is the business resource set used by Go, JS, desktop, and
cmd tests. It is organized by resource domain instead of by test
surface:

```text
resources/
  00-credentials/
  01-tenants/
  03-models/
  04-workflows/
  05-workspaces/
  06-firmwares/
  07-gameplay/
  08-voices/
  09-social/
  90-acl/
  assets/
```

Resource fixture filenames use a local numeric prefix inside each resource
domain directory, for example `00-credentials/00-openai.yaml` or
`04-workflows/06-flowcraft-chat.yaml`. The directory prefix controls
cross-resource apply order, and the file prefix controls order within that
resource domain. `gizclaw admin apply` accepts JSON and YAML, but committed e2e
resource fixtures should use `.yaml`.

Only credential-like provider values should be environment placeholders, such as
`${GIZCLAW_E2E_OPENAI_API_KEY}`. Values are supplied by
`tests/gizclaw-e2e/.env` during setup. `reset_data.sh init/reset` fails before
starting setup when any required provider credential is missing. Do not commit
real provider keys, tokens, app secrets, or access keys. Stable e2e identity key
pairs are committed config fixtures, not env values.

`~/Work/haivivi/env` can be used as a private source for local provider values.
For example, Volc/Doubao maps `bytedance_ark_token` to
`GIZCLAW_E2E_VOLC_ARK_API_KEY`, and maps `bytedance_speech_app_id`,
`bytedance_speech_access_token`, and `bytedance_speech_search_api_key` to the
matching `GIZCLAW_E2E_DOUBAO_*` values. MiniMax maps `minimax_cn_key` /
`minimax_cn_group_id` and
`minimax_global_key` / `minimax_global_group_id` to the matching
`GIZCLAW_E2E_MINIMAX_*` values in `.env`. Qwen should be represented by the
DashScope provider (`GIZCLAW_E2E_DASHSCOPE_API_KEY`) when a DashScope/Tongyi
credential is available.

Generated runtime data under `testdata/server-workspace/data/` and generated
binaries under `testdata/bin/` stay ignored.

## Resource Set

`setup/reset_data.sh init` creates a resource set that looks like a small real
deployment: provider tenants, model rows, voice rows, workflows, workspaces,
firmware entries, ACL policy bindings, and social graph rows. Client, CLI, and
UI tests should be written around this business resource set instead of adding
private per-test or UI-specific resource groups. Tests may still create and delete
`mutation-*` resources for mutation coverage.

Stable business resource IDs:

- Workflow: `flowcraft-support`
- Run-control workflow: `chatroom-direct`
- Chatroom workflow: `family-circle-chatroom`
- Workspace: `support-desk-workspace`
- Run-control workspace: `direct-chatroom-workspace`
- Family chatroom workspace: `family-circle-chatroom-workspace`
- Model: `openai-gpt-4o-mini`
- Gameplay system task models: `reward-claim`, `pet-action` (Volc/Doubao credentials required)
- Credential: `openai-main-credential`
- MiniMax voice metadata row: `minimax-narrator-clone`
- Volc voice metadata row: `volc-tenant:volc-main:zh_female_vv_mars_bigtts`
- Pet species: `rabbit`
- Badge: `founder`
- Firmware: `devkit-firmware-main`
- Firmware channel/artifact: `stable` / `main`
- Mutation-safe names: `mutation-flowcraft-workflow`, `mutation-flowcraft-workspace`,
  `mutation-openai-model`, `mutation-openai-credential`

Bulk fake resource prefixes:

- `flowcraft-scenario-000` through `flowcraft-scenario-119`
- `workspace-scenario-000` through `workspace-scenario-119`
- `fake-openai-chat-000` through `fake-openai-chat-079`
- `fake-openai-credential-000` through `fake-openai-credential-049`
- `devkit-firmware-000` through `devkit-firmware-079`

The committed firmware metadata is applied through ResourceList YAML, but the
downloadable firmware payload is a real tar fixture at
`testdata/assets/firmware/devkit-firmware-main.tar`. During init,
`reset_data.sh` uploads that tar with:

```sh
gizclaw admin firmwares upload-artifact devkit-firmware-main \
  --channel stable \
  -f testdata/assets/firmware/devkit-firmware-main.tar
```

Provider-independent resource rows use schema-valid committed metadata, but the
full e2e resource catalog also includes real provider rows. Required provider
credential values must be present in `.env`; otherwise `reset_data.sh init/reset`
fails fast and no partial e2e setup should be treated as valid. `reward-claim`
and `pet-action` are Volc/Doubao-backed gameplay system task model rows.
`go/admin` owns provider voice sync verification and should run before chat
voice tests.

Workspace history is runtime data. `family-circle-chatroom-workspace` is a normal
chatroom workspace target. `reset_data.sh` must not seed
history entries or audio directly; social and workspace e2e cases should create
history by running the relevant client workflows.

## Identities And CLI Config Homes

`testdata/identities` contains committed WebRTC identity directories for Go,
JS, and desktop harnesses. Each directory stores `identity.key` and an
endpoint-only `config.yaml`:

```yaml
description: Local e2e peer
server:
  endpoint: 127.0.0.1:9820
  public-key: <server-public-key>
```

Stable identities:

- `admin`: setup resource initialization and admin API tests.
- `peer`: ordinary client, workspace, RPC, and chat tests.
- `social-a`: primary social peer.
- `social-b`: secondary social peer.

`testdata/cmd-config-home` is the only committed `XDG_CONFIG_HOME`-style config
root. It is used by `cmd/` tests because those tests exercise real CLI context
behavior.

Context overrides in `.env` select identities:

- `GIZCLAW_E2E_IDENTITIES_HOME`: defaults to
  `tests/gizclaw-e2e/testdata/identities`.
- `GIZCLAW_E2E_ADMIN_IDENTITY`: committed default is `admin`.
- `GIZCLAW_E2E_PEER_IDENTITY`: committed default is `peer`.
- `GIZCLAW_E2E_SOCIAL_PERSON_A_IDENTITY`: committed default is `social-a`.
- `GIZCLAW_E2E_SOCIAL_PERSON_B_IDENTITY`: committed default is `social-b`.

`GIZCLAW_E2E_CONFIG_HOME` is only for CLI config-home tests and setup scripts.
It defaults to `tests/gizclaw-e2e/testdata/cmd-config-home`.
`GIZCLAW_E2E_CMD_GEAR1_CONTEXT` and `GIZCLAW_E2E_CMD_GEAR2_CONTEXT` can
override the setup script peer contexts inside that CLI config home.

Transport is not configured with an environment variable. GizClaw e2e uses
WebRTC only; the server and context config shape is a single `endpoint`.

## Go Tests

`go/admin` contains typed Admin HTTP API contract coverage using the
generated `adminservice` client. It verifies Swagger-defined request/response
schemas, pagination, binary upload/download where the current Admin API exposes
it, provider voice sync prerequisites for chat tests, and selected
mutation-safe paths against the shared setup server.

`go/rpc` contains typed RPC coverage. Test files should be grouped by RPC
module prefix, and individual methods should be split by `Test...` functions.

`go/chat` contains workspace-backed voice conversation and history cases as
ordinary `_test.go` files. It should not use a custom `main.go -case ...`
dispatcher.

`go/social` contains friend and friend-group behavior. These tests are
client-driven, not CLI-story driven, and should cover relation changes,
workspace ACL visibility, message rounds, `workspace.history.updated`, history
list/get cursor behavior, and history replay.

## CLI Tests

`cmd` tests run the real `gizclaw` binary from `testdata/bin/gizclaw` through
Go `os/exec`. They should not use `go run` and should not shortcut through
typed clients.

The `cmd` layout mirrors the real CLI command hierarchy: `root`, `gen-key`,
`context`, `serve`, `service`, `migrate`, `connect`, and `admin`. Each command
directory has one `USER_STORIES.md` plus focused `_test.go` files.

## UI Tests

The old CLI-served browser UI e2e tree has been removed. Desktop UI coverage is
rebuilt under `desktop/` with the Wails follow-up issues.
