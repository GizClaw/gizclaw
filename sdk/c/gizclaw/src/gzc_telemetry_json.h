#ifndef GZC_TELEMETRY_JSON_H
#define GZC_TELEMETRY_JSON_H

#include "gzc_common.h"

// Internal, allocation-free JSON grammar validation for bounded KV values.
static bool telemetry_utf8(gzc_str_t value) {
  if (value.data == NULL)
    return false;
  const unsigned char *p = (const unsigned char *)value.data;
  size_t i = 0;
  while (i < value.len) {
    unsigned char ch = p[i++];
    if (ch < 0x80)
      continue;
    unsigned count;
    uint32_t scalar, minimum;
    if (ch >= 0xc2 && ch <= 0xdf) {
      count = 1;
      scalar = ch & 0x1f;
      minimum = 0x80;
    } else if (ch >= 0xe0 && ch <= 0xef) {
      count = 2;
      scalar = ch & 0x0f;
      minimum = 0x800;
    } else if (ch >= 0xf0 && ch <= 0xf4) {
      count = 3;
      scalar = ch & 7;
      minimum = 0x10000;
    } else
      return false;
    if (value.len - i < count)
      return false;
    while (count-- > 0) {
      ch = p[i++];
      if ((ch & 0xc0) != 0x80)
        return false;
      scalar = (scalar << 6) | (ch & 0x3f);
    }
    if (scalar < minimum || scalar > 0x10ffff || (scalar >= 0xd800 && scalar <= 0xdfff))
      return false;
  }
  return true;
}

static bool telemetry_json_string(const char **cursor, const char *end) {
  const char *p = *cursor;
  if (p == end || *p++ != '"')
    return false;
  while (p < end) {
    unsigned char ch = (unsigned char)*p++;
    if (ch == '"') {
      *cursor = p;
      return true;
    }
    if (ch < 0x20)
      return false;
    if (ch == '\\') {
      if (p == end)
        return false;
      ch = (unsigned char)*p++;
      if (ch == 'u') {
        for (unsigned i = 0; i < 4; i++) {
          if (p == end)
            return false;
          ch = (unsigned char)*p++;
          if (!((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')))
            return false;
        }
      } else if (ch != '"' && ch != '\\' && ch != '/' && ch != 'b' && ch != 'f' && ch != 'n' && ch != 'r' && ch != 't')
        return false;
    }
  }
  return false;
}

static bool telemetry_json_number(const char **cursor, const char *end) {
  const char *p = *cursor;
  if (p < end && *p == '-')
    p++;
  if (p == end)
    return false;
  if (*p == '0')
    p++;
  else {
    if (*p < '1' || *p > '9')
      return false;
    do {
      p++;
    } while (p < end && *p >= '0' && *p <= '9');
  }
  if (p < end && *p == '.') {
    p++;
    if (p == end || *p < '0' || *p > '9')
      return false;
    do {
      p++;
    } while (p < end && *p >= '0' && *p <= '9');
  }
  if (p < end && (*p == 'e' || *p == 'E')) {
    p++;
    if (p < end && (*p == '+' || *p == '-'))
      p++;
    if (p == end || *p < '0' || *p > '9')
      return false;
    do {
      p++;
    } while (p < end && *p >= '0' && *p <= '9');
  }
  *cursor = p;
  return true;
}

static bool telemetry_json(gzc_str_t value) {
  if (value.len == 0 || value.len > GZC_KV_VALUE_MAX_BYTES || !telemetry_utf8(value))
    return false;
  // Valid bounded JSON fits this stack. Malformed open containers are also
  // checked before every push. No recursion or heap.
  char states[GZC_KV_VALUE_MAX_BYTES + 1];
  int top = 0;
  states[0] = 'v';
  const char *p = value.data, *end = p + value.len;
  while (top >= 0) {
    while (p < end && (*p == ' ' || *p == '\t' || *p == '\r' || *p == '\n'))
      p++;
    char state = states[top];
    if (state == 'd') {
      top--;
      continue;
    }
    if (p == end)
      return false;
    if (state == 'a' || state == 'b') {
      if (state == 'a' && *p == ']') {
        p++;
        top--;
        continue;
      }
      states[top] = 'A';
      if ((size_t)(top + 1) >= sizeof(states))
        return false;
      states[++top] = 'v';
      continue;
    }
    if (state == 'o' || state == 'k') {
      if (state == 'o' && *p == '}') {
        p++;
        top--;
        continue;
      }
      if (!telemetry_json_string(&p, end))
        return false;
      states[top] = ':';
      continue;
    }
    if (state == ':') {
      if (*p++ != ':')
        return false;
      states[top] = 'O';
      if ((size_t)(top + 1) >= sizeof(states))
        return false;
      states[++top] = 'v';
      continue;
    }
    if (state == 'A' || state == 'O') {
      if (*p == ',') {
        p++;
        states[top] = state == 'A' ? 'b' : 'k';
        continue;
      }
      if (*p == (state == 'A' ? ']' : '}')) {
        p++;
        top--;
        continue;
      }
      return false;
    }
    states[top] = 'd';
    if (*p == '[' || *p == '{') {
      if ((size_t)(top + 1) >= sizeof(states))
        return false;
      states[++top] = *p++ == '[' ? 'a' : 'o';
      continue;
    }
    if (*p == '"') {
      if (!telemetry_json_string(&p, end))
        return false;
      continue;
    }
    if (*p == '-' || (*p >= '0' && *p <= '9')) {
      if (!telemetry_json_number(&p, end))
        return false;
      continue;
    }
    if (end - p >= 4 && (memcmp(p, "true", 4) == 0 || memcmp(p, "null", 4) == 0)) {
      p += 4;
      continue;
    }
    if (end - p >= 5 && memcmp(p, "false", 5) == 0) {
      p += 5;
      continue;
    }
    return false;
  }
  return p == end;
}

#endif
