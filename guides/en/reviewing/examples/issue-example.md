## Background

- Parent: #123
- Prerequisite of:
  - #126
- Follow up to:
  - #118

Issue writing requirements are currently scattered among Agent instructions, review documents, and existing Issues. Developers cannot confirm the title, Issue Type, body structure, relationship fields, and acceptance requirements from a single entry, resulting in inconsistent information completeness in different Issues.

## Goal

Add a unified Issue format document to the review guidelines so that developers and Agents can use the same contract when creating or reviewing Issues.

This Issue is responsible for:

- Define the title, Issue Type and five-paragraph body structure of the Issue;
- Define the boundary between tracking Issue and implementation Issue;
- Provides a complete Markdown example loaded directly by VitePress.

This issue does not modify the GitHub Issue template, CLI, API, or product behavior.

## Code Changes Tree

```text
guides/.vitepress/
└── config.mts                              # sidebar and example exclusion
guides/en/reviewing/
├── issue-format.md                         # canonical Issue format
├── issue_review.md                         # readiness review entrypoint
└── examples/
    └── issue-example.md                    # complete Markdown example
```

## Design

- Place the Issue format in the "Review Guidelines" and before "Issue Review" in the sidebar.
- Use the `prefix: Subject` title format and keep the GitHub Issue Type and title prefix independent of each other.
- Implement Issue fixed use of five top-level chapters `Background`, `Goal`, `Code Changes Tree`, `Design` and `Test And Acceptance Criteria`.
- The examples are saved in independent Markdown files and loaded by VitePress code snippet syntax; the example files do not generate independent documentation pages.
- The Issue review document only describes the readiness review method, reuses the format contract through links, and does not copy the rules.

## Test And Acceptance Criteria

- The "Review Guidelines" sidebar contains "Issue Format" and is located before "Issue Review".
- Issue format page shows full Markdown example, syntax highlighting and copy button.
- After modifying the sample file, the VitePress page will display the new content simultaneously, without copying the sample text in the format document.
- The sample files do not generate independently accessible documentation pages.
- The following verifications passed:

```sh
git diff --check
npm --prefix guides run build
```
