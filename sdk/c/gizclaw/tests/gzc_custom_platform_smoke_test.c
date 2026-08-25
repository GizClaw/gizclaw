#include "gzc_platform.h"

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

static void *test_malloc(void *userdata, size_t size) {
  (void)userdata;
  return malloc(size);
}

static void *test_realloc(void *userdata, void *ptr, size_t size) {
  (void)userdata;
  return realloc(ptr, size);
}

static void test_free(void *userdata, void *ptr) {
  (void)userdata;
  free(ptr);
}

static int64_t test_time(void *userdata) {
  (void)userdata;
  return 1;
}

static int test_random(void *userdata, uint8_t *out, size_t len) {
  (void)userdata;
  for (size_t i = 0; i < len; i++) {
    out[i] = (uint8_t)i;
  }
  return GZC_OK;
}

static void test_log(void *userdata, gzc_log_level_t level, gzc_str_t message) {
  (void)userdata;
  (void)level;
  (void)message;
}

const gzc_platform_t *gzc_default_platform(void) {
  static const gzc_platform_t platform = {
      NULL,
      test_malloc,
      test_realloc,
      test_free,
      test_time,
      test_time,
      test_random,
      test_log,
  };
  return &platform;
}

int main(void) {
  const gzc_platform_t *platform = gzc_default_platform();
  if (platform == NULL || platform->malloc == NULL || platform->free == NULL) {
    fputs("custom platform is unavailable\n", stderr);
    return 1;
  }
  void *allocation = platform->malloc(platform->userdata, 16u);
  if (allocation == NULL) {
    fputs("custom platform allocation failed\n", stderr);
    return 1;
  }
  platform->free(platform->userdata, allocation);
  return 0;
}
