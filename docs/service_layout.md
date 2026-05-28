# GizClaw Service Tree

This document describes business-level services, not transport service IDs.

Peer Service and Device Service are peer-provided services. Public Service,
Gear Service, and Admin Service are server-provided services.

## Doc Style

- Business RPC-style methods use dotted names: `service.resource.method`.
- Multiple methods on one business resource use braces: `resource.{list,get,create,put,delete}`.
- Resource endpoints use path-first notation: `/path OPERATION[, OPERATION]`.
- Custom HTTP verbs are listed under their resource path as subtree items: `@verb`.

```text
Peer Service
└── peer.ping

Device Service
├── device.info.get
└── device.identifiers.get

Public Service
├── server.info.get
├── /server-info GET
└── /login POST

Gear Service
├── peer.info.{get,put}
├── peer.runtime.get
├── workflow.{list,get,create,put,delete}
├── model.{list,get,create,put,delete}
└── credential.{list,get,create,put,delete}

Admin Service
├── /@apply POST
├── /resources/{kind}/{name} GET, PUT, DELETE
├── /acl/views/{name} LIST, CREATE, GET, PUT, DELETE
├── /acl/roles/{name} LIST, CREATE, GET, PUT, DELETE
├── /acl/policy-bindings/{id} LIST, CREATE, GET, PUT, DELETE
├── /workflows/{name} LIST, CREATE, GET, PUT, DELETE
├── /firmwares/{name} LIST, CREATE, GET, PUT, DELETE
│   ├── @release
│   └── @rollback
├── /credentials/{name} LIST, CREATE, GET, PUT, DELETE
├── /models/{id} LIST, CREATE, GET, PUT, DELETE
├── /dashscope-tenants/{name} LIST, CREATE, GET, PUT, DELETE
├── /gemini-tenants/{name} LIST, CREATE, GET, PUT, DELETE
├── /openai-tenants/{name} LIST, CREATE, GET, PUT, DELETE
├── /minimax-tenants/{name} LIST, CREATE, GET, PUT, DELETE
│   └── @sync-voices
├── /volc-tenants/{name} LIST, CREATE, GET, PUT, DELETE
│   └── @sync-voices
├── /voices/{id} LIST, CREATE, GET, PUT, DELETE
├── /workspaces/{name} LIST, CREATE, GET, PUT, DELETE
├── /peers/{publicKey} LIST, GET, DELETE
│   ├── /info GET, PUT
│   ├── /config GET, PUT
│   ├── /runtime GET
│   ├── @approve
│   ├── @block
│   └── @refresh
└── /peers
    ├── @findPubKeyBySn
    └── @findPubKeyByImei
```
