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

Local Docker E2E applies Admin resources once before testing. For an already
deployed target, provision resources first and set `GIZCLAW_TEST_ENDPOINT` and
`GIZCLAW_TEST_REGISTRATION_TOKEN`; the command has no Admin authority.
Interactive `review.*` scenarios require an attached terminal and
`--parallel 1`.
