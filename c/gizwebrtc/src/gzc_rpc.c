#include "gzc_rpc.h"

#include <string.h>

int gzc_client_wait_rpc_response_internal(gzc_client_t *client, int timeout_ms, gzc_str_t *out_json);

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
  gzc_rtc_channel_t *channel = gzc_client_rpc_channel(client);
  if (platform == NULL || webrtc == NULL || channel == NULL || webrtc->channel_send == NULL) {
    return GZC_ERR_CLOSED;
  }
  gzc_buf_t request;
  gzc_buf_init(&request);
  int rc = gzc_rpc_encode_request_envelope(platform, gzc_str_from_cstr("1"), method, params_json, &request);
  if (rc == GZC_OK) {
    rc = webrtc->channel_send(channel, request.data, request.len, true);
  }
  gzc_buf_free(&request, platform);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t response_json;
  rc = gzc_client_wait_rpc_response_internal(client, 5000, &response_json);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_rpc_decode_response_envelope(response_json, out_response);
}

void gzc_rpc_response_free(gzc_client_t *client, gzc_rpc_response_t *response) {
  (void)client;
  if (response != NULL) {
    memset(response, 0, sizeof(*response));
  }
}
