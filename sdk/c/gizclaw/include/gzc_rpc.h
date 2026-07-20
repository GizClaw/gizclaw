#ifndef GZC_RPC_H
#define GZC_RPC_H

#include "gzc_client.h"
#include "gzc_json.h"
#include "gzc_rpc_frame.h"
#include "payload/ai.pb.h"
#include "payload/edge.pb.h"
#include "payload/enums.pb.h"
#include "payload/firmware.pb.h"
#include "payload/gameplay.pb.h"
#include "payload/social.pb.h"
#include "payload/system.pb.h"
#include "payload/workspace.pb.h"
#include "rpc.pb.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
  int code;
  gzc_str_t message;
  gzc_str_t data_payload;
} gzc_rpc_error_t;

typedef struct {
  gzc_str_t id;
  gzc_str_t result_payload;
  bool has_error;
  gzc_rpc_error_t error;
} gzc_rpc_response_t;

typedef int (*gzc_rpc_frame_cb)(void *userdata, const gzc_rpc_frame_t *frame);
typedef int (*gzc_rpc_speech_audio_cb)(
    void *userdata,
    const uint8_t *data,
    size_t len);

typedef struct gzc_rpc_speech_upload gzc_rpc_speech_upload_t;

int gzc_rpc_encode_request_envelope(
    const gzc_platform_t *platform,
    gzc_str_t id,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_buf_t *out_payload);
int gzc_rpc_decode_response_envelope(gzc_str_t response_payload, gzc_rpc_response_t *out_response);
int gzc_rpc_call_service(
    gzc_client_t *client,
    uint64_t service,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_rpc_response_t *out_response);
int gzc_rpc_call(gzc_client_t *client, gizclaw_rpc_v1_RpcMethod method, gzc_str_t params_payload, gzc_rpc_response_t *out_response);
int gzc_rpc_call_stream(
    gzc_client_t *client,
    gizclaw_rpc_v1_RpcMethod method,
    gzc_str_t params_payload,
    gzc_rpc_frame_cb on_frame,
    void *userdata);
int gzc_rpc_send_frame(gzc_client_t *client, const gzc_rpc_frame_t *frame);
/* Opens an incremental transcription upload on a dedicated Peer RPC stream. */
int gzc_rpc_speech_transcribe_open(
    gzc_client_t *client,
    const gizclaw_rpc_v1_SpeechTranscribeRequest *request,
    gzc_rpc_speech_upload_t **out_upload);
int gzc_rpc_speech_transcribe_write(
    gzc_rpc_speech_upload_t *upload,
    const uint8_t *data,
    size_t len);
/* Sends request EOS, reads the typed response and consumes upload. */
int gzc_rpc_speech_transcribe_finish(
    gzc_rpc_speech_upload_t *upload,
    gizclaw_rpc_v1_SpeechTranscribeResponse *out_response,
    gzc_rpc_error_t *out_error);
void gzc_rpc_speech_transcribe_cancel(gzc_rpc_speech_upload_t *upload);

/* Streams synthesis frames to on_audio after decoding response metadata. */
int gzc_rpc_speech_synthesize(
    gzc_client_t *client,
    const gizclaw_rpc_v1_SpeechSynthesizeRequest *request,
    gizclaw_rpc_v1_SpeechSynthesizeResponse *out_metadata,
    gzc_rpc_speech_audio_cb on_audio,
    void *userdata,
    gzc_rpc_error_t *out_error);
void gzc_rpc_response_free(gzc_client_t *client, gzc_rpc_response_t *response);

#ifdef __cplusplus
}
#endif

#endif
