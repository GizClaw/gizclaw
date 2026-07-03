/* Golden C fixture for gzc-rpcgen cgo tests. */

#ifndef GZC_RPC_DECODE_H
#define GZC_RPC_DECODE_H

#include "gzc_rpc_types.h"

int gzc_ping_response_decode_json(gzc_str_t json, gzc_ping_response_t *out_value);

#endif
