# C SDK And C Binding Review Guide

Use this guide for the C SDK under `sdk/c/gizclaw`, generated C RPC code,
C-facing platform interfaces, and cgo bridge code that exposes Go behavior to C
or C behavior to Go.

This guide is for remote Codex reviewers. The reviewer does not apply fixes.
The goal is one complete review that catches every blocking issue in the
requested scope.

## Review Workflow

1. Map the diff by changed folder, public header, source file, generated output,
   cgo bridge, platform vtable, and ownership area.
2. Review each module independently with the checks below.
3. Review cross-module behavior after the module passes, especially RPC schema,
   generated C output, Go bridge code, JavaScript SDK surfaces, and validation
   boundaries.
4. Audit the review result against the full diff before submitting feedback so
   one review pass can surface all blocking findings.

Report blocking findings only: API/ABI breaks, memory unsafety, ownership
bugs, generated-code drift, invalid platform-vtable behavior, missing
validation, or coverage gaps that should be fixed before the work is healthy.

## Required Inputs

Before reviewing, identify:

- the C headers, sources, generated files, or cgo bridge files under review
- the schema, RPC, platform, or SDK contract that owns the behavior
- the compile, smoke, Go, or generator checks that should pass

If the change modifies generated files, review the generator and schema first.
Generated C output should be regenerated from source contracts, not hand-edited
as the source of truth.

## API And ABI Checks

- Preserve public header compatibility unless the task explicitly changes the
  C SDK contract.
- Review struct layout, enum values, typedef names, callback signatures, and
  exported function names for accidental ABI changes.
- Keep headers and sources synchronized, including declarations, includes,
  ownership comments, and error return semantics.
- Verify generated C RPC methods, request/response structs, encode/decode
  helpers, and method maps match `api/rpc.json` and generator behavior.
- Check that platform vtables remain coherent: required callbacks are checked,
  userdata is threaded correctly, and default platform fallbacks are explicit.

## Memory And Ownership Checks

- Define ownership for every input, output, borrowed pointer, callback pointer,
  and returned buffer.
- Pair allocations with the matching platform allocator/free callback.
- Check all null pointers before dereference at public API and callback
  boundaries.
- Validate lengths before pointer arithmetic, allocation, copy, decode, encode,
  or string construction.
- Check integer conversions between signed types, unsigned types, `size_t`, Go
  lengths, enum values, and wire-format widths.
- Prevent overflow in capacity growth, frame length calculations, and encoded
  buffer sizes.
- Ensure buffers remain valid across callbacks only when the API explicitly
  guarantees that lifetime.
- Avoid storing pointers to stack memory, temporary Go memory, or caller-owned
  buffers beyond their valid lifetime.

## Error, Cleanup, And State Checks

- Verify every fallible allocation, platform callback, crypto operation, HTTP
  call, encode/decode step, and transport operation has a checked result.
- Check cleanup paths for partially initialized structs and multi-step
  operations.
- Ensure reset/free functions are idempotent where callers can reasonably retry
  or clean up after partial failure.
- Confirm error returns do not leave stale output values that callers may treat
  as valid.
- Review state transitions for clients, signaling exchanges, WebRTC peers,
  channels, RPC frames, and telemetry streams.

## Cgo Bridge Checks

- Follow cgo pointer rules: do not let C keep Go pointers unless they are
  represented through `cgo.Handle` or another valid ownership boundary.
- Delete `cgo.Handle` values on every successful and failed lifecycle path.
- Check conversions between C buffers and Go slices for nil pointers, zero
  lengths, maximum lengths, and lifetime.
- Do not call back into Go after the owning backend, sink, peer, or channel has
  been closed.
- Keep C callback IDs and channel labels synchronized with the Go-side
  semantics they represent.

## Test Coverage

- Add unit coverage for pure encode/decode, frame, buffer, key, JSON, and
  signaling behavior when those areas change.
- Add smoke coverage for public C SDK flows when API shape, initialization,
  or platform-vtable behavior changes.
- Add generator golden tests when generated output changes.
- Use Go tests around cgo bridge code when the bridge is the easiest reliable
  validation boundary.
- Include malformed input, boundary length, allocation failure, null pointer,
  and partial-cleanup cases when they are relevant to the changed behavior.

## Validation

Choose validation that proves the touched C surface:

- generator tests or regeneration checks for `tools/gzc-rpcgen` changes
- focused Go tests for packages that compile or exercise C/cgo bridge code
- a C smoke test or compile check for changed translation units when available
- `git diff --check` for documentation-only or workflow-only changes

Do not claim a C SDK change is verified only because unrelated Go tests pass.
If the repo lacks a direct C build command for the touched area, state the
closest executed check and the remaining gap.

## Review Output

For each blocking finding, include severity, file and line, the unsafe behavior
or contract mismatch, and the concrete fix direction. Prefer concrete ownership,
ABI, and validation findings over broad C style advice.

Before submitting, re-read the findings as a reviewer of the review:

- confirm every changed header, source file, generated file, and cgo bridge was
  considered
- confirm validation and test coverage claims match the changed behavior
- confirm ABI, ownership, generated-code, and cross-language boundaries are
  covered when present
- remove duplicate or non-blocking comments
- ensure every remaining finding explains the defect, risk, and fix direction
