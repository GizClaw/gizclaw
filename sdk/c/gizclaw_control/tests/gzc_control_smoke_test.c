/*
 * Offline smoke test for the controller-side C SDK.
 *
 * A stub gzc_http_vtable_t records the request the SDK built and replays a
 * canned response, so the test covers URL construction, request encoding,
 * response decoding, the error classification table, and the caller-owned
 * buffer contract without a network.
 */
#include "gzc_control.h"

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
  char body[512];
  gzc_http_method_t method;
  bool saw_authorization;
  bool saw_content_type;
  int status_code;
  const char *response_body;
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

static void test_set_volume_encodes_body(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 200;
  stub.response_body = "{\"status\":{\"volume\":35,\"muted\":false}}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[512];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_volume_request_t request;
  request.level = 35;
  request.muted = false;
  gzc_control_peer_status_t status;
  check(gzc_control_set_device_volume(&client, &call, &request, &status) == GZC_OK, "set volume");
  check(stub.method == GZC_HTTP_METHOD_PUT, "volume method");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/volume") == 0, "volume url");
  check(strcmp(stub.body, "{\"level\":35,\"muted\":false}") == 0, "volume body");
  check(stub.saw_content_type, "body request sends Content-Type");
  check(status.has_volume && status.volume == 35, "applied volume");

  request.level = 101;
  check(
      gzc_control_set_device_volume(&client, &call, &request, &status) == GZC_ERR_INVALID_ARGUMENT,
      "out-of-range volume rejected");
  check(call.error.kind == GZC_CONTROL_ERROR_INVALID_REQUEST, "out-of-range volume classified");
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

  /* A path segment with reserved characters is percent-encoded. */
  stub.status_code = 204;
  stub.response_body = "";
  check(
      gzc_control_forget_device_saved_wifi(&client, &call, gzc_str_from_cstr("home wifi/2G")) == GZC_OK,
      "forget saved wifi");
  check(
      strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/device/wifi/saved/home%20wifi%2F2G") == 0,
      "encoded ssid segment");
  check(stub.method == GZC_HTTP_METHOD_DELETE, "forget method");
}

static void test_contract_caps(void) {
  stub_t stub;
  gzc_http_vtable_t http;
  gzc_control_client_t client;
  memset(&stub, 0, sizeof(stub));
  stub.status_code = 204;
  stub.response_body = "";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[256];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  char oversized[GZC_CONTROL_MAX_SOUND_BYTES + 2];
  memset(oversized, 'a', sizeof(oversized) - 1);
  oversized[sizeof(oversized) - 1] = 0;
  gzc_control_play_sound_request_t sound;
  memset(&sound, 0, sizeof(sound));
  sound.sound = gzc_str_from_cstr(oversized);
  check(
      gzc_control_play_device_sound(&client, &call, &sound) == GZC_ERR_INVALID_ARGUMENT,
      "oversized sound rejected");

  char long_ssid[GZC_CONTROL_MAX_SSID_BYTES + 2];
  memset(long_ssid, 'b', sizeof(long_ssid) - 1);
  long_ssid[sizeof(long_ssid) - 1] = 0;
  check(
      gzc_control_forget_device_saved_wifi(&client, &call, gzc_str_from_cstr(long_ssid)) ==
          GZC_ERR_INVALID_ARGUMENT,
      "oversized ssid rejected");

  sound.sound = gzc_str_from_cstr("chime");
  sound.has_duration_ms = true;
  sound.duration_ms = 1200;
  check(gzc_control_play_device_sound(&client, &call, &sound) == GZC_OK, "play sound");
  check(strcmp(stub.body, "{\"sound\":\"chime\",\"duration_ms\":1200}") == 0, "play sound body");
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
  stub.response_body =
      "{\"error\":{\"code\":\"DEVICE_OFFLINE\",\"message\":\"device is not connected\","
      "\"details\":{\"peer\":\"pk\"}}}";
  init_client(&client, &stub, &http);

  uint8_t scratch[512];
  uint8_t response[512];
  gzc_control_call_t call;
  check(gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response)) == GZC_OK, "call init");

  gzc_control_wifi_status_t status;
  check(gzc_control_get_device_wifi(&client, &call, &status) == GZC_ERR_HTTP, "offline call fails");
  check(call.error.kind == GZC_CONTROL_ERROR_DEVICE_OFFLINE, "offline kind");
  check(call.error.status_code == 409, "offline status");
  check_str(call.error.code, "DEVICE_OFFLINE", "offline code");
  check_str(call.error.message, "device is not connected", "offline message");
  check_str(call.error.details, "{\"peer\":\"pk\"}", "offline details");

  /* A non-2xx body that is not an ErrorResponse still classifies by status. */
  stub.status_code = 502;
  stub.response_body = "upstream failure";
  check(gzc_control_get_device_wifi(&client, &call, &status) == GZC_ERR_HTTP, "bad gateway fails");
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
  page.limit = 2;
  gzc_control_contact_t contacts[4];
  size_t count = 0;
  bool has_next = false;
  gzc_str_t cursor;
  check(
      gzc_control_list_contacts(&client, &call, &page, contacts, 4, &count, &has_next, &cursor) == GZC_OK,
      "list contacts");
  check(strcmp(stub.url, "https://ap.gizclaw.com/gizclaw/v1/contacts?limit=2") == 0, "contacts url");
  check(count == 2 && has_next, "contacts page");
  check_str(contacts[0].display_name, "Ann", "contact display name");
  check(contacts[1].display_name.len == 0, "absent display name stays empty");
  check_str(cursor, "cur", "contacts cursor");

  /* A caller array smaller than the page reports the overflow. */
  check(
      gzc_control_list_contacts(&client, &call, &page, contacts, 1, &count, &has_next, &cursor) ==
          GZC_ERR_BUFFER_TOO_SMALL,
      "small contact array reports overflow");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "overflow classified");

  stub.response_body = "not json";
  check(
      gzc_control_list_contacts(&client, &call, &page, contacts, 4, &count, &has_next, &cursor) != GZC_OK,
      "malformed body fails");
  check(call.error.kind == GZC_CONTROL_ERROR_MALFORMED_RESPONSE, "malformed body classified");
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

int main(void) {
  test_client_init_rejects_bad_config();
  test_get_device_status();
  test_set_volume_encodes_body();
  test_query_parameters_and_encoding();
  test_contract_caps();
  test_error_classification();
  test_error_response_is_decoded();
  test_transport_failure_is_network();
  test_scratch_exhaustion_is_reported();
  test_lists_and_malformed_bodies();
  test_device_info_raw_and_identifiers();
  if (failures != 0) {
    (void)fprintf(stderr, "%d control SDK smoke checks failed\n", failures);
    return 1;
  }
  (void)printf("gizclaw_control smoke test passed\n");
  return 0;
}
