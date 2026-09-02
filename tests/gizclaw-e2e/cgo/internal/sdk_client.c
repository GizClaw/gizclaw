#include "sdk_client.h"

#include "../../../../sdk/c/gizclaw/cgobackend/gzc_cgo_backend.h"
#include "gzc.h"
#include "pb_decode.h"
#include "pb_encode.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct gzc_cgo_session {
  gzc_cgo_device_state_t device;
  gzc_cgo_backend_t backend;
  gzc_http_vtable_t http;
  gzc_platform_crypto_t crypto;
  gzc_webrtc_vtable_t webrtc;
  gzc_webrtc_media_vtable_t media;
  gzc_client_t *client;
};

static int fail(char *errbuf, unsigned long errbuf_len, const char *message, int rc) {
  if (errbuf != NULL && errbuf_len > 0) {
    (void)snprintf(errbuf, errbuf_len, "%s: %s (%d)", message, gzc_status_string((gzc_status_t)rc), rc);
  }
  return rc == GZC_OK ? GZC_ERR_RPC : rc;
}

static int rpc_fail(
    char *errbuf,
    unsigned long errbuf_len,
    gzc_str_t message,
    int code,
    int *out_rpc_error_code) {
  if (out_rpc_error_code != NULL) {
    *out_rpc_error_code = code;
  }
  if (errbuf != NULL && errbuf_len > 0) {
    size_t count = message.len;
    if (count >= errbuf_len) {
      count = errbuf_len - 1;
    }
    if (count > 0 && message.data != NULL) {
      memcpy(errbuf, message.data, count);
    }
    errbuf[count] = 0;
  }
  return GZC_ERR_RPC;
}

static unsigned long long rpc_service_for_method(
    gizclaw_rpc_v1_RpcMethod method) {
  return method ==
                     gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PEER_LOOKUP ||
                 method ==
                     gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PEER_ASSIGN ||
                 method ==
                     gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_ROUTE_RESOLVE
             ? 0x31u
             : 0u;
}

static int wait_rpc_result(gzc_cgo_session_t *session,
                           gzc_rpc_request_t *request,
                           gzc_rpc_response_t *out_response) {
  int rc;
  while ((rc = gzc_rpc_request_result(request, out_response)) ==
         GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(session->client, 10);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return rc;
}

static int copy_c_string(
    char *out,
    unsigned long out_len,
    const char *value,
    char *errbuf,
    unsigned long errbuf_len,
    const char *field) {
  if (out == NULL || out_len == 0 || value == NULL) {
    return fail(errbuf, errbuf_len, field, GZC_ERR_INVALID_ARGUMENT);
  }
  size_t value_len = strlen(value);
  if (value_len >= out_len) {
    return fail(errbuf, errbuf_len, field, GZC_ERR_INVALID_ARGUMENT);
  }
  memcpy(out, value, value_len + 1);
  return GZC_OK;
}

typedef struct {
  gzc_cgo_stream_frame_t *frames;
  unsigned long count;
  unsigned long cap;
} stream_collect_state_t;

static int append_stream_frame(void *userdata, const gzc_rpc_frame_t *frame) {
  stream_collect_state_t *state = (stream_collect_state_t *)userdata;
  if (state == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (state->count == state->cap) {
    unsigned long next_cap = state->cap == 0 ? 8 : state->cap * 2;
    gzc_cgo_stream_frame_t *next = (gzc_cgo_stream_frame_t *)realloc(state->frames, next_cap * sizeof(*next));
    if (next == NULL) {
      return GZC_ERR_NO_MEMORY;
    }
    memset(next + state->cap, 0, (next_cap - state->cap) * sizeof(*next));
    state->frames = next;
    state->cap = next_cap;
  }
  gzc_cgo_stream_frame_t *out = &state->frames[state->count];
  out->type = (int)frame->type;
  out->data = NULL;
  out->len = (unsigned long)frame->len;
  if (frame->len > 0) {
    out->data = (unsigned char *)malloc(frame->len);
    if (out->data == NULL) {
      return GZC_ERR_NO_MEMORY;
    }
    memcpy(out->data, frame->data, frame->len);
  }
  state->count++;
  return GZC_OK;
}

static int device_respond_message(
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata,
    const pb_msgdesc_t *fields,
    const void *message) {
  uint8_t payload[256];
  pb_ostream_t output = pb_ostream_from_buffer(payload, sizeof(payload));
  if (!pb_encode(&output, fields, message)) {
    return GZC_ERR_RPC;
  }
  const gzc_rpc_provider_response_t response = {
      .payload = payload,
      .payload_len = output.bytes_written,
  };
  return respond(respond_userdata, &response);
}

static int device_respond_error(
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata,
    int code,
    const char *message) {
  const gzc_rpc_provider_response_t response = {
      .has_error = true,
      .error_code = code,
      .error_message = gzc_str_from_cstr(message),
  };
  return respond(respond_userdata, &response);
}

static void device_fill_status(const gzc_cgo_device_state_t *state, gizclaw_rpc_v1_PeerStatus *status) {
  status->has_volume = true;
  status->volume = state->volume;
  status->has_muted = true;
  status->muted = state->muted != 0;
  status->has_battery_percent = true;
  status->battery_percent = 88;
  status->has_charging = true;
  status->charging = true;
}

static bool device_decode(gzc_str_t payload, const pb_msgdesc_t *fields, void *message) {
  pb_istream_t input = pb_istream_from_buffer((const pb_byte_t *)payload.data, payload.len);
  return pb_decode(&input, fields, message);
}

/*
 * Serves the Server-initiated client.device.* and client.wifi.* methods with
 * an in-memory device model so tests can verify the reverse RPC path.
 */
static int device_rpc_provider(
    void *userdata,
    int method,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata) {
  gzc_cgo_session_t *session = (gzc_cgo_session_t *)userdata;
  if (session == NULL || respond == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_cgo_device_state_t *state = &session->device;
  switch ((gizclaw_rpc_v1_RpcMethod)method) {
    case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_STATUS_GET: {
      state->status_calls++;
      gizclaw_rpc_v1_ClientDeviceStatusGetResponse response =
          gizclaw_rpc_v1_ClientDeviceStatusGetResponse_init_zero;
      response.has_value = true;
      device_fill_status(state, &response.value);
      return device_respond_message(respond, respond_userdata,
                                    gizclaw_rpc_v1_ClientDeviceStatusGetResponse_fields, &response);
    }
    case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_VOLUME_SET: {
      gizclaw_rpc_v1_ClientDeviceVolumeSetRequest request =
          gizclaw_rpc_v1_ClientDeviceVolumeSetRequest_init_zero;
      if (!device_decode(request_payload, gizclaw_rpc_v1_ClientDeviceVolumeSetRequest_fields, &request) ||
          request.level < 0 || request.level > 100) {
        return device_respond_error(respond, respond_userdata,
                                    gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                                    "invalid volume");
      }
      state->volume_calls++;
      state->volume = request.level;
      state->muted = request.muted ? 1 : 0;
      gizclaw_rpc_v1_ClientDeviceVolumeSetResponse response =
          gizclaw_rpc_v1_ClientDeviceVolumeSetResponse_init_zero;
      response.has_value = true;
      device_fill_status(state, &response.value);
      return device_respond_message(respond, respond_userdata,
                                    gizclaw_rpc_v1_ClientDeviceVolumeSetResponse_fields, &response);
    }
    case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_SOUND_PLAY: {
      gizclaw_rpc_v1_ClientDeviceSoundPlayRequest request =
          gizclaw_rpc_v1_ClientDeviceSoundPlayRequest_init_zero;
      if (!device_decode(request_payload, gizclaw_rpc_v1_ClientDeviceSoundPlayRequest_fields, &request)) {
        return device_respond_error(respond, respond_userdata,
                                    gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                                    "invalid sound request");
      }
      if (strcmp(request.sound, "chime") != 0 && strcmp(request.sound, "alarm") != 0) {
        return device_respond_error(respond, respond_userdata,
                                    gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                                    "unknown sound");
      }
      state->sound_calls++;
      strcpy(state->last_sound, request.sound);
      state->last_duration_ms = request.has_duration_ms ? request.duration_ms : -1;
      gizclaw_rpc_v1_ClientDeviceSoundPlayResponse response =
          gizclaw_rpc_v1_ClientDeviceSoundPlayResponse_init_zero;
      return device_respond_message(respond, respond_userdata,
                                    gizclaw_rpc_v1_ClientDeviceSoundPlayResponse_fields, &response);
    }
    case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_REBOOT: {
      gizclaw_rpc_v1_ClientDeviceRebootRequest request =
          gizclaw_rpc_v1_ClientDeviceRebootRequest_init_zero;
      if (!device_decode(request_payload, gizclaw_rpc_v1_ClientDeviceRebootRequest_fields, &request)) {
        return device_respond_error(respond, respond_userdata,
                                    gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                                    "invalid reboot request");
      }
      state->reboot_calls++;
      state->last_delay_ms = request.has_delay_ms ? request.delay_ms : -1;
      gizclaw_rpc_v1_ClientDeviceRebootResponse response =
          gizclaw_rpc_v1_ClientDeviceRebootResponse_init_zero;
      return device_respond_message(respond, respond_userdata,
                                    gizclaw_rpc_v1_ClientDeviceRebootResponse_fields, &response);
    }
    case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_STATUS_GET: {
      state->wifi_status_calls++;
      gizclaw_rpc_v1_ClientWifiStatusGetResponse response =
          gizclaw_rpc_v1_ClientWifiStatusGetResponse_init_zero;
      response.has_value = true;
      response.value.connected = true;
      response.value.has_ssid = true;
      strcpy(response.value.ssid, "home");
      response.value.has_rssi_dbm = true;
      response.value.rssi_dbm = -61;
      response.value.has_ip = true;
      strcpy(response.value.ip, "192.0.2.20");
      response.value.has_bssid = true;
      strcpy(response.value.bssid, "aa:bb:cc:dd:ee:ff");
      return device_respond_message(respond, respond_userdata,
                                    gizclaw_rpc_v1_ClientWifiStatusGetResponse_fields, &response);
    }
    case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_SAVED_LIST: {
      state->saved_list_calls++;
      gizclaw_rpc_v1_ClientWifiSavedListResponse response =
          gizclaw_rpc_v1_ClientWifiSavedListResponse_init_zero;
      response.networks_count = (pb_size_t)state->saved_count;
      for (int i = 0; i < state->saved_count; i++) {
        strcpy(response.networks[i].ssid, state->saved[i]);
      }
      return device_respond_message(respond, respond_userdata,
                                    gizclaw_rpc_v1_ClientWifiSavedListResponse_fields, &response);
    }
    case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_SAVED_FORGET: {
      gizclaw_rpc_v1_ClientWifiSavedForgetRequest request =
          gizclaw_rpc_v1_ClientWifiSavedForgetRequest_init_zero;
      if (!device_decode(request_payload, gizclaw_rpc_v1_ClientWifiSavedForgetRequest_fields, &request) ||
          request.ssid[0] == 0) {
        return device_respond_error(respond, respond_userdata,
                                    gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                                    "invalid ssid");
      }
      state->forget_calls++;
      for (int i = 0; i < state->saved_count; i++) {
        if (strcmp(state->saved[i], request.ssid) == 0) {
          for (int j = i + 1; j < state->saved_count; j++) {
            strcpy(state->saved[j - 1], state->saved[j]);
          }
          state->saved_count--;
          gizclaw_rpc_v1_ClientWifiSavedForgetResponse response =
              gizclaw_rpc_v1_ClientWifiSavedForgetResponse_init_zero;
          return device_respond_message(respond, respond_userdata,
                                        gizclaw_rpc_v1_ClientWifiSavedForgetResponse_fields, &response);
        }
      }
      return device_respond_error(respond, respond_userdata,
                                  gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_NOT_FOUND,
                                  "saved network not found");
    }
    default:
      return device_respond_error(respond, respond_userdata,
                                  gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_METHOD_NOT_FOUND,
                                  "unsupported client method");
  }
}

static void device_state_init(gzc_cgo_device_state_t *state) {
  memset(state, 0, sizeof(*state));
  state->volume = 50;
  state->muted = 0;
  state->last_duration_ms = -1;
  state->last_delay_ms = -1;
  state->saved_count = 2;
  strcpy(state->saved[0], "home");
  strcpy(state->saved[1], "office");
}

int gzc_cgo_session_device_state(gzc_cgo_session_t *session, gzc_cgo_device_state_t *out_state) {
  if (session == NULL || out_state == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_state = session->device;
  return GZC_OK;
}

int gzc_cgo_session_open(
    const char *server_endpoint,
    const char *private_key,
    gzc_cgo_session_t **out_session,
    char *errbuf,
    unsigned long errbuf_len) {
  if (server_endpoint == NULL || private_key == NULL || out_session == NULL) {
    return fail(errbuf, errbuf_len, "session open", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_session = NULL;
  gzc_cgo_session_t *session = (gzc_cgo_session_t *)calloc(1, sizeof(*session));
  if (session == NULL) {
    return fail(errbuf, errbuf_len, "session alloc", GZC_ERR_NO_MEMORY);
  }
  device_state_init(&session->device);

  int rc = gzc_cgo_backend_init(&session->backend);
  if (rc != GZC_OK) {
    free(session);
    return fail(errbuf, errbuf_len, "backend init", rc);
  }

  gzc_cgo_backend_http_vtable(&session->backend, &session->http);
  gzc_cgo_backend_crypto_vtable(&session->backend, &session->crypto);
  gzc_cgo_backend_webrtc_vtable(&session->backend, &session->webrtc);
  gzc_cgo_backend_webrtc_media_vtable(&session->backend, &session->media);

  gzc_client_config_t config;
  memset(&config, 0, sizeof(config));
  config.server_endpoint = gzc_str_from_cstr(server_endpoint);
  config.private_key = gzc_str_from_cstr(private_key);
  config.platform = session->backend.platform;
  config.crypto = &session->crypto;
  config.http = &session->http;
  config.webrtc = &session->webrtc;
  config.cipher_mode = GZC_CIPHER_CHACHA20_POLY1305;
  config.connect_timeout_ms = 15000;
  config.write_timeout_ms = 15000;
  config.rpc_provider = device_rpc_provider;
  config.rpc_provider_userdata = session;

  rc = gzc_client_create(&config, &session->client);
  if (rc != GZC_OK) {
    gzc_cgo_backend_deinit(&session->backend);
    free(session);
    return fail(errbuf, errbuf_len, "client create", rc);
  }
  rc = gzc_client_set_webrtc_media(session->client, &session->media);
  if (rc != GZC_OK) {
    gzc_client_destroy(session->client);
    gzc_cgo_backend_deinit(&session->backend);
    free(session);
    return fail(errbuf, errbuf_len, "media registration", rc);
  }
  rc = gzc_client_set_peer_add_ice_server(session->client, gzc_cgo_backend_peer_add_ice_server);
  if (rc != GZC_OK) {
    gzc_client_destroy(session->client);
    gzc_cgo_backend_deinit(&session->backend);
    free(session);
    return fail(errbuf, errbuf_len, "client ICE hook", rc);
  }

  rc = gzc_client_connect(session->client);
  if (rc != GZC_OK) {
    gzc_client_destroy(session->client);
    gzc_cgo_backend_deinit(&session->backend);
    free(session);
    return fail(errbuf, errbuf_len, "client connect", rc);
  }
  *out_session = session;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzc_cgo_session_close(gzc_cgo_session_t *session) {
  if (session == NULL) {
    return;
  }
  if (session->client != NULL) {
    gzc_client_destroy(session->client);
    session->client = NULL;
  }
  gzc_cgo_backend_deinit(&session->backend);
  free(session);
}

int gzc_cgo_session_call_rpc_payload(
    gzc_cgo_session_t *session,
    unsigned method_id,
    const unsigned char *params_payload,
    unsigned long params_payload_len,
    unsigned char **out_result_payload,
    unsigned long *out_result_payload_len,
    int *out_rpc_error_code,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || method_id == 0 || (params_payload == NULL && params_payload_len != 0) || out_result_payload == NULL || out_result_payload_len == NULL) {
    return fail(errbuf, errbuf_len, "call rpc payload", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_result_payload = NULL;
  *out_result_payload_len = 0;
  if (out_rpc_error_code != NULL) {
    *out_rpc_error_code = 0;
  }

  gzc_rpc_response_t response;
  memset(&response, 0, sizeof(response));
  const gizclaw_rpc_v1_RpcMethod method =
      (gizclaw_rpc_v1_RpcMethod)method_id;
  gzc_rpc_request_t *request = NULL;
  int rc = gzc_rpc_request_start(
      session->client, rpc_service_for_method(method), method,
      gzc_str_from_parts((const char *)params_payload, (size_t)params_payload_len),
      5000, &request);
  if (rc == GZC_OK) {
    rc = wait_rpc_result(session, request, &response);
  }
  if (rc != GZC_OK) {
    gzc_rpc_request_destroy(request);
    return fail(errbuf, errbuf_len, "call rpc payload", rc);
  }
  if (response.has_error) {
    rc = rpc_fail(
        errbuf,
        errbuf_len,
        response.error.message,
        response.error.code,
        out_rpc_error_code);
    gzc_rpc_request_destroy(request);
    return rc;
  }

  unsigned char *result = (unsigned char *)malloc(response.result_payload.len == 0 ? 1 : response.result_payload.len);
  if (result == NULL) {
    gzc_rpc_request_destroy(request);
    return fail(errbuf, errbuf_len, "copy result", GZC_ERR_NO_MEMORY);
  }
  memcpy(result, response.result_payload.data, response.result_payload.len);
  gzc_rpc_request_destroy(request);
  *out_result_payload = result;
  *out_result_payload_len = (unsigned long)response.result_payload.len;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_start_rpc_request(
    gzc_cgo_session_t *session,
    unsigned long long service,
    unsigned method_id,
    const unsigned char *params_payload,
    unsigned long params_payload_len,
    int timeout_ms,
    gzc_rpc_request_t **out_request,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || session->client == NULL || method_id == 0 ||
      (params_payload == NULL && params_payload_len != 0) ||
      out_request == NULL) {
    return fail(errbuf, errbuf_len, "start rpc request", GZC_ERR_INVALID_ARGUMENT);
  }
  int rc = gzc_rpc_request_start(
      session->client,
      (uint64_t)service,
      (gizclaw_rpc_v1_RpcMethod)method_id,
      gzc_str_from_parts(
          (const char *)params_payload, (size_t)params_payload_len),
      timeout_ms,
      out_request);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "start rpc request", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_rpc_request_result(
    gzc_rpc_request_t *request,
    unsigned char **out_result_payload,
    unsigned long *out_result_payload_len,
    int *out_rpc_error_code,
    char *errbuf,
    unsigned long errbuf_len) {
  if (request == NULL || out_result_payload == NULL ||
      out_result_payload_len == NULL) {
    return fail(errbuf, errbuf_len, "rpc request result", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_result_payload = NULL;
  *out_result_payload_len = 0;
  if (out_rpc_error_code != NULL) {
    *out_rpc_error_code = 0;
  }
  gzc_rpc_response_t response;
  memset(&response, 0, sizeof(response));
  int rc = gzc_rpc_request_result(request, &response);
  if (rc == GZC_ERR_WOULD_BLOCK) {
    return rc;
  }
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "rpc request result", rc);
  }
  if (response.has_error) {
    return rpc_fail(
        errbuf,
        errbuf_len,
        response.error.message,
        response.error.code,
        out_rpc_error_code);
  }
  unsigned char *result = (unsigned char *)malloc(
      response.result_payload.len == 0 ? 1 : response.result_payload.len);
  if (result == NULL) {
    return fail(errbuf, errbuf_len, "copy rpc request result", GZC_ERR_NO_MEMORY);
  }
  memcpy(result, response.result_payload.data, response.result_payload.len);
  *out_result_payload = result;
  *out_result_payload_len = (unsigned long)response.result_payload.len;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzc_cgo_rpc_request_cancel(gzc_rpc_request_t *request) {
  gzc_rpc_request_cancel(request);
}

void gzc_cgo_rpc_request_destroy(gzc_rpc_request_t *request) {
  gzc_rpc_request_destroy(request);
}

int gzc_cgo_session_register(
    gzc_cgo_session_t *session,
    const char *token,
    char *out_runtime_profile_name,
    unsigned long out_runtime_profile_name_len,
    int *out_rpc_error_code,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || token == NULL || out_runtime_profile_name == NULL ||
      out_runtime_profile_name_len == 0) {
    return fail(errbuf, errbuf_len, "register", GZC_ERR_INVALID_ARGUMENT);
  }
  if (out_rpc_error_code != NULL) {
    *out_rpc_error_code = 0;
  }

  gizclaw_rpc_v1_ServerRegisterRequest request =
      gizclaw_rpc_v1_ServerRegisterRequest_init_zero;
  int rc = copy_c_string(
      request.token,
      sizeof(request.token),
      token,
      errbuf,
      errbuf_len,
      "registration token");
  if (rc != GZC_OK) {
    return rc;
  }
  unsigned char request_payload[gizclaw_rpc_v1_ServerRegisterRequest_size];
  pb_ostream_t request_stream =
      pb_ostream_from_buffer(request_payload, sizeof(request_payload));
  if (!pb_encode(
          &request_stream,
          gizclaw_rpc_v1_ServerRegisterRequest_fields,
          &request)) {
    return fail(errbuf, errbuf_len, "encode registration request", GZC_ERR_RPC);
  }

  unsigned char *result_payload = NULL;
  unsigned long result_payload_len = 0;
  rc = gzc_cgo_session_call_rpc_payload(
      session,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_REGISTER,
      request_payload,
      (unsigned long)request_stream.bytes_written,
      &result_payload,
      &result_payload_len,
      out_rpc_error_code,
      errbuf,
      errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gizclaw_rpc_v1_ServerRegisterResponse response =
      gizclaw_rpc_v1_ServerRegisterResponse_init_zero;
  pb_istream_t response_stream =
      pb_istream_from_buffer(result_payload, (size_t)result_payload_len);
  bool decoded = pb_decode(
      &response_stream,
      gizclaw_rpc_v1_ServerRegisterResponse_fields,
      &response);
  free(result_payload);
  if (!decoded) {
    return fail(errbuf, errbuf_len, "decode registration response", GZC_ERR_RPC);
  }
  rc = copy_c_string(
      out_runtime_profile_name,
      out_runtime_profile_name_len,
      response.runtime_profile_name,
      errbuf,
      errbuf_len,
      "registration runtime profile");
  if (rc != GZC_OK) {
    return rc;
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_firmware_get(
    gzc_cgo_session_t *session,
    int channel,
    int *out_channel,
    int *out_has_description,
    char *out_description,
    unsigned long out_description_len,
    char *out_url,
    unsigned long out_url_len,
    char *out_sha256,
    unsigned long out_sha256_len,
    long long *out_size,
    int *out_rpc_error_code,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || out_channel == NULL || out_has_description == NULL ||
      out_description == NULL || out_description_len == 0 ||
      out_url == NULL || out_url_len == 0 || out_sha256 == NULL ||
      out_sha256_len == 0 || out_size == NULL) {
    return fail(errbuf, errbuf_len, "firmware get", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_channel = 0;
  *out_has_description = 0;
  out_description[0] = 0;
  out_url[0] = 0;
  out_sha256[0] = 0;
  *out_size = 0;
  if (out_rpc_error_code != NULL) {
    *out_rpc_error_code = 0;
  }

  gizclaw_rpc_v1_FirmwareGetRequest request =
      gizclaw_rpc_v1_FirmwareGetRequest_init_zero;
  request.channel = (gizclaw_rpc_v1_FirmwareChannelName)channel;
  unsigned char request_payload[gizclaw_rpc_v1_FirmwareGetRequest_size];
  pb_ostream_t request_stream =
      pb_ostream_from_buffer(request_payload, sizeof(request_payload));
  if (!pb_encode(
          &request_stream,
          gizclaw_rpc_v1_FirmwareGetRequest_fields,
          &request)) {
    return fail(errbuf, errbuf_len, "encode firmware get request", GZC_ERR_RPC);
  }

  unsigned char *result_payload = NULL;
  unsigned long result_payload_len = 0;
  int rc = gzc_cgo_session_call_rpc_payload(
      session,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_FIRMWARE_GET,
      request_payload,
      (unsigned long)request_stream.bytes_written,
      &result_payload,
      &result_payload_len,
      out_rpc_error_code,
      errbuf,
      errbuf_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gizclaw_rpc_v1_FirmwareGetResponse response =
      gizclaw_rpc_v1_FirmwareGetResponse_init_zero;
  pb_istream_t response_stream =
      pb_istream_from_buffer(result_payload, (size_t)result_payload_len);
  bool decoded = pb_decode(
      &response_stream,
      gizclaw_rpc_v1_FirmwareGetResponse_fields,
      &response);
  free(result_payload);
  if (!decoded) {
    return fail(errbuf, errbuf_len, "decode firmware get response", GZC_ERR_RPC);
  }
  int copy_rc = GZC_OK;
  *out_channel = (int)response.channel;
  *out_has_description = response.has_description ? 1 : 0;
  if (response.has_description) {
    copy_rc = copy_c_string(
        out_description,
        out_description_len,
        response.description,
        errbuf,
        errbuf_len,
        "firmware description");
    if (copy_rc != GZC_OK) {
      return copy_rc;
    }
  }
  copy_rc = copy_c_string(
      out_url, out_url_len, response.url, errbuf, errbuf_len, "firmware URL");
  if (copy_rc != GZC_OK) {
    return copy_rc;
  }
  copy_rc = copy_c_string(
      out_sha256, out_sha256_len, response.sha256, errbuf, errbuf_len,
      "firmware SHA-256");
  if (copy_rc != GZC_OK) {
    return copy_rc;
  }
  *out_size = (long long)response.size;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_call_stream_collect(
    gzc_cgo_session_t *session,
    unsigned method_id,
    const unsigned char *params_payload,
    unsigned long params_payload_len,
    const unsigned char *request_data,
    unsigned long request_data_len,
    gzc_cgo_stream_frame_t **out_frames,
    unsigned long *out_frame_count,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || method_id == 0 ||
      (params_payload == NULL && params_payload_len != 0) ||
      (request_data == NULL && request_data_len != 0) ||
      out_frames == NULL || out_frame_count == NULL) {
    return fail(errbuf, errbuf_len, "call stream", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_frames = NULL;
  *out_frame_count = 0;
  stream_collect_state_t state;
  memset(&state, 0, sizeof(state));
  const gizclaw_rpc_v1_RpcMethod method =
      (gizclaw_rpc_v1_RpcMethod)method_id;
  gzc_rpc_request_t *request = NULL;
  int rc = gzc_rpc_request_start_stream(
      session->client, rpc_service_for_method(method), method,
      gzc_str_from_parts((const char *)params_payload, (size_t)params_payload_len),
      5000, append_stream_frame, &state, &request);
  while (rc == GZC_OK && request_data_len > 0 &&
         (rc = gzc_rpc_request_write(request, request_data,
                                     (size_t)request_data_len)) ==
             GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(session->client, 10);
  }
  while (rc == GZC_OK &&
         (rc = gzc_rpc_request_finish_write(request)) ==
             GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(session->client, 10);
  }
  gzc_rpc_response_t response;
  memset(&response, 0, sizeof(response));
  if (rc == GZC_OK) {
    rc = wait_rpc_result(session, request, &response);
  }
  if (rc == GZC_OK && response.has_error) {
    rc = GZC_ERR_RPC;
  }
  gzc_rpc_request_destroy(request);
  if (rc != GZC_OK) {
    gzc_cgo_stream_frames_free(state.frames, state.count);
    return fail(errbuf, errbuf_len, "call stream", rc);
  }
  *out_frames = state.frames;
  *out_frame_count = state.count;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzc_cgo_stream_frames_free(gzc_cgo_stream_frame_t *frames, unsigned long frame_count) {
  if (frames == NULL) {
    return;
  }
  for (unsigned long i = 0; i < frame_count; i++) {
    free(frames[i].data);
  }
  free(frames);
}

int gzc_cgo_session_open_service_channel(
    gzc_cgo_session_t *session,
    unsigned long long service,
    int timeout_ms,
    gzc_service_channel_t **out_channel,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || out_channel == NULL) {
    return fail(errbuf, errbuf_len, "open service channel", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_channel = NULL;
  int rc = gzc_client_open_service_channel(session->client, service, timeout_ms, out_channel);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "open service channel", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_open_event_stream(
    gzc_cgo_session_t *session,
    int timeout_ms,
    gzc_event_stream_t **out_stream,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || out_stream == NULL) {
    return fail(errbuf, errbuf_len, "open event stream", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_stream = NULL;
  int rc = gzc_event_stream_open(session->client, timeout_ms, out_stream);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "open event stream", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_event_stream_send_audio_boundary(
    gzc_event_stream_t *stream,
    const char *stream_id,
    int begin,
    char *errbuf,
    unsigned long errbuf_len) {
  if (stream == NULL || stream_id == NULL || stream_id[0] == 0) {
    return fail(errbuf, errbuf_len, "send event stream boundary", GZC_ERR_INVALID_ARGUMENT);
  }
  gzc_peer_event_t event = gizclaw_events_v1_PeerEvent_init_zero;
  event.version = GZC_PEER_EVENT_VERSION;
  if (begin) {
    event.type = gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_BOS;
    event.which_payload = gizclaw_events_v1_PeerEvent_bos_tag;
    event.payload.bos.kind = gizclaw_events_v1_StreamKind_STREAM_KIND_AUDIO;
    (void)snprintf(
        event.payload.bos.stream_id,
        sizeof(event.payload.bos.stream_id),
        "%s",
        stream_id);
    (void)snprintf(
        event.payload.bos.label,
        sizeof(event.payload.bos.label),
        "%s",
        "cgo-chat");
    (void)snprintf(
        event.payload.bos.mime_type,
        sizeof(event.payload.bos.mime_type),
        "%s",
        "audio/opus");
  } else {
    event.type = gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_EOS;
    event.which_payload = gizclaw_events_v1_PeerEvent_eos_tag;
    event.payload.eos.kind = gizclaw_events_v1_StreamKind_STREAM_KIND_AUDIO;
    (void)snprintf(
        event.payload.eos.stream_id,
        sizeof(event.payload.eos.stream_id),
        "%s",
        stream_id);
    (void)snprintf(
        event.payload.eos.label,
        sizeof(event.payload.eos.label),
        "%s",
        "cgo-chat");
    (void)snprintf(
        event.payload.eos.mime_type,
        sizeof(event.payload.eos.mime_type),
        "%s",
        "audio/opus");
  }
  int rc = gzc_event_stream_send(stream, &event);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "send event stream boundary", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_event_stream_read_encoded(
    gzc_event_stream_t *stream,
    int timeout_ms,
    unsigned char **out_data,
    unsigned long *out_data_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (stream == NULL || out_data == NULL || out_data_len == NULL) {
    return fail(errbuf, errbuf_len, "read event stream", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_data = NULL;
  *out_data_len = 0;
  gzc_peer_event_t event = gizclaw_events_v1_PeerEvent_init_zero;
  int rc = gzc_event_stream_read(stream, timeout_ms, &event);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "read event stream", rc);
  }
  size_t encoded_size = 0;
  if (!pb_get_encoded_size(
          &encoded_size,
          gizclaw_events_v1_PeerEvent_fields,
          &event)) {
    return fail(errbuf, errbuf_len, "size event stream payload", GZC_ERR_RPC);
  }
  unsigned char *data = encoded_size == 0 ? NULL : (unsigned char *)malloc(encoded_size);
  if (encoded_size > 0 && data == NULL) {
    return fail(errbuf, errbuf_len, "allocate event stream payload", GZC_ERR_NO_MEMORY);
  }
  pb_ostream_t output = pb_ostream_from_buffer(data, encoded_size);
  if (!pb_encode(&output, gizclaw_events_v1_PeerEvent_fields, &event)) {
    free(data);
    return fail(errbuf, errbuf_len, "encode event stream payload", GZC_ERR_RPC);
  }
  *out_data = data;
  *out_data_len = (unsigned long)output.bytes_written;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzc_cgo_event_stream_close(gzc_event_stream_t *stream) {
  gzc_event_stream_close(stream);
}

int gzc_cgo_service_channel_send_json(
    gzc_service_channel_t *channel,
    const char *json,
    char *errbuf,
    unsigned long errbuf_len) {
  if (channel == NULL || json == NULL) {
    return fail(errbuf, errbuf_len, "service channel send json", GZC_ERR_INVALID_ARGUMENT);
  }
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.type = GZC_RPC_FRAME_JSON;
  frame.data = (const uint8_t *)json;
  frame.len = strlen(json);
  int rc = gzc_service_channel_send_frame(channel, &frame);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "service channel send json", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_service_channel_send_frame(
    gzc_service_channel_t *channel,
    int type,
    const unsigned char *data,
    unsigned long data_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (channel == NULL || (data == NULL && data_len != 0) ||
      !gzc_rpc_frame_type_valid((gzc_rpc_frame_type_t)type)) {
    return fail(errbuf, errbuf_len, "service channel send frame", GZC_ERR_INVALID_ARGUMENT);
  }
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.type = (gzc_rpc_frame_type_t)type;
  frame.data = data;
  frame.len = (size_t)data_len;
  int rc = gzc_service_channel_send_frame(channel, &frame);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "service channel send frame", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_service_channel_read_frame(
    gzc_service_channel_t *channel,
    int timeout_ms,
    int *out_type,
    unsigned char **out_data,
    unsigned long *out_data_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (channel == NULL || out_type == NULL || out_data == NULL || out_data_len == NULL) {
    return fail(errbuf, errbuf_len, "service channel read frame", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_type = 0;
  *out_data = NULL;
  *out_data_len = 0;
  gzc_buf_t frame_bytes;
  gzc_buf_init(&frame_bytes);
  int rc = gzc_service_channel_read_frame(channel, timeout_ms, &frame_bytes);
  if (rc != GZC_OK) {
    gzc_buf_free(&frame_bytes, gzc_default_platform());
    return fail(errbuf, errbuf_len, "service channel read frame", rc);
  }
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  rc = gzc_rpc_frame_decode(frame_bytes.data, frame_bytes.len, &frame);
  if (rc != GZC_OK) {
    gzc_buf_free(&frame_bytes, gzc_default_platform());
    return fail(errbuf, errbuf_len, "decode service channel frame", rc);
  }
  unsigned char *data = NULL;
  if (frame.len > 0) {
    data = (unsigned char *)malloc(frame.len);
    if (data == NULL) {
      gzc_buf_free(&frame_bytes, gzc_default_platform());
      return fail(errbuf, errbuf_len, "copy service channel frame", GZC_ERR_NO_MEMORY);
    }
    memcpy(data, frame.data, frame.len);
  }
  *out_type = (int)frame.type;
  *out_data = data;
  *out_data_len = (unsigned long)frame.len;
  gzc_buf_free(&frame_bytes, gzc_default_platform());
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzc_cgo_service_channel_close(gzc_service_channel_t *channel) {
  gzc_service_channel_close(channel);
}

int gzc_cgo_session_send_packet(
    gzc_cgo_session_t *session,
    unsigned char protocol,
    const unsigned char *payload,
    unsigned long payload_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || (payload == NULL && payload_len > 0)) {
    return fail(errbuf, errbuf_len, "send packet", GZC_ERR_INVALID_ARGUMENT);
  }
  int rc = gzc_client_send_packet(session->client, protocol, payload, payload_len);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "send packet", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_transport_send_counts(
    gzc_cgo_session_t *session,
    unsigned long long *out_packet_data_channel_calls,
    unsigned long long *out_opus_rtp_calls) {
  if (session == NULL || out_packet_data_channel_calls == NULL ||
      out_opus_rtp_calls == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  uint64_t packet_calls = 0;
  uint64_t opus_calls = 0;
  int rc = gzc_cgo_backend_transport_send_counts(
      &session->backend, &packet_calls, &opus_calls);
  if (rc != GZC_OK) {
    return rc;
  }
  *out_packet_data_channel_calls = (unsigned long long)packet_calls;
  *out_opus_rtp_calls = (unsigned long long)opus_calls;
  return GZC_OK;
}

int gzc_cgo_session_transport_snapshot(
    gzc_cgo_session_t *session,
    gzc_cgo_transport_snapshot_t *out_snapshot) {
  if (session == NULL || out_snapshot == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_snapshot, 0, sizeof(*out_snapshot));
  out_snapshot->backend_handle =
      (unsigned long long)session->backend.handle;
  out_snapshot->packet_channel_id =
      session->backend.packet_channel.in_use
          ? session->backend.packet_channel.id
          : 0;
  out_snapshot->packet_ready =
      session->backend.packet_channel.in_use ? 1 : 0;
  out_snapshot->media_ready =
      session->backend.opus_callback != NULL ? 1 : 0;
  out_snapshot->next_local_channel_id =
      session->backend.next_local_channel_id;
  out_snapshot->active_rpc_channel_id = 0;
  for (unsigned long i = 0; i < GZC_CGO_MAX_LOCAL_CHANNELS; i++) {
    struct gzc_rtc_channel *channel = &session->backend.local_channels[i];
    if (!channel->in_use) {
      continue;
    }
    if (strcmp(channel->label, "giznet/v1/service/32") == 0) {
      out_snapshot->event_channel_id = channel->id;
      continue;
    }
    if (strcmp(channel->label, "giznet/v1/service/0") == 0 &&
        out_snapshot->rpc_channel_count < GZC_CGO_E2E_MAX_CHANNELS) {
      out_snapshot->rpc_channel_ids[out_snapshot->rpc_channel_count] =
          channel->id;
      out_snapshot->rpc_channel_count++;
    }
  }
  return GZC_OK;
}

int gzc_cgo_session_send_battery_telemetry(
    gzc_cgo_session_t *session,
    double percent,
    int charging,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL) {
    return fail(errbuf, errbuf_len, "send battery telemetry", GZC_ERR_INVALID_ARGUMENT);
  }
  gzc_telemetry_observation_t observation;
  memset(&observation, 0, sizeof(observation));
  observation.kind = GZC_TELEMETRY_OBSERVATION_BATTERY;
  observation.battery.has_percent = true;
  observation.battery.percent = percent;
  observation.battery.has_charging = true;
  observation.battery.charging = charging != 0;

  gzc_telemetry_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.observations = &observation;
  frame.observation_count = 1;
  int rc = gzc_client_send_telemetry(session->client, &frame);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "send battery telemetry", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_send_full_telemetry(
    gzc_cgo_session_t *session,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL) {
    return fail(errbuf, errbuf_len, "send full telemetry", GZC_ERR_INVALID_ARGUMENT);
  }
  gzc_telemetry_observation_t observations[4];
  memset(observations, 0, sizeof(observations));

  observations[0].kind = GZC_TELEMETRY_OBSERVATION_BATTERY;
  observations[0].battery.has_percent = true;
  observations[0].battery.percent = 91;
  observations[0].battery.has_charging = true;
  observations[0].battery.charging = true;
  observations[0].battery.has_voltage_mv = true;
  observations[0].battery.voltage_mv = 4120;

  observations[1].observed_at_delta_ms = 10;
  observations[1].kind = GZC_TELEMETRY_OBSERVATION_GNSS;
  observations[1].gnss.latitude = 31.2304;
  observations[1].gnss.longitude = 121.4737;
  observations[1].gnss.has_altitude_m = true;
  observations[1].gnss.altitude_m = 12.5;
  observations[1].gnss.has_accuracy_m = true;
  observations[1].gnss.accuracy_m = 4.2;

  observations[2].observed_at_delta_ms = 20;
  observations[2].kind = GZC_TELEMETRY_OBSERVATION_NETWORK;
  observations[2].network.has_rssi_dbm = true;
  observations[2].network.rssi_dbm = -67;
  observations[2].network.has_signal_level = true;
  observations[2].network.signal_level = 4;
  observations[2].network.has_rat = true;
  observations[2].network.rat = gzc_str_from_cstr("lte");
  observations[2].network.has_operator_name = true;
  observations[2].network.operator_name = gzc_str_from_cstr("test-operator");
  observations[2].network.has_connected = true;
  observations[2].network.connected = true;

  observations[3].observed_at_delta_ms = 30;
  observations[3].kind = GZC_TELEMETRY_OBSERVATION_SYSTEM;
  observations[3].system.has_uptime_seconds = true;
  observations[3].system.uptime_seconds = 3600;
  observations[3].system.has_free_memory_bytes = true;
  observations[3].system.free_memory_bytes = 262144;
  observations[3].system.has_temperature_c = true;
  observations[3].system.temperature_c = 36.5;
  observations[3].system.has_firmware_version = true;
  observations[3].system.firmware_version = gzc_str_from_cstr("e2e-cgo-fw");
  observations[3].system.has_software_version = true;
  observations[3].system.software_version = gzc_str_from_cstr("e2e-cgo-sw");
  observations[3].system.has_hardware_version = true;
  observations[3].system.hardware_version = gzc_str_from_cstr("e2e-cgo-hw");

  gzc_telemetry_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.sequence = 1;
  frame.observations = observations;
  frame.observation_count = sizeof(observations) / sizeof(observations[0]);
  int rc = gzc_client_send_telemetry(session->client, &frame);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "send full telemetry", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_read_packet(
    gzc_cgo_session_t *session,
    int timeout_ms,
    unsigned char *out_protocol,
    unsigned char **out_payload,
    unsigned long *out_payload_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || out_protocol == NULL || out_payload == NULL || out_payload_len == NULL) {
    return fail(errbuf, errbuf_len, "read packet", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_protocol = 0;
  *out_payload = NULL;
  *out_payload_len = 0;
  gzc_buf_t payload;
  gzc_buf_init(&payload);
  uint8_t protocol = 0;
  int rc = gzc_client_read_packet(session->client, timeout_ms, &protocol, &payload);
  if (rc != GZC_OK) {
    gzc_buf_free(&payload, gzc_default_platform());
    return fail(errbuf, errbuf_len, "read packet", rc);
  }
  unsigned char *copy = NULL;
  if (payload.len > 0) {
    copy = (unsigned char *)malloc(payload.len);
    if (copy == NULL) {
      gzc_buf_free(&payload, gzc_default_platform());
      return fail(errbuf, errbuf_len, "copy packet", GZC_ERR_NO_MEMORY);
    }
    memcpy(copy, payload.data, payload.len);
  }
  *out_protocol = protocol;
  *out_payload = copy;
  *out_payload_len = (unsigned long)payload.len;
  gzc_buf_free(&payload, gzc_default_platform());
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_poll(gzc_cgo_session_t *session, int timeout_ms, char *errbuf, unsigned long errbuf_len) {
  if (session == NULL || session->client == NULL) {
    return fail(errbuf, errbuf_len, "poll", GZC_ERR_INVALID_ARGUMENT);
  }
  int rc = gzc_client_poll(session->client, timeout_ms);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "poll", rc);
  }
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzc_cgo_free(void *ptr) {
  free(ptr);
}
