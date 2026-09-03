#ifndef GZC_HTTP_H
#define GZC_HTTP_H

#include "gzc_platform.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
  GZC_HTTP_METHOD_GET = 1,
  GZC_HTTP_METHOD_POST = 2,
  GZC_HTTP_METHOD_PUT = 3,
  GZC_HTTP_METHOD_PATCH = 4,
  GZC_HTTP_METHOD_DELETE = 5,
  GZC_HTTP_METHOD_HEAD = 6,
  GZC_HTTP_METHOD_OPTIONS = 7
} gzc_http_method_t;

typedef struct {
  gzc_str_t name;
  gzc_str_t value;
} gzc_http_header_t;

typedef struct gzc_http_request gzc_http_request_t;

/*
 * Receives one response header during a synchronous request. Name and value
 * are borrowed byte spans valid only for the callback and are not
 * NUL-terminated. Returning any status other than GZC_OK aborts the request
 * and is returned to the caller. A backend may invoke the callback again for
 * each retry attempt, and never retains it after request() returns.
 */
typedef int (*gzc_http_response_header_fn)(
    void *userdata,
    const gzc_http_request_t *request,
    gzc_str_t name,
    gzc_str_t value);

typedef int (*gzc_http_read_fn)(
    void *userdata,
    const gzc_http_request_t *request,
    const uint8_t *chunk,
    size_t chunk_len,
    size_t total_read,
    int64_t remaining);

struct gzc_http_request {
  gzc_http_method_t method;
  gzc_str_t url;
  const gzc_http_header_t *headers;
  size_t header_count;
  const uint8_t *body;
  size_t body_len;

  /* Optional per-header sink. NULL discards the response headers. */
  gzc_http_response_header_fn response_header_cb;
  void *response_header_userdata;

  const char *interface_name;
  int timeout_ms;
  int retry_count;

  uint8_t *chunk_buf;
  size_t chunk_buf_cap;

  uint8_t *response_buf;
  size_t response_buf_cap;
  const gzc_platform_t *response_platform;

  gzc_http_read_fn read_cb;
  void *userdata;
};

typedef struct {
  int status_code;
  int64_t content_length;
  gzc_buf_t body;
} gzc_http_response_t;

typedef struct {
  void *userdata;
  int (*request)(void *userdata, const gzc_http_request_t *request, gzc_http_response_t *out_response);
  void (*response_free)(void *userdata, gzc_http_response_t *response);
} gzc_http_vtable_t;

static inline int gzc_http_status_has_error(int status_code) {
  return status_code < 200 || status_code >= 400;
}

/*
 * Delivers one response header to the request's sink, if it declared one.
 *
 * Backends call this instead of the callback directly so every implementation
 * rejects the same malformed input: an empty name, or a NUL, CR, or LF
 * anywhere in the name or value. Returns GZC_OK when the request declared no
 * sink.
 */
static inline int gzc_http_deliver_response_header(
    const gzc_http_request_t *request,
    const char *name,
    size_t name_len,
    const char *value,
    size_t value_len) {
  if (request == NULL || name == NULL || name_len == 0 || (value == NULL && value_len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  for (size_t i = 0; i < name_len; i++) {
    if (name[i] == 0 || name[i] == '\r' || name[i] == '\n') {
      return GZC_ERR_INVALID_ARGUMENT;
    }
  }
  for (size_t i = 0; i < value_len; i++) {
    if (value[i] == 0 || value[i] == '\r' || value[i] == '\n') {
      return GZC_ERR_INVALID_ARGUMENT;
    }
  }
  if (request->response_header_cb == NULL) {
    return GZC_OK;
  }
  /* Built field-by-field so this header stays link-free: a caller that only
   * includes it never has to link gzc_buffer.c. */
  gzc_str_t header_name;
  gzc_str_t header_value;
  header_name.data = name;
  header_name.len = name_len;
  header_value.data = value;
  header_value.len = value_len;
  return request->response_header_cb(
      request->response_header_userdata, request, header_name, header_value);
}

#ifdef __cplusplus
}
#endif

#endif
