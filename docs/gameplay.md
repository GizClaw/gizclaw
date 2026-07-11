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
- pet adoption pool and rarity weights
- adoption cost per pet pool entry
- allowed badge definitions
- allowed game definitions
- default and game-result rewards
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
    game_rewards:
      game-fetch:
        points_delta: 20
        pet_exp_delta: 25
        badge_exp_delta:
          badge-first-bath: 100
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

Current reward grants are produced by `pet.drive`, either from PetDef action
effects, GameRuleset game-result rewards, or both. Pet attribute deltas from
PetDef actions are applied to the pet state directly; the reward grant ledger
keeps points, pet experience, and badge experience deltas.

Reward grants are separate from point transactions. A reward grant explains the
gameplay reward decision; a point transaction records the balance mutation when
that reward includes a point delta.

### PetDef

`PetDef` defines an adoptable pet type. It is intentionally self-describing so
AI tooling can generate and review a character without scattering the pet
contract across ruleset metadata.

`PetDef.spec` owns:

- `default_locale`
- `attr.life` and `attr.progression`
- `character.prompt`
- `voice.voice_id` and `voice.prompt`
- `drive.actions[]`, including each action's cost, visual clip, and effect
- `visual.refs` for optional AI generation references
- `visual.pixa.asset_ref` and `visual.pixa.metadata`
- `i18n` for user-facing display text

The attribute groups are fixed to `life` and `progression`. Attribute ids
inside each group are PetDef-owned stable machine ids. User-facing labels live
under `i18n`, not beside the machine data.

Example:

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: PetDef
metadata:
  name: petdef-tragon
spec:
  default_locale: en
  workflow_name: pet-care-flowcraft
  attr:
    life:
      hp:
        initial: 100
      wellness:
        initial: 100
      energy:
        initial: 100
      cleanliness:
        initial: 100
    progression:
      xp:
        initial: 0
  character:
    prompt: Compact pixel dragon with a rounded head, small horns, tiny wings, and playful but brave behavior.
  voice:
    voice_id: gizclaw-cute-dragon
    prompt: Bright, curious, and short spoken replies with childlike confidence.
  drive:
    actions:
    - id: idle
      cost: 0
      visual_clip_id: idle
    - id: feed
      cost: 6
      visual_clip_id: feed
      effect:
        attr_delta:
          life:
            wellness: 8
            energy: 4
        pet_exp_delta: 8
    - id: bath
      cost: 5
      visual_clip_id: bath
      effect:
        attr_delta:
          life:
            cleanliness: 15
        pet_exp_delta: 6
    - id: run_left
      cost: 0
      visual_clip_id: run_left
      effect:
        attr_delta:
          life:
            energy: -2
        pet_exp_delta: 2
  visual:
    refs:
      images: []
      videos: []
    pixa:
      asset_ref: asset://h106/tiga/pets/tragon/tragon.pixa
      metadata:
        version: "1"
        canvas:
          width: 60
          height: 60
        clips:
        - id: idle
          action_id: idle
          pixa_clip_name: default
        - id: feed
          action_id: feed
          pixa_clip_name: feed
        - id: bath
          action_id: bath
          pixa_clip_name: bath
        - id: run_left
          action_id: run_left
          pixa_clip_name: run_left
  i18n:
    en:
      display_name: Tragon
      description: A brave little dragon who gets excited when it runs and perks up when you feed, bathe, or heal it.
      attr:
        life:
          hp:
            display_name: HP
          wellness:
            display_name: Wellness
          energy:
            display_name: Energy
          cleanliness:
            display_name: Cleanliness
        progression:
          xp:
            display_name: EXP
      drive:
        actions:
          idle:
            display_name: Idle
          feed:
            display_name: Feed
          bath:
            display_name: Bath
          run_left:
            display_name: Run Left
```

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

Actions are defined by the selected pet's `PetDef.spec.drive.actions[]`.
Unknown actions are rejected.

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
GameRuleset.default_reward + PetDef.action.effect.pet_exp_delta + GameRuleset.game_rewards[game_def_id]
```

Drive applies state in one gameplay transaction:

- deduct the PetDef action cost with a `PointsTransaction`
- record `GameResult` if supplied
- record `RewardGrant` if reward deltas are non-empty
- apply PetDef action `attr_delta` to pet life state
- apply pet experience into pet progression
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
  "attr_delta": {
    "life": {
      "cleanliness": 10
    }
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
server.pet.{list,get,adopt,put,delete,drive,presentation.get,pixa.download}
server.points.get
server.points.transactions.{list,get}
server.badge.{list,get}
server.game_result.{list,get}
server.reward_grant.{list,get}
server.pet_def.pixa.download
server.badge_def.pixa.download
```

The peer RPC PIXA download methods should use the same binary-frame download
pattern as firmware downloads, with simple metadata responses and binary payload
frames. `server.pet.pixa.download` is for an owned pet and resolves its PetDef
visual through pet ownership; `server.pet_def.pixa.download` is for direct
PetDef asset access.

## Current Model Summary

```text
GameRuleset + PetDef + BadgeDef + GameDef
Pet + Badge + PointsAccount + PointsTransaction + GameResult + RewardGrant
```
