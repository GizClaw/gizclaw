# Documentation Review Guide

Use this guide for docs, README, issue text, workflow-only changes, examples,
configuration references, and architecture notes in GizClaw.

This guide is for remote Codex reviewers. The reviewer does not apply fixes.
The goal is one complete review that catches every blocking issue in the
requested scope.

## Review Workflow

1. Map the diff by changed folder, document family, audience, workflow, schema,
   and ownership area.
2. Review each document or workflow module independently with the checks below.
3. Review cross-module behavior after the module passes, especially claims about
   code, schemas, generated files, GitHub workflows, SDKs, and operational
   commands.
4. Audit the review result against the full diff before submitting feedback so
   one review pass can surface all blocking findings.

Report blocking findings only: inaccurate claims, missing acceptance details,
broken commands or links, stale generated-contract descriptions, security
misguidance, or validation gaps that should be fixed before the docs are
healthy.

## Required Inputs

Before reviewing, identify:

- the document type and intended audience
- the code, schema, workflow, issue, or operational behavior the text describes
- the authoritative source that proves each claim
- the focused validation that should pass for the changed files

Do not review docs as standalone prose when they describe live behavior. Verify
the claims against the current repository, generated outputs, scripts, schemas,
or GitHub issue/PR state.

## Accuracy Checks

- Reject docs that present roadmap, planned work, examples, or TODOs as shipped
  behavior.
- Verify command snippets, paths, package names, service names, endpoint names,
  config keys, environment variables, ports, and generated filenames.
- Check that API, RPC, schema, SDK, C binding, and event-stream descriptions
  match the source contract and generated surfaces.
- Keep terminology consistent with `docs/terms.md`, service-layout docs, and
  existing API names.
- Confirm examples use realistic values without committing secrets, local
  credentials, tokens, private paths, or machine-specific state.
- Check diagrams, tables, checklists, and matrices for stale labels, missing
  rows, impossible states, or misleading completion status.

## Scope And Structure Checks

- Keep docs tied to the requested issue, design, or review scope.
- Prefer final-state documentation over migration-history narration unless the
  history is required for operation or compatibility.
- Separate implemented behavior from future work, open questions, and
  non-goals.
- Keep public user-facing docs, internal design notes, API references, and issue
  acceptance criteria distinct when their audiences differ.
- Avoid duplicating long contract details in multiple places unless there is a
  clear owner for future updates.
- If a doc introduces a new taxonomy, verify that it matches the code and does
  not blur boundaries such as public service surfaces, peer services, event
  streams, SDK packages, and local runtime internals.

## Security And Operations Checks

- Treat firmware artifacts, config parsers, RPC framing, SDK decoders, workflow
  inputs, and credentials as untrusted boundaries.
- Require docs to explain safe defaults, cleanup steps, and failure modes when
  the workflow involves services, generated files, credentials, firmware,
  network listeners, or persistent local state.
- Verify GitHub Actions guidance keeps dependencies pinned by commit SHA when
  it edits workflow dependencies.
- Check that docs do not recommend broad cleanup, destructive commands, or
  credential storage without a clear safety boundary.

## Validation

Use focused validation for documentation changes:

- `git diff --check`
- Markdown link or formatting checks when the repo has such tooling
- YAML parsing or workflow/static checks for `.github/workflows` changes
- command smoke checks when a changed doc adds or edits commands
- regeneration checks when docs describe generated API, SDK, or C surfaces

If a command or live workflow cannot be run, state the reason and the remaining
risk. Do not treat proofreading as sufficient validation for docs that describe
executable steps or live contracts.

## Review Output

For each blocking finding, include severity, file and line, the unsupported or
incorrect claim, the authoritative source that contradicts it when available,
and the concrete fix direction.

Do not block on wording preferences alone. Do block on ambiguity when it can
make implementers validate the wrong thing, run the wrong command, expose a
secret, misunderstand a contract, or close an issue before the actual
acceptance criteria are met.

Before submitting, re-read the findings as a reviewer of the review:

- confirm every changed document, workflow, command, schema claim, and generated
  surface description was considered
- confirm validation claims match the changed text
- confirm code, API, issue, and operational sources were checked when the docs
  describe live behavior
- remove duplicate or non-blocking comments
- ensure every remaining finding explains the defect, risk, and fix direction
