# Gameplay System

Gameplay is the points, rewards, and pet system in GizClaw. Its core state is
gameplay points, reward grants, and gameplay projections such as pets, badges,
and game results.

Gameplay is separate from token accounting. Gameplay points are game state,
while future token/accounting systems should own real balances and paid
consumption.

## Resource Model

```text
Admin catalog
├── GameRuleset
├── PetDef
│   └── pixa
├── BadgeDef
│   └── pixa
└── GameDef

Peer runtime state
├── PointsAccount
├── PointsTransaction
├── RewardGrant
├── Pet
├── Badge
└── GameResult
```

Admin catalog resources are shared definitions. Peer runtime resources are
owned by one peer public key and are isolated by owner.

The gameplay runtime has three main responsibilities:

- maintain gameplay point accounts and point transaction history
- issue reward grants from configured gameplay policies
- record gameplay projections and events such as pets, badges, and game result
  history

## Admin Catalog

### GameRuleset

`GameRuleset` is the ACL boundary for gameplay. Admins decide which peers or
views can read or use a ruleset.

It defines:

- initial gameplay point balance
- reward policies for actions and game results
- pet adoption pool and rarity weights
- adoption cost per pet pool entry
- allowed badge definitions
- allowed game definitions
- drive action costs
- life stat decay
- default pet workspace workflow

Example:

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: GameRuleset
metadata:
  name: default-gameplay
spec:
  enabled: true
  default_workflow_name: pet-flowcraft-agent
  points:
    initial_balance: 100
  pet_pool:
  - petdef_id: petdef-cat
    weight: 80
    rarity: common
    adoption_cost: 10
    workflow_name: pet-flowcraft-agent
  - petdef_id: petdef-dragon
    weight: 1
    rarity: legendary
    adoption_cost: 100
    workflow_name: pet-flowcraft-agent
  badge_def_ids:
  - badge-first-bath
  game_def_ids:
  - game-fetch
  drive:
    action_costs:
      bath: 5
      feed: 8
      drink: 2
    action_rewards:
      bath:
        pet_exp_delta: 80
        life_delta:
          clean: 10
      feed:
        pet_exp_delta: 20
        life_delta:
          hunger: 30
      drink:
        life_delta:
          thirst: 30
    game_rewards:
      game-fetch:
        points_delta: 20
        pet_exp_delta: 25
        badge_exp_delta:
          badge-first-bath: 100
    life_decay_per_hour:
      hunger: 1
      thirst: 2
      clean: 1
```

## Points

Gameplay points are the in-game balance used by gameplay features. They are not
token balances and should not be used for paid consumption.

`PointsAccount` stores the current peer-owned gameplay balance for one
`GameRuleset`. `PointsTransaction` records every balance-changing event,
including pet adoption costs, drive action costs, and reward point deltas.

Point transactions should carry a concrete source, such as a pet action, game
result, or reward grant, so later gameplay views and admin debug tools can
explain why the balance changed.

## Rewards

`RewardGrant` is the gameplay reward ledger. It records the reward policy output
that should be applied to peer-owned gameplay state.

A reward grant can include:

- gameplay point delta
- pet experience delta
- badge experience deltas
- pet life stat deltas
- pet ability stat deltas

Current reward grants are produced by `pet.drive`, either from action rewards,
game-result rewards, or both. Future gameplay entry points can also produce
reward grants as long as they use the same policy, idempotency, source, and
ledger rules.

Reward grants are separate from point transactions. A reward grant explains the
gameplay reward decision; a point transaction records the balance mutation when
that reward includes a point delta.

### PetDef

`PetDef` defines an adoptable pet type. It should include the pet's display
identity, initial stats, and the workflow used by adopted pets.

`PetDef` has one binary visual resource:

```text
PetDef pixa
```

The pixa resource is the pet visual/animation package. It should be addressed
as `pixa`, not as a generic asset.

Target API/RPC naming:

```text
Admin API
├── PUT /pet-defs/{id}/pixa
└── GET /pet-defs/{id}/pixa

Peer RPC
└── server.pet_def.pixa.download
```

Suggested content type:

```text
application/vnd.gizclaw.pixa
```

### BadgeDef

`BadgeDef` defines a badge that can be activated and leveled by badge
experience.

`BadgeDef` has one visual resource:

```text
BadgeDef pixa
```

The badge visual format is the same pixa container used by PetDef. Badge pixa
files use a single-frame `icon` clip.

Target API/RPC naming:

```text
Admin API
├── PUT /badge-defs/{id}/pixa
└── GET /badge-defs/{id}/pixa

Peer RPC
└── server.badge_def.pixa.download
```

Content type:

```text
application/vnd.gizclaw.pixa
```

### GameDef

`GameDef` defines a playable game result category. It is referenced by
`pet.drive` through `game_def_id`.

`GameDef` does not own an image in the current gameplay model. If the product
needs game lobby artwork later, add an explicit `cover` or `icon` resource
instead of overloading pet pixa or badge icons.

## Pet System

Pets are one gameplay projection built on top of points and rewards. They are
not the whole gameplay domain.

### Adoption

Adoption is a blind-box draw from the accessible `GameRuleset.pet_pool`.

The runtime normalizes positive `weight` values in the pool and selects one
`PetDef`. The selected pool entry can override adoption cost and workflow.

On success, adoption:

- creates a `Pet`
- creates a gameplay `PointsAccount` if needed
- deducts `adoption_cost`
- writes a `PointsTransaction`
- creates one workspace for the pet
- stores `workspace_name` and `workflow_name` on the pet

Pet workspace workflow selection order:

```text
pet_pool[].workflow_name
PetDef.spec.workflow_name
GameRuleset.spec.default_workflow_name
runtime default
```

Pet workspaces should use a carefully designed Flowcraft workflow when the pet
is conversational. The workspace is where the pet agent talks to the user and
recalls pet-related context.

### Drive

`pet.drive` updates a pet. It can be:

- time-only drive
- action drive, such as bath, feed, or drink
- game-result drive
- action plus game-result drive

Action costs are configured in:

```yaml
drive:
  action_costs:
    bath: 5
    feed: 8
```

Action rewards are configured in:

```yaml
drive:
  action_rewards:
    bath:
      pet_exp_delta: 80
      life_delta:
        clean: 10
```

Game rewards are configured in:

```yaml
drive:
  game_rewards:
    game-fetch:
      points_delta: 20
      pet_exp_delta: 25
      badge_exp_delta:
        badge-first-bath: 100
```

Drive reward calculation is:

```text
default_reward + action_rewards[action] + game_rewards[game_def_id]
```

Drive applies state in one gameplay transaction:

- apply time decay from `life_decay_per_hour`
- deduct action cost with a `PointsTransaction`
- record `GameResult` if supplied
- record `RewardGrant` if reward deltas are non-empty
- apply pet life, ability, exp, and level deltas
- apply badge exp and badge level updates
- record reward points with a `PointsTransaction`
- update pet `last_active_at`

`GameResult.idempotency_key` prevents duplicate game result reward application.

## Game Results

`GameDef` defines a game result category. `GameResult` records a peer-owned
play session or score event and can trigger configured gameplay rewards.

Game results are not limited to a particular frontend game implementation. They
are the gameplay record used to connect a game outcome to reward policy,
idempotency, point transactions, pet progression, and badge progression.

## Agent Memory

SQL gameplay records are the source of truth for gameplay state and accounting.
They are not enough for pet conversations.

After a successful `pet.drive`, the runtime should also write a pet-readable
event into the pet workspace so a Flowcraft pet agent can remember care
activities.

Example event:

```json
{
  "type": "pet_drive",
  "pet_id": "pet-...",
  "action": "bath",
  "game_def_id": "game-fetch",
  "score": 42,
  "points_delta": 15,
  "pet_exp_delta": 105,
  "life_delta": {
    "clean": 10
  },
  "occurred_at": "2026-07-05T00:00:00Z"
}
```

This requires an explicit workspace memory/event writer. Gameplay should not
reach into Flowcraft internals directly.

## Storage

```text
KV stores
├── game-rulesets
├── pet-defs
├── badge-defs
└── game-defs

Object store
└── gameplay-assets
    ├── pet-defs/{id}/pixa
    └── badge-defs/{id}/pixa

SQL store
└── gameplay-db
    ├── pets
    ├── badges
    ├── points accounts
    ├── points transactions
    ├── game results
    └── reward grants

Agent workspace store
└── agenthost
    └── pet workspace runtime files, history, memory, and cache
```

## API Surface

Admin API:

```text
/game-rulesets/{name} LIST, CREATE, GET, PUT, DELETE
/pet-defs/{id} LIST, CREATE, GET, PUT, DELETE
/pet-defs/{id}/pixa GET, PUT
/badge-defs/{id} LIST, CREATE, GET, PUT, DELETE
/badge-defs/{id}/pixa GET, PUT
/game-defs/{id} LIST, CREATE, GET, PUT, DELETE
```

Peer RPC:

```text
server.game_ruleset.get
server.pet.{list,get,adopt,put,delete,drive}
server.points.get
server.points.transactions.{list,get}
server.badge.{list,get}
server.game_result.{list,get}
server.reward_grant.{list,get}
server.pet_def.pixa.download
server.badge_def.pixa.download
```

The peer RPC download methods should use the same binary-frame download pattern
as firmware downloads, with simple metadata responses and binary payload frames.

## Current Model Summary

```text
GameRuleset + PetDef + BadgeDef + GameDef
Pet + Badge + PointsAccount + PointsTransaction + GameResult + RewardGrant
```
