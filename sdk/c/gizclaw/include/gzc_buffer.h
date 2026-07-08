#ifndef GZC_BUFFER_H
#define GZC_BUFFER_H

#include "gzc_common.h"

#ifdef __cplusplus
extern "C" {
#endif

struct gzc_platform;

typedef struct {
  const char *data;
  size_t len;
} gzc_str_t;

typedef struct {
  uint8_t *data;
  size_t len;
  size_t cap;
} gzc_buf_t;

gzc_str_t gzc_str_from_cstr(const char *text);
gzc_str_t gzc_str_from_parts(const char *data, size_t len);

void gzc_buf_init(gzc_buf_t *buf);
void gzc_buf_reset(gzc_buf_t *buf);
void gzc_buf_free(gzc_buf_t *buf, const struct gzc_platform *platform);
int gzc_buf_reserve(gzc_buf_t *buf, const struct gzc_platform *platform, size_t cap);
int gzc_buf_append(gzc_buf_t *buf, const struct gzc_platform *platform, const void *data, size_t len);
int gzc_buf_append_str(gzc_buf_t *buf, const struct gzc_platform *platform, gzc_str_t text);
int gzc_buf_append_cstr(gzc_buf_t *buf, const struct gzc_platform *platform, const char *text);

#ifdef __cplusplus
}
#endif

#endif
