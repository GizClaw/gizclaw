#ifndef GZC_EVENT_H
#define GZC_EVENT_H

#include "events/peer_event.pb.h"
#include "gzc_client.h"

#ifdef __cplusplus
extern "C" {
#endif

#define GZC_SERVICE_PEER_EVENT ((uint64_t)0x20)
#define GZC_PEER_EVENT_VERSION ((uint32_t)1)

typedef struct gzc_event_stream gzc_event_stream_t;
typedef gizclaw_events_v1_PeerEvent gzc_peer_event_t;

/*
 * Opens the connection-owned, reliable Peer Event Stream. A client supports
 * one open non-RPC service channel at a time.
 */
int gzc_event_stream_open(
    gzc_client_t *client,
    int timeout_ms,
    gzc_event_stream_t **out_stream);

/*
 * Encodes event with Nanopb and sends one binary RPC frame. The event and its
 * fixed-size fields are borrowed only for this synchronous call.
 */
int gzc_event_stream_send(
    gzc_event_stream_t *stream,
    const gzc_peer_event_t *event);

/*
 * Reads and decodes one binary frame. out_event is caller-owned and is reset
 * before every decode, including protocol errors. timeout_ms follows
 * gzc_service_channel_read_frame semantics; negative means no deadline.
 */
int gzc_event_stream_read(
    gzc_event_stream_t *stream,
    int timeout_ms,
    gzc_peer_event_t *out_event);

void gzc_event_stream_close(gzc_event_stream_t *stream);

#ifdef __cplusplus
}
#endif

#endif
