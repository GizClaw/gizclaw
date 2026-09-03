/* Client setup, fixed-region allocation, URL encoding, and request dispatch. */
#include "gzc_control_internal.h"

#include <string.h>

#define GZC_CONTROL_DEFAULT_TIMEOUT_MS 30000

static void *region_malloc(void *userdata, size_t size) {
  gzc_control_region_t *region = (gzc_control_region_t *)userdata;
  if (region == NULL || region->handed_out || size > region->cap) {
    return NULL;
  }
  region->handed_out = true;
  return region->data;
}

static void *region_realloc(void *userdata, void *ptr, size_t size) {
  gzc_control_region_t *region = (gzc_control_region_t *)userdata;
  if (region == NULL || size > region->cap) {
    return NULL;
  }
  if (ptr == NULL) {
    return region_malloc(userdata, size);
  }
  if (ptr != region->data) {
    return NULL;
  }
  return region->data;
}

static void region_free(void *userdata, void *ptr) {
  gzc_control_region_t *region = (gzc_control_region_t *)userdata;
  if (region == NULL || ptr == NULL || ptr != region->data) {
    return;
  }
  region->handed_out = false;
}

static int64_t region_time_instant_ms(void *userdata) {
  const gzc_control_region_t *region = (const gzc_control_region_t *)userdata;
  if (region == NULL || region->base == NULL || region->base->time_instant_ms == NULL) {
    return 0;
  }
  return region->base->time_instant_ms(region->base->userdata);
}

static int64_t region_time_unix_ms(void *userdata) {
  const gzc_control_region_t *region = (const gzc_control_region_t *)userdata;
  if (region == NULL || region->base == NULL || region->base->time_unix_ms == NULL) {
    return 0;
  }
  return region->base->time_unix_ms(region->base->userdata);
}

static int region_random(void *userdata, uint8_t *out, size_t len) {
  const gzc_control_region_t *region = (const gzc_control_region_t *)userdata;
  if (region == NULL || region->base == NULL || region->base->random == NULL) {
    return GZC_ERR_UNSUPPORTED;
  }
  return region->base->random(region->base->userdata, out, len);
}

static void region_log(void *userdata, gzc_log_level_t level, gzc_str_t message) {
  const gzc_control_region_t *region = (const gzc_control_region_t *)userdata;
  if (region == NULL || region->base == NULL || region->base->log == NULL) {
    return;
  }
  region->base->log(region->base->userdata, level, message);
}

int gzc_control_region_init(
    gzc_control_region_t *region,
    uint8_t *data,
    size_t cap,
    const gzc_platform_t *base) {
  if (region == NULL || data == NULL || cap == 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(region, 0, sizeof(*region));
  region->data = data;
  region->cap = cap;
  region->base = base;
  region->platform.userdata = region;
  region->platform.malloc = region_malloc;
  region->platform.realloc = region_realloc;
  region->platform.free = region_free;
  region->platform.time_instant_ms = region_time_instant_ms;
  region->platform.time_unix_ms = region_time_unix_ms;
  region->platform.random = region_random;
  region->platform.log = region_log;
  return GZC_OK;
}

void gzc_control_region_buf(gzc_control_region_t *region, gzc_buf_t *out_buf) {
  if (out_buf == NULL) {
    return;
  }
  gzc_buf_init(out_buf);
  if (region != NULL) {
    region->handed_out = false;
  }
}

bool gzc_control_str_empty(gzc_str_t text) { return text.data == NULL || text.len == 0; }

bool gzc_control_str_eq_cstr(gzc_str_t text, const char *other) {
  if (other == NULL) {
    return gzc_control_str_empty(text);
  }
  size_t other_len = strlen(other);
  if (text.len != other_len) {
    return false;
  }
  if (other_len == 0) {
    return true;
  }
  return text.data != NULL && memcmp(text.data, other, other_len) == 0;
}

static bool is_unreserved(unsigned char ch) {
  return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') ||
         ch == '-' || ch == '.' || ch == '_' || ch == '~';
}

int gzc_control_append_encoded(gzc_buf_t *buf, const gzc_platform_t *platform, gzc_str_t text) {
  static const char hex[] = "0123456789ABCDEF";
  if (buf == NULL || platform == NULL || (text.data == NULL && text.len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  for (size_t i = 0; i < text.len; i++) {
    unsigned char ch = (unsigned char)text.data[i];
    int rc;
    if (is_unreserved(ch)) {
      rc = gzc_buf_append(buf, platform, &text.data[i], 1);
    } else {
      char escaped[3] = {'%', hex[ch >> 4], hex[ch & 0x0f]};
      rc = gzc_buf_append(buf, platform, escaped, sizeof(escaped));
    }
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_OK;
}

int gzc_control_append_i64(gzc_buf_t *buf, const gzc_platform_t *platform, int64_t value) {
  /* -9223372036854775808 needs 20 characters plus the sign slot. */
  char digits[21];
  size_t index = sizeof(digits);
  bool negative = value < 0;
  uint64_t magnitude = negative ? (uint64_t)(-(value + 1)) + 1u : (uint64_t)value;
  do {
    digits[--index] = (char)('0' + (magnitude % 10u));
    magnitude /= 10u;
  } while (magnitude != 0u && index != 0);
  if (negative) {
    int rc = gzc_buf_append_cstr(buf, platform, "-");
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return gzc_buf_append(buf, platform, &digits[index], sizeof(digits) - index);
}

int gzc_control_client_init(gzc_control_client_t *client, const gzc_control_config_t *config) {
  if (client == NULL || config == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (gzc_control_str_empty(config->base_url) || gzc_control_str_empty(config->api_key)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (config->http == NULL || config->http->request == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (config->timeout_ms < 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(client, 0, sizeof(*client));
  client->config = *config;
  if (client->config.platform == NULL) {
    client->config.platform = gzc_default_platform();
  }
  if (client->config.timeout_ms == 0) {
    client->config.timeout_ms = GZC_CONTROL_DEFAULT_TIMEOUT_MS;
  }
  /* A trailing slash would double up against the route prefix. */
  while (client->config.base_url.len > 0 &&
         client->config.base_url.data[client->config.base_url.len - 1] == '/') {
    client->config.base_url.len--;
  }
  if (client->config.base_url.len == 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return GZC_OK;
}

int gzc_control_call_init(
    gzc_control_call_t *call,
    uint8_t *scratch,
    size_t scratch_cap,
    uint8_t *response,
    size_t response_cap) {
  if (call == NULL || scratch == NULL || scratch_cap == 0 || response == NULL || response_cap == 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(call, 0, sizeof(*call));
  call->scratch = scratch;
  call->scratch_cap = scratch_cap;
  call->response = response;
  call->response_cap = response_cap;
  return GZC_OK;
}

int gzc_control_fail(gzc_control_call_t *call, gzc_control_error_kind_t kind, int status) {
  if (call != NULL) {
    memset(&call->error, 0, sizeof(call->error));
    call->error.kind = kind;
    call->status_code = 0;
    call->body = gzc_str_from_parts(NULL, 0);
  }
  return status;
}

/*
 * Captures X-Request-ID out of the response headers. Every other header is
 * ignored, and an oversized value is truncated: the id is diagnostic and must
 * never fail an otherwise good call.
 */
static bool header_is(gzc_str_t name, const char *lowercase) {
  size_t len = strlen(lowercase);
  if (name.len != len || name.data == NULL) {
    return false;
  }
  for (size_t i = 0; i < len; i++) {
    char actual = name.data[i];
    if (actual >= 'A' && actual <= 'Z') {
      actual = (char)(actual - 'A' + 'a');
    }
    if (actual != lowercase[i]) {
      return false;
    }
  }
  return true;
}

/*
 * Captures X-Request-ID out of the response headers. Every other header is
 * ignored, and an oversized value is truncated: the id is diagnostic and must
 * never fail an otherwise good call.
 */
static int capture_request_id(
    void *userdata,
    const gzc_http_request_t *request,
    gzc_str_t name,
    gzc_str_t value) {
  (void)request;
  gzc_control_call_t *call = (gzc_control_call_t *)userdata;
  if (call == NULL || !header_is(name, "x-request-id")) {
    return GZC_OK;
  }
  size_t count = value.len;
  if (count > sizeof(call->request_id)) {
    count = sizeof(call->request_id);
  }
  if (count > 0 && value.data != NULL) {
    memcpy(call->request_id, value.data, count);
  }
  call->request_id_len = count;
  return GZC_OK;
}

/* Reads `error.code`, `error.message`, and `error.details` out of a non-2xx
 * body. A body that is not an ErrorResponse leaves every field empty. */
static void decode_error_payload(gzc_str_t body, gzc_control_error_t *out) {
  gzc_str_t error_object;
  if (gzc_json_validate_object(body) != GZC_OK) {
    return;
  }
  if (gzc_json_find_field(body, "error", &error_object) != GZC_OK) {
    return;
  }
  if (gzc_json_validate_object(error_object) != GZC_OK) {
    return;
  }
  (void)gzc_control_opt_str(error_object, "code", &out->code);
  (void)gzc_control_opt_str(error_object, "message", &out->message);
  (void)gzc_control_opt_raw(error_object, "details", &out->details);
}

int gzc_control_send(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_request_t *request) {
  if (client == NULL || call == NULL || request == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(&call->error, 0, sizeof(call->error));
  call->status_code = 0;
  call->body = gzc_str_from_parts(NULL, 0);
  call->request_id_len = 0;

  gzc_http_header_t headers[3];
  size_t header_count = 0;
  headers[header_count].name = gzc_str_from_cstr("Authorization");
  headers[header_count].value = client->config.api_key;
  header_count++;
  headers[header_count].name = gzc_str_from_cstr("Accept");
  headers[header_count].value = gzc_str_from_cstr("application/json");
  header_count++;
  if (!gzc_control_str_empty(request->body)) {
    headers[header_count].name = gzc_str_from_cstr("Content-Type");
    headers[header_count].value = gzc_str_from_cstr("application/json");
    header_count++;
  }

  gzc_http_request_t http_request;
  memset(&http_request, 0, sizeof(http_request));
  http_request.method = request->method;
  http_request.url = request->url;
  http_request.headers = headers;
  http_request.header_count = header_count;
  http_request.body = (const uint8_t *)request->body.data;
  http_request.body_len = request->body.len;
  http_request.response_header_cb = capture_request_id;
  http_request.response_header_userdata = call;
  http_request.timeout_ms = client->config.timeout_ms;
  http_request.interface_name = client->config.interface_name;
  http_request.response_buf = call->response;
  http_request.response_buf_cap = call->response_cap;
  http_request.response_platform = client->config.platform;

  gzc_http_response_t response;
  memset(&response, 0, sizeof(response));
  gzc_buf_init(&response.body);
  int rc = client->config.http->request(client->config.http->userdata, &http_request, &response);
  if (rc != GZC_OK) {
    if (client->config.http->response_free != NULL) {
      client->config.http->response_free(client->config.http->userdata, &response);
    }
    return gzc_control_fail(call, GZC_CONTROL_ERROR_NETWORK, rc);
  }

  /* Keep the body inside the caller's region: a transport that allocated its
   * own buffer is copied over and released before this function returns. */
  size_t length = response.body.len;
  if (length > call->response_cap) {
    if (client->config.http->response_free != NULL) {
      client->config.http->response_free(client->config.http->userdata, &response);
    }
    return gzc_control_fail(call, GZC_CONTROL_ERROR_MALFORMED_RESPONSE, GZC_ERR_BUFFER_TOO_SMALL);
  }
  if (length > 0 && response.body.data != call->response) {
    memcpy(call->response, response.body.data, length);
  }
  call->status_code = response.status_code;
  call->body = gzc_str_from_parts((const char *)call->response, length);
  if (client->config.http->response_free != NULL) {
    client->config.http->response_free(client->config.http->userdata, &response);
  }

  if (response.status_code >= 200 && response.status_code < 300) {
    return GZC_OK;
  }
  call->error.status_code = response.status_code;
  call->error.request_id = gzc_str_from_parts(call->request_id, call->request_id_len);
  decode_error_payload(call->body, &call->error);
  call->error.kind = gzc_control_classify(response.status_code, call->error.code);
  return GZC_ERR_HTTP;
}
