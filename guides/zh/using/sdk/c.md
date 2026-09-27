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

### MHS v0 HWD

`gzc_rpc.h` 导出 `payload/mhs_v0.pb.h`。设备 provider 处理 `RPC_METHOD_CLIENT_MHS_V0_READ/WRITE`（133/134）：先解码外层实例 `id`、`ClientHwd`，再按 HWD 解码或编码内层 protobuf payload。wifi、ble、modem、battery、mic 只提供 read；display、led、speaker 提供 read/write。`client.rpc.methods.list` 只列出真实安装的 RPC handler。响应 bytes 在 respond callback 返回前必须保持有效。详细错误与实例约束见 [provider contract](/zh/developing/api/proto/rpc/client-provided-to-server)。

## tool/v0 过程

设备端只为实际支持的预定义 `ClientTool` 安装 handler；`client.tool.v0.list` 仅返回已安装的子集。控制端的类型化调用统一使用 `client.tool.v0.invoke` RPC。HWD 实例由已绑定 RuntimeProfile 的 MHS v0 manifest 声明，并通过 MHS 读写接口访问。
