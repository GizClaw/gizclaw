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
| `knowledge.search`              | index of the bundled guides           | `search_knowledge`         |

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

## Context and history

`createAssistant` takes a saved `history` and resumes from it; `history()`
returns a copy to save after each turn. The package does not store anything:
the console persists it.

Before each turn `compactHistory` keeps the history within `contextTokens`
(default 32000) minus the instructions and tool descriptions. Tokens are
estimated without a tokenizer, one per CJK character and one per four other
characters, erring high. Over budget, tool results of every turn but the last
are cut to 1500 characters, then those of the last turn. If that is not
enough, a quarter of the budget is reserved for a summary and the most recent
turns that fit in the rest (at most `keepTurns`, default 4) are kept; the other
turns and any earlier summary go to a summarizer agent on the same model. The
history becomes one system item starting with `【较早对话的摘要】`, cut to its
reserve, followed by the kept turns verbatim, so it fits beside the incoming
message unless that message alone exceeds the budget. The turn reports what was
done as `compaction`.

## Knowledge

`createKnowledgeIndex` builds a BM25 index (k1 1.2, b 0.75) over Markdown
documents cut into sections by heading, at most 700 characters each. The
tokenizer needs no dictionary: CJK runs become overlapping character pairs,
other runs lowercase words, and identifiers joined by `_`, `.` or `-` also
yield their parts. The knowledge base is the project's zh guides and nothing
else: `guideDocuments` turns guide files keyed by their path under `guides/`
into documents titled by their first heading and linked to
`https://gizclaw.github.io/gizclaw/`, skipping the pages the site excludes.
The console bundles the guides at build time; `testing/guides.ts` reads them
from the repository for `FakeRuntime` and the tests, so scenarios search the
same text the console ships.

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
