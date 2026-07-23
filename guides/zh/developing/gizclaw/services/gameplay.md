# services/gameplay

`pkgs/gizclaw/services/gameplay` 拥有 Gameplay catalog、玩家状态、奖励行为和数字资产。Gameplay 配置属于连接的 RuntimeProfile，不再有独立 GameRuleset 资源。

## Ownership

Gameplay 拥有 PetDef、BadgeDef、GameDef、Pet、points account、transaction、reward grant、badge progression 和 game result。RuntimeProfile 的 `resources.pet_defs`、`resources.voices`、`resources.game_defs` 和 `resources.badge_defs` map 提供 profile-local alias；`gameplay.adoption.pool` 同时引用 PetDef 与 Voice alias，`gameplay.pet.games` 以 GameDef alias 为直接 key。

领养 Pet 时，服务从当前 connection 的 RuntimeProfile snapshot 解析规则，把池条目的 Voice alias 保存到 system Workspace，并把 RuntimeProfile 名写入 Pet 和相关状态。PetDef 不保存 Voice ID/alias；它保留角色/说话风格、PIXA 和行为到动画的绑定。Pet 创建的 system Workspace 使用内置 `pet-care` Workflow；`pet-care` 不需要出现在 RuntimeProfile 的 `workflows` map 中。

没有有效 PetDef 的 profile 不能领养 Pet；未在当前 profile 中允许的 GameDef 不能提交 game result。非法 alias 和 reward reference 会使 RuntimeProfile validation 失败。删除定义或 RuntimeProfile 不级联删除已有 Gameplay 历史。

## Pet 身份与领养重试

`runtime.adopt` 接受可选的调用方 `id`。这个 ID 是长期存在的 Pet resource identity，不是独立的 operation-level idempotency key。需要安全重试领养的设备在第一次请求前生成并持久化一个有效的 GizClaw custom ID；发生 timeout、断线或无法确认响应结果时，继续使用同一个 ID。

Pet ID 由已认证 Peer 限定 scope。第一次成功创建 `(peer, id)` 时只产生一个 Pet、一个 system Workspace、一条 adoption transaction 和一次 points 扣减；Points 不足的尝试会在占用该 ID 或创建 Pet、Workspace、transaction 前失败。同一 active RuntimeProfile 下的成功领养重试返回已有 Pet、当前 Points account 和原始 adoption transaction，不重新选择 PetDef 或产生写入。重试携带不同 `display_name` 也不会重命名已有 Pet；重命名使用 `server.pet.put`。

不同 Peer 可以使用相同文本 Pet ID，其全局命名的内部 Workspace 仍彼此独立；所有 Pet RPC 都同时根据 authenticated Peer 和 Pet ID 解析资源，因此一个 Peer 不能访问另一个 Peer 的 Pet。同一 Peer 不能跨 RuntimeProfile 复用 ID，也不能在删除 Pet 后复用 ID，因为保留的 adoption history 会继续占用该 ID。不传 `id` 时保持 Server 生成 ID 的行为，每次成功调用仍表示一次新的领养。

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

Gameplay 使用 Workspace owner 和 Pet 领域关系，不创建额外 role 或 policy binding。领养时会独立于 active Pet row 持久化 Pet-to-Workspace binding。Pet 删除在同一个 gameplay SQL database transaction 中创建或复用一条 `kind=pet` PendingDeletion，同时保留 Pet row 及其 binding；该标记不影响 Pet 的读取、list、authorization 或 mutation。不创建 Workspace pending record；points、badge、result、transaction 和 reward grant 历史全部保留。
