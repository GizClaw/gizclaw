/* Golden C fixture for gzc-rpcgen cgo tests. */

#ifndef GZC_RPC_TYPES_H
#define GZC_RPC_TYPES_H

#include "gzc_json.h"

typedef struct {
  int64_t client_send_time;
  bool has_tag;
  gzc_str_t tag;
} gzc_ping_request_t;

typedef struct {
  int64_t server_time;
  bool has_message;
  gzc_str_t message;
} gzc_ping_response_t;

#endif
