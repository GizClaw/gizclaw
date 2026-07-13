#include "gzc_json.h"

#include <ctype.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static const char *skip_ws(const char *p, const char *end) {
  while (p < end && isspace((unsigned char)*p)) {
    p++;
  }
  return p;
}

static int append_char(gzc_json_writer_t *writer, char ch) {
  return gzc_buf_append(writer->buf, writer->platform, &ch, 1);
}

static int append_cstr(gzc_json_writer_t *writer, const char *text) {
  return gzc_buf_append_cstr(writer->buf, writer->platform, text);
}

static int field_prefix(gzc_json_writer_t *writer, const char *name) {
  if (writer == NULL || name == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  int rc = GZC_OK;
  if (writer->need_comma) {
    rc = append_char(writer, ',');
    if (rc != GZC_OK) {
      return rc;
    }
  }
  writer->need_comma = true;
  rc = gzc_json_write_string(writer, gzc_str_from_cstr(name));
  if (rc != GZC_OK) {
    return rc;
  }
  return append_char(writer, ':');
}

void gzc_json_writer_init(gzc_json_writer_t *writer, const gzc_platform_t *platform, gzc_buf_t *buf) {
  if (writer == NULL) {
    return;
  }
  writer->buf = buf;
  writer->platform = platform == NULL ? gzc_default_platform() : platform;
  writer->need_comma = false;
}

int gzc_json_object_begin(gzc_json_writer_t *writer) {
  if (writer == NULL || writer->buf == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  writer->need_comma = false;
  return append_char(writer, '{');
}

int gzc_json_object_end(gzc_json_writer_t *writer) {
  if (writer == NULL || writer->buf == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return append_char(writer, '}');
}

int gzc_json_write_string(gzc_json_writer_t *writer, gzc_str_t value) {
  if (writer == NULL || writer->buf == NULL || (value.data == NULL && value.len != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  int rc = append_char(writer, '"');
  if (rc != GZC_OK) {
    return rc;
  }
  for (size_t i = 0; i < value.len; i++) {
    unsigned char ch = (unsigned char)value.data[i];
    switch (ch) {
    case '"':
      rc = append_cstr(writer, "\\\"");
      break;
    case '\\':
      rc = append_cstr(writer, "\\\\");
      break;
    case '\b':
      rc = append_cstr(writer, "\\b");
      break;
    case '\f':
      rc = append_cstr(writer, "\\f");
      break;
    case '\n':
      rc = append_cstr(writer, "\\n");
      break;
    case '\r':
      rc = append_cstr(writer, "\\r");
      break;
    case '\t':
      rc = append_cstr(writer, "\\t");
      break;
    default:
      if (ch < 0x20) {
        char escaped[7];
        snprintf(escaped, sizeof(escaped), "\\u%04x", ch);
        rc = append_cstr(writer, escaped);
      } else {
        rc = append_char(writer, (char)ch);
      }
      break;
    }
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return append_char(writer, '"');
}

int gzc_json_field_str(gzc_json_writer_t *writer, const char *name, gzc_str_t value) {
  int rc = field_prefix(writer, name);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_json_write_string(writer, value);
}

int gzc_json_field_i64(gzc_json_writer_t *writer, const char *name, int64_t value) {
  char buf[32];
  snprintf(buf, sizeof(buf), "%lld", (long long)value);
  int rc = field_prefix(writer, name);
  if (rc != GZC_OK) {
    return rc;
  }
  return append_cstr(writer, buf);
}

int gzc_json_field_i32(gzc_json_writer_t *writer, const char *name, int32_t value) {
  char buf[24];
  snprintf(buf, sizeof(buf), "%ld", (long)value);
  int rc = field_prefix(writer, name);
  if (rc != GZC_OK) {
    return rc;
  }
  return append_cstr(writer, buf);
}

int gzc_json_field_f64(gzc_json_writer_t *writer, const char *name, double value) {
  char buf[64];
  snprintf(buf, sizeof(buf), "%.17g", value);
  int rc = field_prefix(writer, name);
  if (rc != GZC_OK) {
    return rc;
  }
  return append_cstr(writer, buf);
}

int gzc_json_field_bool(gzc_json_writer_t *writer, const char *name, bool value) {
  int rc = field_prefix(writer, name);
  if (rc != GZC_OK) {
    return rc;
  }
  return append_cstr(writer, value ? "true" : "false");
}

int gzc_json_field_raw(gzc_json_writer_t *writer, const char *name, gzc_str_t raw_json) {
  int rc = field_prefix(writer, name);
  if (rc != GZC_OK) {
    return rc;
  }
  if (raw_json.data == NULL || raw_json.len == 0) {
    return append_cstr(writer, "null");
  }
  return gzc_buf_append_str(writer->buf, writer->platform, raw_json);
}

static const char *skip_string(const char *p, const char *end) {
  if (p >= end || *p != '"') {
    return NULL;
  }
  p++;
  while (p < end) {
    if (*p == '\\') {
      p += 2;
      continue;
    }
    if (*p == '"') {
      return p + 1;
    }
    p++;
  }
  return NULL;
}

static const char *skip_number(const char *p, const char *end) {
  if (p < end && (*p == '-')) {
    p++;
  }
  while (p < end && isdigit((unsigned char)*p)) {
    p++;
  }
  if (p < end && *p == '.') {
    p++;
    while (p < end && isdigit((unsigned char)*p)) {
      p++;
    }
  }
  if (p < end && (*p == 'e' || *p == 'E')) {
    p++;
    if (p < end && (*p == '+' || *p == '-')) {
      p++;
    }
    while (p < end && isdigit((unsigned char)*p)) {
      p++;
    }
  }
  return p;
}

static const char *skip_value(const char *p, const char *end) {
  p = skip_ws(p, end);
  if (p >= end) {
    return NULL;
  }
  if (*p == '"') {
    return skip_string(p, end);
  }
  if (*p == '{' || *p == '[') {
    int depth = 1;
    p++;
    while (p < end) {
      if (*p == '"') {
        p = skip_string(p, end);
        if (p == NULL) {
          return NULL;
        }
        continue;
      }
      if (*p == '{' || *p == '[') {
        depth++;
      } else if (*p == '}' || *p == ']') {
        depth--;
        if (depth == 0) {
          return p + 1;
        }
      }
      p++;
    }
    return NULL;
  }
  if (*p == '-' || isdigit((unsigned char)*p)) {
    return skip_number(p, end);
  }
  if ((end - p) >= 4 && memcmp(p, "true", 4) == 0) {
    return p + 4;
  }
  if ((end - p) >= 5 && memcmp(p, "false", 5) == 0) {
    return p + 5;
  }
  if ((end - p) >= 4 && memcmp(p, "null", 4) == 0) {
    return p + 4;
  }
  return NULL;
}

static const char *validate_json_value(const char *p, const char *end);

static const char *validate_json_number(const char *p, const char *end) {
  if (p < end && *p == '-') {
    p++;
  }
  if (p >= end || !isdigit((unsigned char)*p)) {
    return NULL;
  }
  while (p < end && isdigit((unsigned char)*p)) {
    p++;
  }
  if (p < end && *p == '.') {
    p++;
    if (p >= end || !isdigit((unsigned char)*p)) {
      return NULL;
    }
    while (p < end && isdigit((unsigned char)*p)) {
      p++;
    }
  }
  if (p < end && (*p == 'e' || *p == 'E')) {
    p++;
    if (p < end && (*p == '+' || *p == '-')) {
      p++;
    }
    if (p >= end || !isdigit((unsigned char)*p)) {
      return NULL;
    }
    while (p < end && isdigit((unsigned char)*p)) {
      p++;
    }
  }
  return p;
}

static const char *validate_json_array(const char *p, const char *end) {
  if (p >= end || *p != '[') {
    return NULL;
  }
  p = skip_ws(p + 1, end);
  if (p < end && *p == ']') {
    return p + 1;
  }
  while (p < end) {
    p = validate_json_value(p, end);
    if (p == NULL) {
      return NULL;
    }
    p = skip_ws(p, end);
    if (p < end && *p == ',') {
      p = skip_ws(p + 1, end);
      continue;
    }
    if (p < end && *p == ']') {
      return p + 1;
    }
    return NULL;
  }
  return NULL;
}

static const char *validate_json_object(const char *p, const char *end) {
  if (p >= end || *p != '{') {
    return NULL;
  }
  p = skip_ws(p + 1, end);
  if (p < end && *p == '}') {
    return p + 1;
  }
  while (p < end) {
    if (*p != '"') {
      return NULL;
    }
    p = skip_string(p, end);
    if (p == NULL) {
      return NULL;
    }
    p = skip_ws(p, end);
    if (p >= end || *p != ':') {
      return NULL;
    }
    p = validate_json_value(skip_ws(p + 1, end), end);
    if (p == NULL) {
      return NULL;
    }
    p = skip_ws(p, end);
    if (p < end && *p == ',') {
      p = skip_ws(p + 1, end);
      continue;
    }
    if (p < end && *p == '}') {
      return p + 1;
    }
    return NULL;
  }
  return NULL;
}

static const char *validate_json_value(const char *p, const char *end) {
  p = skip_ws(p, end);
  if (p >= end) {
    return NULL;
  }
  if (*p == '"') {
    return skip_string(p, end);
  }
  if (*p == '{') {
    return validate_json_object(p, end);
  }
  if (*p == '[') {
    return validate_json_array(p, end);
  }
  if (*p == '-' || isdigit((unsigned char)*p)) {
    return validate_json_number(p, end);
  }
  if ((end - p) >= 4 && memcmp(p, "true", 4) == 0) {
    return p + 4;
  }
  if ((end - p) >= 5 && memcmp(p, "false", 5) == 0) {
    return p + 5;
  }
  if ((end - p) >= 4 && memcmp(p, "null", 4) == 0) {
    return p + 4;
  }
  return NULL;
}

int gzc_json_find_field(gzc_str_t object_json, const char *name, gzc_str_t *out_raw) {
  if (object_json.data == NULL || name == NULL || out_raw == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const char *p = skip_ws(object_json.data, object_json.data + object_json.len);
  const char *end = object_json.data + object_json.len;
  if (p >= end || *p != '{') {
    return GZC_ERR_JSON;
  }
  p++;
  size_t name_len = strlen(name);
  while (true) {
    p = skip_ws(p, end);
    if (p >= end) {
      return GZC_ERR_JSON;
    }
    if (*p == '}') {
      return GZC_ERR_JSON;
    }
    const char *key_start = p;
    const char *key_end = skip_string(p, end);
    if (key_end == NULL) {
      return GZC_ERR_JSON;
    }
    p = skip_ws(key_end, end);
    if (p >= end || *p != ':') {
      return GZC_ERR_JSON;
    }
    p++;
    const char *value_start = skip_ws(p, end);
    const char *value_end = skip_value(value_start, end);
    if (value_start == NULL || value_end == NULL) {
      return GZC_ERR_JSON;
    }
    if ((size_t)(key_end - key_start) == name_len + 2 &&
        memcmp(key_start + 1, name, name_len) == 0) {
      out_raw->data = value_start;
      out_raw->len = (size_t)(value_end - value_start);
      return GZC_OK;
    }
    p = skip_ws(value_end, end);
    if (p < end && *p == ',') {
      p++;
      continue;
    }
    if (p < end && *p == '}') {
      return GZC_ERR_JSON;
    }
    return GZC_ERR_JSON;
  }
}

int gzc_json_validate_object(gzc_str_t object_json) {
  if (object_json.data == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const char *p = skip_ws(object_json.data, object_json.data + object_json.len);
  const char *end = object_json.data + object_json.len;
  if (p >= end || *p != '{') {
    return GZC_ERR_JSON;
  }
  const char *value_end = validate_json_object(p, end);
  if (value_end == NULL) {
    return GZC_ERR_JSON;
  }
  value_end = skip_ws(value_end, end);
  return value_end == end ? GZC_OK : GZC_ERR_JSON;
}

int gzc_json_array_iter_init(gzc_str_t array_json, gzc_json_array_iter_t *out_iter) {
  if (array_json.data == NULL || out_iter == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const char *begin = skip_ws(array_json.data, array_json.data + array_json.len);
  const char *end = array_json.data + array_json.len;
  if (begin >= end || *begin != '[') {
    return GZC_ERR_JSON;
  }
  const char *value_end = validate_json_array(begin, end);
  if (value_end == NULL || skip_ws(value_end, end) != end) {
    return GZC_ERR_JSON;
  }
  out_iter->raw = array_json;
  out_iter->offset = (size_t)(begin + 1 - array_json.data);
  out_iter->finished = false;
  return GZC_OK;
}

int gzc_json_array_iter_next(gzc_json_array_iter_t *iter, gzc_str_t *out_raw, bool *out_has_value) {
  if (iter == NULL || out_raw == NULL || out_has_value == NULL || iter->raw.data == NULL ||
      iter->offset > iter->raw.len) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (iter->finished) {
    *out_has_value = false;
    return GZC_OK;
  }
  const char *end = iter->raw.data + iter->raw.len;
  const char *value_start = skip_ws(iter->raw.data + iter->offset, end);
  if (value_start >= end) {
    return GZC_ERR_JSON;
  }
  if (*value_start == ']') {
    iter->finished = true;
    *out_has_value = false;
    return GZC_OK;
  }
  const char *value_end = validate_json_value(value_start, end);
  if (value_end == NULL) {
    return GZC_ERR_JSON;
  }
  const char *next = skip_ws(value_end, end);
  if (next >= end) {
    return GZC_ERR_JSON;
  }
  if (*next == ',') {
    iter->offset = (size_t)(next + 1 - iter->raw.data);
  } else if (*next == ']') {
    iter->offset = (size_t)(next - iter->raw.data);
  } else {
    return GZC_ERR_JSON;
  }
  out_raw->data = value_start;
  out_raw->len = (size_t)(value_end - value_start);
  *out_has_value = true;
  return GZC_OK;
}

int gzc_json_parse_string(gzc_str_t raw_json, gzc_str_t *out) {
  if (raw_json.data == NULL || out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (raw_json.len < 2 || raw_json.data[0] != '"' || raw_json.data[raw_json.len - 1] != '"') {
    return GZC_ERR_JSON;
  }
  for (size_t i = 1; i + 1 < raw_json.len; i++) {
    if (raw_json.data[i] == '\\') {
      return GZC_ERR_UNSUPPORTED;
    }
  }
  out->data = raw_json.data + 1;
  out->len = raw_json.len - 2;
  return GZC_OK;
}

int gzc_json_parse_i64(gzc_str_t raw_json, int64_t *out) {
  if (raw_json.data == NULL || out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (raw_json.len == 0) {
    return GZC_ERR_JSON;
  }
  size_t i = 0;
  bool neg = false;
  if (raw_json.data[i] == '-') {
    neg = true;
    i++;
  }
  if (i == raw_json.len || !isdigit((unsigned char)raw_json.data[i])) {
    return GZC_ERR_JSON;
  }
  int64_t value = 0;
  for (; i < raw_json.len; i++) {
    if (!isdigit((unsigned char)raw_json.data[i])) {
      return GZC_ERR_JSON;
    }
    int64_t digit = (int64_t)(raw_json.data[i] - '0');
    if (value > (INT64_MAX - digit) / 10) {
      return GZC_ERR_JSON;
    }
    value = value * 10 + digit;
  }
  *out = neg ? -value : value;
  return GZC_OK;
}

int gzc_json_parse_i32(gzc_str_t raw_json, int32_t *out) {
  int64_t value = 0;
  int rc = gzc_json_parse_i64(raw_json, &value);
  if (rc != GZC_OK) {
    return rc;
  }
  if (value < INT32_MIN || value > INT32_MAX) {
    return GZC_ERR_JSON;
  }
  *out = (int32_t)value;
  return GZC_OK;
}

int gzc_json_parse_f64(gzc_str_t raw_json, double *out) {
  if (raw_json.data == NULL || out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (raw_json.len >= 128) {
    return GZC_ERR_UNSUPPORTED;
  }
  char buf[128];
  memcpy(buf, raw_json.data, raw_json.len);
  buf[raw_json.len] = 0;
  char *endptr = NULL;
  double value = strtod(buf, &endptr);
  if (endptr != buf + raw_json.len) {
    return GZC_ERR_JSON;
  }
  *out = value;
  return GZC_OK;
}

int gzc_json_parse_bool(gzc_str_t raw_json, bool *out) {
  if (raw_json.data == NULL || out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (raw_json.len == 4 && memcmp(raw_json.data, "true", 4) == 0) {
    *out = true;
    return GZC_OK;
  }
  if (raw_json.len == 5 && memcmp(raw_json.data, "false", 5) == 0) {
    *out = false;
    return GZC_OK;
  }
  return GZC_ERR_JSON;
}
