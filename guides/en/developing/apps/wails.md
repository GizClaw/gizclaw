# Wails App

`apps/wails` is a desktop control plane for managing local and remote GizClaw Server based on Pod. Wails
The window is only responsible for environment management, server life cycle and native desktop integration; Admin UI and Play UI
As a standalone browser application, served through the native HTTP port.

## Module boundaries

```text
apps/wails/
├── internal/
│   ├── appconfig/       # pod.json, directory projection, and permissions
│   ├── bridge/          # Wails capabilities that never return secret keys
│   ├── endpointhealth/  # /server-info health probes
│   ├── localserver/     # local Server lifecycle and bounded logs
│   ├── tray/            # system tray integration
│   └── webui/           # loopback HTTP and local runtime tokens
├── i18n/locales/        # en and zh-CN copy
└── frontend/            # Pod desktop home and Admin/Play browser entry points
```

Desktop App does not copy the server-side business of `pkgs/gizclaw`. `api/http/desktop.json` Yes
Schema source of desktop bridge DTO; generated through `gen:sdk` of `sdk/js` after update
`frontend/src/generated/desktopservice`.

## Local Server bootstrap

`resources/local-server` embeds only Desktop-owned PIXA binaries. The fixed public Raids `v0.2.2`
GitHub archive is the declarative source for `RuntimeProfile/default`,
`RegistrationToken/default-runtime`, and the Credential, Tenant, Model, Voice, Workflow, and PetDef
resources referenced by that profile. Desktop validates and caches the archive privately below its
config root, resolves only the profile dependency closure, and applies dependencies, PIXA binaries,
the profile, and then the token. `runtime-profile.example.yaml` remains documentation-only.

Credential templates come from Raids; credential values remain in Desktop's private
`bootstrap.env` or the process environment. The archive cache, RuntimeProfile, `pod.json`, URLs,
Web Storage, and logs never contain those values. If neither a valid cache nor GitHub is available,
Desktop and remote Pod management remain usable, but new local Pod creation and required local
runtime-contract migration fail before they can partially apply a catalog.

Raids publishes `RegistrationToken/default-runtime` with the deterministic public UUID
`28c4e4e9-a05f-5a7e-815e-9cf9afb6878f`, bound to `RuntimeProfile/default`. Desktop decodes and
validates that committed value; it does not derive a UUID or generate another local token. Local Play
receives the validated value through the protected per-launch Browser Runtime handoff, and the local
Pod share QR carries it in the existing `registration_token` field. The value does not enter the
URL, `pod.json`, a workspace handoff file, Web Storage, or logs. Remote Pods continue to require
their explicitly configured deployment token and never fall back to the Raids public token.

The Raids token is a public reusable enrollment identifier, not an Admin credential. Any Peer that
can reach a Desktop local Server on its LAN-facing address and knows this UUID can attempt to
register into `RuntimeProfile/default`; Admin access still requires the separate Admin identity.
Each local Server persists its own independent resource instances.

Completed local Pods do not replay the full bootstrap catalog during start,
restart, or Desktop upgrade. A legacy local Pod performs one targeted migration
after its Server is ready: Desktop reapplies the resolved Raids dependency closure and PIXA assets,
replaces `RuntimeProfile/default`, applies `RegistrationToken/default-runtime`, retires
`RegistrationToken/app:com.gizclaw.opensource` and `RegistrationToken/desktop-local`, removes the
obsolete workspace token handoff, and records the local catalog version in `pod.json`. A recovered
legacy process is restarted with the current companion
before migration, and the default profile preserves legacy translation aliases
for existing Workspaces. Unreferenced Workflows and other resources, including
user edits, remain unchanged.
Desktop suppresses QR and Play token handoff until this migration completes; a failed apply or
cleanup leaves the old catalog version so a later retry can converge.

## Local Server recovery

Each running local Server stores `workspace/server.pid`. After an abnormal Desktop exit, recovery
first confirms that the PID is alive and retries the Pod's loopback `/server-info` identity check for
up to five seconds. Transient verification failures preserve the PID; a definitive public-key
mismatch removes it. Desktop never signals an unverified PID. For an interrupted bootstrap, cleanup
only removes the workspace after the recovered Server has been stopped; otherwise the PID and
workspace are preserved and cleanup aborts. Normal local Pods with transient recovery failures remain
visible with failed process and health status. Lifecycle mutations retry verification and are rejected
while the PID remains unverified; a definitive identity mismatch clears the stale PID as stopped.

## Pod projection

`pod.json` is the only editable configuration source. After each save, `appconfig.Store` is updated atomically:

- `workspace/config.yaml` of local Pod, where the listening address is `0.0.0.0:<port>`,
  The Server endpoint uses the currently available LAN address and is still published to the local Context.
  `127.0.0.1:<port>`; The LAN address is not written `pod.json`;
- One for each Server configured with Admin identity
  `admin_context/<server-id>/config.yaml`;
- Pod level `client_context/config.yaml` is generated when Client identity is configured;
- remote Pod does not create `workspace/`, nor does it provide process control.

Pod ID and new remote Server ID are generated by bridge and are only used for directory and stable reference, not as
Desktop creates form fields. The remote Pod can save only the Access Point first and then add it from the details
Server; projection logic must support empty `remote_servers`.

The complete local Server workspace defaults are owned by the binary-embedded
`internal/appconfig/templates/local_server_workspace.yaml.gotmpl`; runtime does not read the source
tree. The renderer preserves the generated Server identity, refreshes listen, LAN endpoint, Admin
key, and store inventory, and atomically writes the file with mode `0600`. The template explicitly
uses an info-level stderr `system_log` and creates no LogStore, Volc credential, store sink, or
`query_store`; persisted and queryable logging requires explicit user configuration.

Directories and key files must remain private. Write using temporary files in the same directory, synchronization, rename
Atomic replacement process. The front-end response can only contain statuses such as `admin_configured`, `play_configured`, etc.
Persistence keys cannot be returned.

## Browser Runtime

The static products of Admin and Play are started from `admin.html` and `play.html` respectively. Each
Pod/surface retains one `127.0.0.1:0` listener and creates a distinct random token for every launch,
binding that token to the selected Runtime. The token remains in the local URL query, and the browser
presents it through a same-origin POST whenever it opens or refreshes. Each launch token remains valid
until its listener closes. Runtime responses are not
cached; private keys must not enter URLs, Web Storage, logs, or static files.

The Go part follows [Go coding specifications](/en/coding-styles/go), and the frontend follows
[JavaScript and TypeScript](/en/coding-styles/js).

## Packing boundaries

The macOS distribution is built by `apps/wails/scripts/package-darwin.sh`. Script first generates Wails
application, and then compile the existing `cmd/gizclaw` in the repository into
`Contents/Resources/gizclaw` companion. The desktop process first resolves the file from the application resource directory.
Program; environment variables and `PATH` are only used for development and testing, and are not a prerequisite for the distribution package to run.

## Development and validation

```sh
npm ci
npm --prefix apps/wails/frontend run build
npm --prefix apps/wails/frontend test
npm --prefix apps/wails/frontend run test:e2e
cd apps/wails && go test ./...
./scripts/package-darwin.sh
```

`api/http/desktop.json` is the Desktop OpenAPI source. Update the committed
TypeScript surface with `npm --prefix sdk/js run gen:sdk`.

## Admin UI conventions

Admin is a dense operator console. Use `PageHeader` for breadcrumbs and
page-level actions. `PageSummaryCard` only presents identity, description, and
compact metadata. Create and refresh actions belong in the list-page header;
creation uses a Dialog with a title and description.

The first table column shows the copyable stable unique ID. Clicking a row
opens its detail page, so do not add Open or Actions columns; the final column
is Updated. Bound and truncate long IDs while retaining tooltip/copy access.
Tables must fit the default content width without relying on horizontal scroll.
Buttons inside clickable rows stop event propagation.

Back, Reload, and destructive detail actions remain in the header; summary
cards contain no actions. Use tabs for distinct resource surfaces and put edit
forms in the corresponding tab or a dialog. Sensitive and destructive actions
require confirmation. A create dialog closes only after successful creation or
resolution to an existing resource.
