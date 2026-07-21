#include "gzc.h"
#include "pb_decode.h"
#include "pb_encode.h"

#include <stdio.h>
#include <string.h>

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
  FAKE_RESPONSE_SPEECH_SYNTHESIZE = 6
} fake_response_mode_t;

typedef struct {
  gzc_webrtc_callbacks_t callbacks;
  struct gzc_rtc_peer peer;
  struct gzc_rtc_channel packet_channel;
  struct gzc_rtc_channel rpc_channel;
  struct gzc_rtc_channel service_channels[16];
  struct gzc_rtc_channel edge_channel;
  struct gzc_rtc_channel remote_channels[GZC_RPC_MAX_INBOUND_CHANNELS + 1u];
  gzc_buf_t sent;
  const gzc_platform_t *platform;
  int poll_count;
  int create_channel_count;
  int close_count;
  gzc_rtc_channel_t *last_closed;
  int ice_server_count;
  bool offer_started;
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

static fake_webrtc_t *global_fake_webrtc;

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
  if (global_fake_webrtc != NULL) {
    global_fake_webrtc->close_count++;
    global_fake_webrtc->last_closed = channel;
  }
}

static void fake_peer_close(gzc_rtc_peer_t *peer) {
  (void)peer;
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
    size_t service_index = fake->create_channel_count < 2
                               ? 0
                               : (size_t)(fake->create_channel_count - 2);
    if (service_index >= 16) {
      return GZC_ERR_NO_MEMORY;
    }
    gzc_rtc_channel_t *channel = fake->create_channel_count < 2
                                     ? &fake->rpc_channel
                                     : &fake->service_channels[service_index];
    *out_channel = channel;
    if (fake->callbacks.on_channel_state != NULL) {
      gzc_rtc_channel_info_t info;
      memset(&info, 0, sizeof(info));
      info.label = gzc_str_from_cstr("giznet/v1/service/0");
      info.stream_id = 1;
      info.ordered = true;
      info.reliable = true;
      fake->callbacks.on_channel_state(fake->callbacks.userdata, peer, channel, &info, GZC_RTC_CHANNEL_OPEN);
    }
  } else if (config->label.len == strlen("giznet/v1/service/49") &&
             strncmp(config->label.data, "giznet/v1/service/49", config->label.len) == 0) {
    if (!config->ordered || !config->reliable) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    *out_channel = &fake->edge_channel;
    if (fake->callbacks.on_channel_state != NULL) {
      gzc_rtc_channel_info_t info;
      memset(&info, 0, sizeof(info));
      info.label = gzc_str_from_cstr("giznet/v1/service/49");
      info.stream_id = 2;
      info.ordered = true;
      info.reliable = true;
      fake->callbacks.on_channel_state(fake->callbacks.userdata, peer, &fake->edge_channel, &info, GZC_RTC_CHANNEL_OPEN);
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

static int test_channel_send(gzc_rtc_channel_t *channel, const uint8_t *data, size_t len, bool is_text) {
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
  bool known_channel = channel == &fake->rpc_channel || channel == &fake->edge_channel;
  for (size_t i = 0; i < 16 && !known_channel; i++) {
    known_channel = channel == &fake->service_channels[i];
  }
  if (!known_channel || is_text) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_rtc_channel_t *response_channel = channel;
  gzc_rpc_frame_t request_frame;
  bool is_eos = gzc_rpc_frame_decode(data, len, &request_frame) == GZC_OK &&
                request_frame.type == GZC_RPC_FRAME_EOS;
  if (fake->response_mode == FAKE_RESPONSE_SPEECH_TRANSCRIBE && !is_eos) {
    return gzc_buf_append(&fake->sent, fake->platform, data, len);
  }
  if (is_eos && fake->response_mode != FAKE_RESPONSE_SPEECH_TRANSCRIBE) {
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
  fake->callbacks.on_channel_state(
      fake->callbacks.userdata, &fake->peer, &fake->remote_channels[index], &info,
      GZC_RTC_CHANNEL_OPEN);
}

static void close_remote_rpc(fake_webrtc_t *fake, size_t index) {
  gzc_rtc_channel_info_t info;
  memset(&info, 0, sizeof(info));
  info.label = gzc_str_from_cstr("giznet/v1/service/0");
  info.stream_id = (uint16_t)(3 + index);
  info.ordered = true;
  info.reliable = true;
  fake->callbacks.on_channel_state(
      fake->callbacks.userdata, &fake->peer, &fake->remote_channels[index], &info,
      GZC_RTC_CHANNEL_CLOSED);
}

typedef struct {
  size_t envelope_count;
  size_t frame_count;
  size_t binary_bytes;
} stream_count_t;

typedef struct {
  int code;
  size_t frame_count;
  bool has_error;
  bool message_ok;
} stream_error_t;

static int count_stream_frame(void *userdata, const gzc_rpc_frame_t *frame) {
  stream_count_t *count = (stream_count_t *)userdata;
  if (count == NULL || frame == NULL) {
    return GZC_ERR_RPC;
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

static int count_speech_audio(void *userdata, const uint8_t *data, size_t len) {
  stream_count_t *count = (stream_count_t *)userdata;
  if (count == NULL || data == NULL || len == 0) {
    return GZC_ERR_RPC;
  }
  count->frame_count++;
  count->binary_bytes += len;
  return GZC_OK;
}

static int capture_stream_error_frame(void *userdata, const gzc_rpc_frame_t *frame) {
  stream_error_t *captured = (stream_error_t *)userdata;
  if (captured == NULL || frame == NULL || frame->type != GZC_RPC_FRAME_BINARY) {
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
                           ? "{\"protocol\":\"gizclaw-webrtc\",\"public_key\":\"8mfzTdZB1JA43QmNAMWfTfkj5GC9TJxJFveThi9tvK6J\",\"signaling_path\":\"/custom/offer\",\"ice_servers\":[{\"urls\":[\"turn:edge.example.com:3478?transport=udp\"],\"username\":\"edge\\\"node\",\"credential\":\"sec\\\\ret\"}]}"
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
  gizclaw_rpc_v1_Workspace workspace = gizclaw_rpc_v1_Workspace_init_default;
  workspace.has_owner_public_key = true;
  strcpy(workspace.owner_public_key, "peer-public-key");
  uint8_t workspace_bytes[128];
  pb_ostream_t workspace_output =
      pb_ostream_from_buffer(workspace_bytes, sizeof(workspace_bytes));
  if (expect(pb_encode(&workspace_output, gizclaw_rpc_v1_Workspace_fields,
                       &workspace),
             "encode workspace owner public key") != 0) {
    return 1;
  }
  gizclaw_rpc_v1_Workspace decoded_workspace =
      gizclaw_rpc_v1_Workspace_init_default;
  pb_istream_t workspace_input =
      pb_istream_from_buffer(workspace_bytes, workspace_output.bytes_written);
  if (expect(pb_decode(&workspace_input, gizclaw_rpc_v1_Workspace_fields,
                       &decoded_workspace),
             "decode workspace owner public key") != 0 ||
      expect(decoded_workspace.has_owner_public_key &&
                 strcmp(decoded_workspace.owner_public_key,
                        "peer-public-key") == 0,
             "workspace owner public key uses bounded nanopb storage") != 0) {
    return 1;
  }

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
  rc = gzc_client_set_peer_add_ice_server(client, test_peer_add_ice_server);
  if (expect(rc == GZC_OK, "client ICE hook") != 0) {
    gzc_client_destroy(client);
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
  if (expect(fake_webrtc.ice_server_count == 1, "server-info ICE server applied before offer") != 0) {
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
  rc = gzc_rpc_call(
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
  if (expect(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PET_PIXA_DOWNLOAD == 88, "pet pixa method id value") != 0) {
    return 1;
  }
  if (expect(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_BADGE_DEF_PIXA_DOWNLOAD == 65, "badge pixa method id value") != 0) {
    return 1;
  }

  gzc_buf_reset(&fake_webrtc.sent);
  int create_channel_count_before_edge = fake_webrtc.create_channel_count;
  memset(&response, 0, sizeof(response));
  rc = gzc_rpc_call(
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
  rc = gzc_rpc_call(
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

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO_OVERSIZED_CONTINUATION;
  memset(&response, 0, sizeof(response));
  rc = gzc_rpc_call(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING,
      gzc_str_from_parts((const char *)params.data, params.len),
      &response);
  if (expect(rc == GZC_ERR_RPC, "rpc call rejects oversized continuation response") != 0) {
    return 1;
  }
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
  rc = gzc_rpc_call_stream(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
      gzc_str_from_parts((const char *)params.data, params.len),
      count_stream_frame,
      &stream_count);
  if (expect(rc == GZC_OK, "rpc call stream") != 0) {
    return 1;
  }
  if (expect(stream_count.envelope_count == 1 && stream_count.frame_count == 2 && stream_count.binary_bytes == 3, "stream frames counted") != 0) {
    return 1;
  }

  gizclaw_rpc_v1_SpeechTranscribeRequest transcribe_request =
      gizclaw_rpc_v1_SpeechTranscribeRequest_init_zero;
  strcpy(transcribe_request.model_alias, "asr-main");
  strcpy(
      transcribe_request.content_type,
      "audio/L16;rate=16000;channels=1");
  gzc_rpc_speech_upload_t *upload = NULL;
  fake_webrtc.response_mode = FAKE_RESPONSE_SPEECH_TRANSCRIBE;
  rc = gzc_rpc_speech_transcribe_open(client, &transcribe_request, &upload);
  if (expect(rc == GZC_OK && upload != NULL, "speech transcribe open") != 0) {
    return 1;
  }
  const uint8_t speech_input[] = {0x01, 0x02, 0x03, 0x04};
  rc = gzc_rpc_speech_transcribe_write(upload, speech_input, sizeof(speech_input));
  if (expect(rc == GZC_OK, "speech transcribe write") != 0) {
    gzc_rpc_speech_transcribe_cancel(upload);
    return 1;
  }
  gizclaw_rpc_v1_SpeechTranscribeResponse transcription =
      gizclaw_rpc_v1_SpeechTranscribeResponse_init_zero;
  gzc_rpc_error_t speech_error;
  rc = gzc_rpc_speech_transcribe_finish(upload, &transcription, &speech_error);
  if (expect(rc == GZC_OK && strcmp(transcription.transcript, "hello") == 0,
             "speech transcribe finish") != 0) {
    return 1;
  }

  gizclaw_rpc_v1_SpeechSynthesizeRequest synthesize_request =
      gizclaw_rpc_v1_SpeechSynthesizeRequest_init_zero;
  strcpy(synthesize_request.voice_alias, "narrator");
  strcpy(synthesize_request.text, "hello");
  synthesize_request.accepted_content_types_count = 1;
  strcpy(synthesize_request.accepted_content_types[0], "audio/pcm");
  gizclaw_rpc_v1_SpeechSynthesizeResponse synthesis_metadata =
      gizclaw_rpc_v1_SpeechSynthesizeResponse_init_zero;
  memset(&stream_count, 0, sizeof(stream_count));
  fake_webrtc.response_mode = FAKE_RESPONSE_SPEECH_SYNTHESIZE;
  rc = gzc_rpc_speech_synthesize(
      client,
      &synthesize_request,
      &synthesis_metadata,
      count_speech_audio,
      &stream_count,
      &speech_error);
  if (expect(
          rc == GZC_OK &&
              strcmp(synthesis_metadata.content_type, "audio/pcm") == 0 &&
              stream_count.frame_count == 2 && stream_count.binary_bytes == 3,
          "speech synthesis stream") != 0) {
    return 1;
  }

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO_ERROR;
  memset(&speech_error, 0, sizeof(speech_error));
  rc = gzc_rpc_speech_synthesize(
      client,
      &synthesize_request,
      &synthesis_metadata,
      count_speech_audio,
      &stream_count,
      &speech_error);
  if (expect(
          rc == GZC_ERR_RPC && speech_error.code == 7 &&
              str_eq_cstr(speech_error.message, "denied"),
          "speech synthesis preserves RPC error storage") != 0) {
    return 1;
  }

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO;
  upload = NULL;
  rc = gzc_rpc_speech_transcribe_open(NULL, &transcribe_request, &upload);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "speech transcribe public API") != 0) {
    return 1;
  }
  rc = gzc_rpc_speech_synthesize(
      NULL,
      &synthesize_request,
      &synthesis_metadata,
      count_speech_audio,
      &stream_count,
      &speech_error);
  if (expect(rc == GZC_ERR_INVALID_ARGUMENT, "speech synthesis public API") != 0) {
    return 1;
  }

  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO_OVERSIZED_CONTINUATION;
  memset(&stream_count, 0, sizeof(stream_count));
  rc = gzc_rpc_call_stream(
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
  rc = gzc_rpc_call_stream(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
      gzc_str_from_parts((const char *)params.data, params.len),
      capture_stream_error_frame,
      &stream_error);
  if (expect(rc == GZC_OK, "rpc call stream returns ok with delivered error envelope") != 0) {
    return 1;
  }
  if (expect(
          stream_error.frame_count == 1 && stream_error.has_error && stream_error.code == 7 && stream_error.message_ok,
          "stream error envelope delivered to callback") != 0) {
    return 1;
  }
  fake_webrtc.response_mode = FAKE_RESPONSE_PROTO;

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
  rc = gzc_rpc_call(
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
  speed_request.down_content_length = 3;
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
  if (expect(rc == GZC_OK && response_frames == 5 && saw_response_delimiter &&
                 saw_response_eos && download_bytes == 3 && !inbound_response.has_error &&
                 inbound_response.id.len == oversized_id_len &&
                 memcmp(inbound_response.id.data, oversized_id, oversized_id_len) == 0,
             "continued inbound speed response envelope body and eos") != 0) {
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

  gzc_buf_free(&speed_payload, platform);
  gzc_buf_free(&inbound_request, platform);
  gzc_buf_free(&inbound_framed, platform);

  gzc_buf_free(&params, platform);
  gzc_buf_free(&fake_webrtc.sent, platform);
  rc = gzc_client_close(client);
  if (expect(rc == GZC_OK && gzc_client_poll(client, 0) == GZC_ERR_CLOSED,
             "poll reports closed client") != 0) {
    return 1;
  }
  gzc_client_destroy(client);

  fake_webrtc_t fake_webrtc_no_ice_hook;
  memset(&fake_webrtc_no_ice_hook, 0, sizeof(fake_webrtc_no_ice_hook));
  fake_webrtc_no_ice_hook.platform = platform;
  gzc_buf_init(&fake_webrtc_no_ice_hook.sent);

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
    return 1;
  }
  rc = gzc_client_connect(client_no_ice_hook);
  if (expect(rc == GZC_ERR_UNSUPPORTED, "client connect without ICE hook rejects advertised ICE metadata") != 0) {
    gzc_client_destroy(client_no_ice_hook);
    gzc_buf_free(&fake_webrtc_no_ice_hook.sent, platform);
    return 1;
  }
  if (expect(fake_webrtc_no_ice_hook.ice_server_count == 0, "missing ICE hook skips advertised ICE servers") != 0) {
    gzc_client_destroy(client_no_ice_hook);
    gzc_buf_free(&fake_webrtc_no_ice_hook.sent, platform);
    return 1;
  }
  gzc_client_destroy(client_no_ice_hook);
  gzc_buf_free(&fake_webrtc_no_ice_hook.sent, platform);
  return 0;
}
