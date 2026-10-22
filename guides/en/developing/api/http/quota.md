# Quota HTTP service

Quota obtains time-based authorization before provider calls. The external quota service owns billing rules and conversions; GizClaw reports existing usage and enforces the decision.

## Contract

[quota.json](https://github.com/GizClaw/gizclaw/blob/main/api/http/quota.json) defines HTTP requests and responses, with a generated [Go quota client](https://github.com/GizClaw/gizclaw/tree/main/sdk/go/quota). The service receives known device identifiers and persisted usage snapshots; GizClaw does not collect additional device information for quota.

Distinguish device eligibility expiry from the cache lifetime of an authorization result. Reporting the same cumulative snapshot should not charge twice. Consult the Schema for fields and processing rules.

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

Authorization covers connected Peers, Workspace owners, and independent OpenAI calls. Server refreshes cached decisions and cancels affected provider calls when authorization expires. A refresh failure cannot extend an old decision.

Stop quota workers before closing their usage storage. Refresh, cancellation, and cache cleanup live in [peerquota](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerquota).

## Client errors

Distinguish quota refusal from inability to obtain a valid authorization so clients can explain the result or retry later. Adapters own surface-specific statuses and messages; internal endpoints, credentials, and error chains must not reach callers.

Revocation after streaming begins can leave partial output. Use the protocol's terminal result to determine completion. Error definitions live in [errors.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerquota/errors.go).

Quota does not guarantee per-token reservation or exact billing; [Peer usage](../../gizclaw/services/runtime/peerusage) durability limits still apply.

## Docker acceptance

```sh
bash tests/gizclaw-e2e/setup/run-quota.sh
```

This entrypoint uses isolated HTTP and provider fixtures through real Server, Edge, and SDK paths to verify authorization, refusal, expiry, and active-call revocation. [The test entrypoint](https://github.com/GizClaw/gizclaw/blob/main/tests/gizclaw-e2e/setup/run-quota.sh) owns scenarios and assertions; it is not cloud-provider qualification.
