# CLI

## 查询单个或多个资源

单资源查询输出一个 Resource JSON 对象：

```sh
gizclaw admin show Model model-a --context dev
```

批量查询接收 JSON 数组，数组元素只包含资源的 `kind` 和 `id`，支持混合类型和重复引用：

```json
[
  {"kind":"Model","id":"model-a"},
  {"kind":"RuntimeProfile","id":"profile-a"}
]
```

```sh
gizclaw admin show -f resources.json --context dev
cat resources.json | gizclaw admin show -f - --context dev
```

`-f` 与位置参数互斥，文件或 stdin 必须包含一个 JSON 数组；不支持 YAML。
全部引用在连接前完成校验。空数组输出 `[]`，不建立连接。
非空批次复用一个连接，最多同时查询 8 个资源，没有并发配置参数。
输出是按输入顺序排列的 Resource JSON 数组，重复引用保留各自的位置。

某项查询失败时，对应位置为 `null`，其余查询继续；stderr 报告从 0 开始的索引、
资源类型、ID 和错误，进程以非零状态退出。输入或连接失败时不输出结果数组。
命令取消会传递给查询，尚未发起的查询不再请求服务器；所有查询结束后关闭连接。


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

人工调试一份语音场景时，使用无并发的 `test play`：

```sh
gizclaw test play tests/gizclaw-e2e/giztest/voice.giztest.yaml
gizclaw test play -o ./play-record tests/gizclaw-e2e/giztest/voice.giztest.yaml
```

该命令只接受一个普通文件以及 `repeat: 1`、无 barrier 的文档，固定使用一个 worker。
它先播放一个不写入记录的短提示音，提示测试随即开始。随后按对话顺序播放 `peer_stream` 和
音频 `workspace_relay` 上传的 user Opus 音频，以及实际收到的 assistant Opus 音频。音频边接收
边播放：先积累 200 ms 音频的抗抖缓冲，达到水位后持续解码成 16 kHz 单声道 PCM，并通过独立播放
task 写入默认 PortAudio 输出设备。user EOS 将尾部加入播放队列但不延迟向 Server 推送问题；
assistant EOS 冲刷尾部并等待该轮队列播放完成后再切换说话者。关闭时先关闭 PortAudio，以打断
可能阻塞的 native write，避免播放 task 挂死。摘要同时报告提示音结束到第一份
下行音频收到和开始播放的毫秒数，并输出每轮 packet 音频时长及到包间隔。普通 Giztest 也可通过
`peer_stream` 结果的 `/audio_pacing` 路径断言 packet 数、音频时钟、平均/P95/最大间隔和累计漂移。
默认只实时播放而不写文件。需要保留测试记录时，`-o` / `--output` 必须指向一个尚不存在的
新目录；执行结束后其中包含脱敏的 `report.json`，收到音频时还包含
`audio.ogg`，其中按试听顺序包含 user 与 assistant 的完整对话。没有音频时不会生成伪造的空 Ogg。执行或播放失败也会尽量保存失败 report
和已经收到的有界音频，再返回非零状态。

Play 需要支持当前平台的 cgo、libopus 和 PortAudio native runtime，并在创建远端 client
前检查这些条件。记录目录中的音频是操作者显式选择落盘的真实 response 内容，应按敏感
文件处理。普通 `test run` 不打开音频设备，也不产生额外音频文件。

`--evidence redacted` 是默认模式。`--evidence full` 必须同时提供 `--output`，只把有界的
`workspace_relay` 逐轮与终轮文本写入该 JSON report，不向终端打印。完整 evidence report
仍不包含输入、展开变量、凭据、ID 或音频 payload，但模型或 tester 文本可能含私密内容，
必须按敏感文件处理。

YAML 的 `repeat` 是每个文件的任务数，`--parallel` 是所有文件共享的最大 worker 数。
目录输入递归发现文件并稳定排序。每个任务有独立的临时 clients、variables 和 cleanup；
`save_as` 只写入顶部声明的内存 output 变量，不支持 Save As 文件。
给 `peer_stream` 喂音频的 `speech` 步骤请求 `audio/ogg`；Volc 和 MiniMax voice 都会返回
Ogg/Opus（MiniMax 输出由 Server 转码），翻译类文档不依赖 Workflow 的 provider。
重复语音 Benchmark 可在合成步骤声明 `speech.cache: run`，按文档和展开后的请求缓存成功的
输入 fixture；每个 task 得到独立字节副本，缓存受 output `max_bytes` 限制，并在命令退出时释放。

`server.speech.transcribe` 步骤从引用的 typed variable 获取音频格式。Runner 接受 Ogg/Opus
并将其解码成 16 kHz 单声道 PCM，也接受格式匹配的 `pcm_s16le` 直接上传；其他音频格式会在
打开 RPC 前被拒绝。Runner 按准备完成的 bytes 设置 `content_type`；文档 request 只填写
model 和可选 language，不填写这个由 runner 拥有的 wire metadata。

只验证时延的 `peer_stream` 探针可以在收到第一段 assistant 文本和音频后停止，不等待终止
输出：

```yaml
- id: deployment_response_probe
  client: peer
  peer_stream:
    mode: push-to-talk
    input: ${turn_audio}
    pacing: 20ms
    completion: first_response
    first_text_timeout: 2s
    first_audio_timeout: 3s
  expect:
    /events: {non_empty: true}
    /first_text_ms: {maximum: 2000}
    /first_audio_ms: {maximum: 3000}
```

两个 deadline 都在完整输入 turn 推送完成后开始。两种首响应都到达后，runner 立即关闭该
逻辑 stream；文本/音频 EOS 和剩余回复不属于这个探针。

仅输出文本的 Workflow 禁用不存在的音频模态，并且只声明文本 deadline：

```yaml
- id: text_only_response_probe
  client: peer
  peer_stream:
    mode: realtime
    input: ${turn_audio}
    pacing: 20ms
    completion: first_response
    first_text_timeout: 2s
    require_text: true
    require_audio: false
  expect:
    /text: {non_empty: true}
    /first_text_ms: {maximum: 2000}
```

`rpc_stream` 的 `all.speed_test.run` 步骤向 `expect`、`capture` 和 object `save_as` 暴露以下
稳定结果路径；同一组 canonical 测量字段还会与 `method` 一起进入脱敏 step evidence：

| 路径 | 单位与语义 |
| --- | --- |
| `/up_content_length`、`/down_content_length` | 各方向请求并由测速确认的 bytes。 |
| `/up_bytes`、`/down_bytes` | 各方向实际传输的 bytes。 |
| `/up_duration_ms`、`/down_duration_ms` | 各方向测量耗时，截断为整数毫秒。 |
| `/duration_ms` | 整次调用 wall time，截断为整数毫秒。 |
| `/up_mbps`、`/down_mbps` | 有限且非负的实测 megabits per second。 |
| `/bytes` | 接收 bytes；对该操作等于 `/down_bytes`。 |

未启用的方向仍显式保留数值零。PascalCase 路径 `/UpContentLength`、
`/DownContentLength`、`/UpBytes`、`/DownBytes`、`/UpDuration`、`/DownDuration` 和
`/Duration` 仅作为 compatibility aliases 保留；新文档必须使用上面的 canonical 路径。
三个旧 duration alias 仍使用原始纳秒值。

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

`equals`、`contains`、`contains_all`、`contains_any` 和 `not_contains` 可以在各自
expectation 对象中显式启用确定性规范化：

```yaml
expect:
  /text:
    contains_all: [四点, G7105]
    normalize: [whitespace, punctuation, case, digits]
```

四个选项分别删除 Unicode 空白（`unicode.IsSpace`）、删除 Unicode 标点
（`unicode.IsPunct`，不删除 symbol 或 emoji）、使用 Go Unicode mapping 转小写，以及把
全角数字和 `零一二三四五六七八九` 逐字符映射为 ASCII 数字；不会解析 `十` 等中文数量词，
也不执行 locale-specific case folding。Fragment array 先 join，再对目标和每个操作数应用
同组选项；选项书写顺序不影响结果。`pattern` 和 rune 长度 matcher 仍读取原始 join 文本。
`normalize` 必须与上述五个 matcher 中至少一个一起声明；默认仍为 byte-exact。

远端步骤可以显式启用有界 retry：

```yaml
- id: translated_turn
  client: peer
  timeout: 2m
  retry:
    attempts: 3
    on: [timeout, assertion]
    delay: 5s
  peer_stream:
    mode: text
    input: Translate this sentence.
```

`attempts` 是 2–10 的总尝试次数。`on` 默认 `[timeout]`，还可包含 `assertion`；`delay`
默认为零，显式值必须是大于零且不超过五分钟的 Go duration。只有普通 `steps` 中的
`rpc`、`rpc_stream`、`speech`、`peer_stream` 和 `workspace_relay` 可以 retry；finalizer、
`client_rpc`、`barrier`、`output` 和交互 review 步骤不能 retry。

每次 attempt 重新获得 step timeout，但 task timeout 和调用方取消会限制全部 attempts 与
delay。Timeout 必须保留可由 `errors.Is` 识别的 `context.DeadlineExceeded`，只有相似错误文本
不会被归类；assertion 包括 `expect` 和 `expect_error` mismatch。其他 operation、变量解析、
capture 和取消错误立即终止。失败 attempt 不提交 `save_as` 或 capture，只有成功 attempt
才一次性向后续步骤发布全部输出。

Retry 复用同一 clients、variables、Workspace 和远端状态；不会重连、清空 history、重跑前置
步骤或撤销 RPC/provider 副作用，因此只有在重复当前 operation 符合场景语义时才能启用。
Retry step 保留现有顶层最终 status、error 和 evidence，并增加按顺序排列的 `attempts`，记录
attempt 序号、status、duration、安全 evidence，以及 `timeout`、`assertion`、`operation` 或
`cancelled` failure kind；顶层 duration 包含 retry delay。没有 `retry` 的步骤保持现有报告
结构并省略 `attempts`。

`workspace_relay` 步骤把两个已声明 client 各自选中的 Workspace 接成一场有界对话。
两个 client 必须不同，且各自都要有一个更早的 `server.run.workspace.set` 步骤，
否则离线校验直接拒绝。`first_client` 接收 `input`；它的 assistant 输出被增量转发——
`media: text` 按 fragment、`media: audio` 按到达节奏的 Opus packet——以全新的接收侧
stream ID 作为对侧的 user 输入，每完成一次 assistant 响应后所有权交替。
`max_turns`（2–256）统计双侧完成的 assistant 响应总数，`terminal_client` 必须匹配
`first_client` 与 `max_turns` 的奇偶；终轮响应只捕获、不再转发。可选的 `terminal_media`
默认等于 `media`；text relay 可以声明 `terminal_media: audio`，
只转发文本、按 Opus audio EOS 完成轮次。audio relay 必须按 audio 终止，避免提前截断 packet。
可选正 Go duration `idle_timeout` 在初始输入和每次交接后启动，只由 active 侧未丢弃的进展
重置；静默轮次会带 active client 和从 1 开始的轮次失败，step/document timeout 仍是绝对上限。
`media: audio` 要求双方 Workspace 接受 push-to-talk 输入，且只转发 Opus 媒体（`audio/opus`，或带
`codecs=opus` 的 `audio/ogg`）；active 侧出现任何其他音频 MIME 类型或编码都会使 relay
失败；不支持连续 realtime 转发。

```yaml
- id: run_test_dialogue
  workspace_relay:
    first_client: tester
    second_client: candidate
    input: "${test_brief}"
    media: text
    terminal_media: audio
    idle_timeout: 90s
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

relay 结果暴露 `completed_turns`、`terminal.client`、终轮产生文本时的 `terminal.text`、
`turns.<client>.texts`、`turns.<client>.count`，以及逐轮 `{min, max}` 聚合——text 为
`first_text_ms`/`text_runes`，audio 为 `first_audio_ms`/`audio_bytes`——外加事件与
字节总数。两种 relay media 都可把 `/terminal/text` capture 到 string output；audio
还可把 `/terminal/audio` capture 到 `audio/ogg` Opus output。audio relay 观察到的文本
用于断言和 capture，但不会和音频一起重复转发为 user 输入。v1 每轮最多接收 4,096 个非音频
事件（空音频 chunk 也计入）；非空 Opus 音频按包的 RTP 时钟累计，最多十分钟，并在解码前
受每轮 16 MiB 字节上限保护。每轮上限也计入随后丢弃或来自 inactive 侧的输入，完成轮次后
重置；evidence 仍统计全部事件。整个 relay 的 1 MiB 拼接文本与 16 MiB 音频上限保持有效。
超限即失败且不暴露调节字段。Workspace 在
自己第一个 relay 轮之前 self-start 的响应会被消费并丢弃（其 `interrupted` 标记视为
良性）；一旦某侧持有过轮次，它在对侧 active 期间于 relay 媒体上（`media: text` 为文本、`media: audio` 为音频）
的任何输出都会被视为串轮而使 relay 失败；语音 Workspace 的另一条通道（例如自己已完成
文本轮之后拖尾的 TTS 音频）只消费计数。失败会
标注责任 client 和轮次序号。idle failure 还保留 `deadline`、`idle_timeout_ms`、active
client/turn、最后事件时间和已观察媒体，不包含内容。默认 report 只保留归因、计数、时延和
字节聚合；显式 full evidence 增加有界逐轮和终轮文本。两种模式都不写入输入、secret、ID
或音频 payload。

本地 Docker E2E 会先统一 Apply Admin resources。直接测试已部署环境时，预先准备资源并设置
`GIZCLAW_TEST_ENDPOINT`、`GIZCLAW_TEST_REGISTRATION_TOKEN`；命令本身没有 Admin 权限。
人工 `review.*` 场景要求 attached terminal 和 `--parallel 1`。

### 启动偏移与 think time

调度字段位于文档顶层，默认全部为零：

```yaml
repeat: 16
start_jitter: 30s
stagger: 2s
step_jitter: 3s
seed: 42
```

任务索引 `i` 从零开始，计划启动偏移为 `i × stagger + U[0,start_jitter)`，相对整个
run 的 `started_at` 计算；每份文档重新从 `i = 0` 编号。二者相加，随机部分可能使任务
启动顺序与索引不同。Runner 按计划时间调度，到期后才占并发槽；槽位不足时实际启动会
推迟，不会在拿到槽位后再完整等待一次偏移。启动发生在创建变量、连接与注册客户端之前，
首个 step 的时间另行记录。启动等待不消耗文档 `timeout`；timeout 从实际任务启动计算。

`step_jitter: 3s` 表示首步之后、每个顶层 step 执行前独立抽取 `U[0,3s)` 的等待，
上一步结束后开始计时。它包括顶层 `parallel`，但不改变其子步骤的 `delay` 或相对时序；
retry attempt 和 `finally` 不增加 think time。think time 消耗任务 timeout，step timeout
从等待结束后开始。所有调度等待均响应调用方 context 取消，不创建额外兜底 deadline。
`test run` 把 SIGINT（Ctrl-C）/SIGTERM 转为 context 取消，执行清理并写出失败报告。
显式 `barrier` 会主动重新对齐任务，因此有 barrier 的文档要求三个有效延迟均为零。
`test run` 加载时会应用 CLI 覆盖，显式零参数可以关闭 barrier 文档中的延迟；
`test validate` 校验文档本身声明的值。

这些 duration 字段接受字符串 `"0"`、`0s`，或带 `ns`、`us`/`µs`/`μs`、`ms`、`s`、`m`、
`h` 单位的非负十进制及组合（例如 `1m2.5s`）。范围写法 `0..3s`、负数、空值、未知字段
和会导致最大计划偏移超过 Go duration 范围的组合均被拒绝。随机值为纳秒精度，右边界不包含。

CLI 只覆盖显式给出的字段，优先级为 CLI > 文档 > 默认；显式 `0` 可以关闭文档中的延迟：

```sh
gizclaw test run scenario.giztest.yaml --parallel 16 \
  --start-jitter 30s --stagger 0 --step-jitter 3s --seed 42 --output jitter.json
gizclaw test run scenario.giztest.yaml --parallel 16 \
  --start-jitter 0 --stagger 0 --step-jitter 0 --seed 42 --output lockstep.json
```

`seed` / `--seed` 是 `0..9007199254740991` 内的整数，零也是有效 seed。未提供 CLI seed 时
run 生成并记录一个 seed；文档自己的 seed 可覆盖该回退值。每个任务以有效 seed、文档
`name`、repeat index 派生独立的启动与步骤随机流，因此 worker 完成顺序、其他被选文档和
启动 jitter 不会扰动该任务的 think time。相同文档和有效 seed 重现计划偏移与等待值；实际
启动时间仍取决于 worker 可用性、网络与操作执行耗时。seed 不控制身份、token、provider 输出。

Go/C JSON report 的 `seed` 记录 run seed。每个 task 记录有效 `seed`、`start_jitter`、
`stagger`、`step_jitter`、`planned_start_offset_ms`、`actual_start_offset_ms` 和 `started_at`。
实际启动前被取消的任务，actual offset 为 `null` 且不写 started_at。每个执行过的 step、
cleanup、retry attempt 和已启动的 parallel child 都记录 `started_at` 与相对 run 的
`start_offset_ms`；尚未开始的操作不伪造时间。step 的 `planned_delay_ms` 记录该次 think time，
不计入 step `duration_ms`。偏移用可带小数的毫秒表示；task duration 不含启动等待，run duration
包含等待和 cleanup。混合文档 seed 时，重放须保留各文档 seed，或分别使用 task 的有效 seed。

Go 与 C runner 执行上述调度，且都支持四个 CLI 覆盖参数。JavaScript/Flutter SDK runner
校验并接受这些文档字段，然后忽略调度，报告明确标为 `timing_mode: ignored`；它们不提供
这些 CLI 覆盖参数，不用于 jitter 负载测量。

现有 `benchmark.*concurrency*` 文档显式使用 `start_jitter: 0s`、`stagger: 0s`、
`step_jitter: 0s`，保留同时启动的 worst-case 基准。
`benchmark.flowcraft-voice-assistant.realistic-concurrency-16.giztest.yaml` 默认使用 30 秒启动
jitter 和 3 秒 think time，16 个任务各请求三次长语音回复。它断言回复完成且包含文字、音频，
并保留 `audio_pacing.underruns` 与 `minimum_buffer_ms` 供独立分析；完成通过不代表没有饥饿。
其余原有基准可用上面的 CLI 参数执行 realistic load，不改变文件本身的默认测量含义。
