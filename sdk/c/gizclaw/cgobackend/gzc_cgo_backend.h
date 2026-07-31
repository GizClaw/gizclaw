#ifndef GZC_CGO_BACKEND_H
#define GZC_CGO_BACKEND_H

#include "gzc.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gzc_cgo_backend gzc_cgo_backend_t;

#define GZC_CGO_MAX_LOCAL_CHANNELS 16u

struct gzc_rtc_peer {
  gzc_cgo_backend_t *backend;
  int unused;
};

struct gzc_rtc_channel {
  gzc_cgo_backend_t *backend;
  int id;
  bool remote;
  bool in_use;
  bool ordered;
  bool reliable;
  char label[64];
};

struct gzc_cgo_backend {
  uint64_t handle;
  gzc_webrtc_callbacks_t callbacks;
  gzc_rtc_opus_frame_cb opus_callback;
  void *opus_callback_userdata;
  struct gzc_rtc_peer peer;
  struct gzc_rtc_channel packet_channel;
  struct gzc_rtc_channel local_channels[GZC_CGO_MAX_LOCAL_CHANNELS];
  struct gzc_rtc_channel remote_channels[GZC_RPC_MAX_INBOUND_CHANNELS];
  int next_local_channel_id;
  gzc_platform_t platform_impl;
  const gzc_platform_t *platform;
  gzc_platform_crypto_t crypto;
};

int gzc_cgo_backend_init(gzc_cgo_backend_t *backend);
void gzc_cgo_backend_deinit(gzc_cgo_backend_t *backend);

void gzc_cgo_backend_http_vtable(gzc_cgo_backend_t *backend, gzc_http_vtable_t *out_http);
void gzc_cgo_backend_crypto_vtable(gzc_cgo_backend_t *backend, gzc_platform_crypto_t *out_crypto);
void gzc_cgo_backend_webrtc_vtable(gzc_cgo_backend_t *backend, gzc_webrtc_vtable_t *out_webrtc);
void gzc_cgo_backend_webrtc_media_vtable(
    gzc_cgo_backend_t *backend,
    gzc_webrtc_media_vtable_t *out_media);
int gzc_cgo_backend_transport_send_counts(
    gzc_cgo_backend_t *backend,
    uint64_t *out_packet_data_channel_calls,
    uint64_t *out_opus_rtp_calls);
int gzc_cgo_backend_peer_add_ice_server(gzc_rtc_peer_t *peer, gzc_str_t url, gzc_str_t username, gzc_str_t credential);

void gzc_cgo_emit_channel_state(gzc_cgo_backend_t *backend, int channel_id, gzc_rtc_channel_state_t state);
void gzc_cgo_emit_peer_state(
    gzc_cgo_backend_t *backend,
    gzc_rtc_peer_state_t state);
void gzc_cgo_emit_channel_message(gzc_cgo_backend_t *backend, int channel_id, const uint8_t *data, size_t len, bool is_text);
void gzc_cgo_emit_channel_buffered_amount_low(gzc_cgo_backend_t *backend, int channel_id);
void gzc_cgo_emit_remote_channel(gzc_cgo_backend_t *backend, int channel_id, const char *label, size_t label_len, bool ordered, bool reliable);
void gzc_cgo_emit_opus_frame(
    gzc_cgo_backend_t *backend,
    const uint8_t *opus,
    size_t opus_len);

#ifdef __cplusplus
}
#endif

#endif
