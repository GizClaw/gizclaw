# GizClaw Monitor diagnostic assistant

An agent that operates the Monitor console on the operator's behalf: it
navigates pages, opens links, reads device logs and analyzes them, reads what
the current page shows and calls the console's APIs, then answers from those
facts. What the console can do, the assistant does and then explains; what it
cannot do (such as changing a setting on the device), it only gives guidance
for and never claims to have done.

## Boundary

`@gizclaw/assistant` is a private workspace consumed directly as TypeScript by
the console's Vite build and by Node `--experimental-strip-types`. It does not
depend on React, the DOM or `@gizclaw/gizclaw-control`. The console implements
`AssistantRuntime` with the clients it already uses (`loadNode`, `loadPeer`,
`findPeers`, `loadTelemetry`, `loadDeviceLogs`) and its hash router.

The model is a Chat Completions model from `@openai/agents-openai` pointed at a
node's `/openai/v1` with a GizClaw API key. The Monitor console has no backend,
so tools run in the browser and GizClaw forwards declarations, calls and
results only. `@openai/agents` itself is not used: it bundles realtime, MCP
and WebSocket code the browser does not need. Tracing is disabled so nothing is
exported to the OpenAI platform.

## Runtime and tools

`AssistantRuntime` is the complete list of what the assistant can reach, and
`src/apis.ts` is the complete list of what the model can do: one catalog entry
per tool, naming the runtime methods it calls, its description, its zod
parameters and its implementation. Tools are generated from the catalog, a test
checks that every runtime method is used by exactly one tool, and
`RUNTIME_METHODS` fails to compile when a runtime method is left out of it.

| Runtime method                  | Console source                        | Tool                       |
| ------------------------------- | ------------------------------------- | -------------------------- |
| `page.current`, `view.snapshot` | hash router; the page's view model    | `get_current_page`         |
| `page.navigate`                 | `location.hash`                       | `navigate`                 |
| `page.openLink`                 | `window.open` after the user confirms | `open_link`                |
| `fleet.nodes`                   | `useFleet` node snapshots and rates   | `list_nodes`               |
| `fleet.traffic`                 | `useFleet` in-memory samples          | `get_node_traffic`         |
| `devices.watched`               | watch list                            | `list_devices`             |
| `devices.find`                  | `findBySn` / `findByImei`             | `find_device`              |
| `devices.status`                | `get` + `getRuntime` + `getStatus`    | `get_device_status`        |
| `devices.telemetry`             | `getTelemetryLatest`                  | `get_device_telemetry`     |
| `devices.telemetryRange`        | `queryTelemetry`                      | `query_device_telemetry`   |
| `devices.wifi`                  | `getWifi` + `listSavedWifi`           | `get_device_wifi`          |
| `devices.workspaces`            | `listWorkspaces`                      | `list_device_workspaces`   |
| `devices.history`               | `listWorkspaceHistory`                | `get_conversation_history` |
| `logs.search`                   | `searchLogs`                          | `search_logs`              |

The view model is the page's structured state, never the DOM. Mutating device
APIs (`reboot`, `setVolume`, `playSound`, `find`, Wi-Fi scan/connect/forget,
`deleteWorkspace`, audio player and contact writes) have no runtime method.
`scanWifi` is left out as well because it makes the device act.

A source that fails throws `SourceError(code, message, httpStatus?)`. Tools turn
it into `{ error: { code, message } }` for the model, so a
`DEBUG_ACCESS_FORBIDDEN` becomes something the assistant explains instead of a
broken conversation.

Tools declare non-strict JSON Schema generated from zod and validate arguments
with the same schema. Strict schemas are avoided because not every
RuntimeProfile model enforces them and Gemini rejects them. Optional arguments
accept `null`. Tool arguments are shaped for the model rather than for the
console: `navigate` takes `node_id`, `public_key`, `error_code`, `level` and
`text` and builds the console's log query itself, and `search_logs` returns an
aggregate by level, operation, error code, RPC code and HTTP status, at most 50
compact records, and ready-made `navigate_to_logs` arguments.

Every call is recorded as an `ActionRecord`, returned with each turn for the UI
and for tests. A turn's reply joins every text the assistant produced during
the turn, because models often explain their findings in the same response that
calls a tool and end with a short confirmation. A turn allows 12 model calls.

## Safety

- No tool writes: no reboot, firmware update, Wi-Fi change, volume or deletion.
- Log contents, conversation history, device fields and page data are
  untrusted; instructions in them are not followed, and no tool can reach an
  arbitrary network address.
- Conversation history reaches the model only through GizClaw `/openai/v1`,
  like every other tool result.
- External links always pass through the user's confirmation.

## Scenarios and validation

A scenario is a `World` (nodes and their traffic, devices with status,
telemetry, Wi-Fi, workspaces, history and logs or a source failure, the current
page), the user's messages, a script for the scripted
model, and expectations: tools that must be called (any of several names, with
argument subsets and optionally the error they fail with), the route the conversation ends on, and
facts the reply must contain or must not claim.

- `npm test` runs every scenario through the real Agents SDK runner with
  `FakeRuntime` and `ScriptedModel`. It proves the wiring: tool dispatch,
  argument validation, error mapping, result handoff and navigation.
- `scripts/run-live-scenarios.ts` runs the same scenarios against a real model
  (`GIZCLAW_ASSISTANT_BASE_URL`, `GIZCLAW_ASSISTANT_API_KEY`,
  `GIZCLAW_ASSISTANT_MODEL`), giving each scenario three attempts, and judges
  only tool calls, the final route and required facts. A small model varies
  between runs; three attempts separate that variance from a behavior the
  assistant cannot reach at all. The Go e2e
  `TestAssistantScenariosWithLiveModel` runs it with the Docker stack's
  `doubao-mini-chat`.
