# Testing and E2E

This page documents repository-level test harnesses. Ordinary Go unit tests
still run according to the changed scope. Suites that require a build tag,
Docker, live providers, or human judgment must be started explicitly and must
not be reported as passing when they were not run.

## Credential-backed harness contract

GizClaw, GenX, LoCoMo, and Memory live suites each own one ignored `.env`,
described by a committed credential-only `.env.example`. Every field is
mandatory for every `run*_tests.sh` entrypoint in that harness, including
shorter entrypoints that do not consume every credential. Missing files,
missing/empty/whitespace values, and placeholder values fail before dependency
installation, builds, Docker or service startup, Go tests, and provider calls.
Diagnostics print names, never values.

Entrypoints have fixed checked-in package and test selections. Environment
variables may supply non-secret runtime parameters after an entrypoint is
chosen, but cannot select coverage or turn a selected failure into a skip.
Provider, fixture, network, timeout, rate-limit, and native-runtime failures
therefore fail the command. Direct tagged `go test` commands are not accepted
as live-suite evidence.

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

Firmware OTA changes can run their focused live stack and Admin/RPC/CLI/C SDK
coverage without the unrelated provider-backed suites:

```bash
bash tests/gizclaw-e2e/run_firmware_tests.sh
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

The disruptive Edge and provisioned Volc LogStore selections have their own
fixed entrypoints:

```sh
bash tests/gizclaw-e2e/run_edge_failure_tests.sh
bash tests/gizclaw-e2e/run_gateway_capacity_tests.sh
bash tests/gizclaw-e2e/run_gateway_capacity_100_tests.sh
bash tests/gizclaw-e2e/run_gateway_capacity_500_tests.sh
bash tests/gizclaw-e2e/run_gateway_capacity_1000_tests.sh
bash tests/gizclaw-e2e/run_gateway_capacity_1000_soak_tests.sh
bash tests/gizclaw-e2e/run_turn_relay_tests.sh

GIZCLAW_E2E_VOLC_LOG_ENDPOINT=... \
GIZCLAW_E2E_VOLC_LOG_REGION=... \
GIZCLAW_E2E_VOLC_LOG_TOPIC_ID=... \
  bash tests/gizclaw-e2e/run_volc_log_tests.sh
```

Credential-backed GizClaw entrypoints, including capacity and the focused
Server relay lane, require the same complete `tests/gizclaw-e2e/.env`. The
isolated relay-recovery lane generates runtime-only fixture credentials instead
of consuming provider credentials. The gateway-capacity entrypoint fixes the local
one-Server/two-Edge selection at the 100-session baseline. Its clients still
terminate at Edge, while every physical Edge-to-Server connection is
relay-only through two digest-pinned Coturn 4.7.0 members. Each Edge holds four
gateway upstream associations plus one control/HTTP upstream, so the fixed
topology has ten live Coturn allocations even though logical sessions use only
the four gateway associations per Edge. In addition to
connection hold and ping rounds, all 100 sessions synchronously upload and
download 4 MiB each, with aggregate throughput measured over one shared
wall-clock interval. The single-session control uses a sustained 32 MiB
payload. The machine-readable artifact is written under ignored `testdata/`;
this is not a long-soak or higher-session capacity promise. The dedicated
100-session burst entrypoint repeats three fresh-stack runs with no ramp,
reports establishment rate and Dial p50/p95/p99, transfers 1 MiB per session
in each direction, and records a sustained 32 MiB single-session control. Its
artifact separates key generation, client PeerConnection, offer, ICE
gathering, HTTP signaling, answer-side PeerConnection/SDP/ICE, and the client
ICE-connected, DTLS-connected, and DataChannel-ready milestones. Only the
client SCTP connected boundary is explicitly unsupported. Its hard gates are
100/100 establishment, at least 20 sessions/s,
Dial p95 at most
1 second and p99 at most 5 seconds, and at least 200 Mbps aggregate in each
direction. The single-session ratio remains a reported diagnostic because a
single local sample is too variable to be a reliable concurrent-throughput
gate.

The dedicated 500-session burst entrypoint uses the same fixed three-fresh-
stack, zero-ramp contract with concurrency 500. Each run assigns exactly 250
sessions to each Edge and requires exactly four upstream associations per
Edge. Hard gates are 500/500 usable sessions, zero establishment, ping,
disconnect, restart, or identity-crossover failures, at least 20 sessions/s,
Dial p95 at most 1 second and p99 at most 5 seconds, and exactly 500 MiB
(500 x 1 MiB) transferred in each direction at no less than 200 Mbps aggregate.
The 32 MiB single-session measurements and aggregate ratios are diagnostics,
not gates. Artifacts are written under ignored
`testdata/gateway-capacity-extended/sessions-500-burst/`; every run records the
exact repository head and dirty state, so publishable evidence must come from
the clean final PR head.

The dedicated 1,000-session burst entrypoint fixes relay-only upstreams, a
clean repository head, three fresh stacks, zero ramp, concurrency 1,000, a
30-second hold, and exactly 500 sessions per Edge across four gateway
upstreams. The load driver uses `GOGC=50` and records that value with
`GOMAXPROCS` in the artifact. This measured harness setting collects the
1,000-way client heap in smaller, more frequent cycles so the load driver does
not inject its own GC CPU and RTT spikes into the long-lived workload; it does
not change production processes, pacing, timeouts, or the release barrier. Each
run retains the 20 sessions/s, Dial p95/p99, exact 1 MiB per session and
direction, and 200 Mbps gates. A final liveness round runs after the hold.
Logical-session close and Serve completion must finish within 30
seconds, and stopping both Edges must return the fixed ten Coturn allocations
to zero within 15 seconds. Source-qualified Coturn counters are sampled once
per second during every workload, and any live allocation count other than ten
fails the relay qualification. Edge containers receive a 45-second stop grace
and their entrypoint forwards SIGTERM for up to 40 seconds, so the production
30-second Gateway drain can close its physical upstream pool. The separate
15-second Coturn-zero bound starts only after both Edges stop.

The 1,000-session soak entrypoint is intentionally sequential rather than a
replacement workload. It first runs the same three burst repetitions, verifies
that the repository head stayed clean and unchanged, then starts one fresh
zero-ramp 1,000-session stack for a 60-minute hold. Liveness rounds start every
30 seconds. The artifact keeps the existing `speed_test` as the initial
checkpoint and adds a distinct `final_speed_test` plus `speed_retention`.
Initial and final concurrent upload/download each transfer exactly 1,000 MiB
(1,048,576,000 bytes) at no less than 200 Mbps, and each final direction
retains at least 80% of its
initial aggregate and per-session p01, p05, and p50 throughput. The lower-tail
percentiles catch slow-session degradation; p95 and p99 remain upper-tail
diagnostics and are not retention gates.

Extended artifact version 15 records actual hold boundaries and qualifies the
first and last ten-minute windows. Median round p99 RTT, RSS, open FDs,
completed-GC Go live heap, and goroutine values may grow by at most 20%. Current
Go heap-object bytes remain diagnostic and are not gated because they vary with
the normal GC cycle; the sampler does not force a GC. CPU and network-rate
comparisons apply the same relative limit with 0.10-core and 1,024-byte/s
absolute noise floors; UDP and UDP6 socket medians may grow by at most 20%.
RSS, CPU, and open-FD samples identify one process and start time. The Docker
roles' UDP counts and network counters come from `/proc/<pid>/net`, which is
container-network-namespace evidence rather than a process-only counter.
Source-qualified samples cover the load driver, both Edge roles, both Coturn
roles, and Server once per second, with a maximum accepted gap of 2.1 seconds.
Cumulative CPU and network counters cannot decrease. Unsupported external Go
runtime fields and load-driver namespace socket/network fields are enumerated
explicitly. Any failed initial gate prevents the hold from starting, and
cancellation still performs bounded session and Docker cleanup.

The 100- and 500-session burst runners preserve their accepted payloads and
gates but now use that relay-only upstream topology. Existing workload fields
remain stable. The current version 15 artifact includes optional final-speed
retention, mandatory bounded-cleanup evidence, and the load driver's effective
`GOGC`; the 100- and 500-session entrypoints explicitly retain `GOGC=100`. A
sibling `*-coturn.json`
artifact records each Coturn
member's one-second live allocation and traffic samples, finished-session byte
counters, traffic delta, and the bounded return to zero after both Edge
processes stop. Each member uses one persistent container-side metric stream so
host-side Docker process startup is not part of every sample. It is accepted
only after a pre-workload sample and a non-decreasing millisecond timeline
with no gap above 2.1 seconds. Equal timestamps are accepted only when distinct
nanosecond samples truncate to the same millisecond. The merged
#697/#698 results remain historical direct-upstream observations; current
Coturn measurements are not a production, WAN, or portable throughput SLA.

The standard Docker `turn` role uses the same pinned Coturn image with TURN
REST authentication, a private-container/public-host IPv4 mapping, and a
one-to-one published UDP relay range. Run `run_turn_relay_tests.sh` to verify
that authoritative ServerInfo temporary credentials create a relay-only
Server connection, carry a product Ping, advance Coturn traffic counters, and
clean up. A corrupted client credential must fail without forming the two-sided
allocation pair; the authoritative Server can still create its own valid
one-sided allocation while answering signaling, which project teardown removes.
This focused product evidence does not test the optional embedded Pion TURN
runtime in `pkgs/gizedge`.

When the Docker host exposes container addresses, the capacity script passes
each Edge container's direct endpoint together with the explicit local
`-signaling-base-from-edge` override. The gateway runner otherwise retains the
advertised `transport.endpoint` contract. This avoids published-port proxy
backlog as a load-generator artifact without changing non-local discovery
behavior. The script prints the selected endpoint boundary and falls back to
the published endpoint when direct access is unavailable. WebRTC/ICE still uses
the configured gateway candidates; the Dial barrier and workload are not paced,
batched, or preconnected.

## GenX provider E2E

Provider-backed transformer coverage uses one complete credential inventory:

```sh
cp tests/genx-e2e/.env.example tests/genx-e2e/.env
bash tests/genx-e2e/run_tests.sh
```

Provider-free Match parity and deterministic duplex behavior remain ordinary
tests and run under `go test ./...`.

## Giznet E2E

`tests/giznet-e2e` exercises the public Giznet transport through gizwebrtc:

```sh
go test -tags giznet_e2e ./tests/giznet-e2e/...
go test -tags giznet_e2e ./tests/giznet-e2e/webrtc \
  -run '^$' -bench BenchmarkWebRTCHTTPRoundTrip -benchtime=1x
bash tests/giznet-e2e/run_coturn_tests.sh
```

The ordinary `giznet_e2e` lane keeps its in-process Pion TURN regression and
requires no Docker. The fixed Coturn runner adds the stricter
`giznet_e2e,giznet_coturn_e2e` selection and starts only static-auth and TURN
REST Coturn roles—never GizClaw Server or Edge. It verifies relay-only packet
and service streams, invalid credentials, allocation cleanup, and finished
traffic counters through public Giznet APIs. It also writes an ignored JSON
artifact with 30 direct/static/REST dials, 200 64-byte stream RTT samples, and
three fresh 32 MiB transfers per path and direction, including raw samples,
phase percentiles, direct-versus-relay ratios, repository state, Docker engine,
and the exact pinned Coturn image. These are local transport diagnostics, not
GizClaw gateway or production performance evidence.

### Edge direct-versus-Coturn capacity

The GizClaw-owned Edge topology has one canonical local qualification command:

```sh
bash tests/gizclaw-e2e/run_gateway_relay_capacity_tests.sh
```

It requires a clean repository and the Docker E2E credential file, builds the
CLI and load driver once, then creates 12 fresh projects: direct and
relay-only Edge upstreams at 100 and 500 sessions, three repetitions each.
Both paths keep the same Server, two Edges, two digest-pinned Coturn members,
fixed subnet, four gateway upstreams per Edge, zero ramp, and 1 MiB upload and
download per session. Direct requires zero Coturn allocations and traffic;
relay requires exactly ten live allocations, traffic growth, and return to
zero after Edge shutdown. It then runs the fixed pure-Giznet direct/Coturn
diagnostic and requires same-head evidence when the product comparison is
material; that diagnostic attributes a delta but never replaces the product
matrix.

Every session also sends 50 non-empty Opus packets at 20 ms cadence through the
unreliable packet lane and completes a following RPC Ping. The ignored run
artifacts contain path proof, timing and throughput, exact packet/byte counts,
role CPU/RSS/FD/socket/network samples, Coturn evidence, and a validated
`comparison.json`. This is bounded one-way transport evidence on one local
Docker host. It does not qualify provider processing, decoded audio, WAN/NAT
diversity, production Coturn/deployment capacity, 1,000-session soak, or the
30,000-session product ceiling.

The 2026-08-04 ARM64 OrbStack reference run (Docker 29.4.0, 16 Docker CPUs,
16.8 GB Docker memory) passed all 12 runs and produced these three-run medians:

| Sessions | Path | Upload | Download | Dial p95 / p99 | RPC RTT p99 |
| --- | --- | ---: | ---: | ---: | ---: |
| 100 | direct | 654 Mbps | 578 Mbps | 458 / 472 ms | 18 ms |
| 100 | Coturn | 416 Mbps | 568 Mbps | 452 / 460 ms | 19 ms |
| 500 | direct | 476 Mbps | 612 Mbps | 714 / 1,120 ms | 287 ms |
| 500 | Coturn | 417 Mbps | 606 Mbps | 778 / 819 ms | 503 ms |

Relay/direct upload ratios were 0.636 and 0.876; download ratios were 0.981
and 0.990. The upload and 500-session RTT differences are material, while all
fixed gates, exact reliable bytes, Opus packets, path selection, allocation,
and cleanup checks passed. A same-head pure-Giznet diagnostic, which excludes
the product Edge and Server, measured direct at 818/798 Mbps and REST Coturn at
488/526 Mbps with about 220/219 MB added to Coturn receive/send counters. This
locates the measured boundary at the local Coturn relay path rather than a
GizClaw Edge/Server capacity limit. It is not evidence about production Coturn
hosts or WAN behavior.

## LoCoMo Memory Evaluation

`tests/locomo-e2e` is a GizClaw-owned manual evaluation of production
`memory.Store` implementations. It does not use Flowcraft's evaluator and is
not part of ordinary `go test ./...`, Docker E2E, or required CI. Each live Go
test owns its complete provider, memory-lane, and extraction configuration.
Remote project configuration remains deployment state: the harness neither
mutates it nor presents one endpoint/project as multiple lanes.

Current lanes cover Flowcraft BBH BM25 single-pass, hybrid single/two-pass,
Mem0 Platform default/custom-instructions, and Volc AgentKit Memory default.
The full entrypoint runs every lane:

```sh
cp tests/locomo-e2e/.env.example tests/locomo-e2e/.env
bash tests/locomo-e2e/run_tests.sh
```

Shorter fixed selections are
`run_flowcraft_bm25_tests.sh`, `run_flowcraft_hybrid_tests.sh`,
`run_mem0_tests.sh`, and `run_volc_agentkit_tests.sh` in the same directory.
They all require the same complete LoCoMo `.env`; dataset, report, timeout,
model, endpoint, project, and threshold settings are explicit non-secret
runtime parameters or committed defaults, not credential-file fields.

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

## Memory provider E2E

The three live-model Memory cases use the `gizclaw_memory_e2e` build tag and one
fixed entrypoint:

```sh
cp tests/memory/.env.example tests/memory/.env
bash tests/memory/run_tests.sh
```

Ordinary Memory tests remain credential-free and run under `go test ./...`.
