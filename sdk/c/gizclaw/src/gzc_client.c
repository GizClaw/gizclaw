#include "gzc_client.h"

#include "gzc_client_internal.h"
#include "gzc_json.h"
#include "gzc_rpc_frame.h"
#include "payload/ai.pb.h"
#include "rpc.pb.h"

#include <pb_decode.h>

#include <ctype.h>
#include <stdio.h>
#include <string.h>

static bool valid_tool_name(gzc_str_t name) {
  if (name.data == NULL || name.len == 0u || name.len > 64u ||
      !((name.data[0] >= 'A' && name.data[0] <= 'Z') ||
        (name.data[0] >= 'a' && name.data[0] <= 'z') ||
        name.data[0] == '_')) {
    return false;
  }
  for (size_t i = 1; i < name.len; i++) {
    const char value = name.data[i];
    if (!((value >= 'A' && value <= 'Z') ||
          (value >= 'a' && value <= 'z') ||
          (value >= '0' && value <= '9') || value == '_' || value == '-')) {
      return false;
    }
  }
  return true;
}

typedef struct gzc_rpc_inbound gzc_rpc_inbound_t;
int gzc_rpc_inbound_create(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    gzc_rpc_inbound_t **out_inbound);
int gzc_rpc_inbound_feed(gzc_rpc_inbound_t *inbound, const uint8_t *data, size_t len, bool is_text);
int gzc_rpc_inbound_poll(gzc_rpc_inbound_t *inbound);
int gzc_rpc_inbound_backend_timeout_ms(gzc_rpc_inbound_t *inbound, int requested_timeout_ms);
bool gzc_rpc_inbound_close_requested(gzc_rpc_inbound_t *inbound);
void gzc_rpc_inbound_destroy(gzc_rpc_inbound_t *inbound);

struct gzc_service_channel {
  gzc_client_t *client;
  gzc_rtc_channel_t *rtc;
  gzc_service_channel_t *next;
  /* Non-owning: the request owns its state, buffers, and public handle. */
  gzc_rpc_request_t *rpc_request;
  gzc_buf_t rx;
  uint64_t service;
  bool open;
  bool closed;
  bool close_requested;
  bool write_blocked;
};

typedef struct {
  uint16_t len;
  uint8_t data[GZC_OPUS_MAX_PACKET_SIZE];
} gzc_opus_rx_slot_t;

_Static_assert(
    GZC_OPUS_MAX_PACKET_SIZE <= UINT16_MAX,
    "Opus receive slot length must fit in uint16_t");

struct gzc_client {
  gzc_client_config_t config;
  const gzc_webrtc_media_vtable_t *media;
  gzc_peer_add_ice_server_fn peer_add_ice_server;
  gzc_rtc_peer_t *peer;
  gzc_rtc_channel_t *packet_channel;
  gzc_service_channel_t *service_channels;
  gzc_service_channel_t *event_channel;
  gzc_event_stream_t *event_handle;
  gzc_public_key_t server_public_key;
  /* host[:port] ICE UDP endpoint advertised by /server-info. */
  gzc_buf_t ice_endpoint;
  gzc_buf_t local_sdp;
  gzc_buf_t packet_rx;
  gzc_opus_rx_slot_t *opus_rx;
  size_t opus_rx_capacity;
  size_t opus_rx_head;
  size_t opus_rx_count;
  int opus_rx_error;
  bool read_opus_next;
  bool media_callback_registered;
  gzc_rpc_inbound_t *inbound[GZC_RPC_MAX_INBOUND_CHANNELS];
  gzc_rtc_channel_t *inbound_channels[GZC_RPC_MAX_INBOUND_CHANNELS];
  int dispatch_error;
  size_t service_write_depth;
  bool has_local_sdp;
  bool packet_channel_open;
  bool event_handle_open;
  bool closed;
};

#define GZC_EVENT_RX_MAX_BUFFER_SIZE \
  (4u * (GZC_RPC_MAX_FRAME_SIZE + 4u))

enum {
  gzc_protocol_custom_start = 0x40,
};

static bool valid_opus_packet(const uint8_t *opus, size_t len) {
  if (opus == NULL || len == 0 || len > GZC_OPUS_MAX_PACKET_SIZE) {
    return false;
  }
  const uint8_t config = opus[0] >> 3;
  uint32_t frame_ticks = 0;
  if (config < 12u) {
    static const uint16_t silk_ticks[] = {480u, 960u, 1920u, 2880u};
    frame_ticks = silk_ticks[config & 3u];
  } else if (config < 16u) {
    frame_ticks = (config & 1u) == 0u ? 480u : 960u;
  } else {
    static const uint16_t celt_ticks[] = {120u, 240u, 480u, 960u};
    frame_ticks = celt_ticks[config & 3u];
  }
  uint32_t frame_count = 1u;
  switch (opus[0] & 3u) {
  case 0u:
    break;
  case 1u:
  case 2u:
    frame_count = 2u;
    break;
  case 3u:
    if (len < 2u) {
      return false;
    }
    frame_count = opus[1] & 0x3fu;
    if (frame_count == 0u || frame_count > 48u) {
      return false;
    }
    break;
  }
  return frame_ticks * frame_count <= 5760u;
}

static void clear_opus_rx(gzc_client_t *client) {
  client->opus_rx_head = 0u;
  client->opus_rx_count = 0u;
  client->opus_rx_error = GZC_OK;
  client->read_opus_next = false;
}

static int allocate_opus_rx(gzc_client_t *client) {
  if (client == NULL || client->opus_rx_capacity == 0u ||
      client->opus_rx_capacity > SIZE_MAX / sizeof(gzc_opus_rx_slot_t)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->opus_rx != NULL) {
    return GZC_OK;
  }
  const gzc_platform_t *platform = client->config.platform;
  gzc_opus_rx_slot_t *slots = (gzc_opus_rx_slot_t *)platform->malloc(
      platform->userdata, client->opus_rx_capacity * sizeof(*slots));
  if (slots == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  client->opus_rx = slots;
  clear_opus_rx(client);
  return GZC_OK;
}

static void on_opus_frame(
    void *userdata,
    gzc_rtc_peer_t *peer,
    const uint8_t *opus,
    size_t opus_len) {
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL || peer != client->peer || client->closed ||
      !client->media_callback_registered) {
    return;
  }
  if (!valid_opus_packet(opus, opus_len)) {
    client->opus_rx_error = GZC_ERR_WEBRTC;
    return;
  }
  if (client->opus_rx == NULL || client->opus_rx_capacity == 0u) {
    client->opus_rx_error = GZC_ERR_NO_MEMORY;
    return;
  }
  if (client->opus_rx_count == client->opus_rx_capacity) {
    client->opus_rx_head =
        (client->opus_rx_head + 1u) % client->opus_rx_capacity;
    client->opus_rx_count--;
  }
  const size_t tail =
      (client->opus_rx_head + client->opus_rx_count) %
      client->opus_rx_capacity;
  gzc_opus_rx_slot_t *slot = &client->opus_rx[tail];
  memcpy(slot->data, opus, opus_len);
  slot->len = (uint16_t)opus_len;
  client->opus_rx_count++;
}

static int64_t now_ms(gzc_client_t *client) {
  if (client->config.platform != NULL && client->config.platform->time_instant_ms != NULL) {
    return client->config.platform->time_instant_ms(client->config.platform->userdata);
  }
  return 0;
}

static int configure_service_channel(gzc_client_t *client, gzc_rtc_channel_t *channel) {
  if (client == NULL || channel == NULL || client->config.webrtc == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->config.webrtc->channel_set_buffered_amount_low_threshold == NULL) {
    return GZC_OK;
  }
  return client->config.webrtc->channel_set_buffered_amount_low_threshold(
      channel, (uint64_t)client->config.service_write_low_water_bytes);
}

static gzc_service_channel_t *service_channel_for_rtc(
    gzc_client_t *client,
    gzc_rtc_channel_t *rtc) {
  if (client == NULL || rtc == NULL) {
    return NULL;
  }
  for (gzc_service_channel_t *channel = client->service_channels;
       channel != NULL;
       channel = channel->next) {
    if (channel->rtc == rtc) {
      return channel;
    }
  }
  return NULL;
}

static bool service_channel_is_tracked(
    const gzc_client_t *client,
    const gzc_service_channel_t *target) {
  if (client == NULL || target == NULL) {
    return false;
  }
  for (const gzc_service_channel_t *channel = client->service_channels;
       channel != NULL;
       channel = channel->next) {
    if (channel == target) {
      return true;
    }
  }
  return false;
}

static int service_write_ready(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    bool *blocked,
    bool *out_ready) {
  if (client == NULL || channel == NULL || blocked == NULL || out_ready == NULL ||
      client->config.webrtc == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->config.webrtc->channel_buffered_amount == NULL) {
    *blocked = false;
    *out_ready = true;
    return GZC_OK;
  }
  uint64_t amount = 0;
  int rc = client->config.webrtc->channel_buffered_amount(channel, &amount);
  if (rc != GZC_OK) {
    return rc;
  }
  if (*blocked) {
    if (amount > (uint64_t)client->config.service_write_low_water_bytes) {
      *out_ready = false;
      return GZC_OK;
    }
    *blocked = false;
  }
  if (amount >= (uint64_t)client->config.service_write_high_water_bytes) {
    *blocked = true;
    *out_ready = false;
    return GZC_OK;
  }
  *out_ready = true;
  return GZC_OK;
}

static void fail_service_write(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel) {
  if (client == NULL || channel == NULL) {
    return;
  }
  gzc_service_channel_t *service_channel =
      service_channel_for_rtc(client, channel);
  if (service_channel == NULL) {
    /* A synchronous terminal callback may already have consumed this handle. */
    return;
  }
  service_channel->open = false;
  service_channel->closed = true;
  service_channel->close_requested = true;
  if (service_channel == client->event_channel) {
    client->closed = true;
  }
  if (client->config.webrtc != NULL && client->config.webrtc->channel_close != NULL) {
    client->config.webrtc->channel_close(channel);
  }
}

int gzc_client_try_write_bytes_internal(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    const uint8_t *data,
    size_t len,
    size_t *offset,
    bool *blocked,
    size_t max_chunks) {
  if (client == NULL || channel == NULL || offset == NULL || blocked == NULL ||
      (data == NULL && len != 0) || *offset > len || max_chunks == 0 ||
      client->config.webrtc == NULL || client->config.webrtc->channel_send == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  size_t chunks = 0;
  while (*offset < len && chunks < max_chunks) {
    bool ready = false;
    int rc = service_write_ready(client, channel, blocked, &ready);
    if (rc != GZC_OK || !ready) {
      return rc;
    }
    size_t count = len - *offset;
    if (count > GZC_SERVICE_WRITE_CHUNK_SIZE) {
      count = GZC_SERVICE_WRITE_CHUNK_SIZE;
    }
    rc = client->config.webrtc->channel_send(channel, data + *offset, count, false);
    if (rc == GZC_ERR_WOULD_BLOCK) {
      *blocked = true;
      return GZC_OK;
    }
    if (rc != GZC_OK) {
      return rc;
    }
    *offset += count;
    chunks++;
  }
  return GZC_OK;
}

static int write_segments_until(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    const uint8_t *first,
    size_t first_len,
    const uint8_t *second,
    size_t second_len,
    bool *blocked,
    int64_t deadline_ms) {
  if (client == NULL || channel == NULL || blocked == NULL ||
      (first == NULL && first_len != 0) || (second == NULL && second_len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const uint8_t *segments[2] = {first, second};
  size_t lengths[2] = {first_len, second_len};
  bool partial = false;
  for (size_t segment = 0; segment < 2; segment++) {
    size_t offset = 0;
    while (offset < lengths[segment]) {
      if (deadline_ms <= now_ms(client)) {
        fail_service_write(client, channel);
        return GZC_ERR_TIMEOUT;
      }
      int rc = gzc_client_try_write_bytes_internal(
          client, channel, segments[segment], lengths[segment], &offset, blocked, 1u);
      partial = partial || offset != 0;
      if (rc != GZC_OK) {
        if (partial) {
          fail_service_write(client, channel);
        }
        return rc;
      }
      if (offset == lengths[segment]) {
        break;
      }
      if (client->closed) {
        return GZC_ERR_CLOSED;
      }
      const int64_t remaining = deadline_ms - now_ms(client);
      if (remaining <= 0) {
        fail_service_write(client, channel);
        return GZC_ERR_TIMEOUT;
      }
      const int poll_timeout_ms = remaining < 10 ? (int)remaining : 10;
      rc = gzc_client_poll(client, poll_timeout_ms);
      if (rc != GZC_OK) {
        if (partial) {
          fail_service_write(client, channel);
        }
        return rc;
      }
    }
  }
  return GZC_OK;
}

int gzc_client_write_bytes_internal(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    const uint8_t *data,
    size_t len,
    bool *blocked) {
  const int64_t deadline_ms =
      now_ms(client) + (int64_t)client->config.write_timeout_ms;
  return write_segments_until(
      client, channel, data, len, NULL, 0, blocked, deadline_ms);
}

int gzc_client_write_frame_internal(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    const gzc_rpc_frame_t *frame,
    bool *blocked) {
  if (frame == NULL || (frame->data == NULL && frame->len != 0) ||
      !gzc_rpc_frame_type_valid(frame->type) || frame->len > GZC_RPC_MAX_FRAME_SIZE ||
      (frame->type == GZC_RPC_FRAME_EOS && frame->len != 0)) {
    return GZC_ERR_RPC;
  }
  uint8_t header[4];
  header[0] = (uint8_t)(frame->len & 0xffu);
  header[1] = (uint8_t)((frame->len >> 8) & 0xffu);
  header[2] = (uint8_t)(((uint16_t)frame->type) & 0xffu);
  header[3] = (uint8_t)((((uint16_t)frame->type) >> 8) & 0xffu);
  const int64_t deadline_ms =
      now_ms(client) + (int64_t)client->config.write_timeout_ms;
  return write_segments_until(
      client,
      channel,
      header,
      sizeof(header),
      frame->data,
      frame->len,
      blocked,
      deadline_ms);
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
  return protocol == GZC_PROTOCOL_OPUS_PACKET || protocol >= gzc_protocol_custom_start;
}

/* Four decimal octets, used for the IPv4 tail of an IPv6 literal. */
static bool valid_ipv4_literal(gzc_str_t text) {
  size_t octets = 0;
  size_t i = 0;
  while (i < text.len) {
    size_t digits = 0;
    unsigned value = 0;
    while (i < text.len && isdigit((unsigned char)text.data[i])) {
      value = value * 10u + (unsigned)(text.data[i] - '0');
      digits++;
      i++;
    }
    if (digits == 0 || digits > 3 || value > 255u) {
      return false;
    }
    octets++;
    if (i == text.len) {
      break;
    }
    if (text.data[i] != '.') {
      return false;
    }
    i++;
    if (i == text.len) {
      return false;
    }
  }
  return octets == 4;
}

/*
 * A bracketed host must be an IPv6 literal: at most one "::" run, hextets of
 * one to four hex digits, an optional trailing dotted-quad IPv4 tail, and no
 * zone identifier.
 */
static bool valid_ipv6_literal(gzc_str_t text) {
  if (text.len == 0) {
    return false;
  }
  size_t i = 0;
  size_t hextets = 0;
  bool compressed = false;
  if (text.data[0] == ':') {
    if (text.len < 2 || text.data[1] != ':') {
      return false;
    }
    compressed = true;
    i = 2;
    if (i == text.len) {
      return true;
    }
  }
  while (i < text.len) {
    size_t start = i;
    size_t digits = 0;
    while (i < text.len && isxdigit((unsigned char)text.data[i])) {
      digits++;
      i++;
    }
    if (digits == 0) {
      return false;
    }
    if (i < text.len && text.data[i] == '.') {
      if (!valid_ipv4_literal(gzc_str_from_parts(text.data + start, text.len - start))) {
        return false;
      }
      hextets += 2;
      i = text.len;
      break;
    }
    if (digits > 4) {
      return false;
    }
    hextets += 1;
    if (i == text.len) {
      break;
    }
    if (text.data[i] != ':') {
      return false;
    }
    i++;
    if (i < text.len && text.data[i] == ':') {
      if (compressed) {
        return false;
      }
      compressed = true;
      i++;
      if (i == text.len) {
        break;
      }
    } else if (i == text.len) {
      return false;
    }
  }
  return compressed ? hextets <= 7 : hextets == 8;
}

/*
 * Authority is host[:port]; server-info advertises ICE endpoints this way. An
 * IPv6 host is bracketed. A port, when present, is one to five digits, so
 * scheme-like values such as "http:" are rejected instead of being read as a
 * host with an empty port.
 */
static bool valid_authority(gzc_str_t authority) {
  if (str_empty(authority)) {
    return false;
  }
  for (size_t i = 0; i < authority.len; i++) {
    char ch = authority.data[i];
    if (ch == '/' || ch == '?' || ch == '#' || ch == '@' || ch == '\\' ||
        ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n') {
      return false;
    }
  }
  size_t host_len = authority.len;
  if (authority.data[0] == '[') {
    const char *close = memchr(authority.data, ']', authority.len);
    if (close == NULL || close == authority.data + 1) {
      return false;
    }
    host_len = (size_t)(close - authority.data) + 1u;
    if (!valid_ipv6_literal(gzc_str_from_parts(authority.data + 1, host_len - 2u))) {
      return false;
    }
  } else {
    const char *colon = memchr(authority.data, ':', authority.len);
    host_len = colon == NULL ? authority.len : (size_t)(colon - authority.data);
    for (size_t i = 0; i < host_len; i++) {
      if (authority.data[i] == '[' || authority.data[i] == ']') {
        return false;
      }
    }
  }
  if (host_len == 0) {
    return false;
  }
  if (host_len == authority.len) {
    return true;
  }
  if (authority.data[host_len] != ':') {
    return false;
  }
  size_t port_len = authority.len - host_len - 1u;
  if (port_len == 0 || port_len > 5u) {
    return false;
  }
  for (size_t i = host_len + 1u; i < authority.len; i++) {
    if (!isdigit((unsigned char)authority.data[i])) {
      return false;
    }
  }
  return true;
}

static bool valid_path(gzc_str_t path) {
  return !str_empty(path) && path.data[0] == '/' &&
         !(path.len >= 2 && path.data[1] == '/');
}

/* Returns the length of a supported URL scheme prefix, or zero. */
static size_t url_scheme_len(gzc_str_t url) {
  if (str_has_cstr_prefix(url, "https://")) {
    return 8u;
  }
  if (str_has_cstr_prefix(url, "http://")) {
    return 7u;
  }
  return 0u;
}

static bool str_has_scheme(gzc_str_t value) {
  if (value.data == NULL || value.len < 3) {
    return false;
  }
  for (size_t i = 0; i + 2 < value.len; i++) {
    if (value.data[i] == ':' && value.data[i + 1] == '/' && value.data[i + 2] == '/') {
      return true;
    }
  }
  return false;
}

/*
 * Validates an absolute http or https base URL and returns it without any
 * trailing slash. out_base borrows the caller's storage.
 */
static bool valid_base_url(gzc_str_t url, gzc_str_t *out_base) {
  size_t scheme_len = url_scheme_len(url);
  if (scheme_len == 0u || out_base == NULL) {
    return false;
  }
  size_t authority_len = 0;
  while (scheme_len + authority_len < url.len && url.data[scheme_len + authority_len] != '/') {
    authority_len++;
  }
  if (!valid_authority(gzc_str_from_parts(url.data + scheme_len, authority_len))) {
    return false;
  }
  for (size_t i = scheme_len + authority_len; i < url.len; i++) {
    char ch = url.data[i];
    if (ch == '?' || ch == '#' || ch == '@' ||
        ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n') {
      return false;
    }
    if (ch == '/' && i + 1 < url.len && url.data[i + 1] == '/') {
      return false;
    }
  }
  size_t base_len = url.len;
  while (base_len > scheme_len + authority_len && url.data[base_len - 1] == '/') {
    base_len--;
  }
  *out_base = gzc_str_from_parts(url.data, base_len);
  return true;
}

/*
 * Accepts an absolute http or https base URL, or a bare host[:port] that keeps
 * the historical plaintext lane working.
 */
static bool valid_server_url(gzc_str_t url) {
  gzc_str_t base;
  if (url_scheme_len(url) != 0u) {
    return valid_base_url(url, &base);
  }
  return !str_has_scheme(url) && valid_authority(url);
}

static int build_url(gzc_client_t *client, gzc_str_t base_url, gzc_str_t path, gzc_buf_t *out_url) {
  if (client == NULL || out_url == NULL || !valid_path(path)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_str_t base = base_url;
  bool implicit_http = url_scheme_len(base_url) == 0u;
  if (implicit_http) {
    if (str_has_scheme(base_url) || !valid_authority(base_url)) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
  } else if (!valid_base_url(base_url, &base)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_reset(out_url);
  int rc = GZC_OK;
  if (implicit_http) {
    rc = gzc_buf_append_cstr(out_url, client->config.platform, "http://");
    if (rc != GZC_OK) {
      return rc;
    }
  }
  rc = gzc_buf_append_str(out_url, client->config.platform, base);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append_str(out_url, client->config.platform, path);
}

/*
 * Builds a signaling URL from server-info transport metadata. A bare
 * host[:port] inherits the configured server URL scheme; an absolute URL is
 * used verbatim.
 */
static int build_transport_url(
    gzc_client_t *client,
    gzc_str_t endpoint,
    gzc_str_t path,
    gzc_buf_t *out_url) {
  if (client == NULL || out_url == NULL || !valid_path(path)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  size_t scheme_len = url_scheme_len(client->config.server_url);
  if (str_has_scheme(endpoint) || scheme_len == 0u) {
    return build_url(client, endpoint, path, out_url);
  }
  if (!valid_authority(endpoint)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_reset(out_url);
  int rc = gzc_buf_append(
      out_url,
      client->config.platform,
      (const uint8_t *)client->config.server_url.data,
      scheme_len);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_buf_append_str(out_url, client->config.platform, endpoint);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append_str(out_url, client->config.platform, path);
}

static int build_endpoint_url(gzc_client_t *client, gzc_str_t path, gzc_buf_t *out_url) {
  if (client == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return build_url(client, client->config.server_url, path, out_url);
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
    if (client->peer_add_ice_server == NULL) {
      result = GZC_ERR_UNSUPPORTED;
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
      rc = client->peer_add_ice_server(client->peer, url, username, credential);
      if (rc != GZC_OK) {
        result = rc;
        break;
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
  if (!valid_server_url(client->config.server_url)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_buf_reset(&client->ice_endpoint);

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
      rc = gzc_key_from_text(public_key, &client->server_public_key);
    }
    if (rc == GZC_OK && gzc_key_is_zero(&client->server_public_key)) {
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

  /*
   * The HTTP entry point may terminate TLS on a port that carries no ICE, so
   * the UDP endpoint comes from server-info rather than from the server URL.
   */
  if (gzc_json_find_field(body, "endpoint", &raw) == GZC_OK) {
    gzc_str_t ice_endpoint;
    rc = gzc_json_parse_string(raw, &ice_endpoint);
    if (rc != GZC_OK || !valid_authority(ice_endpoint)) {
      free_http_response(client, &response);
      return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
    }
    rc = gzc_buf_append_str(&client->ice_endpoint, client->config.platform, ice_endpoint);
    if (rc != GZC_OK) {
      free_http_response(client, &response);
      return rc;
    }
  }

  signaling->remote_public_key = client->server_public_key;
  gzc_str_t signaling_endpoint = client->config.server_url;
  gzc_str_t signaling_path = gzc_str_from_cstr(GZC_SIGNALING_PATH);
  bool gateway_transport = false;
  gzc_str_t transport_raw;
  if (gzc_json_find_field(body, "transport", &transport_raw) == GZC_OK) {
    gateway_transport = true;
    if (gzc_json_validate_object(transport_raw) != GZC_OK) {
      free_http_response(client, &response);
      return GZC_ERR_JSON;
    }
    gzc_str_t mode_raw;
    gzc_str_t mode;
    if (gzc_json_find_field(transport_raw, "mode", &mode_raw) != GZC_OK ||
        gzc_json_parse_string(mode_raw, &mode) != GZC_OK ||
        !str_eq_cstr(mode, "edge-gateway")) {
      free_http_response(client, &response);
      return GZC_ERR_UNSUPPORTED;
    }
    gzc_str_t endpoint_raw;
    if (gzc_json_find_field(transport_raw, "endpoint", &endpoint_raw) != GZC_OK ||
        gzc_json_parse_string(endpoint_raw, &signaling_endpoint) != GZC_OK) {
      free_http_response(client, &response);
      return GZC_ERR_INVALID_ARGUMENT;
    }
    gzc_str_t public_key_raw;
    gzc_str_t public_key;
    if (gzc_json_find_field(transport_raw, "public_key", &public_key_raw) != GZC_OK ||
        gzc_json_parse_string(public_key_raw, &public_key) != GZC_OK ||
        gzc_key_from_text(public_key, &signaling->remote_public_key) != GZC_OK ||
        gzc_key_is_zero(&signaling->remote_public_key) ||
        memcmp(signaling->remote_public_key.bytes,
               client->server_public_key.bytes,
               GZC_KEY_SIZE) == 0) {
      free_http_response(client, &response);
      return GZC_ERR_INVALID_ARGUMENT;
    }
    gzc_str_t path_raw;
    if (gzc_json_find_field(transport_raw, "signaling_path", &path_raw) != GZC_OK ||
        gzc_json_parse_string(path_raw, &signaling_path) != GZC_OK) {
      free_http_response(client, &response);
      return GZC_ERR_INVALID_ARGUMENT;
    }
  } else if (gzc_json_find_field(body, "signaling_path", &raw) == GZC_OK) {
    rc = gzc_json_parse_string(raw, &signaling_path);
    if (rc != GZC_OK) {
      free_http_response(client, &response);
      return rc;
    }
  }
  if (!gateway_transport) {
    rc = apply_ice_servers(client, body);
    if (rc != GZC_OK) {
      free_http_response(client, &response);
      return rc;
    }
  }
  rc = build_transport_url(client, signaling_endpoint, signaling_path, signaling_url);
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
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    if (channel == client->inbound_channels[i]) {
      if (state == GZC_RTC_CHANNEL_CLOSED || state == GZC_RTC_CHANNEL_ERROR) {
        gzc_rpc_inbound_t *inbound = client->inbound[i];
        client->inbound[i] = NULL;
        client->inbound_channels[i] = NULL;
        gzc_rpc_inbound_destroy(inbound);
      }
      return;
    }
  }
  gzc_service_channel_t *service_channel =
      service_channel_for_rtc(client, channel);
  bool *open_flag = NULL;
  if (channel == client->packet_channel) {
    open_flag = &client->packet_channel_open;
  } else if (service_channel != NULL) {
    open_flag = &service_channel->open;
  }
  if (state == GZC_RTC_CHANNEL_OPEN) {
    if (open_flag != NULL) {
      *open_flag = true;
    }
    return;
  }
  if (state != GZC_RTC_CHANNEL_CLOSED && state != GZC_RTC_CHANNEL_ERROR) {
    return;
  }
  if (service_channel != NULL) {
    service_channel->open = false;
    service_channel->closed = true;
    service_channel->close_requested = true;
    service_channel->rtc = NULL;
    if (service_channel->rpc_request != NULL) {
      gzc_rpc_request_channel_closed_internal(
          service_channel->rpc_request);
    }
    if (service_channel == client->event_channel) {
      client->closed = true;
    }
    return;
  }
  if (channel == client->packet_channel) {
    client->packet_channel_open = false;
    client->packet_channel = NULL;
    client->closed = true;
  }
}

static void on_channel_buffered_amount_low(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel) {
  (void)userdata;
  (void)peer;
  (void)channel;
  /* The callback is dispatched by peer_poll, which wakes the blocked writer. */
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
  int rc = configure_service_channel(client, channel);
  if (rc == GZC_OK) {
    rc = gzc_rpc_inbound_create(client, channel, &inbound);
  }
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
  gzc_service_channel_t *service_channel =
      service_channel_for_rtc(client, channel);
  if (service_channel != NULL && service_channel->rpc_request != NULL) {
    (void)gzc_rpc_request_feed_internal(
        service_channel->rpc_request, data, len);
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
    if (len > 0 && data[0] < gzc_protocol_custom_start) {
      return;
    }
    (void)append_framed_rx(&client->packet_rx, client->config.platform, data, len);
    return;
  }
  if (service_channel != NULL) {
    if (service_channel == client->event_channel &&
        (service_channel->rx.len > GZC_EVENT_RX_MAX_BUFFER_SIZE ||
         len > GZC_EVENT_RX_MAX_BUFFER_SIZE - service_channel->rx.len)) {
      service_channel->closed = true;
      service_channel->open = false;
      client->closed = true;
      client->dispatch_error = GZC_ERR_NO_MEMORY;
      return;
    }
    int rc = gzc_buf_append(
        &service_channel->rx, client->config.platform, data, len);
    if (rc != GZC_OK) {
      service_channel->closed = true;
      service_channel->open = false;
      client->closed = true;
      client->dispatch_error = rc;
    }
    return;
  }
}

static void release_terminal_rpc_requests(gzc_client_t *client) {
  if (client == NULL) {
    return;
  }
  gzc_service_channel_t *channel = client->service_channels;
  while (channel != NULL) {
    gzc_service_channel_t *next = channel->next;
    if (channel->rpc_request != NULL &&
        gzc_rpc_request_terminal_internal(channel->rpc_request)) {
      gzc_client_release_rpc_request_internal(
          client, channel, channel->rpc_request);
    }
    channel = next;
  }
}

static void fail_pending_rpc_requests(gzc_client_t *client, int status) {
  if (client == NULL || status == GZC_OK) {
    return;
  }
  for (gzc_service_channel_t *channel = client->service_channels;
       channel != NULL;
       channel = channel->next) {
    if (channel->rpc_request != NULL) {
      gzc_rpc_request_transport_error_internal(
          channel->rpc_request, status);
    }
  }
}

static void expire_pending_rpc_requests(gzc_client_t *client) {
  if (client == NULL) {
    return;
  }
  const int64_t instant_ms = now_ms(client);
  for (gzc_service_channel_t *channel = client->service_channels;
       channel != NULL;
       channel = channel->next) {
    if (channel->rpc_request != NULL) {
      gzc_rpc_request_expire_internal(
          channel->rpc_request, instant_ms);
    }
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

static int wait_for_service_channel_open(
    gzc_client_t *client,
    gzc_service_channel_t *channel,
    int timeout_ms) {
  const int64_t start = now_ms(client);
  for (;;) {
    if (!service_channel_is_tracked(client, channel)) {
      return GZC_ERR_CLOSED;
    }
    if (channel->open) {
      return GZC_OK;
    }
    if (client->closed || channel->closed) {
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
}

static int create_service_channel(
    gzc_client_t *client,
    uint64_t service,
    gzc_service_channel_t **out_channel);

int gzc_client_create(const gzc_client_config_t *config, gzc_client_t **out_client) {
  if (config == NULL || out_client == NULL || config->http == NULL || config->webrtc == NULL ||
      config->crypto == NULL ||
      config->webrtc->peer_create == NULL || config->webrtc->peer_start_offer == NULL ||
      config->webrtc->peer_set_remote_sdp == NULL || config->webrtc->peer_create_data_channel == NULL ||
      config->webrtc->peer_poll == NULL ||
      config->webrtc->channel_send == NULL ||
      config->webrtc->channel_close == NULL || config->webrtc->peer_close == NULL ||
      config->http->request == NULL ||
      config->write_timeout_ms <= 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (!valid_server_url(config->server_url)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if ((config->webrtc->channel_buffered_amount == NULL) !=
      (config->webrtc->channel_set_buffered_amount_low_threshold == NULL)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if ((config->tool_handlers == NULL && config->tool_handler_count != 0u) ||
      (config->tool_handlers != NULL && config->tool_handler_count == 0u)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  for (size_t i = 0; i < config->tool_handler_count; i++) {
    if (!valid_tool_name(config->tool_handlers[i].name) ||
        config->tool_handlers[i].handler == NULL) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    for (size_t j = 0; j < i; j++) {
      if (config->tool_handlers[i].name.len == config->tool_handlers[j].name.len &&
          memcmp(config->tool_handlers[i].name.data,
                 config->tool_handlers[j].name.data,
                 config->tool_handlers[i].name.len) == 0) {
        return GZC_ERR_INVALID_ARGUMENT;
      }
    }
  }
  const gzc_platform_t *platform = config->platform == NULL ? gzc_default_platform() : config->platform;
  if (platform->malloc == NULL || platform->realloc == NULL || platform->free == NULL ||
      platform->time_instant_ms == NULL) {
    return GZC_ERR_UNSUPPORTED;
  }
  size_t high_water = config->service_write_high_water_bytes;
  size_t low_water = config->service_write_low_water_bytes;
  if (high_water == 0 && low_water == 0) {
    high_water = GZC_SERVICE_WRITE_HIGH_WATER_DEFAULT;
    low_water = GZC_SERVICE_WRITE_LOW_WATER_DEFAULT;
  } else if (high_water < GZC_SERVICE_WRITE_CHUNK_SIZE || low_water >= high_water) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_client_t *client = (gzc_client_t *)platform->malloc(platform->userdata, sizeof(*client));
  if (client == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(client, 0, sizeof(*client));
  client->config = *config;
  client->config.platform = platform;
  client->config.service_write_high_water_bytes = high_water;
  client->config.service_write_low_water_bytes = low_water;
  gzc_buf_init(&client->local_sdp);
  gzc_buf_init(&client->ice_endpoint);
  gzc_buf_init(&client->packet_rx);
  client->opus_rx_capacity = GZC_OPUS_RX_CAPACITY_DEFAULT;
  *out_client = client;
  return GZC_OK;
}

int gzc_client_set_peer_add_ice_server(gzc_client_t *client, gzc_peer_add_ice_server_fn fn) {
  if (client == NULL || client->peer != NULL || client->packet_channel != NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->peer_add_ice_server = fn;
  return GZC_OK;
}

int gzc_client_ice_endpoint(gzc_client_t *client, gzc_str_t *out_endpoint) {
  if (client == NULL || out_endpoint == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->ice_endpoint.len == 0) {
    return GZC_ERR_UNSUPPORTED;
  }
  *out_endpoint = gzc_str_from_parts((const char *)client->ice_endpoint.data, client->ice_endpoint.len);
  return GZC_OK;
}

int gzc_client_set_opus_rx_capacity(
    gzc_client_t *client,
    size_t capacity) {
  if (client == NULL || client->closed || client->peer != NULL ||
      client->packet_channel != NULL ||
      client->service_channels != NULL || client->opus_rx != NULL ||
      capacity == 0u ||
      capacity > SIZE_MAX / sizeof(gzc_opus_rx_slot_t)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->opus_rx_capacity = capacity;
  clear_opus_rx(client);
  return GZC_OK;
}

int gzc_client_discard_opus_rx(gzc_client_t *client) {
  if (client == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  clear_opus_rx(client);
  return GZC_OK;
}

int gzc_client_set_webrtc_media(
    gzc_client_t *client,
    const gzc_webrtc_media_vtable_t *media) {
  if (client == NULL || client->peer != NULL || client->packet_channel != NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (media != NULL &&
      (media->struct_size < offsetof(gzc_webrtc_media_vtable_t, peer_send_opus) +
                                sizeof(media->peer_send_opus) ||
       media->peer_set_opus_frame_callback == NULL ||
       media->peer_send_opus == NULL)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->media = media;
  return GZC_OK;
}

int gzc_client_connect(gzc_client_t *client) {
  if (client == NULL || client->closed) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->peer != NULL || client->packet_channel != NULL ||
      client->service_channels != NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->media == NULL) {
    return GZC_ERR_UNSUPPORTED;
  }
  int rc = allocate_opus_rx(client);
  if (rc != GZC_OK) {
    return rc;
  }
  client->has_local_sdp = false;
  client->packet_channel_open = false;
  gzc_buf_reset(&client->local_sdp);
  gzc_buf_reset(&client->packet_rx);
  clear_opus_rx(client);
  gzc_webrtc_callbacks_t callbacks;
  memset(&callbacks, 0, sizeof(callbacks));
  callbacks.userdata = client;
  callbacks.on_peer_state = on_peer_state;
  callbacks.on_local_sdp = on_local_sdp;
  callbacks.on_channel_state = on_channel_state;
  callbacks.on_channel_message = on_channel_message;
  callbacks.on_channel_buffered_amount_low = on_channel_buffered_amount_low;
  callbacks.on_remote_channel = on_remote_channel;

  rc = client->config.webrtc->peer_create(client->config.webrtc->userdata, &callbacks, &client->peer);
  if (rc != GZC_OK) {
    goto fail;
  }
  rc = client->media->peer_set_opus_frame_callback(
      client->peer, on_opus_frame, client);
  if (rc != GZC_OK) {
    (void)client->media->peer_set_opus_frame_callback(
        client->peer, NULL, NULL);
    goto fail;
  }
  client->media_callback_registered = true;

  gzc_rtc_channel_config_t packet_cfg;
  memset(&packet_cfg, 0, sizeof(packet_cfg));
  packet_cfg.label = gzc_str_from_cstr("giznet/v1/packet");
  packet_cfg.ordered = false;
  packet_cfg.reliable = false;
  rc = client->config.webrtc->peer_create_data_channel(client->peer, &packet_cfg, &client->packet_channel);
  if (rc != GZC_OK) {
    goto fail;
  }

  rc = create_service_channel(client, 0x20u, &client->event_channel);
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
  rc = wait_for_service_channel_open(
      client, client->event_channel, timeout);
  if (rc != GZC_OK) {
    goto fail;
  }
  return GZC_OK;

fail:
  if (client->peer != NULL && client->media_callback_registered) {
    (void)client->media->peer_set_opus_frame_callback(client->peer, NULL, NULL);
    client->media_callback_registered = false;
  }
  while (client->service_channels != NULL) {
    gzc_service_channel_close(client->service_channels);
  }
  client->event_channel = NULL;
  client->event_handle_open = false;
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
  client->has_local_sdp = false;
  gzc_buf_reset(&client->packet_rx);
  clear_opus_rx(client);
  return rc;
}

int gzc_client_close(gzc_client_t *client) {
  if (client == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->closed = true;
  if (client->event_handle != NULL) {
    gzc_event_stream_invalidate_internal(client->event_handle);
    client->event_handle = NULL;
    client->event_handle_open = false;
  }
  if (client->peer != NULL && client->media_callback_registered) {
    (void)client->media->peer_set_opus_frame_callback(client->peer, NULL, NULL);
    client->media_callback_registered = false;
  }
  clear_opus_rx(client);
  if (client->peer != NULL && client->config.webrtc->channel_close != NULL) {
    for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
      if (client->inbound_channels[i] != NULL) {
        client->config.webrtc->channel_close(client->inbound_channels[i]);
      }
    }
    while (client->service_channels != NULL) {
      gzc_service_channel_close(client->service_channels);
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
  client->packet_channel = NULL;
  while (client->service_channels != NULL) {
    gzc_service_channel_close(client->service_channels);
  }
  client->event_channel = NULL;
  client->event_handle_open = false;
  client->packet_channel_open = false;
  client->has_local_sdp = false;
  return GZC_OK;
}

int gzc_client_poll(gzc_client_t *client, int timeout_ms) {
  if (client == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->closed) {
    if (client->peer != NULL && client->service_write_depth == 0u) {
      (void)gzc_client_close(client);
    }
    return GZC_ERR_CLOSED;
  }
  if (client->peer == NULL) {
    return GZC_ERR_CLOSED;
  }
  if (client->config.webrtc == NULL || client->config.webrtc->peer_poll == NULL) {
    return GZC_ERR_UNSUPPORTED;
  }
  client->dispatch_error = GZC_OK;
  expire_pending_rpc_requests(client);
  if (client->service_write_depth == 0u) {
    release_terminal_rpc_requests(client);
  }
  for (gzc_service_channel_t *channel = client->service_channels;
       channel != NULL; channel = channel->next) {
    if (channel->rpc_request != NULL) {
      int progress_rc =
          gzc_rpc_request_progress_internal(channel->rpc_request);
      if (progress_rc != GZC_OK && progress_rc != GZC_ERR_WOULD_BLOCK) {
        gzc_rpc_request_transport_error_internal(channel->rpc_request,
                                                 progress_rc);
      }
    }
  }
  int backend_timeout_ms = timeout_ms;
  for (gzc_service_channel_t *channel = client->service_channels;
       channel != NULL; channel = channel->next) {
    backend_timeout_ms = gzc_rpc_request_backend_timeout_ms_internal(
        channel->rpc_request, backend_timeout_ms);
    if (backend_timeout_ms == 0) {
      break;
    }
  }
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    backend_timeout_ms =
        gzc_rpc_inbound_backend_timeout_ms(client->inbound[i], backend_timeout_ms);
    if (backend_timeout_ms == 0) {
      break;
    }
  }
  int rc = client->config.webrtc->peer_poll(client->peer, backend_timeout_ms);
  expire_pending_rpc_requests(client);
  if (rc != GZC_OK) {
    fail_pending_rpc_requests(client, rc);
  }
  if (client->service_write_depth == 0u) {
    release_terminal_rpc_requests(client);
  }
  if (rc != GZC_OK) {
    return rc;
  }
  for (gzc_service_channel_t *channel = client->service_channels;
       channel != NULL; channel = channel->next) {
    if (channel->rpc_request != NULL) {
      int progress_rc =
          gzc_rpc_request_progress_internal(channel->rpc_request);
      if (progress_rc != GZC_OK && progress_rc != GZC_ERR_WOULD_BLOCK) {
        gzc_rpc_request_transport_error_internal(channel->rpc_request,
                                                 progress_rc);
      }
    }
  }
  if (client->service_write_depth == 0u) {
    release_terminal_rpc_requests(client);
  }
  if (client->closed) {
    if (client->service_write_depth == 0u) {
      (void)gzc_client_close(client);
    }
    return GZC_ERR_CLOSED;
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

int gzc_client_dispatch_rpc_internal(
    gzc_client_t *client,
    int method,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata) {
  if (client == NULL || respond == NULL ||
      (request_payload.data == NULL && request_payload.len != 0u)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_TOOL_INVOKE) {
    gizclaw_rpc_v1_ToolInvokeRequest request =
        gizclaw_rpc_v1_ToolInvokeRequest_init_zero;
    pb_istream_t stream =
        pb_istream_from_buffer((const pb_byte_t *)request_payload.data,
                               request_payload.len);
    if (!pb_decode(&stream, gizclaw_rpc_v1_ToolInvokeRequest_fields, &request) ||
        request.invoke_name[0] == '\0') {
      const gzc_rpc_provider_response_t response = {
          .has_error = true,
          .error_code =
              gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
          .error_message = {.data = "invalid Tool request", .len = 20u},
      };
      return respond(respond_userdata, &response);
    }
    const size_t name_len = strlen(request.invoke_name);
    for (size_t i = 0; i < client->config.tool_handler_count; i++) {
      const gzc_tool_handler_t *registered =
          &client->config.tool_handlers[i];
      if (registered->name.len == name_len &&
          memcmp(registered->name.data, request.invoke_name, name_len) == 0) {
        return registered->handler(registered->userdata,
                                   request_payload,
                                   respond,
                                   respond_userdata);
      }
    }
    const gzc_rpc_provider_response_t response = {
        .has_error = true,
        .error_code =
            gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_METHOD_NOT_FOUND,
        .error_message = {.data = "Tool unavailable", .len = 16u},
    };
    return respond(respond_userdata, &response);
  }
  if (client->config.rpc_provider == NULL) {
    return GZC_ERR_UNSUPPORTED;
  }
  return client->config.rpc_provider(
      client->config.rpc_provider_userdata,
      method,
      request_payload,
      respond,
      respond_userdata);
}

void gzc_client_destroy(gzc_client_t *client) {
  if (client == NULL) {
    return;
  }
  const gzc_platform_t *platform = client->config.platform == NULL ? gzc_default_platform() : client->config.platform;
  (void)gzc_client_close(client);
  gzc_buf_free(&client->local_sdp, platform);
  gzc_buf_free(&client->ice_endpoint, platform);
  gzc_buf_free(&client->packet_rx, platform);
  if (client->opus_rx != NULL) {
    platform->free(platform->userdata, client->opus_rx);
    client->opus_rx = NULL;
  }
  platform->free(platform->userdata, client);
}

const gzc_platform_t *gzc_client_platform(gzc_client_t *client) {
  return client == NULL ? NULL : client->config.platform;
}

const gzc_webrtc_vtable_t *gzc_client_webrtc(gzc_client_t *client) {
  return client == NULL ? NULL : client->config.webrtc;
}

int gzc_client_acquire_event_channel_internal(
    gzc_client_t *client,
    gzc_event_stream_t *stream,
    gzc_service_channel_t **out_channel) {
  if (client == NULL || stream == NULL || out_channel == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_channel = NULL;
  if (client->closed || client->event_channel == NULL ||
      !client->event_channel->open || client->event_channel->closed) {
    return GZC_ERR_CLOSED;
  }
  if (client->event_handle_open) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->event_handle_open = true;
  client->event_handle = stream;
  *out_channel = client->event_channel;
  return GZC_OK;
}

void gzc_client_release_event_channel_internal(
    gzc_client_t *client,
    gzc_event_stream_t *stream,
    gzc_service_channel_t *channel) {
  if (client != NULL && stream != NULL && channel != NULL &&
      client->event_handle == stream &&
      client->event_channel == channel) {
    client->event_handle_open = false;
    client->event_handle = NULL;
  }
}

int64_t gzc_client_instant_ms_internal(gzc_client_t *client) {
  return client == NULL ? 0 : now_ms(client);
}

int gzc_client_write_timeout_ms_internal(gzc_client_t *client) {
  return client == NULL ? 0 : client->config.write_timeout_ms;
}

int gzc_client_attach_rpc_request_internal(
    gzc_service_channel_t *channel,
    gzc_rpc_request_t *request) {
  if (channel == NULL || request == NULL || channel->client == NULL ||
      channel->rpc_request != NULL || channel->closed || channel->rtc == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  channel->rpc_request = request;
  return GZC_OK;
}

void gzc_client_release_rpc_request_internal(
    gzc_client_t *client,
    gzc_service_channel_t *channel,
    gzc_rpc_request_t *request) {
  if (client == NULL || channel == NULL || request == NULL ||
      channel->client != client || channel->rpc_request != request) {
    return;
  }
  channel->rpc_request = NULL;
  gzc_rpc_request_detach_internal(request, channel);
  gzc_service_channel_close(channel);
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

static int create_service_channel(
    gzc_client_t *client,
    uint64_t service,
    gzc_service_channel_t **out_channel) {
  if (client == NULL || out_channel == NULL || client->peer == NULL || client->config.webrtc == NULL ||
      client->config.webrtc->peer_create_data_channel == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_channel = NULL;
  const gzc_platform_t *platform = client->config.platform == NULL ? gzc_default_platform() : client->config.platform;
  gzc_service_channel_t *channel = (gzc_service_channel_t *)platform->malloc(platform->userdata, sizeof(*channel));
  if (channel == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(channel, 0, sizeof(*channel));
  channel->client = client;
  channel->service = service;
  gzc_buf_init(&channel->rx);
  channel->next = client->service_channels;
  client->service_channels = channel;

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
  rc = configure_service_channel(client, channel->rtc);
  if (rc != GZC_OK) {
    gzc_service_channel_close(channel);
    return rc;
  }
  *out_channel = channel;
  return GZC_OK;
}

int gzc_client_open_service_channel(
    gzc_client_t *client,
    uint64_t service,
    int timeout_ms,
    gzc_service_channel_t **out_channel) {
  int rc = create_service_channel(client, service, out_channel);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_service_channel_t *channel = *out_channel;
  rc = wait_for_service_channel_open(client, channel, timeout_ms);
  if (rc != GZC_OK) {
    if (service_channel_is_tracked(client, channel)) {
      gzc_service_channel_close(channel);
    }
    *out_channel = NULL;
    return rc;
  }
  return GZC_OK;
}

int gzc_client_create_service_channel_internal(
    gzc_client_t *client,
    uint64_t service,
    gzc_service_channel_t **out_channel) {
  return create_service_channel(client, service, out_channel);
}

int gzc_service_channel_send_frame(gzc_service_channel_t *channel, const gzc_rpc_frame_t *frame) {
  if (channel == NULL || channel->client == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (channel->closed || channel->rtc == NULL) {
    return GZC_ERR_CLOSED;
  }
  if (!channel->open)
    return GZC_ERR_WOULD_BLOCK;
  gzc_client_t *client = channel->client;
  client->service_write_depth++;
  int rc = gzc_client_write_frame_internal(
      client, channel->rtc, frame, &channel->write_blocked);
  client->service_write_depth--;
  return rc;
}

int gzc_service_channel_try_write_bytes_internal(
    gzc_service_channel_t *channel,
    const uint8_t *data,
    size_t len,
    size_t *offset) {
  if (channel == NULL || channel->client == NULL || offset == NULL ||
      (data == NULL && len != 0u)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (channel->closed || channel->rtc == NULL) {
    return GZC_ERR_CLOSED;
  }
  if (!channel->open)
    return GZC_ERR_WOULD_BLOCK;
  return gzc_client_try_write_bytes_internal(
      channel->client, channel->rtc, data, len, offset,
      &channel->write_blocked, 1u);
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
  if (channel->rpc_request != NULL) {
    gzc_rpc_request_t *request = channel->rpc_request;
    channel->rpc_request = NULL;
    gzc_rpc_request_client_closed_internal(request);
    gzc_rpc_request_detach_internal(request, channel);
  }
  if (!channel->close_requested && client != NULL && client->peer != NULL && channel->rtc != NULL && client->config.webrtc != NULL &&
      client->config.webrtc->channel_close != NULL) {
    channel->close_requested = true;
    client->config.webrtc->channel_close(channel->rtc);
  }
  if (client != NULL) {
    gzc_service_channel_t **cursor = &client->service_channels;
    while (*cursor != NULL && *cursor != channel) {
      cursor = &(*cursor)->next;
    }
    if (*cursor == channel) {
      *cursor = channel->next;
    }
    if (client->event_channel == channel) {
      client->event_channel = NULL;
      client->event_handle_open = false;
      if (client->event_handle != NULL) {
        gzc_event_stream_invalidate_internal(client->event_handle);
        client->event_handle = NULL;
      }
    }
  }
  channel->client = NULL;
  channel->rtc = NULL;
  channel->closed = true;
  gzc_buf_free(&channel->rx, platform);
  platform->free(platform->userdata, channel);
}

int gzc_client_send_packet(gzc_client_t *client, uint8_t protocol, const uint8_t *payload, size_t len) {
  if (client == NULL || (payload == NULL && len != 0) ||
      !valid_packet_protocol(protocol)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->closed || client->peer == NULL) {
    return GZC_ERR_CLOSED;
  }
  if (protocol == GZC_PROTOCOL_OPUS_PACKET) {
    if (!valid_opus_packet(payload, len)) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    if (client->media == NULL) {
      return GZC_ERR_UNSUPPORTED;
    }
    return client->media->peer_send_opus(client->peer, payload, len);
  }
  if (client->packet_channel == NULL || !client->packet_channel_open ||
      client->config.webrtc == NULL ||
      client->config.webrtc->channel_send == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (len > GZC_RPC_MAX_FRAME_SIZE - 1) {
    return GZC_ERR_RPC;
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

typedef struct {
  const uint8_t *data;
  size_t len;
  size_t packet_message_size;
  uint8_t protocol;
  bool opus;
  bool alternate_after_consume;
} gzc_packet_view_t;

static int wait_packet_view(
    gzc_client_t *client,
    int timeout_ms,
    gzc_packet_view_t *out_view) {
  const int64_t start = now_ms(client);
  size_t message_size = 0;
  bool read_opus = false;
  bool alternate_after_consume = false;
  for (;;) {
    if (client->opus_rx_error != GZC_OK) {
      int rc = client->opus_rx_error;
      client->opus_rx_error = GZC_OK;
      return rc;
    }
    int rc = rx_next_packet_size(&client->packet_rx, &message_size);
    bool packet_ready = rc == GZC_OK;
    bool opus_ready = client->opus_rx_count != 0u;
    if (packet_ready || opus_ready) {
      if (packet_ready && opus_ready) {
        read_opus = client->read_opus_next;
        alternate_after_consume = true;
      } else {
        read_opus = opus_ready;
      }
      break;
    }
    if (rc != GZC_ERR_TIMEOUT) {
      return rc;
    }
    if (timeout_ms >= 0 && now_ms(client) - start >= timeout_ms) {
      return GZC_ERR_TIMEOUT;
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
  }
  memset(out_view, 0, sizeof(*out_view));
  out_view->opus = read_opus;
  out_view->alternate_after_consume = alternate_after_consume;
  if (read_opus) {
    const gzc_opus_rx_slot_t *slot =
        &client->opus_rx[client->opus_rx_head];
    out_view->protocol = GZC_PROTOCOL_OPUS_PACKET;
    out_view->data = slot->data;
    out_view->len = slot->len;
    return GZC_OK;
  }
  size_t payload_len = message_size - 3;
  uint8_t protocol = client->packet_rx.data[2];
  if (!valid_packet_protocol(protocol)) {
    consume_rx(&client->packet_rx, message_size);
    return GZC_ERR_INVALID_ARGUMENT;
  }
  out_view->protocol = protocol;
  out_view->data = client->packet_rx.data + 3;
  out_view->len = payload_len;
  out_view->packet_message_size = message_size;
  return GZC_OK;
}

static void consume_packet_view(
    gzc_client_t *client,
    const gzc_packet_view_t *view) {
  if (view->opus) {
    client->opus_rx_head =
        (client->opus_rx_head + 1u) % client->opus_rx_capacity;
    client->opus_rx_count--;
  } else {
    consume_rx(&client->packet_rx, view->packet_message_size);
  }
  if (view->alternate_after_consume) {
    client->read_opus_next = !client->read_opus_next;
  }
}

int gzc_client_read_packet(
    gzc_client_t *client,
    int timeout_ms,
    uint8_t *out_protocol,
    gzc_buf_t *out_payload) {
  if (client == NULL || out_protocol == NULL || out_payload == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_packet_view_t view;
  int rc = wait_packet_view(client, timeout_ms, &view);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_buf_reset(out_payload);
  rc = gzc_buf_append(
      out_payload, client->config.platform, view.data, view.len);
  if (rc != GZC_OK) {
    return rc;
  }
  *out_protocol = view.protocol;
  consume_packet_view(client, &view);
  return GZC_OK;
}

int gzc_client_read_packet_into(
    gzc_client_t *client,
    int timeout_ms,
    uint8_t *out_protocol,
    uint8_t *out_payload,
    size_t payload_capacity,
    size_t *out_payload_len) {
  if (client == NULL || out_protocol == NULL || out_payload_len == NULL ||
      (out_payload == NULL && payload_capacity != 0u)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_packet_view_t view;
  int rc = wait_packet_view(client, timeout_ms, &view);
  if (rc != GZC_OK) {
    return rc;
  }
  *out_protocol = view.protocol;
  *out_payload_len = view.len;
  if (view.len > payload_capacity) {
    return GZC_ERR_BUFFER_TOO_SMALL;
  }
  if (view.len != 0u) {
    memcpy(out_payload, view.data, view.len);
  }
  consume_packet_view(client, &view);
  return GZC_OK;
}
