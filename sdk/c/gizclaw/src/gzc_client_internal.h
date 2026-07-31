#ifndef GZC_CLIENT_INTERNAL_H
#define GZC_CLIENT_INTERNAL_H

#include "gzc_client.h"

typedef struct gzc_event_stream gzc_event_stream_t;

int gzc_client_acquire_event_channel_internal(
    gzc_client_t *client,
    gzc_event_stream_t *stream,
    gzc_service_channel_t **out_channel);
void gzc_client_release_event_channel_internal(
    gzc_client_t *client,
    gzc_event_stream_t *stream,
    gzc_service_channel_t *channel);
void gzc_event_stream_invalidate_internal(gzc_event_stream_t *stream);

#endif
