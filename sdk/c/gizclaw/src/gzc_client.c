#include "gzc_client.h"

#include "gzc_json.h"
#include "gzc_rpc_frame.h"

#include <stdio.h>
#include <string.h>

typedef struct gzc_rpc_inbound gzc_rpc_inbound_t;
int gzc_rpc_inbound_create(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    gzc_rpc_inbound_t **out_inbound);
int gzc_rpc_inbound_feed(gzc_rpc_inbound_t *inbound, const uint8_t *data, size_t len, bool is_text);
int gzc_rpc_inbound_poll(gzc_rpc_inbound_t *inbound);
bool gzc_rpc_inbound_has_pending_output(gzc_rpc_inbound_t *inbound);
bool gzc_rpc_inbound_close_requested(gzc_rpc_inbound_t *inbound);
void gzc_rpc_inbound_destroy(gzc_rpc_inbound_t *inbound);

struct gzc_service_channel {
  gzc_client_t *client;
  gzc_rtc_channel_t *rtc;
  gzc_buf_t rx;
  uint64_t service;
  bool open;
  bool closed;
};

struct gzc_client {
  gzc_client_config_t config;
  gzc_peer_add_ice_server_fn peer_add_ice_server;
  gzc_rtc_peer_t *peer;
  gzc_rtc_channel_t *packet_channel;
  gzc_rtc_channel_t *rpc_channel;
  gzc_service_channel_t *service_channel;
  gzc_buf_t local_sdp;
  gzc_buf_t packet_rx;
  gzc_buf_t rpc_rx;
  gzc_buf_t rpc_response;
  gzc_rpc_inbound_t *inbound[GZC_RPC_MAX_INBOUND_CHANNELS];
  gzc_rtc_channel_t *inbound_channels[GZC_RPC_MAX_INBOUND_CHANNELS];
  int dispatch_error;
  bool has_local_sdp;
  bool packet_channel_open;
  bool rpc_channel_open;
  bool closed;
};

enum {
  gzc_protocol_stamped_opus_packet = 0x10,
  gzc_protocol_custom_start = 0x40,
};

static int64_t now_ms(gzc_client_t *client) {
  if (client->config.platform != NULL && client->config.platform->time_unix_ms != NULL) {
    return client->config.platform->time_unix_ms(client->config.platform->userdata);
  }
  return 0;
}

static int copy_str(gzc_client_t *client, gzc_str_t src, gzc_buf_t *dst) {
  gzc_buf_reset(dst);
  return gzc_buf_append(dst, client->config.platform, src.data, src.len);
}

static bool str_empty(gzc_str_t value) {
  return value.data == NULL || value.len == 0;
}

static bool str_eq_cstr(gzc_str_t value, const char *want) {
  size_t want_len = strlen(want);
  return value.data != NULL && value.len == want_len && strncmp(value.data, want, want_len) == 0;
}

static bool str_has_cstr_prefix(gzc_str_t value, const char *prefix) {
  size_t prefix_len = strlen(prefix);
  return value.len > prefix_len && strncmp(value.data, prefix, prefix_len) == 0;
}

static bool valid_ice_url(gzc_str_t value) {
  return str_has_cstr_prefix(value, "stun:") || str_has_cstr_prefix(value, "stuns:") ||
         str_has_cstr_prefix(value, "turn:") || str_has_cstr_prefix(value, "turns:");
}

static bool valid_packet_protocol(uint8_t protocol) {
  return protocol == gzc_protocol_stamped_opus_packet || protocol >= gzc_protocol_custom_start;
}

static int build_endpoint_url(gzc_client_t *client, gzc_str_t path, gzc_buf_t *out_url) {
  if (client == NULL || out_url == NULL || str_empty(client->config.server_endpoint) ||
      str_empty(path) || path.data[0] != '/' || (path.len >= 2 && path.data[1] == '/')) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_reset(out_url);
  int rc = gzc_buf_append_cstr(out_url, client->config.platform, "http://");
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_buf_append_str(out_url, client->config.platform, client->config.server_endpoint);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append_str(out_url, client->config.platform, path);
}

static void free_http_response(gzc_client_t *client, gzc_http_response_t *response) {
  if (client->config.http->response_free != NULL) {
    client->config.http->response_free(client->config.http->userdata, response);
  } else {
    gzc_buf_free(&response->body, client->config.platform);
  }
}

static int hex_value(char ch) {
  if (ch >= '0' && ch <= '9') {
    return ch - '0';
  }
  if (ch >= 'a' && ch <= 'f') {
    return ch - 'a' + 10;
  }
  if (ch >= 'A' && ch <= 'F') {
    return ch - 'A' + 10;
  }
  return -1;
}

static int append_utf8(gzc_client_t *client, gzc_buf_t *buf, uint32_t codepoint) {
  uint8_t bytes[4];
  size_t len = 0;
  if (codepoint <= 0x7f) {
    bytes[0] = (uint8_t)codepoint;
    len = 1;
  } else if (codepoint <= 0x7ff) {
    bytes[0] = 0xc0 | (uint8_t)(codepoint >> 6);
    bytes[1] = 0x80 | (uint8_t)(codepoint & 0x3f);
    len = 2;
  } else {
    bytes[0] = 0xe0 | (uint8_t)(codepoint >> 12);
    bytes[1] = 0x80 | (uint8_t)((codepoint >> 6) & 0x3f);
    bytes[2] = 0x80 | (uint8_t)(codepoint & 0x3f);
    len = 3;
  }
  return gzc_buf_append(buf, client->config.platform, bytes, len);
}

static int parse_json_string_for_ice(gzc_client_t *client, gzc_str_t raw_json, gzc_buf_t *scratch, gzc_str_t *out) {
  int rc = gzc_json_parse_string(raw_json, out);
  if (rc == GZC_OK) {
    return GZC_OK;
  }
  if (rc != GZC_ERR_UNSUPPORTED || raw_json.len < 2 || raw_json.data[0] != '"' ||
      raw_json.data[raw_json.len - 1] != '"') {
    return rc;
  }
  gzc_buf_reset(scratch);
  for (size_t i = 1; i + 1 < raw_json.len; i++) {
    unsigned char ch = (unsigned char)raw_json.data[i];
    if (ch != '\\') {
      if (ch < 0x20) {
        return GZC_ERR_JSON;
      }
      rc = gzc_buf_append(scratch, client->config.platform, &ch, 1);
      if (rc != GZC_OK) {
        return rc;
      }
      continue;
    }
    i++;
    if (i + 1 >= raw_json.len) {
      return GZC_ERR_JSON;
    }
    char escaped = raw_json.data[i];
    switch (escaped) {
    case '"':
    case '\\':
    case '/':
      rc = gzc_buf_append(scratch, client->config.platform, &escaped, 1);
      break;
    case 'b': {
      const char value = '\b';
      rc = gzc_buf_append(scratch, client->config.platform, &value, 1);
      break;
    }
    case 'f': {
      const char value = '\f';
      rc = gzc_buf_append(scratch, client->config.platform, &value, 1);
      break;
    }
    case 'n': {
      const char value = '\n';
      rc = gzc_buf_append(scratch, client->config.platform, &value, 1);
      break;
    }
    case 'r': {
      const char value = '\r';
      rc = gzc_buf_append(scratch, client->config.platform, &value, 1);
      break;
    }
    case 't': {
      const char value = '\t';
      rc = gzc_buf_append(scratch, client->config.platform, &value, 1);
      break;
    }
    case 'u': {
      if (i + 4 >= raw_json.len) {
        return GZC_ERR_JSON;
      }
      uint32_t codepoint = 0;
      for (size_t j = 0; j < 4; j++) {
        int value = hex_value(raw_json.data[i + 1 + j]);
        if (value < 0) {
          return GZC_ERR_JSON;
        }
        codepoint = (codepoint << 4) | (uint32_t)value;
      }
      i += 4;
      if (codepoint >= 0xd800 && codepoint <= 0xdfff) {
        return GZC_ERR_UNSUPPORTED;
      }
      rc = append_utf8(client, scratch, codepoint);
      break;
    }
    default:
      return GZC_ERR_JSON;
    }
    if (rc != GZC_OK) {
      return rc;
    }
  }
  out->data = (const char *)scratch->data;
  out->len = scratch->len;
  return GZC_OK;
}

static int apply_ice_servers(gzc_client_t *client, gzc_str_t body) {
  gzc_str_t servers_raw;
  int rc = gzc_json_find_field(body, "ice_servers", &servers_raw);
  if (rc != GZC_OK) {
    return GZC_OK;
  }
  gzc_json_array_iter_t servers;
  rc = gzc_json_array_iter_init(servers_raw, &servers);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_buf_t username_buf;
  gzc_buf_t credential_buf;
  gzc_buf_t url_buf;
  gzc_buf_init(&username_buf);
  gzc_buf_init(&credential_buf);
  gzc_buf_init(&url_buf);
  int result = GZC_OK;
  while (true) {
    gzc_str_t server_raw;
    bool has_server = false;
    rc = gzc_json_array_iter_next(&servers, &server_raw, &has_server);
    if (rc != GZC_OK || !has_server) {
      result = rc;
      break;
    }
    if (gzc_json_validate_object(server_raw) != GZC_OK) {
      result = GZC_ERR_JSON;
      break;
    }
    gzc_str_t username = {0};
    gzc_str_t credential = {0};
    gzc_str_t field_raw;
    if (gzc_json_find_field(server_raw, "username", &field_raw) == GZC_OK) {
      rc = parse_json_string_for_ice(client, field_raw, &username_buf, &username);
      if (rc != GZC_OK) {
        result = rc;
        break;
      }
    }
    if (gzc_json_find_field(server_raw, "credential", &field_raw) == GZC_OK) {
      rc = parse_json_string_for_ice(client, field_raw, &credential_buf, &credential);
      if (rc != GZC_OK) {
        result = rc;
        break;
      }
    }
    gzc_str_t urls_raw;
    if (gzc_json_find_field(server_raw, "urls", &urls_raw) != GZC_OK) {
      result = GZC_ERR_JSON;
      break;
    }
    gzc_json_array_iter_t urls;
    rc = gzc_json_array_iter_init(urls_raw, &urls);
    if (rc != GZC_OK) {
      result = rc;
      break;
    }
    bool applied_url = false;
    while (true) {
      gzc_str_t url_raw;
      bool has_url = false;
      rc = gzc_json_array_iter_next(&urls, &url_raw, &has_url);
      if (rc != GZC_OK) {
        result = rc;
        break;
      }
      if (!has_url) {
        break;
      }
      gzc_str_t url;
      rc = parse_json_string_for_ice(client, url_raw, &url_buf, &url);
      if (rc != GZC_OK || !valid_ice_url(url)) {
        result = rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
        break;
      }
      if (client->peer_add_ice_server != NULL) {
        rc = client->peer_add_ice_server(client->peer, url, username, credential);
        if (rc != GZC_OK) {
          result = rc;
          break;
        }
      }
      applied_url = true;
    }
    if (result != GZC_OK) {
      break;
    }
    if (!applied_url) {
      result = GZC_ERR_INVALID_ARGUMENT;
      break;
    }
  }
  gzc_buf_free(&username_buf, client->config.platform);
  gzc_buf_free(&credential_buf, client->config.platform);
  gzc_buf_free(&url_buf, client->config.platform);
  return result;
}

static int load_server_info(gzc_client_t *client, int timeout_ms, gzc_signaling_config_t *signaling, gzc_buf_t *signaling_url) {
  if (client == NULL || signaling == NULL || signaling_url == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (str_empty(client->config.server_endpoint)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }

  gzc_buf_t server_info_url;
  gzc_buf_init(&server_info_url);
  int rc = build_endpoint_url(client, gzc_str_from_cstr("/server-info"), &server_info_url);
  if (rc != GZC_OK) {
    gzc_buf_free(&server_info_url, client->config.platform);
    return rc;
  }

  gzc_http_request_t request;
  memset(&request, 0, sizeof(request));
  request.method = GZC_HTTP_METHOD_GET;
  request.url = gzc_str_from_parts((const char *)server_info_url.data, server_info_url.len);
  request.timeout_ms = timeout_ms;

  gzc_http_response_t response;
  memset(&response, 0, sizeof(response));
  gzc_buf_init(&response.body);
  rc = client->config.http->request(client->config.http->userdata, &request, &response);
  gzc_buf_free(&server_info_url, client->config.platform);
  if (rc != GZC_OK) {
    free_http_response(client, &response);
    return rc;
  }
  if (gzc_http_status_has_error(response.status_code)) {
    free_http_response(client, &response);
    return GZC_ERR_HTTP;
  }

  gzc_str_t body = gzc_str_from_parts((const char *)response.body.data, response.body.len);
  rc = gzc_json_validate_object(body);
  if (rc != GZC_OK) {
    free_http_response(client, &response);
    return rc;
  }
  gzc_str_t raw;
  rc = gzc_json_find_field(body, "public_key", &raw);
  if (rc == GZC_OK) {
    gzc_str_t public_key;
    rc = gzc_json_parse_string(raw, &public_key);
    if (rc == GZC_OK) {
      rc = gzc_key_from_text(public_key, &signaling->remote_public_key);
    }
    if (rc == GZC_OK && gzc_key_is_zero(&signaling->remote_public_key)) {
      rc = GZC_ERR_INVALID_ARGUMENT;
    }
  }
  if (rc != GZC_OK) {
    free_http_response(client, &response);
    return rc;
  }

  rc = gzc_json_find_field(body, "protocol", &raw);
  if (rc == GZC_OK) {
    gzc_str_t protocol;
    rc = gzc_json_parse_string(raw, &protocol);
    if (rc != GZC_OK || !str_eq_cstr(protocol, "gizclaw-webrtc")) {
      free_http_response(client, &response);
      return rc == GZC_OK ? GZC_ERR_UNSUPPORTED : rc;
    }
  }

  rc = apply_ice_servers(client, body);
  if (rc != GZC_OK) {
    free_http_response(client, &response);
    return rc;
  }

  gzc_str_t signaling_path = gzc_str_from_cstr(GZC_SIGNALING_PATH);
  rc = gzc_json_find_field(body, "signaling_path", &raw);
  if (rc == GZC_OK) {
    rc = gzc_json_parse_string(raw, &signaling_path);
    if (rc != GZC_OK) {
      free_http_response(client, &response);
      return rc;
    }
  }
  rc = build_endpoint_url(client, signaling_path, signaling_url);
  free_http_response(client, &response);
  if (rc != GZC_OK) {
    return rc;
  }
  signaling->signaling_url = gzc_str_from_parts((const char *)signaling_url->data, signaling_url->len);
  return GZC_OK;
}

static int append_framed_rx(gzc_buf_t *rx, const gzc_platform_t *platform, const uint8_t *data, size_t len) {
  if (rx == NULL || (data == NULL && len != 0) || len > GZC_RPC_MAX_FRAME_SIZE) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  uint8_t header[2];
  header[0] = (uint8_t)(len & 0xffu);
  header[1] = (uint8_t)((len >> 8) & 0xffu);
  int rc = gzc_buf_append(rx, platform, header, sizeof(header));
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append(rx, platform, data, len);
}

static void on_peer_state(void *userdata, gzc_rtc_peer_t *peer, gzc_rtc_peer_state_t state) {
  (void)peer;
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL) {
    return;
  }
  if (state == GZC_RTC_PEER_FAILED || state == GZC_RTC_PEER_CLOSED) {
    client->closed = true;
  }
}

static void on_local_sdp(void *userdata, gzc_rtc_peer_t *peer, gzc_rtc_sdp_type_t type, gzc_str_t sdp) {
  (void)peer;
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL || type != GZC_RTC_SDP_OFFER) {
    return;
  }
  if (copy_str(client, sdp, &client->local_sdp) == GZC_OK) {
    client->has_local_sdp = true;
  }
}

static void on_channel_state(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info,
    gzc_rtc_channel_state_t state) {
  (void)peer;
  (void)info;
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL || channel == NULL) {
    return;
  }
  bool *open_flag = NULL;
  if (channel == client->packet_channel) {
    open_flag = &client->packet_channel_open;
  } else if (channel == client->rpc_channel) {
    open_flag = &client->rpc_channel_open;
  } else if (client->service_channel != NULL && channel == client->service_channel->rtc) {
    open_flag = &client->service_channel->open;
  }
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    if (channel == client->inbound_channels[i]) {
      if (state == GZC_RTC_CHANNEL_CLOSED || state == GZC_RTC_CHANNEL_ERROR) {
        gzc_rpc_inbound_destroy(client->inbound[i]);
        client->inbound[i] = NULL;
        client->inbound_channels[i] = NULL;
      }
      return;
    }
  }
  if (open_flag == NULL) {
    return;
  }
  if (state == GZC_RTC_CHANNEL_OPEN) {
    *open_flag = true;
  } else if (state == GZC_RTC_CHANNEL_CLOSED || state == GZC_RTC_CHANNEL_ERROR) {
    *open_flag = false;
    if (client->service_channel != NULL && channel == client->service_channel->rtc) {
      client->service_channel->closed = true;
    }
  }
}

static void on_remote_channel(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info) {
  (void)peer;
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL || channel == NULL || info == NULL) {
    return;
  }
  if (client->closed) {
    if (client->config.webrtc != NULL && client->config.webrtc->channel_close != NULL) {
      client->config.webrtc->channel_close(channel);
    }
    return;
  }
  if (!str_eq_cstr(info->label, "giznet/v1/service/0") || !info->ordered || !info->reliable) {
    if (client->config.webrtc->channel_close != NULL) {
      client->config.webrtc->channel_close(channel);
    }
    return;
  }
  size_t slot = GZC_RPC_MAX_INBOUND_CHANNELS;
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    if (client->inbound[i] == NULL) {
      slot = i;
      break;
    }
  }
  if (slot == GZC_RPC_MAX_INBOUND_CHANNELS) {
    if (client->config.webrtc->channel_close != NULL) {
      client->config.webrtc->channel_close(channel);
    }
    return;
  }
  gzc_rpc_inbound_t *inbound = NULL;
  int rc = gzc_rpc_inbound_create(client, channel, &inbound);
  if (rc != GZC_OK) {
    client->dispatch_error = rc;
    if (client->config.webrtc->channel_close != NULL) {
      client->config.webrtc->channel_close(channel);
    }
    return;
  }
  client->inbound[slot] = inbound;
  client->inbound_channels[slot] = channel;
}

static void on_channel_message(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info,
    const uint8_t *data,
    size_t len,
    bool is_text) {
  (void)peer;
  (void)info;
  gzc_client_t *client = (gzc_client_t *)userdata;
  (void)is_text;
  if (client == NULL || channel == NULL || client->closed) {
    return;
  }
  if (channel == client->rpc_channel) {
    (void)gzc_buf_append(&client->rpc_rx, client->config.platform, data, len);
    return;
  }
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    if (channel == client->inbound_channels[i] && client->inbound[i] != NULL) {
      gzc_rpc_inbound_t *inbound = client->inbound[i];
      int rc = gzc_rpc_inbound_feed(inbound, data, len, is_text);
      if (gzc_rpc_inbound_close_requested(inbound)) {
        client->inbound[i] = NULL;
        client->inbound_channels[i] = NULL;
        if (client->config.webrtc->channel_close != NULL) {
          client->config.webrtc->channel_close(channel);
        }
        gzc_rpc_inbound_destroy(inbound);
      }
      if (rc != GZC_OK) {
        client->dispatch_error = rc;
      }
      return;
    }
  }
  if (channel == client->packet_channel) {
    (void)append_framed_rx(&client->packet_rx, client->config.platform, data, len);
    return;
  }
  if (client->service_channel != NULL && channel == client->service_channel->rtc) {
    (void)gzc_buf_append(&client->service_channel->rx, client->config.platform, data, len);
    return;
  }
}

static int wait_until(gzc_client_t *client, bool *flag, int timeout_ms) {
  const int64_t start = now_ms(client);
  while (!*flag) {
    if (client->closed) {
      return GZC_ERR_CLOSED;
    }
    int rc = gzc_client_poll(client, 10);
    if (rc != GZC_OK) {
      return rc;
    }
    if (timeout_ms >= 0 && now_ms(client) - start >= timeout_ms) {
      return GZC_ERR_TIMEOUT;
    }
  }
  return GZC_OK;
}

static int open_rpc_channel(gzc_client_t *client, int timeout_ms) {
  if (client == NULL || client->peer == NULL || client->config.webrtc == NULL ||
      client->config.webrtc->peer_create_data_channel == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->rpc_channel != NULL && client->rpc_channel_open) {
    return GZC_OK;
  }
  if (client->rpc_channel != NULL) {
    return wait_until(client, &client->rpc_channel_open, timeout_ms);
  }
  client->rpc_channel = NULL;
  client->rpc_channel_open = false;
  gzc_buf_reset(&client->rpc_rx);
  gzc_buf_reset(&client->rpc_response);

  gzc_rtc_channel_config_t rpc_cfg;
  memset(&rpc_cfg, 0, sizeof(rpc_cfg));
  rpc_cfg.label = gzc_str_from_cstr("giznet/v1/service/0");
  rpc_cfg.ordered = true;
  rpc_cfg.reliable = true;
  int rc = client->config.webrtc->peer_create_data_channel(client->peer, &rpc_cfg, &client->rpc_channel);
  if (rc != GZC_OK) {
    client->rpc_channel = NULL;
    return rc;
  }
  return wait_until(client, &client->rpc_channel_open, timeout_ms);
}

static void close_rpc_channel(gzc_client_t *client) {
  if (client == NULL) {
    return;
  }
  if (client->rpc_channel != NULL && client->config.webrtc != NULL && client->config.webrtc->channel_close != NULL) {
    client->config.webrtc->channel_close(client->rpc_channel);
  }
  client->rpc_channel = NULL;
  client->rpc_channel_open = false;
  gzc_buf_reset(&client->rpc_rx);
}

int gzc_client_create(const gzc_client_config_t *config, gzc_client_t **out_client) {
  if (config == NULL || out_client == NULL || config->http == NULL || config->webrtc == NULL ||
      config->crypto == NULL ||
      config->webrtc->peer_create == NULL || config->webrtc->peer_start_offer == NULL ||
      config->webrtc->peer_set_remote_sdp == NULL || config->webrtc->peer_create_data_channel == NULL ||
      config->webrtc->channel_send == NULL || config->webrtc->peer_close == NULL ||
      config->http->request == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = config->platform == NULL ? gzc_default_platform() : config->platform;
  if (platform->malloc == NULL || platform->realloc == NULL || platform->free == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_client_t *client = (gzc_client_t *)platform->malloc(platform->userdata, sizeof(*client));
  if (client == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(client, 0, sizeof(*client));
  client->config = *config;
  client->config.platform = platform;
  gzc_buf_init(&client->local_sdp);
  gzc_buf_init(&client->packet_rx);
  gzc_buf_init(&client->rpc_rx);
  gzc_buf_init(&client->rpc_response);
  *out_client = client;
  return GZC_OK;
}

int gzc_client_set_peer_add_ice_server(gzc_client_t *client, gzc_peer_add_ice_server_fn fn) {
  if (client == NULL || client->peer != NULL || client->packet_channel != NULL || client->rpc_channel != NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->peer_add_ice_server = fn;
  return GZC_OK;
}

int gzc_client_connect(gzc_client_t *client) {
  if (client == NULL || client->closed) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->peer != NULL || client->packet_channel != NULL || client->rpc_channel != NULL ||
      client->service_channel != NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->has_local_sdp = false;
  client->packet_channel_open = false;
  client->rpc_channel_open = false;
  gzc_buf_reset(&client->local_sdp);
  gzc_buf_reset(&client->packet_rx);
  gzc_buf_reset(&client->rpc_rx);
  gzc_buf_reset(&client->rpc_response);
  gzc_webrtc_callbacks_t callbacks;
  memset(&callbacks, 0, sizeof(callbacks));
  callbacks.userdata = client;
  callbacks.on_peer_state = on_peer_state;
  callbacks.on_local_sdp = on_local_sdp;
  callbacks.on_channel_state = on_channel_state;
  callbacks.on_channel_message = on_channel_message;
  callbacks.on_remote_channel = on_remote_channel;

  int rc = client->config.webrtc->peer_create(client->config.webrtc->userdata, &callbacks, &client->peer);
  if (rc != GZC_OK) {
    goto fail;
  }

  gzc_rtc_channel_config_t packet_cfg;
  memset(&packet_cfg, 0, sizeof(packet_cfg));
  packet_cfg.label = gzc_str_from_cstr("giznet/v1/packet");
  packet_cfg.ordered = false;
  packet_cfg.reliable = false;
  rc = client->config.webrtc->peer_create_data_channel(client->peer, &packet_cfg, &client->packet_channel);
  if (rc != GZC_OK) {
    goto fail;
  }

  gzc_rtc_channel_config_t rpc_cfg;
  memset(&rpc_cfg, 0, sizeof(rpc_cfg));
  rpc_cfg.label = gzc_str_from_cstr("giznet/v1/service/0");
  rpc_cfg.ordered = true;
  rpc_cfg.reliable = true;
  rc = client->config.webrtc->peer_create_data_channel(client->peer, &rpc_cfg, &client->rpc_channel);
  if (rc != GZC_OK) {
    goto fail;
  }

  int timeout = client->config.connect_timeout_ms == 0 ? 5000 : client->config.connect_timeout_ms;
  gzc_signaling_config_t signaling;
  memset(&signaling, 0, sizeof(signaling));
  signaling.platform = client->config.platform;
  signaling.crypto = client->config.crypto;
  signaling.cipher_mode = client->config.cipher_mode;
  rc = gzc_key_from_text(client->config.private_key, &signaling.private_key);
  if (rc != GZC_OK) {
    goto fail;
  }
  gzc_buf_t signaling_url;
  gzc_buf_init(&signaling_url);
  rc = load_server_info(client, timeout, &signaling, &signaling_url);
  if (rc != GZC_OK) {
    gzc_buf_free(&signaling_url, client->config.platform);
    goto fail;
  }

  rc = client->config.webrtc->peer_start_offer(client->peer);
  if (rc != GZC_OK) {
    gzc_buf_free(&signaling_url, client->config.platform);
    goto fail;
  }
  rc = wait_until(client, &client->has_local_sdp, timeout);
  if (rc != GZC_OK) {
    gzc_buf_free(&signaling_url, client->config.platform);
    goto fail;
  }

  gzc_signaling_exchange_t exchange;
  memset(&exchange, 0, sizeof(exchange));
  gzc_http_request_t request;
  rc = gzc_signaling_build_offer_request(
      &signaling,
      gzc_str_from_parts((const char *)client->local_sdp.data, client->local_sdp.len),
      &exchange,
      &request);
  if (rc != GZC_OK) {
    gzc_signaling_exchange_free(&exchange, client->config.platform);
    gzc_buf_free(&signaling_url, client->config.platform);
    goto fail;
  }
  request.timeout_ms = timeout;
  gzc_http_response_t response;
  memset(&response, 0, sizeof(response));
  gzc_buf_init(&response.body);
  rc = client->config.http->request(client->config.http->userdata, &request, &response);
  gzc_buf_t answer_sdp;
  gzc_buf_init(&answer_sdp);
  if (rc == GZC_OK) {
    rc = gzc_signaling_parse_answer_response(&signaling, &exchange, &response, &answer_sdp);
  }
  if (rc == GZC_OK) {
    gzc_str_t answer = gzc_str_from_parts((const char *)answer_sdp.data, answer_sdp.len);
    rc = client->config.webrtc->peer_set_remote_sdp(client->peer, GZC_RTC_SDP_ANSWER, answer);
  }
  gzc_buf_free(&answer_sdp, client->config.platform);
  if (client->config.http->response_free != NULL) {
    client->config.http->response_free(client->config.http->userdata, &response);
  } else {
    gzc_buf_free(&response.body, client->config.platform);
  }
  gzc_signaling_exchange_free(&exchange, client->config.platform);
  gzc_buf_free(&signaling_url, client->config.platform);
  if (rc != GZC_OK) {
    goto fail;
  }

  rc = wait_until(client, &client->packet_channel_open, timeout);
  if (rc != GZC_OK) {
    goto fail;
  }
  return GZC_OK;

fail:
  if (client->rpc_channel != NULL) {
    if (client->config.webrtc->channel_close != NULL) {
      client->config.webrtc->channel_close(client->rpc_channel);
    }
    client->rpc_channel = NULL;
  }
  if (client->packet_channel != NULL) {
    if (client->config.webrtc->channel_close != NULL) {
      client->config.webrtc->channel_close(client->packet_channel);
    }
    client->packet_channel = NULL;
  }
  if (client->peer != NULL && client->config.webrtc->peer_close != NULL) {
    client->config.webrtc->peer_close(client->peer);
    client->peer = NULL;
  }
  client->packet_channel_open = false;
  client->rpc_channel_open = false;
  client->has_local_sdp = false;
  gzc_buf_reset(&client->rpc_rx);
  gzc_buf_reset(&client->packet_rx);
  return rc;
}

int gzc_client_close(gzc_client_t *client) {
  if (client == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->closed = true;
  if (client->peer != NULL && client->config.webrtc->channel_close != NULL) {
    for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
      if (client->inbound_channels[i] != NULL) {
        client->config.webrtc->channel_close(client->inbound_channels[i]);
      }
    }
    if (client->service_channel != NULL) {
      gzc_service_channel_close(client->service_channel);
    }
    if (client->rpc_channel != NULL) {
      client->config.webrtc->channel_close(client->rpc_channel);
    }
    if (client->packet_channel != NULL) {
      client->config.webrtc->channel_close(client->packet_channel);
    }
  }
  if (client->peer != NULL && client->config.webrtc->peer_close != NULL) {
    client->config.webrtc->peer_close(client->peer);
  }
  client->peer = NULL;
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    gzc_rpc_inbound_t *inbound = client->inbound[i];
    client->inbound_channels[i] = NULL;
    client->inbound[i] = NULL;
    gzc_rpc_inbound_destroy(inbound);
  }
  client->rpc_channel = NULL;
  client->packet_channel = NULL;
  if (client->service_channel != NULL) {
    gzc_service_channel_close(client->service_channel);
  }
  client->packet_channel_open = false;
  client->rpc_channel_open = false;
  client->has_local_sdp = false;
  return GZC_OK;
}

int gzc_client_poll(gzc_client_t *client, int timeout_ms) {
  if (client == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->closed || client->peer == NULL) {
    return GZC_ERR_CLOSED;
  }
  if (client->config.webrtc == NULL || client->config.webrtc->peer_poll == NULL) {
    return GZC_ERR_UNSUPPORTED;
  }
  client->dispatch_error = GZC_OK;
  int backend_timeout_ms = timeout_ms;
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    if (gzc_rpc_inbound_has_pending_output(client->inbound[i])) {
      backend_timeout_ms = 0;
      break;
    }
  }
  int rc = client->config.webrtc->peer_poll(client->peer, backend_timeout_ms);
  if (rc != GZC_OK) {
    return rc;
  }
  if (client->dispatch_error != GZC_OK) {
    return client->dispatch_error;
  }
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    if (client->inbound[i] != NULL) {
      gzc_rpc_inbound_t *inbound = client->inbound[i];
      gzc_rtc_channel_t *channel = client->inbound_channels[i];
      rc = gzc_rpc_inbound_poll(inbound);
      if (gzc_rpc_inbound_close_requested(inbound)) {
        client->inbound[i] = NULL;
        client->inbound_channels[i] = NULL;
        if (channel != NULL && client->config.webrtc->channel_close != NULL) {
          client->config.webrtc->channel_close(channel);
        }
        gzc_rpc_inbound_destroy(inbound);
      }
      if (rc != GZC_OK) {
        return rc;
      }
    }
  }
  return GZC_OK;
}

void gzc_client_destroy(gzc_client_t *client) {
  if (client == NULL) {
    return;
  }
  const gzc_platform_t *platform = client->config.platform == NULL ? gzc_default_platform() : client->config.platform;
  (void)gzc_client_close(client);
  gzc_buf_free(&client->local_sdp, platform);
  gzc_buf_free(&client->packet_rx, platform);
  gzc_buf_free(&client->rpc_rx, platform);
  gzc_buf_free(&client->rpc_response, platform);
  platform->free(platform->userdata, client);
}

gzc_rtc_channel_t *gzc_client_rpc_channel(gzc_client_t *client) {
  return client == NULL ? NULL : client->rpc_channel;
}

const gzc_platform_t *gzc_client_platform(gzc_client_t *client) {
  return client == NULL ? NULL : client->config.platform;
}

const gzc_webrtc_vtable_t *gzc_client_webrtc(gzc_client_t *client) {
  return client == NULL ? NULL : client->config.webrtc;
}

static void consume_rpc_rx(gzc_client_t *client, size_t len) {
  if (client == NULL || len == 0) {
    return;
  }
  if (len >= client->rpc_rx.len) {
    gzc_buf_reset(&client->rpc_rx);
    return;
  }
  memmove(client->rpc_rx.data, client->rpc_rx.data + len, client->rpc_rx.len - len);
  client->rpc_rx.len -= len;
  client->rpc_rx.data[client->rpc_rx.len] = 0;
}

static void consume_rx(gzc_buf_t *rx, size_t len) {
  if (rx == NULL || len == 0) {
    return;
  }
  if (len >= rx->len) {
    gzc_buf_reset(rx);
    return;
  }
  memmove(rx->data, rx->data + len, rx->len - len);
  rx->len -= len;
  rx->data[rx->len] = 0;
}

static int rpc_rx_next_frame_size(gzc_client_t *client, size_t *out_size) {
  if (client == NULL || out_size == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->rpc_rx.len < 4) {
    return GZC_ERR_TIMEOUT;
  }
  size_t payload_len = (size_t)client->rpc_rx.data[0] | ((size_t)client->rpc_rx.data[1] << 8);
  if (payload_len > GZC_RPC_MAX_FRAME_SIZE) {
    return GZC_ERR_RPC;
  }
  size_t total = 4 + payload_len;
  if (client->rpc_rx.len < total) {
    return GZC_ERR_TIMEOUT;
  }
  *out_size = total;
  return GZC_OK;
}

int gzc_client_reset_rpc_rx_internal(gzc_client_t *client) {
  if (client == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_reset(&client->rpc_rx);
  gzc_buf_reset(&client->rpc_response);
  return GZC_OK;
}

int gzc_client_open_rpc_channel_internal(gzc_client_t *client, int timeout_ms) {
  return open_rpc_channel(client, timeout_ms);
}

void gzc_client_close_rpc_channel_internal(gzc_client_t *client) {
  close_rpc_channel(client);
}

int gzc_client_store_rpc_response_internal(gzc_client_t *client, const uint8_t *data, size_t len, gzc_str_t *out_json) {
  if (client == NULL || out_json == NULL || (data == NULL && len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_reset(&client->rpc_response);
  int rc = gzc_buf_append(&client->rpc_response, client->config.platform, data, len);
  if (rc != GZC_OK) {
    return rc;
  }
  out_json->data = (const char *)client->rpc_response.data;
  out_json->len = client->rpc_response.len;
  return GZC_OK;
}

int gzc_client_read_rpc_frame_internal(gzc_client_t *client, int timeout_ms, gzc_buf_t *out_frame_bytes) {
  if (client == NULL || out_frame_bytes == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const int64_t start = now_ms(client);
  size_t frame_size = 0;
  for (;;) {
    int rc = rpc_rx_next_frame_size(client, &frame_size);
    if (rc == GZC_OK) {
      break;
    }
    if (rc != GZC_ERR_TIMEOUT) {
      return rc;
    }
    if (client->closed) {
      return GZC_ERR_CLOSED;
    }
    if (client->config.webrtc->peer_poll == NULL) {
      return GZC_ERR_TIMEOUT;
    }
    rc = gzc_client_poll(client, 10);
    if (rc != GZC_OK) {
      return rc;
    }
    if (timeout_ms >= 0 && now_ms(client) - start >= timeout_ms) {
      return GZC_ERR_TIMEOUT;
    }
  }
  gzc_buf_reset(out_frame_bytes);
  int rc = gzc_buf_append(out_frame_bytes, client->config.platform, client->rpc_rx.data, frame_size);
  if (rc != GZC_OK) {
    return rc;
  }
  consume_rpc_rx(client, frame_size);
  return GZC_OK;
}

static int rx_next_rpc_frame_size(gzc_buf_t *rx, size_t *out_size) {
  if (rx == NULL || out_size == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (rx->len < 4) {
    return GZC_ERR_TIMEOUT;
  }
  size_t payload_len = (size_t)rx->data[0] | ((size_t)rx->data[1] << 8);
  if (payload_len > GZC_RPC_MAX_FRAME_SIZE) {
    return GZC_ERR_RPC;
  }
  size_t total = 4 + payload_len;
  if (rx->len < total) {
    return GZC_ERR_TIMEOUT;
  }
  *out_size = total;
  return GZC_OK;
}

static int rx_next_packet_size(gzc_buf_t *rx, size_t *out_size) {
  if (rx == NULL || out_size == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (rx->len < 2) {
    return GZC_ERR_TIMEOUT;
  }
  size_t message_len = (size_t)rx->data[0] | ((size_t)rx->data[1] << 8);
  if (message_len == 0 || message_len > GZC_RPC_MAX_FRAME_SIZE) {
    return GZC_ERR_RPC;
  }
  size_t total = 2 + message_len;
  if (rx->len < total) {
    return GZC_ERR_TIMEOUT;
  }
  *out_size = total;
  return GZC_OK;
}

int gzc_client_open_service_channel(
    gzc_client_t *client,
    uint64_t service,
    int timeout_ms,
    gzc_service_channel_t **out_channel) {
  if (client == NULL || out_channel == NULL || client->peer == NULL || client->config.webrtc == NULL ||
      client->config.webrtc->peer_create_data_channel == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->service_channel != NULL) {
    gzc_service_channel_close(client->service_channel);
  }
  const gzc_platform_t *platform = client->config.platform == NULL ? gzc_default_platform() : client->config.platform;
  gzc_service_channel_t *channel = (gzc_service_channel_t *)platform->malloc(platform->userdata, sizeof(*channel));
  if (channel == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(channel, 0, sizeof(*channel));
  channel->client = client;
  channel->service = service;
  gzc_buf_init(&channel->rx);
  client->service_channel = channel;

  char label[64];
  int label_len = snprintf(label, sizeof(label), "giznet/v1/service/%llu", (unsigned long long)service);
  if (label_len <= 0 || (size_t)label_len >= sizeof(label)) {
    gzc_service_channel_close(channel);
    return GZC_ERR_INVALID_ARGUMENT;
  }

  gzc_rtc_channel_config_t cfg;
  memset(&cfg, 0, sizeof(cfg));
  cfg.label = gzc_str_from_parts(label, (size_t)label_len);
  cfg.ordered = true;
  cfg.reliable = true;
  int rc = client->config.webrtc->peer_create_data_channel(client->peer, &cfg, &channel->rtc);
  if (rc != GZC_OK) {
    gzc_service_channel_close(channel);
    return rc;
  }
  rc = wait_until(client, &channel->open, timeout_ms);
  if (rc != GZC_OK) {
    gzc_service_channel_close(channel);
    return rc;
  }
  *out_channel = channel;
  return GZC_OK;
}

int gzc_service_channel_send_frame(gzc_service_channel_t *channel, const gzc_rpc_frame_t *frame) {
  if (channel == NULL || channel->client == NULL || channel->rtc == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_t bytes;
  gzc_buf_init(&bytes);
  int rc = gzc_rpc_frame_encode(channel->client->config.platform, frame, &bytes);
  if (rc == GZC_OK) {
    rc = channel->client->config.webrtc->channel_send(channel->rtc, bytes.data, bytes.len, false);
  }
  gzc_buf_free(&bytes, channel->client->config.platform);
  return rc;
}

int gzc_service_channel_read_frame(gzc_service_channel_t *channel, int timeout_ms, gzc_buf_t *out_frame_bytes) {
  if (channel == NULL || channel->client == NULL || out_frame_bytes == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_client_t *client = channel->client;
  const int64_t start = now_ms(client);
  size_t frame_size = 0;
  for (;;) {
    int rc = rx_next_rpc_frame_size(&channel->rx, &frame_size);
    if (rc == GZC_OK) {
      break;
    }
    if (rc != GZC_ERR_TIMEOUT) {
      return rc;
    }
    if (client->closed || channel->closed) {
      return GZC_ERR_CLOSED;
    }
    if (client->config.webrtc->peer_poll == NULL) {
      return GZC_ERR_TIMEOUT;
    }
    rc = gzc_client_poll(client, 10);
    if (rc != GZC_OK) {
      return rc;
    }
    if (timeout_ms >= 0 && now_ms(client) - start >= timeout_ms) {
      return GZC_ERR_TIMEOUT;
    }
  }
  gzc_buf_reset(out_frame_bytes);
  int rc = gzc_buf_append(out_frame_bytes, client->config.platform, channel->rx.data, frame_size);
  if (rc != GZC_OK) {
    return rc;
  }
  consume_rx(&channel->rx, frame_size);
  return GZC_OK;
}

void gzc_service_channel_close(gzc_service_channel_t *channel) {
  if (channel == NULL) {
    return;
  }
  gzc_client_t *client = channel->client;
  const gzc_platform_t *platform = client == NULL || client->config.platform == NULL ? gzc_default_platform() : client->config.platform;
  if (client != NULL && client->peer != NULL && channel->rtc != NULL && client->config.webrtc != NULL &&
      client->config.webrtc->channel_close != NULL) {
    client->config.webrtc->channel_close(channel->rtc);
  }
  if (client != NULL && client->service_channel == channel) {
    client->service_channel = NULL;
  }
  gzc_buf_free(&channel->rx, platform);
  platform->free(platform->userdata, channel);
}

int gzc_client_send_packet(gzc_client_t *client, uint8_t protocol, const uint8_t *payload, size_t len) {
  if (client == NULL || (payload == NULL && len != 0) || client->packet_channel == NULL || !client->packet_channel_open ||
      client->config.webrtc == NULL || client->config.webrtc->channel_send == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (len > GZC_RPC_MAX_FRAME_SIZE - 1) {
    return GZC_ERR_RPC;
  }
  if (!valid_packet_protocol(protocol)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_t message;
  gzc_buf_init(&message);
  int rc = gzc_buf_append(&message, client->config.platform, &protocol, 1);
  if (rc == GZC_OK) {
    rc = gzc_buf_append(&message, client->config.platform, payload, len);
  }
  if (rc == GZC_OK) {
    rc = client->config.webrtc->channel_send(client->packet_channel, message.data, message.len, false);
  }
  gzc_buf_free(&message, client->config.platform);
  return rc;
}

int gzc_client_read_packet(gzc_client_t *client, int timeout_ms, uint8_t *out_protocol, gzc_buf_t *out_payload) {
  if (client == NULL || out_protocol == NULL || out_payload == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const int64_t start = now_ms(client);
  size_t message_size = 0;
  for (;;) {
    int rc = rx_next_packet_size(&client->packet_rx, &message_size);
    if (rc == GZC_OK) {
      break;
    }
    if (rc != GZC_ERR_TIMEOUT) {
      return rc;
    }
    if (client->closed) {
      return GZC_ERR_CLOSED;
    }
    if (client->config.webrtc->peer_poll == NULL) {
      return GZC_ERR_TIMEOUT;
    }
    rc = gzc_client_poll(client, 10);
    if (rc != GZC_OK) {
      return rc;
    }
    if (timeout_ms >= 0 && now_ms(client) - start >= timeout_ms) {
      return GZC_ERR_TIMEOUT;
    }
  }
  size_t payload_len = message_size - 3;
  uint8_t protocol = client->packet_rx.data[2];
  if (!valid_packet_protocol(protocol)) {
    consume_rx(&client->packet_rx, message_size);
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_protocol = protocol;
  gzc_buf_reset(out_payload);
  int rc = gzc_buf_append(out_payload, client->config.platform, client->packet_rx.data + 3, payload_len);
  if (rc != GZC_OK) {
    return rc;
  }
  consume_rx(&client->packet_rx, message_size);
  return GZC_OK;
}
