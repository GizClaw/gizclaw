#ifndef GZC_HTTP_H
#define GZC_HTTP_H

#include "gzc_platform.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
  gzc_str_t name;
  gzc_str_t value;
} gzc_http_header_t;

typedef struct {
  gzc_str_t url;
  const gzc_http_header_t *headers;
  size_t header_count;
  const uint8_t *body;
  size_t body_len;
  int timeout_ms;
} gzc_http_request_t;

typedef struct {
  int status_code;
  gzc_buf_t body;
} gzc_http_response_t;

typedef struct {
  void *userdata;
  int (*post)(void *userdata, const gzc_http_request_t *request, gzc_http_response_t *out_response);
  void (*response_free)(void *userdata, gzc_http_response_t *response);
} gzc_http_vtable_t;

#ifdef __cplusplus
}
#endif

#endif
