# RuntimeProfile and device registration

`RuntimeProfile` defines the server resources and gameplay configuration available to a device connection. It complements resource ownership: a device can access resources allowed by its RuntimeProfile and resources it owns. Lists place RuntimeProfile resources before owned resources.

## Declarative structure

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata:
  name: h106-tragon
spec:
  resources:
    workflows:
      chat: general-chat
    models:
      primary: model-default
    voices:
      assistant: voice-default
    tools:
      weather: weather-v2
    pet_defs:
      tragon: petdef-tragon
    game_defs:
      dinodive: game-dinodive
    badge_defs:
      dinodive-master: badge-dinodive-master
  gameplay:
    points:
      initial_balance: 100
    pet_pool:
      - pet_def: tragon
        weight: 100
        rarity: common
        adoption_cost: 10
    drive:
      game_rewards:
        dinodive:
          points_delta: 20
          badge_exp_delta:
            dinodive-master: 100
```

Each map under `resources` binds a profile-local alias to a concrete resource name. The values form the allow list. Gameplay configuration uses aliases only inside the RuntimeProfile, so `pet_def: tragon` resolves to `petdef-tragon`. Workflow is the only public resource RPC that exposes this alias namespace: `server.workflow.list/get` with `source=runtime` use the alias as the RPC `id`. Other resource RPCs continue to use concrete names.

Multiple aliases may reference the same concrete resource. Concrete-name resource lists deduplicate those values, while the runtime Workflow list preserves every alias because each alias is a distinct client-facing ID. A missing or deleted concrete resource is skipped; it does not prevent the RuntimeProfile from being stored, loaded, or deleted. RuntimeProfile does not carry icons, display names, or i18n. Product clients map aliases such as `chat` to their own presentation.

## RegistrationToken

An administrator pre-creates a `RegistrationToken` that references one RuntimeProfile. The raw token is returned only by the create response; the server stores only its SHA-256 hash. A token can register multiple devices or connections until it is deleted. It has no enable/disable state and no database usage history or public-key binding. RegistrationToken names also accept the scoped `app:<bundle-id>` form; this does not change the generic custom-resource ID grammar.

The raw token is provisioned to a client through its secure enrollment channel. After connecting, the client calls `server.register`; the server validates the token and snapshots its RuntimeProfile onto that connection. The response contains only `runtime_profile_name`. Updating a RuntimeProfile does not mutate established connections. A reconnect and new registration loads the new configuration.

Public HTTP clients provide the same token in the optional `X-Registration-Token` header on `POST /login`. The resulting bearer session keeps that RuntimeProfile snapshot, so `/openai/v1` resolves the same profile-qualified Models and Voices without requiring a concurrent Peer RPC connection.

Successful and rejected registrations are written to the system log with the Peer public key, connection source, RegistrationToken name, and RuntimeProfile. No token usage records are stored in the business database.

## Access rules

| Source | list / get / use | put / delete |
| --- | --- | --- |
| RuntimeProfile allow list | Allowed without an owner check | Not allowed |
| Current Peer is owner | Allowed | Allowed |
| Friend, FriendGroup, or Pet system Workspace | Allowed by domain relationship | Handled by domain rules |
| Other resource | Hidden and unavailable | Not allowed |

An unregistered device may still call public RPC methods; it simply has no RuntimeProfile resources. Workspace, Workflow, Model, Credential, and Tool resources created through public CRUD record the current Peer as owner. Runtime Workflows are read-only; owned Workflows support public CRUD.

Model and Voice invocation resolves the configured ProviderTenant and its backing Credential internally. Access to a RuntimeProfile-qualified Model or Voice authorizes use of its server-side Credential, but does not expose that Credential through credential list/get or grant mutation. An owner-created Model outside the RuntimeProfile may use only a Credential owned by the same Peer; it cannot select an unrelated server-owned Credential through a ProviderTenant.

Firmware remains an independently managed Admin resource. It is not selected by RegistrationToken, does not appear in connection registration state, and is not projected through peer Firmware RPCs. Deleting a RuntimeProfile or RegistrationToken does not cascade to other resources. An established connection keeps its RuntimeProfile snapshot until disconnect.
