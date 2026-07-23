# 测试与 E2E

本页说明仓库级测试 harness。普通 Go 单元测试仍按改动范围运行；带 build tag、
Docker、真实 provider 或人工判断的套件必须显式启动，不能把未运行记作通过。

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
default/custom instructions 和 Volc AgentKit Memory default。运行一个明确选择的 lane：

```sh
cp tests/locomo-e2e/.env.example tests/locomo-e2e/.env
bash tests/locomo-e2e/run_tests.sh
```

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
