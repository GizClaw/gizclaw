# Documentation

This specification applies to Guides, README, configuration instructions, architecture descriptions, examples, and workflow documentation.

## Content boundaries

- The document describes the final form and does not record the migration process that does not affect use and compatibility.
- Implemented actions, future work, open issues, and non-goals must be separated; the roadmap cannot be written as current capabilities.
- Development guidelines explain project composition, directory responsibilities, module boundaries, and request paths; the signature and annotation of Go symbols are handled by Go doc or Reference.
- User description, internal design, API Reference and issue acceptance criteria are intended for different readers and should not be mixed.
- Track temporary problems and refactoring progress in a PR or Issue; keep that process and temporary Issue numbers out of stable documentation.

## Source of facts

- Code, schemas, scripts, and configuration are the single source of truth (SSoT) for implemented behavior. Documentation explains purpose, module boundaries, interactions, and usage; it does not define another behavior contract.
- Do not hand-maintain function inventories, type definitions, field tables, enum values, defaults, validation limits, route registries, or SQL schemas. Link to source, Go doc, or source-generated references. Usage examples show only what the operation needs, without copying complete definitions.
- When documentation disagrees with the implementation, correct it against current source. Keep unimplemented requirements in a PR or Issue.
- Check the current code, Schema, generated files, scripts and configurations before describing the implementation.
- endpoint, method, package, directory, config key, environment variable, port and command must be consistent with the repository.
- The terminology uses formal names already in the code and contract, and does not create new roles or abstractions for ease of explanation.
- Figures, tables and directory trees are also statements of fact and must be consistent with the text and source code.
- Examples must not contain secrets, credentials, tokens, private paths, or machine-specific state.

## Organization

- The root `README.md` is the repository's only first-party README. Stable
  documentation for modules, SDKs, Apps, examples, test harnesses, and tools
  belongs in the corresponding `guides/` page instead of a second README in a
  subdirectory. Upstream-owned README files inside third-party submodules are
  exempt.
- The directory entry with sub-pages is only responsible for navigation, and specific instructions are placed on the "Overview" page.
- Organize module documentation by capability and responsibility. Link to source or API references rather than restating each file or function.
- Put a Go API Reference or source link at the top of an independent package page. Modules within one package link to their implementation files.
- Link to the owning module's explanation instead of copying complete paragraphs across pages.
- Mermaid diagrams are used only when relationships, call sequences, or state changes are clearer than text; entity numbers and directions should remain readable.

## Commands and Security

- Executable commands must be able to run in the specified working directory and environment.
- Describe safe defaults, failure modes, and cleanup methods when involving services, credentials, firmware, listeners, or persistent state.
- Deletion commands without scope restrictions are not provided; irreversible operations must specify the scope of impact.

## Verify

Documentation changes run at least:

```sh
git diff --check
```

VitePress content should also execute the build commands defined in `guides/` and check for new routes, intrasite links, tables, and Mermaid. When the commands or live contracts in the document cannot be verified, the unverified parts and remaining risks should be clearly stated.
