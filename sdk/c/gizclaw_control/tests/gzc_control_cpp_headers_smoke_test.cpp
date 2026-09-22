/* Confirms the controller header stays C++-consumable and ABI-stable. */
#include "gzc_control.h"

/* The error kinds are contract values shared with the Dart and TypeScript
 * controller SDKs, so their order is part of the ABI. */
static_assert(GZC_CONTROL_ERROR_NONE == 0);
static_assert(GZC_CONTROL_ERROR_UNAUTHORIZED == 1);
static_assert(GZC_CONTROL_ERROR_DEVICE_OFFLINE == 4);
static_assert(GZC_CONTROL_ERROR_DEVICE_ERROR == 8);
static_assert(GZC_CONTROL_ERROR_NETWORK == 14);
/* The C-only kind sits after every shared one, so the shared values match. */
static_assert(GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL == 15);

static_assert(GZC_CONTROL_MAX_SSID_BYTES == 32);
static_assert(GZC_CONTROL_MAX_SOUND_BYTES == 32);
static_assert(GZC_CONTROL_MAX_DISPLAY_NAME_BYTES == 80);

static_assert(GZC_CONTROL_MHS_V0_MAX_BATCH == 32);
static_assert(GZC_CONTROL_MHS_V0_MAX_NAME_BYTES == 64);
static_assert(GZC_CONTROL_MHS_V0_MAX_STRING_BYTES == 256);
static_assert(GZC_CONTROL_MHS_V0_MAX_INT == 9007199254740991LL);

int main() {
  gzc_control_mhs_v0_value_t mhs{};
  gzc_control_mhs_v0_storage_t storage{};
  gzc_control_firmware_update_request_t firmware{};
  gzc_control_social_ping_request_t ping{};
  gzc_control_device_identifiers_t identifiers{};
  gzc_control_client_t client{};
  gzc_control_call_t call{};
  gzc_control_peer_status_t status{};
  gzc_control_device_info_t device{};
  return client.config.timeout_ms != 0 || call.status_code != 0 || status.has_volume ||
         device.has_hardware || mhs.has_int_value || storage.used != 0 ||
         firmware.channel.len != 0 || ping.from_peer_public_key.len != 0 || identifiers.sn.len != 0;
}
