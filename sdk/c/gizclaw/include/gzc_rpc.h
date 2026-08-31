#ifndef GZC_RPC_H
#define GZC_RPC_H

#include "gzc_client.h"
#include "gzc_json.h"
#include "gzc_rpc_frame.h"
#include "payload/ai.pb.h"
#include "payload/edge.pb.h"
#include "payload/enums.pb.h"
#include "payload/firmware.pb.h"
#include "payload/gameplay.pb.h"
#include "payload/social.pb.h"
#include "payload/system.pb.h"
#include "payload/workspace.pb.h"
#include "rpc.pb.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
  int code;
  gzc_str_t message;
  gzc_str_t data_payload;
} gzc_rpc_error_t;

/*
 * Response and nested error strings are borrowed views owned by the request.
 * They remain valid until that request is destroyed.
 */
typedef struct {
  gzc_str_t id;
  gzc_str_t result_payload;
  bool has_error;
  gzc_rpc_error_t error;
} gzc_rpc_response_t;

/*
 * Receives the response envelope, streamed binary frames, and terminating RPC
 * EOS while gzc_client_poll() dispatches the request. frame and frame->data are
 * borrowed until the callback returns. Returning any status other than GZC_OK
 * terminates the request; GZC_ERR_WOULD_BLOCK is normalized to GZC_ERR_RPC
 * because an incoming frame cannot be replayed.
 */
typedef int (*gzc_rpc_frame_cb)(void *userdata, const gzc_rpc_frame_t *frame);

typedef struct gzc_rpc_request gzc_rpc_request_t;

int gzc_rpc_encode_request_envelope(
    const gzc_platform_t *platform,
    gzc_str_t id,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_buf_t *out_payload);
int gzc_rpc_decode_response_envelope(gzc_str_t response_payload, gzc_rpc_response_t *out_response);
/*
 * Starts one unary RPC on a request-owned DataChannel. The caller remains the
 * sole gzc_client_poll() owner; result() never polls. Response views returned
 * by result() remain valid until this request is destroyed.
 */
int gzc_rpc_request_start(
    gzc_client_t *client,
    uint64_t service,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    int timeout_ms,
    gzc_rpc_request_t **out_request);
/*
 * Starts one mixed-frame RPC without closing the request direction. Incoming
 * response, binary data, and EOS frames are delivered from gzc_client_poll().
 * Callback frame storage is borrowed only until the callback returns. The
 * callback must not cancel or destroy its request reentrantly.
 */
int gzc_rpc_request_start_stream(
    gzc_client_t *client,
    uint64_t service,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    int timeout_ms,
    gzc_rpc_frame_cb on_frame,
    void *userdata,
    gzc_rpc_request_t **out_request);
/*
 * Copies and queues one binary frame without polling. Returns
 * GZC_ERR_WOULD_BLOCK while an earlier queued frame is still being written.
 */
int gzc_rpc_request_write(
    gzc_rpc_request_t *request,
    const uint8_t *data,
    size_t len);
/* Queue the request EOS without polling. Idempotent after EOS is queued. */
int gzc_rpc_request_finish_write(gzc_rpc_request_t *request);
/* Returns GZC_ERR_WOULD_BLOCK until the request reaches a terminal state. */
int gzc_rpc_request_result(
    gzc_rpc_request_t *request,
    gzc_rpc_response_t *out_response);
/* Idempotently terminates a pending request with GZC_ERR_CLOSED. */
void gzc_rpc_request_cancel(gzc_rpc_request_t *request);
/* A NULL request is a no-op. The configured platform allocator must outlive it. */
void gzc_rpc_request_destroy(gzc_rpc_request_t *request);
void gzc_rpc_response_free(gzc_client_t *client, gzc_rpc_response_t *response);

#ifdef __cplusplus
}
#endif

#endif
