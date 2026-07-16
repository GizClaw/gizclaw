# Post-development self-review

Self-review occurs after implementation is completed and before remote review is submitted or requested. It's not a quick scan of the diff, it's a `review → fix → verify → re-review` closed loop.

## 1. Re-read requirements

Go back to the Issue, design document or user request and confirm each item:

- Which acceptance criteria are covered by the implementation;
- What content is clearly not in this scope;
- Whether the API, catalog, or behavioral assumptions were changed during development;
- Whether the documentation already reflects the final implementation.

If the implementation deviates from the requirements, the implementation should be corrected or the approved requirements should be updated first. PR reviewers should not be allowed to guess the final goal.

## 2. Create a modified map

Group by changed folder and ownership instead of reading from the first diff all the way to the end:

```text
Schema / Contract
Generated surfaces
Go services and packages
SDK and applications
Tests and fixtures
Guides and workflows
```

Mark all consumers of each source contract to ensure that the generated code and callers are not missed.

## 3. Module-by-module inspection and repair

Use [review items](./review_items) and corresponding [coding specifications](../coding-styles/) for each set of changes. Fix the problem directly after discovering it, and add tests that can prove that the problem will not return.

Priority checks:

- Correctness, failure paths and boundary values;
- ownership, cancellation, closure and partial cleanup;
- public API and package boundary;
- Consistency between Contract and generated files;
- Untrusted input and cross-language conversion.

## 4. Verification

Execute the command that best proves the change is correct, rather than just running the command that is easiest to pass.

- Go behavior change runs `go test ./...` by default.
- Concurrent changes are subject to increased risk `go test -race`.
- Schema changes are regenerated before running all affected SDK and caller tests.
- JavaScript, Dart/Flutter, C and Wails use the build/test defined for each package.
- Documentation runs at least `git diff --check` and the corresponding site build.

Document the command, results, and verification and reasons why it did not run.

## 5. Fresh review

After the repair is completed, start the review from the complete diff again. Do not just look at the lines that were just repaired. This process is repeated until a new round no longer produces new blocking findings.

Confirm before ending:

- Every changed folder has been reviewed;
- Each Contract consumer has been checked;
- Match test coverage with risk of change;
- There are no temporary files, debug code and irrelevant modifications in diff;
- Verification results correspond to current final code, not earlier versions.
