#include "gzc_rpc.h"

#include <string.h>

int gzc_client_reset_rpc_rx_internal(gzc_client_t *client);
int gzc_client_open_rpc_channel_internal(gzc_client_t *client, int timeout_ms);
void gzc_client_close_rpc_channel_internal(gzc_client_t *client);
int gzc_client_read_rpc_frame_internal(gzc_client_t *client, int timeout_ms, gzc_buf_t *out_frame_bytes);
int gzc_client_store_rpc_response_internal(gzc_client_t *client, const uint8_t *data, size_t len, gzc_str_t *out_json);

static int append_frame(const gzc_platform_t *platform, gzc_buf_t *out, gzc_rpc_frame_type_t type, const uint8_t *data, size_t len) {
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.type = type;
  frame.data = data;
  frame.len = len;
  return gzc_rpc_frame_encode(platform, &frame, out);
}

static int append_json_envelope_frames(const gzc_platform_t *platform, gzc_buf_t *out, const uint8_t *data, size_t len) {
  if (len <= GZC_RPC_MAX_FRAME_SIZE) {
    return append_frame(platform, out, GZC_RPC_FRAME_JSON, data, len);
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

static int send_request_envelope(
    gzc_client_t *client,
    const gzc_platform_t *platform,
    const gzc_webrtc_vtable_t *webrtc,
    gzc_rtc_channel_t *channel,
    gzc_str_t method,
    gzc_str_t params_json) {
  gzc_buf_t request;
  gzc_buf_t framed;
  gzc_buf_init(&request);
  gzc_buf_init(&framed);
  int rc = gzc_rpc_encode_request_envelope(platform, gzc_str_from_cstr("1"), method, params_json, &request);
  if (rc == GZC_OK) {
    rc = append_json_envelope_frames(platform, &framed, request.data, request.len);
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
    gzc_str_t params_json,
    gzc_buf_t *out_json) {
  if (out_json == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (platform == NULL) {
    platform = gzc_default_platform();
  }
  gzc_json_writer_t writer;
  gzc_json_writer_init(&writer, platform, out_json);
  int rc = gzc_json_object_begin(&writer);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_json_field_i32(&writer, "v", GZC_API_VERSION);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_json_field_str(&writer, "id", id);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_json_field_str(&writer, "method", method);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_json_field_raw(&writer, "params", params_json);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_json_object_end(&writer);
}

int gzc_rpc_decode_response_envelope(gzc_str_t response_json, gzc_rpc_response_t *out_response) {
  if (out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_response, 0, sizeof(*out_response));
  gzc_str_t raw;
  int rc = gzc_json_find_field(response_json, "id", &raw);
  if (rc == GZC_OK) {
    rc = gzc_json_parse_string(raw, &out_response->id);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  rc = gzc_json_find_field(response_json, "error", &raw);
  if (rc == GZC_OK && !(raw.len == 4 && memcmp(raw.data, "null", 4) == 0)) {
    out_response->has_error = true;
    gzc_str_t error_field;
    if (gzc_json_find_field(raw, "code", &error_field) == GZC_OK) {
      int64_t code = 0;
      rc = gzc_json_parse_i64(error_field, &code);
      if (rc != GZC_OK) {
        return rc;
      }
      out_response->error.code = (int)code;
    }
    if (gzc_json_find_field(raw, "message", &error_field) == GZC_OK) {
      rc = gzc_json_parse_string(error_field, &out_response->error.message);
      if (rc != GZC_OK) {
        return rc;
      }
    }
    if (gzc_json_find_field(raw, "data", &error_field) == GZC_OK) {
      out_response->error.data_json = error_field;
    }
    return GZC_OK;
  }
  rc = gzc_json_find_field(response_json, "result", &raw);
  if (rc == GZC_OK) {
    out_response->result_json = raw;
  }
  return GZC_OK;
}

int gzc_rpc_call_json(gzc_client_t *client, gzc_str_t method, gzc_str_t params_json, gzc_rpc_response_t *out_response) {
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
  rc = send_request_envelope(client, platform, webrtc, channel, method, params_json);
  if (rc != GZC_OK) {
    gzc_client_close_rpc_channel_internal(client);
    return rc;
  }

  gzc_buf_t frame_bytes;
  gzc_buf_t text_response;
  gzc_buf_init(&frame_bytes);
  gzc_buf_init(&text_response);
  gzc_rpc_frame_t frame;
  bool saw_response = false;
  bool reading_text = false;
  for (;;) {
    rc = read_frame(client, platform, 5000, &frame_bytes, &frame);
    if (rc != GZC_OK) {
      break;
    }
    if (frame.type == GZC_RPC_FRAME_EOS) {
      if (reading_text) {
        gzc_str_t response_json;
        rc = gzc_client_store_rpc_response_internal(client, text_response.data, text_response.len, &response_json);
        if (rc == GZC_OK) {
          rc = gzc_rpc_decode_response_envelope(response_json, out_response);
        }
        if (rc != GZC_OK) {
          break;
        }
        saw_response = true;
        reading_text = false;
      }
      rc = saw_response ? GZC_OK : GZC_ERR_RPC;
      break;
    }
    if (reading_text) {
      if (frame.type != GZC_RPC_FRAME_TEXT) {
        rc = GZC_ERR_RPC;
        break;
      }
      rc = gzc_buf_append(&text_response, platform, frame.data, frame.len);
      if (rc != GZC_OK) {
        break;
      }
      continue;
    }
    if (frame.type != GZC_RPC_FRAME_JSON) {
      if (frame.type != GZC_RPC_FRAME_TEXT || saw_response) {
        rc = GZC_ERR_RPC;
        break;
      }
      reading_text = true;
      rc = gzc_buf_append(&text_response, platform, frame.data, frame.len);
      if (rc != GZC_OK) {
        break;
      }
      continue;
    }
    if (saw_response) {
      rc = GZC_ERR_RPC;
      break;
    }
    gzc_str_t response_json;
    rc = gzc_client_store_rpc_response_internal(client, frame.data, frame.len, &response_json);
    if (rc != GZC_OK) {
      break;
    }
    rc = gzc_rpc_decode_response_envelope(response_json, out_response);
    if (rc != GZC_OK) {
      break;
    }
    saw_response = true;
    rc = GZC_OK;
    break;
  }
  gzc_buf_free(&text_response, platform);
  gzc_buf_free(&frame_bytes, platform);
  gzc_client_close_rpc_channel_internal(client);
  return rc;
}

int gzc_rpc_call_stream(
    gzc_client_t *client,
    gzc_str_t method,
    gzc_str_t params_json,
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
  rc = send_request_envelope(client, platform, webrtc, channel, method, params_json);
  if (rc != GZC_OK) {
    gzc_client_close_rpc_channel_internal(client);
    return rc;
  }
  gzc_buf_t frame_bytes;
  gzc_buf_init(&frame_bytes);
  gzc_rpc_frame_t frame;
  for (;;) {
    rc = read_frame(client, platform, 5000, &frame_bytes, &frame);
    if (rc != GZC_OK) {
      break;
    }
    if (frame.type == GZC_RPC_FRAME_EOS) {
      rc = GZC_OK;
      break;
    }
    rc = on_frame(userdata, &frame);
    if (rc != GZC_OK) {
      break;
    }
  }
  gzc_buf_free(&frame_bytes, platform);
  gzc_client_close_rpc_channel_internal(client);
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
