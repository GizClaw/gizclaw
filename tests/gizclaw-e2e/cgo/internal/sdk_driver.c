#include "sdk_driver.h"

#include "bridge.h"
#include "gzc.h"
#include "gzc_rpc_generated.h"

#include <stdio.h>
#include <string.h>

typedef struct {
  gzc_cgo_backend_t backend;
  gzc_http_vtable_t http;
  gzc_webrtc_vtable_t webrtc;
  gzc_client_t *client;
} cgo_sdk_session_t;

typedef struct {
  bool saw_metadata;
  bool saw_binary;
  size_t binary_bytes;
  bool saw_main_firmware_marker;
} firmware_download_state_t;

typedef struct {
  bool saw_ack;
  int64_t expected_down_content_length;
  int64_t expected_up_content_length;
  size_t binary_bytes;
} speed_test_state_t;

static int fail(char *errbuf, unsigned long errbuf_len, const char *message, int rc) {
  if (errbuf != NULL && errbuf_len > 0) {
    (void)snprintf(errbuf, errbuf_len, "%s: %s (%d)", message, gzc_status_string((gzc_status_t)rc), rc);
  }
  return rc == GZC_OK ? GZC_ERR_RPC : rc;
}

static int session_open(cgo_sdk_session_t *session, const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  memset(session, 0, sizeof(*session));
  int rc = gzc_cgo_backend_init(&session->backend, identity_dir);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "backend init", rc);
  }

  gzc_cgo_backend_http_vtable(&session->backend, &session->http);
  gzc_cgo_backend_webrtc_vtable(&session->backend, &session->webrtc);

  gzc_client_config_t config;
  memset(&config, 0, sizeof(config));
  config.signaling_url = gzc_str_from_cstr("http://gizclaw-e2e/webrtc/v1/offer");
  config.platform = gzc_default_platform();
  config.http = &session->http;
  config.webrtc = &session->webrtc;
  config.connect_timeout_ms = 15000;

  rc = gzc_client_create(&config, &session->client);
  if (rc != GZC_OK) {
    gzc_cgo_backend_deinit(&session->backend);
    return fail(errbuf, errbuf_len, "client create", rc);
  }

  rc = gzc_client_connect(session->client);
  if (rc != GZC_OK) {
    gzc_client_destroy(session->client);
    gzc_cgo_backend_deinit(&session->backend);
    session->client = NULL;
    return fail(errbuf, errbuf_len, "client connect", rc);
  }
  return GZC_OK;
}

static void session_close(cgo_sdk_session_t *session) {
  if (session->client != NULL) {
    gzc_client_destroy(session->client);
    session->client = NULL;
  }
  gzc_cgo_backend_deinit(&session->backend);
}

static int call_json(
    cgo_sdk_session_t *session,
    gzc_str_t method,
    int (*encode)(const gzc_platform_t *, const void *, gzc_buf_t *),
    const void *request,
    gzc_rpc_response_t *response,
    char *errbuf,
    unsigned long errbuf_len,
    const char *label) {
  gzc_buf_t params;
  gzc_buf_init(&params);
  int rc = encode(gzc_default_platform(), request, &params);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, label, rc);
  }

  memset(response, 0, sizeof(*response));
  rc = gzc_rpc_call_json(
      session->client,
      method,
      gzc_str_from_parts((const char *)params.data, params.len),
      response);
  gzc_buf_free(&params, gzc_default_platform());
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, label, rc);
  }
  if (response->has_error) {
    return fail(errbuf, errbuf_len, label, GZC_ERR_RPC);
  }
  return GZC_OK;
}

static int encode_ping_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_ping_request_encode_json(platform, (const gzc_ping_request_t *)value, out_json);
}

static int encode_speed_test_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_speed_test_request_encode_json(platform, (const gzc_speed_test_request_t *)value, out_json);
}

static int encode_server_get_runtime_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_server_get_runtime_request_encode_json(platform, (const gzc_server_get_runtime_request_t *)value, out_json);
}

static int encode_server_get_status_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_server_get_status_request_encode_json(platform, (const gzc_server_get_status_request_t *)value, out_json);
}

static int encode_server_put_status_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_server_put_status_request_encode_json(platform, (const gzc_server_put_status_request_t *)value, out_json);
}

static int encode_firmware_list_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_firmware_list_request_encode_json(platform, (const gzc_firmware_list_request_t *)value, out_json);
}

static int encode_firmware_get_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_firmware_get_request_encode_json(platform, (const gzc_firmware_get_request_t *)value, out_json);
}

static int encode_firmware_download_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_firmware_files_download_request_encode_json(platform, (const gzc_firmware_files_download_request_t *)value, out_json);
}

static int encode_server_get_run_status_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_server_get_run_status_request_encode_json(platform, (const gzc_server_get_run_status_request_t *)value, out_json);
}

static int encode_server_get_run_workspace_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_server_get_run_workspace_request_encode_json(platform, (const gzc_server_get_run_workspace_request_t *)value, out_json);
}

static int encode_server_set_run_workspace_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_server_set_run_workspace_request_encode_json(platform, (const gzc_server_set_run_workspace_request_t *)value, out_json);
}

static int encode_server_reload_run_workspace_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_server_reload_run_workspace_request_encode_json(platform, (const gzc_server_reload_run_workspace_request_t *)value, out_json);
}

static int encode_server_list_run_workspace_history_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_server_list_run_workspace_history_request_encode_json(platform, (const gzc_server_list_run_workspace_history_request_t *)value, out_json);
}

static int encode_workspace_get_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_workspace_get_request_encode_json(platform, (const gzc_workspace_get_request_t *)value, out_json);
}

static int encode_contact_create_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_contact_create_request_encode_json(platform, (const gzc_contact_create_request_t *)value, out_json);
}

static int encode_contact_get_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_contact_get_request_encode_json(platform, (const gzc_contact_get_request_t *)value, out_json);
}

static int encode_contact_list_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_contact_list_request_encode_json(platform, (const gzc_contact_list_request_t *)value, out_json);
}

static int encode_friend_invite_token_create_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_invite_token_create_request_encode_json(platform, (const gzc_friend_invite_token_create_request_t *)value, out_json);
}

static int encode_friend_add_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_add_request_encode_json(platform, (const gzc_friend_add_request_t *)value, out_json);
}

static int encode_friend_group_create_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_create_request_encode_json(platform, (const gzc_friend_group_create_request_t *)value, out_json);
}

static int encode_friend_group_get_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_get_request_encode_json(platform, (const gzc_friend_group_get_request_t *)value, out_json);
}

static int encode_friend_group_list_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_list_request_encode_json(platform, (const gzc_friend_group_list_request_t *)value, out_json);
}

static int encode_friend_group_invite_token_create_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_invite_token_create_request_encode_json(platform, (const gzc_friend_group_invite_token_create_request_t *)value, out_json);
}

static int encode_friend_group_join_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_join_request_encode_json(platform, (const gzc_friend_group_join_request_t *)value, out_json);
}

static int encode_friend_group_member_list_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_member_list_request_encode_json(platform, (const gzc_friend_group_member_list_request_t *)value, out_json);
}

static int encode_friend_group_message_send_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_message_send_request_encode_json(platform, (const gzc_friend_group_message_send_request_t *)value, out_json);
}

static int encode_friend_group_message_get_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_message_get_request_encode_json(platform, (const gzc_friend_group_message_get_request_t *)value, out_json);
}

static int encode_friend_group_message_list_request(const gzc_platform_t *platform, const void *value, gzc_buf_t *out_json) {
  return gzc_friend_group_message_list_request_encode_json(platform, (const gzc_friend_group_message_list_request_t *)value, out_json);
}

static bool str_eq(gzc_str_t got, const char *want) {
  size_t want_len = strlen(want);
  return got.len == want_len && strncmp(got.data, want, want_len) == 0;
}

static bool str_nonempty(gzc_str_t got) {
  return got.data != NULL && got.len > 0;
}

static bool raw_contains(gzc_str_t raw, gzc_str_t needle) {
  if (raw.data == NULL || needle.data == NULL || needle.len == 0 || raw.len < needle.len) {
    return false;
  }
  for (size_t i = 0; i <= raw.len - needle.len; i++) {
    if (memcmp(raw.data + i, needle.data, needle.len) == 0) {
      return true;
    }
  }
  return false;
}

static int copy_str_to_storage(char *storage, size_t storage_len, gzc_str_t src, gzc_str_t *out) {
  if (storage == NULL || out == NULL || src.data == NULL || storage_len == 0 || src.len >= storage_len) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memcpy(storage, src.data, src.len);
  storage[src.len] = 0;
  *out = gzc_str_from_parts(storage, src.len);
  return GZC_OK;
}

static uint32_t read_u32_le(const uint8_t *data) {
  return (uint32_t)data[0] | ((uint32_t)data[1] << 8) | ((uint32_t)data[2] << 16) | ((uint32_t)data[3] << 24);
}

static int send_event_json(gzc_service_channel_t *event_channel, const char *json) {
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.type = GZC_RPC_FRAME_JSON;
  frame.data = (const uint8_t *)json;
  frame.len = strlen(json);
  return gzc_service_channel_send_frame(event_channel, &frame);
}

static int send_stamped_opus_packet(cgo_sdk_session_t *session, uint64_t timestamp_ms, const uint8_t *packet, size_t packet_len) {
  gzc_buf_t stamped;
  gzc_buf_init(&stamped);
  uint8_t header[8];
  header[0] = 1;
  header[1] = (uint8_t)((timestamp_ms >> 48) & 0xffu);
  header[2] = (uint8_t)((timestamp_ms >> 40) & 0xffu);
  header[3] = (uint8_t)((timestamp_ms >> 32) & 0xffu);
  header[4] = (uint8_t)((timestamp_ms >> 24) & 0xffu);
  header[5] = (uint8_t)((timestamp_ms >> 16) & 0xffu);
  header[6] = (uint8_t)((timestamp_ms >> 8) & 0xffu);
  header[7] = (uint8_t)(timestamp_ms & 0xffu);
  int rc = gzc_buf_append(&stamped, gzc_default_platform(), header, sizeof(header));
  if (rc == GZC_OK) {
    rc = gzc_buf_append(&stamped, gzc_default_platform(), packet, packet_len);
  }
  if (rc == GZC_OK) {
    rc = gzc_client_send_packet(session->client, 0x10, stamped.data, stamped.len);
  }
  gzc_buf_free(&stamped, gzc_default_platform());
  return rc;
}

static int set_chat_workspace(cgo_sdk_session_t *session, const char *workspace_name, char *errbuf, unsigned long errbuf_len) {
  if (workspace_name == NULL || workspace_name[0] == 0) {
    return fail(errbuf, errbuf_len, "chat workspace name", GZC_ERR_INVALID_ARGUMENT);
  }
  gzc_rpc_response_t response;
  gzc_server_set_run_workspace_request_t set_request;
  memset(&set_request, 0, sizeof(set_request));
  set_request.workspace_name = gzc_str_from_cstr(workspace_name);
  int rc = call_json(
      session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_RUN_WORKSPACE_SET),
      encode_server_set_run_workspace_request,
      &set_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.run.workspace.set for chat roundtrip");
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_server_set_run_workspace_response_t set_response;
  memset(&set_response, 0, sizeof(set_response));
  rc = gzc_server_set_run_workspace_response_decode_json(response.result_json, &set_response);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "decode server.run.workspace.set for chat roundtrip", rc);
  }
  if (!str_eq(set_response.workspace_name, workspace_name)) {
    return fail(errbuf, errbuf_len, "invalid server.run.workspace.set for chat roundtrip", GZC_ERR_RPC);
  }
  gzc_server_reload_run_workspace_request_t reload_request;
  memset(&reload_request, 0, sizeof(reload_request));
  rc = call_json(
      session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD),
      encode_server_reload_run_workspace_request,
      &reload_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.run.workspace.reload for chat roundtrip");
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_server_reload_run_workspace_response_t reload_response;
  memset(&reload_response, 0, sizeof(reload_response));
  rc = gzc_server_reload_run_workspace_response_decode_json(response.result_json, &reload_response);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "decode server.run.workspace.reload for chat roundtrip", rc);
  }
  if (!str_eq(reload_response.workspace_name, workspace_name)) {
    return fail(errbuf, errbuf_len, "invalid server.run.workspace.reload for chat roundtrip", GZC_ERR_RPC);
  }
  return GZC_OK;
}

int gzc_cgo_run_ping(const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_ping_request_t request;
  memset(&request, 0, sizeof(request));
  request.client_send_time = 12345;

  gzc_rpc_response_t response;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_ALL_PING),
      encode_ping_request,
      &request,
      &response,
      errbuf,
      errbuf_len,
      "call all.ping");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }

  gzc_ping_response_t decoded;
  memset(&decoded, 0, sizeof(decoded));
  rc = gzc_ping_response_decode_json(response.result_json, &decoded);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode ping response", rc);
  }
  if (decoded.server_time <= 0) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid ping server_time", GZC_ERR_RPC);
  }

  session_close(&session);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_run_server_runtime(const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_server_get_runtime_request_t request;
  memset(&request, 0, sizeof(request));
  gzc_rpc_response_t response;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_RUNTIME_GET),
      encode_server_get_runtime_request,
      &request,
      &response,
      errbuf,
      errbuf_len,
      "call server.runtime.get");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }

  gzc_server_get_runtime_response_t runtime;
  memset(&runtime, 0, sizeof(runtime));
  rc = gzc_server_get_runtime_response_decode_json(response.result_json, &runtime);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.runtime.get", rc);
  }
  if (!runtime.online || runtime.last_seen_at.len == 0) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.runtime.get", GZC_ERR_RPC);
  }

  session_close(&session);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_run_server_status(const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_server_put_status_request_t put_request;
  memset(&put_request, 0, sizeof(put_request));
  put_request.has_volume = true;
  put_request.volume = 42;
  put_request.has_battery_percent = true;
  put_request.battery_percent = 87;
  put_request.has_charging = true;
  put_request.charging = true;
  put_request.has_muted = true;
  put_request.muted = false;
  put_request.has_labels = true;
  put_request.labels.raw = gzc_str_from_cstr("{\"mode\":\"cgo-rpc\"}");

  gzc_rpc_response_t response;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_STATUS_PUT),
      encode_server_put_status_request,
      &put_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.status.put");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_server_put_status_response_t put_response;
  memset(&put_response, 0, sizeof(put_response));
  rc = gzc_server_put_status_response_decode_json(response.result_json, &put_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.status.put", rc);
  }
  if (!put_response.has_volume || put_response.volume != 42 || !put_response.has_battery_percent ||
      put_response.battery_percent != 87 || !put_response.has_charging || !put_response.charging ||
      !put_response.has_muted || put_response.muted) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.status.put", GZC_ERR_RPC);
  }
  session_close(&session);

  rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_server_get_status_request_t get_request;
  memset(&get_request, 0, sizeof(get_request));
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_STATUS_GET),
      encode_server_get_status_request,
      &get_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.status.get");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_server_get_status_response_t get_response;
  memset(&get_response, 0, sizeof(get_response));
  rc = gzc_server_get_status_response_decode_json(response.result_json, &get_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.status.get", rc);
  }
  if (!get_response.has_volume || get_response.volume != 42 || !get_response.has_battery_percent ||
      get_response.battery_percent != 87 || !get_response.has_labels || get_response.labels.raw.len == 0) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.status.get", GZC_ERR_RPC);
  }

  session_close(&session);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

static int speed_test_frame_cb(void *userdata, const gzc_rpc_frame_t *frame) {
  speed_test_state_t *state = (speed_test_state_t *)userdata;
  if (state == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (frame->type == GZC_RPC_FRAME_JSON) {
    if (state->saw_ack) {
      return GZC_ERR_RPC;
    }
    gzc_rpc_response_t response;
    memset(&response, 0, sizeof(response));
    int rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts((const char *)frame->data, frame->len), &response);
    if (rc != GZC_OK || response.has_error) {
      return rc == GZC_OK ? GZC_ERR_RPC : rc;
    }
    gzc_speed_test_response_t decoded;
    memset(&decoded, 0, sizeof(decoded));
    rc = gzc_speed_test_response_decode_json(response.result_json, &decoded);
    if (rc != GZC_OK) {
      return rc;
    }
    if (decoded.down_content_length != state->expected_down_content_length ||
        decoded.up_content_length != state->expected_up_content_length) {
      return GZC_ERR_RPC;
    }
    state->saw_ack = true;
    return GZC_OK;
  }
  if (frame->type != GZC_RPC_FRAME_BINARY) {
    return GZC_ERR_RPC;
  }
  state->binary_bytes += frame->len;
  return GZC_OK;
}

int gzc_cgo_run_speed_test(const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_speed_test_request_t request;
  memset(&request, 0, sizeof(request));
  request.down_content_length = 4096;
  request.up_content_length = 0;

  gzc_buf_t params;
  gzc_buf_init(&params);
  rc = encode_speed_test_request(gzc_default_platform(), &request, &params);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "encode speed test request", rc);
  }

  speed_test_state_t state;
  memset(&state, 0, sizeof(state));
  state.expected_down_content_length = request.down_content_length;
  state.expected_up_content_length = request.up_content_length;
  rc = gzc_rpc_call_stream(
      session.client,
      gzc_str_from_cstr(GZC_RPC_METHOD_ALL_SPEED_TEST_RUN),
      gzc_str_from_parts((const char *)params.data, params.len),
      speed_test_frame_cb,
      &state);
  gzc_buf_free(&params, gzc_default_platform());
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "call all.speed_test.run", rc);
  }
  if (!state.saw_ack || state.binary_bytes != (size_t)request.down_content_length) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid all.speed_test.run stream", GZC_ERR_RPC);
  }

  session_close(&session);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_run_firmware_json(const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_firmware_list_request_t list_request;
  memset(&list_request, 0, sizeof(list_request));
  list_request.has_limit = true;
  list_request.limit = 5;

  gzc_rpc_response_t response;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FIRMWARE_LIST),
      encode_firmware_list_request,
      &list_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.firmware.list");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_firmware_list_response_t list_response;
  memset(&list_response, 0, sizeof(list_response));
  rc = gzc_firmware_list_response_decode_json(response.result_json, &list_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.firmware.list", rc);
  }
  if (list_response.items.raw.len <= 2) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "empty server.firmware.list", GZC_ERR_RPC);
  }

  gzc_firmware_get_request_t get_request;
  memset(&get_request, 0, sizeof(get_request));
  get_request.firmware_id = gzc_str_from_cstr("devkit-firmware-main");
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FIRMWARE_GET),
      encode_firmware_get_request,
      &get_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.firmware.get");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_firmware_get_response_t get_response;
  memset(&get_response, 0, sizeof(get_response));
  rc = gzc_firmware_get_response_decode_json(response.result_json, &get_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.firmware.get", rc);
  }
  if (get_response.name.len != strlen("devkit-firmware-main") ||
      strncmp(get_response.name.data, "devkit-firmware-main", get_response.name.len) != 0 ||
      get_response.slots.raw.len == 0) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.firmware.get", GZC_ERR_RPC);
  }

  session_close(&session);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

static bool bytes_contains(const uint8_t *data, size_t len, const char *needle) {
  size_t needle_len = strlen(needle);
  if (needle_len == 0 || len < needle_len) {
    return false;
  }
  for (size_t i = 0; i <= len - needle_len; i++) {
    if (memcmp(data + i, needle, needle_len) == 0) {
      return true;
    }
  }
  return false;
}

static int firmware_download_frame_cb(void *userdata, const gzc_rpc_frame_t *frame) {
  firmware_download_state_t *state = (firmware_download_state_t *)userdata;
  if (state == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (frame->type == GZC_RPC_FRAME_JSON) {
    if (state->saw_metadata) {
      return GZC_ERR_RPC;
    }
    gzc_rpc_response_t response;
    memset(&response, 0, sizeof(response));
    int rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts((const char *)frame->data, frame->len), &response);
    if (rc != GZC_OK || response.has_error) {
      return rc == GZC_OK ? GZC_ERR_RPC : rc;
    }
    gzc_firmware_files_download_response_t metadata;
    memset(&metadata, 0, sizeof(metadata));
    rc = gzc_firmware_files_download_response_decode_json(response.result_json, &metadata);
    if (rc != GZC_OK) {
      return rc;
    }
    if (metadata.firmware_id.len != strlen("devkit-firmware-main") ||
        strncmp(metadata.firmware_id.data, "devkit-firmware-main", metadata.firmware_id.len) != 0 ||
        metadata.path.len != strlen("firmware/main.bin") ||
        strncmp(metadata.path.data, "firmware/main.bin", metadata.path.len) != 0) {
      return GZC_ERR_RPC;
    }
    state->saw_metadata = true;
    return GZC_OK;
  }
  if (frame->type != GZC_RPC_FRAME_BINARY) {
    return GZC_ERR_RPC;
  }
  state->saw_binary = true;
  state->binary_bytes += frame->len;
  if (bytes_contains(frame->data, frame->len, "GIZCLAW_MAIN_FIRMWARE_V1")) {
    state->saw_main_firmware_marker = true;
  }
  return GZC_OK;
}

int gzc_cgo_run_firmware_download(const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_firmware_files_download_request_t request;
  memset(&request, 0, sizeof(request));
  request.channel.raw = gzc_str_from_cstr("\"stable\"");
  request.firmware_id = gzc_str_from_cstr("devkit-firmware-main");
  request.path = gzc_str_from_cstr("firmware/main.bin");

  gzc_buf_t params;
  gzc_buf_init(&params);
  rc = encode_firmware_download_request(gzc_default_platform(), &request, &params);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "encode firmware download request", rc);
  }

  firmware_download_state_t state;
  memset(&state, 0, sizeof(state));
  rc = gzc_rpc_call_stream(
      session.client,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FIRMWARE_FILES_DOWNLOAD),
      gzc_str_from_parts((const char *)params.data, params.len),
      firmware_download_frame_cb,
      &state);
  gzc_buf_free(&params, gzc_default_platform());
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "call server.firmware.files.download", rc);
  }
  if (!state.saw_metadata || !state.saw_binary || state.binary_bytes == 0 || !state.saw_main_firmware_marker) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid firmware download stream", GZC_ERR_RPC);
  }

  session_close(&session);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_run_chat_workspace(const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_rpc_response_t response;
  gzc_workspace_get_request_t workspace_request;
  memset(&workspace_request, 0, sizeof(workspace_request));
  workspace_request.name = gzc_str_from_cstr("direct-chatroom-workspace");
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_WORKSPACE_GET),
      encode_workspace_get_request,
      &workspace_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.workspace.get");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_workspace_get_response_t workspace;
  memset(&workspace, 0, sizeof(workspace));
  rc = gzc_workspace_get_response_decode_json(response.result_json, &workspace);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.workspace.get", rc);
  }
  if (!str_eq(workspace.name, "direct-chatroom-workspace") || !str_eq(workspace.workflow_name, "chatroom-direct")) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.workspace.get", GZC_ERR_RPC);
  }

  gzc_server_set_run_workspace_request_t set_request;
  memset(&set_request, 0, sizeof(set_request));
  set_request.workspace_name = gzc_str_from_cstr("direct-chatroom-workspace");
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_RUN_WORKSPACE_SET),
      encode_server_set_run_workspace_request,
      &set_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.run.workspace.set");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_server_set_run_workspace_response_t set_response;
  memset(&set_response, 0, sizeof(set_response));
  rc = gzc_server_set_run_workspace_response_decode_json(response.result_json, &set_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.run.workspace.set", rc);
  }
  if (!str_eq(set_response.workspace_name, "direct-chatroom-workspace")) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.run.workspace.set", GZC_ERR_RPC);
  }

  gzc_server_get_run_workspace_request_t get_request;
  memset(&get_request, 0, sizeof(get_request));
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_RUN_WORKSPACE_GET),
      encode_server_get_run_workspace_request,
      &get_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.run.workspace.get");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_server_get_run_workspace_response_t get_response;
  memset(&get_response, 0, sizeof(get_response));
  rc = gzc_server_get_run_workspace_response_decode_json(response.result_json, &get_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.run.workspace.get", rc);
  }
  if (!str_eq(get_response.workspace_name, "direct-chatroom-workspace") || !str_nonempty(get_response.runtime_state.raw)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.run.workspace.get", GZC_ERR_RPC);
  }

  gzc_server_get_run_status_request_t status_request;
  memset(&status_request, 0, sizeof(status_request));
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_RUN_STATUS),
      encode_server_get_run_status_request,
      &status_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.run.status");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_server_get_run_status_response_t status_response;
  memset(&status_response, 0, sizeof(status_response));
  rc = gzc_server_get_run_status_response_decode_json(response.result_json, &status_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.run.status", rc);
  }
  if (!str_nonempty(status_response.state.raw)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.run.status", GZC_ERR_RPC);
  }

  gzc_server_list_run_workspace_history_request_t history_request;
  memset(&history_request, 0, sizeof(history_request));
  history_request.has_limit = true;
  history_request.limit = 5;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY),
      encode_server_list_run_workspace_history_request,
      &history_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.run.workspace.history");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_server_list_run_workspace_history_response_t history_response;
  memset(&history_response, 0, sizeof(history_response));
  rc = gzc_server_list_run_workspace_history_response_decode_json(response.result_json, &history_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.run.workspace.history", rc);
  }
  if (!str_nonempty(history_response.items.raw)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.run.workspace.history", GZC_ERR_RPC);
  }

  session_close(&session);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_run_chat_roundtrip(
    const char *identity_dir,
    const char *workspace_name,
    const unsigned char *packet_blob,
    unsigned long packet_blob_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (packet_blob == NULL || packet_blob_len < 4) {
    return fail(errbuf, errbuf_len, "chat roundtrip packets", GZC_ERR_INVALID_ARGUMENT);
  }
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = set_chat_workspace(&session, workspace_name, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }

  gzc_service_channel_t *event_channel = NULL;
  rc = gzc_client_open_service_channel(session.client, 32, 15000, &event_channel);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "open chat event service", rc);
  }

  rc = send_event_json(
      event_channel,
      "{\"v\":1,\"type\":\"bos\",\"stream_id\":\"cgo-chat\",\"label\":\"cgo-chat\",\"kind\":\"audio\",\"mime_type\":\"audio/opus\"}");
  if (rc != GZC_OK) {
    gzc_service_channel_close(event_channel);
    session_close(&session);
    return fail(errbuf, errbuf_len, "send chat BOS", rc);
  }

  const uint8_t *cursor = packet_blob;
  const uint8_t *end = packet_blob + packet_blob_len;
  uint32_t packet_count = read_u32_le(cursor);
  cursor += 4;
  if (packet_count == 0 || packet_count > 4096) {
    gzc_service_channel_close(event_channel);
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid chat packet count", GZC_ERR_INVALID_ARGUMENT);
  }
  uint64_t timestamp_ms = 1;
  for (uint32_t i = 0; i < packet_count; i++) {
    if ((size_t)(end - cursor) < 4) {
      gzc_service_channel_close(event_channel);
      session_close(&session);
      return fail(errbuf, errbuf_len, "truncated chat packet length", GZC_ERR_INVALID_ARGUMENT);
    }
    uint32_t packet_len = read_u32_le(cursor);
    cursor += 4;
    if (packet_len == 0 || (uint32_t)(end - cursor) < packet_len) {
      gzc_service_channel_close(event_channel);
      session_close(&session);
      return fail(errbuf, errbuf_len, "truncated chat packet payload", GZC_ERR_INVALID_ARGUMENT);
    }
    rc = send_stamped_opus_packet(&session, timestamp_ms, cursor, packet_len);
    if (rc != GZC_OK) {
      gzc_service_channel_close(event_channel);
      session_close(&session);
      return fail(errbuf, errbuf_len, "send chat opus packet", rc);
    }
    if (session.webrtc.peer_poll != NULL) {
      rc = session.webrtc.peer_poll(&session.backend.peer, 20);
      if (rc != GZC_OK) {
        gzc_service_channel_close(event_channel);
        session_close(&session);
        return fail(errbuf, errbuf_len, "pace chat opus packet", rc);
      }
    }
    cursor += packet_len;
    timestamp_ms += 20;
  }

  rc = send_event_json(
      event_channel,
      "{\"v\":1,\"type\":\"eos\",\"stream_id\":\"cgo-chat\",\"label\":\"cgo-chat\",\"kind\":\"audio\",\"mime_type\":\"audio/opus\"}");
  if (rc != GZC_OK) {
    gzc_service_channel_close(event_channel);
    session_close(&session);
    return fail(errbuf, errbuf_len, "send chat EOS", rc);
  }

  bool saw_text = false;
  bool saw_event_eos = false;
  size_t event_frames = 0;
  size_t downlink_packets = 0;
  gzc_buf_t frame_bytes;
  gzc_buf_t packet_payload;
  gzc_buf_init(&frame_bytes);
  gzc_buf_init(&packet_payload);
  int64_t deadline = gzc_default_platform()->time_unix_ms(NULL) + 90000;
  while (gzc_default_platform()->time_unix_ms(NULL) < deadline) {
    rc = gzc_service_channel_read_frame(event_channel, 50, &frame_bytes);
    if (rc == GZC_OK) {
      event_frames++;
      gzc_rpc_frame_t frame;
      memset(&frame, 0, sizeof(frame));
      rc = gzc_rpc_frame_decode(frame_bytes.data, frame_bytes.len, &frame);
      if (rc != GZC_OK) {
        break;
      }
      gzc_str_t raw = gzc_str_from_parts((const char *)frame.data, frame.len);
      if (frame.type == GZC_RPC_FRAME_JSON || frame.type == GZC_RPC_FRAME_TEXT) {
        if (raw_contains(raw, gzc_str_from_cstr("\"type\":\"text.")) && raw_contains(raw, gzc_str_from_cstr("\"text\""))) {
          saw_text = true;
        }
        if (raw_contains(raw, gzc_str_from_cstr("\"type\":\"eos\""))) {
          saw_event_eos = true;
        }
      }
    } else if (rc != GZC_ERR_TIMEOUT) {
      break;
    }

    uint8_t protocol = 0;
    rc = gzc_client_read_packet(session.client, 50, &protocol, &packet_payload);
    if (rc == GZC_OK) {
      if (protocol == 0x10 && packet_payload.len > 8 && packet_payload.data[0] == 1) {
        downlink_packets++;
      }
    } else if (rc != GZC_ERR_TIMEOUT) {
      break;
    }

    if (saw_text && downlink_packets > 0 && saw_event_eos) {
      break;
    }
  }
  gzc_buf_free(&frame_bytes, gzc_default_platform());
  gzc_buf_free(&packet_payload, gzc_default_platform());
  gzc_service_channel_close(event_channel);
  session_close(&session);
  if (rc != GZC_OK && rc != GZC_ERR_TIMEOUT) {
    return fail(errbuf, errbuf_len, "read chat roundtrip", rc);
  }
  if (!saw_text || downlink_packets == 0) {
    if (errbuf != NULL && errbuf_len > 0) {
      (void)snprintf(
          errbuf,
          errbuf_len,
          "chat roundtrip missing text or audio: events=%lu saw_text=%d saw_eos=%d downlink_packets=%lu",
          (unsigned long)event_frames,
          saw_text ? 1 : 0,
          saw_event_eos ? 1 : 0,
          (unsigned long)downlink_packets);
    }
    return GZC_ERR_TIMEOUT;
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_run_social_basic(const char *identity_dir, char *errbuf, unsigned long errbuf_len) {
  cgo_sdk_session_t session;
  int rc = session_open(&session, identity_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  long long unique = (long long)gzc_default_platform()->time_unix_ms(NULL);
  char contact_name[96];
  char contact_phone[32];
  char group_name[96];
  snprintf(contact_name, sizeof(contact_name), "C SDK Social Contact %lld", unique);
  snprintf(contact_phone, sizeof(contact_phone), "+1555%010lld", unique % 10000000000LL);
  snprintf(group_name, sizeof(group_name), "c-sdk-social-group-%lld", unique);

  gzc_rpc_response_t response;
  gzc_contact_create_request_t contact_request;
  memset(&contact_request, 0, sizeof(contact_request));
  contact_request.has_display_name = true;
  contact_request.display_name = gzc_str_from_cstr(contact_name);
  contact_request.has_phone_number = true;
  contact_request.phone_number = gzc_str_from_cstr(contact_phone);
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_CONTACT_CREATE),
      encode_contact_create_request,
      &contact_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.contact.create");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_contact_create_response_t contact_create;
  memset(&contact_create, 0, sizeof(contact_create));
  rc = gzc_contact_create_response_decode_json(response.result_json, &contact_create);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.contact.create", rc);
  }
  if (!contact_create.has_id || !str_nonempty(contact_create.id) ||
      !contact_create.has_display_name || !str_eq(contact_create.display_name, contact_name)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.contact.create", GZC_ERR_RPC);
  }
  char contact_id_storage[128];
  gzc_str_t contact_id;
  rc = copy_str_to_storage(contact_id_storage, sizeof(contact_id_storage), contact_create.id, &contact_id);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "copy server.contact.create id", rc);
  }

  gzc_contact_get_request_t contact_get_request;
  memset(&contact_get_request, 0, sizeof(contact_get_request));
  contact_get_request.id = contact_id;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_CONTACT_GET),
      encode_contact_get_request,
      &contact_get_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.contact.get");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_contact_get_response_t contact_get;
  memset(&contact_get, 0, sizeof(contact_get));
  rc = gzc_contact_get_response_decode_json(response.result_json, &contact_get);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.contact.get", rc);
  }
  if (!contact_get.has_id || contact_get.id.len != contact_id.len ||
      strncmp(contact_get.id.data, contact_id.data, contact_id.len) != 0) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.contact.get", GZC_ERR_RPC);
  }

  gzc_contact_list_request_t contact_list_request;
  memset(&contact_list_request, 0, sizeof(contact_list_request));
  contact_list_request.has_limit = true;
  contact_list_request.limit = 1000;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_CONTACT_LIST),
      encode_contact_list_request,
      &contact_list_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.contact.list");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_contact_list_response_t contact_list;
  memset(&contact_list, 0, sizeof(contact_list));
  rc = gzc_contact_list_response_decode_json(response.result_json, &contact_list);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.contact.list", rc);
  }
  if (!raw_contains(contact_list.items.raw, contact_id)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.contact.list", GZC_ERR_RPC);
  }

  gzc_friend_group_create_request_t group_request;
  memset(&group_request, 0, sizeof(group_request));
  group_request.name = gzc_str_from_cstr(group_name);
  group_request.has_description = true;
  group_request.description = gzc_str_from_cstr("created by cgo C SDK test");
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_CREATE),
      encode_friend_group_create_request,
      &group_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.friend_group.create");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_friend_group_create_response_t group_create;
  memset(&group_create, 0, sizeof(group_create));
  rc = gzc_friend_group_create_response_decode_json(response.result_json, &group_create);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.friend_group.create", rc);
  }
  if (!group_create.has_id || !str_nonempty(group_create.id) ||
      !group_create.has_name || !str_eq(group_create.name, group_name) ||
      !group_create.has_workspace_name || !str_nonempty(group_create.workspace_name)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.friend_group.create", GZC_ERR_RPC);
  }
  char group_id_storage[128];
  gzc_str_t group_id;
  rc = copy_str_to_storage(group_id_storage, sizeof(group_id_storage), group_create.id, &group_id);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "copy server.friend_group.create id", rc);
  }

  gzc_friend_group_get_request_t group_get_request;
  memset(&group_get_request, 0, sizeof(group_get_request));
  group_get_request.id = group_id;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_GET),
      encode_friend_group_get_request,
      &group_get_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.friend_group.get");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_friend_group_get_response_t group_get;
  memset(&group_get, 0, sizeof(group_get));
  rc = gzc_friend_group_get_response_decode_json(response.result_json, &group_get);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.friend_group.get", rc);
  }
  if (!group_get.has_id || group_get.id.len != group_id.len ||
      strncmp(group_get.id.data, group_id.data, group_id.len) != 0) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.friend_group.get", GZC_ERR_RPC);
  }

  gzc_friend_group_list_request_t group_list_request;
  memset(&group_list_request, 0, sizeof(group_list_request));
  group_list_request.has_limit = true;
  group_list_request.limit = 1000;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_LIST),
      encode_friend_group_list_request,
      &group_list_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.friend_group.list");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_friend_group_list_response_t group_list;
  memset(&group_list, 0, sizeof(group_list));
  rc = gzc_friend_group_list_response_decode_json(response.result_json, &group_list);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.friend_group.list", rc);
  }
  if (!raw_contains(group_list.items.raw, group_id)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.friend_group.list", GZC_ERR_RPC);
  }

  gzc_friend_group_invite_token_create_request_t token_request;
  memset(&token_request, 0, sizeof(token_request));
  token_request.friend_group_id = group_id;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE),
      encode_friend_group_invite_token_create_request,
      &token_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.friend_group.invite_token.create");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_friend_group_invite_token_create_response_t token_response;
  memset(&token_response, 0, sizeof(token_response));
  rc = gzc_friend_group_invite_token_create_response_decode_json(response.result_json, &token_response);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.friend_group.invite_token.create", rc);
  }
  if (!str_nonempty(token_response.invite_token) || !str_nonempty(token_response.expires_at)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.friend_group.invite_token.create", GZC_ERR_RPC);
  }

  gzc_friend_group_message_send_request_t message_request;
  memset(&message_request, 0, sizeof(message_request));
  message_request.friend_group_id = group_id;
  message_request.audio_content_type = gzc_str_from_cstr("audio/opus");
  message_request.audio_base64 = gzc_str_from_cstr("bm90LXJlYWwtb3B1cy1idXQtcnBjLXBheWxvYWQ=");
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND),
      encode_friend_group_message_send_request,
      &message_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.friend_group.messages.send");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_friend_group_message_send_response_t message_send;
  memset(&message_send, 0, sizeof(message_send));
  rc = gzc_friend_group_message_send_response_decode_json(response.result_json, &message_send);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.friend_group.messages.send", rc);
  }
  if (!message_send.has_id || !str_nonempty(message_send.id) ||
      !message_send.has_friend_group_id || message_send.friend_group_id.len != group_id.len ||
      strncmp(message_send.friend_group_id.data, group_id.data, group_id.len) != 0) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.friend_group.messages.send", GZC_ERR_RPC);
  }
  char message_id_storage[128];
  gzc_str_t message_id;
  rc = copy_str_to_storage(message_id_storage, sizeof(message_id_storage), message_send.id, &message_id);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "copy server.friend_group.messages.send id", rc);
  }

  gzc_friend_group_message_get_request_t message_get_request;
  memset(&message_get_request, 0, sizeof(message_get_request));
  message_get_request.friend_group_id = group_id;
  message_get_request.id = message_id;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET),
      encode_friend_group_message_get_request,
      &message_get_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.friend_group.messages.get");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_friend_group_message_get_response_t message_get;
  memset(&message_get, 0, sizeof(message_get));
  rc = gzc_friend_group_message_get_response_decode_json(response.result_json, &message_get);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.friend_group.messages.get", rc);
  }
  if (!message_get.has_id || message_get.id.len != message_id.len ||
      strncmp(message_get.id.data, message_id.data, message_id.len) != 0) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.friend_group.messages.get", GZC_ERR_RPC);
  }

  gzc_friend_group_message_list_request_t message_list_request;
  memset(&message_list_request, 0, sizeof(message_list_request));
  message_list_request.has_friend_group_id = true;
  message_list_request.friend_group_id = group_id;
  message_list_request.has_limit = true;
  message_list_request.limit = 1000;
  rc = call_json(
      &session,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST),
      encode_friend_group_message_list_request,
      &message_list_request,
      &response,
      errbuf,
      errbuf_len,
      "call server.friend_group.messages.list");
  if (rc != GZC_OK) {
    session_close(&session);
    return rc;
  }
  gzc_friend_group_message_list_response_t message_list;
  memset(&message_list, 0, sizeof(message_list));
  rc = gzc_friend_group_message_list_response_decode_json(response.result_json, &message_list);
  if (rc != GZC_OK) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "decode server.friend_group.messages.list", rc);
  }
  if (!raw_contains(message_list.items.raw, message_id)) {
    session_close(&session);
    return fail(errbuf, errbuf_len, "invalid server.friend_group.messages.list", GZC_ERR_RPC);
  }

  session_close(&session);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_run_social_relationships(
    const char *identity_a_dir,
    const char *identity_b_dir,
    char *errbuf,
    unsigned long errbuf_len) {
  cgo_sdk_session_t session_a;
  int rc = session_open(&session_a, identity_a_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  cgo_sdk_session_t session_b;
  rc = session_open(&session_b, identity_b_dir, errbuf, errbuf_len);
  if (rc != GZC_OK) {
    session_close(&session_a);
    return rc;
  }

  gzc_rpc_response_t response;

  gzc_friend_invite_token_create_request_t friend_token_request;
  memset(&friend_token_request, 0, sizeof(friend_token_request));
  rc = call_json(
      &session_b,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE),
      encode_friend_invite_token_create_request,
      &friend_token_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-b server.friend.invite_token.create");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_invite_token_create_response_t friend_token;
  memset(&friend_token, 0, sizeof(friend_token));
  rc = gzc_friend_invite_token_create_response_decode_json(response.result_json, &friend_token);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-b server.friend.invite_token.create", rc);
  }
  if (!str_nonempty(friend_token.invite_token) || !str_nonempty(friend_token.expires_at)) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-b server.friend.invite_token.create", GZC_ERR_RPC);
  }
  char friend_token_storage[512];
  gzc_str_t friend_invite_token;
  rc = copy_str_to_storage(friend_token_storage, sizeof(friend_token_storage), friend_token.invite_token, &friend_invite_token);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "copy peer-b friend invite token", rc);
  }

  gzc_friend_add_request_t friend_add_request;
  memset(&friend_add_request, 0, sizeof(friend_add_request));
  friend_add_request.invite_token = friend_invite_token;
  rc = call_json(
      &session_a,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_ADD),
      encode_friend_add_request,
      &friend_add_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-a server.friend.add");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_add_response_t friend_add;
  memset(&friend_add, 0, sizeof(friend_add));
  rc = gzc_friend_add_response_decode_json(response.result_json, &friend_add);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-a server.friend.add", rc);
  }
  if (!friend_add.has_id || !str_nonempty(friend_add.id) ||
      !friend_add.has_workspace_name || !str_nonempty(friend_add.workspace_name) ||
      !friend_add.has_peer_public_key || !str_nonempty(friend_add.peer_public_key)) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-a server.friend.add", GZC_ERR_RPC);
  }

  gzc_friend_group_create_request_t group_request;
  memset(&group_request, 0, sizeof(group_request));
  group_request.name = gzc_str_from_cstr("c-sdk-cross-user-group");
  group_request.has_description = true;
  group_request.description = gzc_str_from_cstr("created by cgo C SDK relationship test");
  rc = call_json(
      &session_a,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_CREATE),
      encode_friend_group_create_request,
      &group_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-a server.friend_group.create");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_group_create_response_t group_create;
  memset(&group_create, 0, sizeof(group_create));
  rc = gzc_friend_group_create_response_decode_json(response.result_json, &group_create);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-a server.friend_group.create", rc);
  }
  if (!group_create.has_id || !str_nonempty(group_create.id) ||
      !group_create.has_workspace_name || !str_nonempty(group_create.workspace_name)) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-a server.friend_group.create", GZC_ERR_RPC);
  }
  char group_id_storage[128];
  gzc_str_t group_id;
  rc = copy_str_to_storage(group_id_storage, sizeof(group_id_storage), group_create.id, &group_id);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "copy peer-a friend group id", rc);
  }

  gzc_friend_group_invite_token_create_request_t group_token_request;
  memset(&group_token_request, 0, sizeof(group_token_request));
  group_token_request.friend_group_id = group_id;
  rc = call_json(
      &session_a,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE),
      encode_friend_group_invite_token_create_request,
      &group_token_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-a server.friend_group.invite_token.create");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_group_invite_token_create_response_t group_token;
  memset(&group_token, 0, sizeof(group_token));
  rc = gzc_friend_group_invite_token_create_response_decode_json(response.result_json, &group_token);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-a server.friend_group.invite_token.create", rc);
  }
  if (!str_nonempty(group_token.invite_token) || !str_nonempty(group_token.expires_at)) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-a server.friend_group.invite_token.create", GZC_ERR_RPC);
  }
  char group_token_storage[512];
  gzc_str_t group_invite_token;
  rc = copy_str_to_storage(group_token_storage, sizeof(group_token_storage), group_token.invite_token, &group_invite_token);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "copy peer-a group invite token", rc);
  }

  gzc_friend_group_join_request_t join_request;
  memset(&join_request, 0, sizeof(join_request));
  join_request.invite_token = group_invite_token;
  rc = call_json(
      &session_b,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_JOIN),
      encode_friend_group_join_request,
      &join_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-b server.friend_group.join");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_group_join_response_t join_response;
  memset(&join_response, 0, sizeof(join_response));
  rc = gzc_friend_group_join_response_decode_json(response.result_json, &join_response);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-b server.friend_group.join", rc);
  }
  if (!raw_contains(join_response.group.raw, group_id) || !raw_contains(join_response.member.raw, group_id)) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-b server.friend_group.join", GZC_ERR_RPC);
  }

  gzc_friend_group_member_list_request_t member_list_request;
  memset(&member_list_request, 0, sizeof(member_list_request));
  member_list_request.has_friend_group_id = true;
  member_list_request.friend_group_id = group_id;
  member_list_request.has_limit = true;
  member_list_request.limit = 1000;
  rc = call_json(
      &session_b,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST),
      encode_friend_group_member_list_request,
      &member_list_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-b server.friend_group.members.list");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_group_member_list_response_t member_list;
  memset(&member_list, 0, sizeof(member_list));
  rc = gzc_friend_group_member_list_response_decode_json(response.result_json, &member_list);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-b server.friend_group.members.list", rc);
  }
  if (!str_nonempty(member_list.items.raw)) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-b server.friend_group.members.list", GZC_ERR_RPC);
  }

  gzc_friend_group_message_send_request_t message_request;
  memset(&message_request, 0, sizeof(message_request));
  message_request.friend_group_id = group_id;
  message_request.audio_content_type = gzc_str_from_cstr("audio/opus");
  message_request.audio_base64 = gzc_str_from_cstr("Yy1zZGstY3Jvc3MtdXNlci1zb2NpYWwtbWVzc2FnZQ==");
  rc = call_json(
      &session_b,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND),
      encode_friend_group_message_send_request,
      &message_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-b server.friend_group.messages.send");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_group_message_send_response_t message_send;
  memset(&message_send, 0, sizeof(message_send));
  rc = gzc_friend_group_message_send_response_decode_json(response.result_json, &message_send);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-b server.friend_group.messages.send", rc);
  }
  if (!message_send.has_id || !str_nonempty(message_send.id) ||
      !message_send.has_friend_group_id || message_send.friend_group_id.len != group_id.len ||
      strncmp(message_send.friend_group_id.data, group_id.data, group_id.len) != 0) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-b server.friend_group.messages.send", GZC_ERR_RPC);
  }
  char message_id_storage[128];
  gzc_str_t message_id;
  rc = copy_str_to_storage(message_id_storage, sizeof(message_id_storage), message_send.id, &message_id);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "copy peer-b message id", rc);
  }

  gzc_friend_group_message_get_request_t message_get_request;
  memset(&message_get_request, 0, sizeof(message_get_request));
  message_get_request.friend_group_id = group_id;
  message_get_request.id = message_id;
  rc = call_json(
      &session_a,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET),
      encode_friend_group_message_get_request,
      &message_get_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-a server.friend_group.messages.get");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_group_message_get_response_t message_get;
  memset(&message_get, 0, sizeof(message_get));
  rc = gzc_friend_group_message_get_response_decode_json(response.result_json, &message_get);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-a server.friend_group.messages.get", rc);
  }
  if (!message_get.has_id || message_get.id.len != message_id.len ||
      strncmp(message_get.id.data, message_id.data, message_id.len) != 0) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-a server.friend_group.messages.get", GZC_ERR_RPC);
  }

  gzc_friend_group_message_list_request_t message_list_request;
  memset(&message_list_request, 0, sizeof(message_list_request));
  message_list_request.has_friend_group_id = true;
  message_list_request.friend_group_id = group_id;
  message_list_request.has_limit = true;
  message_list_request.limit = 1000;
  rc = call_json(
      &session_a,
      gzc_str_from_cstr(GZC_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST),
      encode_friend_group_message_list_request,
      &message_list_request,
      &response,
      errbuf,
      errbuf_len,
      "call peer-a server.friend_group.messages.list");
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return rc;
  }
  gzc_friend_group_message_list_response_t message_list;
  memset(&message_list, 0, sizeof(message_list));
  rc = gzc_friend_group_message_list_response_decode_json(response.result_json, &message_list);
  if (rc != GZC_OK) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "decode peer-a server.friend_group.messages.list", rc);
  }
  if (!raw_contains(message_list.items.raw, message_id)) {
    session_close(&session_b);
    session_close(&session_a);
    return fail(errbuf, errbuf_len, "invalid peer-a server.friend_group.messages.list", GZC_ERR_RPC);
  }

  session_close(&session_b);
  session_close(&session_a);
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}
