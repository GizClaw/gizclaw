# Go Review Guide

Use this guide for remote Codex review of Go changes in GizClaw. The reviewer
does not apply fixes. The goal is one complete review that catches every
blocking issue in the requested scope.

## Review Workflow

1. Map the diff by changed folder, package, generated surface, and ownership
   area.
2. Review each module independently with the checks below.
3. Review cross-module behavior after the module passes, especially schema,
   generated code, API, SDK, C binding, and validation boundaries.
4. Audit the review result against the full diff before submitting feedback so
   one review pass can surface all blocking findings.

## Required Inputs

Before reviewing, identify:

- the code, diff, files, or branch under review
- the relevant issue, task notes, docs, schema, or explicit requirements
- the focused validation commands that should pass

If the user gave a review scope, follow that scope. If no scope is specified,
start with the PR diff, then infer module ownership from the changed folders and
repository context.

## Review Sources

Review Go changes against:

- repository code and surrounding package context
- task requirements, issue text, docs, comments, and module READMEs
- `AGENTS.md` and this guide
- Effective Go style expectations for naming, control flow, package design,
  comments, errors, concurrency, and readability
- the test architecture expected for the changed behavior

## Blocking Findings

Report only issues that should block healthy completion:

- meaningful API, package-boundary, lifecycle, or concurrency defects
- logic errors, edge-case misses, state-transition bugs, or regressions
- missing error propagation or insufficient error context
- missing cancellation, goroutine cleanup, channel ownership, timer/ticker
  cleanup, or resource release
- generated API surfaces that are stale relative to schemas or source contracts
- weak or missing tests for important boundaries, failure paths, or regressions
- validation that failed, was skipped without justification, or is too narrow
- maintainability issues with clear bug risk or long-term cost
- inappropriate Git content such as unrelated files, build artifacts, secrets,
  generated caches, accidental binaries, broken symlinks, or bad submodule/LFS
  changes

Do not block on optional style polish unless it creates real correctness,
clarity, compatibility, or maintenance risk.

## Go-Specific Checks

- Run or require `gofmt`/`go fmt` for touched Go files.
- Check package names, exported/unexported names, getter/setter names,
  interface names, and MixedCaps conventions.
- Keep package boundaries clear and exported symbols minimal.
- Verify exported package, type, function, and method comments where public API
  is added or changed.
- Prefer simple control flow, early returns, and clear short-variable reuse.
- Check that functions have a single coherent responsibility and clear
  parameter/return semantics.
- Validate `defer` usage for correctness, ordering, and cost.
- Review slice, map, append, copy, capacity, and zero-value behavior.
- Avoid hidden side effects in constructors and `init`.
- Verify pointer vs value receiver choices and method-set implications.
- Prefer interfaces at the consumer boundary and avoid premature abstraction.
- Check type assertions, type switches, and conversions for safe failure paths.
- Require comments for necessary side-effect imports.
- Review embedding for accidental overexposure or ambiguous promoted members.
- For concurrency, verify goroutine lifetime, cancellation, channel direction,
  channel closing ownership, race risk, blocking risk, and leak behavior.
- Reserve `panic` for truly exceptional conditions and keep `recover` at a
  sensible boundary.

## Test Coverage

Choose test coverage by risk and ownership:

- Unit tests should cover pure logic, boundaries, failure cases, and regression
  cases at the smallest useful package boundary.
- Prefer table-driven tests and subtests when they make coverage clearer.
- Tests should validate observable behavior instead of private implementation
  details.
- Use integration tests when behavior crosses package, filesystem, HTTP, RPC,
  database, message, timeout, cancellation, retry, or serialization boundaries.
- Use real dependencies or test servers when mocks cannot prove correctness.
- Consider `go test -race` when touched code uses goroutines, channels, shared
  state, callbacks, or long-lived workers.
- Consider benchmarks for hot paths, serialization, allocation-sensitive code,
  batching, or concurrency bottlenecks when performance is part of the change.
- For long-running workers, streams, watchers, servers, timers, contexts, and
  connection lifecycles, require leak or soak-style coverage when normal unit
  tests cannot expose lifecycle failures.

## Validation

For Go behavior changes, default to:

```sh
go test ./...
```

A scoped equivalent is acceptable only when the change is genuinely local and
the scope is explained. Schema or generated-surface changes must also include
regeneration plus the language-specific checks for every generated consumer.

## Review Output

Use concise findings with severity, file and line, and actionable guidance.
Before submitting, re-read the findings as a reviewer of the review:

- confirm every changed Go package and generated surface was considered
- confirm validation and test coverage claims match the changed behavior
- confirm cross-language or schema boundaries are covered when present
- remove duplicate or non-blocking comments
- ensure every remaining finding explains the defect, risk, and fix direction

If no blocking findings remain after that audit, state that the review found no
blocking issues and mention any validation that could not be independently
confirmed.
