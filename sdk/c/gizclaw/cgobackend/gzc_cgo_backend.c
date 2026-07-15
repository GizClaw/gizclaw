#include "gzc_cgo_backend.h"

#include <stdlib.h>
#include <string.h>

uint64_t gzcGoBackendCreate(void);
void gzcGoBackendDestroy(uint64_t handle);
void gzcGoBackendSetCBackend(uint64_t handle, gzc_cgo_backend_t *backend);
int gzcGoHTTPRequest(uint64_t handle, int method, const char *url, size_t url_len, const gzc_http_header_t *headers, size_t header_count, const uint8_t *data, size_t len, int *out_status, uint8_t **out_data, size_t *out_len);
int gzcGoKeyPairFromPrivate(const uint8_t *private_key, uint8_t *out_private_key, uint8_t *out_public_key);
int gzcGoDH(const uint8_t *private_key, const uint8_t *remote_public_key, uint8_t *out_shared);
int gzcGoHKDFSHA256(const uint8_t *secret, size_t secret_len, const uint8_t *salt, size_t salt_len, const char *info, size_t info_len, uint8_t *out, size_t out_len);
int gzcGoAEADSeal(int mode, const uint8_t *key, size_t key_len, const uint8_t *nonce, size_t nonce_len, const uint8_t *plaintext, size_t plaintext_len, const uint8_t *aad, size_t aad_len, uint8_t **out_data, size_t *out_len);
int gzcGoAEADOpen(int mode, const uint8_t *key, size_t key_len, const uint8_t *nonce, size_t nonce_len, const uint8_t *ciphertext, size_t ciphertext_len, const uint8_t *aad, size_t aad_len, uint8_t **out_data, size_t *out_len);
int gzcGoRandom(uint8_t *out, size_t len);
int64_t gzcGoTimeUnixMs(void);
int gzcGoPeerCreate(uint64_t handle);
int gzcGoPeerAddICEServer(uint64_t handle, const char *url, size_t url_len, const char *username, size_t username_len, const char *credential, size_t credential_len);
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

static gzc_str_t bridge_str_from_parts(const char *data, size_t len) {
  gzc_str_t out = {data, len};
  return out;
}

static gzc_str_t bridge_str_from_cstr(const char *text) {
  return bridge_str_from_parts(text, text == NULL ? 0 : strlen(text));
}

static int bridge_buf_replace(
    gzc_buf_t *out,
    const gzc_platform_t *platform,
    const uint8_t *data,
    size_t len) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  out->len = 0;
  if (platform == NULL || platform->realloc == NULL ||
      (data == NULL && len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (len == SIZE_MAX) {
    return GZC_ERR_NO_MEMORY;
  }
  size_t required = len + 1;
  if (required > out->cap) {
    size_t next = out->cap == 0 ? 64 : out->cap;
    while (next < required) {
      if (next > SIZE_MAX / 2) {
        next = required;
        break;
      }
      next *= 2;
    }
    void *resized = platform->realloc(platform->userdata, out->data, next);
    if (resized == NULL) {
      return GZC_ERR_NO_MEMORY;
    }
    out->data = (uint8_t *)resized;
    out->cap = next;
  }
  if (len != 0) {
    memcpy(out->data, data, len);
  }
  out->data[len] = 0;
  out->len = len;
  return GZC_OK;
}

static void *bridge_malloc(void *userdata, size_t size) {
  (void)userdata;
  return malloc(size);
}

static void *bridge_realloc(void *userdata, void *ptr, size_t size) {
  (void)userdata;
  return realloc(ptr, size);
}

static void bridge_free(void *userdata, void *ptr) {
  (void)userdata;
  free(ptr);
}

static int64_t bridge_time_unix_ms(void *userdata) {
  (void)userdata;
  return gzcGoTimeUnixMs();
}

static int bridge_random(void *userdata, uint8_t *out, size_t len) {
  (void)userdata;
  return gzcGoRandom(out, len);
}

static void bridge_log(void *userdata, gzc_log_level_t level, gzc_str_t message) {
  (void)userdata;
  (void)level;
  (void)message;
}

int gzc_cgo_backend_init(gzc_cgo_backend_t *backend) {
  if (backend == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(backend, 0, sizeof(*backend));
  backend->platform_impl.userdata = backend;
  backend->platform_impl.malloc = bridge_malloc;
  backend->platform_impl.realloc = bridge_realloc;
  backend->platform_impl.free = bridge_free;
  backend->platform_impl.time_unix_ms = bridge_time_unix_ms;
  backend->platform_impl.random = bridge_random;
  backend->platform_impl.log = bridge_log;
  backend->platform = &backend->platform_impl;
  backend->peer.backend = backend;
  backend->packet_channel.backend = backend;
  backend->packet_channel.id = gzc_cgo_channel_packet;
  backend->rpc_channel.backend = backend;
  backend->rpc_channel.id = gzc_cgo_channel_rpc;
  backend->event_channel.backend = backend;
  backend->event_channel.id = gzc_cgo_channel_event;
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    backend->remote_channels[i].backend = backend;
    backend->remote_channels[i].remote = true;
  }
  backend->handle = gzcGoBackendCreate();
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

static int bridge_http_request(void *userdata, const gzc_http_request_t *request, gzc_http_response_t *out_response) {
  gzc_cgo_backend_t *backend = (gzc_cgo_backend_t *)userdata;
  if (backend == NULL || request == NULL || out_response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (request->method != GZC_HTTP_METHOD_GET && request->method != GZC_HTTP_METHOD_POST) {
    return GZC_ERR_UNSUPPORTED;
  }
  memset(out_response, 0, sizeof(*out_response));
  uint8_t *body = NULL;
  size_t body_len = 0;
  int status = 0;
  int rc = gzcGoHTTPRequest(
      backend->handle,
      (int)request->method,
      request->url.data,
      request->url.len,
      request->headers,
      request->header_count,
      request->body,
      request->body_len,
      &status,
      &body,
      &body_len);
  if (rc != GZC_OK) {
    return rc;
  }
  out_response->status_code = status;
  out_response->body.data = body;
  out_response->body.len = body_len;
  out_response->body.cap = body_len;
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
  out_http->request = bridge_http_request;
  out_http->response_free = bridge_http_response_free;
}

static int bridge_keypair_from_private(void *userdata, const gzc_key_t *private_key, gzc_keypair_t *out_keypair) {
  (void)userdata;
  if (private_key == NULL || out_keypair == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzcGoKeyPairFromPrivate(private_key->bytes, out_keypair->private_key.bytes, out_keypair->public_key.bytes);
}

static int bridge_dh(void *userdata, const gzc_keypair_t *local, const gzc_public_key_t *remote, gzc_key_t *out_shared) {
  (void)userdata;
  if (local == NULL || remote == NULL || out_shared == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzcGoDH(local->private_key.bytes, remote->bytes, out_shared->bytes);
}

static int bridge_hkdf_sha256(
    void *userdata,
    const uint8_t *secret,
    size_t secret_len,
    const uint8_t *salt,
    size_t salt_len,
    gzc_str_t info,
    uint8_t *out,
    size_t out_len) {
  (void)userdata;
  if (secret == NULL || (salt == NULL && salt_len != 0) || info.data == NULL || out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzcGoHKDFSHA256(secret, secret_len, salt, salt_len, info.data, info.len, out, out_len);
}

static int bridge_aead(
    gzc_cgo_backend_t *backend,
    bool seal,
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
  if (backend == NULL || key == NULL || nonce == NULL || (input == NULL && input_len != 0) ||
      (aad == NULL && aad_len != 0) || out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  uint8_t *raw = NULL;
  size_t raw_len = 0;
  int rc = seal
      ? gzcGoAEADSeal((int)mode, key, key_len, nonce, nonce_len, input, input_len, aad, aad_len, &raw, &raw_len)
      : gzcGoAEADOpen((int)mode, key, key_len, nonce, nonce_len, input, input_len, aad, aad_len, &raw, &raw_len);
  if (rc == GZC_OK) {
    rc = bridge_buf_replace(out, backend->platform, raw, raw_len);
  }
  free(raw);
  return rc;
}

static int bridge_aead_seal(
    void *userdata,
    gzc_cipher_mode_t mode,
    const uint8_t *key,
    size_t key_len,
    const uint8_t *nonce,
    size_t nonce_len,
    const uint8_t *plaintext,
    size_t plaintext_len,
    const uint8_t *aad,
    size_t aad_len,
    gzc_buf_t *out_ciphertext) {
  return bridge_aead((gzc_cgo_backend_t *)userdata, true, mode, key, key_len, nonce, nonce_len, plaintext, plaintext_len, aad, aad_len, out_ciphertext);
}

static int bridge_aead_open(
    void *userdata,
    gzc_cipher_mode_t mode,
    const uint8_t *key,
    size_t key_len,
    const uint8_t *nonce,
    size_t nonce_len,
    const uint8_t *ciphertext,
    size_t ciphertext_len,
    const uint8_t *aad,
    size_t aad_len,
    gzc_buf_t *out_plaintext) {
  return bridge_aead((gzc_cgo_backend_t *)userdata, false, mode, key, key_len, nonce, nonce_len, ciphertext, ciphertext_len, aad, aad_len, out_plaintext);
}

void gzc_cgo_backend_crypto_vtable(gzc_cgo_backend_t *backend, gzc_platform_crypto_t *out_crypto) {
  memset(out_crypto, 0, sizeof(*out_crypto));
  out_crypto->userdata = backend;
  out_crypto->keypair_from_private = bridge_keypair_from_private;
  out_crypto->dh = bridge_dh;
  out_crypto->hkdf_sha256 = bridge_hkdf_sha256;
  out_crypto->aead_seal = bridge_aead_seal;
  out_crypto->aead_open = bridge_aead_open;
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
        bridge_str_from_parts(sdp, sdp_len));
  }
  free(sdp);
  return GZC_OK;
}

int gzc_cgo_backend_peer_add_ice_server(gzc_rtc_peer_t *peer, gzc_str_t url, gzc_str_t username, gzc_str_t credential) {
  gzc_cgo_backend_t *backend = peer == NULL ? NULL : peer->backend;
  if (backend == NULL || url.data == NULL || url.len == 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzcGoPeerAddICEServer(
      backend->handle,
      url.data,
      url.len,
      username.data,
      username.len,
      credential.data,
      credential.len);
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

static gzc_rtc_channel_t *remote_channel_by_id(gzc_cgo_backend_t *backend, int channel_id) {
  if (backend == NULL) {
    return NULL;
  }
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    gzc_rtc_channel_t *channel = &backend->remote_channels[i];
    if (channel->in_use && channel->id == channel_id) {
      return channel;
    }
  }
  return NULL;
}

static void fill_channel_info(const gzc_rtc_channel_t *channel, gzc_rtc_channel_info_t *info) {
  memset(info, 0, sizeof(*info));
  if (channel->remote) {
    info->label = bridge_str_from_cstr(channel->label);
    info->stream_id = (uint16_t)channel->id;
    info->ordered = channel->ordered;
    info->reliable = channel->reliable;
  } else if (channel->id == gzc_cgo_channel_packet) {
    info->label = bridge_str_from_cstr("giznet/v1/packet");
    info->stream_id = 0;
    info->ordered = false;
    info->reliable = false;
  } else if (channel->id == gzc_cgo_channel_rpc) {
    info->label = bridge_str_from_cstr("giznet/v1/service/0");
    info->stream_id = 1;
    info->ordered = true;
    info->reliable = true;
  } else {
    info->label = bridge_str_from_cstr("giznet/v1/service/32");
    info->stream_id = 2;
    info->ordered = true;
    info->reliable = true;
  }
}

void gzc_cgo_emit_remote_channel(
    gzc_cgo_backend_t *backend,
    int channel_id,
    const char *label,
    size_t label_len,
    bool ordered,
    bool reliable) {
  if (backend == NULL || label == NULL || label_len >= sizeof(backend->remote_channels[0].label)) {
    if (backend != NULL) {
      gzcGoChannelClose(backend->handle, channel_id);
    }
    return;
  }
  gzc_rtc_channel_t *channel = NULL;
  for (size_t i = 0; i < GZC_RPC_MAX_INBOUND_CHANNELS; i++) {
    if (!backend->remote_channels[i].in_use) {
      channel = &backend->remote_channels[i];
      break;
    }
  }
  if (channel == NULL) {
    gzcGoChannelClose(backend->handle, channel_id);
    return;
  }
  channel->id = channel_id;
  channel->in_use = true;
  channel->ordered = ordered;
  channel->reliable = reliable;
  memcpy(channel->label, label, label_len);
  channel->label[label_len] = 0;
  if (backend->callbacks.on_remote_channel == NULL) {
    channel->in_use = false;
    gzcGoChannelClose(backend->handle, channel_id);
    return;
  }
  gzc_rtc_channel_info_t info;
  fill_channel_info(channel, &info);
  backend->callbacks.on_remote_channel(
      backend->callbacks.userdata, &backend->peer, channel, &info);
}

void gzc_cgo_emit_channel_state(gzc_cgo_backend_t *backend, int channel_id, gzc_rtc_channel_state_t state) {
  if (backend == NULL || backend->callbacks.on_channel_state == NULL) {
    return;
  }
  gzc_rtc_channel_t *channel = remote_channel_by_id(backend, channel_id);
  if (channel == NULL) {
    if (channel_id == gzc_cgo_channel_packet) {
      channel = &backend->packet_channel;
    } else if (channel_id == gzc_cgo_channel_rpc) {
      channel = &backend->rpc_channel;
    } else if (channel_id == gzc_cgo_channel_event) {
      channel = &backend->event_channel;
    } else {
      return;
    }
  }
  gzc_rtc_channel_info_t info;
  fill_channel_info(channel, &info);
  backend->callbacks.on_channel_state(
      backend->callbacks.userdata,
      &backend->peer,
      channel,
      &info,
      state);
  if (channel->remote && (state == GZC_RTC_CHANNEL_CLOSED || state == GZC_RTC_CHANNEL_ERROR)) {
    channel->in_use = false;
    channel->id = 0;
    channel->label[0] = 0;
  }
}

void gzc_cgo_emit_channel_message(gzc_cgo_backend_t *backend, int channel_id, const uint8_t *data, size_t len, bool is_text) {
  if (backend == NULL || backend->callbacks.on_channel_message == NULL) {
    return;
  }
  gzc_rtc_channel_t *channel = remote_channel_by_id(backend, channel_id);
  if (channel == NULL) {
    if (channel_id == gzc_cgo_channel_packet) {
      channel = &backend->packet_channel;
    } else if (channel_id == gzc_cgo_channel_rpc) {
      channel = &backend->rpc_channel;
    } else if (channel_id == gzc_cgo_channel_event) {
      channel = &backend->event_channel;
    } else {
      return;
    }
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
