//go:build cgo

package rpcgen

/*
#cgo CFLAGS: -std=c99 -Wall -Wextra -I${SRCDIR}/../../../c/gizwebrtc/include -I${SRCDIR}/../../../c/gizwebrtc/src -I${SRCDIR}/testdata/golden

#include <string.h>

#include "gzc_common.c"
#include "gzc_buffer.c"
#include "gzc_platform.c"
#include "gzc_json.c"
#include "gzc_rpc_methods.h"
#include "gzc_rpc_encode.c"
#include "gzc_rpc_decode.c"

static int golden_encode_required(void) {
  gzc_ping_request_t req;
  memset(&req, 0, sizeof(req));
  req.client_send_time = 42;

  gzc_buf_t out;
  gzc_buf_init(&out);
  int rc = gzc_ping_request_encode_json(gzc_default_platform(), &req, &out);
  if (rc != GZC_OK) {
    return rc;
  }
  const char *want = "{\"client_send_time\":42}";
  int ok = out.len == strlen(want) && memcmp(out.data, want, out.len) == 0;
  gzc_buf_free(&out, gzc_default_platform());
  return ok ? GZC_OK : GZC_ERR_JSON;
}

static int golden_encode_optional(void) {
  gzc_ping_request_t req;
  memset(&req, 0, sizeof(req));
  req.client_send_time = 42;
  req.has_tag = true;
  req.tag = gzc_str_from_cstr("edge");

  gzc_buf_t out;
  gzc_buf_init(&out);
  int rc = gzc_ping_request_encode_json(gzc_default_platform(), &req, &out);
  if (rc != GZC_OK) {
    return rc;
  }
  const char *want = "{\"client_send_time\":42,\"tag\":\"edge\"}";
  int ok = out.len == strlen(want) && memcmp(out.data, want, out.len) == 0;
  gzc_buf_free(&out, gzc_default_platform());
  return ok ? GZC_OK : GZC_ERR_JSON;
}

static int golden_decode_required(void) {
  gzc_ping_response_t resp;
  memset(&resp, 0, sizeof(resp));
  int rc = gzc_ping_response_decode_json(gzc_str_from_cstr("{\"server_time\":99}"), &resp);
  if (rc != GZC_OK) {
    return rc;
  }
  if (resp.server_time != 99 || resp.has_message) {
    return GZC_ERR_JSON;
  }
  return GZC_OK;
}

static int golden_decode_optional(void) {
  gzc_ping_response_t resp;
  memset(&resp, 0, sizeof(resp));
  int rc = gzc_ping_response_decode_json(gzc_str_from_cstr("{\"message\":\"ok\",\"server_time\":99}"), &resp);
  if (rc != GZC_OK) {
    return rc;
  }
  if (resp.server_time != 99 || !resp.has_message) {
    return GZC_ERR_JSON;
  }
  if (resp.message.len != 2 || memcmp(resp.message.data, "ok", 2) != 0) {
    return GZC_ERR_JSON;
  }
  return GZC_OK;
}

static int golden_method_constant(void) {
  return strcmp(GZC_RPC_METHOD_ALL_PING, "all.ping") == 0 ? GZC_OK : GZC_ERR_JSON;
}
*/
import "C"

func runGoldenCEncodeRequired() int {
	return int(C.golden_encode_required())
}

func runGoldenCEncodeOptional() int {
	return int(C.golden_encode_optional())
}

func runGoldenCDecodeRequired() int {
	return int(C.golden_decode_required())
}

func runGoldenCDecodeOptional() int {
	return int(C.golden_decode_optional())
}

func runGoldenCMethodConstant() int {
	return int(C.golden_method_constant())
}

func goldenCOk() int {
	return int(C.GZC_OK)
}
