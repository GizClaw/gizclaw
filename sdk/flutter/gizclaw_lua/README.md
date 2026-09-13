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
this GizOS Host's registry. `list()` reports the configured runtime ID and all
registered names. An App's optional `requires` must be a subset of these names;
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
A private native `_gizclaw_result` module collects only the calling job's result,
because the current firmware Host exposes status but has no public return-value
accessor. This small adapter uses the firmware execution-context header and is
compiled against the checkout on every native rebuild. It does not alter GizOS.
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

- Memory, Time, Task, Queue, Sync, Log: GizOS Desktop core providers.
- Filesystem: GizOS POSIX host filesystem, mounted behind a read-only relative
  path adapter. App packages are SHA-256/size verified, expanded with limits,
  and reject unsafe paths, links, Lua bytecode and invalid UTF-8 Lua text.
- Timer: the Desktop provider itself uses canonical unsupported Timer. Lua
  sleep uses the firmware's monotonic deadline fallback, as on GizOS Desktop.
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

```sh
flutter pub get
GIZOS_ROOT=/Users/idy/GizClaw/gizos ./tool/with_gizos.sh test --no-pub
dart format --set-exit-if-changed .
flutter analyze --no-pub
```

Flutter sanitizes hook environments; the wrapper records `GIZOS_ROOT` in ignored
`.dart_tool/gizos-root`. Sources are read in place. No Bazel command or generated
RPC update is needed. Builds download and SHA-256 verify Lua 5.5 at revision
`a5522f06d2679b8f18534fd6a9968f7eb539dc31` and yyjson at the checkout's
`MODULE.bazel` pin. Download/build caches stay under `.dart_tool`.

Source lists are derived from the corresponding GizOS BUILD files:

| Source target | Compiled content |
| --- | --- |
| `third_party/lua.BUILD.bazel` | Upstream Lua C sources selected by GizOS |
| `libs/lua:lua_core` | VM and text loader |
| `libs/lua:lua_runtime` | Host, job scheduler, events, firmware modules, task names |
| `libs/runtime:runtime` | Full `RUNTIME_SRCS`, including input/event/audio and task names |
| `libs/pal:unsupported` | All canonical unsupported PAL implementations |
| `libs/pal/providers/desktop/pal_core:core` | `h2_desktop_platform_core.cpp`, compiled as C++17 |
| `libs/pal/providers/posix/pal_core:host_fs` | `h2_posix_host_fs.c` |
| GizOS yyjson pin | `src/yyjson.c` |
| This package | `native/host.c` |

GizOS PAL/Runtime/Lua headers are included directly. Desktop simulators and
network/production audio providers are not linked. Unexpected source-list or
vendor layouts fail the hook. Tests cover async echo, missing requirements,
RGB565 pixels, Runtime button events, pending-capability cancellation, PCM
loopback, package safety, job limits/lifecycle, and default/custom slot skins.
