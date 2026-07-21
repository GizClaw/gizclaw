# services/gameplay

`pkgs/gizclaw/services/gameplay` owns the Gameplay catalog, player state, rewards, and digital assets. Gameplay configuration now belongs to a connection's RuntimeProfile; there is no separate GameRuleset resource.

## Ownership

Gameplay owns PetDef, BadgeDef, GameDef, Pet, points accounts, transactions, reward grants, badge progression, and game results. RuntimeProfile `resources.pet_defs`, `resources.voices`, `resources.game_defs`, and `resources.badge_defs` maps provide profile-local aliases. Each `gameplay.adoption.pool` entry references both a PetDef and Voice alias, while `gameplay.pet.games` uses GameDef aliases as direct keys.

Pet adoption resolves rules from the current connection's RuntimeProfile snapshot, stores the selected pool entry's Voice alias in the system Workspace, and records the RuntimeProfile name on the Pet and related state. PetDef contains no Voice ID or alias; it retains character/speaking style, PIXA, and behavior-to-animation bindings. The Pet system Workspace uses the built-in `pet-care` Workflow; `pet-care` does not need to appear in the RuntimeProfile `workflows` map.

A profile with no valid PetDef cannot adopt a Pet, and a GameDef not allowed by the current profile cannot submit a game result. Invalid aliases and reward references fail RuntimeProfile validation. Deleting a definition or RuntimeProfile does not cascade into existing Gameplay history.

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

where weights sum to 1 and $p>1$. Full care stats produce zero deficit and therefore no life loss; lower care stats accelerate life loss. The Server evaluates the piecewise analytic integral, so settlement depends on initial state and elapsed time rather than request frequency. When life reaches zero, the Pet atomically enters `dead` with an immutable `died_at`; a dead Pet cannot Drive again.

EXP required for the next level is `ceil(base_exp + log_scale * ln(current_level))`, with `log_scale` bounded to `0..100` so level calculation remains bounded. Cumulative EXP is not consumed by leveling. Initial points, adoption weights/costs, and every Pet policy value come only from RuntimeProfile; Server config has no fallback.

Every game must be configured explicitly in both `resources.game_defs` and `gameplay.pet.games`; there is no default. Submitting an unconfigured game is an exact no-op: no time settlement, points/energy deduction, game result, reward-model call, EXP, or badge. A configured game validates resources before invoking the current connection's authorized model. The model can grant only Pet EXP and eligible badge EXP within configured maxima. Model failure or invalid output produces no gameplay write. An idempotency key prevents a successful result from charging, evaluating, or rewarding twice.

Gameplay uses Workspace ownership and the Pet domain relationship. It does not create extra roles or policy bindings. Pet deletion removes its system Workspace before the Pet row and preserves points, badge, result, transaction, and reward-grant history.
