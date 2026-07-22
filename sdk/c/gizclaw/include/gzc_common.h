#ifndef GZC_COMMON_H
#define GZC_COMMON_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define GZC_API_VERSION 2

typedef enum {
  GZC_OK = 0,
  GZC_ERR_INVALID_ARGUMENT = -1,
  GZC_ERR_NO_MEMORY = -2,
  GZC_ERR_HTTP = -3,
  GZC_ERR_WEBRTC = -4,
  GZC_ERR_SIGNALING = -5,
  GZC_ERR_RPC = -6,
  GZC_ERR_TIMEOUT = -7,
  GZC_ERR_CLOSED = -8,
  GZC_ERR_UNSUPPORTED = -9,
  GZC_ERR_JSON = -10
} gzc_status_t;

const char *gzc_status_string(gzc_status_t status);

#ifdef __cplusplus
}
#endif

#endif
