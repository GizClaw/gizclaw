#include "sdk_client.h"

#include "../../../../sdk/c/gizclaw/cgobackend/gzc_cgo_backend.h"
#include "gzc.h"
#include "gzc_rpc_generated.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct gzc_cgo_session {
  gzc_cgo_backend_t backend;
  gzc_http_vtable_t http;
  gzc_platform_crypto_t crypto;
  gzc_webrtc_vtable_t webrtc;
  gzc_client_t *client;
};

static int fail(char *errbuf, unsigned long errbuf_len, const char *message, int rc) {
  if (errbuf != NULL && errbuf_len > 0) {
    (void)snprintf(errbuf, errbuf_len, "%s: %s (%d)", message, gzc_status_string((gzc_status_t)rc), rc);
  }
  return rc == GZC_OK ? GZC_ERR_RPC : rc;
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

  int rc = gzc_cgo_backend_init(&session->backend);
  if (rc != GZC_OK) {
    free(session);
    return fail(errbuf, errbuf_len, "backend init", rc);
  }

  gzc_cgo_backend_http_vtable(&session->backend, &session->http);
  gzc_cgo_backend_crypto_vtable(&session->backend, &session->crypto);
  gzc_cgo_backend_webrtc_vtable(&session->backend, &session->webrtc);

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

  rc = gzc_client_create(&config, &session->client);
  if (rc != GZC_OK) {
    gzc_cgo_backend_deinit(&session->backend);
    free(session);
    return fail(errbuf, errbuf_len, "client create", rc);
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

int gzc_cgo_session_call_json(
    gzc_cgo_session_t *session,
    const char *method,
    const char *params_json,
    char **out_result_json,
    unsigned long *out_result_json_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || method == NULL || params_json == NULL || out_result_json == NULL || out_result_json_len == NULL) {
    return fail(errbuf, errbuf_len, "call json", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_result_json = NULL;
  *out_result_json_len = 0;

  gzc_rpc_response_t response;
  memset(&response, 0, sizeof(response));
  int rc = gzc_rpc_call_json(
      session->client,
      gzc_str_from_cstr(method),
      gzc_str_from_cstr(params_json),
      &response);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "call json", rc);
  }
  if (response.has_error) {
    return fail(errbuf, errbuf_len, "rpc error", GZC_ERR_RPC);
  }

  char *result = (char *)malloc(response.result_json.len + 1);
  if (result == NULL) {
    return fail(errbuf, errbuf_len, "copy result", GZC_ERR_NO_MEMORY);
  }
  memcpy(result, response.result_json.data, response.result_json.len);
  result[response.result_json.len] = '\0';
  *out_result_json = result;
  *out_result_json_len = (unsigned long)response.result_json.len;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

int gzc_cgo_session_call_stream_collect(
    gzc_cgo_session_t *session,
    const char *method,
    const char *params_json,
    gzc_cgo_stream_frame_t **out_frames,
    unsigned long *out_frame_count,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || method == NULL || params_json == NULL || out_frames == NULL || out_frame_count == NULL) {
    return fail(errbuf, errbuf_len, "call stream", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_frames = NULL;
  *out_frame_count = 0;
  stream_collect_state_t state;
  memset(&state, 0, sizeof(state));
  int rc = gzc_rpc_call_stream(
      session->client,
      gzc_str_from_cstr(method),
      gzc_str_from_cstr(params_json),
      append_stream_frame,
      &state);
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
  if (session == NULL || session->webrtc.peer_poll == NULL) {
    return fail(errbuf, errbuf_len, "poll", GZC_ERR_INVALID_ARGUMENT);
  }
  int rc = session->webrtc.peer_poll(&session->backend.peer, timeout_ms);
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
