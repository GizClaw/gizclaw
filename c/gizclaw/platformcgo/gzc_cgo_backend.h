#ifndef GZC_CGO_BACKEND_H
#define GZC_CGO_BACKEND_H

#include "gzc.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gzc_cgo_backend gzc_cgo_backend_t;

struct gzc_rtc_peer {
  gzc_cgo_backend_t *backend;
  int unused;
};

struct gzc_rtc_channel {
  gzc_cgo_backend_t *backend;
  int id;
};

struct gzc_cgo_backend {
  uint64_t handle;
  gzc_webrtc_callbacks_t callbacks;
  struct gzc_rtc_peer peer;
  struct gzc_rtc_channel packet_channel;
  struct gzc_rtc_channel rpc_channel;
  struct gzc_rtc_channel event_channel;
  gzc_platform_t platform_impl;
  const gzc_platform_t *platform;
  gzc_platform_crypto_t crypto;
  char signaling_url[256];
  char private_key_text[GZC_KEY_TEXT_CAP];
  char server_public_key_text[GZC_KEY_TEXT_CAP];
};

int gzc_cgo_backend_init(gzc_cgo_backend_t *backend, const char *identity_dir);
void gzc_cgo_backend_deinit(gzc_cgo_backend_t *backend);

void gzc_cgo_backend_http_vtable(gzc_cgo_backend_t *backend, gzc_http_vtable_t *out_http);
void gzc_cgo_backend_crypto_vtable(gzc_cgo_backend_t *backend, gzc_platform_crypto_t *out_crypto);
void gzc_cgo_backend_webrtc_vtable(gzc_cgo_backend_t *backend, gzc_webrtc_vtable_t *out_webrtc);

void gzc_cgo_emit_channel_state(gzc_cgo_backend_t *backend, int channel_id, gzc_rtc_channel_state_t state);
void gzc_cgo_emit_channel_message(gzc_cgo_backend_t *backend, int channel_id, const uint8_t *data, size_t len, bool is_text);

#ifdef __cplusplus
}
#endif

#endif
