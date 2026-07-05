#include "gzc_rpc_frame.h"

bool gzc_rpc_frame_type_valid(gzc_rpc_frame_type_t type) {
  return type == GZC_RPC_FRAME_EOS || type == GZC_RPC_FRAME_JSON || type == GZC_RPC_FRAME_BINARY ||
         type == GZC_RPC_FRAME_TEXT;
}

int gzc_rpc_frame_encode(const gzc_platform_t *platform, const gzc_rpc_frame_t *frame, gzc_buf_t *out_bytes) {
  if (frame == NULL || out_bytes == NULL || (frame->data == NULL && frame->len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (!gzc_rpc_frame_type_valid(frame->type)) {
    return GZC_ERR_RPC;
  }
  if (frame->len > GZC_RPC_MAX_FRAME_SIZE) {
    return GZC_ERR_RPC;
  }
  if (frame->type == GZC_RPC_FRAME_EOS && frame->len != 0) {
    return GZC_ERR_RPC;
  }
  if (platform == NULL) {
    platform = gzc_default_platform();
  }
  uint8_t header[4];
  header[0] = (uint8_t)(frame->len & 0xffu);
  header[1] = (uint8_t)((frame->len >> 8) & 0xffu);
  header[2] = (uint8_t)(((uint16_t)frame->type) & 0xffu);
  header[3] = (uint8_t)((((uint16_t)frame->type) >> 8) & 0xffu);
  int rc = gzc_buf_append(out_bytes, platform, header, sizeof(header));
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append(out_bytes, platform, frame->data, frame->len);
}

int gzc_rpc_frame_decode(const uint8_t *data, size_t len, gzc_rpc_frame_t *out_frame) {
  if (data == NULL || out_frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (len < 4) {
    return GZC_ERR_RPC;
  }
  size_t payload_len = (size_t)data[0] | ((size_t)data[1] << 8);
  gzc_rpc_frame_type_t type = (gzc_rpc_frame_type_t)((uint16_t)data[2] | ((uint16_t)data[3] << 8));
  if (!gzc_rpc_frame_type_valid(type)) {
    return GZC_ERR_RPC;
  }
  if (payload_len > GZC_RPC_MAX_FRAME_SIZE || len != 4 + payload_len) {
    return GZC_ERR_RPC;
  }
  if (type == GZC_RPC_FRAME_EOS && payload_len != 0) {
    return GZC_ERR_RPC;
  }
  out_frame->type = type;
  out_frame->data = data + 4;
  out_frame->len = payload_len;
  return GZC_OK;
}
