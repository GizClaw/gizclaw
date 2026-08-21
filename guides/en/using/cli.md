# CLI

## Validate declarative Resources offline

Use `admin validate` to check one declarative Resource or one `ResourceList` before applying it:

```sh
gizclaw admin validate -f resource.yaml
gizclaw admin validate -f resource.json
printf '%s\n' '{"apiVersion":"gizclaw.admin/v1alpha1","kind":"ResourceList","spec":{"items":[]}}' \
  | gizclaw admin validate -f -
```

File inputs support `.json`, `.yaml`, and `.yml`. `-f -` reads JSON from stdin. The command performs the same `${VAR}` and `${VAR:-default}` expansion and accepts the same generated `KindResource` compatibility aliases as `admin apply`.

A valid concrete Resource exits with status `0` and writes one compact JSON object followed by a newline:

```json
{"valid":true,"kind":"Credential","id":"openai-main"}
```

A valid list reports its item count without printing any item specs:

```json
{"valid":true,"kind":"ResourceList","items":3}
```

Invalid input exits non-zero and reports the input plus value-redacted JSON Pointer diagnostics. The command never prints Resource spec values or expanded environment values, so Credential Resources can be checked in CI without exposing their body.

Validation is completely offline: it does not read a GizClaw context, contact Server, or mutate storage. Passing means that the expanded document matches the Resource OpenAPI schema embedded in that binary and can be decoded as its declared kind. It does not prove that referenced IDs exist, credentials authenticate, provider/body combinations pass Server business rules, dependencies are reachable, or the Resource can be applied or run successfully.

## Run Giztest

`test validate` recursively validates strict `*.giztest.yaml` documents offline.
`test run` connects to the endpoint declared by each file. The complete
selection must validate before any ephemeral identity or remote operation is
created:

```sh
gizclaw test validate -f tests/gizclaw-e2e/giztest
gizclaw test run tests/gizclaw-e2e/giztest --parallel 10 --output report.json
```

YAML `repeat` is the task count for that file. `--parallel` is the maximum
worker count shared by all files. Directory discovery is recursive and stable.
Every task owns isolated clients, variables, and cleanup. `save_as` assigns a
declared in-memory output variable; file Save As is unsupported.
For repeated voice benchmarks, `speech.cache: run` may cache a successful saved
synthesis fixture for that document and resolved request. Each task receives a
separate byte copy; the cache is bounded by the declared output `max_bytes` and
is discarded when the command exits.

Step `expect` maps JSON Pointers to expectation objects. One expectation object
may combine several matchers; the step passes only when every matcher passes:

| Matcher | Operand | Semantics |
| --- | --- | --- |
| `equals` | any non-null value | JSON equality |
| `present` | boolean | pointer resolves (or, with `false`, does not) |
| `non_empty` | boolean | value is a non-empty string, array, or object |
| `count` | integer ≥ 0 | array length equals the operand |
| `contains` | non-empty string, ≤ 256 runes | string target contains the substring |
| `contains_all` | 1–16 such strings | every listed substring occurs |
| `contains_any` | 1–16 such strings | at least one listed substring occurs |
| `not_contains` | one such string or 1–16 of them | no listed substring occurs |
| `pattern` | RE2 source, 1–256 bytes | string target matches the pattern |
| `minimum` / `maximum` | number | numeric target (JSON number, or a decimal string such as a protojson int64) is within the inclusive bound |
| `min_length` / `max_length` | integer 0–1048576 | string target's rune count is within the bound |

String matchers accept a string value or an array whose elements are all
strings; an array is joined with the empty separator first, so `peer_stream`
`/text` fragments are asserted as one logical response. Lengths count Unicode
runes, not bytes. `minimum`/`maximum` fit numeric fields such as `peer_stream`
`/first_text_ms`. Validation rejects, offline and before any connection, a
non-compiling `pattern`, `min_length` above `max_length`, `minimum` above
`maximum`, and `present: false` combined with any value matcher. Failed content
matchers report only the pointer and matcher name — never the asserted text —
so redacted reports stay free of response content.

A `workspace_relay` step connects two declared clients' selected Workspaces as
one bounded conversation. Both named clients must be distinct and must each
have an earlier `server.run.workspace.set` step; validation rejects anything
else offline. `first_client` receives `input`; its assistant output is
forwarded incrementally — fragment by fragment for `media: text`, arrival-paced
Opus packet by packet for `media: audio` — as user input for the other side
under a fresh receiving-side stream ID, then ownership alternates after each
completed assistant response. `max_turns` (2–256) counts completed assistant
responses across both sides, and `terminal_client` must match the parity of
`first_client` and `max_turns`; the terminal response is captured without being
forwarded again. `media: audio` requires both Workspaces to accept push-to-talk
input; continuous realtime relay is unsupported.

```yaml
- id: run_test_dialogue
  workspace_relay:
    first_client: tester
    second_client: candidate
    input: "${test_brief}"
    media: text
    max_turns: 15
    terminal_client: tester
  capture:
    verdict: /terminal/text
  expect:
    /terminal/client: {equals: tester}
    /terminal/text: {equals: PASS}
    /completed_turns: {equals: 15}
    /turns/candidate/first_text_ms/max: {maximum: 6000}
    /turns/candidate/text_runes/min: {minimum: 15}
```

The relay result exposes `completed_turns`, `terminal.client`,
`terminal.text` (`media: text`), `turns.<client>.count`, and per-turn
`{min, max}` aggregates — `first_text_ms`/`text_runes` for text,
`first_audio_ms`/`audio_bytes` for audio — plus aggregate event and byte
counts. `capture` may assign only `/terminal/text` to a string output variable
or, for audio, `/terminal/audio` to an `audio/ogg` Opus output variable.
Fixed v1 safety limits — 4,096 received events per completed turn, 1 MiB
joined text, and 16 MiB audio per relay — fail the relay when exceeded and
expose no tuning fields; the event limit is per turn because voice-enabled
Workspaces stream hundreds of Opus packets per response. A self-start
reply emitted by a Workspace before its first relay turn is consumed and
discarded (its `interrupted` marker is benign); once a side has held a turn,
any output on the relayed media (text for `media: text`, audio for
`media: audio`) while the other side is active fails the relay as turn
mixing; the other channel of a voice Workspace — for example TTS audio that
trails its own completed text turn — is consumed and counted only. Failures
name the responsible client and turn index. Reports carry only the client-name
attribution, counts, timings, and byte aggregates: relayed text, prompts,
tester reasoning, and audio bytes never appear in report evidence.

Local Docker E2E applies Admin resources once before testing. For an already
deployed target, provision resources first and set `GIZCLAW_TEST_ENDPOINT` and
`GIZCLAW_TEST_REGISTRATION_TOKEN`; the command has no Admin authority.
Interactive `review.*` scenarios require an attached terminal and
`--parallel 1`.
