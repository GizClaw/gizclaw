#ifndef GZC_RPC_H
#define GZC_RPC_H

#include "gzc_client.h"
#include "gzc_json.h"
#include "gzc_rpc_frame.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
  int code;
  gzc_str_t message;
  gzc_str_t data_json;
} gzc_rpc_error_t;

typedef struct {
  gzc_str_t id;
  gzc_str_t result_json;
  bool has_error;
  gzc_rpc_error_t error;
} gzc_rpc_response_t;

typedef int (*gzc_rpc_frame_cb)(void *userdata, const gzc_rpc_frame_t *frame);

int gzc_rpc_encode_request_envelope(
    const gzc_platform_t *platform,
    gzc_str_t id,
    gzc_str_t method,
    gzc_str_t params_json,
    gzc_buf_t *out_json);
int gzc_rpc_decode_response_envelope(gzc_str_t response_json, gzc_rpc_response_t *out_response);
int gzc_rpc_call_json(gzc_client_t *client, gzc_str_t method, gzc_str_t params_json, gzc_rpc_response_t *out_response);
int gzc_rpc_call_stream(
    gzc_client_t *client,
    gzc_str_t method,
    gzc_str_t params_json,
    gzc_rpc_frame_cb on_frame,
    void *userdata);
int gzc_rpc_send_frame(gzc_client_t *client, const gzc_rpc_frame_t *frame);
void gzc_rpc_response_free(gzc_client_t *client, gzc_rpc_response_t *response);

#ifdef __cplusplus
}
#endif

#endif
