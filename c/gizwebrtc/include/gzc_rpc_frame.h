#ifndef GZC_RPC_FRAME_H
#define GZC_RPC_FRAME_H

#include "gzc_platform.h"

#ifdef __cplusplus
extern "C" {
#endif

#define GZC_RPC_MAX_FRAME_SIZE 65535u

typedef enum {
  GZC_RPC_FRAME_EOS = 0,
  GZC_RPC_FRAME_JSON = 1,
  GZC_RPC_FRAME_BINARY = 2,
  GZC_RPC_FRAME_TEXT = 3
} gzc_rpc_frame_type_t;

typedef struct {
  gzc_rpc_frame_type_t type;
  const uint8_t *data;
  size_t len;
} gzc_rpc_frame_t;

bool gzc_rpc_frame_type_valid(gzc_rpc_frame_type_t type);
int gzc_rpc_frame_encode(const gzc_platform_t *platform, const gzc_rpc_frame_t *frame, gzc_buf_t *out_bytes);
int gzc_rpc_frame_decode(const uint8_t *data, size_t len, gzc_rpc_frame_t *out_frame);

#ifdef __cplusplus
}
#endif

#endif
