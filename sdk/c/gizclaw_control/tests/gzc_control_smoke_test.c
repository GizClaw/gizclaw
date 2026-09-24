/*
 * Offline smoke test for the controller-side C SDK.
 *
 * A stub gzc_http_vtable_t records the request the SDK built and replays a
 * canned response, so the test covers URL construction, request encoding,
 * response decoding, the error classification table, and the caller-owned
 * buffer contract without a network.
 */
#include "gzc_control.h"

#include <math.h>
#include <stdio.h>
#include <string.h>

static int failures;

static void check(bool condition, const char *what) {
  if (!condition) {
    failures++;
    (void)fprintf(stderr, "FAIL %s\n", what);
  }
}

static void check_str(gzc_str_t actual, const char *expected, const char *what) {
  size_t expected_len = strlen(expected);
  bool equal = actual.len == expected_len &&
               (expected_len == 0 || (actual.data != NULL && memcmp(actual.data, expected, expected_len) == 0));
  if (!equal) {
    failures++;
    (void)fprintf(
        stderr, "FAIL %s: got %.*s, want %s\n", what, (int)actual.len,
        actual.data == NULL ? "" : actual.data, expected);
  }
}

/* Transport stub: captures one request and answers with a canned response. */
typedef struct {
  char url[512];
  char body[64 * 1024];
  gzc_http_method_t method;
  bool saw_authorization;
  bool saw_content_type;
  int status_code;
  const char *response_body;
  const char *request_id;
  int result;
  int free_calls;
} stub_t;

static int stub_request(void *userdata, const gzc_http_request_t *request, gzc_http_response_t *out_response) {
  stub_t *stub = (stub_t *)userdata;
  stub->method = request->method;
  size_t url_len = request->url.len < sizeof(stub->url) - 1 ? request->url.len : sizeof(stub->url) - 1;
  memcpy(stub->url, request->url.data, url_len);
  stub->url[url_len] = 0;
  size_t body_len = request->body_len < sizeof(stub->body) - 1 ? request->body_len : sizeof(stub->body) - 1;
  if (body_len > 0) {
    memcpy(stub->body, request->body, body_len);
  }
  stub->body[body_len] = 0;
  stub->saw_authorization = false;
  stub->saw_content_type = false;
  for (size_t i = 0; i < request->header_count; i++) {
    if (request->headers[i].name.len == 13 &&
        memcmp(request->headers[i].name.data, "Authorization", 13) == 0) {
      stub->saw_authorization = true;
    }
    if (request->headers[i].name.len == 12 &&
        memcmp(request->headers[i].name.data, "Content-Type", 12) == 0) {
      stub->saw_content_type = true;
    }
  }
  if (stub->result != GZC_OK) {
    return stub->result;
  }
  /* Replay the response headers through the platform contract's sink. */
  int header_rc = gzc_http_deliver_response_header(request, "Content-Type", 12, "application/json", 16);
  if (header_rc == GZC_OK && stub->request_id != NULL) {
    header_rc = gzc_http_deliver_response_header(
        request, "x-request-id", 12, stub->request_id, strlen(stub->request_id));
  }
  if (header_rc != GZC_OK) {
    return header_rc;
  }
  out_response->status_code = stub->status_code;
  size_t length = stub->response_body == NULL ? 0 : strlen(stub->response_body);
  if (length > request->response_buf_cap) {
    return GZC_ERR_BUFFER_TOO_SMALL;
  }
  if (length > 0) {
    memcpy(request->response_buf, stub->response_body, length);
  }
  out_response->body.data = request->response_buf;
  out_response->body.len = length;
  out_response->body.cap = request->response_buf_cap;
  out_response->content_length = (int64_t)length;
  return GZC_OK;
}

static void stub_response_free(void *userdata, gzc_http_response_t *response) {
  stub_t *stub = (stub_t *)userdata;
  stub->free_calls++;
  /* The body lives in the caller's region; nothing to release. */
  (void)response;
}

static void init_client(gzc_control_client_t *client, stub_t *stub, gzc_http_vtable_t *http) {
  memset(http, 0, sizeof(*http));
  http->userdata = stub;
  http->request = stub_request;
  http->response_free = stub_response_free;
  gzc_control_config_t config;
  memset(&config, 0, sizeof(config));
  config.base_url = gzc_str_from_cstr("https://ap.gizclaw.com/");
  config.api_key = gzc_str_from_cstr("Bearer gizclaw_sk_v1_example");
  config.http = http;
  check(gzc_control_client_init(client, &config) == GZC_OK, "client init");
}

static void test_client_init_rejects_bad_config(void) {
  gzc_control_client_t client;
  gzc_control_config_t config;
  stub_t stub;
  gzc_http_vtable_t http;
  memset(&stub, 0, sizeof(stub));
  memset(&http, 0, sizeof(http));
  http.userdata = &stub;
  http.request = stub_request;

  memset(&config, 0, sizeof(config));
  config.api_key = gzc_str_from_cstr("k");
  config.http = &http;
  check(gzc_control_client_init(&client, &config) == GZC_ERR_INVALID_ARGUMENT, "empty base_url rejected");

  memset(&config, 0, sizeof(config));
  config.base_url = gzc_str_from_cstr("https://ap.gizclaw.com");
  config.http = &http;
  check(gzc_control_client_init(&client, &config) == GZC_ERR_INVALID_ARGUMENT, "empty api_key rejected");

  memset(&config, 0, sizeof(config));
  config.base_url = gzc_str_from_cstr("https://ap.gizclaw.com");
  config.api_key = gzc_str_from_cstr("k");
  check(gzc_control_client_init(&client, &config) == GZC_ERR_INVALID_ARGUMENT, "missing transport rejected");
}

static void test_get_device_status(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body =
      "{\"reported_at\":\"2026-09-03T00:00:00Z\",\"volume\":35,\"muted\":false,"
      "\"battery_percent\":88,\"gnss_latitude\":31.2,\"labels\":{\"room\":\"lab\"},"
      "\"details\":{\"firmware\":\"1.0.0\"}}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_peer_status_t status;
  check(gzc_control_get_device_status(&client, &call, &status) == GZC_OK, "get status");
  check(stub.method == GZC_HTTP_METHOD_GET, "status method");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/status") == 0, "status url");
  check(stub.saw_authorization, "status sends Authorization");
  check(!stub.saw_content_type, "bodyless request sends no Content-Type");
  check(stub.free_calls == 1, "response released");
  check(status.has_volume && status.volume == 35, "status volume");
  check(status.has_muted && !status.muted, "status muted");
  check(status.has_battery_percent && status.battery_percent == 88, "status battery");
  check(!status.has_charging, "absent status field stays unset");
  check(status.has_gnss_latitude, "status gnss latitude present");
  check_str(status.reported_at, "2026-09-03T00:00:00Z", "status reported_at");
  check(status.raw.len == strlen(stub.response_body), "status raw covers the response");

  gzc_control_pair_t labels[4];
  size_t label_count = 0;
  check(gzc_control_peer_status_labels(&status, labels, 4, &label_count) == GZC_OK, "decode labels");
  check(label_count == 1, "label count");
  check_str(labels[0].key, "room", "label key");
  check_str(labels[0].value, "lab", "label value");
}

/*
 * A label scan must delimit escaped keys and values correctly. Whether the
 * escape decodes is gzc_json_parse_string's call, the same as for every other
 * string field, but the scan itself must not mistake an escaped quote for the
 * end of the token.
 */
static void test_peer_status_labels_span_escapes(void) {
  gzc_control_peer_status_t status;
  gzc_control_pair_t labels[4];
  size_t count = 0;

  memset(&status, 0, sizeof(status));
  status.labels = gzc_str_from_cstr("{\"a\\\"b\":\"v\", \"plain\":\"w\"}");
  int rc = gzc_control_peer_status_labels(&status, labels, 4, &count);
  /* The escaped key is delimited across the quote, so the scan reaches the
   * second pair rather than stopping early or misreading the object. */
  check(rc == GZC_ERR_UNSUPPORTED, "an escaped label defers to the JSON codec");

  memset(&status, 0, sizeof(status));
  status.labels = gzc_str_from_cstr("{ \"room\" : \"lab\" , \"line\" : \"a\" }");
  check(gzc_control_peer_status_labels(&status, labels, 4, &count) == GZC_OK, "whitespace is skipped");
  check(count == 2, "both labels decoded");
  check_str(labels[1].key, "line", "second label key");

  memset(&status, 0, sizeof(status));
  status.labels = gzc_str_from_cstr("{}");
  check(gzc_control_peer_status_labels(&status, labels, 4, &count) == GZC_OK, "empty labels");
  check(count == 0, "empty labels decode to none");

  memset(&status, 0, sizeof(status));
  status.labels = gzc_str_from_cstr("{\"a\":\"1\",\"b\":\"2\"}");
  check(
      gzc_control_peer_status_labels(&status, labels, 1, &count) == GZC_ERR_BUFFER_TOO_SMALL,
      "a small label array reports overflow");
}

static void test_mhs_volume_encodes_body(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body = "{\"states\":[{\"device_id\":\"audio.main\",\"state\":\"volume\",\"value\":35},{\"device_id\":\"audio.main\",\"state\":\"muted\",\"value\":false}]}";
  init_client(&client, &stub, &http);
  uint8_t scratch[1024], response[1024];
  char strings[128];
  gzc_control_mhs_v0_storage_t storage = {strings, sizeof(strings), 0};
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");
  gzc_control_mhs_v0_state_value_t request[2] = {0}, applied[2];
  request[0].device_id = request[1].device_id = gzc_str_from_cstr("audio.main");
  request[0].state = gzc_str_from_cstr("volume");
  request[0].value.kind = GZC_CONTROL_MHS_V0_VALUE_INT;
  request[0].value.int_value = 35;
  request[1].state = gzc_str_from_cstr("muted");
  request[1].value.kind = GZC_CONTROL_MHS_V0_VALUE_BOOL;
  size_t count;
  check(gzc_control_write_mhs_v0_states(&client, &call, request, 2, &storage, applied, 2, &count) == GZC_OK, "write volume states");
  check(stub.method == GZC_HTTP_METHOD_PATCH, "volume method");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/mhs/v0/states") == 0, "volume route");
  check(strcmp(stub.body, stub.response_body) == 0, "volume and mute encoded together");
  check(stub.saw_content_type, "body request sends Content-Type");
  check(count == 2 && applied[0].value.int_value == 35 && !applied[1].value.bool_value, "applied states");
}

static void test_query_parameters_and_encoding(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body =
      "{\"peer_public_key\":\"pk\",\"field\":\"battery.percent\",\"start_time_ms\":1,"
      "\"end_time_ms\":2,\"step_ms\":1,\"points\":[{\"observed_at_unix_ms\":1,\"value\":88}]}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_telemetry_query_t query;
  memset(&query, 0, sizeof(query));
  query.field = gzc_str_from_cstr(GZC_CONTROL_TELEMETRY_BATTERY_PERCENT);
  query.start_time_ms = 1;
  query.end_time_ms = 2;
  query.step_ms = 1;
  query.order = GZC_CONTROL_ORDER_DESC;
  gzc_control_telemetry_point_t points[4];
  size_t count = 0;
  check(gzc_control_query_device_telemetry(&client, &call, &query, points, 4, &count) == GZC_OK, "query telemetry");
  check(
      strcmp(
          stub.url,
          "https://ap.gizclaw.com/gizclaw/v1/device/telemetry?field=battery.percent"
          "&start_time_ms=1&end_time_ms=2&step_ms=1&order=desc") == 0,
      "telemetry url");
  check(count == 1 && points[0].observed_at_unix_ms == 1, "telemetry points");

  stub.response_body = "{\"peer_public_key\":\"pk\",\"values\":[{\"field\":\"battery.percent\",\"value\":88,\"observed_at_unix_ms\":1}]}";
  gzc_control_telemetry_value_t latest[1];
  gzc_str_t peer;
  check(gzc_control_get_device_telemetry_latest(&client, &call,
                                                gzc_str_from_cstr(GZC_CONTROL_TELEMETRY_BATTERY_PERCENT), latest, 1, &count, &peer) == GZC_OK,
        "latest telemetry");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/telemetry/battery.percent/latest") == 0,
        "latest field path");
  check(count == 1 && latest[0].value == 88, "latest telemetry value");
  check(gzc_control_get_device_telemetry_latest(&client, &call,
                                                gzc_str_from_parts(NULL, 0), latest, 1, &count, &peer) == GZC_ERR_INVALID_ARGUMENT,
        "latest requires a field");

  /* A path segment with reserved characters is percent-encoded. */
  stub.status_code = 200;
  stub.response_body = "{\"result\":{}}";
  check(
      gzc_control_forget_device_saved_wifi(&client, &call, gzc_str_from_cstr("home wifi/2G")) == GZC_OK,
      "forget saved wifi");
  check(
      strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/tool/v0/invoke") == 0,
      "encoded ssid segment");
  check(stub.method == GZC_HTTP_METHOD_POST, "forget method");
  check(strstr(stub.body, "home wifi/2G") != NULL, "SSID is carried in typed args");
}

/*
 * The contract caps are documented, not enforced: an oversized value must
 * reach the Server so the caller observes its 400 rather than a local
 * rejection the sibling SDKs do not make.
 */
static void test_contract_caps_are_not_enforced_locally(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 400;
  stub.response_body = "{\"error\":{\"code\":\"INVALID_REQUEST\",\"message\":\"sound too long\"}}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[512];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  char oversized[GZC_CONTROL_MAX_SOUND_BYTES + 2];
  memset(oversized, 'a', sizeof(oversized) - 1);
  oversized[sizeof(oversized) - 1] = 0;
  gzc_control_play_sound_request_t sound;
  memset(&sound, 0, sizeof(sound));
  sound.sound = gzc_str_from_cstr(oversized);
  check(gzc_control_play_device_sound(&client, &call, &sound) == GZC_ERR_HTTP, "oversized sound is sent");
  check(call.error.kind == GZC_CONTROL_ERROR_INVALID_REQUEST, "the Server's 400 is what classifies");

  char long_ssid[GZC_CONTROL_MAX_SSID_BYTES + 2];
  memset(long_ssid, 'b', sizeof(long_ssid) - 1);
  long_ssid[sizeof(long_ssid) - 1] = 0;
  check(
      gzc_control_forget_device_saved_wifi(&client, &call, gzc_str_from_cstr(long_ssid)) == GZC_ERR_HTTP,
      "oversized ssid is sent");

  gzc_control_mhs_v0_state_value_t volume = {0}, applied;
  volume.device_id = gzc_str_from_cstr("audio.main");
  volume.state = gzc_str_from_cstr("volume");
  volume.value.kind = GZC_CONTROL_MHS_V0_VALUE_INT;
  volume.value.int_value = 101;
  char strings[128];
  gzc_control_mhs_v0_storage_t storage = {strings, sizeof(strings), 0};
  size_t count;
  check(gzc_control_write_mhs_v0_states(&client, &call, &volume, 1, &storage, &applied, 1, &count) == GZC_ERR_HTTP,
        "out-of-range product volume is validated by Server");
  check(call.error.kind == GZC_CONTROL_ERROR_INVALID_REQUEST, "invalid product value classified");

  /* A value that cannot form a request at all is still refused locally. */
  memset(&sound, 0, sizeof(sound));
  check(
      gzc_control_play_device_sound(&client, &call, &sound) == GZC_ERR_INVALID_ARGUMENT,
      "an empty sound is refused");
  check(
      gzc_control_forget_device_saved_wifi(&client, &call, gzc_str_from_parts(NULL, 0)) ==
          GZC_ERR_INVALID_ARGUMENT,
      "an empty ssid segment is refused");

  stub.status_code = 200;
  stub.response_body = "{\"result\":{}}";
  sound.sound = gzc_str_from_cstr("chime");
  sound.has_duration_ms = true;
  sound.duration_ms = 1200;
  check(gzc_control_play_device_sound(&client, &call, &sound) == GZC_OK, "play sound");
  check(strcmp(stub.body, "{\"tool\":\"sound.play\",\"args\":{\"sound\":\"chime\",\"duration_ms\":1200}}") == 0, "play sound body");
}

/*
 * The find body is optional: NULL and an absent duration both send `{}` so
 * the device picks its own ring time, and a negative duration still reaches
 * the Server, whose 400 is what classifies it.
 */
static void test_find_device(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body = "{\"result\":{}}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[512];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  check(gzc_control_find_device(&client, &call, NULL) == GZC_OK, "find with device default");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/tool/v0/invoke") == 0, "find path");
  check(stub.method == GZC_HTTP_METHOD_POST, "find method");
  check(strcmp(stub.body, "{\"tool\":\"device.find\",\"args\":{}}") == 0, "find without request sends an empty object");
  check(call.status_code == 200, "find status");

  gzc_control_find_request_t find;
  memset(&find, 0, sizeof(find));
  check(gzc_control_find_device(&client, &call, &find) == GZC_OK, "find without duration");
  check(strcmp(stub.body, "{\"tool\":\"device.find\",\"args\":{}}") == 0, "absent duration is omitted");

  find.has_duration_ms = true;
  find.duration_ms = 8000;
  check(gzc_control_find_device(&client, &call, &find) == GZC_OK, "find with duration");
  check(strcmp(stub.body, "{\"tool\":\"device.find\",\"args\":{\"duration_ms\":8000}}") == 0, "find duration body");

  stub.status_code = 400;
  stub.response_body = "{\"error\":{\"code\":\"INVALID_REQUEST\",\"message\":\"duration_ms must be non-negative\"}}";
  find.duration_ms = -1;
  check(gzc_control_find_device(&client, &call, &find) == GZC_ERR_HTTP, "negative duration is sent");
  check(strcmp(stub.body, "{\"tool\":\"device.find\",\"args\":{\"duration_ms\":-1}}") == 0, "negative duration body");
  check(call.error.kind == GZC_CONTROL_ERROR_INVALID_REQUEST, "negative duration classifies on the response");

  stub.status_code = 501;
  stub.response_body = "{\"error\":{\"code\":\"DEVICE_UNSUPPORTED\",\"message\":\"device does not support find\"}}";
  check(gzc_control_find_device(&client, &call, NULL) == GZC_ERR_HTTP, "unsupported find fails");
  check(call.error.kind == GZC_CONTROL_ERROR_DEVICE_UNSUPPORTED, "unsupported find classifies");

  check(gzc_control_find_device(NULL, &call, NULL) == GZC_ERR_INVALID_ARGUMENT, "find requires a client");
  check(gzc_control_find_device(&client, NULL, NULL) == GZC_ERR_INVALID_ARGUMENT, "find requires a call");
}

/* The header sink must reject header injection identically in every backend. */
static void test_response_header_validation(void) {
  gzc_http_request_t request;
  memset(&request, 0, sizeof(request));
  check(
      gzc_http_deliver_response_header(&request, "X-Request-ID", 12, "abc", 3) == GZC_OK,
      "header without a sink is accepted");
  check(
      gzc_http_deliver_response_header(NULL, "X", 1, "a", 1) == GZC_ERR_INVALID_ARGUMENT,
      "null request rejected");
  check(
      gzc_http_deliver_response_header(&request, "", 0, "a", 1) == GZC_ERR_INVALID_ARGUMENT,
      "empty header name rejected");
  check(
      gzc_http_deliver_response_header(&request, "X\nY", 3, "a", 1) == GZC_ERR_INVALID_ARGUMENT,
      "newline in header name rejected");
  check(
      gzc_http_deliver_response_header(&request, "X", 1, "a\r", 2) == GZC_ERR_INVALID_ARGUMENT,
      "carriage return in header value rejected");
}

static void test_error_classification(void) {
  struct {
    int status;
    const char *code;
    gzc_control_error_kind_t want;
  } cases[] = {
      {401, "", GZC_CONTROL_ERROR_UNAUTHORIZED},
      {403, "", GZC_CONTROL_ERROR_FORBIDDEN},
      {404, "", GZC_CONTROL_ERROR_NOT_FOUND},
      {409, "DEVICE_OFFLINE", GZC_CONTROL_ERROR_DEVICE_OFFLINE},
      {504, "DEVICE_TIMEOUT", GZC_CONTROL_ERROR_DEVICE_TIMEOUT},
      {400, "DEVICE_REJECTED", GZC_CONTROL_ERROR_DEVICE_REJECTED},
      {501, "DEVICE_UNSUPPORTED", GZC_CONTROL_ERROR_DEVICE_UNSUPPORTED},
      {502, "DEVICE_ERROR", GZC_CONTROL_ERROR_DEVICE_ERROR},
      {409, "CONTACT_EXISTS", GZC_CONTROL_ERROR_CONFLICT},
      {400, "INVALID_ARGUMENT", GZC_CONTROL_ERROR_INVALID_REQUEST},
      {500, "INTERNAL", GZC_CONTROL_ERROR_SERVER},
      {503, "", GZC_CONTROL_ERROR_SERVER},
      {418, "", GZC_CONTROL_ERROR_UNEXPECTED_STATUS},
      /* A DEVICE_* code wins over the status, matching the sibling SDKs. */
      {500, "DEVICE_OFFLINE", GZC_CONTROL_ERROR_DEVICE_OFFLINE},
  };
  for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
    gzc_control_error_kind_t got = gzc_control_classify(cases[i].status, gzc_str_from_cstr(cases[i].code));
    if (got != cases[i].want) {
      failures++;
      (void)fprintf(
          stderr, "FAIL classify(%d, %s) = %s, want %s\n", cases[i].status, cases[i].code,
          gzc_control_error_kind_string(got), gzc_control_error_kind_string(cases[i].want));
    }
  }
}

static void test_error_response_is_decoded(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 409;
  stub.request_id = "req-1";
  stub.response_body =
      "{\"error\":{\"code\":\"DEVICE_OFFLINE\",\"message\":\"device is not connected\","
      "\"details\":{\"peer\":\"pk\"}}}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[512];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_peer_status_t status;
  check(gzc_control_read_device_status(&client, &call, &status) == GZC_ERR_HTTP, "offline call fails");
  check(call.error.kind == GZC_CONTROL_ERROR_DEVICE_OFFLINE, "offline kind");
  check(call.error.status_code == 409, "offline status");
  check_str(call.error.code, "DEVICE_OFFLINE", "offline code");
  check_str(call.error.message, "device is not connected", "offline message");
  check_str(call.error.details, "{\"peer\":\"pk\"}", "offline details");
  check_str(call.error.request_id, "req-1", "offline request id");

  /* A non-2xx body that is not an ErrorResponse still classifies by status. */
  stub.status_code = 502;
  stub.response_body = "upstream failure";
  check(gzc_control_read_device_status(&client, &call, &status) == GZC_ERR_HTTP, "bad gateway fails");
  check(call.error.kind == GZC_CONTROL_ERROR_SERVER, "bad gateway kind");
  check(call.error.code.len == 0, "bad gateway has no code");
}

static void test_transport_failure_is_network(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.result = GZC_ERR_TIMEOUT;
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[256];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_device_runtime_t runtime;
  check(gzc_control_get_device_runtime(&client, &call, &runtime) == GZC_ERR_TIMEOUT, "transport failure returns");
  check(call.error.kind == GZC_CONTROL_ERROR_NETWORK, "transport failure kind");
  check(call.status_code == 0, "transport failure has no status");
}

static void test_scratch_exhaustion_is_reported(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body = "{}";
  init_client(&client, &stub, &http);

  /* Too small to hold the URL, so the call fails before any transport use. */
  uint8_t scratch[8];
  uint8_t response[64];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_peer_status_t status;
  check(gzc_control_get_device_status(&client, &call, &status) == GZC_ERR_NO_MEMORY, "small scratch fails");
  check(call.error.kind == GZC_CONTROL_ERROR_NETWORK, "small scratch kind");
  check(stub.url[0] == 0, "no request was sent");
}

static void test_lists_and_malformed_bodies(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body =
      "{\"items\":[{\"name\":\"c1\",\"display_name\":\"Ann\"},{\"name\":\"c2\"}],"
      "\"has_next\":true,\"next_cursor\":\"cur\"}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_page_t page;
  memset(&page, 0, sizeof(page));
  page.has_limit = true;
  page.limit = 2;
  gzc_control_contact_t contacts[4];
  size_t count = 0;
  bool has_next = false;
  gzc_str_t cursor;
  check(
      gzc_control_list_contacts(&client, &call, &page, contacts, 4, &count, &has_next, &cursor) == GZC_OK,
      "list contacts");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/contacts?limit=2") == 0, "contacts url");

  /* A zero limit is a real request the Server rejects, not an absent one. */
  gzc_control_page_t zero;
  memset(&zero, 0, sizeof(zero));
  zero.has_limit = true;
  (void)gzc_control_list_contacts(&client, &call, &zero, contacts, 4, &count, &has_next, &cursor);
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/contacts?limit=0") == 0, "zero limit is sent");
  memset(&zero, 0, sizeof(zero));
  (void)gzc_control_list_contacts(&client, &call, &zero, contacts, 4, &count, &has_next, &cursor);
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/contacts") == 0, "absent limit is omitted");
  check(count == 2 && has_next, "contacts page");
  check_str(contacts[0].display_name, "Ann", "contact display name");
  check(contacts[1].display_name.len == 0, "absent display name stays empty");
  check_str(cursor, "cur", "contacts cursor");

  /* A caller array smaller than the page reports the overflow. */
  check(
      gzc_control_list_contacts(&client, &call, &page, contacts, 1, &count, &has_next, &cursor) ==
          GZC_ERR_BUFFER_TOO_SMALL,
      "small contact array reports overflow");
  check(call.error.kind == GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL, "overflow is a caller-capacity failure");

  stub.response_body = "not json";
  check(
      gzc_control_list_contacts(&client, &call, &page, contacts, 4, &count, &has_next, &cursor) != GZC_OK,
      "malformed body fails");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "malformed body classified");
}

static void test_device_runtime_profile(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body =
      "{\"name\":\"h106-tiga\",\"revision\":\"rev-1\",\"workflows\":["
      "{\"name\":\"story.aesop\",\"tags\":[\"9-12\",\"stories\"]},"
      "{\"name\":\"story.alice\",\"tags\":[\"6-8\",\"stories\"]}]}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_device_runtime_profile_t profile;
  gzc_control_runtime_profile_workflow_t workflows[4];
  size_t count = 0;
  gzc_str_t selector[] = {gzc_str_from_cstr("6-8"), gzc_str_from_cstr("stories")};
  check(
      gzc_control_get_device_runtime_profile(&client, &call, selector, 2, &profile, workflows, 4, &count) == GZC_OK,
      "get runtime profile");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/runtime-profile?tags=6-8&tags=stories") == 0,
        "runtime profile tag url");
  check_str(profile.name, "h106-tiga", "runtime profile name");
  check_str(profile.revision, "rev-1", "runtime profile revision");
  check(count == 2, "runtime profile workflow count");
  check_str(workflows[0].name, "story.aesop", "first workflow");
  check_str(workflows[1].name, "story.alice", "second workflow");

  gzc_str_t tags[4];
  size_t tag_count = 0;
  check(gzc_control_runtime_profile_workflow_tags(&workflows[1], tags, 4, &tag_count) == GZC_OK && tag_count == 2,
        "workflow tags");
  check_str(tags[0], "6-8", "first tag");
  check_str(tags[1], "stories", "second tag");
  check(gzc_control_runtime_profile_workflow_tags(&workflows[1], tags, 1, &tag_count) == GZC_ERR_BUFFER_TOO_SMALL,
        "small tag array reports overflow");
  check(gzc_control_runtime_profile_workflow_tags(NULL, tags, 4, &tag_count) == GZC_ERR_INVALID_ARGUMENT,
        "null workflow is rejected");
  check(gzc_control_get_device_runtime_profile(&client, &call, NULL, 1, &profile, workflows, 4, &count) ==
            GZC_ERR_INVALID_ARGUMENT,
        "null tag array is rejected");
  check(gzc_control_get_device_runtime_profile(&client, &call, NULL, 0, &profile, NULL, 4, &count) ==
            GZC_ERR_INVALID_ARGUMENT,
        "null workflow array with capacity is rejected");
  check(gzc_control_get_device_runtime_profile(&client, &call, NULL, 0, &profile, workflows, 1, &count) ==
            GZC_ERR_BUFFER_TOO_SMALL,
        "small workflow array reports overflow");
  check(call.error.kind == GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL, "workflow overflow kind");

  stub.response_body = "{\"name\":\"h106-tiga\",\"revision\":\"rev-1\",\"workflows\":[{\"name\":\"bad\"}]}";
  check(gzc_control_get_device_runtime_profile(&client, &call, NULL, 0, &profile, workflows, 4, &count) != GZC_OK,
        "workflow without tags fails");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "missing tags is malformed");

  stub.response_body = "{\"name\":\"h106-tiga\",\"revision\":\"rev-1\"}";
  check(gzc_control_get_device_runtime_profile(&client, &call, NULL, 0, &profile, workflows, 4, &count) != GZC_OK,
        "missing workflows fails");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "missing workflows is malformed");

  stub.status_code = 403;
  stub.response_body = "{\"error\":{\"code\":\"API_KEY_OWNER_UNAVAILABLE\",\"message\":\"Forbidden\"}}";
  check(gzc_control_get_device_runtime_profile(&client, &call, NULL, 0, &profile, workflows, 4, &count) != GZC_OK,
        "unbound owner fails");
  check(call.error.kind == GZC_CONTROL_ERROR_FORBIDDEN, "unbound owner is forbidden");
}

static void test_device_workspaces(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body =
      "[{\"id\":\"ws-aesop\",\"name\":\"aesop-save\","
      "\"workflow_name\":\"story.aesop\",\"available\":true,\"system\":false,"
      "\"created_at\":\"2026-09-01T08:00:00Z\",\"updated_at\":\"2026-09-01T09:00:00Z\","
      "\"last_active_at\":\"2026-09-01T10:00:00Z\"},"
      "{\"id\":\"ws-pet\",\"name\":\"pet\",\"available\":false,\"system\":true,"
      "\"created_at\":\"2026-09-01T08:00:00Z\",\"updated_at\":\"2026-09-01T08:00:00Z\","
      "\"last_active_at\":\"2026-09-01T08:00:00Z\"}]";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_workspace_filter_t filter = {
      .workflow_name = gzc_str_from_cstr("story.aesop"),
  };
  gzc_control_device_workspace_t items[4];
  size_t count = 0;
  check(gzc_control_list_device_workspaces(&client, &call, &filter, items, 4, &count) == GZC_OK, "list workspaces");
  check(stub.method == GZC_HTTP_METHOD_GET, "list workspaces method");
  check(
      strcmp(
          stub.url,
          "https://ap.gizclaw.com/gizclaw/v1/device/workspaces?workflow_name=story.aesop") ==
          0,
      "list workspaces url");
  check(count == 2, "workspace count");
  check_str(items[0].id, "ws-aesop", "workspace id");
  check_str(items[0].workflow_name, "story.aesop", "workspace workflow name");
  check(items[0].available && !items[0].system, "workspace flags");
  check_str(items[0].last_active_at, "2026-09-01T10:00:00Z", "workspace last active");
  check(items[1].system && !items[1].available, "system workspace flags");
  check(items[1].workflow_name.len == 0, "unresolved workflow name is empty");

  check(gzc_control_list_device_workspaces(&client, &call, NULL, items, 4, &count) == GZC_OK, "unfiltered list");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/workspaces") == 0, "unfiltered url");
  check(
      gzc_control_list_device_workspaces(&client, &call, NULL, items, 1, &count) == GZC_ERR_BUFFER_TOO_SMALL,
      "small workspace array reports overflow");
  check(call.error.kind == GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL, "workspace overflow kind");
  check(
      gzc_control_list_device_workspaces(&client, &call, NULL, NULL, 4, &count) == GZC_ERR_INVALID_ARGUMENT,
      "null workspace array with capacity is rejected");

  stub.response_body = "{\"items\":[]}";
  check(gzc_control_list_device_workspaces(&client, &call, NULL, items, 4, &count) != GZC_OK, "object body fails");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "object body is malformed");
  stub.response_body = "[{\"id\":\"ws\",\"name\":\"n\",\"system\":false}]";
  check(gzc_control_list_device_workspaces(&client, &call, NULL, items, 4, &count) != GZC_OK, "missing fields fail");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "missing fields are malformed");

  stub.status_code = 202;
  stub.response_body = "";
  check(
      gzc_control_delete_device_workspace(&client, &call, gzc_str_from_cstr("ws/aesop")) == GZC_OK,
      "delete workspace");
  check(stub.method == GZC_HTTP_METHOD_DELETE, "delete workspace method");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/workspaces/ws%2Faesop") == 0, "delete url");
  check(
      gzc_control_delete_device_workspace(&client, &call, gzc_str_from_parts(NULL, 0)) == GZC_ERR_INVALID_ARGUMENT,
      "empty workspace id is rejected");

  stub.status_code = 409;
  stub.response_body = "{\"error\":{\"code\":\"SYSTEM_WORKSPACE_DELETE_FORBIDDEN\",\"message\":\"system\"}}";
  check(
      gzc_control_delete_device_workspace(&client, &call, gzc_str_from_cstr("ws-pet")) != GZC_OK,
      "system workspace delete fails");
  check(call.error.kind == GZC_CONTROL_ERROR_CONFLICT, "system workspace delete is a conflict");
  check_str(call.error.code, "SYSTEM_WORKSPACE_DELETE_FORBIDDEN", "system workspace delete code");
}

static void test_friends(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  init_client(&client, &stub, &http);
  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  stub.status_code = 404;
  stub.response_body = "{\"error\":{\"code\":\"INVITE_TOKEN_NOT_FOUND\",\"message\":\"none\"}}";
  gzc_control_invite_token_t token;
  check(gzc_control_get_friend_invite_token(&client, &call, &token) != GZC_OK, "missing invite token fails");
  check(call.error.kind == GZC_CONTROL_ERROR_NOT_FOUND, "missing invite token is not found");
  check_str(call.error.code, "INVITE_TOKEN_NOT_FOUND", "missing invite token code");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friends/invite-token") == 0, "invite token url");

  stub.status_code = 200;
  stub.response_body = "{\"invite_token\":\"0123456789abcdef\",\"expires_at\":\"2026-09-19T01:02:03Z\"}";
  check(gzc_control_create_friend_invite_token(&client, &call, NULL, &token) == GZC_OK, "create invite token");
  check(stub.method == GZC_HTTP_METHOD_POST && strcmp(stub.body, "{}") == 0, "default invite token body");
  gzc_control_invite_token_request_t ttl = {.has_ttl_seconds = true, .ttl_seconds = 604800};
  check(gzc_control_create_friend_invite_token(&client, &call, &ttl, &token) == GZC_OK, "create ttl invite token");
  check(strcmp(stub.body, "{\"ttl_seconds\":604800}") == 0, "ttl invite token body");
  check_str(token.invite_token, "0123456789abcdef", "invite token value");
  check_str(token.expires_at, "2026-09-19T01:02:03Z", "invite token expiry");

  stub.status_code = 204;
  stub.response_body = "";
  check(gzc_control_clear_friend_invite_token(&client, &call) == GZC_OK, "clear invite token");
  check(stub.method == GZC_HTTP_METHOD_DELETE, "clear invite token method");

  const char *friend_json =
      "{\"name\":\"7hwy\",\"peer_public_key\":\"7hwy\",\"workspace_name\":\"social-direct-1\","
      "\"created_at\":\"2026-09-12T01:02:03Z\",\"updated_at\":\"2026-09-12T01:02:03Z\","
      "\"info\":{\"display_name\":\"Kitchen\",\"emoji\":\"x\"}}";
  stub.status_code = 201;
  stub.response_body = friend_json;
  gzc_control_friend_t friend_value;
  check(
      gzc_control_add_friend(&client, &call, gzc_str_from_cstr("0123"), &friend_value) == GZC_OK, "add friend");
  check(strcmp(stub.body, "{\"invite_token\":\"0123\"}") == 0, "add friend body");
  check_str(friend_value.workspace_name, "social-direct-1", "friend workspace");
  check(friend_value.has_info, "friend has info");
  check_str(friend_value.info.display_name, "Kitchen", "friend display name");
  check(
      gzc_control_add_friend(&client, &call, gzc_str_from_parts(NULL, 0), &friend_value) ==
          GZC_ERR_INVALID_ARGUMENT,
      "empty invite token is rejected");

  stub.status_code = 409;
  stub.response_body = "{\"error\":{\"code\":\"FRIEND_ALREADY_EXISTS\",\"message\":\"friends\"}}";
  check(
      gzc_control_add_friend(&client, &call, gzc_str_from_cstr("0123"), &friend_value) != GZC_OK,
      "duplicate friend fails");
  check(call.error.kind == GZC_CONTROL_ERROR_CONFLICT, "duplicate friend is a conflict");
  check_str(call.error.code, "FRIEND_ALREADY_EXISTS", "duplicate friend code");

  stub.status_code = 200;
  stub.response_body =
      "{\"items\":[{\"name\":\"7hwy\",\"peer_public_key\":\"7hwy\",\"workspace_name\":\"w\","
      "\"created_at\":\"t\",\"updated_at\":\"t\"}],\"has_next\":true,\"next_cursor\":\"c1\"}";
  gzc_control_page_t page = {.has_limit = true, .limit = 1};
  gzc_control_friend_t friends[2];
  size_t count = 0;
  bool has_next = false;
  gzc_str_t cursor;
  check(
      gzc_control_list_friends(&client, &call, &page, friends, 2, &count, &has_next, &cursor) == GZC_OK,
      "list friends");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friends?limit=1") == 0, "list friends url");
  check(count == 1 && has_next && !friends[0].has_info, "friend page without info");
  check_str(cursor, "c1", "friends cursor");

  stub.response_body = friend_json;
  check(gzc_control_get_friend(&client, &call, gzc_str_from_cstr("7hwy"), &friend_value) == GZC_OK, "get friend");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friends/7hwy") == 0, "get friend url");
  stub.response_body = "{\"name\":\"7hwy\"}";
  check(
      gzc_control_get_friend(&client, &call, gzc_str_from_cstr("7hwy"), &friend_value) != GZC_OK,
      "friend without required fields fails");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "friend without fields is malformed");

  stub.status_code = 204;
  stub.response_body = "";
  check(gzc_control_delete_friend(&client, &call, gzc_str_from_cstr("7hwy")) == GZC_OK, "delete friend");
  check(stub.method == GZC_HTTP_METHOD_DELETE, "delete friend method");
}

static void test_friend_groups(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  init_client(&client, &stub, &http);
  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");
  const char *group_json =
      "{\"name\":\"family room\",\"my_role\":\"owner\",\"display_name\":\"Family\","
      "\"workspace_name\":\"social-group-1\"}";
  const char *member_json =
      "{\"name\":\"7hwy\",\"peer_public_key\":\"7hwy\",\"role\":\"member\",\"info\":{\"display_name\":\"Kitchen\"}}";
  gzc_str_t name = gzc_str_from_cstr("family room");

  stub.status_code = 201;
  stub.response_body = group_json;
  gzc_control_friend_group_request_t create = {
      .name = gzc_str_from_cstr("family room"),
      .display_name = gzc_str_from_cstr("Family"),
  };
  gzc_control_friend_group_t group;
  check(gzc_control_create_friend_group(&client, &call, &create, &group) == GZC_OK, "create group");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups") == 0, "create group url");
  check(strcmp(stub.body, "{\"name\":\"family room\",\"display_name\":\"Family\"}") == 0, "create group body");
  check_str(group.my_role, "owner", "group role");

  stub.status_code = 200;
  check(gzc_control_put_friend_group(&client, &call, name, &create, &group) == GZC_OK, "put group");
  check(stub.method == GZC_HTTP_METHOD_PUT, "put group method");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups/family%20room") == 0, "put group url");
  check(strcmp(stub.body, "{\"display_name\":\"Family\"}") == 0, "put group body omits name");
  check(gzc_control_get_friend_group(&client, &call, name, &group) == GZC_OK, "get group");
  check_str(group.workspace_name, "social-group-1", "group workspace");

  char join_body[256];
  (void)snprintf(join_body, sizeof(join_body), "{\"group\":%s,\"member\":%s}", group_json, member_json);
  stub.response_body = join_body;
  gzc_control_friend_group_join_request_t join = {
      .invite_token = gzc_str_from_cstr("abc"),
      .name = gzc_str_from_cstr("family room"),
  };
  gzc_control_friend_group_member_t member;
  check(gzc_control_join_friend_group(&client, &call, &join, &group, &member) == GZC_OK, "join group");
  check(!member.has_online && member.last_seen_at.len == 0, "join omits member presence");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups/@join") == 0, "join url");
  check(strcmp(stub.body, "{\"invite_token\":\"abc\",\"name\":\"family room\"}") == 0, "join body");
  check_str(member.role, "member", "joined member role");
  check(member.has_info, "joined member info");
  check_str(member.info.display_name, "Kitchen", "joined member name");
  stub.response_body = "{\"group\":{\"name\":\"g\",\"my_role\":\"member\"}}";
  check(gzc_control_join_friend_group(&client, &call, &join, &group, &member) != GZC_OK, "join without member fails");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "join without member is malformed");

  stub.response_body = "{\"invite_token\":\"g1\",\"expires_at\":\"2026-09-12T02:00:00Z\"}";
  gzc_control_invite_token_t token;
  check(gzc_control_get_friend_group_invite_token(&client, &call, name, &token) == GZC_OK, "get group token");
  check(
      strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups/family%20room/invite-token") == 0,
      "group token url");
  gzc_control_invite_token_request_t ttl = {.has_ttl_seconds = true, .ttl_seconds = 3600};
  check(gzc_control_create_friend_group_invite_token(&client, &call, name, &ttl, &token) == GZC_OK, "create group token");
  check(stub.method == GZC_HTTP_METHOD_POST && strcmp(stub.body, "{\"ttl_seconds\":3600}") == 0, "group token body");

  stub.response_body = "{\"items\":["
                       "{\"name\":\"a\",\"peer_public_key\":\"a\",\"role\":\"owner\",\"online\":true,\"last_seen_at\":\"2026-09-12T00:30:00Z\"},"
                       "{\"name\":\"7hwy\",\"peer_public_key\":\"7hwy\",\"role\":\"member\",\"online\":false,\"info\":{\"display_name\":\"Kitchen\"}}"
                       "],\"has_next\":false}";
  gzc_control_friend_group_member_t members[2];
  size_t count = 0;
  bool has_next = true;
  gzc_str_t cursor;
  check(
      gzc_control_list_friend_group_members(&client, &call, name, NULL, members, 2, &count, &has_next, &cursor) ==
          GZC_OK,
      "list members");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups/family%20room/members") == 0, "members url");
  check(count == 2 && !has_next && cursor.len == 0, "members page");
  check(!members[0].has_info && members[1].has_info, "member info presence");
  check(members[0].has_online && members[0].online, "online member presence");
  check_str(members[0].last_seen_at, "2026-09-12T00:30:00Z", "member last seen");
  check(members[1].has_online && !members[1].online && members[1].last_seen_at.len == 0, "offline member presence");
  check(
      gzc_control_list_friend_group_members(&client, &call, name, NULL, members, 1, &count, &has_next, &cursor) ==
          GZC_ERR_BUFFER_TOO_SMALL,
      "small member array reports overflow");

  stub.status_code = 201;
  stub.response_body = member_json;
  gzc_control_friend_group_member_request_t add = {
      .peer_public_key = gzc_str_from_cstr("7hwy"),
      .member_name = gzc_str_from_cstr("kids"),
      .role = gzc_str_from_cstr("member"),
  };
  check(gzc_control_add_friend_group_member(&client, &call, name, &add, &member) == GZC_OK, "add member");
  check(!member.has_online && member.last_seen_at.len == 0, "add omits member presence");
  check(
      strcmp(stub.body, "{\"peer_public_key\":\"7hwy\",\"member_name\":\"kids\",\"role\":\"member\"}") == 0,
      "add member body");
  stub.status_code = 200;
  check(
      gzc_control_put_friend_group_member(
          &client, &call, name, gzc_str_from_cstr("7hwy"), gzc_str_from_cstr("admin"), &member) == GZC_OK,
      "put member");
  check(
      strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups/family%20room/members/7hwy") == 0,
      "put member url");
  check(strcmp(stub.body, "{\"role\":\"admin\"}") == 0, "put member body");
  check(!member.has_online && member.last_seen_at.len == 0, "put omits member presence");

  stub.status_code = 204;
  stub.response_body = "";
  check(
      gzc_control_delete_friend_group_member(&client, &call, name, gzc_str_from_cstr("7hwy")) == GZC_OK,
      "delete member");
  check(stub.method == GZC_HTTP_METHOD_DELETE, "delete member method");
  check(
      gzc_control_delete_friend_group_member(&client, &call, name, gzc_str_from_parts(NULL, 0)) ==
          GZC_ERR_INVALID_ARGUMENT,
      "empty member name is rejected");
  check(gzc_control_clear_friend_group_invite_token(&client, &call, name) == GZC_OK, "clear group token");
  check(
      strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups/family%20room/invite-token") == 0 &&
          stub.method == GZC_HTTP_METHOD_DELETE,
      "clear group token route");
  check(gzc_control_delete_friend_group(&client, &call, name) == GZC_OK, "delete group");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups/family%20room") == 0, "delete group url");

  check(gzc_control_leave_friend_group(&client, &call, name) == GZC_OK, "leave group");
  check(stub.method == GZC_HTTP_METHOD_POST && !stub.saw_content_type, "leave sends no body");
  check(
      strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/friend-groups/family%20room/@leave") == 0, "leave url");
  stub.status_code = 409;
  stub.response_body = "{\"error\":{\"code\":\"FRIEND_GROUP_OWNER_CANNOT_LEAVE\",\"message\":\"owner\"}}";
  check(gzc_control_leave_friend_group(&client, &call, name) != GZC_OK, "owner leave fails");
  check(call.error.kind == GZC_CONTROL_ERROR_CONFLICT, "owner leave is a conflict");
  check_str(call.error.code, "FRIEND_GROUP_OWNER_CANNOT_LEAVE", "owner leave code");
  stub.status_code = 403;
  stub.response_body = "{\"error\":{\"code\":\"FRIEND_GROUP_PERMISSION_DENIED\",\"message\":\"owner\"}}";
  check(gzc_control_delete_friend_group(&client, &call, name) != GZC_OK, "member dissolve fails");
  check(call.error.kind == GZC_CONTROL_ERROR_FORBIDDEN, "member dissolve is forbidden");
}

static void test_device_wifi_scan_and_connect(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body = "{\"result\":{\"networks\":[{\"ssid\":\"Office\",\"bssid\":\"aa:bb:cc:dd:ee:ff\",\"rssi_dbm\":-42,\"frequency_mhz\":5180,\"security\":\"wpa3\"},{\"ssid\":\"Open Network\"}]}}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_wifi_scan_request_t scan;
  memset(&scan, 0, sizeof(scan));
  scan.has_timeout_ms = true;
  scan.timeout_ms = 8000;
  gzc_control_wifi_scan_result_t networks[4];
  size_t count = 0;
  check(
      gzc_control_scan_device_wifi(&client, &call, &scan, networks, 4, &count) == GZC_OK,
      "scan device wifi");
  check(stub.method == GZC_HTTP_METHOD_POST, "scan method");
  check(
      strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/tool/v0/invoke") == 0,
      "scan url");
  check(strcmp(stub.body, "{\"tool\":\"wifi.scan\",\"args\":{\"timeout_ms\":8000}}") == 0, "scan body");
  check(count == 2, "scan network count");
  check(
      networks[0].has_rssi_dbm && networks[0].rssi_dbm == -42 &&
          networks[0].has_frequency_mhz && networks[0].frequency_mhz == 5180,
      "scan signal fields");
  check(
      !networks[1].has_rssi_dbm && networks[1].security.len == 0,
      "scan omits absent fields");

  /* rssi_dbm and frequency_mhz are int64 on the wire, so a value the Go, JS
   * and Flutter SDKs can represent must not fail here either. */
  stub.response_body = "{\"result\":{\"networks\":[{\"ssid\":\"Wide\",\"rssi_dbm\":-2147483649,\"frequency_mhz\":2147483648}]}}";
  check(
      gzc_control_scan_device_wifi(&client, &call, &scan, networks, 4, &count) == GZC_OK,
      "scan decodes out-of-int32 metrics");
  check(
      count == 1 && networks[0].rssi_dbm == INT64_C(-2147483649) &&
          networks[0].frequency_mhz == INT64_C(2147483648),
      "scan metrics keep int64 range");

  /* Omitting the request lets the Server apply its own scan timeout. */
  stub.response_body = "{\"result\":{\"networks\":[]}}";
  check(
      gzc_control_scan_device_wifi(&client, &call, NULL, networks, 4, &count) == GZC_OK,
      "scan without a request");
  check(strcmp(stub.body, "{\"tool\":\"wifi.scan\",\"args\":{}}") == 0, "default scan body");

  /* The join is accepted with 202; the device switches networks afterwards. */
  stub.status_code = 200;
  stub.response_body = "{\"result\":{}}";
  gzc_control_wifi_connect_request_t connect;
  memset(&connect, 0, sizeof(connect));
  connect.ssid = gzc_str_from_cstr("Office");
  connect.passphrase = gzc_str_from_cstr("correct-horse");
  check(
      gzc_control_connect_device_wifi(&client, &call, &connect) == GZC_OK,
      "connect device wifi");
  check(stub.method == GZC_HTTP_METHOD_POST, "connect method");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/tool/v0/invoke") == 0, "connect url");
  check(
      strcmp(stub.body, "{\"tool\":\"wifi.connect\",\"args\":{\"ssid\":\"Office\",\"passphrase\":\"correct-horse\"}}") == 0,
      "connect body");

  /* An open network carries no passphrase field at all. */
  connect.passphrase = gzc_str_from_parts(NULL, 0);
  check(
      gzc_control_connect_device_wifi(&client, &call, &connect) == GZC_OK,
      "connect open network");
  check(strcmp(stub.body, "{\"tool\":\"wifi.connect\",\"args\":{\"ssid\":\"Office\"}}") == 0, "open network body");

  /* A missing SSID is rejected before anything reaches the transport. */
  connect.ssid = gzc_str_from_parts(NULL, 0);
  check(
      gzc_control_connect_device_wifi(&client, &call, &connect) == GZC_ERR_INVALID_ARGUMENT,
      "connect requires an ssid");
  check(
      call.error.kind == GZC_CONTROL_ERROR_INVALID_REQUEST,
      "missing ssid is an invalid request");
}

static void test_device_info_raw_and_identifiers(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body =
      "{\"name\":\"claw-1\",\"emoji\":\"C\",\"hardware\":{\"manufacturer\":\"GizClaw\","
      "\"model\":\"H106\"},\"identifiers\":{\"sn\":\"SN1\",\"imeis\":[{\"tac\":\"12345678\","
      "\"serial\":\"901234\"}],\"labels\":[{\"key\":\"line\",\"value\":\"a\"}]},"
      "\"unmodeled\":{\"future\":true}}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_device_info_t device;
  check(gzc_control_get_device(&client, &call, &device) == GZC_OK, "get device");
  check_str(device.name, "claw-1", "device name");
  check(device.has_hardware, "device hardware present");
  check_str(device.hardware.model, "H106", "device model");
  check(device.hardware.hardware_revision.len == 0, "absent hardware field stays empty");
  check(device.has_identifiers, "device identifiers present");
  check_str(device.identifiers_sn, "SN1", "device sn");
  check(device.raw.len == strlen(stub.response_body), "device raw covers the response");

  gzc_control_peer_imei_t imeis[2];
  size_t imei_count = 0;
  check(gzc_control_device_info_imeis(&device, imeis, 2, &imei_count) == GZC_OK, "decode imeis");
  check(imei_count == 1, "imei count");
  check_str(imeis[0].tac, "12345678", "imei tac");

  gzc_control_pair_t labels[2];
  size_t label_count = 0;
  check(gzc_control_device_info_labels(&device, labels, 2, &label_count) == GZC_OK, "decode device labels");
  check(label_count == 1, "device label count");
  check_str(labels[0].key, "line", "device label key");
}

static void test_audioplayer(void) {
  stub_t stub = {0};
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  stub.status_code = 200;
  stub.response_body = "{\"result\":{\"state\":\"buffering\",\"current_index\":0,\"position_ms\":0,\"repeat\":\"all\",\"playlist_length\":1,\"playlist_revision\":2,\"observed_at_unix_ms\":1700000000000}}";
  init_client(&client, &stub, &http);
  uint8_t scratch[2048], response[2048];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "audio call init");
  gzc_control_audioplayer_status_t status;
  check(gzc_control_play_device_audioplayer(&client, &call, 0, &status) == GZC_OK, "audio play");
  check(strcmp(stub.body, "{\"tool\":\"audioplayer.play\",\"args\":{\"index\":0}}") == 0, "explicit zero index");
  check(status.has_current_index && status.current_index == 0 && status.position_ms == 0, "zero status fields");
  check_str(status.state, "buffering", "audio state");
  gzc_control_audioplayer_item_t item = {gzc_str_from_cstr("https://media.example/music.mp3"), gzc_str_from_cstr("a\"b"), {0}};
  check(gzc_control_set_device_audioplayer_playlist(&client, &call, &item, 1, &status) == GZC_OK, "audio set list");
  check(strcmp(stub.body, "{\"tool\":\"audioplayer.playlist.set\",\"args\":{\"items\":[{\"url\":\"https://media.example/music.mp3\",\"title\":\"a\\\"b\"}]}}") == 0, "playlist JSON escaping");
  check(gzc_control_append_device_audioplayer_playlist(&client, &call, &item, 1, &status) == GZC_OK, "audio append");
  check(stub.method == GZC_HTTP_METHOD_POST, "append POST");
  check(gzc_control_set_device_audioplayer_mode(&client, &call, gzc_str_from_cstr("all"), &status) == GZC_OK, "audio mode");
  check(gzc_control_stop_device_audioplayer(&client, &call, &status) == GZC_OK, "audio stop");
  stub.response_body = "{\"result\":{\"items\":[{\"url\":\"https://media.example/music.mp3\"}],\"playlist_revision\":2}}";
  size_t count;
  int64_t revision;
  check(gzc_control_get_device_audioplayer_playlist(&client, &call, &item, 1, &count, &revision) == GZC_OK, "audio get list");
  check(count == 1 && revision == 2, "audio list metadata");
  stub.response_body = "{\"result\":{}}";
  check(gzc_control_get_device_audioplayer(&client, &call, &status) == GZC_ERR_JSON, "reject malformed audio status");
}

/* Settings, factory reset, the capability list, the Workspace switch and the
 * control-app Tools: request shape on the wire and decode of the answers. */
static void test_mhs_settings_workspace_and_tools(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  init_client(&client, &stub, &http);

  uint8_t scratch[1024];
  uint8_t response[1024];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  stub.response_body = "{\"states\":[{\"device_id\":\"device\",\"state\":\"screen.brightness\",\"value\":40},{\"device_id\":\"device\",\"state\":\"alert.mode\",\"value\":\"ring\"},{\"device_id\":\"device\",\"state\":\"nfc.enabled\",\"value\":true}]}";
  gzc_control_mhs_v0_state_ref_t refs[3] = {
      {gzc_str_from_cstr("device"), gzc_str_from_cstr("screen.brightness")},
      {gzc_str_from_cstr("device"), gzc_str_from_cstr("alert.mode")},
      {gzc_str_from_cstr("device"), gzc_str_from_cstr("nfc.enabled")},
  };
  char strings[256];
  gzc_control_mhs_v0_storage_t storage = {strings, sizeof(strings), 0};
  gzc_control_mhs_v0_state_value_t states[3];
  size_t count = 0;
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 3, &storage, states, 3, &count) == GZC_OK, "read settings states");
  check(count == 3 && states[0].value.int_value == 40 && states[2].value.bool_value, "typed settings states");
  check_str(states[1].value.string_value, "ring", "settings enum");
  check(strstr(stub.body, "cellular.enabled") == NULL, "unrequested settings absent");
  gzc_control_mhs_v0_state_value_t patch[2] = {0};
  patch[0].device_id = patch[1].device_id = gzc_str_from_cstr("device");
  patch[0].state = gzc_str_from_cstr("alert.mode");
  patch[0].value.kind = GZC_CONTROL_MHS_V0_VALUE_STRING;
  patch[0].value.string_value = gzc_str_from_cstr("silent");
  patch[1].state = gzc_str_from_cstr("sleep.timeout-ms");
  patch[1].value.kind = GZC_CONTROL_MHS_V0_VALUE_INT;
  stub.response_body = "{\"states\":[{\"device_id\":\"device\",\"state\":\"alert.mode\",\"value\":\"silent\"},{\"device_id\":\"device\",\"state\":\"sleep.timeout-ms\",\"value\":0}]}";
  storage.used = 0;
  check(gzc_control_write_mhs_v0_states(&client, &call, patch, 2, &storage, states, 3, &count) == GZC_OK, "patch settings states");
  check(strcmp(stub.body, stub.response_body) == 0, "patch includes only selected settings and preserves zero");

  stub.status_code = 200;
  stub.response_body = "{\"result\":{}}";
  gzc_control_factory_reset_request_t reset;
  memset(&reset, 0, sizeof(reset));
  reset.has_keep_network = true;
  reset.keep_network = true;
  check(gzc_control_factory_reset_device(&client, &call, &reset) == GZC_OK, "factory reset");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/tool/v0/invoke") == 0, "factory reset path");
  check(strcmp(stub.body, "{\"tool\":\"device.factory_reset\",\"args\":{\"keep_network\":true}}") == 0, "factory reset body");

  stub.status_code = 200;
  stub.response_body = "{\"result\":{}}";
  gzc_control_run_workspace_request_t run;
  memset(&run, 0, sizeof(run));
  run.workflow_name = gzc_str_from_cstr("bedtime");
  run.has_kickoff = true;
  run.kickoff = true;
  check(gzc_control_set_device_run_workspace(&client, &call, &run) == GZC_OK, "run workspace");
  check(stub.method == GZC_HTTP_METHOD_POST, "run workspace method");
  check(strcmp(stub.body, "{\"tool\":\"run.workspace.set\",\"args\":{\"collection\":\"stories\",\"workflow_name\":\"bedtime\",\"kickoff\":true}}") == 0, "run workspace body");

  stub.status_code = 200;
  stub.response_body = "{\"tools\":[\"device.find\",\"device.reboot\"]}";
  gzc_str_t tools[2];
  check(gzc_control_list_device_tools(&client, &call, tools, 2, &count) == GZC_OK, "list tools");
  check(count == 2, "tool count");
  check_str(tools[0], "device.find", "tool name");
  check_str(tools[1], "device.reboot", "second tool name");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/tool/v0/tools") == 0, "list tool route");
  check(gzc_control_list_device_tools(&client, &call, tools, 1, &count) == GZC_ERR_BUFFER_TOO_SMALL, "bounded tool list");
}

/* MHS uses the same offline transport as the other control routes. */
static void test_mhs_v0(void) {
  stub_t stub = {0};
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  init_client(&client, &stub, &http);
  uint8_t scratch[64 * 1024];
  uint8_t response[16 * 1024];
  char strings[16 * 1024];
  gzc_control_mhs_v0_storage_t storage = {strings, sizeof(strings), 0};
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "MHS call init");
  stub.status_code = 200;
  stub.response_body =
      "{\"devices\":[{\"id\":\"display.main\",\"kind\":\"custom\",\"description\":\"line\\n二\",\"tags\":[\"室内\",\"a\\\"b\"],\"states\":["
      "{\"name\":\"enabled\",\"type\":\"bool\",\"access\":\"read_write\"},"
      "{\"name\":\"count\",\"type\":\"int\",\"access\":\"read\",\"min\":-9007199254740991,\"max\":9007199254740991,\"step\":1},"
      "{\"name\":\"temperature\",\"type\":\"double\",\"access\":\"read\",\"min\":0,\"step\":0.5,\"unit\":\"℃\",\"description\":\"sensor\"},"
      "{\"name\":\"label\",\"type\":\"string\",\"access\":\"read_write\"},"
      "{\"name\":\"mode\",\"type\":\"enum\",\"access\":\"read_write\",\"enum_values\":[\"\",\"自动\",\"a\\nb\"]}]}]}";
  gzc_control_mhs_v0_device_t devices[2];
  gzc_control_mhs_v0_state_t states[5];
  gzc_str_t texts[3];
  size_t count = 0;
  check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, devices, 2, &count) == GZC_OK && count == 1, "MHS manifest");
  check(stub.method == GZC_HTTP_METHOD_GET, "MHS manifest method");
  check_str(gzc_str_from_cstr(stub.url), "https://ap.gizclaw.com/gizclaw/v1/device/mhs/v0/manifest", "MHS manifest route");
  check_str(devices[0].id, "display.main", "MHS device id");
  check_str(devices[0].kind, "custom", "MHS open device kind");
  check_str(devices[0].description, "line\n二", "MHS description unescaped");
  check(gzc_control_mhs_v0_device_tags(&devices[0], &storage, texts, 3, &count) == GZC_OK && count == 2, "MHS tags");
  check_str(texts[1], "a\"b", "MHS tag unescaped");
  check(gzc_control_mhs_v0_device_states(&devices[0], &storage, states, 5, &count) == GZC_OK && count == 5, "MHS states");
  for (size_t i = 0; i < 5; i++)
    check(states[i].type == (gzc_control_mhs_v0_type_t)i, "MHS manifest type enums");
  check(states[0].access == GZC_CONTROL_MHS_V0_ACCESS_READ_WRITE && !states[0].has_min, "MHS access and absent constraint");
  check(states[1].has_min && states[1].min == -9007199254740991.0 && states[1].has_max && states[1].max == 9007199254740991.0 && states[1].has_step && states[1].step == 1, "MHS exact int constraints");
  check(states[2].has_min && states[2].min == 0 && !states[2].has_max && states[2].step == 0.5, "MHS double constraints");
  check_str(states[2].unit, "℃", "MHS unit");
  check_str(states[2].description, "sensor", "MHS state description");
  check(gzc_control_mhs_v0_state_enum_values(&states[4], &storage, texts, 3, &count) == GZC_OK && count == 3, "MHS enum values");
  check_str(texts[0], "", "MHS empty enum member");
  check_str(texts[2], "a\nb", "MHS enum escape");
  check(gzc_control_mhs_v0_device_states(&devices[0], &storage, states, 1, &count) == GZC_ERR_BUFFER_TOO_SMALL && count == 1, "MHS nested capacity");
  check(gzc_control_mhs_v0_device_tags(&devices[0], &storage, NULL, 0, &count) == GZC_ERR_BUFFER_TOO_SMALL, "MHS tag capacity");
  check(gzc_control_mhs_v0_state_enum_values(&states[4], &storage, texts, 1, &count) == GZC_ERR_BUFFER_TOO_SMALL && count == 1, "MHS enum capacity");
  check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, NULL, 0, &count) == GZC_ERR_BUFFER_TOO_SMALL && count == 0 && call.error.kind == GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL, "MHS manifest capacity classified");
  storage.cap = 0;
  check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, devices, 2, &count) == GZC_ERR_BUFFER_TOO_SMALL && call.error.kind == GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL, "MHS string capacity classified");
  storage.cap = sizeof(strings);
  storage.used = 0;
  stub.response_body = "{\"devices\":[]}";
  check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, NULL, 0, &count) == GZC_OK && count == 0, "MHS unconfigured manifest");

  gzc_control_mhs_v0_state_ref_t refs[32];
  for (size_t i = 0; i < 32; i++) {
    refs[i].device_id = gzc_str_from_cstr("display.main");
    refs[i].state = gzc_str_from_cstr("temperature");
  }
  gzc_control_mhs_v0_state_value_t values[32];
  stub.response_body = "{\"states\":[{\"device_id\":\"display.main\",\"state\":\"temperature\",\"value\":0}]}";
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_OK && count == 1, "MHS read");
  check(stub.method == GZC_HTTP_METHOD_POST && stub.saw_content_type && stub.saw_authorization, "MHS read transport");
  check_str(gzc_str_from_cstr(stub.url), "https://ap.gizclaw.com/gizclaw/v1/device/mhs/v0/read", "MHS read route");
  check_str(gzc_str_from_cstr(stub.body), "{\"states\":[{\"device_id\":\"display.main\",\"state\":\"temperature\"}]}", "MHS read encoded");
  check(values[0].value.kind == GZC_CONTROL_MHS_V0_VALUE_INT && values[0].value.has_int_value && values[0].value.int_value == 0 && values[0].value.double_value == 0, "MHS integral JSON supports double manifest");
  check_str(values[0].value.number_json, "0", "MHS numeric token retained");

  const struct {
    const char *json;
    bool exact;
    int64_t integer;
    gzc_control_mhs_v0_value_kind_t kind;
  } numbers[] = {
      {"9007199254740991", true, GZC_CONTROL_MHS_V0_MAX_INT, GZC_CONTROL_MHS_V0_VALUE_INT},
      {"-9007199254740991", true, -GZC_CONTROL_MHS_V0_MAX_INT, GZC_CONTROL_MHS_V0_VALUE_INT},
      {"1.0", true, 1, GZC_CONTROL_MHS_V0_VALUE_DOUBLE},
      {"10e-1", true, 1, GZC_CONTROL_MHS_V0_VALUE_DOUBLE},
      {"9.007199254740991e15", true, GZC_CONTROL_MHS_V0_MAX_INT, GZC_CONTROL_MHS_V0_VALUE_DOUBLE},
      {"9007199254740990.5", false, 0, GZC_CONTROL_MHS_V0_VALUE_DOUBLE},
      {"9007199254740992", false, 0, GZC_CONTROL_MHS_V0_VALUE_DOUBLE},
      {"1e-300", false, 0, GZC_CONTROL_MHS_V0_VALUE_DOUBLE},
      {"-0", true, 0, GZC_CONTROL_MHS_V0_VALUE_INT},
  };
  char body[4096];
  for (size_t i = 0; i < sizeof(numbers) / sizeof(numbers[0]); i++) {
    (void)snprintf(body, sizeof(body), "{\"states\":[{\"device_id\":\"display.main\",\"state\":\"temperature\",\"value\":%s}]}", numbers[i].json);
    stub.response_body = body;
    check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_OK, "MHS number decode");
    check(values[0].value.kind == numbers[i].kind && values[0].value.has_int_value == numbers[i].exact, "MHS number kind and exactness");
    if (numbers[i].exact)
      check(values[0].value.int_value == numbers[i].integer, "MHS exact integer view");
    check_str(values[0].value.number_json, numbers[i].json, "MHS preserves original number");
    check(values[0].value.number_is_integer_token == (strpbrk(numbers[i].json, ".eE") == NULL), "MHS raw number kind independent of safe integer range");
    if (numbers[i].kind == GZC_CONTROL_MHS_V0_VALUE_INT) {
      check(gzc_control_write_mhs_v0_states(&client, &call, values, 1, &storage, values, 32, &count) == GZC_OK, "MHS int round trip");
      if (strcmp(numbers[i].json, "-0") != 0)
        check_str(gzc_str_from_cstr(stub.body), body, "MHS int encoded exactly");
    }
  }
  stub.response_body = "{\"states\":[{\"device_id\":\"display.main\",\"state\":\"label\",\"value\":\"a\\n\\\"\\\\\\u4e8c\\ud83d\\ude00\"}]}";
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_OK, "MHS escaped UTF-8 read");
  check_str(values[0].value.string_value, "a\n\"\\二😀", "MHS string decoded");
  check_str(call.body, stub.response_body, "MHS raw response intact");
  check(gzc_control_write_mhs_v0_states(&client, &call, values, 1, &storage, values, 32, &count) == GZC_OK, "MHS escaped string round trip");
  check(stub.method == GZC_HTTP_METHOD_PATCH, "MHS write method");
  check_str(gzc_str_from_cstr(stub.url), "https://ap.gizclaw.com/gizclaw/v1/device/mhs/v0/states", "MHS write route");
  check_str(gzc_str_from_cstr(stub.body), "{\"states\":[{\"device_id\":\"display.main\",\"state\":\"label\",\"value\":\"a\\n\\\"\\\\二😀\"}]}", "MHS string reencoded");

  gzc_control_mhs_v0_state_value_t request = {0};
  request.device_id = refs[0].device_id;
  request.state = refs[0].state;
  request.value.kind = GZC_CONTROL_MHS_V0_VALUE_BOOL;
  request.value.bool_value = true;
  stub.response_body = "{\"states\":[{\"device_id\":\"display.main\",\"state\":\"temperature\",\"value\":false}]}";
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_OK && !values[0].value.bool_value, "MHS returns applied value rather than request");
  check(strstr(stub.body, "\"value\":true") != NULL, "MHS bool encoded");
  request.value.kind = GZC_CONTROL_MHS_V0_VALUE_DOUBLE;
  request.value.double_value = 0.5;
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_OK && strstr(stub.body, "\"value\":0.5") != NULL, "MHS double encoded");
  request.value.double_value = 0;
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_OK && strstr(stub.body, "\"value\":0") != NULL, "MHS integral double encoded");

  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 32, &storage, values, 32, &count) == GZC_OK, "MHS 32-state batch");
  int calls = stub.free_calls;
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 0, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS empty batch rejected");
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 33, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT && call.error.kind == GZC_CONTROL_ERROR_INVALID_REQUEST, "MHS oversized batch rejected");
  char name[65];
  memset(name, 'a', sizeof(name));
  refs[0].state = gzc_str_from_parts(name, 65);
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS name 65 bytes rejected");
  const char *bad_names[] = {"", "Upper", "a..b", "a-", "é", "a_b"};
  for (size_t i = 0; i < sizeof(bad_names) / sizeof(bad_names[0]); i++) {
    refs[0].state = gzc_str_from_cstr(bad_names[i]);
    check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS invalid ASCII name rejected");
  }
  request.value.kind = GZC_CONTROL_MHS_V0_VALUE_INT;
  request.value.int_value = GZC_CONTROL_MHS_V0_MAX_INT + 1;
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS unsafe integer rejected");
  request.value.int_value = -GZC_CONTROL_MHS_V0_MAX_INT - 1;
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS unsafe negative integer rejected");
  request.value.kind = GZC_CONTROL_MHS_V0_VALUE_DOUBLE;
  request.value.double_value = NAN;
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS NaN rejected");
  request.value.double_value = INFINITY;
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS infinity rejected");
  request.value.kind = (gzc_control_mhs_v0_value_kind_t)99;
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS unknown value kind rejected");
  char long_string[257];
  memset(long_string, 'x', sizeof(long_string));
  request.value.kind = GZC_CONTROL_MHS_V0_VALUE_STRING;
  request.value.string_value = gzc_str_from_parts(long_string, sizeof(long_string));
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS 257-byte string rejected");
  request.value.string_value = gzc_str_from_parts("a\0b", 3);
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS NUL rejected");
  const char invalid_utf8[] = {(char)0xC0, (char)0x80};
  request.value.string_value = gzc_str_from_parts(invalid_utf8, sizeof(invalid_utf8));
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS invalid UTF-8 rejected");
  check(stub.free_calls == calls, "MHS invalid input never sent");
  refs[0].state = gzc_str_from_parts(name, 64);
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_OK, "MHS 64-byte name accepted");
  request.value.string_value = gzc_str_from_parts(long_string, 256);
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_OK, "MHS 256-byte string accepted");
  request.value.string_value = gzc_str_from_parts(NULL, 0);
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_OK && strstr(stub.body, "\"value\":\"\"") != NULL, "MHS empty string encoded");

  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, NULL, 0, &count) == GZC_ERR_BUFFER_TOO_SMALL && call.error.kind == GZC_CONTROL_ERROR_OUTPUT_TOO_SMALL, "MHS response capacity classified");
  const char *bad_values[] = {"null", "{}", "[]", "1e999", "01", "\"\\u0000\"", "\"\\ud800\"", "\"\\udc00\"", "\"\\q\""};
  for (size_t i = 0; i < sizeof(bad_values) / sizeof(bad_values[0]); i++) {
    (void)snprintf(body, sizeof(body), "{\"states\":[{\"device_id\":\"display.main\",\"state\":\"temperature\",\"value\":%s}]}", bad_values[i]);
    stub.response_body = body;
    check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_ERR_JSON && call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "MHS malformed value classified");
  }
  const char *bad_bodies[] = {"{}", "{\"states\":null}", "{\"states\":[]}", "{\"states\":[{}]}", "{\"states\":[{\"device_id\":\"UPPER\",\"state\":\"x\",\"value\":1}]}"};
  for (size_t i = 0; i < sizeof(bad_bodies) / sizeof(bad_bodies[0]); i++) {
    stub.response_body = bad_bodies[i];
    check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_ERR_JSON, "MHS malformed state list rejected");
  }
  const char *bad_manifests[] = {"{}", "{\"devices\":null}", "{\"devices\":[{}]}", "{\"devices\":[{\"id\":\"x\",\"kind\":\"x\",\"states\":[]}]}"};
  for (size_t i = 0; i < sizeof(bad_manifests) / sizeof(bad_manifests[0]); i++) {
    stub.response_body = bad_manifests[i];
    check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, devices, 2, &count) == GZC_ERR_JSON, "MHS malformed manifest rejected");
  }
  const char *bad_states[] = {
      "[{\"name\":\"a\",\"type\":\"unknown\",\"access\":\"read\"}]",
      "[{\"name\":\"a\",\"type\":\"bool\",\"access\":\"write\"}]",
      "[{\"name\":\"a\",\"type\":\"double\",\"access\":\"read\",\"step\":0}]",
      "[{\"name\":\"a\",\"type\":\"int\",\"access\":\"read\",\"min\":9007199254740990.5}]",
      "[{\"name\":\"a\",\"type\":\"enum\",\"access\":\"read\"}]",
  };
  for (size_t i = 0; i < sizeof(bad_states) / sizeof(bad_states[0]); i++) {
    devices[0].states = gzc_str_from_cstr(bad_states[i]);
    check(gzc_control_mhs_v0_device_states(&devices[0], &storage, states, 5, &count) == GZC_ERR_JSON, "MHS malformed manifest state rejected");
  }
  /* Capacity is measured in decoded UTF-8 bytes, not escape-token bytes. */
  devices[0].tags = gzc_str_from_cstr("[\"\\u4e8c\"]");
  storage.used = 0;
  storage.cap = 3;
  check(gzc_control_mhs_v0_device_tags(&devices[0], &storage, texts, 3, &count) == GZC_OK && storage.used == 3, "MHS exact decoded string capacity");
  check_str(texts[0], "二", "MHS unicode in exact capacity");
  storage.used = 0;
  storage.cap = 2;
  check(gzc_control_mhs_v0_device_tags(&devices[0], &storage, texts, 3, &count) == GZC_ERR_BUFFER_TOO_SMALL && count == 0, "MHS UTF-8 capacity cannot truncate codepoint");
  storage.cap = sizeof(strings);
  storage.used = 0;
  char utf8_limit[260];
  for (size_t i = 0; i < sizeof(utf8_limit); i += 4)
    memcpy(utf8_limit + i, "😀", 4);
  request.value.string_value = gzc_str_from_parts(utf8_limit, 256);
  stub.response_body = "{\"states\":[{\"device_id\":\"display.main\",\"state\":\"label\",\"value\":\"\"}]}";
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_OK, "MHS UTF-8 byte limit accepts 64 emoji");
  request.value.string_value.len = 260;
  check(gzc_control_write_mhs_v0_states(&client, &call, &request, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS UTF-8 byte limit rejects 65 emoji");
  (void)snprintf(body, sizeof(body), "{\"states\":[{\"device_id\":\"x\",\"state\":\"label\",\"value\":\"%.*s\"}]}", 260, utf8_limit);
  stub.response_body = body;
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_ERR_JSON, "MHS response UTF-8 byte limit");
  size_t body_len = (size_t)snprintf(body, sizeof(body), "{\"states\":[");
  for (size_t i = 0; i < 33; i++) {
    body_len += (size_t)snprintf(body + body_len, sizeof(body) - body_len, "%s{\"device_id\":\"x\",\"state\":\"v%zu\",\"value\":true}", i == 0 ? "" : ",", i);
  }
  (void)snprintf(body + body_len, sizeof(body) - body_len, "]}");
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_ERR_JSON && call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "MHS response 33-state batch rejected before capacity");

  check(gzc_control_get_mhs_v0_manifest(NULL, &call, &storage, devices, 2, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS null client");
  check(gzc_control_get_mhs_v0_manifest(&client, NULL, &storage, devices, 2, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS null call");
  check(gzc_control_get_mhs_v0_manifest(&client, &call, NULL, devices, 2, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS null storage");
  check(gzc_control_read_mhs_v0_states(&client, &call, NULL, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS null read request");
  check(gzc_control_write_mhs_v0_states(&client, &call, NULL, 1, &storage, values, 32, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS null write request");
  check(gzc_control_mhs_v0_device_states(NULL, &storage, states, 5, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS null nested input");
  check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, NULL, 1, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS null nonempty output");
  check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, devices, 2, NULL) == GZC_ERR_INVALID_ARGUMENT, "MHS null count");
  storage.used = storage.cap + 1;
  check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, devices, 2, &count) == GZC_ERR_INVALID_ARGUMENT, "MHS invalid storage bounds");
  storage.used = 0;

  const struct {
    int status;
    const char *code;
    gzc_control_error_kind_t kind;
  } errors[] = {
      {400, "INVALID_REQUEST", GZC_CONTROL_ERROR_INVALID_REQUEST},
      {400, "DEVICE_REJECTED", GZC_CONTROL_ERROR_DEVICE_REJECTED},
      {404, "MHS_STATE_NOT_FOUND", GZC_CONTROL_ERROR_NOT_FOUND},
      {409, "DEVICE_OFFLINE", GZC_CONTROL_ERROR_DEVICE_OFFLINE},
      {501, "DEVICE_UNSUPPORTED", GZC_CONTROL_ERROR_DEVICE_UNSUPPORTED},
      {504, "DEVICE_TIMEOUT", GZC_CONTROL_ERROR_DEVICE_TIMEOUT},
      {502, "DEVICE_ERROR", GZC_CONTROL_ERROR_DEVICE_ERROR},
  };
  for (size_t i = 0; i < sizeof(errors) / sizeof(errors[0]); i++) {
    stub.status_code = errors[i].status;
    (void)snprintf(body, sizeof(body), "{\"error\":{\"code\":\"%s\",\"message\":\"test\"}}", errors[i].code);
    stub.response_body = body;
    check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_ERR_HTTP && call.error.kind == errors[i].kind && call.status_code == errors[i].status, "MHS HTTP error classification");
    check_str(call.error.code, errors[i].code, "MHS error code preserved");
  }
  stub.result = GZC_ERR_TIMEOUT;
  check(gzc_control_get_mhs_v0_manifest(&client, &call, &storage, devices, 2, &count) == GZC_ERR_TIMEOUT && call.error.kind == GZC_CONTROL_ERROR_NETWORK, "MHS transport failure");
  stub.result = GZC_OK;
  call.scratch_cap = 8;
  check(gzc_control_read_mhs_v0_states(&client, &call, refs, 1, &storage, values, 32, &count) == GZC_ERR_NO_MEMORY && call.error.kind == GZC_CONTROL_ERROR_NETWORK, "MHS scratch exhaustion");
}

static void test_additional_typed_tools(void) {
  stub_t stub = {0};
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  init_client(&client, &stub, &http);
  uint8_t scratch[2048], response[2048];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "typed tools init");
  stub.status_code = 200;
  stub.response_body = "{\"result\":{\"model\":\"speaker\",\"manufacturer\":\"maker\",\"hardware_revision\":\"2\"}}";
  gzc_control_hardware_info_t hardware;
  check(gzc_control_get_device_hardware(&client, &call, &hardware) == GZC_OK, "hardware tool");
  check_str(hardware.model, "speaker", "hardware model");
  check(strcmp(stub.body, "{\"tool\":\"info.get\",\"args\":{}}") == 0, "info empty args");
  stub.response_body = "{\"result\":{\"sn\":\"sn-1\",\"imeis\":[],\"labels\":[]}}";
  gzc_control_device_identifiers_t identifiers;
  check(gzc_control_get_device_identifiers(&client, &call, &identifiers) == GZC_OK, "identifiers tool");
  check_str(identifiers.sn, "sn-1", "device serial");
  stub.response_body = "{\"result\":{\"volume\":20}}";
  gzc_control_peer_status_t status;
  check(gzc_control_read_device_status(&client, &call, &status) == GZC_OK && status.has_volume && status.volume == 20, "live status tool");
  stub.response_body = "{\"result\":{}}";
  gzc_control_firmware_update_request_t firmware = {gzc_str_from_cstr("stable"), gzc_str_from_cstr("abcdef")};
  check(gzc_control_update_device_firmware(&client, &call, &firmware) == GZC_OK, "firmware tool");
  check(strcmp(stub.body, "{\"tool\":\"firmware.update\",\"args\":{\"channel\":\"stable\",\"sha256\":\"abcdef\"}}") == 0, "firmware typed args");
  check(gzc_control_update_device_firmware(&client, &call, NULL) == GZC_OK, "firmware optional args");
  gzc_control_social_ping_request_t ping = {gzc_str_from_cstr("key"), gzc_str_from_cstr("Alice"), {0}};
  check(gzc_control_ping_device(&client, &call, &ping) == GZC_OK, "social ping tool");
  check(strcmp(stub.body, "{\"tool\":\"social.ping\",\"args\":{\"from_peer_public_key\":\"key\",\"from_display_name\":\"Alice\"}}") == 0, "social ping typed args");
  check(gzc_control_ping_device(&client, &call, NULL) == GZC_ERR_INVALID_ARGUMENT, "typed request required");
  check(stub.method == GZC_HTTP_METHOD_POST && strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/tool/v0/invoke") == 0, "single invoke transport");
  stub.response_body = "{\"result\":null}";
  check(gzc_control_get_device_hardware(&client, &call, &hardware) == GZC_ERR_JSON, "missing typed tool result rejected");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "malformed result classified");
}

int main(void) {
  test_mhs_v0();
  test_additional_typed_tools();
  test_audioplayer();
  test_client_init_rejects_bad_config();
  test_get_device_status();
  test_peer_status_labels_span_escapes();
  test_mhs_volume_encodes_body();
  test_query_parameters_and_encoding();
  test_contract_caps_are_not_enforced_locally();
  test_find_device();
  test_response_header_validation();
  test_error_classification();
  test_error_response_is_decoded();
  test_transport_failure_is_network();
  test_scratch_exhaustion_is_reported();
  test_lists_and_malformed_bodies();
  test_device_runtime_profile();
  test_device_workspaces();
  test_device_wifi_scan_and_connect();
  test_device_info_raw_and_identifiers();
  test_friends();
  test_friend_groups();
  test_mhs_settings_workspace_and_tools();
  if (failures != 0) {
    (void)fprintf(stderr, "%d control SDK smoke checks failed\n", failures);
    return 1;
  }
  (void)printf("gizclaw_control smoke test passed\n");
  return 0;
}
