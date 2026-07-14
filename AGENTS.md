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

## Documentation Map

- Start with `guides/zh/developing/index.md` for architecture, API design,
  package ownership, CLI, applications, and SDK development.
- Use `guides/zh/reviewing/index.md` and
  `guides/zh/reviewing/review_items.md` for review workflow and the mapping from
  changed paths to applicable documentation.
- Use `guides/zh/coding-styles/index.md` for language and documentation
  conventions.
- Use `guides/zh/using/index.md` for CLI, application, and SDK usage.
- Use `guides/references/index.md` for Go references and local Flutter/TypeScript
  reference generation.

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

## Review Requirements

- These guides are for remote Codex reviewers. Reviewers should not apply fixes;
  they should produce one complete, actionable review.
- Treat each review as a blocking-finding pass over the requested scope. Report
  concrete defects, regression risks, missing validation, or missing coverage;
  do not substitute optional style polish for requirement or behavior checks.
- Start by grouping the diff by changed folder or ownership area, then review
  each module with the matching guide before checking cross-module contracts.
- Before submitting review feedback, audit the findings against the full diff:
  confirm every changed folder, generated surface, validation claim, and
  language boundary has been checked, merge duplicates, and ensure each issue
  has severity, file/line evidence, and an actionable fix direction.
- For Go changes, follow `guides/zh/coding-styles/go.md` and the Go section of
  `guides/zh/reviewing/review_items.md`.
- For JavaScript and TypeScript changes, follow
  `guides/zh/coding-styles/js.md` and the matching review items.
- For Dart and Flutter changes, follow
  `guides/zh/coding-styles/dart-flutter.md` and the matching review items.
- For C SDK and C-facing binding changes, follow
  `guides/zh/coding-styles/c.md` and the matching review items.
- For documentation, README, issue text, and workflow-only changes, follow
  `guides/zh/coding-styles/docs.md` and the documentation review items.
- If a change crosses Go, JavaScript, C, schema, or generated-code boundaries,
  verify the source schema, generated outputs, language-specific call sites, and
  tests together rather than reviewing each surface in isolation.

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
