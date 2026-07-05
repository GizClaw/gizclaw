# GizClaw Service Tree

This document describes business-level services, not transport service IDs.

All Service is provided by every RPC peer. Client Service is provided by the
client peer. Server Service and Admin Service are provided by the GizClaw
server.

## Implementation Packages

Peer-facing social resources are implemented as focused packages:

```text
pkg/gizclaw/services/social/contact
pkg/gizclaw/services/social/friend
pkg/gizclaw/services/social/friendgroup
```

## Doc Style

- Business RPC-style methods use dotted names: `service.resource.method`.
- Multiple methods on one business resource use braces: `resource.{list,get,create,put,delete}`.
- Resource endpoints use path-first notation: `/path OPERATION[, OPERATION]`.
- Custom HTTP verbs are listed under their resource path as subtree items: `@verb`.

```text
All Service
└── all.ping

Client Service
├── client.info.get
└── client.identifiers.get

Server Service
├── server.info.{get,put}
├── server.runtime.get
├── server.status.{get,put}
├── server.run.say
├── /server-info GET
├── /login POST
├── server.workspace.{list,get,create,put,delete}
├── server.workflow.{list,get,create,put,delete}
├── server.model.{list,get,create,put,delete}
├── server.credential.{list,get,create,put,delete}
├── server.run.agent.{get,set}
├── server.run.{reload,status,stop}
├── server.contact.{list,get,create,put,delete}
├── server.friend.requests.{list,create}
├── server.friend.requests.accept
├── server.friend.requests.reject
├── server.friend.{list,delete}
├── server.friend_group.{list,get,create,put,delete}
├── server.friend_group.members.{list,add,put,delete}
├── server.friend_group.messages.{list,get,send}
├── server.firmware.{list,get}
├── server.firmware.files.download
├── server.game_ruleset.get
├── server.pet_def.pixa.download
├── server.badge_def.icon.download
├── server.pet.{list,get,adopt,put,delete,drive}
├── server.points.get
├── server.points.transactions.{list,get}
├── server.badge.{list,get}
├── server.game_result.{list,get}
└── server.reward_grant.{list,get}

Admin Service
├── /@apply POST
├── /resources/{kind}/{name} GET, PUT, DELETE
├── /acl/views/{name} LIST, CREATE, GET, PUT, DELETE
├── /acl/roles/{name} LIST, CREATE, GET, PUT, DELETE
├── /acl/policy-bindings/{id} LIST, CREATE, GET, PUT, DELETE
├── /workflows/{name} LIST, CREATE, GET, PUT, DELETE
├── /firmwares/{name} LIST, CREATE, GET, PUT, DELETE
│   └── /artifacts/{channel} GET, PUT, DELETE
│       ├── /entries GET
│       ├── /tree GET
│       ├── /stat GET
│       └── /download GET
├── /credentials/{name} LIST, CREATE, GET, PUT, DELETE
├── /models/{id} LIST, CREATE, GET, PUT, DELETE
├── /game-rulesets/{name} LIST, CREATE, GET, PUT, DELETE
├── /pet-defs/{id} LIST, CREATE, GET, PUT, DELETE
│   └── /pixa GET, PUT
├── /badge-defs/{id} LIST, CREATE, GET, PUT, DELETE
│   └── /icon GET, PUT
├── /game-defs/{id} LIST, CREATE, GET, PUT, DELETE
├── /dashscope-tenants/{name} LIST, CREATE, GET, PUT, DELETE
├── /gemini-tenants/{name} LIST, CREATE, GET, PUT, DELETE
├── /openai-tenants/{name} LIST, CREATE, GET, PUT, DELETE
├── /minimax-tenants/{name} LIST, CREATE, GET, PUT, DELETE
│   └── @sync-voices
├── /volc-tenants/{name} LIST, CREATE, GET, PUT, DELETE
│   └── @sync-voices
├── /voices/{id} LIST, CREATE, GET, PUT, DELETE
├── /workspaces/{name} LIST, CREATE, GET, PUT, DELETE
├── /peers/{publicKey}/contacts/{id} LIST, CREATE, GET, PUT, DELETE
├── /peers/{publicKey}/friend-requests/{id} LIST, CREATE, GET, PUT, DELETE
│   ├── @accept
│   └── @reject
├── /peers/{publicKey}/friends/{id} LIST, GET, DELETE
├── /friend-groups/{id} LIST, CREATE, GET, PUT, DELETE
│   ├── /members LIST, CREATE, GET, PUT, DELETE
│   └── /messages LIST, CREATE, GET
├── /peers/{publicKey} LIST, GET, DELETE
│   ├── /info GET, PUT
│   ├── /config GET, PUT
│   ├── /runtime GET
│   ├── /status GET
│   ├── /pets/{id} LIST, GET
│   ├── /badges/{id} LIST, GET
│   ├── /points GET
│   │   └── /transactions/{id} LIST, GET
│   ├── /game-results/{id} LIST, GET
│   ├── /reward-grants/{id} LIST, GET
│   ├── @approve
│   ├── @block
│   └── @refresh
└── /peers
    ├── @findPubKeyBySn
    └── @findPubKeyByImei
```
