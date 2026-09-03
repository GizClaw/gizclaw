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

Every `gzc_str_t` in a decoded model points into `response` and stays valid until the same `gzc_control_call_t` is reused. List routes take a caller array and its capacity, and report `GZC_ERR_BUFFER_TOO_SMALL` when the page is larger. Open-ended schemas (`PeerStatus`, `DeviceInfo`) expose `raw` beside their typed fields, matching the Dart and TypeScript controller packages.

Request string caps come straight from the contract: SSID 32 bytes, sound 32 bytes, display_name 80 bytes. An oversized value returns `GZC_ERR_INVALID_ARGUMENT` before any transport call.

### Error classification

A failed call fills `gzc_control_call_t.error` with a `gzc_control_error_t`. The `kind` values and the rules that pick them match `sdk/flutter/gizclaw_control` and `sdk/js/gizclaw-control` exactly: `DEVICE_*` is matched on the response body's `error.code`, everything else on the HTTP status.

| `kind` | Condition |
| --- | --- |
| `GZC_CONTROL_ERROR_UNAUTHORIZED` / `FORBIDDEN` / `NOT_FOUND` | `401` / `403` / `404` |
| `GZC_CONTROL_ERROR_DEVICE_OFFLINE` | `409 DEVICE_OFFLINE` |
| `GZC_CONTROL_ERROR_DEVICE_TIMEOUT` | `504 DEVICE_TIMEOUT` |
| `GZC_CONTROL_ERROR_DEVICE_REJECTED` | `400 DEVICE_REJECTED` |
| `GZC_CONTROL_ERROR_DEVICE_UNSUPPORTED` | `501 DEVICE_UNSUPPORTED` |
| `GZC_CONTROL_ERROR_DEVICE_ERROR` | `502 DEVICE_ERROR` |
| `GZC_CONTROL_ERROR_CONFLICT` / `INVALID_REQUEST` | any other `409` / any other `400` |
| `GZC_CONTROL_ERROR_SERVER` | any other `5xx` |
| `GZC_CONTROL_ERROR_UNEXPECTED_STATUS` | any other non-2xx |
| `GZC_CONTROL_ERROR_MALFORMED_RESPONSE` | a 2xx body that is not the contract type |
| `GZC_CONTROL_ERROR_NETWORK` | no HTTP response, or the request could not be built |
| `GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL` | a well-formed page larger than the caller's array; retry with more room or a smaller `limit` |

The last kind has no counterpart in the Dart and TypeScript packages, which allocate their own lists; it sits after every shared kind so their values stay aligned.

`gzc_control_call_t.error.request_id` carries the `X-Request-ID` response header. The transport delivers response headers one at a time through the `response_header_cb` sink on `gzc_http_request_t`; a backend that sets no headers simply leaves `request_id` empty.

## Access point URL

`gzc_client_config_t.server_endpoint` is the HTTP access point of the Server or Edge. It accepts an `http://` or `https://` base URL such as `https://ap.gizclaw.com`, and a bare `host:port` still resolves to `http`. A path prefix is preserved, a trailing slash is dropped, and query strings, fragments and userinfo are rejected.

A TLS access point can terminate on a port that carries no ICE, so the access point authority is not the WebRTC media address. The SDK needs no extra configuration for that: the answer SDP carries the Server's ICE candidates.

## Download and verify

Download both the archive and its sidecar before adding it to a build:

```sh
version=1.2.3
base="https://github.com/GizClaw/gizclaw/releases/download/v${version}"
curl --fail --location --remote-name "$base/gizclaw-c-sdk-${version}.tar.gz"
curl --fail --location --remote-name "$base/gizclaw-c-sdk-${version}.tar.gz.sha256"
sha256sum --check "gizclaw-c-sdk-${version}.tar.gz.sha256"
```

The archive has one `gizclaw-c-sdk-X.Y.Z/` root and contains `MODULE.bazel`, `BUILD.bazel`, the public and generated C surface, the exact packaged nanopb runtime, licenses, smoke fixtures, and `SOURCE_PROVENANCE.json`. Provenance binds the version, GizClaw source commit and epoch, and nanopb gitlink commit.

## Bzlmod consumption

Until the module is registered in Bazel Central Registry, the root consumer declares the version and overrides it with the verified Release archive:

```starlark
bazel_dep(name = "gizclaw_c_sdk", version = "1.2.3")

archive_override(
    module_name = "gizclaw_c_sdk",
    urls = [
        "https://github.com/GizClaw/gizclaw/releases/download/v1.2.3/gizclaw-c-sdk-1.2.3.tar.gz",
    ],
    integrity = "sha256-<base64 SHA-256 of the verified archive>",
    strip_prefix = "gizclaw-c-sdk-1.2.3",
)
```

Keep the URL, module version, `strip_prefix`, and integrity value on the same immutable release. Convert the verified hex digest to the Subresource Integrity value required by Bazel, or obtain it from an internal dependency update tool; never omit archive integrity.

The module exports:

- `@gizclaw_c_sdk//:gizclaw_core`: portable SDK and packaged nanopb runtime without `src/gzc_platform.c`.
- `@gizclaw_c_sdk//:default_platform`: the libc/POSIX implementation of `gzc_default_platform()`.
- `@gizclaw_c_sdk//:gizclaw`: desktop composition of the two targets.

Firmware uses `gizclaw_core` and links its PAL-owned implementation of the existing `gzc_default_platform()` function. That implementation returns the firmware `gzc_platform_t` with allocator, clock, entropy, and logging callbacks; the firmware also supplies its HTTP, crypto, and WebRTC vtables. Desktop consumers can depend on `gizclaw` for the existing nullable-platform fallback.

The archive does not own a firmware toolchain, final link, image packaging, flashing, credentials, or provider configuration. Consumers must not patch the extracted SDK or fetch another nanopb copy; upgrade to a release containing the required source fix instead.
