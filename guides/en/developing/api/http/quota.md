# Quota HTTP service

GizClaw calls the external HTTP implementation selected by RuntimeProfile to authorize Peer provider calls. The service owns billing and quota conversion.

## Contract

`api/http/quota.json` owns the wire contract and generates `sdk/go/quota`.

Authenticated `POST /v1/quota` uses `Authorization: Bearer <api_key>`. Its request carries `peer_public_key`, optional complete shared `DeviceIdentifiers` (SN, all IMEIs and labels), and retained hourly `usage`. Identifiers are sent as already known; quota adds no identifier acquisition RPC.

Each usage item contains the provider billing `model_id`, exact UTC `hour`, and nonnegative cumulative `quantity`. Receivers retain the maximum quantity for each Peer/model/hour, so replay does not debit the same usage again. Initial reports can contain an empty array. GizClaw flushes usage before reading SQL and does not fabricate an empty report when storage is unavailable.

```json
{
  "expires_at": "2030-01-01T00:00:00Z",
  "valid_until": "2029-12-01T00:00:00Z"
}
```

- Optional nullable `expires_at` is the usable deadline. Omitted/null means unlimited; an elapsed timestamp denies use.
- Required non-null `valid_until` is this decision's lifetime and must be a future timestamp when received. Missing, malformed or elapsed validity never authorizes use.

## RuntimeProfile

```yaml
spec:
  quota:
    endpoint: https://quota.example.com/v1/quota
    api_key: ${QUOTA_API_KEY}
```

Quota configuration is required on resource writes. The complete HTTP(S) endpoint must contain no userinfo, query or fragment; the key must be nonempty and contain no newlines. Existing SQL resources retain their original data; an old unconfigured profile cannot authorize provider calls and must be updated by its administrator. Configure `services.peer_usage.store` so reports can read retained SQL usage.

## Runtime behavior

Real generator, transformer and speech calls acquire authorization before provider I/O. Connected Peers, Workspace owners and detached OpenAI HTTP requests use the corresponding Peer binding.

The Server refreshes at the midpoint of decision validity, including denied and unlimited decisions. Attempts are bounded to five seconds and failed attempts retry after one second. A transient failure preserves a still-valid old decision but extends neither deadline. Expiry, denial or stale decision validity cancels active provider calls; a successful renewal can extend an active call without canceling it early. Idle state is reclaimed after five minutes. Shutdown cancels/joins quota workers before closing usage pools.

If standalone speech synthesis loses authorization after sending metadata, it ends audio with early EOS. Already delivered audio can be partial; subsequent calls return permission denied. Denial before metadata returns permission denied directly.

This is time authorization, without per-token reservations or numeric hard caps. Provider-unreported or unflushed volatile usage retains the boundaries documented in [Peer usage](/en/developing/gizclaw/services/runtime/peerusage).

## Docker acceptance

Run `bash tests/gizclaw-e2e/setup/run-quota.sh` for real Linux GizClaw, fixture and Giztest containers. The local HTTP/WebSocket provider fixture emits deterministic OpenAI/MiniMax usage and captures quota reports. This acceptance does not claim live cloud-provider qualification.
