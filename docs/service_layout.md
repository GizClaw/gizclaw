# GizClaw Surface Layout

This document describes user-facing API surfaces. Transport service IDs are
named only when they are part of the public SDK contract.

## Transport Services

`Service*` names are reserved for RPC and HTTP surfaces carried over reliable
giznet service streams:

```text
ServicePeerRPC    = 0x00
ServicePeerHTTP   = 0x01
ServicePeerOpenAI = 0x02
ServiceAdminHTTP  = 0x10
```

Event streams and media streams are not listed as services here. They are
defined in `docs/event_stream.md`.

## Doc Style

- Business RPC-style methods use dotted names:
  `surface.resource.method`.
- Multiple methods on one business resource use braces:
  `resource.{list,get,create,put,delete}`.
- HTTP endpoints use path-first notation:
  `/path OPERATION[, OPERATION]`.
- Custom HTTP verbs are listed under their resource path as subtree items:
  `@verb`.

Peer-reported status updates are projected from telemetry event packets into
`server.status.get`; there is no peer-facing `server.status.put` RPC method.

## Peer RPC Surface

The peer RPC surface uses `ServicePeerRPC`.

```text
Common RPC
└── all.ping

Client RPC
├── client.info.get
└── client.identifiers.get

Server RPC
├── server.info.{get,put}
├── server.runtime.get
├── server.status.get
├── server.run.say
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
├── server.badge_def.pixa.download
├── server.pet.{list,get,adopt,put,delete,drive}
├── server.points.get
├── server.points.transactions.{list,get}
├── server.badge.{list,get}
├── server.game_result.{list,get}
└── server.reward_grant.{list,get}
```

## Peer HTTP Surface

The peer HTTP surface uses `ServicePeerHTTP`.

```text
/server-info GET
/login POST
/webrtc/v1/offer POST
```

## Peer OpenAI-Compatible HTTP Surface

The peer OpenAI-compatible HTTP surface uses `ServicePeerOpenAI`. It exposes
OpenAI-compatible HTTP routes over the peer connection and is separate from the
peer HTTP bootstrap/signaling surface.

## Admin HTTP Surface

The admin HTTP surface uses `ServiceAdminHTTP`.

```text
/@apply POST
/resources/{kind}/{name} GET, PUT, DELETE
/acl/views/{name} LIST, CREATE, GET, PUT, DELETE
/acl/roles/{name} LIST, CREATE, GET, PUT, DELETE
/acl/policy-bindings/{id} LIST, CREATE, GET, PUT, DELETE
/workflows/{name} LIST, CREATE, GET, PUT, DELETE
/firmwares/{name} LIST, CREATE, GET, PUT, DELETE
└── /artifacts/{channel} GET, PUT, DELETE
    ├── /entries GET
    ├── /tree GET
    ├── /stat GET
    └── /download GET
/credentials/{name} LIST, CREATE, GET, PUT, DELETE
/models/{id} LIST, CREATE, GET, PUT, DELETE
/game-rulesets/{name} LIST, CREATE, GET, PUT, DELETE
/pet-defs/{id} LIST, CREATE, GET, PUT, DELETE
└── /pixa GET, PUT
/badge-defs/{id} LIST, CREATE, GET, PUT, DELETE
└── /pixa GET, PUT
/game-defs/{id} LIST, CREATE, GET, PUT, DELETE
/dashscope-tenants/{name} LIST, CREATE, GET, PUT, DELETE
/gemini-tenants/{name} LIST, CREATE, GET, PUT, DELETE
/openai-tenants/{name} LIST, CREATE, GET, PUT, DELETE
/minimax-tenants/{name} LIST, CREATE, GET, PUT, DELETE
└── @sync-voices
/volc-tenants/{name} LIST, CREATE, GET, PUT, DELETE
└── @sync-voices
/voices/{id} LIST, CREATE, GET, PUT, DELETE
/workspaces/{name} LIST, CREATE, GET, PUT, DELETE
/peers/{publicKey}/contacts/{id} LIST, CREATE, GET, PUT, DELETE
/peers/{publicKey}/friend-requests/{id} LIST, CREATE, GET, PUT, DELETE
├── @accept
└── @reject
/peers/{publicKey}/friends/{id} LIST, GET, DELETE
/friend-groups/{id} LIST, CREATE, GET, PUT, DELETE
├── /members LIST, CREATE, GET, PUT, DELETE
└── /messages LIST, CREATE, GET
/peers/{publicKey} LIST, GET, DELETE
├── /info GET, PUT
├── /config GET, PUT
├── /runtime GET
├── /status GET
├── /pets/{id} LIST, GET
├── /badges/{id} LIST, GET
├── /points GET
│   └── /transactions/{id} LIST, GET
├── /game-results/{id} LIST, GET
├── /reward-grants/{id} LIST, GET
├── @approve
├── @block
└── @refresh
/peers
├── @findPubKeyBySn
└── @findPubKeyByImei
```

## Implementation Packages

Peer-facing social resources are implemented as focused packages:

```text
pkgs/gizclaw/services/social/contact
pkgs/gizclaw/services/social/friend
pkgs/gizclaw/services/social/friendgroup
```
