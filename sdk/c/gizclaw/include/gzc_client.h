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

typedef struct {
  const uint8_t *payload;
  size_t payload_len;
  bool has_error;
  int error_code;
  gzc_str_t error_message;
} gzc_rpc_provider_response_t;

/* Consumes one provider response synchronously. Borrowed response views only
 * need to remain valid until this function returns. */
typedef int (*gzc_rpc_provider_respond_fn)(
    void *userdata,
    const gzc_rpc_provider_response_t *response);

/*
 * Handles server-initiated client.* methods. Request and response payloads are
 * protobuf message bytes. The provider must call respond exactly once before
 * returning GZC_OK.
 */
typedef int (*gzc_rpc_provider_fn)(
    void *userdata,
    int method,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata);

/* Maximum live server-created ServicePeerRPC exchanges per client. */
#define GZC_RPC_MAX_INBOUND_CHANNELS 4u

typedef struct {
  gzc_str_t server_endpoint;
  gzc_str_t private_key;
  const gzc_platform_t *platform;
  const gzc_platform_crypto_t *crypto;
  const gzc_http_vtable_t *http;
  const gzc_webrtc_vtable_t *webrtc;
  gzc_cipher_mode_t cipher_mode;
  int connect_timeout_ms;
  gzc_rpc_provider_fn rpc_provider;
  void *rpc_provider_userdata;
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
int gzc_service_channel_send_frame(gzc_service_channel_t *channel, const gzc_rpc_frame_t *frame);
int gzc_service_channel_read_frame(gzc_service_channel_t *channel, int timeout_ms, gzc_buf_t *out_frame_bytes);
void gzc_service_channel_close(gzc_service_channel_t *channel);

int gzc_client_send_packet(gzc_client_t *client, uint8_t protocol, const uint8_t *payload, size_t len);
int gzc_client_read_packet(gzc_client_t *client, int timeout_ms, uint8_t *out_protocol, gzc_buf_t *out_payload);

#ifdef __cplusplus
}
#endif

#endif
