#include "bridge.h"

#include "../../../../sdk/c/gizclaw/cgobackend/gzc_cgo_backend.h"
#include "gzc.h"
#include "gzc_control.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* Implemented in Go by provider.go. */
extern int gztGoProvider(
    unsigned long long handle,
    int method,
    void *request_payload,
    size_t request_payload_len,
    void **out_payload,
    size_t *out_payload_len,
    int *out_error_code,
    char *out_error_message,
    size_t out_error_message_cap);

struct gzt_session {
  gzc_cgo_backend_t backend;
  gzc_http_vtable_t http;
  gzc_platform_crypto_t crypto;
  gzc_webrtc_vtable_t webrtc;
  gzc_webrtc_media_vtable_t media;
  gzc_client_t *client;
  unsigned long long provider_handle;
};

static int fail(char *errbuf, unsigned long errbuf_len, const char *message, int rc) {
  if (errbuf != NULL && errbuf_len > 0) {
    (void)snprintf(errbuf, errbuf_len, "%s: %s (%d)", message, gzc_status_string((gzc_status_t)rc), rc);
  }
  return rc == GZC_OK ? GZC_ERR_RPC : rc;
}

static void set_error(char *errbuf, unsigned long errbuf_len, const char *message) {
  if (errbuf != NULL && errbuf_len > 0) {
    (void)snprintf(errbuf, errbuf_len, "%s", message);
  }
}

/* Bound on one bridge call when the caller supplies no deadline. */
#define GZT_DEFAULT_TIMEOUT_MS 30000

void gzt_free(void *ptr) { free(ptr); }

/*
 * Answers one server-initiated client.* method by handing the encoded request
 * to Go, which decodes it, applies the document's scripted response, and
 * returns encoded response bytes or a structured RPC error.
 */
static int provider(
    void *userdata,
    int method,
    gzc_str_t request_payload,
    gzc_rpc_provider_respond_fn respond,
    void *respond_userdata) {
  gzt_session_t *session = (gzt_session_t *)userdata;
  void *payload = NULL;
  size_t payload_len = 0;
  int error_code = 0;
  char error_message[256];
  error_message[0] = 0;
  int rc = gztGoProvider(
      session->provider_handle, method, (void *)request_payload.data, request_payload.len,
      &payload, &payload_len, &error_code, error_message, sizeof(error_message));
  if (rc != GZC_OK) {
    free(payload);
    return rc;
  }
  gzc_rpc_provider_response_t response;
  memset(&response, 0, sizeof(response));
  if (error_code != 0) {
    response.has_error = true;
    response.error_code = error_code;
    response.error_message = gzc_str_from_cstr(error_message);
  } else {
    response.payload = (const uint8_t *)payload;
    response.payload_len = payload_len;
  }
  rc = respond(respond_userdata, &response);
  free(payload);
  return rc;
}

int gzt_session_open(
    const char *endpoint,
    const char *private_key,
    unsigned long long provider_handle,
    gzt_session_t **out_session,
    char *errbuf,
    unsigned long errbuf_len) {
  if (endpoint == NULL || private_key == NULL || out_session == NULL) {
    return fail(errbuf, errbuf_len, "session open", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_session = NULL;
  gzt_session_t *session = (gzt_session_t *)calloc(1, sizeof(*session));
  if (session == NULL) {
    return fail(errbuf, errbuf_len, "session alloc", GZC_ERR_NO_MEMORY);
  }
  session->provider_handle = provider_handle;

  int rc = gzc_cgo_backend_init(&session->backend);
  if (rc != GZC_OK) {
    free(session);
    return fail(errbuf, errbuf_len, "backend init", rc);
  }
  gzc_cgo_backend_http_vtable(&session->backend, &session->http);
  gzc_cgo_backend_crypto_vtable(&session->backend, &session->crypto);
  gzc_cgo_backend_webrtc_vtable(&session->backend, &session->webrtc);
  gzc_cgo_backend_webrtc_media_vtable(&session->backend, &session->media);

  gzc_client_config_t config;
  memset(&config, 0, sizeof(config));
  config.server_url = gzc_str_from_cstr(endpoint);
  config.private_key = gzc_str_from_cstr(private_key);
  config.platform = session->backend.platform;
  config.crypto = &session->crypto;
  config.http = &session->http;
  config.webrtc = &session->webrtc;
  config.cipher_mode = GZC_CIPHER_CHACHA20_POLY1305;
  config.connect_timeout_ms = 15000;
  config.write_timeout_ms = 15000;
  if (provider_handle != 0) {
    config.rpc_provider = provider;
    config.rpc_provider_userdata = session;
  }

  rc = gzc_client_create(&config, &session->client);
  if (rc == GZC_OK) {
    rc = gzc_client_set_webrtc_media(session->client, &session->media);
  }
  if (rc == GZC_OK) {
    rc = gzc_client_set_peer_add_ice_server(session->client, gzc_cgo_backend_peer_add_ice_server);
  }
  if (rc == GZC_OK) {
    rc = gzc_client_connect(session->client);
  }
  if (rc != GZC_OK) {
    if (session->client != NULL) {
      gzc_client_destroy(session->client);
    }
    gzc_cgo_backend_deinit(&session->backend);
    free(session);
    return fail(errbuf, errbuf_len, "client connect", rc);
  }
  *out_session = session;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzt_session_close(gzt_session_t *session) {
  if (session == NULL) {
    return;
  }
  if (session->client != NULL) {
    gzc_client_destroy(session->client);
  }
  gzc_cgo_backend_deinit(&session->backend);
  free(session);
}

int gzt_session_poll(gzt_session_t *session, int timeout_ms, char *errbuf, unsigned long errbuf_len) {
  if (session == NULL) {
    return fail(errbuf, errbuf_len, "poll", GZC_ERR_INVALID_ARGUMENT);
  }
  int rc = gzc_client_poll(session->client, timeout_ms);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "poll", rc);
  }
  return GZC_OK;
}

/* Drives poll until the request settles, so the caller stays the poll owner. */
static int wait_rpc_result(gzt_session_t *session, gzc_rpc_request_t *request, gzc_rpc_response_t *out_response) {
  int rc;
  while ((rc = gzc_rpc_request_result(request, out_response)) == GZC_ERR_WOULD_BLOCK) {
    rc = gzc_client_poll(session->client, 10);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return rc;
}

static void copy_error_message(gzc_str_t message, char *out, unsigned long out_len) {
  if (out == NULL || out_len == 0) {
    return;
  }
  size_t count = message.len;
  if (count >= out_len) {
    count = out_len - 1;
  }
  if (count > 0 && message.data != NULL) {
    memcpy(out, message.data, count);
  }
  out[count] = 0;
}

int gzt_session_call_rpc(
    gzt_session_t *session,
    unsigned method_id,
    const unsigned char *payload,
    unsigned long payload_len,
    int timeout_ms,
    unsigned char **out_payload,
    unsigned long *out_payload_len,
    int *out_rpc_error_code,
    char *out_error_message,
    unsigned long out_error_message_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || method_id == 0 || out_payload == NULL || out_payload_len == NULL ||
      out_rpc_error_code == NULL) {
    return fail(errbuf, errbuf_len, "call rpc", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_payload = NULL;
  *out_payload_len = 0;
  *out_rpc_error_code = 0;

  gzc_rpc_request_t *request = NULL;
  int rc = gzc_rpc_request_start(
      session->client, 0, (gizclaw_rpc_v1_RpcMethod)method_id,
      gzc_str_from_parts((const char *)payload, payload_len),
      timeout_ms > 0 ? timeout_ms : GZT_DEFAULT_TIMEOUT_MS, &request);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "start rpc", rc);
  }
  gzc_rpc_response_t response;
  memset(&response, 0, sizeof(response));
  rc = wait_rpc_result(session, request, &response);
  if (rc != GZC_OK) {
    gzc_rpc_request_destroy(request);
    return fail(errbuf, errbuf_len, "await rpc", rc);
  }
  if (response.has_error) {
    *out_rpc_error_code = response.error.code;
    copy_error_message(response.error.message, out_error_message, out_error_message_len);
    gzc_rpc_request_destroy(request);
    return GZC_OK;
  }
  if (response.result_payload.len > 0) {
    unsigned char *copy = (unsigned char *)malloc(response.result_payload.len);
    if (copy == NULL) {
      gzc_rpc_request_destroy(request);
      return fail(errbuf, errbuf_len, "copy rpc result", GZC_ERR_NO_MEMORY);
    }
    memcpy(copy, response.result_payload.data, response.result_payload.len);
    *out_payload = copy;
    *out_payload_len = (unsigned long)response.result_payload.len;
  }
  gzc_rpc_request_destroy(request);
  return GZC_OK;
}

/* --- Controller SDK dispatch -------------------------------------------- */

struct gzt_control {
  gzc_cgo_backend_t backend;
  gzc_http_vtable_t http;
};

int gzt_control_open(gzt_control_t **out_control, char *errbuf, unsigned long errbuf_len) {
  if (out_control == NULL) {
    return fail(errbuf, errbuf_len, "control open", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_control = NULL;
  gzt_control_t *control = (gzt_control_t *)calloc(1, sizeof(*control));
  if (control == NULL) {
    return fail(errbuf, errbuf_len, "control alloc", GZC_ERR_NO_MEMORY);
  }
  int rc = gzc_cgo_backend_init(&control->backend);
  if (rc != GZC_OK) {
    free(control);
    return fail(errbuf, errbuf_len, "control backend init", rc);
  }
  gzc_cgo_backend_http_vtable(&control->backend, &control->http);
  *out_control = control;
  return GZC_OK;
}

void gzt_control_close(gzt_control_t *control) {
  if (control == NULL) {
    return;
  }
  gzc_cgo_backend_deinit(&control->backend);
  free(control);
}

/* Reads one optional field of the step's JSON request body. */
static bool body_str(gzc_str_t body, const char *name, gzc_str_t *out) {
  gzc_str_t raw;
  if (body.len == 0 || gzc_json_find_field(body, name, &raw) != GZC_OK) {
    return false;
  }
  return gzc_json_parse_string(raw, out) == GZC_OK;
}

static bool body_i64(gzc_str_t body, const char *name, int64_t *out) {
  gzc_str_t raw;
  if (body.len == 0 || gzc_json_find_field(body, name, &raw) != GZC_OK) {
    return false;
  }
  return gzc_json_parse_i64(raw, out) == GZC_OK;
}

static bool body_bool(gzc_str_t body, const char *name, bool *out) {
  gzc_str_t raw;
  if (body.len == 0 || gzc_json_find_field(body, name, &raw) != GZC_OK) {
    return false;
  }
  return gzc_json_parse_bool(raw, out) == GZC_OK;
}

/* Splits `path` into the route under /gizclaw/v1, plus any trailing segment. */
typedef struct {
  gzc_str_t route;
  gzc_str_t tail;
} gzt_route_t;

static bool route_is(const gzt_route_t *route, const char *prefix, bool with_tail) {
  size_t len = strlen(prefix);
  if (route->route.len != len || memcmp(route->route.data, prefix, len) != 0) {
    return false;
  }
  return with_tail ? route->tail.len > 0 : route->tail.len == 0;
}

/* Splits a route so `/contacts/alice` yields route `/contacts` and tail
 * `alice`. Known two-segment routes stay whole. */
static void split_route(gzc_str_t path, gzt_route_t *out) {
  /*
   * Routes the controller SDK addresses as a whole, longest first: a prefix
   * check alone would split `/device/telemetry/latest` into
   * `/device/telemetry` plus a `latest` segment.
   */
  static const char *whole[] = {
      "/device/telemetry/aggregate",
      "/device/actions/play-sound",
      "/device/telemetry/latest",
      "/device/actions/reboot",
      "/device/wifi/saved",
      "/device/telemetry",
      "/device/runtime",
      "/device/status",
      "/device/volume",
      "/api-keys/self",
      "/device/wifi",
      "/api-keys",
      "/contacts",
      "/device",
  };
  out->route = path;
  out->tail = gzc_str_from_parts(NULL, 0);
  for (size_t i = 0; i < sizeof(whole) / sizeof(whole[0]); i++) {
    size_t len = strlen(whole[i]);
    if (path.len == len && memcmp(path.data, whole[i], len) == 0) {
      return;
    }
  }
  for (size_t i = 0; i < sizeof(whole) / sizeof(whole[0]); i++) {
    size_t len = strlen(whole[i]);
    if (path.len > len && memcmp(path.data, whole[i], len) == 0 && path.data[len] == '/') {
      out->route = gzc_str_from_parts(path.data, len);
      out->tail = gzc_str_from_parts(path.data + len + 1, path.len - len - 1);
      return;
    }
  }
}

/* Reads one query parameter out of the step path's query string. */
static bool query_param(gzc_str_t query, const char *name, gzc_str_t *out) {
  size_t name_len = strlen(name);
  size_t i = 0;
  while (i < query.len) {
    size_t start = i;
    while (i < query.len && query.data[i] != '&') {
      i++;
    }
    gzc_str_t pair = gzc_str_from_parts(query.data + start, i - start);
    if (i < query.len) {
      i++;
    }
    if (pair.len > name_len && memcmp(pair.data, name, name_len) == 0 && pair.data[name_len] == '=') {
      *out = gzc_str_from_parts(pair.data + name_len + 1, pair.len - name_len - 1);
      return true;
    }
  }
  return false;
}

static bool query_i64(gzc_str_t query, const char *name, int64_t *out) {
  gzc_str_t raw;
  return query_param(query, name, &raw) && gzc_json_parse_i64(raw, out) == GZC_OK;
}

/*
 * Reads the cursor and limit a list step put in its query string. A limit of
 * zero is preserved so the step can observe the Server rejecting it.
 */
static void read_page(gzc_str_t query, gzc_control_page_t *out) {
  int64_t limit = 0;
  memset(out, 0, sizeof(*out));
  (void)query_param(query, "cursor", &out->cursor);
  if (query_i64(query, "limit", &limit)) {
    out->has_limit = true;
    out->limit = (int32_t)limit;
  }
}

/* Percent-decodes one path segment in place into out, which must hold len
 * bytes. Giztest paths only ever carry SDK-encoded segments. */
static size_t decode_segment(gzc_str_t segment, char *out, size_t cap) {
  size_t written = 0;
  for (size_t i = 0; i < segment.len && written < cap; i++) {
    if (segment.data[i] == '%' && i + 2 < segment.len) {
      char hex[3] = {segment.data[i + 1], segment.data[i + 2], 0};
      out[written++] = (char)strtol(hex, NULL, 16);
      i += 2;
      continue;
    }
    out[written++] = segment.data[i];
  }
  return written;
}

int gzt_control_request(
    gzt_control_t *control_host,
    const char *base_url,
    const char *api_key,
    const char *method,
    const char *path,
    const char *request_json,
    int timeout_ms,
    int *out_status,
    unsigned char **out_body,
    unsigned long *out_body_len,
    int *out_error_kind,
    char *errbuf,
    unsigned long errbuf_len) {
  if (control_host == NULL || base_url == NULL || api_key == NULL || method == NULL || path == NULL ||
      out_status == NULL || out_body == NULL || out_body_len == NULL || out_error_kind == NULL) {
    return fail(errbuf, errbuf_len, "control request", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_status = 0;
  *out_body = NULL;
  *out_body_len = 0;
  *out_error_kind = GZC_CONTROL_ERROR_NONE;

  gzc_str_t full = gzc_str_from_cstr(path);
  gzc_str_t prefix = gzc_str_from_cstr(GZC_CONTROL_PATH_PREFIX);
  if (full.len < prefix.len || memcmp(full.data, prefix.data, prefix.len) != 0) {
    set_error(errbuf, errbuf_len, "path is outside the /gizclaw/v1 controller contract");
    return GZC_ERR_UNSUPPORTED;
  }
  gzc_str_t suffix = gzc_str_from_parts(full.data + prefix.len, full.len - prefix.len);
  gzc_str_t query = gzc_str_from_parts(NULL, 0);
  for (size_t i = 0; i < suffix.len; i++) {
    if (suffix.data[i] == '?') {
      query = gzc_str_from_parts(suffix.data + i + 1, suffix.len - i - 1);
      suffix = gzc_str_from_parts(suffix.data, i);
      break;
    }
  }
  gzt_route_t route;
  split_route(suffix, &route);

  char segment[256];
  size_t segment_len = decode_segment(route.tail, segment, sizeof(segment));
  gzc_str_t tail = gzc_str_from_parts(segment, segment_len);
  gzc_str_t body = gzc_str_from_cstr(request_json);

  gzc_control_config_t config;
  memset(&config, 0, sizeof(config));
  config.base_url = gzc_str_from_cstr(base_url);
  config.api_key = gzc_str_from_cstr(api_key);
  config.http = &control_host->http;
  config.platform = control_host->backend.platform;
  config.timeout_ms = timeout_ms;
  gzc_control_client_t control;
  int rc = gzc_control_client_init(&control, &config);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "control client init", rc);
  }

  uint8_t scratch[1024];
  uint8_t response[64 * 1024];
  gzc_control_call_t call;
  rc = gzc_control_call_init(&call, scratch, sizeof(scratch), response, sizeof(response));
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "control call init", rc);
  }

  bool get = strcmp(method, "GET") == 0;
  bool post = strcmp(method, "POST") == 0;
  bool put = strcmp(method, "PUT") == 0;
  bool del = strcmp(method, "DELETE") == 0;

  /* Throwaway decode targets: the runner asserts on the raw body, while the
   * typed call is what proves the controller SDK covers the route. */
  gzc_control_api_key_t api_keys[32];
  gzc_control_contact_t contacts[64];
  gzc_control_telemetry_value_t telemetry_values[32];
  gzc_control_telemetry_point_t telemetry_points[512];
  gzc_control_telemetry_bucket_t telemetry_buckets[512];
  gzc_str_t ssids[32];
  gzc_control_peer_status_t status;
  gzc_control_device_info_t device;
  gzc_control_device_runtime_t runtime;
  gzc_control_wifi_status_t wifi;
  gzc_control_contact_t contact;
  gzc_control_api_key_t api_key_value;
  gzc_str_t text;
  size_t count = 0;
  bool has_next = false;

  if (get && route_is(&route, "/device", false)) {
    rc = gzc_control_get_device(&control, &call, &device);
  } else if (get && route_is(&route, "/device/runtime", false)) {
    rc = gzc_control_get_device_runtime(&control, &call, &runtime);
  } else if (get && route_is(&route, "/device/status", false)) {
    rc = gzc_control_get_device_status(&control, &call, &status);
  } else if (get && route_is(&route, "/device/telemetry/latest", false)) {
    gzc_str_t fields = gzc_str_from_parts(NULL, 0);
    (void)query_param(query, "fields", &fields);
    rc = gzc_control_get_device_telemetry_latest(
        &control, &call, fields, telemetry_values,
        sizeof(telemetry_values) / sizeof(telemetry_values[0]), &count, &text);
  } else if (get && route_is(&route, "/device/telemetry", false)) {
    gzc_control_telemetry_query_t query_spec;
    gzc_str_t order = gzc_str_from_parts(NULL, 0);
    int64_t limit = 0;
    memset(&query_spec, 0, sizeof(query_spec));
    if (!query_param(query, "field", &query_spec.field)) {
      set_error(errbuf, errbuf_len, "telemetry range query requires a field parameter");
      return GZC_ERR_UNSUPPORTED;
    }
    (void)query_i64(query, "start_time_ms", &query_spec.start_time_ms);
    (void)query_i64(query, "end_time_ms", &query_spec.end_time_ms);
    (void)query_i64(query, "step_ms", &query_spec.step_ms);
    if (query_i64(query, "limit", &limit)) {
      query_spec.limit = (int32_t)limit;
    }
    if (query_param(query, "order", &order)) {
      query_spec.order = order.len == 3 && memcmp(order.data, "asc", 3) == 0
                             ? GZC_CONTROL_ORDER_ASC
                             : GZC_CONTROL_ORDER_DESC;
    }
    rc = gzc_control_query_device_telemetry(
        &control, &call, &query_spec, telemetry_points,
        sizeof(telemetry_points) / sizeof(telemetry_points[0]), &count);
  } else if (get && route_is(&route, "/device/telemetry/aggregate", false)) {
    gzc_control_telemetry_aggregate_query_t aggregate;
    gzc_str_t mode = gzc_str_from_parts(NULL, 0);
    memset(&aggregate, 0, sizeof(aggregate));
    if (!query_param(query, "field", &aggregate.field) || !query_param(query, "aggregate", &mode)) {
      set_error(errbuf, errbuf_len, "telemetry aggregate query requires field and aggregate");
      return GZC_ERR_UNSUPPORTED;
    }
    (void)query_i64(query, "start_time_ms", &aggregate.start_time_ms);
    (void)query_i64(query, "end_time_ms", &aggregate.end_time_ms);
    (void)query_i64(query, "bucket_ms", &aggregate.bucket_ms);
    static const struct {
      const char *name;
      gzc_control_aggregate_t value;
    } modes[] = {
        {"avg", GZC_CONTROL_AGGREGATE_AVG},
        {"min", GZC_CONTROL_AGGREGATE_MIN},
        {"max", GZC_CONTROL_AGGREGATE_MAX},
        {"sum", GZC_CONTROL_AGGREGATE_SUM},
        {"count", GZC_CONTROL_AGGREGATE_COUNT},
        {"last", GZC_CONTROL_AGGREGATE_LAST},
    };
    bool matched = false;
    for (size_t i = 0; i < sizeof(modes) / sizeof(modes[0]); i++) {
      size_t len = strlen(modes[i].name);
      if (mode.len == len && memcmp(mode.data, modes[i].name, len) == 0) {
        aggregate.aggregate = modes[i].value;
        matched = true;
        break;
      }
    }
    if (!matched) {
      set_error(errbuf, errbuf_len, "unsupported telemetry aggregate mode");
      return GZC_ERR_UNSUPPORTED;
    }
    rc = gzc_control_aggregate_device_telemetry(
        &control, &call, &aggregate, telemetry_buckets,
        sizeof(telemetry_buckets) / sizeof(telemetry_buckets[0]), &count);
  } else if (get && route_is(&route, "/device/wifi", false)) {
    rc = gzc_control_get_device_wifi(&control, &call, &wifi);
  } else if (get && route_is(&route, "/device/wifi/saved", false)) {
    rc = gzc_control_list_device_saved_wifi(
        &control, &call, ssids, sizeof(ssids) / sizeof(ssids[0]), &count);
  } else if (del && route_is(&route, "/device/wifi/saved", true)) {
    rc = gzc_control_forget_device_saved_wifi(&control, &call, tail);
  } else if (put && route_is(&route, "/device/volume", false)) {
    gzc_control_volume_request_t request;
    int64_t level = 0;
    memset(&request, 0, sizeof(request));
    (void)body_i64(body, "level", &level);
    (void)body_bool(body, "muted", &request.muted);
    request.level = (int32_t)level;
    rc = gzc_control_set_device_volume(&control, &call, &request, &status);
  } else if (post && route_is(&route, "/device/actions/play-sound", false)) {
    gzc_control_play_sound_request_t request;
    int64_t duration = 0;
    memset(&request, 0, sizeof(request));
    (void)body_str(body, "sound", &request.sound);
    if (body_i64(body, "duration_ms", &duration)) {
      request.has_duration_ms = true;
      request.duration_ms = (int32_t)duration;
    }
    rc = gzc_control_play_device_sound(&control, &call, &request);
  } else if (post && route_is(&route, "/device/actions/reboot", false)) {
    gzc_control_reboot_request_t request;
    int64_t delay = 0;
    memset(&request, 0, sizeof(request));
    if (body_i64(body, "delay_ms", &delay)) {
      request.has_delay_ms = true;
      request.delay_ms = (int32_t)delay;
    }
    rc = gzc_control_reboot_device(&control, &call, &request);
  } else if (get && route_is(&route, "/api-keys", false)) {
    gzc_control_page_t page;
    read_page(query, &page);
    rc = gzc_control_list_api_keys(
        &control, &call, &page, api_keys, sizeof(api_keys) / sizeof(api_keys[0]), &count, &text);
  } else if (post && route_is(&route, "/api-keys", false)) {
    gzc_control_api_key_create_request_t request;
    memset(&request, 0, sizeof(request));
    (void)body_str(body, "display_name", &request.display_name);
    (void)body_bool(body, "manage_api_keys", &request.manage_api_keys);
    rc = gzc_control_create_api_key(&control, &call, &request, &api_key_value, &text);
  } else if (get && route_is(&route, "/api-keys/self", false)) {
    rc = gzc_control_get_self_api_key(&control, &call, &api_key_value);
  } else if (del && route_is(&route, "/api-keys/self", false)) {
    rc = gzc_control_revoke_self_api_key(&control, &call);
  } else if (get && route_is(&route, "/api-keys", true)) {
    rc = gzc_control_get_api_key(&control, &call, tail, &api_key_value);
  } else if (del && route_is(&route, "/api-keys", true)) {
    rc = gzc_control_revoke_api_key(&control, &call, tail);
  } else if (get && route_is(&route, "/contacts", false)) {
    gzc_control_page_t page;
    read_page(query, &page);
    rc = gzc_control_list_contacts(
        &control, &call, &page, contacts, sizeof(contacts) / sizeof(contacts[0]), &count, &has_next,
        &text);
  } else if (post && route_is(&route, "/contacts", false)) {
    gzc_control_contact_request_t request;
    memset(&request, 0, sizeof(request));
    (void)body_str(body, "name", &request.name);
    (void)body_str(body, "display_name", &request.display_name);
    (void)body_str(body, "phone_number", &request.phone_number);
    rc = gzc_control_create_contact(&control, &call, &request, &contact);
  } else if (get && route_is(&route, "/contacts", true)) {
    rc = gzc_control_get_contact(&control, &call, tail, &contact);
  } else if (put && route_is(&route, "/contacts", true)) {
    gzc_control_contact_request_t request;
    memset(&request, 0, sizeof(request));
    (void)body_str(body, "display_name", &request.display_name);
    (void)body_str(body, "phone_number", &request.phone_number);
    rc = gzc_control_put_contact(&control, &call, tail, &request, &contact);
  } else if (del && route_is(&route, "/contacts", true)) {
    rc = gzc_control_delete_contact(&control, &call, tail);
  } else {
    set_error(errbuf, errbuf_len, "route is not part of the controller SDK contract");
    return GZC_ERR_UNSUPPORTED;
  }

  *out_status = call.status_code;
  *out_error_kind = (int)call.error.kind;
  if (call.body.len > 0) {
    unsigned char *copy = (unsigned char *)malloc(call.body.len);
    if (copy == NULL) {
      return fail(errbuf, errbuf_len, "copy control body", GZC_ERR_NO_MEMORY);
    }
    memcpy(copy, call.body.data, call.body.len);
    *out_body = copy;
    *out_body_len = (unsigned long)call.body.len;
  }
  /* A classified HTTP failure is a normal outcome: the step decides whether
   * the status was expected. Only a transport or encoding failure is an
   * error here. */
  if (rc != GZC_OK && call.status_code == 0) {
    return fail(errbuf, errbuf_len, "control request", rc);
  }
  return GZC_OK;
}
