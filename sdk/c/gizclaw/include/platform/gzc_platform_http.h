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

#ifdef __cplusplus
}
#endif

#endif
