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
typedef struct gzc_service_channel gzc_service_channel_t;

int gzc_cgo_session_open(
    const char *server_endpoint,
    const char *private_key,
    gzc_cgo_session_t **out_session,
    char *errbuf,
    unsigned long errbuf_len);
void gzc_cgo_session_close(gzc_cgo_session_t *session);
int gzc_cgo_session_call_json(
    gzc_cgo_session_t *session,
    const char *method,
    const char *params_payload,
    char **out_result_payload,
    unsigned long *out_result_payload_len,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_session_call_stream_collect(
    gzc_cgo_session_t *session,
    const char *method,
    const char *params_payload,
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
int gzc_cgo_service_channel_send_json(
    gzc_service_channel_t *channel,
    const char *json,
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
