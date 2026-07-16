# Issue format

GizClaw Issue is the design and acceptance contract before implementation. Issues must allow developers who have not participated in the pre-discussion to confirm the boundaries of the issue, find the correct ownership, complete the implementation, and use clear evidence to determine whether it can be closed.

Issues only describe the current final state to be delivered. Don't write roadmaps, migration processes, or assumptions into existing behavior that aren't supported by code or documentation.

## Title

Use the title uniformly:

```text
prefix: Subject
```

Rules:

- `prefix` Use lowercase letters or numbers. You can use `/` to represent the module level. It cannot contain spaces, empty segments, or `/` at the beginning and end.
- Leave a space after the colon.
- `Subject` Describe the goal after completion, and do not use expressions such as "process it" or "optimize relevant logic" that cannot be judged independently.
- Title prefix and GitHub Issue Type are two independent fields. prefix represents the issue or module category, and Issue Type represents the organizational form of the work.

Example:

```text
bug: Fix token refresh race
firmware/ota: Add recovery status reporting
admin/ui: Add workspace editor
tracking: Split server mesh delivery
```

## GitHub Issue Type

The repository uses `Bug`, `Feature`, and `Task`:

| Issue Type | Usage Scenario | Content Requirements |
| --- | --- | --- |
| `Bug` | The existing behavior is inconsistent with the contract or expectations | Write down the current behavior, correct behavior and regression verification |
| `Feature` | Add user-visible capabilities or deliverables with specific implementation | Include complete design, change scope and acceptance criteria |
| `Task` | Only used for tracking/container Issue | Must have sub-issues, it does not have specific implementation design and acceptance details |

Do not automatically set it to `Task` just because the Issue has sub-issues. As long as the parent Issue still has specific code changes, implementation designs, or direct acceptance criteria, `Bug` or `Feature` should be selected based on the deliverable.

## Implement the body structure of Issue

Issues with implementation-specific scope must use the five top-level sections in the following order:

```markdown
## Background

## Goal

## Code Changes Tree

## Design

## Test And Acceptance Criteria
```

Chapter names and order remain consistent. You can add a third-level chapter that matches the task in `Design`, but do not create another chapter of the same level to distract the scope or acceptance conditions.

### Background

Write dependencies first, then describe the current status, cause of the problem, and necessary context.

Relationship fields use strict Markdown lists:

```markdown
## Background

- Parent: #123
- Prerequisite of:
  - #130
  - #131
- Follow up to:
  - #118

Current state and problem background.
```

Relationship meaning:

- `Parent`: Which tracking or larger deliverable the current Issue belongs to, only one inline Issue reference is allowed.
- `Prerequisite of`: Which Issues must wait for the current Issue, always use a nested list.
- `Follow up to`: Which existing issues the current Issue depends on, extends or cleans up, always use nested lists.

Do not write bare `Parent: #123`, `Related` or relationship lines with duplicate titles. Loose reference relationships are written into the background text; GitHub's native parent/sub-issue relationships still need to be set up separately in GitHub.

### Goal

`Goal` Describe the ownership and boundaries after completion:

- The behavior or output that the current Issue must produce;
- The module, contract or user path owned by the current Issue;
- Clarify irresponsible adjacent capabilities;
- Is it possible to close the Issue directly after completion?

If an Issue contains multiple targets that can be delivered and verified independently, it should be split into tracking Issues and sub-issues.

### Code Changes Tree

`Code Changes Tree` Use the real path in the repository to describe planned changes, without copying the entire repository directory:

```text
cmd/internal/commands/example/
└── command.go                 # CLI command and flags
pkgs/gizclaw/example/
├── service.go                 # domain behavior
└── service_test.go            # behavior and error coverage
guides/en/using/
└── example.md                 # user-facing final behavior
```

Rules:

- Read the `AGENTS.md` applicable to the root directory and target directory first, and then determine the ownership, test and build file locations.
- Only the sources, schemas, generated products, tests, configurations, scripts and documents owned by the current Issue are listed.
- Delete item tag `(delete)`; generate file tag that needs to be submitted `(generated, committed)`.
- Do not list build output, cache, download dependencies, logs, temporary files and irrelevant paths.
- If the correct path still cannot be confirmed from the repository and Guides, leave it as a design issue and don't fake the directory tree.

### Design

`Design` Write the contract that the implementation must abide by, rather than fixing every line of code in advance. Supplement according to the task:

- API, RPC, CLI, configuration, Schema or file format;
- Call path, state transition, ownership, asynchronous behavior and cleanup;
- Errors, timeouts, retries, cancellations, partial failures and unsupported behavior;
- Platform differences between Server, Edge, device, desktop, browser and each SDK;
- Third-party dependencies, provider boundaries and build processes;
- compatibility, migration, security and clear non-goals.

If there are multiple feasible solutions that will change the direction of the product or architecture, add `Open Design Questions` a third-level chapter and wait for a decision. Don't mark an Issue as ready with unsupported assumptions.

### Test And Acceptance Criteria

Each acceptance criterion must describe an observable and reproducible completion state, including:

- Which caller completes what action;
- What results must be met by the normal path, error path and compatible path;
- Which source contracts, generated surfaces, callers and documents need to be synchronized;
- What actual commands or tests were used to prove completion;
- Allowed `SKIP`, `UNSUPPORTED` or blocked evidence when Docker, network, credentials, provider account or hardware are required.

Go behavior changes include `go test ./...` by default; if a scoped equivalent is used, the Issue must explain why the scope is sufficient. Documentation, README or workflow-only changes contain at least:

```sh
git diff --check
```

Do not use acceptance conditions such as "Complete development", "Test passed", "Optimize code", etc. that cannot independently prove behavior.

## Complete example

The following example shows a `Feature` Issue with specific implementation scope. The Issue numbers in the examples are only used to illustrate the relational field format.

- Title: `guides: Document the repository Issue format`
- Issue Type: `Feature`

<<< ./examples/issue-example.md

## Tracking Issue

`Task` tracking Issue only maintains delivery boundaries, sub-issue lists, dependencies and status, and does not directly have specific code trees, implementation designs or test details. The specific content is the responsibility of sub-issues.

If the repository template requires that the tracking issue still retains five chapters, these chapters can only describe sub-issue ownership and summary completion conditions, and cannot re-copy the implementation design of each sub-task.

## Ready check

After creating or updating an Issue, confirm each item:

- Title matches `prefix: Subject`.
- Issue Type is consistent with the work form, `Task` is only used for containers with sub-issues.
- The five chapters that implement the Issue exist and are in the correct order.
- Background relationship fields use strict Markdown lists and set native GitHub relationships synchronously.
- Goal clearly states ownership, deliverables and non-goals.
- Code Changes Tree from current repository structure and applicable `AGENTS.md`.
- Design covers relevant contracts, life cycles, errors, platforms and compatibility boundaries.
- Acceptance criteria observable and lists validation commands that match the risk.
- Does not contain secrets, credentials, tokens, private paths, temporary state, or descriptions that treat the roadmap as the current fact.
- Issues remain in an unready state while pending issues would change the implementation direction.

For the review method of Issue readiness, see [Issue Review](./issue_review).
