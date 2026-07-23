# Testing and E2E

This page documents repository-level test harnesses. Ordinary Go unit tests
still run according to the changed scope. Suites that require a build tag,
Docker, live providers, or human judgment must be started explicitly and must
not be reported as passing when they were not run.

## GizClaw Docker E2E

`tests/gizclaw-e2e` is the Docker-backed full GizClaw environment. Its Go tests
use the `gizclaw_e2e` build tag and are therefore excluded from ordinary
`go test ./...` runs.

```text
tests/gizclaw-e2e/
├── docker/      # Compose services and container entrypoints
├── setup/       # environment lifecycle and seed scripts
├── testdata/    # committed identities/resources and ignored runtime output
├── cmd/         # real gizclaw CLI tests
├── go/          # Admin, chat, gameplay, RPC, and social tests
├── js/          # JavaScript/TypeScript WebRTC tests
└── desktop/     # Wails shell, Admin, and Play tests
```

Copy the provider credential template first. `.env` is only for provider
credentials; runtime addresses, resource/model/voice IDs, and E2E identities do
not belong there. Never commit real credentials.

```sh
cp tests/gizclaw-e2e/.env.example tests/gizclaw-e2e/.env
bash tests/gizclaw-e2e/run_tests.sh
```

The full gate installs locked Node workspaces, initializes nanopb, builds the
E2E CLI, starts Compose, waits for Server/Desktop, runs JS, Desktop, C/cgo,
Admin, chat, gameplay, RPC, social, and CLI phases in order, and performs
bounded cleanup. The total deadline defaults to 90 minutes. Per-phase defaults
are 15 minutes, with 30 minutes for Docker setup and CLI, 45 minutes for live
chat, and 5 minutes for cleanup. Positive integer seconds may be supplied in:

- `GIZCLAW_E2E_FULL_DEADLINE_SECONDS`
- `GIZCLAW_E2E_PHASE_DEADLINE_SECONDS`
- `GIZCLAW_E2E_PREFLIGHT_DEADLINE_SECONDS`
- `GIZCLAW_E2E_DOCKER_SETUP_DEADLINE_SECONDS`
- `GIZCLAW_E2E_DOCKER_CLEANUP_DEADLINE_SECONDS`
- `GIZCLAW_E2E_CHAT_DEADLINE_SECONDS`
- `GIZCLAW_E2E_CLI_DEADLINE_SECONDS`

### Manual environment

Start or stop only the environment with:

```sh
bash tests/gizclaw-e2e/setup/docker-compose-up.sh
bash tests/gizclaw-e2e/setup/docker-compose-down.sh
```

Setup selects random free Edge and Admin host ports. Firmware or LAN clients
need an explicitly reachable address:

```sh
GIZCLAW_E2E_EDGE_HOST=192.168.1.20 \
  bash tests/gizclaw-e2e/setup/docker-compose-up.sh
```

Generated state lives below `tests/gizclaw-e2e/testdata/docker/<project>/` and
the latest environment entrypoint is
`tests/gizclaw-e2e/testdata/docker/current.env`:

```sh
set -a
source tests/gizclaw-e2e/testdata/docker/current.env
set +a
```

`GIZCLAW_E2E_EDGE_ENDPOINT` is client-facing and
`GIZCLAW_E2E_SERVER_ENDPOINT` is host-Admin-facing. The remaining generated
variables provide the CLI config home, identity home, Desktop URL, and Compose
project. Reset the standard resource set with:

```sh
bash tests/gizclaw-e2e/setup/reset-data.sh reset --context remote-admin
```

`init` only applies fixtures, `clear` only removes known fixtures, and `reset`
performs both. Only credential placeholders are expanded from `.env`; missing
provider credentials fail before a partial setup can be treated as valid.
Workspace history is runtime data and must not be seeded by the reset script.

### Suite ownership

- `go/admin` validates typed contracts with the generated Admin HTTP client.
- `go/rpc` groups typed RPC tests by module.
- `go/chat` covers workspace voice, interruption, history, and memory.
- `go/social` covers relations, domain workspaces, messages, and history events from clients.
- `cmd` executes `testdata/bin/gizclaw` with `os/exec`; it must not bypass the CLI with `go run` or typed clients.
- `desktop/shell` covers the Pod shell; `desktop/admin` and `desktop/play` cover browser surfaces.
- `js/admin` covers WebRTC Admin fetch; `js/rpc` covers peer and server-initiated RPC.

Human audio review is separate from the automated gate:

```sh
bash tests/gizclaw-e2e/run_human_review_tests.sh
```

## Giznet E2E

`tests/giznet-e2e` exercises the public Giznet transport through gizwebrtc:

```sh
go test -tags giznet_e2e ./tests/giznet-e2e/...
go test -tags giznet_e2e ./tests/giznet-e2e/webrtc \
  -run '^$' -bench BenchmarkWebRTCHTTPRoundTrip -benchtime=1x
```

## LoCoMo Memory Evaluation

`tests/locomo-e2e` is a GizClaw-owned manual evaluation of production
`memory.Store` implementations. It does not use Flowcraft's evaluator and is
not part of ordinary `go test ./...`, Docker E2E, or required CI. Each live Go
test owns its complete provider, memory-lane, and extraction configuration.
Remote project configuration remains deployment state: the harness neither
mutates it nor presents one endpoint/project as multiple lanes.

Current lanes cover Flowcraft BBH BM25 single-pass, hybrid single/two-pass,
Mem0 Platform default/custom-instructions, and Volc AgentKit Memory default.
Run one explicitly selected lane with:

```sh
cp tests/locomo-e2e/.env.example tests/locomo-e2e/.env
bash tests/locomo-e2e/run_tests.sh
```

The script has a 30-minute default whole-test timeout and bounded session and
question stages. The runner calls `memory.Store.Observe` by official session,
recalls for every question, asks the configured model to answer, and computes
EM, F1, and evidence-hit metrics locally. The default gate requires aggregate
F1 of at least `0.05`, evidence hit rate of at least `0.50` for evidence-aware
stores, and one materialized fact per selected session. Provider failures and
timeouts remain failures. Ignored `reports/` output must never contain secrets.

### Dataset and license

`testdata/locomo10_smoke.jsonl` is a Git LFS object adapted for noncommercial
use from SNAP Research's LoCoMo `locomo10.json`. It contains the first three
sessions of `conv-30` (58 turns) and six questions whose evidence is entirely
within those sessions. It is a contract smoke set, not a full benchmark. Exact
upstream commit, checksum, subset, and transformation information is recorded
in `locomo10_smoke.manifest.json`.

The subset is distributed under
[CC BY-NC 4.0](https://creativecommons.org/licenses/by-nc/4.0/) for
noncommercial use only; `LICENSE.locomo.txt` preserves the license. Upstream
timestamps have no timezone. The stored `Z` is only a deterministic Go
`ObservedAt` mapping and does not claim an original timezone. Run `git lfs pull`
after cloning; the loader rejects unresolved LFS pointers.

Offline validation:

```sh
go test -race -tags gizclaw_locomo_e2e \
  -run 'TestDataset|TestScore|TestPreflight|TestRedaction|TestSession|TestRunBenchmark|TestAwait' \
  ./tests/locomo-e2e
bash -n tests/locomo-e2e/run_tests.sh
git lfs fsck
```
