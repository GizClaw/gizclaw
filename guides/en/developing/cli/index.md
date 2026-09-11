# CLI <Badge type="warning" text="WIP" />

> This page currently only defines the directory and ownership of the CLI. The command tree, configuration, connection and running process still need to be added one by one.

`cmd/gizclaw` is the executable entry of GizClaw CLI, `cmd/internal` saves the internal implementation of CLI assembly, commands, connections, logs, paths, services and local stores.

The CLI is responsible for converting user input and local environment into stable SDK or service calls and does not own GizClaw domain resources, RPC contracts or transport implementations. Reusable client capabilities should be entered into the corresponding SDK; server-side business behavior should be entered into `pkgs/gizclaw`.

## Directory

```text
cmd/
├── gizclaw/          # executable main
└── internal/
    ├── commands/     # command tree and command entry points
    ├── connection/   # CLI context connections, delegating to sdk/go/gizcli/contextconn
    ├── adminapi/     # Admin API adapter
    ├── deviceapi/    # Device-facing adapter
    ├── peerapi/      # Peer-facing adapter
    ├── server/       # local server wiring
    ├── service/      # CLI service wiring
    ├── storage/      # CLI-owned local state
    └── stores/       # CLI store construction
```

Manifest preparation, single and batch reads, error text, and NOT_FOUND detection for `gizclaw admin apply|show|delete|validate` come from `sdk/go/gizcli/adminresource`, the same implementation used by the [Terraform Provider](/en/using/terraform). `cmd/terraform-provider-gizclaw` is a separate provider executable that depends only on these SDK packages, not on `cmd/internal`.

When modifying the API or RPC called by the CLI, you should also read the corresponding [API Design](../api/overview) and SDK documentation.
