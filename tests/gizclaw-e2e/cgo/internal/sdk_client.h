#ifndef GIZCLAW_E2E_CGO_SDK_CLIENT_H
#define GIZCLAW_E2E_CGO_SDK_CLIENT_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gzc_cgo_session gzc_cgo_session_t;
typedef struct gzc_cgo_stream_frame {
  int type;
  unsigned char *data;
  unsigned long len;
} gzc_cgo_stream_frame_t;
#define GZC_CGO_E2E_MAX_CHANNELS 16
typedef struct gzc_cgo_transport_snapshot {
  unsigned long long backend_handle;
  int packet_channel_id;
  int packet_ready;
  int event_channel_id;
  int media_ready;
  int next_local_channel_id;
  int active_rpc_channel_id;
  unsigned long rpc_channel_count;
  int rpc_channel_ids[GZC_CGO_E2E_MAX_CHANNELS];
} gzc_cgo_transport_snapshot_t;
typedef struct gzc_service_channel gzc_service_channel_t;
typedef struct gzc_event_stream gzc_event_stream_t;

int gzc_cgo_session_open(
    const char *server_endpoint,
    const char *private_key,
    gzc_cgo_session_t **out_session,
    char *errbuf,
    unsigned long errbuf_len);
void gzc_cgo_session_close(gzc_cgo_session_t *session);
int gzc_cgo_session_call_rpc_payload(
    gzc_cgo_session_t *session,
    unsigned method_id,
    const unsigned char *params_payload,
    unsigned long params_payload_len,
    unsigned char **out_result_payload,
    unsigned long *out_result_payload_len,
    int *out_rpc_error_code,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_register(
    gzc_cgo_session_t *session,
    const char *token,
    char *out_runtime_profile_name,
    unsigned long out_runtime_profile_name_len,
    int *out_has_firmware_name,
    char *out_firmware_name,
    unsigned long out_firmware_name_len,
    int *out_rpc_error_code,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_firmware_get(
    gzc_cgo_session_t *session,
    int channel,
    char *out_name,
    unsigned long out_name_len,
    int *out_channel,
    int *out_has_description,
    char *out_description,
    unsigned long out_description_len,
    char *out_url,
    unsigned long out_url_len,
    char *out_sha256,
    unsigned long out_sha256_len,
    long long *out_size,
    int *out_rpc_error_code,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_call_stream_collect(
    gzc_cgo_session_t *session,
    unsigned method_id,
    const unsigned char *params_payload,
    unsigned long params_payload_len,
    gzc_cgo_stream_frame_t **out_frames,
    unsigned long *out_frame_count,
    char *errbuf,
    unsigned long errbuf_len);
void gzc_cgo_stream_frames_free(gzc_cgo_stream_frame_t *frames, unsigned long frame_count);
int gzc_cgo_session_open_service_channel(
    gzc_cgo_session_t *session,
    unsigned long long service,
    int timeout_ms,
    gzc_service_channel_t **out_channel,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_open_event_stream(
    gzc_cgo_session_t *session,
    int timeout_ms,
    gzc_event_stream_t **out_stream,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_event_stream_send_audio_boundary(
    gzc_event_stream_t *stream,
    const char *stream_id,
    int begin,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_event_stream_read_encoded(
    gzc_event_stream_t *stream,
    int timeout_ms,
    unsigned char **out_data,
    unsigned long *out_data_len,
    char *errbuf,
    unsigned long errbuf_len);
void gzc_cgo_event_stream_close(gzc_event_stream_t *stream);
int gzc_cgo_service_channel_send_json(
    gzc_service_channel_t *channel,
    const char *json,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_service_channel_send_frame(
    gzc_service_channel_t *channel,
    int type,
    const unsigned char *data,
    unsigned long data_len,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_service_channel_read_frame(
    gzc_service_channel_t *channel,
    int timeout_ms,
    int *out_type,
    unsigned char **out_data,
    unsigned long *out_data_len,
    char *errbuf,
    unsigned long errbuf_len);
void gzc_cgo_service_channel_close(gzc_service_channel_t *channel);
int gzc_cgo_session_send_packet(
    gzc_cgo_session_t *session,
    unsigned char protocol,
    const unsigned char *payload,
    unsigned long payload_len,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_transport_send_counts(
    gzc_cgo_session_t *session,
    unsigned long long *out_packet_data_channel_calls,
    unsigned long long *out_opus_rtp_calls);
int gzc_cgo_session_transport_snapshot(
    gzc_cgo_session_t *session,
    gzc_cgo_transport_snapshot_t *out_snapshot);
int gzc_cgo_session_send_battery_telemetry(
    gzc_cgo_session_t *session,
    double percent,
    int charging,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_send_full_telemetry(
    gzc_cgo_session_t *session,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_read_packet(
    gzc_cgo_session_t *session,
    int timeout_ms,
    unsigned char *out_protocol,
    unsigned char **out_payload,
    unsigned long *out_payload_len,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_poll(gzc_cgo_session_t *session, int timeout_ms, char *errbuf, unsigned long errbuf_len);
void gzc_cgo_free(void *ptr);

#ifdef __cplusplus
}
#endif

#endif
