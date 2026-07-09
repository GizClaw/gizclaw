#include "gzc_rpc.h"

#include "pb_decode.h"
#include "pb_encode.h"

#include <stdint.h>
#include <string.h>

int gzc_client_reset_rpc_rx_internal(gzc_client_t *client);
int gzc_client_open_rpc_channel_internal(gzc_client_t *client, int timeout_ms);
void gzc_client_close_rpc_channel_internal(gzc_client_t *client);
int gzc_client_read_rpc_frame_internal(gzc_client_t *client, int timeout_ms, gzc_buf_t *out_frame_bytes);
int gzc_client_store_rpc_response_internal(gzc_client_t *client, const uint8_t *data, size_t len, gzc_str_t *out_payload);

typedef struct {
  const uint8_t *data;
  size_t len;
} gzc_pb_bytes_arg_t;

static int append_frame(const gzc_platform_t *platform, gzc_buf_t *out, gzc_rpc_frame_type_t type, const uint8_t *data, size_t len) {
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.type = type;
  frame.data = data;
  frame.len = len;
  return gzc_rpc_frame_encode(platform, &frame, out);
}

static int append_binary_envelope_frame(const gzc_platform_t *platform, gzc_buf_t *out, const uint8_t *data, size_t len) {
  if (len <= GZC_RPC_MAX_FRAME_SIZE) {
    return append_frame(platform, out, GZC_RPC_FRAME_BINARY, data, len);
  }
  size_t offset = 0;
  while (offset < len) {
    size_t chunk = len - offset;
    if (chunk > GZC_RPC_MAX_FRAME_SIZE) {
      chunk = GZC_RPC_MAX_FRAME_SIZE;
    }
    int rc = append_frame(platform, out, GZC_RPC_FRAME_TEXT, data + offset, chunk);
    if (rc != GZC_OK) {
      return rc;
    }
    offset += chunk;
  }
  return GZC_OK;
}

static bool encode_pb_bytes(pb_ostream_t *stream, const pb_field_t *field, void *const *arg) {
  const gzc_pb_bytes_arg_t *bytes = (const gzc_pb_bytes_arg_t *)(*arg);
  const uint8_t *data = bytes != NULL && bytes->data != NULL ? bytes->data : (const uint8_t *)"";
  size_t len = bytes != NULL ? bytes->len : 0;
  return pb_encode_tag_for_field(stream, field) && pb_encode_string(stream, data, len);
}

static int encode_pb_message(
    const gzc_platform_t *platform,
    const pb_msgdesc_t *fields,
    const void *message,
    gzc_buf_t *out_payload) {
  pb_ostream_t sizing = PB_OSTREAM_SIZING;
  if (!pb_encode(&sizing, fields, message)) {
    return GZC_ERR_RPC;
  }
  size_t size = sizing.bytes_written;
  uint8_t *buf = (uint8_t *)platform->malloc(platform->userdata, size == 0 ? 1 : size);
  if (buf == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  pb_ostream_t stream = pb_ostream_from_buffer(buf, size);
  int rc = GZC_OK;
  if (!pb_encode(&stream, fields, message)) {
    rc = GZC_ERR_RPC;
  } else {
    rc = gzc_buf_append(out_payload, platform, buf, size);
  }
  platform->free(platform->userdata, buf);
  return rc;
}

static int read_pb_varint(const uint8_t *data, size_t len, size_t *offset, uint64_t *out) {
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

static int read_pb_bytes(const uint8_t *data, size_t len, size_t *offset, gzc_str_t *out) {
  uint64_t size = 0;
  int rc = read_pb_varint(data, len, offset, &size);
  if (rc != GZC_OK || size > len - *offset) {
    return GZC_ERR_RPC;
  }
  *out = gzc_str_from_parts((const char *)(data + *offset), (size_t)size);
  *offset += (size_t)size;
  return GZC_OK;
}

static int skip_pb_value(const uint8_t *data, size_t len, size_t *offset, unsigned wire_type) {
  uint64_t ignored = 0;
  switch (wire_type) {
    case 0:
      return read_pb_varint(data, len, offset, &ignored);
    case 1:
      if (len - *offset < 8) {
        return GZC_ERR_RPC;
      }
      *offset += 8;
      return GZC_OK;
    case 2:
      return read_pb_bytes(data, len, offset, &(gzc_str_t){0});
    case 5:
      if (len - *offset < 4) {
        return GZC_ERR_RPC;
      }
      *offset += 4;
      return GZC_OK;
    default:
      return GZC_ERR_RPC;
  }
}

static int decode_rpc_error_payload(gzc_str_t payload, gzc_rpc_error_t *out_error) {
  const uint8_t *data = (const uint8_t *)payload.data;
  size_t offset = 0;
  while (offset < payload.len) {
    uint64_t key = 0;
    int rc = read_pb_varint(data, payload.len, &offset, &key);
    if (rc != GZC_OK) {
      return rc;
    }
    unsigned field = (unsigned)(key >> 3);
    unsigned wire_type = (unsigned)(key & 0x7u);
    if (field == 1) {
      if (wire_type != 0) {
        return GZC_ERR_RPC;
      }
      uint64_t code = 0;
      rc = read_pb_varint(data, payload.len, &offset, &code);
      if (rc != GZC_OK) {
        return rc;
      }
      out_error->code = (int)(int32_t)code;
    } else if (field == 2) {
      if (wire_type != 2) {
        return GZC_ERR_RPC;
      }
      rc = read_pb_bytes(data, payload.len, &offset, &out_error->message);
      if (rc != GZC_OK) {
        return rc;
      }
    } else {
      rc = skip_pb_value(data, payload.len, &offset, wire_type);
      if (rc != GZC_OK) {
        return rc;
      }
    }
  }
  return GZC_OK;
}

static int decode_rpc_response_payload(gzc_str_t payload, gzc_rpc_response_t *out_response) {
  const uint8_t *data = (const uint8_t *)payload.data;
  size_t offset = 0;
  while (offset < payload.len) {
    uint64_t key = 0;
    int rc = read_pb_varint(data, payload.len, &offset, &key);
    if (rc != GZC_OK) {
      return rc;
    }
    unsigned field = (unsigned)(key >> 3);
    unsigned wire_type = (unsigned)(key & 0x7u);
    if (field == 1) {
      if (wire_type != 2) {
        return GZC_ERR_RPC;
      }
      rc = read_pb_bytes(data, payload.len, &offset, &out_response->id);
      if (rc != GZC_OK) {
        return rc;
      }
    } else if (field == 2) {
      if (wire_type != 2) {
        return GZC_ERR_RPC;
      }
      rc = read_pb_bytes(data, payload.len, &offset, &out_response->result_payload);
      if (rc != GZC_OK) {
        return rc;
      }
    } else if (field == 3) {
      if (wire_type != 2) {
        return GZC_ERR_RPC;
      }
      gzc_str_t error_payload;
      rc = read_pb_bytes(data, payload.len, &offset, &error_payload);
      if (rc != GZC_OK) {
        return rc;
      }
      out_response->has_error = true;
      rc = decode_rpc_error_payload(error_payload, &out_response->error);
      if (rc != GZC_OK) {
        return rc;
      }
    } else {
      rc = skip_pb_value(data, payload.len, &offset, wire_type);
      if (rc != GZC_OK) {
        return rc;
      }
    }
  }
  return GZC_OK;
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
    gizclaw_rpc_v1_RpcMethod method,
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
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_buf_t *out_payload) {
  if (out_payload == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (platform == NULL) {
    platform = gzc_default_platform();
  }
  if (platform->malloc == NULL || platform->free == NULL || method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_UNSPECIFIED) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_pb_bytes_arg_t id_arg = {(const uint8_t *)id.data, id.len};
  gzc_pb_bytes_arg_t payload_arg = {(const uint8_t *)params_payload.data, params_payload.len};
  gizclaw_rpc_v1_RpcRequest request = gizclaw_rpc_v1_RpcRequest_init_zero;
  request.id.funcs.encode = encode_pb_bytes;
  request.id.arg = &id_arg;
  request.method = method;
  request.payload.funcs.encode = encode_pb_bytes;
  request.payload.arg = &payload_arg;
  return encode_pb_message(platform, gizclaw_rpc_v1_RpcRequest_fields, &request, out_payload);
}

int gzc_rpc_decode_response_envelope(gzc_str_t response_payload, gzc_rpc_response_t *out_response) {
  if (out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_response, 0, sizeof(*out_response));
  return decode_rpc_response_payload(response_payload, out_response);
}

int gzc_rpc_call(gzc_client_t *client, gizclaw_rpc_v1_RpcMethod method, gzc_str_t params_payload, gzc_rpc_response_t *out_response) {
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
    gizclaw_rpc_v1_RpcMethod method,
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
  gzc_buf_t envelope;
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
        gzc_rpc_frame_t response_frame;
        memset(&response_frame, 0, sizeof(response_frame));
        response_frame.type = GZC_RPC_FRAME_BINARY;
        response_frame.data = envelope.data;
        response_frame.len = envelope.len;
        gzc_rpc_response_t response;
        rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts((const char *)response_frame.data, response_frame.len), &response);
        if (rc != GZC_OK) {
          break;
        }
        saw_response = true;
        rc = on_frame(userdata, &response_frame);
        if (rc != GZC_OK) {
          break;
        }
        if (response.has_error) {
          rc = GZC_ERR_RPC;
          break;
        }
        continue;
      }
      rc = saw_response ? GZC_OK : GZC_ERR_RPC;
      break;
    }
    if (!saw_response) {
      if (frame.type == GZC_RPC_FRAME_TEXT) {
        saw_continuation = true;
        rc = gzc_buf_append(&envelope, platform, frame.data, frame.len);
        if (rc != GZC_OK) {
          break;
        }
        continue;
      }
      if (frame.type != GZC_RPC_FRAME_BINARY || saw_continuation) {
        rc = GZC_ERR_RPC;
        break;
      }
      gzc_rpc_response_t response;
      rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts((const char *)frame.data, frame.len), &response);
      if (rc != GZC_OK) {
        break;
      }
      saw_response = true;
      rc = on_frame(userdata, &frame);
      if (rc != GZC_OK) {
        break;
      }
      if (response.has_error) {
        rc = GZC_ERR_RPC;
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
  gzc_buf_free(&envelope, platform);
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
