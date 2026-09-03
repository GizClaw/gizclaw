# C SDK

GizClaw 会在每个规范 `vMAJOR.MINOR.PATCH` GitHub Release 中附带一个可复现的 C SDK 源码包。C SDK 没有独立的 runtime version：源码包版本 `X.Y.Z` 与仓库 tag `vX.Y.Z` 指向同一个 source commit。

## 两个 C package

`sdk/c/` 下按角色划分为两个 package，源码包同时携带两者：

| Package | 目录 | 角色 | Transport |
| --- | --- | --- | --- |
| `gizclaw` | `sdk/c/gizclaw` | 设备侧：把固件或进程接入为 GizClaw device/Peer，覆盖 signaling、WebRTC、Peer RPC 与 Telemetry | 加密 `/webrtc/v1/offer` signaling 与 WebRTC DataChannel |
| `gizclaw_control` | `sdk/c/gizclaw_control` | 控制侧：持 [API Key](../api-keys) 读取并控制已绑定的设备 | HTTPS `/gizclaw/v1` |

`gizclaw_control` 只复用设备侧 SDK 的 `platform/gzc_platform_http.h` transport 抽象与 `gzc_json.h` codec，不引入任何新依赖，也不参与 WebRTC。两者在源码包中分别对应 `@gizclaw_c_sdk//:gizclaw`（或 `gizclaw_core`）与 `@gizclaw_c_sdk//:gizclaw_control`。

### `gizclaw_control` 的内存契约

package 自身不做任何分配。调用方声明 `gzc_control_client_t`，并为每次调用提供两块缓冲区：`scratch` 承载请求 URL 与 request body，`response` 承载响应 body 并作为全部解码结果的存储：

```c
#include "gzc_control.h"

gzc_control_config_t config = {0};
config.base_url = gzc_str_from_cstr("https://ap.gizclaw.com");
config.api_key = gzc_str_from_cstr("Bearer gizclaw_sk_v1_...");
config.http = &http_vtable; /* 与设备侧 SDK 相同的 gzc_http_vtable_t */

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

解码结果中的每个 `gzc_str_t` 都指向 `response`，在同一个 `gzc_control_call_t` 被复用前保持有效。列表路由由调用方提供数组与容量；数组不足时返回 `GZC_ERR_BUFFER_TOO_SMALL`。开放结构（`PeerStatus`、`DeviceInfo`）在 typed 字段之外提供 `raw`，与 Dart 和 TypeScript 控制侧 package 一致。

Request 侧的字符串上限直接取自 contract：SSID 32 字节、sound 32 字节、display_name 80 字节，超限在发出请求前就返回 `GZC_ERR_INVALID_ARGUMENT`。

### 错误分类

失败调用把 `gzc_control_call_t.error` 填成 `gzc_control_error_t`。`kind` 的取值与判定规则与 `sdk/flutter/gizclaw_control`、`sdk/js/gizclaw-control` 完全一致：`DEVICE_*` 按响应体的 `error.code` 判定，其余按 HTTP status。

| `kind` | 条件 |
| --- | --- |
| `GZC_CONTROL_ERROR_UNAUTHORIZED` / `FORBIDDEN` / `NOT_FOUND` | `401` / `403` / `404` |
| `GZC_CONTROL_ERROR_DEVICE_OFFLINE` | `409 DEVICE_OFFLINE` |
| `GZC_CONTROL_ERROR_DEVICE_TIMEOUT` | `504 DEVICE_TIMEOUT` |
| `GZC_CONTROL_ERROR_DEVICE_REJECTED` | `400 DEVICE_REJECTED` |
| `GZC_CONTROL_ERROR_DEVICE_UNSUPPORTED` | `501 DEVICE_UNSUPPORTED` |
| `GZC_CONTROL_ERROR_DEVICE_ERROR` | `502 DEVICE_ERROR` |
| `GZC_CONTROL_ERROR_CONFLICT` / `INVALID_REQUEST` | 其他 `409` / 其他 `400` |
| `GZC_CONTROL_ERROR_SERVER` | 其他 `5xx` |
| `GZC_CONTROL_ERROR_UNEXPECTED_STATUS` | 其他非 2xx |
| `GZC_CONTROL_ERROR_MALFORMED_RESPONSE` | 2xx 但 body 不符合 contract 类型 |
| `GZC_CONTROL_ERROR_NETWORK` | 未产生 HTTP 响应，或请求无法构造 |
| `GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL` | 响应本身正常，但一页数据超出调用方数组容量；换更大的数组或更小的 `limit` 重试 |

最后一个 kind 在 Dart 与 TypeScript 包中没有对应项（它们自行分配列表），因此排在全部共享 kind 之后，保持共享取值一致。

`gzc_control_call_t.error.request_id` 携带 `X-Request-ID` 响应 header。transport 通过 `gzc_http_request_t` 上的 `response_header_cb` sink 逐条投递响应 header；未提供 header 的 backend 只会让 `request_id` 保持为空。

## 下载与校验

接入构建前同时下载源码包及其 sidecar：

```sh
version=1.2.3
base="https://github.com/GizClaw/gizclaw/releases/download/v${version}"
curl --fail --location --remote-name "$base/gizclaw-c-sdk-${version}.tar.gz"
curl --fail --location --remote-name "$base/gizclaw-c-sdk-${version}.tar.gz.sha256"
sha256sum --check "gizclaw-c-sdk-${version}.tar.gz.sha256"
```

源码包只有一个 `gizclaw-c-sdk-X.Y.Z/` 根目录，包含 `MODULE.bazel`、`BUILD.bazel`、C public/generated surface、精确的 nanopb runtime、license、smoke fixture 和 `SOURCE_PROVENANCE.json`。Provenance 将版本、GizClaw source commit/epoch 与 nanopb gitlink commit 绑定在一起。

## Bzlmod 消费

模块进入 Bazel Central Registry 前，根 consumer 声明版本，并用已经校验的 Release 源码包覆盖来源：

```starlark
bazel_dep(name = "gizclaw_c_sdk", version = "1.2.3")

archive_override(
    module_name = "gizclaw_c_sdk",
    urls = [
        "https://github.com/GizClaw/gizclaw/releases/download/v1.2.3/gizclaw-c-sdk-1.2.3.tar.gz",
    ],
    integrity = "sha256-<已校验源码包 SHA-256 的 base64 值>",
    strip_prefix = "gizclaw-c-sdk-1.2.3",
)
```

URL、module version、`strip_prefix` 与 integrity 必须来自同一个不可变 Release。将已校验的十六进制 digest 转换为 Bazel 要求的 Subresource Integrity 值，或由内部依赖更新工具生成；不能省略 archive integrity。

模块导出：

- `@gizclaw_c_sdk//:gizclaw_core`：portable SDK 与内置 nanopb runtime，不包含 `src/gzc_platform.c`。
- `@gizclaw_c_sdk//:default_platform`：libc/POSIX `gzc_default_platform()` 实现。
- `@gizclaw_c_sdk//:gizclaw`：组合以上两个 target 的 desktop 入口。

Firmware 使用 `gizclaw_core`，并链接 PAL 拥有的现有 `gzc_default_platform()` 实现。该实现返回带 allocator、clock、entropy 与 logging callback 的 firmware `gzc_platform_t`；firmware 仍负责 HTTP、crypto 和 WebRTC vtable。Desktop consumer 可以依赖 `gizclaw`，继续使用现有 nullable-platform fallback。

源码包不拥有 firmware toolchain、最终链接、image packaging、烧录、credential 或 provider 配置。Consumer 不能 patch 解压后的 SDK 或另取一份 nanopb；需要 portability 修复时应升级到包含修复的 GizClaw Release。
