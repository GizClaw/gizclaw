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

For manual debugging of one voice scenario, use the non-concurrent `test play`
mode:

```sh
gizclaw test play tests/gizclaw-e2e/giztest/voice.giztest.yaml
gizclaw test play -o ./play-record tests/gizclaw-e2e/giztest/voice.giztest.yaml
```

The command accepts one regular file whose document has `repeat: 1` and no
barrier, and always uses one worker. It emits a short cue that is not recorded,
then starts the test immediately. In conversation order, it plays the user Opus
audio uploaded by `peer_stream` and audio `workspace_relay` steps, followed by
the assistant Opus audio actually received. Playback streams after a 200 ms
audio buffer reaches its start watermark; later packets are decoded to 16 kHz
mono PCM as they arrive and an independent playback task writes them to
PortAudio. User EOS enqueues its tail without delaying the outbound request;
assistant EOS flushes its tail and waits for the queued utterance before the
next speaker. Shutdown closes PortAudio first so a blocked native write cannot
hang the playback task. The summary reports milliseconds
from the end of the cue to first downlink receipt and playback, plus per-utterance
packet audio duration and arrival-gap timing. `peer_stream` also exposes these
metrics under `/audio_pacing`, so normal Giztest expectations can enforce the
packet count, encoded audio clock, mean/P95/maximum interval, and cumulative
drift.
By default, play mode only streams to the audio device and writes no files.
To preserve a run, `-o` / `--output` must name a new directory. The committed record contains a
redacted `report.json` and, when audio arrived, `audio.ogg` containing the full
user-and-assistant conversation in playback order; an empty Ogg is not
invented when no audio arrived. Execution or playback failures still preserve
the failed report and any bounded audio already received when the record can be
committed, then return non-zero.

Play requires cgo, libopus, and a PortAudio native runtime supported on the
current platform; it checks these before creating a remote client. The audio
in the record directory is real response content explicitly persisted by the
operator and must be treated as sensitive. Normal `test run` never opens an
audio device or writes an additional audio file.

`--evidence redacted` is the default. `--evidence full` requires `--output`
and writes bounded `workspace_relay` per-turn and terminal text into that JSON
report without printing it to the terminal. Treat a full-evidence report as
sensitive; it still excludes inputs, expanded variables, credentials, IDs, and
audio payloads, but model or tester text may contain private content.

YAML `repeat` is the task count for that file. `--parallel` is the maximum
worker count shared by all files. Directory discovery is recursive and stable.
Every task owns isolated clients, variables, and cleanup. `save_as` assigns a
declared in-memory output variable; file Save As is unsupported.
A `speech` step that feeds `peer_stream` requests `audio/ogg`; both Volc and
MiniMax voices deliver Ogg/Opus for it (MiniMax output is transcoded on the
Server), so translation documents do not depend on the Workflow's provider.
For repeated voice benchmarks, `speech.cache: run` may cache a successful saved
synthesis fixture for that document and resolved request. Each task receives a
separate byte copy; the cache is bounded by the declared output `max_bytes` and
is discarded when the command exits.

A `server.speech.transcribe` step takes its audio format from the referenced
typed variable. The runner accepts Ogg/Opus, which it decodes to 16 kHz mono
PCM, or matching `pcm_s16le` input for direct upload. It rejects other audio
formats before opening the RPC and sets `content_type` for the prepared bytes;
the document request contains the model and optional language, not this
runner-owned wire metadata.

A latency-only `peer_stream` probe can stop after the first assistant text and
audio instead of waiting for terminal output:

```yaml
- id: deployment_response_probe
  client: peer
  peer_stream:
    mode: push-to-talk
    input: ${turn_audio}
    pacing: 20ms
    completion: first_response
    first_text_timeout: 2s
    first_audio_timeout: 3s
  expect:
    /events: {non_empty: true}
    /first_text_ms: {maximum: 2000}
    /first_audio_ms: {maximum: 3000}
```

Both deadlines begin after the complete input turn has been pushed. Once both
first-response events arrive, the runner closes that logical stream immediately;
text/audio EOS and the remainder of the response are outside this probe.

A text-only Workflow disables the absent audio modality and declares only its
text deadline:

```yaml
- id: text_only_response_probe
  client: peer
  peer_stream:
    mode: realtime
    input: ${turn_audio}
    pacing: 20ms
    completion: first_response
    first_text_timeout: 2s
    require_text: true
    require_audio: false
  expect:
    /text: {non_empty: true}
    /first_text_ms: {maximum: 2000}
```

An `rpc_stream` step for `all.speed_test.run` exposes these stable result paths
to `expect`, `capture`, and an object `save_as`. The same canonical measurement
fields, plus `method`, appear in redacted step evidence:

| Path | Unit and meaning |
| --- | --- |
| `/up_content_length`, `/down_content_length` | Requested and acknowledged bytes for each direction. |
| `/up_bytes`, `/down_bytes` | Bytes actually transferred in each direction. |
| `/up_duration_ms`, `/down_duration_ms` | Each direction's measured duration, truncated to integer milliseconds. |
| `/duration_ms` | Whole-call wall time, truncated to integer milliseconds. |
| `/up_mbps`, `/down_mbps` | Measured megabits per second as finite non-negative numbers. |
| `/bytes` | Received bytes; for this operation it equals `/down_bytes`. |

An omitted direction keeps explicit numeric zero values. PascalCase paths
`/UpContentLength`, `/DownContentLength`, `/UpBytes`, `/DownBytes`,
`/UpDuration`, `/DownDuration`, and `/Duration` remain compatibility aliases;
new documents must use the canonical paths above. The three legacy duration
aliases retain raw nanoseconds.

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

`equals`, `contains`, `contains_all`, `contains_any`, and `not_contains` may opt
into deterministic normalization on their expectation object:

```yaml
expect:
  /text:
    contains_all: [四点, G7105]
    normalize: [whitespace, punctuation, case, digits]
```

The options remove Unicode whitespace (`unicode.IsSpace`), remove Unicode
punctuation (`unicode.IsPunct`, but not symbols or emoji), lowercase with Go's
Unicode mapping, and map full-width digits plus
`零一二三四五六七八九` one-for-one to ASCII digits. They do not parse Chinese
quantities such as `十` or apply locale-specific case folding. Fragment arrays
are joined before normalization, and the same selected rules apply to the
target and every operand. Configuration order does not change the result.
`pattern` and rune-length matchers continue to inspect the original joined
text. Normalization is opt-in and cannot be declared without one of the five
affected matchers; byte-exact matching remains the default.

Remote steps may opt into bounded retries:

```yaml
- id: translated_turn
  client: peer
  timeout: 2m
  retry:
    attempts: 3
    on: [timeout, assertion]
    delay: 5s
  peer_stream:
    mode: text
    input: Translate this sentence.
```

`attempts` is the total count from 2 through 10. `on` defaults to `[timeout]`
and may also include `assertion`; `delay` defaults to zero and, when present,
must be a positive Go duration no greater than five minutes. Retry is available
only for `rpc`, `rpc_stream`, `speech`, `peer_stream`, and `workspace_relay` in
the ordinary `steps` list. Finalizers, `client_rpc`, `barrier`, `output`, and
interactive review steps cannot retry.

Each attempt gets a fresh step timeout, while the task timeout and caller
cancellation bound all attempts and delays. Timeout classification requires a
wrapped `context.DeadlineExceeded`; matching error text alone is not enough.
Assertion failures include `expect` and `expect_error` mismatches. Other
operation, resolution, capture, and cancellation failures stop immediately.
Failed attempts do not commit `save_as` or captures; only the successful
attempt publishes all outputs to later steps.

Retry reuses the same clients, variables, Workspace, and remote state. It does
not reconnect, reset history, rerun earlier steps, or roll back RPC/provider
side effects, so scenario authors must opt in only when repeating that step is
valid. A retrying step retains its existing top-level final status, error, and
evidence and adds ordered `attempts` records with the attempt number, status,
duration, safe evidence, and a failure kind of `timeout`, `assertion`,
`operation`, or `cancelled`. The top-level duration includes retry delays.
Steps without `retry` keep the existing report shape and omit `attempts`.

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
forwarded again. Optional `terminal_media` defaults to `media`. A text relay may
set `terminal_media: audio` to forward text while completing each turn on Opus
audio EOS; an audio relay must terminate on audio to avoid truncating packets.
Optional positive Go-duration `idle_timeout` starts after the initial input and
each handoff, resets only on non-discarded active-side progress, and fails a
silent turn with its active client and one-based turn. Step and document
timeouts remain absolute bounds. `media: audio` requires both Workspaces to
accept push-to-talk input and forwards only Opus media (`audio/opus`, or `audio/ogg` with
`codecs=opus`); any other assistant audio MIME type or codec from the active
side fails the relay, and continuous realtime relay is unsupported.

```yaml
- id: run_test_dialogue
  workspace_relay:
    first_client: tester
    second_client: candidate
    input: "${test_brief}"
    media: text
    terminal_media: audio
    idle_timeout: 90s
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
`terminal.text` whenever the terminal response produced text,
`turns.<client>.texts`, `turns.<client>.count`, and per-turn
`{min, max}` aggregates — `first_text_ms`/`text_runes` for text,
`first_audio_ms`/`audio_bytes` for audio — plus aggregate event and byte
counts. `capture` may assign `/terminal/text` to a string output variable for
either relay media or, for audio, `/terminal/audio` to an `audio/ogg` Opus
output variable. Text observed during an audio relay is retained for assertions
and capture but is not forwarded alongside audio as duplicate user input.
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
name the responsible client and turn index. Idle failures additionally report
`deadline`, `idle_timeout_ms`, active client/turn, last event time, and observed
media without content. Default reports carry only attribution, counts, timings,
and byte aggregates. Full evidence explicitly adds the bounded per-turn and
terminal relay text; neither mode includes inputs, secrets, IDs, or audio payloads.

Local Docker E2E applies Admin resources once before testing. For an already
deployed target, provision resources first and set `GIZCLAW_TEST_ENDPOINT` and
`GIZCLAW_TEST_REGISTRATION_TOKEN`; the command has no Admin authority.
Interactive `review.*` scenarios require an attached terminal and
`--parallel 1`.
