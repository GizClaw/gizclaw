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
