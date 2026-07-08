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

typedef enum {
  FAKE_RESPONSE_JSON = 0,
  FAKE_RESPONSE_BINARY_STREAM = 1
} fake_response_mode_t;

typedef struct {
  gzc_webrtc_callbacks_t callbacks;
  struct gzc_rtc_peer peer;
  struct gzc_rtc_channel packet_channel;
  struct gzc_rtc_channel rpc_channel;
  gzc_buf_t sent;
  const gzc_platform_t *platform;
  int poll_count;
  int create_channel_count;
  fake_response_mode_t response_mode;
} fake_webrtc_t;

typedef struct {
  const gzc_platform_t *platform;
  const char *server_info_body;
  int get_count;
  int post_count;
} fake_http_t;

typedef struct {
  const gzc_platform_t *platform;
} fake_crypto_t;

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
  info.label = gzc_str_from_cstr("giznet/v1/packet");
  info.stream_id = 0;
  info.ordered = false;
  info.reliable = false;
  fake->callbacks.on_channel_state(fake->callbacks.userdata, peer, &fake->packet_channel, &info, GZC_RTC_CHANNEL_OPEN);
  memset(&info, 0, sizeof(info));
  info.label = gzc_str_from_cstr("giznet/v1/service/0");
  info.stream_id = 1;
  info.ordered = true;
  info.reliable = true;
  fake->callbacks.on_channel_state(fake->callbacks.userdata, peer, &fake->rpc_channel, &info, GZC_RTC_CHANNEL_OPEN);
  return GZC_OK;
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
      fake->callbacks.on_channel_state(fake->callbacks.userdata, peer, &fake->packet_channel, &info, GZC_RTC_CHANNEL_OPEN);
    }
  } else if (config->label.len == strlen("giznet/v1/service/0") &&
             strncmp(config->label.data, "giznet/v1/service/0", config->label.len) == 0) {
    if (!config->ordered || !config->reliable) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    *out_channel = &fake->rpc_channel;
    if (fake->callbacks.on_channel_state != NULL) {
      gzc_rtc_channel_info_t info;
      memset(&info, 0, sizeof(info));
      info.label = gzc_str_from_cstr("giznet/v1/service/0");
      info.stream_id = 1;
      info.ordered = true;
      info.reliable = true;
      fake->callbacks.on_channel_state(fake->callbacks.userdata, peer, &fake->rpc_channel, &info, GZC_RTC_CHANNEL_OPEN);
    }
  } else {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  fake->create_channel_count++;
  return GZC_OK;
}

static int test_peer_poll(gzc_rtc_peer_t *peer, int timeout_ms) {
  (void)peer;
  (void)timeout_ms;
  global_fake_webrtc->poll_count++;
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

static size_t first_frame_size(const gzc_buf_t *bytes) {
  if (bytes == NULL || bytes->len < 4) {
    return 0;
  }
  return 4 + ((size_t)bytes->data[0] | ((size_t)bytes->data[1] << 8));
}

static int test_channel_send(gzc_rtc_channel_t *channel, const uint8_t *data, size_t len, bool is_text) {
  fake_webrtc_t *fake = global_fake_webrtc;
  if (channel == &fake->packet_channel && !is_text) {
    gzc_buf_reset(&fake->sent);
    return gzc_buf_append(&fake->sent, fake->platform, data, len);
  }
  if (channel != &fake->rpc_channel || is_text) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_reset(&fake->sent);
  int rc = gzc_buf_append(&fake->sent, fake->platform, data, len);
  if (rc != GZC_OK) {
    return rc;
  }
  if (fake->response_mode == FAKE_RESPONSE_BINARY_STREAM) {
    const uint8_t first[] = {0x01, 0x02};
    const uint8_t second[] = {0x03};
    gzc_buf_t framed;
    gzc_buf_init(&framed);
    rc = append_test_frame(fake->platform, &framed, GZC_RPC_FRAME_BINARY, first, sizeof(first));
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
          &fake->rpc_channel,
          NULL,
          framed.data,
          framed.len,
          false);
    }
    gzc_buf_free(&framed, fake->platform);
    return rc;
  }
  const char *response = "{\"v\":1,\"id\":\"1\",\"result\":{\"server_time\":99}}";
  gzc_buf_t framed;
  gzc_buf_init(&framed);
  gzc_rpc_frame_t json_frame;
  memset(&json_frame, 0, sizeof(json_frame));
  json_frame.type = GZC_RPC_FRAME_JSON;
  json_frame.data = (const uint8_t *)response;
  json_frame.len = strlen(response);
  rc = gzc_rpc_frame_encode(fake->platform, &json_frame, &framed);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_rpc_frame_t eos_frame;
  memset(&eos_frame, 0, sizeof(eos_frame));
  eos_frame.type = GZC_RPC_FRAME_EOS;
  rc = gzc_rpc_frame_encode(fake->platform, &eos_frame, &framed);
  if (rc != GZC_OK) {
    gzc_buf_free(&framed, fake->platform);
    return rc;
  }
  fake->callbacks.on_channel_message(
      fake->callbacks.userdata,
      &fake->peer,
      &fake->rpc_channel,
      NULL,
      framed.data,
      framed.len,
      false);
  gzc_buf_free(&framed, fake->platform);
  return GZC_OK;
}

typedef struct {
  size_t frame_count;
  size_t binary_bytes;
} stream_count_t;

static int count_stream_frame(void *userdata, const gzc_rpc_frame_t *frame) {
  stream_count_t *count = (stream_count_t *)userdata;
  if (count == NULL || frame == NULL || frame->type != GZC_RPC_FRAME_BINARY) {
    return GZC_ERR_RPC;
  }
  count->frame_count++;
  count->binary_bytes += frame->len;
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
                           ? "{\"protocol\":\"gizclaw-webrtc\",\"public_key\":\"8mfzTdZB1JA43QmNAMWfTfkj5GC9TJxJFveThi9tvK6J\",\"signaling_path\":\"/custom/offer\"}"
                           : fake->server_info_body;
    return gzc_buf_append_cstr(&out_response->body, fake->platform, body);
  }
  fake->post_count++;
  if (request->method != GZC_HTTP_METHOD_POST ||
      !str_eq_cstr(request->url, "http://example.invalid:9820/custom/offer") ||
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

int main(void) {
  const gzc_platform_t *platform = gzc_default_platform();
  fake_webrtc_t fake_webrtc;
  memset(&fake_webrtc, 0, sizeof(fake_webrtc));
  fake_webrtc.platform = platform;
  gzc_buf_init(&fake_webrtc.sent);

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
  webrtc.channel_close = fake_channel_close;
  webrtc.peer_close = fake_peer_close;

  gzc_http_vtable_t http;
  memset(&http, 0, sizeof(http));
  http.userdata = &fake_http;
  http.request = test_http_request;
  http.response_free = test_http_response_free;

  gzc_client_config_t config;
  memset(&config, 0, sizeof(config));
  config.server_endpoint = gzc_str_from_cstr("example.invalid:9820");
  config.private_key = gzc_str_from_cstr("7gyGAp71YXQRoxmFBaHxofQXAipvgHyBKPyxmdSJxyvz");
  config.platform = platform;
  config.crypto = &crypto;
  config.http = &http;
  config.webrtc = &webrtc;
  config.cipher_mode = GZC_CIPHER_PLAINTEXT;
  config.connect_timeout_ms = 1000;

  gzc_client_t *client = NULL;
  rc = gzc_client_create(&config, &client);
  if (expect(rc == GZC_OK, "client create") != 0) {
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
  if (expect(fake_webrtc.create_channel_count == 2, "packet and rpc channels created during connect") != 0) {
    return 1;
  }

  gzc_json_t malformed_json = {gzc_str_from_cstr("{\"public_key\":\"x\",}")};
  if (expect(gzc_json_validate_object(malformed_json.raw) == GZC_ERR_JSON, "malformed object rejected") != 0) {
    return 1;
  }
  malformed_json.raw = gzc_str_from_cstr("{\"value\":-}");
  if (expect(gzc_json_validate_object(malformed_json.raw) == GZC_ERR_JSON, "malformed number rejected") != 0) {
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
  gzc_rpc_frame_t sent_frame;
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, first_frame_size(&fake_webrtc.sent), &sent_frame);
  if (expect(rc == GZC_OK && sent_frame.type == GZC_RPC_FRAME_JSON, "request json frame") != 0) {
    return 1;
  }
  gzc_str_t method_raw;
  rc = gzc_json_find_field(gzc_str_from_parts((const char *)sent_frame.data, sent_frame.len), "method", &method_raw);
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

  fake_webrtc.response_mode = FAKE_RESPONSE_BINARY_STREAM;
  stream_count_t stream_count;
  memset(&stream_count, 0, sizeof(stream_count));
  rc = gzc_rpc_call_stream(
      client,
      gzc_str_from_cstr(GZC_RPC_METHOD_ALL_SPEED_TEST_RUN),
      gzc_str_from_parts((const char *)params.data, params.len),
      count_stream_frame,
      &stream_count);
  if (expect(rc == GZC_OK, "rpc call stream") != 0) {
    return 1;
  }
  if (expect(stream_count.frame_count == 2 && stream_count.binary_bytes == 3, "stream binary frames counted") != 0) {
    return 1;
  }
  fake_webrtc.response_mode = FAKE_RESPONSE_JSON;

  const uint8_t telemetry_payload[] = {0x01, 0x02, 0x03};
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
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "reject received legacy reserved packet protocol") != 0) {
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
  rc = gzc_rpc_call_json(
      client,
      gzc_str_from_cstr(GZC_RPC_METHOD_ALL_PING),
      gzc_str_from_parts((const char *)large_params.data, large_params.len),
      &response);
  if (expect(rc == GZC_OK, "rpc call json with large params") != 0) {
    return 1;
  }
  rc = gzc_rpc_frame_decode(fake_webrtc.sent.data, first_frame_size(&fake_webrtc.sent), &sent_frame);
  if (expect(rc == GZC_OK && sent_frame.type == GZC_RPC_FRAME_TEXT, "large request starts with text frame") != 0) {
    return 1;
  }
  gzc_buf_free(&large_params, platform);

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

  gzc_buf_free(&params, platform);
  gzc_buf_free(&fake_webrtc.sent, platform);
  gzc_client_destroy(client);
  return 0;
}
