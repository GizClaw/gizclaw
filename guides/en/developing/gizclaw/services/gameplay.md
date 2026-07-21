# services/gameplay

`pkgs/gizclaw/services/gameplay` owns the Gameplay catalog, player state, rewards, and digital assets. Gameplay configuration now belongs to a connection's RuntimeProfile; there is no separate GameRuleset resource.

## Ownership

Gameplay owns PetDef, BadgeDef, GameDef, Pet, points accounts, transactions, reward grants, badge progression, and game results. RuntimeProfile `resources.pet_defs`, `resources.voices`, `resources.game_defs`, and `resources.badge_defs` maps provide profile-local aliases. Each `gameplay.adoption.pool` entry references both a PetDef and Voice alias, while `gameplay.rewards` references the remaining resource aliases.

Pet adoption resolves rules from the current connection's RuntimeProfile snapshot, stores the selected pool entry's Voice alias in the system Workspace, and records the RuntimeProfile name on the Pet and related state. PetDef contains no Voice ID or alias; it retains character/speaking style, PIXA, and behavior-to-animation bindings. The Pet system Workspace uses the built-in `pet-care` Workflow; `pet-care` does not need to appear in the RuntimeProfile `workflows` map.

A profile with no valid PetDef cannot adopt a Pet, and a GameDef not allowed by the current profile cannot submit a game result. Invalid aliases and reward references fail RuntimeProfile validation. Deleting a definition or RuntimeProfile does not cascade into existing Gameplay history.

Initial points, adoption weights/costs, default rewards, per-game rewards, and per-Pet-action rewards come only from RuntimeProfile. Server config contains no gameplay-policy fallback, and PetDef retains intrinsic action behavior without owning points or EXP deltas.

Gameplay uses Workspace ownership and the Pet domain relationship. It does not create extra roles or policy bindings. Pet deletion removes its system Workspace before the Pet row and preserves points, badge, result, transaction, and reward-grant history.
