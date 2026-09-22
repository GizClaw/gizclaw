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

Each members-list item carries optional `online` (Server-local connection state) and `last_seen_at` (RFC 3339 UTC; absent when unknown or the read failed); members returned by add, put and join omit both. C uses `has_online` + `online` and `last_seen_at` (`gzc_str_t`).

### MHS v0 controller API

`gzc_control_get_mhs_v0_manifest` reads the offline manifest into a caller-owned array of `gzc_control_mhs_v0_device_t`. Decode nested arrays with `gzc_control_mhs_v0_device_states`, `gzc_control_mhs_v0_device_tags` and `gzc_control_mhs_v0_state_enum_values`; each takes an output array, capacity and decoded count. State types and access modes are C enums, and `has_min` / `has_max` / `has_step` distinguish absent constraints from zero.

`gzc_control_read_mhs_v0_states` accepts `gzc_control_mhs_v0_state_ref_t` keys; `gzc_control_write_mhs_v0_states` accepts `gzc_control_mhs_v0_state_value_t` values and returns the device's actual applied values. Both require 1–32 entries. IDs and names follow the manifest's ASCII syntax and are at most 64 bytes; string/enum values are valid UTF-8 without NUL, at most 256 bytes. Invalid input returns `GZC_ERR_INVALID_ARGUMENT` before transport. The Server validates duplicate keys, access, manifest types, bounds and enum membership, retaining its normal HTTP errors, including `404 MHS_STATE_NOT_FOUND`.

`gzc_control_mhs_v0_value_t.kind` describes the JSON representation, not the manifest type. Both numeric kinds provide `double_value`; `has_int_value` indicates an exact `int_value` within ±9007199254740991, including integral decimal/exponent tokens. `number_is_integer_token` preserves its integer versus decimal/exponent syntax, even outside the safe integer range; `number_json` retains the original token. A manifest `double` state can arrive as `0` with kind `GZC_CONTROL_MHS_V0_VALUE_INT`: use `double_value`. On write, `VALUE_INT` uses `int_value`, `VALUE_DOUBLE` uses finite `double_value`, and `VALUE_STRING` uses `string_value` for both string and enum states. The decoded metadata `has_int_value`, `number_is_integer_token` and `number_json` is ignored on write.

All MHS calls and nested decoders take `gzc_control_mhs_v0_storage_t`, initialized as `{data, capacity, 0}`. Plain strings and nested JSON arrays borrow `call.response`; escaped strings are decoded into this separate caller-owned region. HTTP calls reset `used` after sending; nested decoders append. Keep both regions alive and distinct from request scratch/input JSON. A string region at least as large as `response_cap` is sufficient when decoding each nested list once. The SDK never allocates or grows either region. Array/string exhaustion returns `GZC_ERR_BUFFER_TOO_SMALL`; HTTP calls classify it as `GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL`, while nested helpers return the code directly. Only the decoded prefix is valid after overflow. Size request scratch for the full batch and JSON escaping; the 512-byte example above only covers small requests.

```c
char mhs_strings[8192];
gzc_control_mhs_v0_storage_t storage = {mhs_strings, sizeof(mhs_strings), 0};
gzc_control_mhs_v0_state_ref_t key = {
    gzc_str_from_cstr("display.main"), gzc_str_from_cstr("brightness")};
gzc_control_mhs_v0_state_value_t applied[1];
size_t count = 0;
int rc = gzc_control_read_mhs_v0_states(
    &client, &call, &key, 1, &storage, applied, 1, &count);
if (rc != GZC_OK) {
  return;
}
```

See [MHS HTTP semantics](/en/developing/api/http/public#mhs-v0-hardware-states) for atomic writes and timeout/read-back behavior.

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

## Manual source lists and the v0.20.0 upgrade

Device integrations upgrading from v0.19.x to v0.20.0 must add
`generated/giznet/admission.pb.c` to their compiled source list. `gzc_client.c` and
the signaling encoder reference its generated descriptor unconditionally, even
with default `open` admission and no configured credential. Release Bazel targets
include `generated/**/*.c` recursively and pick it up automatically; handwritten
Makefile/CMake lists must be updated.

The complete device-library inputs follow the archive's `BUILD.bazel` (owned by
`sdk/c/gizclaw/packaging/BUILD.bazel.in` in the repository):

| Input | Path relative to the archive root | Repository path |
| --- | --- | --- |
| Portable SDK | `src/*.c`, excluding `src/gzc_platform.c` | `sdk/c/gizclaw/src/` |
| Every generated C file | Recursive `generated/**/*.c` | `sdk/c/gizclaw/generated/` |
| Pinned nanopb runtime | `third_party/nanopb/pb_common.c`, `pb_decode.c`, `pb_encode.c` | The same filenames under `third_party/nanopb/upstream/` |
| Optional default platform | `src/gzc_platform.c` | `sdk/c/gizclaw/src/gzc_platform.c` |

Generated inputs include admission, RPC, every payload, events, and Google
protobuf helpers. Do not select only `rpc.pb.c` or restrict future generated
outputs to the `.pb.c` suffix. Telemetry is implemented in the portable SDK's
`src/gzc_telemetry.c`. Run this command from a verified, extracted Release archive
root to print the complete portable SDK, generated, and nanopb source list:

```sh
python3 - <<'PY'
from pathlib import Path
sources = sorted(p for p in Path("src").glob("*.c") if p.name != "gzc_platform.c")
sources += sorted(Path("generated").rglob("*.c"))
sources += [Path("third_party/nanopb") / name
            for name in ("pb_common.c", "pb_decode.c", "pb_encode.c")]
for source in sources:
    if not source.is_file():
        raise SystemExit(f"missing source: {source}")
    print(source.as_posix())
PY
```

Compile these files as C11 with `include`, `generated`, and `third_party/nanopb`
as include directories relative to the archive root. The repository equivalents
are `sdk/c/gizclaw/include`, `sdk/c/gizclaw/generated`, and
`third_party/nanopb/upstream`. Preserve generated subdirectories so includes such
as `giznet/admission.pb.h` and `payload/*.pb.h` resolve; adding every generated
subdirectory separately is unnecessary. Keep private headers beside their sources
in `src` too.

Desktop builds additionally compile `src/gzc_platform.c`; firmware builds link
their own `gzc_default_platform()` implementation. Choose exactly one platform
implementation. Integrators still supply HTTP, crypto, and WebRTC platform
vtables. Exclude `tests/` and the repository's `cgobackend/` from the portable
device library, and do not mix in a different nanopb runtime version.
The controller package `gizclaw_control` retains its independent `control/src/*.c`
inputs and `control/include`, `control/src` directories, reusing device-library
public/generated include directories. This device handshake upgrade does not
require control-only consumers to compile the full device SDK.

## Device handshake admission

Call `gzc_client_set_admission_credential(client, &credential)` on the owner thread before
connect, using the generated `giznet_v1_AdmissionCredential` with version, type, and value fields.
The SDK validates its encoded size and copies the structure, then protobuf-encodes and seals it
inside AEAD during connect. The caller may release its structure afterwards. `NULL` clears it;
connected or closed clients reject changes. Replacement and destroy free the copy, and a failed
setter preserves the old value. Existing public struct layouts and connect signatures remain.

`gzc_registration_token_credential(gzc_str_from_cstr(registrationToken), &credential)` constructs
GizClaw version 1 and type `gizclaw.com/registration_token`. Oversized input, invalid pointers, or embedded
NUL return `GZC_ERR_INVALID_ARGUMENT` without changing the output. Generated string arrays require
bounded, NUL-terminated UTF-8: type allows 128 bytes and value 512 bytes. The complete protobuf
encoding must fit in 4096 bytes independently of the field limits. This business helper is outside
signaling.

`gzc_signaling_build_offer_request_with_credential(config, offer_sdp, &credential, exchange,
request)` borrows the structure for the call; `gzc_signaling_encode_admission_credential` also
supports independent encoding. The old builder preserves bare SDP. Excessive lengths,
unterminated strings, empty encodings, or plaintext cipher return `GZC_ERR_INVALID_ARGUMENT`;
allocation failure returns `GZC_ERR_NO_MEMORY`. Call `server.register` after connecting to bind
resources; see [Security Policy](../../developing/gizclaw/server/security-policy).

The exported `GZC_REGISTRATION_TOKEN_CREDENTIAL_TYPE` constant defines the built-in type and is used by the helper. Values are limited to 512 UTF-8 bytes; construction returns an error or throws for larger input. Custom policies should use their own domain prefix; built-in types reserve `gizclaw.com/`.

## MHS v0 hardware states

`gzc_rpc.h` exposes `payload/mhs.pb.h`. Handle `RPC_METHOD_CLIENT_MHS_V0_READ/WRITE` (133/134) in `rpc_provider` with generated `ClientMhsV0*` nanopb codecs. The provider answers `client.rpc.methods.get` with only implemented methods; absent handlers return `GZC_ERR_UNSUPPORTED`. Response bytes are borrowed during the respond callback and must remain valid until it returns.

This is GizClaw's MHS-inspired pre-standard v0, with no official compatibility claim. Manifests work offline; reads/writes allow at most 32 unique keys. Drivers validate the whole batch and enforce safety limits. See [Public API](/en/developing/api/http/public) and the [provider contract](/en/developing/api/proto/rpc/client-provided-to-server).
