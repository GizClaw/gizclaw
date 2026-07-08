#include "gzc_rpc.h"
#include "../generated/gzc_rpc_methods.h"

#include <string.h>

int gzc_client_reset_rpc_rx_internal(gzc_client_t *client);
int gzc_client_open_rpc_channel_internal(gzc_client_t *client, int timeout_ms);
void gzc_client_close_rpc_channel_internal(gzc_client_t *client);
int gzc_client_read_rpc_frame_internal(gzc_client_t *client, int timeout_ms, gzc_buf_t *out_frame_bytes);
int gzc_client_store_rpc_response_internal(gzc_client_t *client, const uint8_t *data, size_t len, gzc_str_t *out_payload);

static int append_frame(const gzc_platform_t *platform, gzc_buf_t *out, gzc_rpc_frame_type_t type, const uint8_t *data, size_t len) {
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.type = type;
  frame.data = data;
  frame.len = len;
  return gzc_rpc_frame_encode(platform, &frame, out);
}

static int append_binary_envelope_frame(const gzc_platform_t *platform, gzc_buf_t *out, const uint8_t *data, size_t len) {
  if (len > GZC_RPC_MAX_FRAME_SIZE) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return append_frame(platform, out, GZC_RPC_FRAME_BINARY, data, len);
}

static int append_varint(const gzc_platform_t *platform, gzc_buf_t *out, uint64_t value) {
  uint8_t buf[10];
  size_t n = 0;
  do {
    uint8_t b = (uint8_t)(value & 0x7fu);
    value >>= 7;
    if (value != 0) {
      b |= 0x80u;
    }
    buf[n++] = b;
  } while (value != 0 && n < sizeof(buf));
  return gzc_buf_append(out, platform, buf, n);
}

static int append_key(const gzc_platform_t *platform, gzc_buf_t *out, unsigned field, unsigned wire_type) {
  return append_varint(platform, out, ((uint64_t)field << 3) | wire_type);
}

static int append_proto_bytes(const gzc_platform_t *platform, gzc_buf_t *out, unsigned field, const uint8_t *data, size_t len) {
  int rc = append_key(platform, out, field, 2);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = append_varint(platform, out, (uint64_t)len);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append(out, platform, data, len);
}

static int append_proto_str(const gzc_platform_t *platform, gzc_buf_t *out, unsigned field, gzc_str_t value) {
  return append_proto_bytes(platform, out, field, (const uint8_t *)value.data, value.len);
}

static int append_proto_u32(const gzc_platform_t *platform, gzc_buf_t *out, unsigned field, unsigned value) {
  int rc = append_key(platform, out, field, 0);
  if (rc != GZC_OK) {
    return rc;
  }
  return append_varint(platform, out, value);
}

static int read_varint(const uint8_t *data, size_t len, size_t *offset, uint64_t *out) {
  uint64_t value = 0;
  unsigned shift = 0;
  while (*offset < len && shift <= 63) {
    uint8_t b = data[(*offset)++];
    value |= ((uint64_t)(b & 0x7fu)) << shift;
    if ((b & 0x80u) == 0) {
      *out = value;
      return GZC_OK;
    }
    shift += 7;
  }
  return GZC_ERR_RPC;
}

static int read_proto_bytes(const uint8_t *data, size_t len, size_t *offset, gzc_str_t *out) {
  uint64_t n = 0;
  int rc = read_varint(data, len, offset, &n);
  if (rc != GZC_OK) {
    return rc;
  }
  if (n > len - *offset) {
    return GZC_ERR_RPC;
  }
  *out = gzc_str_from_parts((const char *)(data + *offset), (size_t)n);
  *offset += (size_t)n;
  return GZC_OK;
}

static int skip_proto_field(const uint8_t *data, size_t len, size_t *offset, unsigned wire_type) {
  uint64_t ignored = 0;
  gzc_str_t bytes;
  switch (wire_type) {
    case 0:
      return read_varint(data, len, offset, &ignored);
    case 2:
      return read_proto_bytes(data, len, offset, &bytes);
    default:
      return GZC_ERR_RPC;
  }
}

static int method_id(gzc_str_t method, unsigned *out_id) {
  for (size_t i = 0; i < GZC_RPC_METHOD_COUNT; i++) {
    const char *name = gzc_rpc_methods[i].method;
    if (strlen(name) == method.len && memcmp(name, method.data, method.len) == 0) {
      *out_id = gzc_rpc_methods[i].method_id;
      return GZC_OK;
    }
  }
  return GZC_ERR_RPC;
}

static int decode_frame_bytes(gzc_buf_t *frame_bytes, gzc_rpc_frame_t *out_frame) {
  if (frame_bytes == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzc_rpc_frame_decode(frame_bytes->data, frame_bytes->len, out_frame);
}

static int read_frame(gzc_client_t *client, const gzc_platform_t *platform, int timeout_ms, gzc_buf_t *frame_bytes, gzc_rpc_frame_t *out_frame) {
  int rc = gzc_client_read_rpc_frame_internal(client, timeout_ms, frame_bytes);
  if (rc != GZC_OK) {
    return rc;
  }
  (void)platform;
  return decode_frame_bytes(frame_bytes, out_frame);
}

static void close_rpc_channel_on_error(gzc_client_t *client, int rc) {
  if (rc != GZC_OK) {
    gzc_client_close_rpc_channel_internal(client);
  }
}

static int send_request_envelope(
    gzc_client_t *client,
    const gzc_platform_t *platform,
    const gzc_webrtc_vtable_t *webrtc,
    gzc_rtc_channel_t *channel,
    gzc_str_t method,
    gzc_str_t params_payload) {
  gzc_buf_t request;
  gzc_buf_t framed;
  gzc_buf_init(&request);
  gzc_buf_init(&framed);
  int rc = gzc_rpc_encode_request_envelope(platform, gzc_str_from_cstr("1"), method, params_payload, &request);
  if (rc == GZC_OK) {
    rc = append_binary_envelope_frame(platform, &framed, request.data, request.len);
  }
  if (rc == GZC_OK) {
    rc = append_frame(platform, &framed, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  if (rc == GZC_OK) {
    rc = gzc_client_reset_rpc_rx_internal(client);
  }
  if (rc == GZC_OK) {
    rc = webrtc->channel_send(channel, framed.data, framed.len, false);
  }
  gzc_buf_free(&request, platform);
  gzc_buf_free(&framed, platform);
  return rc;
}

int gzc_rpc_encode_request_envelope(
    const gzc_platform_t *platform,
    gzc_str_t id,
    gzc_str_t method,
    gzc_str_t params_payload,
    gzc_buf_t *out_payload) {
  if (out_payload == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (platform == NULL) {
    platform = gzc_default_platform();
  }
  unsigned id_value = 0;
  int rc = method_id(method, &id_value);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = append_proto_str(platform, out_payload, 1, id);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = append_proto_u32(platform, out_payload, 2, id_value);
  if (rc != GZC_OK) {
    return rc;
  }
  return append_proto_bytes(platform, out_payload, 3, (const uint8_t *)params_payload.data, params_payload.len);
}

int gzc_rpc_decode_response_envelope(gzc_str_t response_payload, gzc_rpc_response_t *out_response) {
  if (out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_response, 0, sizeof(*out_response));
  size_t offset = 0;
  while (offset < response_payload.len) {
    uint64_t key = 0;
    int rc = read_varint((const uint8_t *)response_payload.data, response_payload.len, &offset, &key);
    if (rc != GZC_OK) {
      return rc;
    }
    unsigned field = (unsigned)(key >> 3);
    unsigned wire_type = (unsigned)(key & 0x7u);
    if (field == 1 && wire_type == 2) {
      rc = read_proto_bytes((const uint8_t *)response_payload.data, response_payload.len, &offset, &out_response->id);
    } else if (field == 2 && wire_type == 2) {
      rc = read_proto_bytes((const uint8_t *)response_payload.data, response_payload.len, &offset, &out_response->result_payload);
    } else if (field == 3 && wire_type == 2) {
      gzc_str_t error_payload;
      rc = read_proto_bytes((const uint8_t *)response_payload.data, response_payload.len, &offset, &error_payload);
      if (rc != GZC_OK) {
        return rc;
      }
      out_response->has_error = true;
      size_t error_offset = 0;
      while (error_offset < error_payload.len) {
        uint64_t error_key = 0;
        rc = read_varint((const uint8_t *)error_payload.data, error_payload.len, &error_offset, &error_key);
        if (rc != GZC_OK) {
          return rc;
        }
        unsigned error_field = (unsigned)(error_key >> 3);
        unsigned error_wire_type = (unsigned)(error_key & 0x7u);
        if (error_field == 1 && error_wire_type == 0) {
          uint64_t code = 0;
          rc = read_varint((const uint8_t *)error_payload.data, error_payload.len, &error_offset, &code);
          out_response->error.code = (int)(int32_t)code;
        } else if (error_field == 2 && error_wire_type == 2) {
          rc = read_proto_bytes((const uint8_t *)error_payload.data, error_payload.len, &error_offset, &out_response->error.message);
        } else {
          rc = skip_proto_field((const uint8_t *)error_payload.data, error_payload.len, &error_offset, error_wire_type);
        }
        if (rc != GZC_OK) {
          return rc;
        }
      }
    } else {
      rc = skip_proto_field((const uint8_t *)response_payload.data, response_payload.len, &offset, wire_type);
    }
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_OK;
}

int gzc_rpc_call(gzc_client_t *client, gzc_str_t method, gzc_str_t params_payload, gzc_rpc_response_t *out_response) {
  if (client == NULL || out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  const gzc_webrtc_vtable_t *webrtc = gzc_client_webrtc(client);
  if (platform == NULL || webrtc == NULL || webrtc->channel_send == NULL) {
    return GZC_ERR_CLOSED;
  }
  int rc = gzc_client_open_rpc_channel_internal(client, 5000);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_rtc_channel_t *channel = gzc_client_rpc_channel(client);
  if (channel == NULL) {
    gzc_client_close_rpc_channel_internal(client);
    return GZC_ERR_CLOSED;
  }
  rc = send_request_envelope(client, platform, webrtc, channel, method, params_payload);
  if (rc != GZC_OK) {
    gzc_client_close_rpc_channel_internal(client);
    return rc;
  }

  gzc_buf_t frame_bytes;
  gzc_buf_t envelope;
  gzc_buf_init(&frame_bytes);
  gzc_buf_init(&envelope);
  gzc_rpc_frame_t frame;
  bool saw_response = false;
  bool saw_continuation = false;
  for (;;) {
    rc = read_frame(client, platform, 5000, &frame_bytes, &frame);
    if (rc != GZC_OK) {
      break;
    }
    if (frame.type == GZC_RPC_FRAME_EOS) {
      if (saw_continuation && !saw_response) {
        gzc_str_t response_payload;
        rc = gzc_client_store_rpc_response_internal(client, envelope.data, envelope.len, &response_payload);
        if (rc != GZC_OK) {
          break;
        }
        rc = gzc_rpc_decode_response_envelope(response_payload, out_response);
        if (rc != GZC_OK) {
          break;
        }
        saw_response = true;
      }
      rc = saw_response ? GZC_OK : GZC_ERR_RPC;
      break;
    }
    if (frame.type == GZC_RPC_FRAME_TEXT) {
      if (saw_response) {
        rc = GZC_ERR_RPC;
        break;
      }
      saw_continuation = true;
      rc = gzc_buf_append(&envelope, platform, frame.data, frame.len);
      if (rc != GZC_OK) {
        break;
      }
      continue;
    }
    if (frame.type != GZC_RPC_FRAME_BINARY || saw_response || saw_continuation) {
      rc = GZC_ERR_RPC;
      break;
    }
    gzc_str_t response_payload;
    rc = gzc_client_store_rpc_response_internal(client, frame.data, frame.len, &response_payload);
    if (rc != GZC_OK) {
      break;
    }
    rc = gzc_rpc_decode_response_envelope(response_payload, out_response);
    if (rc != GZC_OK) {
      break;
    }
    saw_response = true;
    rc = GZC_OK;
    continue;
  }
  gzc_buf_free(&envelope, platform);
  gzc_buf_free(&frame_bytes, platform);
  close_rpc_channel_on_error(client, rc);
  return rc;
}

int gzc_rpc_call_stream(
    gzc_client_t *client,
    gzc_str_t method,
    gzc_str_t params_payload,
    gzc_rpc_frame_cb on_frame,
    void *userdata) {
  if (client == NULL || on_frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  const gzc_webrtc_vtable_t *webrtc = gzc_client_webrtc(client);
  if (platform == NULL || webrtc == NULL || webrtc->channel_send == NULL) {
    return GZC_ERR_CLOSED;
  }
  int rc = gzc_client_open_rpc_channel_internal(client, 5000);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_rtc_channel_t *channel = gzc_client_rpc_channel(client);
  if (channel == NULL) {
    gzc_client_close_rpc_channel_internal(client);
    return GZC_ERR_CLOSED;
  }
  rc = send_request_envelope(client, platform, webrtc, channel, method, params_payload);
  if (rc != GZC_OK) {
    gzc_client_close_rpc_channel_internal(client);
    return rc;
  }
  gzc_buf_t frame_bytes;
  gzc_buf_init(&frame_bytes);
  gzc_rpc_frame_t frame;
  bool saw_response = false;
  for (;;) {
    rc = read_frame(client, platform, 5000, &frame_bytes, &frame);
    if (rc != GZC_OK) {
      break;
    }
    if (frame.type == GZC_RPC_FRAME_EOS) {
      rc = saw_response ? GZC_OK : GZC_ERR_RPC;
      break;
    }
    if (!saw_response) {
      if (frame.type != GZC_RPC_FRAME_BINARY) {
        rc = GZC_ERR_RPC;
        break;
      }
      gzc_rpc_response_t response;
      rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts((const char *)frame.data, frame.len), &response);
      if (rc != GZC_OK) {
        break;
      }
      if (response.has_error) {
        rc = GZC_ERR_RPC;
        break;
      }
      saw_response = true;
      rc = on_frame(userdata, &frame);
      if (rc != GZC_OK) {
        break;
      }
      continue;
    }
    if (frame.type == GZC_RPC_FRAME_JSON || frame.type == GZC_RPC_FRAME_TEXT) {
      rc = GZC_ERR_RPC;
      break;
    }
    rc = on_frame(userdata, &frame);
    if (rc != GZC_OK) {
      break;
    }
  }
  gzc_buf_free(&frame_bytes, platform);
  close_rpc_channel_on_error(client, rc);
  return rc;
}

int gzc_rpc_send_frame(gzc_client_t *client, const gzc_rpc_frame_t *frame) {
  if (client == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  const gzc_webrtc_vtable_t *webrtc = gzc_client_webrtc(client);
  gzc_rtc_channel_t *channel = gzc_client_rpc_channel(client);
  if (platform == NULL || webrtc == NULL || channel == NULL || webrtc->channel_send == NULL) {
    return GZC_ERR_CLOSED;
  }
  gzc_buf_t framed;
  gzc_buf_init(&framed);
  int rc = gzc_rpc_frame_encode(platform, frame, &framed);
  if (rc == GZC_OK) {
    rc = webrtc->channel_send(channel, framed.data, framed.len, false);
  }
  gzc_buf_free(&framed, platform);
  return rc;
}

void gzc_rpc_response_free(gzc_client_t *client, gzc_rpc_response_t *response) {
  (void)client;
  if (response != NULL) {
    memset(response, 0, sizeof(*response));
  }
}
