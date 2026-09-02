#include "gzc_rpc.h"

#include "gzc_client_internal.h"

#include "pb_decode.h"
#include "pb_encode.h"

#include <limits.h>
#include <stdint.h>
#include <string.h>

#define GZC_RPC_MAX_ENVELOPE_SIZE (GZC_RPC_MAX_FRAME_SIZE * 16u)
#define GZC_RPC_MAX_REQUEST_RX_SIZE \
  (GZC_RPC_MAX_ENVELOPE_SIZE + (17u * 4u))
#define GZC_RPC_DOWNLOAD_FRAMES_PER_POLL 16u
int64_t gzc_client_instant_ms_internal(gzc_client_t *client);
int gzc_client_write_timeout_ms_internal(gzc_client_t *client);
int gzc_client_dispatch_rpc_internal(
    gzc_client_t *client,
    int method,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata);
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

struct gzc_rpc_request {
  gzc_client_t *client;
  const gzc_platform_t *platform;
  gzc_service_channel_t *channel;
  gzc_buf_t rx;
  gzc_buf_t envelope;
  gzc_buf_t response_payload;
  gzc_buf_t tx;
  size_t tx_offset;
  size_t tx_frame_end;
  int64_t deadline_ms;
  size_t continuation_count;
  int status;
  bool saw_response;
  bool saw_continuation;
  bool write_finished;
  gzc_rpc_frame_cb on_frame;
  void *frame_userdata;
};

static int request_terminalize(gzc_rpc_request_t *request, int status);

static int request_invoke_callback(
    gzc_rpc_request_t *request,
    const gzc_rpc_frame_t *frame) {
  const int rc = request->on_frame(request->frame_userdata, frame);
  return rc == GZC_ERR_WOULD_BLOCK ? GZC_ERR_RPC : rc;
}

int gzc_rpc_request_progress_internal(gzc_rpc_request_t *request) {
  if (request == NULL)
    return GZC_ERR_INVALID_ARGUMENT;
  if (gzc_rpc_request_terminal_internal(request))
    return request->status;
  if (request->tx_offset >= request->tx.len) {
    gzc_buf_reset(&request->tx);
    request->tx_offset = 0u;
    request->tx_frame_end = 0u;
    return GZC_OK;
  }
  if (request->tx_frame_end == 0u) {
    if (request->tx.len - request->tx_offset < 4u)
      return request_terminalize(request, GZC_ERR_RPC);
    const size_t payload_len =
        (size_t)request->tx.data[request->tx_offset] |
        ((size_t)request->tx.data[request->tx_offset + 1u] << 8u);
    if (payload_len > GZC_RPC_MAX_FRAME_SIZE ||
        payload_len + 4u > request->tx.len - request->tx_offset) {
      return request_terminalize(request, GZC_ERR_RPC);
    }
    request->tx_frame_end = request->tx_offset + payload_len + 4u;
  }
  const int rc = gzc_service_channel_try_write_bytes_internal(
      request->channel, request->tx.data, request->tx_frame_end,
      &request->tx_offset);
  if (rc == GZC_ERR_WOULD_BLOCK)
    return rc;
  if (rc != GZC_OK)
    return request_terminalize(request, rc);
  if (request->tx_offset < request->tx_frame_end)
    return GZC_ERR_WOULD_BLOCK;
  if (request->tx_offset < request->tx.len) {
    request->tx_frame_end = 0u;
    return GZC_ERR_WOULD_BLOCK;
  }
  gzc_buf_reset(&request->tx);
  request->tx_offset = 0u;
  request->tx_frame_end = 0u;
  return GZC_OK;
}

static int request_queue_frame(gzc_rpc_request_t *request,
                               const gzc_rpc_frame_t *frame) {
  if (request == NULL || frame == NULL)
    return GZC_ERR_INVALID_ARGUMENT;
  if (gzc_rpc_request_terminal_internal(request))
    return request->status;
  if (request->tx_offset < request->tx.len)
    return GZC_ERR_WOULD_BLOCK;
  gzc_buf_reset(&request->tx);
  request->tx_offset = 0u;
  request->tx_frame_end = 0u;
  const int rc = gzc_rpc_frame_encode(request->platform, frame, &request->tx);
  if (rc != GZC_OK)
    return rc;
  const int progress_rc = gzc_rpc_request_progress_internal(request);
  return progress_rc == GZC_ERR_WOULD_BLOCK ? GZC_OK : progress_rc;
}

static int request_terminalize(gzc_rpc_request_t *request, int status) {
  if (request == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (request->status == GZC_ERR_WOULD_BLOCK) {
    request->status = status;
  }
  return request->status;
}

bool gzc_rpc_request_terminal_internal(const gzc_rpc_request_t *request) {
  return request != NULL && request->status != GZC_ERR_WOULD_BLOCK;
}

void gzc_rpc_request_detach_internal(
    gzc_rpc_request_t *request,
    gzc_service_channel_t *channel) {
  if (request != NULL && request->channel == channel) {
    request->channel = NULL;
    request->client = NULL;
  }
}

void gzc_rpc_request_channel_closed_internal(gzc_rpc_request_t *request) {
  (void)request_terminalize(request, GZC_ERR_CLOSED);
}

void gzc_rpc_request_client_closed_internal(gzc_rpc_request_t *request) {
  (void)request_terminalize(request, GZC_ERR_CLOSED);
}

void gzc_rpc_request_transport_error_internal(
    gzc_rpc_request_t *request,
    int status) {
  (void)request_terminalize(request, status);
}

void gzc_rpc_request_expire_internal(
    gzc_rpc_request_t *request,
    int64_t now_ms) {
  if (request != NULL && now_ms >= request->deadline_ms) {
    (void)request_terminalize(request, GZC_ERR_TIMEOUT);
  }
}

int gzc_rpc_request_backend_timeout_ms_internal(
    const gzc_rpc_request_t *request,
    int requested_timeout_ms) {
  if (request == NULL || gzc_rpc_request_terminal_internal(request)) {
    return requested_timeout_ms;
  }
  const int64_t remaining =
      request->deadline_ms - gzc_client_instant_ms_internal(request->client);
  if (remaining <= 0) {
    return 0;
  }
  if (requested_timeout_ms < 0 || remaining < requested_timeout_ms) {
    return remaining > INT_MAX ? INT_MAX : (int)remaining;
  }
  return requested_timeout_ms;
}

static void request_release_channel(gzc_rpc_request_t *request) {
  if (request == NULL || request->client == NULL || request->channel == NULL) {
    return;
  }
  gzc_client_release_rpc_request_internal(
      request->client, request->channel, request);
}

static void consume_request_rx(gzc_rpc_request_t *request, size_t len) {
  if (len >= request->rx.len) {
    gzc_buf_reset(&request->rx);
    return;
  }
  memmove(request->rx.data, request->rx.data + len, request->rx.len - len);
  request->rx.len -= len;
  request->rx.data[request->rx.len] = 0;
}

static int request_store_response(
    gzc_rpc_request_t *request,
    const uint8_t *data,
    size_t len) {
  if (len > GZC_RPC_MAX_ENVELOPE_SIZE) {
    return GZC_ERR_RPC;
  }
  gzc_buf_reset(&request->response_payload);
  int rc = gzc_buf_append(
      &request->response_payload, request->platform, data, len);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_rpc_response_t decoded;
  return gzc_rpc_decode_response_envelope(
      gzc_str_from_parts(
          (const char *)request->response_payload.data,
          request->response_payload.len),
      &decoded);
}

static int request_process_frame(
    gzc_rpc_request_t *request,
    const gzc_rpc_frame_t *frame) {
  if (frame->type == GZC_RPC_FRAME_EOS) {
    if (frame->len != 0u) {
      return GZC_ERR_RPC;
    }
    if (request->saw_continuation && !request->saw_response) {
      int rc = request_store_response(
          request, request->envelope.data, request->envelope.len);
      if (rc != GZC_OK) {
        return rc;
      }
      request->saw_response = true;
      if (request->on_frame != NULL) {
        gzc_rpc_frame_t response_frame;
        memset(&response_frame, 0, sizeof(response_frame));
        response_frame.type = GZC_RPC_FRAME_BINARY;
        response_frame.data = request->response_payload.data;
        response_frame.len = request->response_payload.len;
        int callback_rc =
            request_invoke_callback(request, &response_frame);
        if (callback_rc != GZC_OK)
          return callback_rc;
      }
    }
    if (!request->saw_response)
      return GZC_ERR_RPC;
    return request->on_frame == NULL
               ? GZC_OK
               : request_invoke_callback(request, frame);
  }
  if (frame->type == GZC_RPC_FRAME_TEXT) {
    if (request->saw_response || request->continuation_count >= 16u ||
        frame->len > GZC_RPC_MAX_ENVELOPE_SIZE ||
        request->envelope.len >
            GZC_RPC_MAX_ENVELOPE_SIZE - frame->len) {
      return GZC_ERR_RPC;
    }
    request->saw_continuation = true;
    request->continuation_count++;
    return gzc_buf_append(
        &request->envelope, request->platform, frame->data, frame->len);
  }
  if (frame->type != GZC_RPC_FRAME_BINARY) {
    return GZC_ERR_RPC;
  }
  if (request->saw_response) {
    return request->on_frame == NULL
               ? GZC_ERR_RPC
               : request_invoke_callback(request, frame);
  }
  if (request->saw_continuation)
    return GZC_ERR_RPC;
  int rc = request_store_response(request, frame->data, frame->len);
  if (rc == GZC_OK) {
    request->saw_response = true;
    if (request->on_frame != NULL)
      rc = request_invoke_callback(request, frame);
  }
  return rc;
}

int gzc_rpc_request_feed_internal(
    gzc_rpc_request_t *request,
    const uint8_t *data,
    size_t len) {
  if (request == NULL || (data == NULL && len != 0u)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (request->client != NULL) {
    gzc_rpc_request_expire_internal(
        request, gzc_client_instant_ms_internal(request->client));
  }
  if (gzc_rpc_request_terminal_internal(request)) {
    return request->status;
  }
  if (len > GZC_RPC_MAX_REQUEST_RX_SIZE ||
      request->rx.len > GZC_RPC_MAX_REQUEST_RX_SIZE - len) {
    return request_terminalize(request, GZC_ERR_RPC);
  }
  int rc = gzc_buf_append(&request->rx, request->platform, data, len);
  if (rc != GZC_OK) {
    return request_terminalize(request, rc);
  }
  while (request->rx.len >= 4u) {
    const size_t payload_len =
        (size_t)request->rx.data[0] |
        ((size_t)request->rx.data[1] << 8);
    if (payload_len > GZC_RPC_MAX_FRAME_SIZE) {
      return request_terminalize(request, GZC_ERR_RPC);
    }
    const size_t frame_len = payload_len + 4u;
    if (request->rx.len < frame_len) {
      break;
    }
    gzc_rpc_frame_t frame;
    rc = gzc_rpc_frame_decode(request->rx.data, frame_len, &frame);
    if (rc == GZC_OK) {
      rc = request_process_frame(request, &frame);
    }
    consume_request_rx(request, frame_len);
    if (rc != GZC_ERR_WOULD_BLOCK &&
        (rc != GZC_OK || frame.type == GZC_RPC_FRAME_EOS)) {
      return request_terminalize(request, rc);
    }
  }
  return GZC_OK;
}

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

static int append_request_frame(gzc_rpc_request_t *request,
                                const gzc_rpc_frame_t *frame) {
  return gzc_rpc_frame_encode(request->platform, frame, &request->tx);
}

static int queue_request_envelope(
    gzc_rpc_request_t *rpc_request,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    bool finish_write) {
  gzc_buf_t request;
  gzc_buf_init(&request);
  int rc = gzc_rpc_encode_request_envelope(
      rpc_request->platform,
      gzc_str_from_cstr("1"),
      method,
      params_payload,
      &request);
  size_t offset = 0u;
  while (rc == GZC_OK && offset < request.len) {
    size_t chunk = request.len - offset;
    if (chunk > GZC_RPC_MAX_FRAME_SIZE) {
      chunk = GZC_RPC_MAX_FRAME_SIZE;
    }
    gzc_rpc_frame_t frame;
    memset(&frame, 0, sizeof(frame));
    frame.type = request.len <= GZC_RPC_MAX_FRAME_SIZE
                     ? GZC_RPC_FRAME_BINARY
                     : GZC_RPC_FRAME_TEXT;
    frame.data = request.data + offset;
    frame.len = chunk;
    rc = append_request_frame(rpc_request, &frame);
    offset += chunk;
  }
  if (rc == GZC_OK && finish_write) {
    gzc_rpc_frame_t eos;
    memset(&eos, 0, sizeof(eos));
    eos.type = GZC_RPC_FRAME_EOS;
    rc = append_request_frame(rpc_request, &eos);
    rpc_request->write_finished = rc == GZC_OK;
  }
  gzc_buf_free(&request, rpc_request->platform);
  if (rc == GZC_OK) {
    int progress_rc = gzc_rpc_request_progress_internal(rpc_request);
    if (progress_rc != GZC_ERR_WOULD_BLOCK)
      rc = progress_rc;
  }
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

static int rpc_request_start_internal(
    gzc_client_t *client,
    uint64_t service,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    int timeout_ms,
    bool finish_write,
    gzc_rpc_frame_cb on_frame,
    void *frame_userdata,
    gzc_rpc_request_t **out_request) {
  if (out_request != NULL) {
    *out_request = NULL;
  }
  if (client == NULL || out_request == NULL || timeout_ms <= 0 ||
      (params_payload.data == NULL && params_payload.len != 0u)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  if (platform == NULL) {
    return GZC_ERR_CLOSED;
  }
  gzc_rpc_request_t *request =
      (gzc_rpc_request_t *)platform->malloc(
          platform->userdata, sizeof(*request));
  if (request == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(request, 0, sizeof(*request));
  request->client = client;
  request->platform = platform;
  request->status = GZC_ERR_WOULD_BLOCK;
  request->on_frame = on_frame;
  request->frame_userdata = frame_userdata;
  gzc_buf_init(&request->rx);
  gzc_buf_init(&request->envelope);
  gzc_buf_init(&request->response_payload);
  gzc_buf_init(&request->tx);
  const int64_t start_ms = gzc_client_instant_ms_internal(client);
  request->deadline_ms =
      start_ms > INT64_MAX - timeout_ms
          ? INT64_MAX
          : start_ms + (int64_t)timeout_ms;

  bool attached = false;
  int rc = gzc_client_create_service_channel_internal(
      client, service, &request->channel);
  if (rc == GZC_OK) {
    rc = gzc_client_attach_rpc_request_internal(request->channel, request);
    attached = rc == GZC_OK;
  }
  if (rc == GZC_OK) {
    rc = queue_request_envelope(request, method, params_payload, finish_write);
  }
  if (rc != GZC_OK &&
      (!gzc_rpc_request_terminal_internal(request) ||
       request->status == GZC_ERR_CLOSED || request->status == rc)) {
    (void)request_terminalize(request, rc);
    if (attached) {
      request_release_channel(request);
    } else if (request->channel != NULL) {
      gzc_service_channel_close(request->channel);
      request->channel = NULL;
      request->client = NULL;
    }
    gzc_rpc_request_destroy(request);
    return rc;
  }
  if (gzc_rpc_request_terminal_internal(request)) {
    request_release_channel(request);
  }
  *out_request = request;
  return GZC_OK;
}

int gzc_rpc_request_start(
    gzc_client_t *client,
    uint64_t service,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    int timeout_ms,
    gzc_rpc_request_t **out_request) {
  return rpc_request_start_internal(
      client,
      service,
      method,
      params_payload,
      timeout_ms,
      true,
      NULL,
      NULL,
      out_request);
}

int gzc_rpc_request_start_stream(
    gzc_client_t *client,
    uint64_t service,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    int timeout_ms,
    gzc_rpc_frame_cb on_frame,
    void *userdata,
    gzc_rpc_request_t **out_request) {
  if (on_frame == NULL) {
    if (out_request != NULL)
      *out_request = NULL;
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return rpc_request_start_internal(
      client, service, method, params_payload, timeout_ms, false,
      on_frame, userdata, out_request);
}

int gzc_rpc_request_write(gzc_rpc_request_t *request,
                          const uint8_t *data,
                          size_t len) {
  if (request == NULL || data == NULL || len == 0u ||
      len > GZC_RPC_MAX_FRAME_SIZE) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (request->write_finished)
    return GZC_ERR_CLOSED;
  const gzc_rpc_frame_t frame = {
      .type = GZC_RPC_FRAME_BINARY, .data = data, .len = len};
  return request_queue_frame(request, &frame);
}

int gzc_rpc_request_finish_write(gzc_rpc_request_t *request) {
  if (request == NULL)
    return GZC_ERR_INVALID_ARGUMENT;
  if (request->write_finished)
    return GZC_OK;
  const gzc_rpc_frame_t eos = {.type = GZC_RPC_FRAME_EOS};
  const int rc = request_queue_frame(request, &eos);
  if (rc == GZC_OK)
    request->write_finished = true;
  return rc;
}

int gzc_rpc_request_result(
    gzc_rpc_request_t *request,
    gzc_rpc_response_t *out_response) {
  if (request == NULL || out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (!gzc_rpc_request_terminal_internal(request) && request->client != NULL) {
    gzc_rpc_request_expire_internal(
        request, gzc_client_instant_ms_internal(request->client));
    if (gzc_rpc_request_terminal_internal(request)) {
      request_release_channel(request);
    }
  }
  if (!gzc_rpc_request_terminal_internal(request)) {
    return GZC_ERR_WOULD_BLOCK;
  }
  if (request->status != GZC_OK) {
    return request->status;
  }
  return gzc_rpc_decode_response_envelope(
      gzc_str_from_parts(
          (const char *)request->response_payload.data,
          request->response_payload.len),
      out_response);
}

void gzc_rpc_request_cancel(gzc_rpc_request_t *request) {
  if (request == NULL) {
    return;
  }
  (void)request_terminalize(request, GZC_ERR_CLOSED);
  request_release_channel(request);
}

void gzc_rpc_request_destroy(gzc_rpc_request_t *request) {
  if (request == NULL) {
    return;
  }
  gzc_rpc_request_cancel(request);
  gzc_buf_free(&request->rx, request->platform);
  gzc_buf_free(&request->envelope, request->platform);
  gzc_buf_free(&request->response_payload, request->platform);
  gzc_buf_free(&request->tx, request->platform);
  request->platform->free(request->platform->userdata, request);
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

static bool inbound_is_client_method(gizclaw_rpc_v1_RpcMethod method) {
  switch (method) {
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_INFO_GET:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_IDENTIFIERS_GET:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_TOOL_INVOKE:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_STATUS_GET:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_VOLUME_SET:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_SOUND_PLAY:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_DEVICE_REBOOT:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_STATUS_GET:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_SAVED_LIST:
  case gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_WIFI_SAVED_FORGET:
    return true;
  default:
    return false;
  }
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
  if (inbound_is_client_method(request.method)) {
    if (!payload_arg.seen) {
      return inbound_error(inbound,
                           gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS,
                           "missing client RPC payload");
    }
    inbound->phase = GZC_INBOUND_WAIT_EOS;
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

typedef struct {
  struct gzc_rpc_inbound *inbound;
  gzc_buf_t encoded_response;
  int response_rc;
  bool responded;
  bool has_error;
  int error_code;
  char error_message[128];
} gzc_inbound_provider_response_t;

static int inbound_provider_respond(
    void *userdata,
    const gzc_rpc_provider_response_t *response) {
  gzc_inbound_provider_response_t *context =
      (gzc_inbound_provider_response_t *)userdata;
  if (context == NULL || response == NULL || context->responded ||
      (response->payload == NULL && response->payload_len != 0u) ||
      (response->error_message.data == NULL &&
       response->error_message.len != 0u)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  context->responded = true;
  context->has_error = response->has_error;
  context->error_code = response->error_code;
  if (response->has_error) {
    size_t message_len = response->error_message.len;
    if (message_len >= sizeof(context->error_message)) {
      message_len = sizeof(context->error_message) - 1u;
    }
    if (message_len != 0u) {
      memcpy(
          context->error_message,
          response->error_message.data,
          message_len);
    }
    context->error_message[message_len] = '\0';
    context->response_rc = GZC_OK;
    return context->response_rc;
  }
  context->response_rc = inbound_encode_response(
      context->inbound,
      response->payload,
      response->payload_len,
      false,
      gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_UNSPECIFIED,
      NULL,
      &context->encoded_response);
  return context->response_rc;
}

static int inbound_finish_provider(struct gzc_rpc_inbound *inbound) {
  gzc_inbound_provider_response_t provider_response;
  memset(&provider_response, 0, sizeof(provider_response));
  provider_response.inbound = inbound;
  gzc_buf_init(&provider_response.encoded_response);
  int rc = gzc_client_dispatch_rpc_internal(
      inbound->client,
      (int)inbound->method,
      gzc_str_from_parts(
          (const char *)inbound->payload.data,
          inbound->payload.len),
      inbound_provider_respond,
      &provider_response);
  if (rc == GZC_ERR_UNSUPPORTED) {
    gzc_buf_free(&provider_response.encoded_response, inbound->platform);
    return inbound_error(
        inbound,
        gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_METHOD_NOT_FOUND,
        "client RPC provider not configured");
  }
  if (rc != GZC_OK || !provider_response.responded ||
      provider_response.response_rc != GZC_OK) {
    gzc_buf_free(&provider_response.encoded_response, inbound->platform);
    return inbound_error(
        inbound,
        gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INTERNAL_ERROR,
        "client RPC provider failed");
  }
  if (provider_response.has_error) {
    gizclaw_rpc_v1_RpcErrorCode error_code;
    switch (provider_response.error_code) {
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_PARSE_ERROR:
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_REQUEST:
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_METHOD_NOT_FOUND:
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INVALID_PARAMS:
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INTERNAL_ERROR:
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_BAD_REQUEST:
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_FORBIDDEN:
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_NOT_FOUND:
    case gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_CONFLICT:
      error_code =
          (gizclaw_rpc_v1_RpcErrorCode)provider_response.error_code;
      break;
    default:
      error_code =
          gizclaw_rpc_v1_RpcErrorCode_RPC_ERROR_CODE_INTERNAL_ERROR;
      break;
    }
    gzc_buf_free(&provider_response.encoded_response, inbound->platform);
    return inbound_error(
        inbound,
        error_code,
        provider_response.error_message);
  }

  rc = inbound_send_response_envelope(
      inbound,
      provider_response.encoded_response.data,
      provider_response.encoded_response.len,
      false);
  if (rc == GZC_OK) {
    rc = inbound_queue_frame(inbound, GZC_RPC_FRAME_EOS, NULL, 0);
  }
  gzc_buf_free(&provider_response.encoded_response, inbound->platform);
  if (rc != GZC_OK) {
    return inbound_close_transport(inbound, rc);
  }
  inbound->request_done = true;
  inbound->response_envelope_sent = true;
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
      if (inbound_is_client_method(inbound->method)) {
        return inbound_finish_provider(inbound);
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
                           "unexpected request body");
    }
    if (inbound->method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_PING) {
      return inbound_finish_ping(inbound);
    }
    return inbound_finish_provider(inbound);
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
  uint8_t chunk[4096];
  size_t download_frames = 0;
  for (;;) {
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
    if (inbound->download_sent < inbound->download_expected) {
      if (download_frames >= GZC_RPC_DOWNLOAD_FRAMES_PER_POLL) {
        return GZC_OK;
      }
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
      download_frames++;
      continue;
    }
    if (!inbound->response_eos_sent) {
      int rc = inbound_queue_frame(inbound, GZC_RPC_FRAME_EOS, NULL, 0);
      if (rc != GZC_OK) {
        return inbound_close_transport(inbound, rc);
      }
      inbound->response_eos_sent = true;
      continue;
    }
    if (inbound->request_done) {
      return inbound_close_transport(inbound, GZC_OK);
    }
    return GZC_OK;
  }
}

int gzc_rpc_inbound_backend_timeout_ms(
    struct gzc_rpc_inbound *inbound,
    int requested_timeout_ms) {
  if (inbound == NULL) {
    return requested_timeout_ms;
  }
  if (inbound->tx_offset < inbound->tx.len) {
    if (!inbound->write_blocked) {
      return 0;
    }
    int64_t remaining =
        (int64_t)gzc_client_write_timeout_ms_internal(inbound->client) -
        (gzc_client_instant_ms_internal(inbound->client) - inbound->write_started_ms);
    if (remaining <= 0) {
      return 0;
    }
    if (requested_timeout_ms < 0 || remaining < requested_timeout_ms) {
      return (int)remaining;
    }
    return requested_timeout_ms;
  }
  if (inbound->close_after_write ||
      (inbound->method == gizclaw_rpc_v1_RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN &&
       inbound->phase != GZC_INBOUND_TERMINAL && inbound->response_envelope_sent &&
       !inbound->response_eos_sent)) {
    return 0;
  }
  return requested_timeout_ms;
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
