# C SDK

GizClaw publishes its C SDK as a deterministic source archive attached to each canonical `vMAJOR.MINOR.PATCH` GitHub Release. The SDK does not have an independent runtime version: archive version `X.Y.Z` identifies the same source commit as repository tag `vX.Y.Z`.

## Two C packages

`sdk/c/` is split by role, and the source archive carries both:

| Package | Directory | Role | Transport |
| --- | --- | --- | --- |
| `gizclaw` | `sdk/c/gizclaw` | Device side: run firmware or a process as a GizClaw device/Peer, covering signaling, WebRTC, Peer RPC, and Telemetry | encrypted `/webrtc/v1/offer` signaling and WebRTC DataChannels |
| `gizclaw_control` | `sdk/c/gizclaw_control` | Controller side: read and control the bound device with an [API key](../api-keys) | HTTPS `/gizclaw/v1` |

`gizclaw_control` reuses only the device SDK's `platform/gzc_platform_http.h` transport abstraction and its `gzc_json.h` codec. It adds no dependency and takes no part in WebRTC. In the archive the two are `@gizclaw_c_sdk//:gizclaw` (or `gizclaw_core`) and `@gizclaw_c_sdk//:gizclaw_control`.

### The `gizclaw_control` memory contract

The package never allocates. The caller declares a `gzc_control_client_t` and supplies two regions per call: `scratch` carries the request URL and body, and `response` carries the response body and backs every decoded model:

```c
#include "gzc_control.h"

gzc_control_config_t config = {0};
config.base_url = gzc_str_from_cstr("https://ap.gizclaw.com");
config.api_key = gzc_str_from_cstr("Bearer gizclaw_sk_v1_...");
config.http = &http_vtable; /* the same gzc_http_vtable_t the device SDK uses */

gzc_control_client_t client;
if (gzc_control_client_init(&client, &config) != GZC_OK) {
  return;
}

uint8_t scratch[512];
uint8_t response[8192];
gzc_control_call_t call;
gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response));

gzc_control_peer_status_t status;
if (gzc_control_get_device_status(&client, &call, &status) == GZC_OK && status.has_volume) {
  use_volume(status.volume);
}
```

Decoded strings normally point into `response` and stay valid until the same `gzc_control_call_t` is reused. MHS escaped strings use the additional caller-owned storage described below. List routes take a caller array and its capacity, and report `GZC_ERR_BUFFER_TOO_SMALL` when the page is larger. Open-ended schemas (`PeerStatus`, `DeviceInfo`) expose `raw` beside their typed fields, matching the Dart and TypeScript controller packages.

Request string caps come straight from the contract: SSID 32 bytes, sound 32 bytes, display_name 80 bytes. An oversized value returns `GZC_ERR_INVALID_ARGUMENT` before any transport call.

Device settings and control-app routes map to `gzc_control_get_device_settings`, `gzc_control_update_device_settings` (`gzc_control_device_settings_t`; enum members are their wire strings and unset members are not sent), `gzc_control_factory_reset_device`, `gzc_control_list_device_rpc_methods`, `gzc_control_set_device_run_workspace`, and `gzc_control_list_device_tools` / `gzc_control_invoke_device_tool`. A Tool's `i18n` and `input_schema` come back as raw JSON. The invoke result's `data_json` is an escaped string on the wire; the SDK writes the unescaped JSON into that call's `scratch` and leaves `call.body` as the exact response.

Friend and Friend Group routes map to the `gzc_control_*_friend*` and `gzc_control_*_friend_group*` functions, covering invite tokens (optional `ttl_seconds` in `gzc_control_invite_token_request_t`), befriending, listing, leaving, dissolving, and member management. Group roles are returned as strings (`owner`, `admin`, `member`), and `has_info` marks whether `info` is present.

Each members-list item carries optional `online` (Server-local connection state), `last_seen_at` (RFC 3339 UTC; absent when unknown or the read failed) and `in_room` (whether the device is running the Group's Workspace on the answering Server, which is what attaches it to the Group's SFU Room; true implies online); members returned by add, put and join omit all three. C uses `has_online` + `online`, `last_seen_at` (`gzc_str_t`) and `has_in_room` + `in_room`.

### MHS v0 HWDs

`gzc_rpc.h` exports `payload/mhs_v0.pb.h`. A device provider handles `RPC_METHOD_CLIENT_MHS_V0_READ/WRITE` (133/134): decode the outer instance `id` and `ClientHwd`, then decode or encode the HWD-specific inner protobuf payload. Wifi, ble, modem, battery and mic provide read only; display, led and speaker provide read/write. `client.rpc.methods.list` includes only installed RPC handlers. Response bytes remain borrowed until the respond callback returns. See the [provider contract](/en/developing/api/proto/rpc/client-provided-to-server) for errors and instance rules.

## tool/v0 procedures

Device providers install handlers for the predefined `ClientTool` values they implement. `client.tool.v0.list` reports that installed subset; typed control calls use the single `client.tool.v0.invoke` RPC. HWD instances use the bound RuntimeProfile MHS v0 manifest and its read/write calls.
