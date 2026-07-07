# AGENTS Guide

## Repository Context

- GizClaw is an out-of-the-box agent runtime and edge server mesh for GizClaw
  devices.
- This is a Go-first repository, not a Go-only repository. The repo also owns
  OpenAPI/RPC schemas, generated SDK surfaces, JavaScript packages, C-facing
  bindings, Wails desktop code, documentation, GitHub Actions, and e2e
  harnesses.
- The project does not use Bazel.
- GitHub Actions CI is present, but local review and validation still matter.

## Work Style

- Inspect the current code, docs, issue, or workflow before changing behavior.
- Keep changes tied to the requested issue, design, or review scope.
- Do not present roadmap items as completed product features.
- Prefer final-state docs and issue text over migration-history narration.
- Mainline work is issue-driven local development. PR metadata checks are not
  required unless the task explicitly asks for PR review or PR cleanup.

## Validation

- For Go behavior changes, run `go test ./...` unless a scoped equivalent is
  clearly justified by the change.
- For API or schema changes, regenerate committed generated code and run the
  relevant Go, JavaScript, C, or e2e checks for the changed contract.
- For docs, README, and workflow-only changes, use focused validation such as
  `git diff --check`, YAML parsing, and workflow/static checks.
- For fuzz-test work, fuzz seeds must pass under normal `go test`; targeted
  fuzz campaigns can use `go test -run=^$ -fuzz=Fuzz -fuzztime=...`.
- Record the validation commands and results in the final response.

## Review Policy

- Verify scope and requirement compliance before reviewing implementation
  details.
- Check logic correctness, edge cases, error handling, cleanup behavior, and API
  compatibility.
- Reject placeholder implementations, fake outputs, dead registrations, and
  TODO-only behavior.
- Check dependency hygiene and avoid unintended external-provider coupling.
- When code changes generated API surfaces, verify generated files are fresh and
  consistent with source schemas.
- For tests, prefer coverage that matches risk and ownership. Avoid mechanical
  test-file splitting rules when fuzz, integration, e2e, or generated-code
  coverage is the better fit.

## Security And Dependencies

- Do not commit secrets, local credentials, build artifacts, logs, or temporary
  files.
- Keep `LICENSE`, `SECURITY.md`, and `.github/dependabot.yml` deliberate and
  current.
- GitHub Actions dependencies should remain pinned by commit SHA.
- Dependabot configuration should cover GitHub Actions, Go modules, npm
  packages, and maintained submodules in this repo.
- Treat firmware artifacts, config parsers, RPC framing, SDK decoders, and
  workflow inputs as untrusted boundaries.

## Commit Hygiene

- Keep unrelated changes out of the same commit.
- Use clear `{module}: {subject}` commit titles, for example `repo: update
  security metadata` or `giznet: tighten webrtc stream cleanup`.
- Do not rewrite or revert user changes unless explicitly requested.
- `openteam/` is local workspace metadata and must remain ignored.

## Reviewer Output Expectations

Review feedback should include:

- clear pass/fail status;
- file and line references for each issue;
- actionable fix guidance;
- priority levels such as `P0`, `P1`, or `P2`.

## Non-Goals

- No Bazel checks.
- No PR title or PR body checks unless the active task explicitly uses a PR.
- No fixed repository-wide coverage percentage gate for every change; use
  change-specific risk and validation instead.
