#include "gzc_signaling.h"

#include <stdio.h>
#include <string.h>

static const char gzc_b64url[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

static int append_str(gzc_buf_t *buf, const gzc_platform_t *platform, const char *text) {
  return gzc_buf_append(buf, platform, text, strlen(text));
}

static int append_int64(gzc_buf_t *buf, const gzc_platform_t *platform, int64_t value) {
  char text[24];
  int len = snprintf(text, sizeof(text), "%lld", (long long)value);
  if (len <= 0 || (size_t)len >= sizeof(text)) {
    return GZC_ERR_SIGNALING;
  }
  return gzc_buf_append(buf, platform, text, (size_t)len);
}

static int base64url_no_pad(const uint8_t *data, size_t len, char *out, size_t out_cap, size_t *out_len) {
  if ((data == NULL && len != 0) || out == NULL || out_cap == 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  size_t pos = 0;
  for (size_t i = 0; i < len; i += 3) {
    uint32_t chunk = ((uint32_t)data[i]) << 16;
    size_t remain = len - i;
    if (remain > 1) {
      chunk |= ((uint32_t)data[i + 1]) << 8;
    }
    if (remain > 2) {
      chunk |= (uint32_t)data[i + 2];
    }
    size_t chars = remain >= 3 ? 4 : remain + 1;
    for (size_t j = 0; j < chars; j++) {
      if (pos + 1 >= out_cap) {
        return GZC_ERR_NO_MEMORY;
      }
      out[pos++] = gzc_b64url[(chunk >> (18 - 6 * j)) & 0x3fu];
    }
  }
  if (pos >= out_cap) {
    return GZC_ERR_NO_MEMORY;
  }
  out[pos] = '\0';
  if (out_len != NULL) {
    *out_len = pos;
  }
  return GZC_OK;
}

static int build_aad(
    const gzc_platform_t *platform,
    const char *public_key_text,
    int64_t timestamp,
    const char *nonce_text,
    bool response,
    gzc_buf_t *out) {
  gzc_buf_reset(out);
  int rc = append_str(out, platform, "POST\n" GZC_SIGNALING_PATH "\n");
  if (rc == GZC_OK) {
    rc = append_str(out, platform, public_key_text);
  }
  if (rc == GZC_OK) {
    rc = append_str(out, platform, "\n");
  }
  if (rc == GZC_OK) {
    rc = append_int64(out, platform, timestamp);
  }
  if (rc == GZC_OK) {
    rc = append_str(out, platform, "\n");
  }
  if (rc == GZC_OK) {
    rc = append_str(out, platform, nonce_text);
  }
  if (rc == GZC_OK && response) {
    rc = append_str(out, platform, "\nanswer");
  }
  return rc;
}

static int derive_signaling(
    const gzc_signaling_config_t *config,
    const uint8_t nonce_raw[GZC_SIGNALING_NONCE_SIZE],
    int64_t timestamp,
    uint8_t request_key[32],
    uint8_t request_nonce[GZC_SIGNALING_AEAD_NONCE_SIZE],
    uint8_t response_key[32],
    uint8_t response_nonce[GZC_SIGNALING_AEAD_NONCE_SIZE],
    gzc_keypair_t *out_keypair) {
  const gzc_platform_crypto_t *crypto = config->crypto;
  gzc_keypair_t keypair;
  int rc = crypto->keypair_from_private(crypto->userdata, &config->private_key, &keypair);
  if (rc != GZC_OK) {
    return rc;
  }
  gzc_key_t shared;
  rc = crypto->dh(crypto->userdata, &keypair, &config->remote_public_key, &shared);
  if (rc != GZC_OK) {
    return rc;
  }

  uint8_t salt[GZC_SIGNALING_NONCE_SIZE + 24];
  memcpy(salt, nonce_raw, GZC_SIGNALING_NONCE_SIZE);
  int ts_len = snprintf((char *)salt + GZC_SIGNALING_NONCE_SIZE, sizeof(salt) - GZC_SIGNALING_NONCE_SIZE, "%lld", (long long)timestamp);
  if (ts_len <= 0 || (size_t)ts_len >= sizeof(salt) - GZC_SIGNALING_NONCE_SIZE) {
    return GZC_ERR_SIGNALING;
  }
  size_t salt_len = GZC_SIGNALING_NONCE_SIZE + (size_t)ts_len;

  rc = crypto->hkdf_sha256(
      crypto->userdata,
      shared.bytes,
      GZC_KEY_SIZE,
      salt,
      salt_len,
      gzc_str_from_cstr("giznet/gizwebrtc/http-signaling/v1 c2s"),
      request_key,
      32);
  if (rc == GZC_OK) {
    rc = crypto->hkdf_sha256(
        crypto->userdata,
        shared.bytes,
        GZC_KEY_SIZE,
        salt,
        salt_len,
        gzc_str_from_cstr("giznet/gizwebrtc/http-signaling/v1 s2c"),
        response_key,
        32);
  }
  if (rc == GZC_OK) {
    rc = crypto->hkdf_sha256(
        crypto->userdata,
        shared.bytes,
        GZC_KEY_SIZE,
        salt,
        salt_len,
        gzc_str_from_cstr("giznet/gizwebrtc/http-signaling/v1 c2s nonce"),
        request_nonce,
        GZC_SIGNALING_AEAD_NONCE_SIZE);
  }
  if (rc == GZC_OK) {
    rc = crypto->hkdf_sha256(
        crypto->userdata,
        shared.bytes,
        GZC_KEY_SIZE,
        salt,
        salt_len,
        gzc_str_from_cstr("giznet/gizwebrtc/http-signaling/v1 s2c nonce"),
        response_nonce,
        GZC_SIGNALING_AEAD_NONCE_SIZE);
  }
  if (rc == GZC_OK && out_keypair != NULL) {
    *out_keypair = keypair;
  }
  return rc;
}

void gzc_signaling_exchange_init(gzc_signaling_exchange_t *exchange) {
  if (exchange == NULL) {
    return;
  }
  memset(exchange, 0, sizeof(*exchange));
  gzc_buf_init(&exchange->body);
}

void gzc_signaling_exchange_free(gzc_signaling_exchange_t *exchange, const gzc_platform_t *platform) {
  if (exchange == NULL) {
    return;
  }
  gzc_buf_free(&exchange->body, platform);
  memset(exchange, 0, sizeof(*exchange));
}

int gzc_signaling_build_offer_request(
    const gzc_signaling_config_t *config,
    gzc_str_t offer_sdp,
    gzc_signaling_exchange_t *exchange,
    gzc_http_request_t *out_request) {
  if (config == NULL || config->platform == NULL || config->crypto == NULL || exchange == NULL ||
      out_request == NULL || config->signaling_url.data == NULL || offer_sdp.data == NULL ||
      config->crypto->keypair_from_private == NULL || config->crypto->key_to_text == NULL ||
      config->crypto->dh == NULL || config->crypto->hkdf_sha256 == NULL ||
      config->crypto->aead_seal == NULL || config->platform->random == NULL ||
      config->platform->time_unix_ms == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (gzc_key_is_zero(&config->private_key) || gzc_key_is_zero(&config->remote_public_key)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }

  gzc_signaling_exchange_init(exchange);
  exchange->cipher_mode = config->cipher_mode == 0 ? GZC_CIPHER_CHACHA20_POLY1305 : config->cipher_mode;
  exchange->timestamp = config->platform->time_unix_ms(config->platform->userdata) / 1000;

  uint8_t nonce_raw[GZC_SIGNALING_NONCE_SIZE];
  int rc = config->platform->random(config->platform->userdata, nonce_raw, sizeof(nonce_raw));
  if (rc != GZC_OK) {
    return rc;
  }
  size_t nonce_text_len = 0;
  rc = base64url_no_pad(nonce_raw, sizeof(nonce_raw), exchange->nonce_text, sizeof(exchange->nonce_text), &nonce_text_len);
  if (rc != GZC_OK) {
    return rc;
  }
  int ts_len = snprintf(exchange->timestamp_text, sizeof(exchange->timestamp_text), "%lld", (long long)exchange->timestamp);
  if (ts_len <= 0 || (size_t)ts_len >= sizeof(exchange->timestamp_text)) {
    return GZC_ERR_SIGNALING;
  }

  uint8_t request_key[32];
  uint8_t request_nonce[GZC_SIGNALING_AEAD_NONCE_SIZE];
  gzc_keypair_t keypair;
  rc = derive_signaling(
      config,
      nonce_raw,
      exchange->timestamp,
      request_key,
      request_nonce,
      exchange->response_key,
      exchange->response_nonce,
      &keypair);
  if (rc != GZC_OK) {
    return rc;
  }
  size_t public_key_text_len = 0;
  rc = config->crypto->key_to_text(
      config->crypto->userdata,
      &keypair.public_key,
      exchange->public_key_text,
      sizeof(exchange->public_key_text),
      &public_key_text_len);
  if (rc != GZC_OK) {
    return rc;
  }

  gzc_buf_t aad;
  gzc_buf_init(&aad);
  rc = build_aad(config->platform, exchange->public_key_text, exchange->timestamp, exchange->nonce_text, false, &aad);
  if (rc == GZC_OK) {
    rc = config->crypto->aead_seal(
        config->crypto->userdata,
        exchange->cipher_mode,
        request_key,
        sizeof(request_key),
        request_nonce,
        sizeof(request_nonce),
        (const uint8_t *)offer_sdp.data,
        offer_sdp.len,
        aad.data,
        aad.len,
        &exchange->body);
  }
  gzc_buf_free(&aad, config->platform);
  if (rc != GZC_OK) {
    return rc;
  }

  strcpy(exchange->content_type, "application/octet-stream");
  exchange->headers[0].name = gzc_str_from_cstr("Content-Type");
  exchange->headers[0].value = gzc_str_from_cstr(exchange->content_type);
  exchange->headers[1].name = gzc_str_from_cstr("X-Giznet-Public-Key");
  exchange->headers[1].value = gzc_str_from_parts(exchange->public_key_text, public_key_text_len);
  exchange->headers[2].name = gzc_str_from_cstr("X-Giznet-Timestamp");
  exchange->headers[2].value = gzc_str_from_parts(exchange->timestamp_text, (size_t)ts_len);
  exchange->headers[3].name = gzc_str_from_cstr("X-Giznet-Nonce");
  exchange->headers[3].value = gzc_str_from_parts(exchange->nonce_text, nonce_text_len);

  memset(out_request, 0, sizeof(*out_request));
  out_request->method = GZC_HTTP_METHOD_POST;
  out_request->url = config->signaling_url;
  out_request->headers = exchange->headers;
  out_request->header_count = GZC_SIGNALING_HEADER_COUNT;
  out_request->body = exchange->body.data;
  out_request->body_len = exchange->body.len;
  return GZC_OK;
}

int gzc_signaling_parse_answer_response(
    const gzc_signaling_config_t *config,
    const gzc_signaling_exchange_t *exchange,
    const gzc_http_response_t *response,
    gzc_buf_t *out_answer_sdp) {
  if (config == NULL || config->platform == NULL || config->crypto == NULL || exchange == NULL ||
      response == NULL || out_answer_sdp == NULL || config->crypto->aead_open == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (gzc_http_status_has_error(response->status_code)) {
    return GZC_ERR_HTTP;
  }
  gzc_buf_t aad;
  gzc_buf_init(&aad);
  int rc = build_aad(config->platform, exchange->public_key_text, exchange->timestamp, exchange->nonce_text, true, &aad);
  if (rc == GZC_OK) {
    rc = config->crypto->aead_open(
        config->crypto->userdata,
        exchange->cipher_mode,
        exchange->response_key,
        sizeof(exchange->response_key),
        exchange->response_nonce,
        sizeof(exchange->response_nonce),
        response->body.data,
        response->body.len,
        aad.data,
        aad.len,
        out_answer_sdp);
  }
  gzc_buf_free(&aad, config->platform);
  return rc;
}
