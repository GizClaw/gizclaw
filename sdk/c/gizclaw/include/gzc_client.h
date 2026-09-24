#ifndef GZC_CLIENT_H
#define GZC_CLIENT_H

#include "gzc_http.h"
#include "gzc_rpc_frame.h"
#include "gzc_signaling.h"
#include "gzc_webrtc.h"
#include "payload/tool.pb.h"

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

/*
 * Consumes one provider response synchronously. The payload and error-message
 * views are borrowed and only need to remain valid until this function
 * returns.
 */
typedef int (*gzc_rpc_provider_respond_fn)(
    void *userdata,
    const gzc_rpc_provider_response_t *response);

/*
 * Handles an installed MHS read or write method. Request and response payloads are
 * protobuf message bytes. request_payload is borrowed until this callback
 * returns. The provider must call respond exactly once before returning GZC_OK.
 */
typedef int (*gzc_rpc_provider_fn)(
    void *userdata,
    int method,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata);

/*
 * Handles one ClientTool. request_payload and the response payload are the
 * encoded messages selected by ClientTool, without the invoke envelope.
 * Views are borrowed for the synchronous call. Validate all arguments before
 * changing hardware; return INVALID_ARGUMENT for rejected arguments.
 */
typedef int (*gzc_tool_handler_fn)(
    void *userdata,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata);

typedef struct {
  gizclaw_rpc_v1_ClientTool tool;
  gzc_tool_handler_fn handler;
  void *userdata;
} gzc_tool_handler_t;

/* Maximum live server-created ServicePeerRPC exchanges per client. */
#define GZC_RPC_MAX_INBOUND_CHANNELS 4u
/* Balance embedded buffering with fewer native DataChannel messages. */
#define GZC_SERVICE_WRITE_CHUNK_SIZE (4u * 1024u)
#define GZC_SERVICE_WRITE_HIGH_WATER_DEFAULT (256u * 1024u)
#define GZC_SERVICE_WRITE_LOW_WATER_DEFAULT (64u * 1024u)
#define GZC_PROTOCOL_OPUS_PACKET 0x10u
#define GZC_OPUS_MAX_PACKET_SIZE 1275u
#define GZC_OPUS_RX_CAPACITY_DEFAULT 64u

typedef struct {
  /*
   * HTTP access point of the Server or Edge. A bare host[:port] resolves to
   * http, and an "http://" or "https://" base URL such as
   * "https://ap.gizclaw.com" selects the scheme explicitly. A path prefix is
   * preserved and a trailing slash is ignored. Any other scheme, a query
   * string, a fragment and userinfo are rejected.
   */
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
  /* Called synchronously after decoding, including unimplemented tools.
   * tool is UNSPECIFIED for a base or MHS method. No SDK mutex is held. */
  void (*rpc_observer)(void *userdata, int method, gizclaw_rpc_v1_ClientTool tool);
  void *rpc_observer_userdata;
  gzc_rpc_provider_fn mhs_read;
  gzc_rpc_provider_fn mhs_write;
  void *mhs_userdata;
  /* Borrowed for the client's lifetime. Only these tools are advertised. */
  const gzc_tool_handler_t *tool_handlers;
  size_t tool_handler_count;
} gzc_client_config_t;

int gzc_client_create(const gzc_client_config_t *config, gzc_client_t **out_client);
int gzc_client_set_peer_add_ice_server(gzc_client_t *client, gzc_peer_add_ice_server_fn fn);
/*
 * Replaces the bounded Opus receive ring before connect. The SDK allocates
 * capacity fixed-size packet slots once through the configured platform
 * allocator. A full ring discards the oldest Opus packet.
 */
int gzc_client_set_opus_rx_capacity(
    gzc_client_t *client,
    size_t capacity);
/* Discards every queued Opus packet on the serialized poll-owner thread. */
int gzc_client_discard_opus_rx(gzc_client_t *client);
/*
 * Registers the borrowed v1 media extension before connect. A normal connect
 * requires this extension. NULL clears the registration before connect.
 * Existing public configuration structs remain ABI-stable.
 */
int gzc_client_set_webrtc_media(
    gzc_client_t *client,
    const gzc_webrtc_media_vtable_t *media);
#define GZC_REGISTRATION_TOKEN_CREDENTIAL_TYPE "gizclaw.com/registration_token"

/* Constructs a GizClaw registration-token credential (value <=512 UTF-8 bytes); rejects an oversized
 * token or embedded NUL. out is only written on success. */
int gzc_registration_token_credential(gzc_str_t token, giznet_v1_AdmissionCredential *out);
/* Copies a structured credential after checking the 128-byte type, 512-byte
 * value and encoded 4096-byte limits.
 * Call before connect on the client owner thread; NULL clears the credential.
 * The client owns the copy until replacement or destroy. Existing structs and
 * connect signatures retain their ABI. Non-NULL credentials require AEAD. */
int gzc_client_set_admission_credential(
    gzc_client_t *client, const giznet_v1_AdmissionCredential *credential);
int gzc_client_connect(gzc_client_t *client);
/*
 * Drives queued WebRTC callbacks and inbound RPC work on the caller's thread.
 * Exactly one serialized caller owns polling; the same loop advances every
 * outstanding gzc_rpc_request_t and server-initiated RPC.
 */
int gzc_client_poll(gzc_client_t *client, int timeout_ms);
int gzc_client_close(gzc_client_t *client);
void gzc_client_destroy(gzc_client_t *client);

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
/*
 * Copies one packet into caller-owned storage without allocating. On
 * GZC_ERR_BUFFER_TOO_SMALL, out_payload_len reports the required capacity and
 * the packet remains queued for retry. out_payload may be NULL only when
 * payload_capacity is zero.
 */
int gzc_client_read_packet_into(
    gzc_client_t *client,
    int timeout_ms,
    uint8_t *out_protocol,
    uint8_t *out_payload,
    size_t payload_capacity,
    size_t *out_payload_len);

#ifdef __cplusplus
}
#endif

#endif
