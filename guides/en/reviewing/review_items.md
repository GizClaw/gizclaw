# Review Items

This document defines review items common to Issue reviews, post-development self-reviews, and PR Agent reviews. Each review must find applicable documents based on the change path and check them one by one; when an item is indeed not applicable, the object and reason should be explained, and the entire set of contents cannot be summarized as "checked."

## Find applicable documents

The review begins by building a set of rules in the following order:

1. Read the repository root directory `AGENTS.md` and confirm the repository-level mandatory rules.
2. List the paths planned to be modified by the Issue, or all changed directories in the implementation.
3. According to the "Path and Development Guidelines" table below, read the development documents corresponding to each ownership root.
4. Read the corresponding [coding specification](../coding-styles/) according to the file type.
5. When changing the API, RPC, generated code or SDK, read the documentation and implementation of the source contract, provider, adapter and all committed consumers at the same time.

When multiple ownership roots are involved in a review, all applicable documents must be merged. When issues, documents, schemas, and production code conflict with each other, the conflict should be clearly pointed out as a problem and you cannot choose a more convenient side as the basis for review.

### Paths and development guides

| Changed path or ownership | Required development guide | Key checks |
| --- | --- | --- |
| `api/http/**` | [HTTP API Overview](/en/developing/api/http/overview), the corresponding [Admin API](/en/developing/api/http/admin), [Public API](/en/developing/api/http/public) or [OpenAI Compatible](/en/developing/api/http/openai-compatible) | Surface ownership, shared/resource dependencies, generated results and server implementation |
| `api/proto/rpc/**` | [Proto API Overview](/en/developing/api/proto/overview), [Peer RPC](/en/developing/api/proto/rpc/overview) and corresponding provider-direction page | Method provider/consumer, message source of truth, generate surface |
| `api/proto/telemetry/**` | [Telemetry](/en/developing/api/proto/telemetry) | Telemetry contract, coding, code generation and consumer end |
| `pkgs/giznet/**` | [pkgs/giznet](/en/developing/giznet) | WebRTC, PeerConn, transport and connection lifecycle |
| `pkgs/gizedge/**` | [pkgs/gizedge](/en/developing/gizedge) | Device/Edge/Server signaling, reverse proxy and RPC routing |
| `pkgs/gizclaw/**` | [pkgs/gizclaw overview](/en/developing/gizclaw/overview) | Service layout, role visibility, root package assembly boundary |
| `pkgs/gizclaw/peer_*.go` | [Peer Overview](/en/developing/gizclaw/peer/overview) and the corresponding Management, Authorization, Connection or Services page | Peer identity, online connection, authorization and service surface |
| `pkgs/gizclaw/server*.go` | [Server Overview](/en/developing/gizclaw/server/overview) and corresponding module pages | Server assembly, HTTP surface, security policy and lifecycle |
| `pkgs/gizclaw/rpc*.go` | [RPC Overview](/en/developing/gizclaw/rpc/overview) and corresponding Client, Server or capability pages | Go RPC implementation and `api/proto/rpc` contract consistency |
| `pkgs/gizclaw/services/**` | [Services Overview](/en/developing/gizclaw/services/overview) and the corresponding domain page | Domain ownership, persistence boundaries and cross-service dependencies |
| `pkgs/gizclaw/api/**` | [Generated Go API](/en/developing/gizclaw/api) and corresponding `api/**` source contract | Generated file freshness; manual maintenance of generated surface is not allowed |
| `pkgs/gizclaw/contextstore/**` | [Context Store](/en/developing/gizclaw/contextstore) | Config context, type safety and call boundaries |
| `pkgs/gizclaw/customid/**` | [Custom ID](/en/developing/gizclaw/customid) | ID encoding, parsing and compatibility |
| `pkgs/genx/**` | [GenX Overview](/en/developing/genx/overview) and the corresponding Generators, Transformers, Segmentors, Profilers, Labelers or Match pages | Stream/EOS, interface, Mux, provider adapter and public pipeline boundaries |
| `pkgs/audio/**` | [Audio Overview](/en/developing/audio/overview) and the corresponding codec, PCM, resampler or voiceprint page | Frame/codec contract, sample format, buffer and real-time processing |
| `pkgs/store/**` | [Store Overview](/en/developing/stores/overview) and corresponding store page | Interface contract, specific backend constraints, persistence and concurrency semantics |
| `sdk/js/**` | Corresponds to [HTTP API](/en/developing/api/http/overview) or [Proto API](/en/developing/api/proto/overview), and [JavaScript and TypeScript](/en/coding-styles/js) | SDK surface, client generation, runtime differences and error handling |
| `sdk/flutter/**` | Corresponding API/Proto documentation, and [Dart and Flutter](/en/coding-styles/dart-flutter) | Dart SDK contract, WebRTC transport, Stream and generated message |
| `sdk/c/**` | Corresponds to [Peer RPC](/en/developing/api/proto/rpc/overview), and [C and cgo](/en/coding-styles/c) | C API/ABI, nanopb generated code, ownership and cgo bridge |
| `apps/gizclaw-app/**` | [Dart and Flutter](/en/coding-styles/dart-flutter) and the SDK/API documentation used by App | App and SDK boundaries, Widget lifecycle, platform behavior |
| `apps/wails/**` | [JavaScript and TypeScript](/en/coding-styles/js), [Go](/en/coding-styles/go) and API documentation used by App | Go bridge, frontend runtime, generate client and desktop lifecycle |
| `guides/**`, `README.md`, `AGENTS.md` | [Document Coding Specifications](/en/coding-styles/docs) | Final form, source of truth, links, navigation, commands and repetitions source of truth |
| `.github/**` | [Document Coding Specification](/en/coding-styles/docs) and repository root `AGENTS.md` | Workflow permissions, SHA pin, secret boundary and actual execution commands |
| `tests/**`, `examples/**` | All development documents and corresponding coding specifications of the module being tested or demonstrated | Testing whether to prove the production contract rather than establishing a second set of behaviors |

Paths in the table are matched by ownership and do not require that a subdirectory with the same name exists on the file system. For example, `peer_*.go`, `server*.go` and `rpc*.go` are currently in the same Go package, but the corresponding pages should still be read according to their actual module responsibilities during review.

### File types and coding conventions

| Changed file | Required guide |
| --- | --- |
| `*.go`, `go.mod`, `go.sum` | [Go](/en/coding-styles/go) |
| `*.js`, `*.jsx`, `*.ts`, `*.tsx`, JavaScript package/lockfile | [JavaScript and TypeScript](/en/coding-styles/js) |
| `*.dart`, `pubspec.yaml`, Flutter platform wiring | [Dart and Flutter](/en/coding-styles/dart-flutter) |
| `*.c`, `*.h`, nanopb output, cgo bridge | [C and cgo](/en/coding-styles/c); cgo reads simultaneously [Go](/en/coding-styles/go) |
| `*.md`, VitePress, README, configuration examples, workflow instructions | [Documentation](/en/coding-styles/docs) |
| OpenAPI, Protobuf, generation configuration | [Contract rules in coding specification overview](/en/coding-styles/) and all language specifications for generating consumers |

### Additional documentation across Contracts

| Modify content | Must also check |
| --- | --- |
| HTTP Schema or route | Corresponding to HTTP API page, Go server/peer implementation, generated Go API, JavaScript/Dart consumer |
| Peer RPC method/message | Corresponding to provider-direction page, `pkgs/gizclaw/rpc`, Go/Dart/C generated code and caller |
| Telemetry Schema | Telemetry document, generated message, sender and all receivers |
| WebRTC signaling/transport | Giznet, Gizedge, Public API, SDK transport and connection lifecycle testing |
| GenX Stream/EOS | GenX overview, specific Adapters, public stream processing, and all stream consumers |

## Requirements and Scope

- Whether the change solves an actual problem in the Issue, Design Document, or Explicit Request.
- Are acceptance criteria, error paths, compatibility requirements, or non-functional constraints missing?
- Whether to mix in refactorings, dependencies, makefiles or temporary artifacts that are not relevant to the current target.
- Does the document describe the final behavior rather than writing the plan, TODO or migration process as a completed capability?

## Modules and dependencies

- Whether the new code is located in a package or directory that has this behavior.
- Whether the public API is kept minimal and whether there are internal types that are leaked only for convenience of calling.
- Whether the generic abstraction has unexpected dependencies on specific providers, product resources, or UI.
- Whether constructor, registration process and `init` hide network, goroutine or global state side effects.
- Whether the dependency is necessary, and whether the lockfile, submodule and license-related files are consistent.

## Contract and generated code

- Whether OpenAPI, Protobuf or other Schema is still the only source of truth.
- Whether the generated code is regenerated from the current Schema rather than modified manually.
- Whether the method, message, field, enum, error and stream shape of Go, JavaScript, Dart and C SDK are consistent.
- Whether the provider/consumer directions of server, client, edge-node and device are consistent with the contract.
- Whether compatibility changes are explicitly required and cover all callers.

## Logic and status

- Are normal paths, null values, boundary values, repeated calls, out-of-order and failure status correct.
- Whether the error is propagated and contains sufficient context to locate the operation and object.
- Whether the state machine has a state where it is impossible to exit, skip verification, or complete repeatedly.
- Whether retry, timeout, reconnect and fallback are capped and do not mask eventual failure.
- placeholder, fake output, dead registration, and TODO-only behavior must not be implemented as completions.

## Life cycle and concurrency

- Whether goroutine, Future, stream, subscription, timer, connection, native handle and buffer have clear owners.
- Whether success, failure, cancel and partial initialization can all clean up resources.
- Whether there is a race, deadlock, blocking or leak in channel closing, callback, lock and shared state.
- Whether it is still possible to receive callbacks or update status after the UI is destroyed, the peer is disconnected, and the service is stopped.

## Input, security and platform boundaries

- Whether HTTP, RPC, event stream, configuration, firmware, workflow input and SDK payload are processed as untrusted input.
- Whether length, pointer, cast, JSON, enum, path and credential are verified at their respective boundaries.
- Whether to submit secrets, tokens, logs, caches, binaries, machine paths or temporary files.
- Whether GitHub Actions dependency continues to be pinned to commit SHA.

## Language specialization

### Go

- Whether package/API, error, slice/map, receiver, embedding and `defer` comply with [Go coding specifications](../coding-styles/go).
- Whether the handwritten cross-package API directly uses the defined type of the definer, and whether the true ownership of the type is hidden through alias, isomorphic wrapper, or simply renamed DTO.
- Whether the repository's own generator generates cross-package alias; if so, whether to repair it from the generator. Alias ​​or helper signatures directly generated by third-party generators are not subject to handwritten code problem reports, and must not be manually rewritten, maintained fork or add output normalizer just to meet the specifications. File names and build comments are not substitutes for build chain evidence.
- Whether the goroutine, context, channel, timer and connection life cycle are closed.
- Whether the concurrency risk requires `go test -race`, leak test or soak test.
- Whether to run `modernize ./...` and review the handwritten code diagnostics involved in this change; whether the existing diagnostics outside the scope are truthfully recorded, whether the repository's own generator output is returned to the generator for repair, and whether the third-party generated files remain unmodified manually.

### JavaScript and TypeScript

- Whether Promise is processed, and whether AbortSignal, subscription and effect are cleared.
- Whether the external payload bypasses type boundaries and whether the browser/Wails/Node runtime assumptions hold.
- Whether the UI overrides loading, empty, error, stale and permission denied.

### Dart and Flutter

- Whether Future, StreamSubscription, controller and peer connections are closed correctly.
- `BuildContext`/`setState` after `await` Check whether `mounted`.
- Whether the SDK and App ownership are separated, and whether the Widget copies the transport or contract implementation.

### C and cgo- Whether public header, ABI, struct layout, enum and callback signature are compatible.
- Whether pointer, buffer, length, allocator and ownership are explicitly safe.
- C Whether Go pointer is saved incorrectly, `cgo.Handle` Whether it is released in all paths.

### Documentation

- Whether endpoints, methods, packages, directories, configurations, and commands are supported by the current code or schema.
- Are diagrams, links, directory trees, and completion status accurate.
- Whether stable documents are mixed with temporary issues, unimplemented solutions or machine environments.

## Testing and Verification

- Test whether the change covers the true behavior, failure paths and regression risks, rather than just verifying the internal implementation.
- Whether integration/E2E is required for behavior across package, network, storage, schema or runtime.
- Whether the generated code runs regeneration check.
- Whether the validation command belongs to the actual modified package, and whether the result actually passes.
- Whether skipped validations explain the reasons and remaining risks.

## Git content

- Whether the PR title and review-ready commit history comply with [PR and Commit format](./pr-commit-format).
- No extraneous modifications, merge residue, broken symlinks or unexpected permission changes in the diff.
- LFS, submodule, lockfile and makefile changes have clear origins.
- `git diff --check` Passed, the commit boundary is consistent with module ownership.
