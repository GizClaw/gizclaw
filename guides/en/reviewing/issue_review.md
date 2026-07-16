# Issue review

Issue review is used to confirm that the issue has been described to the extent that it can be independently implemented and verified by developers. The title, Issue Type, five paragraphs of text, and relationship fields must first conform to [Issue format](./issue-format); this page focuses on product or architecture boundaries and does not lock the implementation into unverified code solutions in advance.

## Problem definition

- The title accurately describes a problem or deliverable to be solved.
- Contextual description of current behavior, desired behavior, and why change is needed.
- Key terms, roles, and provider/consumer orientations are consistent with code and existing contracts.
- Do not write speculation, future roadmap, or adjacent issues as current facts.

## Scope

- Make it clear what results this Issue has and what it is not responsible for.
- When multiple independent deliverables are involved, use tracking Issues to separate them from sub-issues; the parent Issue retains product boundaries, and the specific implementation is borne by the child Issue.
- If there are dependencies on other Issues, PRs or Contracts, clearly indicate the direction and sequence of dependencies.
- Do not introduce roles, configuration fields, taxonomy or abstractions that have not yet been established in the repository for the sake of "completeness".

## Acceptance criteria

Each acceptance criterion should describe an observable, verifiable completion state, such as:

- Which caller can complete what behavior;
- Which error, cancellation or compatibility paths must be established;
- Which Schemas, generated surfaces or documents must be synchronized;
- What test or command is used to demonstrate completion.

Avoid using expressions such as "complete refactoring", "optimize code", "support related functions", etc. that cannot be determined independently.

## Enforcement boundaries

- Indicate the primary ownership directory, source contract, and affected consumers.
- If the implementation method has been determined by the architecture, describe the boundaries that must be adhered to; otherwise, only describe the constraints and do not forge detailed code in the Issue.
- Breaking changes, migrations, generated code and cross-language effects must be listed explicitly.
- When there are risks to security, resource life cycle, persistence and untrusted input, they should enter the acceptance scope.

## Verification plan

The Issue should describe verification that matches the risk:

- Applicable boundaries of unit, integration, E2E, fuzz, race or smoke test;
- Schema regeneration and all language consumer checks;
- Static validation of documents, configurations or workflows;
- Who and where verification is performed that cannot be performed in the current environment.

## Review conclusion

An Issue is considered ready only when the problem, scope, acceptance criteria, ownership and verification method are clear enough. Ambiguities and errors in the documentation should be corrected directly after review; if decisions are still missing that would change the direction of the implementation, the issue should be left open and should not be pretended to be implementable.
