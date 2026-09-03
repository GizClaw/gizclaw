/*
 * Controller-side C client for the GizClaw API-key HTTP surface.
 *
 * The package talks to `/gizclaw/v1` routes over HTTPS with
 * `Authorization: Bearer gizclaw_sk_v1_...`. The device side of GizClaw
 * (WebRTC, RPC, telemetry) stays in `sdk/c/gizclaw`; this package reuses only
 * its transport abstraction (`platform/gzc_platform_http.h`) and its JSON
 * codec (`gzc_json.h`).
 *
 * Memory ownership
 * ----------------
 * Nothing here allocates. The caller owns every buffer:
 *
 *   - `gzc_control_client_t` is a plain struct the caller declares. Its config
 *     holds borrowed `gzc_str_t` values that must outlive the client.
 *   - Every call takes a `gzc_control_call_t` holding two caller-owned
 *     regions: `scratch` for the request URL and body, and `response` for the
 *     response body.
 *   - Decoded models borrow from `call->response`. They stay valid until the
 *     same `gzc_control_call_t` is reused for another call.
 */
#ifndef GZC_CONTROL_H
#define GZC_CONTROL_H

#include "gzc_json.h"
#include "platform/gzc_platform_http.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Route prefix every operation targets. */
#define GZC_CONTROL_PATH_PREFIX "/gizclaw/v1"

/*
 * Contract string caps, in UTF-8 bytes, taken from `api/http/peer.json`.
 * Requests carrying a longer value are rejected with
 * GZC_ERR_INVALID_ARGUMENT before any transport call.
 */
#define GZC_CONTROL_MAX_SSID_BYTES 32
#define GZC_CONTROL_MAX_SOUND_BYTES 32
#define GZC_CONTROL_MAX_DISPLAY_NAME_BYTES 80

/*
 * Longest `X-Request-ID` the client keeps. A longer header is truncated
 * rather than rejected: the id is diagnostic, not part of the contract.
 */
#define GZC_CONTROL_MAX_REQUEST_ID_BYTES 64

/* Volume level bounds for `PUT /gizclaw/v1/device/volume`. */
#define GZC_CONTROL_MIN_VOLUME_LEVEL 0
#define GZC_CONTROL_MAX_VOLUME_LEVEL 100

/*
 * Stable classification of a failed `/gizclaw/v1` routes call.
 *
 * Device control codes (`DEVICE_*`) are matched on the `error.code` string the
 * Server emits; the remaining kinds are matched on the HTTP status. The
 * enumeration and the classification match `sdk/flutter/gizclaw_control`
 * (`GizClawControlErrorKind`) and `sdk/js/gizclaw-control`
 * (`GizClawControlErrorKind`) exactly.
 */
typedef enum {
  /* The call succeeded; no error was classified. */
  GZC_CONTROL_ERROR_NONE = 0,
  /* 401: missing, invalid, or revoked API key. */
  GZC_CONTROL_ERROR_UNAUTHORIZED,
  /* 403: the API key does not authorize this operation. */
  GZC_CONTROL_ERROR_FORBIDDEN,
  /* 404: the key, contact, or saved Wi-Fi network does not exist. */
  GZC_CONTROL_ERROR_NOT_FOUND,
  /* 409 DEVICE_OFFLINE: no active device connection, or rebooting. */
  GZC_CONTROL_ERROR_DEVICE_OFFLINE,
  /* 504 DEVICE_TIMEOUT: the device did not answer within the control timeout. */
  GZC_CONTROL_ERROR_DEVICE_TIMEOUT,
  /* 400 DEVICE_REJECTED: the device rejected the command parameters. */
  GZC_CONTROL_ERROR_DEVICE_REJECTED,
  /* 501 DEVICE_UNSUPPORTED: the firmware does not implement the method. */
  GZC_CONTROL_ERROR_DEVICE_UNSUPPORTED,
  /* 502 DEVICE_ERROR: the device answered with an unexpected RPC error. */
  GZC_CONTROL_ERROR_DEVICE_ERROR,
  /* 409 with any other code, such as a duplicate contact. */
  GZC_CONTROL_ERROR_CONFLICT,
  /* 400 with any other code: the request itself was rejected. */
  GZC_CONTROL_ERROR_INVALID_REQUEST,
  /* Any other 5xx. */
  GZC_CONTROL_ERROR_SERVER,
  /* Any other non-2xx status. */
  GZC_CONTROL_ERROR_UNEXPECTED_STATUS,
  /* A 2xx response whose body could not be decoded as the contract type. */
  GZC_CONTROL_ERROR_MALFORMED_RESPONSE,
  /* The request never produced an HTTP response, or it could not be built. */
  GZC_CONTROL_ERROR_NETWORK
} gzc_control_error_kind_t;

/* Human-readable name of kind, matching the Dart and TypeScript spelling. */
const char *gzc_control_error_kind_string(gzc_control_error_kind_t kind);

/*
 * Maps a non-2xx status and an optional `error.code` to an error kind.
 *
 * `code` may be an empty string when the body carried no `ErrorResponse`.
 * Exposed so callers can classify statuses they obtained elsewhere.
 */
gzc_control_error_kind_t gzc_control_classify(int status_code, gzc_str_t code);

/*
 * Failure of one `/gizclaw/v1` routes call.
 *
 * `code`, `message`, and `details` borrow from the call's response region and
 * are empty when the body carried no `ErrorResponse`. `status_code` is 0 for
 * GZC_CONTROL_ERROR_NETWORK. `details` is the raw `error.details` JSON object.
 *
 * `request_id` is the `X-Request-ID` response header, captured through the
 * transport's response-header sink and copied into the call. It is empty when
 * the Server set none or the transport delivers no headers.
 */
typedef struct {
  gzc_control_error_kind_t kind;
  int status_code;
  gzc_str_t code;
  gzc_str_t message;
  gzc_str_t details;
  gzc_str_t request_id;
} gzc_control_error_t;

/* Configuration of one controller client. Every pointer is borrowed. */
typedef struct {
  /* Server origin, optionally with a path prefix, such as
   * "https://ap.gizclaw.com". A trailing slash is ignored. */
  gzc_str_t base_url;
  /* Complete `gizclaw_sk_v1_...` credential. */
  gzc_str_t api_key;
  /* Transport. Required; `request` must be set. */
  const gzc_http_vtable_t *http;
  /* Allocator, clock, and logging. NULL selects gzc_default_platform(). Only
   * used for logging and for freeing a transport-allocated response body. */
  const gzc_platform_t *platform;
  /* Per-request timeout in milliseconds. 0 selects 30000. */
  int timeout_ms;
  /* Optional bound interface name passed through to the transport. */
  const char *interface_name;
} gzc_control_config_t;

/* Controller client. Declared by the caller; initialize with
 * gzc_control_client_init(). Holds no owned resources, so there is no
 * destructor. Safe to share across threads only if the transport is. */
typedef struct {
  gzc_control_config_t config;
} gzc_control_client_t;

/*
 * Validates config and copies it into client.
 *
 * Returns GZC_ERR_INVALID_ARGUMENT when base_url is empty, api_key is empty,
 * or http->request is NULL.
 */
int gzc_control_client_init(gzc_control_client_t *client, const gzc_control_config_t *config);

/*
 * Caller-owned state of one call.
 *
 * `scratch` holds the request URL and the encoded request body; 512 bytes is
 * enough for every route in this package plus a short contact body. `response`
 * receives the response body and backs every decoded model, so it must stay
 * untouched while the models are read.
 *
 * The remaining fields are outputs, rewritten by every call.
 */
typedef struct {
  uint8_t *scratch;
  size_t scratch_cap;
  uint8_t *response;
  size_t response_cap;

  /* Storage for the captured X-Request-ID; error.request_id points here. */
  char request_id[GZC_CONTROL_MAX_REQUEST_ID_BYTES];
  size_t request_id_len;

  /* HTTP status of the last call, or 0 when no response arrived. */
  int status_code;
  /* Response body of the last call, borrowing from `response`. */
  gzc_str_t body;
  /* Classification of the last call. kind is GZC_CONTROL_ERROR_NONE on success. */
  gzc_control_error_t error;
} gzc_control_call_t;

/* Binds caller-owned regions to a call. Both regions are required. */
int gzc_control_call_init(
    gzc_control_call_t *call,
    uint8_t *scratch,
    size_t scratch_cap,
    uint8_t *response,
    size_t response_cap);

/* --- Models ------------------------------------------------------------- */

/* One API key owned by the bound device (`APIKey`). */
typedef struct {
  gzc_str_t name;
  gzc_str_t display_name;
  gzc_str_t prefix;
  gzc_str_t api_key;
  bool manage_api_keys;
  gzc_str_t created_at;
} gzc_control_api_key_t;

/* Body of `POST /gizclaw/v1/api-keys` (`APIKeyCreateRequest`). */
typedef struct {
  /* At most GZC_CONTROL_MAX_DISPLAY_NAME_BYTES UTF-8 bytes. */
  gzc_str_t display_name;
  bool manage_api_keys;
} gzc_control_api_key_create_request_t;

/* Hardware description of the bound device (`HardwareInfo`). */
typedef struct {
  gzc_str_t manufacturer;
  gzc_str_t model;
  gzc_str_t hardware_revision;
} gzc_control_hardware_info_t;

/* One IMEI declared by the device (`PeerIMEI`). */
typedef struct {
  gzc_str_t name;
  gzc_str_t tac;
  gzc_str_t serial;
} gzc_control_peer_imei_t;

/* One key/value pair, used for `PeerLabel` and for `PeerStatus.labels`. */
typedef struct {
  gzc_str_t key;
  gzc_str_t value;
} gzc_control_pair_t;

/*
 * Identity, hardware, and identifiers of the bound device (`DeviceInfo`).
 *
 * The contract leaves `DeviceInfo` open. `raw` is the complete response object
 * so callers can read keys this SDK does not model; iterate the identifier
 * arrays with gzc_control_device_info_imeis() and
 * gzc_control_device_info_labels().
 */
typedef struct {
  gzc_str_t name;
  gzc_str_t emoji;
  bool has_hardware;
  gzc_control_hardware_info_t hardware;
  bool has_identifiers;
  gzc_str_t identifiers_sn;
  /* Raw `identifiers.imeis` and `identifiers.labels` JSON arrays, empty when
   * absent. */
  gzc_str_t identifiers_imeis;
  gzc_str_t identifiers_labels;
  /* Complete response object, including unmodeled keys. */
  gzc_str_t raw;
} gzc_control_device_info_t;

/* Decodes up to cap IMEIs from info into out and reports the decoded count. */
int gzc_control_device_info_imeis(
    const gzc_control_device_info_t *info,
    gzc_control_peer_imei_t *out,
    size_t cap,
    size_t *out_count);

/* Decodes up to cap labels from info into out and reports the decoded count. */
int gzc_control_device_info_labels(
    const gzc_control_device_info_t *info,
    gzc_control_pair_t *out,
    size_t cap,
    size_t *out_count);

/* Online runtime of the bound device (shared `Runtime`). */
typedef struct {
  bool online;
  gzc_str_t last_seen_at;
  gzc_str_t last_addr;
  bool has_rx_bytes;
  int64_t rx_bytes;
  bool has_tx_bytes;
  int64_t tx_bytes;
} gzc_control_device_runtime_t;

/*
 * Latest status reported by the device (shared `PeerStatus`).
 *
 * Every field is optional in the contract. `raw` is the complete response
 * object; `labels` and `details` are the raw JSON objects, empty when absent.
 */
typedef struct {
  gzc_str_t reported_at;
  bool has_volume;
  int32_t volume;
  bool has_muted;
  bool muted;
  bool has_battery_percent;
  int32_t battery_percent;
  bool has_charging;
  bool charging;
  bool has_gnss_latitude;
  double gnss_latitude;
  bool has_gnss_longitude;
  double gnss_longitude;
  bool has_gnss_altitude_m;
  double gnss_altitude_m;
  bool has_gnss_accuracy_m;
  double gnss_accuracy_m;
  gzc_str_t labels;
  gzc_str_t details;
  gzc_str_t raw;
} gzc_control_peer_status_t;

/* Decodes up to cap `labels` entries from status and reports the count. */
int gzc_control_peer_status_labels(
    const gzc_control_peer_status_t *status,
    gzc_control_pair_t *out,
    size_t cap,
    size_t *out_count);

/* Queryable telemetry field names (`PeerTelemetryField`). */
#define GZC_CONTROL_TELEMETRY_BATTERY_PERCENT "battery.percent"
#define GZC_CONTROL_TELEMETRY_BATTERY_CHARGING "battery.charging"
#define GZC_CONTROL_TELEMETRY_BATTERY_VOLTAGE_MV "battery.voltage_mv"
#define GZC_CONTROL_TELEMETRY_GNSS_LATITUDE "gnss.latitude"
#define GZC_CONTROL_TELEMETRY_GNSS_LONGITUDE "gnss.longitude"
#define GZC_CONTROL_TELEMETRY_GNSS_ALTITUDE_M "gnss.altitude_m"
#define GZC_CONTROL_TELEMETRY_GNSS_ACCURACY_M "gnss.accuracy_m"
#define GZC_CONTROL_TELEMETRY_NETWORK_RSSI_DBM "network.rssi_dbm"
#define GZC_CONTROL_TELEMETRY_NETWORK_SIGNAL_LEVEL "network.signal_level"
#define GZC_CONTROL_TELEMETRY_NETWORK_CONNECTED "network.connected"
#define GZC_CONTROL_TELEMETRY_SYSTEM_UPTIME_SECONDS "system.uptime_seconds"
#define GZC_CONTROL_TELEMETRY_SYSTEM_FREE_MEMORY_BYTES "system.free_memory_bytes"
#define GZC_CONTROL_TELEMETRY_SYSTEM_TEMPERATURE_C "system.temperature_c"

/* Bucket aggregate mode (`PeerTelemetryAggregate`). */
typedef enum {
  GZC_CONTROL_AGGREGATE_AVG = 0,
  GZC_CONTROL_AGGREGATE_MIN,
  GZC_CONTROL_AGGREGATE_MAX,
  GZC_CONTROL_AGGREGATE_SUM,
  GZC_CONTROL_AGGREGATE_COUNT,
  GZC_CONTROL_AGGREGATE_LAST
} gzc_control_aggregate_t;

/* Telemetry point ordering (`PeerTelemetryOrder`). */
typedef enum {
  GZC_CONTROL_ORDER_UNSET = 0,
  GZC_CONTROL_ORDER_ASC,
  GZC_CONTROL_ORDER_DESC
} gzc_control_order_t;

/* Latest value of one telemetry field (`PeerTelemetryValue`). */
typedef struct {
  gzc_str_t field;
  double value;
  int64_t observed_at_unix_ms;
} gzc_control_telemetry_value_t;

/* One sampled telemetry point (`PeerTelemetryPoint`). */
typedef struct {
  int64_t observed_at_unix_ms;
  double value;
} gzc_control_telemetry_point_t;

/* One aggregated telemetry bucket (`PeerTelemetryAggregatePoint`). */
typedef struct {
  int64_t bucket_start_time_ms;
  double value;
} gzc_control_telemetry_bucket_t;

/* Body of `PUT /gizclaw/v1/device/volume` (`DeviceVolumeSetRequest`). */
typedef struct {
  /* Absolute level from GZC_CONTROL_MIN_VOLUME_LEVEL to
   * GZC_CONTROL_MAX_VOLUME_LEVEL. */
  int32_t level;
  bool muted;
} gzc_control_volume_request_t;

/* Body of `POST /gizclaw/v1/device/actions/play-sound`
 * (`DevicePlaySoundRequest`). */
typedef struct {
  /* Device-defined identifier; at most GZC_CONTROL_MAX_SOUND_BYTES bytes. */
  gzc_str_t sound;
  bool has_duration_ms;
  int32_t duration_ms;
} gzc_control_play_sound_request_t;

/* Body of `POST /gizclaw/v1/device/actions/reboot` (`DeviceRebootRequest`). */
typedef struct {
  bool has_delay_ms;
  int32_t delay_ms;
} gzc_control_reboot_request_t;

/* Current Wi-Fi status of the device (`DeviceWifiStatus`). */
typedef struct {
  bool connected;
  gzc_str_t ssid;
  bool has_rssi_dbm;
  int32_t rssi_dbm;
  gzc_str_t ip;
  gzc_str_t bssid;
} gzc_control_wifi_status_t;

/* One contact owned by the bound device (`Contact`). */
typedef struct {
  gzc_str_t name;
  gzc_str_t display_name;
  gzc_str_t phone_number;
  gzc_str_t created_at;
  gzc_str_t updated_at;
} gzc_control_contact_t;

/* Body of `POST /gizclaw/v1/contacts` (`ContactCreateRequest`) and of
 * `PUT /gizclaw/v1/contacts/{contactName}` (`ContactPutRequest`, which ignores
 * `name`). Empty optional fields are omitted from the encoded body. */
typedef struct {
  gzc_str_t name;
  /* At most GZC_CONTROL_MAX_DISPLAY_NAME_BYTES UTF-8 bytes. */
  gzc_str_t display_name;
  gzc_str_t phone_number;
} gzc_control_contact_request_t;

/*
 * Cursor page options shared by the two list routes. An empty cursor leaves
 * the parameter off the request.
 *
 * has_limit is what puts `limit` on the request, so a caller can send limit=0
 * and observe the Server reject it. A zero limit is not the same as an absent
 * one.
 */
typedef struct {
  gzc_str_t cursor;
  bool has_limit;
  int32_t limit;
} gzc_control_page_t;

/* --- API keys ----------------------------------------------------------- */

/* `POST /gizclaw/v1/api-keys`. out_api_key is the recoverable credential. */
int gzc_control_create_api_key(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_api_key_create_request_t *request,
    gzc_control_api_key_t *out_value,
    gzc_str_t *out_api_key);

/* `GET /gizclaw/v1/api-keys`. Decodes up to cap items and reports the count
 * and the `next_cursor`, which is empty on the last page. */
int gzc_control_list_api_keys(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_page_t *page,
    gzc_control_api_key_t *out_items,
    size_t cap,
    size_t *out_count,
    gzc_str_t *out_next_cursor);

/* `GET /gizclaw/v1/api-keys/self`. */
int gzc_control_get_self_api_key(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_api_key_t *out_value);

/* `DELETE /gizclaw/v1/api-keys/self`. */
int gzc_control_revoke_self_api_key(gzc_control_client_t *client, gzc_control_call_t *call);

/* `GET /gizclaw/v1/api-keys/{apiKeyName}`. */
int gzc_control_get_api_key(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t api_key_name,
    gzc_control_api_key_t *out_value);

/* `DELETE /gizclaw/v1/api-keys/{apiKeyName}`. */
int gzc_control_revoke_api_key(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t api_key_name);

/* --- Device reads ------------------------------------------------------- */

/* `GET /gizclaw/v1/device`. */
int gzc_control_get_device(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_device_info_t *out_device);

/* `GET /gizclaw/v1/device/runtime`. */
int gzc_control_get_device_runtime(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_device_runtime_t *out_runtime);

/* `GET /gizclaw/v1/device/status`. Returns the stored snapshot without
 * contacting the device. */
int gzc_control_get_device_status(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_peer_status_t *out_status);

/*
 * `GET /gizclaw/v1/device/telemetry/latest`.
 *
 * `fields` is a comma-separated list of telemetry field names; an empty value
 * requests every supported field.
 */
int gzc_control_get_device_telemetry_latest(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t fields,
    gzc_control_telemetry_value_t *out_values,
    size_t cap,
    size_t *out_count,
    gzc_str_t *out_peer_public_key);

/* Options of `GET /gizclaw/v1/device/telemetry`. step_ms and limit are
 * omitted when 0; order is omitted when GZC_CONTROL_ORDER_UNSET. */
typedef struct {
  gzc_str_t field;
  int64_t start_time_ms;
  int64_t end_time_ms;
  int64_t step_ms;
  int32_t limit;
  gzc_control_order_t order;
} gzc_control_telemetry_query_t;

/* `GET /gizclaw/v1/device/telemetry`. */
int gzc_control_query_device_telemetry(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_telemetry_query_t *query,
    gzc_control_telemetry_point_t *out_points,
    size_t cap,
    size_t *out_count);

/* Options of `GET /gizclaw/v1/device/telemetry/aggregate`. */
typedef struct {
  gzc_str_t field;
  int64_t start_time_ms;
  int64_t end_time_ms;
  int64_t bucket_ms;
  gzc_control_aggregate_t aggregate;
} gzc_control_telemetry_aggregate_query_t;

/* `GET /gizclaw/v1/device/telemetry/aggregate`. */
int gzc_control_aggregate_device_telemetry(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_telemetry_aggregate_query_t *query,
    gzc_control_telemetry_bucket_t *out_points,
    size_t cap,
    size_t *out_count);

/* --- Device control ----------------------------------------------------- */

/* `PUT /gizclaw/v1/device/volume`. out_status is the `PeerStatus` the device
 * reported after applying the volume. */
int gzc_control_set_device_volume(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_volume_request_t *request,
    gzc_control_peer_status_t *out_status);

/* `POST /gizclaw/v1/device/actions/play-sound`. */
int gzc_control_play_device_sound(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_play_sound_request_t *request);

/*
 * `POST /gizclaw/v1/device/actions/reboot`.
 *
 * The device acknowledges before rebooting; later control calls fail with
 * GZC_CONTROL_ERROR_DEVICE_OFFLINE until it reconnects.
 */
int gzc_control_reboot_device(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_reboot_request_t *request);

/* `GET /gizclaw/v1/device/wifi`. */
int gzc_control_get_device_wifi(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_control_wifi_status_t *out_status);

/* `GET /gizclaw/v1/device/wifi/saved`. Decodes up to cap SSIDs. */
int gzc_control_list_device_saved_wifi(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t *out_ssids,
    size_t cap,
    size_t *out_count);

/* `DELETE /gizclaw/v1/device/wifi/saved/{ssid}`. */
int gzc_control_forget_device_saved_wifi(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t ssid);

/* --- Contacts ----------------------------------------------------------- */

/* `GET /gizclaw/v1/contacts`. */
int gzc_control_list_contacts(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_page_t *page,
    gzc_control_contact_t *out_items,
    size_t cap,
    size_t *out_count,
    bool *out_has_next,
    gzc_str_t *out_next_cursor);

/* `POST /gizclaw/v1/contacts`. */
int gzc_control_create_contact(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    const gzc_control_contact_request_t *request,
    gzc_control_contact_t *out_contact);

/* `GET /gizclaw/v1/contacts/{contactName}`. */
int gzc_control_get_contact(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t contact_name,
    gzc_control_contact_t *out_contact);

/* `PUT /gizclaw/v1/contacts/{contactName}`. `request->name` is ignored. */
int gzc_control_put_contact(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t contact_name,
    const gzc_control_contact_request_t *request,
    gzc_control_contact_t *out_contact);

/* `DELETE /gizclaw/v1/contacts/{contactName}`. */
int gzc_control_delete_contact(
    gzc_control_client_t *client,
    gzc_control_call_t *call,
    gzc_str_t contact_name);

#ifdef __cplusplus
}
#endif

#endif
