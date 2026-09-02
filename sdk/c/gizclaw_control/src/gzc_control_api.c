/*
 * The `/gizclaw/v1` routes routes.
 *
 * Every function builds its URL and request body inside the caller's scratch
 * region, sends one request, and decodes the response in place.
 */
#include "gzc_control_internal.h"

#include <string.h>

/* One request being assembled inside a call's scratch region. */
typedef struct {
  gzc_control_region_t region;
  const gzc_platform_t *platform;
  gzc_buf_t buf;
  size_t url_len;
  bool query_started;
  int rc;
} gzc_control_builder_t;

/* Starts a URL as `<base_url>/gizclaw/v1<route>`. */
static void builder_begin(
    gzc_control_builder_t *builder,
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const char *route) {
  memset(builder, 0, sizeof(*builder));
  builder->rc = gzc_control_region_init(
      &builder->region, call->scratch, call->scratch_cap, client->config.platform);
  if (builder->rc != GZC_OK) {
    return;
  }
  builder->platform = &builder->region.platform;
  gzc_control_region_buf(&builder->region, &builder->buf);
  builder->rc = gzc_buf_append_str(&builder->buf, builder->platform, client->config.base_url);
  if (builder->rc == GZC_OK) {
    builder->rc = gzc_buf_append_cstr(&builder->buf, builder->platform, GZC_CONTROL_PATH_PREFIX);
  }
  if (builder->rc == GZC_OK) {
    builder->rc = gzc_buf_append_cstr(&builder->buf, builder->platform, route);
  }
}

/* Appends one percent-encoded path segment, preceded by a slash. */
static void builder_segment(gzc_control_builder_t *builder, gzc_str_t segment) {
  if (builder->rc != GZC_OK) {
    return;
  }
  if (gzc_control_str_empty(segment)) {
    builder->rc = GZC_ERR_INVALID_ARGUMENT;
    return;
  }
  builder->rc = gzc_buf_append_cstr(&builder->buf, builder->platform, "/");
  if (builder->rc == GZC_OK) {
    builder->rc = gzc_control_append_encoded(&builder->buf, builder->platform, segment);
  }
}

static void builder_query_prefix(gzc_control_builder_t *builder, const char *name) {
  builder->rc = gzc_buf_append_cstr(&builder->buf, builder->platform, builder->query_started ? "&" : "?");
  builder->query_started = true;
  if (builder->rc == GZC_OK) {
    builder->rc = gzc_buf_append_cstr(&builder->buf, builder->platform, name);
  }
  if (builder->rc == GZC_OK) {
    builder->rc = gzc_buf_append_cstr(&builder->buf, builder->platform, "=");
  }
}

/* Appends `name=value` percent-encoded; an empty value is skipped. */
static void builder_query_str(gzc_control_builder_t *builder, const char *name, gzc_str_t value) {
  if (builder->rc != GZC_OK || gzc_control_str_empty(value)) {
    return;
  }
  builder_query_prefix(builder, name);
  if (builder->rc == GZC_OK) {
    builder->rc = gzc_control_append_encoded(&builder->buf, builder->platform, value);
  }
}

static void builder_query_i64(gzc_control_builder_t *builder, const char *name, int64_t value) {
  if (builder->rc != GZC_OK) {
    return;
  }
  builder_query_prefix(builder, name);
  if (builder->rc == GZC_OK) {
    builder->rc = gzc_control_append_i64(&builder->buf, builder->platform, value);
  }
}

/* Freezes the URL so the body can be appended after it in the same region. */
static gzc_str_t builder_url(gzc_control_builder_t *builder) {
  if (builder->rc != GZC_OK) {
    return gzc_str_from_parts(NULL, 0);
  }
  builder->url_len = builder->buf.len;
  return gzc_str_from_parts((const char *)builder->buf.data, builder->url_len);
}

/* Body written after the URL. The region never moves, so the URL slice handed
 * out by builder_url() stays valid while the body grows. */
static void builder_body_begin(gzc_control_builder_t *builder, gzc_json_writer_t *writer) {
  if (builder->rc != GZC_OK) {
    return;
  }
  /* Keep the URL NUL-terminated for transports that treat url.data as a C
   * string, then start the body one byte later. */
  static const char terminator = 0;
  builder->rc = gzc_buf_append(&builder->buf, builder->platform, &terminator, 1);
  if (builder->rc != GZC_OK) {
    return;
  }
  builder->url_len = builder->buf.len;
  gzc_json_writer_init(writer, builder->platform, &builder->buf);
  builder->rc = gzc_json_object_begin(writer);
}

static gzc_str_t builder_body(gzc_control_builder_t *builder, gzc_json_writer_t *writer) {
  if (builder->rc == GZC_OK) {
    builder->rc = gzc_json_object_end(writer);
  }
  if (builder->rc != GZC_OK) {
    return gzc_str_from_parts(NULL, 0);
  }
  return gzc_str_from_parts(
      (const char *)builder->buf.data + builder->url_len, builder->buf.len - builder->url_len);
}

/* Validates a contract-capped request string. */
static int check_cap(gzc_str_t value, size_t cap, bool required) {
  if (gzc_control_str_empty(value)) {
    return required ? GZC_ERR_INVALID_ARGUMENT : GZC_OK;
  }
  return value.len > cap ? GZC_ERR_INVALID_ARGUMENT : GZC_OK;
}

/* Sends the assembled request, or reports the build failure through call. */
static int builder_send(
    gzc_control_builder_t *builder,
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_http_method_t method,
    gzc_str_t url,
    gzc_str_t body) {
  if (builder->rc != GZC_OK) {
    return gzc_control_fail(call, GZC_CONTROL_ERROR_NETWORK, builder->rc);
  }
  gzc_control_request_t request;
  request.method = method;
  request.url = url;
  request.body = body;
  return gzc_control_send(client, call, &request);
}

/* Validates a 2xx body as a JSON object and reports a decode failure as
 * GZC_CONTROL_ERROR_MALFORMED_RESPONSE. */
static int decoded_object(gzc_control_call_t *call, gzc_str_t *out_object) {
  int rc = gzc_json_validate_object(call->body);
  if (rc != GZC_OK) {
    call->error.kind = GZC_CONTROL_ERROR_MALFORMED_RESPONSE;
    call->error.status_code = call->status_code;
    return rc;
  }
  *out_object = call->body;
  return GZC_OK;
}

static int decode_failed(gzc_control_call_t *call, int rc) {
  call->error.kind = GZC_CONTROL_ERROR_MALFORMED_RESPONSE;
  call->error.status_code = call->status_code;
  return rc;
}

static int check_args(gzc_control_client_t *client, gzc_control_call_t *call) {
  if (client == NULL || call == NULL || call->scratch == NULL || call->response == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return GZC_OK;
}

/* --- API keys ----------------------------------------------------------- */

int gzc_control_create_api_key(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_api_key_create_request_t *request,
    gzc_control_api_key_t *out_value,
    gzc_str_t *out_api_key) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || request == NULL || out_value == NULL || out_api_key == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  rc = check_cap(request->display_name, GZC_CONTROL_MAX_DISPLAY_NAME_BYTES, true);
  if (rc != GZC_OK) {
    return gzc_control_fail(call, GZC_CONTROL_ERROR_INVALID_REQUEST, rc);
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/api-keys");
  gzc_str_t url = builder_url(&builder);
  gzc_json_writer_t writer;
  builder_body_begin(&builder, &writer);
  if (builder.rc == GZC_OK) {
    builder.rc = gzc_json_field_str(&writer, "display_name", request->display_name);
  }
  if (builder.rc == GZC_OK) {
    builder.rc = gzc_json_field_bool(&writer, "manage_api_keys", request->manage_api_keys);
  }
  gzc_str_t body = builder_body(&builder, &writer);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_POST, url, body);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t value_raw = gzc_str_from_parts(NULL, 0);
  rc = gzc_control_opt_raw(object, "value", &value_raw);
  if (rc == GZC_OK && gzc_control_str_empty(value_raw)) {
    rc = GZC_ERR_JSON;
  }
  if (rc == GZC_OK) {
    rc = gzc_control_decode_api_key(value_raw, out_value);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object, "api_key", out_api_key);
  }
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_list_api_keys(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_page_t *page,
    gzc_control_api_key_t *out_items,
    size_t cap,
    size_t *out_count,
    gzc_str_t *out_next_cursor) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_count == NULL || out_next_cursor == NULL || (out_items == NULL && cap != 0)) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  *out_count = 0;
  *out_next_cursor = gzc_str_from_parts(NULL, 0);
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/api-keys");
  if (page != NULL) {
    builder_query_str(&builder, "cursor", page->cursor);
    if (page->limit > 0) {
      builder_query_i64(&builder, "limit", page->limit);
    }
  }
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t items = gzc_str_from_parts(NULL, 0);
  rc = gzc_control_opt_raw(object, "items", &items);
  if (rc == GZC_OK) {
    rc = gzc_control_decode_array(
        items, out_items, sizeof(*out_items), cap, out_count, gzc_control_decode_api_key_item);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object, "next_cursor", out_next_cursor);
  }
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

/* Shared body of the routes returning one APIKey. */
static int get_api_key_route(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const char *route,
    gzc_str_t segment,
    gzc_control_api_key_t *out_value) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_value == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, route);
  if (!gzc_control_str_empty(segment)) {
    builder_segment(&builder, segment);
  }
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_decode_api_key(object, out_value);
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

/* Shared body of every DELETE route, all of which answer 204. */
static int delete_route(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const char *route,
    gzc_str_t segment) {
  int rc = check_args(client, call);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, route);
  if (!gzc_control_str_empty(segment)) {
    builder_segment(&builder, segment);
  }
  gzc_str_t url = builder_url(&builder);
  return builder_send(&builder, client, call, GZC_HTTP_METHOD_DELETE, url, gzc_str_from_parts(NULL, 0));
}

int gzc_control_get_self_api_key(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_api_key_t *out_value) {
  return get_api_key_route(client, call, "/api-keys/self", gzc_str_from_parts(NULL, 0), out_value);
}

int gzc_control_revoke_self_api_key(gzc_control_client_t *client, gzc_control_call_t *call) {
  return delete_route(client, call, "/api-keys/self", gzc_str_from_parts(NULL, 0));
}

int gzc_control_get_api_key(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t api_key_name,
    gzc_control_api_key_t *out_value) {
  if (gzc_control_str_empty(api_key_name)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return get_api_key_route(client, call, "/api-keys", api_key_name, out_value);
}

int gzc_control_revoke_api_key(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t api_key_name) {
  if (gzc_control_str_empty(api_key_name)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return delete_route(client, call, "/api-keys", api_key_name);
}

/* --- Device reads ------------------------------------------------------- */

int gzc_control_get_device(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_device_info_t *out_device) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_device == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device");
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_decode_device_info(object, out_device);
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_get_device_runtime(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_device_runtime_t *out_runtime) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_runtime == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/runtime");
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_decode_device_runtime(object, out_runtime);
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_get_device_status(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_peer_status_t *out_status) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_status == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/status");
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_decode_peer_status(object, out_status);
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_get_device_telemetry_latest(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t fields,
    gzc_control_telemetry_value_t *out_values,
    size_t cap,
    size_t *out_count,
    gzc_str_t *out_peer_public_key) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_count == NULL || out_peer_public_key == NULL ||
      (out_values == NULL && cap != 0)) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  *out_count = 0;
  *out_peer_public_key = gzc_str_from_parts(NULL, 0);
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/telemetry/latest");
  builder_query_str(&builder, "fields", fields);
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_req_str(object, "peer_public_key", out_peer_public_key);
  gzc_str_t values = gzc_str_from_parts(NULL, 0);
  if (rc == GZC_OK) {
    rc = gzc_control_opt_raw(object, "values", &values);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_decode_array(
        values, out_values, sizeof(*out_values), cap, out_count,
        gzc_control_decode_telemetry_value_item);
  }
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_query_device_telemetry(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_telemetry_query_t *query,
    gzc_control_telemetry_point_t *out_points,
    size_t cap,
    size_t *out_count) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || query == NULL || out_count == NULL || (out_points == NULL && cap != 0)) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  if (gzc_control_str_empty(query->field)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_count = 0;
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/telemetry");
  builder_query_str(&builder, "field", query->field);
  builder_query_i64(&builder, "start_time_ms", query->start_time_ms);
  builder_query_i64(&builder, "end_time_ms", query->end_time_ms);
  if (query->step_ms > 0) {
    builder_query_i64(&builder, "step_ms", query->step_ms);
  }
  if (query->limit > 0) {
    builder_query_i64(&builder, "limit", query->limit);
  }
  if (query->order == GZC_CONTROL_ORDER_ASC) {
    builder_query_str(&builder, "order", gzc_str_from_cstr("asc"));
  } else if (query->order == GZC_CONTROL_ORDER_DESC) {
    builder_query_str(&builder, "order", gzc_str_from_cstr("desc"));
  }
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t points = gzc_str_from_parts(NULL, 0);
  rc = gzc_control_opt_raw(object, "points", &points);
  if (rc == GZC_OK) {
    rc = gzc_control_decode_array(
        points, out_points, sizeof(*out_points), cap, out_count,
        gzc_control_decode_telemetry_point_item);
  }
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

static const char *aggregate_wire_value(gzc_control_aggregate_t aggregate) {
  switch (aggregate) {
  case GZC_CONTROL_AGGREGATE_AVG:
    return "avg";
  case GZC_CONTROL_AGGREGATE_MIN:
    return "min";
  case GZC_CONTROL_AGGREGATE_MAX:
    return "max";
  case GZC_CONTROL_AGGREGATE_SUM:
    return "sum";
  case GZC_CONTROL_AGGREGATE_COUNT:
    return "count";
  case GZC_CONTROL_AGGREGATE_LAST:
    return "last";
  }
  return NULL;
}

int gzc_control_aggregate_device_telemetry(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_telemetry_aggregate_query_t *query,
    gzc_control_telemetry_bucket_t *out_points,
    size_t cap,
    size_t *out_count) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || query == NULL || out_count == NULL || (out_points == NULL && cap != 0)) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  const char *aggregate = aggregate_wire_value(query->aggregate);
  if (gzc_control_str_empty(query->field) || aggregate == NULL || query->bucket_ms <= 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_count = 0;
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/telemetry/aggregate");
  builder_query_str(&builder, "field", query->field);
  builder_query_i64(&builder, "start_time_ms", query->start_time_ms);
  builder_query_i64(&builder, "end_time_ms", query->end_time_ms);
  builder_query_i64(&builder, "bucket_ms", query->bucket_ms);
  builder_query_str(&builder, "aggregate", gzc_str_from_cstr(aggregate));
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t points = gzc_str_from_parts(NULL, 0);
  rc = gzc_control_opt_raw(object, "points", &points);
  if (rc == GZC_OK) {
    rc = gzc_control_decode_array(
        points, out_points, sizeof(*out_points), cap, out_count,
        gzc_control_decode_telemetry_bucket_item);
  }
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

/* --- Device control ----------------------------------------------------- */

int gzc_control_set_device_volume(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_volume_request_t *request,
    gzc_control_peer_status_t *out_status) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || request == NULL || out_status == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  if (request->level < GZC_CONTROL_MIN_VOLUME_LEVEL || request->level > GZC_CONTROL_MAX_VOLUME_LEVEL) {
    return gzc_control_fail(call, GZC_CONTROL_ERROR_INVALID_REQUEST, GZC_ERR_INVALID_ARGUMENT);
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/volume");
  gzc_str_t url = builder_url(&builder);
  gzc_json_writer_t writer;
  builder_body_begin(&builder, &writer);
  if (builder.rc == GZC_OK) {
    builder.rc = gzc_json_field_i32(&writer, "level", request->level);
  }
  if (builder.rc == GZC_OK) {
    builder.rc = gzc_json_field_bool(&writer, "muted", request->muted);
  }
  gzc_str_t body = builder_body(&builder, &writer);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_PUT, url, body);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t status_raw = gzc_str_from_parts(NULL, 0);
  rc = gzc_control_opt_raw(object, "status", &status_raw);
  if (rc == GZC_OK && gzc_control_str_empty(status_raw)) {
    rc = GZC_ERR_JSON;
  }
  if (rc == GZC_OK) {
    rc = gzc_control_decode_peer_status(status_raw, out_status);
  }
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_play_device_sound(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_play_sound_request_t *request) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || request == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  rc = check_cap(request->sound, GZC_CONTROL_MAX_SOUND_BYTES, true);
  if (rc != GZC_OK) {
    return gzc_control_fail(call, GZC_CONTROL_ERROR_INVALID_REQUEST, rc);
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/actions/play-sound");
  gzc_str_t url = builder_url(&builder);
  gzc_json_writer_t writer;
  builder_body_begin(&builder, &writer);
  if (builder.rc == GZC_OK) {
    builder.rc = gzc_json_field_str(&writer, "sound", request->sound);
  }
  if (builder.rc == GZC_OK && request->has_duration_ms) {
    builder.rc = gzc_json_field_i32(&writer, "duration_ms", request->duration_ms);
  }
  gzc_str_t body = builder_body(&builder, &writer);
  return builder_send(&builder, client, call, GZC_HTTP_METHOD_POST, url, body);
}

int gzc_control_reboot_device(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_reboot_request_t *request) {
  int rc = check_args(client, call);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/actions/reboot");
  gzc_str_t url = builder_url(&builder);
  gzc_json_writer_t writer;
  builder_body_begin(&builder, &writer);
  if (builder.rc == GZC_OK && request != NULL && request->has_delay_ms) {
    builder.rc = gzc_json_field_i32(&writer, "delay_ms", request->delay_ms);
  }
  gzc_str_t body = builder_body(&builder, &writer);
  return builder_send(&builder, client, call, GZC_HTTP_METHOD_POST, url, body);
}

int gzc_control_get_device_wifi(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_wifi_status_t *out_status) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_status == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/wifi");
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_decode_wifi_status(object, out_status);
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_list_device_saved_wifi(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t *out_ssids,
    size_t cap,
    size_t *out_count) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_count == NULL || (out_ssids == NULL && cap != 0)) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  *out_count = 0;
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/device/wifi/saved");
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t networks = gzc_str_from_parts(NULL, 0);
  rc = gzc_control_opt_raw(object, "networks", &networks);
  if (rc == GZC_OK) {
    rc = gzc_control_decode_array(
        networks, out_ssids, sizeof(*out_ssids), cap, out_count, gzc_control_decode_saved_wifi_item);
  }
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_forget_device_saved_wifi(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t ssid) {
  int rc = check_cap(ssid, GZC_CONTROL_MAX_SSID_BYTES, true);
  if (rc != GZC_OK) {
    return rc;
  }
  return delete_route(client, call, "/device/wifi/saved", ssid);
}

/* --- Contacts ----------------------------------------------------------- */

int gzc_control_list_contacts(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_page_t *page,
    gzc_control_contact_t *out_items,
    size_t cap,
    size_t *out_count,
    bool *out_has_next,
    gzc_str_t *out_next_cursor) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_count == NULL || out_has_next == NULL || out_next_cursor == NULL ||
      (out_items == NULL && cap != 0)) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  *out_count = 0;
  *out_has_next = false;
  *out_next_cursor = gzc_str_from_parts(NULL, 0);
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/contacts");
  if (page != NULL) {
    builder_query_str(&builder, "cursor", page->cursor);
    if (page->limit > 0) {
      builder_query_i64(&builder, "limit", page->limit);
    }
  }
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t items = gzc_str_from_parts(NULL, 0);
  rc = gzc_control_opt_raw(object, "items", &items);
  if (rc == GZC_OK) {
    rc = gzc_control_decode_array(
        items, out_items, sizeof(*out_items), cap, out_count, gzc_control_decode_contact_item);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_bool(object, "has_next", out_has_next);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object, "next_cursor", out_next_cursor);
  }
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

/* Writes the optional contact fields shared by create and put. */
static void write_contact_fields(
    gzc_control_builder_t *builder,
    gzc_json_writer_t *writer,
    const gzc_control_contact_request_t *request) {
  if (builder->rc == GZC_OK && !gzc_control_str_empty(request->display_name)) {
    builder->rc = gzc_json_field_str(writer, "display_name", request->display_name);
  }
  if (builder->rc == GZC_OK && !gzc_control_str_empty(request->phone_number)) {
    builder->rc = gzc_json_field_str(writer, "phone_number", request->phone_number);
  }
}

int gzc_control_create_contact(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_contact_request_t *request,
    gzc_control_contact_t *out_contact) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || request == NULL || out_contact == NULL) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  if (gzc_control_str_empty(request->name)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  rc = check_cap(request->display_name, GZC_CONTROL_MAX_DISPLAY_NAME_BYTES, false);
  if (rc != GZC_OK) {
    return gzc_control_fail(call, GZC_CONTROL_ERROR_INVALID_REQUEST, rc);
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/contacts");
  gzc_str_t url = builder_url(&builder);
  gzc_json_writer_t writer;
  builder_body_begin(&builder, &writer);
  if (builder.rc == GZC_OK) {
    builder.rc = gzc_json_field_str(&writer, "name", request->name);
  }
  write_contact_fields(&builder, &writer, request);
  gzc_str_t body = builder_body(&builder, &writer);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_POST, url, body);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_decode_contact(object, out_contact);
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_get_contact(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t contact_name,
    gzc_control_contact_t *out_contact) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || out_contact == NULL || gzc_control_str_empty(contact_name)) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/contacts");
  builder_segment(&builder, contact_name);
  gzc_str_t url = builder_url(&builder);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_GET, url, gzc_str_from_parts(NULL, 0));
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_decode_contact(object, out_contact);
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_put_contact(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t contact_name,
    const gzc_control_contact_request_t *request,
    gzc_control_contact_t *out_contact) {
  int rc = check_args(client, call);
  if (rc != GZC_OK || request == NULL || out_contact == NULL || gzc_control_str_empty(contact_name)) {
    return rc == GZC_OK ? GZC_ERR_INVALID_ARGUMENT : rc;
  }
  rc = check_cap(request->display_name, GZC_CONTROL_MAX_DISPLAY_NAME_BYTES, false);
  if (rc != GZC_OK) {
    return gzc_control_fail(call, GZC_CONTROL_ERROR_INVALID_REQUEST, rc);
  }
  gzc_control_builder_t builder;
  builder_begin(&builder, client, call, "/contacts");
  builder_segment(&builder, contact_name);
  gzc_str_t url = builder_url(&builder);
  gzc_json_writer_t writer;
  builder_body_begin(&builder, &writer);
  write_contact_fields(&builder, &writer, request);
  gzc_str_t body = builder_body(&builder, &writer);
  rc = builder_send(&builder, client, call, GZC_HTTP_METHOD_PUT, url, body);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_str_t object;
  rc = decoded_object(call, &object);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_decode_contact(object, out_contact);
  return rc == GZC_OK ? GZC_OK : decode_failed(call, rc);
}

int gzc_control_delete_contact(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t contact_name) {
  if (gzc_control_str_empty(contact_name)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return delete_route(client, call, "/contacts", contact_name);
}
