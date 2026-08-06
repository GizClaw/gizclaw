# services/gameplay

`pkgs/gizclaw/services/gameplay` owns the Gameplay catalog, player state, rewards, and digital assets. Gameplay configuration now belongs to a connection's RuntimeProfile; there is no separate GameRuleset resource.

## Ownership

Gameplay owns PetDef, BadgeDef, GameDef, Pet, points accounts, transactions, reward grants, badge progression, and game results. RuntimeProfile `resources.pet_defs`, `resources.game_defs`, and `resources.badge_defs` maps provide profile-local aliases. Each `gameplay.adoption.pool` entry references only a PetDef alias, while `gameplay.pet.games` uses GameDef aliases as direct keys.

Pet adoption first validates the current connection's RuntimeProfile and the canonical Workflow ID in `workflows.system.pet`, then creates an owner-bound system Workspace and records the exact RuntimeProfile ID on the Pet and related state. A Pet Workspace stores no persona, conversation, model, voice, or other execution parameters and cannot be rewritten through generic Workspace put. PetDef contains no Voice ID/alias or local i18n; it retains character/speaking style, PIXA, and behavior-to-animation bindings. Presentation text comes from the RuntimeProfile `pet_defs` binding.

`driver: pet` is a domain wrapper with no built-in execution graph. Its `pet` field contains a same-shaped Workflow spec that may select `flowcraft`, `chatroom`, `doubao-realtime`, or `ast-translate`, but cannot recursively select `pet`. The wrapper injects current Pet context only; the nested driver owns graph, voice, model, and toolkit configuration. Memory is selected once by the outer Workflow and passed to the nested driver; the reusable inner spec cannot declare another Memory alias. Every alias resolves through the system Workspace owner's RuntimeProfile, so replacing the nested driver does not require changing Pet or Workspace data.

A profile with no valid PetDef cannot adopt a Pet, and a GameDef not allowed by the current profile cannot submit a game result. Invalid aliases and reward references fail RuntimeProfile validation. Deleting a definition or RuntimeProfile does not cascade into existing Gameplay history.

## Pet identity and adoption retries

`runtime.adopt` requires a non-empty Peer-scoped `name` and `display_name`. The name is the durable Peer-facing Pet resource selector, not a canonical Admin ID or a separate operation-level idempotency key. A device that needs retry-safe adoption generates and persists a valid GizClaw custom name before the first request, then reuses it after a timeout, disconnect, or other uncertain response. The Server always generates the Pet's canonical internal ID.

Pet names are scoped by the authenticated Peer. The first successful adoption of `(peer, name)` creates one Pet, one system Workspace, one adoption transaction, and one points charge. An unaffordable attempt fails before reserving the name or creating a Pet, Workspace, or transaction. Repeating a successful adoption under the same active RuntimeProfile returns the existing Pet, the current Points account, and the original adoption transaction without selecting another PetDef or writing again. A different `display_name` on the retry does not rename the Pet; callers use `server.pet.put` for that operation.

Different Peers may use the same textual Pet name. Their globally named internal Workspaces remain distinct, and every Pet RPC resolves both the authenticated Peer and Pet name. One Peer cannot address another Peer's Pet. The same Peer cannot reuse a name across RuntimeProfiles or after deleting the Pet because retained adoption history continues to reserve it.

Gameplay records follow the same Peer vocabulary. `GameResult`, `PointsTransaction`, and `RewardGrant` expose their canonical internal record IDs verbatim as `name`, and get operations accept that value through `name`. Cross-record selectors use `game_result_name`, `reward_grant_name`, `source_name`, and `pet_name`; `GAMEPLAY_REWARD_UPDATED.reward_grant_name` is the same value accepted by `server.reward_grant.get`. Admin and persistence surfaces retain canonical `id` fields.

## Fixed Pet contract

Every Pet has the same `life`, `health`, `satiety`, `hygiene`, `mood`, and `energy` stats in the fixed 0..100 range. Adoption initializes every stat to 100 and progression to `experience = 0`, `level = 1`. The behavior contract is fixed to `feed`, `bathe`, `play`, and `heal`, which raise satiety, hygiene, mood, and health respectively. PetDef does not define stat or behavior semantics. Its `visual.bindings.behaviors` and `visual.bindings.states` bind the fixed contract to that PetDef's PIXA clips. `idle`, `sick`, `dead`, and optional `sleep` are state visuals, not Drive behaviors.

RuntimeProfile `gameplay.pet` defines time policy, the level curve, each fixed behavior's energy cost/stat delta, and each allowed GameDef's points/energy cost and model reward policy. Behaviors apply deltas capped at 100. A successful behavior grants `energy_cost / energy_per_pet_exp` EXP. Energy recovers passively with elapsed time and does not require sleep.

Care stats decay linearly by their configured hourly rates. Define normalized deficit as

$$
D(t)=\sum_i w_i\left(1-\frac{s_i(t)}{100}\right),\qquad s_i(t)=\max(0,s_i(0)-r_i t)
$$

The life loss over an elapsed interval is

$$
\Delta life=L_{max}\int_0^T D(t)^p\,dt
$$

where weights sum to 1 and $p>1$. Full care stats produce zero deficit and therefore no life loss; lower care stats accelerate life loss. The Server evaluates the piecewise analytic integral, so settlement depends on initial state and elapsed time rather than request frequency.

`server.pet.drive` accepts an empty Drive containing only `pet_name` as a Server-authoritative time tick. It settles the elapsed interval from `state_settled_at`, persists care decay, energy recovery, life loss, and the new checkpoint, and returns the updated Pet without creating a behavior, game result, cost, or reward. Successive new ticks compose to the same state as one tick over the same total interval. When the optional request-level idempotency key is present, retrying that same empty Drive does not settle time again; a new key or no key starts a new tick.

When life reaches zero, the Pet atomically enters `dead` at the formula-derived death checkpoint with an immutable `died_at`, so terminal state is also independent of tick frequency. Behavior and game-result Drives cannot target a dead Pet; an empty Drive returns its unchanged terminal snapshot.

EXP required for the next level is `ceil(base_exp + log_scale * ln(current_level))`, with `log_scale` bounded to `0..100` so level calculation remains bounded. Cumulative EXP is not consumed by leveling. Initial points, adoption weights/costs, and every Pet policy value come only from RuntimeProfile; Server config has no fallback.

Every game must be configured explicitly in both `resources.game_defs` and `gameplay.pet.games`; there is no default. Submitting an unconfigured game is an exact no-op: no time settlement, points/energy deduction, game result, reward-model call, EXP, or badge. A configured game validates resources before invoking the current connection's authorized model. The model can grant only Pet EXP and eligible badge EXP within configured maxima. Model failure or invalid output produces no gameplay write. An idempotency key prevents a successful result from charging, evaluating, or rewarding twice.

## Drive Facts and Workspace Memory

Each successful state-changing care behavior or configured game result produces one fixed-template `kind=event` Fact. Care uses the committed `RewardGrant.ID`; a game uses the committed `GameResult.ID`. Their `Observation.ID` values are `gameplay/drive/reward_grant/<id>` and `gameplay/drive/game_result/<id>`. The Observation contains exactly one direct `FactCandidate`, fixes `Scope.AppID` to `Pet.WorkspaceName`, and leaves the other scope dimensions empty. Text and attributes come only from validated, committed Pet, result, and reward fields. They exclude the owner key, provider configuration, credentials, idempotency keys, and raw game payload, and are never sent through model extraction.

Gameplay inserts `gameplay_drive_fact_outbox` in the same SQL transaction that commits the Pet, result, and reward. Empty ticks, rejected Drives, insufficient resources, dead Pets, unconfigured games, and idempotent replays of a completed Drive do not create outbox rows. The Server starts one dispatcher loop for the complete Gameplay service; it does not create a resident service per Pet. Compare-and-set claims advance `pending`, `submitted`, `delivered`, and `blocked` states. When a provider returns an asynchronous operation, the opaque locator is persisted before the dispatcher resumes it through `OperationWaiter`. Transient failures use exponential backoff; configuration and contract failures use a slower blocked retry.

Delivery resolves the Workspace Memory binding selected by the Pet's outer Workflow and leases a logical `Scope.AppID = Pet.WorkspaceName` Store through the existing `memorystore.Registry`. An operation records a credential-free digest of its physical binding. If the RuntimeProfile selects a different physical binding before completion, the old locator is never passed to the new backend. Pet death and ordinary Pet deletion do not delete the Workspace, outbox, or delivered Fact; those records continue to follow the Workspace Memory lifecycle.

Gameplay uses Workspace ownership and the Pet domain relationship. It does not create extra roles or policy bindings. Adoption persists a Pet-to-Workspace binding independently of the active Pet row. Pet deletion atomically creates or reuses one `kind=pet` PendingDeletion in the same gameplay SQL database while retaining the Pet row and its binding; the marker does not change Pet reads, lists, authorization, or mutations. No Workspace pending record is created. Points, badges, results, transactions, and reward-grant history are preserved.

## Workspace conversation rewards

`gameplay.workspace_reward` is an optional RuntimeProfile policy. It coalesces
successive AgentHost-authored History entries in one Workspace into a debounced
window. The first `gear` entry freezes the beneficiary; later group-chat
participants do not replace that Peer. Server startup checkpoints existing
History without retroactive rewards, and imported, replayed, or legacy entries
are ineligible. A window must contain both beneficiary input and Agent output.

One persistent dispatcher serves the whole Gameplay service. The callback after
a successful AgentHost append only records the exact History high-water and
wakes the dispatcher; it never calls a model. Each window freezes the current
RuntimeProfile revision, LLM Model resource, Points prompt, BadgeDef
`reward_prompt` values, tiers, limits, and rolling budget. Profile updates affect
only later windows. The dispatcher reads bounded `origin=agenthost` text and
performs one snapshot-specific `genx.FuncTool` structured invoke, then locally
validates score, reason, Badge aliases, and EXP bounds. This evaluator is not an
Admin `Tool`, built-in Tool, Toolkit, or `giztools` capability.

Points tier mapping, BadgeDef ID resolution, rolling budgets, `RewardGrant`,
Points transaction, Badge EXP, window completion, and checkpoint advancement
commit in one Gameplay SQL transaction. Model failures retry within a bound;
invalid output becomes terminally blocked; claims recover after restart and a
window cannot grant twice. A successful state change sends
`GAMEPLAY_REWARD_UPDATED` only to the beneficiary as an invalidation hint, after
which clients fetch authoritative Gameplay state. Deterministic task rewards
belong to a separate task system and do not use this evaluator.
