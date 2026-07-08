//go:build cgo

package rpcgen

/*
#cgo CFLAGS: -std=c99 -Wall -Wextra -I${SRCDIR}/../../../sdk/c/gizclaw/include -I${SRCDIR}/../../../sdk/c/gizclaw/src -I${SRCDIR}/testdata/golden/want

#include <string.h>

#include "gzc_common.c"
#include "gzc_buffer.c"
#include "gzc_platform.c"
#include "gzc_json.c"
#include "gzc_rpc_methods.h"
#include "gzc_rpc_encode.c"
#include "gzc_rpc_decode.c"

static int bytes_eq(const gzc_buf_t *out, const uint8_t *want, size_t want_len) {
  return out->len == want_len && memcmp(out->data, want, want_len) == 0;
}

static int golden_encode_required(void) {
  gzc_ping_request_t req;
  memset(&req, 0, sizeof(req));
  req.client_send_time = 42;

  gzc_buf_t out;
  gzc_buf_init(&out);
  int rc = gzc_ping_request_encode_proto(gzc_default_platform(), &req, &out);
  if (rc != GZC_OK) {
    return rc;
  }
  const uint8_t want[] = {0x08, 0x2a};
  int ok = bytes_eq(&out, want, sizeof(want));
  gzc_buf_free(&out, gzc_default_platform());
  return ok ? GZC_OK : GZC_ERR_RPC;
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
  int rc = gzc_ping_request_encode_proto(gzc_default_platform(), &req, &out);
  if (rc != GZC_OK) {
    return rc;
  }
  const uint8_t want_prefix[] = {0x08, 0x2a, 0x12, 0x04, 'e', 'd', 'g', 'e', 0x1a};
  int ok = out.len > sizeof(want_prefix) && memcmp(out.data, want_prefix, sizeof(want_prefix)) == 0;
  gzc_buf_free(&out, gzc_default_platform());
  return ok ? GZC_OK : GZC_ERR_RPC;
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
  int rc = gzc_speed_test_request_encode_proto(gzc_default_platform(), &req, &out);
  if (rc != GZC_OK) {
    return rc;
  }
  const uint8_t want_prefix[] = {0x08, 0x80, 0x10, 0x12};
  int ok = out.len > sizeof(want_prefix) && memcmp(out.data, want_prefix, sizeof(want_prefix)) == 0;
  gzc_buf_free(&out, gzc_default_platform());
  return ok ? GZC_OK : GZC_ERR_RPC;
}

static int golden_decode_required(void) {
  gzc_ping_response_t resp;
  memset(&resp, 0, sizeof(resp));
  const uint8_t payload[] = {0x10, 0x01, 0x18, 0x63};
  int rc = gzc_ping_response_decode_proto(gzc_str_from_parts((const char *)payload, sizeof(payload)), &resp);
  if (rc != GZC_OK) {
    return rc;
  }
  if (resp.server_time != 99 || !resp.ok || resp.has_labels) {
    return GZC_ERR_RPC;
  }
  return GZC_OK;
}

static int golden_decode_optional(void) {
  gzc_server_run_say_response_t resp;
  memset(&resp, 0, sizeof(resp));
  const char *diagnostics = "{\"route\":\"fast\"}";
  uint8_t payload[64];
  size_t n = 0;
  payload[n++] = 0x08;
  payload[n++] = 0x01;
  payload[n++] = 0x12;
  payload[n++] = (uint8_t)strlen(diagnostics);
  memcpy(payload + n, diagnostics, strlen(diagnostics));
  n += strlen(diagnostics);
  payload[n++] = 0x18;
  payload[n++] = 0x07;
  int rc = gzc_server_run_say_response_decode_proto(gzc_str_from_parts((const char *)payload, n), &resp);
  if (rc != GZC_OK) {
    return rc;
  }
  if (!resp.accepted || !resp.has_queue_position || resp.queue_position != 7 || !resp.has_diagnostics) {
    return GZC_ERR_RPC;
  }
  const char *want = "{\"route\":\"fast\"}";
  if (resp.diagnostics.raw.len != strlen(want) || memcmp(resp.diagnostics.raw.data, want, resp.diagnostics.raw.len) != 0) {
    return GZC_ERR_RPC;
  }
  return GZC_OK;
}

static int golden_decode_speed_test(void) {
  gzc_speed_test_response_t resp;
  memset(&resp, 0, sizeof(resp));
  const uint8_t payload[] = {
    0x08, 0x80, 0x10,
    0x11, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x29, 0x40,
    0x1a, 0x09, '[', '1', '.', '5', ',', '2', '.', '5', ']',
    0x20, 0x80, 0x08,
  };
  int rc = gzc_speed_test_response_decode_proto(gzc_str_from_parts((const char *)payload, sizeof(payload)), &resp);
  if (rc != GZC_OK) {
    return rc;
  }
  if (resp.down_content_length != 2048 || resp.up_content_length != 1024 || resp.duration_ms != 12.5 || !resp.has_samples) {
    return GZC_ERR_RPC;
  }
  const char *want = "[1.5,2.5]";
  if (resp.samples.raw.len != strlen(want) || memcmp(resp.samples.raw.data, want, resp.samples.raw.len) != 0) {
    return GZC_ERR_RPC;
  }
  return GZC_OK;
}

static int golden_method_constant(void) {
  if (strcmp(GZC_RPC_METHOD_ALL_PING, "all.ping") != 0) {
    return GZC_ERR_RPC;
  }
  if (strcmp(GZC_RPC_METHOD_ALL_SPEED_TEST_RUN, "all.speed_test.run") != 0) {
    return GZC_ERR_RPC;
  }
  if (strcmp(GZC_RPC_METHOD_SERVER_RUN_SAY, "server.run.say") != 0) {
    return GZC_ERR_RPC;
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
