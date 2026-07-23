#ifndef GZC_PLATFORM_H
#define GZC_PLATFORM_H

#include "gzc_buffer.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
  GZC_LOG_DEBUG = 0,
  GZC_LOG_INFO = 1,
  GZC_LOG_WARN = 2,
  GZC_LOG_ERROR = 3
} gzc_log_level_t;

typedef struct gzc_platform {
  void *userdata;
  void *(*malloc)(void *userdata, size_t size);
  void *(*realloc)(void *userdata, void *ptr, size_t size);
  void (*free)(void *userdata, void *ptr);
  /* Monotonic milliseconds with an arbitrary epoch, used for elapsed time. */
  int64_t (*time_instant_ms)(void *userdata);
  /* Unix milliseconds, used only for protocol timestamps and telemetry. */
  int64_t (*time_unix_ms)(void *userdata);
  int (*random)(void *userdata, uint8_t *out, size_t len);
  void (*log)(void *userdata, gzc_log_level_t level, gzc_str_t message);
} gzc_platform_t;

const gzc_platform_t *gzc_default_platform(void);

#ifdef __cplusplus
}
#endif

#endif
