# 测试与 E2E

本页说明仓库级测试 harness。普通 Go 单元测试仍按改动范围运行；带 build tag、
Docker、真实 provider 或人工判断的套件必须显式启动，不能把未运行记作通过。

## Store E2E

`tests/store-e2e` 通过导出的 Store API 验证 PostgreSQL 与 ClickHouse，不依赖
production package 的私有 test hook。目录内每个 Go 文件都使用 `store_e2e` build
tag，因此普通 `go test ./...` 不会选择这些测试或访问外部数据库。快速 SQLite
集成测试继续和对应 package 的单元测试放在同一个普通 `*_test.go` 文件中。

PostgreSQL 与 ClickHouse 测试分别使用 `TestPostgreSQL...` 和
`TestClickHouse...` 命名。CI 只选择当前 job 已启动的 backend；被选中的 backend
缺少 DSN 时必须失败，不能记为 skip：

```sh
GIZCLAW_TEST_POSTGRES_DSN='postgres://…' \
  go test -tags=store_e2e -count=1 -p 1 -run '^TestPostgreSQL' ./tests/store-e2e
GIZCLAW_TEST_CLICKHOUSE_DSN='clickhouse://…' \
  go test -tags=store_e2e -count=1 -p 1 -run '^TestClickHouse' ./tests/store-e2e
```

每个测试使用独立表名并尽力清理。错误、日志和 CI 输出不得打印 DSN、数据库
credential 或 Store payload。

### Cloud ObjectStore conformance

同一个 tagged package 包含 `TestObjectStore`。通过
`GIZCLAW_OBJECTSTORE_PROVIDER` 选择一个已经存在的测试 bucket/container；值必须是
`volc-tos`、`aliyun-oss`、`gcs` 或 `azure-blob`：

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

TOS 可另外使用 `GIZCLAW_TOS_SESSION_TOKEN`，OSS 可使用
`GIZCLAW_OSS_SECURITY_TOKEN`；Azure identity 来自标准
`DefaultAzureCredential` environment 或 managed identity chain。每轮使用生成的
`gizclaw-e2e/` logical prefix，并验证 cleanup 后没有 residue。不得打印或提交
credential value。没有可用 account 时，应把对应 provider 明确记录为 `SKIP` 并保留
interoperability risk；只完成 tagged compile 不能算 live pass。

## Credential-backed harness 约束

GizClaw、GenX 和 Memory 的 live suite 各自只拥有一个 ignored `.env`，
由 committed、仅含 credential 的 `.env.example` 定义。每个字段对该 harness 的每个
`run*_tests.sh` 都是必填项，即使某个短入口并不消费其中全部 credential。缺文件、
缺字段、空值、纯空白或占位值必须在安装依赖、build、启动 Docker/service、执行 Go
测试或访问 provider 前直接失败；诊断只能打印字段名，不能打印值。

每个入口的 package 和 test selection 固定在仓库脚本中。入口选定后可以通过环境变量
提供非秘密 runtime 参数，但不能用环境变量选择 coverage，也不能把已选测试的失败改成
skip。Provider、fixture、网络、timeout、rate limit 或 native runtime 问题都必须使
命令失败。对这些脚本入口，绕过入口的 tagged `go test` 不能作为 live suite 的验收证据。
LoCoMo 是下文说明的例外：其 Go 测试名就是受支持的 selector。

## GizClaw Docker E2E

`tests/gizclaw-e2e` 是 Docker-backed 的完整 GizClaw 环境。Go 测试使用
`gizclaw_e2e` build tag，因此不会进入普通 `go test ./...`。

```text
tests/gizclaw-e2e/
├── docker/      # Compose services 与容器入口
├── setup/       # 环境启动、停止和 seed 脚本
├── testdata/    # committed identities、resources 与 ignored runtime output
├── cmd/         # 真实 gizclaw CLI 测试
├── giztest/     # 声明式 Peer RPC、Workflow 与 benchmark 场景
├── go/          # Admin、delete、Edge 与 OpenAI 专项测试
└── js/          # JavaScript/TypeScript WebRTC 测试
```

先复制 provider credential 模板。`.env` 只能保存 provider credential，不能保存
runtime 地址、resource ID、model/voice ID 或 E2E identity；真实密钥不得提交。

```sh
cp tests/gizclaw-e2e/.env.example tests/gizclaw-e2e/.env
bash tests/gizclaw-e2e/run_tests.sh
```

Firmware OTA 变更可以只启动所需的 live stack，并执行相关 Admin/RPC/CLI/C SDK
覆盖，不运行无关的 provider-backed suites：

```bash
bash tests/gizclaw-e2e/run_firmware_tests.sh
```

托管删除变更使用固定的 production vertical-slice 入口。该入口校验统一的 credential
file，启动隔离的 Docker stack，并在独立的 Peer RPC 删除测试包中覆盖 Pet、Workspace、
Friend Group 和 Peer。测试会验证使用中资源被终止，以及 Peer tombstone 在 Server 重启后
仍然生效；成功或失败后都会清理 project，且不运行无关的 provider-backed 场景：

```bash
bash tests/gizclaw-e2e/run_pending_deletion_tests.sh
```

完整 gate 会安装锁定的 Node workspace、初始化 nanopb submodule、构建 E2E CLI、
启动 Compose、等待 Server 与 Edge，然后依次运行 JS、C/cgo、Go Admin/OpenAI、CLI
和 Giztest 套件，最后执行一次有界清理。总 deadline 默认 90 分钟；
各 phase 默认 15 分钟，Docker setup 和 CLI 为 30 分钟，live chat 为 45 分钟，
cleanup 为 5 分钟。可通过以下正整数秒变量覆盖：

- `GIZCLAW_E2E_FULL_DEADLINE_SECONDS`
- `GIZCLAW_E2E_PHASE_DEADLINE_SECONDS`
- `GIZCLAW_E2E_PREFLIGHT_DEADLINE_SECONDS`
- `GIZCLAW_E2E_DOCKER_SETUP_DEADLINE_SECONDS`
- `GIZCLAW_E2E_DOCKER_CLEANUP_DEADLINE_SECONDS`
- `GIZCLAW_E2E_CHAT_DEADLINE_SECONDS`
- `GIZCLAW_E2E_CLI_DEADLINE_SECONDS`

### 手动环境

只启动或停止环境：

```sh
bash tests/gizclaw-e2e/setup/docker-compose-up.sh
bash tests/gizclaw-e2e/setup/docker-compose-down.sh
```

setup 自动选择随机可用的 Edge/Admin host ports。每个 Edge host port 必须同时可用于 TCP
和 UDP，并把两种协议都映射到 container `9821`；不再存在独立的 gateway endpoint 或 UDP
port。Firmware 或 LAN client 需要显式提供可达地址：

```sh
GIZCLAW_E2E_EDGE_HOST=192.168.1.20 \
  bash tests/gizclaw-e2e/setup/docker-compose-up.sh
```

生成状态位于 `tests/gizclaw-e2e/testdata/docker/<project>/`，最新环境入口是
`tests/gizclaw-e2e/testdata/docker/current.env`：

```sh
set -a
source tests/gizclaw-e2e/testdata/docker/current.env
set +a
```

其中 `GIZCLAW_E2E_EDGE_ENDPOINT` 同时是面向 client 的 HTTP/signaling 与 WebRTC ICE
endpoint，
`GIZCLAW_E2E_SERVER_ENDPOINT` 面向 host Admin，其他变量提供 CLI config home、
identity home 和 Compose project。需要重置标准资源时使用：

```sh
bash tests/gizclaw-e2e/setup/reset-data.sh reset --context remote-admin
```

`init` 只 apply、`clear` 只删除已知 fixture、`reset` 先 clear 再 init。脚本只从
`.env` 展开 credential placeholders；provider credential 缺失时必须 fail fast。
Workspace history 是运行时数据，不能由 reset 脚本直接 seed。

### Suite ownership

- `go/admin` 使用 generated Admin HTTP client 验证 typed contract。
- `go/delete` 保留需要 Admin 观察、重启与 tombstone 的删除测试。
- `go/edge` 保留 TURN relay、sibling-close、故障恢复和网络诊断。
- `go/openai` 保留 OpenAI 兼容 API 的 typed SDK 测试。
- `giztest/*.giztest.yaml` 验证 Peer RPC、conversation、social、gameplay 和 Workflow 行为。
- `cmd` 通过 `os/exec` 运行 `testdata/bin/gizclaw`，不能用 `go run` 或 typed client 绕过 CLI。
- `js/admin` 验证 WebRTC Admin fetch；`js/rpc` 验证 peer 与 server-initiated RPC。

### Giztest 场景

每个 Giztest 文件是独立 user story：文件自行创建临时 Peer、Workspace、invite、group
等可变资源，并在 `finally` 中清理。设备身份按任务随机生成；测试之间不共享固定 device
key 或输出。`clients` 可声明同一任务内同时在线的多个设备。

`repeat` 只决定一个文件展开多少个任务；全局并行度只能由 CLI `--parallel` 决定。
文件名以 `benchmark.` 开头的场景用于重复、barrier、并发或延迟测量。Runner 递归读取目录，
固定 worker pool 对所有文件统一调度，并把每个任务和 cleanup 写入脱敏 JSON report：

```sh
gizclaw test validate -f tests/gizclaw-e2e/giztest
gizclaw test run tests/gizclaw-e2e/giztest --parallel 10 \
  --output tests/gizclaw-e2e/testdata/giztest-report.json
```

Giztest 不执行 Admin Apply。标准 Docker setup 先一次性 apply 全部 fixture（包括专用
RuntimeProfile 和 run-scoped registration token），随后 JavaScript、C/cgo、Go、CLI 和
Giztest 共用该环境。远端目标可预先 provision 资源，再只提供
`GIZCLAW_TEST_ENDPOINT` 与 `GIZCLAW_TEST_REGISTRATION_TOKEN`。

音频和 binary 只作为带 `media_type`、`codec`、`max_bytes` 的内存变量传递；`save_as`
只赋值变量，不写文件。`speech.cache: run` 仅允许用于带 `save_as` 的语音合成步骤：同一次
CLI 运行按文档、步骤和展开后的请求缓存一份成功的只读输入 fixture，再为每个 repeat task
复制独立字节，避免把输入准备阶段的 TTS 容量误当成 Workflow 并发目标。

对于 `server.speech.transcribe`，Giztest 根据 typed audio variable 与 runner 的实际转换推导
上传 `content_type`：Ogg/Opus 解码成 16 kHz 单声道 PCM，格式匹配的 `pcm_s16le` 以相同
wire type 原样上传，其他音频格式在 RPC 打开前失败；文档不拥有这项 wire metadata。

`peer_stream.terminal_label` 默认等待 `assistant` 的文本和音频 EOS；
Chatroom 中以已持久化用户 transcript 为终止边界的场景显式设为 `transcript`。
`peer_stream.completion: first_response` 是面向部署探针的有界替代模式。
`require_text` 和 `require_audio` 选择必须等待的模态，二者都默认为 true；每个必需模态必须
声明对应的正数 Go duration `first_text_timeout` 或 `first_audio_timeout`，禁用的模态不声明
对应 deadline，并且至少保留一个必需模态。deadline 只在完整 turn 输入推送完成后开始；
runner 一旦观察到所有必需模态的第一段非空 assistant chunk 就成功并关闭该逻辑 stream，
不等待任何 EOS。缺少必需模态时分别以 `deadline=first_text_timeout` 或
`deadline=first_audio_timeout` 失败。该模式不能与 `interrupt_after`、`terminal_label` 或
`wait_for_history` 组合。
`peer_stream.idle_timeout`（Go duration，可选）限制的是不活动时长而不是总时长：runner 在
turn 输入推送完成后启动计时器，每收到一个 chunk（不区分 label）就重置，`interrupt_after`
的替换 turn 推送后重新启动，终止 EOS 被接受后停止。流停滞时步骤以
`peer_stream idle timeout exceeded` 失败，而持续在流式输出的长回复会通过。步骤 `timeout`
与文档 `timeout` 仍是绝对上限；两类同时设置时先到者生效。`peer_stream` 证据始终包含
`events` 与 `last_event_ms`，设置了该字段时增加 `idle_timeout_ms`，失败时用 `deadline`
指明触发的上限（`idle_timeout`、`first_text_timeout`、`first_audio_timeout`、`timeout` 或
`cancelled`）。失败步骤会在 `error` 旁保留
操作返回的证据，报告因此能区分停滞与过长回复。`gizclaw test validate` 拒绝无法解析或非正的
`idle_timeout`。人工 `review` 文件必须单独用 `--parallel 1` 在终端运行。

当 `peer_stream` 收到 assistant Opus 时，结果与脱敏 evidence 在
`audio_pacing` 下提供接收侧 pacing 指标：`packets`、`audio_ms`、
`target_span_ms`、`receive_span_ms`、`mean_packet_ms`、`mean_interval_ms`、
`p95_interval_ms`、`max_interval_ms`、`drift_ms`、`absolute_drift_ms` 和
`buffer_surplus_ms`。间隔来自 stream reader 收到每包的 monotonic 时间，不包含后续断言、
保存或 PortAudio 播放耗时；`buffer_surplus_ms` 为正表示网络到包领先于 Opus 音频时钟。
所有 `*_ms` 字段的单位都是毫秒。`target_span_ms` 是除最后一包外各包时长之和，
`drift_ms = receive_span_ms - target_span_ms`，`buffer_surplus_ms = -drift_ms`；P95 对到包
间隔使用 nearest-rank。只有一包时仅提供 `packets` 与 `audio_ms`；没有 assistant Opus 时
不提供 `audio_pacing`。
Giztest 文件使用普通 `expect` 数值约束断言这些路径，不增加另一套 pacing schema。
`flowcraft-voice-assistant.push-to-talk-roundtrip.giztest.yaml` 与
`doubao-realtime-conversation.realtime-roundtrip.giztest.yaml` 都要求至少 101 包、20 ms Opus
frame、平均间隔 12 到 21 ms、P95 不超过 30 ms、最大间隔不超过 100 ms，并要求最终缓冲
盈余在 450 到 550 ms 之间，分别覆盖 push-to-talk 与 realtime 下发。这些区间允许 pacer
围绕 500 ms 目标有界恢复，但不要求网络上每包严格等于 20 ms。

`workspace_relay` 在一个 task 内把两个已选中的 Workspace 接成一场有界对话：
tester Workflow 拥有测试意图、生成的用户行为、语义评判和最终裁决；Giztest 拥有传输、
封帧、`max_turns` 与固定字节/事件上限、归因、失败阶段和清理。转发是流式的——源端
响应尚未结束，第一个符合条件的文本 fragment 或按到达节奏的 Opus packet 就已带着
接收侧 stream ID 与 user 角色进入对侧 Workspace——终轮响应只捕获、不再转发。报告
保留每个 client 的轮次计数、`{min, max}` 时延/大小聚合和终端侧。`terminal_media` 可显式
把 text 转发与 Opus audio EOS 轮次边界分开；`idle_timeout` 按 active 轮次限制不活动时间，
由 active 侧进展重置，触发时记录 deadline、client、turn、最后事件和已观察媒体。audio relay
保留有界 assistant 文本用于断言和终轮 capture，但不重复转发文本。默认 report 不含内容；
本地 `--evidence full --output <path>` 才写入有界 relay 文本，产物属于敏感文件，但仍不包含
输入、凭据、ID 或音频 payload。`workspace-relay.workflow-tester.giztest.yaml` 在标准 gate
运行普通 candidate/tester 配对，`workspace-relay.doubao-realtime-workflow-tester.giztest.yaml`
验证多模态 candidate 的 text 转发和 audio EOS 完成；`run_workspace_relay_tests.sh` 启动
一套隔离栈，先后运行两个 repeat-1 与 repeat-20 relay gate（后者以 `--parallel 20` 运行
`benchmark.workspace-relay.workflow-tester-20.giztest.yaml`），并保证清理。

### OpenAI Conversations 与 Responses E2E

标准 GizClaw Docker runner 包含必须执行的 `go:openai` phase，目录为 `tests/gizclaw-e2e/go/openai`。它使用 pinned 官方 OpenAI Go SDK 通过 authenticated `ServicePeerOpenAI` 创建隔离的 Peer-owned Conversation Workspace，完成三轮文本、组合 transcription 到 Response 再到 speech，并验证 background cancel、stream client abort 与同 Conversation 恢复；所有 mutation 前先注册 Workspace cleanup。

成功运行会在 ignored `tests/gizclaw-e2e/testdata/openai-compatibility/` 下写入脱敏 monotonic timing evidence。Artifact 只含 schema/version、target/case、受限 media size、数字 phase timing 与 status，不能包含 credential、ID、prompt、transcript、generated text、media、URL 或 provider error。仅做 tagged compile 只是诊断，不能代替 `bash tests/gizclaw-e2e/run_tests.sh`。

### Workflow 10 路和 20 路并发与打断

固定入口选择十个明确的 `benchmark.*-10|-20.giztest.yaml` 文件。每个文件的
`repeat` 创建 10 或 20 个独立 Peer 和 Workspace；任务到达该文件自己的 barrier 后开始，
`--parallel` 是唯一并行度来源：

```sh
bash tests/gizclaw-e2e/run_workflow_concurrency_10_tests.sh
bash tests/gizclaw-e2e/run_workflow_concurrency_20_tests.sh
```

两个固定入口每个并发档位各选择 10 个正式文件，覆盖 Realtime、Realtime Duplex、
Flowcraft、Eino 和 Translate 的普通与打断场景。同一 repository head 必须先通过 10 路，
再执行 20 路。每个文件内部的任务共享一个 barrier，但所有文件仍由同一个全局 worker
pool 调度；因此报告必须保留 document 和 repeat 归属，不能把总任务数误报成单一
Workflow 的并发数。每个 task 始终复用自己的物理连接、Workspace runtime 和
PeerStream，并要求 terminal output 与 cleanup 完成。Runner 将脱敏 task/step evidence、
容器资源采样和 20 路 runtime profile 写到 ignored `testdata/workflow-concurrency/`。
语音输入 Benchmark 在内存中缓存不可变的合成输入，并把 barrier 放在 Workspace 和输入准备
完成之后，确保真正同时开始的是 10 或 20 条已就绪 PeerStream，而不是并发压测 TTS。

每个入口都在 Docker setup 前校验完整 `.env`，不接受环境变量改变 coverage 或并发数，不会
retry、fallback 或换新 session 来制造通过。只有当每个 terminal cause 都是带完整结构化
trailer 的火山云 `4xxxxxxx`/`5xxxxxxx` 错误时，整个 wave 才记为 provider-only `SKIP`。
Transformer 本地生成的 provider-completion 保护错误仍然使测试失败。任何混入的本地协议、deadline、setup、
runtime 或 cleanup 错误仍使测试失败；provider 错误 artifact 会保留。20 路入口同时启用
runtime profiling，并且只有采集到完整的非空 `manifest.json` 才算成功。资源采样和 profile
用于定位，不构成内存无泄漏、provider SLA 或 production capacity 承诺。

人工音频判断与自动 gate 分离：

```sh
bash tests/gizclaw-e2e/run_human_review_tests.sh
```

需要固定 selection 的 sibling-close、破坏性 Edge 场景和已 provision 的 Volc
LogStore 使用各自入口：

```sh
bash tests/gizclaw-e2e/run_sibling_close_tests.sh
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

需要 credential 的 GizClaw 入口（包括 capacity 和 focused Server relay lane）要求同一份
完整的 `tests/gizclaw-e2e/.env`；隔离的 relay-recovery lane 会生成仅运行时 fixture
credential，不消费 provider credential。Sibling-close 入口在
独立 Docker 环境中固定连续运行三次 JavaScript、C/cgo 与 Go 的并发 service/Event
场景，不接受环境变量改变 selection 或重复次数。Gateway capacity
入口固定运行本机 one-Server/two-Edge 的 100-session 基线。client 仍终止在 Edge，
但每条物理 Edge-to-Server connection 都必须 relay-only 经过两个 digest-pinned Coturn
4.7.0 member。每个 Edge 持有 4 条 gateway upstream association 和 1 条 control/HTTP
upstream，因此固定拓扑共有 10 个 live Coturn allocation；逻辑 session 仍只使用每个
Edge 的 4 条 gateway association。除保持连接和多轮 ping 外，
100 个 session 还会同步执行每路 4 MiB upload 和 download，并按共享 wall-clock 记录
聚合吞吐；单路对照使用 32 MiB sustained payload。machine-readable artifact 写到
ignored 的 `testdata/`；该入口不属于长时间或更高连接数的容量承诺。专用 100-session
burst 入口不设置 ramp，在 3 个全新 stack 上重复测试，报告 establishment rate 与 Dial
p50/p95/p99，每个 session 分方向传输 1 MiB，并记录 32 MiB 单路 sustained 对照。artifact
分别记录 key generation、客户端 PeerConnection、offer、ICE gathering、HTTP signaling、
answer 侧 PeerConnection/SDP/ICE，以及客户端 ICE connected、DTLS connected 和
DataChannel ready milestone；只有客户端 SCTP connected 边界明确标记为不支持。硬门槛为
100/100 建连、至少 20 sessions/s、Dial p95 不超过 1 秒
且 p99 不超过 5 秒，以及上传、
下载分别至少 200 Mbps aggregate。单路倍率保留为诊断数据；本机单次单路样本波动过大，
不能作为并发总吞吐的可靠 gate。

专用 500-session burst 入口同样固定运行 3 个全新 stack、0 ramp 和 500 concurrency。
每轮必须给两个 Edge 各分配 250 个 session，并要求每个 Edge 恰好使用 4 条 upstream
association。硬门槛为 500/500 usable sessions，establishment、ping、disconnect、restart
和 identity crossover 均为 0，至少 20 sessions/s，Dial p95 不超过 1 秒且 p99 不超过
5 秒，以及每个方向精确完成 500 MiB（500 x 1 MiB）、aggregate throughput 不低于
200 Mbps。32 MiB 单路结果与 aggregate ratio 只用于诊断，不作为 gate。artifact 写入
ignored 的 `testdata/gateway-capacity-extended/sessions-500-burst/`；每轮记录精确 repository
head 和 dirty state，可发布证据必须来自最终 clean PR head。

专用 1,000-session burst 入口固定 relay-only upstream、clean repository head、三个 fresh
stack、zero ramp、concurrency 1,000、30 秒 hold，以及每台 Edge 通过四条 gateway upstream
恰好承载 500 个 session。Load driver 固定 `GOGC=200`，并与 `GOMAXPROCS` 一起写入
artifact。这个实测 harness 参数避免约 2 GiB client heap 的回收成为 synchronized
transfer 的限制环节；长时间稳定性仍由当前 process CPU 与 completed-GC live-heap
证据把关。它不改变 production process、pacing、timeout 或 release barrier。每轮继续执行 20 sessions/s、Dial p95/p99、每 session 每方向
精确 1 MiB 和 200 Mbps gate，并在 hold 后执行 final liveness。Logical-session close 与
Serve completion 必须在 30 秒内结束；两台 Edge 停止后，固定十条 Coturn allocation 必须
在 15 秒内归零。每轮 workload 都按一秒间隔记录 source-qualified Coturn counter；relay
qualification 中任一 live sample 不是十条 allocation 都直接失败。Edge container 获得
45 秒 stop grace，entrypoint 最多转发并等待 SIGTERM 40 秒，使 production 30 秒 Gateway
drain 能关闭 physical upstream pool；独立的 15 秒 Coturn 归零上限从两台 Edge 都停止后
才开始计算。

1,000-session soak 入口是有序验收，不是替代 workload。它先运行相同的三轮 burst，确认
repository head 保持 clean 且未变化，再用一个 fresh zero-ramp 1,000-session stack 执行
60 分钟 hold。Liveness round 每 30 秒开始一次；runner 至少每 30 秒以及每轮 liveness
开始和结束时输出一次 hold heartbeat，包含 established/active session、累计与单轮 ping、
unexpected disconnect、open FD、RSS、goroutine，以及 Docker role 的最少 sample 数、
最大历史 gap 与最大当前 sample age。采样流停更或历史 gap 超过 2.1 秒时也立即失败；
任何超额 ping failure、unexpected disconnect、identity crossover 或过长 ping round 都会
使零失败验收不可恢复，因此 runner 立即执行有界清理，不再等待 hold deadline。每个测速
run 在开始、完成及运行中的每 15 秒输出 progress，避免把无输出误认为健康。Artifact 保留现有 `speed_test` 作为
initial checkpoint，并新增独立的 `final_speed_test` 与 `speed_retention`；initial/final upload 和
download 均精确传输 1,000 MiB（1,048,576,000 bytes）、达到至少 200 Mbps，且 final 每个
方向保留 initial aggregate
及 per-session p01、p05、p50 throughput 的至少 80%。低尾 percentile 用于捕获慢 session
退化；p95 与 p99 保留为快尾诊断，不作为 retention gate。
Fresh stack 的 HTTP 与 ready-file 等待同样每 15 秒输出 service state 和 elapsed time；
compose 已启动后的长时间静默不能作为 readiness 证据。
在一次有序的 1,000-session 验收中，runner 从要求的 clean head 构建一个按 run ID 隔离的
service image，并在后续 repetition 中复用这一份完全相同的镜像。每轮仍重新创建 container、
network、volume、port 与 runtime credential。失败的尝试只保留按 clean HEAD 隔离的镜像，
使同一 HEAD 的重试无需再次 build；HEAD 改变后使用新 tag，整组验收完成后删除这份精确
镜像。每个
fresh stack ready 后、测量前都执行相同的 120 秒稳定窗口，并每 15 秒输出 container health
心跳；执行首次镜像构建或复用镜像的 repetition 都不例外。
每个 1,000-session fresh stack 清理完成后保留固定 120 秒稳定窗口，每 15 秒输出剩余时间，
避免把 Docker VM 的延迟资源回收计入下一轮 capacity 测量。任一 upload gate 已失败时不再
执行 download，因为该轮验收已不可恢复。

Extended artifact version 18 记录实际 hold boundary，并验收最初与最后十分钟窗口。每轮
p99 RTT 的 median、RSS、open FD、最近一次 completed GC 的 Go live heap，以及 goroutine
值，增长最多为 20%。当前 Go heap-object bytes 保留为诊断值，但因其会随正常 GC cycle
波动而不作为增长 gate；sampler 不会强制触发 GC。
CPU 和 network rate 采用相同的相对门槛，并分别设置 0.10 core 与 1,024 bytes/s 的绝对
噪声下限；UDP 与 UDP6 socket median 增长最多为 20%。RSS、CPU 与 open-FD sample 标识
同一 process 及 start time；Docker role 的 UDP/socket 与 network counter 来自
`/proc/<pid>/net`，属于 container network namespace 证据，而非 process-only counter。
Darwin 与 Linux 上的 load driver CPU counter 来自 `getrusage` 的累计 process user+system
CPU；其他平台保留明确标注来源的 Go runtime active-CPU fallback。
Load driver、两台 Edge、两个 Coturn 与 Server 的 source-qualified sample 每秒记录，允许的
最大 gap 为 2.1 秒；cumulative CPU 与 network counter 不得下降。外部 Go runtime field
及 load driver 的 namespace socket/network field 无法获取时必须逐项明确为 unsupported。
任何 initial gate 失败都会阻止 hold 开始，cancellation 仍执行有界 session 与 Docker cleanup。

100/500-session burst runner 保留既有 payload 和 gate，但当前都使用上述 relay-only
upstream 拓扑。既有 workload field 保持不变；当前 version 18 artifact 包含 optional
final-speed retention、mandatory bounded-cleanup evidence，以及 load driver 的 effective
`GOGC`；100/500-session 入口显式保留 `GOGC=100`。同目录的
`*-coturn.json` sidecar 记录两个
Coturn member 的一秒间隔 live allocation/traffic sample、finished-session byte counter、
traffic delta，以及两个 Edge 停止后有界归零结果。每个 member 使用一条长期运行的
container-side metric stream，避免把 host 侧 Docker process 启动开销计入每次 sample。
验收要求 workload 前已产出第一条 sample，毫秒 timestamp 不递减且相邻 gap 不超过
2.1 秒；只有不同纳秒 sample 截断到同一毫秒时才允许 timestamp 相等。已合并的
#697/#698 结果仍是历史
direct-upstream 观测；当前
Coturn 数据不是 production、WAN 或可移植吞吐 SLA。

2026-08-07 的权威验收在 clean executable head
`a2ff5b791a5c60c3b80052204717ac277e43c885` 上只运行一次
`run_gateway_capacity_1000_soak_tests.sh`。Host 为 Darwin/arm64、16 logical CPUs、
Go 1.26.4 与 64 GiB RAM；隔离的 service image 运行在 OrbStack 2.2.1 Linux/aarch64
Docker，配置 16 logical CPUs 与 15.67 GiB RAM。三轮 prerequisite fresh-stack burst
均建立 1,000/1,000 sessions、每个 Edge 恰好 500 sessions、failure 为 0；establishment
rate 分别为 159.90、1,118.18、158.99 sessions/s，Dial p95/p99 分别为
681.57/776.75 ms、749.00/806.92 ms、589.81/669.13 ms，同步 upload/download 分别为
453.54/482.89、415.54/455.50、484.35/413.58 Mbps。每轮每方向精确传输
1,000 MiB、保持十条 relay allocation 存活，并通过有界 session 与 Coturn 清理。

新的 soak stack 随后以 1,074.63 sessions/s 建立 1,000/1,000 sessions，Dial p95/p99
为 718.53/838.54 ms。60 分钟内 122,000 次验收 Ping 全部完成；Ping failure、disconnect、
identity crossover、process/container exit 与 restart 均为 0，aggregate RTT p99 为
474.93 ms。Initial upload/download 为 415.51/425.25 Mbps，final upload/download 为
424.20/524.18 Mbps；final aggregate retention 为 102.09%/123.26%，per-session
p01/p05/p50 中最低 retention 为 96.66%，全部 throughput gate 通过。

Late median round-p99 RTT 下降 11.11%。两个 Edge 的 late-window RSS 分别增长 10.89%
与 16.49%，load driver 为 -52.64%，Server 为 -0.65%，两个 Coturn member 均约为
-2.78%；load driver 的 completed-GC live heap 增长 10.98%，FD 与 goroutine median
不变。六个角色全部通过其支持的 RSS、CPU、FD、heap、goroutine、UDP/UDP6 与 network-rate
gate；每个角色至少提供 3,679 个一秒 sample，最大 gap 为 1.033 秒。两个 Edge 全程保持
relay-only；Coturn sidecar 记录 received 2,414,392,388 bytes、sent 2,381,483,034 bytes；
logical-session cleanup 在 45.55 ms 内完成且 close failure 为 0，两个 Coturn member 均在
15 秒上限内从五条 allocation 归零。后续 documentation-only commit 不改变这份已验收
executable。

标准 Docker `turn` role 使用同一 pinned Coturn image、TURN REST 认证、container-private/
host-public IPv4 mapping 与一对一发布的 UDP relay range。`run_turn_relay_tests.sh` 验证
authoritative ServerInfo 临时凭据能建立 relay-only Server connection、完成产品 Ping、
推进 Coturn traffic counter 并清理。损坏 client credential 必须失败且不能形成双侧
allocation pair；authoritative Server 在回答 signaling 时仍可能建立自己合法的单侧
allocation，最终由 project teardown 清除。这个 focused 产品证据不验证 `pkgs/gizedge`
中可选的 embedded Pion TURN runtime。

如果 Docker host 可直达 container address，capacity script 会同时传入每个 Edge container
的 direct endpoint 和显式的本机 `-signaling-base-from-edge` override；其他调用仍遵守
advertised `transport.endpoint` contract。这样不会改变非本机 discovery 行为，同时避免把
published-port proxy backlog 误判为 load generator 以外的瓶颈。script 会打印选定的
endpoint boundary，不可直达时回退到 published endpoint。WebRTC/ICE 仍使用同一外部端口的
Edge endpoint candidates，Dial barrier 和 workload 不会被 pacing、batching 或 preconnect。

## GenX provider E2E

Provider-backed transformer coverage 使用一份完整 credential inventory：

```sh
cp tests/genx-e2e/.env.example tests/genx-e2e/.env
bash tests/genx-e2e/run_tests.sh
```

MiniMax 的 API key 必须与同一区域的 voice base URL 成对配置；runner 不会用默认区域
替代缺失的 `GIZCLAW_GENX_E2E_MINIMAX_BASE_URL`。

Provider-free Match parity 与 deterministic duplex behavior 保持为普通测试，
由 `go test ./...` 执行。

## Giznet E2E

`tests/giznet-e2e` 通过 gizwebrtc 验证公开 Giznet transport：

```sh
go test -tags giznet_e2e ./tests/giznet-e2e/...
go test -tags giznet_e2e ./tests/giznet-e2e/webrtc \
  -run '^$' -bench BenchmarkWebRTCHTTPRoundTrip -benchtime=1x
bash tests/giznet-e2e/run_coturn_tests.sh
```

普通 `giznet_e2e` lane 保留不依赖 Docker 的 in-process Pion TURN regression。固定 Coturn
runner 使用更严格的 `giznet_e2e,giznet_coturn_e2e` selection，只启动 static-auth 和
TURN REST Coturn role，不启动 GizClaw Server 或 Edge。它通过公开 Giznet API 验证
relay-only packet/service stream、错误凭据、allocation cleanup 与 finished traffic
counter；同时写入 ignored JSON artifact，包含 direct/static/REST 各 30 次 Dial、200 次
64-byte stream RTT、每条 path/每个方向 3 次全新 32 MiB transfer，以及 raw sample、phase
percentile、direct-versus-relay ratio、repository state、Docker engine 和精确 Coturn pin。
这些结果只属于本机 transport 诊断，不是 GizClaw gateway 或 production 性能证据。

### Edge direct 与 Coturn capacity 对照

GizClaw 自有 Edge 拓扑使用一个固定的本机验收入口：

```sh
bash tests/gizclaw-e2e/run_gateway_relay_capacity_tests.sh
```

命令要求 clean repository 和 Docker E2E credential file；CLI 会在与 Docker 原生架构一致的
Linux Go 基础容器中以 CGO build 一次，load driver 在 host build 一次。容量镜像只 COPY
该 Linux CLI、entrypoint 与必要配置，不安装 npm、不下载 Go modules，Server 与两个 Edge
启动时也不会重新编译。随后命令创建 12 个 fresh project：direct/relay-only 两种 Edge
upstream path、100/500 session、
每组各三次。两种 path 使用相同的 Server、两台 Edge、两个 digest-pinned Coturn member、
固定 subnet、每台 Edge 四条 gateway upstream、zero ramp，以及每 session 每方向 1 MiB。
Direct 必须保持 Coturn allocation 和 workload traffic 为零；relay 必须维持正好十条 live
allocation、证明 traffic 增长，并在 Edge 关闭后归零。随后命令运行固定的纯 Giznet
direct/Coturn 诊断；产品 comparison 出现 material 差异时必须提供 same-head 证据。该诊断
只用于归因，不能替代产品矩阵。

每个 session 还会通过 unreliable packet lane 按 20 ms cadence 发送 50 个 non-empty Opus
packet，并在之后完成 RPC Ping。Ignored artifacts 记录 path proof、timing/throughput、精确
packet/byte、各 role CPU/RSS/FD/socket/network、Coturn evidence 和校验后的
`comparison.json`。这只证明单台本机 Docker host 上的有界 one-way transport，不代表
provider processing、decoded audio、WAN/NAT diversity、production Coturn/deployment capacity、
1,000-session soak 或 30,000-session product ceiling。

2026-08-04 的 ARM64 OrbStack 参考实测（Docker 29.4.0、Docker 16 CPU、16.8 GB memory）
通过全部 12 轮，三次中位数如下：

| Session | Path | Upload | Download | Dial p95 / p99 | RPC RTT p99 |
| --- | --- | ---: | ---: | ---: | ---: |
| 100 | direct | 654 Mbps | 578 Mbps | 458 / 472 ms | 18 ms |
| 100 | Coturn | 416 Mbps | 568 Mbps | 452 / 460 ms | 19 ms |
| 500 | direct | 476 Mbps | 612 Mbps | 714 / 1,120 ms | 287 ms |
| 500 | Coturn | 417 Mbps | 606 Mbps | 778 / 819 ms | 503 ms |

Relay/direct 的 upload ratio 为 0.636 和 0.876，download ratio 为 0.981 和 0.990。
Upload 与 500-session RTT 差异属于 material，但所有固定门槛、精确 reliable bytes、Opus
packet、path selection、allocation 和 cleanup 检查均通过。同一 clean head 的纯 Giznet
诊断不包含产品 Edge 和 Server，测得 direct 818/798 Mbps、REST Coturn 488/526 Mbps，
同时 Coturn receive/send counter 增长约 220/219 MB。因此本次实测边界归属于本机 Coturn
relay path，而不是 GizClaw Edge/Server capacity；它不代表 production Coturn host 或 WAN。

## LoCoMo Memory Evaluation

`tests/locomo-e2e` 是 GizClaw 自有的 production `memory.Store` 人工评测，不使用
Flowcraft LoCoMo evaluator，也不属于普通 `go test ./...`、Docker E2E 或 required CI。
每个 live test 在对应 Go 文件中完整定义 provider、memory lane 和 extraction config；
remote project 配置由部署拥有，harness 不修改它，也不会把一个 endpoint/project
冒充成多条 lane。

当前 lane 包括 Flowcraft BBH BM25 single-pass、hybrid single/two-pass、Mem0 Platform
default/custom instructions 和 Volc AgentKit Memory default。LoCoMo 是 tagged Go 测试包，
不是 shell runner。用标准 `go test -run` 选择 backend 组；被选择的测试只校验自己消费的
环境变量，缺失或占位值会失败，未选择 backend 的变量不会被检查：

```sh
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run '^TestLoCoMoVolcAgentKit' ./tests/locomo-e2e
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run '^TestLoCoMoMem0Platform' ./tests/locomo-e2e
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run '^TestLoCoMoFlowcraft' ./tests/locomo-e2e
```

`.env.example` 只是变量清单；值通过进程环境注入，测试包不会读取 `.env` 文件。
包级 timeout 使用 30 分钟，并分别限制 session 与 question。Runner 按官方 session
调用 `memory.Store.Observe`，逐题 recall，再用配置模型回答并在本地计算 EM、F1 和
evidence-hit 和 adversarial rejection。只有 answerable 问题进入 EM/F1 和 evidence-hit；
category 5 只接受规范化后精确等于 `unknown`、`not mentioned` 或
`no information available` 的拒答。默认 gate 要求 aggregate F1 不低于 `0.05`，有 evidence 的 store 要求
hit rate 不低于 `0.50`，且每个选中 session 至少 materialize 一个 fact。Provider error
或 timeout 是失败，不能降级成 skip/pass。报告写入 ignored `reports/`，只包含 ID、分数和耗时，
不得包含 credential、conversation、question、answer、prediction 或 recalled text。

### Dataset 与许可

`testdata/locomo10_smoke.jsonl` 是通过 Git LFS 保存的 SNAP Research LoCoMo
`locomo10.json` 非商业适配子集：包含 `conv-30` 前三个 session 和 `conv-26` 第一个
session（共 76 turns），以及覆盖 category 1 到 5 的八个问题。它只用于 contract smoke，不代表完整 benchmark。
精确 upstream commit、checksum、subset 和 transformation 记录在
`locomo10_smoke.manifest.json`。

该子集按 [CC BY-NC 4.0](https://creativecommons.org/licenses/by-nc/4.0/) 分发，
仅限非商业用途；许可全文保存在 `LICENSE.locomo.txt`。上游时间没有 timezone，数据中的
`Z` 只表示确定性的 Go `ObservedAt` 映射，不宣称原始 timezone。clone 后执行
`git lfs pull`；loader 会拒绝未解析的 LFS pointer。

离线验证：

```sh
go test -race -tags gizclaw_locomo_e2e \
  -run 'TestDataset|TestScore|TestAdversarial|TestAggregate|TestPreflight|TestRedaction|TestSession|TestRunBenchmark|TestAwait' \
  ./tests/locomo-e2e
git lfs fsck
```

## Memory provider E2E

三个 live-model Memory case 使用 `gizclaw_memory_e2e` build tag 和一个固定入口：

```sh
cp tests/memory/.env.example tests/memory/.env
bash tests/memory/run_tests.sh
```

普通 Memory 测试保持 credential-free，并由 `go test ./...` 执行。
