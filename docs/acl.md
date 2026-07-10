# GizClaw ACL

This document describes the current ACL model used by peer RPC, admin HTTP,
and ResourceManager.

## Terms

| Term | Meaning |
| --- | --- |
| `ResourceManager` | The admin apply/import layer for declarative resources. It accepts `ResourceKind` values such as `Workspace`, `Workflow`, `Credential`, `Contact`, and `FriendGroup`. |
| `ResourceKind` | The declarative resource enum in `api/resource/resource.json`. It is broader than ACL and includes resources that are not directly protected by ACL policies. |
| `ACLResource` | The target object of an ACL policy binding. It is stored as `kind:id` and is independent from the ResourceManager envelope type. |
| `ACLResourceKind` | The ACL resource enum in `api/type/acl_resource.json`. Only these kinds can appear in `ACLPolicy.resource`. |
| Collection resource | The synthetic `ACLResource` `{kind, "__collection__"}`. It is used only to authorize creating new concrete resources of that kind. |
| Subject | The identity granted access by a policy binding: a peer public key, a view, or all peers. |
| Role | A named reusable list of generic ACL permissions. |
| Permission | One of the generic ACL permissions: `read`, `use`, `create`, or `admin`. Permissions are not resource-specific strings. |

ACL is intentionally narrower than ResourceManager. A ResourceManager resource
can create or update server state without becoming an ACL resource kind.

## ResourceManager Resources

These ResourceManager resources are directly backed by ACL resource kinds:

| ResourceManager kind | ACL resource kind | Notes |
| --- | --- | --- |
| `Workspace` | `workspace` | Peer runtime list/get/use/update/delete checks use workspace ACL. |
| `Workflow` | `workflow` | Peer runtime list/get/use/update/delete checks use workflow ACL. |
| `Model` | `model` | Peer runtime list/get/use/update/delete and AI runtime checks use model ACL. |
| `Credential` | `credential` | Peer runtime list/get/use/update/delete and AI runtime checks use credential ACL. |
| `Voice` | `voice` | Peer voice list/get and speech runtime checks use voice ACL. |
| `ACLView` | `view` | Views are used as grouped ACL subjects and can also be addressed as ACL resources. |
| `Firmware` | `firmware` | Peer firmware list/get/download checks use firmware ACL. |
| `GameRuleset` | `gameruleset` | Peer gameplay entry points check ruleset ACL before using a ruleset. |
| `Tool` | `tool` | Peer Tool CRUD, ToolKit construction, and invocation use Tool ACL. |

These ResourceManager resources are not direct ACL resource kinds:

| ResourceManager kind | Access model |
| --- | --- |
| `ACLPolicyBinding`, `ACLRole` | Define ACL state; they are not themselves ACL resources. |
| `Contact`, `Friend`, `FriendGroup`, `FriendGroupInviteToken`, `FriendGroupMember` | Scoped by authenticated peer and social-service rules. Social friend/friend-group creation may create a backing workspace and grant workspace ACL. |
| `PeerConfig` | Scoped by admin and peer config service rules. Firmware selected by peer config is still checked as a `firmware` ACL resource when peers read it. |
| `DashScopeTenant`, `GeminiTenant`, `MiniMaxTenant`, `OpenAITenant`, `VolcTenant` | Provider configuration resources. Runtime access is mediated through referenced `model`, `voice`, and `credential` ACL resources. |
| `PetDef`, `BadgeDef`, `GameDef` | Gameplay catalog resources selected through `GameRuleset`; they are not direct ACL resource kinds. |
| `Pet`, `Badge`, `PointsAccount`, `PointsTransaction`, `GameResult`, `RewardGrant` | Peer-owned gameplay runtime state. Access is scoped by the authenticated peer owner. |
| `ResourceList` | Apply/import wrapper only. |

## ACL Resource Kinds

| ACL resource kind | Resource id | Runtime permissions currently checked |
| --- | --- | --- |
| `workspace` | Workspace name or `__collection__` | `read`, `use`, `create`, `admin` |
| `workflow` | Workflow name or `__collection__` | `read`, `use`, `create`, `admin` |
| `model` | Model id or `__collection__` | `read`, `use`, `create`, `admin` |
| `credential` | Credential name or `__collection__` | `read`, `use`, `create`, `admin` |
| `voice` | Voice id | `read`, `use` |
| `view` | View name | ACL grouping and view administration |
| `firmware` | Firmware id | `read` |
| `gameruleset` | Ruleset name | `read`, `use`, `admin` |
| `tool` | Tool id or `__collection__` | `read`, `use`, `create`, `admin` |

The permission enum is shared across all ACL resource kinds. For example, a
policy binding grants resource `{kind:"workspace", id:"demo"}` permission
`use`; it does not grant a string named `workspace.use`.

## Subjects

| Subject kind | ID | Meaning |
| --- | --- | --- |
| `pk` | Peer public key | One peer identity. |
| `view` | View name | A grouped subject for curated access. |
| `all_peers` | Empty | Default subject that every connected peer can inherit. |

Authorization checks try the requested subject and then inherit matching
`all_peers` bindings. Peer runtime authorization also checks the peer's
concrete public-key subject first and then checks matching view subjects for
the same concrete resource.

## Permissions

| Permission | Meaning |
| --- | --- |
| `read` | List or get metadata/state for an existing concrete resource. |
| `use` | Use an existing concrete resource at runtime. |
| `create` | Create a new concrete resource. This is checked only against `{kind:"...", id:"__collection__"}`. |
| `admin` | Update, delete, or administratively manage an existing concrete resource. |

`create` and `admin` are separate. Creating a resource through peer runtime RPC
requires `create` on the collection resource, not `admin` on the future
concrete id.

## Collection Create Checks

Collection resources use the reserved id `__collection__`:

```text
workspace:__collection__ + create
workflow:__collection__ + create
model:__collection__ + create
credential:__collection__ + create
tool:__collection__ + create
```

Create checks do not fall back from a concrete resource to a collection
resource. The caller must be granted the exact collection `create` permission
for the resource kind being created.

## Runtime Checks

| Operation | Required ACL checks |
| --- | --- |
| `server.workspace.list/get` | Concrete `workspace` + `read` |
| `server.workspace.create` | `workspace:__collection__` + `create`; referenced concrete `workflow` + `use` |
| `server.workspace.put/delete` | Concrete `workspace` + `admin`; `put` also checks referenced concrete `workflow` + `use` |
| `server.workflow.list/get` | Concrete `workflow` + `read` |
| `server.workflow.create` | `workflow:__collection__` + `create` |
| `server.workflow.put/delete` | Concrete `workflow` + `admin` |
| `server.model.list/get` | Concrete `model` + `read` |
| `server.model.create` | `model:__collection__` + `create` |
| `server.model.put/delete` | Concrete `model` + `admin` |
| `server.credential.list/get` | Concrete `credential` + `read` |
| `server.credential.create` | `credential:__collection__` + `create` |
| `server.credential.put/delete` | Concrete `credential` + `admin` |
| `server.tool.list/get` | Concrete `tool` + `read`; list omits denied Tools |
| `server.tool.create` | `tool:__collection__` + `create`; the ID must use the authenticated peer's `peer.<public-key>.` namespace and the Tool must be `source: device` |
| `server.tool.put/delete` | Concrete `tool` + `admin`; peers may modify only their own device Tool namespace |
| ToolKit construction and Tool invocation | Concrete `tool` + `use`; runtime availability and executor registration are checked separately |
| `server.voice.list/get` | Concrete `voice` + `read` |
| `server.firmware.list/get/download` | Concrete `firmware` + `read` |
| `server.run.agent.set` | Target concrete `workspace` + `use` |
| `server.run.reload` | Current pending agent concrete `workspace` + `use`; concrete `workflow` + `use`; referenced concrete `model` + `use`; referenced concrete `credential` + `use` |
| `server.run.say` | Selected concrete `voice` + `use`; selected TTS concrete `model` + `use`; referenced concrete `credential` + `use` |
| Workspace history reads | Concrete `workspace` + `read` |
| OpenAI-compatible AI calls | Referenced concrete `model` + `use` |
| Peergenx calls | Referenced concrete resource + required generic permission |
| `server.game_ruleset.get` | Concrete `gameruleset` + `read` |
| `server.pet.adopt` | Concrete `gameruleset` + `use` |

Peer-owned gameplay runtime resources are isolated by caller public key instead
of generic ACL resources:

```text
caller public key == owner_public_key
```

This owner check applies to:

- `server.pet.{list,get,put,delete,drive}`
- `server.points.get`
- `server.points.transactions.{list,get}`
- `server.badge.{list,get}`
- `server.game_result.{list,get}`
- `server.reward_grant.{list,get}`

Social contact, friend, and friend-group RPCs are not authorized as
`contact`, `friend`, or `friend_group` ACL resources. When a social relation
creates a shared workspace, access to that workspace is represented by normal
`workspace` ACL bindings.

## Default Ownership Rules

| Create path | Subject to bind | Resource to bind | Permissions |
| --- | --- | --- | --- |
| Peer creates workspace | `pk:{peerPublicKey}` | `workspace:{name}` | `read`, `use`, `admin` |
| Peer creates workflow | `pk:{peerPublicKey}` | `workflow:{name}` | `read`, `use`, `admin` |
| Peer creates model | `pk:{peerPublicKey}` | `model:{id}` | `read`, `use`, `admin` |
| Peer creates credential | `pk:{peerPublicKey}` | `credential:{name}` | `read`, `use`, `admin` |
| Peer creates device Tool | `pk:{peerPublicKey}` | `tool:{id}` | `read`, `use`, `admin` |

The caller also needs the relevant collection `create` permission before the
resource is created.

## Shared Resource Rules

| Shared resource | Subject | Resource | Typical permissions |
| --- | --- | --- | --- |
| Built-in model for everyone | `all_peers` | `model:{id}` | `read`, `use` |
| Built-in model for a view | `view:{name}` | `model:{id}` | `read`, `use` |
| Shared credential for one peer | `pk:{peerPublicKey}` | `credential:{name}` | `read`, `use` |
| Shared credential for a view | `view:{name}` | `credential:{name}` | `read`, `use` |
| Shared voice for everyone | `all_peers` | `voice:{id}` | `read`, `use` |
| Shared firmware for one peer | `pk:{peerPublicKey}` | `firmware:{id}` | `read` |
| Shared gameplay ruleset for a view | `view:{name}` | `gameruleset:{name}` | `read`, `use` |
