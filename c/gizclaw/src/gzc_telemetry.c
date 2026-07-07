#include "gzc_telemetry.h"

int gzc_client_send_telemetry(gzc_client_t *client, const gzc_telemetry_frame_t *frame) {
  if (client == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_default_platform();
  gzc_buf_t payload;
  gzc_buf_init(&payload);
  int rc = gzc_telemetry_encode_frame(frame, platform, &payload);
  if (rc == GZC_OK) {
    rc = gzc_client_send_packet(client, GZC_PROTOCOL_TELEMETRY, payload.data, payload.len);
  }
  gzc_buf_free(&payload, platform);
  return rc;
}
