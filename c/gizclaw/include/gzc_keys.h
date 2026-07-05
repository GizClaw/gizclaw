#ifndef GZC_KEYS_H
#define GZC_KEYS_H

#include "gzc_buffer.h"

#ifdef __cplusplus
extern "C" {
#endif

#define GZC_KEY_SIZE 32
#define GZC_KEY_TEXT_CAP 65

typedef struct {
  uint8_t bytes[GZC_KEY_SIZE];
} gzc_key_t;

typedef gzc_key_t gzc_public_key_t;

typedef struct {
  gzc_key_t private_key;
  gzc_public_key_t public_key;
} gzc_keypair_t;

int gzc_key_from_bytes(const uint8_t *bytes, size_t len, gzc_key_t *out_key);
int gzc_key_to_bytes(const gzc_key_t *key, uint8_t out_bytes[GZC_KEY_SIZE]);
int gzc_key_from_text(gzc_str_t text, gzc_key_t *out_key);
int gzc_key_to_text(const gzc_key_t *key, char *out_text, size_t out_text_cap, size_t *out_text_len);
int gzc_key_is_zero(const gzc_key_t *key);

#ifdef __cplusplus
}
#endif

#endif
