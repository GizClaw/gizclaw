#include "gzc_keys.h"

#include <string.h>

static const char gzc_base58_alphabet[] = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";

static int gzc_is_space(char c) {
  return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v';
}

static int gzc_base58_digit(char c) {
  for (int i = 0; i < 58; i++) {
    if (gzc_base58_alphabet[i] == c) {
      return i;
    }
  }
  return -1;
}

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

int gzc_key_from_text(gzc_str_t text, gzc_key_t *out_key) {
  if (text.data == NULL || out_key == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }

  size_t start = 0;
  size_t end = text.len;
  while (start < end && gzc_is_space(text.data[start])) {
    start++;
  }
  while (end > start && gzc_is_space(text.data[end - 1])) {
    end--;
  }
  if (start == end) {
    return GZC_ERR_INVALID_ARGUMENT;
  }

  uint8_t decoded[GZC_KEY_SIZE];
  memset(decoded, 0, sizeof(decoded));
  size_t leading_zeros = 0;
  while (start + leading_zeros < end && text.data[start + leading_zeros] == gzc_base58_alphabet[0]) {
    leading_zeros++;
  }

  for (size_t i = start; i < end; i++) {
    int digit = gzc_base58_digit(text.data[i]);
    if (digit < 0) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
    int carry = digit;
    for (size_t j = GZC_KEY_SIZE; j > 0; j--) {
      int value = (int)decoded[j - 1] * 58 + carry;
      decoded[j - 1] = (uint8_t)(value & 0xff);
      carry = value >> 8;
    }
    if (carry != 0) {
      return GZC_ERR_INVALID_ARGUMENT;
    }
  }

  size_t first_nonzero = 0;
  while (first_nonzero < GZC_KEY_SIZE && decoded[first_nonzero] == 0) {
    first_nonzero++;
  }
  size_t decoded_len = first_nonzero == GZC_KEY_SIZE ? 0 : GZC_KEY_SIZE - first_nonzero;
  if (leading_zeros + decoded_len != GZC_KEY_SIZE) {
    return GZC_ERR_INVALID_ARGUMENT;
  }

  memcpy(out_key->bytes, decoded, GZC_KEY_SIZE);
  return GZC_OK;
}

int gzc_key_to_text(const gzc_key_t *key, char *out_text, size_t out_text_cap, size_t *out_text_len) {
  if (key == NULL || out_text == NULL || out_text_cap == 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }

  size_t leading_zeros = 0;
  while (leading_zeros < GZC_KEY_SIZE && key->bytes[leading_zeros] == 0) {
    leading_zeros++;
  }

  uint8_t digits[GZC_KEY_TEXT_CAP];
  size_t digit_len = 0;
  for (size_t i = leading_zeros; i < GZC_KEY_SIZE; i++) {
    int carry = key->bytes[i];
    for (size_t j = 0; j < digit_len; j++) {
      int value = (int)digits[j] * 256 + carry;
      digits[j] = (uint8_t)(value % 58);
      carry = value / 58;
    }
    while (carry > 0) {
      if (digit_len >= sizeof(digits)) {
        return GZC_ERR_NO_MEMORY;
      }
      digits[digit_len++] = (uint8_t)(carry % 58);
      carry /= 58;
    }
  }

  size_t total_len = leading_zeros + digit_len;
  if (out_text_cap <= total_len) {
    return GZC_ERR_NO_MEMORY;
  }

  size_t pos = 0;
  for (size_t i = 0; i < leading_zeros; i++) {
    out_text[pos++] = gzc_base58_alphabet[0];
  }
  for (size_t i = digit_len; i > 0; i--) {
    out_text[pos++] = gzc_base58_alphabet[digits[i - 1]];
  }
  out_text[pos] = '\0';
  if (out_text_len != NULL) {
    *out_text_len = pos;
  }
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
