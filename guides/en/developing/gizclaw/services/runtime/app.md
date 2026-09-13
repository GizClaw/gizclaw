# Apps

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/app)

An App is a device-installed Lua program. Admin resources use `kind: App`, a canonical `metadata.id`, and `spec.package: {url, sha256, size}`. There is no App enabled flag or version. Generic Resource apply/get/put/delete manage the server catalog; deleting a catalog entry does not uninstall a device package.

Apply downloads the HTTPS `.tar.zlib` archive, verifies its exact compressed size and lowercase SHA-256, and stores its parsed manifest in the `apps` SQL table. Limits are 16 MiB compressed, 64 MiB unpacked, 1024 archive entries, 256 KiB for app.json, and 128 methods. Paths must be relative with no empty, dot or parent segments and no backslashes. Links and special files are rejected. Lua files must be UTF-8 text, not bytecode; binary assets are allowed. The entry must be an existing `.lua` file.

The root `app.json` declares `app_name`, `runtime`, `entry`, and `methods`. App and method names match `^[A-Za-z_][A-Za-z0-9_-]{0,63}$`. App name is immutable. Runtime is one opaque pattern-constrained profile ID, such as `runtime.lua.gizos`, compared by exact equality. Each method declares name, mode (`call` or `job`), description (at most 1024 characters), and an object input JSON Schema. The entry module returns a table of functions keyed by method name. Each function receives one decoded argument table and returns a JSON-encodable value.

`RuntimeProfile.spec.resources.apps` maps aliases to `{resource_id, i18n}`. On connection, the server requests `client.app.list` and installs profile packages with an exactly matching runtime and all required capabilities whose SHA-256 is missing or different. Reconciliation failures are logged and do not reject the connection. PeerConn caches the runtime, reported capabilities, and installed package hashes, updating the cache after successful installs. App methods remain absent until reconciliation succeeds. Model projection and invocation read this cache without requesting the device list again and expose all methods whose runtime, required capabilities, and package hash match as `<app_name>__<method>`. Collisions with server Tool invocation names or other App methods fail toolkit construction.

Call methods use `client.app.invoke`; job methods use `client.app.job.start` and return `{job_id}`. Invocation uses the current accepted peer connection, defaults to 10 seconds, and returns recoverable JSON errors with code `timeout` or `unavailable`. Arguments must encode a JSON object within 4096 bytes; results must be valid JSON within the same limit. The RPC contract also provides uninstall and job cancellation, with payloads owned by `api/proto/rpc/payload/app.proto`.

App display names and i18n belong to RuntimeProfileBinding at deploy time, not to App.


## Capability requirements and host hooks

The optional manifest `requires` array contains at most 64 unique capability names matching `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`, for example `"requires": ["litelink.notify"]`. Omission or an empty array means no capability requirements; null is invalid. The server validates and retains this field in the App read model. `client.app.list` reports every capability registered in the host. An App with any missing requirement or a mismatched runtime is neither installed nor exposed to the model, even if its exact SHA-256 is already installed.

Hooks follow the firmware GizOS `libs/lua` `h2_lua_host` contract. The embedding application calls `h2_lua_register_capability(name, call, cancel)` before `h2_lua_host_start()`; the registry is frozen after startup. Lua calls `capability.call(name, payload, options)` and observes exactly `ok, output, error`. Asynchronous handlers return `H2_PAL_ERR_WOULD_BLOCK` and later complete through `h2_lua_capability_complete`. Capability names are namespaced, such as `litelink.notify`.

The board provides PAL display, button, touch, and audio facilities; these are not registered per App. Lua Apps register event callbacks through `runtime.components.on/off` and `runtime.event.*`. Board data describes display_width, display_height, buttons (name to key), touch, and audio. Skins supply display, controls, status, and button(name) presentation slots; the runtime never reads the skin.
