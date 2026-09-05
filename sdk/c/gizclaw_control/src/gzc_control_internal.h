/* Internal helpers shared by the controller-side C SDK sources. */
#ifndef GZC_CONTROL_INTERNAL_H
#define GZC_CONTROL_INTERNAL_H

#include "gzc_control.h"

/*
 * Fixed allocator over one caller-owned region.
 *
 * gzc_buf_t and gzc_json_writer_t reach for platform->realloc as they grow.
 * Binding them to this platform keeps every byte inside the caller's region:
 * the allocator hands out that region once and refuses to grow past it with
 * GZC_ERR_NO_MEMORY, so the package never touches the heap.
 */
typedef struct {
  uint8_t *data;
  size_t cap;
  bool handed_out;
  const gzc_platform_t *base;
  gzc_platform_t platform;
} gzc_control_region_t;

/* Binds data/cap to region and prepares region->platform. `base` supplies the
 * clock, entropy, and logging callbacks and may be NULL. */
int gzc_control_region_init(
    gzc_control_region_t *region,
    uint8_t *data,
    size_t cap,
    const gzc_platform_t *base);

/* Prepares an empty gzc_buf_t writing into region. */
void gzc_control_region_buf(gzc_control_region_t *region, gzc_buf_t *out_buf);

/* True when text is empty or its data pointer is NULL. */
bool gzc_control_str_empty(gzc_str_t text);

/* True when both sides hold the same bytes. */
bool gzc_control_str_eq_cstr(gzc_str_t text, const char *other);

/* Appends text percent-encoded as one URL path segment or query value.
 * Unreserved characters (RFC 3986 2.3) pass through unchanged. */
int gzc_control_append_encoded(gzc_buf_t *buf, const gzc_platform_t *platform, gzc_str_t text);

/* Appends a base-10 int64. */
int gzc_control_append_i64(gzc_buf_t *buf, const gzc_platform_t *platform, int64_t value);

/* One request the package sends. `body` is empty for bodyless methods. */
typedef struct {
  gzc_http_method_t method;
  gzc_str_t url;
  gzc_str_t body;
} gzc_control_request_t;

/*
 * Sends request, stores the status and body in call, and classifies failures.
 *
 * Returns GZC_OK only for a 2xx response. Otherwise call->error.kind names the
 * failure and the return value is GZC_ERR_HTTP for a classified HTTP status or
 * the transport's own status code for a transport failure.
 */
int gzc_control_send(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_request_t *request);

/* Marks call as a locally detected failure of kind and returns status. */
int gzc_control_fail(gzc_control_call_t *call, gzc_control_error_kind_t kind, int status);

/* Reads one field of object_json and reports whether it is present and not
 * JSON null. Returns GZC_OK with *out_present false for a missing field. */
int gzc_control_field(gzc_str_t object_json, const char *name, gzc_str_t *out_raw, bool *out_present);

/* Optional-field readers. Each leaves the output untouched and reports
 * present=false when the field is absent or null. */
int gzc_control_opt_str(gzc_str_t object_json, const char *name, gzc_str_t *out);
int gzc_control_opt_bool(gzc_str_t object_json, const char *name, bool *out, bool *out_present);
int gzc_control_opt_i32(gzc_str_t object_json, const char *name, int32_t *out, bool *out_present);
int gzc_control_opt_i64(gzc_str_t object_json, const char *name, int64_t *out, bool *out_present);
int gzc_control_opt_f64(gzc_str_t object_json, const char *name, double *out, bool *out_present);
int gzc_control_opt_raw(gzc_str_t object_json, const char *name, gzc_str_t *out);

/* Required-field readers. Return GZC_ERR_JSON when the field is missing. */
int gzc_control_req_str(gzc_str_t object_json, const char *name, gzc_str_t *out);
int gzc_control_req_bool(gzc_str_t object_json, const char *name, bool *out);
int gzc_control_req_i64(gzc_str_t object_json, const char *name, int64_t *out);
int gzc_control_req_f64(gzc_str_t object_json, const char *name, double *out);

int gzc_control_decode_audioplayer_status(gzc_str_t object_json, gzc_control_audioplayer_status_t *out);
int gzc_control_decode_audioplayer_item(gzc_str_t object_json, void *out);

/* Model decoders over one already-validated JSON object. */
int gzc_control_decode_api_key(gzc_str_t object_json, gzc_control_api_key_t *out);
int gzc_control_decode_device_info(gzc_str_t object_json, gzc_control_device_info_t *out);
int gzc_control_decode_device_runtime(gzc_str_t object_json, gzc_control_device_runtime_t *out);
int gzc_control_decode_peer_status(gzc_str_t object_json, gzc_control_peer_status_t *out);
int gzc_control_decode_wifi_status(gzc_str_t object_json, gzc_control_wifi_status_t *out);
int gzc_control_decode_wifi_scan_result(
    gzc_str_t object_json,
    gzc_control_wifi_scan_result_t *out);
int gzc_control_decode_contact(gzc_str_t object_json, gzc_control_contact_t *out);
int gzc_control_decode_telemetry_value(gzc_str_t object_json, gzc_control_telemetry_value_t *out);
int gzc_control_decode_telemetry_point(gzc_str_t object_json, gzc_control_telemetry_point_t *out);
int gzc_control_decode_telemetry_bucket(gzc_str_t object_json, gzc_control_telemetry_bucket_t *out);

/* Decodes an item of a JSON array into a caller-provided element. */
typedef int (*gzc_control_decode_fn)(gzc_str_t object_json, void *out);

/*
 * Decodes up to cap elements of array_json into out, each stride bytes wide,
 * and reports the decoded count.
 *
 * Returns GZC_ERR_BUFFER_TOO_SMALL when the array holds more than cap
 * elements, after filling out with the first cap of them.
 */
int gzc_control_decode_array(
    gzc_str_t array_json,
    void *out,
    size_t stride,
    size_t cap,
    size_t *out_count,
    gzc_control_decode_fn decode);

/* gzc_control_decode_fn adapters for the list routes. */
int gzc_control_decode_api_key_item(gzc_str_t object_json, void *out);
int gzc_control_decode_contact_item(gzc_str_t object_json, void *out);
int gzc_control_decode_telemetry_value_item(gzc_str_t object_json, void *out);
int gzc_control_decode_telemetry_point_item(gzc_str_t object_json, void *out);
int gzc_control_decode_telemetry_bucket_item(gzc_str_t object_json, void *out);
int gzc_control_decode_saved_wifi_item(gzc_str_t object_json, void *out);
int gzc_control_decode_wifi_scan_result_item(gzc_str_t object_json, void *out);

#endif
