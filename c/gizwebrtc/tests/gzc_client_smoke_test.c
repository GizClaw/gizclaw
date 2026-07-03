#include "gzc.h"
#include "gzc_rpc_generated.h"

#include <stdio.h>
#include <string.h>

struct gzc_rtc_peer {
  int unused;
};

struct gzc_rtc_channel {
  int unused;
};

typedef struct {
  gzc_webrtc_callbacks_t callbacks;
  struct gzc_rtc_peer peer;
  struct gzc_rtc_channel channel;
  gzc_buf_t sent;
  const gzc_platform_t *platform;
  int poll_count;
} fake_webrtc_t;

typedef struct {
  const gzc_platform_t *platform;
  int post_count;
} fake_http_t;

static int fake_peer_create(void *userdata, const gzc_webrtc_callbacks_t *callbacks, gzc_rtc_peer_t **out_peer) {
  fake_webrtc_t *fake = (fake_webrtc_t *)userdata;
  fake->callbacks = *callbacks;
  *out_peer = &fake->peer;
  return GZC_OK;
}

static void fake_channel_close(gzc_rtc_channel_t *channel) {
  (void)channel;
}

static void fake_peer_close(gzc_rtc_peer_t *peer) {
  (void)peer;
}

static fake_webrtc_t *global_fake_webrtc;

static int test_peer_create(void *userdata, const gzc_webrtc_callbacks_t *callbacks, gzc_rtc_peer_t **out_peer) {
  fake_webrtc_t *fake = (fake_webrtc_t *)userdata;
  global_fake_webrtc = fake;
  return fake_peer_create(userdata, callbacks, out_peer);
}

static int test_peer_start_offer(gzc_rtc_peer_t *peer) {
  fake_webrtc_t *fake = global_fake_webrtc;
  gzc_str_t offer = gzc_str_from_cstr("v=0\r\nfake-offer\r\n");
  fake->callbacks.on_local_sdp(fake->callbacks.userdata, peer, GZC_RTC_SDP_OFFER, offer);
  return GZC_OK;
}

static int test_peer_set_remote_sdp(gzc_rtc_peer_t *peer, gzc_rtc_sdp_type_t type, gzc_str_t sdp) {
  fake_webrtc_t *fake = global_fake_webrtc;
  (void)type;
  (void)sdp;
  gzc_rtc_channel_info_t info;
  memset(&info, 0, sizeof(info));
  info.label = gzc_str_from_cstr("rpc");
  info.stream_id = 1;
  info.ordered = true;
  info.reliable = true;
  fake->callbacks.on_channel_state(fake->callbacks.userdata, peer, &fake->channel, &info, GZC_RTC_CHANNEL_OPEN);
  return GZC_OK;
}

static int test_peer_create_data_channel(gzc_rtc_peer_t *peer, const gzc_rtc_channel_config_t *config, gzc_rtc_channel_t **out_channel) {
  (void)peer;
  if (config == NULL || config->label.data == NULL || strncmp(config->label.data, "rpc", config->label.len) != 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_channel = &global_fake_webrtc->channel;
  return GZC_OK;
}

static int test_peer_poll(gzc_rtc_peer_t *peer, int timeout_ms) {
  (void)peer;
  (void)timeout_ms;
  global_fake_webrtc->poll_count++;
  return GZC_OK;
}

static int test_channel_send(gzc_rtc_channel_t *channel, const uint8_t *data, size_t len, bool is_text) {
  fake_webrtc_t *fake = global_fake_webrtc;
  if (channel != &fake->channel || !is_text) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_reset(&fake->sent);
  int rc = gzc_buf_append(&fake->sent, fake->platform, data, len);
  if (rc != GZC_OK) {
    return rc;
  }
  const char *response = "{\"v\":1,\"id\":\"1\",\"result\":{\"server_time\":99}}";
  fake->callbacks.on_channel_message(
      fake->callbacks.userdata,
      &fake->peer,
      &fake->channel,
      NULL,
      (const uint8_t *)response,
      strlen(response),
      true);
  return GZC_OK;
}

static int test_http_post(void *userdata, const gzc_http_request_t *request, gzc_http_response_t *out_response) {
  fake_http_t *fake = (fake_http_t *)userdata;
  fake->post_count++;
  if (request == NULL || request->body == NULL || request->body_len == 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  out_response->status_code = 200;
  gzc_buf_init(&out_response->body);
  return gzc_buf_append_cstr(&out_response->body, fake->platform, "v=0\r\nfake-answer\r\n");
}

static void test_http_response_free(void *userdata, gzc_http_response_t *response) {
  fake_http_t *fake = (fake_http_t *)userdata;
  gzc_buf_free(&response->body, fake->platform);
}

static int expect(bool ok, const char *message) {
  if (!ok) {
    fprintf(stderr, "FAIL: %s\n", message);
    return 1;
  }
  return 0;
}

int main(void) {
  const gzc_platform_t *platform = gzc_default_platform();
  fake_webrtc_t fake_webrtc;
  memset(&fake_webrtc, 0, sizeof(fake_webrtc));
  fake_webrtc.platform = platform;
  gzc_buf_init(&fake_webrtc.sent);

  fake_http_t fake_http;
  memset(&fake_http, 0, sizeof(fake_http));
  fake_http.platform = platform;

  gzc_webrtc_vtable_t webrtc;
  memset(&webrtc, 0, sizeof(webrtc));
  webrtc.userdata = &fake_webrtc;
  webrtc.peer_create = test_peer_create;
  webrtc.peer_start_offer = test_peer_start_offer;
  webrtc.peer_set_remote_sdp = test_peer_set_remote_sdp;
  webrtc.peer_create_data_channel = test_peer_create_data_channel;
  webrtc.peer_poll = test_peer_poll;
  webrtc.channel_send = test_channel_send;
  webrtc.channel_close = fake_channel_close;
  webrtc.peer_close = fake_peer_close;

  gzc_http_vtable_t http;
  memset(&http, 0, sizeof(http));
  http.userdata = &fake_http;
  http.post = test_http_post;
  http.response_free = test_http_response_free;

  gzc_client_config_t config;
  memset(&config, 0, sizeof(config));
  config.signaling_url = gzc_str_from_cstr("https://example.invalid/signal");
  config.platform = platform;
  config.http = &http;
  config.webrtc = &webrtc;
  config.connect_timeout_ms = 1000;

  gzc_client_t *client = NULL;
  int rc = gzc_client_create(&config, &client);
  if (expect(rc == GZC_OK, "client create") != 0) {
    return 1;
  }
  rc = gzc_client_connect(client);
  if (expect(rc == GZC_OK, "client connect") != 0) {
    return 1;
  }
  if (expect(fake_http.post_count == 1, "http post called once") != 0) {
    return 1;
  }

  gzc_ping_request_t ping;
  memset(&ping, 0, sizeof(ping));
  ping.client_send_time = 42;
  gzc_buf_t params;
  gzc_buf_init(&params);
  rc = gzc_ping_request_encode_json(platform, &ping, &params);
  if (expect(rc == GZC_OK, "encode ping request") != 0) {
    return 1;
  }
  gzc_rpc_response_t response;
  rc = gzc_rpc_call_json(client, gzc_str_from_cstr(GZC_RPC_METHOD_ALL_PING), gzc_str_from_parts((const char *)params.data, params.len), &response);
  if (expect(rc == GZC_OK, "rpc call json") != 0) {
    return 1;
  }
  if (expect(response.result_json.len > 0, "rpc call captured result json") != 0) {
    return 1;
  }
  if (expect(fake_webrtc.sent.len > 0, "channel send captured payload") != 0) {
    return 1;
  }
  gzc_str_t method_raw;
  rc = gzc_json_find_field(gzc_str_from_parts((const char *)fake_webrtc.sent.data, fake_webrtc.sent.len), "method", &method_raw);
  if (expect(rc == GZC_OK, "request method field") != 0) {
    return 1;
  }
  gzc_str_t method;
  rc = gzc_json_parse_string(method_raw, &method);
  if (expect(rc == GZC_OK && method.len == strlen(GZC_RPC_METHOD_ALL_PING) &&
                 strncmp(method.data, GZC_RPC_METHOD_ALL_PING, method.len) == 0,
             "request method value") != 0) {
    return 1;
  }

  gzc_ping_response_t decoded;
  rc = gzc_ping_response_decode_json(gzc_str_from_cstr("{\"server_time\":99}"), &decoded);
  if (expect(rc == GZC_OK && decoded.server_time == 99, "decode ping response") != 0) {
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

  gzc_buf_free(&params, platform);
  gzc_buf_free(&fake_webrtc.sent, platform);
  gzc_client_destroy(client);
  return 0;
}
