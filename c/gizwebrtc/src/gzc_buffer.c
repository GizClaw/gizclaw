#include "gzc_buffer.h"

#include "gzc_platform.h"

#include <limits.h>
#include <string.h>

gzc_str_t gzc_str_from_cstr(const char *text) {
  gzc_str_t out;
  out.data = text;
  out.len = text == NULL ? 0 : strlen(text);
  return out;
}

gzc_str_t gzc_str_from_parts(const char *data, size_t len) {
  gzc_str_t out;
  out.data = data;
  out.len = len;
  return out;
}

void gzc_buf_init(gzc_buf_t *buf) {
  if (buf == NULL) {
    return;
  }
  buf->data = NULL;
  buf->len = 0;
  buf->cap = 0;
}

void gzc_buf_reset(gzc_buf_t *buf) {
  if (buf != NULL) {
    buf->len = 0;
  }
}

void gzc_buf_free(gzc_buf_t *buf, const struct gzc_platform *platform) {
  if (buf == NULL || buf->data == NULL) {
    return;
  }
  if (platform != NULL && platform->free != NULL) {
    platform->free(platform->userdata, buf->data);
  }
  buf->data = NULL;
  buf->len = 0;
  buf->cap = 0;
}

int gzc_buf_reserve(gzc_buf_t *buf, const struct gzc_platform *platform, size_t cap) {
  if (buf == NULL || platform == NULL || platform->realloc == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (cap <= buf->cap) {
    return GZC_OK;
  }
  size_t next = buf->cap == 0 ? 64 : buf->cap;
  while (next < cap) {
    if (next > ((size_t)-1) / 2) {
      next = cap;
      break;
    }
    next *= 2;
  }
  void *data = platform->realloc(platform->userdata, buf->data, next);
  if (data == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  buf->data = (uint8_t *)data;
  buf->cap = next;
  return GZC_OK;
}

int gzc_buf_append(gzc_buf_t *buf, const struct gzc_platform *platform, const void *data, size_t len) {
  if (buf == NULL || platform == NULL || (data == NULL && len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (len > ((size_t)-1) - buf->len) {
    return GZC_ERR_NO_MEMORY;
  }
  int rc = gzc_buf_reserve(buf, platform, buf->len + len + 1);
  if (rc != GZC_OK) {
    return rc;
  }
  if (len != 0) {
    memcpy(buf->data + buf->len, data, len);
  }
  buf->len += len;
  buf->data[buf->len] = 0;
  return GZC_OK;
}

int gzc_buf_append_str(gzc_buf_t *buf, const struct gzc_platform *platform, gzc_str_t text) {
  return gzc_buf_append(buf, platform, text.data, text.len);
}

int gzc_buf_append_cstr(gzc_buf_t *buf, const struct gzc_platform *platform, const char *text) {
  return gzc_buf_append_str(buf, platform, gzc_str_from_cstr(text));
}
