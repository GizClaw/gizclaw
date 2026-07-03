#ifndef GZC_CLIENT_H
#define GZC_CLIENT_H

#include "gzc_http.h"
#include "gzc_webrtc.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gzc_client gzc_client_t;

typedef struct {
  gzc_str_t signaling_url;
  gzc_str_t public_key;
  gzc_str_t private_key;
  const gzc_platform_t *platform;
  const gzc_http_vtable_t *http;
  const gzc_webrtc_vtable_t *webrtc;
  int connect_timeout_ms;
  void *userdata;
} gzc_client_config_t;

int gzc_client_create(const gzc_client_config_t *config, gzc_client_t **out_client);
int gzc_client_connect(gzc_client_t *client);
int gzc_client_close(gzc_client_t *client);
void gzc_client_destroy(gzc_client_t *client);

gzc_rtc_channel_t *gzc_client_rpc_channel(gzc_client_t *client);
const gzc_platform_t *gzc_client_platform(gzc_client_t *client);
const gzc_webrtc_vtable_t *gzc_client_webrtc(gzc_client_t *client);

#ifdef __cplusplus
}
#endif

#endif
