# CLI

## 离线校验声明式 Resource

在 apply 之前，可以用 `admin validate` 校验一个声明式 Resource 或一个 `ResourceList`：

```sh
gizclaw admin validate -f resource.yaml
gizclaw admin validate -f resource.json
printf '%s\n' '{"apiVersion":"gizclaw.admin/v1alpha1","kind":"ResourceList","spec":{"items":[]}}' \
  | gizclaw admin validate -f -
```

文件输入支持 `.json`、`.yaml` 和 `.yml`；`-f -` 从 stdin 读取 JSON。该命令与 `admin apply` 复用相同的 `${VAR}`、`${VAR:-default}` 展开规则，也接受相同的生成型 `KindResource` 兼容别名。

具体 Resource 校验成功时以状态码 `0` 退出，并输出一个紧凑 JSON object 和换行：

```json
{"valid":true,"kind":"Credential","id":"openai-main"}
```

有效列表只报告 item 数量，不输出任何 item spec：

```json
{"valid":true,"kind":"ResourceList","items":3}
```

无效输入以非零状态退出，并使用输入标识与脱敏的 JSON Pointer diagnostics 说明问题。命令不会打印 Resource spec value 或展开后的环境变量值，因此可以在 CI 中安全校验 Credential Resource，而不暴露其 body。

校验过程完全离线：不会读取 GizClaw context、连接 Server 或修改 storage。通过校验只表示展开后的 document 符合同一二进制所嵌入的 Resource OpenAPI schema，并能按声明的 kind 解码；它不能证明引用 ID 存在、凭据可认证、provider/body 组合满足 Server 业务规则、依赖服务可达，也不能证明 Resource 可以成功 apply 或运行。

## 运行 Giztest

`test validate` 离线递归校验严格的 `*.giztest.yaml`；`test run` 连接每个文件自己声明的
endpoint。所有文件先整体通过校验，才会创建临时身份或产生远端操作：

```sh
gizclaw test validate -f tests/gizclaw-e2e/giztest
gizclaw test run tests/gizclaw-e2e/giztest --parallel 10 --output report.json
```

YAML 的 `repeat` 是每个文件的任务数，`--parallel` 是所有文件共享的最大 worker 数。
目录输入递归发现文件并稳定排序。每个任务有独立的临时 clients、variables 和 cleanup；
`save_as` 只写入顶部声明的内存 output 变量，不支持 Save As 文件。
重复语音 Benchmark 可在合成步骤声明 `speech.cache: run`，按文档和展开后的请求缓存成功的
输入 fixture；每个 task 得到独立字节副本，缓存受 output `max_bytes` 限制，并在命令退出时释放。

步骤的 `expect` 把 JSON Pointer 映射到 expectation 对象。一个 expectation 对象可以组合多个
matcher，全部通过时该断言才通过：

| Matcher | 操作数 | 语义 |
| --- | --- | --- |
| `equals` | 任意非 null 值 | JSON 相等 |
| `present` | boolean | pointer 可解析（`false` 表示必须不存在） |
| `non_empty` | boolean | 值是非空 string、array 或 object |
| `count` | ≥ 0 的整数 | 数组长度等于操作数 |
| `contains` | 非空 string，≤ 256 rune | 字符串目标包含该子串 |
| `contains_all` | 1–16 个上述 string | 所有子串都必须出现 |
| `contains_any` | 1–16 个上述 string | 至少一个子串出现 |
| `not_contains` | 一个上述 string 或 1–16 个 | 任何子串都不得出现 |
| `pattern` | RE2 源码，1–256 字节 | 字符串目标匹配该正则 |
| `minimum` / `maximum` | number | 数值目标（JSON number，或 protojson int64 之类的十进制字符串）落在闭区间边界内 |
| `min_length` / `max_length` | 0–1048576 的整数 | 字符串目标的 rune 数落在边界内 |

字符串类 matcher 接受 string 值，或元素全部为 string 的数组；数组先按空分隔符 join，
因此 `peer_stream` 的 `/text` fragment 会作为一条完整响应断言。长度按 Unicode rune 计数
而不是字节。`minimum`/`maximum` 适用于 `peer_stream` 的 `/first_text_ms` 等数值字段。
校验在离线阶段、任何连接建立之前拒绝：无法编译的 `pattern`、`min_length` 大于
`max_length`、`minimum` 大于 `maximum`，以及 `present: false` 与任何取值 matcher 的组合。
内容类 matcher 失败时只报告 pointer 和 matcher 名，绝不回显被断言的文本，脱敏报告因此
不包含响应内容。

`workspace_relay` 步骤把两个已声明 client 各自选中的 Workspace 接成一场有界对话。
两个 client 必须不同，且各自都要有一个更早的 `server.run.workspace.set` 步骤，
否则离线校验直接拒绝。`first_client` 接收 `input`；它的 assistant 输出被增量转发——
`media: text` 按 fragment、`media: audio` 按到达节奏的 Opus packet——以全新的接收侧
stream ID 作为对侧的 user 输入，每完成一次 assistant 响应后所有权交替。
`max_turns`（2–256）统计双侧完成的 assistant 响应总数，`terminal_client` 必须匹配
`first_client` 与 `max_turns` 的奇偶；终轮响应只捕获、不再转发。`media: audio`
要求双方 Workspace 接受 push-to-talk 输入，不支持连续 realtime 转发。

```yaml
- id: run_test_dialogue
  workspace_relay:
    first_client: tester
    second_client: candidate
    input: "${test_brief}"
    media: text
    max_turns: 15
    terminal_client: tester
  capture:
    verdict: /terminal/text
  expect:
    /terminal/client: {equals: tester}
    /terminal/text: {equals: PASS}
    /completed_turns: {equals: 15}
    /turns/candidate/first_text_ms/max: {maximum: 6000}
    /turns/candidate/text_runes/min: {minimum: 15}
```

relay 结果暴露 `completed_turns`、`terminal.client`、`terminal.text`
（`media: text`）、`turns.<client>.count`，以及逐轮 `{min, max}` 聚合——text 为
`first_text_ms`/`text_runes`，audio 为 `first_audio_ms`/`audio_bytes`——外加事件与
字节总数。`capture` 只允许把 `/terminal/text` 赋给 string output 变量，或在 audio
下把 `/terminal/audio` 赋给 `audio/ogg` Opus output 变量。v1 固定安全上限——每完成一轮 4,096
个接收事件、整个 relay 1 MiB 拼接文本与 16 MiB 音频——超限即失败且不暴露调节字段；
事件上限按轮计数，因为带语音的 Workspace 每次响应会流出数百个 Opus 包。Workspace 在
自己第一个 relay 轮之前 self-start 的响应会被消费并丢弃（其 `interrupted` 标记视为
良性）；一旦某侧持有过轮次，它在对侧 active 期间于 relay 媒体上（`media: text` 为文本、`media: audio` 为音频）
的任何输出都会被视为串轮而使 relay 失败；语音 Workspace 的另一条通道（例如自己已完成
文本轮之后拖尾的 TTS 音频）只消费计数。失败会
标注责任 client 和轮次序号。报告只保留 client 名称归因、计数、时延和字节聚合：被转发的
文本、prompt、tester 推理和音频字节绝不进入报告 evidence。

本地 Docker E2E 会先统一 Apply Admin resources。直接测试已部署环境时，预先准备资源并设置
`GIZCLAW_TEST_ENDPOINT`、`GIZCLAW_TEST_REGISTRATION_TOKEN`；命令本身没有 Admin 权限。
人工 `review.*` 场景要求 attached terminal 和 `--parallel 1`。
