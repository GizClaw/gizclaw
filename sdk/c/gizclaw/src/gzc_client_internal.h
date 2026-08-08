#ifndef GZC_CLIENT_INTERNAL_H
#define GZC_CLIENT_INTERNAL_H

#include "gzc_client.h"

typedef struct gzc_event_stream gzc_event_stream_t;
typedef struct gzc_rpc_request gzc_rpc_request_t;

int gzc_client_acquire_event_channel_internal(
    gzc_client_t *client,
    gzc_event_stream_t *stream,
    gzc_service_channel_t **out_channel);
void gzc_client_release_event_channel_internal(
    gzc_client_t *client,
    gzc_event_stream_t *stream,
    gzc_service_channel_t *channel);
void gzc_event_stream_invalidate_internal(gzc_event_stream_t *stream);
int gzc_client_attach_rpc_request_internal(
    gzc_service_channel_t *channel,
    gzc_rpc_request_t *request,
    bool legacy_alias);
void gzc_client_release_rpc_request_internal(
    gzc_client_t *client,
    gzc_service_channel_t *channel,
    gzc_rpc_request_t *request);
int gzc_service_channel_send_frame_until_internal(
    gzc_service_channel_t *channel,
    const gzc_rpc_frame_t *frame,
    int64_t deadline_ms);

int gzc_rpc_request_feed_internal(
    gzc_rpc_request_t *request,
    const uint8_t *data,
    size_t len);
void gzc_rpc_request_channel_closed_internal(gzc_rpc_request_t *request);
void gzc_rpc_request_client_closed_internal(gzc_rpc_request_t *request);
void gzc_rpc_request_transport_error_internal(
    gzc_rpc_request_t *request,
    int status);
void gzc_rpc_request_expire_internal(
    gzc_rpc_request_t *request,
    int64_t now_ms);
bool gzc_rpc_request_terminal_internal(const gzc_rpc_request_t *request);
void gzc_rpc_request_detach_internal(
    gzc_rpc_request_t *request,
    gzc_service_channel_t *channel);

#endif
