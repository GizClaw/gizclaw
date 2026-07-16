# Coding Conventions

GizClaw is a Go-first, but not Go-only, repository. Source code, schemas, generated SDKs, C bindings, the Wails frontend, and documentation together form the product contract. Changes must preserve every boundary they cross.

## Choose the applicable convention

| Scope of changes | Coding standards |
| --- | --- |
| Go packages, services, concurrency and lifecycle | [Go](./go) |
| JavaScript, TypeScript, SDK and front-end | [JavaScript and TypeScript](./js) |
| Dart SDK and Flutter App | [Dart and Flutter](./dart-flutter) |
| C SDK, C binding and cgo bridge | [C and cgo](./c) |
| Guide, README, configuration instructions and architecture documentation | [Documentation](./docs) |

## General rules

### Determine ownership first

Place new code in the package or directory that owns the behavior. Do not move provider-specific logic, product resources, transport details, or persistence concerns into a general abstraction merely for convenience.

Keep public APIs minimal. Export types or functions only when external callers need them. Go doc and generated references describe public symbols, while the development guides explain module responsibilities and boundaries.

### Contract has only one source

OpenAPI, Protobuf, and other schemas are the source of truth for generated surfaces. When changing a contract, update the source schema, regenerate every committed output, and verify the affected Go, JavaScript, Dart, C, and application callers together. Generated files must not be maintained directly as source code.

### External input is not trusted

HTTP, RPC, event streams, configuration, firmware, SDK payloads, workflow inputs, and cross-language buffers must all be verified at their respective boundaries. There should be clear behavior for resolution failures, cancellation, timeouts, partial initialization, and connection closing.

### The life cycle must be closed

Code that creates a goroutine, stream, subscription, timer, file, network connection, native handle, or buffer must also define cancellation, shutdown, and failure cleanup paths. The creator does not have to close the resource, but closing ownership must be unique and clear.

### Testing follows risks

Tests verify observable behavior rather than mechanically pursuing one test per file. Use unit test first for pure logic; use integration or E2E across package, schema, network, storage and runtime boundaries; concurrent and long-running components need to cover cancellation, leakage and race risk.

## Minimum check before submission

- Format all modified source files.
- Run the build, test, or build command defined by the package to which the change belongs.
- Go behavior changes run `go test ./...` by default; only use a narrower scope if the change is truly local, and explain the reason.
- Contract changes regenerate and verify all affected language surfaces.
- Documentation and configuration changes run at least `git diff --check` and verify new links and commands.
- Do not commit secrets, credentials, logs, caches, temporary files, build products, or irrelevant changes.
