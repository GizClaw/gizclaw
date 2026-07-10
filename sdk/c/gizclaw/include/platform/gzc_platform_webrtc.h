#ifndef GZC_WEBRTC_H
#define GZC_WEBRTC_H

#include "gzc_platform.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gzc_rtc_peer gzc_rtc_peer_t;
typedef struct gzc_rtc_channel gzc_rtc_channel_t;

typedef enum {
  GZC_RTC_PEER_NEW = 0,
  GZC_RTC_PEER_CONNECTING = 1,
  GZC_RTC_PEER_CONNECTED = 2,
  GZC_RTC_PEER_DISCONNECTED = 3,
  GZC_RTC_PEER_FAILED = 4,
  GZC_RTC_PEER_CLOSED = 5
} gzc_rtc_peer_state_t;

typedef enum {
  GZC_RTC_SDP_OFFER = 1,
  GZC_RTC_SDP_ANSWER = 2
} gzc_rtc_sdp_type_t;

typedef enum {
  GZC_RTC_CHANNEL_OPEN = 1,
  GZC_RTC_CHANNEL_CLOSED = 2,
  GZC_RTC_CHANNEL_ERROR = 3
} gzc_rtc_channel_state_t;

typedef struct {
  gzc_str_t label;
  uint16_t stream_id;
  bool ordered;
  bool reliable;
} gzc_rtc_channel_info_t;

typedef struct {
  gzc_str_t label;
  bool ordered;
  bool reliable;
} gzc_rtc_channel_config_t;

typedef void (*gzc_rtc_peer_state_cb)(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_peer_state_t state);

typedef void (*gzc_rtc_local_sdp_cb)(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_sdp_type_t type,
    gzc_str_t sdp);

typedef void (*gzc_rtc_channel_state_cb)(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info,
    gzc_rtc_channel_state_t state);

typedef void (*gzc_rtc_channel_message_cb)(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info,
    const uint8_t *data,
    size_t len,
    bool is_text);

/*
 * Announces a remotely created channel before any state or message callback
 * for that channel. channel and info->label are borrowed for the callback;
 * the channel remains valid until its close/error callback or peer_close.
 */
typedef void (*gzc_rtc_remote_channel_cb)(
    void *userdata,
    gzc_rtc_peer_t *peer,
    gzc_rtc_channel_t *channel,
    const gzc_rtc_channel_info_t *info);

typedef struct {
  void *userdata;
  gzc_rtc_peer_state_cb on_peer_state;
  gzc_rtc_local_sdp_cb on_local_sdp;
  gzc_rtc_channel_state_cb on_channel_state;
  gzc_rtc_channel_message_cb on_channel_message;
  gzc_rtc_remote_channel_cb on_remote_channel;
} gzc_webrtc_callbacks_t;

typedef struct {
  void *userdata;
  int (*peer_create)(void *userdata, const gzc_webrtc_callbacks_t *callbacks, gzc_rtc_peer_t **out_peer);
  int (*peer_start_offer)(gzc_rtc_peer_t *peer);
  int (*peer_set_remote_sdp)(gzc_rtc_peer_t *peer, gzc_rtc_sdp_type_t type, gzc_str_t sdp);
  int (*peer_create_data_channel)(gzc_rtc_peer_t *peer, const gzc_rtc_channel_config_t *config, gzc_rtc_channel_t **out_channel);
  int (*peer_poll)(gzc_rtc_peer_t *peer, int timeout_ms);
  int (*channel_send)(gzc_rtc_channel_t *channel, const uint8_t *data, size_t len, bool is_text);
  void (*channel_close)(gzc_rtc_channel_t *channel);
  void (*peer_close)(gzc_rtc_peer_t *peer);
  int (*peer_add_ice_server)(gzc_rtc_peer_t *peer, gzc_str_t url, gzc_str_t username, gzc_str_t credential);
} gzc_webrtc_vtable_t;

#ifdef __cplusplus
}
#endif

#endif
