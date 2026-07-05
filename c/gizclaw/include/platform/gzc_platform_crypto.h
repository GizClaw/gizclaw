#ifndef GZC_PLATFORM_CRYPTO_H
#define GZC_PLATFORM_CRYPTO_H

#include "gzc_buffer.h"
#include "gzc_keys.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
  GZC_CIPHER_CHACHA20_POLY1305 = 1,
  GZC_CIPHER_AES_256_GCM = 2,
  GZC_CIPHER_PLAINTEXT = 3
} gzc_cipher_mode_t;

typedef struct {
  void *userdata;
  int (*keypair_from_private)(void *userdata, const gzc_key_t *private_key, gzc_keypair_t *out_keypair);
  int (*key_from_text)(void *userdata, gzc_str_t text, gzc_key_t *out_key);
  int (*key_to_text)(void *userdata, const gzc_key_t *key, char *out_text, size_t out_text_cap, size_t *out_text_len);
  int (*dh)(void *userdata, const gzc_keypair_t *local, const gzc_public_key_t *remote, gzc_key_t *out_shared);
  int (*hkdf_sha256)(
      void *userdata,
      const uint8_t *secret,
      size_t secret_len,
      const uint8_t *salt,
      size_t salt_len,
      gzc_str_t info,
      uint8_t *out,
      size_t out_len);
  int (*aead_seal)(
      void *userdata,
      gzc_cipher_mode_t mode,
      const uint8_t *key,
      size_t key_len,
      const uint8_t *nonce,
      size_t nonce_len,
      const uint8_t *plaintext,
      size_t plaintext_len,
      const uint8_t *aad,
      size_t aad_len,
      gzc_buf_t *out_ciphertext);
  int (*aead_open)(
      void *userdata,
      gzc_cipher_mode_t mode,
      const uint8_t *key,
      size_t key_len,
      const uint8_t *nonce,
      size_t nonce_len,
      const uint8_t *ciphertext,
      size_t ciphertext_len,
      const uint8_t *aad,
      size_t aad_len,
      gzc_buf_t *out_plaintext);
} gzc_platform_crypto_t;

#ifdef __cplusplus
}
#endif

#endif
