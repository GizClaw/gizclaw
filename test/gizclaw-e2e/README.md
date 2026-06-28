# GizClaw E2E

This directory contains the manual/setup-driven GizClaw e2e suites. These tests
depend on a prepared local e2e server, a shared business resource set, and generated
CLI config homes. Go test files in this tree require the `gizclaw_e2e` build tag
so they are not pulled into ordinary `go test ./...` runs.

## Modules

- `testdata/`: committed config/resource data plus ignored generated runtime files.
- `setup/`: lifecycle scripts for building the CLI, starting services,
  resetting shared data, granting the default client view, and stopping
  services.
- `client/`: typed client and protocol-level e2e tests.
- `cmd/`: user-facing `gizclaw` CLI command e2e tests.
- `ui/`: browser UI e2e tests for Admin UI, Play UI, and cross-surface smoke
  checks.

## Standard Flow

1. Copy `test/gizclaw-e2e/.env.example` to `test/gizclaw-e2e/.env`, then fill
   provider credential values. The same file may also override context roles
   when running against existing local or remote dev contexts. Runtime
   addresses, resource names, resource IDs, model IDs, voice IDs, and e2e
   identity keys are committed fixtures, not env values.

2. Build the e2e CLI binary:

```sh
./test/gizclaw-e2e/setup/build.sh
```

3. Start the local e2e server:

```sh
./test/gizclaw-e2e/setup/start-server.sh
```

4. Clear and initialize server resources:

```sh
./test/gizclaw-e2e/setup/reset_data.sh
```

To let another peer public key use the default shared client view, apply a
`PeerConfig` for that key:

```sh
./test/gizclaw-e2e/setup/apply_client_view.sh <peer-public-key>
```

`reset_data.sh` only rebuilds resource state: provider tenants, models,
workflows, workspaces, firmware metadata, ACL rows, and social graph
resources. It does not call provider sync operations. It must not seed runtime
history, message records, replay audio, or other non-resource state.

5. Run client tests that create runtime state. These should run before any UI
   test that expects conversations, history entries, replay data, or social
   message state to already exist:

```sh
go test -tags gizclaw_e2e -count=1 ./test/gizclaw-e2e/client/admin
go test -tags gizclaw_e2e -count=1 ./test/gizclaw-e2e/client/chat
go test -tags gizclaw_e2e -count=1 ./test/gizclaw-e2e/client/rpc
go test -tags gizclaw_e2e -count=1 ./test/gizclaw-e2e/client/social
```

6. Run CLI story tests against the same setup-created server and resource
   catalog:

```sh
go test -tags gizclaw_e2e -count=1 ./test/gizclaw-e2e/cmd/connect
```

### Giznet Transport Parity

The committed setup server exposes both giznet transports:

- `9820/udp`: Noise over UDP
- `9820/tcp`: public HTTP API and WebRTC signaling
- `9821/udp` and `9821/tcp`: WebRTC ICE

For #90 transport parity, run the same client suites twice with separate
contexts and a data reset between runs. The Noise context is
`e2e-client`; the WebRTC context is `e2e-client-webrtc`.

```sh
./test/gizclaw-e2e/setup/reset_data.sh
GIZCLAW_E2E_CLIENT_CONTEXT=e2e-client \
go test -tags gizclaw_e2e -count=1 \
  ./test/gizclaw-e2e/client/admin \
  ./test/gizclaw-e2e/client/rpc \
  ./test/gizclaw-e2e/client/chat \
  ./test/gizclaw-e2e/client/social \
  ./test/gizclaw-e2e/cmd/connect

./test/gizclaw-e2e/setup/reset_data.sh
GIZCLAW_E2E_CLIENT_CONTEXT=e2e-client-webrtc \
go test -tags gizclaw_e2e -count=1 \
  ./test/gizclaw-e2e/client/admin \
  ./test/gizclaw-e2e/client/rpc \
  ./test/gizclaw-e2e/client/chat \
  ./test/gizclaw-e2e/client/social \
  ./test/gizclaw-e2e/cmd/connect
```

The harness logs the selected context transport and dial address when it binds
an alias or opens a direct `gizcli.Client` connection.

7. For browser UI tests, start the matching UI surface after the needed client
   tests have created runtime state:

```sh
./test/gizclaw-e2e/setup/start-admin-ui.sh
./test/gizclaw-e2e/setup/start-play-ui.sh
```

Then run the relevant UI suite:

```sh
go test -tags gizclaw_e2e -count=1 ./test/gizclaw-e2e/ui/admin/...
go test -tags gizclaw_e2e -count=1 ./test/gizclaw-e2e/ui/play/...
go test -tags gizclaw_e2e -count=1 ./test/gizclaw-e2e/ui/smoke/...
```

8. Stop e2e services when finished:

```sh
./test/gizclaw-e2e/setup/stop.sh
```

The full e2e run is intentionally ordered. Setup creates resource state, client
tests exercise the server and create runtime state, and UI tests verify the
browser surfaces against the resulting server state. Do not make UI tests depend
on setup-created runtime records.

## Test Data

`testdata/resources` is the business resource set used by client,
cmd, and UI tests. It is organized by resource domain instead of by test
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
`test/gizclaw-e2e/.env` during setup. `reset_data.sh` skips real-provider
fixtures whose required credentials are empty, while still initializing fake
fixtures with committed non-secret defaults. Do not commit real provider keys,
tokens, app secrets, or access keys. Stable e2e identity key pairs are committed
config fixtures, not env values.

`~/Work/haivivi/env` can be used as a private source for local provider values.
For example, MiniMax maps `minimax_cn_key` / `minimax_cn_group_id` and
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

Provider-independent resource rows use schema-valid committed metadata and do
not require real provider credentials. Real provider resources still depend on
credential values from `.env` and are skipped when those values are absent.
`reward-claim` and `pet-action` are Volc/Doubao-backed gameplay system task
model rows; business RPC tests skip when reset_data did not apply them.
`client/admin` owns provider voice sync verification and should run before chat
voice tests.

Workspace history is runtime data. `family-circle-chatroom-workspace` is a normal
chatroom workspace target. `reset_data.sh` must not seed
history entries or audio directly; social and workspace e2e cases should create
history by running the relevant client workflows.

## Config Homes

`testdata/admin-config-home` and `testdata/gizclaw-config-home` are
`XDG_CONFIG_HOME` roots. They must contain the normal `gizclaw/` config layout
and committed client `identity.key` fixtures. Context config files must store
the server `public-key` directly; do not point contexts at the server
`identity.key`, because that file is the server private key.

Optional role overrides in `.env` let e2e suites target existing context homes
without changing test code:

- `GIZCLAW_E2E_ADMIN_SETUP_CONFIG_HOME` / `GIZCLAW_E2E_ADMIN_SETUP_CONTEXT`:
  setup resource initialization.
- `GIZCLAW_E2E_ADMIN_CLI_CONFIG_HOME` / `GIZCLAW_E2E_ADMIN_CLI_CONTEXT`:
  admin CLI story target role.
- `GIZCLAW_E2E_CLIENT_CONFIG_HOME` / `GIZCLAW_E2E_CLIENT_CONTEXT`: ordinary
  client, workspace, and RPC cases. The committed contexts are `e2e-client`
  for Noise over UDP and `e2e-client-webrtc` for WebRTC.
- `GIZCLAW_E2E_SOCIAL_PERSON_A_CONFIG_HOME` /
  `GIZCLAW_E2E_SOCIAL_PERSON_A_CONTEXT`: social role A.
- `GIZCLAW_E2E_SOCIAL_PERSON_B_CONFIG_HOME` /
  `GIZCLAW_E2E_SOCIAL_PERSON_B_CONTEXT`: social role B.
- `GIZCLAW_E2E_PLAY_UI_CONFIG_HOME` / `GIZCLAW_E2E_PLAY_UI_CONTEXT`: Play UI
  launcher.
- `GIZCLAW_E2E_PLAY_CLI_CONFIG_HOME` / `GIZCLAW_E2E_PLAY_CLI_CONTEXT`: play CLI
  story target role.

Unset values fall back to the committed `testdata` config homes and context
names.

The setup scripts, chat client tests, UI resource lookup, and social peer A/B
harness read their matching role overrides. Most `cmd/*` story tests still
create isolated sandbox contexts unless a specific story opts into one of the
CLI target roles.

## Client Tests

`client/admin` contains typed Admin HTTP API contract coverage using the
generated `adminservice` client. It verifies Swagger-defined request/response
schemas, pagination, binary upload/download where the current Admin API exposes
it, provider voice sync prerequisites for chat tests, and selected
mutation-safe paths against the shared setup server.

`client/rpc` contains typed RPC coverage. Test files should be grouped by RPC
module prefix, and individual methods should be split by `Test...` functions.

`client/chat` contains workspace-backed voice conversation and history cases as
ordinary `_test.go` files. It should not use a custom `main.go -case ...`
dispatcher.

`client/social` contains friend and friend-group behavior. These tests are
client-driven, not CLI-story driven, and should cover relation changes,
workspace ACL visibility, message rounds, `workspace.history.updated`, history
list/get cursor behavior, and history replay.

## CLI Tests

`cmd` tests run the real `gizclaw` binary from `testdata/bin/gizclaw` through
Go `os/exec`. They should not use `go run` and should not shortcut through
typed clients.

The `cmd` layout mirrors the real CLI command hierarchy: `root`, `gen-key`,
`context`, `serve`, `service`, `migrate`, `connect`, `admin`, and `play`. Each
command directory has one `USER_STORIES.md` plus focused `_test.go` files.

`cmd/play` tests the `gizclaw play` command. Browser Play UI behavior belongs
under `ui/play`.

## UI Tests

`ui/admin` and `ui/play` are browser UI tests organized by visible page or major
surface. Missing business resources should be added under the matching
`testdata/resources/<NN-domain>/` directory and initialized by
`setup/reset_data.sh`.

`ui/smoke` contains cross-surface checks, such as opening both Admin UI and Play
UI against the same shared test service.
