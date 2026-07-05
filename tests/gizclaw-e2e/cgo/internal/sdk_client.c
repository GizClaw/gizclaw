#include "sdk_client.h"

#include "bridge.h"
#include "gzc.h"
#include "gzc_rpc_generated.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct gzc_cgo_session {
  gzc_cgo_backend_t backend;
  gzc_http_vtable_t http;
  gzc_platform_crypto_t crypto;
  gzc_webrtc_vtable_t webrtc;
  gzc_client_t *client;
};

static int fail(char *errbuf, unsigned long errbuf_len, const char *message, int rc) {
  if (errbuf != NULL && errbuf_len > 0) {
    (void)snprintf(errbuf, errbuf_len, "%s: %s (%d)", message, gzc_status_string((gzc_status_t)rc), rc);
  }
  return rc == GZC_OK ? GZC_ERR_RPC : rc;
}

int gzc_cgo_session_open(
    const char *signaling_url,
    const char *private_key,
    const char *server_public_key,
    gzc_cgo_session_t **out_session,
    char *errbuf,
    unsigned long errbuf_len) {
  if (signaling_url == NULL || private_key == NULL || server_public_key == NULL || out_session == NULL) {
    return fail(errbuf, errbuf_len, "session open", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_session = NULL;
  gzc_cgo_session_t *session = (gzc_cgo_session_t *)calloc(1, sizeof(*session));
  if (session == NULL) {
    return fail(errbuf, errbuf_len, "session alloc", GZC_ERR_NO_MEMORY);
  }

  int rc = gzc_cgo_backend_init(&session->backend);
  if (rc != GZC_OK) {
    free(session);
    return fail(errbuf, errbuf_len, "backend init", rc);
  }

  gzc_cgo_backend_http_vtable(&session->backend, &session->http);
  gzc_cgo_backend_crypto_vtable(&session->backend, &session->crypto);
  gzc_cgo_backend_webrtc_vtable(&session->backend, &session->webrtc);

  gzc_client_config_t config;
  memset(&config, 0, sizeof(config));
  config.signaling_url = gzc_str_from_cstr(signaling_url);
  config.server_public_key = gzc_str_from_cstr(server_public_key);
  config.private_key = gzc_str_from_cstr(private_key);
  config.platform = session->backend.platform;
  config.crypto = &session->crypto;
  config.http = &session->http;
  config.webrtc = &session->webrtc;
  config.cipher_mode = GZC_CIPHER_CHACHA20_POLY1305;
  config.connect_timeout_ms = 15000;

  rc = gzc_client_create(&config, &session->client);
  if (rc != GZC_OK) {
    gzc_cgo_backend_deinit(&session->backend);
    free(session);
    return fail(errbuf, errbuf_len, "client create", rc);
  }

  rc = gzc_client_connect(session->client);
  if (rc != GZC_OK) {
    gzc_client_destroy(session->client);
    gzc_cgo_backend_deinit(&session->backend);
    free(session);
    return fail(errbuf, errbuf_len, "client connect", rc);
  }
  *out_session = session;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzc_cgo_session_close(gzc_cgo_session_t *session) {
  if (session == NULL) {
    return;
  }
  if (session->client != NULL) {
    gzc_client_destroy(session->client);
    session->client = NULL;
  }
  gzc_cgo_backend_deinit(&session->backend);
  free(session);
}

int gzc_cgo_session_call_json(
    gzc_cgo_session_t *session,
    const char *method,
    const char *params_json,
    char **out_result_json,
    unsigned long *out_result_json_len,
    char *errbuf,
    unsigned long errbuf_len) {
  if (session == NULL || method == NULL || params_json == NULL || out_result_json == NULL || out_result_json_len == NULL) {
    return fail(errbuf, errbuf_len, "call json", GZC_ERR_INVALID_ARGUMENT);
  }
  *out_result_json = NULL;
  *out_result_json_len = 0;

  gzc_rpc_response_t response;
  memset(&response, 0, sizeof(response));
  int rc = gzc_rpc_call_json(
      session->client,
      gzc_str_from_cstr(method),
      gzc_str_from_cstr(params_json),
      &response);
  if (rc != GZC_OK) {
    return fail(errbuf, errbuf_len, "call json", rc);
  }
  if (response.has_error) {
    return fail(errbuf, errbuf_len, "rpc error", GZC_ERR_RPC);
  }

  char *result = (char *)malloc(response.result_json.len + 1);
  if (result == NULL) {
    return fail(errbuf, errbuf_len, "copy result", GZC_ERR_NO_MEMORY);
  }
  memcpy(result, response.result_json.data, response.result_json.len);
  result[response.result_json.len] = '\0';
  *out_result_json = result;
  *out_result_json_len = (unsigned long)response.result_json.len;
  if (errbuf != NULL && errbuf_len > 0) {
    errbuf[0] = 0;
  }
  return GZC_OK;
}

void gzc_cgo_free(void *ptr) {
  free(ptr);
}
