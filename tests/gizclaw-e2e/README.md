# GizClaw E2E

This directory contains the Docker-backed GizClaw e2e environment and the test
suites that run against it. Go test files in this tree require the
`gizclaw_e2e` build tag so they are not included in ordinary `go test ./...`
runs.

## Directory Layout

- `docker/`: Docker Compose services, entrypoints, and container-only setup
  scripts.
- `setup/`: host-facing tools for starting, stopping, and interacting with the
  Docker e2e environment. Invoke these scripts with `bash`.
- `testdata/`: committed identities, resources, and ignored runtime output.
- `cmd/`: user-facing `gizclaw` CLI e2e tests.
- `go/`: Go Admin API, chat, gameplay, RPC, and social e2e tests.
- `js/`: JavaScript/TypeScript e2e tests.
- `desktop/`: Wails desktop shell e2e tests.

## Credentials

Copy the example file and fill every provider credential before starting the
Docker e2e environment:

```sh
cp tests/gizclaw-e2e/.env.example tests/gizclaw-e2e/.env
```

`tests/gizclaw-e2e/.env` is for provider credentials only. Do not put runtime
addresses, config homes, identity directories, resource IDs, model IDs, voice
IDs, or e2e identity keys in `.env`.

The setup fails before resource initialization if any required credential is
missing or still looks like a placeholder. Do not commit real provider keys,
tokens, app secrets, or access keys.

## Start The Environment

Run the full ordered e2e gate:

```sh
bash tests/gizclaw-e2e/run_tests.sh
```

The script builds the host e2e CLI, starts the Docker Compose stack, waits for
the server and desktop surface, writes a generated host-side runtime env, runs
the ordered JS, desktop, Admin API, chat, gameplay, RPC, social, and selected
CLI suites, and stops the stack on success or failure.

For manual work, start only the Docker e2e environment:

```sh
bash tests/gizclaw-e2e/setup/docker-compose-up.sh
```

By default, the setup picks a random free host port and writes client contexts
that use `127.0.0.1:<port>`.

For LAN firmware clients, publish an address that the client can reach:

```sh
GIZCLAW_E2E_SERVER_HOST=192.168.1.20 \
  bash tests/gizclaw-e2e/setup/docker-compose-up.sh
```

To choose the mapped port explicitly:

```sh
GIZCLAW_E2E_DOCKER_SERVER_PORT=19820 \
GIZCLAW_E2E_SERVER_ENDPOINT=192.168.1.20:19820 \
  bash tests/gizclaw-e2e/setup/docker-compose-up.sh
```

The same host port is mapped to container `9820/tcp` and `9820/udp`. The server
binds `listen: 0.0.0.0:9820` inside the container. Generated client contexts use
the public `endpoint`, and the WebRTC answer advertises that endpoint for ICE
UDP and TCP host candidates when the endpoint host is a concrete IP. ICE TCP
shares the same public TCP endpoint as HTTP APIs and WebRTC signaling.

This setup expects clients to reach the published TCP and UDP port directly. It
does not configure STUN, TURN, mDNS, or ICE Lite.

## Runtime Env

Manual startup writes runtime state under:

```text
tests/gizclaw-e2e/testdata/docker/<project>/
  cmd-config-home/
  identities/
  docker.env
```

It also writes the latest environment path:

```text
tests/gizclaw-e2e/testdata/docker/current.env
```

Source it before running host-side manual commands:

```sh
source tests/gizclaw-e2e/testdata/docker/current.env
```

Important values in `current.env`:

- `GIZCLAW_E2E_SERVER_ENDPOINT`: client-facing server endpoint.
- `GIZCLAW_E2E_SERVER_PUBLIC_KEY`: server public key for CLI contexts.
- `GIZCLAW_E2E_CONFIG_HOME`: generated CLI config home used by cmd tests.
- `GIZCLAW_E2E_IDENTITIES_HOME`: generated identity directory used by Go/JS
  harnesses.
- `GIZCLAW_E2E_DESKTOP_URL`: Docker desktop surface URL.
- `GIZCLAW_E2E_DOCKER_PROJECT`: Docker Compose project name.

## Create A Context Home

Use a separate `XDG_CONFIG_HOME` when you want to interact with the e2e server
without modifying your normal GizClaw CLI contexts.

Start the Docker e2e environment, then run:

```sh
source tests/gizclaw-e2e/testdata/docker/current.env

mkdir -p tests/gizclaw-e2e/testdata/bin
go build -o tests/gizclaw-e2e/testdata/bin/gizclaw ./cmd/gizclaw

export XDG_CONFIG_HOME="$(mktemp -d)"
gizclaw_bin="tests/gizclaw-e2e/testdata/bin/gizclaw"

"$gizclaw_bin" context create my-e2e \
  --server "$GIZCLAW_E2E_SERVER_ENDPOINT" \
  --public-key "$GIZCLAW_E2E_SERVER_PUBLIC_KEY" \
  --description "Manual e2e context"

"$gizclaw_bin" context use my-e2e
"$gizclaw_bin" context info
```

`context create` generates a new client identity. If that identity should use
the default shared client view, pass its `identity_public` from
`gizclaw context info` to the Docker-backed setup server:

```sh
bash tests/gizclaw-e2e/setup/apply_client_view.sh <identity_public>
```

Then regular CLI commands can use the context:

```sh
"$gizclaw_bin" connect set-name "Manual E2E Client" --context my-e2e
"$gizclaw_bin" connect ping --context my-e2e
```

## Run Manual Tests

Start the environment and source `current.env`, then run focused suites:

```sh
go test -tags gizclaw_e2e -count=1 \
  -skip '^(TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$' \
  ./tests/gizclaw-e2e/go/admin

go test -tags gizclaw_e2e -count=1 \
  -skip '^(TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$' \
  ./tests/gizclaw-e2e/go/chat

go test -tags gizclaw_e2e -count=1 \
  -skip '^(TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$' \
  ./tests/gizclaw-e2e/go/gameplay

go test -tags gizclaw_e2e -count=1 \
  -skip '^(TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$' \
  ./tests/gizclaw-e2e/go/rpc

go test -tags gizclaw_e2e -count=1 \
  -skip '^(TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$' \
  ./tests/gizclaw-e2e/go/social

go test -tags gizclaw_e2e -count=1 ./tests/gizclaw-e2e/desktop/...
go test -tags gizclaw_e2e -count=1 ./tests/gizclaw-e2e/cmd/connect
```

Human-review cases are separate because they require interactive audio review:

```sh
bash tests/gizclaw-e2e/run_human_review_tests.sh
```

## Stop The Environment

Stop the current Docker e2e environment and remove generated runtime state:

```sh
bash tests/gizclaw-e2e/setup/docker-compose-down.sh
```

Generated server data, Docker runtime contexts, and binaries stay ignored:

```text
tests/gizclaw-e2e/testdata/server-workspace/data/
tests/gizclaw-e2e/testdata/docker/
tests/gizclaw-e2e/testdata/bin/
```

## Resource Set

Docker setup creates a small real deployment: provider tenants, model rows,
voice rows, workflows, workspaces, firmware entries, ACL policy bindings, and
social graph rows. Client, CLI, and UI tests should use this shared business
resource set instead of adding private per-test or UI-specific resource groups.
Tests may still create and delete `mutation-*` resources for mutation coverage.

Resource fixtures live under `testdata/resources`:

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

Only credential-like provider values should be environment placeholders, such as
`${GIZCLAW_E2E_OPENAI_API_KEY}`. Values are supplied by `.env` during Docker
setup.

Stable business resource IDs:

- Workflow: `flowcraft-support`
- Run-control workflow: `chatroom-direct`
- Chatroom workflow: `family-circle-chatroom`
- Workspace: `support-desk-workspace`
- Run-control workspace: `direct-chatroom-workspace`
- Family chatroom workspace: `family-circle-chatroom-workspace`
- Model: `openai-gpt-4o-mini`
- Credential: `openai-main-credential`
- MiniMax voice metadata row: `minimax-narrator-clone`
- Volc voice metadata row: `volc-tenant:volc-main:zh_female_vv_mars_bigtts`
- Firmware: `devkit-firmware-main`
- Firmware channel/artifact: `stable` / `main`
- Mutation-safe names: `mutation-flowcraft-workflow`,
  `mutation-flowcraft-workspace`, `mutation-openai-model`,
  `mutation-openai-credential`

Bulk fake resource prefixes:

- `flowcraft-scenario-000` through `flowcraft-scenario-119`
- `workspace-scenario-000` through `workspace-scenario-119`
- `fake-openai-chat-000` through `fake-openai-chat-079`
- `fake-openai-credential-000` through `fake-openai-credential-049`
- `devkit-firmware-000` through `devkit-firmware-079`

The committed firmware metadata is applied through ResourceList YAML, and the
downloadable firmware payload is the tar fixture at
`testdata/assets/firmware/devkit-firmware-main.tar`.

Provider-independent resource rows use schema-valid committed metadata, but the
full e2e resource catalog also includes real provider rows. Required provider
credential values must be present in `.env`; otherwise `reset_data.sh init/reset`
fails fast and no partial e2e setup should be treated as valid. `go/admin` owns
provider voice sync verification and should run before chat voice tests.

Workspace history is runtime data. `family-circle-chatroom-workspace` is a normal
chatroom workspace target. `reset_data.sh` must not seed
history entries or audio directly; social and workspace e2e cases should create
history by running the relevant client workflows.

## Identities And CLI Config Homes

`testdata/identities` contains committed WebRTC identity directories for Go,
JS, and desktop harnesses. Each directory stores a `config.yaml` with both the
local identity and server endpoint:

```yaml
description: Local e2e peer
identity:
  private-key: <client-private-key>
server:
  endpoint: 127.0.0.1:9820
  public-key: <server-public-key>
```

Stable identities:

- `admin`: setup resource initialization and admin API tests.
- `peer`: ordinary client, workspace, RPC, and chat tests.
- `social-a`: primary social peer.
- `social-b`: secondary social peer.

`testdata/cmd-config-home` is the committed CLI config root used by `cmd/`
tests. Docker e2e runs copy it into the generated runtime directory and rewrite
server endpoints there, leaving committed fixtures unchanged.

## Test Suite Notes

`go/admin` contains typed Admin HTTP API contract coverage using the generated
`adminservice` client.

`go/rpc` contains typed RPC coverage. Test files should be grouped by RPC module
prefix, and individual methods should be split by `Test...` functions.

`go/chat` contains workspace-backed voice conversation and history cases as
ordinary `_test.go` files.

`go/social` contains friend and friend-group behavior. These tests are
client-driven and should cover relation changes, workspace ACL visibility,
message rounds, `workspace.history.updated`, history list/get cursor behavior,
and history replay.

`cmd` tests run the real `gizclaw` binary from `testdata/bin/gizclaw` through
Go `os/exec`. They should not use `go run` and should not shortcut through
typed clients.

`desktop` contains Wails desktop shell coverage.
