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

GIZCLAW_E2E_VOLC_LOG_ENDPOINT=... \
GIZCLAW_E2E_VOLC_LOG_REGION=... \
GIZCLAW_E2E_VOLC_LOG_TOPIC_ID=... \
  bash tests/gizclaw-e2e/run_volc_log_tests.sh
```

六个 GizClaw 入口都要求同一份完整的 `tests/gizclaw-e2e/.env`。Sibling-close 入口在
独立 Docker 环境中固定连续运行三次 JavaScript、C/cgo 与 Go 的并发 service/Event
场景，不接受环境变量改变 selection 或重复次数。Gateway capacity
入口固定运行本机 one-Server/two-Edge 的 100-session 基线：除保持连接和多轮 ping 外，
100 个 session 还会同步执行每路 4 MiB upload 和 download，并按共享 wall-clock 记录
聚合吞吐；单路对照使用 32 MiB sustained payload。machine-readable artifact 写到
ignored 的 `testdata/`；该入口不属于长时间或更高连接数的容量承诺。专用 100-session
burst 入口不设置 ramp，在 3 个全新 stack 上重复测试，报告 establishment rate 与 Dial
p50/p95/p99，每个 session 分方向传输 1 MiB，并记录 32 MiB 单路 sustained 对照。硬门槛为
100/100 建连、至少 20 sessions/s、Dial p95 不超过 1 秒且 p99 不超过 5 秒，以及上传、
下载分别至少 200 Mbps aggregate。单路倍率保留为诊断数据；本机单次单路样本波动过大，
不能作为并发总吞吐的可靠 gate。

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
```

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
