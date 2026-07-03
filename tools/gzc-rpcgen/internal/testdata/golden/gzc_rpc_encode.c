/* Golden C fixture for gzc-rpcgen cgo tests. */

#include "gzc_rpc_encode.h"

int gzc_ping_request_encode_json(const gzc_platform_t *platform, const gzc_ping_request_t *value, gzc_buf_t *out_json) {
  if (value == NULL || out_json == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_json_writer_t writer;
  gzc_json_writer_init(&writer, platform, out_json);
  int rc = gzc_json_object_begin(&writer);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_json_field_i64(&writer, "client_send_time", value->client_send_time);
  if (rc != GZC_OK) {
    return rc;
  }
  if (value->has_tag) {
    rc = gzc_json_field_str(&writer, "tag", value->tag);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return gzc_json_object_end(&writer);
}
