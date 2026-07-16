# PR Agent review

PR Agent review targets remote Codex reviewers. Reviewer only submits a complete and executable review result once and does not directly modify the PR code.

## Input

Read before you begin:

- PR’s base/head and full diff;
- PR title and review-ready commit history;
- Associate Issues, design documents and acceptance criteria;
- `AGENTS.md` for the repository and related directories;
- Change the [coding specification](../coding-styles/) corresponding to the language;
- Testing and verification results declared by the author.

PR titles and commit history are checked according to [PR and Commit format](./pr-commit-format) by default. The wording and formatting of the PR body are not part of the default metadata checks, but the scope, associated Issue and validation statements still need to be read.

## Review Process

### 1. Check PR and Commit format

Verify that the PR title accurately covers the final diff, that the title, content, and boundaries of each retained commit meet formatting requirements, and that there are no interim commits in the review-ready history.

### 2. Group by ownership

First list all changed folders, packages, Schemas, generated surfaces and language boundaries. Use corresponding rules for review module by module, and do not skip smaller directories because a major module passes.

### 3. Demand first, then implement

First determine whether the PR has completed the correct issue and whether it has exceeded the scope, and then check the logic, errors, life cycle and code structure. When the requirements are not established, polishing the implementation details should not continue to be the main feedback.

### 4. Check cross-module Contract

When changes span Schema, Go, JavaScript, Dart/Flutter, or C, treat the source contract, makefiles, callers, and tests as a whole. Missing on either side may be blocking finding.

### 5. Verification

The verification command must prove that the surface was modified. Just because the unrelated Go test passes, it does not mean that C, Flutter, generated SDK or front-end behavior has been verified.

### 6. Review review results

Check the complete diff again before submitting:

- Whether all changed folders have been overwritten;
- Whether findings are repeated;
- Whether each item actually blocks health completion;
- Whether the severity, file, line number, risk and repair direction are complete;
- Whether generated code and cross-language consumers are missing.

## Pass conditions

When there is no blocking finding, the clear output passes and explains:

- Review of covered modules;
- PR title and review-ready commits have met the format requirements;
- Confirmed validation;
- Verification and residual risk that cannot be independently run or confirmed.

Reviewers should not give a passing conclusion because they “don’t have time to continue reading”.
