#include "gzc.h"
#include "pb_decode.h"
#include "pb_encode.h"

#include <stddef.h>
#include <stdio.h>
#include <string.h>

_Static_assert(
    sizeof(gzc_peer_event_t) < 10000,
    "Peer Event must retain the Nanopb oneof union layout");
_Static_assert(
    sizeof(((gizclaw_rpc_v1_APIKeyCreateRequest *)0)->display_name) == 81,
    "API key display name must retain its bounded Nanopb capacity");
_Static_assert(
    sizeof(((gizclaw_rpc_v1_APIKeyCreateResponse *)0)->api_key) == 96,
    "API key secret must retain its bounded Nanopb capacity");
_Static_assert(
    sizeof(((gizclaw_rpc_v1_APIKey *)0)->api_key) == 96,
    "listed API key must retain its bounded Nanopb capacity");
_Static_assert(
    sizeof(((gizclaw_rpc_v1_APIKeyListResponse *)0)->items) /
            sizeof(((gizclaw_rpc_v1_APIKeyListResponse *)0)->items[0]) ==
        100,
    "API key list must retain its bounded Nanopb capacity");

struct gzc_rtc_peer {
  int unused;
};

struct gzc_rtc_channel {
  int id;
};

typedef enum {
  FAKE_RESPONSE_PROTO = 0,
  FAKE_RESPONSE_PROTO_CONTINUATION = 1,
  FAKE_RESPONSE_PROTO_OVERSIZED_CONTINUATION = 2,
  FAKE_RESPONSE_BINARY_STREAM = 3,
  FAKE_RESPONSE_PROTO_ERROR = 4,
  FAKE_RESPONSE_SPEECH_TRANSCRIBE = 5,
  FAKE_RESPONSE_SPEECH_SYNTHESIZE = 6,
  FAKE_RESPONSE_SPEECH_EXTRACT = 7,
  FAKE_RESPONSE_SPEED_TEST = 8,
  FAKE_RESPONSE_DEFERRED_PROTO = 9
} fake_response_mode_t;

typedef struct {
  int64_t instant_ms;
  int64_t unix_ms;
  int64_t instant_step_ms;
} fake_clock_t;

typedef struct {
  gzc_webrtc_callbacks_t callbacks;
  struct gzc_rtc_peer peer;
  struct gzc_rtc_channel packet_channel;
  struct gzc_rtc_channel service_channels[16];
  struct gzc_rtc_channel edge_channel;
  struct gzc_rtc_channel remote_channels[GZC_RPC_MAX_INBOUND_CHANNELS + 1u];
  gzc_buf_t sent;
  gzc_buf_t outgoing;
  gzc_buf_t native_sent;
  gzc_buf_t opus_sent;
  gzc_rtc_opus_frame_cb opus_callback;
  void *opus_callback_userdata;
  uint8_t pending_opus[32];
  size_t pending_opus_len;
  int opus_register_count;
  int opus_unregister_count;
  int opus_send_count;
  int opus_send_result;
  int opus_register_result;
  const gzc_platform_t *platform;
  fake_clock_t *clock;
  uint64_t buffered_amount;
  uint64_t low_threshold;
  uint64_t threshold_values[8];
  gzc_rtc_channel_t *threshold_channels[8];
  size_t max_send_len;
  size_t threshold_count;
  int send_calls;
  int fail_send_call;
  int would_block_send_call;
  int poll_count;
  int poll_result_once;
  int last_poll_timeout_ms;
  int create_channel_count;
  size_t next_service_channel;
  int next_service_channel_id;
  bool service_channel_in_use[16];
  gzc_rtc_channel_state_t terminal_on_send;
  gzc_client_t *client;
  gzc_rtc_channel_t *last_send_channel;
  bool edge_channel_in_use;
  int peer_close_count;
  int close_count;
  int stale_close_count;
  gzc_rtc_channel_t *last_closed;
  gzc_rtc_channel_t *terminal_channels[32];
  size_t terminal_channel_count;
  int ice_server_count;
  bool offer_started;
  bool drain_on_poll;
  bool emit_low_event;
  fake_response_mode_t response_mode;
  bool speed_request_seen;
  int64_t speed_upload_bytes;
  int64_t speed_ack_up_bytes;
  int64_t speed_ack_down_bytes;
  int64_t speed_download_bytes;
  gzc_rtc_channel_t *speed_response_channel;
  bool speed_upload_eos_seen;
  bool speed_response_eos_sent;
  bool speed_full_duplex_observed;
  bool speed_flood_download;
  bool defer_next_service_open;
  bool close_peer_on_poll;
} fake_webrtc_t;

typedef struct {
  const gzc_platform_t *platform;
  const char *server_info_body;
  const char *expected_post_url;
  int get_count;
  int post_count;
} fake_http_t;

typedef struct {
  const gzc_platform_t *platform;
} fake_crypto_t;

typedef enum {
  FAKE_RPC_PROVIDER_SUCCESS = 0,
  FAKE_RPC_PROVIDER_ERROR = 1,
  FAKE_RPC_PROVIDER_NO_RESPONSE = 2
} fake_rpc_provider_mode_t;

typedef struct {
  int call_count;
  int method;
  size_t last_payload_len;
  fake_rpc_provider_mode_t mode;
} fake_rpc_provider_t;

typedef struct {
  int call_count;
} fake_tool_handler_t;

static fake_webrtc_t *global_fake_webrtc;
static bool fail_next_malloc;
static bool fail_next_realloc;

static int send_fake_speed_download(fake_webrtc_t *fake);

static void *test_malloc(void *userdata, size_t size) {
  (void)userdata;
  if (fail_next_malloc) {
    fail_next_malloc = false;
    return NULL;
  }
  const gzc_platform_t *platform = gzc_default_platform();
  return platform->malloc(platform->userdata, size);
}

static void *test_realloc(void *userdata, void *ptr, size_t size) {
  (void)userdata;
  if (fail_next_realloc) {
    fail_next_realloc = false;
    return NULL;
  }
  const gzc_platform_t *platform = gzc_default_platform();
  return platform->realloc(platform->userdata, ptr, size);
}

static int test_tool_handler(
    void *userdata,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata) {
  fake_tool_handler_t *handler = (fake_tool_handler_t *)userdata;
  if (handler == NULL || respond == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gizclaw_rpc_v1_ToolInvokeRequest request =
      gizclaw_rpc_v1_ToolInvokeRequest_init_zero;
  pb_istream_t input =
      pb_istream_from_buffer((const pb_byte_t *)request_payload.data,
                             request_payload.len);
  if (!pb_decode(&input, gizclaw_rpc_v1_ToolInvokeRequest_fields, &request) ||
      strcmp(request.invoke_name, "volume_set") != 0) {
    return GZC_ERR_RPC;
  }
  handler->call_count++;
  gizclaw_rpc_v1_ToolInvokeResponse result =
      gizclaw_rpc_v1_ToolInvokeResponse_init_zero;
  strcpy(result.data_json, "{\"ok\":true}");
  uint8_t payload[32];
  pb_ostream_t output = pb_ostream_from_buffer(payload, sizeof(payload));
  if (!pb_encode(&output, gizclaw_rpc_v1_ToolInvokeResponse_fields, &result)) {
    return GZC_ERR_RPC;
  }
  const gzc_rpc_provider_response_t response = {
      .payload = payload,
      .payload_len = output.bytes_written,
  };
  return respond(respond_userdata, &response);
}

static int test_rpc_provider(
    void *userdata,
    int method,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata) {
  fake_rpc_provider_t *provider = (fake_rpc_provider_t *)userdata;
  if (provider == NULL || respond == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  provider->call_count++;
  provider->method = method;
  provider->last_payload_len = request_payload.len;
  if (provider->mode == FAKE_RPC_PROVIDER_NO_RESPONSE) {
    return GZC_OK;
  }
  if (provider->mode == FAKE_RPC_PROVIDER_ERROR) {
    char message[] = "provider denied";
    const gzc_rpc_provider_response_t response = {
        .has_error = true,
        .error_code =
            gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_FORBIDDEN,
        .error_message = {
            .data = message,
            .len = sizeof(message) - 1u,
        },
    };
    int rc = respond(respond_userdata, &response);
    memset(message, 'x', sizeof(message) - 1u);
    return rc;
  }
  uint8_t response_payload[] = {0x0a, 0x00};
  const gzc_rpc_provider_response_t response = {
      .payload = response_payload,
      .payload_len = sizeof(response_payload),
  };
  int rc = respond(respond_userdata, &response);
  memset(response_payload, 0xff, sizeof(response_payload));
  return rc;
}

static int64_t test_time_instant_ms(void *userdata) {
  fake_clock_t *clock = (fake_clock_t *)userdata;
  if (clock == NULL) {
    return 0;
  }
  clock->instant_ms += clock->instant_step_ms;
  return clock->instant_ms;
}

static int64_t test_time_unix_ms(void *userdata) {
  fake_clock_t *clock = (fake_clock_t *)userdata;
  return clock == NULL ? 0 : clock->unix_ms;
}

static bool str_eq_cstr(gzc_str_t value, const char *want) {
  size_t want_len = strlen(want);
  return value.len == want_len && strncmp(value.data, want, want_len) == 0;
}

static int fake_peer_create(void *userdata, const gzc_webrtc_callbacks_t *callbacks, gzc_rtc_peer_t **out_peer) {
  fake_webrtc_t *fake = (fake_webrtc_t *)userdata;
  fake->callbacks = *callbacks;
  *out_peer = &fake->peer;
  return GZC_OK;
}

static bool fake_channel_is_terminal(
    const fake_webrtc_t *fake,
    const gzc_rtc_channel_t *channel) {
  for (size_t i = 0; i < fake->terminal_channel_count; i++) {
    if (fake->terminal_channels[i] == channel) {
      return true;
    }
  }
  return false;
}

static void fake_emit_channel_state(
    fake_webrtc_t *fake,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info,
    gzc_rtc_channel_state_t state) {
  if (state == GZC_RTC_CHANNEL_OPEN) {
    for (size_t i = 0; i < fake->terminal_channel_count; i++) {
      if (fake->terminal_channels[i] == channel) {
        fake->terminal_channel_count--;
        fake->terminal_channels[i] =
            fake->terminal_channels[fake->terminal_channel_count];
        break;
      }
    }
  } else if ((state == GZC_RTC_CHANNEL_CLOSED ||
              state == GZC_RTC_CHANNEL_ERROR) &&
             !fake_channel_is_terminal(fake, channel) &&
             fake->terminal_channel_count <
                 sizeof(fake->terminal_channels) /
                     sizeof(fake->terminal_channels[0])) {
    fake->terminal_channels[fake->terminal_channel_count++] = channel;
    if (channel == &fake->edge_channel) {
      fake->edge_channel_in_use = false;
    }
    for (size_t i = 0; i < 16u; i++) {
      if (channel == &fake->service_channels[i]) {
        fake->service_channel_in_use[i] = false;
      }
    }
  }
  fake->callbacks.on_channel_state(
      fake->callbacks.userdata, &fake->peer, channel, info, state);
}

static void fake_channel_close(gzc_rtc_channel_t *channel) {
  if (global_fake_webrtc != NULL) {
    if (fake_channel_is_terminal(global_fake_webrtc, channel)) {
      global_fake_webrtc->stale_close_count++;
    }
    global_fake_webrtc->close_count++;
    global_fake_webrtc->last_closed = channel;
    if (channel == &global_fake_webrtc->edge_channel) {
      global_fake_webrtc->edge_channel_in_use = false;
    }
    for (size_t i = 0; i < 16u; i++) {
      if (channel == &global_fake_webrtc->service_channels[i]) {
        global_fake_webrtc->service_channel_in_use[i] = false;
      }
    }
  }
}

static void fake_peer_close(gzc_rtc_peer_t *peer) {
  (void)peer;
  if (global_fake_webrtc != NULL) {
    global_fake_webrtc->peer_close_count++;
  }
}

static int test_peer_set_opus_frame_callback(
    gzc_rtc_peer_t *peer,
    gzc_rtc_opus_frame_cb callback,
    void *callback_userdata) {
  fake_webrtc_t *fake = global_fake_webrtc;
  if (fake == NULL || peer != &fake->peer) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  fake->opus_callback = callback;
  fake->opus_callback_userdata = callback_userdata;
  if (callback == NULL) {
    fake->opus_unregister_count++;
  } else {
    fake->opus_register_count++;
    if (fake->opus_register_result != GZC_OK) {
      return fake->opus_register_result;
    }
  }
  return GZC_OK;
}

static int test_peer_send_opus(
    gzc_rtc_peer_t *peer,
    const uint8_t *opus,
    size_t opus_len) {
  fake_webrtc_t *fake = global_fake_webrtc;
  if (fake == NULL || peer != &fake->peer || opus == NULL || opus_len == 0u) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (fake->opus_send_result != GZC_OK) {
    return fake->opus_send_result;
  }
  gzc_buf_reset(&fake->opus_sent);
  int rc = gzc_buf_append(&fake->opus_sent, fake->platform, opus, opus_len);
  if (rc == GZC_OK) {
    fake->opus_send_count++;
  }
  return rc;
}

static int test_peer_create(void *userdata, const gzc_webrtc_callbacks_t *callbacks, gzc_rtc_peer_t **out_peer) {
  fake_webrtc_t *fake = (fake_webrtc_t *)userdata;
  global_fake_webrtc = fake;
  return fake_peer_create(userdata, callbacks, out_peer);
}

static int test_peer_start_offer(gzc_rtc_peer_t *peer) {
  fake_webrtc_t *fake = global_fake_webrtc;
  fake->offer_started = true;
  gzc_str_t offer = gzc_str_from_cstr("v=0\r\nfake-offer\r\n");
  fake->callbacks.on_local_sdp(fake->callbacks.userdata, peer, GZC_RTC_SDP_OFFER, offer);
  return GZC_OK;
}

static int test_peer_add_ice_server(gzc_rtc_peer_t *peer, gzc_str_t url, gzc_str_t username, gzc_str_t credential) {
  fake_webrtc_t *fake = global_fake_webrtc;
  (void)peer;
  if (fake == NULL || fake->offer_started || !str_eq_cstr(url, "turn:edge.example.com:3478?transport=udp") ||
      !str_eq_cstr(username, "edge\"node") || !str_eq_cstr(credential, "sec\\ret")) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  fake->ice_server_count++;
  return GZC_OK;
}

static int test_peer_set_remote_sdp(gzc_rtc_peer_t *peer, gzc_rtc_sdp_type_t type, gzc_str_t sdp) {
  fake_webrtc_t *fake = global_fake_webrtc;
  (void)peer;
  (void)type;
  (void)sdp;
  gzc_rtc_channel_info_t info;
  memset(&info, 0, sizeof(info));
  info.label = gzc_str_from_cstr("giznet/v1/packet");
  info.stream_id = 0;
  info.ordered = false;
  info.reliable = false;
  fake_emit_channel_state(
      fake, &fake->packet_channel, &info, GZC_RTC_CHANNEL_OPEN);
  return GZC_OK;
}

static gzc_rtc_channel_t *allocate_fake_service_channel(fake_webrtc_t *fake) {
  for (size_t offset = 0; offset < 15u; offset++) {
    size_t index = (fake->next_service_channel + offset) % 15u;
    if (!fake->service_channel_in_use[index]) {
      fake->service_channel_in_use[index] = true;
      fake->next_service_channel = (index + 1u) % 15u;
      fake->service_channels[index].id = ++fake->next_service_channel_id;
      return &fake->service_channels[index];
    }
  }
  return NULL;
}

static int test_peer_create_data_channel(gzc_rtc_peer_t *peer, const gzc_rtc_channel_config_t *config, gzc_rtc_channel_t **out_channel) {
  (void)peer;
  fake_webrtc_t *fake = global_fake_webrtc;
  if (config == NULL || config->label.data == NULL || out_channel == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (config->label.len == strlen("giznet/v1/packet") &&
      strncmp(config->label.data, "giznet/v1/packet", config->label.len) == 0) {
    if (config->ordered || config->reliable) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    *out_channel = &fake->packet_channel;
    if (fake->callbacks.on_channel_state != NULL) {
      gzc_rtc_channel_info_t info;
      memset(&info, 0, sizeof(info));
      info.label = gzc_str_from_cstr("giznet/v1/packet");
      info.stream_id = 0;
      info.ordered = false;
      info.reliable = false;
      fake_emit_channel_state(
          fake, &fake->packet_channel, &info, GZC_RTC_CHANNEL_OPEN);
    }
  } else if (config->label.len == strlen("giznet/v1/service/0") &&
             strncmp(config->label.data, "giznet/v1/service/0", config->label.len) == 0) {
    if (!config->ordered || !config->reliable) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    gzc_rtc_channel_t *channel =
        allocate_fake_service_channel(fake);
    if (channel == NULL) {
      return GZC_ERR_CHANNEL_LIMIT;
    }
    *out_channel = channel;
    if (fake->callbacks.on_channel_state != NULL) {
      gzc_rtc_channel_info_t info;
      memset(&info, 0, sizeof(info));
      info.label = gzc_str_from_cstr("giznet/v1/service/0");
      info.stream_id = 1;
      info.ordered = true;
      info.reliable = true;
      fake_emit_channel_state(fake, channel, &info, GZC_RTC_CHANNEL_OPEN);
    }
  } else if (
      (config->label.len == strlen("giznet/v1/service/49") &&
       strncmp(
           config->label.data,
           "giznet/v1/service/49",
           config->label.len) == 0) ||
      (config->label.len == strlen("giznet/v1/service/48") &&
       strncmp(
           config->label.data,
           "giznet/v1/service/48",
           config->label.len) == 0)) {
    if (!config->ordered || !config->reliable) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    gzc_rtc_channel_t *channel = NULL;
    if (config->label.data[config->label.len - 1u] == '9' &&
        !fake->edge_channel_in_use) {
      channel = &fake->edge_channel;
      fake->edge_channel_in_use = true;
    } else {
      channel = allocate_fake_service_channel(fake);
    }
    if (channel == NULL) {
      return GZC_ERR_CHANNEL_LIMIT;
    }
    *out_channel = channel;
    if (fake->defer_next_service_open) {
      fake->defer_next_service_open = false;
    } else if (fake->callbacks.on_channel_state != NULL) {
      gzc_rtc_channel_info_t info;
      memset(&info, 0, sizeof(info));
      info.label = config->label;
      info.stream_id =
          config->label.data[config->label.len - 1u] == '9' ? 49u : 48u;
      info.ordered = true;
      info.reliable = true;
      fake_emit_channel_state(fake, channel, &info, GZC_RTC_CHANNEL_OPEN);
    }
  } else if (config->label.len == strlen("giznet/v1/service/32") &&
             strncmp(config->label.data, "giznet/v1/service/32", config->label.len) == 0) {
    if (!config->ordered || !config->reliable) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    gzc_rtc_channel_t *channel = &fake->service_channels[15];
    if (fake->service_channel_in_use[15]) {
      return GZC_ERR_CHANNEL_LIMIT;
    }
    fake->service_channel_in_use[15] = true;
    *out_channel = channel;
    if (fake->callbacks.on_channel_state != NULL) {
      gzc_rtc_channel_info_t info;
      memset(&info, 0, sizeof(info));
      info.label = gzc_str_from_cstr("giznet/v1/service/32");
      info.stream_id = 32;
      info.ordered = true;
      info.reliable = true;
      fake_emit_channel_state(fake, channel, &info, GZC_RTC_CHANNEL_OPEN);
    }
  } else {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  fake->create_channel_count++;
  return GZC_OK;
}

static int test_peer_poll(gzc_rtc_peer_t *peer, int timeout_ms) {
  (void)peer;
  fake_webrtc_t *fake = global_fake_webrtc;
  fake->poll_count++;
  fake->last_poll_timeout_ms = timeout_ms;
  if (fake->clock != NULL) {
    fake->clock->instant_ms += timeout_ms > 0 ? timeout_ms : 1;
    fake->clock->unix_ms -= 1000;
  }
  if (fake->poll_result_once != GZC_OK) {
    int rc = fake->poll_result_once;
    fake->poll_result_once = GZC_OK;
    return rc;
  }
  if (fake->close_peer_on_poll) {
    fake->close_peer_on_poll = false;
    fake->callbacks.on_peer_state(
        fake->callbacks.userdata,
        &fake->peer,
        GZC_RTC_PEER_CLOSED);
  }
  if (fake->pending_opus_len != 0u && fake->opus_callback != NULL) {
    size_t len = fake->pending_opus_len;
    fake->pending_opus_len = 0u;
    fake->opus_callback(
        fake->opus_callback_userdata,
        &fake->peer,
        fake->pending_opus,
        len);
  }
  if (fake->drain_on_poll && fake->buffered_amount > fake->low_threshold) {
    fake->buffered_amount = fake->low_threshold;
    if (fake->emit_low_event && fake->callbacks.on_channel_buffered_amount_low != NULL) {
      fake->callbacks.on_channel_buffered_amount_low(
          fake->callbacks.userdata,
          &fake->peer,
          fake->last_send_channel);
    }
  }
  if (fake->response_mode == FAKE_RESPONSE_SPEED_TEST &&
      fake->speed_request_seen) {
    return send_fake_speed_download(fake);
  }
  return GZC_OK;
}

static int append_test_frame(const gzc_platform_t *platform, gzc_buf_t *out, gzc_rpc_frame_type_t type, const uint8_t *data, size_t len) {
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.type = type;
  frame.data = data;
  frame.len = len;
  return gzc_rpc_frame_encode(platform, &frame, out);
}

static int append_test_varint(const gzc_platform_t *platform, gzc_buf_t *out, uint64_t value) {
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

static int append_test_key(const gzc_platform_t *platform, gzc_buf_t *out, unsigned field, unsigned wire_type) {
  return append_test_varint(platform, out, ((uint64_t)field << 3) | wire_type);
}

static int append_test_proto_bytes(const gzc_platform_t *platform, gzc_buf_t *out, unsigned field, const uint8_t *data, size_t len) {
  int rc = append_test_key(platform, out, field, 2);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = append_test_varint(platform, out, len);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append(out, platform, data, len);
}

static int append_test_proto_varint(const gzc_platform_t *platform, gzc_buf_t *out, unsigned field, uint64_t value) {
  int rc = append_test_key(platform, out, field, 0);
  if (rc != GZC_OK) {
    return rc;
  }
  return append_test_varint(platform, out, value);
}

static int send_deferred_rpc_response(
    fake_webrtc_t *fake,
    gzc_rtc_channel_t *channel,
    int64_t server_time,
    bool fail_request_allocation) {
  gzc_buf_t result;
  gzc_buf_t response;
  gzc_buf_t framed;
  gzc_buf_init(&result);
  gzc_buf_init(&response);
  gzc_buf_init(&framed);
  int rc = append_test_proto_varint(
      fake->platform, &result, 1, (uint64_t)server_time);
  if (rc == GZC_OK) {
    rc = append_test_proto_bytes(
        fake->platform, &response, 1, (const uint8_t *)"1", 1u);
  }
  if (rc == GZC_OK) {
    rc = append_test_proto_bytes(
        fake->platform, &response, 2, result.data, result.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(
        fake->platform,
        &framed,
        GZC_RPC_FRAME_BINARY,
        response.data,
        response.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(
        fake->platform, &framed, GZC_RPC_FRAME_EOS, NULL, 0u);
  }
  if (rc == GZC_OK) {
    fail_next_realloc = fail_request_allocation;
    fake->callbacks.on_channel_message(
        fake->callbacks.userdata,
        &fake->peer,
        channel,
        NULL,
        framed.data,
        framed.len,
        false);
    fail_next_realloc = false;
  }
  gzc_buf_free(&result, fake->platform);
  gzc_buf_free(&response, fake->platform);
  gzc_buf_free(&framed, fake->platform);
  return rc;
}

static int encode_test_pb_message(
    const gzc_platform_t *platform,
    const pb_msgdesc_t *fields,
    const void *message,
    gzc_buf_t *out) {
  pb_ostream_t sizing = PB_OSTREAM_SIZING;
  if (!pb_encode(&sizing, fields, message)) {
    return GZC_ERR_RPC;
  }
  uint8_t *buf = (uint8_t *)platform->malloc(platform->userdata, sizing.bytes_written == 0 ? 1 : sizing.bytes_written);
  if (buf == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  pb_ostream_t stream = pb_ostream_from_buffer(buf, sizing.bytes_written);
  int rc = GZC_OK;
  if (!pb_encode(&stream, fields, message)) {
    rc = GZC_ERR_RPC;
  } else {
    rc = gzc_buf_append(out, platform, buf, sizing.bytes_written);
  }
  platform->free(platform->userdata, buf);
  return rc;
}

static int decode_test_pb_message(gzc_str_t payload, const pb_msgdesc_t *fields, void *message) {
  pb_istream_t stream = pb_istream_from_buffer((const pb_byte_t *)payload.data, payload.len);
  return pb_decode(&stream, fields, message) ? GZC_OK : GZC_ERR_RPC;
}

static bool count_repeated_message(pb_istream_t *stream, const pb_field_t *field, void **arg) {
  (void)field;
  size_t *count = (size_t *)(*arg);
  if (count == NULL) {
    return false;
  }
  (*count)++;
  return pb_read(stream, NULL, stream->bytes_left);
}

static int read_test_varint(const uint8_t *data, size_t len, size_t *offset, uint64_t *out) {
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

static int read_test_proto_method_id(gzc_str_t payload, unsigned *out_method_id) {
  size_t offset = 0;
  while (offset < payload.len) {
    uint64_t key = 0;
    int rc = read_test_varint((const uint8_t *)payload.data, payload.len, &offset, &key);
    if (rc != GZC_OK) {
      return rc;
    }
    unsigned field = (unsigned)(key >> 3);
    unsigned wire_type = (unsigned)(key & 0x7u);
    if (field == 2 && wire_type == 0) {
      uint64_t value = 0;
      rc = read_test_varint((const uint8_t *)payload.data, payload.len, &offset, &value);
      if (rc != GZC_OK) {
        return rc;
      }
      *out_method_id = (unsigned)value;
      return GZC_OK;
    }
    if (wire_type == 0) {
      uint64_t ignored = 0;
      rc = read_test_varint((const uint8_t *)payload.data, payload.len, &offset, &ignored);
    } else if (wire_type == 2) {
      uint64_t size = 0;
      rc = read_test_varint((const uint8_t *)payload.data, payload.len, &offset, &size);
      if (rc == GZC_OK && size <= payload.len - offset) {
        offset += (size_t)size;
      } else if (rc == GZC_OK) {
        rc = GZC_ERR_RPC;
      }
    } else {
      rc = GZC_ERR_RPC;
    }
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_ERR_RPC;
}

static size_t first_frame_size(const gzc_buf_t *bytes) {
  if (bytes == NULL || bytes->len < 4) {
    return 0;
  }
  return 4 + ((size_t)bytes->data[0] | ((size_t)bytes->data[1] << 8));
}

static int send_fake_speed_response_envelope(
    fake_webrtc_t *fake,
    gzc_rtc_channel_t *channel) {
  gzc_buf_t result;
  gzc_buf_t response;
  gzc_buf_t framed;
  gzc_buf_init(&result);
  gzc_buf_init(&response);
  gzc_buf_init(&framed);
  int rc = append_test_proto_varint(
      fake->platform, &result, 1, (uint64_t)fake->speed_ack_down_bytes);
  if (rc == GZC_OK) {
    rc = append_test_proto_varint(
        fake->platform, &result, 2, (uint64_t)fake->speed_ack_up_bytes);
  }
  if (rc == GZC_OK) {
    rc = append_test_proto_bytes(
        fake->platform, &response, 1,
        (const uint8_t *)"1", 1u);
  }
  if (rc == GZC_OK) {
    rc = append_test_proto_bytes(
        fake->platform, &response, 2, result.data, result.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(
        fake->platform, &framed, GZC_RPC_FRAME_BINARY,
        response.data, response.len);
  }
  if (rc == GZC_OK) {
    fake->callbacks.on_channel_message(
        fake->callbacks.userdata, &fake->peer, channel, NULL,
        framed.data, framed.len, false);
  }
  gzc_buf_free(&result, fake->platform);
  gzc_buf_free(&response, fake->platform);
  gzc_buf_free(&framed, fake->platform);
  return rc;
}

static int send_fake_speed_download(fake_webrtc_t *fake) {
  if (fake == NULL || fake->speed_response_channel == NULL ||
      fake->speed_response_eos_sent) {
    return GZC_OK;
  }
  if (fake->speed_flood_download) {
    static const uint8_t flood[66u * 1024u] = {0};
    fake->speed_flood_download = false;
    fake->callbacks.on_channel_message(
        fake->callbacks.userdata,
        &fake->peer,
        fake->speed_response_channel,
        NULL,
        flood,
        sizeof(flood),
        false);
    return GZC_OK;
  }
  gzc_buf_t framed;
  gzc_buf_init(&framed);
  static const uint8_t payload[32u * 1024u] = {0};
  int rc = GZC_OK;
  for (size_t frame_count = 0;
       rc == GZC_OK && frame_count < 16u &&
       fake->speed_download_bytes < fake->speed_ack_down_bytes;
       frame_count++) {
    gzc_buf_reset(&framed);
    size_t count =
        (size_t)(fake->speed_ack_down_bytes - fake->speed_download_bytes);
    if (count > sizeof(payload)) {
      count = sizeof(payload);
    }
    rc = append_test_frame(
        fake->platform,
        &framed,
        GZC_RPC_FRAME_BINARY,
        payload,
        count);
    if (rc == GZC_OK) {
      fake->speed_download_bytes += (int64_t)count;
      if (!fake->speed_upload_eos_seen &&
          (fake->speed_upload_bytes > 0 || fake->outgoing.len > 0)) {
        fake->speed_full_duplex_observed = true;
      }
    }
    if (rc == GZC_OK) {
      fake->callbacks.on_channel_message(
          fake->callbacks.userdata,
          &fake->peer,
          fake->speed_response_channel,
          NULL,
          framed.data,
          framed.len,
          false);
    }
  }
  gzc_buf_reset(&framed);
  if (rc == GZC_OK &&
      fake->speed_download_bytes >= fake->speed_ack_down_bytes &&
      fake->speed_upload_eos_seen) {
    rc = append_test_frame(
        fake->platform,
        &framed,
        GZC_RPC_FRAME_EOS,
        NULL,
        0);
    if (rc == GZC_OK) {
      fake->speed_response_eos_sent = true;
    }
    if (rc == GZC_OK) {
      fake->callbacks.on_channel_message(
          fake->callbacks.userdata,
          &fake->peer,
          fake->speed_response_channel,
          NULL,
          framed.data,
          framed.len,
          false);
    }
  }
  gzc_buf_free(&framed, fake->platform);
  return rc;
}

static int test_channel_send_frame(gzc_rtc_channel_t *channel, const uint8_t *data, size_t len, bool is_text) {
  fake_webrtc_t *fake = global_fake_webrtc;
  if (channel == &fake->packet_channel && !is_text) {
    gzc_buf_reset(&fake->sent);
    return gzc_buf_append(&fake->sent, fake->platform, data, len);
  }
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS + 1u; i++) {
    if (channel == &fake->remote_channels[i] && !is_text) {
      return gzc_buf_append(&fake->sent, fake->platform, data, len);
    }
  }
  if (channel == &fake->service_channels[15] && !is_text) {
    gzc_buf_reset(&fake->sent);
    return gzc_buf_append(
        &fake->sent, fake->platform, data, len);
  }
  bool known_channel = channel == &fake->edge_channel;
  for (size_t i = 0; i < 16 && !known_channel; i++) {
    known_channel = channel == &fake->service_channels[i];
  }
  if (!known_channel || is_text) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_rtc_channel_t *response_channel = channel;
  gzc_rpc_frame_t request_frame;
  int frame_rc = gzc_rpc_frame_decode(data, len, &request_frame);
  bool is_eos =
      frame_rc == GZC_OK && request_frame.type == GZC_RPC_FRAME_EOS;
  if (fake->response_mode == FAKE_RESPONSE_SPEED_TEST) {
    if (frame_rc != GZC_OK) {
      return frame_rc;
    }
    if (request_frame.type == GZC_RPC_FRAME_BINARY) {
      if (!fake->speed_request_seen) {
        unsigned method_id = 0;
        int rc = read_test_proto_method_id(
            gzc_str_from_parts(
                (const char *)request_frame.data, request_frame.len),
            &method_id);
        if (rc != GZC_OK ||
            method_id !=
                gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN) {
          return GZC_ERR_RPC;
        }
        fake->speed_request_seen = true;
        fake->speed_response_channel = response_channel;
        return send_fake_speed_response_envelope(
            fake,
            response_channel);
      } else {
        fake->speed_upload_bytes += (int64_t)request_frame.len;
      }
      return gzc_buf_append(
          &fake->sent, fake->platform, data, len);
    }
    if (request_frame.type != GZC_RPC_FRAME_EOS ||
        !fake->speed_request_seen ||
        fake->speed_upload_bytes != fake->speed_ack_up_bytes) {
      return GZC_ERR_RPC;
    }
    fake->speed_upload_eos_seen = true;
    return GZC_OK;
  }
  if (fake->response_mode == FAKE_RESPONSE_DEFERRED_PROTO) {
    return GZC_OK;
  }
  if ((fake->response_mode == FAKE_RESPONSE_SPEECH_TRANSCRIBE ||
       fake->response_mode == FAKE_RESPONSE_SPEECH_EXTRACT) &&
      !is_eos) {
    return gzc_buf_append(&fake->sent, fake->platform, data, len);
  }
  if (is_eos && fake->response_mode != FAKE_RESPONSE_SPEECH_TRANSCRIBE &&
      fake->response_mode != FAKE_RESPONSE_SPEECH_EXTRACT) {
    return GZC_OK;
  }
  gzc_buf_reset(&fake->sent);
  int rc = gzc_buf_append(&fake->sent, fake->platform, data, len);
  if (rc != GZC_OK) {
    return rc;
  }
  if (fake->response_mode == FAKE_RESPONSE_BINARY_STREAM ||
      fake->response_mode == FAKE_RESPONSE_SPEECH_SYNTHESIZE) {
    const char *response_id = "1";
    const uint8_t first[] = {0x01, 0x02};
    const uint8_t second[] = {0x03};
    gzc_buf_t response_result;
    gzc_buf_t response_payload;
    gzc_buf_t framed;
    gzc_buf_init(&response_result);
    gzc_buf_init(&response_payload);
    gzc_buf_init(&framed);
    if (fake->response_mode == FAKE_RESPONSE_SPEECH_SYNTHESIZE) {
      rc = append_test_proto_bytes(
          fake->platform,
          &response_result,
          1,
          (const uint8_t *)"audio/pcm",
          strlen("audio/pcm"));
    } else {
      rc = append_test_proto_varint(fake->platform, &response_result, 1, 3);
    }
    if (rc == GZC_OK && fake->response_mode == FAKE_RESPONSE_BINARY_STREAM) {
      rc = append_test_proto_varint(fake->platform, &response_result, 2, 0);
    }
    if (rc == GZC_OK) {
      rc = append_test_proto_bytes(fake->platform, &response_payload, 1, (const uint8_t *)response_id, strlen(response_id));
    }
    if (rc == GZC_OK) {
      rc = append_test_proto_bytes(fake->platform, &response_payload, 2, response_result.data, response_result.len);
    }
    if (rc == GZC_OK) {
      rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_BINARY, response_payload.data, response_payload.len);
    }
    if (rc == GZC_OK) {
      rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_BINARY, first, sizeof(first));
    }
    if (rc == GZC_OK) {
      rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_BINARY, second, sizeof(second));
    }
    if (rc == GZC_OK) {
      rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_EOS, NULL, 0);
    }
    if (rc == GZC_OK) {
      fake->callbacks.on_channel_message(
          fake->callbacks.userdata,
          &fake->peer,
          response_channel,
          NULL,
          framed.data,
          framed.len,
          false);
    }
    gzc_buf_free(&response_result, fake->platform);
    gzc_buf_free(&response_payload, fake->platform);
    gzc_buf_free(&framed, fake->platform);
    return rc;
  }
  if (fake->response_mode == FAKE_RESPONSE_PROTO_OVERSIZED_CONTINUATION) {
    gzc_buf_t framed;
    gzc_buf_init(&framed);
    uint8_t *chunk = (uint8_t *)fake->platform->malloc(fake->platform->userdata, GZC_RPC_MAX_FRAME_SIZE);
    if (chunk == NULL) {
      return GZC_ERR_NO_MEMORY;
    }
    memset(chunk, 0, GZC_RPC_MAX_FRAME_SIZE);
    for (size_t i = 0; i < 17 && rc == GZC_OK; i++) {
      rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_TEXT, chunk, GZC_RPC_MAX_FRAME_SIZE);
    }
    if (rc == GZC_OK) {
      rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_EOS, NULL, 0);
    }
    if (rc == GZC_OK) {
      fake->callbacks.on_channel_message(
          fake->callbacks.userdata,
          &fake->peer,
          response_channel,
          NULL,
          framed.data,
          framed.len,
          false);
    }
    fake->platform->free(fake->platform->userdata, chunk);
    gzc_buf_free(&framed, fake->platform);
    return rc;
  }
  const char *response_id = "1";
  gzc_buf_t response_result;
  gzc_buf_t response_error;
  gzc_buf_t response_payload;
  gzc_buf_t framed;
  gzc_buf_init(&response_result);
  gzc_buf_init(&response_error);
  gzc_buf_init(&response_payload);
  gzc_buf_init(&framed);
  if (fake->response_mode == FAKE_RESPONSE_PROTO_ERROR) {
    rc = append_test_proto_varint(fake->platform, &response_error, 1, 7);
    if (rc == GZC_OK) {
      rc = append_test_proto_bytes(fake->platform, &response_error, 2, (const uint8_t *)"denied", strlen("denied"));
    }
  } else if (fake->response_mode == FAKE_RESPONSE_SPEECH_TRANSCRIBE) {
    rc = append_test_proto_bytes(
        fake->platform,
        &response_result,
        1,
        (const uint8_t *)"hello",
        strlen("hello"));
  } else if (fake->response_mode == FAKE_RESPONSE_SPEECH_EXTRACT) {
    rc = append_test_proto_bytes(
        fake->platform,
        &response_result,
        1,
        (const uint8_t *)"name is GizClaw",
        strlen("name is GizClaw"));
    if (rc == GZC_OK) {
      rc = append_test_proto_bytes(
          fake->platform,
          &response_result,
          2,
          (const uint8_t *)"{\"name\":\"GizClaw\"}",
          strlen("{\"name\":\"GizClaw\"}"));
    }
  } else {
    rc = append_test_proto_varint(fake->platform, &response_result, 1, 99);
  }
  if (rc == GZC_OK) {
    rc = append_test_proto_bytes(fake->platform, &response_payload, 1, (const uint8_t *)response_id, strlen(response_id));
  }
  if (rc == GZC_OK && fake->response_mode == FAKE_RESPONSE_PROTO_ERROR) {
    rc = append_test_proto_bytes(fake->platform, &response_payload, 3, response_error.data, response_error.len);
  }
  if (rc == GZC_OK) {
    if (fake->response_mode != FAKE_RESPONSE_PROTO_ERROR) {
      rc = append_test_proto_bytes(fake->platform, &response_payload, 2, response_result.data, response_result.len);
    }
  }
  if (rc == GZC_OK && fake->response_mode == FAKE_RESPONSE_PROTO_CONTINUATION) {
    size_t split = response_payload.len / 2;
    rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_TEXT, response_payload.data, split);
    if (rc == GZC_OK) {
      rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_TEXT, response_payload.data + split, response_payload.len - split);
    }
  } else if (rc == GZC_OK) {
    rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_BINARY, response_payload.data, response_payload.len);
  }
  if (rc != GZC_OK) {
    gzc_buf_free(&response_result, fake->platform);
    gzc_buf_free(&response_error, fake->platform);
    gzc_buf_free(&response_payload, fake->platform);
    return rc;
  }
  gzc_rpc_frame_t eos_frame;
  memset(&eos_frame, 0, sizeof(eos_frame));
  eos_frame.type = GZC_RPC_FRAME_EOS;
  rc = gzc_rpc_frame_encode(fake->platform, &eos_frame, &framed);
  if (rc != GZC_OK) {
    gzc_buf_free(&response_result, fake->platform);
    gzc_buf_free(&response_error, fake->platform);
    gzc_buf_free(&response_payload, fake->platform);
    gzc_buf_free(&framed, fake->platform);
    return rc;
  }
  fake->callbacks.on_channel_message(
      fake->callbacks.userdata,
      &fake->peer,
      response_channel,
      NULL,
      framed.data,
      framed.len,
      false);
  gzc_buf_free(&response_result, fake->platform);
  gzc_buf_free(&response_error, fake->platform);
  gzc_buf_free(&response_payload, fake->platform);
  gzc_buf_free(&framed, fake->platform);
  return GZC_OK;
}

static void consume_test_bytes(gzc_buf_t *bytes, size_t len) {
  if (len >= bytes->len) {
    bytes->len = 0;
    if (bytes->data != NULL) {
      bytes->data[0] = 0;
    }
    return;
  }
  memmove(bytes->data, bytes->data + len, bytes->len - len);
  bytes->len -= len;
  bytes->data[bytes->len] = 0;
}

static int test_channel_send(gzc_rtc_channel_t *channel, const uint8_t *data, size_t len, bool is_text) {
  fake_webrtc_t *fake = global_fake_webrtc;
  if (fake == NULL || (data == NULL && len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  fake->send_calls++;
  if (fake->fail_send_call == fake->send_calls) {
    return GZC_ERR_WEBRTC;
  }
  if (fake->would_block_send_call == fake->send_calls) {
    return GZC_ERR_WOULD_BLOCK;
  }
  if (len > fake->max_send_len) {
    fake->max_send_len = len;
  }
  fake->buffered_amount += len;
  bool service_channel = channel == &fake->edge_channel;
  for (size_t i = 0; i < 16 && !service_channel; i++) {
    service_channel = channel == &fake->service_channels[i];
  }
  if (!service_channel || is_text) {
    return test_channel_send_frame(channel, data, len, is_text);
  }
  fake->last_send_channel = channel;
  int rc = gzc_buf_append(&fake->native_sent, fake->platform, data, len);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_buf_append(&fake->outgoing, fake->platform, data, len);
  while (rc == GZC_OK) {
    size_t frame_size = first_frame_size(&fake->outgoing);
    if (frame_size < 4 || fake->outgoing.len < frame_size) {
      break;
    }
    gzc_buf_t frame;
    gzc_buf_init(&frame);
    rc = gzc_buf_append(&frame, fake->platform, fake->outgoing.data, frame_size);
    consume_test_bytes(&fake->outgoing, frame_size);
    if (rc == GZC_OK) {
      rc = test_channel_send_frame(channel, frame.data, frame.len, false);
    }
    gzc_buf_free(&frame, fake->platform);
  }
  return rc;
}

static int test_channel_buffered_amount(gzc_rtc_channel_t *channel, uint64_t *out_bytes) {
  (void)channel;
  if (global_fake_webrtc == NULL || out_bytes == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_bytes = global_fake_webrtc->buffered_amount;
  return GZC_OK;
}

static int test_channel_set_buffered_amount_low_threshold(
    gzc_rtc_channel_t *channel,
    uint64_t bytes) {
  (void)channel;
  if (global_fake_webrtc == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  fake_webrtc_t *fake = global_fake_webrtc;
  fake->low_threshold = bytes;
  if (fake->threshold_count < sizeof(fake->threshold_values) / sizeof(fake->threshold_values[0])) {
    fake->threshold_values[fake->threshold_count] = bytes;
    fake->threshold_channels[fake->threshold_count] = channel;
    fake->threshold_count++;
  }
  return GZC_OK;
}

static void announce_remote_rpc(fake_webrtc_t *fake, size_t index) {
  gzc_rtc_channel_info_t info;
  memset(&info, 0, sizeof(info));
  info.label = gzc_str_from_cstr("giznet/v1/service/0");
  info.stream_id = (uint16_t)(3 + index);
  info.ordered = true;
  info.reliable = true;
  fake->remote_channels[index].id = (int)(3 + index);
  fake->callbacks.on_remote_channel(
      fake->callbacks.userdata, &fake->peer, &fake->remote_channels[index], &info);
  fake_emit_channel_state(
      fake, &fake->remote_channels[index], &info, GZC_RTC_CHANNEL_OPEN);
}

static void close_remote_rpc_with_state(
    fake_webrtc_t *fake,
    size_t index,
    gzc_rtc_channel_state_t state) {
  gzc_rtc_channel_info_t info;
  memset(&info, 0, sizeof(info));
  info.label = gzc_str_from_cstr("giznet/v1/service/0");
  info.stream_id = (uint16_t)(3 + index);
  info.ordered = true;
  info.reliable = true;
  fake_emit_channel_state(
      fake, &fake->remote_channels[index], &info, state);
}

static void close_remote_rpc(fake_webrtc_t *fake, size_t index) {
  close_remote_rpc_with_state(
      fake, index, GZC_RTC_CHANNEL_CLOSED);
}

typedef struct {
  size_t envelope_count;
  size_t frame_count;
  size_t binary_bytes;
  size_t eos_count;
} stream_count_t;

typedef struct {
  int code;
  size_t frame_count;
  size_t eos_count;
  bool has_error;
  bool message_ok;
} stream_error_t;

static int count_stream_frame(void *userdata, const gzc_rpc_frame_t *frame) {
  stream_count_t *count = (stream_count_t *)userdata;
  if (count == NULL || frame == NULL) {
    return GZC_ERR_RPC;
  }
  if (frame->type == GZC_RPC_FRAME_EOS) {
    if (frame->len != 0u) {
      return GZC_ERR_RPC;
    }
    count->eos_count++;
    return GZC_OK;
  }
  if (frame->type != GZC_RPC_FRAME_BINARY) {
    return GZC_ERR_RPC;
  }
  if (count->envelope_count == 0) {
    count->envelope_count++;
    return GZC_OK;
  }
  count->frame_count++;
  count->binary_bytes += frame->len;
  return GZC_OK;
}

static int block_stream_frame(void *userdata, const gzc_rpc_frame_t *frame) {
  (void)userdata;
  (void)frame;
  return GZC_ERR_WOULD_BLOCK;
}

static uint64_t test_rpc_service(gizclaw_rpc_v1_RpcMethod method) {
  return method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PEER_LOOKUP ||
                 method ==
                     gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PEER_ASSIGN ||
                 method ==
                     gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_ROUTE_RESOLVE ||
                 method ==
                     gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_API_KEY_RESOLVE
             ? 0x31u
             : 0u;
}

static gzc_rpc_request_t *test_last_rpc_request;

static int test_rpc_call(gzc_client_t *client,
                         gizclaw_rpc_v1_RpcMethod method,
                         gzc_str_t params,
                         gzc_rpc_response_t *out_response) {
  gzc_rpc_request_destroy(test_last_rpc_request);
  test_last_rpc_request = NULL;
  gzc_rpc_request_t *request = NULL;
  int rc = gzc_rpc_request_start(client, test_rpc_service(method), method,
                                 params, 5000, &request);
  while (rc == GZC_OK &&
         (rc = gzc_rpc_request_result(request, out_response)) ==
             GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(client, 10);
  }
  test_last_rpc_request = request;
  return rc;
}

static int test_rpc_call_stream(gzc_client_t *client,
                                gizclaw_rpc_v1_RpcMethod method,
                                gzc_str_t params,
                                gzc_rpc_frame_cb on_frame,
                                void *userdata) {
  gzc_rpc_request_t *request = NULL;
  int rc = gzc_rpc_request_start_stream(
      client, test_rpc_service(method), method, params, 5000, on_frame,
      userdata, &request);
  while (rc == GZC_OK &&
         (rc = gzc_rpc_request_finish_write(request)) ==
             GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(client, 10);
  }
  gzc_rpc_response_t response;
  while (rc == GZC_OK &&
         (rc = gzc_rpc_request_result(request, &response)) ==
             GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(client, 10);
  }
  gzc_rpc_request_destroy(request);
  return rc;
}

static int capture_stream_error_frame(void *userdata, const gzc_rpc_frame_t *frame) {
  stream_error_t *captured = (stream_error_t *)userdata;
  if (captured == NULL || frame == NULL) {
    return GZC_ERR_RPC;
  }
  if (frame->type == GZC_RPC_FRAME_EOS) {
    if (frame->len != 0u) {
      return GZC_ERR_RPC;
    }
    captured->eos_count++;
    return GZC_OK;
  }
  if (frame->type != GZC_RPC_FRAME_BINARY) {
    return GZC_ERR_RPC;
  }
  gzc_rpc_response_t response;
  int rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts((const char *)frame->data, frame->len), &response);
  if (rc != GZC_OK) {
    return rc;
  }
  captured->frame_count++;
  captured->has_error = response.has_error;
  captured->code = response.error.code;
  captured->message_ok = str_eq_cstr(response.error.message, "denied");
  return GZC_OK;
}

static int test_http_request(void *userdata, const gzc_http_request_t *request, gzc_http_response_t *out_response) {
  fake_http_t *fake = (fake_http_t *)userdata;
  if (request == NULL || out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  out_response->status_code = 200;
  gzc_buf_init(&out_response->body);
  if (request->method == GZC_HTTP_METHOD_GET) {
    fake->get_count++;
    if (!str_eq_cstr(request->url, "http://example.invalid:9820/server-info")) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    const char *body = fake->server_info_body == NULL
                           ? "{\"protocol\":\"gizclaw-webrtc\",\"public_key\":\"8mfzTdZB1JA43QmNAMWfTfkj5GC9TJxJFveThi9tvK6J\",\"endpoint\":\"ice.invalid:9820\",\"signaling_path\":\"/custom/offer\",\"ice_servers\":[{\"urls\":[\"turn:edge.example.com:3478?transport=udp\"],\"username\":\"edge\\\"node\",\"credential\":\"sec\\\\ret\"}]}"
                           : fake->server_info_body;
    return gzc_buf_append_cstr(&out_response->body, fake->platform, body);
  }
  fake->post_count++;
  const char *expected_post_url = fake->expected_post_url == NULL
                                      ? "http://example.invalid:9820/custom/offer"
                                      : fake->expected_post_url;
  if (request->method != GZC_HTTP_METHOD_POST ||
      !str_eq_cstr(request->url, expected_post_url) ||
      request->body == NULL || request->body_len == 0 ||
      request->header_count != GZC_SIGNALING_HEADER_COUNT) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzc_buf_append_cstr(&out_response->body, fake->platform, "v=0\r\nfake-answer\r\n");
}

static void test_http_response_free(void *userdata, gzc_http_response_t *response) {
  fake_http_t *fake = (fake_http_t *)userdata;
  gzc_buf_free(&response->body, fake->platform);
}

static int test_keypair_from_private(void *userdata, const gzc_key_t *private_key, gzc_keypair_t *out_keypair) {
  (void)userdata;
  if (private_key == NULL || out_keypair == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  out_keypair->private_key = *private_key;
  memset(&out_keypair->public_key, 0x22, sizeof(out_keypair->public_key));
  return GZC_OK;
}

static int test_dh(void *userdata, const gzc_keypair_t *local, const gzc_public_key_t *remote, gzc_key_t *out_shared) {
  (void)userdata;
  if (local == NULL || remote == NULL || out_shared == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_shared, 0x33, sizeof(*out_shared));
  return GZC_OK;
}

static int test_hkdf_sha256(
    void *userdata,
    const uint8_t *secret,
    size_t secret_len,
    const uint8_t *salt,
    size_t salt_len,
    gzc_str_t info,
    uint8_t *out,
    size_t out_len) {
  (void)userdata;
  (void)secret;
  (void)secret_len;
  (void)salt;
  (void)salt_len;
  (void)info;
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0x44, out_len);
  return GZC_OK;
}

static int test_aead_copy(
    void *userdata,
    gzc_cipher_mode_t mode,
    const uint8_t *key,
    size_t key_len,
    const uint8_t *nonce,
    size_t nonce_len,
    const uint8_t *input,
    size_t input_len,
    const uint8_t *aad,
    size_t aad_len,
    gzc_buf_t *out) {
  fake_crypto_t *fake = (fake_crypto_t *)userdata;
  (void)mode;
  (void)key;
  (void)key_len;
  (void)nonce;
  (void)nonce_len;
  (void)aad;
  (void)aad_len;
  if ((input == NULL && input_len != 0) || out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzc_buf_append(out, fake->platform, input, input_len);
}

static int expect(bool ok, const char *message) {
  if (!ok) {
    fprintf(stderr, "FAIL: %s\n", message);
    return 1;
  }
  return 0;
}

static int hex_nibble(char value) {
  if (value >= '0' && value <= '9') {
    return value - '0';
  }
  if (value >= 'a' && value <= 'f') {
    return value - 'a' + 10;
  }
  return -1;
}

static int test_peer_event_golden_vectors(void) {
  static const char *const vectors[] = {
      "0801100152220a0873747265616d2d611001180220022a0475736572320a617564696f2f6f707573",
      "080110025a4b0a0873747265616d2d611002180320022a09617373697374616e74320a617564696f2f6f7075733a220a1743484154524f4f4d5f4d454d4245525f52454d4f564544120772656d6f766564",
      "08011003621e0a0873747265616d2d62100118022209617373697374616e742a0368656c",
      "080110046a200a0873747265616d2d62100218032209617373697374616e742a0568656c6c6f",
      "0801100572100a0a6469726563742d612d6210021804",
      "080110067a180a06706565722d62120a6469726563742d612d6218022005",
      "080110078201190a0767726f75702d61120a67726f75702d726f6f6d18042006",
      "080110088a01170a0a776f726b666c6f772d6112076772616e742d611807",
  };
  for (size_t i = 0; i < sizeof(vectors) / sizeof(vectors[0]); ++i) {
    const char *hex = vectors[i];
    size_t hex_len = strlen(hex);
    uint8_t expected[128];
    if ((hex_len & 1u) != 0 || hex_len / 2u > sizeof(expected)) {
      return 1;
    }
    size_t expected_len = hex_len / 2u;
    for (size_t j = 0; j < expected_len; ++j) {
      int high = hex_nibble(hex[j * 2u]);
      int low = hex_nibble(hex[j * 2u + 1u]);
      if (high < 0 || low < 0) {
        return 1;
      }
      expected[j] = (uint8_t)((high << 4) | low);
    }
    gzc_peer_event_t event = gizclaw_events_v1_PeerEvent_init_zero;
    pb_istream_t input = pb_istream_from_buffer(expected, expected_len);
    if (!pb_decode(&input, gizclaw_events_v1_PeerEvent_fields, &event)) {
      return 1;
    }
    uint8_t encoded[128];
    pb_ostream_t output = pb_ostream_from_buffer(encoded, sizeof(encoded));
    if (!pb_encode(&output, gizclaw_events_v1_PeerEvent_fields, &event) ||
        output.bytes_written != expected_len ||
        memcmp(encoded, expected, expected_len) != 0) {
      return 1;
    }
  }
  return 0;
}

static int test_json_ascii_classification(void) {
  const char all_ascii_whitespace[] =
      "\t\n\v\f\r {\"value\":0}\t\n\v\f\r ";
  if (expect(
          gzc_json_validate_object(
              gzc_str_from_cstr(all_ascii_whitespace)) == GZC_OK,
          "accept every ASCII JSON whitespace byte") != 0) {
    return 1;
  }

  int64_t parsed = -1;
  if (expect(
          gzc_json_parse_i64(gzc_str_from_cstr("0"), &parsed) == GZC_OK &&
              parsed == 0,
          "accept lower ASCII digit boundary") != 0 ||
      expect(
          gzc_json_parse_i64(gzc_str_from_cstr("9"), &parsed) == GZC_OK &&
              parsed == 9,
          "accept upper ASCII digit boundary") != 0 ||
      expect(
          gzc_json_parse_i64(gzc_str_from_cstr("/"), &parsed) ==
              GZC_ERR_JSON,
          "reject byte below ASCII digit range") != 0 ||
      expect(
          gzc_json_parse_i64(gzc_str_from_cstr(":"), &parsed) ==
              GZC_ERR_JSON,
          "reject byte above ASCII digit range") != 0) {
    return 1;
  }

  const char high_digit_bytes[] = {(char)0x80, (char)0xff};
  for (size_t i = 0; i < sizeof(high_digit_bytes); ++i) {
    if (expect(
            gzc_json_parse_i64(
                gzc_str_from_parts(&high_digit_bytes[i], 1), &parsed) ==
                GZC_ERR_JSON,
            "reject non-ASCII byte as a digit") != 0) {
      return 1;
    }
  }

  const char high_separator_json[] = {
      '{', '"', 'v', '"', ':', '0', (char)0x80, '}'};
  if (expect(
          gzc_json_validate_object(gzc_str_from_parts(
              high_separator_json, sizeof(high_separator_json))) ==
              GZC_ERR_JSON,
          "reject non-ASCII byte as a JSON separator") != 0) {
    return 1;
  }

  const char high_string_json[] = {
      '{', '"', 'v', '"', ':', '"', (char)0x80, (char)0xff, '"', '}'};
  return expect(
      gzc_json_validate_object(gzc_str_from_parts(
          high_string_json, sizeof(high_string_json))) == GZC_OK,
      "preserve high bytes inside a delimited JSON string");
}

static int test_device_control_payload_bounds(void) {
  uint8_t buffer[128];
  gizclaw_rpc_v1_ClientWifiSavedForgetRequest forget =
      gizclaw_rpc_v1_ClientWifiSavedForgetRequest_init_zero;
  memset(forget.ssid, 'a', 32);
  forget.ssid[32] = '\0';
  pb_ostream_t output = pb_ostream_from_buffer(buffer, sizeof(buffer));
  if (expect(pb_encode(&output, gizclaw_rpc_v1_ClientWifiSavedForgetRequest_fields, &forget),
             "32-byte ssid encodes within the nanopb bound") != 0) {
    return 1;
  }
  gizclaw_rpc_v1_ClientWifiSavedForgetRequest decoded =
      gizclaw_rpc_v1_ClientWifiSavedForgetRequest_init_zero;
  pb_istream_t input = pb_istream_from_buffer(buffer, output.bytes_written);
  if (expect(pb_decode(&input, gizclaw_rpc_v1_ClientWifiSavedForgetRequest_fields, &decoded) &&
                 strcmp(decoded.ssid, forget.ssid) == 0,
             "32-byte ssid round trips") != 0) {
    return 1;
  }

  /* A 33-byte ssid is a valid protobuf string but exceeds the bounded field. */
  uint8_t oversized[64];
  size_t oversized_len = 0;
  oversized[oversized_len++] = 0x0a;
  oversized[oversized_len++] = 33;
  memset(oversized + oversized_len, 'b', 33);
  oversized_len += 33;
  input = pb_istream_from_buffer(oversized, oversized_len);
  decoded = (gizclaw_rpc_v1_ClientWifiSavedForgetRequest)
      gizclaw_rpc_v1_ClientWifiSavedForgetRequest_init_zero;
  if (expect(!pb_decode(&input, gizclaw_rpc_v1_ClientWifiSavedForgetRequest_fields, &decoded),
             "33-byte ssid is rejected by the nanopb bound") != 0) {
    return 1;
  }

  gizclaw_rpc_v1_ClientDeviceSoundPlayRequest sound =
      gizclaw_rpc_v1_ClientDeviceSoundPlayRequest_init_zero;
  memset(sound.sound, 'c', 32);
  sound.sound[32] = '\0';
  sound.has_duration_ms = true;
  sound.duration_ms = 1500;
  output = pb_ostream_from_buffer(buffer, sizeof(buffer));
  if (expect(pb_encode(&output, gizclaw_rpc_v1_ClientDeviceSoundPlayRequest_fields, &sound),
             "32-byte sound encodes within the nanopb bound") != 0) {
    return 1;
  }
  gizclaw_rpc_v1_ClientDeviceSoundPlayRequest decoded_sound =
      gizclaw_rpc_v1_ClientDeviceSoundPlayRequest_init_zero;
  input = pb_istream_from_buffer(buffer, output.bytes_written);
  if (expect(pb_decode(&input, gizclaw_rpc_v1_ClientDeviceSoundPlayRequest_fields, &decoded_sound) &&
                 strcmp(decoded_sound.sound, sound.sound) == 0 &&
                 decoded_sound.has_duration_ms && decoded_sound.duration_ms == 1500,
             "sound request round trips") != 0) {
    return 1;
  }

  gizclaw_rpc_v1_ClientWifiSavedListResponse saved =
      gizclaw_rpc_v1_ClientWifiSavedListResponse_init_zero;
  saved.networks_count = 2;
  strcpy(saved.networks[0].ssid, "home");
  strcpy(saved.networks[1].ssid, "office");
  uint8_t saved_buffer[256];
  output = pb_ostream_from_buffer(saved_buffer, sizeof(saved_buffer));
  if (expect(pb_encode(&output, gizclaw_rpc_v1_ClientWifiSavedListResponse_fields, &saved),
             "saved network list encodes") != 0) {
    return 1;
  }
  gizclaw_rpc_v1_ClientWifiSavedListResponse decoded_saved =
      gizclaw_rpc_v1_ClientWifiSavedListResponse_init_zero;
  input = pb_istream_from_buffer(saved_buffer, output.bytes_written);
  if (expect(pb_decode(&input, gizclaw_rpc_v1_ClientWifiSavedListResponse_fields, &decoded_saved) &&
                 decoded_saved.networks_count == 2 &&
                 strcmp(decoded_saved.networks[1].ssid, "office") == 0,
             "saved network list round trips") != 0) {
    return 1;
  }

  gizclaw_rpc_v1_ClientDeviceVolumeSetResponse volume =
      gizclaw_rpc_v1_ClientDeviceVolumeSetResponse_init_zero;
  volume.has_value = true;
  volume.value.has_volume = true;
  volume.value.volume = 35;
  volume.value.has_muted = true;
  volume.value.muted = true;
  output = pb_ostream_from_buffer(saved_buffer, sizeof(saved_buffer));
  if (expect(pb_encode(&output, gizclaw_rpc_v1_ClientDeviceVolumeSetResponse_fields, &volume),
             "volume response with PeerStatus encodes") != 0) {
    return 1;
  }
  gizclaw_rpc_v1_ClientDeviceVolumeSetResponse decoded_volume =
      gizclaw_rpc_v1_ClientDeviceVolumeSetResponse_init_zero;
  input = pb_istream_from_buffer(saved_buffer, output.bytes_written);
  if (expect(pb_decode(&input, gizclaw_rpc_v1_ClientDeviceVolumeSetResponse_fields, &decoded_volume) &&
                 decoded_volume.has_value && decoded_volume.value.has_volume &&
                 decoded_volume.value.volume == 35 && decoded_volume.value.muted,
             "volume response round trips") != 0) {
    return 1;
  }
  return 0;
}

int main(void) {
  if (expect(test_peer_event_golden_vectors() == 0,
             "all Peer Event golden vectors match Nanopb") != 0 ||
      test_json_ascii_classification() != 0) {
    return 1;
  }
  fake_clock_t clock = {
      .instant_ms = 1000,
      .unix_ms = INT64_C(1700000000000),
      .instant_step_ms = 0,
  };
  gzc_platform_t test_platform = *gzc_default_platform();
  test_platform.userdata = &clock;
  test_platform.malloc = test_malloc;
  test_platform.realloc = test_realloc;
  test_platform.time_instant_ms = test_time_instant_ms;
  test_platform.time_unix_ms = test_time_unix_ms;
  const gzc_platform_t *platform = &test_platform;
  fake_webrtc_t fake_webrtc;
  memset(&fake_webrtc, 0, sizeof(fake_webrtc));
  fake_webrtc.platform = platform;
  fake_webrtc.clock = &clock;
  fake_webrtc.drain_on_poll = true;
  fake_webrtc.emit_low_event = true;
  gzc_buf_init(&fake_webrtc.sent);
  gzc_buf_init(&fake_webrtc.outgoing);
  gzc_buf_init(&fake_webrtc.native_sent);
  gzc_buf_init(&fake_webrtc.opus_sent);

  fake_http_t fake_http;
  memset(&fake_http, 0, sizeof(fake_http));
  fake_http.platform = platform;

  fake_crypto_t fake_crypto;
  memset(&fake_crypto, 0, sizeof(fake_crypto));
  fake_crypto.platform = platform;

  gzc_key_t roundtrip_key;
  int rc = gzc_key_from_text(gzc_str_from_cstr(" 7gyGAp71YXQRoxmFBaHxofQXAipvgHyBKPyxmdSJxyvz\n"), &roundtrip_key);
  if (expect(rc == GZC_OK, "key from text") != 0) {
    return 1;
  }
  char roundtrip_text[GZC_KEY_TEXT_CAP];
  size_t roundtrip_text_len = 0;
  rc = gzc_key_to_text(&roundtrip_key, roundtrip_text, sizeof(roundtrip_text), &roundtrip_text_len);
  if (expect(rc == GZC_OK && roundtrip_text_len == strlen("7gyGAp71YXQRoxmFBaHxofQXAipvgHyBKPyxmdSJxyvz") &&
                 strcmp(roundtrip_text, "7gyGAp71YXQRoxmFBaHxofQXAipvgHyBKPyxmdSJxyvz") == 0,
             "key to text") != 0) {
    return 1;
  }

  gzc_platform_crypto_t crypto;
  memset(&crypto, 0, sizeof(crypto));
  crypto.userdata = &fake_crypto;
  crypto.keypair_from_private = test_keypair_from_private;
  crypto.dh = test_dh;
  crypto.hkdf_sha256 = test_hkdf_sha256;
  crypto.aead_seal = test_aead_copy;
  crypto.aead_open = test_aead_copy;

  gzc_webrtc_vtable_t webrtc;
  memset(&webrtc, 0, sizeof(webrtc));
  webrtc.userdata = &fake_webrtc;
  webrtc.peer_create = test_peer_create;
  webrtc.peer_start_offer = test_peer_start_offer;
  webrtc.peer_set_remote_sdp = test_peer_set_remote_sdp;
  webrtc.peer_create_data_channel = test_peer_create_data_channel;
  webrtc.peer_poll = test_peer_poll;
  webrtc.channel_send = test_channel_send;
  webrtc.channel_buffered_amount = test_channel_buffered_amount;
  webrtc.channel_set_buffered_amount_low_threshold =
      test_channel_set_buffered_amount_low_threshold;
  webrtc.channel_close = fake_channel_close;
  webrtc.peer_close = fake_peer_close;
  gzc_webrtc_media_vtable_t media;
  memset(&media, 0, sizeof(media));
  media.struct_size = sizeof(media);
  media.peer_set_opus_frame_callback = test_peer_set_opus_frame_callback;
  media.peer_send_opus = test_peer_send_opus;

  gzc_http_vtable_t http;
  memset(&http, 0, sizeof(http));
  http.userdata = &fake_http;
  http.request = test_http_request;
  http.response_free = test_http_response_free;

  gzc_client_config_t config;
  memset(&config, 0, sizeof(config));
  config.server_url = gzc_str_from_cstr("http://example.invalid:9820");
  config.private_key = gzc_str_from_cstr("7gyGAp71YXQRoxmFBaHxofQXAipvgHyBKPyxmdSJxyvz");
  config.platform = platform;
  config.crypto = &crypto;
  config.http = &http;
  config.webrtc = &webrtc;
  config.cipher_mode = GZC_CIPHER_PLAINTEXT;
  config.connect_timeout_ms = 1000;
  config.write_timeout_ms = 1000;
  fake_rpc_provider_t rpc_provider;
  memset(&rpc_provider, 0, sizeof(rpc_provider));
  config.rpc_provider = test_rpc_provider;
  config.rpc_provider_userdata = &rpc_provider;
  fake_tool_handler_t tool_handler;
  memset(&tool_handler, 0, sizeof(tool_handler));
  const gzc_tool_handler_t tool_handlers[] = {{
      .name = {.data = "volume_set", .len = 10u},
      .handler = test_tool_handler,
      .userdata = &tool_handler,
  }};
  config.tool_handlers = tool_handlers;
  config.tool_handler_count = 1u;

  gzc_client_t *client = NULL;
  gzc_client_config_t invalid_config = config;
  invalid_config.write_timeout_ms = 0;
  rc = gzc_client_create(&invalid_config, &client);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT && client == NULL,
             "write timeout is required") != 0) {
    return 1;
  }
  invalid_config = config;
  invalid_config.service_write_high_water_bytes = GZC_SERVICE_WRITE_CHUNK_SIZE - 1;
  invalid_config.service_write_low_water_bytes = 1;
  rc = gzc_client_create(&invalid_config, &client);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT && client == NULL,
             "high water covers one service chunk") != 0) {
    return 1;
  }
  gzc_webrtc_vtable_t incomplete_webrtc = webrtc;
  incomplete_webrtc.channel_buffered_amount = NULL;
  invalid_config = config;
  invalid_config.webrtc = &incomplete_webrtc;
  rc = gzc_client_create(&invalid_config, &client);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT && client == NULL,
             "flow-control callbacks are supplied as a pair") != 0) {
    return 1;
  }
  gzc_webrtc_vtable_t backpressure_webrtc = webrtc;
  backpressure_webrtc.channel_buffered_amount = NULL;
  backpressure_webrtc.channel_set_buffered_amount_low_threshold = NULL;
  gzc_client_t *backpressure_client = NULL;
  invalid_config = config;
  invalid_config.webrtc = &backpressure_webrtc;
  rc = gzc_client_create(&invalid_config, &backpressure_client);
  if (expect(rc == GZC_OK && backpressure_client != NULL,
             "send backpressure can replace buffered-amount callbacks") != 0) {
    return 1;
  }
  gzc_client_destroy(backpressure_client);
  gzc_platform_t platform_without_instant = *platform;
  platform_without_instant.time_instant_ms = NULL;
  invalid_config = config;
  invalid_config.platform = &platform_without_instant;
  rc = gzc_client_create(&invalid_config, &client);
  if (expect(rc == GZC_ERR_UNSUPPORTED && client == NULL,
             "monotonic instant clock is required") != 0) {
    return 1;
  }
  if (expect(GZC_ERR_CHANNEL_LIMIT == -12,
             "channel-limit status value") != 0 ||
      expect(strcmp(gzc_status_string(GZC_ERR_CHANNEL_LIMIT),
                    "data channel limit reached") == 0,
             "channel-limit status string") != 0 ||
      expect(GZC_ERR_BUFFER_TOO_SMALL == -13,
             "buffer-too-small status value") != 0 ||
      expect(strcmp(gzc_status_string(GZC_ERR_BUFFER_TOO_SMALL),
                    "buffer too small") == 0,
             "buffer-too-small status string") != 0) {
    return 1;
  }
  static const char *const rejected_server_urls[] = {
      "",
      "ftp://example.invalid:9820",
      "http:",
      "https:",
      "mailto:device@example.invalid",
      "http://",
      "http://example.invalid:9820?probe=1",
      "http://example.invalid:9820#fragment",
      "http://user@example.invalid:9820",
      "http://example.invalid:9820//double",
      "example.invalid:",
      "example.invalid:port",
      "example.invalid:998877",
      ":9820",
      "[::1:9820",
      "[]:9820",
  };
  for (size_t i = 0; i < sizeof(rejected_server_urls) / sizeof(rejected_server_urls[0]); i++) {
    invalid_config = config;
    invalid_config.server_url = gzc_str_from_cstr(rejected_server_urls[i]);
    rc = gzc_client_create(&invalid_config, &client);
    if (expect(rc == GZC_ERR_INVALID_ARGUMENT && client == NULL,
               "server URL must be an absolute http or https URL") != 0) {
      return 1;
    }
  }
  static const char *const accepted_server_urls[] = {
      "example.invalid:9820",
      "example.invalid",
      "[::1]:9820",
      "http://example.invalid:9820",
      "https://ap.gizclaw.com",
      "https://ap.gizclaw.com/",
      "https://ap.gizclaw.com/prefix",
  };
  for (size_t i = 0; i < sizeof(accepted_server_urls) / sizeof(accepted_server_urls[0]); i++) {
    gzc_client_t *url_client = NULL;
    invalid_config = config;
    invalid_config.server_url = gzc_str_from_cstr(accepted_server_urls[i]);
    rc = gzc_client_create(&invalid_config, &url_client);
    if (expect(rc == GZC_OK && url_client != NULL,
               "http, https and bare host:port server URLs are accepted") != 0) {
      return 1;
    }
    gzc_client_destroy(url_client);
  }
  invalid_config = config;
  invalid_config.tool_handler_count = 0u;
  rc = gzc_client_create(&invalid_config, &client);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT && client == NULL,
             "Tool handlers require a non-empty registration array") != 0) {
    return 1;
  }
  rc = gzc_client_create(&config, &client);
  if (expect(rc == GZC_OK, "client create") != 0) {
    return 1;
  }
  rc = gzc_client_set_opus_rx_capacity(client, 0u);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject an empty Opus receive ring") != 0) {
    return 1;
  }
  rc = gzc_client_set_opus_rx_capacity(client, 32u);
  if (expect(rc == GZC_OK,
             "configure a smaller Opus receive ring") != 0) {
    return 1;
  }
  rc = gzc_client_set_opus_rx_capacity(
      client, GZC_OPUS_RX_CAPACITY_DEFAULT);
  if (expect(rc == GZC_OK && GZC_OPUS_RX_CAPACITY_DEFAULT == 64u,
             "restore the 64-packet default Opus receive ring") != 0) {
    return 1;
  }
  fake_webrtc.client = client;
  rc = gzc_client_connect(client);
  if (expect(
          rc == GZC_ERR_UNSUPPORTED &&
              fake_webrtc.create_channel_count == 0,
          "connect rejects a backend without mandatory Opus media") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  gzc_webrtc_media_vtable_t incomplete_media = media;
  incomplete_media.peer_send_opus = NULL;
  rc = gzc_client_set_webrtc_media(client, &incomplete_media);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject incomplete media extension") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  gzc_webrtc_media_vtable_t truncated_media = media;
  truncated_media.struct_size =
      offsetof(gzc_webrtc_media_vtable_t, peer_send_opus);
  rc = gzc_client_set_webrtc_media(client, &truncated_media);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject truncated media extension") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  struct {
    gzc_webrtc_media_vtable_t v1;
    void *future;
  } extended_media;
  memset(&extended_media, 0, sizeof(extended_media));
  extended_media.v1 = media;
  extended_media.v1.struct_size = sizeof(extended_media);
  rc = gzc_client_set_webrtc_media(client, &extended_media.v1);
  if (expect(rc == GZC_OK, "accept larger media extension") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  rc = gzc_client_set_webrtc_media(client, &media);
  if (expect(rc == GZC_OK, "register Opus media extension") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  rc = gzc_client_set_webrtc_media(client, NULL);
  if (expect(rc == GZC_OK, "clear Opus media extension") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  rc = gzc_client_set_webrtc_media(client, &media);
  if (expect(rc == GZC_OK, "replace Opus media extension") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  rc = gzc_client_set_webrtc_media(client, &incomplete_media);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "rejected media replacement preserves registration") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  rc = gzc_client_set_peer_add_ice_server(client, test_peer_add_ice_server);
  if (expect(rc == GZC_OK, "client ICE hook") != 0) {
    gzc_client_destroy(client);
    return 1;
  }
  fail_next_malloc = true;
  rc = gzc_client_connect(client);
  if (expect(rc == GZC_ERR_NO_MEMORY && !fail_next_malloc,
             "connect reports fixed Opus ring allocation failure") != 0) {
    return 1;
  }
  fake_webrtc.opus_register_result = GZC_ERR_WEBRTC;
  rc = gzc_client_connect(client);
  fake_webrtc.opus_register_result = GZC_OK;
  if (expect(
          rc == GZC_ERR_WEBRTC &&
              fake_webrtc.opus_register_count == 1 &&
              fake_webrtc.opus_unregister_count == 1,
          "failed media registration unregisters partial callback") != 0) {
    return 1;
  }
  rc = gzc_client_connect(client);
  if (expect(rc == GZC_OK, "client connect") != 0) {
    return 1;
  }
  if (expect(fake_http.get_count == 1, "server-info get called once") != 0) {
    return 1;
  }
  if (expect(fake_http.post_count == 1, "http post called once") != 0) {
    return 1;
  }
  if (expect(
          fake_webrtc.create_channel_count == 2,
          "connect creates Packet and Event without an idle RPC channel") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.low_threshold == GZC_SERVICE_WRITE_LOW_WATER_DEFAULT,
             "default low-water threshold is installed") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.ice_server_count == 1, "server-info ICE server applied before offer") != 0) {
    return 1;
  }
  gzc_str_t ice_endpoint;
  memset(&ice_endpoint, 0, sizeof(ice_endpoint));
  if (expect(gzc_client_ice_endpoint(client, &ice_endpoint) == GZC_OK &&
                 str_eq_cstr(ice_endpoint, "ice.invalid:9820"),
             "server-info advertises the ICE UDP endpoint") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.opus_register_count == 2,
             "Opus callback registered before offer") != 0) {
    return 1;
  }

  gzc_service_channel_t *same_service_first = NULL;
  gzc_service_channel_t *same_service_second = NULL;
  gzc_service_channel_t *different_service = NULL;
  rc = gzc_client_open_service_channel(
      client, 49, 1000, &same_service_first);
  if (rc == GZC_OK) {
    rc = gzc_client_open_service_channel(
        client, 49, 1000, &same_service_second);
  }
  if (rc == GZC_OK) {
    rc = gzc_client_open_service_channel(
        client, 48, 1000, &different_service);
  }
  if (expect(
          rc == GZC_OK && same_service_first != NULL &&
              same_service_second != NULL && different_service != NULL,
          "same-ID and different-ID service channels coexist") != 0) {
    return 1;
  }
  gzc_service_channel_close(same_service_first);
  gzc_rpc_frame_t coexist_eos = {.type = GZC_RPC_FRAME_EOS};
  if (expect(
          gzc_service_channel_send_frame(
              same_service_second, &coexist_eos) == GZC_OK &&
              gzc_service_channel_send_frame(
                  different_service, &coexist_eos) == GZC_OK,
          "closing one service channel leaves its peers usable") != 0) {
    return 1;
  }
  gzc_service_channel_close(same_service_second);
  gzc_service_channel_close(different_service);

  const gzc_rtc_channel_state_t service_terminal_states[] = {
      GZC_RTC_CHANNEL_CLOSED,
      GZC_RTC_CHANNEL_ERROR,
  };
  for (size_t i = 0;
       i < sizeof(service_terminal_states) /
               sizeof(service_terminal_states[0]);
       i++) {
    gzc_service_channel_t *terminal_channel = NULL;
    rc = gzc_client_open_service_channel(
        client, 49, 1000, &terminal_channel);
    int close_count_before_terminal = fake_webrtc.close_count;
    gzc_rtc_channel_info_t info;
    memset(&info, 0, sizeof(info));
    info.label = gzc_str_from_cstr("giznet/v1/service/49");
    info.stream_id = 49;
    info.ordered = true;
    info.reliable = true;
    if (rc == GZC_OK) {
      fake_emit_channel_state(
          &fake_webrtc,
          &fake_webrtc.edge_channel,
          &info,
          service_terminal_states[i]);
      fake_emit_channel_state(
          &fake_webrtc,
          &fake_webrtc.edge_channel,
          &info,
          service_terminal_states[i]);
    }
    if (expect(
            rc == GZC_OK && terminal_channel != NULL &&
                gzc_service_channel_send_frame(
                    terminal_channel, &coexist_eos) == GZC_ERR_CLOSED,
            "terminal service callback invalidates the borrowed channel") != 0) {
      return 1;
    }
    gzc_service_channel_close(terminal_channel);
    if (expect(
            fake_webrtc.close_count == close_count_before_terminal &&
                fake_webrtc.stale_close_count == 0,
            "terminal service cleanup skips provider close after duplicate notification") != 0) {
      return 1;
    }
  }

  gzc_service_channel_t *explicit_close_channel = NULL;
  rc = gzc_client_open_service_channel(
      client, 49, 1000, &explicit_close_channel);
  int close_count_before_explicit = fake_webrtc.close_count;
  gzc_service_channel_close(explicit_close_channel);
  gzc_rtc_channel_info_t explicit_close_info;
  memset(&explicit_close_info, 0, sizeof(explicit_close_info));
  explicit_close_info.label = gzc_str_from_cstr("giznet/v1/service/49");
  explicit_close_info.stream_id = 49;
  explicit_close_info.ordered = true;
  explicit_close_info.reliable = true;
  fake_emit_channel_state(
      &fake_webrtc,
      &fake_webrtc.edge_channel,
      &explicit_close_info,
      GZC_RTC_CHANNEL_CLOSED);
  if (expect(
          rc == GZC_OK &&
              fake_webrtc.close_count == close_count_before_explicit + 1 &&
              fake_webrtc.stale_close_count == 0,
          "explicit service close remains exactly once before a late terminal callback") != 0) {
    return 1;
  }

  gzc_service_channel_t *capacity_channels[15] = {0};
  for (size_t i = 0; i < 15u && rc == GZC_OK; i++) {
    rc = gzc_client_open_service_channel(
        client, 48, 1000, &capacity_channels[i]);
  }
  gzc_service_channel_t *overflow_channel = NULL;
  if (expect(rc == GZC_OK,
             "15 caller-created channels coexist with Event") != 0 ||
      expect(gzc_client_open_service_channel(
                 client, 48, 1000, &overflow_channel) ==
                     GZC_ERR_CHANNEL_LIMIT &&
                 overflow_channel == NULL,
             "sixteenth caller channel reports channel limit") != 0 ||
      expect(gzc_service_channel_send_frame(
                 capacity_channels[14], &coexist_eos) == GZC_OK,
             "channel-limit failure leaves existing channels usable") != 0) {
    return 1;
  }
  gzc_service_channel_close(capacity_channels[0]);
  capacity_channels[0] = NULL;
  rc = gzc_client_open_service_channel(
      client, 48, 1000, &capacity_channels[0]);
  if (expect(rc == GZC_OK && capacity_channels[0] != NULL,
             "released channel slot can be reused") != 0) {
    return 1;
  }
  for (size_t i = 0; i < 15u; i++) {
    gzc_service_channel_close(capacity_channels[i]);
  }

  gzc_service_channel_t *bounded_channel = NULL;
  rc = gzc_client_open_service_channel(client, 49, 1000, &bounded_channel);
  if (expect(rc == GZC_OK && bounded_channel != NULL,
             "open bounded service channel") != 0) {
    return 1;
  }
  int (*buffered_amount_fn)(gzc_rtc_channel_t *, uint64_t *) =
      webrtc.channel_buffered_amount;
  int (*set_low_threshold_fn)(gzc_rtc_channel_t *, uint64_t) =
      webrtc.channel_set_buffered_amount_low_threshold;
  webrtc.channel_buffered_amount = NULL;
  webrtc.channel_set_buffered_amount_low_threshold = NULL;
  fake_webrtc.would_block_send_call = fake_webrtc.send_calls + 1;
  gzc_buf_reset(&fake_webrtc.native_sent);
  int backpressure_poll_count = fake_webrtc.poll_count;
  rc = gzc_service_channel_send_frame(bounded_channel, &(gzc_rpc_frame_t){
                                                           .type = GZC_RPC_FRAME_EOS,
                                                       });
  fake_webrtc.would_block_send_call = 0;
  webrtc.channel_buffered_amount = buffered_amount_fn;
  webrtc.channel_set_buffered_amount_low_threshold = set_low_threshold_fn;
  if (expect(rc == GZC_OK && fake_webrtc.poll_count > backpressure_poll_count &&
                 fake_webrtc.native_sent.len == 4u,
             "would-block send retries without consuming the service bytes") != 0) {
    return 1;
  }
  uint8_t *large_payload =
      (uint8_t *)platform->malloc(platform->userdata, GZC_RPC_MAX_FRAME_SIZE);
  if (large_payload == NULL) {
    return 1;
  }
  memset(large_payload, 0xa5, GZC_RPC_MAX_FRAME_SIZE);
  gzc_rpc_frame_t large_frame;
  memset(&large_frame, 0, sizeof(large_frame));
  large_frame.type = GZC_RPC_FRAME_BINARY;
  large_frame.data = large_payload;
  large_frame.len = GZC_RPC_MAX_FRAME_SIZE;
  fake_webrtc.buffered_amount = GZC_SERVICE_WRITE_HIGH_WATER_DEFAULT;
  fake_webrtc.max_send_len = 0;
  gzc_buf_reset(&fake_webrtc.native_sent);
  int poll_count_before_write = fake_webrtc.poll_count;
  rc = gzc_service_channel_send_frame(bounded_channel, &large_frame);
  if (expect(rc == GZC_OK && fake_webrtc.poll_count > poll_count_before_write,
             "high water waits for low-water notification") != 0 ||
      expect(fake_webrtc.max_send_len == GZC_SERVICE_WRITE_CHUNK_SIZE,
             "service writes are chunked to 4 KiB") != 0 ||
      expect(fake_webrtc.native_sent.len == GZC_RPC_MAX_FRAME_SIZE + 4u &&
                 fake_webrtc.native_sent.data[0] == 0xffu &&
                 fake_webrtc.native_sent.data[1] == 0xffu &&
                 memcmp(fake_webrtc.native_sent.data + 4u,
                        large_payload,
                        GZC_RPC_MAX_FRAME_SIZE) == 0,
             "large borrowed frame preserves the service byte stream") != 0) {
    platform->free(platform->userdata, large_payload);
    return 1;
  }
  platform->free(platform->userdata, large_payload);

  gzc_buf_reset(&fake_webrtc.native_sent);
  fake_webrtc.buffered_amount = GZC_SERVICE_WRITE_HIGH_WATER_DEFAULT;
  fake_webrtc.emit_low_event = false;
  rc = gzc_service_channel_send_frame(bounded_channel, &(gzc_rpc_frame_t){
                                                           .type = GZC_RPC_FRAME_EOS,
                                                       });
  fake_webrtc.emit_low_event = true;
  if (expect(rc == GZC_OK && fake_webrtc.native_sent.len == 4u,
             "writer rechecks after a drain that races the low-water event") != 0) {
    return 1;
  }

  uint8_t partial_payload[GZC_SERVICE_WRITE_CHUNK_SIZE + 1u];
  memset(partial_payload, 0x5a, sizeof(partial_payload));
  gzc_rpc_frame_t partial_frame;
  memset(&partial_frame, 0, sizeof(partial_frame));
  partial_frame.type = GZC_RPC_FRAME_BINARY;
  partial_frame.data = partial_payload;
  partial_frame.len = sizeof(partial_payload);
  fake_webrtc.buffered_amount = 0;
  fake_webrtc.fail_send_call = fake_webrtc.send_calls + 2;
  int close_count_before_failure = fake_webrtc.close_count;
  rc = gzc_service_channel_send_frame(bounded_channel, &partial_frame);
  fake_webrtc.fail_send_call = 0;
  if (expect(rc == GZC_ERR_WEBRTC &&
                 fake_webrtc.close_count == close_count_before_failure + 1 &&
                 fake_webrtc.last_closed == &fake_webrtc.edge_channel,
             "partial service write failure closes the channel") != 0 ||
      expect(gzc_service_channel_send_frame(bounded_channel, &partial_frame) == GZC_ERR_CLOSED,
             "partial service write failure is locally terminal") != 0) {
    return 1;
  }
  gzc_buf_reset(&fake_webrtc.outgoing);
  gzc_buf_reset(&fake_webrtc.native_sent);
  fake_webrtc.buffered_amount = 0;
  gzc_service_channel_close(bounded_channel);
  if (expect(fake_webrtc.close_count == close_count_before_failure + 1,
             "releasing a failed service channel does not close it twice") != 0) {
    return 1;
  }

  gzc_service_channel_t *timeout_channel = NULL;
  rc = gzc_client_open_service_channel(client, 49, 1000, &timeout_channel);
  if (expect(rc == GZC_OK && timeout_channel != NULL,
             "open service channel for write timeout") != 0) {
    return 1;
  }
  fake_webrtc.drain_on_poll = false;
  fake_webrtc.buffered_amount = GZC_SERVICE_WRITE_HIGH_WATER_DEFAULT;
  gzc_buf_reset(&fake_webrtc.native_sent);
  int64_t timeout_start = clock.instant_ms;
  int64_t unix_start = clock.unix_ms;
  close_count_before_failure = fake_webrtc.close_count;
  rc = gzc_service_channel_send_frame(timeout_channel, &partial_frame);
  if (expect(rc == GZC_ERR_TIMEOUT && fake_webrtc.native_sent.len == 0 &&
                 fake_webrtc.close_count == close_count_before_failure + 1,
             "write timeout before the first chunk closes the channel") != 0 ||
      expect(clock.instant_ms - timeout_start >= config.write_timeout_ms &&
                 clock.unix_ms < unix_start,
             "write timeout uses instant time while Unix time moves backward") != 0) {
    return 1;
  }
  gzc_service_channel_close(timeout_channel);

  timeout_channel = NULL;
  rc = gzc_client_open_service_channel(client, 49, 1000, &timeout_channel);
  if (expect(rc == GZC_OK && timeout_channel != NULL,
             "reopen service channel for partial write timeout") != 0) {
    return 1;
  }
  fake_webrtc.buffered_amount = GZC_SERVICE_WRITE_HIGH_WATER_DEFAULT - 1u;
  gzc_buf_reset(&fake_webrtc.native_sent);
  gzc_buf_reset(&fake_webrtc.outgoing);
  close_count_before_failure = fake_webrtc.close_count;
  rc = gzc_service_channel_send_frame(timeout_channel, &partial_frame);
  if (expect(rc == GZC_ERR_TIMEOUT && fake_webrtc.native_sent.len == 4u &&
                 fake_webrtc.close_count == close_count_before_failure + 1,
             "write timeout after a partial frame is terminal") != 0 ||
      expect(gzc_service_channel_send_frame(timeout_channel, &partial_frame) == GZC_ERR_CLOSED,
             "partial timeout never retries later chunks") != 0) {
    return 1;
  }
  gzc_buf_reset(&fake_webrtc.outgoing);
  gzc_service_channel_close(timeout_channel);
  fake_webrtc.drain_on_poll = true;
  fake_webrtc.buffered_amount = 0;

  timeout_channel = NULL;
  rc = gzc_client_open_service_channel(client, 49, 1000, &timeout_channel);
  if (expect(rc == GZC_OK && timeout_channel != NULL,
             "open service channel for read timeout") != 0) {
    return 1;
  }
  gzc_buf_t timeout_read;
  gzc_buf_init(&timeout_read);
  timeout_start = clock.instant_ms;
  unix_start = clock.unix_ms;
  rc = gzc_service_channel_read_frame(timeout_channel, 25, &timeout_read);
  if (expect(rc == GZC_ERR_TIMEOUT && clock.instant_ms - timeout_start >= 25 &&
                 clock.unix_ms < unix_start,
             "read timeout uses instant time while Unix time moves backward") != 0) {
    return 1;
  }
  gzc_buf_free(&timeout_read, platform);
  gzc_service_channel_close(timeout_channel);

  gzc_event_stream_t *event_stream = NULL;
  const int event_channel_create_count = fake_webrtc.create_channel_count;
  const int event_channel_close_count = fake_webrtc.close_count;
  rc = gzc_event_stream_open(client, 1000, &event_stream);
  if (expect(rc == GZC_OK && event_stream != NULL,
             "open Peer Event Stream") != 0) {
    return 1;
  }
  gzc_event_stream_t *duplicate_event_stream = NULL;
  rc = gzc_event_stream_open(client, 1000, &duplicate_event_stream);
  if (expect(
          rc == GZC_ERR_INVALID_ARGUMENT &&
              duplicate_event_stream == NULL &&
              fake_webrtc.create_channel_count ==
                  event_channel_create_count,
          "second Event access handle is rejected without a new channel") !=
      0) {
    return 1;
  }
  gzc_peer_event_t outbound_event =
      gizclaw_events_v1_PeerEvent_init_zero;
  outbound_event.version = GZC_PEER_EVENT_VERSION;
  outbound_event.type =
      gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_BOS;
  outbound_event.which_payload = gizclaw_events_v1_PeerEvent_bos_tag;
  outbound_event.payload.bos.kind =
      gizclaw_events_v1_StreamKind_STREAM_KIND_AUDIO;
  (void)snprintf(
      outbound_event.payload.bos.stream_id,
      sizeof(outbound_event.payload.bos.stream_id),
      "%s",
      "audio-c");
  gzc_buf_reset(&fake_webrtc.sent);
  rc = gzc_event_stream_send(event_stream, &outbound_event);
  if (expect(rc == GZC_OK, "encode and send Nanopb Peer Event") != 0) {
    return 1;
  }
  gzc_peer_event_t invalid_domain_event =
      gizclaw_events_v1_PeerEvent_init_zero;
  invalid_domain_event.version = GZC_PEER_EVENT_VERSION;
  invalid_domain_event.type =
      gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED;
  invalid_domain_event.which_payload =
      gizclaw_events_v1_PeerEvent_workspace_history_updated_tag;
  (void)snprintf(
      invalid_domain_event.payload.workspace_history_updated.workspace_name,
      sizeof(invalid_domain_event.payload.workspace_history_updated.workspace_name),
      "%s",
      " ");
  if (expect(
          gzc_event_stream_send(event_stream, &invalid_domain_event) ==
              GZC_ERR_RPC,
          "reject Peer Event with missing resource identifier") != 0) {
    return 1;
  }
  gzc_rpc_frame_t event_frame;
  rc = gzc_rpc_frame_decode(
      fake_webrtc.sent.data, fake_webrtc.sent.len, &event_frame);
  if (expect(
          rc == GZC_OK && event_frame.type == GZC_RPC_FRAME_BINARY,
          "Peer Event uses binary frame") != 0) {
    return 1;
  }
  gzc_peer_event_t sent_event =
      gizclaw_events_v1_PeerEvent_init_zero;
  pb_istream_t sent_event_input =
      pb_istream_from_buffer(event_frame.data, event_frame.len);
  if (expect(
          pb_decode(
              &sent_event_input,
              gizclaw_events_v1_PeerEvent_fields,
              &sent_event) &&
              sent_event.type ==
                  gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_BOS &&
              sent_event.which_payload ==
                  gizclaw_events_v1_PeerEvent_bos_tag,
          "sent Peer Event decodes with Nanopb") != 0) {
    return 1;
  }

  (void)snprintf(
      outbound_event.payload.bos.stream_id,
      sizeof(outbound_event.payload.bos.stream_id),
      "%s",
      "\t\n\v\f\r ");
  if (expect(
          gzc_event_stream_send(event_stream, &outbound_event) == GZC_ERR_RPC,
          "reject Event identifier containing only ASCII whitespace") != 0) {
    return 1;
  }
  outbound_event.payload.bos.stream_id[0] = (char)0x80;
  outbound_event.payload.bos.stream_id[1] = '\0';
  if (expect(
          gzc_event_stream_send(event_stream, &outbound_event) == GZC_OK,
          "treat a high Event identifier byte as non-whitespace") != 0) {
    return 1;
  }
  (void)snprintf(
      outbound_event.payload.bos.stream_id,
      sizeof(outbound_event.payload.bos.stream_id),
      "%s",
      "audio-c");
  gzc_event_stream_close(event_stream);
  if (expect(
          fake_webrtc.close_count == event_channel_close_count &&
              fake_webrtc.create_channel_count ==
                  event_channel_create_count,
          "Event handle close leaves the physical channel open") != 0) {
    return 1;
  }

  event_stream = NULL;
  rc = gzc_event_stream_open(client, 1000, &event_stream);
  if (expect(
          rc == GZC_OK && event_stream != NULL &&
              fake_webrtc.create_channel_count ==
                  event_channel_create_count,
          "reopen Peer Event access reuses the physical channel") != 0) {
    return 1;
  }
  gzc_buf_t event_payload;
  gzc_buf_t encoded_event_frame;
  gzc_buf_init(&event_payload);
  gzc_buf_init(&encoded_event_frame);
  rc = encode_test_pb_message(
      platform,
      gizclaw_events_v1_PeerEvent_fields,
      &outbound_event,
      &event_payload);
  if (rc == GZC_OK) {
    rc = append_test_frame(
        platform,
        &encoded_event_frame,
        GZC_RPC_FRAME_BINARY,
        event_payload.data,
        event_payload.len);
  }
  if (expect(rc == GZC_OK, "frame inbound Peer Event") != 0) {
    return 1;
  }
  gzc_rtc_channel_info_t event_info;
  memset(&event_info, 0, sizeof(event_info));
  event_info.label = gzc_str_from_cstr("giznet/v1/service/32");
  event_info.stream_id = 32;
  event_info.ordered = true;
  event_info.reliable = true;
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata,
      &fake_webrtc.peer,
      &fake_webrtc.service_channels[15],
      &event_info,
      encoded_event_frame.data,
      encoded_event_frame.len,
      false);
  gzc_peer_event_t inbound_event =
      gizclaw_events_v1_PeerEvent_init_zero;
  rc = gzc_event_stream_read(event_stream, 1000, &inbound_event);
  if (expect(
          rc == GZC_OK &&
              inbound_event.which_payload ==
                  gizclaw_events_v1_PeerEvent_bos_tag &&
              strcmp(inbound_event.payload.bos.stream_id, "audio-c") == 0,
          "read framed Nanopb Peer Event") != 0) {
    return 1;
  }
  gzc_buf_reset(&encoded_event_frame);
  rc = append_test_frame(
      platform,
      &encoded_event_frame,
      GZC_RPC_FRAME_JSON,
      NULL,
      0);
  if (rc == GZC_OK) {
    fake_webrtc.callbacks.on_channel_message(
        fake_webrtc.callbacks.userdata,
        &fake_webrtc.peer,
        &fake_webrtc.service_channels[15],
        &event_info,
        encoded_event_frame.data,
        encoded_event_frame.len,
        false);
    inbound_event.version = 99;
    rc = gzc_event_stream_read(event_stream, 1000, &inbound_event);
  }
  if (expect(
          rc == GZC_ERR_RPC && inbound_event.version == 0,
          "reject JSON Peer Event and reset output") != 0) {
    return 1;
  }
  gzc_buf_free(&event_payload, platform);
  gzc_buf_free(&encoded_event_frame, platform);
  gzc_event_stream_close(event_stream);

  gzc_json_t malformed_json = {gzc_str_from_cstr("{\"public_key\":\"x\",}")};
  if (expect(gzc_json_validate_object(malformed_json.raw) == GZC_ERR_JSON, "malformed object rejected") != 0) {
    return 1;
  }
  malformed_json.raw = gzc_str_from_cstr("{\"value\":-}");
  if (expect(gzc_json_validate_object(malformed_json.raw) == GZC_ERR_JSON, "malformed number rejected") != 0) {
    return 1;
  }

  gizclaw_rpc_v1_PingRequest ping;
  memset(&ping, 0, sizeof(ping));
  ping.client_send_time = 42;
  gzc_buf_t params;
  gzc_buf_init(&params);
  rc = encode_test_pb_message(platform, gizclaw_rpc_v1_PingRequest_fields, &ping, &params);
  if (expect(rc == GZC_OK, "encode ping request") != 0) {
    return 1;
  }
  gzc_rpc_response_t response;
  rc = test_rpc_call(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      &response);
  if (expect(rc == GZC_OK, "rpc call") != 0) {
    return 1;
  }
  if (expect(response.result_payload.len > 0, "rpc call captured result payload") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.sent.len > 0, "channel send captured payload") != 0) {
    return 1;
  }
  gzc_rpc_frame_t sent_frame;
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, first_frame_size(&fake_webrtc.sent), &sent_frame);
  if (expect(rc == GZC_OK && sent_frame.type == GZC_RPC_FRAME_BINARY, "request protobuf frame") != 0) {
    return 1;
  }
  unsigned method_id = 0;
  rc = read_test_proto_method_id(gzc_str_from_parts((const char *)sent_frame.data, sent_frame.len), &method_id);
  if (expect(rc == GZC_OK, "request method id field") != 0) {
    return 1;
  }
  if (expect(method_id == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING, "request method id value") != 0) {
    return 1;
  }
  if (expect(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PET_PIXA_DOWNLOAD == 87, "pet pixa method id value") != 0) {
    return 1;
  }
  if (expect(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_BADGE_DEF_PIXA_DOWNLOAD == 64, "badge pixa method id value") != 0) {
    return 1;
  }
  if (expect(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_API_KEY_CREATE == 96, "API key create method id value") != 0) {
    return 1;
  }
  if (expect(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_API_KEY_LIST == 97, "API key list method id value") != 0) {
    return 1;
  }
  if (expect(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_API_KEY_REVOKE == 98, "API key revoke method id value") != 0) {
    return 1;
  }
  gizclaw_rpc_v1_APIKeyCreateResponse api_key_response =
      gizclaw_rpc_v1_APIKeyCreateResponse_init_zero;
  api_key_response.has_value = true;
  strcpy(api_key_response.value.name, "key_0123456789012345678901");
  strcpy(api_key_response.value.display_name, "phone");
  strcpy(api_key_response.value.prefix, "gizclaw_sk_v1_01234567...");
  strcpy(api_key_response.value.api_key, "gizclaw_sk_v1_0123456789012345678901234567890123456789012");
  api_key_response.value.manage_api_keys = true;
  strcpy(api_key_response.value.created_at, "2026-08-19T00:00:00Z");
  strcpy(api_key_response.api_key, "gizclaw_sk_v1_0123456789012345678901234567890123456789012");
  uint8_t api_key_payload[gizclaw_rpc_v1_APIKeyCreateResponse_size];
  pb_ostream_t api_key_output =
      pb_ostream_from_buffer(api_key_payload, sizeof(api_key_payload));
  if (expect(
          pb_encode(
              &api_key_output,
              gizclaw_rpc_v1_APIKeyCreateResponse_fields,
              &api_key_response),
          "API key response encode") != 0) {
    return 1;
  }
  gizclaw_rpc_v1_APIKeyCreateResponse decoded_api_key_response =
      gizclaw_rpc_v1_APIKeyCreateResponse_init_zero;
  pb_istream_t api_key_input =
      pb_istream_from_buffer(api_key_payload, api_key_output.bytes_written);
  if (expect(
          pb_decode(
              &api_key_input,
              gizclaw_rpc_v1_APIKeyCreateResponse_fields,
              &decoded_api_key_response) &&
              decoded_api_key_response.has_value &&
              decoded_api_key_response.value.manage_api_keys &&
              strcmp(decoded_api_key_response.value.name, api_key_response.value.name) == 0 &&
              strcmp(decoded_api_key_response.value.api_key, api_key_response.value.api_key) == 0 &&
              strcmp(decoded_api_key_response.api_key, api_key_response.api_key) == 0,
          "API key response round trip") != 0) {
    return 1;
  }

  gzc_buf_reset(&fake_webrtc.sent);
  int create_channel_count_before_edge = fake_webrtc.create_channel_count;
  memset(&response, 0, sizeof(response));
  rc = test_rpc_call(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PEER_LOOKUP,
      gzc_str_from_parts((const char *)params.data, params.len),
      &response);
  if (expect(rc == GZC_OK, "edge rpc call") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.create_channel_count == create_channel_count_before_edge + 1, "edge rpc opens service 49 channel") != 0) {
    return 1;
  }
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, first_frame_size(&fake_webrtc.sent), &sent_frame);
  if (expect(rc == GZC_OK && sent_frame.type == GZC_RPC_FRAME_BINARY, "edge request protobuf frame") != 0) {
    return 1;
  }
  method_id = 0;
  rc = read_test_proto_method_id(gzc_str_from_parts((const char *)sent_frame.data, sent_frame.len), &method_id);
  if (expect(rc == GZC_OK, "edge request method id field") != 0) {
    return 1;
  }
  if (expect(method_id == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PEER_LOOKUP, "edge request method id value") != 0) {
    return 1;
  }

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO_CONTINUATION;
  memset(&response, 0, sizeof(response));
  rc = test_rpc_call(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      &response);
  if (expect(rc == GZC_OK, "rpc call continuation response") != 0) {
    return 1;
  }
  if (expect(response.result_payload.len > 0, "rpc call continuation captured result payload") != 0) {
    return 1;
  }

  gizclaw_rpc_v1_PingResponse decoded;
  memset(&decoded, 0, sizeof(decoded));
  rc = decode_test_pb_message(response.result_payload, gizclaw_rpc_v1_PingResponse_fields, &decoded);
  if (expect(rc == GZC_OK && decoded.server_time == 99, "decode ping response") != 0) {
    return 1;
  }

  gizclaw_rpc_v1_SpeechTranscribeRequest transcribe_request =
      gizclaw_rpc_v1_SpeechTranscribeRequest_init_zero;
  strcpy(transcribe_request.model_name, "asr-main");
  strcpy(transcribe_request.content_type,
         "audio/L16;rate=16000;channels=1");
  gzc_buf_t speech_params;
  gzc_buf_init(&speech_params);
  rc = encode_test_pb_message(
      platform, gizclaw_rpc_v1_SpeechTranscribeRequest_fields,
      &transcribe_request, &speech_params);
  fake_webrtc.response_mode = FAKE_RESPONSE_SPEECH_TRANSCRIBE;
  stream_count_t speech_stream_count;
  memset(&speech_stream_count, 0, sizeof(speech_stream_count));
  gzc_rpc_request_t *speech_request = NULL;
  if (rc == GZC_OK) {
    rc = gzc_rpc_request_start_stream(
        client, 0u,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_TRANSCRIBE,
        gzc_str_from_parts((const char *)speech_params.data,
                           speech_params.len),
        5000, count_stream_frame, &speech_stream_count, &speech_request);
  }
  const uint8_t speech_input[] = {0x01, 0x02, 0x03, 0x04};
  while (rc == GZC_OK &&
         (rc = gzc_rpc_request_write(speech_request, speech_input,
                                     sizeof(speech_input))) ==
             GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(client, 0);
  }
  while (rc == GZC_OK &&
         (rc = gzc_rpc_request_finish_write(speech_request)) ==
             GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(client, 0);
  }
  gzc_rpc_response_t speech_response;
  while (rc == GZC_OK &&
         (rc = gzc_rpc_request_result(speech_request, &speech_response)) ==
             GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(client, 0);
  }
  gizclaw_rpc_v1_SpeechTranscribeResponse transcription =
      gizclaw_rpc_v1_SpeechTranscribeResponse_init_zero;
  if (rc == GZC_OK) {
    rc = decode_test_pb_message(
        speech_response.result_payload,
        gizclaw_rpc_v1_SpeechTranscribeResponse_fields, &transcription);
  }
  if (expect(
          rc == GZC_OK && strcmp(transcription.transcript, "hello") == 0 &&
              speech_stream_count.envelope_count == 1 &&
              speech_stream_count.eos_count == 1,
          "mixed request writes data and receives response through poll") !=
      0) {
    gzc_rpc_request_destroy(speech_request);
    gzc_buf_free(&speech_params, platform);
    return 1;
  }
  if (expect(
          gzc_rpc_request_write(speech_request, speech_input,
                                sizeof(speech_input)) == GZC_ERR_CLOSED &&
              gzc_rpc_request_finish_write(speech_request) == GZC_OK,
          "mixed request rejects data after request EOS") != 0) {
    gzc_rpc_request_destroy(speech_request);
    gzc_buf_free(&speech_params, platform);
    return 1;
  }
  gzc_rpc_request_destroy(speech_request);
  gzc_buf_free(&speech_params, platform);

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO_OVERSIZED_CONTINUATION;
  memset(&response, 0, sizeof(response));
  rc = test_rpc_call(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      &response);
  if (expect(rc == GZC_ERR_RPC, "rpc call rejects oversized continuation response") != 0) {
    return 1;
  }
  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO;

  fake_webrtc.defer_next_service_open = true;
  const int poll_count_before_async_start = fake_webrtc.poll_count;
  const int close_count_before_async_start = fake_webrtc.close_count;
  gzc_rpc_request_t *unopened_request = NULL;
  rc = gzc_rpc_request_start(
      client, 48u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000, &unopened_request);
  if (expect(
          rc == GZC_OK && unopened_request != NULL &&
              fake_webrtc.poll_count == poll_count_before_async_start,
          "request start does not poll while its DataChannel opens") != 0 ||
      expect(
          gzc_rpc_request_result(unopened_request, &response) ==
              GZC_ERR_WOULD_BLOCK,
          "unopened request remains pending") != 0) {
    gzc_rpc_request_destroy(unopened_request);
    return 1;
  }
  gzc_rpc_request_destroy(unopened_request);
  if (expect(
          fake_webrtc.close_count == close_count_before_async_start + 1,
          "destroy closes an unopened request channel") != 0) {
    return 1;
  }

  fake_webrtc.response_mode = FAKE_RESPONSE_DEFERRED_PROTO;
  gzc_rpc_request_t *async_requests[2] = {NULL, NULL};
  gzc_rtc_channel_t *async_channels[2] = {NULL, NULL};
  for (size_t i = 0; i < 2u; i++) {
    rc = gzc_rpc_request_start(
        client,
        0u,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
        gzc_str_from_parts((const char *)params.data, params.len),
        1000,
        &async_requests[i]);
    async_channels[i] = fake_webrtc.last_send_channel;
    if (expect(
            rc == GZC_OK && async_requests[i] != NULL &&
                async_channels[i] != NULL,
            "public unary request owns its service channel") != 0) {
      return 1;
    }
  }
  gzc_rpc_response_t async_response_first;
  gzc_rpc_response_t async_response_second;
  if (expect(
          gzc_rpc_request_result(
              async_requests[0], &async_response_first) ==
                  GZC_ERR_WOULD_BLOCK &&
              gzc_rpc_request_result(
                  async_requests[1], &async_response_second) ==
                  GZC_ERR_WOULD_BLOCK,
          "concurrent unary requests remain pending without caller polling") !=
      0) {
    return 1;
  }
  rc = send_deferred_rpc_response(
      &fake_webrtc, async_channels[1], 202, false);
  if (rc == GZC_OK) {
    rc = gzc_client_poll(client, 0);
  }
  if (rc == GZC_OK) {
    rc = gzc_rpc_request_result(
        async_requests[1], &async_response_second);
  }
  gzc_str_t second_result_view = async_response_second.result_payload;
  rc = rc == GZC_OK
           ? send_deferred_rpc_response(
                 &fake_webrtc, async_channels[0], 101, false)
           : rc;
  if (rc == GZC_OK) {
    rc = gzc_client_poll(client, 0);
  }
  if (rc == GZC_OK) {
    rc = gzc_rpc_request_result(
        async_requests[0], &async_response_first);
  }
  gizclaw_rpc_v1_PingResponse async_first_ping =
      gizclaw_rpc_v1_PingResponse_init_zero;
  gizclaw_rpc_v1_PingResponse async_second_ping =
      gizclaw_rpc_v1_PingResponse_init_zero;
  if (rc == GZC_OK) {
    rc = decode_test_pb_message(
        async_response_first.result_payload,
        gizclaw_rpc_v1_PingResponse_fields,
        &async_first_ping);
  }
  if (rc == GZC_OK) {
    rc = decode_test_pb_message(
        second_result_view,
        gizclaw_rpc_v1_PingResponse_fields,
        &async_second_ping);
  }
  if (expect(
          rc == GZC_OK && async_first_ping.server_time == 101 &&
              async_second_ping.server_time == 202,
          "request-owned response views survive sibling completion") != 0) {
    return 1;
  }
  gzc_rpc_request_destroy(async_requests[0]);
  gzc_rpc_request_destroy(async_requests[1]);

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO_ERROR;
  gzc_rpc_request_t *remote_error_request = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000,
      &remote_error_request);
  memset(&async_response_first, 0, sizeof(async_response_first));
  if (rc == GZC_OK) {
    rc = gzc_rpc_request_result(
        remote_error_request, &async_response_first);
  }
  if (expect(
          rc == GZC_OK && async_response_first.has_error &&
              async_response_first.error.code == 7,
          "remote unary RPC errors remain successful transport results") != 0) {
    return 1;
  }
  gzc_rpc_request_destroy(remote_error_request);
  fake_webrtc.response_mode = FAKE_RESPONSE_DEFERRED_PROTO;

  gzc_rpc_request_t *failed_request = NULL;
  gzc_rpc_request_t *healthy_request = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000,
      &failed_request);
  gzc_rtc_channel_t *failed_channel = fake_webrtc.last_send_channel;
  if (rc == GZC_OK) {
    rc = gzc_rpc_request_start(
        client,
        0u,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
        gzc_str_from_parts((const char *)params.data, params.len),
        1000,
        &healthy_request);
  }
  gzc_rtc_channel_t *healthy_channel = fake_webrtc.last_send_channel;
  if (rc == GZC_OK) {
    rc = send_deferred_rpc_response(
        &fake_webrtc, failed_channel, 303, true);
  }
  if (rc == GZC_OK) {
    rc = gzc_client_poll(client, 0);
  }
  if (expect(
          rc == GZC_OK &&
              gzc_rpc_request_result(
                  failed_request, &async_response_first) ==
                  GZC_ERR_NO_MEMORY,
          "request-local allocation failure remains local") != 0) {
    return 1;
  }
  rc = send_deferred_rpc_response(
      &fake_webrtc, healthy_channel, 404, false);
  if (rc == GZC_OK) {
    rc = gzc_client_poll(client, 0);
  }
  if (rc == GZC_OK) {
    rc = gzc_rpc_request_result(
        healthy_request, &async_response_second);
  }
  if (expect(rc == GZC_OK, "sibling unary request survives local OOM") != 0) {
    return 1;
  }
  gzc_rpc_request_destroy(failed_request);
  gzc_rpc_request_destroy(healthy_request);

  gzc_rpc_request_t *cancelled_request = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000,
      &cancelled_request);
  gzc_rpc_request_cancel(cancelled_request);
  gzc_rpc_request_cancel(cancelled_request);
  if (expect(
          rc == GZC_OK &&
              gzc_rpc_request_result(
                  cancelled_request, &async_response_first) ==
                  GZC_ERR_CLOSED,
          "unary request cancellation is idempotent") != 0) {
    return 1;
  }
  gzc_rpc_request_destroy(cancelled_request);
  gzc_rpc_request_destroy(NULL);

  gzc_rpc_request_t *remote_closed_request = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000,
      &remote_closed_request);
  gzc_rtc_channel_t *remote_closed_channel = fake_webrtc.last_send_channel;
  if (rc == GZC_OK) {
    fake_emit_channel_state(
        &fake_webrtc,
        remote_closed_channel,
        NULL,
        GZC_RTC_CHANNEL_CLOSED);
    rc = gzc_client_poll(client, 0);
  }
  if (expect(
          rc == GZC_OK &&
              gzc_rpc_request_result(
                  remote_closed_request, &async_response_first) ==
                  GZC_ERR_CLOSED,
          "remote close before response EOS terminates only its request") != 0) {
    return 1;
  }
  gzc_rpc_request_destroy(remote_closed_request);

  gzc_rpc_request_t *transport_failed_request = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000,
      &transport_failed_request);
  fake_webrtc.poll_result_once = GZC_ERR_WEBRTC;
  if (rc == GZC_OK) {
    rc = gzc_client_poll(client, 0);
  }
  if (expect(
          rc == GZC_ERR_WEBRTC &&
              gzc_rpc_request_result(
                  transport_failed_request, &async_response_first) ==
                  GZC_ERR_WEBRTC,
          "client poll transport error terminates pending unary requests") !=
      0) {
    return 1;
  }
  gzc_rpc_request_destroy(transport_failed_request);
  rc = GZC_OK;

  gzc_rpc_request_t *timeout_request = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      5,
      &timeout_request);
  clock.instant_ms += 10;
  if (expect(
          rc == GZC_OK &&
              gzc_rpc_request_result(
                  timeout_request, &async_response_first) ==
                  GZC_ERR_TIMEOUT,
          "unary request result enforces its total deadline without polling") !=
      0) {
    return 1;
  }
  gzc_rpc_request_destroy(timeout_request);

  gzc_rpc_request_t *poll_timeout_request = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      5,
      &poll_timeout_request);
  if (rc == GZC_OK) {
    rc = gzc_client_poll(client, -1);
  }
  if (expect(
          rc == GZC_OK && fake_webrtc.last_poll_timeout_ms == 5 &&
              gzc_rpc_request_result(
                  poll_timeout_request, &async_response_first) ==
                  GZC_ERR_TIMEOUT,
          "client poll clamps idle waits to the nearest request deadline") !=
      0) {
    return 1;
  }
  gzc_rpc_request_destroy(poll_timeout_request);

  gzc_rpc_request_t *capacity_requests[15] = {0};
  for (size_t i = 0; i < 15u && rc == GZC_OK; i++) {
    rc = gzc_rpc_request_start(
        client,
        0u,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
        gzc_str_from_parts((const char *)params.data, params.len),
        1000,
        &capacity_requests[i]);
  }
  gzc_rpc_request_t *overflow_request = NULL;
  if (expect(rc == GZC_OK, "15 concurrent unary request handles coexist") !=
          0 ||
      expect(
          gzc_rpc_request_start(
              client,
              0u,
              gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
              gzc_str_from_parts((const char *)params.data, params.len),
              1000,
              &overflow_request) == GZC_ERR_CHANNEL_LIMIT &&
              overflow_request == NULL,
          "unary request capacity reports channel limit") != 0) {
    return 1;
  }
  gzc_rpc_request_cancel(capacity_requests[0]);
  gzc_rpc_request_destroy(capacity_requests[0]);
  capacity_requests[0] = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000,
      &capacity_requests[0]);
  if (expect(rc == GZC_OK, "cancelled unary request releases its channel slot") !=
      0) {
    return 1;
  }
  clock.instant_ms += 1001;
  rc = gzc_client_poll(client, 0);
  if (expect(
          rc == GZC_OK &&
              gzc_rpc_request_result(
                  capacity_requests[0], &async_response_first) ==
                  GZC_ERR_TIMEOUT,
          "client polling expires unattended unary requests") != 0) {
    return 1;
  }
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000,
      &overflow_request);
  if (expect(
          rc == GZC_OK && overflow_request != NULL,
          "poll-expired unary requests release channel capacity before result inspection") !=
      0) {
    return 1;
  }
  for (size_t i = 0; i < 15u; i++) {
    gzc_rpc_request_destroy(capacity_requests[i]);
  }
  gzc_rpc_request_destroy(overflow_request);
  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO;

  gzc_buf_t list_payload;
  gzc_buf_init(&list_payload);
  rc = append_test_proto_varint(platform, &list_payload, 1, 0);
  if (rc == GZC_OK) {
    rc = append_test_proto_bytes(platform, &list_payload, 2, NULL, 0);
  }
  if (rc == GZC_OK) {
    rc = append_test_proto_bytes(platform, &list_payload, 2, NULL, 0);
  }
  if (expect(rc == GZC_OK, "build repeated list payload") != 0) {
    gzc_buf_free(&list_payload, platform);
    return 1;
  }
  size_t model_items = 0;
  gizclaw_rpc_v1_ModelListResponse model_list = gizclaw_rpc_v1_ModelListResponse_init_zero;
  model_list.items.funcs.decode = count_repeated_message;
  model_list.items.arg = &model_items;
  rc = decode_test_pb_message(
      gzc_str_from_parts((const char *)list_payload.data, list_payload.len),
      gizclaw_rpc_v1_ModelListResponse_fields,
      &model_list);
  if (expect(rc == GZC_OK, "decode repeated list payload") != 0) {
    gzc_buf_free(&list_payload, platform);
    return 1;
  }
  if (expect(model_items == 2, "repeated payload decodes all entries") != 0) {
    gzc_buf_free(&list_payload, platform);
    return 1;
  }
  gzc_buf_free(&list_payload, platform);

  fake_webrtc.response_mode = FAKE_RESPONSE_BINARY_STREAM;
  stream_count_t stream_count;
  memset(&stream_count, 0, sizeof(stream_count));
  rc = test_rpc_call_stream(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
      gzc_str_from_parts((const char *)params.data, params.len),
      count_stream_frame,
      &stream_count);
  if (expect(rc == GZC_OK, "rpc call stream") != 0) {
    return 1;
  }
  if (expect(stream_count.envelope_count == 1 &&
                 stream_count.frame_count == 2 &&
                 stream_count.binary_bytes == 3 &&
                 stream_count.eos_count == 1,
             "stream frames and terminal eos counted") != 0) {
    return 1;
  }

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO_OVERSIZED_CONTINUATION;
  memset(&stream_count, 0, sizeof(stream_count));
  rc = test_rpc_call_stream(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
      gzc_str_from_parts((const char *)params.data, params.len),
      count_stream_frame,
      &stream_count);
  if (expect(rc == GZC_ERR_RPC, "rpc stream rejects oversized continuation response") != 0) {
    return 1;
  }

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO_ERROR;
  stream_error_t stream_error;
  memset(&stream_error, 0, sizeof(stream_error));
  rc = test_rpc_call_stream(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
      gzc_str_from_parts((const char *)params.data, params.len),
      capture_stream_error_frame,
      &stream_error);
  if (expect(rc == GZC_OK, "rpc call stream returns ok with delivered error envelope") != 0) {
    return 1;
  }
  if (expect(
          stream_error.frame_count == 1 && stream_error.eos_count == 1 &&
              stream_error.has_error && stream_error.code == 7 &&
              stream_error.message_ok,
          "stream error envelope and terminal eos delivered to callback") != 0) {
    return 1;
  }
  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO;

  rc = test_rpc_call_stream(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
      gzc_str_from_parts((const char *)params.data, params.len),
      block_stream_frame,
      NULL);
  if (expect(
          rc == GZC_ERR_RPC,
          "stream callback cannot discard a frame with would-block") != 0) {
    return 1;
  }

  const uint8_t telemetry_payload[] = {0x01, 0x02, 0x03};
  const uint8_t opus_payload[] = {0xf8, 0x55};
  int send_calls_before_opus = fake_webrtc.send_calls;
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      opus_payload,
      sizeof(opus_payload));
  if (expect(
          rc == GZC_OK && fake_webrtc.opus_send_count == 1 &&
              fake_webrtc.send_calls == send_calls_before_opus &&
              fake_webrtc.opus_sent.len == sizeof(opus_payload) &&
              memcmp(fake_webrtc.opus_sent.data, opus_payload,
                     sizeof(opus_payload)) == 0,
          "Opus packet uses media RTP extension only") != 0) {
    return 1;
  }
  fake_webrtc.opus_send_result = GZC_ERR_WOULD_BLOCK;
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      opus_payload,
      sizeof(opus_payload));
  fake_webrtc.opus_send_result = GZC_OK;
  if (expect(rc == GZC_ERR_WOULD_BLOCK &&
                 fake_webrtc.opus_send_count == 1,
             "Opus send preserves backend backpressure") != 0) {
    return 1;
  }
  const uint8_t one_byte_opus[] = {0xf8};
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      one_byte_opus,
      sizeof(one_byte_opus));
  if (expect(rc == GZC_OK,
             "accept one-byte Opus packet") != 0) {
    return 1;
  }
  uint8_t max_opus[GZC_OPUS_MAX_PACKET_SIZE];
  memset(max_opus, 0, sizeof(max_opus));
  max_opus[0] = 0xf8;
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      max_opus,
      sizeof(max_opus));
  if (expect(rc == GZC_OK,
             "accept maximum-size Opus packet") != 0) {
    return 1;
  }
  uint8_t oversized_opus[GZC_OPUS_MAX_PACKET_SIZE + 1u];
  memset(oversized_opus, 0, sizeof(oversized_opus));
  oversized_opus[0] = 0xf8;
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      oversized_opus,
      sizeof(oversized_opus));
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject oversized Opus packet") != 0) {
    return 1;
  }
  rc = gzc_client_set_webrtc_media(client, &media);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject late media registration") != 0) {
    return 1;
  }
  rc = gzc_client_set_opus_rx_capacity(client, 32u);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject late Opus receive ring changes") != 0) {
    return 1;
  }
  rc = gzc_client_send_packet(
      client, GZC_PROTOCOL_OPUS_PACKET, NULL, 0u);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject empty Opus packet") != 0) {
    return 1;
  }
  const uint8_t invalid_count_opus[] = {0xfb, 0x00};
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      invalid_count_opus,
      sizeof(invalid_count_opus));
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject invalid Opus frame count") != 0) {
    return 1;
  }
  const uint8_t overlong_opus[] = {0x03, 0x0d};
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      overlong_opus,
      sizeof(overlong_opus));
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT,
             "reject Opus duration over 120 ms") != 0) {
    return 1;
  }
  memcpy(fake_webrtc.pending_opus, opus_payload, sizeof(opus_payload));
  fake_webrtc.pending_opus_len = sizeof(opus_payload);
  gzc_buf_t received_opus_payload;
  gzc_buf_init(&received_opus_payload);
  uint8_t received_opus_protocol = 0;
  rc = gzc_client_read_packet(
      client, 100, &received_opus_protocol, &received_opus_payload);
  if (expect(
          rc == GZC_OK &&
              received_opus_protocol == GZC_PROTOCOL_OPUS_PACKET &&
              received_opus_payload.len == sizeof(opus_payload) &&
              memcmp(received_opus_payload.data, opus_payload,
                     sizeof(opus_payload)) == 0,
          "poll-dispatched remote Opus returns through read_packet") != 0) {
    gzc_buf_free(&received_opus_payload, platform);
    return 1;
  }
  gzc_buf_free(&received_opus_payload, platform);

  fake_webrtc.opus_callback(
      fake_webrtc.opus_callback_userdata,
      &fake_webrtc.peer,
      opus_payload,
      sizeof(opus_payload));
  uint8_t fixed_opus_payload[GZC_OPUS_MAX_PACKET_SIZE];
  size_t fixed_opus_len = 0u;
  rc = gzc_client_read_packet_into(
      client,
      0,
      &received_opus_protocol,
      fixed_opus_payload,
      1u,
      &fixed_opus_len);
  if (expect(
          rc == GZC_ERR_BUFFER_TOO_SMALL &&
              received_opus_protocol == GZC_PROTOCOL_OPUS_PACKET &&
              fixed_opus_len == sizeof(opus_payload),
          "fixed read reports required capacity without consuming Opus") != 0) {
    return 1;
  }
  rc = gzc_client_read_packet_into(
      client,
      0,
      &received_opus_protocol,
      fixed_opus_payload,
      sizeof(fixed_opus_payload),
      &fixed_opus_len);
  if (expect(
          rc == GZC_OK &&
              received_opus_protocol == GZC_PROTOCOL_OPUS_PACKET &&
              fixed_opus_len == sizeof(opus_payload) &&
              memcmp(fixed_opus_payload, opus_payload, fixed_opus_len) == 0,
          "fixed read copies queued Opus without allocation") != 0) {
    return 1;
  }
  fake_webrtc.opus_callback(
      fake_webrtc.opus_callback_userdata,
      &fake_webrtc.peer,
      opus_payload,
      sizeof(opus_payload));
  rc = gzc_client_discard_opus_rx(client);
  if (expect(rc == GZC_OK,
             "explicit Opus discard clears the receive ring") != 0) {
    return 1;
  }
  rc = gzc_client_read_packet_into(
      client,
      0,
      &received_opus_protocol,
      fixed_opus_payload,
      sizeof(fixed_opus_payload),
      &fixed_opus_len);
  if (expect(rc == GZC_ERR_TIMEOUT,
             "discarded Opus is not delivered later") != 0) {
    return 1;
  }

  const uint8_t direct_for_fairness[] = {0x40, 0xd0};
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata,
      &fake_webrtc.peer,
      &fake_webrtc.packet_channel,
      NULL,
      direct_for_fairness,
      sizeof(direct_for_fairness),
      false);
  fake_webrtc.opus_callback(
      fake_webrtc.opus_callback_userdata,
      &fake_webrtc.peer,
      opus_payload,
      sizeof(opus_payload));
  gzc_buf_init(&received_opus_payload);
  rc = gzc_client_read_packet_into(
      client,
      0,
      &received_opus_protocol,
      fixed_opus_payload,
      sizeof(fixed_opus_payload),
      &fixed_opus_len);
  if (expect(
          rc == GZC_OK && received_opus_protocol == 0x40 &&
              fixed_opus_len == 1u && fixed_opus_payload[0] == 0xd0,
          "fixed read handles Direct Packet before simultaneous Opus") != 0) {
    return 1;
  }
  rc = gzc_client_read_packet(
      client, 0, &received_opus_protocol, &received_opus_payload);
  if (expect(
          rc == GZC_OK &&
              received_opus_protocol == GZC_PROTOCOL_OPUS_PACKET,
          "simultaneous packet and Opus strictly alternate") != 0) {
    return 1;
  }
  gzc_buf_free(&received_opus_payload, platform);

  const uint8_t direct_during_opus_overflow[] = {0x40, 0xe1};
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata,
      &fake_webrtc.peer,
      &fake_webrtc.packet_channel,
      NULL,
      direct_during_opus_overflow,
      sizeof(direct_during_opus_overflow),
      false);
  for (uint16_t i = 0; i < 66u; i++) {
    const uint8_t queued_opus[] = {0xf8, (uint8_t)i};
    fake_webrtc.opus_callback(
        fake_webrtc.opus_callback_userdata,
        &fake_webrtc.peer,
        queued_opus,
        sizeof(queued_opus));
  }
  gzc_buf_init(&received_opus_payload);
  uint16_t overflow_opus_index = 0u;
  uint8_t overflow_direct_count = 0u;
  for (uint16_t i = 0; i < 65u; i++) {
    rc = gzc_client_read_packet(
        client, 0, &received_opus_protocol, &received_opus_payload);
    if (rc == GZC_OK &&
        received_opus_protocol == GZC_PROTOCOL_OPUS_PACKET) {
      if (expect(
              received_opus_payload.len == 2u &&
                  received_opus_payload.data[1] ==
                      (uint8_t)(overflow_opus_index + 2u),
              "Opus RX queue drops oldest and preserves newest") != 0) {
        return 1;
      }
      overflow_opus_index++;
    } else if (
        rc == GZC_OK && received_opus_protocol == 0x40 &&
        received_opus_payload.len == 1u &&
        received_opus_payload.data[0] == 0xe1) {
      overflow_direct_count++;
    } else {
      (void)expect(false, "Opus overflow returned unexpected packet");
      return 1;
    }
  }
  if (expect(
          overflow_opus_index == 64u && overflow_direct_count == 1u,
          "Opus overflow never evicts Direct Packet") != 0) {
    return 1;
  }
  fail_next_realloc = true;
  fake_webrtc.opus_callback(
      fake_webrtc.opus_callback_userdata,
      &fake_webrtc.peer,
      opus_payload,
      sizeof(opus_payload));
  rc = gzc_client_read_packet_into(
      client,
      0,
      &received_opus_protocol,
      fixed_opus_payload,
      sizeof(fixed_opus_payload),
      &fixed_opus_len);
  if (expect(
          rc == GZC_OK && fail_next_realloc &&
              received_opus_protocol == GZC_PROTOCOL_OPUS_PACKET &&
              fixed_opus_len == sizeof(opus_payload),
          "Opus callback and fixed read perform no packet allocation") != 0) {
    return 1;
  }
  fail_next_realloc = false;
  const uint8_t invalid_remote_opus[] = {0xfb, 0x00};
  fake_webrtc.opus_callback(
      fake_webrtc.opus_callback_userdata,
      &fake_webrtc.peer,
      invalid_remote_opus,
      sizeof(invalid_remote_opus));
  rc = gzc_client_read_packet(
      client, 0, &received_opus_protocol, &received_opus_payload);
  if (expect(rc == GZC_ERR_WEBRTC,
             "invalid remote Opus reports one WebRTC error") != 0) {
    return 1;
  }
  gzc_buf_free(&received_opus_payload, platform);

  rc = gzc_client_send_packet(client, GZC_PROTOCOL_TELEMETRY, telemetry_payload, sizeof(telemetry_payload));
  if (expect(rc == GZC_OK, "send telemetry packet") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.sent.len == sizeof(telemetry_payload) + 1 && fake_webrtc.sent.data[0] == GZC_PROTOCOL_TELEMETRY &&
                 memcmp(fake_webrtc.sent.data + 1, telemetry_payload, sizeof(telemetry_payload)) == 0,
             "telemetry packet is protocol-prefixed") != 0) {
    return 1;
  }
  size_t sent_len_before_reserved = fake_webrtc.sent.len;
  rc = gzc_client_send_packet(client, 0x11, telemetry_payload, sizeof(telemetry_payload));
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "reject legacy reserved telemetry protocol") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.sent.len == sent_len_before_reserved, "reserved telemetry protocol is not sent") != 0) {
    return 1;
  }
  rc = gzc_client_send_packet(client, 0x3f, telemetry_payload, sizeof(telemetry_payload));
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "reject reserved packet protocol") != 0) {
    return 1;
  }
  gzc_buf_t received_packet_payload;
  gzc_buf_init(&received_packet_payload);
  uint8_t received_protocol = 0;
  const uint8_t reserved_received_packet[] = {0x11, 0xaa};
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata,
      &fake_webrtc.peer,
      &fake_webrtc.packet_channel,
      NULL,
      reserved_received_packet,
      sizeof(reserved_received_packet),
      false);
  rc = gzc_client_read_packet(client, 0, &received_protocol, &received_packet_payload);
  if (expect(
          rc == GZC_ERR_TIMEOUT,
          "silently ignore received reserved packet protocol") != 0) {
    gzc_buf_free(&received_packet_payload, platform);
    return 1;
  }
  const uint8_t opus_data_channel_packet[] = {
      GZC_PROTOCOL_OPUS_PACKET, 0xf8, 0x55};
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata,
      &fake_webrtc.peer,
      &fake_webrtc.packet_channel,
      NULL,
      opus_data_channel_packet,
      sizeof(opus_data_channel_packet),
      false);
  rc = gzc_client_read_packet(
      client, 0, &received_protocol, &received_packet_payload);
  if (expect(
          rc == GZC_ERR_TIMEOUT,
          "silently ignore Opus on the Direct Packet channel") != 0) {
    gzc_buf_free(&received_packet_payload, platform);
    return 1;
  }
  const uint8_t valid_received_packet[] = {GZC_PROTOCOL_TELEMETRY, 0xbb, 0xcc};
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata,
      &fake_webrtc.peer,
      &fake_webrtc.packet_channel,
      NULL,
      valid_received_packet,
      sizeof(valid_received_packet),
      false);
  rc = gzc_client_read_packet(client, 0, &received_protocol, &received_packet_payload);
  if (expect(rc == GZC_OK && received_protocol == GZC_PROTOCOL_TELEMETRY &&
                 received_packet_payload.len == sizeof(valid_received_packet) - 1 &&
                 memcmp(received_packet_payload.data, valid_received_packet + 1, received_packet_payload.len) == 0,
             "read valid received telemetry packet") != 0) {
    gzc_buf_free(&received_packet_payload, platform);
    return 1;
  }
  gzc_buf_free(&received_packet_payload, platform);
  event_stream = NULL;
  rc = gzc_event_stream_open(client, 1000, &event_stream);
  if (rc == GZC_OK) {
    rc = gzc_event_stream_send(event_stream, &outbound_event);
  }
  if (expect(
          rc == GZC_OK,
          "Event remains usable after ignored Direct Packet protocols") != 0) {
    return 1;
  }
  gzc_event_stream_close(event_stream);
  event_stream = NULL;
  uint8_t *max_telemetry_payload = (uint8_t *)platform->malloc(platform->userdata, GZC_RPC_MAX_FRAME_SIZE);
  if (expect(max_telemetry_payload != NULL, "allocate max telemetry packet") != 0) {
    return 1;
  }
  memset(max_telemetry_payload, 0xa5, GZC_RPC_MAX_FRAME_SIZE);
  rc = gzc_client_send_packet(client, GZC_PROTOCOL_TELEMETRY, max_telemetry_payload, GZC_RPC_MAX_FRAME_SIZE - 1);
  if (expect(rc == GZC_OK, "send max telemetry packet") != 0) {
    platform->free(platform->userdata, max_telemetry_payload);
    return 1;
  }
  if (expect(fake_webrtc.sent.len == GZC_RPC_MAX_FRAME_SIZE && fake_webrtc.sent.data[0] == GZC_PROTOCOL_TELEMETRY,
             "max telemetry packet includes protocol byte") != 0) {
    platform->free(platform->userdata, max_telemetry_payload);
    return 1;
  }
  rc = gzc_client_send_packet(client, GZC_PROTOCOL_TELEMETRY, max_telemetry_payload, GZC_RPC_MAX_FRAME_SIZE);
  platform->free(platform->userdata, max_telemetry_payload);
  if (expect(rc == GZC_ERR_RPC, "reject oversized telemetry packet") != 0) {
    return 1;
  }
  gzc_telemetry_frame_t empty_telemetry_frame;
  memset(&empty_telemetry_frame, 0, sizeof(empty_telemetry_frame));
  rc = gzc_client_send_telemetry(client, &empty_telemetry_frame);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "reject empty telemetry frame") != 0) {
    return 1;
  }
  gzc_telemetry_observation_t observation;
  memset(&observation, 0, sizeof(observation));
  observation.kind = GZC_TELEMETRY_OBSERVATION_BATTERY;
  observation.battery.has_percent = true;
  observation.battery.percent = 77;
  gzc_telemetry_frame_t telemetry_frame;
  memset(&telemetry_frame, 0, sizeof(telemetry_frame));
  telemetry_frame.sequence = 7;
  telemetry_frame.observations = &observation;
  telemetry_frame.observation_count = 1;
  if (expect(telemetry_frame.observations[0].battery.percent == 77, "telemetry public structs are usable") != 0) {
    return 1;
  }
  rc = gzc_client_send_telemetry(client, &telemetry_frame);
  if (expect(rc == GZC_OK, "send encoded telemetry frame") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.sent.len > 1 && fake_webrtc.sent.data[0] == GZC_PROTOCOL_TELEMETRY,
             "encoded telemetry packet is protocol-prefixed") != 0) {
    return 1;
  }
  if (expect(telemetry_frame.observed_at_unix_ms == 0, "send telemetry does not mutate frame timestamp") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.sent.len > 4 && fake_webrtc.sent.data[3] == 0x10,
             "send telemetry stamps observed_at_unix_ms") != 0) {
    return 1;
  }

  gzc_buf_t large_params;
  gzc_buf_init(&large_params);
  const char quote = '"';
  const char x = 'x';
  rc = gzc_buf_append(&large_params, platform, &quote, 1);
  for (size_t i = 0; rc == GZC_OK && i < 70000; i++) {
    rc = gzc_buf_append(&large_params, platform, &x, 1);
  }
  if (rc == GZC_OK) {
    rc = gzc_buf_append(&large_params, platform, &quote, 1);
  }
  if (expect(rc == GZC_OK, "build large params") != 0) {
    return 1;
  }
  rc = test_rpc_call(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)large_params.data, large_params.len),
      &response);
  if (expect(rc == GZC_OK, "send oversized protobuf request envelope as continuation frames") != 0) {
    gzc_buf_free(&large_params, platform);
    return 1;
  }
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, first_frame_size(&fake_webrtc.sent), &sent_frame);
  if (expect(rc == GZC_OK && sent_frame.type == GZC_RPC_FRAME_TEXT, "oversized request starts with text continuation frame") != 0) {
    gzc_buf_free(&large_params, platform);
    return 1;
  }
  if (expect(response.result_payload.len > 0, "oversized rpc call captured result payload") != 0) {
    gzc_buf_free(&large_params, platform);
    return 1;
  }
  gzc_buf_free(&large_params, platform);

  gzc_buf_t invalid_envelope;
  gzc_buf_init(&invalid_envelope);
  rc = gzc_rpc_encode_request_envelope(
      platform,
      gzc_str_from_parts(NULL, 1),
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_cstr("{}"),
      &invalid_envelope);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "reject invalid request id string") != 0) {
    gzc_buf_free(&invalid_envelope, platform);
    return 1;
  }
  rc = gzc_rpc_encode_request_envelope(
      platform,
      gzc_str_from_cstr("1"),
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts(NULL, 1),
      &invalid_envelope);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "reject invalid params payload string") != 0) {
    gzc_buf_free(&invalid_envelope, platform);
    return 1;
  }
  gzc_buf_free(&invalid_envelope, platform);
  gzc_rpc_response_t invalid_response;
  rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts(NULL, 1), &invalid_response);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "reject invalid response payload string") != 0) {
    return 1;
  }

  gzc_str_t raw_nested;
  rc = gzc_json_find_field(
      gzc_str_from_cstr("{\"result\":{\"items\":[{\"id\":\"a\"}],\"ok\":true},\"id\":\"1\"}"),
      "result",
      &raw_nested);
  if (expect(rc == GZC_OK && raw_nested.len > 10, "find nested result raw json") != 0) {
    return 1;
  }

  gzc_str_t escaped;
  rc = gzc_json_parse_string(gzc_str_from_cstr("\"a\\nb\""), &escaped);
  if (expect(rc == GZC_ERR_UNSUPPORTED, "escaped string is not silently decoded") != 0) {
    return 1;
  }
  int32_t too_big = 0;
  rc = gzc_json_parse_i32(gzc_str_from_cstr("2147483648"), &too_big);
  if (expect(rc == GZC_ERR_JSON, "i32 overflow rejected") != 0) {
    return 1;
  }

  gzc_buf_t encoded_binary;
  gzc_buf_init(&encoded_binary);
  const uint8_t binary_payload[] = {0x00, 0xff, 0x10};
  gzc_rpc_frame_t binary_frame;
  memset(&binary_frame, 0, sizeof(binary_frame));
  binary_frame.type = GZC_RPC_FRAME_BINARY;
  binary_frame.data = binary_payload;
  binary_frame.len = sizeof(binary_payload);
  rc = gzc_rpc_frame_encode(platform, &binary_frame, &encoded_binary);
  if (expect(rc == GZC_OK, "encode binary frame") != 0) {
    return 1;
  }
  gzc_rpc_frame_t decoded_binary;
  rc = gzc_rpc_frame_decode(encoded_binary.data, encoded_binary.len, &decoded_binary);
  if (expect(rc == GZC_OK && decoded_binary.type == GZC_RPC_FRAME_BINARY &&
                 decoded_binary.len == sizeof(binary_payload) && memcmp(decoded_binary.data, binary_payload, sizeof(binary_payload)) == 0,
             "decode binary frame") != 0) {
    return 1;
  }
  const uint8_t trailing = 0;
  rc = gzc_buf_append(&encoded_binary, platform, &trailing, 1);
  if (expect(rc == GZC_OK, "append trailing byte") != 0) {
    return 1;
  }
  rc = gzc_rpc_frame_decode(encoded_binary.data, encoded_binary.len, &decoded_binary);
  if (expect(rc == GZC_ERR_RPC, "reject trailing frame bytes") != 0) {
    return 1;
  }
  uint8_t bad_eos[] = {1, 0, 0, 0, 0};
  rc = gzc_rpc_frame_decode(bad_eos, sizeof(bad_eos), &decoded_binary);
  if (expect(rc == GZC_ERR_RPC, "reject eos with payload") != 0) {
    return 1;
  }
  gzc_buf_free(&encoded_binary, platform);

  if (expect(GZC_RPC_MAX_INBOUND_CHANNELS == 4u, "inbound RPC channel limit") != 0) {
    return 1;
  }
  if (expect(gzc_client_poll(NULL, 0) == GZC_ERR_INVALID_ARGUMENT, "poll rejects null client") != 0) {
    return 1;
  }

  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_t inbound_request;
  gzc_buf_t inbound_framed;
  gzc_buf_init(&inbound_request);
  gzc_buf_init(&inbound_framed);
  rc = gzc_rpc_encode_request_envelope(
      platform,
      gzc_str_from_cstr("server-ping"),
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      &inbound_request);
  if (rc == GZC_OK) {
    size_t split = inbound_request.len / 2;
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_TEXT,
                           inbound_request.data, split);
    if (rc == GZC_OK) {
      rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_TEXT,
                             inbound_request.data + split,
                             inbound_request.len - split);
    }
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  if (expect(rc == GZC_OK, "build continued inbound ping request") != 0) {
    return 1;
  }
  gzc_buf_reset(&fake_webrtc.sent);
  int close_count_before_ping = fake_webrtc.close_count;
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL,
      inbound_framed.data, inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  if (expect(rc == GZC_OK, "poll serves inbound ping") != 0) {
    return 1;
  }
  size_t inbound_frame_size = first_frame_size(&fake_webrtc.sent);
  gzc_rpc_frame_t inbound_frame;
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size, &inbound_frame);
  if (expect(rc == GZC_OK && inbound_frame.type == GZC_RPC_FRAME_BINARY,
             "inbound ping response envelope") != 0) {
    return 1;
  }
  gzc_rpc_response_t inbound_response;
  rc = gzc_rpc_decode_response_envelope(
      gzc_str_from_parts((const char *)inbound_frame.data, inbound_frame.len),
      &inbound_response);
  if (expect(rc == GZC_OK && str_eq_cstr(inbound_response.id, "server-ping") &&
                 !inbound_response.has_error,
             "inbound ping response preserves id") != 0) {
    return 1;
  }
  gizclaw_rpc_v1_PingResponse inbound_ping = gizclaw_rpc_v1_PingResponse_init_zero;
  rc = decode_test_pb_message(
      inbound_response.result_payload,
      gizclaw_rpc_v1_PingResponse_fields,
      &inbound_ping);
  if (expect(rc == GZC_OK && inbound_ping.server_time > 0,
             "inbound ping response payload") != 0) {
    return 1;
  }
  size_t eos_offset = inbound_frame_size;
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data + eos_offset,
                            fake_webrtc.sent.len - eos_offset, &inbound_frame);
  if (expect(rc == GZC_OK && inbound_frame.type == GZC_RPC_FRAME_EOS,
             "inbound ping response eos") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.close_count == close_count_before_ping + 1 &&
                 fake_webrtc.last_closed == &fake_webrtc.remote_channels[0],
             "completed inbound ping releases its channel slot") != 0) {
    return 1;
  }

  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&inbound_request);
  gzc_buf_reset(&inbound_framed);
  gzc_buf_reset(&fake_webrtc.sent);
  rc = gzc_rpc_encode_request_envelope(
      platform, gzc_str_from_cstr("client-info"),
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_INFO_GET,
      gzc_str_from_parts("", 0), &inbound_request);
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                           inbound_request.data, inbound_request.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL,
                           0);
  }
  if (expect(rc == GZC_OK, "build inbound client-info request") != 0) {
    return 1;
  }
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL, inbound_framed.data,
      inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  if (expect(rc == GZC_OK, "poll dispatches inbound client-info") != 0) {
    return 1;
  }
  inbound_frame_size = first_frame_size(&fake_webrtc.sent);
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size,
                            &inbound_frame);
  if (rc == GZC_OK) {
    rc = gzc_rpc_decode_response_envelope(
        gzc_str_from_parts((const char *)inbound_frame.data,
                           inbound_frame.len),
        &inbound_response);
  }
  if (expect(rc == GZC_OK && !inbound_response.has_error &&
                 inbound_response.result_payload.len == 2u &&
                 (uint8_t)inbound_response.result_payload.data[0] == 0x0au &&
                 (uint8_t)inbound_response.result_payload.data[1] == 0x00u &&
                 rpc_provider.call_count == 1 &&
                 rpc_provider.method ==
                     gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_INFO_GET &&
                 rpc_provider.last_payload_len == 0u,
             "inbound client-info dispatches configured provider") != 0) {
    return 1;
  }
  close_remote_rpc(&fake_webrtc, 0);

  if (test_device_control_payload_bounds() != 0) {
    return 1;
  }
  {
    gizclaw_rpc_v1_ClientDeviceVolumeSetRequest volume_request =
        gizclaw_rpc_v1_ClientDeviceVolumeSetRequest_init_zero;
    volume_request.level = 35;
    volume_request.muted = true;
    gzc_buf_t control_payload;
    gzc_buf_init(&control_payload);
    rc = encode_test_pb_message(
        platform, gizclaw_rpc_v1_ClientDeviceVolumeSetRequest_fields,
        &volume_request, &control_payload);
    if (expect(rc == GZC_OK && control_payload.len > 0u,
               "encode device volume request") != 0) {
      return 1;
    }
    static const gizclaw_rpc_v1_RpcMethod control_methods[] = {
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_STATUS_GET,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_VOLUME_SET,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_SOUND_PLAY,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_REBOOT,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_STATUS_GET,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_SAVED_LIST,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_SAVED_FORGET,
    };
    static const int control_method_ids[] = {100, 101, 102, 103, 104, 105, 106};
    for (size_t i = 0; i < sizeof(control_methods) / sizeof(control_methods[0]); i++) {
      if (expect((int)control_methods[i] == control_method_ids[i],
                 "device control method id matches rpc.proto") != 0) {
        return 1;
      }
      gzc_str_t control_params = gzc_str_from_parts("", 0);
      if (control_methods[i] ==
          gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_VOLUME_SET) {
        control_params = gzc_str_from_parts((const char *)control_payload.data,
                                            control_payload.len);
      }
      announce_remote_rpc(&fake_webrtc, 0);
      gzc_buf_reset(&inbound_request);
      gzc_buf_reset(&inbound_framed);
      gzc_buf_reset(&fake_webrtc.sent);
      rc = gzc_rpc_encode_request_envelope(
          platform, gzc_str_from_cstr("client-device-control"),
          control_methods[i], control_params, &inbound_request);
      if (rc == GZC_OK) {
        rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                               inbound_request.data, inbound_request.len);
      }
      if (rc == GZC_OK) {
        rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS,
                               NULL, 0);
      }
      if (expect(rc == GZC_OK, "build inbound device control request") != 0) {
        return 1;
      }
      int control_calls_before = rpc_provider.call_count;
      fake_webrtc.callbacks.on_channel_message(
          fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
          &fake_webrtc.remote_channels[0], NULL, inbound_framed.data,
          inbound_framed.len, false);
      rc = gzc_client_poll(client, 0);
      if (expect(rc == GZC_OK, "poll dispatches inbound device control") != 0) {
        return 1;
      }
      inbound_frame_size = first_frame_size(&fake_webrtc.sent);
      rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size,
                                &inbound_frame);
      if (rc == GZC_OK) {
        rc = gzc_rpc_decode_response_envelope(
            gzc_str_from_parts((const char *)inbound_frame.data,
                               inbound_frame.len),
            &inbound_response);
      }
      if (expect(rc == GZC_OK && !inbound_response.has_error &&
                     rpc_provider.call_count == control_calls_before + 1 &&
                     rpc_provider.method == (int)control_methods[i] &&
                     rpc_provider.last_payload_len == control_params.len,
                 "inbound device control dispatches configured provider") != 0) {
        return 1;
      }
      close_remote_rpc(&fake_webrtc, 0);
    }
    gzc_buf_free(&control_payload, platform);
  }

  gizclaw_rpc_v1_ToolInvokeRequest tool_request =
      gizclaw_rpc_v1_ToolInvokeRequest_init_zero;
  strcpy(tool_request.invoke_name, "volume_set");
  gzc_buf_t tool_payload;
  gzc_buf_init(&tool_payload);
  rc = encode_test_pb_message(
      platform, gizclaw_rpc_v1_ToolInvokeRequest_fields, &tool_request,
      &tool_payload);
  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&inbound_request);
  gzc_buf_reset(&inbound_framed);
  gzc_buf_reset(&fake_webrtc.sent);
  if (rc == GZC_OK) {
    rc = gzc_rpc_encode_request_envelope(
        platform, gzc_str_from_cstr("client-tool"),
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_TOOL_INVOKE,
        gzc_str_from_parts((const char *)tool_payload.data, tool_payload.len),
        &inbound_request);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                           inbound_request.data, inbound_request.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL,
                           0);
  }
  if (expect(rc == GZC_OK, "build inbound client Tool request") != 0) {
    return 1;
  }
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL, inbound_framed.data,
      inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  inbound_frame_size = first_frame_size(&fake_webrtc.sent);
  if (rc == GZC_OK) {
    rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size,
                              &inbound_frame);
  }
  if (rc == GZC_OK) {
    rc = gzc_rpc_decode_response_envelope(
        gzc_str_from_parts((const char *)inbound_frame.data,
                           inbound_frame.len),
        &inbound_response);
  }
  gizclaw_rpc_v1_ToolInvokeResponse tool_response =
      gizclaw_rpc_v1_ToolInvokeResponse_init_zero;
  if (rc == GZC_OK) {
    rc = decode_test_pb_message(
        inbound_response.result_payload,
        gizclaw_rpc_v1_ToolInvokeResponse_fields, &tool_response);
  }
  if (expect(rc == GZC_OK && !inbound_response.has_error &&
                 strcmp(tool_response.data_json, "{\"ok\":true}") == 0 &&
                 tool_handler.call_count == 1,
             "inbound client Tool dispatches exact-name handler") != 0) {
    return 1;
  }
  close_remote_rpc(&fake_webrtc, 0);

  strcpy(tool_request.invoke_name, "brightness_set");
  gzc_buf_reset(&tool_payload);
  rc = encode_test_pb_message(
      platform, gizclaw_rpc_v1_ToolInvokeRequest_fields, &tool_request,
      &tool_payload);
  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&inbound_request);
  gzc_buf_reset(&inbound_framed);
  gzc_buf_reset(&fake_webrtc.sent);
  if (rc == GZC_OK) {
    rc = gzc_rpc_encode_request_envelope(
        platform, gzc_str_from_cstr("missing-client-tool"),
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_TOOL_INVOKE,
        gzc_str_from_parts((const char *)tool_payload.data, tool_payload.len),
        &inbound_request);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                           inbound_request.data, inbound_request.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL,
                           0);
  }
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL, inbound_framed.data,
      inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  inbound_frame_size = first_frame_size(&fake_webrtc.sent);
  if (rc == GZC_OK) {
    rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size,
                              &inbound_frame);
  }
  if (rc == GZC_OK) {
    rc = gzc_rpc_decode_response_envelope(
        gzc_str_from_parts((const char *)inbound_frame.data,
                           inbound_frame.len),
        &inbound_response);
  }
  if (expect(rc == GZC_OK && inbound_response.has_error &&
                 inbound_response.error.code ==
                     gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_METHOD_NOT_FOUND &&
                 tool_handler.call_count == 1,
             "missing client Tool handler is unavailable") != 0) {
    return 1;
  }
  gzc_buf_free(&tool_payload, platform);
  close_remote_rpc(&fake_webrtc, 0);

  gzc_buf_reset(&inbound_request);
  gzc_buf_reset(&inbound_framed);
  rc = gzc_rpc_encode_request_envelope(
      platform, gzc_str_from_cstr("client-info"),
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_INFO_GET,
      gzc_str_from_parts("", 0), &inbound_request);
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                           inbound_request.data, inbound_request.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL,
                           0);
  }
  if (expect(rc == GZC_OK, "restore inbound client-info request") != 0) {
    return 1;
  }

  rpc_provider.mode = FAKE_RPC_PROVIDER_ERROR;
  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&fake_webrtc.sent);
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL, inbound_framed.data,
      inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  if (expect(rc == GZC_OK, "poll dispatches provider error") != 0) {
    return 1;
  }
  inbound_frame_size = first_frame_size(&fake_webrtc.sent);
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size,
                            &inbound_frame);
  if (rc == GZC_OK) {
    rc = gzc_rpc_decode_response_envelope(
        gzc_str_from_parts((const char *)inbound_frame.data,
                           inbound_frame.len),
        &inbound_response);
  }
  if (expect(rc == GZC_OK && inbound_response.has_error &&
                 inbound_response.error.code ==
                     gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_FORBIDDEN &&
                 str_eq_cstr(inbound_response.error.message, "provider denied"),
             "provider error response copies borrowed message") != 0) {
    return 1;
  }
  close_remote_rpc(&fake_webrtc, 0);

  rpc_provider.mode = FAKE_RPC_PROVIDER_NO_RESPONSE;
  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&fake_webrtc.sent);
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL, inbound_framed.data,
      inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  if (expect(rc == GZC_OK, "poll handles provider without response") != 0) {
    return 1;
  }
  inbound_frame_size = first_frame_size(&fake_webrtc.sent);
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size,
                            &inbound_frame);
  if (rc == GZC_OK) {
    rc = gzc_rpc_decode_response_envelope(
        gzc_str_from_parts((const char *)inbound_frame.data,
                           inbound_frame.len),
        &inbound_response);
  }
  if (expect(rc == GZC_OK && inbound_response.has_error &&
                 inbound_response.error.code ==
                     gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INTERNAL_ERROR,
             "provider must respond exactly once") != 0) {
    return 1;
  }
  close_remote_rpc(&fake_webrtc, 0);
  rpc_provider.mode = FAKE_RPC_PROVIDER_SUCCESS;

  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&fake_webrtc.sent);
  fake_webrtc.buffered_amount = GZC_SERVICE_WRITE_HIGH_WATER_DEFAULT;
  fake_webrtc.drain_on_poll = false;
  int close_count_before_deferred_timeout = fake_webrtc.close_count;
  int64_t deferred_unix_start = clock.unix_ms;
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL,
      inbound_framed.data, inbound_framed.len, false);
  rc = gzc_client_poll(client, 37);
  if (expect(rc == GZC_OK && fake_webrtc.last_poll_timeout_ms == 0,
             "new deferred output requests an immediate poll") != 0) {
    return 1;
  }
  rc = gzc_client_poll(client, 37);
  if (expect(rc == GZC_OK && fake_webrtc.last_poll_timeout_ms == 37,
             "backpressured deferred output preserves the caller poll timeout") != 0) {
    return 1;
  }
  rc = gzc_client_poll(client, 60000);
  if (expect(rc == GZC_ERR_TIMEOUT && fake_webrtc.sent.len == 0 &&
                 fake_webrtc.last_poll_timeout_ms > 0 &&
                 fake_webrtc.last_poll_timeout_ms < config.write_timeout_ms &&
                 fake_webrtc.close_count == close_count_before_deferred_timeout + 1 &&
                 fake_webrtc.last_closed == &fake_webrtc.remote_channels[0],
             "blocked deferred output caps poll by its timeout and closes") != 0 ||
      expect(clock.unix_ms < deferred_unix_start,
             "deferred timeout ignores backward Unix clock movement") != 0) {
    return 1;
  }
  fake_webrtc.drain_on_poll = true;
  fake_webrtc.buffered_amount = 0;
  rc = GZC_OK;

  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    announce_remote_rpc(&fake_webrtc, i);
  }
  int close_count_before_limit = fake_webrtc.close_count;
  announce_remote_rpc(&fake_webrtc, GZC_RPC_MAX_INBOUND_CHANNELS);
  if (expect(fake_webrtc.close_count == close_count_before_limit + 1 &&
                 fake_webrtc.last_closed ==
                     &fake_webrtc.remote_channels[GZC_RPC_MAX_INBOUND_CHANNELS],
             "fifth inbound RPC channel is rejected") != 0) {
    return 1;
  }
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    close_remote_rpc(&fake_webrtc, i);
  }

  gizclaw_rpc_v1_SpeedTestRequest speed_request = gizclaw_rpc_v1_SpeedTestRequest_init_zero;
  speed_request.up_content_length = 2;
  speed_request.down_content_length = 17 * 4096;
  gzc_buf_t speed_payload;
  gzc_buf_init(&speed_payload);
  size_t oversized_id_len = GZC_RPC_MAX_FRAME_SIZE;
  char *oversized_id = (char *)platform->malloc(platform->userdata, oversized_id_len);
  if (oversized_id == NULL) {
    return 1;
  }
  memset(oversized_id, 's', oversized_id_len);
  gzc_buf_reset(&inbound_request);
  gzc_buf_reset(&inbound_framed);
  rc = encode_test_pb_message(
      platform, gizclaw_rpc_v1_SpeedTestRequest_fields, &speed_request,
      &speed_payload);
  if (rc == GZC_OK) {
    rc = gzc_rpc_encode_request_envelope(
        platform, gzc_str_from_parts(oversized_id, oversized_id_len),
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
        gzc_str_from_parts((const char *)speed_payload.data, speed_payload.len),
        &inbound_request);
  }
  for (size_t request_offset = 0;
       rc == GZC_OK && request_offset < inbound_request.len;) {
    size_t chunk = inbound_request.len - request_offset;
    if (chunk > GZC_RPC_MAX_FRAME_SIZE) {
      chunk = GZC_RPC_MAX_FRAME_SIZE;
    }
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_TEXT,
                           inbound_request.data + request_offset, chunk);
    request_offset += chunk;
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  const uint8_t upload_body[] = {0x01, 0x02};
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                           upload_body, sizeof(upload_body));
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  if (expect(rc == GZC_OK, "build inbound speed request") != 0) {
    return 1;
  }
  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&fake_webrtc.sent);
  int close_count_before_speed = fake_webrtc.close_count;
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL,
      inbound_framed.data, inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  if (expect(rc == GZC_OK, "poll serves inbound full-duplex speed test") != 0) {
    return 1;
  }
  clock.instant_ms += config.write_timeout_ms - 4;
  for (size_t i = 0; i < 4 && fake_webrtc.close_count == close_count_before_speed; i++) {
    rc = gzc_client_poll(client, 0);
    if (expect(rc == GZC_OK,
               "each deferred response batch gets an independent timeout") != 0) {
      return 1;
    }
  }
  size_t offset = 0;
  size_t response_frames = 0;
  size_t download_bytes = 0;
  bool saw_response_delimiter = false;
  bool saw_response_eos = false;
  gzc_buf_t continued_response;
  gzc_buf_init(&continued_response);
  while (offset < fake_webrtc.sent.len) {
    size_t size = first_frame_size(&(gzc_buf_t){
        .data = fake_webrtc.sent.data + offset,
        .len = fake_webrtc.sent.len - offset,
        .cap = fake_webrtc.sent.len - offset});
    rc = gzc_rpc_frame_decode(fake_webrtc.sent.data + offset, size, &inbound_frame);
    if (rc != GZC_OK) {
      return 1;
    }
    response_frames++;
    if (!saw_response_delimiter && inbound_frame.type == GZC_RPC_FRAME_TEXT) {
      rc = gzc_buf_append(&continued_response, platform, inbound_frame.data, inbound_frame.len);
      if (rc != GZC_OK) {
        return 1;
      }
    } else if (!saw_response_delimiter && inbound_frame.type == GZC_RPC_FRAME_EOS) {
      saw_response_delimiter = true;
    } else if (saw_response_delimiter && inbound_frame.type == GZC_RPC_FRAME_BINARY) {
      download_bytes += inbound_frame.len;
    } else if (saw_response_delimiter && inbound_frame.type == GZC_RPC_FRAME_EOS) {
      saw_response_eos = true;
    } else {
      return 1;
    }
    offset += size;
  }
  rc = gzc_rpc_decode_response_envelope(
      gzc_str_from_parts((const char *)continued_response.data, continued_response.len),
      &inbound_response);
  if (expect(rc == GZC_OK && response_frames == 21 && saw_response_delimiter &&
                 saw_response_eos &&
                 download_bytes == (size_t)speed_request.down_content_length &&
                 !inbound_response.has_error &&
                 inbound_response.id.len == oversized_id_len &&
                 memcmp(inbound_response.id.data, oversized_id, oversized_id_len) == 0,
             "continued inbound speed response batches body frames and eos") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.close_count == close_count_before_speed + 1 &&
                 fake_webrtc.last_closed == &fake_webrtc.remote_channels[0],
             "completed inbound speed test releases its channel slot") != 0) {
    return 1;
  }
  gzc_buf_free(&continued_response, platform);
  platform->free(platform->userdata, oversized_id);
  close_remote_rpc(&fake_webrtc, 0);

  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&inbound_request);
  gzc_buf_reset(&inbound_framed);
  gzc_buf_reset(&fake_webrtc.sent);
  rc = gzc_rpc_encode_request_envelope(
      platform, gzc_str_from_cstr("unknown-method"),
      (gizclaw_rpc_v1_RpcMethod)999,
      gzc_str_from_parts((const char *)params.data, params.len),
      &inbound_request);
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                           inbound_request.data, inbound_request.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  if (expect(rc == GZC_OK, "build unknown inbound method") != 0) {
    return 1;
  }
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL,
      inbound_framed.data, inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  if (rc != GZC_OK) {
    return 1;
  }
  inbound_frame_size = first_frame_size(&fake_webrtc.sent);
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size, &inbound_frame);
  if (rc == GZC_OK) {
    rc = gzc_rpc_decode_response_envelope(
        gzc_str_from_parts((const char *)inbound_frame.data, inbound_frame.len),
        &inbound_response);
  }
  if (expect(rc == GZC_OK && inbound_response.has_error &&
                 inbound_response.error.code ==
                     gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_METHOD_NOT_FOUND,
             "unknown inbound method returns method-not-found") != 0) {
    return 1;
  }
  close_remote_rpc(&fake_webrtc, 0);

  const gizclaw_rpc_v1_RpcMethod missing_payload_methods[] = {
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_INFO_GET,
  };
  for (size_t i = 0; i < sizeof(missing_payload_methods) / sizeof(missing_payload_methods[0]); i++) {
    announce_remote_rpc(&fake_webrtc, 0);
    gzc_buf_reset(&inbound_request);
    gzc_buf_reset(&inbound_framed);
    gzc_buf_reset(&fake_webrtc.sent);
    rc = append_test_proto_bytes(
        platform, &inbound_request, 1, (const uint8_t *)"missing-payload",
        strlen("missing-payload"));
    if (rc == GZC_OK) {
      rc = append_test_proto_varint(
          platform, &inbound_request, 2, (uint64_t)missing_payload_methods[i]);
    }
    if (rc == GZC_OK) {
      rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                             inbound_request.data, inbound_request.len);
    }
    if (rc == GZC_OK) {
      rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL, 0);
    }
    if (expect(rc == GZC_OK, "build missing-payload inbound request") != 0) {
      return 1;
    }
    fake_webrtc.callbacks.on_channel_message(
        fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
        &fake_webrtc.remote_channels[0], NULL,
        inbound_framed.data, inbound_framed.len, false);
    rc = gzc_client_poll(client, 0);
    if (rc != GZC_OK) {
      return 1;
    }
    inbound_frame_size = first_frame_size(&fake_webrtc.sent);
    rc = gzc_rpc_frame_decode(
        fake_webrtc.sent.data, inbound_frame_size, &inbound_frame);
    if (rc == GZC_OK) {
      rc = gzc_rpc_decode_response_envelope(
          gzc_str_from_parts((const char *)inbound_frame.data, inbound_frame.len),
          &inbound_response);
    }
    if (expect(rc == GZC_OK && inbound_response.has_error &&
                   inbound_response.error.code ==
                       gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
               "missing inbound payload returns invalid-params") != 0) {
      return 1;
    }
    close_remote_rpc(&fake_webrtc, 0);
  }

  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&inbound_request);
  gzc_buf_reset(&inbound_framed);
  gzc_buf_reset(&fake_webrtc.sent);
  rc = gzc_rpc_encode_request_envelope(
      platform, gzc_str_from_parts("", 0),
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      &inbound_request);
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                           inbound_request.data, inbound_request.len);
  }
  if (rc == GZC_OK) {
    rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  if (expect(rc == GZC_OK, "build empty-id inbound request") != 0) {
    return 1;
  }
  fake_webrtc.callbacks.on_channel_message(
      fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
      &fake_webrtc.remote_channels[0], NULL,
      inbound_framed.data, inbound_framed.len, false);
  rc = gzc_client_poll(client, 0);
  if (rc != GZC_OK) {
    return 1;
  }
  inbound_frame_size = first_frame_size(&fake_webrtc.sent);
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, inbound_frame_size, &inbound_frame);
  if (rc == GZC_OK) {
    rc = gzc_rpc_decode_response_envelope(
        gzc_str_from_parts((const char *)inbound_frame.data, inbound_frame.len),
        &inbound_response);
  }
  if (expect(rc == GZC_OK && inbound_response.has_error &&
                 inbound_response.error.code ==
                     gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_REQUEST,
             "empty inbound request id returns invalid-request") != 0) {
    return 1;
  }
  close_remote_rpc(&fake_webrtc, 0);

  announce_remote_rpc(&fake_webrtc, 0);
  gzc_buf_reset(&inbound_framed);
  gzc_buf_reset(&fake_webrtc.sent);
  const uint8_t malformed_protobuf[] = {0xff};
  rc = append_test_frame(platform, &inbound_framed, GZC_RPC_FRAME_BINARY,
                         malformed_protobuf, sizeof(malformed_protobuf));
  int close_count_before_malformed = fake_webrtc.close_count;
  if (rc == GZC_OK) {
    fake_webrtc.callbacks.on_channel_message(
        fake_webrtc.callbacks.userdata, &fake_webrtc.peer,
        &fake_webrtc.remote_channels[0], NULL,
        inbound_framed.data, inbound_framed.len, false);
  }
  if (expect(rc == GZC_OK && fake_webrtc.close_count == close_count_before_malformed + 1 &&
                 fake_webrtc.sent.len == 0,
             "malformed inbound protobuf without id closes channel") != 0) {
    return 1;
  }
  close_remote_rpc(&fake_webrtc, 0);

  event_stream = NULL;
  rc = gzc_event_stream_open(client, 1000, &event_stream);
  if (expect(
          rc == GZC_OK && event_stream != NULL,
          "acquire Event handle before connection close") != 0) {
    return 1;
  }
  fake_webrtc.response_mode = FAKE_RESPONSE_DEFERRED_PROTO;
  gzc_rpc_request_t *client_lifetime_request = NULL;
  rc = gzc_rpc_request_start(
      client,
      0u,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      1000,
      &client_lifetime_request);
  if (expect(
          rc == GZC_OK && client_lifetime_request != NULL,
          "pending unary request exists before client destruction") != 0) {
    return 1;
  }
  gzc_rtc_opus_frame_cb late_opus_callback =
      fake_webrtc.opus_callback;
  void *late_opus_callback_userdata =
      fake_webrtc.opus_callback_userdata;
  fake_emit_channel_state(
      &fake_webrtc,
      &fake_webrtc.packet_channel,
      NULL,
      GZC_RTC_CHANNEL_CLOSED);
  fake_emit_channel_state(
      &fake_webrtc,
      &fake_webrtc.packet_channel,
      NULL,
      GZC_RTC_CHANNEL_CLOSED);
  const int peer_close_count_before_failure =
      fake_webrtc.peer_close_count;
  rc = gzc_client_poll(client, 0);
  if (expect(
          rc == GZC_ERR_CLOSED &&
              fake_webrtc.peer_close_count ==
                  peer_close_count_before_failure + 1 &&
              fake_webrtc.stale_close_count == 0,
          "mandatory packet failure closes the physical Peer") != 0) {
    return 1;
  }
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      (const uint8_t[]){0xf8, 0x55},
      2u);
  if (expect(
          rc == GZC_ERR_CLOSED,
          "closed mandatory packet channel stops the connection") != 0) {
    return 1;
  }
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_TELEMETRY,
      (const uint8_t[]){0x01},
      1u);
  if (expect(
          rc == GZC_ERR_CLOSED,
          "closed mandatory packet channel blocks direct packets") != 0) {
    return 1;
  }

  gzc_buf_free(&speed_payload, platform);
  gzc_buf_free(&inbound_request, platform);
  gzc_buf_free(&inbound_framed, platform);

  gzc_buf_free(&params, platform);
  gzc_buf_free(&fake_webrtc.sent, platform);
  gzc_buf_free(&fake_webrtc.outgoing, platform);
  gzc_buf_free(&fake_webrtc.native_sent, platform);
  gzc_buf_free(&fake_webrtc.opus_sent, platform);
  rc = gzc_client_close(client);
  if (expect(rc == GZC_OK && gzc_client_poll(client, 0) == GZC_ERR_CLOSED,
             "poll reports closed client") != 0) {
    return 1;
  }
  if (expect(
          gzc_rpc_request_result(
              client_lifetime_request, &response) == GZC_ERR_CLOSED,
          "client close invalidates but does not free a unary request handle") !=
      0) {
    return 1;
  }
  if (expect(fake_webrtc.opus_unregister_count == 2,
             "Opus callback unregistered before peer close") != 0) {
    return 1;
  }
  if (expect(
          gzc_event_stream_send(event_stream, &outbound_event) ==
              GZC_ERR_CLOSED,
          "client close invalidates a live Event access handle") != 0) {
    return 1;
  }
  gzc_event_stream_close(event_stream);
  event_stream = NULL;
  rc = gzc_client_send_packet(
      client,
      GZC_PROTOCOL_OPUS_PACKET,
      opus_payload,
      sizeof(opus_payload));
  if (expect(rc == GZC_ERR_CLOSED,
             "Opus send reports closed client") != 0) {
    return 1;
  }
  late_opus_callback(
      late_opus_callback_userdata,
      &fake_webrtc.peer,
      opus_payload,
      sizeof(opus_payload));
  gzc_buf_init(&received_opus_payload);
  rc = gzc_client_read_packet(
      client, -1, &received_opus_protocol, &received_opus_payload);
  if (expect(rc == GZC_ERR_CLOSED,
             "late native Opus callback is ignored after close") != 0) {
    return 1;
  }
  gzc_buf_free(&received_opus_payload, platform);
  rc = gzc_client_close(client);
  if (expect(rc == GZC_OK &&
                 fake_webrtc.opus_unregister_count == 2,
             "repeated close is idempotent") != 0) {
    return 1;
  }
  gzc_rpc_request_destroy(test_last_rpc_request);
  test_last_rpc_request = NULL;
  gzc_client_destroy(client);
  gzc_rpc_request_destroy(client_lifetime_request);

  fake_webrtc_t fake_webrtc_custom;
  memset(&fake_webrtc_custom, 0, sizeof(fake_webrtc_custom));
  fake_webrtc_custom.platform = platform;
  fake_webrtc_custom.clock = &clock;
  fake_webrtc_custom.drain_on_poll = true;
  fake_webrtc_custom.emit_low_event = true;
  gzc_buf_init(&fake_webrtc_custom.sent);
  gzc_buf_init(&fake_webrtc_custom.outgoing);
  gzc_buf_init(&fake_webrtc_custom.native_sent);
  fake_http_t fake_http_custom;
  memset(&fake_http_custom, 0, sizeof(fake_http_custom));
  fake_http_custom.platform = platform;
  gzc_webrtc_vtable_t webrtc_custom = webrtc;
  webrtc_custom.userdata = &fake_webrtc_custom;
  gzc_http_vtable_t http_custom = http;
  http_custom.userdata = &fake_http_custom;
  gzc_client_config_t config_custom = config;
  config_custom.http = &http_custom;
  config_custom.webrtc = &webrtc_custom;
  config_custom.service_write_high_water_bytes = 512u * 1024u;
  config_custom.service_write_low_water_bytes = 128u * 1024u;
  gzc_client_t *client_custom = NULL;
  rc = gzc_client_create(&config_custom, &client_custom);
  if (rc == GZC_OK) {
    rc = gzc_client_set_webrtc_media(client_custom, &media);
  }
  fake_webrtc_custom.client = client_custom;
  if (rc == GZC_OK) {
    rc = gzc_client_set_peer_add_ice_server(client_custom, test_peer_add_ice_server);
  }
  if (rc == GZC_OK) {
    rc = gzc_client_connect(client_custom);
  }
  gzc_service_channel_t *custom_channel = NULL;
  if (rc == GZC_OK) {
    rc = gzc_client_open_service_channel(client_custom, 49, 1000, &custom_channel);
  }
  if (rc == GZC_OK) {
    announce_remote_rpc(&fake_webrtc_custom, 0);
  }
  if (expect(rc == GZC_OK && fake_webrtc_custom.threshold_count == 3u &&
                 fake_webrtc_custom.threshold_values[0] == 128u * 1024u &&
                 fake_webrtc_custom.threshold_values[1] == 128u * 1024u &&
                 fake_webrtc_custom.threshold_values[2] == 128u * 1024u &&
                 fake_webrtc_custom.threshold_channels[0] == &fake_webrtc_custom.service_channels[15] &&
                 fake_webrtc_custom.threshold_channels[1] == &fake_webrtc_custom.edge_channel &&
                 fake_webrtc_custom.threshold_channels[2] == &fake_webrtc_custom.remote_channels[0],
             "custom low-water threshold applies to every service channel") != 0) {
    return 1;
  }
  gzc_rpc_frame_t custom_eos;
  memset(&custom_eos, 0, sizeof(custom_eos));
  custom_eos.type = GZC_RPC_FRAME_EOS;
  fake_webrtc_custom.buffered_amount = 300u * 1024u;
  int custom_poll_count = fake_webrtc_custom.poll_count;
  rc = gzc_service_channel_send_frame(custom_channel, &custom_eos);
  if (expect(rc == GZC_OK && fake_webrtc_custom.poll_count == custom_poll_count,
             "custom high-water allows writes above the embedded default") != 0) {
    return 1;
  }
  fake_webrtc_custom.buffered_amount = 512u * 1024u;
  rc = gzc_service_channel_send_frame(custom_channel, &custom_eos);
  if (expect(rc == GZC_OK && fake_webrtc_custom.poll_count > custom_poll_count &&
                 fake_webrtc_custom.buffered_amount == 128u * 1024u + 4u,
             "custom high-water resumes from the custom low-water boundary") != 0) {
    return 1;
  }
  gzc_service_channel_close(custom_channel);
  close_remote_rpc_with_state(
      &fake_webrtc_custom, 0, GZC_RTC_CHANNEL_ERROR);
  close_remote_rpc_with_state(
      &fake_webrtc_custom, 0, GZC_RTC_CHANNEL_ERROR);
  gzc_rtc_channel_info_t custom_event_info;
  memset(&custom_event_info, 0, sizeof(custom_event_info));
  custom_event_info.label = gzc_str_from_cstr("giznet/v1/service/32");
  custom_event_info.stream_id = 32;
  custom_event_info.ordered = true;
  custom_event_info.reliable = true;
  int close_count_before_event_terminal = fake_webrtc_custom.close_count;
  fake_emit_channel_state(
      &fake_webrtc_custom,
      &fake_webrtc_custom.service_channels[15],
      &custom_event_info,
      GZC_RTC_CHANNEL_ERROR);
  fake_emit_channel_state(
      &fake_webrtc_custom,
      &fake_webrtc_custom.service_channels[15],
      &custom_event_info,
      GZC_RTC_CHANNEL_ERROR);
  rc = gzc_client_poll(client_custom, 0);
  if (expect(
          rc == GZC_ERR_CLOSED &&
              fake_webrtc_custom.close_count ==
                  close_count_before_event_terminal + 1 &&
              fake_webrtc_custom.last_closed ==
                  &fake_webrtc_custom.packet_channel &&
              fake_webrtc_custom.stale_close_count == 0,
          "terminal Event callback closes the client without re-closing consumed channels") != 0) {
    return 1;
  }
  gzc_client_close(client_custom);
  gzc_client_destroy(client_custom);
  gzc_buf_free(&fake_webrtc_custom.sent, platform);
  gzc_buf_free(&fake_webrtc_custom.outgoing, platform);
  gzc_buf_free(&fake_webrtc_custom.native_sent, platform);

  fake_webrtc_t fake_webrtc_no_ice_hook;
  memset(&fake_webrtc_no_ice_hook, 0, sizeof(fake_webrtc_no_ice_hook));
  fake_webrtc_no_ice_hook.platform = platform;
  fake_webrtc_no_ice_hook.clock = &clock;
  fake_webrtc_no_ice_hook.drain_on_poll = true;
  fake_webrtc_no_ice_hook.emit_low_event = true;
  gzc_buf_init(&fake_webrtc_no_ice_hook.sent);
  gzc_buf_init(&fake_webrtc_no_ice_hook.outgoing);
  gzc_buf_init(&fake_webrtc_no_ice_hook.native_sent);

  fake_http_t fake_http_no_ice_hook;
  memset(&fake_http_no_ice_hook, 0, sizeof(fake_http_no_ice_hook));
  fake_http_no_ice_hook.platform = platform;

  gzc_webrtc_vtable_t webrtc_no_ice_hook = webrtc;
  webrtc_no_ice_hook.userdata = &fake_webrtc_no_ice_hook;

  gzc_http_vtable_t http_no_ice_hook = http;
  http_no_ice_hook.userdata = &fake_http_no_ice_hook;

  gzc_client_config_t config_no_ice_hook = config;
  config_no_ice_hook.http = &http_no_ice_hook;
  config_no_ice_hook.webrtc = &webrtc_no_ice_hook;

  gzc_client_t *client_no_ice_hook = NULL;
  rc = gzc_client_create(&config_no_ice_hook, &client_no_ice_hook);
  if (expect(rc == GZC_OK, "client create without ICE hook") != 0) {
    gzc_buf_free(&fake_webrtc_no_ice_hook.sent, platform);
    gzc_buf_free(&fake_webrtc_no_ice_hook.outgoing, platform);
    gzc_buf_free(&fake_webrtc_no_ice_hook.native_sent, platform);
    return 1;
  }
  rc = gzc_client_set_webrtc_media(client_no_ice_hook, &media);
  if (expect(
          rc == GZC_OK,
          "client without ICE hook still registers mandatory media") != 0) {
    gzc_client_destroy(client_no_ice_hook);
    return 1;
  }
  rc = gzc_client_connect(client_no_ice_hook);
  if (expect(rc == GZC_ERR_UNSUPPORTED, "client connect without ICE hook rejects advertised ICE metadata") != 0) {
    gzc_client_destroy(client_no_ice_hook);
    gzc_buf_free(&fake_webrtc_no_ice_hook.sent, platform);
    gzc_buf_free(&fake_webrtc_no_ice_hook.outgoing, platform);
    gzc_buf_free(&fake_webrtc_no_ice_hook.native_sent, platform);
    return 1;
  }
  if (expect(fake_webrtc_no_ice_hook.ice_server_count == 0, "missing ICE hook skips advertised ICE servers") != 0) {
    gzc_client_destroy(client_no_ice_hook);
    gzc_buf_free(&fake_webrtc_no_ice_hook.sent, platform);
    gzc_buf_free(&fake_webrtc_no_ice_hook.outgoing, platform);
    gzc_buf_free(&fake_webrtc_no_ice_hook.native_sent, platform);
    return 1;
  }
  gzc_client_destroy(client_no_ice_hook);
  gzc_buf_free(&fake_webrtc_no_ice_hook.sent, platform);
  gzc_buf_free(&fake_webrtc_no_ice_hook.outgoing, platform);
  gzc_buf_free(&fake_webrtc_no_ice_hook.native_sent, platform);

  fake_webrtc_t fake_webrtc_gateway;
  memset(&fake_webrtc_gateway, 0, sizeof(fake_webrtc_gateway));
  fake_webrtc_gateway.platform = platform;
  fake_webrtc_gateway.clock = &clock;
  fake_webrtc_gateway.drain_on_poll = true;
  fake_webrtc_gateway.emit_low_event = true;
  gzc_buf_init(&fake_webrtc_gateway.sent);
  gzc_buf_init(&fake_webrtc_gateway.outgoing);
  gzc_buf_init(&fake_webrtc_gateway.native_sent);
  gzc_buf_init(&fake_webrtc_gateway.opus_sent);

  fake_http_t fake_http_gateway;
  memset(&fake_http_gateway, 0, sizeof(fake_http_gateway));
  fake_http_gateway.platform = platform;
  fake_http_gateway.expected_post_url = "http://edge.invalid:9821/edge/offer";
  fake_http_gateway.server_info_body =
      "{\"protocol\":\"gizclaw-webrtc\","
      "\"public_key\":\"8mfzTdZB1JA43QmNAMWfTfkj5GC9TJxJFveThi9tvK6J\","
      "\"ice_servers\":[{\"urls\":[\"turn:server.invalid:3478\"]}],"
      "\"transport\":{\"mode\":\"edge-gateway\","
      "\"endpoint\":\"edge.invalid:9821\","
      "\"public_key\":\"FNSseo3ePDEyJR27qEbDCSKBX4baMg826xXcanV4Huqs\","
      "\"signaling_path\":\"/edge/offer\"}}";

  gzc_webrtc_vtable_t webrtc_gateway = webrtc;
  webrtc_gateway.userdata = &fake_webrtc_gateway;
  gzc_http_vtable_t http_gateway = http;
  http_gateway.userdata = &fake_http_gateway;
  gzc_client_config_t config_gateway = config;
  config_gateway.http = &http_gateway;
  config_gateway.webrtc = &webrtc_gateway;

  gzc_client_t *client_gateway = NULL;
  rc = gzc_client_create(&config_gateway, &client_gateway);
  if (rc == GZC_OK) {
    rc = gzc_client_set_webrtc_media(client_gateway, &media);
  }
  if (rc == GZC_OK) {
    rc = gzc_client_connect(client_gateway);
  }
  if (expect(rc == GZC_OK, "gateway client connects without authoritative ICE hook") != 0 ||
      expect(fake_http_gateway.post_count == 1, "gateway offer uses Edge endpoint") != 0 ||
      expect(fake_webrtc_gateway.ice_server_count == 0, "gateway ignores authoritative ICE servers") != 0) {
    gzc_client_destroy(client_gateway);
    gzc_buf_free(&fake_webrtc_gateway.sent, platform);
    gzc_buf_free(&fake_webrtc_gateway.outgoing, platform);
    gzc_buf_free(&fake_webrtc_gateway.native_sent, platform);
    gzc_buf_free(&fake_webrtc_gateway.opus_sent, platform);
    return 1;
  }
  rc = gzc_client_send_packet(
      client_gateway,
      GZC_PROTOCOL_OPUS_PACKET,
      (const uint8_t[]){0xf8, 0x55},
      2u);
  if (expect(
          rc == GZC_OK && fake_webrtc_gateway.opus_send_count == 1,
          "gateway client uses the mandatory Opus media extension") != 0) {
    gzc_client_destroy(client_gateway);
    gzc_buf_free(&fake_webrtc_gateway.sent, platform);
    gzc_buf_free(&fake_webrtc_gateway.outgoing, platform);
    gzc_buf_free(&fake_webrtc_gateway.native_sent, platform);
    gzc_buf_free(&fake_webrtc_gateway.opus_sent, platform);
    return 1;
  }
  fake_webrtc_gateway.defer_next_service_open = true;
  fake_webrtc_gateway.close_peer_on_poll = true;
  gzc_service_channel_t *closing_channel = NULL;
  rc = gzc_client_open_service_channel(
      client_gateway, 49u, 1000, &closing_channel);
  if (expect(
          rc == GZC_ERR_CLOSED && closing_channel == NULL &&
              fake_webrtc_gateway.stale_close_count == 0,
          "peer close while opening a service channel does not reuse the freed handle") != 0) {
    gzc_client_destroy(client_gateway);
    gzc_buf_free(&fake_webrtc_gateway.sent, platform);
    gzc_buf_free(&fake_webrtc_gateway.outgoing, platform);
    gzc_buf_free(&fake_webrtc_gateway.native_sent, platform);
    gzc_buf_free(&fake_webrtc_gateway.opus_sent, platform);
    return 1;
  }
  gzc_client_destroy(client_gateway);
  gzc_buf_free(&fake_webrtc_gateway.sent, platform);
  gzc_buf_free(&fake_webrtc_gateway.outgoing, platform);
  gzc_buf_free(&fake_webrtc_gateway.native_sent, platform);
  gzc_buf_free(&fake_webrtc_gateway.opus_sent, platform);
  return 0;
}
