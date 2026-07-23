# Documentation

This specification applies to Guides, README, configuration instructions, architecture descriptions, examples, and workflow documentation.

## Content boundaries

- The document describes the final form and does not record the migration process that does not affect use and compatibility.
- Implemented actions, future work, open issues, and non-goals must be separated; the roadmap cannot be written as current capabilities.
- Development guidelines explain project composition, directory responsibilities, module boundaries, and request paths; the signature and annotation of Go symbols are handled by Go doc or Reference.
- User description, internal design, API Reference and issue acceptance criteria are intended for different readers and should not be mixed.
- Current issues are only recorded in GitHub Issues, and stable documents do not reference temporary issue numbers or refactoring statuses.

## Source of facts

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
- Module documents are organized according to capabilities and responsibilities, and file names are not copied mechanically; the corresponding files, main structures and functions can be listed when positioning and implementation are required.
- Independent Go packages provide a Go API Reference link at the top of the page; modules within the same package directly put public symbol links into "Core Structure and Main Function".
- Avoid copying the complete contract on multiple pages. Each fact should have a clear owner, with links and necessary summaries to other pages.
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
