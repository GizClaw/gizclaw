# services/gameplay

`pkgs/gizclaw/services/gameplay` 拥有 Gameplay catalog、玩家状态、奖励行为和数字资产。Gameplay 配置属于连接的 RuntimeProfile，不再有独立 GameRuleset 资源。

## Ownership

Gameplay 拥有 PetDef、BadgeDef、GameDef、Pet、points account、transaction、reward grant、badge progression 和 game result。RuntimeProfile 的 `resources.pet_defs`、`resources.game_defs` 和 `resources.badge_defs` map 提供 profile-local alias；`gameplay.adoption.pool` 只引用 PetDef alias，`gameplay.pet.games` 以 GameDef alias 为直接 key。

领养 Pet 时，服务先验证当前 connection 的 RuntimeProfile 和 `workflows.system.pet` 中的 canonical Workflow ID，再创建归该 Peer 所有的 system Workspace，并把精确的 RuntimeProfile ID 写入 Pet 和相关状态。Pet Workspace 不保存 persona、conversation、model、voice 或其他执行参数，也不能通过通用 Workspace put 改写。PetDef 不保存 Voice ID/alias 或本地 i18n；它保留角色/说话风格、PIXA 和行为到动画的绑定，展示文本来自 RuntimeProfile 的 `pet_defs` binding。

`driver: pet` 是领域 wrapper，不内置执行图。它的 `pet` 字段嵌套一个相同形状、但禁止再次选择 `pet` 的 Workflow spec，例如 `flowcraft`、`chatroom`、`doubao-realtime` 或 `ast-translate`。Pet wrapper 只向内层 Workflow 注入当前 Pet 上下文；内层 driver 拥有 graph、voice、model 和 toolkit 配置。Memory 只由外层 Workflow 选择一次并传给内层 driver；可复用的内层 spec 不能再声明 Memory alias。所有 alias 都从 system Workspace owner 的 RuntimeProfile 解析，因此替换嵌套 driver 不需要修改 Pet 或 Workspace 数据。

没有有效 PetDef 的 profile 不能领养 Pet；未在当前 profile 中允许的 GameDef 不能提交 game result。非法 alias 和 reward reference 会使 RuntimeProfile validation 失败。删除定义或 RuntimeProfile 不级联删除已有 Gameplay 历史。

## Pet 身份与领养重试

`runtime.adopt` 要求非空的 Peer-scoped `name` 和 `display_name`。这个 name 是长期存在的 Peer-facing Pet resource selector，不是 canonical Admin ID，也不是独立的 operation-level idempotency key。需要安全重试领养的设备在第一次请求前生成并持久化一个有效的 GizClaw custom name；发生 timeout、断线或无法确认响应结果时，继续使用同一个 name。Pet 的 canonical internal ID 始终由 Server 生成。

Pet name 由已认证 Peer 限定 scope。第一次成功创建 `(peer, name)` 时只产生一个 Pet、一个 system Workspace、一条 adoption transaction 和一次 points 扣减；Points 不足的尝试会在占用该 name 或创建 Pet、Workspace、transaction 前失败。同一 active RuntimeProfile 下的成功领养重试返回已有 Pet、当前 Points account 和原始 adoption transaction，不重新选择 PetDef 或产生写入。重试携带不同 `display_name` 也不会重命名已有 Pet；重命名使用 `server.pet.put`。

不同 Peer 可以使用相同文本 Pet name，其全局命名的内部 Workspace 仍彼此独立；所有 Pet RPC 都同时根据 authenticated Peer 和 Pet name 解析资源，因此一个 Peer 不能访问另一个 Peer 的 Pet。同一 Peer 不能跨 RuntimeProfile 复用 name，也不能在删除 Pet 后复用 name，因为保留的 adoption history 会继续占用该 name。

## 固定 Pet 契约

所有 Pet 都拥有同一组 `life`、`health`、`satiety`、`hygiene`、`mood`、`energy` 数值，范围固定为 0..100，领养时全部为 100；成长状态固定为 `experience = 0`、`level = 1`。行为 contract 固定为 `feed`、`bathe`、`play`、`heal`，分别增加 satiety、hygiene、mood、health。PetDef 不定义数值和行为语义，只通过 `visual.bindings.behaviors` 和 `visual.bindings.states` 把固定 contract 绑定到自身 PIXA clip；`idle`、`sick`、`dead` 与可选 `sleep` 是状态动画，不是 Drive 行为。

RuntimeProfile 的 `gameplay.pet` 定义时间规则、升级曲线、每个固定行为的 energy cost/stat delta，以及每个允许 GameDef 的 points/energy cost 和模型奖励策略。行为以 delta 修改数值并在 100 截断；成功行为获得 `energy_cost / energy_per_pet_exp` EXP。Energy 随经过时间被动恢复，不依赖 sleep。

照料数值按每小时配置线性衰减。令归一化缺口为

$$
D(t)=\sum_i w_i\left(1-\frac{s_i(t)}{100}\right),\qquad s_i(t)=\max(0,s_i(0)-r_i t)
$$

则时间区间内的生命损失为

$$
\Delta life=L_{max}\int_0^T D(t)^p\,dt
$$

其中权重和为 1，$p>1$。满状态时缺口为 0，因此 life 不减少；照料数值越低，life 衰减越快。Server 使用分段解析积分，使结果只取决于起始状态和经过时间，不取决于请求频率。

`server.pet.drive` 接受只包含 `pet_id` 的空 Drive，作为由 Server 权威时间驱动的一次 tick。它从 `state_settled_at` 结算经过区间，持久化照料数值衰减、energy 恢复、life 损失和新 checkpoint，并返回更新后的 Pet；它不创建 behavior、game result、cost 或 reward。多个新的连续 tick 与对相同总时长执行一次 tick 得到一致状态。请求携带可选的顶层 idempotency key 时，使用同一 key 重试空 Drive 不会再次结算时间；新 key 或不带 key 表示新的 tick。

life 到 0 时，Pet 在公式计算出的死亡 checkpoint 原子进入 `dead` 并写入不可变 `died_at`，因此终态也不依赖 tick 频率。behavior 和 game-result Drive 不能再作用于 dead Pet；空 Drive 返回其不再变化的终态快照。

升级到下一级所需 EXP 为 `ceil(base_exp + log_scale * ln(current_level))`；`log_scale` 限定为 `0..100`，以保证等级计算工作量有界。累计 EXP 不会被升级消耗。初始 points、领养 weight/cost 和全部 Pet policy 只来自 RuntimeProfile，Server config 没有 fallback。

每个游戏必须在 `resources.game_defs` 和 `gameplay.pet.games` 中显式配置，不存在 default。未配置游戏的提交是精确 no-op：不结算时间、不扣 points/energy、不写 game result、不调用奖励模型、不增加 EXP/badge。已配置游戏先验证资源，再调用当前连接允许的模型；模型只能在配置上限内发放 Pet EXP 和 eligible badge EXP，失败或非法输出不会产生任何 gameplay 写入。idempotency key 保证成功结果不会重复扣费、调用模型或发奖。

## Drive Fact 与 Workspace Memory

每个成功且改变状态的 care behavior 或已配置 game result 都会生成一个固定模板的 `kind=event` Fact。care 使用已提交的 `RewardGrant.ID`，game 使用已提交的 `GameResult.ID`；对应 `Observation.ID` 分别为 `gameplay/drive/reward_grant/<id>` 和 `gameplay/drive/game_result/<id>`。Observation 只包含一个 direct `FactCandidate`，`Scope.AppID` 固定为 `Pet.WorkspaceName`，其他 scope 维度为空。文本和 attributes 只来自已验证并提交的 Pet、result 和 reward 字段，不包含 owner key、provider 配置、credential、idempotency key 或原始 game payload，也不会经过模型重新提取。

Gameplay 在提交 Pet、result 和 reward 的同一个 SQL transaction 中写入 `gameplay_drive_fact_outbox`。空 tick、拒绝的 Drive、资源不足、dead Pet、未配置 game 和已完成 Drive 的幂等重放都不写 outbox。Server 为整个 Gameplay service 启动一条 dispatcher 循环，不会为每只 Pet 创建常驻服务。dispatcher 通过 compare-and-set claim 推进 `pending`、`submitted`、`delivered` 和 `blocked`；provider 返回异步 operation 时，先保存不透明 locator，再通过 `OperationWaiter` 恢复等待。临时错误指数退避，配置或契约错误进入较慢的 blocked 重试。

投递使用 Pet 外层 Workflow 已选择的 Workspace Memory binding，并通过现有 `memorystore.Registry` 租用 `Scope.AppID = Pet.WorkspaceName` 的逻辑 Store。operation 同时记录不含 credential 的物理 binding digest；RuntimeProfile 在 operation 完成前切换物理 binding 时，旧 locator 不会交给新 backend。Pet death 或普通 Pet deletion 不删除 Workspace、outbox 或已投递 Fact；它们继续遵循 Workspace Memory 自身的生命周期。

Gameplay 使用 Workspace owner 和 Pet 领域关系，不创建额外 role 或 policy binding。领养时会独立于 active Pet row 持久化 Pet-to-Workspace binding。Pet 删除在同一个 gameplay SQL database transaction 中创建或复用一条 `kind=pet` PendingDeletion，同时保留 Pet row 及其 binding；该标记不影响 Pet 的读取、list、authorization 或 mutation。不创建 Workspace pending record；points、badge、result、transaction 和 reward grant 历史全部保留。

## Workspace 对话奖励

`gameplay.workspace_reward` 是 RuntimeProfile 可选策略。它把同一 Workspace 中由
AgentHost 新写入的连续 History 合并成 debounce window；window 的第一条 `gear`
记录固定受益 Peer，后续群聊参与者不会改变受益人。Server 启动时只把已有 History
设为 checkpoint，不为导入、回放或升级前的记录补发奖励。只评价同时包含受益人
输入和 Agent 输出的完整 window。

Gameplay 为整个服务运行一个持久 dispatcher。AgentHost append 成功后的 callback
只记录精确 History high-water 并唤醒 dispatcher，不调用模型。window 冻结当前
RuntimeProfile revision、LLM Model resource、Points prompt、BadgeDef `reward_prompt`、
tier、限额和滚动预算；profile 更新只影响后续 window。dispatcher 读取冻结边界内
且 `origin=agenthost` 的文本，执行一次 snapshot-specific `genx.FuncTool` structured
invoke，再在本地验证 score、reason、Badge alias 和 EXP 上限。这个 evaluator 不是
Admin `Tool`、built-in Tool、Toolkit 或 `giztools` capability。

Points tier 映射、BadgeDef ID 解析、滚动预算、`RewardGrant`、Points transaction、
Badge EXP、window 完成和 checkpoint 在同一个 Gameplay SQL transaction 中提交。
模型失败使用有界重试；非法输出进入 terminal blocked 状态；重启后 claim 可以恢复，
相同 window 不会重复发奖。成功改变状态后只向受益 Peer 发送
`GAMEPLAY_REWARD_UPDATED` invalidation Event；客户端收到后重新拉取权威 Gameplay
状态。确定性任务奖励属于独立任务系统，不经过该模型 evaluator。
