# Quota HTTP service

RuntimeProfile optionally selects a quota policy for Peer provider calls. The supported policies are unlimited and custom; only custom calls an external HTTP implementation, which owns billing and quota conversion.

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
    type: custom
    endpoint: https://quota.example.com/v1/quota
    api_key: ${QUOTA_API_KEY}
```

`spec.quota` is optional. Omission or null uses unlimited behavior, as does an explicit `quota: {type: unlimited}`. These policies perform no quota HTTP request and require neither a quota service nor SQL usage storage. Independently configured `services.peer_usage.store` continues to record provider-billed usage. An empty object or an unknown policy type is rejected.

`type: custom` requires both endpoint and api_key. The complete HTTP(S) endpoint must contain no userinfo, query or fragment; the key must be nonempty and contain no newlines. Configure `services.peer_usage.store` so custom reports can read retained SQL usage. Missing or failed reporting dependencies deny custom calls rather than sending fabricated empty usage. SQL stores absent policy as JSON null and explicit policies as tagged objects; existing unconfigured profiles remain unlimited.

## Runtime behavior

Real generator, transformer and speech calls acquire authorization before provider I/O. Connected Peers, Workspace owners and detached OpenAI HTTP requests use the corresponding Peer binding.

For custom policy, the Server refreshes at the midpoint of HTTP decision validity, including denied decisions and responses with omitted/null usable expiry. Attempts are bounded to five seconds and failed attempts retry after one second. A transient failure preserves a still-valid old decision but extends neither deadline. Expiry, denial or stale decision validity cancels active provider calls; a successful renewal can extend an active call without canceling it early. Idle state is reclaimed after five minutes. Shutdown cancels/joins quota workers before closing usage pools.

## Client errors

Quota failures retain types distinct from ordinary resource authorization errors. An elapsed usable deadline or an explicit denial in a valid decision is `QUOTA_EXHAUSTED`. No valid decision, elapsed decision validity, failed reporting dependencies, or an unavailable custom controller is `QUOTA_UNAVAILABLE`. A transient refresh failure preserves a still-valid old decision until its original deadlines.

| Surface | Exhausted | Unavailable |
| --- | --- | --- |
| HTTP chat / foreground Responses | HTTP 403, `permission_error`, `quota_exhausted` | HTTP 503, `service_unavailable_error`, `quota_unavailable` |
| Chat / speech / transcription SSE | Error object with the above code/type/message | Error object with the above code/type/message |
| Responses SSE / background polling | `response.failed` / failed Response, error code `quota_exhausted` | `response.failed` / failed Response, error code `quota_unavailable` |
| Device Workspace EOS | `EventError.code=QUOTA_EXHAUSTED`, `retryable=false` | `EventError.code=QUOTA_UNAVAILABLE`, `retryable=true` |
| Fresh speech RPC | `PERMISSION_DENIED` (7), `Reason=QUOTA_EXHAUSTED` | `UNAVAILABLE` (14), `Reason=QUOTA_UNAVAILABLE` |

Every exhaustion message is `Quota exhausted.`; every unavailability message is `Quota unavailable.`. Public payloads exclude endpoints, credentials, and internal error chains. SSE whose headers have already been sent retains HTTP 200 and terminates with a structured error. Eino error EOS belongs to the affected response StreamID and does not fail unrelated turns. Fatal runtime status messages retain the stable code and safe message.

Revocation during provider opening or `Generator.Invoke` retains the typed quota cause, closes late stream handles, and preserves reported provider usage. Failure of an active audio epoch synthesizes EOS with the same safe error contract. Identified quota errors take precedence over accompanying cancellation, while ordinary Workspace replacement retains error-free EOS.

If standalone speech synthesis loses authorization after metadata, it ends audio with early EOS without inserting a second protobuf response into binary audio. Delivered audio may be partial; the next fresh request uses the status and Reason above.

This is time authorization, without per-token reservations or numeric hard caps. Provider-unreported or unflushed volatile usage retains the boundaries documented in [Peer usage](/en/developing/gizclaw/services/runtime/peerusage).

## Docker acceptance

Run `bash tests/gizclaw-e2e/setup/run-quota.sh` for real Linux GizClaw, fixture and Giztest containers. The local HTTP/WebSocket provider fixture emits deterministic OpenAI/MiniMax usage and captures quota reports. Fifteen cases cover ten custom-policy deadline/reporting scenarios, omitted policy, explicit unlimited, exhausted/unavailable Eino dialogue, and one allowed HTTP Responses scenario. The unlimited case invokes real provider adapters and assert zero quota requests for its Peer. HTTP chat and Responses assert complete 403/503 envelopes. A fixture helper uses the real SDK PeerStream through Edge/Server to assert a fresh request after an allowed reply expires, EOS code/message/retryable, distinct response identity, and zero provider request increment during denial. This acceptance does not claim live cloud-provider qualification.
