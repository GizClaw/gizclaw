/* Golden C fixture for gzc-rpcgen cgo tests. */

#include "gzc_rpc_decode.h"

#include <string.h>

int gzc_ping_response_decode_json(gzc_str_t json, gzc_ping_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc = gzc_json_find_field(json, "server_time", &raw);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_json_parse_i64(raw, &out_value->server_time);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_OK;
}
