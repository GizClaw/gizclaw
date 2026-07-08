#ifndef GZC_SIGNALING_H
#define GZC_SIGNALING_H

#include "gzc_keys.h"
#include "platform/gzc_platform_crypto.h"
#include "platform/gzc_platform_http.h"

#ifdef __cplusplus
extern "C" {
#endif

#define GZC_SIGNALING_PATH "/webrtc/v1/offer"
#define GZC_SIGNALING_NONCE_SIZE 16
#define GZC_SIGNALING_AEAD_NONCE_SIZE 12
#define GZC_SIGNALING_HEADER_COUNT 4

typedef struct {
  const gzc_platform_t *platform;
  const gzc_platform_crypto_t *crypto;
  gzc_cipher_mode_t cipher_mode;
  gzc_str_t signaling_url;
  gzc_key_t private_key;
  gzc_public_key_t remote_public_key;
} gzc_signaling_config_t;

typedef struct {
  gzc_http_header_t headers[GZC_SIGNALING_HEADER_COUNT];
  char content_type[25];
  char public_key_text[GZC_KEY_TEXT_CAP];
  char timestamp_text[24];
  char nonce_text[24];
  uint8_t response_key[32];
  uint8_t response_nonce[GZC_SIGNALING_AEAD_NONCE_SIZE];
  gzc_cipher_mode_t cipher_mode;
  int64_t timestamp;
  gzc_buf_t body;
} gzc_signaling_exchange_t;

void gzc_signaling_exchange_init(gzc_signaling_exchange_t *exchange);
void gzc_signaling_exchange_free(gzc_signaling_exchange_t *exchange, const gzc_platform_t *platform);
int gzc_signaling_build_offer_request(
    const gzc_signaling_config_t *config,
    gzc_str_t offer_sdp,
    gzc_signaling_exchange_t *exchange,
    gzc_http_request_t *out_request);
int gzc_signaling_parse_answer_response(
    const gzc_signaling_config_t *config,
    const gzc_signaling_exchange_t *exchange,
    const gzc_http_response_t *response,
    gzc_buf_t *out_answer_sdp);

#ifdef __cplusplus
}
#endif

#endif
