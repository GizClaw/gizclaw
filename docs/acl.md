# GizClaw ACL Matrix

This file lists the target ACL-managed subjects, resources, permissions, and
business actions for Server Service and Admin Service.

The current ACL schema supports admin/runtime shared resources. Peer-owned
wallet, pet, and reward state is scoped by the authenticated peer context and is
not modeled as ACL resources.

## Target ACL-Controlled Resource Kinds

```text
workspace
workflow
model
credential
voice
view
pet_species
badge
contact
friend
friend_request
friend_group
```

## Subjects

| Subject kind | ID | Meaning |
| --- | --- | --- |
| `pk` | peer public key | One peer identity. |
| `view` | view name | A grouped subject for curated access. |
| `all_peers` | empty | Default subject that every connected peer can inherit. |

## Resource Matrix

| Resource kind | ID | Owner | Permissions | Server Service usage | Admin Service usage |
| --- | --- | --- | --- | --- | --- |
| `workspace` | workspace name | peer or admin | `workspace.{read,use,admin}` | `server.workspace.{list,get,create,put,delete}`, `server.run.reload` | `/workspaces/{name}` |
| `workflow` | workflow name | peer or admin | `workflow.{read,use,admin}` | `server.workflow.{list,get,create,put,delete}`, `server.run.reload` | `/workflows/{name}` |
| `model` | model id | peer or admin | `model.{read,use,admin}` | `server.model.{list,get,create,put,delete}`, `server.run.reload`, `server.run.say` | `/models/{id}` |
| `credential` | credential name | peer or admin | `credential.{read,use,admin}` | `server.credential.{list,get,create,put,delete}`, `server.run.reload`, `server.run.say` | `/credentials/{name}` |
| `voice` | voice id | admin | `voice.{read,use,admin}` | `server.run.say`, voice selection, and runtime use | `/voices/{id}` |
| `view` | view name | admin | `view.{read,use,admin}` | read/use resources exposed by a view | `/acl/views/{name}` |
| `pet_species` | species id | admin | `pet_species.{read,use,admin}` | pet adoption species selection | `/pet-species/{id}`, `/pet-species/{id}/pixa` |
| `badge` | badge id | admin | `badge.{read,use,admin}` | reward badge grant validation | `/badges/{id}`, `/badges/{id}/icon` |
| `contact` | contact id | peer | `contact.{read,use,admin}` | `server.contact.{list,get,create,put,delete}` | `/peers/{publicKey}/contacts/{id}` |
| `friend` | friend relation id | peer pair | `friend.{read,use,admin}` | `server.friend.{list,delete}` | `/peers/{publicKey}/friends/{id}` |
| `friend_request` | request id | peer pair | `friend_request.{read,use,admin}` | `server.friend.requests.{list,create}`, `server.friend.requests.accept`, `server.friend.requests.reject` | `/peers/{publicKey}/friend-requests/{id}` |
| `friend_group` | friend group id | peer or admin | `friend_group.{read,use,admin}` | `server.friend_group.{list,get,create,put,delete}`, `server.friend_group.members.{list,add,put,delete}` | `/friend-groups/{id}` |

## Permission Mapping

| Permission suffix | Meaning |
| --- | --- |
| `read` | List or get metadata/state for a resource. |
| `use` | Use the resource at runtime or perform normal owner actions. |
| `admin` | Create, update, delete, or manage ACL for the resource. |

## Runtime ACL Checks

| Operation | Required ACL checks |
| --- | --- |
| `server.run.agent.set` | target `workspace.use` |
| `server.run.reload` | current pending agent workspace `workspace.use`, `workflow.use`, referenced `model.use`, referenced `credential.use` |
| `server.run.say` | selected `voice.use`, selected TTS `model.use`, referenced `credential.use` |
| `server.friend_group.messages.list` | `friend_group.read` |
| `server.friend_group.messages.get` | `friend_group.read` |
| `server.friend_group.messages.send` | `friend_group.use` |
| `server.pet.adopt` | selected `pet_species.use` |
| reward badge grant | generated `badge.use` |

## Default Ownership Rules

| Create path | Subject to bind | Resource to bind | Role |
| --- | --- | --- | --- |
| Peer creates workspace | `pk:{peerPublicKey}` | `workspace:{name}` | workspace owner/admin |
| Peer creates workflow | `pk:{peerPublicKey}` | `workflow:{name}` | workflow owner/admin |
| Peer creates model | `pk:{peerPublicKey}` | `model:{id}` | model owner/admin |
| Peer creates credential | `pk:{peerPublicKey}` | `credential:{name}` | credential owner/admin |
| Peer creates contact | `pk:{peerPublicKey}` | `contact:{id}` | contact owner/admin |
| Peer creates friend group | `pk:{peerPublicKey}` | `friend_group:{id}` | friend group owner/admin |

## Shared Resource Rules

| Shared resource | Subject | Resource | Role |
| --- | --- | --- | --- |
| Built-in model for everyone | `all_peers` | `model:{id}` | model reader/user |
| Built-in model for a friend group | `view:{name}` | `model:{id}` | model reader/user |
| Shared credential for one peer | `pk:{peerPublicKey}` | `credential:{name}` | credential user |
| Shared credential for a friend group | `view:{name}` | `credential:{name}` | credential user |
| Shared voice for everyone | `all_peers` | `voice:{id}` | voice reader/user |
| Shared pet species for everyone | `all_peers` | `pet_species:{id}` | pet species user |
| Shared badge grant for everyone | `all_peers` | `badge:{id}` | badge user |
