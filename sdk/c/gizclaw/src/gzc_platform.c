#ifndef _POSIX_C_SOURCE
#define _POSIX_C_SOURCE 200809L
#endif

#include "gzc_platform.h"

#include <stdlib.h>
#include <time.h>

static void *gzc_default_malloc(void *userdata, size_t size) {
  (void)userdata;
  return malloc(size);
}

static void *gzc_default_realloc(void *userdata, void *ptr, size_t size) {
  (void)userdata;
  return realloc(ptr, size);
}

static void gzc_default_free(void *userdata, void *ptr) {
  (void)userdata;
  free(ptr);
}

static int64_t gzc_default_time_unix_ms(void *userdata) {
  (void)userdata;
  return (int64_t)time(NULL) * 1000;
}

static int64_t gzc_default_time_instant_ms(void *userdata) {
  (void)userdata;
  struct timespec now;
  if (clock_gettime(CLOCK_MONOTONIC, &now) != 0) {
    if (timespec_get(&now, TIME_UTC) != TIME_UTC) {
      return (int64_t)time(NULL) * 1000;
    }
  }
  return (int64_t)now.tv_sec * 1000 + now.tv_nsec / 1000000;
}

static int gzc_default_random(void *userdata, uint8_t *out, size_t len) {
  (void)userdata;
  if (out == NULL && len != 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  for (size_t i = 0; i < len; i++) {
    out[i] = (uint8_t)(rand() & 0xff);
  }
  return GZC_OK;
}

static void gzc_default_log(void *userdata, gzc_log_level_t level, gzc_str_t message) {
  (void)userdata;
  (void)level;
  (void)message;
}

const gzc_platform_t *gzc_default_platform(void) {
  static const gzc_platform_t platform = {
      NULL,
      gzc_default_malloc,
      gzc_default_realloc,
      gzc_default_free,
      gzc_default_time_instant_ms,
      gzc_default_time_unix_ms,
      gzc_default_random,
      gzc_default_log,
  };
  return &platform;
}
