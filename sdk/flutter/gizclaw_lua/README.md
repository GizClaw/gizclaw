# gizclaw_lua

Native Lua App hosting for Flutter peers. The core `gizclaw` package owns
`GizClawAppHost`, job completion events, and the `client.app.*` RPC handlers;
it does not build or download Lua. Depend on `gizclaw_lua` only when hosting
Lua Apps.

```dart
import 'dart:io';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_lua/gizclaw_lua.dart';

final host = GizClawLuaAppHost(
  runtime: 'runtime.lua.example',
  storageDirectory: Directory('apps'),
);
final handlers = GizClawPeerRpcHandlers(
  deviceInfo: () => DeviceInfo(name: 'lua-peer'),
  appHost: host,
);
```

Choose the runtime ID matching the modules actually provided by the client.
The VM Core does not supply GizOS display/audio or Runtime Host modules.
One host exclusively owns its storage directory. Stop RPC handling and await
`host.close()` when disposing the peer. Subscribe to `host.jobCompletions`
before starting jobs to observe results or errors.

The host downloads and verifies App packages by SHA-256 and size, extracts
`.tar.zlib` archives, validates `app.json` and confined paths, and runs text
Lua entry modules. Packaged `require`, JSON helpers, call methods, and
cancellable jobs are supported. See the public host API for execution limits.

## Native build and tests

A GizOS checkout is required, including `libs/lua/include`,
`libs/lua/src/core`, and `third_party/lua.BUILD.bazel`. A native C compiler
is also required. macOS desktop is the validated platform; this package uses
`dart:ffi` and does not support browsers.

From this package directory:

```sh
flutter pub get
dart format --set-exit-if-changed .
flutter analyze --no-pub
GIZOS_ROOT=/path/to/gizos ./tool/with_gizos.sh test --no-pub
```

Flutter sanitizes the native hook environment. The wrapper records
`GIZOS_ROOT` in ignored `.dart_tool/gizos-root`, then runs Flutter in the
caller's current directory. Use the same wrapper for the consuming app's
`run` or `build` command, e.g. from that app's directory:
`GIZOS_ROOT=/path/to/gizos /path/to/gizclaw_lua/tool/with_gizos.sh run`.
The hook also accepts the `gizos_root` native hook user define and discovers
a sibling `gizos` checkout when no explicit path is provided.

The hook downloads the Lua archive pinned to revision
`a5522f06d2679b8f18534fd6a9968f7eb539dc31` from
[Lua upstream](https://codeload.github.com/lua/lua/tar.gz/a5522f06d2679b8f18534fd6a9968f7eb539dc31).
Its required SHA-256 is
`b85ede9dbc4a4292addcfefebe15168acc7f1d42c674cc297d74f0e8ff2f2dd8`.
Downloaded sources and native build output are cached under `.dart_tool`.
A cold build needs network access to that archive; cached archives are
verified again. GizOS sources are compiled in place. No Bazel command runs.

CI testing needs a separate native-host job with an authenticated GizOS
checkout at a reviewed revision, the native C toolchain, Flutter, dependency
installation, and the commands above. Core SDK and giztest jobs do not need
GizOS or Lua.
