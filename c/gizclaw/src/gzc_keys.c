#include "gzc_keys.h"

#include <string.h>

int gzc_key_from_bytes(const uint8_t *bytes, size_t len, gzc_key_t *out_key) {
  if (bytes == NULL || out_key == NULL || len != GZC_KEY_SIZE) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memcpy(out_key->bytes, bytes, GZC_KEY_SIZE);
  return GZC_OK;
}

int gzc_key_to_bytes(const gzc_key_t *key, uint8_t out_bytes[GZC_KEY_SIZE]) {
  if (key == NULL || out_bytes == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memcpy(out_bytes, key->bytes, GZC_KEY_SIZE);
  return GZC_OK;
}

int gzc_key_is_zero(const gzc_key_t *key) {
  if (key == NULL) {
    return 1;
  }
  uint8_t result = 0;
  for (size_t i = 0; i < GZC_KEY_SIZE; i++) {
    result |= key->bytes[i];
  }
  return result == 0;
}
