/* Golden C fixture for gzc-rpcgen cgo tests. */

#ifndef GZC_RPC_ENCODE_H
#define GZC_RPC_ENCODE_H

#include "gzc_rpc_types.h"

int gzc_ping_request_encode_json(const gzc_platform_t *platform, const gzc_ping_request_t *value, gzc_buf_t *out_json);

#endif
