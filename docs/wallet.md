# Wallet And Points

This document describes GizClaw wallet behavior for peer-facing business
features.

## Model

Each authenticated peer has exactly one wallet. The wallet is resolved from the
current peer context; clients do not pass a wallet id, public key, gear id, or
currency.

```json
{
  "id": "wallet-<peer-id>",
  "token_balance": 0,
  "point_balance": 0,
  "created_at": "2026-06-12T00:00:00Z",
  "updated_at": "2026-06-12T00:00:00Z"
}
```

Balances:

- `token_balance`: token balance reserved for token-style business use.
- `point_balance`: points used by pet and reward flows.

There is no currency field. Wallet access is not ACL-bound; it is scoped by the
authenticated peer.

## RPC

Peer clients can read their wallet and transaction audit trail through:

```text
wallet.get
wallet.transactions.list
wallet.transactions.get
```

`wallet.get` lazily creates a zero-balance wallet row if the peer has no wallet
yet.

`wallet.transactions.list` supports `cursor` and `limit`, defaults to newest
transactions first, and returns:

```json
{
  "items": [],
  "has_next": false,
  "next_cursor": null
}
```

`limit` uses zero-value semantics:

- `0` or omitted: default page size.
- Negative values: default page size.
- Values above the maximum are clamped by the service.

## Transactions

Every balance change is recorded as a wallet transaction:

```json
{
  "id": "20260612T120000.000000000Z-...",
  "token_delta": 0,
  "point_delta": -100,
  "reason": "pet_adopt",
  "created_at": "2026-06-12T12:00:00Z"
}
```

Rules:

- A transaction records both `token_delta` and `point_delta`.
- At least one delta must be non-zero.
- Final balances cannot be negative.
- Balance update and transaction insert happen in one SQL transaction.
- Transactions are scoped to the current peer wallet.

Current reasons:

| Reason | Delta direction | Trigger |
| --- | --- | --- |
| `pet_adopt` | points deducted | `pet.adopt` charges the adoption cost. |
| `pet_feed` | usually points deducted | `pet.feed` applies the server-generated pet action decision. |
| `pet_wash` | usually points deducted | `pet.wash` applies the server-generated pet action decision. |
| `pet_play` | usually points deducted | `pet.play` applies the server-generated pet action decision. |
| `reward_claim` | points awarded | `reward.claim` applies the server-generated reward decision. |

There are no admin adjustment or system adjustment reasons in this design.

## Call Flow And Point Usage

### `wallet.get`

```text
peer RPC wallet.get
  -> resolve current authenticated peer id
  -> open wallet SQL store
  -> ensure wallet schema exists
  -> SELECT wallet WHERE peer_id = current peer
  -> if missing, INSERT zero-balance wallet row
  -> return WalletObject
```

This method never accepts a wallet id from the client. The peer identity comes
from the active giznet connection.

### `wallet.transactions.list`

```text
peer RPC wallet.transactions.list(cursor, limit)
  -> resolve current authenticated peer id
  -> normalize limit: 0/negative means default, too large means max
  -> SELECT wallet_transactions WHERE peer_id = current peer
  -> apply cursor over ORDER BY created_at DESC, id DESC
  -> return one page plus has_next/next_cursor
```

The result is newest-first. A peer cannot list another peer's transactions
because every query is filtered by the current peer id.

### `wallet.transactions.get`

```text
peer RPC wallet.transactions.get(id)
  -> resolve current authenticated peer id
  -> validate id is non-empty
  -> SELECT wallet_transactions WHERE peer_id = current peer AND id = request id
  -> return transaction or not found
```

The transaction id alone is not enough to read a record. The current peer id is
always part of the lookup.

### Pet Adoption

`pet.adopt` deducts points from the current peer wallet before creating the pet.
If the wallet does not have enough points, adoption fails and no pet is written.

Call flow:

```text
peer RPC pet.adopt(name, optional id)
  -> resolve current authenticated peer id
  -> reject duplicate pet id for this peer
  -> select a usable PetSpecies with `use` on the `pet_species` ACL resource
  -> select an available Voice
  -> wallet.AddTransaction(point_delta = -adoption_cost, reason = pet_adopt)
       -> ensure wallet row
       -> reject negative final point_balance
       -> UPDATE wallet balance and INSERT wallet transaction in one SQL tx
  -> create PetObject with server-assigned species_id, voice_id, life, ability
  -> store pet under current peer
  -> return PetObject
```

The adoption transaction uses:

```text
reason = pet_adopt
point_delta = -adoption_cost
```

The first implementation uses the service default adoption cost unless the
server wiring overrides it.

### Pet Actions

`pet.feed`, `pet.wash`, and `pet.play` accept a prompt and invoke the configured
server-side pet action generator. The generator returns a strict
`PetActionDecision`:

The generator is configured by `system_tasks.pet_action.generator` in server
config. It must use a `model/<model-id>` pattern, for example
`model/qwen-flash`. The common setup uses the same model for pet actions and
reward claims.

```json
{
  "point_delta": -1,
  "life_delta": {
    "satiety": 10,
    "cleanliness": 0,
    "mood": 0,
    "energy": 0,
    "health": 0
  },
  "ability_delta": {
    "level": 0,
    "exp": 4,
    "charm": 0,
    "intelligence": 0,
    "stamina": 0,
    "luck": 0
  }
}
```

If `point_delta` is non-zero, the service writes a wallet transaction with the
action-specific reason. If the generated decision is invalid, missing, or would
make the wallet negative, the action fails and does not update the pet.

Call flow:

```text
peer RPC pet.feed / pet.wash / pet.play(pet_id, prompt)
  -> resolve current authenticated peer id
  -> load pet WHERE owner = current peer AND id = pet_id
  -> reject empty prompt
  -> invoke system_tasks.pet_action.generator with action, prompt, pet state
  -> require strict decide_pet_action tool output
  -> if point_delta != 0:
       wallet.AddTransaction(point_delta, reason = pet_feed/pet_wash/pet_play)
       -> reject negative final point_balance
  -> apply life_delta with service clamp rules
  -> apply ability_delta and server-owned level recalculation
  -> store updated pet under current peer
  -> return PetObject
```

If the generator returns an error, no tool call, wrong tool call, invalid JSON,
or a decision that cannot be applied, the wallet and pet are left unchanged.

Pet level changes are server-owned and derived from ability experience. There is
no `pet.level-up` RPC.

### Reward Claim

`reward.claim` accepts only a prompt from the peer. The server invokes the
configured reward claim generator and requires a strict `RewardDecision`:

The generator is configured by `system_tasks.reward_claim.generator` in server
config. It must use a `model/<model-id>` pattern, for example
`model/qwen-flash`. The common setup uses the same model for reward claims and
pet actions.

```json
{
  "badge_id": "founder",
  "point_amount": 9
}
```

If `point_amount` is positive, the wallet is credited with:

```text
reason = reward_claim
point_delta = point_amount
```

If `badge_id` is present, the badge must exist and pass the `use` check on the
`badge` ACL resource for the flow. Failed reward claims do not mutate wallet
state or reward history and do not advance cooldown.

Call flow:

```text
peer RPC reward.claim(prompt)
  -> resolve current authenticated peer id
  -> reject empty prompt
  -> check cooldown for this peer
  -> invoke system_tasks.reward_claim.generator with peer id and prompt
  -> require strict decide_reward tool output
  -> validate point_amount >= 0
  -> require badge_id or positive point_amount
  -> if badge_id is present:
       load Badge and check `use` on the `badge` ACL resource
  -> if point_amount > 0:
       wallet.AddTransaction(point_delta = point_amount, reason = reward_claim)
       -> UPDATE wallet balance and INSERT wallet transaction in one SQL tx
  -> store RewardObject under current peer
  -> return RewardObject
```

If cooldown is active or generated output is invalid, no wallet transaction and
no reward row are written.

## Storage

Wallet state is SQL-backed. Configure it with a logical SQL store:

```yaml
storage:
  wallet-db:
    kind: sql
    sqlite:
      dir: data/wallet.sqlite

stores:
  wallets:
    kind: sql
    storage: wallet-db

wallets:
  store: wallets
```

The wallet service creates these tables if they do not already exist:

```sql
CREATE TABLE IF NOT EXISTS wallets (
  peer_id TEXT PRIMARY KEY,
  id TEXT NOT NULL UNIQUE,
  token_balance INTEGER NOT NULL,
  point_balance INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_transactions (
  peer_id TEXT NOT NULL,
  id TEXT NOT NULL,
  token_delta INTEGER NOT NULL,
  point_delta INTEGER NOT NULL,
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (peer_id, id),
  FOREIGN KEY (peer_id) REFERENCES wallets(peer_id)
);

CREATE INDEX IF NOT EXISTS wallet_transactions_peer_created_desc
  ON wallet_transactions(peer_id, created_at DESC, id DESC);
```

`peer_id` is an internal storage key derived from authenticated peer identity.
It is not serialized in wallet API objects.
