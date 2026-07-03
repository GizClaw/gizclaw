//go:build cgo

package rpcgen

/*
#cgo CFLAGS: -std=c99 -Wall -Wextra -I${SRCDIR}/../../../c/gizwebrtc/include -I${SRCDIR}/../../../c/gizwebrtc/src -I${SRCDIR}/testdata/golden/want

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
  req.has_trace = true;
  req.trace.raw = gzc_str_from_cstr("{\"trace_id\":\"t-1\"}");

  gzc_buf_t out;
  gzc_buf_init(&out);
  int rc = gzc_ping_request_encode_json(gzc_default_platform(), &req, &out);
  if (rc != GZC_OK) {
    return rc;
  }
  const char *want = "{\"client_send_time\":42,\"tag\":\"edge\",\"trace\":{\"trace_id\":\"t-1\"}}";
  int ok = out.len == strlen(want) && memcmp(out.data, want, out.len) == 0;
  gzc_buf_free(&out, gzc_default_platform());
  return ok ? GZC_OK : GZC_ERR_JSON;
}

static int golden_encode_speed_test(void) {
  gzc_speed_test_request_t req;
  memset(&req, 0, sizeof(req));
  req.down_content_length = 2048;
  req.up_content_length = 1024;
  req.has_payload_hint = true;
  req.payload_hint.raw = gzc_str_from_cstr("{\"pattern\":\"zero\"}");
  req.has_sample_count = true;
  req.sample_count = 3;

  gzc_buf_t out;
  gzc_buf_init(&out);
  int rc = gzc_speed_test_request_encode_json(gzc_default_platform(), &req, &out);
  if (rc != GZC_OK) {
    return rc;
  }
  const char *want = "{\"down_content_length\":2048,\"payload_hint\":{\"pattern\":\"zero\"},\"sample_count\":3,\"up_content_length\":1024}";
  int ok = out.len == strlen(want) && memcmp(out.data, want, out.len) == 0;
  gzc_buf_free(&out, gzc_default_platform());
  return ok ? GZC_OK : GZC_ERR_JSON;
}

static int golden_decode_required(void) {
  gzc_ping_response_t resp;
  memset(&resp, 0, sizeof(resp));
  int rc = gzc_ping_response_decode_json(gzc_str_from_cstr("{\"ok\":true,\"server_time\":99}"), &resp);
  if (rc != GZC_OK) {
    return rc;
  }
  if (resp.server_time != 99 || !resp.ok || resp.has_labels) {
    return GZC_ERR_JSON;
  }
  return GZC_OK;
}

static int golden_decode_optional(void) {
  gzc_server_run_say_response_t resp;
  memset(&resp, 0, sizeof(resp));
  int rc = gzc_server_run_say_response_decode_json(gzc_str_from_cstr("{\"accepted\":true,\"diagnostics\":{\"route\":\"fast\"},\"queue_position\":7}"), &resp);
  if (rc != GZC_OK) {
    return rc;
  }
  if (!resp.accepted || !resp.has_queue_position || resp.queue_position != 7 || !resp.has_diagnostics) {
    return GZC_ERR_JSON;
  }
  const char *want = "{\"route\":\"fast\"}";
  if (resp.diagnostics.raw.len != strlen(want) || memcmp(resp.diagnostics.raw.data, want, resp.diagnostics.raw.len) != 0) {
    return GZC_ERR_JSON;
  }
  return GZC_OK;
}

static int golden_decode_speed_test(void) {
  gzc_speed_test_response_t resp;
  memset(&resp, 0, sizeof(resp));
  int rc = gzc_speed_test_response_decode_json(gzc_str_from_cstr("{\"down_content_length\":2048,\"duration_ms\":12.5,\"samples\":[1.5,2.5],\"up_content_length\":1024}"), &resp);
  if (rc != GZC_OK) {
    return rc;
  }
  if (resp.down_content_length != 2048 || resp.up_content_length != 1024 || resp.duration_ms != 12.5 || !resp.has_samples) {
    return GZC_ERR_JSON;
  }
  const char *want = "[1.5,2.5]";
  if (resp.samples.raw.len != strlen(want) || memcmp(resp.samples.raw.data, want, resp.samples.raw.len) != 0) {
    return GZC_ERR_JSON;
  }
  return GZC_OK;
}

static int golden_method_constant(void) {
  if (strcmp(GZC_RPC_METHOD_ALL_PING, "all.ping") != 0) {
    return GZC_ERR_JSON;
  }
  if (strcmp(GZC_RPC_METHOD_ALL_SPEED_TEST_RUN, "all.speed_test.run") != 0) {
    return GZC_ERR_JSON;
  }
  if (strcmp(GZC_RPC_METHOD_SERVER_RUN_SAY, "server.run.say") != 0) {
    return GZC_ERR_JSON;
  }
  return GZC_OK;
}
*/
import "C"

func runGoldenCEncodeRequired() int {
	return int(C.golden_encode_required())
}

func runGoldenCEncodeOptional() int {
	return int(C.golden_encode_optional())
}

func runGoldenCEncodeSpeedTest() int {
	return int(C.golden_encode_speed_test())
}

func runGoldenCDecodeRequired() int {
	return int(C.golden_decode_required())
}

func runGoldenCDecodeOptional() int {
	return int(C.golden_decode_optional())
}

func runGoldenCDecodeSpeedTest() int {
	return int(C.golden_decode_speed_test())
}

func runGoldenCMethodConstant() int {
	return int(C.golden_method_constant())
}

func goldenCOk() int {
	return int(C.GZC_OK)
}
