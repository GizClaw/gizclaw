# 运行时 Tools

`toolkit` 保存 Admin HTTP Tool resources；`toolcatalog` 从当前 Peer 的 RuntimeProfile
解析运行时工具，供 AgentHost 与 `server.tool.list/get` 共同使用。设备工具只存在于运行时，
不向 Admin tools 表写入合成资源，也不通过本机 HTTP 控制设备。

## 配置与权限

```yaml
resources:
  tools:
    web-search:
      resource_id: volc-web-search
      i18n:
        en: {display_name: Web search}
        zh-CN: {display_name: 搜索网页}
    screen.brightness:
      mhs: {id: display.main, operation: write, fields: [brightness_percent]}
      i18n:
        en: {display_name: Screen brightness}
        zh-CN: {display_name: 设置本机屏幕亮度}
    music-play:
      client_tool: {name: audioplayer.play}
      i18n:
        en: {display_name: Play music}
        zh-CN: {display_name: 播放音乐}
workflows:
  general-assistant:
    resource_id: eino-chat-assistant
    toolkit:
      tool_names: [web-search, screen.brightness, music-play]
    i18n:
      en: {display_name: Assistant}
      zh-CN: {display_name: 助手}
```

每个 Tool binding 必须选择且只选择一个来源：`resource_id`、`mhs` 或 `client_tool`。
Tool 和 Workflow 使用专用 binding 类型。Workflow binding 的 `toolkit.tool_names` 是唯一
注入授权来源，值必须是同一 Profile 声明的 Tool aliases。省略、空策略、空列表都不注入。
Workspace 的 `toolkit.tool_names` 只取交集；省略不再收窄，显式空列表禁用全部工具。

Eino Workflow binding 可选 `toolkit.verification_model`，引用同一 Profile 的 chat-model
alias。主模型仍看到原生 Tool declarations 并提出调用；固定目标 MHS 读取与有副作用的调用在发送设备 RPC
或 HTTP POST 前，由该模型基于真实对话、当前目录的业务上下文、固定目标、参数及本轮工具结果独立校验。
目录上下文包含其他候选的目标、说明和可用性，用于区分配置的默认目标与当前候选；
MHS 能力另按精确 id/hwd 汇总可读、可写和已授权写字段；读能力不能授予写能力。
真实用户输入按原有顺序单独投影，助手提议不进入用户授权序列。
私有 HTTP executor 与认证字段不参与校验。当前目录读取失败时拒绝调用校验模型及执行。
它只批准或拒绝当前候选，不能替换目标或修改参数。其他只读操作不增加这次校验。缺少真实
对话、校验模型错误或非法决定都拒绝执行；校验后仍执行原有权限与资源重读。
该 alias 必须绑定 `llm` Model。此配置目前只支持 Eino，其他 Workflow driver 会在
Profile 验证时拒绝。最终回复还会核对本轮成功结果，拒绝漏执行、重复追问已齐槽位、
虚假完成和编造结果；最多让主模型纠正两次。中间调用文本和被拒绝的草稿不发布，
只有通过检查的最终文本进入输出；校验失败或纠正耗尽以错误结束该轮，不发布未校验文本。
每个被校验的候选和最终回复检查都会增加模型调用、成本和延迟；纠正也可能增加调用。
固定目标 MHS 读取也在访问设备前核对请求目标，避免相对调整先读取错误对象；其他只读调用仍不增加该校验。
语义判断仍需真实模型回归验证，不能当成确定性证明。

Workflow resource 的旧 `spec.toolkit.tool_ids` 不授予运行时权限。已有配置需要在 Profile
的 Workflow binding 中显式选择 aliases。旧 Workspace `tool_ids` 仍只能收窄所选 HTTP
resources；不能与 `tool_names` 混用，也不能授予 inner tools。新 Peer Workspace 选择按 alias
持久化，换绑时保持选择；失效 alias 原样投影，但不会被模型调用，不回退到 HTTP 调用名。

Profile alias 是 1–63 字节的 lowercase kebab-case segments，可用点号连接，不接受下划线。
模型函数名把点号逐字替换为下划线，其余字符保留。因此 `screen.brightness` 对应
`screen_brightness`，`screen-brightness` 对应 `screen-brightness`；这是可逆映射。
目录的 `name` 是 alias，`invoke_name` 是该映射后的函数名。HTTP resource 的 immutable
`spec.invoke_name` 只属于资源实现，不参与 Peer 选择或模型身份。

## 设备与能力

MHS binding 固定当前 Peer manifest 中的实例 ID；manifest 决定 HWD，模型不能提交或改变
`id/hwd`。Read 接收空对象，write 只接受 `fields` 中的 HWD 专属参数，并要求至少一个参数。
非法范围、类型、额外字段和不存在的实例在接触设备前拒绝。

`client.rpc.methods.list` 的 `mhs_v0` 返回设备明确实现的实例及 `write_fields`；SDK 的
`MhsCapabilities` provider 负责报告真实实现。通用 display Schema 中有 `enabled` 不代表
当前硬件支持写它。未报告能力的旧设备显示能力未知，其 inner Tool 不注入模型。
`client.mhs.v0.read` 也可携带 `write_capabilities`，供设备调用方检查具体读写能力。

ClientTool binding 固定一个注册表程序名，input schema 来自同一嵌入契约。程序 enum
选择对应 protobuf 请求与响应；参数不能更换底层程序。`client.tool.v0.list` 只报告设备
已安装的程序。`run.workspace.set` 仍通过当前 owner 的 Workspace/Workflow 解析规则选择目标。

一次目录解析按协议 family 查询能力；同一解析内复用查询结果。执行前重新读取 Profile、
Workflow binding、Workspace 收窄及目标能力，只解析被调用的 alias，不向无关设备发送请求。
设备操作复用按 owner 串行的控制通道；排队期间再次校验权限和绑定。离线、撤权、未实现、
错误参数、断线与超时都不选择另一工具或另一 Peer。模型必须以实际成功结果确认动作。

## 目录

`server.tool.list/get` 返回 alias、显示信息、参数 Schema、来源、固定目标与支持/在线/可用状态。
`workflow_name` 或 `workspace_name` 可选择最终子集，不能同时指定两者。Workspace 必须属于
调用 Peer。HTTP credentials 与 auth 配置不出现在目录中。

已配置但资源删除、禁用或设备能力未知的条目仍可发现，`available` 为 false，
`unavailable_reason` 说明原因。离线时不能推断硬件不支持，需同时检查 `online` 与 reason。
HTTP 的在线标记表示 Server 可解析该资源；禁用状态单独体现在 available，不探测远端 provider 健康。
分页 cursor 绑定 Profile revision 与查询 scope。

## HTTP 执行

HTTP resources 固定 HTTPS GET 或 JSON POST、参数映射、结果 pointer、超时和大小上限。
`giztools` 拒绝 redirect、环境 proxy、private/loopback/link-local 等禁止地址，并校验状态、
content type 和 JSON。执行不自动重试。凭据在调用时由 Server 解析，不交给模型。

内置工具与 HTTP 的 ToolCall/ToolResult 均留在 Transformer continuation 中，不成为公开
assistant control stream。协议确定性回归与真实模型、Docker 验收边界见 [测试与 E2E](../../../testing)。

## 持久化与结果边界

Admin HTTP resources 继续保存在 SQL `tools` 表：canonical ID 为主键，私有 `invoke_name`
有唯一约束。type、enabled、description、version、timestamps 为独立列，input Schema、
triggers、metadata、HTTP 配置为 JSON。更新校验 row revision 和 creation incarnation；
轮换 secret 或删除重建后会重读当前资源，不能恢复旧 secret。枚举使用最多 256 行的 ID 排序批次。
认证支持 none、bearer、header_api_key、volc_ark、volc_search、volc_openapi、aliyun_app_code、
aliyun_openapi_v3。直接 secret 是 write-only，同方法省略保留，替换轮换，改方法移除。
调用在完成凭据解析后、HTTP dispatch 前再次校验权限与资源；原始错误不进入模型结果。

inner `run.workspace.set` 要求模型选择 Profile `workflow_name` alias；控制 App 仍可通过
原有接口指定所属 Workspace name。程序应答表示接受请求，后续 reload 才提交切换。
Audio `play` 的 index 可省略，保留设备默认曲目语义，不由 Runtime 猜一个索引。

拒绝后的纠正反馈携带同一安全目录与原始用户上下文，供主模型重新核对目标和参数；它不选替代工具、不合成参数、不产生新的授权。语音消息只含音频时，用户序列保留当前实际转写。

间接请求依真实业务规则解释：若系统明确为唯一指名对象的不适表达配置相对幅度，校验使用实际读取和该幅度，不能把它一概判成记录状态。记录、否定、保持现状和多个对象的评价仍不授予新变更；未指名且未配置唯一焦点时须澄清目标。允许任意播放时可使用设备默认曲目，不要求补歌名。

校验模型只返回一个必填的有限 reason 枚举：approved 才批准，其他已声明值拒绝；没有第二个布尔字段。未知值、重复字段、缺失字段、多余字段、尾随数据与 provider 错误仍拒绝执行。

校验按真实用户的时间顺序恢复请求：取消结束旧待补动作，后续孤立数值不恢复旧目标，
此时询问新动作和对象属于必要澄清。只读状态查询与变更授权分别判断；相对调整使用
本轮同一目标的实际读值及业务配置，历史陈述不能代替读取。成功空 ACK 只证明工具
定义的动作，不能证明尚未返回的程序内容或主动开场。语义决定仍可能误判，需真实模型验收。

事实记录不授权该轮修改，但其中唯一明确的本机对象可供后续明确的新动作解析指代；当前轮仍需读取实际状态。目录中的其他对象或助手列举不能替用户制造新选择。多个用户对象的指代仍有歧义，取消后的孤立数值也不恢复旧待补动作。

校验输入与拒绝反馈还提供本轮 MHS 结果的紧凑投影，保留精确 id/hwd、读写操作、实际值和有界错误码；历史、用户陈述、助手提议与其他来源不成为当前执行证明。投影不推断意图或计算参数。`intent_rejected`、`intent_unverified` 和 `invalid_arguments` 表示执行前拒绝，不能把它们描述为设备尝试失败；后续候选仍独立校验。

本轮 ClientTool 结果另保留固定 procedure、按原生调用关联的实际参数、真实返回字段和成功或拒绝状态。
成功空 ACK 表示该参数请求已被接受；校验同一用户请求的后续候选时须考虑此前成功，
不能因 ACK 为空重复执行。它不证明 reload、程序内容或额外副作用，也不是结果缓存；
下一轮明确要求再次执行仍是新请求。历史、助手提议、无关联结果和 HTTP 私有结果不进入该投影。

默认焦点按精确 id/hwd 的对象判断，read/write 两个能力不构成两个默认对象。
明确配置的默认对象优先于名称本身不配置默认值的一般说明；无明确配置仍需澄清。
相对结果按实际读值和业务边界处理，不能把计算越界误当用户给了非法绝对值而漏执行。
音乐回复使用真实返回的曲目标题，不根据风格提示追加未返回的版本或编号。
默认播放的空参数成功 ACK 已满足任意播放请求；不要求补索引或曲名。
实际返回的 `current_index=0` 表示第一首，`state=playing` 表示返回的播放状态。
没有返回标题时可简短确认播放，不猜曲名，也不把设备默认选择说成随机抽取。
ClientTool 候选只有在本轮存在同一固定程序和相同实际参数的成功记录时，才向校验器
提供 `already_completed` 拒绝值；读列表、其他程序、历史、拒绝或不同参数不能提供它。
这只限制不可能的拒绝理由，不自动批准、跳过校验或缓存执行结果；明确重复请求仍由模型判断。

可选参数未被请求时必须省略；`null` 是显式值，不等于省略。Schema 拒绝和语义参数拒绝反馈
说明这一差异，但不替主模型删除字段、补值或选择目标。默认播放仍由模型提交空参数并独立校验。
没有真实历史请求的孤立数值不能恢复待补动作；无本轮成功结果的完成声明仍拒绝。
实际播放返回的零基索引可与同版本的真实列表共同证明曲名；版本不一致或无列表时不推断标题。

执行校验只核对本次候选参数；旧的被拒字段不属于当前省略或更正后的候选。程序选择
的 `workflow_name` 是必需目标；`kickoff=false` 或省略不触发主动开场，`true` 仍须明确授权。
待补绝对亮度后的负数不是相对降低请求；非法绝对值须等待合法更正，不能换算成另一合法值。

程序选择的模型 Schema 仅描述 Profile Workflow alias，不描述已排除的 Workspace 参数。
合法当前候选只含必需目标和省略或 false 的 kickoff 时，不提供 unrequested_parameter 理由；
其余意图、目标、参数和重复执行校验仍独立运行。这不批准候选、不更改参数、不缓存结果。
歧义回复整段都不得先宣布具体目标再询问确认；后续澄清不抵消先前的未授权选择。

负号本身不授权相对降低；这个限制只处理非法绝对数值，不能覆盖用户后来明确提出的
“暗一点”等相对动作。唯一用户对象与本轮实际读取、可信业务幅度共同核对候选值；
已配置的相对幅度不要求用户另给绝对数值，旧状态陈述不表示必须保持该值。

最终回复逐句核对整个草稿。不支持无损暂停续播时，不能先承诺停止再说明不能续播；只说明未执行，或中立询问是否另行停止。用户后来明确要求停止仍是新的可执行请求。`intent_rejected`、`intent_unverified` 和 `invalid_arguments` 是执行前拒绝，不能描述成设备拒绝或设备执行失败；真实设备错误与真实成功读状态按各自结果描述。

普通亮度请求仅补齐对象后，仍需明确询问具体数值；相对默认幅度只用于用户实际提出的方向动作。多目标变更按用户点名顺序串行执行，当前结果用于确认前一项已成功；不能按目录顺序重排，不能重复已成功的动作。

目标撤权或仅可读时，整份最终回复须明确未修改。开头以“我来设置”或执行语气复述命令，不能被后面的不能执行说明抵消；中立确认收到用户值并说明未执行仍可通过。其他目标可写不授予替代操作，后续明确的新请求仍独立判断。
