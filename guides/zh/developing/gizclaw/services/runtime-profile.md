# RuntimeProfile 与设备注册

`RuntimeProfile` 是设备连接能够看到的运行环境。Workflow、Model、Voice、Tool、PetDef、GameDef、BadgeDef 和 Path 等真实资源都由管理员创建；Peer 不能创建这些资源，只能创建 Workspace 状态和领养 Pet 实例。

## 声明式结构

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata:
  id: default
spec:
  workflows:
    system:
      friend_chatroom: chatroom
      group_chatroom: chatroom
      pet: pet-care
    collections:
      assistants:
        doubao-realtime:
          resource_id: doubao-realtime-conversation
          i18n:
            en: {display_name: Doubao Assistant}
            zh-CN: {display_name: 豆包助手}
      raids:
        journey:
          resource_id: flowcraft-journey-guide
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
      pet-memory:
        layout_id: pet-memory
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
    pet_defs:
      codex:
        resource_id: petdef-codex
        i18n:
          en: {display_name: Codex}
          zh-CN: {display_name: Codex}
  gameplay:
    points:
      initial_balance: 100
    adoption:
      pool:
        - {pet_def: codex, weight: 100, rarity: common, adoption_cost: 10}
    pet:
      time:
        care_decay_per_hour: {health: 0.5, satiety: 1.3888888889, hygiene: 0.7, mood: 1}
        energy_recovery_per_hour: 10
        life_decay:
          max_loss_per_hour: 4
          exponent: 2
          contributing_weights: {health: 0.25, satiety: 0.25, hygiene: 0.25, mood: 0.25}
      experience:
        energy_per_pet_exp: 5
        leveling: {base_exp: 30, log_scale: 10}
      actions:
        feed: {energy_cost: 10, stat_delta: 10}
        bathe: {energy_cost: 10, stat_delta: 10}
        play: {energy_cost: 10, stat_delta: 10}
        heal: {energy_cost: 10, stat_delta: 10}
      games: {}
```

`workflows.system` 的三个值是管理员创建的真实 Workflow ID，不是 Collection alias。私聊与群聊分别使用 `friend_chatroom`、`group_chatroom`，Pet 领养使用 `pet`。RuntimeProfile 创建或更新时会验证这些 ID、预期的外层 driver，以及 Workflow 内部使用的 Model、Voice、Tool alias。

可选 Workflow alias 位于 `workflows.collections.<collection>.<alias>`。Alias ID 在所有 Collection 之间全局唯一；客户端拥有固定的 Collection 菜单、顺序、图标与 Collection 翻译。RuntimeProfile 只提供动态 Workflow 成员，以及 alias 自己的 `en`、`zh-CN` 显示文本，不包含顶层 locale 或 Collection 展示配置。

`resources` 下的 map 把环境 alias 绑定到管理员创建的真实资源 ID。Model alias 表示 `chat`、`extraction`、`embedding`、`asr`、`realtime`、`translation` 这类稳定用途，不包含 provider 或真实 Model 名。Model 和 Voice alias 是互相独立的环境变量，不属于 Workflow Collection。Workflow spec 和 Workspace 参数保存符号 alias；每次 Workspace reload 都从当前 RuntimeProfile 重新解析。因此同一个 App 或固件可以切换生产、调试 RuntimeProfile，而无需重新构建。

每个 RuntimeProfile alias 都是总长 1–63 字节、由 `.` 分隔的 lowercase kebab-case segment。`asr`、`extract` 等无点名称表示共享能力；`journey.model`、`journey.narrator`、`story.journey-center-earth` 等名称表示可独立绑定的 consumer 槽位。完整名称始终是平面 map 中的一个 opaque key；Server 原样保留，不按 segment 查找，不支持 prefix、wildcard，也不会从 `journey.narrator` fallback 到 `narrator`。`journey.narrator` 与 `journey-narrator` 是两个不同 alias。空 segment、下划线以及 segment 内的首尾连字符均不合法。

`resources.memories` 是产品拥有的长期 Memory 部署 binding。每个 alias 选择一个 Admin `MemoryLayout`、一个 driver 和唯一的 typed connection。封闭的 connection variant 包括：显式目录 `flowcraft_object_store`、DSN 形式的 `flowcraft_postgresql`、Redis 8 URL 形式的 `flowcraft_redis8`、带 endpoint/API key/Project ID 的 `mem0`，以及带 endpoint/API key/Memory Project ID 的 `volc_mem0`。`flowcraft_redis8` 接受 `redis://` 或启用证书校验的 `rediss://`，后者可选 `tls_ca_file`。外部连接值直接保存在这个仅 Admin 可读的 RuntimeProfile 中，不引用 Credential，也不会通过 Peer API projection 暴露。Driver 必须与 connection type 匹配；Flowcraft Layout 使用的 model alias 必须存在于同一 RuntimeProfile。

这个 binding alias 表示 Workflow 标量 `memory` 字段选择的 named physical source。在相同 Workspace、driver 与 physical binding 下，修改 extraction policy、Graph Recall/Observe policy、prompt 或 `top_k` 不会创建新的 canonical data namespace；修改 driver 或 connection 可以切换到另一个数据源，但不会自动迁移或删除旧数据。

`flowcraft_bbh` 不再是受支持的 connection。仍使用它的已持久化 profile 会在读取或 runtime 解析时被拒绝，错误会指出具体 profile 与 binding；管理员仍可通过 `PUT` 将其显式替换为 `flowcraft_redis8` 或 `flowcraft_object_store`。Profile 被拒绝、替换或删除时，GizClaw 都不会迁移、重新解释或删除旧的 managed local directory；operator 必须先保留或备份该目录，并在切换 binding 前显式完成所需的数据转移。

每个 `gameplay.adoption.pool` 条目只引用一个 `pet_defs` alias；PetDef 的本地化名称也来自这个 RuntimeProfile binding，不在 PetDef 中重复保存 i18n。PetDef 只保存宠物角色/说话风格、PIXA 元数据和固定行为到动画 clip 的绑定。Pet Workflow 使用的 Model、Voice 和 Tool 都由真实 Workflow spec 中的 alias 声明，并从该 system Workspace owner 的 RuntimeProfile 解析。

`gameplay.pet` 必须完整配置固定 Pet 的时间衰减、被动 energy 恢复、升级曲线与四个标准行为。`games` 没有 default；每个 key 必须同时存在于 `resources.game_defs`，并独立配置 energy/points cost 与 reward model、prompt 和奖励上限。未配置 GameDef 的 Drive 是无写入的 no-op。

`gameplay.workspace_reward` 配置基于 Workspace 对话质量的 AI 奖励。启用时必须完整
声明允许的 Workspace 种类、debounce、transcript 上限、LLM evaluator、Points
tier、Badge allowlist 和滚动预算。evaluator model 是 `resources.models` 中的普通
LLM alias；每个 Badge alias 必须存在于 `resources.badge_defs`，对应 BadgeDef 还要
声明非空 `reward_prompt`。仅发放 Points 时，`badges` map 可以为空。例如：

```yaml
resources:
  models:
    reward-evaluator:
      resource_id: reward-evaluator-model
  badge_defs:
    science:
      resource_id: badge-science
gameplay:
  points:
    initial_balance: 100
  workspace_reward:
    enabled: true
    workspace_kinds: [workflow, direct_chatroom, group_chatroom]
    debounce: {quiet_period: 2m, max_window_age: 15m}
    transcript: {max_entries: 100, max_text_bytes: 65536}
    evaluation:
      model: reward-evaluator
      points_prompt: Reward thoughtful conversation and demonstrated learning progress.
      score_min: 0
      score_max: 100
      qualifying_score: 80
    points:
      tiers:
      - {min_score: 80, delta: 5}
      - {min_score: 90, delta: 10}
    badges:
      science: {max_exp_per_window: 5}
    rolling_budget: {period: 24h, points_max: 50, badge_exp_max: 20}
```

`workspace_reward: {enabled: false}` 是规范化的关闭形式；字段缺失也表示不发放对话
奖励。策略在每个 debounce window 开始时冻结，后续 RuntimeProfile 或 BadgeDef
更新只作用于新 window。它不注册 Admin Tool、built-in Tool 或 Toolkit。

规范化后的 spec 有确定性的 opaque revision。Catalog list/get 响应携带 RuntimeProfile ID 与 revision，分页 cursor 与 revision 绑定。每次 list、get、Workspace reload 和 standalone Speech 调用使用一个一致快照；并发更新从下一次操作开始生效。

RuntimeProfile 在创建和更新时校验完整依赖图，再发布新的 revision。Workspace reload 等快照读取信任已经持久化的 revision，不会再次遍历 Workflow、Model、Voice、Tool、Memory 或 gameplay 依赖。每个 consumer 只解析自己实际使用的 binding：选中的依赖不可用时由该 consumer 返回错误，而无关资源不可用不会阻塞快照或不受影响的 Workspace。

## RegistrationToken

`RegistrationToken` 是普通的 Admin binding 资源。它自己的 `metadata.id` 由调用方提供；必填的 `spec.token` 通过 `runtime_profile_id` 绑定一个 RuntimeProfile canonical ID，也可以通过 `firmware_id` 独立绑定一个 Firmware ID。Admin create、put、get、list、delete、apply 和 show 使用同一份可读状态。Server 持久化完整状态并维护 SHA-256 lookup index；修改 token 时会原子替换 index，重复 apply 相同 ID 和配置则返回 unchanged。

RuntimeProfile 与 RegistrationToken 的部署 ownership 相互独立。Raids 提供可复用基础资源及
公开的 `RuntimeProfile/default`、`RegistrationToken/default-runtime` 契约。Desktop 为本地
Server 消费这对资源；其中确定性 UUID 是公开注册标识，不是 Admin 凭证。产品平台和其他
部署仍拥有自己的 RegistrationToken，可独立安装 default 或产品专用 profile，并把显式
token 绑定到任意一个。

`server.register` 把连接关联到 RuntimeProfile，内部持久化 canonical RuntimeProfile ID 与可选 Firmware ID。`runtime_profile_name` wire 字段原样携带 canonical RuntimeProfile ID，因为 RuntimeProfile 没有独立的 Peer name；这是正常的 Peer name 投影规则，不是兼容字段。Registration 不返回 Firmware identity；Server 只通过内部 `firmware_id` binding 解析 Firmware，`server.firmware.get` 仅返回所选 channel 的配置。Owner-bound Workspace 即使在 owner 离线时，也会通过持久化的 canonical RuntimeProfile ID 解析当前 revision；owner 后续成功注册可替换该选择。RegistrationToken 和 Peer 都不保存 Firmware channel；stable、beta 或 develop 由设备自行选择。更新或切换 RuntimeProfile 只改变后续操作使用的环境，不重写 Workspace context 或已经保存的内部 binding。

RegistrationToken 只通过可靠 Peer connection 上的 `server.register` 提交。注册成功或失败日志不包含提交的 token 值；Public HTTP 不接受 RegistrationToken。

## Peer surface 与 ownership

- Workflow、Model、Voice 和 Tool list/get 只返回安全的 scoped-name projection。AST Workflow projection 会携带 Workspace 默认语言对，客户端不再从动态 name 推断行为；projection 不暴露真实 ID、provider、tenant、credential、owner 或 executor routing。
- Workflow list 必须传 Collection；Workflow get 只传当前 RuntimeProfile 投影出的 name；不存在 `source=runtime|owned`。
- Peer RPC 不提供 Workflow、Model、Credential 和 Tool create/put/delete；真实资源统一由 Admin 管理。
- Workspace create 必须传 `collection` 与 `workflow_name`，Workspace list 必须传 `collection`。Server 把 Collection 保存为内部 Workspace label，但 Peer RPC 不返回通用 labels。同一个 typed create capability 也供 OpenAI Conversation 创建使用；Admin 不能 create 或 apply Workspace。
- Workflow binding 删除后，不隐藏也不删除 Workspace。list/get 仍返回 Workspace，reload/run 在相同 Peer name 恢复前返回 not found。
- Pet 实例仍是 Peer/领域状态；领养与所有 reward 数值都来自 `gameplay`，Server config 只保存运行参数。

Firmware 仍是独立 Admin 资源，不进入 RuntimeProfile projection。RegistrationToken 可以独立绑定 Firmware ID，但不绑定 channel。Credential 与 ProviderTenant 只是真实 Model、Voice 在 Server 侧使用的依赖，不会暴露给设备。

Mutation coordination 分别归属 owner、profile ID、token ID 与 token hash。跨 key mutation 按 canonical 顺序取锁，在保留 binding commit/rollback 和 token index 原子性的同时允许无关 owner/profile/token 继续。Registration commit 仍处于同 owner transaction 边界，但不持有 Server 全局 mutation mutex。
