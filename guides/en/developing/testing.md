# Testing and E2E

This page documents repository-level test harnesses. Ordinary Go unit tests
still run according to the changed scope. Suites that require a build tag,
Docker, live providers, or human judgment must be started explicitly and must
not be reported as passing when they were not run.

CI runs only tests that require no online database or live AI service. Database
integration jobs provision temporary PostgreSQL, ClickHouse or PGVector instances;
protocol and SDK E2E use isolated local services and deterministic provider fixtures.
E2E and quality evaluations that need an online database, real model calls or provider
credentials run explicitly on the local host, outside CI, including manual CI dispatch.

E2E entrypoints that build the GizClaw CLI install the locked Node workspaces
and build the embedded console before Go compilation, including container builds.
No manual asset or manifest copy is required; standalone build prerequisites are
documented in [Monitor](monitor).

## PostgreSQL History concurrency and retention

Set `GIZCLAW_TEST_POSTGRES_DSN` to an isolated local PostgreSQL 17 instance configured with `max_connections=300`:

```sh
go test -tags=store_e2e -count=1 -run '^TestPostgreSQLLog' ./tests/store-e2e
go test -count=1 -run '^TestPostgreSQLHistory' ./cmd/internal/server ./pkgs/gizclaw
```

Store regressions cover independent-record progress, same-key conflicts and atomic rollback, concurrent missing-partition creation, UTC day boundaries, expired-key reuse, mutations interleaved with maintenance, and reclamation without writes. `TestPostgreSQLHistoryGiztest` runs real Server/WebRTC, eight independent Workspaces and 96 streams writing eight records each to the same table. A PostgreSQL trigger adds 30ms of work to every record. A deterministic Eino echo Graph removes external model variability; metadata, run state, Workspace and internal Eino History use real PostgreSQL in an isolated schema. The scenario retains the two-second first-text deadline, then completes a dialogue turn to await real EOS and persisted history, and checks all cleanup steps. It does not qualify an external model or first audio.

The optional load uses 128 independent streams, four batches per stream and four records per batch. Save separate evidence directories for different versions and use identical database settings and delay:

```sh
GIZCLAW_TEST_PG_LOG_EVIDENCE="$(mktemp -d)" GIZCLAW_TEST_PG_LOG_DELAY_MS=5 \
  go test -tags=store_e2e -count=1 -v -run '^TestPostgreSQLLogAppendLoad$' ./tests/store-e2e
```

`summary.json` records persisted counts, throughput and Append latency. `locks.json` records `pg_stat_activity`, wait types and blocker PID samples. Set `GIZCLAW_TEST_HISTORY_GIZTEST_EVIDENCE` to retain the Giztest report and runner log. Inputs are synthetic and local; do not commit logs or reports.

The default Docker Giztest stack uses real self-hosted GizClaw Mem0 with pgvector.
Server and Mem0 share one PostgreSQL instance using separate databases and
non-superuser roles. `bash tests/gizclaw-e2e/run_mem0_tests.sh` runs real model
extraction, semantic vector recall, independent policy reload, Peer/Workspace
isolation and cleanup through Server and WebRTC RPC. This real-model regression
is started explicitly on the local host and is not part of GitHub Actions. It
uses the local E2E credential file and does not use Mem0 Cloud quota. Compose
waits for service readiness and cleans its own containers,
volumes and network on exit.

`mem0-self-hosted.batch.giztest.yaml` submits user input and assistant output
through one real Eino `memory_observe` node, asserting two recalled facts before
and after reload and empty recall from another Workspace and Peer. A deterministic
Eino Starlark node produces the answer; Mem0 SDK, embedding and PGVector are real.
The same runner tests a lost committed HTTP reply, six independent adapters
retrying concurrently, payload conflicts, authentication, delete and purge.
Python integration injects failure before the second real candidate write, then
uses three fresh processes to resume against real embedding/PGVector. Final
cleanup checks both the vector collection and observation reservations are empty.

## API key and device identifier limits

`bash tests/gizclaw-e2e/run_resource_limit_tests.sh` builds all four runners and executes the complete resource-limit lane.

`go test ./cmd/internal/server -run 'TestAPIKeyLimitGiztestGo|TestIdentifierLimitGiztestsGo' -count=1` starts a real Server and Edge on temporary state. The shared `server.api_key.limit.giztest.yaml` scenario verifies rejection of the eleventh key with RPC `RESOURCE_EXHAUSTED`/`API_KEY_LIMIT_REACHED` and HTTP `409 API_KEY_LIMIT_REACHED`, unchanged counts after rejection, independent owner quotas, and restored capacity after revocation. Giztest's `expect_error.reason` compares the structured RPC reason exactly; a matching code with a missing or different reason also fails.

The three Go Giztests in `tests/gizclaw-e2e/testdata/identifier-limits/` cover the combined SN/IMEI limit, a full SN reverse index, and a full IMEI reverse index. Devices report identifiers through the real WebRTC SDK. The fixture only resolves the test key's owner and forwards refresh/get over authenticated Admin HTTP; it never creates an error response. Scenarios require the real handler's corresponding 409 error, no identifiers written to the source record, and a still-usable device ping.

Go, JavaScript, C, and native Flutter execute the API key scenario. `TestAPIKeyLimitSDKGiztests` uses the `gizclaw_sdk_e2e` build tag and requires `GIZCLAW_LIMIT_C_RUNNER` and `GIZCLAW_LIMIT_FLUTTER_RUNNER` to name built runners. Missing runners or skipped steps fail. The SN/IMEI Admin fixture belongs to the Go lane; the device control C SDK does not provide Admin APIs.

## Real WebRTC Peer blocking regression

`go test ./cmd/internal/server -run '^TestPeerBlockedSDKWebRTC$' -count=1 -v`
uses temporary SQLite state, a real Server, Go SDK WebRTC, and Admin HTTP to
verify concurrent RPC stream opening and online block disconnection, blocked-key
reconnect rejection, and successful reconnect/ping after approve. It covers omitted `peer-admission`
(default open) and registration-token mode; the latter also requires handshake
`peer_forbidden`. It has no build tag and runs in normal Go CI without an AI
provider, external credentials, or Docker.

## Live Doubao low-latency Giztest

Set a real `GIZCLAW_E2E_VOLC_ARK_API_KEY`, then run:

```sh
GIZCLAW_E2E_DOUBAO_FAST_MODEL=doubao-seed-2-1-lite-260915 \
GIZCLAW_E2E_SERVICE_TIER_REPORT_DIR="$(mktemp -d)" \
  go test -tags=gizclaw_provider_e2e ./cmd/internal/server \
  -run '^TestDoubaoServiceTierGiztest$' -count=1 -timeout=5m -v
```

The test creates a Credential, Volc Tenant, `service_tier: fast` Model, Eino Workflow, RuntimeProfile, and RegistrationToken through Admin HTTP. It starts a real Server with temporary state and runs three WebRTC text turns from `testdata/doubao-service-tier/fast.giztest.yaml`. The default model is `doubao-seed-2-1-lite-260915`; `GIZCLAW_E2E_DOUBAO_FAST_MODEL` can select another Model ID or Endpoint ID with low-latency service enabled.

A transparent observer forwards requests to the real Ark HTTPS API without replacing upstream responses. All three turns must return the expected text and EOS, request `fast`, receive HTTP 200, and report actual upstream execution as `fast`. A fallback to `default` or an absent tier fails acceptance. The report directory contains redacted `giztest.json` and `ark-tiers.json` with only tier, status, timing, token metadata, and failure codes. Missing credentials fail explicitly. This test incurs real provider usage and is excluded from ordinary Go tests without the build tag.

## RegistrationToken admission and lifecycle

```sh
bash tests/gizclaw-e2e/run_admission_tests.sh
```

This fixed lane starts a real Server over temporary SQLite state with `peer-admission: registration-token`, provisions an administrative Peer, and creates resources through Admin HTTP. It needs no AI credentials or Docker. Go, Node, protoc, and Flutter are required; a missing runner fails. Linux requires a graphical session, `libpulse-dev`, and a running PulseAudio device for Flutter WebRTC initialization. CI starts a PulseAudio null sink, makes its monitor the default source, and runs the same script under `xvfb-run -a`.

The Go, JavaScript, Flutter, and C Giztest runners encode `registration_token` as an SDK credential during initial signaling and call `server.register` after connecting. Reconnect reuses the public key without a handshake credential. Documents can override signaling with `clients.<name>.admission_credential: {version, type, value}`; value supports variables and registration_token retains its registration RPC meaning.

The successful document under `tests/gizclaw-e2e/testdata/admission/` covers new registration, idempotent repeats, credential-free reconnect after exhausting capacity, and repeated registration. Negative documents and disabled/expired/exhausted resources require both runner failure and exactly one real Server `403 peer_forbidden` response. Startup failures, missing requests, and skipped documents do not pass. A standalone Go SDK case lives in `tests/gizclaw-e2e/go/admission/`; giznet WebRTC e2e additionally checks custom generic type/version values.

The Admission SDK E2E CI job runs the complete lane. Ordinary Go tests run the Go Giztest scenarios. PostgreSQL Integration runs `TestPostgreSQLRegistrationTokenLifecycle`, covering old-schema migration, editable limits, idempotence, and independent connections racing for the last slot. SQLite runs equivalent lifecycle and independent-connection concurrency tests.

### Concurrent RegistrationToken activation

```sh
npm ci && npm run build:console
git lfs pull --include 'third_party/audio/prebuilt/*/<os>-<arch>/**'
bash tests/gizclaw-e2e/run_registration_load_tests.sh
```

Replace `<os>-<arch>` with the host platform, such as `darwin-arm64` or `linux-amd64`. This entry requires Go, Docker and Python 3, with no model credentials. It pins PostgreSQL 17 and Redis 8.4.6 by digest, limits them to 2 CPU / 1 GiB and 1 CPU / 256 MiB respectively, and sets PG `max_connections=300`. Server/Go Giztest defaults to `GOMAXPROCS=8`. Cleanup removes only this run's containers and volumes. Evidence remains in ignored `.testbench/registration-*`, or the directory supplied as the first argument.

PG defaults to a Docker volume. `GIZCLAW_TEST_REGISTRATION_PG_TMPFS=1` selects a 256 MiB memory filesystem within the same 1 GiB container limit to isolate lock behavior from long host disk stalls. It does not measure disk persistence performance. Before/after comparisons must use the same storage option and retain `settings.txt` and `containers.json`.

`GIZCLAW_TEST_REGISTRATION_CONCURRENCY` selects 2–128 distinct public keys, defaulting to 64. The fixture starts the production Server with an isolated SQL schema and Redis prefix, provisions Profile/Token resources over Admin HTTP, and uses Go Giztest over real WebRTC for registration and four retries per public key. Signaling still uses registration-token admission. The fixture serializes signaling HTTP requests to respect the policy's concurrent same-token lookup rejection, then releases all registration RPCs through an HTTP rendezvous. RPC timing excludes setup and rendezvous and does not represent unrestricted handshake burst capacity.

Cases cover a shared token, independent tokens, a shared token with 20,000 existing activations, shared/independent tokens with a fixed 20 ms owner-write delay, and a limited token whose exhausted capacity still permits retries. The delay is a lock-scope diagnostic, not a production latency prediction. Limited tokens still serialize capacity checks; unlimited-token throughput does not describe that path. Each case verifies activation and owner counts and saves generated Giztest documents, reports, errors, PG activity and blocking PIDs sampled every 10 ms, `pg_stat_statements`, container settings, binary SHA-256 and source information. Summaries include throughput, p50/p95/p99, sampled waiters and transaction age. Sampled query age is not exact cumulative PostgreSQL lock-wait time; missing a wait in samples does not prove it never occurred.

`TestPostgreSQLRegistrationIndependentProgress` holds one owner's row in an external transaction and requires a different owner sharing the token to commit. Limited tokens and same-owner retries provide serialization controls. Other `TestPostgreSQLRegistration*` cases use actual blocking PIDs to coordinate last-slot races, limits added while waiting, both orders of token update/delete races, and cancellation rollback. They use `GIZCLAW_TEST_POSTGRES_DSN` and independent service instances. SQLite lifecycle and concurrency coverage remains part of ordinary `go test`.

### Docker admission and blocking

```sh
bash tests/gizclaw-e2e/run_admission_docker_tests.sh
```

This fixed lane enables `peer-admission: registration-token` in an isolated Compose
project. `docker/docker-compose.admission.yaml` runs Server and two Edges using the
standard Server template, without TURN, Redis, LiveKit or provider services. It
neither reads nor mounts the provider `.env`, and omits SFU configuration and
provider resource initialization. No model calls or AI credentials are needed.
The lane uses the advertised Server TCP ICE listener; relay behavior is outside
its acceptance scope. The standard `run_tests.sh` keeps its original Compose file,
credential preflight, resource initialization and default `open` behavior.
Admission sets `PREBUILD_CLI=1` to compile the CLI once during image construction;
all three services reuse the existing prebuilt entrypoint. This keeps cold builds
outside readiness checks on small CI runners. Standard images still build at
startup by default. Failures print the tail of all three service logs before
project-scoped cleanup.

The entrypoint registers the configured Admin identity through Edge, then uses the
direct-Server admin CLI context. This is the existing Edge logical-connection boundary,
not an admission exemption for the Admin key; every tested device connects directly
to Server. CLI JSON commands create a RuntimeProfile and independent valid, disabled,
expired and single-activation tokens per runner, and check activation counts. CLI
administration exercises the deployed management surface without Terraform state;
the tests never write directly to a database.

Docker and in-process lanes share the `testdata/admission/` documents and the
`admissiontest` Go driver. It forwards real HTTP/signaling responses. Negative cases
require both a failed runner report and exactly one Server `403 peer_forbidden`;
unrelated connection failures and skipped documents cannot pass. The block document
first completes registration, RPC, API-key creation and a real HTTP self lookup.
Before releasing that response, the driver issues Admin block and checks that the
online Peer becomes blocked/offline. A disconnect document calls RPC on the old
connection and requires an explicit connection-closed error; timeouts cannot pass.
The Flutter SDK propagates mandatory event-session closure before native cleanup
and shares one Peer close operation with caller cleanup. The runner uses the SDK
close helper for cleanup/reconnect; Linux native execution verifies this ordering. The rejection case requires
`peer_forbidden` on reconnect with the same public key. The recovery case approves the blocked Peer
and verifies credential-free reconnect and app-config RPC with that same key.
Orchestration follows real requests without adding DSL operations, fixed sleeps or
an overall fallback deadline. Existing Go/C ICE establishment retries remain valid;
reconnect offers are counted relative to the block checkpoint, while forbidden
handshakes still require exactly one refusal.

Local execution requires Docker and all tools needed by the in-process lane. The
Admission Docker E2E CI job uses Xvfb and a PulseAudio null sink without provider
secrets. The entrypoint always creates its own `GIZCLAW_E2E_DOCKER_PROJECT` and env
file. On success, failure or interruption, `setup/docker-compose-down.sh` removes
only that project's containers, networks, volumes and runtime state.

For manual stack startup, use
`bash tests/gizclaw-e2e/setup/docker-compose-up.sh --admission`. This starts the stack
without provider fixtures; the lane above owns Admin bootstrap and full acceptance.
The ordinary stack also accepts `GIZCLAW_E2E_PEER_ADMISSION=registration-token`, but
requires Admin provisioning. This switch governs Server signaling only and does
not protect client handshakes terminated at Edge.

The base-image builders in `setup/docker-compose-up.sh` and `setup/build-linux-cgo.sh`
share `setup/docker-base.sh`. Unset source overrides preserve `Dockerfile.cn.base`'s
CN defaults and existing CN image tag. Overrides use a separate `custom-base` tag
and always evaluate the Docker build, so a cached CN image cannot hide changed
sources; Docker layer caching still applies. `GIZCLAW_E2E_DOCKER_BASE_IMAGE` can
select an explicit output tag. CI supplies the official sources below; APT selects
`APT_MIRROR` on amd64 and `APT_PORTS_MIRROR` on arm64:

```sh
GIZCLAW_E2E_DOCKER_BASE_FROM=ubuntu:24.04 \
GIZCLAW_E2E_APT_MIRROR=http://archive.ubuntu.com/ubuntu \
GIZCLAW_E2E_APT_PORTS_MIRROR=http://ports.ubuntu.com/ubuntu-ports \
GIZCLAW_E2E_GO_MIRROR=https://go.dev/dl \
GIZCLAW_E2E_NODE_MIRROR=https://nodejs.org/dist \
GIZCLAW_E2E_GOPROXY=https://proxy.golang.org,direct \
GIZCLAW_E2E_GOSUMDB=sum.golang.org \
GIZCLAW_E2E_NPM_REGISTRY=https://registry.npmjs.org \
  bash tests/gizclaw-e2e/run_admission_docker_tests.sh
```

The admission Compose file can also be parsed and torn down without any provider
variables or generated runtime env file. The lane selects that file before setup,
and setup persists its runtime state before building images, so build failures
retain project-scoped cleanup.

## RuntimeProfile configuration persistence regression

`go test ./cmd/internal/server -run '^TestRuntimeProfileAppConfigGiztest$' -count=1`
starts a real Server with temporary SQLite storage, creates and updates configuration over
Admin HTTP, and runs the `server.app_config.get/list` scenarios through the Go Giztest CLI's
WebRTC driver. It covers pagination, device reconnect, configuration replacement, reads after
a Server restart, and clearing with an empty map or omitted configuration. The update and
clear assertions live in `tests/gizclaw-e2e/testdata/app-config/` and run at their corresponding
lifecycle stages. No external AI provider or credentials are required; ordinary Go CI runs
this path.

PostgreSQL uses isolated test schemas to verify existing-table column addition, repeated
initialization, and configuration persistence:

```sh
GIZCLAW_TEST_POSTGRES_DSN='postgres://…' \
  go test ./pkgs/gizclaw/services/system/runtimeprofile -run '^TestPostgreSQLRuntimeProfileAppConfig$' -count=1
```

Local runs skip PostgreSQL when the DSN is unset. The PostgreSQL Integration CI job requires
the DSN and runs this package. Each test cleans up only its own schema.


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

It runs Redis 7.0, two Servers with distinct local runtime state, two Edges whose configured Server order is reversed, and one single-node LiveKit; both Servers point `services.sfu` at the same signaling URL with test credentials generated for that Compose project's lifetime. The Go cases verify fixed Peer homes through both Edges, API Key routing through an Edge to the owner Server, foreign-Server rejection, local-only PeerRun writes, shared-KV versus local-state isolation, lazy SFU Room creation, and bounded reconnection after a LiveKit restart. giztest then runs the `sfu.*.giztest.yaml` scenarios serially: clients register on different Servers through different Edges and verify cross-Server friend creation, group join, the member cap, revocation after member removal, and that a `listen`-mode broadcast reaches only the other members of the room. The real LiveKit is the only acceptance environment; no in-memory fake replaces it. It does not test Workflow Workspace routing.

`sfu.*.audio-bytes` uses the committed Ogg/Opus fixture and `sfu.friend-group.members-in-room` exchanges no audio; neither needs speech-provider credentials and both always run. The other three SFU scenarios need TTS/ASR and only run when `tests/gizclaw-e2e/.env` (or the file `GIZCLAW_E2E_CREDENTIAL_FILE` points at) carries the complete Volc/Doubao credentials, which is also when the seed adds the `asr` and `narrator` aliases. Those three synthesize Chinese phrases and transcribe with `language: zh-CN`, matching the seeded Chinese `narrator` voice. To debug a transcript assertion, run with `GIZCLAW_E2E_GIZTEST_EVIDENCE=full`: the report then records the transcript that was actually recognized.

The provider-free SFU scenarios cover these boundaries:

| Scenario (without `sfu.` and `.audio-bytes`) | Coverage |
| --- | --- |
| `friend.cross-server` | Bidirectional PTT and correlated first-packet timing between friends on different Servers, without self audio or packet samples |
| `friend-group.remove-readd` | Three-client first-packet correlation; after revocation the removed member receives no audio or packet sample while the remaining member keeps receiving; re-add and select the original Workspace to restore both directions |
| `friend-group.reconnect-readd` | Reconnect while removed without regaining membership; after re-add, select the original Workspace and restore both audio directions without registering again |
| `friend-group.rapid-readd` | Three immediate remove/add cycles without waiting for periodic revocation, with PTT and realtime audio |
| `friend-group.mixed-server-members` | Two members per Server; local and remote removal preserves remaining audio; duplicate add preserves the member count; owner-add and invite-rejoin restore broadcasts |
| `friend.delete-readd` | Both friends are revoked after deletion; re-adding creates a new Workspace and restores audio |
| `friend-group.delete-recreate` | Reusing a deleted group's local name creates a new Workspace; former members not invited back cannot hear the new group |
| `workspace.isolation-switch` | Two cross-Server groups remain isolated while a member switches groups and returns |
| `workspace.stop-reconnect` | Both runtimes stop and select the same Workspace again; a same-identity Peer reconnect restores bidirectional PTT/realtime audio without registering again |
| `friend-group.members-in-room` | `in_room` on `server.friend_group.members.list`: false while online without a runtime, true after selecting the Group's Workspace, false while running another Group's Workspace, after `server.run.stop` and after a reconnect; another member on the same Server sees the same values; `online` stays true throughout |

Media assertions check actual Opus bytes and packet counts without retrying media steps; runtime waits are bounded. The rapid-add case exercises the revocation/activation race window without guaranteeing a particular interleaving. Each scenario uses isolated Peers and resources and cleans up runtimes, social resources, and Peers in `finally`. The report is written to `tests/gizclaw-e2e/testdata/multi-server/sfu-report.json`. These cases do not verify device speaker playback or speech transcripts.

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

### Remote Memory scope purge

The same tagged package contains `TestMemoryScopePurge`, which checks the
Workspace deletion memory purge against a live Mem0-family provider. Select it
with `GIZCLAW_MEMORY_PROVIDER`:

| Provider | Required variables | Optional |
| --- | --- | --- |
| `volc-mem0` | `GIZCLAW_VOLC_MEM0_ENDPOINT`, `GIZCLAW_VOLC_MEM0_API_KEY` | |
| `mem0-self-hosted` | `GIZCLAW_MEM0_SELF_HOSTED_URL` | `GIZCLAW_MEM0_SELF_HOSTED_API_KEY` |
| `mem0-platform` | `GIZCLAW_MEM0_API_KEY` | `GIZCLAW_MEM0_ENDPOINT` (default `https://api.mem0.ai`) |

```sh
GIZCLAW_MEMORY_PROVIDER=volc-mem0 \
GIZCLAW_VOLC_MEM0_ENDPOINT=https://... GIZCLAW_VOLC_MEM0_API_KEY=... \
  go test -tags=store_e2e -count=1 -v -run '^TestMemoryScopePurge$' ./tests/store-e2e
```

The Volc data-plane key selects the memory project, so use a key of a dedicated
test project; no project ID or AccessKey is needed. The self-hosted lane targets
the repository's Mem0 OSS service (`build/mem0/Dockerfile`,
`mem0ai 2.2.1`), which serves the standard entity-scoped `GET /memories` and
`DELETE /memories` routes and reads its model key from a mounted
`tests/gizclaw-e2e/.env`:

```sh
PLATFORM=linux/amd64 IMAGE=gizclaw-mem0 build/build-mem0.sh
docker run -d --rm -p 127.0.0.1:18000:8000 \
  -e MEM0_CREDENTIAL_FILE=/run/gizclaw-e2e.env \
  -v "$PWD/tests/gizclaw-e2e/.env:/run/gizclaw-e2e.env:ro" gizclaw-mem0
GIZCLAW_MEMORY_PROVIDER=mem0-self-hosted GIZCLAW_MEM0_SELF_HOSTED_URL=http://127.0.0.1:18000 \
  go test -tags=store_e2e -count=1 -v -run '^TestMemoryScopePurge$' ./tests/store-e2e
```

An unset provider skips the test; an unknown provider or a missing required
variable fails before any request, and provider errors or timeouts fail rather
than skip. Each run writes a direct Fact to two generated Workspace IDs and
submits an extraction for the first, purges the first while an asynchronous job
may still run, waits for the job to finish, and repeats purge and verification
the way Workspace deletion retries `memory_residual`. It then fails if the first
Workspace gains a late Fact during a settle interval or if the second Workspace
lost its Fact. The generated `gizclaw-e2e-purge-<unix-nanos>-a`/`-b` Workspace IDs
bound through `memory.BindApp` are the only scopes the test touches, and cleanup
purges both until verification reports empty, also after a failure. The log
records how many purge rounds verification needed.


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
├── cgo/         # cgo tests and the Giztest runner whose clients are the C SDKs
├── cmd/         # real gizclaw CLI tests
├── giztest/     # declarative Peer RPC, Workflow, and benchmark scenarios
├── go/          # focused Admin, delete, Edge, and OpenAI tests
├── terraform/   # Terraform provider against a real Server
├── js/          # JavaScript/TypeScript WebRTC tests and giztest runner
└── flutter/     # Flutter/Dart giztest runner
```

`workflow_catalog_test.go` below `tests/gizclaw-e2e/testdata` holds static
fixture checks that need neither Docker nor credentials. They cover the Workflow
catalog, Eino graphs, Workspaces, and the Server/Edge config templates. Go
`./...` patterns skip directories named `testdata`, so the package is not part
of `go test ./...`; the CI Go Test job runs it separately, and local fixture
changes must run it too:

```bash
go test -count=1 ./tests/gizclaw-e2e/testdata
```

Business fixtures compose Eino Prompt, ChatModel, Memory and typed State directly.
A bounded Starlark node owns deterministic Werewolf, mystery and poetry rules;
the model narrates the authoritative result. Werewolf persists seats, roles,
eliminations, ballots and victory. During play, the complete role map stays out
of character prompts and long-term Memory; publicly revealed final identities
may be used for review. Rules render the player's private role, inspection and
wolf teammate hints directly. Each character receives its own role and allowed
knowledge. Teammate rejection details stay out of public Memory. Rules render
actual ballot counts and surviving faction counts directly.
The mystery retains the four evidence requirements of Rainy Night
Gramophone; motive requires both the will and newspaper, and undiscovered
individual clues stay out of narration context. Poetry uses the Tang/Song/Yuan/Ming answer catalogue, awards each
badge once and adds 100 points per checkpoint. Memory observes public state.
Native History supplies conversation turns; only game progress is persisted in
Graph State, without a second conversation channel store.

Screenplay prompts specify character positions, responses to the player's current
reasoning, available actions at each stage and narrative closure. Current State
overrides prior narration and long-term Memory. Mystery narration uses acquired
evidence only, poetry hints do not supply answers, and Werewolf dialogue follows
fixed seats without replacing rule decisions. The Werewolf referee announcement
is rendered directly from rules; the model supplies living-character dialogue
during play and may answer as an eliminated character in the final review.
The final publisher rejects model declarations of phases, tallies, victory or
private identities. Character history projects NPC dialogue and the player's
public daytime statements, excluding private hints and night actions. Death records distinguish attacks,
poison, exile and hunter shots. The final identity table is rendered directly by
the referee and stays outside long-term Memory. Werewolf renders the referee
announcement in its rules node. Native Eino Batch calls at most two
relevant characters in parallel with separate identities, wolf teammate
knowledge, public dialogue and each character's last position. They do not share
the full role table during play. Each character also receives its own deterministic ballot target before
speaking, matching the NPC ballot subsequently committed by rules; other
characters' unsubmitted intentions stay out of its prompt and public Memory.
NPC wolf voting excludes known teammates. An attacked or exiled NPC hunter
shoots a surviving target from its last public ballot, with a seat-and-round
fallback when that target is unavailable. A player hunter gets a shoot-or-decline
choice before victory is settled. Poisoned hunters cannot shoot. Rules answer
final hunter-rule questions and farewells directly.
The publisher validates character speech and combines it with the referee
announcement in one assistant text output with one turn-ending boundary.
VoiceAdapter selects character Voices within one audio route. First-response
measurement includes this turn's character generation. Game progress uses State and dialogue uses History;
long-term Memory is observed once for a public final audit, rather than every
turn. Journey and ensemble stories
resolve earlier commitments in the final chapter, then enter review or a brief
farewell. The quality player's initial brief is a goal, not an established event
or identity. These configurations still require actual provider-backed dialogue
assessment; written prompt requirements do not prove passing quality.

Candidate prompts separate history from the current input, repeat authoritative
State beside that input. Werewolf characters use temperature 0.3; other candidates
use 0.2. Journey advances only through a complete affirmative continuation command;
questions and negations leave the chapter unchanged. Ensemble stories also require
an explicit current-chapter agreement, renewed after an objection, before advancing. Werewolf
recognizes explicit action clauses without executing past ballot discussion or negated
skills. The quality player tracks its 20 utterances in native State, advances the
main objective, asks distinct review questions and then says farewell. It does
not invent completion when the objective remains unfinished.

`eino_catalog_test.go` compiles actual Resource and Workspace configurations,
executes their native pipelines, and checks illegal actions, private projections,
insufficient evidence, wrong answers and completed games. These local model and
Memory fixtures do not replace live-provider or device voice acceptance.
`eino-werewolf-game.push-to-talk-roundtrip` starts a game by voice, then advances
to daylight with text to verify voiced NPC replies, one audio stream and complete
turn boundaries. VoiceAdapter removes configured character markers from visible text.
Werewolf character nodes use `max_tokens: 256`; other screenplay LLM nodes use
2048. Latency comparisons use a 128-token direct
answer and an additional 64-token planner in the planned variant. Both are
validated independently.

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

The shared firmware metadata scenario is `server.firmware.metadata.get.giztest.yaml`.
It covers objects, strings, arrays, numbers, booleans, explicit null, large integer precision,
missing or invalid keys, exact flat-key matching, binding after reconnect, and the existing
stable channel. Each run executes 16 steps and one Peer deletion cleanup; a failed step or
native runner fails the test.

Both entry points below use temporary Servers, real WebRTC, and Admin APIs without provider credentials:

```sh
go test ./cmd/internal/server -run '^TestFirmwareMetadataGiztest$' -count=1
bash tests/gizclaw-e2e/run_firmware_metadata_tests.sh
```

The first runs with ordinary Go tests. The second builds and runs the Go, JavaScript, C,
and Flutter native runners against the same scenario and Firmware fixture. The full Docker
firmware suite also executes this scenario.

Managed-deletion changes use a fixed production vertical-slice entrypoint. It
validates the shared credential file, starts an isolated Docker stack, and runs
the dedicated Peer RPC deletion package for Workspace, Friend Group, and Peer
resources. The suite covers active-use termination and Peer tombstone
survival across a Server restart, then cleans the project after success or
failure without running unrelated provider-backed scenarios:

```bash
bash tests/gizclaw-e2e/run_pending_deletion_tests.sh
```

The full gate installs locked Node workspaces, initializes nanopb, builds the
E2E CLI, starts Compose, waits for Server and Edge, runs JS, C/cgo, Go
Admin/OpenAI, CLI, Terraform provider, and Giztest phases in order, and performs one bounded
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


Standard runners exclude cross-server `sfu.*` documents, which are verified by `run_multi_server_tests.sh` with its two-Server environment. The intentionally failing `failure-cleanup.giztest.yaml` runs only in its dedicated cleanup phase, outside the successful JavaScript, Flutter, C, and standard Go scenario batches.

The standard JavaScript batch uses `setup/run_js_giztest.py` to start a separate Node process for each document, with at most four processes running concurrently. This isolates native WebRTC lifetimes between scenarios. Each process has a 600-second external deadline; timeouts, process failures, and scenario failures remain failures in the combined report.

The JavaScript runner uses `protoc` to generate temporary descriptors from the repository RPC schema. It converts scenario Protobuf JSON requests into SDK objects and converts responses back into Protobuf JSON before assertions. Enums, oneofs, default fields, and 64-bit integers follow the same schema; unknown fields and invalid enums are rejected. Both `npm run giztest` and `npm run test:giztest-unit` prepare the descriptors automatically.

### Manual environment

The standard Docker environment includes a single-node LiveKit with temporary SFU credentials generated for each setup. Group RPC, event-stream, and social tests use this service inside the Compose network. LiveKit publishes no host ports and is removed with the test environment.

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
`GIZCLAW_E2E_SERVER_ENDPOINT` is host-Admin-facing, while `GIZCLAW_TEST_ENDPOINT`
always points to the Edge. The remaining generated
variables provide the CLI config home, identity home, and Compose
project. Reset the standard resource set with:

```sh
bash tests/gizclaw-e2e/setup/reset-data.sh reset --context remote-admin
```

`init` only applies fixtures, `clear` only removes known fixtures, and `reset`
performs both. Only credential placeholders are expanded from `.env`; missing
provider credentials fail before a partial setup can be treated as valid.
Workspace history is runtime data and must not be seeded by the reset script.

Server apply may silently ignore fields the Schema does not declare, so a
successful reset does not prove the fixtures match the Schema.
`go test ./cmd/internal/commands/admin -run '^TestAdminValidateE2EResourceFixtures$'`
runs `admin validate` offline over every fixture under `testdata/resources/`,
including the dedicated `mhs/` and `safety-fence/` catalogs, with placeholder
values for `${...}` references.

### Suite ownership

- `go/admin` validates typed contracts with the generated Admin HTTP client.
- `go/delete` retains deletion checks that require Admin observation, restart, and tombstones.
- `go/edge` retains TURN relay, sibling-close, failure recovery, and network diagnostics.
- `go/openai` retains typed SDK coverage of the OpenAI-compatible API.
- `giztest/*.giztest.yaml` covers Peer RPC, conversation, social, and Workflow behavior.
- `cmd` executes `testdata/bin/gizclaw` with `os/exec`; it must not bypass the CLI with `go run` or typed clients.
- `js/admin` covers WebRTC Admin fetch; `js/rpc` covers peer and server-initiated RPC.
- `js/giztest`, `flutter/giztest`, and `cgo/giztest` run the same giztest scenarios with their own SDKs; see the next section.

### Giztest runners

The same `giztest/*.giztest.yaml` scenarios run under four runners, each using
its own language's SDK, so one scenario suite validates the contract and every
SDK:

| Runner | Entry point | Device side | Controller side |
| --- | --- | --- | --- |
| Go | `gizclaw test run` | `sdk/go/gizcli` | HTTP inside the runner |
| JavaScript | `npm run giztest -- run` (`tests/gizclaw-e2e/js/giztest/index.ts`) | `@gizclaw/gizclaw` | `@gizclaw/gizclaw-control` |
| Flutter | the built `giztest` desktop binary (`tests/gizclaw-e2e/flutter/giztest`) | `gizclaw` | `gizclaw_control` |
| C | the built `giztest-c` binary (`tests/gizclaw-e2e/cgo/giztest`) | `sdk/c/gizclaw` through cgo | `sdk/c/gizclaw_control` |

All four accept the same command line (`validate -f <path>`, `run <path>
--parallel N --output <report>`) and write the same report JSON structure. The
JavaScript and Flutter runners implement only the `rpc`, `client_rpc`, `http`,
`output` and `reconnect` step kinds; a document using any other step kind, or
an `audio` or `binary` variable, is reported as skipped on stderr rather than
passing silently. The C runner implements `rpc`, `client_rpc`, `http` and
`reconnect`, and reports
an unsupported document the same way, naming the step and operation. All three
are pointed at the whole scenario directory; a document that is malformed
rather than merely unsupported is still an error, never a skip.

The Go and C runners share `pkgs/giztest`, which owns the document schema,
variables, captures and expectations, the task runner, and the report. Each
supplies a `giztest.Driver` and its per-task `giztest.Session` while the runner
keeps `barrier`, `output`, and `review`. `api/giztest/giztest.schema.json` is
the cross-language document contract every runner validates against.

See [Start offsets and think time](/en/using/cli#start-offsets-and-think-time)
for scheduling and report fields. Go/C share seeded start jitter, stagger and
step think time. JavaScript/Flutter validate and accept these top-level fields
but ignore scheduling and record `timing_mode: ignored`, so SDK contract checks
do not replace Go/C load measurements. Existing concurrency benchmarks retain
explicit zero delays; the realistic concurrency-16 scenario defaults to jitter.
Explicit zero CLI overrides provide a lockstep control with the same document.

The Flutter runner is a desktop binary rather than a plain Dart CLI because the
device side needs the `flutter_webrtc` platform implementation. `run_tests.sh`
builds and runs it on macOS and Linux and skips it on other hosts.

Every runner projects an `rpc` step response to proto3 JSON with proto field
names: `int64` and `uint64` fields are strings, and implicit-presence fields
left at their default — scalars, enums, repeated fields, and maps — are present
(`has_next: false`, `items: []`). Go uses protojson's `EmitUnpopulated`, the
JavaScript runner uses `alwaysEmitImplicit`, and the Flutter runner fills the
same fields from the generated `payloadFieldIsProto3Optional` table because the
Dart protobuf runtime does not record proto3 `optional`. The one remaining
difference is an unset message, oneof, or proto3 `optional` field: Go renders it
as `null`, while JavaScript and Flutter omit it.

### Giztest scenarios

AST consecutive-turn regressions live in
`volc-ast-translate.push-to-talk-consecutive-turns.giztest.yaml` and
`volc-ast-translate.realtime-consecutive-turns.giztest.yaml`. Each synthesizes
Chinese audio once and replays it for three Chinese-to-French turns in one
Workspace, retaining a 30-second `idle_timeout` and requiring nonempty text,
audio, and matching response EOS boundaries. The standard discovery rules in
`run_tests.sh` execute both files; CI's Release Contract job validates their
documents. Live Volc calls require the standard Docker fixtures and credentials.
Authorization blocking, abnormal AST closure, and CLI completion regressions run
under ordinary `go test` without credentials. Set `GIZCLAW_TEST_ENDPOINT` to test
the direct Server or Edge entrypoint separately; a direct pass does not verify Edge.

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

`server.runtime_profile.tags.giztest.yaml` uses real WebRTC RPC to verify unfiltered, single-tag, and multi-tag Workflow queries with AND semantics, no match or OR across age tags, invalid tags, list pagination with selector-bound cursors, get projection, and Workspace creation by alias. `server.device.runtime_profile.get.giztest.yaml` checks the corresponding Public HTTP filter. Local `go test ./cmd/internal/server -run '^TestRuntimeProfileAndWorkspaceToolkitGiztest$' -count=1` runs the RPC scenario and, after an Admin Profile update, `testdata/runtime-profile/updated.giztest.yaml` to confirm the rotated SQLite snapshot is visible over RPC. The Public HTTP scenario runs in the standard Giztest environment.

The device control and Contact Public HTTP contract is covered by the `server.device.*` and
`server.contacts.*` scenarios, and friends and Friend Groups by `server.friends.http` and
`server.friend_groups.http` (two devices with their own API keys cover long-lived invite codes, a
repeated befriend, a non-owner dissolving, the owner leaving, and a member leaving). Plain `go test`
also runs `pkgs/gizclaw/integration_peer_http_social_test.go`: it starts an in-process Server with an
SFU endpoint, devices complete `server.register` and `server.api_key.create` over real WebRTC
connections, and the generated Go `peerhttp` client calls the friend and friend group routes, then
manages a disconnected device with its API key over another device's connection. An `http` step sends one Public HTTP request to the current client's
`access_point` origin (`method`, `path`, `headers`, JSON `body`, optional `status`); the response JSON
is the step value for `expect`, `capture`, and `save_as`, and a 4xx/5xx without a declared `status` is
an assertion failure. The API key comes from a `server.api_key.create` step with
`capture: {api_key: /api_key}` and is sent as the `Authorization: "Bearer ${api_key}"` header.

`http.query` supports string, number, and boolean query parameters. Exact variable
references retain their types, such as `query: {timestamp: "${checkpoint}"}` for
server-issued SSE checkpoint numbers. `http.response_format: sse` projects a finite
response as `{events: [{event, data}], last_event, raw}`. JSON data is decoded,
other data stays text, and only blank-line-terminated events with data dispatch.
An incomplete terminal frame cannot satisfy a done assertion. Parsing is bounded
to 4 MiB and 16384 events. Go/C, JS, and Flutter use
`api/giztest/testdata/http_sse_vectors.json` for shared conformance tests.

`server.peer.sync.giztest.yaml` covers initial reset, continuation, unchanged
state, owner isolation, creation, coalesced final updates, deletion, device
reconnect, stale checkpoints, and invalid timestamps or missing/invalid/revoked
API keys. `go test ./cmd/internal/server -run '^TestPeerSyncGiztest$' -count=1`
starts real Server and Edge processes over temporary state. Devices connect
through Gateway/WebRTC, and Edge forwards HTTP SSE to the authoritative Server.
The report requires all 23 steps and both cleanup steps to pass; ordinary Go CI
runs this test too.

JavaScript and native Flutter reuse the same scenario. Build the Flutter runner,
set `GIZCLAW_SYNC_FLUTTER_RUNNER` to its executable, and run
`go test -tags=gizclaw_sdk_e2e ./cmd/internal/server -run '^TestPeerSyncSDKGiztests$' -count=1`.
Missing runners and execution failures fail rather than counting validation as
acceptance. The C controller SDK currently has no `/sync` route, so its runner
explicitly marks this scenario unsupported; Go/C share decoder unit coverage.

A `client_rpc` step names `client.mhs.v0.read/write`, `client.tool.v0.invoke/list`, or `client.rpc.methods.list`. For an invoke step, `tool` selects the predefined `ClientTool` payload. The runner installs the scripted provider response when the client connects; `response: {error_code: 3}` answers a canonical error. Uninstalled tools answer `UNIMPLEMENTED`, and `expect_calls` proves that a later HTTP call reached the provider. A Server-side validation case expects zero calls.


A `reconnect` step drops that client's Peer connection and dials a replacement
on the same identity, reproducing how a device reaches the Server again after a
reboot or a network switch. The Server ends such a transition exactly when a
replacement connection arrives for the same owner; until then control routes
answer `409 DEVICE_OFFLINE`. The optional `await_ms` bounds the redial and is
capped at 60000. The step completes after one RPC round trip on the replacement,
because behind an Edge the dial returns before the Edge's tunnel session reaches
the Server. The scripted providers are reinstalled on the new connection
and keep their call counts, so `expect_calls` asserts the total across both.
A scenario installs one `response` per method, so the device reports the same
values before and after; what a step after `reconnect` asserts is that a control
route recovered from `409`, not that a method answered differently.

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
An `audio` or `binary` input variable that declares `env` reads standard base64
bytes from that environment variable; the runner decodes them and applies
`max_bytes`, so provider-free scenarios can push a committed fixture.
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

Push-to-talk `peer_stream` input uses the device lifecycle on one logical
StreamID: control-only BOS, audio BOS, Opus packets after the SDK receives
`AUDIO_INPUT_READY`, audio EOS, then control-only EOS. `pacing` applies only
to audio packets and adds no delay between control boundaries.
`/response_count` in the result and evidence counts assistant response StreamIDs
with content that were neither ignored nor interrupted. A single PTT turn can
assert 1 even when the server mixes several replies into one audio downlink.
Assistant terminal completion continues collecting output for 250 ms after
the first complete reply. `reply_observation` sets another positive Go duration;
the PTT regressions use 1 second to include late second replies. This is a
bounded observation, not proof against output beyond that window. Empty input,
`first_response`, `input_sent`, and transcript completion use their own bounds.
Audio input also reports `/input_packets`, `/input_ms`, and `/pushed_packets`.
Audio BOS waits 500 ms after control BOS by default. `hold_before_audio` can
set another nonnegative Go duration for a different device threshold. PTT
omits wire `input_mode`; Workspace parameters determine the actual input mode.
`eino-voice-assistant.push-to-talk-long-input.giztest.yaml` sends at least 60
seconds of multi-sentence speech, trims tail silence before release, and requires
one reply with matching text and audio EOS.
`eino-voice-assistant.push-to-talk-short-history.giztest.yaml` verifies that a
short utterance whose final ASR arrives after input end keeps one user history
entry with nonempty text and a downloadable recording. `replay_available` alone
is not evidence of an audio asset.

`peer_stream.terminal_label` defaults to `assistant`; that completion requires
text and audio EOS boundaries from the same response, with nonempty content
for each required modality. Late EOS events without an observed BOS or content
do not complete a turn, and EOS events from different StreamIDs cannot be combined.
Turns that complete on the
persisted user transcript declare `transcript` explicitly.

`peer_stream.overlap_input: true` repeats the declared audio input on the same PeerStream.
It supports `push-to-talk` and `realtime`. After sending the first input (including realtime
VAD tail silence) and receiving first-response audible assistant audio, it starts the second input
without closing the stream or sending an explicit interruption. An already completed first
audio response fails. Success requires the second input's first audio packet to be successfully
sent before receipt of the first response's audio EOS. Both responses must close their text
and audio routes; the second must contain text and audio and complete without errors.
A first response reporting `interrupted` after the second input is allowed and recorded as
`first_response_interrupted`; the test does not require a particular provider interruption policy.
Only `mode`, `input`, and `pacing` can accompany this option; use the step `timeout` for the bound.
Results include `input_overlap`, `session_connection_reused`, `second_input_sent`,
`first_audio_ms`, `second_input_audio_ms`, `first_audio_eos_ms`, and second-response EOS flags.
The Doubao, Eino, and Eino `*-overlapping-input.giztest.yaml` scenarios cover both input
modes in the Go runner. JavaScript and Flutter runners skip the unsupported `peer_stream` operation.

Empty assistant BOS events do not establish response ownership; actual text or audio content does. The scenarios use a single Chinese counting request to reduce extra VAD turns caused by pauses within the recording.

`peer_stream.completion: first_response` is the bounded deployment-probe
alternative. `require_text` and `require_audio` select its required modalities
and both default to true. Every required modality needs its corresponding
positive `first_text_timeout` or `first_audio_timeout` Go duration; a disabled
modality omits its deadline, and at least one modality must remain required.
The deadlines start only after the whole turn input has been pushed. The runner
succeeds and closes that logical stream as soon as it has observed the first
assistant content for every required modality (a non-empty text fragment, or
the first audible audio frame); it does not wait for
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
sends its declared audio input. The Go runner also lets later steps use the same
`session` and `keep_open: true` without re-arm: it sends a new BOS immediately
to start another turn while the previous response is pending. Consumed,
cross-client, or cross-task sessions cannot be used for re-arm.

From waiting for reload through completion of the replacement response, any assistant EOS error code or message fails the step, including `interrupted`; error-free EOS and the exact input-reload notification remain valid.

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
`p95_interval_ms`, `max_interval_ms`, `drift_ms`, `absolute_drift_ms`,
`buffer_surplus_ms`, and the continuous-playback simulation `prebuffer_ms`,
`underruns`, `underrun_ms`, `max_underrun_ms`, and `minimum_buffer_ms`. Intervals
use the stream reader's monotonic receipt time. A separate stage decodes Opus,
classifies audibility, and observes events after that timestamp, so its work,
assertions, persistence, and PortAudio playback cannot delay the next receipt
measurement. A positive `buffer_surplus_ms` means network delivery is ahead of
the Opus media clock.
`max_interval_ms` remains diagnostic: one arrival gap can exceed 100 ms while
the 500 ms prebuffer still carries continuous playback. The Doubao realtime
roundtrip gates playback with zero underruns and a positive minimum buffer,
alongside mean/P95 cadence and an upper bound on buffer surplus, rather than
a hard maximum gap. A covered arrival gap can lower cumulative buffer surplus
below the 500 ms pacing target without interrupting playback, so the roundtrip
does not impose a lower bound on that diagnostic value.
All `*_ms` values use milliseconds. `target_span_ms` is the sum of every packet
duration except the last, `drift_ms = receive_span_ms - target_span_ms`, and
`buffer_surplus_ms = -drift_ms`. P95 uses nearest-rank selection over arrival
gaps. With one packet, only `packets` and `audio_ms` are present; with no
assistant Opus, `audio_pacing` is absent.

The playback simulation replays the arrival trace through a client that
buffers `prebuffer_ms` (500 ms) before it starts and then consumes audio on
its own clock. `minimum_buffer_ms` is the lowest the buffer ever falls; a
negative value means one gap outlasted the audio held at that moment, which
the listener hears as a stall. `underruns` counts those stalls and
`underrun_ms` and `max_underrun_ms` report the total and longest silence.
A single stall cannot move the P95 interval, so playback continuity is
asserted through `underruns` and `minimum_buffer_ms` instead. A reply
shorter than `prebuffer_ms` only starts once every packet has arrived and
cannot underrun, so only `prebuffer_ms` is reported.
Giztest documents assert these paths through ordinary numeric `expect`
constraints rather than a separate pacing schema.
`eino-voice-assistant.push-to-talk-roundtrip.giztest.yaml` and
`doubao-realtime-conversation.realtime-roundtrip.giztest.yaml` require 20 ms
Opus frames, a mean interval from 12 through 21 ms, P95 no greater than 30 ms,
maximum interval no greater than 100 ms, at least 101 packets, no underruns
with a positive `minimum_buffer_ms`, and final buffer
surplus from 450 through 550 ms. The two cases cover push-to-talk and realtime
delivery respectively. Those ranges permit bounded recovery around the 500 ms
target without demanding an unrealistic exact 20 ms arrival for every network
packet.

Both of those cases exercise a single turn, where the downlink pacer is
building its target for the first time. The turns that follow are the ones that
regress: the idle wall clock between them is not audio the client consumed, so
charging it to the pacer makes every later turn arrive ahead of real time.
`eino-voice-assistant.push-to-talk-multi-turn-pacing.giztest.yaml` and
`eino-concurrency-assistant.push-to-talk-multi-turn-pacing.giztest.yaml` take
three turns against one Workspace and require at least 200 packets on each, a
`buffer_surplus_ms` no greater than 700 ms, no underruns, and a positive
`minimum_buffer_ms`. The packet floor keeps a short reply from satisfying the
pacing assertions without exercising them, since the surplus a broken pacer
accumulates grows with the length of the turn. The two cases cover the eino
and eino drivers, which share the same cascaded text-to-TTS downlink.

`first_audio_ms` records the first **audible frame**, not the first audio
packet. The runner decodes every Opus packet on the stream reader at 16 kHz
mono and counts a frame as audible once its decoded peak reaches about
-42 dBFS (sample magnitude 256): provider TTS output opens with a low-level
lead-in before speech (measured peaks up to about 210), and speech onsets
measure 300 and above. A payload that cannot be decoded, and non-Opus audio,
count as audible, as every payload did before silence could be told apart.
Silence received ahead of the first audible frame is summed on the Opus clock
as `leading_silence_ms`, and the part of it that decodes to exact digital
silence is reported again as `leading_digital_silence_ms`. It records what the
receiver observed, not who produced it: the downlink mixer fills a track that
has no buffered audio with digital silence, and a provider could emit it too.
Both are accumulated per stream and reported for the
stream that produced the first audible frame, so the tail of an earlier reply
still arriving from the downlink buffer is not charged to this one. Both are
present in the `peer_stream` (including `listen`) result and evidence, and
that time is not credited as first audio. The `first_response` audio deadline
and the `overlap_input` second-turn start use the same definition.
`eino-concurrency-assistant.push-to-talk-leading-silence.giztest.yaml` takes
three turns on one Workspace and requires `leading_digital_silence_ms` of at most
80 ms and `leading_silence_ms` of at most 200 ms on each: a reply track created
before its TTS has decoded audio makes the mixer send digital silence for it
first, the pacer delivers that silence into the device buffer faster than real
time, and the device must play it before the reply can be heard. The 80 ms
bound leaves room for the 60 ms preroll the server keeps ahead of speech,
should a provider's lead-in be digital silence, plus one 20 ms frame for a
track that opens a frame before its first write lands. The 300 to 600 ms
low-level lead-in provider TTS emits before speech is trimmed by the server to
that preroll, which the `leading_silence_ms` bound holds it to.

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
adds bounded relay text plus the transcript a `server.speech.transcribe` step
recognized, and produces a sensitive artifact without adding inputs,
credentials, IDs, or audio payloads.
`workspace-relay.workflow-tester.giztest.yaml` runs the live candidate/tester
pair inside the standard gate;
`workspace-relay.doubao-realtime-workflow-tester.giztest.yaml` proves text
forwarding with audio EOS completion against a multimodal candidate; and
`run_workspace_relay_tests.sh` starts one isolated stack, runs both repeat-1
gates and the repeat-20 relay gate
(`benchmark.workspace-relay.workflow-tester-20.giztest.yaml` with
`--parallel 20`), and always cleans the stack up.
The paired tester Workflow uses seven probe turns and an eighth verdict turn.
If the model emits a bare `PASS`/`FAIL` or empty text during a probe, the
publisher asks a follow-up instead; the final model verdict is published as is
against the brief's per-reply criteria. Probe questions must not demand a
definitive culprit or completed story when the brief only checks relevant,
nonempty host replies; a reasoned statement that clues are insufficient is a
responsive answer.

### Eino audio input comparison

```sh
bash tests/gizclaw-e2e/run_audio_input_comparison_tests.sh
```

`eino-audio-input` compares two [audio input paths](/en/developing/gizclaw/services/ai#eino-audio-input-path) with the same `audio-llm` (`doubao-lite-audio-chat`, upstream `doubao-seed-2-1-lite-260915`) and `multilingual-voice`. The `e2e-giztest` Profile omits PTT ASR; the `e2e-audio-asr` Profile selects `ptt_asr_model: asr` (`volc-bigasr-sauc`) on its Workflow binding. Legacy Workflow `asr_model` is ignored and Workspaces cannot choose the source. The script starts one isolated stack and runs `benchmark.eino-audio-input-comparison` for Mandarin, Sichuanese, Cantonese, English, Japanese and Spanish. Two Peers register with the two Profiles, create Workspaces on the same Workflow, assert the effective path after reload, then compare transcripts and replies for the same recording. `setup/audio_input_comparison.py` scores keyword groups and saves reports, logs and first transcript/text/audio timings in `testdata/audio-input-comparison/`. Missing expected transcription or reply on the native path fails a sample; ASR remains the comparison baseline. `GIZCLAW_AUDIO_SAMPLES=mandarin,cantonese` selects samples; `setup/audio_input_comparison_test.py` verifies parsing and scoring offline.

A `peer_stream` result's `/transcript` is the non-interim `transcript`-labelled text and `/reply` is the kept `assistant`-labelled text; `/text` still holds every text fragment in arrival order. The regular-phase `eino-audio-input.push-to-talk-transcript` first asserts that reload reports the `model` path, then asserts `/transcript`, `/reply`, and the History write for a Mandarin question. `eino-audio-input.path-selection` uses RPC only, so the runner of every language executes it: it asserts the binding default `model`, the Workspace parameter overriding it with `asr`, `realtime` input keeping `asr`, `model` again after switching back to Push-to-Talk, and `eino-concurrency-assistant`, which declares only `asr_model`, falling back to `asr` when `model` is preferred.

### Broadcast scenarios: listen, parallel, and input_sent

In SFU Workspace broadcast scenarios the response appears on the other clients
in the room rather than on the sender. The runner provides four extensions for
that, each following the existing schema, validation, evidence, and timeout
conventions:

- `peer_stream.mode: listen` is a receive-only operation. It requires a Go
  duration `duration` (positive, at most 5m), pushes no input, and records every
  chunk the PeerStream delivers within that window. An Opus blob with any label
  counts as received audio, because SFU downlink labels identify the remote
  participant. The result exposes `audio_bytes`, `packets`, `events`,
  `streams`, `first_text_ms`, `first_transcript_ms`, `first_audio_ms`,
  `leading_silence_ms`, `leading_digital_silence_ms`, `last_event_ms`, `duration_ms`, `listened_ms`, bounded `text`, `audio_pacing`, and `/audio` in the same encoding as the
  existing `peer_stream` result (Ogg/Opus, present only when a `/audio` capture
  is declared and audio arrived, bounded by the output variable's
  `max_bytes`), so it feeds `server.speech.transcribe` directly. Receiving no
  audio is not an error; the document asserts `audio_bytes` with `expect`.
  `first_text_ms`, `first_transcript_ms`, and `first_audio_ms` are measured on
  the same clock — milliseconds from the moment the listen window opens, and 0
  when nothing of that kind arrived — so an agent-initiated greeting
  (`conversation.initiative: CONVERSATION_PARAMETERS_INITIATIVE_AGENT`) can gate
  its first-text latency as well as its first-audio latency. Only an explicit
  `transcript` label counts as a transcript; every other text fragment counts
  toward `first_text_ms`, matching how any Opus blob counts as received audio.
  A listen may sit in the same `parallel` step as the speaking client's own
  turn: both children share that connection's single Peer Event Stream
  subscription, and the speaker's own audio never comes back
  (mix-minus-self), so the sender asserts `audio_bytes` equal to 0. Listen
  cannot set `input`, `pacing`, `interrupt_after`, `idle_timeout`,
  `completion`, `terminal_label`, `require_text`, `require_audio`,
  `wait_for_history`, `session`, `keep_open`, or `await_rearm`. The step fails
  when the PeerStream closes before the window ends, when the step or document
  timeout expires, or when a terminal error arrives.
- SFU send probes use `measure_first_packet: true` with `completion: input_sent`
  to record the first nonempty Opus packet. Same-task listeners correlate its
  unique ID and sender, separating startup wait from send-to-receive latency.
  An empty array means no corresponding packet could be proved, not zero latency.
  See [CLI packet timing](/en/using/cli) for fields and monotonic clock boundaries.
  Regression tests cover holds, delivery delay, fan-out, repeated audio, lost/self/
  failed delivery, PCM preservation and the Schema contract.
- Step-level `parallel`. A `parallel` step owns a list of child steps, starts
  every one of them at the same moment, waits for all of them, and reports
  them together, so "one client speaks while another listens" needs no
  separate synchronization step. A child is a plain step body: `client` plus
  exactly one `peer_stream` operation in any mode, between 2 and 16 of them,
  each with an `id` that is required and unique across the whole document. A
  child cannot declare `parallel`, `barrier`, `retry`, `timeout`, `save_as`,
  `capture`, `expect`, or `expect_error`, and cannot use a persistent
  peer_stream session: the assertions belong to the parallel step. A child may
  declare `delay`, a positive duration up to one minute, and then starts that
  long after the group is released instead of with it. That is the one way to
  place a child's window inside another child's activity, which a contention
  scenario needs: a speaker's own utterance opens on its first voiced frame,
  so a window that starts with the group measures the leading silence, when
  the speaker was still an ordinary listener. `delay` is rejected on any step
  that is not a parallel child, because a plain step already runs in order and
  a delay there would only be a sleep hiding a missing wait. The
  parallel step itself keeps `id`, `timeout`, `capture`, `expect`,
  `expect_error`, `retry`, and `save_as`, takes no `client`, and is not
  allowed in `finally`.
  The step's result is an object keyed by child `id`, so `capture` and
  `expect` address one child's result with a `/<child_id>/...` JSON pointer; a
  pointer that names no declared child is rejected by validation. The runner
  resolves every child's inputs on the task goroutine first, because
  `Variables` is not safe for concurrent use, and releases the children only
  once all of them are prepared; when one child fails to prepare, the step
  fails and no child starts.
  The step's `timeout` bounds the whole group: on expiry the runner cancels
  every child, waits up to 30s more for them to release their PeerStream, and
  reports each child's own outcome. A child that ignores cancellation is
  recorded as unfinished and still owns the task's shared clients, so the task
  ends there and reports it: `finally` steps and client teardown are skipped
  rather than run against a stream that is still in use.
  One failing child fails the parallel step, and the report's `children` array
  keeps every child's `status`, `duration_ms`, `error`, and evidence. Play
  mode does not support parallel steps, and a driver that cannot run steps
  concurrently does not list the `parallel` operation, so such a document is
  rejected or skipped by `validate` instead of failing at run time.
- `peer_stream.empty_input: true` is valid for `push-to-talk` only. It sends
  control-only BOS/EOS on one StreamID without opening an audio channel or
  sending frames, matching a device released before its first captured frame.
  It cannot combine with `input`, `require_text`, `require_audio`,
  `interrupt_after`, or `completion: first_response`. Default completion
  observes the `idle_timeout` quiet window after sending (250 ms when omitted)
  and rejects assistant text or audio. No reply EOS is required.
  `completion: input_sent` only confirms delivery of the input.
- `peer_stream.trim_trailing_silence: true` is valid for `push-to-talk` with
  audio `input` only: before the turn is sent, the runner drops every Opus
  packet after the last voiced one (decoded peak of about -30 dBFS or more),
  so the EOS follows the last word the way a device ends a turn when the key
  is released as the word ends. Synthesized input otherwise ends with a
  decaying tail and silence that a device never sends. It cannot be combined
  with `empty_input` or `overlap_input`, and input without a voiced packet
  fails the step.
- `peer_stream.completion: input_sent` is valid for `push-to-talk` and
  `realtime`: the step completes once the input is fully pushed (including the
  EOS for push-to-talk) without waiting for its own text or audio output and
  without a terminal label. It cannot be combined with `first_text_timeout`,
  `first_audio_timeout`, `wait_for_history`, `require_text`, `require_audio`,
  `interrupt_after`, or `terminal_label`. The result adds `input_sent`,
  `input_packets`, `input_ms` (the Opus duration of the declared input),
  `pushed_packets` (realtime includes the tail silence), and `input_sent_ms`,
  and keeps counts of any output that happened to arrive while pushing.

```yaml
steps:
  - id: alice_speaks
    timeout: 20s
    parallel:
      - id: bob_listen
        client: bob
        peer_stream:
          mode: listen
          duration: 8s
      - id: alice_speak
        client: alice
        peer_stream:
          mode: push-to-talk
          input: ${alice_audio}
          completion: input_sent
    capture:
      bob_received: /bob_listen/audio
    expect:
      /bob_listen/audio_bytes: {minimum: 2000}
      /bob_listen/packets: {minimum: 50}
      /alice_speak/input_sent: {equals: true}
  - id: bob_transcript
    client: bob
    speech:
      method: server.speech.transcribe
      request: {model_name: asr, language: zh-CN}
      input: ${bob_received}
    expect:
      /transcript: {contains: 今天的天气非常好, normalize: [case, punctuation, whitespace]}
```

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
interruption scenarios for Realtime, Realtime Duplex, Eino, and
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

For focused live-provider validation of the Seed V2 SDK or adapter, use the fixed entrypoint:

```sh
bash tests/genx-e2e/run_seed_v2_tests.sh
```

Run it on a trusted development host or protected runner with Doubao Speech access and the
same complete credential inventory. It runs normal Seed V2 synthesis and repeated interruption
with `-race -count=1`, checking real audio BOS/data/EOS, replacement-route ordering after two
interruptions, and clean completion of the final turn. Credential, provider, network, timeout,
and race errors fail the command; arguments and environment variables cannot change test
selection. Deterministic truncation, empty audio, and structured error fields remain covered
by the `doubaotts` fake-server regressions.

The MiniMax API key must be paired with the voice base URL for the same region;
the runner does not substitute a default region when
`GIZCLAW_GENX_E2E_MINIMAX_BASE_URL` is missing.

Live calls cover usage metering: `usage_test.go` requires the OpenAI and Gemini
(`GIZCLAW_GENX_E2E_GEMINI_API_KEY`) Generators' streamed and structured calls,
Volc realtime dialog, realtime duplex, AST, and DashScope realtime to report
token records with the correct provider, model, and unit; the realtime tests
keep the session open until the provider's usage arrives. The Volc ASR, Seed V2
TTS, and MiniMax TTS provider tests also assert the millisecond and character
records, and Seed V2's billed characters must equal the Unicode character count
of the synthesized text.

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
`memory.Store` implementations. It does not use Eino's evaluator and is
not part of ordinary `go test ./...`, Docker E2E, or required CI. Each live Go
test owns its complete provider, memory-lane, and extraction configuration.
Volc remote project configuration remains deployment state and the harness
does not mutate it.

Current lanes cover Eino Redis 8 BM25 single-pass, hybrid single/two-pass,
self-hosted Mem0 with Qdrant/pgvector, Mem0 Platform default/custom-instructions, and Volc AgentKit
Memory default/request custom instructions. LoCoMo is a tagged Go test package. The Docker runner starts the
pinned Redis 8 service, self-hosted Mem0 and its backend for the selected group, runs
the tagged Go tests from the host against those containers, and always removes
their containers and volumes. The Mem0 Platform and Volc groups continue to use
standard `go test -run` against their remote providers.
Selected tests validate only the environment variables they consume. Missing
or placeholder values fail, and unselected backend variables are not inspected:

```sh
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run '^TestLoCoMoVolcAgentKit(Default|ProtocolSmoke)$' ./tests/locomo-e2e
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run '^TestLoCoMoMem0Platform' ./tests/locomo-e2e
tests/locomo-e2e/run_docker.sh mem0
tests/locomo-e2e/run_docker.sh mem0-pgvector
tests/locomo-e2e/run_docker.sh all
```

Use `.env.example` as a variable inventory and inject values through the
process environment; the test package and runner do not read `.env` files.
For a controlled Volc comparison, select `TestLoCoMoVolcAgentKitCustomInstructions`
and set `GIZCLAW_LOCOMO_E2E_VOLC_CUSTOM_INSTRUCTIONS` to the same business extraction
instructions as the self-hosted service. Each inferred write sends request-local
`custom_instructions` and the same speaker/time text. Defaults are turn ingestion,
Top-K 50 and minimum F1 0.20. The managed backend's internal models, base templates
and extraction token budget remain provider-controlled. Every Volc benchmark
registers cleanup before writing, purges only its independent Scope after success
or failure, verifies it empty three consecutive times, and records cleanup in the
redacted report. Shared project strategies are not modified. VPC-only endpoints
require verified access through the project's VPC.

The Mem0 group uses the same extraction and embedding model/key/base-URL
environment variables as Eino. Both Qdrant and PGVector containers use the `cmd/mem0` service pinned to `mem0ai 2.2.1`, and
defaults to the domestic `deepseek-v4-flash` extractor/answer model through
`https://api.deepseek.com`
and `qwen3.7-text-embedding` with 1024 dimensions. Select the LLM adapter with
`GIZCLAW_LOCOMO_E2E_MODEL_PROVIDER`; supported values are `deepseek` and
`bytedance`. Keep `GIZCLAW_LOCOMO_E2E_EMBEDDING_DIMENSIONS` aligned with the
selected embedding service because Mem0 must create its Qdrant collection or PostgreSQL vector column with
the exact vector width.
Run a remote Mem0 Platform lane separately only when its endpoint, API key, and
configuration fingerprint are available; those credentials are not required by
the Docker groups. Direct Go test runs require the matching `GIZCLAW_LOCOMO_E2E_MEM0_SELF_HOSTED_URL`; the runner points both at its Docker
services. Override `GIZCLAW_LOCOMO_E2E_MEM0_PORT` when the default port is unavailable.

The LoCoMo runner prepares a standard Mem0 base for the actual Docker architecture.
Set `GIZCLAW_LOCOMO_E2E_MEM0_BASE_FLAVOR=cn` for the CN base. The PG lane sends
its controlled business instruction through each Observe request as `prompt`,
matching MemoryLayout delivery. Override it with
`GIZCLAW_LOCOMO_E2E_MEM0_CUSTOM_INSTRUCTIONS`; reports fingerprint that request
policy. The service owns model/storage configuration and no business Layout registry.

The `mem0` group uses embedded Qdrant. `mem0-pgvector` starts Mem0 and an independent PostgreSQL 17/pgvector container, exposes only the Mem0 HTTP port on host loopback (default `18001`), and publishes no database port. Its test requires the health response's `vector_store` to be `pgvector` before running the same real extraction, recall, and question-answering evaluation. Missing database configuration or failed initialization never falls back to Qdrant. Direct Go tests use `GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_URL`; override the Docker HTTP port with `GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_PORT`. The report profile is `mem0_self_hosted_pgvector`. `all` includes both Mem0 backends. Database credentials are fixed temporary Docker fixtures; database data and SQLite history are removed when the runner exits. This lane does not use cloud PostgreSQL.

The PG lane defaults to an 8192-token extraction output budget, overridden by `GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_MAX_TOKENS`. The test reads the service's actual budget and includes it in its fingerprint and report. The service logs only response metadata such as finish reason, JSON validity, and candidate count to diagnose truncation or empty extraction without logging model content. The Qdrant lane retains its original 2000-token default.

Model names must be available to the selected account. Override `GIZCLAW_LOCOMO_E2E_EXTRACTION_MODEL` and `GIZCLAW_LOCOMO_E2E_ANSWER_MODEL` separately. For an account supporting the non-thinking Flash compatibility alias `deepseek-chat`:

```sh
GIZCLAW_LOCOMO_E2E_EXTRACTION_MODEL=deepseek-chat \
GIZCLAW_LOCOMO_E2E_ANSWER_MODEL=deepseek-flash \
  tests/locomo-e2e/run_docker.sh mem0-pgvector
```

Set `GIZCLAW_LOCOMO_E2E_MEM0_LLM_API_KEY` and
`GIZCLAW_LOCOMO_E2E_MEM0_LLM_BASE_URL` to override only Mem0 extraction. Empty
values retain the shared model key/base URL. Answer generation continues to use
`GIZCLAW_LOCOMO_E2E_MODEL_*`, allowing an extraction-only comparison.
`GIZCLAW_LOCOMO_E2E_MEM0_LLM_THINKING` accepts `enabled`, `disabled`, or an empty
value. Nonempty values are forwarded as `thinking.type` in the OpenAI-compatible
request; empty values preserve the model's default behavior. Set this only when
the model API supports the field. For example, use an enabled Doubao Seed 2.1
Turbo endpoint for Mem0 extraction while retaining DeepSeek answers:

```sh
GIZCLAW_LOCOMO_E2E_MEM0_LLM_API_KEY="$GIZCLAW_VOLC_ARK_API_KEY" \
GIZCLAW_LOCOMO_E2E_MEM0_LLM_BASE_URL=https://ark.cn-beijing.volces.com/api/v3 \
GIZCLAW_LOCOMO_E2E_EXTRACTION_MODEL=doubao-seed-2-1-turbo-260628 \
GIZCLAW_LOCOMO_E2E_MEM0_LLM_THINKING=disabled \
GIZCLAW_LOCOMO_E2E_ANSWER_MODEL=deepseek-flash \
  tests/locomo-e2e/run_docker.sh mem0-pgvector
```

The PG lane reads the actual extraction model, Mem0 LLM provider, thinking mode,
and token budget from health, verifies the declared model, and records them in
the report and fingerprint. The report's `provider` identifies the answer model
adapter; `extraction_provider` identifies Mem0's LLM adapter (`openai` for an
OpenAI-compatible API).

The Docker runner defaults to a 60-minute package timeout (`GIZCLAW_LOCOMO_E2E_TEST_TIMEOUT`) and bounded observation and
question stages. PGVector defaults to one production `memory.Store.Observe` per turn and Top-K 50; other lanes retain session ingestion and Top-K 10. Override these with `GIZCLAW_LOCOMO_E2E_OBSERVATION_GRANULARITY=turn|session` and `GIZCLAW_LOCOMO_E2E_TOP_K`. Empty turns are allowed, but each selected session must still satisfy the fixture fact minimum. The self-hosted adapter includes speaker names and UTC source times in extraction text. Fixtures assign roles consistently by speaker and retain image query/caption text. PGVector uses fixed two-person extraction instructions and records their hash. Extraction errors, incomplete/invalid JSON, and reported facts missing from storage fail instead of becoming valid empty writes. The runner
recalls for every question, asks the configured model to answer, and computes
EM, F1, evidence-hit, and adversarial-rejection metrics locally. Only answerable
questions contribute to EM/F1 and evidence-hit. Category 5 accepts the exact
normalized rejections `unknown`, `not mentioned`, and
`no information available`. PGVector requires aggregate F1 of at least `0.20`; other lanes retain `0.05`, evidence hit rate of at least `0.50` for evidence-aware
stores, and one materialized fact per selected session. Provider failures and
timeouts remain failures. Ignored `reports/` output contains IDs, scores, and
timings, but no conversation, question, answer, prediction, or recalled text.

Regression checks have two layers. `tests/locomo-e2e/run_regression.sh` requires no model credentials: it runs Go fixture/scoring/regression tests, Python HTTP/config tests, and real PostgreSQL checks for known-vector similarity conversion, filtering/ranking, and scope isolation. CI runs this entrypoint as `Mem0 PGVector Regression`; passing it does not establish live model quality.

Doubao `doubao-embedding-vision-251215` accepts text through Ark `/embeddings/multimodal`. Set `GIZCLAW_LOCOMO_E2E_MEM0_EMBEDDING_PROTOCOL=ark_multimodal` and explicitly provide the Ark embedding key/base URL, model, and 1024 or 2048 dimensions. Each text is one request, preserving one vector per batch input. Corpus/query instructions differ and their fingerprint enters reports. Provider errors or invalid vectors cannot become successful empty writes. Changing embedders requires regenerating vectors in a separate collection; Qwen and Doubao vectors cannot be mixed.

Run complete conv-30 quality evaluation explicitly on the local host with `tests/locomo-e2e/run_docker.sh mem0-pgvector`, configuring model credentials as described here and retaining redacted JSON. CI neither calls real models nor provides a manual trigger for live model evaluation. Conv-30 is not the complete ten-conversation LoCoMo benchmark.

Self-hosted LoCoMo uses `sdk/go/mem0` health checks and the production adapter
with generated request DTOs/HTTP clients. PG instructions travel as request-local
`prompt`. The separate load test requires real extraction, at least one persisted
ADD and nonempty semantic reads; direct import or vector inserts do not qualify.
Start a dedicated test Mem0/PG instance and set its URL:

```sh
GIZCLAW_MEM0_LOAD=1 \
GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_URL=http://127.0.0.1:18001 \
go test -tags gizclaw_locomo_e2e -timeout 20m -count=1 -v \
  -run '^TestMem0SDKConcurrentLoad$' ./tests/locomo-e2e
```

Defaults offer 32 writes/s and 12 reads/s, warm up for 10 seconds and measure
60 seconds. Acceptance counts actual successful completions within that window
against 30 writes/s + 10 reads/s; errors in any phase fail. Override the matching
`GIZCLAW_MEM0_LOAD_WRITE_RPS`, `READ_RPS`, `SECONDS` and `WARMUP_SECONDS`
variables with the same prefix. Each write gets an independent synthetic
Workspace Scope and approximately 1000 input tokens; reads use a preseeded scope.
Reports contain timing, completion and fact counts without dialogue, model
answers or credentials. Cleanup only purges this run's generated scopes and
verifies they are empty. This qualifies aggregate throughput; one Scope's writes
remain serialized. Model RPM/TPM, remote latency and PG pooling can limit capacity.

Live reports record SDK/models, the requested service tier, extraction-policy and answer-prompt fingerprints, ingestion granularity, Top-K, category metrics, and question IDs. Set `GIZCLAW_LOCOMO_E2E_BASELINE_REPORT` to a prior redacted report to reject F1 drops greater than `0.02`, provenance-hit drops greater than `0.05`, or any adversarial-rejection decline. Category F1 drops cannot exceed `max(0.10, MAX_F1_DROP)`. Override tolerances through the matching `MAX_*_DROP` variables. Different datasets, models, prompts, budgets, or ingestion units, and incomplete/error-bearing reports are incompatible; SDK upgrades and service-tier changes remain comparable for quality. Throughput requires separate validation.

```sh
# Deterministic checks; no model credentials required.
tests/locomo-e2e/run_regression.sh
# Live quality regression; retain the baseline model settings.
GIZCLAW_LOCOMO_E2E_BASELINE_REPORT=/path/to/prior-report.json \
  tests/locomo-e2e/run_docker.sh mem0-pgvector
```

Token F1 and observation-provenance hit are repository metrics, not the official permissive LLM judge score. Provenance identifies the Observe input; Mem0 provides no exact per-fact citations, so this is not a manually verified evidence-recall rate.

### Dataset and license

`testdata/locomo10_smoke.jsonl` is a Git LFS object adapted for noncommercial
use from SNAP Research's LoCoMo `locomo10.json`. It contains the first three
sessions of `conv-30` plus session 1 of `conv-26` (76 turns total) and eight
questions across categories 1 through 5. It is a contract smoke set, not a
full benchmark. Exact
upstream commit, checksum, subset, and transformation information is recorded
in `locomo10_smoke.manifest.json`.

The upstream project publishes one `data/locomo10.json` file rather than a
`testdata` directory or one file per conversation. This repository's
`testdata` directory contains derived, harness-native fixtures. The optional
`locomo10_conv30.jsonl` fixture is the complete `conv-30` tier: 19 sessions,
369 turns, and 105 questions. Select it without changing the smoke default:

```sh
GIZCLAW_LOCOMO_E2E_DATASET=tests/locomo-e2e/testdata/locomo10_conv30.jsonl \
```

`tests/locomo-e2e/cmd/fixturegen` reproducibly converts one conversation from
the pinned upstream JSON into the JSONL contract and writes its manifest. The
generator requires the expected upstream SHA-256 and refuses drift; each
manifest records the source commit/hash, selected IDs, transformation, license,
and derived hash. The JSONL fixtures are stored with Git LFS because they
contain upstream dataset content.

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


## OpenAI Conversations and Responses E2E

The standard GizClaw Docker runner owns a mandatory `go:openai` phase under `tests/gizclaw-e2e/go/openai`. It uses the pinned official OpenAI Go SDK over authenticated `ServicePeerOpenAI`, creates an isolated Peer-owned Conversation Workspace, completes three text turns, composes transcription to Response to speech, exercises background cancel and streamed-client abort followed by same-Conversation recovery, and registers Workspace cleanup before mutation.

Successful runs write redacted monotonic timing evidence below ignored `tests/gizclaw-e2e/testdata/openai-compatibility/`. Artifacts contain only schema/version, target/case, bounded media sizes, numeric phase timings, and status; they must not contain credentials, IDs, prompts, transcripts, generated text, media, URLs, or provider errors. A tagged compile is diagnostic only and does not replace `bash tests/gizclaw-e2e/run_tests.sh`.

`TestAssistantScenariosWithLiveModel` in the same phase reuses that harness's API key and `/openai/v1` to run `web/assistant/scripts/run-live-scenarios.ts` with `node --experimental-strip-types`: the Monitor diagnostic assistant's scenario set executes tools against `FakeRuntime` while every model call goes to the RuntimeProfile `llm` (`doubao-lite-chat`). Each scenario gets up to three attempts and is judged only on tool calls, the final route, and key facts in the reply; a scenario that fails all three fails the phase. A passing run prints only scenario names, attempts, and failed checks; the JSON report with generated replies and tool results is printed only on failure. The test needs the `web/assistant` dependencies installed by the root `npm ci`, which the runner's `preflight:npm-ci` phase provides.

## Deterministic multi-role audio Giztest

```sh
go test ./cmd/internal/commands/giztest -run '^Test(EinoMultiVoiceGiztest|MixedProviderSpeakerVoicesGiztest|EinoBranchMultiVoiceGiztest|MultiRole.*)$' -count=1
```

The suite shares the fake provider in `voice_fixture_test.go` while retaining real
Eino factories, AudioDock, the Go Giztest runner and CLI receiver. No
external network, credentials or Docker are required. Audioplayer Giztest runs the
whole suite after building Console assets, reusing its audio test environment.

- `eino-voices/multi-turn.giztest.yaml` runs four Eino turns whose Starlark selector
  prefixes each reply with a `【speaker】` marker selected through `speaker_voices`.
  `multi-role-voices/multi-turn.giztest.yaml` runs fox, bird, owl, bear, unknown
  (default fallback), bear, bear and fox in one invocation. Four Eino publisher
  nodes use `node_voices`. Distinct deterministic payloads for every turn detect
  stale TTS audio even across consecutive turns with the same role.
- Per-turn `audio_integrity/sha256` verifies payload and order; `streams=1`,
  `max_active=1`, `open=0` and `violations=0` check BOS/EOS, interleaving and late
  data. Session-wide ownership and TTS call counts also verify that ordinary
  multi-turn synthesis has at most one active call.
- `TestMixedProviderSpeakerVoicesGiztest` runs `eino-voices/mixed-providers` and
  `multi-role-voices/mixed-providers` (Eino): one reply whose narrator Voice
  natively returns Ogg/Opus and whose `【fox】` Voice natively returns MP3,
  through AgentHost with Workspace History. It requires text and audio EOS,
  `streams=1` and the digest of both segments as one `audio/ogg` stream, checks
  that every Voice was asked for `format=ogg_opus`, and that History stores the
  reply text with both segments' Opus packets in order. The `ignore-format`
  fault keeps the MP3 output and must fail with AudioDock's mixed audio MIME
  error.
- `audio_pacing` requires 40 packets of 20 ms, a maximum interval of 150 ms, no
  underruns with a 500 ms prebuffer and a nonnegative minimum buffer. Both
  workflows have `long-reply.giztest.yaml` cases with over 2 KB of text and 160
  packets. Fake TTS injects repeatable 15/25/20 ms jitter; a 900 ms stall must fail.
- `TestMultiRoleVoiceModesGiztest` sends real Opus input to fake ASR in push-to-talk
  (transcribe after input EOS) and realtime (final transcript while input remains
  open) modes. This validates provider boundaries and mode wiring, not real ASR/VAD.
- `TestMultiRoleVoiceInterrupt` sends B after receiving A's eighth packet while
  TTS remains active, for both workflows and input modes. It verifies A's valid
  audio prefix and B's complete 40 packets and digest, with no cross-role overlap.
  Both text/plain and audio/opus routes are tracked by StreamID and MIME route:
  A must have interrupted EOS on both routes before B is accepted, and B must
  have normal EOS on both routes. Any A chunk after B starts, including empty
  chunks and EOS, fails. `TestMultiRoleVoiceInterruptAssertions` independently
  rejects missing A text interrupted EOS, late A text after B starts, and missing
  B text EOS, alongside a complete dual-route positive case.
- Fault tests reject wrong voice, interleave, stall and truncation without EOS.
  AudioDock rejects truncation with `TTS ended without EOS`. Independent
  `TestMultiRoleVoiceAssertions` cases feed raw packet traces to individual digest,
  maximum-active, open-stream and late-packet expectations, plus the 150/151 ms
  interval and 500/501 ms buffer boundaries. One failing expectation cannot mask
  a broken sibling expectation.

Rapid input shares the interruption test above: a new input BOS superseding the
previous reply is the AudioDock/Eino barge-in contract. The test positively
asserts a valid A prefix and interrupted EOS on both routes, all 40/40 B packets
and normal EOS on both routes, no A chunks after B starts, and `max_active=1`, without a duplicate scenario. This provider-boundary
suite does not qualify real voice identity, Server/Edge/WebRTC pacing or device
playback.

## Monitor API giztest

```sh
bash tests/gizclaw-e2e/run_monitor_tests.sh
```

Requires Docker, Node/npm and Python 3. The runner uses `gizclaw-go:linux-<arch>-cn-base` for the Docker architecture, building it from `build/gizclaw/Dockerfile.cn.base` if absent. Override it with `GIZCLAW_E2E_DOCKER_BASE_IMAGE`. Go modules and build outputs use dedicated Docker cache volumes; `GIZCLAW_MONITOR_MODCACHE` and `GIZCLAW_MONITOR_BUILDCACHE` accept volume names or absolute paths.

The runner generates identities, starts isolated Server/Edge processes, seeds a Workflow and runs `tests/gizclaw-e2e/giztest/server.monitor.*.giztest.yaml`:

- authorization: runtime debug access, revocation and invalid input.
- history: real WebRTC text conversations persisted before workspace lookup, search, pagination and cross-peer isolation.
- logs: persisted HTTP completion records, pagination and cursor binding.
- audio: authenticated retained Ogg download, missing credentials and missing assets.
- node: independent Monitor Token authentication, rejection of device public keys and local node metrics.

SQLite, filesystem assets and a script Workflow avoid external model dependencies. The audio fixture writes an Ogg asset into the real History/Asset Store; this is not a TTS synthesis test. Containers and temporary runtime data are removed on exit; reports remain under `.testbench/monitor-*/reports/`. This lane neither reads cloud E2E credentials nor validates cloud TLS IAM permissions.

### Terraform provider

`bash tests/gizclaw-e2e/run_terraform_provider_tests.sh` starts an isolated real Server from the shared workspace fixture and needs no model/provider credentials, only a `terraform` CLI on `PATH` (or named by `GIZCLAW_E2E_TERRAFORM`). It builds `terraform-provider-gizclaw` with `CGO_ENABLED=0` into a filesystem mirror and installs it through `provider_installation` with `terraform init`. Resources are checked on the Server with `gizclaw admin show`. It runs two tests:

- `TestTerraformProviderAppliesCatalogSelection` resolves `gizclaw_catalog` from the layered fixtures under `tests/gizclaw-e2e/terraform/testdata` and applies the selection with `gizclaw_resource`. It checks override precedence, unselected entries, raid testers, `overridden_ids`, and `raids`, then a no-change refresh plan, a catalog edit, deselection deletes, and `terraform destroy`.
- `TestTerraformProviderResourceLifecycle` manages one resource of every Admin-appliable kind: Credential, the six provider Tenants, Model, Voice, MemoryLayout, Workflow, Tool, Firmware, RuntimeProfile, RegistrationToken, Contact, Friend, FriendGroup, FriendGroupMember, and FriendGroupInviteToken. Every kind goes through create, update, a no-change refresh, and import. Refresh must detect out-of-band changes (Credential specs, which the Server never returns, stay configured) and out-of-band deletes, and apply restores them. The test also covers `input_revision`, explicit `api_version`, replacement on `resource_id` change, environment placeholder expansion and a missing variable, provider-side and Server-side rejections, a Server restart, idempotent destroy, version 0 state upgrade, and `gizclaw_catalog` with the Server stopped.

The Server gets a placeholder SFU URL so Friend and Friend Group resources can bind their Room identity; nothing listens there. Workspace is Peer-owned and only its Admin rejection is checked. The `endpoint` override is not exercised because it requires https. The dedicated CI job and the full gate run this same entrypoint.

### GNSS reporting invoke Giztest

`bash tests/gizclaw-e2e/run_gnss_reporting_tests.sh` builds the Go, JavaScript, C and native
Flutter runners and executes five `server.device.gnss.reporting.*` scenarios against temporary
SQLite state, a real Server/Edge and WebRTC. Every runner must execute all 44 regular steps
and 6 cleanup steps. Missing runners, skipped steps or failed cleanup fail the lane. The
ordinary Go tests run the Go lane.

The scenarios cover true/false and repeated set requests, API-key owner routing to separate
devices, preserving device-provided values, rejecting missing or invalid booleans before
reverse RPC, unavailable procedures, device rejection and a response without `enabled`.
`client_rpc` simulates device responses only. Real SDKs encode and transport the calls, and
the real Server performs authentication and result validation. Firmware owns defaults,
persistence and actual GNSS reporting behavior.

Run the Go lane alone:

```sh
go test ./cmd/internal/server -run '^TestGNSSReportingGiztestGo$' -count=1
```

### Audioplayer Giztest

`bash tests/gizclaw-e2e/run_audioplayer_tests.sh` starts an isolated real Server and Edge with SQLite runtime storage and no model/provider credentials. It runs the six `server.device.audioplayer.*` scenarios and always cleans up its containers and ephemeral identities; reports remain under the ignored `.testbench` directory. The dedicated CI job runs this same entrypoint.

Scripted device providers test HTTP authorization, validation, reverse RPC, playlist contracts and snapshot projection; they do not download or play music. The five control scenarios are supported by Go, JavaScript, Flutter and C runners. The separate `telemetry` step sends a protobuf-JSON `frame` with the Go device SDK over the actual packet channel; other runners explicitly skip this operation. Packet acceptance is not persistence: the telemetry scenario polls `server.status.get`, then checks HTTP status for progress, errors, stale-observation protection and OTA coexistence. It does not add a Dart telemetry transport or claim hardware playback acceptance.

### Initial RTP/BOS ordering regression

`bash tests/gizclaw-e2e/run_rtp_bos_tests.sh` runs a real Server, Edge and Go
Giztest receiver on an isolated local Docker internal network, using ephemeral
identities and SQLite without provider credentials. The `testdata/rtp-bos/` build
overlay supplies deterministic ASR/TTS with BOS attached to the first audio frame,
and delays receiver audio BOS processing by 200 ms while RTP reading continues.
The scenario checks complete audio, BOS/EOS, one active stream, zero integrity
violations and packet pacing. The Audioplayer CI job runs the same entry point
and uploads `.testbench/rtp-bos-*/reports/`. This is not a cloud model performance
or physical device acceptance test.

## Local slow TTS regression

`bash tests/gizclaw-e2e/run_slow_tts_tests.sh` runs a real Server, Edge and Go Peer
on an internal Docker network with ephemeral identities and SQLite. It reads no
provider credentials. A Go build overlay substitutes only the default peergenx
provider builder; Workflow factories, AudioDock, AgentHost, WebRTC, first-response
timing and the audio receiver use the revision under test. The ASR fixture emits
a fixed transcript from input audio. TTS waits 12 seconds at startup, announces an empty
audio BOS, waits 200 ms for synthesis, then emits 80 valid 20 ms Opus frames. Context cancellation bounds
these delays.

`slow-tts.*.giztest.yaml` covers Eino push-to-talk, Eino realtime and Eino
realtime. First-response steps retain the 2-second text deadline; a separate
Peer with the same workflow checks text/audio EOS, nonempty audio, overlap and pacing. Realtime
turns reuse the session to replace input during earlier TTS startup. CI runs this
suite in the Audioplayer Giztest job. The standard provider-backed runner excludes
these dedicated fixtures. Reports remain in `.testbench/slow-tts-*/reports/`; exit
cleanup removes containers, the image and temporary runtime state.

### Client teardown and output acknowledgement regression

`bash tests/gizclaw-e2e/run_observer_lifecycle_tests.sh` starts a real local Server, Edge and Go Giztest in an internal Docker network, using temporary Peer identities, SQLite and deterministic streaming Generator/ASR/TTS providers. The same first-response close/replacement document runs three times. Separate cases keep a realtime stream open before disconnect/reconnect, repeated `server.run.stop`, active Workspace deletion and Peer deletion. Normal-completion controls check text/audio EOS and delivered Workspace History.

The build overlay replaces only providers, records full test public identities and shortens the native profiler cadence to two seconds. Stream handling and RPC lifecycle use the tested implementation. Pending-deletion scanning runs every second to verify asynchronous local cleanup. Reports verify goroutine/heap/allocs manifest sizes and SHA-256, check task-end and delayed Eino observer/execute waits, record total goroutine counts, and join SQLite Workspaces, graph states and retirement markers by test Peer ownership. This lane does not qualify Mem0 scopes, PostgreSQL, Redis, all filesystem objects or physical Peer-run row reclamation.

Reports and identity mappings remain in the permission-restricted ignored `.testbench/observer-lifecycle-*/reports/` directory. Teardown removes the project's containers, image and temporary runtime state. The lane reads no live provider credentials.

### Speaker segment regression

Deterministic tests use distinguishable Opus voices to verify five ordered segments, exact audio digests, stripped text, a single stream and `audio_pacing.underruns=0`. The Docker runner starts local Server/Edge on an internal network with a test-only provider overlay and runs both Eino Graph configurations without credentials. Containers are cleaned up; reports remain under `.testbench/speaker-segments-*/reports/`. The Audioplayer Giztest CI job runs both gates.

```sh
go test ./cmd/internal/commands/giztest -run '^TestSpeakerSegmentsGiztest$' -count=1
bash tests/gizclaw-e2e/run_speaker_segment_tests.sh
```

The standard provider-backed Giztest phase also runs `eino-speaker-voices.text-roundtrip` and `eino-speaker-sequence.text-roundtrip` with real credentials. The real LLM repeats a marked script, and Volc TTS speaks it with the `narrator`, `assistant-voice` and `story-bird` aliases. The scenarios assert that configured markers are stripped while unknown markers remain, that audio arrives as one stream with no violations and `audio_pacing.underruns=0`, and that ASR transcribes the segment content in order. Run them alone against a started Docker stack:

```sh
tests/gizclaw-e2e/testdata/bin/gizclaw test run \
  tests/gizclaw-e2e/giztest/eino-speaker-voices.text-roundtrip.giztest.yaml \
  tests/gizclaw-e2e/giztest/eino-speaker-sequence.text-roundtrip.giztest.yaml --parallel 2
```

`eino-mixed-provider-voices.text-roundtrip` and `eino-mixed-speaker-sequence.text-roundtrip` also run in that phase. The narrator is the Volc `narrator` Voice and the `【弟弟】` character is the MiniMax CN system Voice `minimax-boy` (`speech-2.6-turbo`, no `provider_data.format` override), so one reply mixes providers whose Voices default to different formats. The scenarios check that the MiniMax Voice synthesizes with the test account, that the reply ends with text and audio EOS as one audio stream (`streams=1`, `max_active=1`, `open=0`, `violations=0`), that ASR hears the segments in order, and that Workspace History stores the reply as a replayable agent entry. History requests explicitly use descending order; the Go runner unwraps the protobuf `value`, so assertions use `/available`, `/items/0/type`, `/items/0/replay_available`, and `/items/0/text`. The three-segment script retains a 20 KB audio minimum; transcription uses `森林.*苹果.*日出` to check content order while allowing ASR punctuation differences instead of requiring verbatim text. These scenarios do not impose a playback-underrun threshold on live providers. JS, C, and Flutter runners explicitly skip these audio documents, as they do the single-provider precedent. They need the MiniMax CN credential from `tests/gizclaw-e2e/.env`; the `minimax-cn` tenant uses `https://api.minimaxi.com`. Run them alone against a started Docker stack:

```sh
tests/gizclaw-e2e/testdata/bin/gizclaw test run \
  tests/gizclaw-e2e/giztest/eino-mixed-provider-voices.text-roundtrip.giztest.yaml \
  tests/gizclaw-e2e/giztest/eino-mixed-speaker-sequence.text-roundtrip.giztest.yaml --parallel 2
```

Three speech-rate scenarios also run in that phase. `server.run.workspace.reload` carries the RPC contract: it sends `tts_speech_rate_percent: 80` with `reload-with-options` and confirms the stored parameter with `server.workspace.get`, so the JS, C, and Flutter runners exercise that encoding too. `eino-voice-assistant.tts-speech-rate` covers storage, out-of-range rejection (`INVALID_ARGUMENT`), and a real reply spoken at the rate; `dashscope-realtime-conversation.tts-speech-rate` covers a provider without a native rate still speaking through the transformer time-stretch.

`eino-mixed-provider-voices.tts-speech-rate` proves the rate takes effect: two Workspaces bound to `eino-mixed-provider-voices` read the same script, the reply at the Workflow rate asserts 700..1400 audio packets (20 ms each, about 14..28 seconds), and the Workspace set to 60% asserts at least 1600 packets (about 32 seconds). Measured runs produce about 1116..1122 and 1836..1892 packets, leaving more than 20% margin on both bounds. Both the Volc narrator and the MiniMax character segment slow down. JS, C, and Flutter runners skip this scenario as well:

```sh
tests/gizclaw-e2e/testdata/bin/gizclaw test run \
  tests/gizclaw-e2e/giztest/eino-mixed-provider-voices.tts-speech-rate.giztest.yaml
```

Offline safety-fence tests cover parameters, RPC, Profile SQL/revisions, and driver injection. The E2E RuntimeProfile fixture defines four independent complete prompts named `alpha`, `bravo`, `charlie`, and `delta`. `server.workspace.safety-fence.roundtrip.giztest.yaml` covers RPC discovery, successful reload without a selection, custom identifier roundtrip, and malformed values; `server.device.runtime_profile.get.giztest.yaml` covers HTTP discovery without exposing prompts; `server.workspace.safety-fence.missing-profile.giztest.yaml` verifies that reload fails for an undefined `child` entry; and `sfu.workspace.switch.giztest.yaml` verifies valid-identifier no-op behavior. Workspace Go tests verify Admin put 400 because Giztest ephemeral Peer connections have no Admin HTTP permission. The five `safety-fence-*.giztest.yaml` scenarios cover explicit prompt injection for Eino, and three Realtime drivers.

Run these nine scenarios with the dedicated minimal resource catalog in `testdata/resources/safety-fence/`, an isolated Docker project, and standard E2E provider credentials:

```sh
GIZCLAW_E2E_CREDENTIAL_FILE=tests/gizclaw-e2e/.env \
  bash tests/gizclaw-e2e/run_safety_fence_tests.sh
```

The script writes separate RPC, provider, HTTP, and SFU JSON reports under `tests/gizclaw-e2e/.testbench/` and removes its project containers and temporary credential environment on exit. The dedicated entrypoint selects only the dependencies of these nine scenarios instead of the full standard resource catalog. Offline parsing does not prove real provider behavior.


For quota protocol, RuntimeProfile configuration and real Docker fixture acceptance, see [Quota](/en/developing/api/http/quota).

The quota Docker lane additionally checks complete HTTP chat/Responses rejection envelopes and real SDK Workspace dialogue over Edge/Server. Its Eino helpers observe exhausted and unavailable error EOS, exact public details, response identity, and no denied provider I/O. These deterministic protocol fixtures do not claim live cloud-provider or hardware qualification.

## Screenplay quality

`script-quality.*.giztest.yaml` runs 20 candidate replies for Werewolf, mystery,
poetry, journey and multi-role storytelling. An independent player Workspace
drives the dialogue; a separate judge rates character consistency, information
boundaries, responsiveness, progression and closure. Each criterion requires
3/4. Deterministic rule tests remain separate from model quality assessment.

Within its own Docker stack, the quality runner derives text configurations from these five native resources, removing only `voice_adapter` while preserving Graphs, deterministic rules, models and memory bindings. Scenario logic is not copied; other E2E/voice lanes retain the original resources. `text-workflows.json` records the configurations used.

```bash
bash tests/gizclaw-e2e/run_script_quality_tests.sh
GIZCLAW_E2E_SCRIPT_QUALITY_CASES="werewolf murder-mystery" \
  bash tests/gizclaw-e2e/run_script_quality_tests.sh
```

The runner accepts `GIZCLAW_E2E_CREDENTIAL_FILE`, starts and cleans its isolated
Docker project, and calls real providers. It does not merge or deploy business
configuration. Every selected case runs with its own JSON report; any quality or
execution failure produces a nonzero exit. Default artifacts live under ignored
`testdata/script-quality/`. The terminal summary contains scores and turns; full
reports contain dialogue and quoted judgments and should be handled as dialogue
content. Player/judge workflows use native Eino Prompt/ChatModel; the
Docker E2E Profile's text `llm`, judge `script-judge`, and audio `audio-llm`
aliases all select Seed 2.1 Lite (`doubao-seed-2-1-lite-260915`), request
`service_tier: fast`, and disable thinking by default. The `script-judge` alias
can still select the judge model independently. The live low-latency Giztest
and first-response matrix on this page verify the actual tier and latency.

## Runtime Tool aliases

Run the isolated native Server/Edge/Peer lane with the authorized provider
credential file; its contents remain process-local:

```bash
GIZCLAW_RUNTIME_TOOL_CREDENTIAL_FILE=/secure/gizclaw.env \
  bash tests/gizclaw-e2e/run_runtime_tool_tests.sh
```

The default matrix contains 428 native documents and 1280 tasks: 80 business
utterances, 20 actual-model dialogs, unpredictable results, long history and
target-isolation cases at 10/30/60/100 available Tools with three repetitions,
plus contract checks, an external HTTPS echo and two probes. Each task has an
independent Profile. Filter/repeat/smoke overrides are diagnostic subsets and
cannot replace full acceptance. Every failure and skipped or missing task is
retained under ignored `.testbench/runtime-tools-*/reports/`, with exact source
and binary hashes, source patch, inputs, decoded protocol receipts and reports.
If a container interruption produces no giztest.json, the summary is FAIL with every
expected task missing and its outcome unknown; it fabricates no execution or timing scores.

Only this lane's test Server container uses `223.5.5.5` and `119.29.29.29` as external DNS
forwarders. Docker service names still resolve within the project network; host, shared Docker
and production settings are not changed. DNS, connection and provider failures remain FAIL,
and neither resolver configuration nor a rerun overwrites an earlier failed receipt.

Business bindings select `verification_model`; the primary model still produces
native calls and clarification. Independent checks cover each mutating proposal, fixed-target MHS read
and final reply, with at most two corrections. Complete-parameter readiness
includes this execution cost. Final text is buffered for checking, so its first
chunk latency must be distinguished from unverified streaming. Semantic
misclassification, exhausted corrections and provider/session errors remain
failures. Mutation counts, fixed targets and parameters use exact native
assertions; a separate real model checks zero-action replies for false claims.

Program fixtures have independent Workflow/Workspace IDs and program system
content. After checking the acknowledged selection's enum, parameters and count,
the lane explicitly reloads and checks selected/active Workspace and `RUNNING`.
An ACK alone does not prove committed reload or physical script playback.
G02 executes an external HTTPS Tool and requires the model to use the random
value returned from a private fixed header, absent from user input. Positive
echo acceptance does not qualify every provider or credential-revocation path.
Focus fixtures update real Profile metadata; audio fixtures qualify protocol
state. Neither proves a production focus UI or physical audio playout.

G03 invokes `device.reboot` using a real owner API key and checks enum 4 and exact protobuf JSON parameters. During the acknowledged reboot transition, catalog and HTTP writes reject that owner while another Peer on the same Profile stays available. Both write counters must remain zero. This qualifies the Server transition and owner isolation without physical hardware.

G04 configures an 800-byte description for each of 100 genuinely available catalog entries, forcing a protobuf continuation envelope, then checks the entire catalog, descriptions and availability. Go Giztest unary RPC requires EOS and bounds a complete envelope to 16 maximum frames; truncated, mixed and oversized envelopes fail.

G05 directly plays indices 0→1→0 through an owner API key, checking enum 14, the effective index and cumulative decoded calls after each action. G06 first completes one real model playback turn, then issues two same-owner HTTP playback calls to prove the device channel remains available after a conversation. Go SDK inbound unary RPC releases its request transport on both success and failure; every request still owns a separate channel.

Native fixtures explicitly retain the absence of a default focus in lamp/screen catalog descriptions. With a configured focus, other objects are explicitly identified as non-default. This context grants no new user intent, and an existing pending target still takes precedence.

The lane hashes its runner, business inputs and Monitor templates alongside code; initialized private-key configuration is excluded from reports. Server runtime profiles use a dedicated ObjectStore, are captured every five minutes and are retained before temporary-state cleanup. Server and Edge container logs each retain up to 256 MiB, with 32 MiB for the control fixture, preserving failure timelines beyond default rotation. Profiles and logs support diagnosis and do not replace task receipts or prove acceptance.


## Lua application Giztest simulator

The Go Giztest runner installs a stateful device simulator through `client_rpc.response.lua_apps`; the three Lua procedures share one state. Configuration contains `capacity_bytes` (a 0–64 MiB budget for package files and update staging), `installed` (the initial installed-app catalog), and optional `packages` (HTTP(S) URLs mapped to base64-encoded, real `.lua-app.tar.zlib` archives). Initial catalog entries do not model built-in firmware files; dynamically installed files consume the capacity budget. URLs without an explicit fixture are fetched by HTTP(S) GET, with at most ten equally validated HTTP(S) redirects and request cancellation propagation. Both valid data URL forms are decoded incrementally into the same package validator, with a 256 KiB encoded limit including the prefix.

The simulator incrementally decodes zlib and USTAR, verifies the manifest, lengths and SHA-256 values, and rejects path traversal, links, duplicate files, truncation and trailing data. Insufficient capacity returns `UNIMPLEMENTED` and preserves the previous version and catalog. After successful installation, `list` reads the simulator's actual state. `run` checks that the app exists and records the received string parameters without executing a Lua VM. Requests are recorded only when `/requests` assertions need them, under the ordinary Giztest evidence contract.

This Go lane runs isolated Docker Server, Edge and real WebRTC Peer processes to cover installation, listing, launch, insufficient space, unsupported handlers, missing IDs and rejected parameters, without model credentials:

```sh
bash tests/gizclaw-e2e/run_lua_app_tests.sh
```

Scenarios are in `tests/gizclaw-e2e/testdata/lua-app/`; the simulator belongs to the Go runner. Go, JavaScript, Flutter and C have separate protobuf/handler regressions. Simulator and SDK success does not qualify downloads or game execution on physical hardware.

The real-model `LUA01` case in the Runtime Tool lane requests the Tetris game in single-player, easy mode. It requires application discovery followed by exactly one `lua.app.run`, with the received `app_id=tetris`, `mode=single` and `difficulty=easy`:

```sh
GIZCLAW_RUNTIME_TOOL_CASE_FILTER=LUA01 GIZCLAW_RUNTIME_TOOL_REPEAT=1 \
  bash tests/gizclaw-e2e/run_runtime_tool_tests.sh
```

This command uses the existing Runtime Tool credentials and retains the real model reply and decoded device receipts. It qualifies this selected subset, not the complete Runtime Tool pressure matrix.

## Live audio regression for original business Graphs

`TestRuntimeProfileAudioGiztests` is a local opt-in provider test. Start an isolated real Mem0/Pgvector service using `cmd/mem0/config.example.yaml`, and supply Ark, Doubao Speech/Search and the MiniMax CN/global credentials required by the original Voice bindings. The Deploy checkout must contain the Raids catalog, H106 Tiga Profile and original Giztest documents:

```sh
GIZCLAW_TEST_AUDIO_DEPLOY_ROOT=/path/to/deploy \
GIZCLAW_TEST_AUDIO_EVIDENCE="$(mktemp -d)" \
GIZCLAW_TEST_MEM0_ENDPOINT=http://127.0.0.1:8000 \
  go test -tags=gizclaw_provider_e2e ./cmd/internal/server \
  -run '^TestRuntimeProfileAudioGiztests$' -count=1 -timeout=20m -v
```

`GIZCLAW_E2E_MEM0_API_KEY` authenticates Mem0. Authenticated Admin HTTP binds real resources in a temporary SQLite Server. Only the local Profile selects native Lite and the test Mem0 service; the original multirole adventure, animal guessing, topic and main chat Workflows retain their business Graphs and prompts. Original Giztests exercise two audio turns, the text opening, reply EOS and cleanup over real WebRTC. Main chat keeps Doubao Realtime and additionally tests the same Workflow with external ASR for PTT, continuous realtime, interruption and next-turn recovery.

Evidence includes source Graph/document SHA-256, scenario reports, sanitized transparent Ark/Mem0 observations and provider usage. Observers forward real upstream responses. The native Lite phase requires successful audio transcription and text reply requests, positive usage and no independent BigASR usage; external ASR phases require actual ASR usage. This test incurs real provider charges and stays outside CI. It does not qualify physical devices or deployments. Never commit its reports or logs.
