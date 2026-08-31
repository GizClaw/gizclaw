# Testing and E2E

This page documents repository-level test harnesses. Ordinary Go unit tests
still run according to the changed scope. Suites that require a build tag,
Docker, live providers, or human judgment must be started explicitly and must
not be reported as passing when they were not run.

## Store E2E

`tests/store-e2e` verifies Redis 7.0, PostgreSQL, and ClickHouse through exported Store APIs
without production-package-private test hooks. Every Go file in the directory
uses the `store_e2e` build tag, so ordinary `go test ./...` neither selects these
tests nor contacts an external database. Fast SQLite integration stays beside
the corresponding package unit tests in a normal `*_test.go` file.

Redis, PostgreSQL, and ClickHouse tests use `TestRedis...`, `TestPostgreSQL...`,
and `TestClickHouse...` names respectively. CI selects only the backend provisioned by the current job;
a selected backend fails rather than skips when its DSN is absent:

```sh
GIZCLAW_TEST_REDIS_DSN='redis://127.0.0.1:6379/15' \
  go test -tags=store_e2e -count=1 -p 1 -run '^TestRedis' ./tests/store-e2e
GIZCLAW_TEST_POSTGRES_DSN='postgres://…' \
  go test -tags=store_e2e -count=1 -p 1 -run '^TestPostgreSQL' ./tests/store-e2e
GIZCLAW_TEST_CLICKHOUSE_DSN='clickhouse://…' \
  go test -tags=store_e2e -count=1 -p 1 -run '^TestClickHouse' ./tests/store-e2e
```

Every test uses an isolated table name and performs best-effort cleanup. Errors,
logs, and CI output must not print DSNs, database credentials, or Store payloads.

The Redis gate uses two independent clients against one database and verifies cross-client visibility, ordering, expiration, batches, conditional-create races, compare-and-mutate races, prefix isolation, and connector lifecycle. The selected endpoint must be single-node Redis 7.0; Redis Cluster is outside the Store contract.

The credential-free multi-Server Docker gate is:

```sh
bash tests/gizclaw-e2e/run_multi_server_tests.sh
```

It runs Redis 7.0, two Servers with distinct local runtime state, and two Edges whose configured Server order is reversed. It verifies fixed Peer homes through both Edges, foreign-Server rejection, local-only PeerRun writes, and side-effect-free cross-Server Social conflicts. It does not test Workspace routing.

### Cloud ObjectStore conformance

The same tagged package contains `TestObjectStore`. Select exactly one
pre-existing test bucket or container with `GIZCLAW_OBJECTSTORE_PROVIDER` set to
`volc-tos`, `aliyun-oss`, `gcs`, or `azure-blob`:

```sh
GIZCLAW_OBJECTSTORE_PROVIDER=volc-tos \
GIZCLAW_TOS_ENDPOINT=https://tos-cn-beijing.volces.com \
GIZCLAW_TOS_REGION=cn-beijing GIZCLAW_TOS_BUCKET=... \
GIZCLAW_TOS_ACCESS_KEY_ID=... GIZCLAW_TOS_ACCESS_KEY_SECRET=... \
  go test -tags=store_e2e -count=1 -run '^TestObjectStore$' ./tests/store-e2e

GIZCLAW_OBJECTSTORE_PROVIDER=aliyun-oss \
GIZCLAW_OSS_ENDPOINT=https://oss-cn-hangzhou.aliyuncs.com \
GIZCLAW_OSS_BUCKET=... GIZCLAW_OSS_ACCESS_KEY_ID=... \
GIZCLAW_OSS_ACCESS_KEY_SECRET=... \
  go test -tags=store_e2e -count=1 -run '^TestObjectStore$' ./tests/store-e2e

GIZCLAW_OBJECTSTORE_PROVIDER=gcs GIZCLAW_GCS_BUCKET=... \
GOOGLE_APPLICATION_CREDENTIALS=/secure/credentials.json \
  go test -tags=store_e2e -count=1 -run '^TestObjectStore$' ./tests/store-e2e

GIZCLAW_OBJECTSTORE_PROVIDER=azure-blob \
GIZCLAW_AZURE_BLOB_ACCOUNT_URL=https://example.blob.core.windows.net \
GIZCLAW_AZURE_BLOB_CONTAINER=... \
  go test -tags=store_e2e -count=1 -run '^TestObjectStore$' ./tests/store-e2e
```

TOS may additionally use `GIZCLAW_TOS_SESSION_TOKEN`; OSS may use
`GIZCLAW_OSS_SECURITY_TOKEN`. Azure identity comes from the standard
`DefaultAzureCredential` environment or managed identity chain. Each run uses a
generated `gizclaw-e2e/` logical prefix and verifies cleanup leaves no residue.
Never print or commit credential values. When an account is unavailable, record
that provider as `SKIP` and retain the interoperability risk; a tagged compile
without a live account is not passing live evidence.

## Credential-backed harness contract

GizClaw, GenX, and Memory live suites each own one ignored `.env`,
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
as live-suite evidence for those script-owned harnesses. LoCoMo is the
exception described below: its Go test names are the supported selectors.

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
├── giztest/     # declarative Peer RPC, Workflow, and benchmark scenarios
├── go/          # focused Admin, delete, Edge, and OpenAI tests
└── js/          # JavaScript/TypeScript WebRTC tests
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

Managed-deletion changes use a fixed production vertical-slice entrypoint. It
validates the shared credential file, starts an isolated Docker stack, and runs
the dedicated Peer RPC deletion package for Pet, Workspace, Friend Group, and
Peer resources. The suite covers active-use termination and Peer tombstone
survival across a Server restart, then cleans the project after success or
failure without running unrelated provider-backed scenarios:

```bash
bash tests/gizclaw-e2e/run_pending_deletion_tests.sh
```

The full gate installs locked Node workspaces, initializes nanopb, builds the
E2E CLI, starts Compose, waits for Server and Edge, runs JS, C/cgo, Go
Admin/OpenAI, CLI, and Giztest phases in order, and performs one bounded
cleanup. The total deadline defaults to 90 minutes. Per-phase defaults
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

Setup selects random free Edge and Admin host ports. Each Edge host port is
available for both TCP and UDP and maps both protocols to container port
`9821`; it does not have a separate gateway endpoint or UDP port. Firmware or
LAN clients need an explicitly reachable address:

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

`GIZCLAW_E2E_EDGE_ENDPOINT` is the client-facing HTTP/signaling and WebRTC ICE
endpoint, and
`GIZCLAW_E2E_SERVER_ENDPOINT` is host-Admin-facing. The remaining generated
variables provide the CLI config home, identity home, and Compose
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
- `go/delete` retains deletion checks that require Admin observation, restart, and tombstones.
- `go/edge` retains TURN relay, sibling-close, failure recovery, and network diagnostics.
- `go/openai` retains typed SDK coverage of the OpenAI-compatible API.
- `giztest/*.giztest.yaml` covers Peer RPC, conversation, social, gameplay, and Workflow behavior.
- `cmd` executes `testdata/bin/gizclaw` with `os/exec`; it must not bypass the CLI with `go run` or typed clients.
- `js/admin` covers WebRTC Admin fetch; `js/rpc` covers peer and server-initiated RPC.

### Giztest scenarios

Each Giztest file is an independent user story. It creates its own mutable
Peers, Workspaces, invites, groups, and related resources and removes them in
`finally`. Device identities are generated per task; files do not share fixed
device keys or outputs. `clients` may keep several devices connected within one
task.

`repeat` only expands a file into tasks. CLI `--parallel` is the sole global
parallelism control. Files prefixed with `benchmark.` own repeat, barrier,
concurrency, or latency measurements. Recursive directory runs use one fixed
worker pool and write every task and cleanup result to a redacted JSON report:

```sh
gizclaw test validate -f tests/gizclaw-e2e/giztest
gizclaw test run tests/gizclaw-e2e/giztest --parallel 10 \
  --output tests/gizclaw-e2e/testdata/giztest-report.json
```

Giztest never performs Admin Apply. Standard Docker setup applies all fixtures,
including the dedicated RuntimeProfile and run-scoped registration token, once
before JavaScript, C/cgo, Go, CLI, or Giztest starts. A pre-provisioned remote
target may instead provide `GIZCLAW_TEST_ENDPOINT` and
`GIZCLAW_TEST_REGISTRATION_TOKEN` directly.

### Eino first-response qualification

Run the provider-backed Eino release gate from a clean tracked worktree:

```sh
bash tests/gizclaw-e2e/run_eino_first_response_tests.sh
```

The runner builds one CLI revision, starts one isolated Server/Edge stack, and
runs the same ten-task text-only, configured-ASR Push-to-Talk, and Realtime
documents through Server and Edge at `--parallel 1` and `--parallel 8`.
Voice-enabled cases record a non-empty transcript within 700 milliseconds as
the ASR transport qualification, then continue to enforce the separately owned
end-to-end release gate of assistant text within 2 seconds and assistant audio
within 3 seconds, all from the completed-input origin. Passing the transcript
qualification does not make a red end-to-end result pass. Separate Push-to-Talk
and Realtime terminal roundtrips verify text/audio EOS, and all scenarios
require their three cleanup steps.
The atomic `testdata/eino-first-response/manifest.json` records the Git revision,
dirty state, fixture hashes, redacted report hashes, task and cleanup counts,
and maximum observed transcript/text/audio timings without recording endpoint or
credential values. A dirty exploratory run must opt in with
`GIZCLAW_E2E_ALLOW_DIRTY=1`; it is not release evidence.

The matrix qualifies the exact RuntimeProfile resources applied by the stack.
A warm follow-up pass cannot replace a failed sample, and changing the chat
Model, upstream revision, tenant, endpoint, ASR Model, or Voice invalidates the
previous receipt. GizClaw does not add retries or automatic Provider fallback
to make this gate pass.

Audio and binary values remain in bounded memory with declared `media_type`,
`codec`, and `max_bytes`. `save_as` assigns a variable and never writes a file.
`speech.cache: run` is limited to saved synthesis steps. It caches one successful
immutable input fixture per document, step, and resolved request for the current
CLI invocation, then gives every repeated task its own byte copy. This preserves
isolated task variables without turning input-fixture TTS capacity into the
Workflow concurrency target.

For `server.speech.transcribe`, Giztest derives the upload `content_type` from
the typed audio variable and the runner's conversion. Ogg/Opus is decoded to
16 kHz mono PCM; matching `pcm_s16le` input passes through with that same wire
type. Other audio formats fail before the RPC opens. The document does not own
this wire metadata.

`peer_stream.terminal_label` defaults to `assistant`; that completion requires
observed text and audio EOS boundaries. Chatroom turns that complete on the
persisted user transcript declare `transcript` explicitly.
`peer_stream.completion: first_response` is the bounded deployment-probe
alternative. `require_text` and `require_audio` select its required modalities
and both default to true. Every required modality needs its corresponding
positive `first_text_timeout` or `first_audio_timeout` Go duration; a disabled
modality omits its deadline, and at least one modality must remain required.
The deadlines start only after the whole turn input has been pushed. The runner
succeeds and closes that logical stream as soon as it has observed the first
non-empty assistant chunk for every required modality; it does not wait for
either EOS. A missing required modality fails with
`deadline=first_text_timeout` or `deadline=first_audio_timeout`. This completion
cannot be combined with `interrupt_after`, `terminal_label`, or
`wait_for_history`.
`peer_stream.idle_timeout` (Go duration, optional) bounds inactivity instead of
total length: the runner arms the timer after the turn input is pushed, resets
it on every received chunk regardless of label, re-arms it after an
`interrupt_after` replacement turn, and stops it once the terminal EOS is
accepted. A stall fails the step with `peer_stream idle timeout exceeded`,
while a long reply that keeps streaming passes. The step `timeout` and the
document `timeout` remain absolute bounds; when both kinds are set the earlier
expiry wins. `peer_stream` evidence always carries `events` and
`last_event_ms`, adds `idle_timeout_ms` when the field is set, and on failure
names the bound that fired as `deadline` (`idle_timeout`,
`first_text_timeout`, `first_audio_timeout`, `timeout`, or `cancelled`). Failed
steps keep the evidence their operation returned next to
`error`, so reports distinguish a stall from an over-long reply. `gizclaw test
validate` rejects unparsable or non-positive `idle_timeout` values.
Interactive `review` files must run alone in an attached terminal with
`--parallel 1`.

A continuous realtime-route regression can retain the same logical
`gizcli.PeerStream` with a task-scoped `peer_stream.session`. The first
`mode: realtime` step sets both `session` and `keep_open: true`; a later
realtime step for the same client uses that `session` with
`await_rearm: INPUT_ROUTE_RELOADED`. The latter first consumes the exact
retryable user-audio EOS for the old route, sends a fresh BOS, and only then
sends its declared audio input. A session can be created once and consumed
once. Unknown, duplicate, already-consumed, cross-client, or cross-task
sessions fail before input is sent.

Persistent sessions cannot be combined with `retry`, `interrupt_after`, or
`finally`. A re-arm step closes the consumed session after success unless it
also sets `keep_open: true` to retain it for another re-arm. On task success,
failure, timeout, or cancellation, the runner closes every unconsumed session
before RPC finalizers. Reports record only boolean evidence such as
`session_connection_reused`, `reload_eos_observed`, `replacement_bos_sent`,
and `stream_id_changed`; they do not record raw stream IDs, audio payloads, or
transcripts.

The corresponding JavaScript Client WebRTC integration regression runs in the
`audio-reload` mode of
`tests/gizclaw-e2e/js/streams/peer_conversation_lifecycle_e2e.test.ts`. It
activates the SDK's `createContinuousAudioRouteRearm` owner on the same Event
channel. The owner installs and removes its own Event subscription; Client code
does not dispatch EOS events into it. It keeps the realtime user-audio route and
uplink track active, consumes the `INPUT_ROUTE_RELOADED` EOS, sends a fresh BOS
after reload, and reports the replacement stream ID to the Client. The test then
sends a second audio segment and waits for a response with the same microphone
source, track, Event channel, and PeerConnection. The test must not frame the
replacement BOS itself; doing so would verify only the Server protocol, not
JavaScript Client recovery.
When TTS is outside the behavior under test,
`GIZCLAW_E2E_INPUT_PCM_PATH` may select a pre-decoded, non-empty mono signed
16-bit `.pcm` fixture below `tests/gizclaw-e2e/testdata/pcm/`. The resolved
regular file is limited to 16 MiB. `GIZCLAW_E2E_INPUT_PCM_SAMPLE_RATE` defaults
to 16000 and must be a positive integer divisible by 100. This bypasses only
input-fixture synthesis, not the realtime WebRTC route or its post-reload
response checks.

When `peer_stream` receives assistant Opus, its result and redacted evidence
expose receiver-side pacing under `audio_pacing`: `packets`, `audio_ms`,
`target_span_ms`, `receive_span_ms`, `mean_packet_ms`, `mean_interval_ms`,
`p95_interval_ms`, `max_interval_ms`, `drift_ms`, `absolute_drift_ms`, and
`buffer_surplus_ms`. Intervals use the stream reader's monotonic receipt time,
before assertions, persistence, or PortAudio playback; a positive
`buffer_surplus_ms` means network delivery is ahead of the Opus media clock.
All `*_ms` values use milliseconds. `target_span_ms` is the sum of every packet
duration except the last, `drift_ms = receive_span_ms - target_span_ms`, and
`buffer_surplus_ms = -drift_ms`. P95 uses nearest-rank selection over arrival
gaps. With one packet, only `packets` and `audio_ms` are present; with no
assistant Opus, `audio_pacing` is absent.
Giztest documents assert these paths through ordinary numeric `expect`
constraints rather than a separate pacing schema.
`flowcraft-voice-assistant.push-to-talk-roundtrip.giztest.yaml` and
`doubao-realtime-conversation.realtime-roundtrip.giztest.yaml` require 20 ms
Opus frames, a mean interval from 12 through 21 ms, P95 no greater than 30 ms,
maximum interval no greater than 100 ms, at least 101 packets, and final buffer
surplus from 450 through 550 ms. The two cases cover push-to-talk and realtime
delivery respectively. Those ranges permit bounded recovery around the 500 ms
target without demanding an unrealistic exact 20 ms arrival for every network
packet.

`workspace_relay` connects two selected Workspaces in one task as one bounded
conversation: the tester Workflow owns test intent, generated user behavior,
semantic evaluation, and its final verdict, while Giztest owns transport,
framing, `max_turns` and fixed byte/event bounds, attribution, failure stages,
and cleanup. Forwarding is streaming — the first eligible text fragment or
arrival-paced Opus packet reaches the receiving Workspace before the source
response completes, with a receiving-side stream ID and user role — and the
terminal response is captured without being forwarded. Reports keep per-client
turn counts, `{min, max}` latency/size aggregates, and the terminal side.
`terminal_media` may explicitly separate a text-forwarded turn from its Opus
audio EOS boundary. `idle_timeout` bounds inactivity per active turn, resets on
active-side progress, and records deadline, client, turn, last-event, and
observed-media evidence when it fires. Audio relays retain bounded assistant
text for assertion and terminal capture without forwarding duplicate text.
Reports remain content-free by default; local `--evidence full --output <path>`
adds bounded relay text and produces a sensitive artifact without adding inputs,
credentials, IDs, or audio payloads.
`workspace-relay.workflow-tester.giztest.yaml` runs the live candidate/tester
pair inside the standard gate;
`workspace-relay.doubao-realtime-workflow-tester.giztest.yaml` proves text
forwarding with audio EOS completion against a multimodal candidate; and
`run_workspace_relay_tests.sh` starts one isolated stack, runs both repeat-1
gates and the repeat-20 relay gate
(`benchmark.workspace-relay.workflow-tester-20.giztest.yaml` with
`--parallel 20`), and always cleans the stack up.

### Ten- and twenty-lane Workflow concurrency and interruption

The fixed entrypoint selects ten explicit `benchmark.*-10|-20.giztest.yaml`
files. Each file's `repeat` creates 10 or 20 independent Peers and Workspaces;
tasks wait at that file's barrier, and CLI `--parallel` is the only concurrency
control:

```sh
bash tests/gizclaw-e2e/run_workflow_concurrency_10_tests.sh
bash tests/gizclaw-e2e/run_workflow_concurrency_20_tests.sh
```

Each fixed entrypoint selects ten required files covering ordinary and
interruption scenarios for Realtime, Realtime Duplex, Flowcraft, Eino, and
Translate. The 10-lane gate must pass before the 20-lane gate on the same
repository head. Repeats within one file share one barrier, while one global
worker pool schedules tasks from every selected file. Reports therefore retain
document and repeat ownership rather than presenting the total task count as
one Workflow's concurrency. Every task keeps its own physical connection,
Workspace runtime, and PeerStream until terminal output and cleanup complete.
Voice-input benchmarks cache the immutable synthesized input in memory and place
their barrier after Workspace and input preparation, so the measured wave starts
10 or 20 ready PeerStreams together rather than concurrently load-testing TTS.
Redacted task/step evidence, container resource samples, and 20-lane runtime
profiles are stored under ignored `testdata/workflow-concurrency/`.

Each entrypoint validates the complete `.env` before Docker setup. Environment
variables cannot change coverage or concurrency, and retry, provider fallback,
or replacement sessions cannot manufacture a pass. A terminal failure becomes a
provider-only `SKIP` only when every cause is a complete structured Volcengine
error with a `4xxxxxxx` or `5xxxxxxx` code. Transformer-owned provider-completion
guards still fail the test. Any local protocol,
deadline, setup, runtime, or cleanup error mixed into the wave still fails it;
the provider failure artifact remains available. The 20-lane entrypoint also
enables runtime profiling and succeeds only when a complete non-empty
`manifest.json` is collected. Resource samples and profiles support diagnosis;
they are not proof of leak freedom, a provider SLA, or production capacity.

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
bash tests/gizclaw-e2e/run_observability_tests.sh

GIZCLAW_E2E_VOLC_LOG_ENDPOINT=... \
GIZCLAW_E2E_VOLC_LOG_REGION=... \
GIZCLAW_E2E_VOLC_LOG_TOPIC_ID=... \
  bash tests/gizclaw-e2e/run_volc_log_tests.sh
```

The observability entrypoint sends one real AI text turn through Edge to the
Server. Server and Edge `giz_webrtc_*`/`giz_edge_webrtc_*` samples go to an
E2E-only Prometheus Remote Write protocol fixture, while Server system logs go
to an isolated SQLite `log.immutable` Store. Acceptance queries the fixture for
the core metric families and `node_role=application|edge`, then uses Admin Log
Query to retrieve the user input, the AI response actually delivered, and the
`turn_started`, `agent_input_first_push`, `output_first_event`, and
`turn_terminal` lifecycle stages under one
`(peer_public_key, tunnel_session_id, turn_index)`. The default Docker E2E
memory metrics Store and stderr log behavior remain unchanged.

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
upstreams. The load driver uses `GOGC=200` and records that value with
`GOMAXPROCS` in the artifact. This measured harness setting keeps collection
of the roughly 2 GiB client heap from becoming the limiting stage during the
synchronized transfer; current process CPU and completed-GC live-heap evidence
still gate long-lived stability. It does not change production processes,
pacing, timeouts, or the release barrier. Each
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
30 seconds. The runner prints a hold heartbeat at least every 30 seconds and at
the start and end of each liveness round. Each line reports established and
active sessions, cumulative and per-round ping results, unexpected disconnects,
open FDs, RSS, goroutines, and the minimum sample count, largest historical gap,
and largest current sample age across Docker roles. A stalled sample stream or
a historical gap above 2.1 seconds also fails immediately. Any excess ping
failure, unexpected disconnect, identity crossover, or overlong ping round makes
the zero-failure qualification irrecoverable, so the runner performs bounded
cleanup instead of waiting for the hold deadline. Every speed run prints progress
at start, completion, and every 15 seconds while active, so missing output is not
treated as healthy execution. The artifact keeps the existing
`speed_test` as the initial checkpoint and adds a distinct `final_speed_test`
plus `speed_retention`.
Initial and final concurrent upload/download each transfer exactly 1,000 MiB
(1,048,576,000 bytes) at no less than 200 Mbps, and each final direction
retains at least 80% of its
initial aggregate and per-session p01, p05, and p50 throughput. The lower-tail
percentiles catch slow-session degradation; p95 and p99 remain upper-tail
diagnostics and are not retention gates.
Fresh-stack HTTP and ready-file waits likewise print the service state and
elapsed time every 15 seconds; silence after Compose startup is not readiness
evidence.
Within one ordered 1,000-session qualification, the runner builds one
run-ID-scoped service image from the required clean head and reuses that exact
image for later repetitions. Containers, networks, volumes, ports, and runtime
credentials remain fresh for every repetition. A failed attempt retains only
the clean-HEAD-scoped image so a retry on that same head avoids another build;
a changed head uses a different tag, and a completed qualification removes its
exact image. After every fresh stack reaches readiness, including the
one that performed the initial image build, a 120-second stabilization window
prints 15-second container-health heartbeats before measurement. Repetitions
that reuse the image follow the same window.
After each 1,000-session fresh stack is removed, a fixed 120-second stabilization
window reports its remaining time every 15 seconds so delayed Docker-VM resource
reclamation is not charged to the next capacity measurement. A failed upload
gate skips download because the run can no longer qualify.

Extended artifact version 18 records actual hold boundaries and qualifies the
first and last ten-minute windows. Median round p99 RTT, RSS, open FDs,
completed-GC Go live heap, and goroutine values may grow by at most 20%. Current
Go heap-object bytes remain diagnostic and are not gated because they vary with
the normal GC cycle; the sampler does not force a GC. CPU and network-rate
comparisons apply the same relative limit with 0.10-core and 1,024-byte/s
absolute noise floors; UDP and UDP6 socket medians may grow by at most 20%.
RSS, CPU, and open-FD samples identify one process and start time. The Docker
roles' UDP counts and network counters come from `/proc/<pid>/net`, which is
container-network-namespace evidence rather than a process-only counter.
The load driver's Darwin/Linux CPU counter is cumulative process user plus
system CPU from `getrusage`; other platforms retain an explicitly named
Go-runtime active-CPU fallback.
Source-qualified samples cover the load driver, both Edge roles, both Coturn
roles, and Server once per second, with a maximum accepted gap of 2.1 seconds.
Cumulative CPU and network counters cannot decrease. Unsupported external Go
runtime fields and load-driver namespace socket/network fields are enumerated
explicitly. Any failed initial gate prevents the hold from starting, and
cancellation still performs bounded session and Docker cleanup.

The 100- and 500-session burst runners preserve their accepted payloads and
gates but now use that relay-only upstream topology. Existing workload fields
remain stable. The current version 18 artifact includes optional final-speed
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

The authoritative 2026-08-07 qualification ran
`run_gateway_capacity_1000_soak_tests.sh` once on clean executable head
`a2ff5b791a5c60c3b80052204717ac277e43c885`. The host was Darwin/arm64 with 16
logical CPUs, Go 1.26.4, and 64 GiB RAM; the isolated service image ran on
OrbStack 2.2.1 Linux/aarch64 Docker with 16 logical CPUs and 15.67 GiB RAM.
The three prerequisite fresh-stack bursts each established 1,000/1,000
sessions, exactly 500 per Edge, with zero failures. Their establishment rates
were 159.90, 1,118.18, and 158.99 sessions/s; Dial p95/p99 values were
681.57/776.75 ms, 749.00/806.92 ms, and 589.81/669.13 ms; synchronized
upload/download rates were 453.54/482.89, 415.54/455.50, and 484.35/413.58
Mbps. Each direction transferred exactly 1,000 MiB, all ten relay allocations
remained live, and bounded session and Coturn cleanup passed.

The fresh soak then established 1,000/1,000 sessions at 1,074.63 sessions/s,
with Dial p95/p99 of 718.53/838.54 ms. All 122,000 accepted Pings completed over
60 minutes with zero Ping failures, disconnects, identity crossovers, exits, or
restarts; aggregate RTT p99 was 474.93 ms. Initial upload/download were
415.51/425.25 Mbps and final upload/download were 424.20/524.18 Mbps. Final
aggregate retention was 102.09%/123.26%; the lowest accepted per-session
p01/p05/p50 retention was 96.66%, so every throughput gate passed.

The late median round-p99 RTT fell by 11.11%. Late-window RSS growth was 10.89%
and 16.49% for the two Edges, -52.64% for the load driver, -0.65% for Server,
and about -2.78% for both Coturn members. The load driver's completed-GC live
heap grew 10.98%; its FD and goroutine medians were unchanged. All six roles
passed their supported RSS, CPU, FD, heap, goroutine, UDP/UDP6, and network-rate
gates. Each role supplied at least 3,679 one-second samples with a maximum gap
of 1.033 seconds. Both Edges remained relay-only, the Coturn sidecar recorded
2,414,392,388 received and 2,381,483,034 sent bytes, logical-session cleanup
completed in 45.55 ms with no close failure, and both Coturn members returned
from five to zero allocations within the 15-second bound. Documentation-only
commits after this result do not change the qualified executable.

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
the Edge endpoint candidates on that same external port; the Dial barrier and workload are not paced,
batched, or preconnected.

## GenX provider E2E

Provider-backed transformer coverage uses one complete credential inventory:

```sh
cp tests/genx-e2e/.env.example tests/genx-e2e/.env
bash tests/genx-e2e/run_tests.sh
```

The MiniMax API key must be paired with the voice base URL for the same region;
the runner does not substitute a default region when
`GIZCLAW_GENX_E2E_MINIMAX_BASE_URL` is missing.

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

It requires a clean repository and the Docker E2E credential file. The CLI is
CGO-built once in the Linux Go base image matching Docker's native architecture,
and the load driver is built once on the host. The capacity image only copies
that Linux CLI, the entrypoints, and required configuration; it does not install
npm dependencies, download Go modules, or compile again when Server and Edge
containers start. The command then creates 12 fresh projects: direct and
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
Volc remote project configuration remains deployment state and the harness
does not mutate it.

Current lanes cover Flowcraft Redis 8 BM25 single-pass, hybrid single/two-pass,
self-hosted Mem0, Mem0 Platform default/custom-instructions, and Volc AgentKit
Memory default. LoCoMo is a tagged Go test package. The Docker runner starts the
pinned Redis 8 service, self-hosted Mem0, or both for the selected group, runs
the tagged Go tests from the host against those containers, and always removes
their containers and volumes. The Mem0 Platform and Volc groups continue to use
standard `go test -run` against their remote providers.
Selected tests validate only the environment variables they consume. Missing
or placeholder values fail, and unselected backend variables are not inspected:

```sh
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run '^TestLoCoMoVolcAgentKit' ./tests/locomo-e2e
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run '^TestLoCoMoMem0Platform' ./tests/locomo-e2e
tests/locomo-e2e/run_docker.sh mem0
tests/locomo-e2e/run_docker.sh flowcraft
tests/locomo-e2e/run_docker.sh all
```

Use `.env.example` as a variable inventory and inject values through the
process environment; the test package and runner do not read `.env` files.
The Mem0 group uses the same extraction and embedding model/key/base-URL
environment variables as Flowcraft. Its container pins `mem0ai 2.0.3` and
defaults to the domestic `deepseek-v4-flash` extractor/answer model
and `qwen3.7-text-embedding` with 1024 dimensions. Select the LLM adapter with
`GIZCLAW_LOCOMO_E2E_MODEL_PROVIDER`; supported values are `deepseek` and
`bytedance`. Keep `GIZCLAW_LOCOMO_E2E_EMBEDDING_DIMENSIONS` aligned with the
selected embedding service because Mem0 must create its Qdrant collection with
the exact vector width.
Run a remote Mem0 Platform lane separately only when its endpoint, API key, and
configuration fingerprint are available; those credentials are not required by
the Docker groups. Direct Go test runs require the matching `GIZCLAW_LOCOMO_E2E_FLOWCRAFT_REDIS8_URL` or
`GIZCLAW_LOCOMO_E2E_MEM0_SELF_HOSTED_URL`; the runner points both at its Docker
services. Override `GIZCLAW_LOCOMO_E2E_REDIS8_PORT` or
`GIZCLAW_LOCOMO_E2E_MEM0_PORT` when the default port is unavailable. Use a
30-minute package timeout and bounded session and
question stages. The runner calls `memory.Store.Observe` by official session,
recalls for every question, asks the configured model to answer, and computes
EM, F1, evidence-hit, and adversarial-rejection metrics locally. Only answerable
questions contribute to EM/F1 and evidence-hit. Category 5 accepts the exact
normalized rejections `unknown`, `not mentioned`, and
`no information available`. The default gate requires aggregate F1 of at least
`0.05`, evidence hit rate of at least `0.50` for evidence-aware
stores, and one materialized fact per selected session. Provider failures and
timeouts remain failures. Ignored `reports/` output contains IDs, scores, and
timings, but no conversation, question, answer, prediction, or recalled text.

### Dataset and license

`testdata/locomo10_smoke.jsonl` is a Git LFS object adapted for noncommercial
use from SNAP Research's LoCoMo `locomo10.json`. It contains the first three
sessions of `conv-30` plus session 1 of `conv-26` (76 turns total) and eight
questions across categories 1 through 5. It is a contract smoke set, not a
full benchmark. Exact
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
  -run 'TestDataset|TestScore|TestAdversarial|TestAggregate|TestPreflight|TestRedaction|TestSession|TestRunBenchmark|TestAwait' \
  ./tests/locomo-e2e
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

## OpenAI Conversations and Responses E2E

The standard GizClaw Docker runner owns a mandatory `go:openai` phase under `tests/gizclaw-e2e/go/openai`. It uses the pinned official OpenAI Go SDK over authenticated `ServicePeerOpenAI`, creates an isolated Peer-owned Conversation Workspace, completes three text turns, composes transcription to Response to speech, exercises background cancel and streamed-client abort followed by same-Conversation recovery, and registers Workspace cleanup before mutation.

Successful runs write redacted monotonic timing evidence below ignored `tests/gizclaw-e2e/testdata/openai-compatibility/`. Artifacts contain only schema/version, target/case, bounded media sizes, numeric phase timings, and status; they must not contain credentials, IDs, prompts, transcripts, generated text, media, URLs, or provider errors. A tagged compile is diagnostic only and does not replace `bash tests/gizclaw-e2e/run_tests.sh`.
