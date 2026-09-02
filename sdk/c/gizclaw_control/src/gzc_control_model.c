/*
 * Field readers and model decoders for the `/gizclaw/v1` routes contract.
 *
 * Decoding never copies: every gzc_str_t borrows from the response body the
 * caller supplied through gzc_control_call_t.
 */
#include "gzc_control_internal.h"

#include <string.h>

/* JSON null, the only literal a contract field uses to mean "absent". */
static bool is_null(gzc_str_t raw) { return gzc_control_str_eq_cstr(raw, "null"); }

int gzc_control_field(gzc_str_t object_json, const char *name, gzc_str_t *out_raw, bool *out_present) {
  if (out_raw == NULL || out_present == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_present = false;
  gzc_str_t raw;
  int rc = gzc_json_find_field(object_json, name, &raw);
  if (rc != GZC_OK) {
    /* A missing field is not a decode failure; the caller decides. */
    return rc == GZC_ERR_JSON ? GZC_OK : rc;
  }
  if (is_null(raw)) {
    return GZC_OK;
  }
  *out_raw = raw;
  *out_present = true;
  return GZC_OK;
}

int gzc_control_opt_str(gzc_str_t object_json, const char *name, gzc_str_t *out) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK || !present) {
    return rc;
  }
  return gzc_json_parse_string(raw, out);
}

int gzc_control_opt_bool(gzc_str_t object_json, const char *name, bool *out, bool *out_present) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK || !present) {
    return rc;
  }
  rc = gzc_json_parse_bool(raw, out);
  if (rc == GZC_OK && out_present != NULL) {
    *out_present = true;
  }
  return rc;
}

int gzc_control_opt_i32(gzc_str_t object_json, const char *name, int32_t *out, bool *out_present) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK || !present) {
    return rc;
  }
  rc = gzc_json_parse_i32(raw, out);
  if (rc == GZC_OK && out_present != NULL) {
    *out_present = true;
  }
  return rc;
}

int gzc_control_opt_i64(gzc_str_t object_json, const char *name, int64_t *out, bool *out_present) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK || !present) {
    return rc;
  }
  rc = gzc_json_parse_i64(raw, out);
  if (rc == GZC_OK && out_present != NULL) {
    *out_present = true;
  }
  return rc;
}

int gzc_control_opt_f64(gzc_str_t object_json, const char *name, double *out, bool *out_present) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK || !present) {
    return rc;
  }
  rc = gzc_json_parse_f64(raw, out);
  if (rc == GZC_OK && out_present != NULL) {
    *out_present = true;
  }
  return rc;
}

int gzc_control_opt_raw(gzc_str_t object_json, const char *name, gzc_str_t *out) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK || !present) {
    return rc;
  }
  *out = raw;
  return GZC_OK;
}

int gzc_control_req_str(gzc_str_t object_json, const char *name, gzc_str_t *out) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK) {
    return rc;
  }
  if (!present) {
    return GZC_ERR_JSON;
  }
  return gzc_json_parse_string(raw, out);
}

int gzc_control_req_bool(gzc_str_t object_json, const char *name, bool *out) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK) {
    return rc;
  }
  if (!present) {
    return GZC_ERR_JSON;
  }
  return gzc_json_parse_bool(raw, out);
}

int gzc_control_req_i64(gzc_str_t object_json, const char *name, int64_t *out) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK) {
    return rc;
  }
  if (!present) {
    return GZC_ERR_JSON;
  }
  return gzc_json_parse_i64(raw, out);
}

int gzc_control_req_f64(gzc_str_t object_json, const char *name, double *out) {
  gzc_str_t raw;
  bool present = false;
  int rc = gzc_control_field(object_json, name, &raw, &present);
  if (rc != GZC_OK) {
    return rc;
  }
  if (!present) {
    return GZC_ERR_JSON;
  }
  return gzc_json_parse_f64(raw, out);
}

int gzc_control_decode_array(
    gzc_str_t array_json,
    void *out,
    size_t stride,
    size_t cap,
    size_t *out_count,
    gzc_control_decode_fn decode) {
  if (out_count == NULL || decode == NULL || stride == 0 || (out == NULL && cap != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_count = 0;
  if (gzc_control_str_empty(array_json)) {
    return GZC_OK;
  }
  gzc_json_array_iter_t iter;
  int rc = gzc_json_array_iter_init(array_json, &iter);
  if (rc != GZC_OK) {
    return rc;
  }
  for (;;) {
    gzc_str_t item;
    bool has_value = false;
    rc = gzc_json_array_iter_next(&iter, &item, &has_value);
    if (rc != GZC_OK) {
      return rc;
    }
    if (!has_value) {
      return GZC_OK;
    }
    if (*out_count == cap) {
      return GZC_ERR_BUFFER_TOO_SMALL;
    }
    rc = decode(item, (uint8_t *)out + (*out_count * stride));
    if (rc != GZC_OK) {
      return rc;
    }
    (*out_count)++;
  }
}

int gzc_control_decode_api_key(gzc_str_t object_json, gzc_control_api_key_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_control_req_str(object_json, "name", &out->name);
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "display_name", &out->display_name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "prefix", &out->prefix);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "api_key", &out->api_key);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_bool(object_json, "manage_api_keys", &out->manage_api_keys, NULL);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "created_at", &out->created_at);
  }
  return rc;
}

int gzc_control_decode_api_key_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_api_key(object_json, (gzc_control_api_key_t *)out);
}

int gzc_control_decode_device_info(gzc_str_t object_json, gzc_control_device_info_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc != GZC_OK) {
    return rc;
  }
  out->raw = object_json;
  rc = gzc_control_opt_str(object_json, "name", &out->name);
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "emoji", &out->emoji);
  }
  gzc_str_t nested;
  if (rc == GZC_OK) {
    nested = gzc_str_from_parts(NULL, 0);
    rc = gzc_control_opt_raw(object_json, "hardware", &nested);
    if (rc == GZC_OK && !gzc_control_str_empty(nested)) {
      out->has_hardware = true;
      rc = gzc_control_opt_str(nested, "manufacturer", &out->hardware.manufacturer);
      if (rc == GZC_OK) {
        rc = gzc_control_opt_str(nested, "model", &out->hardware.model);
      }
      if (rc == GZC_OK) {
        rc = gzc_control_opt_str(nested, "hardware_revision", &out->hardware.hardware_revision);
      }
    }
  }
  if (rc == GZC_OK) {
    nested = gzc_str_from_parts(NULL, 0);
    rc = gzc_control_opt_raw(object_json, "identifiers", &nested);
    if (rc == GZC_OK && !gzc_control_str_empty(nested)) {
      out->has_identifiers = true;
      rc = gzc_control_opt_str(nested, "sn", &out->identifiers_sn);
      if (rc == GZC_OK) {
        rc = gzc_control_opt_raw(nested, "imeis", &out->identifiers_imeis);
      }
      if (rc == GZC_OK) {
        rc = gzc_control_opt_raw(nested, "labels", &out->identifiers_labels);
      }
    }
  }
  return rc;
}

static int decode_imei_item(gzc_str_t object_json, void *out) {
  gzc_control_peer_imei_t *imei = (gzc_control_peer_imei_t *)out;
  memset(imei, 0, sizeof(*imei));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "tac", &imei->tac);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "serial", &imei->serial);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "name", &imei->name);
  }
  return rc;
}

static int decode_pair_item(gzc_str_t object_json, void *out) {
  gzc_control_pair_t *pair = (gzc_control_pair_t *)out;
  memset(pair, 0, sizeof(*pair));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "key", &pair->key);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "value", &pair->value);
  }
  return rc;
}

int gzc_control_device_info_imeis(
    const gzc_control_device_info_t *info,
    gzc_control_peer_imei_t *out,
    size_t cap,
    size_t *out_count) {
  if (info == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzc_control_decode_array(
      info->identifiers_imeis, out, sizeof(*out), cap, out_count, decode_imei_item);
}

int gzc_control_device_info_labels(
    const gzc_control_device_info_t *info,
    gzc_control_pair_t *out,
    size_t cap,
    size_t *out_count) {
  if (info == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzc_control_decode_array(
      info->identifiers_labels, out, sizeof(*out), cap, out_count, decode_pair_item);
}

int gzc_control_decode_device_runtime(gzc_str_t object_json, gzc_control_device_runtime_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_bool(object_json, "online", &out->online);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "last_seen_at", &out->last_seen_at);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "last_addr", &out->last_addr);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_i64(object_json, "rx_bytes", &out->rx_bytes, &out->has_rx_bytes);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_i64(object_json, "tx_bytes", &out->tx_bytes, &out->has_tx_bytes);
  }
  return rc;
}

int gzc_control_decode_peer_status(gzc_str_t object_json, gzc_control_peer_status_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc != GZC_OK) {
    return rc;
  }
  out->raw = object_json;
  rc = gzc_control_opt_str(object_json, "reported_at", &out->reported_at);
  if (rc == GZC_OK) {
    rc = gzc_control_opt_i32(object_json, "volume", &out->volume, &out->has_volume);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_bool(object_json, "muted", &out->muted, &out->has_muted);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_i32(object_json, "battery_percent", &out->battery_percent, &out->has_battery_percent);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_bool(object_json, "charging", &out->charging, &out->has_charging);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_f64(object_json, "gnss_latitude", &out->gnss_latitude, &out->has_gnss_latitude);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_f64(object_json, "gnss_longitude", &out->gnss_longitude, &out->has_gnss_longitude);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_f64(object_json, "gnss_altitude_m", &out->gnss_altitude_m, &out->has_gnss_altitude_m);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_f64(object_json, "gnss_accuracy_m", &out->gnss_accuracy_m, &out->has_gnss_accuracy_m);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_raw(object_json, "labels", &out->labels);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_raw(object_json, "details", &out->details);
  }
  return rc;
}

/*
 * `PeerStatus.labels` is a JSON object of string values, so it is walked by
 * key rather than by the array iterator.
 */
int gzc_control_peer_status_labels(
    const gzc_control_peer_status_t *status,
    gzc_control_pair_t *out,
    size_t cap,
    size_t *out_count) {
  if (status == NULL || out_count == NULL || (out == NULL && cap != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *out_count = 0;
  gzc_str_t labels = status->labels;
  if (gzc_control_str_empty(labels)) {
    return GZC_OK;
  }
  int rc = gzc_json_validate_object(labels);
  if (rc != GZC_OK) {
    return rc;
  }
  /* Reuse the array iterator over the object body by walking "key": value
   * pairs: gzc_json.h exposes lookup by name only, so the scan is local. */
  size_t index = 1;
  while (index < labels.len) {
    while (index < labels.len && (labels.data[index] == ' ' || labels.data[index] == '\t' ||
                                  labels.data[index] == '\n' || labels.data[index] == '\r' ||
                                  labels.data[index] == ',')) {
      index++;
    }
    if (index >= labels.len || labels.data[index] == '}') {
      return GZC_OK;
    }
    if (labels.data[index] != '"') {
      return GZC_ERR_JSON;
    }
    size_t key_start = index;
    index++;
    while (index < labels.len && labels.data[index] != '"') {
      if (labels.data[index] == '\\') {
        return GZC_ERR_UNSUPPORTED;
      }
      index++;
    }
    if (index >= labels.len) {
      return GZC_ERR_JSON;
    }
    index++;
    gzc_str_t key_raw = gzc_str_from_parts(labels.data + key_start, index - key_start);
    while (index < labels.len && (labels.data[index] == ' ' || labels.data[index] == '\t' ||
                                  labels.data[index] == '\n' || labels.data[index] == '\r')) {
      index++;
    }
    if (index >= labels.len || labels.data[index] != ':') {
      return GZC_ERR_JSON;
    }
    index++;
    while (index < labels.len && (labels.data[index] == ' ' || labels.data[index] == '\t' ||
                                  labels.data[index] == '\n' || labels.data[index] == '\r')) {
      index++;
    }
    if (index >= labels.len || labels.data[index] != '"') {
      return GZC_ERR_JSON;
    }
    size_t value_start = index;
    index++;
    while (index < labels.len && labels.data[index] != '"') {
      if (labels.data[index] == '\\') {
        return GZC_ERR_UNSUPPORTED;
      }
      index++;
    }
    if (index >= labels.len) {
      return GZC_ERR_JSON;
    }
    index++;
    gzc_str_t value_raw = gzc_str_from_parts(labels.data + value_start, index - value_start);
    if (*out_count == cap) {
      return GZC_ERR_BUFFER_TOO_SMALL;
    }
    rc = gzc_json_parse_string(key_raw, &out[*out_count].key);
    if (rc == GZC_OK) {
      rc = gzc_json_parse_string(value_raw, &out[*out_count].value);
    }
    if (rc != GZC_OK) {
      return rc;
    }
    (*out_count)++;
  }
  return GZC_ERR_JSON;
}

int gzc_control_decode_wifi_status(gzc_str_t object_json, gzc_control_wifi_status_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_bool(object_json, "connected", &out->connected);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "ssid", &out->ssid);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_i32(object_json, "rssi_dbm", &out->rssi_dbm, &out->has_rssi_dbm);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "ip", &out->ip);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "bssid", &out->bssid);
  }
  return rc;
}

int gzc_control_decode_contact(gzc_str_t object_json, gzc_control_contact_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "name", &out->name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "display_name", &out->display_name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "phone_number", &out->phone_number);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "created_at", &out->created_at);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "updated_at", &out->updated_at);
  }
  return rc;
}

int gzc_control_decode_contact_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_contact(object_json, (gzc_control_contact_t *)out);
}

int gzc_control_decode_telemetry_value(gzc_str_t object_json, gzc_control_telemetry_value_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "field", &out->field);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_f64(object_json, "value", &out->value);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_i64(object_json, "observed_at_unix_ms", &out->observed_at_unix_ms);
  }
  return rc;
}

int gzc_control_decode_telemetry_point(gzc_str_t object_json, gzc_control_telemetry_point_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_i64(object_json, "observed_at_unix_ms", &out->observed_at_unix_ms);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_f64(object_json, "value", &out->value);
  }
  return rc;
}

int gzc_control_decode_telemetry_bucket(gzc_str_t object_json, gzc_control_telemetry_bucket_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_i64(object_json, "bucket_start_time_ms", &out->bucket_start_time_ms);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_f64(object_json, "value", &out->value);
  }
  return rc;
}

int gzc_control_decode_telemetry_value_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_telemetry_value(object_json, (gzc_control_telemetry_value_t *)out);
}

int gzc_control_decode_telemetry_point_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_telemetry_point(object_json, (gzc_control_telemetry_point_t *)out);
}

int gzc_control_decode_telemetry_bucket_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_telemetry_bucket(object_json, (gzc_control_telemetry_bucket_t *)out);
}

int gzc_control_decode_saved_wifi_item(gzc_str_t object_json, void *out) {
  gzc_str_t *ssid = (gzc_str_t *)out;
  int rc = gzc_json_validate_object(object_json);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_control_req_str(object_json, "ssid", ssid);
}
