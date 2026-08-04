# 测试与 E2E

本页说明仓库级测试 harness。普通 Go 单元测试仍按改动范围运行；带 build tag、
Docker、真实 provider 或人工判断的套件必须显式启动，不能把未运行记作通过。

## Credential-backed harness 约束

GizClaw、GenX、LoCoMo 和 Memory 的 live suite 各自只拥有一个 ignored `.env`，
由 committed、仅含 credential 的 `.env.example` 定义。每个字段对该 harness 的每个
`run*_tests.sh` 都是必填项，即使某个短入口并不消费其中全部 credential。缺文件、
缺字段、空值、纯空白或占位值必须在安装依赖、build、启动 Docker/service、执行 Go
测试或访问 provider 前直接失败；诊断只能打印字段名，不能打印值。

每个入口的 package 和 test selection 固定在仓库脚本中。入口选定后可以通过环境变量
提供非秘密 runtime 参数，但不能用环境变量选择 coverage，也不能把已选测试的失败改成
skip。Provider、fixture、网络、timeout、rate limit 或 native runtime 问题都必须使
命令失败。绕过入口的 tagged `go test` 不能作为 live suite 的验收证据。

## GizClaw Docker E2E

`tests/gizclaw-e2e` 是 Docker-backed 的完整 GizClaw 环境。Go 测试使用
`gizclaw_e2e` build tag，因此不会进入普通 `go test ./...`。

```text
tests/gizclaw-e2e/
├── docker/      # Compose services 与容器入口
├── setup/       # 环境启动、停止和 seed 脚本
├── testdata/    # committed identities、resources 与 ignored runtime output
├── cmd/         # 真实 gizclaw CLI 测试
├── go/          # Admin、chat、gameplay、RPC 与 social 测试
├── js/          # JavaScript/TypeScript WebRTC 测试
└── desktop/     # Wails shell、Admin 与 Play 测试
```

先复制 provider credential 模板。`.env` 只能保存 provider credential，不能保存
runtime 地址、resource ID、model/voice ID 或 E2E identity；真实密钥不得提交。

```sh
cp tests/gizclaw-e2e/.env.example tests/gizclaw-e2e/.env
bash tests/gizclaw-e2e/run_tests.sh
```

完整 gate 会安装锁定的 Node workspace、初始化 nanopb submodule、构建 E2E CLI、
启动 Compose、等待 Server/Desktop，然后依次运行 JS、Desktop、C/cgo、Admin、chat、
gameplay、RPC、social 和 CLI 套件，最后执行有界清理。总 deadline 默认 90 分钟；
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

setup 自动选择随机可用的 Edge/Admin host ports。Firmware 或 LAN client 需要显式
提供可达地址：

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

其中 `GIZCLAW_E2E_EDGE_ENDPOINT` 面向 client，
`GIZCLAW_E2E_SERVER_ENDPOINT` 面向 host Admin，其他变量提供 CLI config home、
identity home、Desktop URL 和 Compose project。需要重置标准资源时使用：

```sh
bash tests/gizclaw-e2e/setup/reset-data.sh reset --context remote-admin
```

`init` 只 apply、`clear` 只删除已知 fixture、`reset` 先 clear 再 init。脚本只从
`.env` 展开 credential placeholders；provider credential 缺失时必须 fail fast。
Workspace history 是运行时数据，不能由 reset 脚本直接 seed。

### Suite ownership

- `go/admin` 使用 generated Admin HTTP client 验证 typed contract。
- `go/rpc` 按 RPC module 划分 typed RPC 测试。
- `go/chat` 验证 workspace voice、stream interruption、history 和 memory。
- `go/social` 从 client 侧验证 relation、domain workspace、message 和 history event。
- `cmd` 通过 `os/exec` 运行 `testdata/bin/gizclaw`，不能用 `go run` 或 typed client 绕过 CLI。
- `desktop/shell` 验证 Pod shell；`desktop/admin` 和 `desktop/play` 验证浏览器 surface。
- `js/admin` 验证 WebRTC Admin fetch；`js/rpc` 验证 peer 与 server-initiated RPC。

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

100/500-session burst runner 保留既有 payload 和 gate，但当前都使用上述 relay-only
upstream 拓扑。主 workload JSON schema 不变；同目录的 `*-coturn.json` sidecar 记录两个
Coturn member 的 live allocation、finished-session byte counter、traffic delta，以及两个
Edge 停止后有界归零结果。已合并的 #697/#698 结果仍是历史 direct-upstream 观测；当前
Coturn 数据不是 production、WAN 或可移植吞吐 SLA。

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
endpoint boundary，不可直达时回退到 published endpoint。WebRTC/ICE 仍使用配置的 gateway
candidates，Dial barrier 和 workload 不会被 pacing、batching 或 preconnect。

## GenX provider E2E

Provider-backed transformer coverage 使用一份完整 credential inventory：

```sh
cp tests/genx-e2e/.env.example tests/genx-e2e/.env
bash tests/genx-e2e/run_tests.sh
```

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

命令要求 clean repository 和 Docker E2E credential file；CLI 与 load driver 只 build 一次，
随后创建 12 个 fresh project：direct/relay-only 两种 Edge upstream path、100/500 session、
每组各三次。两种 path 使用相同的 Server、两台 Edge、两个 digest-pinned Coturn member、
固定 subnet、每台 Edge 四条 gateway upstream、zero ramp，以及每 session 每方向 1 MiB。
Direct 必须保持 Coturn allocation 和 workload traffic 为零；relay 必须维持正好十条 live
allocation、证明 traffic 增长，并在 Edge 关闭后归零。

每个 session 还会通过 unreliable packet lane 按 20 ms cadence 发送 50 个 non-empty Opus
packet，并在之后完成 RPC Ping。Ignored artifacts 记录 path proof、timing/throughput、精确
packet/byte、各 role CPU/RSS/FD/socket/network、Coturn evidence 和校验后的
`comparison.json`。这只证明单台本机 Docker host 上的有界 one-way transport，不代表
provider processing、decoded audio、WAN/NAT diversity、production Coturn/deployment capacity、
1,000-session soak 或 30,000-session product ceiling。

## LoCoMo Memory Evaluation

`tests/locomo-e2e` 是 GizClaw 自有的 production `memory.Store` 人工评测，不使用
Flowcraft LoCoMo evaluator，也不属于普通 `go test ./...`、Docker E2E 或 required CI。
每个 live test 在对应 Go 文件中完整定义 provider、memory lane 和 extraction config；
remote project 配置由部署拥有，harness 不修改它，也不会把一个 endpoint/project
冒充成多条 lane。

当前 lane 包括 Flowcraft BBH BM25 single-pass、hybrid single/two-pass、Mem0 Platform
default/custom instructions 和 Volc AgentKit Memory default。完整入口运行全部 lane：

```sh
cp tests/locomo-e2e/.env.example tests/locomo-e2e/.env
bash tests/locomo-e2e/run_tests.sh
```

同目录中的 `run_flowcraft_bm25_tests.sh`、`run_flowcraft_hybrid_tests.sh`、
`run_mem0_tests.sh` 和 `run_volc_agentkit_tests.sh` 提供固定的短 selection。
它们仍要求同一份完整 LoCoMo `.env`；dataset、report、timeout、model、endpoint、
project 与 threshold 是显式非秘密 runtime 参数或 committed default，不属于
credential 文件。

脚本具有 30 分钟默认总 timeout，并分别限制 session 与 question。Runner 按官方 session
调用 `memory.Store.Observe`，逐题 recall，再用配置模型回答并在本地计算 EM、F1 和
evidence-hit。默认 gate 要求 aggregate F1 不低于 `0.05`，有 evidence 的 store 要求
hit rate 不低于 `0.50`，且每个选中 session 至少 materialize 一个 fact。Provider error
或 timeout 是失败，不能降级成 skip/pass。报告写入 ignored `reports/`，不得包含 credential。

### Dataset 与许可

`testdata/locomo10_smoke.jsonl` 是通过 Git LFS 保存的 SNAP Research LoCoMo
`locomo10.json` 非商业适配子集：包含 `conv-30` 前三个 session（58 turns）和六个
evidence 完全落在这些 session 中的问题。它只用于 contract smoke，不代表完整 benchmark。
精确 upstream commit、checksum、subset 和 transformation 记录在
`locomo10_smoke.manifest.json`。

该子集按 [CC BY-NC 4.0](https://creativecommons.org/licenses/by-nc/4.0/) 分发，
仅限非商业用途；许可全文保存在 `LICENSE.locomo.txt`。上游时间没有 timezone，数据中的
`Z` 只表示确定性的 Go `ObservedAt` 映射，不宣称原始 timezone。clone 后执行
`git lfs pull`；loader 会拒绝未解析的 LFS pointer。

离线验证：

```sh
go test -race -tags gizclaw_locomo_e2e \
  -run 'TestDataset|TestScore|TestPreflight|TestRedaction|TestSession|TestRunBenchmark|TestAwait' \
  ./tests/locomo-e2e
bash -n tests/locomo-e2e/run_tests.sh
git lfs fsck
```

## Memory provider E2E

三个 live-model Memory case 使用 `gizclaw_memory_e2e` build tag 和一个固定入口：

```sh
cp tests/memory/.env.example tests/memory/.env
bash tests/memory/run_tests.sh
```

普通 Memory 测试保持 credential-free，并由 `go test ./...` 执行。
