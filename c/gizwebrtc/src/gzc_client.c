#include "gzc_client.h"

#include "gzc_json.h"

#include <string.h>

struct gzc_client {
  gzc_client_config_t config;
  gzc_rtc_peer_t *peer;
  gzc_rtc_channel_t *rpc_channel;
  gzc_buf_t local_sdp;
  gzc_buf_t rpc_response;
  bool has_local_sdp;
  bool rpc_channel_open;
  bool has_rpc_response;
  bool closed;
};

static int64_t now_ms(gzc_client_t *client) {
  if (client->config.platform != NULL && client->config.platform->time_unix_ms != NULL) {
    return client->config.platform->time_unix_ms(client->config.platform->userdata);
  }
  return 0;
}

static int copy_str(gzc_client_t *client, gzc_str_t src, gzc_buf_t *dst) {
  gzc_buf_reset(dst);
  return gzc_buf_append(dst, client->config.platform, src.data, src.len);
}

static void on_peer_state(void *userdata, gzc_rtc_peer_t *peer, gzc_rtc_peer_state_t state) {
  (void)peer;
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL) {
    return;
  }
  if (state == GZC_RTC_PEER_FAILED || state == GZC_RTC_PEER_CLOSED) {
    client->closed = true;
  }
}

static void on_local_sdp(void *userdata, gzc_rtc_peer_t *peer, gzc_rtc_sdp_type_t type, gzc_str_t sdp) {
  (void)peer;
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL || type != GZC_RTC_SDP_OFFER) {
    return;
  }
  if (copy_str(client, sdp, &client->local_sdp) == GZC_OK) {
    client->has_local_sdp = true;
  }
}

static void on_channel_state(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info,
    gzc_rtc_channel_state_t state) {
  (void)peer;
  (void)info;
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL || channel == NULL || channel != client->rpc_channel) {
    return;
  }
  if (state == GZC_RTC_CHANNEL_OPEN) {
    client->rpc_channel_open = true;
  } else if (state == GZC_RTC_CHANNEL_CLOSED || state == GZC_RTC_CHANNEL_ERROR) {
    client->rpc_channel_open = false;
  }
}

static void on_channel_message(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info,
    const uint8_t *data,
    size_t len,
    bool is_text) {
  (void)peer;
  (void)info;
  gzc_client_t *client = (gzc_client_t *)userdata;
  if (client == NULL || channel == NULL || channel != client->rpc_channel || !is_text) {
    return;
  }
  gzc_buf_reset(&client->rpc_response);
  if (gzc_buf_append(&client->rpc_response, client->config.platform, data, len) == GZC_OK) {
    client->has_rpc_response = true;
  }
}

static int wait_until(gzc_client_t *client, bool *flag, int timeout_ms) {
  const int64_t start = now_ms(client);
  while (!*flag) {
    if (client->closed) {
      return GZC_ERR_CLOSED;
    }
    int rc = GZC_OK;
    if (client->config.webrtc->peer_poll != NULL) {
      rc = client->config.webrtc->peer_poll(client->peer, 10);
      if (rc != GZC_OK) {
        return rc;
      }
    } else {
      return GZC_ERR_TIMEOUT;
    }
    if (timeout_ms >= 0 && now_ms(client) - start >= timeout_ms) {
      return GZC_ERR_TIMEOUT;
    }
  }
  return GZC_OK;
}

int gzc_client_create(const gzc_client_config_t *config, gzc_client_t **out_client) {
  if (config == NULL || out_client == NULL || config->http == NULL || config->webrtc == NULL ||
      config->webrtc->peer_create == NULL || config->webrtc->peer_start_offer == NULL ||
      config->webrtc->peer_set_remote_sdp == NULL || config->webrtc->peer_create_data_channel == NULL ||
      config->webrtc->channel_send == NULL || config->webrtc->peer_close == NULL ||
      config->http->post == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = config->platform == NULL ? gzc_default_platform() : config->platform;
  if (platform->malloc == NULL || platform->realloc == NULL || platform->free == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  gzc_client_t *client = (gzc_client_t *)platform->malloc(platform->userdata, sizeof(*client));
  if (client == NULL) {
    return GZC_ERR_NO_MEMORY;
  }
  memset(client, 0, sizeof(*client));
  client->config = *config;
  client->config.platform = platform;
  gzc_buf_init(&client->local_sdp);
  gzc_buf_init(&client->rpc_response);
  *out_client = client;
  return GZC_OK;
}

int gzc_client_connect(gzc_client_t *client) {
  if (client == NULL || client->closed) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->peer != NULL || client->rpc_channel != NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  client->has_local_sdp = false;
  client->rpc_channel_open = false;
  client->has_rpc_response = false;
  gzc_buf_reset(&client->local_sdp);
  gzc_buf_reset(&client->rpc_response);
  gzc_webrtc_callbacks_t callbacks;
  memset(&callbacks, 0, sizeof(callbacks));
  callbacks.userdata = client;
  callbacks.on_peer_state = on_peer_state;
  callbacks.on_local_sdp = on_local_sdp;
  callbacks.on_channel_state = on_channel_state;
  callbacks.on_channel_message = on_channel_message;

  int rc = client->config.webrtc->peer_create(client->config.webrtc->userdata, &callbacks, &client->peer);
  if (rc != GZC_OK) {
    goto fail;
  }

  gzc_rtc_channel_config_t channel_cfg;
  memset(&channel_cfg, 0, sizeof(channel_cfg));
  channel_cfg.label = gzc_str_from_cstr("rpc");
  channel_cfg.ordered = true;
  channel_cfg.reliable = true;
  rc = client->config.webrtc->peer_create_data_channel(client->peer, &channel_cfg, &client->rpc_channel);
  if (rc != GZC_OK) {
    goto fail;
  }

  rc = client->config.webrtc->peer_start_offer(client->peer);
  if (rc != GZC_OK) {
    goto fail;
  }
  int timeout = client->config.connect_timeout_ms == 0 ? 5000 : client->config.connect_timeout_ms;
  rc = wait_until(client, &client->has_local_sdp, timeout);
  if (rc != GZC_OK) {
    goto fail;
  }

  gzc_http_request_t request;
  memset(&request, 0, sizeof(request));
  request.url = client->config.signaling_url;
  request.body = client->local_sdp.data;
  request.body_len = client->local_sdp.len;
  request.timeout_ms = timeout;
  gzc_http_response_t response;
  memset(&response, 0, sizeof(response));
  gzc_buf_init(&response.body);
  rc = client->config.http->post(client->config.http->userdata, &request, &response);
  if (rc == GZC_OK && (response.status_code < 200 || response.status_code >= 300)) {
    rc = GZC_ERR_HTTP;
  }
  if (rc == GZC_OK) {
    gzc_str_t answer = gzc_str_from_parts((const char *)response.body.data, response.body.len);
    rc = client->config.webrtc->peer_set_remote_sdp(client->peer, GZC_RTC_SDP_ANSWER, answer);
  }
  if (client->config.http->response_free != NULL) {
    client->config.http->response_free(client->config.http->userdata, &response);
  } else {
    gzc_buf_free(&response.body, client->config.platform);
  }
  if (rc != GZC_OK) {
    goto fail;
  }

  rc = wait_until(client, &client->rpc_channel_open, timeout);
  if (rc != GZC_OK) {
    goto fail;
  }
  return GZC_OK;

fail:
  if (client->rpc_channel != NULL && client->config.webrtc->channel_close != NULL) {
    client->config.webrtc->channel_close(client->rpc_channel);
    client->rpc_channel = NULL;
  }
  if (client->peer != NULL && client->config.webrtc->peer_close != NULL) {
    client->config.webrtc->peer_close(client->peer);
    client->peer = NULL;
  }
  client->rpc_channel_open = false;
  client->has_local_sdp = false;
  client->has_rpc_response = false;
  return rc;
}

int gzc_client_close(gzc_client_t *client) {
  if (client == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (client->rpc_channel != NULL && client->config.webrtc->channel_close != NULL) {
    client->config.webrtc->channel_close(client->rpc_channel);
    client->rpc_channel = NULL;
  }
  if (client->peer != NULL && client->config.webrtc->peer_close != NULL) {
    client->config.webrtc->peer_close(client->peer);
    client->peer = NULL;
  }
  client->closed = true;
  return GZC_OK;
}

void gzc_client_destroy(gzc_client_t *client) {
  if (client == NULL) {
    return;
  }
  const gzc_platform_t *platform = client->config.platform == NULL ? gzc_default_platform() : client->config.platform;
  (void)gzc_client_close(client);
  gzc_buf_free(&client->local_sdp, platform);
  gzc_buf_free(&client->rpc_response, platform);
  platform->free(platform->userdata, client);
}

gzc_rtc_channel_t *gzc_client_rpc_channel(gzc_client_t *client) {
  return client == NULL ? NULL : client->rpc_channel;
}

const gzc_platform_t *gzc_client_platform(gzc_client_t *client) {
  return client == NULL ? NULL : client->config.platform;
}

const gzc_webrtc_vtable_t *gzc_client_webrtc(gzc_client_t *client) {
  return client == NULL ? NULL : client->config.webrtc;
}

int gzc_client_wait_rpc_response_internal(gzc_client_t *client, int timeout_ms, gzc_str_t *out_json) {
  if (client == NULL || out_json == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  int rc = wait_until(client, &client->has_rpc_response, timeout_ms);
  if (rc != GZC_OK) {
    return rc;
  }
  out_json->data = (const char *)client->rpc_response.data;
  out_json->len = client->rpc_response.len;
  client->has_rpc_response = false;
  return GZC_OK;
}
