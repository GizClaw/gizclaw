# AGENTS Guide

## Always

- GizClaw is a Go-first repository that also owns OpenAPI/RPC schemas,
  generated SDKs, JavaScript packages, C bindings, documentation, GitHub
  Actions, and e2e harnesses.
- Inspect the current Issue, code, Schema, generated output, docs, and workflow
  before changing behavior. Keep the change inside the requested scope.
- Describe the final supported state. Do not present roadmap items, migration
  history, placeholders, fake output, dead registrations, or TODO-only behavior
  as complete.
- Do not commit secrets, credentials, logs, caches, build output, temporary
  files, or local workspace metadata. `openteam/` must remain ignored.
- Do not run Bazel checks. Use validation matched to the changed surface rather
  than a fixed repository-wide coverage percentage.

## How To Find The Rules

| When you need to | Read first |
| --- | --- |
| Create or update an Issue | [Issue format](guides/zh/reviewing/issue-format.md), then [Issue review](guides/zh/reviewing/issue_review.md) and the affected module guides |
| Review an Issue for implementation readiness | [Issue format](guides/zh/reviewing/issue-format.md), [Issue review](guides/zh/reviewing/issue_review.md), and the planned paths in [review items](guides/zh/reviewing/review_items.md) |
| Implement or change repository behavior | [developing index](guides/zh/developing/index.md), the path mapping in [review items](guides/zh/reviewing/review_items.md), and the matching [coding style](guides/zh/coding-styles/index.md) |
| Change HTTP, RPC, telemetry, generated code, or an SDK | The matching [API design guide](guides/zh/developing/api/overview.md), every affected consumer guide, and the applicable language rules |
| Change Go | [Go style](guides/zh/coding-styles/go.md) and the Go section of [review items](guides/zh/reviewing/review_items.md) |
| Change JavaScript or TypeScript | [JavaScript and TypeScript style](guides/zh/coding-styles/js.md) and the matching review items |
| Change Dart or Flutter | [Dart and Flutter style](guides/zh/coding-styles/dart-flutter.md) and the matching review items |
| Change C, cgo, or C-facing SDKs | [C and cgo style](guides/zh/coding-styles/c.md), plus Go rules for cgo and the matching API contract |
| Change docs, README, examples, or workflows | [Documentation style](guides/zh/coding-styles/docs.md) and the documentation/workflow rows in [review items](guides/zh/reviewing/review_items.md) |
| Perform self-review | [Self review](guides/zh/reviewing/self_review.md) and all applicable [review items](guides/zh/reviewing/review_items.md) |
| Review a PR | [PR Agent review](guides/zh/reviewing/pr_agent_review.md), [PR and commit format](guides/zh/reviewing/pr-commit-format.md), and all applicable [review items](guides/zh/reviewing/review_items.md) |
| Prepare a PR title or commit | [PR and commit format](guides/zh/reviewing/pr-commit-format.md) |
| Use the CLI, apps, or SDKs | [Using index](guides/zh/using/index.md) |
| Generate or browse API references | [References index](guides/references/index.md) |

When a change spans multiple rows, combine every applicable guide. A nested
`AGENTS.md` adds rules for its directory and does not replace this file.

## How To Validate

| Change | Minimum validation |
| --- | --- |
| Go behavior | `go test ./...`, unless a scoped equivalent is clearly justified |
| API or Schema | Regenerate committed outputs and test every affected language and consumer |
| Docs, README, or workflow only | `git diff --check` plus the relevant build, parser, link, or static check |
| Fuzz tests | Seeds pass under normal `go test`; use a bounded targeted fuzz campaign when needed |

Record the commands, results, skipped checks, and remaining risk in the final
response. GitHub Actions supplements local validation; it does not replace it.

## How To Review

- Verify scope and requirements before implementation details.
- Group the diff by changed ownership area, apply every matching guide, then
  check cross-module contracts and generated surfaces together.
- Report one complete blocking-finding pass. Each finding needs `P0`, `P1`, or
  `P2`, an exact file and line, the concrete risk, and an actionable fix.
- Remote reviewers do not apply fixes. When no blocking finding remains, state
  that the review passed and list validation that could not be confirmed.

## Repository Hygiene

- Keep unrelated changes out of the same commit and do not rewrite user changes.
- Keep `LICENSE`, `SECURITY.md`, and `.github/dependabot.yml` deliberate and current.
- Pin GitHub Actions dependencies by commit SHA.
- Treat firmware, config, RPC, SDK decoding, and workflow inputs as untrusted.
