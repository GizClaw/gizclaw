#ifndef GZC_JSON_H
#define GZC_JSON_H

#include "gzc_platform.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
  gzc_str_t raw;
} gzc_json_t;

typedef struct {
  gzc_buf_t *buf;
  const gzc_platform_t *platform;
  bool need_comma;
} gzc_json_writer_t;

void gzc_json_writer_init(gzc_json_writer_t *writer, const gzc_platform_t *platform, gzc_buf_t *buf);
int gzc_json_object_begin(gzc_json_writer_t *writer);
int gzc_json_object_end(gzc_json_writer_t *writer);
int gzc_json_field_str(gzc_json_writer_t *writer, const char *name, gzc_str_t value);
int gzc_json_field_i64(gzc_json_writer_t *writer, const char *name, int64_t value);
int gzc_json_field_i32(gzc_json_writer_t *writer, const char *name, int32_t value);
int gzc_json_field_f64(gzc_json_writer_t *writer, const char *name, double value);
int gzc_json_field_bool(gzc_json_writer_t *writer, const char *name, bool value);
int gzc_json_field_raw(gzc_json_writer_t *writer, const char *name, gzc_str_t raw_json);
int gzc_json_write_string(gzc_json_writer_t *writer, gzc_str_t value);

int gzc_json_find_field(gzc_str_t object_json, const char *name, gzc_str_t *out_raw);
int gzc_json_validate_object(gzc_str_t object_json);
int gzc_json_parse_string(gzc_str_t raw_json, gzc_str_t *out);
int gzc_json_parse_i64(gzc_str_t raw_json, int64_t *out);
int gzc_json_parse_i32(gzc_str_t raw_json, int32_t *out);
int gzc_json_parse_f64(gzc_str_t raw_json, double *out);
int gzc_json_parse_bool(gzc_str_t raw_json, bool *out);

#ifdef __cplusplus
}
#endif

#endif
