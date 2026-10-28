# Quota HTTP service

Quota applies time-based authorization obtained by background queries to provider calls. The external quota service owns billing rules and conversions; GizClaw reports existing usage and enforces known usable deadlines.

## Contract

[quota.json](https://github.com/GizClaw/gizclaw/blob/main/api/http/quota.json) defines HTTP requests and responses, with a generated [Go quota client](https://github.com/GizClaw/gizclaw/tree/main/sdk/go/quota). The service receives known device identifiers and persisted usage snapshots; GizClaw does not collect additional device information for quota.

Distinguish device eligibility expiry from the freshness deadline of a query result. Reporting the same cumulative snapshot should not charge twice. Consult the Schema for fields and processing rules.

## RuntimeProfile

Minimal external-authorization configuration:

```yaml
spec:
  quota:
    type: custom
    endpoint: https://quota.example.com/v1/quota
    api_key: ${QUOTA_API_KEY}
```

A custom policy needs Peer usage SQL storage to report actual persisted consumption. Use an unlimited policy when external authorization is unnecessary; it does not call the quota service. See [RuntimeProfile quota](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/system/runtimeprofile/quota.go) for configuration parsing.

## Runtime behavior

Authorization covers connected Peers, Workspace owners, and independent OpenAI calls. Provider calls use the latest local decision without waiting for hook network requests or usage reporting; Server queries and refreshes in the background. An unknown initial decision allows use. Decisions are process-local; restart or idle-state reclamation starts again with an unknown decision.

`valid_until` only schedules background refresh. Stale results, hook timeouts, HTTP errors, invalid responses, and usage-report failures do not revoke authorization or interrupt active calls. Failures retain the latest decision while background retries continue, with WARN logs rate-limited per Peer/policy. Logs contain only the Peer identity and a safe failure reason, excluding endpoints, credentials, response bodies, and underlying provider errors.

Only an explicit `expires_at` usable deadline can reject new calls and cancel affected provider calls, including a previously returned future deadline that later elapses. A known expiry remains enforced through stale results and refresh failures until a successful response extends it or grants unlimited use. Without an explicit usable deadline, use continues even if refresh remains unavailable.

Stop quota workers before closing their usage storage. Refresh, cancellation, and cache cleanup live in [peerquota](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerquota).

Hook requests for all Peers and policies reuse one service-owned HTTP client and keep-alive connection pool. Each worker retains its SDK client, with authentication headers still set by its own policy. Success, HTTP errors, and JSON parsing failures read and close response bodies so reusable connections return to the pool. Response sizes and request deadlines remain bounded, and redirects are not followed. Service shutdown cancels and joins workers before closing idle connections.

## Client errors

Explicit expiry still returns quota-exhausted errors. Hook failures only produce WARN logs and do not return quota-unavailable errors to clients. Adapters still map configuration and service-lifecycle errors to their corresponding statuses and messages; internal endpoints, credentials, and error chains must not reach callers.

Revocation after streaming begins can leave partial output. Use the protocol's terminal result to determine completion. Error definitions live in [errors.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerquota/errors.go).

Quota does not guarantee per-token reservation or exact billing; [Peer usage](../../gizclaw/services/runtime/peerusage) durability limits still apply.

## Docker acceptance

```sh
bash tests/gizclaw-e2e/setup/run-quota.sh
```

This entrypoint uses isolated HTTP and provider fixtures through real Server, Edge, and SDK paths to verify authorization, refusal, expiry, and active-call revocation. Giztests hold a controllable hook response to prove initial and subsequent calls do not wait for authorization, continue calling after a real HTTP timeout, and retain stale decisions through refresh failures, including explicit denials. The Docker runner correlates fixture failures with Server WARN logs for the same Peer, verifies safe error classification and rate limiting, and confirms HTTP connection reuse across Peers using connection addresses observed by the fixture. [The test entrypoint](https://github.com/GizClaw/gizclaw/blob/main/tests/gizclaw-e2e/setup/run-quota.sh) owns scenarios and assertions; it is not cloud-provider qualification.
