#include "bridge.h"

#include <stdlib.h>
#include <string.h>

uint64_t gzcGoBackendCreate(const char *identity_dir);
void gzcGoBackendDestroy(uint64_t handle);
void gzcGoBackendSetCBackend(uint64_t handle, gzc_cgo_backend_t *backend);
int gzcGoHTTPPost(uint64_t handle, const uint8_t *data, size_t len, uint8_t **out_data, size_t *out_len);
int gzcGoPeerCreate(uint64_t handle);
int gzcGoPeerStartOffer(uint64_t handle, char **out_sdp, size_t *out_len);
int gzcGoPeerSetRemoteSDP(uint64_t handle, const char *sdp, size_t len);
int gzcGoPeerCreateDataChannel(uint64_t handle, const char *label, size_t len, int channel_id, bool ordered, bool reliable);
int gzcGoPeerPoll(uint64_t handle, int timeout_ms);
int gzcGoChannelSend(uint64_t handle, int channel_id, const uint8_t *data, size_t len, bool is_text);
void gzcGoChannelClose(uint64_t handle, int channel_id);
void gzcGoPeerClose(uint64_t handle);

enum {
  gzc_cgo_channel_packet = 0,
  gzc_cgo_channel_rpc = 1,
  gzc_cgo_channel_event = 2
};

int gzc_cgo_backend_init(gzc_cgo_backend_t *backend, const char *identity_dir) {
  if (backend == NULL || identity_dir == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(backend, 0, sizeof(*backend));
  backend->platform = gzc_default_platform();
  backend->peer.backend = backend;
  backend->packet_channel.backend = backend;
  backend->packet_channel.id = gzc_cgo_channel_packet;
  backend->rpc_channel.backend = backend;
  backend->rpc_channel.id = gzc_cgo_channel_rpc;
  backend->event_channel.backend = backend;
  backend->event_channel.id = gzc_cgo_channel_event;
  backend->handle = gzcGoBackendCreate(identity_dir);
  if (backend->handle == 0) {
    return GZC_ERR_WEBRTC;
  }
  gzcGoBackendSetCBackend(backend->handle, backend);
  return GZC_OK;
}

void gzc_cgo_backend_deinit(gzc_cgo_backend_t *backend) {
  if (backend == NULL || backend->handle == 0) {
    return;
  }
  gzcGoBackendDestroy(backend->handle);
  backend->handle = 0;
}

static int bridge_http_post(void *userdata, const gzc_http_request_t *request, gzc_http_response_t *out_response) {
  gzc_cgo_backend_t *backend = (gzc_cgo_backend_t *)userdata;
  if (backend == NULL || request == NULL || out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_response, 0, sizeof(*out_response));
  uint8_t *answer = NULL;
  size_t answer_len = 0;
  int rc = gzcGoHTTPPost(backend->handle, request->body, request->body_len, &answer, &answer_len);
  if (rc != GZC_OK) {
    return rc;
  }
  out_response->status_code = 200;
  out_response->body.data = answer;
  out_response->body.len = answer_len;
  out_response->body.cap = answer_len;
  return GZC_OK;
}

static void bridge_http_response_free(void *userdata, gzc_http_response_t *response) {
  (void)userdata;
  if (response == NULL) {
    return;
  }
  free(response->body.data);
  response->body.data = NULL;
  response->body.len = 0;
  response->body.cap = 0;
}

void gzc_cgo_backend_http_vtable(gzc_cgo_backend_t *backend, gzc_http_vtable_t *out_http) {
  memset(out_http, 0, sizeof(*out_http));
  out_http->userdata = backend;
  out_http->post = bridge_http_post;
  out_http->response_free = bridge_http_response_free;
}

static int bridge_peer_create(void *userdata, const gzc_webrtc_callbacks_t *callbacks, gzc_rtc_peer_t **out_peer) {
  gzc_cgo_backend_t *backend = (gzc_cgo_backend_t *)userdata;
  if (backend == NULL || callbacks == NULL || out_peer == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  backend->callbacks = *callbacks;
  int rc = gzcGoPeerCreate(backend->handle);
  if (rc != GZC_OK) {
    return rc;
  }
  *out_peer = &backend->peer;
  return GZC_OK;
}

static int bridge_peer_start_offer(gzc_rtc_peer_t *peer) {
  gzc_cgo_backend_t *backend = peer == NULL ? NULL : peer->backend;
  if (backend == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  char *sdp = NULL;
  size_t sdp_len = 0;
  int rc = gzcGoPeerStartOffer(backend->handle, &sdp, &sdp_len);
  if (rc != GZC_OK) {
    return rc;
  }
  if (backend->callbacks.on_local_sdp != NULL) {
    backend->callbacks.on_local_sdp(
        backend->callbacks.userdata,
        &backend->peer,
        GZC_RTC_SDP_OFFER,
        gzc_str_from_parts(sdp, sdp_len));
  }
  free(sdp);
  return GZC_OK;
}

static int bridge_peer_set_remote_sdp(gzc_rtc_peer_t *peer, gzc_rtc_sdp_type_t type, gzc_str_t sdp) {
  gzc_cgo_backend_t *backend = peer == NULL ? NULL : peer->backend;
  if (backend == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (type != GZC_RTC_SDP_ANSWER) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  int rc = gzcGoPeerSetRemoteSDP(backend->handle, sdp.data, sdp.len);
  if (rc != GZC_OK) {
    return rc;
  }
  return GZC_OK;
}

static int bridge_peer_create_data_channel(
    gzc_rtc_peer_t *peer,
    const gzc_rtc_channel_config_t *config,
    gzc_rtc_channel_t **out_channel) {
  gzc_cgo_backend_t *backend = peer == NULL ? NULL : peer->backend;
  if (backend == NULL || config == NULL || out_channel == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_rtc_channel_t *channel = NULL;
  if (config->label.len == strlen("giznet/v1/packet") &&
      strncmp(config->label.data, "giznet/v1/packet", config->label.len) == 0) {
    channel = &backend->packet_channel;
  } else if (config->label.len == strlen("giznet/v1/service/0") &&
             strncmp(config->label.data, "giznet/v1/service/0", config->label.len) == 0) {
    channel = &backend->rpc_channel;
  } else if (config->label.len == strlen("giznet/v1/service/32") &&
             strncmp(config->label.data, "giznet/v1/service/32", config->label.len) == 0) {
    channel = &backend->event_channel;
  } else {
    return GZC_ERR_UNSUPPORTED;
  }
  int rc = gzcGoPeerCreateDataChannel(
      backend->handle,
      config->label.data,
      config->label.len,
      channel->id,
      config->ordered,
      config->reliable);
  if (rc != GZC_OK) {
    return rc;
  }
  *out_channel = channel;
  return GZC_OK;
}

static int bridge_peer_poll(gzc_rtc_peer_t *peer, int timeout_ms) {
  gzc_cgo_backend_t *backend = peer == NULL ? NULL : peer->backend;
  if (backend == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzcGoPeerPoll(backend->handle, timeout_ms);
}

static int bridge_channel_send(gzc_rtc_channel_t *channel, const uint8_t *data, size_t len, bool is_text) {
  if (channel == NULL || channel->backend == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzcGoChannelSend(channel->backend->handle, channel->id, data, len, is_text);
}

static void bridge_channel_close(gzc_rtc_channel_t *channel) {
  if (channel != NULL && channel->backend != NULL) {
    gzcGoChannelClose(channel->backend->handle, channel->id);
  }
}

static void bridge_peer_close(gzc_rtc_peer_t *peer) {
  if (peer != NULL && peer->backend != NULL) {
    gzcGoPeerClose(peer->backend->handle);
  }
}

void gzc_cgo_backend_webrtc_vtable(gzc_cgo_backend_t *backend, gzc_webrtc_vtable_t *out_webrtc) {
  memset(out_webrtc, 0, sizeof(*out_webrtc));
  out_webrtc->userdata = backend;
  out_webrtc->peer_create = bridge_peer_create;
  out_webrtc->peer_start_offer = bridge_peer_start_offer;
  out_webrtc->peer_set_remote_sdp = bridge_peer_set_remote_sdp;
  out_webrtc->peer_create_data_channel = bridge_peer_create_data_channel;
  out_webrtc->peer_poll = bridge_peer_poll;
  out_webrtc->channel_send = bridge_channel_send;
  out_webrtc->channel_close = bridge_channel_close;
  out_webrtc->peer_close = bridge_peer_close;
}

void gzc_cgo_emit_channel_state(gzc_cgo_backend_t *backend, int channel_id, gzc_rtc_channel_state_t state) {
  if (backend == NULL || backend->callbacks.on_channel_state == NULL) {
    return;
  }
  gzc_rtc_channel_t *channel = &backend->rpc_channel;
  if (channel_id == gzc_cgo_channel_packet) {
    channel = &backend->packet_channel;
  } else if (channel_id == gzc_cgo_channel_event) {
    channel = &backend->event_channel;
  }
  gzc_rtc_channel_info_t info;
  memset(&info, 0, sizeof(info));
  if (channel_id == gzc_cgo_channel_packet) {
    info.label = gzc_str_from_cstr("giznet/v1/packet");
    info.stream_id = 0;
    info.ordered = false;
    info.reliable = false;
  } else if (channel_id == gzc_cgo_channel_rpc) {
    info.label = gzc_str_from_cstr("giznet/v1/service/0");
    info.stream_id = 1;
    info.ordered = true;
    info.reliable = true;
  } else {
    info.label = gzc_str_from_cstr("giznet/v1/service/32");
    info.stream_id = 2;
    info.ordered = true;
    info.reliable = true;
  }
  backend->callbacks.on_channel_state(
      backend->callbacks.userdata,
      &backend->peer,
      channel,
      &info,
      state);
}

void gzc_cgo_emit_channel_message(gzc_cgo_backend_t *backend, int channel_id, const uint8_t *data, size_t len, bool is_text) {
  if (backend == NULL || backend->callbacks.on_channel_message == NULL) {
    return;
  }
  gzc_rtc_channel_t *channel = &backend->rpc_channel;
  if (channel_id == gzc_cgo_channel_packet) {
    channel = &backend->packet_channel;
  } else if (channel_id == gzc_cgo_channel_event) {
    channel = &backend->event_channel;
  }
  backend->callbacks.on_channel_message(
      backend->callbacks.userdata,
      &backend->peer,
      channel,
      NULL,
      data,
      len,
      is_text);
}
