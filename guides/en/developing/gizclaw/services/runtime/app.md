# Apps

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/app)

An App is a device-installed Lua program. Admin resources use `kind: App`, a canonical `metadata.id`, and `spec.package: {url, sha256, size}`. There is no App enabled flag or version. Generic Resource apply/get/put/delete manage the server catalog; deleting a catalog entry does not uninstall a device package.

Apply downloads the HTTPS `.tar.zlib` archive, verifies its exact compressed size and lowercase SHA-256, and stores its parsed manifest in the `apps` SQL table. Limits are 16 MiB compressed, 64 MiB unpacked, 1024 archive entries, 256 KiB for app.json, and 128 methods. Paths must be relative with no empty, dot or parent segments and no backslashes. Links and special files are rejected. Lua files must be UTF-8 text, not bytecode; binary assets are allowed. The entry must be an existing `.lua` file.

The root `app.json` declares `app_name`, `runtime`, `entry`, and `methods`. App and method names match `^[A-Za-z_][A-Za-z0-9_-]{0,63}$`. App name is immutable. Runtime is one opaque pattern-constrained profile ID, such as `runtime.lua.gizos`, compared by exact equality. Each method declares name, mode (`call` or `job`), description (at most 1024 characters), and an object input JSON Schema. The entry module returns a table of functions keyed by method name. Each function receives one decoded argument table and returns a JSON-encodable value.

`RuntimeProfile.spec.resources.apps` maps aliases to `{resource_id, i18n}`. On connection, the server requests `client.app.list` and installs matching-runtime profile packages whose SHA-256 is missing or different. Reconciliation failures are logged and do not reject the connection. PeerConn caches the runtime and installed package hashes, updating the cache after successful installs. App methods remain absent until reconciliation succeeds. Model projection and invocation read this cache without requesting the device list again and expose all matching methods as `<app_name>__<method>`. Collisions with server Tool invocation names or other App methods fail toolkit construction.

Call methods use `client.app.invoke`; job methods use `client.app.job.start` and return `{job_id}`. Invocation uses the current accepted peer connection, defaults to 10 seconds, and returns recoverable JSON errors with code `timeout` or `unavailable`. Arguments must encode a JSON object within 4096 bytes; results must be valid JSON within the same limit. The RPC contract also provides uninstall and job cancellation, with payloads owned by `api/proto/rpc/payload/app.proto`.

App display names and i18n belong to RuntimeProfileBinding at deploy time, not to App.
