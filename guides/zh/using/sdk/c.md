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

解码字符串通常指向 `response`，在同一个 `gzc_control_call_t` 被复用前保持有效；MHS 转义字符串使用下文所述的额外调用方存储。列表路由由调用方提供数组与容量；数组不足时返回 `GZC_ERR_BUFFER_TOO_SMALL`。开放结构（`PeerStatus`、`DeviceInfo`）在 typed 字段之外提供 `raw`，与 Dart 和 TypeScript 控制侧 package 一致。

Request 侧的字符串上限直接取自 contract：SSID 32 字节、sound 32 字节、display_name 80 字节，超限在发出请求前就返回 `GZC_ERR_INVALID_ARGUMENT`。

设备配置与控制 App 相关 route 对应 `gzc_control_get_device_settings`、`gzc_control_update_device_settings`（`gzc_control_device_settings_t`，枚举成员为 wire 字符串，未设置的成员不发送）、`gzc_control_factory_reset_device`、`gzc_control_list_device_rpc_methods`、`gzc_control_set_device_run_workspace` 与 `gzc_control_list_device_tools` / `gzc_control_invoke_device_tool`。Tool 的 `i18n` 与 `input_schema` 以原始 JSON 返回；`invoke` 的 `data_json` 在 wire 上是转义字符串，SDK 把反转义后的 JSON 写入该次调用的 `scratch`，`call.body` 保持原始响应不变。

好友与群组 route 对应 `gzc_control_*_friend*` 与 `gzc_control_*_friend_group*` 函数，覆盖邀请码（`gzc_control_invite_token_request_t` 的可选 `ttl_seconds`）、加好友、列表、退群、解散与成员管理。群角色以字符串（`owner`、`admin`、`member`）返回，`info` 以 `has_info` 标记是否存在。

成员列表的每项带可选的 `online`（Server 本地连接状态）与 `last_seen_at`（RFC 3339 UTC；未知或读取失败时省略），add、put、join 返回的成员不带这两个字段。C 使用 `has_online` + `online` 与 `last_seen_at`（`gzc_str_t`）。

### MHS v0 控制侧 API

`gzc_control_get_mhs_v0_manifest` 把离线清单解码到调用方提供的 `gzc_control_mhs_v0_device_t` 数组。嵌套列表使用 `gzc_control_mhs_v0_device_states`、`gzc_control_mhs_v0_device_tags` 和 `gzc_control_mhs_v0_state_enum_values` 解码；每个函数都接收输出数组、容量并返回已解码数量。状态类型与访问权限使用 C enum，`has_min` / `has_max` / `has_step` 区分缺省约束和零值。

`gzc_control_read_mhs_v0_states` 接收 `gzc_control_mhs_v0_state_ref_t` key 数组；`gzc_control_write_mhs_v0_states` 接收 `gzc_control_mhs_v0_state_value_t` 数组，返回设备实际生效值。两者每批要求 1–32 项。ID 与名称遵循清单的 ASCII 语法，最多 64 字节；string/enum 值必须是无 NUL 的有效 UTF-8，最多 256 字节。非法输入在发送前返回 `GZC_ERR_INVALID_ARGUMENT`。重复 key、访问权限、清单类型、范围和 enum 成员由 Server 校验，保留正常 HTTP 错误，包括 `404 MHS_STATE_NOT_FOUND`。

`gzc_control_mhs_v0_value_t.kind` 表示 JSON 表达形式，不表示清单类型。两种数值 kind 都提供 `double_value`；`has_int_value` 表示同时存在 ±9007199254740991 范围内精确的 `int_value`，也包括实际为整数的小数或指数 token。`number_is_integer_token` 区分原始整数 token 和小数/指数 token，即使超出安全整数范围也保留该信息；`number_json` 保留原始 token。清单中的 `double` 状态可能以 `0` 返回，kind 为 `GZC_CONTROL_MHS_V0_VALUE_INT`，此时仍使用 `double_value`。写入时 `VALUE_INT` 使用 `int_value`，`VALUE_DOUBLE` 使用有限的 `double_value`，`VALUE_STRING` 使用 `string_value` 承载 string/enum；解码元数据 `has_int_value`、`number_is_integer_token`、`number_json` 不参与编码。

所有 MHS 调用及嵌套解码函数接收以 `{data, capacity, 0}` 初始化的 `gzc_control_mhs_v0_storage_t`。普通字符串及嵌套 JSON 数组借用 `call.response`，转义字符串解码到这块独立的调用方缓冲区。HTTP 调用在发送后重置 `used`，嵌套解码继续追加。两块存储都需保持有效，且不得与请求 scratch 或输入 JSON 重叠。每个嵌套列表只解码一次时，字符串存储容量不小于 `response_cap` 即可容纳完整结果。SDK 不分配、不扩容。数组或字符串存储不足返回 `GZC_ERR_BUFFER_TOO_SMALL`：HTTP 调用同时分类为 `GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL`，嵌套 helper 直接返回错误码；溢出后只有已解码的前缀有效。请求 scratch 应容纳整批数据及 JSON 转义，上文的 512 字节示例只适用于小请求。

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

原子写入和超时后重新读取的语义见 [MHS HTTP 契约](/zh/developing/api/http/public#mhs-v0-硬件状态)。

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

## 接入点 URL

`gzc_client_config_t.server_endpoint` 是 Server 或 Edge 的 HTTP 接入点。它接受 `http://` 或 `https://` 基础 URL，例如 `https://ap.gizclaw.com`；裸 `host:port` 仍然可用，并按 `http` 解析。路径前缀会保留，结尾斜杠会被去掉，查询串、fragment 和 userinfo 会被拒绝。

TLS 接入点所在端口可能不承载 ICE，因此接入点的 authority 不是 WebRTC 媒体地址。SDK 不需要为此额外配置：answer SDP 会带上 Server 的 ICE candidate。

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

## 手写源文件清单与 v0.20.0 升级

从 v0.19.x 升级到 v0.20.0 的设备侧工程，必须把
`generated/giznet/admission.pb.c` 加入编译源文件清单。`gzc_client.c` 和
signaling 编码器无条件引用其生成描述符，即使使用默认 `open` 准入、未设置
credential，也需要链接这个文件。Release 的 Bazel target 递归包含
`generated/**/*.c`，会自动纳入；自有 Makefile/CMake 的手写清单需要同步。

完整设备库输入以源码包的 `BUILD.bazel` 为准（仓库源为
`sdk/c/gizclaw/packaging/BUILD.bazel.in`）：

| 输入 | 源码包根目录下的路径 | 仓库中的路径 |
| --- | --- | --- |
| Portable SDK | `src/*.c`，排除 `src/gzc_platform.c` | `sdk/c/gizclaw/src/` |
| 全部生成的 C 文件 | 递归 `generated/**/*.c` | `sdk/c/gizclaw/generated/` |
| 固定版本 nanopb runtime | `third_party/nanopb/pb_common.c`、`pb_decode.c`、`pb_encode.c` | `third_party/nanopb/upstream/` 中的同名文件 |
| 默认 platform，可选 | `src/gzc_platform.c` | `sdk/c/gizclaw/src/gzc_platform.c` |

生成目录包含 admission、RPC、所有 payload、events 和 Google protobuf 辅助文件；
不要只选 `rpc.pb.c`，也不要只按 `.pb.c` 后缀挑选未来的生成输出。
Telemetry 实现在 portable SDK 的 `src/gzc_telemetry.c` 中。
在已校验并解压的 Release 源码包根目录执行以下命令，输出完整的 portable
SDK、生成文件和 nanopb 编译清单：

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

以 C11 编译以上文件，include 路径加入源码包根目录下的 `include`、
`generated`、`third_party/nanopb`；直接使用仓库时对应
`sdk/c/gizclaw/include`、`sdk/c/gizclaw/generated`、
`third_party/nanopb/upstream`。保留生成文件的子目录结构，让
`giznet/admission.pb.h`、`payload/*.pb.h` 等 include 可以解析；不需要逐个添加
生成子目录。`src` 中的私有头文件也必须随源文件保留。

Desktop 构建再加入 `src/gzc_platform.c`；firmware 构建链接自己的
`gzc_default_platform()` 实现，两种实现只选一种。HTTP、crypto、WebRTC 的
platform vtable 仍由集成方提供。不要将 `tests/` 或仓库 `cgobackend/` 加入
portable 设备库，也不要混用其他版本的 nanopb runtime。
控制侧 `gizclaw_control` 继续使用独立的 `control/src/*.c` 与
`control/include`、`control/src`，复用设备库的 public/generated include 路径；
它不因这次设备握手升级而需要编译整套设备 SDK。

## 设备握手准入

连接前在 client owner 线程调用 `gzc_client_set_admission_credential(client, &credential)`，
其中 `credential` 是生成的 `giznet_v1_AdmissionCredential`，含 version、type、value。
SDK 校验 protobuf 编码上限后复制结构，下次 connect 时编码并在 AEAD 内发送；调用方
随后可释放自己的结构。传 `NULL` 清除，已连接或已关闭 client 不接受修改；替换或
销毁时释放副本，设置失败保留旧值。既有 public struct 布局和 connect 签名不变。

`gzc_registration_token_credential(gzc_str_from_cstr(registrationToken), &credential)`
构造 GizClaw version 1、type `gizclaw.com/registration_token` 凭证，超限、非法 pointer 或嵌入 NUL
返回 `GZC_ERR_INVALID_ARGUMENT`，失败不修改输出。生成的字符串数组要求有界、以 NUL
结尾的 UTF-8；type 最多 128 bytes，value 最多 512 bytes，整体 protobuf 编码最多
4096 bytes，独立于字段上限。helper 的业务常量不属于 signaling 层。

低层 `gzc_signaling_build_offer_request_with_credential(config, offer_sdp, &credential,
exchange, request)` 只在调用内借用结构；`gzc_signaling_encode_admission_credential`
可独立编码。旧 builder 保留裸 SDP。超限、未终止字符串、空编码结构或明文 cipher
返回 `GZC_ERR_INVALID_ARGUMENT`，分配失败返回 `GZC_ERR_NO_MEMORY`。连接后仍需
`server.register` 完成绑定，见 [Security Policy](../../developing/gizclaw/server/security-policy)。

内置 type 由导出常量 `GZC_REGISTRATION_TOKEN_CREDENTIAL_TYPE` 定义，helper 引用该常量。value 最多 512 个 UTF-8 字节；超限在 helper 构造时返回错误或抛出异常。自定义 policy 应使用自己的域名前缀，内置类型保留 `gizclaw.com/` 前缀。

## MHS v0 硬件状态

`gzc_rpc.h` 导出 `payload/mhs.pb.h`。在 `rpc_provider` 中处理 `RPC_METHOD_CLIENT_MHS_V0_READ/WRITE`（133/134），使用生成的 `ClientMhsV0*` nanopb 编解码。provider 自行回答 `client.rpc.methods.list`，仅列出真实安装的方法；缺少 handler 返回 `GZC_ERR_UNSUPPORTED`。响应 bytes 在 respond callback 内借用，必须在返回前有效。

这是 GizClaw 自有的 MHS-inspired 预标准 v0，不声称官方兼容。manifest 离线可读，每次读写最多 32 个唯一 key；写入必须整批验证且驱动执行安全限制。完整错误与边界见 [Public API](/zh/developing/api/http/public) 和 [provider contract](/zh/developing/api/proto/rpc/client-provided-to-server)。

## tool/v0 过程

设备端只为实际支持的预定义 `ClientTool` 安装 handler；`client.tool.v0.list` 仅返回已安装的子集。控制端的类型化调用统一使用 `client.tool.v0.invoke` RPC。硬件状态由已绑定 RuntimeProfile 的 MHS v0 manifest 声明，并通过 MHS 读写接口访问。
