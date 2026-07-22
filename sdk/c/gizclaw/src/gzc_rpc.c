#include "gzc_rpc.h"

#include "pb_decode.h"
#include "pb_encode.h"

#include <stdint.h>
#include <string.h>

#define GZC_RPC_MAX_ENVELOPE_SIZE (GZC_RPC_MAX_FRAME_SIZE * 16u)
/* Keep client reads beyond the default server deadlines by a transport grace window. */
#define GZC_RPC_SPEECH_TRANSCRIPTION_TIMEOUT_MS 80000
#define GZC_RPC_SPEECH_SYNTHESIS_TIMEOUT_MS 125000
#define GZC_SERVICE_PEER_RPC 0x00u
#define GZC_SERVICE_EDGE_RPC 0x31u

int gzc_client_reset_rpc_rx_internal(gzc_client_t *client);
int gzc_client_open_rpc_channel_internal(gzc_client_t *client, int timeout_ms);
void gzc_client_close_rpc_channel_internal(gzc_client_t *client);
int gzc_client_read_rpc_frame_internal(gzc_client_t *client, int timeout_ms, gzc_buf_t *out_frame_bytes);
int gzc_client_store_rpc_response_internal(gzc_client_t *client, const uint8_t *data, size_t len, gzc_str_t *out_payload);
int gzc_client_write_rpc_frame_internal(gzc_client_t *client, const gzc_rpc_frame_t *frame);
int64_t gzc_client_instant_ms_internal(gzc_client_t *client);
int gzc_client_write_timeout_ms_internal(gzc_client_t *client);
int gzc_client_try_write_bytes_internal(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    const uint8_t *data,
    size_t len,
    size_t *offset,
    bool *blocked,
    size_t max_chunks);

typedef struct {
  const uint8_t *data;
  size_t len;
} gzc_pb_bytes_arg_t;

typedef struct {
  gzc_str_t *out;
} gzc_pb_view_arg_t;

struct gzc_rpc_speech_upload {
  gzc_client_t *client;
  const gzc_platform_t *platform;
  gzc_service_channel_t *channel;
};

static int append_envelope_continuation(gzc_buf_t *envelope, const gzc_platform_t *platform, const gzc_rpc_frame_t *frame) {
  if (envelope == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (frame->len > GZC_RPC_MAX_ENVELOPE_SIZE || envelope->len > GZC_RPC_MAX_ENVELOPE_SIZE - frame->len) {
    return GZC_ERR_RPC;
  }
  return gzc_buf_append(envelope, platform, frame->data, frame->len);
}

static bool encode_pb_bytes(pb_ostream_t *stream, const pb_field_t *field, void *const *arg) {
  const gzc_pb_bytes_arg_t *bytes = (const gzc_pb_bytes_arg_t *)(*arg);
  size_t len = bytes != NULL ? bytes->len : 0;
  if (bytes == NULL || len == 0) {
    return pb_encode_tag_for_field(stream, field) && pb_encode_string(stream, (const uint8_t *)"", 0);
  }
  if (bytes->data == NULL) {
    return false;
  }
  const uint8_t *data = bytes->data;
  return pb_encode_tag_for_field(stream, field) && pb_encode_string(stream, data, len);
}

static int encode_pb_message(
    const gzc_platform_t *platform,
    const pb_msgdesc_t *fields,
    const void *message,
    gzc_buf_t *out_payload) {
  pb_ostream_t sizing = PB_OSTREAM_SIZING;
  if (!pb_encode(&sizing, fields, message)) {
    return GZC_ERR_RPC;
  }
  size_t size = sizing.bytes_written;
  uint8_t *buf = (uint8_t *)platform->malloc(platform->userdata, size == 0 ? 1 : size);
  if (buf == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  pb_ostream_t stream = pb_ostream_from_buffer(buf, size);
  int rc = GZC_OK;
  if (!pb_encode(&stream, fields, message)) {
    rc = GZC_ERR_RPC;
  } else {
    rc = gzc_buf_append(out_payload, platform, buf, size);
  }
  platform->free(platform->userdata, buf);
  return rc;
}

static bool decode_pb_view(pb_istream_t *stream, const pb_field_t *field, void **arg) {
  (void)field;
  gzc_pb_view_arg_t *view = (gzc_pb_view_arg_t *)(*arg);
  if (view == NULL || view->out == NULL || (stream->state == NULL && stream->bytes_left != 0)) {
    return false;
  }
  *view->out = gzc_str_from_parts((const char *)stream->state, stream->bytes_left);
  if (!pb_read(stream, NULL, stream->bytes_left)) {
    return false;
  }
  return true;
}

static bool decode_pb_string_view(pb_istream_t *stream, pb_wire_type_t wire_type, gzc_str_t *out) {
  if (wire_type != PB_WT_STRING) {
    return false;
  }
  pb_istream_t substream;
  if (!pb_make_string_substream(stream, &substream)) {
    return false;
  }
  *out = gzc_str_from_parts((const char *)substream.state, substream.bytes_left);
  return pb_read(&substream, NULL, substream.bytes_left) &&
         pb_close_string_substream(stream, &substream);
}

static int decode_frame_bytes(gzc_buf_t *frame_bytes, gzc_rpc_frame_t *out_frame) {
  if (frame_bytes == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzc_rpc_frame_decode(frame_bytes->data, frame_bytes->len, out_frame);
}

static int read_frame(gzc_client_t *client, const gzc_platform_t *platform, int timeout_ms, gzc_buf_t *frame_bytes, gzc_rpc_frame_t *out_frame) {
  int rc = gzc_client_read_rpc_frame_internal(client, timeout_ms, frame_bytes);
  if (rc != GZC_OK) {
    return rc;
  }
  (void)platform;
  return decode_frame_bytes(frame_bytes, out_frame);
}

static int read_service_frame(gzc_service_channel_t *channel, int timeout_ms, gzc_buf_t *frame_bytes, gzc_rpc_frame_t *out_frame) {
  int rc = gzc_service_channel_read_frame(channel, timeout_ms, frame_bytes);
  if (rc != GZC_OK) {
    return rc;
  }
  return decode_frame_bytes(frame_bytes, out_frame);
}

static void close_rpc_channel_on_error(gzc_client_t *client, int rc) {
  if (rc != GZC_OK) {
    gzc_client_close_rpc_channel_internal(client);
  }
}

static bool is_edge_rpc_method(gizclaw_rpc_v1_RpcMethod method) {
  return method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PEER_LOOKUP ||
         method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_PEER_ASSIGN ||
         method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_ROUTE_RESOLVE;
}

static int send_request_envelope(
    gzc_client_t *client,
    const gzc_platform_t *platform,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload) {
  gzc_buf_t request;
  gzc_buf_init(&request);
  int rc = gzc_rpc_encode_request_envelope(platform, gzc_str_from_cstr("1"), method, params_payload, &request);
  if (rc == GZC_OK) {
    rc = gzc_client_reset_rpc_rx_internal(client);
  }
  size_t offset = 0;
  while (rc == GZC_OK && offset < request.len) {
    size_t chunk = request.len - offset;
    if (chunk > GZC_RPC_MAX_FRAME_SIZE) {
      chunk = GZC_RPC_MAX_FRAME_SIZE;
    }
    gzc_rpc_frame_t frame;
    memset(&frame, 0, sizeof(frame));
    frame.type = request.len <= GZC_RPC_MAX_FRAME_SIZE ? GZC_RPC_FRAME_BINARY : GZC_RPC_FRAME_TEXT;
    frame.data = request.data + offset;
    frame.len = chunk;
    rc = gzc_client_write_rpc_frame_internal(client, &frame);
    offset += chunk;
  }
  if (rc == GZC_OK) {
    gzc_rpc_frame_t eos;
    memset(&eos, 0, sizeof(eos));
    eos.type = GZC_RPC_FRAME_EOS;
    rc = gzc_client_write_rpc_frame_internal(client, &eos);
  }
  gzc_buf_free(&request, platform);
  return rc;
}

static int send_request_envelope_service(
    gzc_service_channel_t *channel,
    const gzc_platform_t *platform,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    bool end_request) {
  gzc_buf_t request;
  gzc_buf_init(&request);
  int rc = gzc_rpc_encode_request_envelope(platform, gzc_str_from_cstr("1"), method, params_payload, &request);
  if (rc == GZC_OK) {
    if (request.len <= GZC_RPC_MAX_FRAME_SIZE) {
      gzc_rpc_frame_t frame;
      memset(&frame, 0, sizeof(frame));
      frame.type = GZC_RPC_FRAME_BINARY;
      frame.data = request.data;
      frame.len = request.len;
      rc = gzc_service_channel_send_frame(channel, &frame);
    } else {
      size_t offset = 0;
      while (offset < request.len && rc == GZC_OK) {
        size_t chunk = request.len - offset;
        if (chunk > GZC_RPC_MAX_FRAME_SIZE) {
          chunk = GZC_RPC_MAX_FRAME_SIZE;
        }
        gzc_rpc_frame_t frame;
        memset(&frame, 0, sizeof(frame));
        frame.type = GZC_RPC_FRAME_TEXT;
        frame.data = request.data + offset;
        frame.len = chunk;
        rc = gzc_service_channel_send_frame(channel, &frame);
        offset += chunk;
      }
    }
  }
  if (rc == GZC_OK && end_request) {
    gzc_rpc_frame_t eos;
    memset(&eos, 0, sizeof(eos));
    eos.type = GZC_RPC_FRAME_EOS;
    rc = gzc_service_channel_send_frame(channel, &eos);
  }
  gzc_buf_free(&request, platform);
  return rc;
}

int gzc_rpc_encode_request_envelope(
    const gzc_platform_t *platform,
    gzc_str_t id,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_buf_t *out_payload) {
  if (out_payload == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (platform == NULL) {
    platform = gzc_default_platform();
  }
  if (platform->malloc == NULL || platform->free == NULL || method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_UNSPECIFIED) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if ((id.data == NULL && id.len != 0) || (params_payload.data == NULL && params_payload.len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_pb_bytes_arg_t id_arg = {(const uint8_t *)id.data, id.len};
  gzc_pb_bytes_arg_t payload_arg = {(const uint8_t *)params_payload.data, params_payload.len};
  gizclaw_rpc_v1_RpcRequest request = gizclaw_rpc_v1_RpcRequest_init_zero;
  request.id.funcs.encode = encode_pb_bytes;
  request.id.arg = &id_arg;
  request.method = method;
  request.payload.funcs.encode = encode_pb_bytes;
  request.payload.arg = &payload_arg;
  return encode_pb_message(platform, gizclaw_rpc_v1_RpcRequest_fields, &request, out_payload);
}

int gzc_rpc_decode_response_envelope(gzc_str_t response_payload, gzc_rpc_response_t *out_response) {
  if (out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_response, 0, sizeof(*out_response));
  if (response_payload.data == NULL && response_payload.len != 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  pb_istream_t stream = pb_istream_from_buffer((const pb_byte_t *)response_payload.data, response_payload.len);
  while (stream.bytes_left > 0) {
    pb_wire_type_t wire_type;
    uint32_t tag;
    bool eof = false;
    if (!pb_decode_tag(&stream, &wire_type, &tag, &eof) || eof) {
      return GZC_ERR_RPC;
    }
    if (tag == gizclaw_rpc_v1_RpcResponse_id_tag) {
      if (!decode_pb_string_view(&stream, wire_type, &out_response->id)) {
        return GZC_ERR_RPC;
      }
    } else if (tag == gizclaw_rpc_v1_RpcResponse_payload_tag) {
      if (!decode_pb_string_view(&stream, wire_type, &out_response->result_payload)) {
        return GZC_ERR_RPC;
      }
    } else if (tag == gizclaw_rpc_v1_RpcResponse_error_tag) {
      if (wire_type != PB_WT_STRING) {
        return GZC_ERR_RPC;
      }
      pb_istream_t error_stream;
      if (!pb_make_string_substream(&stream, &error_stream)) {
        return GZC_ERR_RPC;
      }
      gzc_pb_view_arg_t message_arg = {&out_response->error.message};
      gizclaw_rpc_v1_RpcError error = gizclaw_rpc_v1_RpcError_init_zero;
      error.message.funcs.decode = decode_pb_view;
      error.message.arg = &message_arg;
      if (!pb_decode(&error_stream, gizclaw_rpc_v1_RpcError_fields, &error) ||
          !pb_close_string_substream(&stream, &error_stream)) {
        return GZC_ERR_RPC;
      }
      out_response->has_error = true;
      out_response->error.code = (int)error.code;
    } else if (!pb_skip_field(&stream, wire_type)) {
      return GZC_ERR_RPC;
    }
  }
  return GZC_OK;
}

int gzc_rpc_call_service(
    gzc_client_t *client,
    uint64_t service,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_rpc_response_t *out_response) {
  if (client == NULL || out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  if (platform == NULL) {
    return GZC_ERR_CLOSED;
  }
  gzc_service_channel_t *channel = NULL;
  int rc = gzc_client_open_service_channel(client, service, 5000, &channel);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = send_request_envelope_service(channel, platform, method, params_payload, true);
  if (rc != GZC_OK) {
    gzc_service_channel_close(channel);
    return rc;
  }

  gzc_buf_t frame_bytes;
  gzc_buf_t envelope;
  gzc_buf_init(&frame_bytes);
  gzc_buf_init(&envelope);
  gzc_rpc_frame_t frame;
  bool saw_response = false;
  bool saw_continuation = false;
  for (;;) {
    rc = read_service_frame(channel, 5000, &frame_bytes, &frame);
    if (rc != GZC_OK) {
      break;
    }
    if (frame.type == GZC_RPC_FRAME_EOS) {
      if (saw_continuation && !saw_response) {
        gzc_str_t response_payload;
        rc = gzc_client_store_rpc_response_internal(client, envelope.data, envelope.len, &response_payload);
        if (rc != GZC_OK) {
          break;
        }
        rc = gzc_rpc_decode_response_envelope(response_payload, out_response);
        if (rc != GZC_OK) {
          break;
        }
        saw_response = true;
      }
      rc = saw_response ? GZC_OK : GZC_ERR_RPC;
      break;
    }
    if (frame.type == GZC_RPC_FRAME_TEXT) {
      if (saw_response) {
        rc = GZC_ERR_RPC;
        break;
      }
      saw_continuation = true;
      rc = append_envelope_continuation(&envelope, platform, &frame);
      if (rc != GZC_OK) {
        break;
      }
      continue;
    }
    if (frame.type != GZC_RPC_FRAME_BINARY || saw_response || saw_continuation) {
      rc = GZC_ERR_RPC;
      break;
    }
    gzc_str_t response_payload;
    rc = gzc_client_store_rpc_response_internal(client, frame.data, frame.len, &response_payload);
    if (rc != GZC_OK) {
      break;
    }
    rc = gzc_rpc_decode_response_envelope(response_payload, out_response);
    if (rc != GZC_OK) {
      break;
    }
    saw_response = true;
    rc = GZC_OK;
    continue;
  }
  gzc_buf_free(&envelope, platform);
  gzc_buf_free(&frame_bytes, platform);
  gzc_service_channel_close(channel);
  return rc;
}

int gzc_rpc_call(gzc_client_t *client, gizclaw_rpc_v1_RpcMethod method, gzc_str_t params_payload, gzc_rpc_response_t *out_response) {
  if (client == NULL || out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (is_edge_rpc_method(method)) {
    return gzc_rpc_call_service(client, GZC_SERVICE_EDGE_RPC, method, params_payload, out_response);
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  const gzc_webrtc_vtable_t *webrtc = gzc_client_webrtc(client);
  if (platform == NULL || webrtc == NULL || webrtc->channel_send == NULL) {
    return GZC_ERR_CLOSED;
  }
  int rc = gzc_client_open_rpc_channel_internal(client, 5000);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_rtc_channel_t *channel = gzc_client_rpc_channel(client);
  if (channel == NULL) {
    gzc_client_close_rpc_channel_internal(client);
    return GZC_ERR_CLOSED;
  }
  rc = send_request_envelope(client, platform, method, params_payload);
  if (rc != GZC_OK) {
    gzc_client_close_rpc_channel_internal(client);
    return rc;
  }

  gzc_buf_t frame_bytes;
  gzc_buf_t envelope;
  gzc_buf_init(&frame_bytes);
  gzc_buf_init(&envelope);
  gzc_rpc_frame_t frame;
  bool saw_response = false;
  bool saw_continuation = false;
  for (;;) {
    rc = read_frame(client, platform, 5000, &frame_bytes, &frame);
    if (rc != GZC_OK) {
      break;
    }
    if (frame.type == GZC_RPC_FRAME_EOS) {
      if (saw_continuation && !saw_response) {
        gzc_str_t response_payload;
        rc = gzc_client_store_rpc_response_internal(client, envelope.data, envelope.len, &response_payload);
        if (rc != GZC_OK) {
          break;
        }
        rc = gzc_rpc_decode_response_envelope(response_payload, out_response);
        if (rc != GZC_OK) {
          break;
        }
        saw_response = true;
      }
      rc = saw_response ? GZC_OK : GZC_ERR_RPC;
      break;
    }
    if (frame.type == GZC_RPC_FRAME_TEXT) {
      if (saw_response) {
        rc = GZC_ERR_RPC;
        break;
      }
      saw_continuation = true;
      rc = append_envelope_continuation(&envelope, platform, &frame);
      if (rc != GZC_OK) {
        break;
      }
      continue;
    }
    if (frame.type != GZC_RPC_FRAME_BINARY || saw_response || saw_continuation) {
      rc = GZC_ERR_RPC;
      break;
    }
    gzc_str_t response_payload;
    rc = gzc_client_store_rpc_response_internal(client, frame.data, frame.len, &response_payload);
    if (rc != GZC_OK) {
      break;
    }
    rc = gzc_rpc_decode_response_envelope(response_payload, out_response);
    if (rc != GZC_OK) {
      break;
    }
    saw_response = true;
    rc = GZC_OK;
    continue;
  }
  gzc_buf_free(&envelope, platform);
  gzc_buf_free(&frame_bytes, platform);
  close_rpc_channel_on_error(client, rc);
  return rc;
}

static int gzc_rpc_call_stream_with_timeout(
    gzc_client_t *client,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_rpc_frame_cb on_frame,
    void *userdata,
    int response_timeout_ms) {
  if (client == NULL || on_frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  const gzc_webrtc_vtable_t *webrtc = gzc_client_webrtc(client);
  if (platform == NULL || webrtc == NULL || webrtc->channel_send == NULL) {
    return GZC_ERR_CLOSED;
  }
  int rc = gzc_client_open_rpc_channel_internal(client, 5000);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_rtc_channel_t *channel = gzc_client_rpc_channel(client);
  if (channel == NULL) {
    gzc_client_close_rpc_channel_internal(client);
    return GZC_ERR_CLOSED;
  }
  rc = send_request_envelope(client, platform, method, params_payload);
  if (rc != GZC_OK) {
    gzc_client_close_rpc_channel_internal(client);
    return rc;
  }
  gzc_buf_t frame_bytes;
  gzc_buf_init(&frame_bytes);
  gzc_buf_t envelope;
  gzc_buf_init(&envelope);
  gzc_rpc_frame_t frame;
  bool saw_response = false;
  bool saw_continuation = false;
  for (;;) {
    rc = read_frame(client, platform, response_timeout_ms, &frame_bytes, &frame);
    if (rc != GZC_OK) {
      break;
    }
    if (frame.type == GZC_RPC_FRAME_EOS) {
      if (saw_continuation && !saw_response) {
        gzc_rpc_frame_t response_frame;
        memset(&response_frame, 0, sizeof(response_frame));
        response_frame.type = GZC_RPC_FRAME_BINARY;
        response_frame.data = envelope.data;
        response_frame.len = envelope.len;
        gzc_rpc_response_t response;
        rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts((const char *)response_frame.data, response_frame.len), &response);
        if (rc != GZC_OK) {
          break;
        }
        saw_response = true;
        rc = on_frame(userdata, &response_frame);
        if (rc != GZC_OK) {
          break;
        }
        if (response.has_error) {
          rc = GZC_OK;
          break;
        }
        continue;
      }
      rc = saw_response ? GZC_OK : GZC_ERR_RPC;
      break;
    }
    if (!saw_response) {
      if (frame.type == GZC_RPC_FRAME_TEXT) {
        saw_continuation = true;
        rc = append_envelope_continuation(&envelope, platform, &frame);
        if (rc != GZC_OK) {
          break;
        }
        continue;
      }
      if (frame.type != GZC_RPC_FRAME_BINARY || saw_continuation) {
        rc = GZC_ERR_RPC;
        break;
      }
      gzc_rpc_response_t response;
      rc = gzc_rpc_decode_response_envelope(gzc_str_from_parts((const char *)frame.data, frame.len), &response);
      if (rc != GZC_OK) {
        break;
      }
      saw_response = true;
      rc = on_frame(userdata, &frame);
      if (rc != GZC_OK) {
        break;
      }
      continue;
    }
    if (frame.type == GZC_RPC_FRAME_JSON || frame.type == GZC_RPC_FRAME_TEXT) {
      rc = GZC_ERR_RPC;
      break;
    }
    rc = on_frame(userdata, &frame);
    if (rc != GZC_OK) {
      break;
    }
  }
  gzc_buf_free(&envelope, platform);
  gzc_buf_free(&frame_bytes, platform);
  close_rpc_channel_on_error(client, rc);
  return rc;
}

int gzc_rpc_call_stream(
    gzc_client_t *client,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_rpc_frame_cb on_frame,
    void *userdata) {
  return gzc_rpc_call_stream_with_timeout(
      client, method, params_payload, on_frame, userdata, 5000);
}

int gzc_rpc_speech_transcribe_open(
    gzc_client_t *client,
    const gizclaw_rpc_v1_SpeechTranscribeRequest *request,
    gzc_rpc_speech_upload_t **out_upload) {
  if (client == NULL || request == NULL || out_upload == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_upload = NULL;
  const gzc_platform_t *platform = gzc_client_platform(client);
  if (platform == NULL || platform->malloc == NULL || platform->free == NULL) {
    return GZC_ERR_CLOSED;
  }
  gzc_rpc_speech_upload_t *upload =
      (gzc_rpc_speech_upload_t *)platform->malloc(platform->userdata, sizeof(*upload));
  if (upload == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(upload, 0, sizeof(*upload));
  upload->client = client;
  upload->platform = platform;

  gzc_buf_t payload;
  gzc_buf_init(&payload);
  int rc = encode_pb_message(
      platform,
      gizclaw_rpc_v1_SpeechTranscribeRequest_fields,
      request,
      &payload);
  if (rc == GZC_OK) {
    rc = gzc_client_open_service_channel(
        client,
        GZC_SERVICE_PEER_RPC,
        5000,
        &upload->channel);
  }
  if (rc == GZC_OK) {
    rc = send_request_envelope_service(
        upload->channel,
        platform,
        gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_TRANSCRIBE,
        gzc_str_from_parts((const char *)payload.data, payload.len),
        false);
  }
  gzc_buf_free(&payload, platform);
  if (rc != GZC_OK) {
    if (upload->channel != NULL) {
      gzc_service_channel_close(upload->channel);
    }
    platform->free(platform->userdata, upload);
    return rc;
  }
  *out_upload = upload;
  return GZC_OK;
}

int gzc_rpc_speech_transcribe_write(
    gzc_rpc_speech_upload_t *upload,
    const uint8_t *data,
    size_t len) {
  if (upload == NULL || upload->channel == NULL || data == NULL || len == 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  size_t offset = 0;
  while (offset < len) {
    size_t chunk = len - offset;
    if (chunk > GZC_RPC_MAX_FRAME_SIZE) {
      chunk = GZC_RPC_MAX_FRAME_SIZE;
    }
    gzc_rpc_frame_t frame;
    memset(&frame, 0, sizeof(frame));
    frame.type = GZC_RPC_FRAME_BINARY;
    frame.data = data + offset;
    frame.len = chunk;
    int rc = gzc_service_channel_send_frame(upload->channel, &frame);
    if (rc != GZC_OK) {
      return rc;
    }
    offset += chunk;
  }
  return GZC_OK;
}

static void speech_upload_destroy(gzc_rpc_speech_upload_t *upload) {
  if (upload == NULL) {
    return;
  }
  if (upload->channel != NULL) {
    gzc_service_channel_close(upload->channel);
    upload->channel = NULL;
  }
  upload->platform->free(upload->platform->userdata, upload);
}

int gzc_rpc_speech_transcribe_finish(
    gzc_rpc_speech_upload_t *upload,
    gizclaw_rpc_v1_SpeechTranscribeResponse *out_response,
    gzc_rpc_error_t *out_error) {
  if (upload == NULL || upload->channel == NULL || out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_response, 0, sizeof(*out_response));
  if (out_error != NULL) {
    memset(out_error, 0, sizeof(*out_error));
  }
  gzc_rpc_frame_t eos;
  memset(&eos, 0, sizeof(eos));
  eos.type = GZC_RPC_FRAME_EOS;
  int rc = gzc_service_channel_send_frame(upload->channel, &eos);
  gzc_buf_t frame_bytes;
  gzc_buf_t envelope;
  gzc_buf_init(&frame_bytes);
  gzc_buf_init(&envelope);
  gzc_rpc_response_t response;
  memset(&response, 0, sizeof(response));
  bool saw_response = false;
  bool continued = false;
  while (rc == GZC_OK) {
    gzc_rpc_frame_t frame;
    rc = read_service_frame(
        upload->channel,
        GZC_RPC_SPEECH_TRANSCRIPTION_TIMEOUT_MS,
        &frame_bytes,
        &frame);
    if (rc != GZC_OK) {
      break;
    }
    if (frame.type == GZC_RPC_FRAME_TEXT && !saw_response) {
      continued = true;
      rc = append_envelope_continuation(&envelope, upload->platform, &frame);
      continue;
    }
    if (frame.type == GZC_RPC_FRAME_BINARY && !saw_response && !continued) {
      gzc_str_t response_payload;
      rc = gzc_client_store_rpc_response_internal(
          upload->client,
          frame.data,
          frame.len,
          &response_payload);
      if (rc != GZC_OK) {
        break;
      }
      rc = gzc_rpc_decode_response_envelope(
          response_payload,
          &response);
      saw_response = rc == GZC_OK;
      continue;
    }
    if (frame.type != GZC_RPC_FRAME_EOS) {
      rc = GZC_ERR_RPC;
      break;
    }
    if (!saw_response && continued) {
      rc = gzc_rpc_decode_response_envelope(
          gzc_str_from_parts((const char *)envelope.data, envelope.len),
          &response);
      saw_response = rc == GZC_OK;
    }
    if (rc == GZC_OK && !saw_response) {
      rc = GZC_ERR_RPC;
    }
    break;
  }
  if (rc == GZC_OK && response.has_error) {
    if (out_error != NULL) {
      *out_error = response.error;
    }
    rc = GZC_ERR_RPC;
  }
  if (rc == GZC_OK) {
    pb_istream_t stream = pb_istream_from_buffer(
        (const pb_byte_t *)response.result_payload.data,
        response.result_payload.len);
    if (!pb_decode(
            &stream,
            gizclaw_rpc_v1_SpeechTranscribeResponse_fields,
            out_response)) {
      rc = GZC_ERR_RPC;
    }
  }
  gzc_buf_free(&envelope, upload->platform);
  gzc_buf_free(&frame_bytes, upload->platform);
  speech_upload_destroy(upload);
  return rc;
}

void gzc_rpc_speech_transcribe_cancel(gzc_rpc_speech_upload_t *upload) {
  speech_upload_destroy(upload);
}

typedef struct {
  gzc_client_t *client;
  gizclaw_rpc_v1_SpeechSynthesizeResponse *metadata;
  gzc_rpc_speech_audio_cb on_audio;
  void *userdata;
  gzc_rpc_error_t *out_error;
  bool saw_metadata;
  int rc;
} gzc_speech_synthesis_context_t;

static int speech_synthesis_frame(void *userdata, const gzc_rpc_frame_t *frame) {
  gzc_speech_synthesis_context_t *context =
      (gzc_speech_synthesis_context_t *)userdata;
  if (!context->saw_metadata) {
    gzc_str_t response_payload;
    int rc = gzc_client_store_rpc_response_internal(
        context->client,
        frame->data,
        frame->len,
        &response_payload);
    if (rc != GZC_OK) {
      context->rc = rc;
      return rc;
    }
    gzc_rpc_response_t response;
    rc = gzc_rpc_decode_response_envelope(response_payload, &response);
    if (rc != GZC_OK) {
      context->rc = rc;
      return rc;
    }
    if (response.has_error) {
      if (context->out_error != NULL) {
        *context->out_error = response.error;
      }
      context->rc = GZC_ERR_RPC;
      return context->rc;
    }
    pb_istream_t stream = pb_istream_from_buffer(
        (const pb_byte_t *)response.result_payload.data,
        response.result_payload.len);
    if (!pb_decode(
            &stream,
            gizclaw_rpc_v1_SpeechSynthesizeResponse_fields,
            context->metadata)) {
      context->rc = GZC_ERR_RPC;
      return context->rc;
    }
    context->saw_metadata = true;
    return GZC_OK;
  }
  context->rc = context->on_audio(
      context->userdata,
      frame->data,
      frame->len);
  return context->rc;
}

int gzc_rpc_speech_synthesize(
    gzc_client_t *client,
    const gizclaw_rpc_v1_SpeechSynthesizeRequest *request,
    gizclaw_rpc_v1_SpeechSynthesizeResponse *out_metadata,
    gzc_rpc_speech_audio_cb on_audio,
    void *userdata,
    gzc_rpc_error_t *out_error) {
  if (client == NULL || request == NULL || out_metadata == NULL ||
      on_audio == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  if (platform == NULL) {
    return GZC_ERR_CLOSED;
  }
  memset(out_metadata, 0, sizeof(*out_metadata));
  if (out_error != NULL) {
    memset(out_error, 0, sizeof(*out_error));
  }
  gzc_buf_t payload;
  gzc_buf_init(&payload);
  int rc = encode_pb_message(
      platform,
      gizclaw_rpc_v1_SpeechSynthesizeRequest_fields,
      request,
      &payload);
  if (rc != GZC_OK) {
    gzc_buf_free(&payload, platform);
    return rc;
  }
  gzc_speech_synthesis_context_t context;
  memset(&context, 0, sizeof(context));
  context.client = client;
  context.metadata = out_metadata;
  context.on_audio = on_audio;
  context.userdata = userdata;
  context.out_error = out_error;
  rc = gzc_rpc_call_stream_with_timeout(
      client,
      gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_SYNTHESIZE,
      gzc_str_from_parts((const char *)payload.data, payload.len),
      speech_synthesis_frame,
      &context,
      GZC_RPC_SPEECH_SYNTHESIS_TIMEOUT_MS);
  gzc_buf_free(&payload, platform);
  if (context.rc != GZC_OK) {
    return context.rc;
  }
  if (rc == GZC_OK && !context.saw_metadata) {
    return GZC_ERR_RPC;
  }
  return rc;
}

int gzc_rpc_send_frame(gzc_client_t *client, const gzc_rpc_frame_t *frame) {
  if (client == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzc_client_write_rpc_frame_internal(client, frame);
}

void gzc_rpc_response_free(gzc_client_t *client, gzc_rpc_response_t *response) {
  (void)client;
  if (response != NULL) {
    memset(response, 0, sizeof(*response));
  }
}

typedef enum {
  GZC_INBOUND_ENVELOPE = 0,
  GZC_INBOUND_WAIT_EOS = 1,
  GZC_INBOUND_SPEED_BODY = 2,
  GZC_INBOUND_TERMINAL = 3
} gzc_inbound_phase_t;

struct gzc_rpc_inbound {
  gzc_client_t *client;
  const gzc_platform_t *platform;
  gzc_rtc_channel_t *channel;
  gzc_buf_t rx;
  gzc_buf_t envelope;
  gzc_buf_t id;
  gzc_buf_t payload;
  gzc_buf_t tx;
  size_t tx_offset;
  int64_t write_started_ms;
  gzc_inbound_phase_t phase;
  gizclaw_rpc_v1_RpcMethod method;
  uint64_t upload_expected;
  uint64_t upload_received;
  uint64_t download_expected;
  uint64_t download_sent;
  bool continuation;
  bool decoded_envelope;
  bool request_done;
  bool response_envelope_sent;
  bool response_envelope_flushed;
  bool response_eos_sent;
  bool write_blocked;
  bool close_after_write;
  bool close_requested;
};

typedef struct {
  gzc_buf_t *out;
  const gzc_platform_t *platform;
  int rc;
  bool seen;
} gzc_pb_copy_arg_t;

static bool decode_pb_copy(pb_istream_t *stream, const pb_field_t *field, void **arg) {
  (void)field;
  gzc_pb_copy_arg_t *copy = (gzc_pb_copy_arg_t *)(*arg);
  if (copy == NULL || copy->out == NULL || copy->platform == NULL ||
      (stream->state == NULL && stream->bytes_left != 0)) {
    return false;
  }
  copy->seen = true;
  copy->rc = gzc_buf_append(copy->out, copy->platform, stream->state, stream->bytes_left);
  if (copy->rc != GZC_OK) {
    return false;
  }
  return pb_read(stream, NULL, stream->bytes_left);
}

static void inbound_consume(gzc_buf_t *rx, size_t len) {
  if (len >= rx->len) {
    gzc_buf_reset(rx);
    return;
  }
  memmove(rx->data, rx->data + len, rx->len - len);
  rx->len -= len;
  rx->data[rx->len] = 0;
}

static int inbound_queue_frame(
    struct gzc_rpc_inbound *inbound,
    gzc_rpc_frame_type_t type,
    const uint8_t *data,
    size_t len) {
  gzc_rpc_frame_t frame;
  memset(&frame, 0, sizeof(frame));
  frame.type = type;
  frame.data = data;
  frame.len = len;
  if (inbound->tx_offset >= inbound->tx.len) {
    inbound->write_started_ms = gzc_client_instant_ms_internal(inbound->client);
  }
  return gzc_rpc_frame_encode(inbound->platform, &frame, &inbound->tx);
}

static int inbound_encode_response(
    struct gzc_rpc_inbound *inbound,
    const uint8_t *payload,
    size_t payload_len,
    bool has_error,
    gizclaw_rpc_v1_RpcErrorCode error_code,
    const char *error_message,
    gzc_buf_t *out) {
  gzc_pb_bytes_arg_t id_arg = {inbound->id.data, inbound->id.len};
  gzc_pb_bytes_arg_t payload_arg = {payload, payload_len};
  gzc_pb_bytes_arg_t message_arg = {
      (const uint8_t *)(error_message == NULL ? "" : error_message),
      error_message == NULL ? 0 : strlen(error_message)};
  gizclaw_rpc_v1_RpcResponse response = gizclaw_rpc_v1_RpcResponse_init_zero;
  response.id.funcs.encode = encode_pb_bytes;
  response.id.arg = &id_arg;
  if (has_error) {
    response.has_error = true;
    response.error.code = error_code;
    response.error.message.funcs.encode = encode_pb_bytes;
    response.error.message.arg = &message_arg;
  } else {
    response.payload.funcs.encode = encode_pb_bytes;
    response.payload.arg = &payload_arg;
  }
  gzc_buf_reset(out);
  return encode_pb_message(inbound->platform, gizclaw_rpc_v1_RpcResponse_fields, &response, out);
}

static int inbound_send_response_envelope(
    struct gzc_rpc_inbound *inbound,
    const uint8_t *data,
    size_t len,
    bool body_follows) {
  if (len > GZC_RPC_MAX_ENVELOPE_SIZE) {
    return GZC_ERR_RPC;
  }
  if (len <= GZC_RPC_MAX_FRAME_SIZE) {
    return inbound_queue_frame(inbound, GZC_RPC_FRAME_BINARY, data, len);
  }
  size_t offset = 0;
  while (offset < len) {
    size_t chunk = len - offset;
    if (chunk > GZC_RPC_MAX_FRAME_SIZE) {
      chunk = GZC_RPC_MAX_FRAME_SIZE;
    }
    int rc = inbound_queue_frame(inbound, GZC_RPC_FRAME_TEXT, data + offset, chunk);
    if (rc != GZC_OK) {
      return rc;
    }
    offset += chunk;
  }
  if (body_follows) {
    return inbound_queue_frame(inbound, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  return GZC_OK;
}

static int inbound_send_response_payload(
    struct gzc_rpc_inbound *inbound,
    const pb_msgdesc_t *fields,
    const void *message) {
  gzc_buf_t payload;
  gzc_buf_t response;
  gzc_buf_init(&payload);
  gzc_buf_init(&response);
  int rc = encode_pb_message(inbound->platform, fields, message, &payload);
  if (rc == GZC_OK) {
    rc = inbound_encode_response(inbound, payload.data, payload.len, false,
                                 gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_UNSPECIFIED,
                                 NULL, &response);
  }
  if (rc == GZC_OK) {
    bool body_follows =
        inbound->method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN;
    rc = inbound_send_response_envelope(
        inbound, response.data, response.len, body_follows);
  }
  if (rc == GZC_OK) {
    inbound->response_envelope_sent = true;
  }
  gzc_buf_free(&response, inbound->platform);
  gzc_buf_free(&payload, inbound->platform);
  return rc;
}

static int inbound_close_transport(struct gzc_rpc_inbound *inbound, int rc) {
  inbound->phase = GZC_INBOUND_TERMINAL;
  if (inbound->tx_offset < inbound->tx.len) {
    inbound->close_after_write = true;
  } else {
    inbound->close_requested = true;
  }
  return rc;
}

static int inbound_error(
    struct gzc_rpc_inbound *inbound,
    gizclaw_rpc_v1_RpcErrorCode code,
    const char *message) {
  if ((!inbound->decoded_envelope && inbound->id.len == 0) || inbound->response_envelope_sent) {
    return inbound_close_transport(inbound, GZC_OK);
  }
  gzc_buf_t response;
  gzc_buf_init(&response);
  int rc = inbound_encode_response(inbound, NULL, 0, true, code, message, &response);
  if (rc == GZC_OK) {
    rc = inbound_send_response_envelope(inbound, response.data, response.len, false);
  }
  if (rc == GZC_OK) {
    rc = inbound_queue_frame(inbound, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  gzc_buf_free(&response, inbound->platform);
  inbound->response_envelope_sent = rc == GZC_OK;
  inbound->response_eos_sent = rc == GZC_OK;
  if (rc != GZC_OK) {
    return inbound_close_transport(inbound, rc);
  }
  return inbound_close_transport(inbound, GZC_OK);
}

static int inbound_decode_request(struct gzc_rpc_inbound *inbound, const uint8_t *data, size_t len) {
  gzc_buf_reset(&inbound->id);
  gzc_buf_reset(&inbound->payload);
  gzc_pb_copy_arg_t id_arg = {&inbound->id, inbound->platform, GZC_OK, false};
  gzc_pb_copy_arg_t payload_arg = {&inbound->payload, inbound->platform, GZC_OK, false};
  gizclaw_rpc_v1_RpcRequest request = gizclaw_rpc_v1_RpcRequest_init_zero;
  request.id.funcs.decode = decode_pb_copy;
  request.id.arg = &id_arg;
  request.payload.funcs.decode = decode_pb_copy;
  request.payload.arg = &payload_arg;
  pb_istream_t stream = pb_istream_from_buffer(data, len);
  if (!pb_decode(&stream, gizclaw_rpc_v1_RpcRequest_fields, &request)) {
    int copy_rc = id_arg.rc != GZC_OK ? id_arg.rc : payload_arg.rc;
    if (copy_rc != GZC_OK) {
      return inbound_close_transport(inbound, copy_rc);
    }
    return inbound_error(inbound,
                         gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_PARSE_ERROR,
                         "malformed protobuf request");
  }
  inbound->decoded_envelope = true;
  inbound->method = request.method;
  if (inbound->id.len == 0 || request.method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_UNSPECIFIED) {
    return inbound_error(inbound,
                         gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_REQUEST,
                         "request id and method are required");
  }

  pb_istream_t payload_stream = pb_istream_from_buffer(inbound->payload.data, inbound->payload.len);
  if (request.method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING) {
    if (!payload_arg.seen) {
      return inbound_error(inbound,
                           gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                           "missing ping payload");
    }
    gizclaw_rpc_v1_PingRequest ping = gizclaw_rpc_v1_PingRequest_init_zero;
    if (!pb_decode(&payload_stream, gizclaw_rpc_v1_PingRequest_fields, &ping)) {
      return inbound_error(inbound,
                           gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                           "invalid ping payload");
    }
    inbound->phase = GZC_INBOUND_WAIT_EOS;
    return GZC_OK;
  }
  if (request.method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN) {
    if (!payload_arg.seen) {
      return inbound_error(inbound,
                           gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                           "missing speed-test payload");
    }
    gizclaw_rpc_v1_SpeedTestRequest speed = gizclaw_rpc_v1_SpeedTestRequest_init_zero;
    if (!pb_decode(&payload_stream, gizclaw_rpc_v1_SpeedTestRequest_fields, &speed) ||
        speed.up_content_length < 0 || speed.down_content_length < 0 ||
        speed.up_content_length > ((int64_t)1 << 30) ||
        speed.down_content_length > ((int64_t)1 << 30)) {
      return inbound_error(inbound,
                           gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                           "invalid speed-test lengths");
    }
    inbound->upload_expected = (uint64_t)speed.up_content_length;
    inbound->download_expected = (uint64_t)speed.down_content_length;
    gizclaw_rpc_v1_SpeedTestResponse response = gizclaw_rpc_v1_SpeedTestResponse_init_zero;
    response.up_content_length = speed.up_content_length;
    response.down_content_length = speed.down_content_length;
    int rc = inbound_send_response_payload(
        inbound, gizclaw_rpc_v1_SpeedTestResponse_fields, &response);
    if (rc != GZC_OK) {
      return inbound_close_transport(inbound, rc);
    }
    inbound->phase = GZC_INBOUND_SPEED_BODY;
    return GZC_OK;
  }
  return inbound_error(inbound,
                       gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_METHOD_NOT_FOUND,
                       "method not found");
}

static int inbound_finish_ping(struct gzc_rpc_inbound *inbound) {
  const gzc_platform_t *platform = inbound->platform;
  if (platform->time_unix_ms == NULL) {
    return inbound_error(inbound,
                         gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INTERNAL_ERROR,
                         "clock unavailable");
  }
  gizclaw_rpc_v1_PingResponse response = gizclaw_rpc_v1_PingResponse_init_zero;
  response.server_time = platform->time_unix_ms(platform->userdata);
  int rc = inbound_send_response_payload(inbound, gizclaw_rpc_v1_PingResponse_fields, &response);
  if (rc == GZC_OK) {
    rc = inbound_queue_frame(inbound, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  if (rc != GZC_OK) {
    return inbound_close_transport(inbound, rc);
  }
  inbound->request_done = true;
  inbound->response_eos_sent = true;
  return inbound_close_transport(inbound, GZC_OK);
}

static int inbound_process_frame(struct gzc_rpc_inbound *inbound, const gzc_rpc_frame_t *frame) {
  if (inbound->phase == GZC_INBOUND_TERMINAL) {
    return inbound_close_transport(inbound, GZC_OK);
  }
  if (inbound->phase == GZC_INBOUND_ENVELOPE) {
    if (frame->type == GZC_RPC_FRAME_TEXT) {
      inbound->continuation = true;
      int rc = append_envelope_continuation(&inbound->envelope, inbound->platform, frame);
      if (rc != GZC_OK) {
        return inbound_error(inbound,
                             gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_PARSE_ERROR,
                             "request envelope too large");
      }
      return GZC_OK;
    }
    if (frame->type == GZC_RPC_FRAME_BINARY && !inbound->continuation) {
      return inbound_decode_request(inbound, frame->data, frame->len);
    }
    if (frame->type == GZC_RPC_FRAME_EOS && inbound->continuation) {
      int rc = inbound_decode_request(inbound, inbound->envelope.data, inbound->envelope.len);
      if (rc != GZC_OK || inbound->phase == GZC_INBOUND_TERMINAL) {
        return rc;
      }
      if (inbound->method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING) {
        return inbound_finish_ping(inbound);
      }
      return GZC_OK;
    }
    return inbound_error(inbound,
                         gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_REQUEST,
                         "invalid request envelope frame");
  }
  if (inbound->phase == GZC_INBOUND_WAIT_EOS) {
    if (frame->type != GZC_RPC_FRAME_EOS) {
      return inbound_error(inbound,
                           gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                           "unexpected ping request body");
    }
    return inbound_finish_ping(inbound);
  }
  if (inbound->phase == GZC_INBOUND_SPEED_BODY) {
    if (inbound->request_done) {
      return inbound_error(inbound,
                           gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                           "duplicate speed-test request terminator");
    }
    if (frame->type == GZC_RPC_FRAME_BINARY) {
      uint64_t remaining = inbound->upload_expected - inbound->upload_received;
      if (remaining == 0 || (uint64_t)frame->len > remaining) {
        return inbound_error(inbound,
                             gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                             "speed-test upload exceeds declared length");
      }
      inbound->upload_received += (uint64_t)frame->len;
      return GZC_OK;
    }
    if (frame->type == GZC_RPC_FRAME_EOS) {
      if (inbound->upload_received != inbound->upload_expected) {
        return inbound_error(inbound,
                             gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                             "speed-test upload is truncated");
      }
      inbound->request_done = true;
      return GZC_OK;
    }
    return inbound_error(inbound,
                         gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                         "invalid speed-test body frame");
  }
  return GZC_OK;
}

int gzc_rpc_inbound_create(
    gzc_client_t *client,
    gzc_rtc_channel_t *channel,
    struct gzc_rpc_inbound **out_inbound) {
  if (client == NULL || channel == NULL || out_inbound == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  const gzc_webrtc_vtable_t *webrtc = gzc_client_webrtc(client);
  if (platform == NULL || webrtc == NULL || webrtc->channel_send == NULL) {
    return GZC_ERR_CLOSED;
  }
  struct gzc_rpc_inbound *inbound =
      (struct gzc_rpc_inbound *)platform->malloc(platform->userdata, sizeof(*inbound));
  if (inbound == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(inbound, 0, sizeof(*inbound));
  inbound->client = client;
  inbound->platform = platform;
  inbound->channel = channel;
  inbound->phase = GZC_INBOUND_ENVELOPE;
  gzc_buf_init(&inbound->rx);
  gzc_buf_init(&inbound->envelope);
  gzc_buf_init(&inbound->id);
  gzc_buf_init(&inbound->payload);
  gzc_buf_init(&inbound->tx);
  *out_inbound = inbound;
  return GZC_OK;
}

int gzc_rpc_inbound_feed(
    struct gzc_rpc_inbound *inbound,
    const uint8_t *data,
    size_t len,
    bool is_text) {
  if (inbound == NULL || (data == NULL && len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (is_text) {
    return inbound_close_transport(inbound, GZC_OK);
  }
  int rc = gzc_buf_append(&inbound->rx, inbound->platform, data, len);
  if (rc != GZC_OK) {
    return inbound_close_transport(inbound, rc);
  }
  while (inbound->rx.len >= 4) {
    size_t payload_len = (size_t)inbound->rx.data[0] | ((size_t)inbound->rx.data[1] << 8);
    size_t frame_len = 4 + payload_len;
    if (frame_len > 4 + GZC_RPC_MAX_FRAME_SIZE) {
      return inbound_close_transport(inbound, GZC_ERR_RPC);
    }
    if (inbound->rx.len < frame_len) {
      break;
    }
    gzc_rpc_frame_t frame;
    rc = gzc_rpc_frame_decode(inbound->rx.data, frame_len, &frame);
    if (rc != GZC_OK) {
      return inbound_close_transport(inbound, GZC_OK);
    }
    rc = inbound_process_frame(inbound, &frame);
    inbound_consume(&inbound->rx, frame_len);
    if (rc != GZC_OK) {
      return rc;
    }
    if (inbound->phase == GZC_INBOUND_TERMINAL) {
      if (inbound->rx.len != 0) {
        return inbound_close_transport(inbound, GZC_OK);
      }
      return GZC_OK;
    }
  }
  return GZC_OK;
}

int gzc_rpc_inbound_poll(struct gzc_rpc_inbound *inbound) {
  if (inbound == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (inbound->tx_offset < inbound->tx.len) {
    int timeout_ms = gzc_client_write_timeout_ms_internal(inbound->client);
    if (timeout_ms <= 0 ||
        gzc_client_instant_ms_internal(inbound->client) - inbound->write_started_ms >= timeout_ms) {
      gzc_buf_reset(&inbound->tx);
      inbound->tx_offset = 0;
      inbound->write_started_ms = 0;
      inbound->phase = GZC_INBOUND_TERMINAL;
      inbound->close_after_write = false;
      inbound->close_requested = true;
      return GZC_ERR_TIMEOUT;
    }
    int rc = gzc_client_try_write_bytes_internal(
        inbound->client,
        inbound->channel,
        inbound->tx.data,
        inbound->tx.len,
        &inbound->tx_offset,
        &inbound->write_blocked,
        16);
    if (rc != GZC_OK) {
      gzc_buf_reset(&inbound->tx);
      inbound->tx_offset = 0;
      inbound->write_started_ms = 0;
      inbound->phase = GZC_INBOUND_TERMINAL;
      inbound->close_after_write = false;
      inbound->close_requested = true;
      return rc;
    }
    if (inbound->tx_offset < inbound->tx.len) {
      return GZC_OK;
    }
    gzc_buf_reset(&inbound->tx);
    inbound->tx_offset = 0;
    inbound->write_started_ms = 0;
    if (inbound->response_envelope_sent) {
      inbound->response_envelope_flushed = true;
    }
  }
  if (inbound->phase == GZC_INBOUND_TERMINAL) {
    if (inbound->close_after_write) {
      inbound->close_after_write = false;
      inbound->close_requested = true;
    }
    return GZC_OK;
  }
  if (inbound->method != gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN ||
      !inbound->response_envelope_flushed) {
    return GZC_OK;
  }
  uint8_t chunk[4096];
  if (inbound->download_sent < inbound->download_expected) {
    size_t count = sizeof(chunk);
    uint64_t remaining = inbound->download_expected - inbound->download_sent;
    if (remaining < count) {
      count = (size_t)remaining;
    }
    for (size_t i = 0; i < count; i++) {
      chunk[i] = (uint8_t)((inbound->download_sent + i) & 0xffu);
    }
    int rc = inbound_queue_frame(inbound, GZC_RPC_FRAME_BINARY, chunk, count);
    if (rc != GZC_OK) {
      return inbound_close_transport(inbound, rc);
    }
    inbound->download_sent += count;
    return GZC_OK;
  }
  if (!inbound->response_eos_sent) {
    int rc = inbound_queue_frame(inbound, GZC_RPC_FRAME_EOS, NULL, 0);
    if (rc != GZC_OK) {
      return inbound_close_transport(inbound, rc);
    }
    inbound->response_eos_sent = true;
  }
  if (inbound->request_done && inbound->response_eos_sent) {
    return inbound_close_transport(inbound, GZC_OK);
  }
  return GZC_OK;
}

bool gzc_rpc_inbound_has_pending_output(struct gzc_rpc_inbound *inbound) {
  return inbound != NULL &&
         (inbound->tx_offset < inbound->tx.len || inbound->close_after_write ||
          (inbound->method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN &&
           inbound->phase != GZC_INBOUND_TERMINAL && inbound->response_envelope_sent &&
           !inbound->response_eos_sent));
}

bool gzc_rpc_inbound_close_requested(struct gzc_rpc_inbound *inbound) {
  return inbound != NULL && inbound->close_requested;
}

void gzc_rpc_inbound_destroy(struct gzc_rpc_inbound *inbound) {
  if (inbound == NULL) {
    return;
  }
  const gzc_platform_t *platform = inbound->platform;
  gzc_buf_free(&inbound->rx, platform);
  gzc_buf_free(&inbound->envelope, platform);
  gzc_buf_free(&inbound->id, platform);
  gzc_buf_free(&inbound->payload, platform);
  gzc_buf_free(&inbound->tx, platform);
  platform->free(platform->userdata, inbound);
}
