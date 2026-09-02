/*
 * C surface the cgo Giztest runner drives.
 *
 * One session owns a connected device client from sdk/c/gizclaw and answers
 * server-initiated client.* RPCs through a Go provider. The control entry
 * point drives sdk/c/gizclaw_control so a Giztest `http` step exercises the
 * typed controller SDK rather than a raw HTTP client.
 */
#ifndef GIZCLAW_E2E_CGO_GIZTEST_BRIDGE_H
#define GIZCLAW_E2E_CGO_GIZTEST_BRIDGE_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gzt_session gzt_session_t;
typedef struct gzt_control gzt_control_t;

/*
 * Opens and connects one device client.
 *
 * provider_handle is the cgo.Handle the C provider passes back to Go when the
 * Server calls a client.* method; pass 0 to answer every client.* method with
 * METHOD_NOT_FOUND.
 */
int gzt_session_open(
    const char *endpoint,
    const char *private_key,
    unsigned long long provider_handle,
    gzt_session_t **out_session,
    char *errbuf,
    unsigned long errbuf_len);

void gzt_session_close(gzt_session_t *session);

/* Pumps the client's transport for at most timeout_ms. */
int gzt_session_poll(gzt_session_t *session, int timeout_ms, char *errbuf, unsigned long errbuf_len);

/*
 * Calls one server.* method with an already-encoded protobuf payload and
 * returns the encoded response payload, which the caller frees with gzt_free.
 *
 * A structured RPC error sets out_rpc_error_code and fills out_error_message.
 */
int gzt_session_call_rpc(
    gzt_session_t *session,
    unsigned method_id,
    const unsigned char *payload,
    unsigned long payload_len,
    unsigned char **out_payload,
    unsigned long *out_payload_len,
    int *out_rpc_error_code,
    char *out_error_message,
    unsigned long out_error_message_len,
    char *errbuf,
    unsigned long errbuf_len);

/*
 * Sends one `/gizclaw/v1` request through the controller SDK.
 *
 * method and path come from the Giztest step; path is the absolute route
 * including the `/gizclaw/v1` prefix. request_json is the encoded request body
 * or NULL. The response status and raw body are reported so the runner can
 * assert on them; out_body is freed with gzt_free. out_error_kind receives the
 * gzc_control_error_kind_t the SDK classified, and is 0 on success.
 *
 * Returns GZC_ERR_UNSUPPORTED when the route is outside the controller SDK's
 * contract, so an unsupported step fails loudly instead of passing.
 */
/*
 * Opens a controller client host. It owns its own HTTP backend so a
 * `/gizclaw/v1` request runs independently of the device client, letting the
 * device keep polling and answer the server-initiated RPC the request
 * triggers.
 */
int gzt_control_open(gzt_control_t **out_control, char *errbuf, unsigned long errbuf_len);
void gzt_control_close(gzt_control_t *control);

int gzt_control_request(
    gzt_control_t *control,
    const char *base_url,
    const char *api_key,
    const char *method,
    const char *path,
    const char *request_json,
    int *out_status,
    unsigned char **out_body,
    unsigned long *out_body_len,
    int *out_error_kind,
    char *errbuf,
    unsigned long errbuf_len);

void gzt_free(void *ptr);

#ifdef __cplusplus
}
#endif

#endif
