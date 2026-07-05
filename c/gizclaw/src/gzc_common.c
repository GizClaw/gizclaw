#include "gzc_common.h"

const char *gzc_status_string(gzc_status_t status) {
  switch (status) {
  case GZC_OK:
    return "ok";
  case GZC_ERR_INVALID_ARGUMENT:
    return "invalid argument";
  case GZC_ERR_NO_MEMORY:
    return "no memory";
  case GZC_ERR_HTTP:
    return "http error";
  case GZC_ERR_WEBRTC:
    return "webrtc error";
  case GZC_ERR_SIGNALING:
    return "signaling error";
  case GZC_ERR_RPC:
    return "rpc error";
  case GZC_ERR_TIMEOUT:
    return "timeout";
  case GZC_ERR_CLOSED:
    return "closed";
  case GZC_ERR_UNSUPPORTED:
    return "unsupported";
  case GZC_ERR_JSON:
    return "json error";
  default:
    return "unknown";
  }
}
