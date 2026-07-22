#ifndef GZC_CLIENT_H
#define GZC_CLIENT_H

#include "gzc_http.h"
#include "gzc_rpc_frame.h"
#include "gzc_signaling.h"
#include "gzc_webrtc.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gzc_client gzc_client_t;
typedef struct gzc_service_channel gzc_service_channel_t;

/* Maximum live server-created ServicePeerRPC exchanges per client. */
#define GZC_RPC_MAX_INBOUND_CHANNELS 4u
#define GZC_SERVICE_WRITE_CHUNK_SIZE 1400u
#define GZC_SERVICE_WRITE_HIGH_WATER_DEFAULT (256u * 1024u)
#define GZC_SERVICE_WRITE_LOW_WATER_DEFAULT (64u * 1024u)

typedef struct {
  gzc_str_t server_endpoint;
  gzc_str_t private_key;
  const gzc_platform_t *platform;
  const gzc_platform_crypto_t *crypto;
  const gzc_http_vtable_t *http;
  const gzc_webrtc_vtable_t *webrtc;
  gzc_cipher_mode_t cipher_mode;
  int connect_timeout_ms;
  /* Required positive timeout for accepting a logical service write locally. */
  int write_timeout_ms;
  /* A zero pair selects the embedded defaults above. Otherwise low < high. */
  size_t service_write_high_water_bytes;
  size_t service_write_low_water_bytes;
  void *userdata;
} gzc_client_config_t;

int gzc_client_create(const gzc_client_config_t *config, gzc_client_t **out_client);
int gzc_client_set_peer_add_ice_server(gzc_client_t *client, gzc_peer_add_ice_server_fn fn);
int gzc_client_connect(gzc_client_t *client);
/*
 * Drives queued WebRTC callbacks and inbound RPC work on the caller's thread.
 * Applications serving server-initiated RPCs must call this repeatedly.
 */
int gzc_client_poll(gzc_client_t *client, int timeout_ms);
int gzc_client_close(gzc_client_t *client);
void gzc_client_destroy(gzc_client_t *client);

gzc_rtc_channel_t *gzc_client_rpc_channel(gzc_client_t *client);
const gzc_platform_t *gzc_client_platform(gzc_client_t *client);
const gzc_webrtc_vtable_t *gzc_client_webrtc(gzc_client_t *client);

int gzc_client_open_service_channel(
    gzc_client_t *client,
    uint64_t service,
    int timeout_ms,
    gzc_service_channel_t **out_channel);
/* frame and frame->data are borrowed until this synchronous call returns. */
int gzc_service_channel_send_frame(gzc_service_channel_t *channel, const gzc_rpc_frame_t *frame);
int gzc_service_channel_read_frame(gzc_service_channel_t *channel, int timeout_ms, gzc_buf_t *out_frame_bytes);
void gzc_service_channel_close(gzc_service_channel_t *channel);

int gzc_client_send_packet(gzc_client_t *client, uint8_t protocol, const uint8_t *payload, size_t len);
int gzc_client_read_packet(gzc_client_t *client, int timeout_ms, uint8_t *out_protocol, gzc_buf_t *out_payload);

#ifdef __cplusplus
}
#endif

#endif
