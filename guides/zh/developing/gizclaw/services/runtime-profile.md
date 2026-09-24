# RuntimeProfile 与设备注册

`RuntimeProfile` 是设备连接能够看到的运行环境。Workflow、Model、Voice、Tool 和 Path 等真实资源都由管理员创建；Peer 不能创建这些资源，只能创建 Workspace 状态。

## 声明式结构

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata:
  id: default
spec:
  workflows:
    doubao-realtime:
      resource_id: doubao-realtime-conversation
      tags: [6-8岁, 助手]
      i18n:
        en: {display_name: Doubao Assistant}
        zh-CN: {display_name: 豆包助手}
    journey:
      resource_id: flowcraft-journey-guide
      tags: [6-8岁, 故事]
      i18n:
        en: {display_name: Journey Guide}
        zh-CN: {display_name: 旅途向导}
  resources:
    models:
      chat:
        resource_id: doubao-seed-2-0-lite
        i18n:
          en: {display_name: Chat}
          zh-CN: {display_name: 对话}
      extraction:
        resource_id: deepseek-v4-flash
        i18n:
          en: {display_name: Extraction}
          zh-CN: {display_name: 信息提取}
      embedding:
        resource_id: qwen3.7-text-embedding
        i18n:
          en: {display_name: Embedding}
          zh-CN: {display_name: 文本向量}
      asr:
        resource_id: volc-bigasr-sauc
        i18n:
          en: {display_name: Speech Recognition}
          zh-CN: {display_name: 语音识别}
    memories:
      assistant-memory:
        layout_id: assistant-memory
        driver: flowcraft
        connection:
          type: flowcraft_redis8
          url: redis://redis:6379/0
    voices:
      cute-pet:
        resource_id: volc-tenant:volc-main:zh_male_naiqimengwa_mars_bigtts
        i18n:
          en: {display_name: Cute Pet}
          zh-CN: {display_name: 奶气萌宠}
```

`workflows` 是以 alias 为 key 的平面 map。每个 binding 可带 `tags` 字符串数组；Server 只做精确字符串匹配，不解析年龄或内容类别。查询传多个 tag 时取交集；不传 tag 返回全部。Tag 不参与 Workflow 身份，修改 tag 不改变已有 Workspace 的 workflow name。RuntimeProfile 创建或更新时会验证每个引用的真实 Workflow ID、其 driver，以及 Workflow 内部使用的 Model、Voice、Tool alias。Friend 与 Friend Group 的 Workspace 固定绑定内置 `system-sfu` Workflow，不经 RuntimeProfile 选择，见 [services/social](/zh/developing/gizclaw/services/social#sfu-workspace)。

Workflow alias 位于 `workflows.<alias>`，在 RuntimeProfile 内唯一。客户端自行决定菜单、顺序、图标和 tag 的展示文本；RuntimeProfile 提供 Workflow 成员、tags 以及 alias 自己的 `en`、`zh-CN` 显示文本。

`resources` 下的 map 把环境 alias 绑定到管理员创建的真实资源 ID。Model alias 表示 `chat`、`extraction`、`embedding`、`asr`、`realtime`、`translation` 这类稳定用途，不包含 provider 或真实 Model 名。Model 和 Voice alias 是互相独立的环境变量，不属于 Workflow tags。Workflow spec 和 Workspace 参数保存符号 alias；每次 Workspace reload 都从当前 RuntimeProfile 重新解析。因此同一个 App 或固件可以切换生产、调试 RuntimeProfile，而无需重新构建。

`resources.tools` 绑定供 AI 和 Workflow runtime 使用的 Admin HTTP Tool。设备过程调用使用预定义的 `tool/v0` 注册表，与这个目录互相独立。

每个 RuntimeProfile alias 都是总长 1–63 字节、由 `.` 分隔的 lowercase kebab-case segment。`asr`、`extract` 等无点名称表示共享能力；`journey.model`、`journey.narrator`、`story.journey-center-earth` 等名称表示可独立绑定的 consumer 槽位。完整名称始终是平面 map 中的一个 opaque key；Server 原样保留，不按 segment 查找，不支持 prefix、wildcard，也不会从 `journey.narrator` fallback 到 `narrator`。`journey.narrator` 与 `journey-narrator` 是两个不同 alias。空 segment、下划线以及 segment 内的首尾连字符均不合法。

`resources.memories` 是产品拥有的长期 Memory 部署 binding。每个 alias 选择一个 Admin `MemoryLayout`、一个 driver 和唯一的 typed connection。封闭的 connection variant 包括：托管本地 `flowcraft_bbh`、显式目录 `flowcraft_object_store`、DSN 形式的 `flowcraft_postgresql`、Redis 8.4+ URL 形式的 `flowcraft_redis8`、带 endpoint/API key/Project ID 的 `mem0`，以及带 endpoint/API key/Memory Project ID 的 `volc_mem0`。`flowcraft_bbh` 将数据写入 Server Workspace root，不依赖外部服务；`flowcraft_redis8` 接受 `redis://` 或启用证书校验的 `rediss://`，后者可选 `tls_ca_file`。外部连接值直接保存在这个仅 Admin 可读的 RuntimeProfile 中，不引用 Credential，也不会通过 Peer API projection 暴露。Driver 必须与 connection type 匹配；Flowcraft Layout 使用的 model alias 必须存在于同一 RuntimeProfile。

这个 binding alias 表示 Workflow 标量 `memory` 字段选择的 named physical source。在相同 Workspace、driver 与 physical binding 下，修改 extraction policy、Graph Recall/Observe policy、prompt 或 `top_k` 不会创建新的 canonical data namespace；修改 driver 或 connection 可以切换到另一个数据源，但不会自动迁移或删除旧数据。删除 Workspace 时只清除它在当前 binding 中的数据，见 [Memory Store](/zh/developing/stores/memory#memorylayout、runtimeprofile-与-workflow)。

`flowcraft_bbh` 不再是受支持的 connection。仍使用它的已持久化 profile 会在读取或 runtime 解析时被拒绝，错误会指出具体 profile 与 binding；管理员仍可通过 `PUT` 将其显式替换为 `flowcraft_redis8` 或 `flowcraft_object_store`。Profile 被拒绝、替换或删除时，GizClaw 都不会迁移、重新解释或删除旧的 managed local directory；operator 必须先保留或备份该目录，并在切换 binding 前显式完成所需的数据转移。

## app_config

`app_config` 随同一条 RuntimeProfile 记录保存在 `app_config_json` 列中，创建、更新和所有读取路径都保留原始字符串值。省略配置保存为 JSON `null`，显式空 map 保存为 `{}`；两者都表示没有下发配置，更新时会清除旧值。初始化为缺少该列的已有表补列，保留其他字段和版本；原先未保存的配置需由管理员重新提交。

`spec.app_config` 是可选的不透明设备配置下发通道，把设备自己的产品配置放进它已经使用的 RuntimeProfile，而不需要重新构建固件。它是一个 key-value map：key 使用与其他 RuntimeProfile alias 完全相同的语法（1–63 字节、`.` 分隔的 lowercase kebab-case segment），value 是任意 string。

Server 原样存储并返回每个 value：不解析、不 trim、不做编码转换，也不校验它是不是 JSON。value 用什么格式由设备自己决定。Server 只校验 key 语法、单个 value 不超过 4096 字节、条目不超过 64 个；normalize 后出现重复 key 时拒绝整次写入。key 语法与 value 的字节上限都在 Server 归一化时校验：OpenAPI 3.0 没有 `propertyNames`，`maxLength` 也只能表达字符数，而设备侧按 UTF-8 字节静态分配，因此归一化比 schema 更严格。

```yaml
spec:
  app_config:
    ui.theme: dark
    app.entrypoints: |
      {"home": "/tab/home", "settings": "/tab/settings"}
    feature.flags: beta-voice,beta-pet
```

app_config key 与 Workflow、Model、Voice、Tool 等 binding alias 属于互相独立的命名空间，不参与全局 alias 唯一性检查：`app_config` 里的 `chat` 与 `resources.models` 里的 `chat` 互不冲突。

设备通过 `server.app_config.list` 与 `server.app_config.get` 只读访问，没有写入方法；同一个 RuntimeProfile 下的所有设备读到相同内容，不存在 per-Peer 配置。任何持有该绑定的已注册设备都能读到全部 key 与 value，因此这里不能存放 credential、API key 或任何 secret；凭证仍由 Credential 与 ProviderTenant 在 Server 侧解析，不进入 projection。

app_config 参与 spec 归一化和 revision 计算，因此改配置就会产生新的 revision，设备可以缓存 revision 并在未变化时跳过重新拉取。

规范化后的 spec 有确定性的 opaque revision。Catalog list/get 响应携带 RuntimeProfile ID 与 revision，分页 cursor 与 revision 绑定。每次 list、get、Workspace reload 和 standalone Speech 调用使用一个一致快照；并发更新从下一次操作开始生效。

RuntimeProfile 在创建和更新时校验完整依赖图，再发布新的 revision。Workspace reload 等快照读取信任已经持久化的 revision，不会再次遍历 Workflow、Model、Voice、Tool 或 Memory 依赖。每个 consumer 只解析自己实际使用的 binding：选中的依赖不可用时由该 consumer 返回错误，而无关资源不可用不会阻塞快照或不受影响的 Workspace。

## RegistrationToken

`RegistrationToken` 是普通的 Admin binding 资源。它自己的 `metadata.id` 由调用方提供；必填的 `spec.token` 通过 `runtime_profile_id` 绑定一个 RuntimeProfile canonical ID，也可以通过 `firmware_id` 独立绑定一个 Firmware ID。Admin create、put、get、list、delete、apply 和 show 使用同一份可读状态。Server 持久化完整状态并维护 token 唯一索引；修改 token 时会原子更新索引，重复 apply 相同 ID 和配置则返回 unchanged。

RuntimeProfile 与 RegistrationToken 的部署 ownership 相互独立。Raids 提供可复用基础资源及
公开的 `RuntimeProfile/default`、`RegistrationToken/default-runtime` 契约。Desktop 为本地
Server 消费这对资源；其中确定性 UUID 是公开注册标识，不是 Admin 凭证。产品平台和其他
部署仍拥有自己的 RegistrationToken，可独立安装 default 或产品专用 profile，并把显式
token 绑定到任意一个。

`server.register` 把连接关联到 RuntimeProfile，内部持久化 canonical RuntimeProfile ID 与可选 Firmware ID。`runtime_profile_name` wire 字段原样携带 canonical RuntimeProfile ID，因为 RuntimeProfile 没有独立的 Peer name；这是正常的 Peer name 投影规则，不是兼容字段。Registration 不返回 Firmware identity；Server 只通过内部 `firmware_id` binding 解析 Firmware，`server.firmware.get` 仅返回所选 channel 的配置。Owner-bound Workspace 即使在 owner 离线时，也会通过持久化的 canonical RuntimeProfile ID 解析当前 revision；owner 后续成功注册可替换该选择。RegistrationToken 和 Peer 都不保存 Firmware channel；stable、beta 或 develop 由设备自行选择。更新或切换 RuntimeProfile 只改变后续操作使用的环境，不重写 Workspace context 或已经保存的内部 binding。

RegistrationToken 通过可靠 Peer connection 上的 `server.register` 完成注册。启用 [registration-token 准入 policy](../server/security-policy) 时，也可在 WebRTC offer 的 AEAD 内部携带相同 token，供只读准入检查；这不绑定 owner、firmware 或 runtime，token 不会沿 Conn 传递。其他 Public HTTP endpoint 不接受 RegistrationToken。注册和握手日志均不包含提交的 token 值。

### 生命周期与激活

Admin create/put 与声明式 `spec` 支持 `enabled`（省略为 true）、`expires_at`（RFC3339 时间，省略或 null 为永不过期）和 `max_activations`（非负整数，省略或 null 为不限次数）。过期时间是排他的：到达该时刻即不再接受新激活。上限为 0 表示不接受新激活。Admin get/list/create/put/delete 响应的 `activation_count` 返回当前激活数量。

一次激活是一个新公钥第一次用该 token 成功执行 `server.register`。SQL `registration_token_activations` 以 `(token_id, peer_public_key)` 为唯一键，保存第一次激活的 `activated_at`；计数来自行数。相同 token 与公钥重复注册不新增记录，即使管理员后来禁用、缩短有效期或降低上限，已有激活仍可幂等注册。token 被删除或值被替换后，旧值不再有效；删除 token 同时删除它的激活记录，保留已有 owner/firmware 绑定。

`server.register` 是权威写入点：事务先取得 token 写锁，再判断 enabled、到期时间、数量和是否已激活，插入激活记录，同时写入 `runtime_profile_owners` 的 RuntimeProfile 与 firmware 绑定。SQLite 与 PostgreSQL 都在读之前取得写锁；两个新设备争最后一个名额时只能成功一个。拒绝返回 `PermissionDenied`，SQL 失败回滚整个事务。只有提交成功后才发布连接内快照。Peer 的 firmware 读取优先使用这份 SQL 绑定，尚无 SQL firmware 的老设备继续读取既有 Peer 数据。

管理员可延长或缩短有效期、调高或调低上限，包括调到低于已激活数；这些操作沿用 incarnation / row_version 乐观并发，冲突返回 409。省略或 null 会清除可空限制，省略 enabled 恢复 true。禁用、过期与降低上限只阻止新激活，不吊销已有设备；已知且可用的 Peer 不带凭证重连仍成功。握手只读预检不预留名额，预检后被其他设备抢完额度时，register 仍会拒绝且不产生半绑定。管理员恢复 token 后负缓存最多保留 1 秒，独立的失败预算仍可能阻止该窗口内的查询。

旧数据库初始化会分别用 PostgreSQL 的 `ADD COLUMN IF NOT EXISTS` 与 SQLite 的列检查/`ADD COLUMN` 补齐新列。已有 token 默认启用、永不过期、不限次数；迁移不推测历史 token 激活归属，已有 owner/firmware 绑定和无凭证重连保持可用。

```yaml
spec:
  token: device-enrollment
  runtime_profile_id: default
  enabled: true
  expires_at: "2035-01-01T00:00:00Z"
  max_activations: 100
```

CLI 的 `admin registration-tokens create/put -f` 接受对应 JSON 字段；Terraform 的 `gizclaw_resource` 使用同一 spec，省略默认值不会造成持续 plan 差异。`web/console` 是监控控制台，没有 RegistrationToken 管理页面。

## Peer surface 与 ownership

- Workflow、Model、Voice 和 Tool list/get 只返回安全的 scoped-name projection。AST Workflow projection 会携带 Workspace 默认语言对，客户端不再从动态 name 推断行为；projection 不暴露真实 ID、provider、tenant、credential、owner 或 executor routing。
- Workflow list 可传多个 `tags` 做交集筛选；Workflow get 只传当前 RuntimeProfile 投影出的 name；不存在 `source=runtime|owned`。
- Peer RPC 不提供 Workflow、Model、Credential 和 Tool create/put/delete；真实资源统一由 Admin 管理。
- Workspace create 必须传 `workflow_name`，Workspace list 不要求过滤条件。Server 在内部 Workspace label 中保存创建时的 workflow name；Peer RPC 不返回通用 labels。同一个 typed create capability 也供 OpenAI Conversation 创建使用；Admin 不能 create 或 apply Workspace。
- Workflow binding 删除后，不隐藏也不删除 Workspace。list/get 仍返回 Workspace，reload/run 在相同 Peer name 恢复前返回 not found。

Firmware 仍是独立 Admin 资源，不进入 RuntimeProfile projection。RegistrationToken 可以独立绑定 Firmware ID，但不绑定 channel。Credential 与 ProviderTenant 只是真实 Model、Voice 在 Server 侧使用的依赖，不会暴露给设备。

RuntimeProfile 使用 SQL `runtime_profiles`、`registration_tokens`、`registration_token_activations` 和 `runtime_profile_owners` 表。资源配置保留 JSON，身份、版本、限制和绑定分别保存为列。列表把游标与数量限制下推 SQL，Profile 与 token 更新/删除比较 incarnation 和 row_version。注册的事务与快照发布按同一 owner 串行，无关 owner 可并行；token 写锁保证跨进程限额一致。

`services/runtime/runtimeprofile` 在 Server 初始化时从持久 SQL 的所有 RuntimeProfile 构建一份纯内存 SQLite 索引。持久 SQL 仍保存完整 Profile 且是权威数据；内存库把每个 Workflow、Model、Voice、Tool、Memory、app_config、safety fence 与 MHS v0 device 拆成独立的 `(runtime_profile_id, kind, name, value_json)` 行，并把 Workflow tags 拆成可检索行。`Index.ListProfileIDs` 枚举全部 Profile，`Index.GetEntry` 按 Profile ID、kind 和 name 精确读取，`Index.ListEntries` 可跨 Profile 按 kind 读取条目，`Index.ListWorkflowsByTags` 对多个普通字符串 tag 做 AND 查询；设备 Workflow catalog 也按 Profile revision 从这个 SQLite 快照筛选；这些内部查询不读取磁盘，也不向 Peer 暴露包含凭证的条目。内存 SQLite 实例发布后设为只读。RuntimeProfile 在本 Server 持久提交后立即重建一个新实例，后台也每 5 分钟从持久 SQL 重建；新实例完成后原子切换，并关闭旧实例。其他 Server 的写入由下一次定时轮换纳入，也可调用 `RefreshMemoryIndex` 提前重建。进程关闭时释放内存库，重启后从持久数据重建。

Admin 创建和更新 registration token 时，原始输入必须不超过 512 个 UTF-8 字节（不是 512 个字符），与 admission value 上限一致；超限返回 400，不能写入数据库。

## Workspace 安全围栏

Workspace 六个 AI driver 的可选 `safety_fence_level` 固定为 `off`、`general`、`child`。`off` 不注入文本；`general` 用于约束色情、暴力、违法等 NSFW 内容；`child` 进一步要求儿童适龄。GizClaw 只定义级别，不内置任何围栏文案。租户在 RuntimeProfile 的 `spec.safety_fences.general.prompt` 和 `spec.safety_fences.child.prompt` 分别配置完整文本，长度为 1–4096 个 Unicode 字符。`child` 不继承、也不拼接 `general`；儿童档需要的全部规则必须写在自己的 prompt 中。没有 `off` 配置项。

参数缺省时保留 Workspace 已存值；从未设置等同于 `off`。显式修改从下一次 reload 生效。设备可以在 `server.run.workspace.reload-with-options.parameters` 中与 `input` 一起发送，无需额外调用。非法枚举在 parameters.set、reload-with-options、create 和 put 被拒绝（RPC `INVALID_ARGUMENT`，Admin HTTP put 为 400）。SFU system Workspace 接受合法值并 no-op，不保存、不解析 Profile。

支持系统提示的 driver 在 reload 时，从 Workspace owner 绑定的 RuntimeProfile 当前快照解析非 off 档位。缺少所选条目、prompt 无效或 Profile 不可用时，reload 明确失败；缺档错误包含 Workspace 名、级别和 Profile ID，不会静默降级为 off。参数更新与 reload 并非同一事务：reload 失败不会撤销已经保存的级别，调用方需修复 Profile 或显式改为 off 后重试。

GizClaw 从不自行决定围栏放在哪里：它只把所选档位的文案（`off` 时为空字符串）以一个具名变量交给 Workflow，是否使用、放在哪个提示词的哪个位置，都由 Workflow（包括 raid 中的各个 Workflow）自己决定。没有引用该变量的 Workflow 不受围栏影响。

| Driver | Workflow 中的引用方式 |
| --- | --- |
| Flowcraft | 每轮执行前写入 Board 变量 `safety_fence`；在 LLM 节点的 `system_prompt` 中写 `${board.safety_fence}`。同名的产品 Board 输入会被覆盖。 |
| Eino | 保留 binding `input.safety_fence`（`string`），batch、race 与子图继承同一值；prompt 节点通过 `inputs: {safety_fence: {from: input.safety_fence}}` 绑定后在模板中引用。 |
| Doubao Realtime、Doubao Realtime Duplex、DashScope Realtime | 在 Workflow 或 Workspace 的 `instructions` 中写占位符 `${input.safety_fence}`。围栏以 `safety_fence` transformer pattern 参数下发，peergenx 构建 transformer 时替换全部占位符并去掉首尾空白；没有占位符的 instructions 原样传给 provider。占位符带点号，因此不会被 `gizclaw admin apply` 与 Terraform provider 的 `${NAME}` 环境变量展开误替换。 |

ASTTranslate 的当前 provider 路径没有系统提示入口：合法级别被接受并保存，但不提供变量，也不解析 Profile；Profile 未配置该档同样不影响 reload。SFU system Workspace 同理。设备因此可以把同一个级别发给全部 Workspace。

围栏是发给模型的系统提示，约束效果仍依赖所选模型；这项配置不提供独立的内容审核器。

## MHS v0 硬件清单

`spec.mhs.v0` 是 RuntimeProfile 拥有的硬件清单，不由设备上报。`mhs/v0` 是 GizClaw 自己的、受 MHS 启发的预标准协议，不声称与官方 Model Hardware Standard 兼容；未来官方兼容版本使用 `v1`。`mhs/v0` 仅处理状态读写；预定义的设备过程调用由 `tool/v0` 承载。两者都不提供变更通知、slot 或 stream。

```yaml
spec:
  mhs:
    v0:
      devices:
      - id: display.main
        kind: display
        description: 主屏幕
        tags: [正面显示]
        states:
        - {name: brightness, type: int, access: read_write, min: 0, max: 100, step: 5, unit: '%'}
      - id: battery.main
        kind: battery
        states:
        - {name: level, type: int, access: read, min: 0, max: 100, unit: '%'}
```

`devices[].id` 全局唯一，`states[].name` 在设备内唯一，均为 1–64 ASCII 字节，匹配 `^[a-z][a-z0-9]*([.-][a-z0-9]+)*$`。`kind` 是非空开放字符串；description、unit 与自然语言 tags 可选。state type 固定为 `bool`、`int`、`double`、`string` 或 `enum`，access 为 `read` 或 `read_write`。

只有 int/double 可以声明有限的 min/max/step，min 不大于 max，step 必须为正；int 的约束和取值都是 ±9007199254740991 内的整数，以保证 JSON/JavaScript 精确表达。步进以 min（缺省为 0）为起点，按十进制值计算网格。enum 必须提供非空且不重复的 enum_values，其他类型禁止该字段。string/enum 值必须是无 NUL 的合法 UTF-8，最多 256 字节。创建、PUT 与 apply 在保存前验证整份清单，错误包含设备和状态位置。

清单保存在 `runtime_profiles.mhs_json`，参与 spec revision；已有 SQL 表初始化时补列，旧记录缺省为 null。Admin 的 get/list/put/apply/show 使用同一 shared schema。省略 mhs 会清除旧清单。控制 App 的专用 manifest endpoint 只投影清单，不暴露 Profile 的资源绑定或凭据，设备离线时同样可读；未定义清单返回 `{"devices":[]}`。见 [Public API](/zh/developing/api/http/public#mhs-v0-硬件状态)。
