#ifndef GIZCLAW_E2E_CGO_SDK_CLIENT_H
#define GIZCLAW_E2E_CGO_SDK_CLIENT_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gzc_cgo_session gzc_cgo_session_t;

int gzc_cgo_session_open(
    const char *signaling_url,
    const char *private_key,
    const char *server_public_key,
    gzc_cgo_session_t **out_session,
    char *errbuf,
    unsigned long errbuf_len);
void gzc_cgo_session_close(gzc_cgo_session_t *session);
int gzc_cgo_session_call_json(
    gzc_cgo_session_t *session,
    const char *method,
    const char *params_json,
    char **out_result_json,
    unsigned long *out_result_json_len,
    char *errbuf,
    unsigned long errbuf_len);
void gzc_cgo_free(void *ptr);

#ifdef __cplusplus
}
#endif

#endif
