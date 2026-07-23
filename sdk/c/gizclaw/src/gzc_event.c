#include "gzc_event.h"

#include <ctype.h>
#include <pb_decode.h>
#include <pb_encode.h>
#include <string.h>

#include "gzc_rpc_frame.h"

struct gzc_event_stream {
  gzc_service_channel_t *channel;
  const gzc_platform_t *platform;
};

static int has_non_space(const char *value) {
  if (value == NULL) {
    return 0;
  }
  for (; *value != '\0'; ++value) {
    if (!isspace((unsigned char)*value)) {
      return 1;
    }
  }
  return 0;
}

static int peer_event_validate(const gzc_peer_event_t *event, int allow_unknown) {
  if (event == NULL || event->version != GZC_PEER_EVENT_VERSION) {
    return GZC_ERR_RPC;
  }
  if (event->which_payload == 0 &&
      !(allow_unknown &&
        event->type >
            gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED)) {
    return GZC_ERR_RPC;
  }
  switch (event->type) {
  case gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_BOS:
    return event->which_payload == gizclaw_events_v1_PeerEvent_bos_tag &&
                   has_non_space(event->payload.bos.stream_id)
               ? GZC_OK
               : GZC_ERR_RPC;
  case gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_EOS:
    return event->which_payload == gizclaw_events_v1_PeerEvent_eos_tag &&
                   has_non_space(event->payload.eos.stream_id)
               ? GZC_OK
               : GZC_ERR_RPC;
  case gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA:
    return event->which_payload == gizclaw_events_v1_PeerEvent_text_delta_tag &&
                   has_non_space(event->payload.text_delta.stream_id)
               ? GZC_OK
               : GZC_ERR_RPC;
  case gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
    return event->which_payload == gizclaw_events_v1_PeerEvent_text_done_tag &&
                   has_non_space(event->payload.text_done.stream_id)
               ? GZC_OK
               : GZC_ERR_RPC;
  case gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED:
    return event->which_payload ==
                       gizclaw_events_v1_PeerEvent_workspace_history_updated_tag &&
                   has_non_space(
                       event->payload.workspace_history_updated.workspace_name)
               ? GZC_OK
               : GZC_ERR_RPC;
  case gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED:
    return event->which_payload ==
                       gizclaw_events_v1_PeerEvent_friend_relationship_updated_tag &&
                   has_non_space(
                       event->payload.friend_relationship_updated.peer_public_key) &&
                   has_non_space(
                       event->payload.friend_relationship_updated.workspace_name)
               ? GZC_OK
               : GZC_ERR_RPC;
  case gizclaw_events_v1_PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED:
    return event->which_payload ==
                       gizclaw_events_v1_PeerEvent_friend_group_updated_tag &&
                   has_non_space(event->payload.friend_group_updated.friend_group_id) &&
                   has_non_space(event->payload.friend_group_updated.workspace_name)
               ? GZC_OK
               : GZC_ERR_RPC;
  default:
    return allow_unknown && event->which_payload == 0 ? GZC_OK : GZC_ERR_RPC;
  }
}

int gzc_event_stream_open(
    gzc_client_t *client,
    int timeout_ms,
    gzc_event_stream_t **out_stream) {
  if (client == NULL || out_stream == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_stream = NULL;
  const gzc_platform_t *platform = gzc_client_platform(client);
  if (platform == NULL) {
    platform = gzc_default_platform();
  }
  gzc_event_stream_t *stream =
      (gzc_event_stream_t *)platform->malloc(platform->userdata, sizeof(*stream));
  if (stream == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(stream, 0, sizeof(*stream));
  stream->platform = platform;
  int rc = gzc_client_open_service_channel(
      client, GZC_SERVICE_PEER_EVENT, timeout_ms, &stream->channel);
  if (rc != GZC_OK) {
    platform->free(platform->userdata, stream);
    return rc;
  }
  *out_stream = stream;
  return GZC_OK;
}

int gzc_event_stream_send(
    gzc_event_stream_t *stream,
    const gzc_peer_event_t *event) {
  if (stream == NULL || stream->channel == NULL || event == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  int rc = peer_event_validate(event, 0);
  if (rc != GZC_OK) {
    return rc;
  }
  size_t payload_size = 0;
  if (!pb_get_encoded_size(
          &payload_size, gizclaw_events_v1_PeerEvent_fields, event)) {
    return GZC_ERR_RPC;
  }
  uint8_t *payload = (uint8_t *)stream->platform->malloc(
      stream->platform->userdata, payload_size);
  if (payload == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  pb_ostream_t output =
      pb_ostream_from_buffer(payload, payload_size);
  if (!pb_encode(
          &output, gizclaw_events_v1_PeerEvent_fields, event)) {
    stream->platform->free(stream->platform->userdata, payload);
    return GZC_ERR_RPC;
  }
  gzc_rpc_frame_t frame = {
      .type = GZC_RPC_FRAME_BINARY,
      .data = payload,
      .len = output.bytes_written,
  };
  rc = gzc_service_channel_send_frame(stream->channel, &frame);
  stream->platform->free(stream->platform->userdata, payload);
  return rc;
}

int gzc_event_stream_read(
    gzc_event_stream_t *stream,
    int timeout_ms,
    gzc_peer_event_t *out_event) {
  if (stream == NULL || stream->channel == NULL || out_event == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_peer_event_t empty = gizclaw_events_v1_PeerEvent_init_zero;
  *out_event = empty;
  gzc_buf_t frame_bytes;
  gzc_buf_init(&frame_bytes);
  int rc = gzc_service_channel_read_frame(
      stream->channel, timeout_ms, &frame_bytes);
  if (rc != GZC_OK) {
    gzc_buf_free(&frame_bytes, stream->platform);
    return rc;
  }
  gzc_rpc_frame_t frame;
  rc = gzc_rpc_frame_decode(frame_bytes.data, frame_bytes.len, &frame);
  if (rc == GZC_OK && frame.type == GZC_RPC_FRAME_EOS) {
    rc = GZC_ERR_CLOSED;
  } else if (rc == GZC_OK && frame.type != GZC_RPC_FRAME_BINARY) {
    rc = GZC_ERR_RPC;
  }
  if (rc == GZC_OK) {
    pb_istream_t input = pb_istream_from_buffer(frame.data, frame.len);
    if (!pb_decode(
            &input, gizclaw_events_v1_PeerEvent_fields, out_event)) {
      rc = GZC_ERR_RPC;
    }
  }
  if (rc == GZC_OK) {
    rc = peer_event_validate(out_event, 1);
  }
  if (rc != GZC_OK) {
    *out_event = empty;
  }
  gzc_buf_free(&frame_bytes, stream->platform);
  return rc;
}

void gzc_event_stream_close(gzc_event_stream_t *stream) {
  if (stream == NULL) {
    return;
  }
  gzc_service_channel_close(stream->channel);
  stream->channel = NULL;
  stream->platform->free(stream->platform->userdata, stream);
}
