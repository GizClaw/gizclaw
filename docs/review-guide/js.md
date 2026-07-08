# JavaScript And TypeScript Review Guide

Use this guide for JavaScript and TypeScript changes in GizClaw, including the
SDK packages under `sdk/js`, shared JS packages, Wails frontend code, generated
OpenAPI clients, and Playwright or node test harnesses.

This guide is for remote Codex reviewers. The reviewer does not apply fixes.
The goal is one complete review that catches every blocking issue in the
requested scope.

## Review Workflow

1. Map the diff by changed folder, package, generated client, frontend route,
   test harness, and ownership area.
2. Review each module independently with the checks below.
3. Review cross-module behavior after the module passes, especially OpenAPI/RPC
   schemas, generated clients, Go server contracts, C SDK surfaces, and runtime
   validation.
4. Audit the review result against the full diff before submitting feedback so
   one review pass can surface all blocking findings.

Report blocking findings only: defects, regressions, missing validation,
contract drift, unsafe runtime assumptions, or coverage gaps that should be
fixed before the work is healthy.

## Required Inputs

Before reviewing, identify:

- the package, app, generated client, or frontend area under review
- the schema, API, RPC, issue, or UI workflow requirements behind the change
- the package-local validation commands that should pass

If package scripts exist, prefer those over invented commands. For this repo,
use commands such as `npm --prefix sdk/js test`,
`npm --prefix sdk/js/gizclaw test`, or the relevant frontend build/test command
when the touched package owns one.

## Contract And Generated-Code Checks

- Verify OpenAPI/RPC schema changes are regenerated into committed TypeScript
  surfaces when required.
- Check generated files against their source schemas instead of hand-editing
  generated output.
- Review SDK method names, request/response types, cursor fields, streaming
  shapes, and error payload handling against the server contract.
- Confirm generated runtime helpers still match fetch/SSE/event-stream usage
  and browser/desktop runtime assumptions.
- If a contract crosses Go, JavaScript, C, or schema files, review the source
  schema, generated outputs, and call sites together.

## Runtime Behavior Checks

- Check async control flow for missing `await`, swallowed rejections,
  unhandled promise lifecycles, and lost errors.
- Verify cancellation, abort signals, timeouts, retries, and cleanup paths for
  long-running requests, streams, subscriptions, and UI effects.
- Review SSE, WebRTC, RPC, and event-stream parsing for partial messages,
  reconnect behavior, malformed payloads, ordering assumptions, and terminal
  states.
- Avoid trusting `any`, unchecked casts, JSON payloads, or generated raw fields
  without a boundary check when the data is external or schema-loose.
- Verify browser, Wails desktop, node, and test runtime differences before
  using globals, storage, crypto, URL, stream, or timer APIs.
- Check dependency changes for necessity, lockfile consistency, bundle impact,
  and unintended external-provider coupling.

## Frontend Checks

- Review loading, empty, error, success, stale-data, and permission-denied
  states for touched UI flows.
- Verify forms validate user input before sending RPC/API requests and keep
  server-side errors visible.
- Check list, detail, pagination, and cursor behavior against backend
  contracts.
- Ensure UI state does not present roadmap or placeholder behavior as a shipped
  feature.
- For generated clients in frontend code, prefer typed SDK boundaries over
  duplicated URL construction or ad hoc JSON parsing.
- Review accessibility and interaction basics when components change:
  keyboard reachability, focus behavior, labels, disabled states, and stable
  button semantics.

## Test Coverage

- Unit tests should cover pure transformations, parser behavior, SDK helpers,
  error handling, and regression cases.
- Component or integration tests should cover user-facing workflows when state,
  API calls, routing, or generated clients interact.
- E2E tests are appropriate when correctness depends on browser/Wails runtime,
  page navigation, streaming updates, or cross-service behavior.
- Add fixtures for malformed payloads, empty pages, pagination boundaries,
  aborted requests, and provider/server errors when those cases are part of the
  changed behavior.

## Validation

Choose the narrowest validation that proves the changed JavaScript or
TypeScript surface:

- `npm --prefix sdk/js test`
- `npm --prefix sdk/js/gizclaw test`
- the relevant frontend package build, test, or Playwright command
- regeneration commands such as `npm --prefix sdk/js run gen:sdk` when schemas
  or generated SDK files change

If validation is skipped because a package has no script or a dependency is
unavailable, state that explicitly and explain the remaining risk.

## Review Output

For each blocking finding, include severity, file and line, the failing
behavior or contract mismatch, and the concrete fix direction. Do not report
optional formatting preferences unless they hide a real correctness or
maintainability problem.

Before submitting, re-read the findings as a reviewer of the review:

- confirm every changed package, generated client, frontend route, and test
  surface was considered
- confirm validation and test coverage claims match the changed behavior
- confirm schema, server, SDK, and runtime boundaries are covered when present
- remove duplicate or non-blocking comments
- ensure every remaining finding explains the defect, risk, and fix direction
