# MCP client and resource cache

Go API reference: [`pkgs/mcp`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/mcp).

`pkgs/mcp` provides a concurrent Streamable HTTP client backed by the official
Go MCP SDK. It supports tool discovery and invocation, resource and URI-template
discovery, and resource reads with text, binary contents, and metadata. Protocol
values use the SDK's own types. The package has no dependency on RuntimeProfile,
Workspace, GenX, or Eino.

## Connection ownership

`Dial` explicitly connects and negotiates capabilities. Each client represents
one endpoint and authorization context. Headers are copied, redirects are
rejected, and request deadlines and response limits cover protocol operations.
List operations collect all pages with bounded pagination. Unsupported server
capabilities return an error. Tool execution errors retain the MCP result's
`IsError` flag; protocol and transport errors are Go errors. Tool call results
are never cached by this package, and calls are not automatically retried here.

The owner closes the client after its consumers stop. Closing cancels outstanding
operations and releases the MCP session. HTTP transports supplied by the caller
remain caller-owned. Canceling the context used to dial after connection does not
close the session; each operation has its own context.

## Shared local resources

Create one `ResourceCache` for workflows sharing a client's authorization
context. Construction performs no I/O and starts no workers. `Refresh` discovers
the catalogs, preloads listed resources, and refreshes concrete template URIs
previously read through the cache. Resource requests use bounded concurrency.

A successful refresh publishes a complete local version atomically. A failed
refresh retains the previous version. Resources removed from the catalog are
removed from the new version. MCP does not provide a remote transaction across
resource reads, so local publication does not imply that the remote contents
were captured simultaneously. The official SDK may satisfy protocol requests
from its server-directed TTL cache.

Cached reads perform no network I/O and remain available during refresh failures
or after the client is closed. A first read of an uncached URI loads and caches
its body. Concurrent misses for that URI share the network request; canceling
one waiter does not cancel other waiters. Shared requests are bounded by the
client's timeout and terminate when the client is closed. Failed loads are not
cached. Resource count and serialized byte limits bound the retained cache.

Acquire a `Snapshot` once per workflow turn to keep the same local version across
multiple reads. Snapshot methods never fetch missing data and return
`ErrNotCached` on a miss. Returned SDK values are independent copies. Retaining
old snapshots also retains their data, so consumers should release them when the
turn ends.

Cache hits do not expire or refresh synchronously. The owner must call `Refresh`
or start and join the blocking `Run` method with its own lifecycle context.
`Run` refreshes immediately and on a timer. A supplied error callback receives
refresh failures while the worker keeps the last good version and retries on
the next tick. Without a callback, the first error stops the worker. Cancellation
or client closure stops it. This cache uses polling rather than resource
subscriptions or notification-triggered refresh.

## Validation

Run from the repository root:

```sh
go test -race ./pkgs/mcp
```

The local HTTP fixtures exercise legacy stateful MCP and current stateless MCP,
pagination, authenticated requests, tool errors, binary resource contents,
cancellation, shutdown, snapshot publication, cache limits, and shared reads.
