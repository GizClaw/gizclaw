# GizClaw Lua for Flutter

`GizClawLuaAppHost` executes installed Lua Apps on the GizOS firmware Host.
`package:gizclaw` owns only the App host interface and `client.app.*` RPC wiring.
This package owns native assets, package storage, board PAL adapters, and widgets.
macOS desktop is the verified target. Linux uses the same source closure but is
not verified here. Mobile, Windows, and Flutter Web builds are not supported.

## Host and board

```dart
final board = GizClawLuaBoard(
  displayWidth: 240,
  displayHeight: 240,
  buttons: const [
    GizClawLuaButton('ok', key: LogicalKeyboardKey.enter),
    GizClawLuaButton('back', key: LogicalKeyboardKey.escape),
  ],
  touch: true,
);
final host = GizClawLuaAppHost(
  runtime: 'runtime.lua.my_desktop',
  storageDirectory: Directory('/absolute/path/to/apps'),
  board: board,
);
host.registerCapability('test.echo', (input, options) async => input);
await host.start();
// Bind host to GizClawClient's appHost parameter. The server reconciles installs.
final view = GizClawLuaBoardView(board: board);
// At shutdown:
await host.close();
board.dispose();
```

Register capabilities before `start()` or the first execution/installation.
Registration is frozen after start, and duplicate names are rejected. There are
at most 16 names, each namespaced and shorter than 48 ASCII characters, matching
this GizOS Host's registry. `list()` reports the configured runtime ID and names from the host-owned Dart
registry used for registration; it does not enumerate the GizOS registry. An App's optional `requires` must be a subset of these names;
installation and invocation reject missing requirements. Runtime matching is
exact string equality. The runtime ID is a required constructor argument: this
adapter must not advertise `runtime.lua.gizos` as a fully implemented profile.

The native registry callback copies input into a bounded queue and returns
`H2_PAL_ERR_WOULD_BLOCK`. A 5 ms Dart timer dispatches the async handlers and calls
`h2_lua_capability_complete`. Cancellation calls the optional Dart `cancel`
callback; the handler must implement its own cancellation. Late completions are
ignored. The firmware accepts capability outputs of at most 511 UTF-8 bytes and
errors of at most 191 bytes; oversized outputs become a capability error instead
of leaving a job waiting. Inputs/options are bounded to 4095 bytes by the bridge.
Dart callbacks never run on native workers. Capability output is a string:

```lua
local capability = require('capability')
local json = require('json')
return {
  echo = function(input)
    local ok, output, err = capability.call('test.echo', input, {})
    assert(ok, err)
    return json.decode(output)
  end,
}
```

Apps use the firmware `require`, `json`, `runtime`, `display`, `lcd_touch`,
`audio`, `system`, and `capability` modules. There is no custom require or JSON
shim. `require('helper')` loads the packaged `helper.lua`; dotted module names
resolve to nested directories, using the firmware's module-name restrictions.
The entry file is compiled as text inside a method-dispatch wrapper. Args must
be a JSON object; each method receives its decoded table. The wrapper validates
the entry table and method functions, then JSON-encodes the method result.

Every invocation/job uses `h2_lua_job_submit_text` and an isolated firmware VM.
The Host owns one worker and a bounded set of live jobs (default 8, maximum 64).
The method wrapper returns its JSON-encoded value. On success the bridge uses
`h2_lua_job_get_result()` to query its byte length and copy the complete result
before releasing the job. It uses public Runtime/Lua/PAL headers only.
`GizClawLuaVm` remains a separate synchronous, low-level Core API, without App
hosting, timeouts, or board modules; use the App host for untrusted executions.

## Board and skins

Board data consists of dimensions, ordered named buttons and optional keyboard
keys, touch presence, and audio input/output presence. A button's component ID
is its index plus one (`board.componentId('ok')`). `pushButton('ok', true/false)`
feeds Runtime PUSH_EDGE input; the adapter drains Runtime events and dispatches
them to live Lua jobs. Apps subscribe with `runtime.components.on(id,
runtime.event.BUTTON_DOWN, callback)` and can wait using `runtime.sleep`.
Keyboard and pointer holds are combined by input source. Focus/lifecycle loss
releases input. Touch coordinates are framebuffer coordinates; `pushTouch`
uses kind 1=down, 2=move, 3=up and a bounded 64-event queue.

The default skin creates a display, one control per named button, and a status
line. A custom skin places the same slots in an arbitrary widget tree:

```dart
GizClawLuaBoardView(
  board: board,
  controls: yourHostLifecycleControls,
  skin: const Column(children: [
    GizClawLuaBoardSlot.status(),
    GizClawLuaBoardSlot.display(),
    GizClawLuaBoardSlot.button('ok'),
    GizClawLuaBoardSlot.button('back'),
    GizClawLuaBoardSlot.controls(),
  ]),
)
```

`controls` is supplied by the embedding app (for example, start/shutdown UI).
An omitted controls widget is empty. Slots bind through their enclosing board
view; unknown button names fail. Skins never enter the Lua Runtime. Display
uses RGB565 at the declared board size; PAL draw/present operations copy into
native storage, then Dart receives framebuffer snapshots. `board.framebuffer`
returns a copy; `board.pixelAt(x, y)` permits deterministic readback. Display
and touch are exclusive firmware resources: concurrent jobs acquiring the same
display/touch fail busy rather than sharing a job-owned lifecycle.

## PAL coverage and limits

- Memory, Time, Task, Queue, Sync, Log: package-owned POSIX/pthreads services
  in `native/os_posix.c`. Queues are bounded, use monotonic timeouts, wake on
  close, and drain retained items. Sync provides mutexes; semaphore/condition
  operations are unsupported. Tasks honor minimum stack size and join ownership.
- Filesystem: package-owned read-only relative path provider. Directory-fd
  traversal with `openat` and `O_NOFOLLOW` rejects symlink escapes. App packages are SHA-256/size verified, expanded with limits,
  and reject unsafe paths, links, Lua bytecode and invalid UTF-8 Lua text.
- Timer: canonical unsupported Timer from the source package. Lua sleep uses
  the firmware Host's monotonic deadline fallback; no timer thread is required.
- Display, Button, Touch: real Flutter framebuffer/input bridge; no skin state
  is visible to Lua.
- Audio: **test-only in-memory PCM source/sink**, enabled by board flags. Mono
  16 kHz S16LE, 320 samples/frame, eight frames per source/sink and four tracks.
  `host.pushAudioInput(bytes)` injects whole frames;
  `host.readAudioOutput()` drains captured bytes. No microphone capture,
  speaker playback, mixing, volume control, or playback clock is implemented.
- Remaining PAL services, including network, BLE, codecs and physical sensors,
  use canonical unsupported providers. The optional BLE link library is not
  enabled. These gaps are why no firmware runtime profile ID is assumed.

One host exclusively owns its storage directory and board. Replacement and
uninstall update the index atomically; retired package generations are retained
until host shutdown so live jobs can still require their own modules. Shutdown
cancels and drains all jobs before deleting retired generations. An interrupted
process may leave unreferenced generations on disk. Package asset files are
retained, but this package adds no independent Lua asset-loading API.

## Native build and verification

The only GizOS build input is `gizos-lua-runtime-src.tar.gz` or its extracted
source directory. No GizOS checkout, Bazel metadata, separate Lua/yyjson download,
precompiled runtime, or RPC regeneration is used.

```sh
flutter pub get
GIZOS_LUA_RUNTIME_SRC=/absolute/path/gizos-lua-runtime-src.tar.gz \
  ./tool/with_gizos.sh test --no-pub
dart format --set-exit-if-changed .
flutter analyze --no-pub
```

Resolution prefers `GIZOS_LUA_RUNTIME_SRC` from the hook environment, then the
same named hook user-define (a path), then the local setting forwarded by the
wrapper. Flutter versions that sanitize hook environments need the wrapper;
it records the absolute path in ignored `.dart_tool/gizos-lua-runtime-src`.
Run it again to change that path; delete that setting to return to the pinned
package. `GIZOS_ROOT` has no effect. A consuming project's hook user-define can
also supply the path through its pubspec:

```yaml
hooks:
  user_defines:
    gizclaw_lua:
      GIZOS_LUA_RUNTIME_SRC: /absolute/path/gizos-lua-runtime-src.tar.gz
```

Without a local override, `runtimePackagePin` in `hook/runtime_package.dart`
provides the single `{url, sha256, size}` reference. It is empty until GizOS
publishes a release; builds then fail with instructions to set
`GIZOS_LUA_RUNTIME_SRC`. Downloads require both SHA-256 and size verification.
Local tarballs are also checked against SHA-256/size when the pin is populated;
extracted directories are explicit development overrides and are not digest
pinned. Extraction rejects absolute/traversal paths, links, special files, and
duplicate entries. Caches live under the hook output directory, keyed by archive
SHA-256. Schema versions other than `1` and profiles other than
`runtime.lua.gizos` fail before compilation. That source profile describes the
lower runtime; the embedding host still declares its own supported runtime ID.

`CBuilder` compiles exactly `manifest.json.sources`, once per source, using the
common and corresponding `compilation_units` flags/defines and manifest include
directories. Each group produces a private static archive; the final native
asset links all groups plus the target's `per_os.link_flags`. CBuilder supplies
the target architecture, sysroot, PIC and deployment version. Apple compilation
suppresses unused command-line warnings caused by CBuilder's linker-only flag
on static compile steps; manifest warning/error flags remain in effect.

| Source owner | Compiled content |
| --- | --- |
| Source package: Lua vendor | Selected patched Lua 5.5 C files in `external/...h2_vendor_lua/src/` |
| Source package: yyjson vendor | `external/...h2_vendor_yyjson/src/src/yyjson.c` |
| Source package: `libs/lua/src/core/` | Core VM and text loader |
| Source package: `libs/lua/src/modules/`, `runtime/` | Modules, Host, jobs, events and task names |
| Source package: `libs/runtime/src/` | Runtime sources selected by the manifest |
| Source package: `libs/pal/src/unsupported/` | Canonical unsupported API implementations |
| This package: `native/host.c` | Host bridge, display/button/touch/audio vtables |
| This package: `native/os_posix.c` | Memory/log/time/task/queue/mutex and read-only filesystem |

No PAL provider or C++ source from GizOS is linked. Native builds support
macOS and Linux hosts; Linux is unverified. iOS, Android, Windows and Web are
not supported by this hook. Tests cover source-package verification, async echo,
missing requirements, RGB565 pixels, Runtime button events, pending-capability
cancellation, PCM loopback, App package safety, job lifecycle and board skins.
