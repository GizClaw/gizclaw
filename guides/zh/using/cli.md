# CLI

[Giztest Schema](https://github.com/GizClaw/gizclaw/blob/main/api/giztest/giztest.schema.json) · [Runner 与结果实现](https://github.com/GizClaw/gizclaw/tree/main/cmd/internal/commands/giztest)

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
gizclaw test run tests/gizclaw-e2e/giztest --parallel 10 --report report.json
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
新目录；执行结束后包含独立 HTML、报告、对话文本和分段音频。没有音频时不会生成空音频文件。执行或播放失败也会尽量保存失败 report
和已经收到的有界音频，再返回非零状态。

### 导出单个测试的独立 HTML

每次运行一个 Giztest 文件，指定 `--output`（短参数 `-o`）即可生成可分发的记录目录：

```sh
gizclaw test run scenario.giztest.yaml --output ./out
```

`--output` 只接受一个普通 Giztest 文件，输出目录须不存在且父目录须已存在。
文件中的 repeat、并行步骤和 finally 都保存在这一份记录中。不指定输出时按通常方式验收。

```text
out/
├── index.html       # Go template 渲染的独立 HTML，包含全部步骤、文本及分段音频数据
├── report.json      # 验收报告
├── replay.json      # 本次测试记录的数据
├── transcript.txt   # 对话文本
└── audio/           # 原始 Ogg/Opus 和分段 WAV
```

页面以对话卡片为主。每个发言片段有固定帧号 `#0001`、说话方、时间及分段音频，
可直接用“第 N 帧”指出问题。点击帧号或在顶部输入编号定位；同一记录内的帧号不会随暂停、
拖动、倍速和完整记录视图变化。测试步骤默认折叠在“测试详情”，其中的大 JSON 也单独折叠；
预生成语音素材保留在详情中，不作为已发生的发言插入对话。

点击“开始回放”后，发言及音频按事件时间出现。每段的“播放此段”按钮定位到同一时间线，
暂停、拖动和倍速同时作用于所有并行音频；不生成合并的完整对话音频文件。
两个 Workflow 之间已确认的音频／完整文字转发，以及该输入的识别文字，以引用保留在详情，
对话中只展示、播放原发言一次。匹配的并行首包探针也引用完整录音；独立回复即使内容相同也
各自展示。普通用户音频及其识别文字合在同一输入卡片，不隐藏真实输入。
转发去重要求录制中的输入变量与输出捕获关系，或同一 `workspace_relay` 声明的客户端对和媒体
类型，并同时校验内容；仅内容相同不足以确认转发。旧记录没有来源绑定时保留相同内容的输入。

时间栏区分步骤开始、持续时长、首字和首音，均与顶部进度共用同一时间原点，并显示到毫秒。
有识别结束事件时，另列识别结束至首字／首音的等待；文字流建立不代表模型已返回正文。
默认只展示当前时间已到达的事件。“查看全部记录”暂停回放并显式进入完整记录视图；重置回到
时间零点，后续事件仍隐藏。

生成时通过 Go `html/template` 渲染，CSS、JavaScript、测试数据和分段音频都内嵌在 HTML 中。
分发整个目录或只复制 `index.html` 都可以离线回放，不依赖 Giztest、Go、源码模板、Web Server，
也不读取其他 JSON 或音频文件。`audio/` 保存的原始文件仅供分析或再次使用。

回放使用相对于 `report.started_at` 的 event 时间，不跳过启动延迟和静默等待。
录制保存每个音频 chunk 的到达时间和媒体时长；突发到达的 chunk 顺序排队，队列耗尽后的
包间停顿保留为静默。`interrupted` 等终止错误及 PeerStream 关闭按事件时间停止相关流的
排队音频，普通 EOS 则让已收到的音频播放完。浏览器先解码音频，再启动统一时钟。
这些时间来自 Giztest 的发送／接收观察，不是物理扬声器播放回执。旧记录缺少 chunk 时间
和连接关闭事件时，页面会注明限制；缺失的停顿或打断需重新录制才能还原。
prepared speech 可单独试听；relay 转发副本和匹配的并行首包探针以引用展示，自动回放只播放
原录音一次，独立流仍可重叠。无音频的测试不生成空音频文件。
失败或 Ctrl+C/SIGTERM 也会保存已记录内容与失败结果。录制最多保留 128 MiB 文本/编码音频
及 200,000 个媒体/控制 chunk，超限或音频导出失败明确标注部分记录并返回执行错误。

`test play -o <目录>` 使用同一记录结构。自动化只需要汇总 JSON 时使用
`test run --report <文件>`；该模式仍支持多个文件及目录。`--report` 指向输出目录内时只允许
根目录的 `report.json`。`--evidence full` 接受 `--output` 或 `--report` 作为目的地。
录制不改变验收断言，默认 JSON evidence 仍保持脱敏。输出的文本和语音应按敏感记录处理。

Play 需要支持当前平台的 cgo、libopus 和 PortAudio native runtime，并在创建远端 client
前检查这些条件。记录目录中的音频是操作者显式选择落盘的真实 response 内容，应按敏感
文件处理。普通 `test run` 不打开音频设备；显式设置 `--output` 才额外保存会话媒体。

`--evidence redacted` 是默认模式。`--evidence full` 必须同时提供 `--output` 或 `--report`，
把有界的 `workspace_relay` 逐轮与终轮文本、`peer_stream` 输入与输出、speech 转写，
以及受支持 RPC 的目录或 Workspace 证据写入 JSON report，不自动向终端打印。
音频输入可能以 Base64 字节进入完整 evidence report，首包探针还包含临时关联 packet ID。
这些证据可能含私密对话或资源内容，必须按敏感文件处理。

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

`peer_stream.mode: push-to-talk` 发送与设备一致的控制 BOS、音频 BOS、Opus、音频 EOS、
控制 EOS，同一轮保持同一 StreamID；`pacing` 只作用于音频包。空 PTT 只发送控制边界。
完整单轮对话可断言 `/response_count: {equals: 1}`，检测多次回复被混进同一音频下行的情况；
该值排除已忽略或打断的回复。音频输入通过 `/input_ms` 报告媒体时长。
Assistant terminal completion 在完整回复后继续收集 250 ms；观察到新的 response 或 MIME channel 开始时，取消旧观察计时器，等待所有已开始的 text/audio channel 匹配 EOS。旧回复完成不能掩盖后续未收尾 channel；多条文本仍可共用一路音频。`reply_observation: 1s`
可延长这段有界观察，再统计 `response_count`。该设置不用于空输入、`first_response`、
`input_sent`、重叠输入或 transcript completion。
控制 BOS 后默认按住 500 ms 才打开音频通道；`hold_before_audio` 可指定非负 Go duration，
用于其它设备门限。PTT 不在 wire 声明 `input_mode`，实际输入模式来自 Workspace 参数。

SFU 首包探针在非空 `push-to-talk` + `completion: input_sent` 的发送步骤设置
`measure_first_packet: true`。Go runner 为首个非空 Opus 包加入唯一测试 padding 标记，
保持解码 PCM 和媒体时长；正常输入不增加标记。Canonical silence/DTX 包保持原样，
发送证据的 `uncorrelated_reason: silence_or_dtx` 表示无法关联。只有同一包的完整内容和
SFU 发送者身份匹配才统计接收。

发送步骤的 `/audio_first_packet` 与同一 task 中 `listen` 步骤的
`/audio_first_packets` 数组同时进入结果、capture/save_as 和脱敏 JSON evidence。
字段由 `api/giztest/audio-packet-timing.schema.json` 定义：

只有成功发送并收到对应包才有接收样本；丢包、自收或撤权后的无音频返回空数组，不填零延迟。
收到后续音频不能替代丢失首包。重复接收同一标记只计一次；每个 listen 最多保留 128 个样本，
超限通过 `audio_first_packets_dropped` 计数。`first_audio_ms` 保留从开始收听至首个可听音频的
旧含义，包括发送前等待和前导静音；不能减去固定 500 ms 作为转发延迟。共享偏移只在同一
task 的 Go runner 内可比较，不能与 report 的 wall-clock `started_at`、其它 task 或机器混用。

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

未启用的方向仍显式保留数值零。PascalCase 路径 `/UpContentLength`、
`/DownContentLength`、`/UpBytes`、`/DownBytes`、`/UpDuration`、`/DownDuration` 和
`/Duration` 仅作为 compatibility aliases 保留；新文档必须使用上面的 canonical 路径。
三个旧 duration alias 仍使用原始纳秒值。

步骤的 `expect` 把 JSON Pointer 映射到 expectation 对象。一个 expectation 对象可以组合多个
matcher，全部通过时该断言才通过：

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

Giztest 记录 relay 的功能结果；剧情质量评测由 Raids 负责。

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
  --start-jitter 30s --stagger 0 --step-jitter 3s --seed 42 --report jitter.json
gizclaw test run scenario.giztest.yaml --parallel 16 \
  --start-jitter 0 --stagger 0 --step-jitter 0 --seed 42 --report lockstep.json
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
`benchmark.eino-voice-assistant.realistic-concurrency-16.giztest.yaml` 默认使用 30 秒启动
jitter 和 3 秒 think time，16 个任务各请求三次长语音回复。它断言回复完成且包含文字、音频，
并保留 `audio_pacing.underruns` 与 `minimum_buffer_ms` 供独立分析；完成通过不代表没有饥饿。
其余原有基准可用上面的 CLI 参数执行 realistic load，不改变文件本身的默认测量含义。

## PendingDeletion 恢复

正常删除由归属 Server 的后台 worker 自动完成。已确认的旧 Firmware 投影误判 failed 也由 home Server 在启动和后续扫描时自动核查并恢复，无需先执行 CLI。它只处理符合精确错误、旧计划、raw generation 和本区空快照条件的记录；其他终态失败保持可诊断状态。下面的在线命令用于可选预览、诊断和显式重试，不是正常清理的必经步骤。

`admin pending-deletions` 使用当前或指定 context 连接在线区域 Server。`recover` 默认只读，一次最多扫描 500 条 failed task，并输出逐条候选、`source/kind/last_error_code` 分组计数及 `next_cursor`；数量仅代表这一页。`--source` 必填，`--kind`、`--error-code`、`--from` 和 `--until` 限定范围；时间使用 RFC3339，前者包含、后者不包含。

```sh
gizclaw admin pending-deletions recover --context region-a --source peer \
  --error-code peer_replacement_ambiguous --limit 100
gizclaw admin pending-deletions recover --context region-a --source peer \
  --error-code peer_replacement_ambiguous --limit 100 --execute --interval 200ms --wait 2m
gizclaw admin pending-deletions get <deletion-id> --source peer --context region-a
```

执行使用相同的 list/get/retry API，不直接删 Redis key。先完整输出本页 preview，再按序重新读取候选并重试；已变化的状态报告 `changed`。请求并发固定为 1，最小间隔 100ms，业务执行并发由 Server 的 `pending_deletion.workers` 决定。`--wait` 默认零，只报告 queued；设置正值时全批次最多等待 30 分钟，输出 completed、failed 或 pending。拒绝、状态变化、请求失败或等待结束仍有 pending 时非零退出；已成功的操作不回滚。

把输出的 `next_cursor` 传给下一次相同 filter 的命令可以有界推进。游标仅表示列表位置，不是全局快照或授权凭据。任务执行时仍会重新检查 home、marker、lease 和 generation。中断前逐条结果已输出；若请求已发出但结果缺失，先 get 对应 ID 确认状态。再次运行只选择 failed，queued/running 不会再次认领；completed 的 task 已被领域 finalization 消费。不要把 queued 或 CLI 返回 0 当成清理完成。

错误处理按领域证据进行：

| 错误 | 处理路径 |
| --- | --- |
| `peer_home_mismatch` | 从固定 route 确认 home，在该区域 Server 的 context 上重试；不重指派 route。 |
| `peer_home_unknown`／`peer_home_invalid` | 保留 marker，核查原始注册、route 与各区域数据归属；不能靠本区空表或手写 owner 猜测。 |
| `peer_replacement_ambiguous` | home Server 自动恢复经核实的旧 Firmware 投影误判；真实 KV generation 改变仍拒绝，保留原记录与计划供核查。 |
| `social_cleanup_failed` | 先修复确切 Social 存储或依赖错误，再用相同 retry；涉及跨区域副本的记录仍须完成各区域证据核查。 |
| `retirement_scope_ambiguous`／`social_replica_scope_unknown` | 保留任务和围栏；当前入口不提供强制完成或跳过区域依赖的参数。核查计划选中的精确 ID 与对应区域，不能以删除 task 替代领域清理。 |
| `peer_record_missing` | 核查记录是否损坏、丢失或被外部删除；没有准确原始 generation 时不能通过本入口完成。 |
| Memory／对象／SQL 暂时失败 | 恢复对应服务后重试；所有 child Workspace task 完成并验证 absent 前保留 owner binding。 |

成功会回收精确 task、marker、locator、retirement plan 与任务索引，保留永久 Peer deleted 墓碑、API Key retired 围栏、固定 route、全局配置和无关数据。PeerRun 仍是本区诊断目录。部署更新与实际生产执行是独立运维步骤。

## 离线迁移 Redis KV 到 TiKV

`gizclaw kv-migrate <dir> --source redis-old --target tikv` 按 workspace 中的逻辑 Store prefix 导入并双向校验 KV 数据。停止全部源、目标写入者后，依次执行 `--dry-run`、导入和 `--verify-only`，校验成功后再修改 Store binding 并恢复服务。完整的数据类型、TTL、重复执行、冲突和回切边界见 [KV 离线迁移](../developing/stores/kv#redis-tikv-migration)。
