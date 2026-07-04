#ifndef GIZCLAW_E2E_CGO_SDK_DRIVER_H
#define GIZCLAW_E2E_CGO_SDK_DRIVER_H

#ifdef __cplusplus
extern "C" {
#endif

int gzc_cgo_run_ping(const char *identity_dir, char *errbuf, unsigned long errbuf_len);
int gzc_cgo_run_server_runtime(const char *identity_dir, char *errbuf, unsigned long errbuf_len);
int gzc_cgo_run_server_status(const char *identity_dir, char *errbuf, unsigned long errbuf_len);
int gzc_cgo_run_speed_test(const char *identity_dir, char *errbuf, unsigned long errbuf_len);
int gzc_cgo_run_firmware_json(const char *identity_dir, char *errbuf, unsigned long errbuf_len);
int gzc_cgo_run_firmware_download(const char *identity_dir, char *errbuf, unsigned long errbuf_len);
int gzc_cgo_run_chat_workspace(const char *identity_dir, char *errbuf, unsigned long errbuf_len);
int gzc_cgo_run_chat_roundtrip(
    const char *identity_dir,
    const char *workspace_name,
    const unsigned char *packet_blob,
    unsigned long packet_blob_len,
    char *errbuf,
    unsigned long errbuf_len);
int gzc_cgo_run_social_basic(const char *identity_dir, char *errbuf, unsigned long errbuf_len);
int gzc_cgo_run_social_relationships(
    const char *identity_a_dir,
    const char *identity_b_dir,
    char *errbuf,
    unsigned long errbuf_len);

#ifdef __cplusplus
}
#endif

#endif
