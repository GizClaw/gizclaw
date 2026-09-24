/*
 * Field readers and model decoders for the `/gizclaw/v1` routes contract.
 *
 * Models borrow from the caller-owned response body; MHS escaped strings use
 * a separate caller-owned string region.
 */
#include "gzc_control_internal.h"

#include <math.h>
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

int gzc_control_decode_runtime_profile_workflow_item(gzc_str_t object_json, void *out) {
  gzc_control_runtime_profile_workflow_t *workflow = out;
  memset(workflow, 0, sizeof(*workflow));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "name", &workflow->name);
  }
  bool present = false;
  if (rc == GZC_OK) {
    rc = gzc_control_field(object_json, "tags", &workflow->tags, &present);
  }
  if (rc == GZC_OK && !present) {
    rc = GZC_ERR_JSON;
  }
  if (rc == GZC_OK) {
    gzc_json_array_iter_t iter;
    rc = gzc_json_array_iter_init(workflow->tags, &iter);
  }
  return rc;
}

int gzc_control_decode_device_workspace_item(gzc_str_t object_json, void *out) {
  gzc_control_device_workspace_t *workspace = out;
  memset(workspace, 0, sizeof(*workspace));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "id", &workspace->id);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "name", &workspace->name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "workflow_name", &workspace->workflow_name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_bool(object_json, "available", &workspace->available);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_bool(object_json, "system", &workspace->system);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "created_at", &workspace->created_at);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "updated_at", &workspace->updated_at);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "last_active_at", &workspace->last_active_at);
  }
  return rc;
}

int gzc_control_runtime_profile_workflow_tags(
    const gzc_control_runtime_profile_workflow_t *workflow,
    gzc_str_t *out,
    size_t cap,
    size_t *out_count) {
  if (workflow == NULL || out_count == NULL || (out == NULL && cap != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  return gzc_control_decode_array(
      workflow->tags, out, sizeof(*out), cap, out_count, gzc_control_decode_string_item);
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
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "active_workspace_name", &out->active_workspace_name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "pending_workspace_name", &out->pending_workspace_name);
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
  gzc_str_t player;
  rc = gzc_control_field(object_json, "audioplayer", &player, &out->has_audioplayer);
  if (rc != GZC_OK)
    return rc;
  if (out->has_audioplayer) {
    rc = gzc_control_decode_audioplayer_status(player, &out->audioplayer);
    if (rc != GZC_OK)
      return rc;
  }
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
    rc = gzc_control_opt_str(object_json, "activity", &out->activity);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "activity_detail", &out->activity_detail);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "firmware_version", &out->firmware_version);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_f64(object_json, "wifi_rssi_dbm", &out->wifi_rssi_dbm, &out->has_wifi_rssi_dbm);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_f64(object_json, "cellular_rssi_dbm", &out->cellular_rssi_dbm, &out->has_cellular_rssi_dbm);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_f64(
        object_json, "cellular_signal_level", &out->cellular_signal_level, &out->has_cellular_signal_level);
  }
  return rc;
}

/* Skips the JSON string token starting at index, honoring escapes, and
 * reports the token including both quotes. */
static int label_scan_string_token(gzc_str_t text, size_t *index, gzc_str_t *out) {
  size_t i = *index;
  if (i >= text.len || text.data[i] != '"') {
    return GZC_ERR_JSON;
  }
  size_t start = i;
  i++;
  while (i < text.len) {
    char ch = text.data[i];
    if (ch == '\\') {
      /* Skip the escape and whatever it escapes; \uXXXX is covered because
       * none of the four hex digits can be an unescaped quote. */
      i += 2;
      continue;
    }
    if (ch == '"') {
      i++;
      *out = gzc_str_from_parts(text.data + start, i - start);
      *index = i;
      return GZC_OK;
    }
    i++;
  }
  return GZC_ERR_JSON;
}

static void label_skip_ws(gzc_str_t text, size_t *index) {
  while (*index < text.len) {
    char ch = text.data[*index];
    if (ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r') {
      return;
    }
    (*index)++;
  }
}

/*
 * `PeerStatus.labels` is a JSON object of string values, so it is walked by
 * key rather than by the array iterator, which only handles arrays.
 *
 * The scan honors escapes when finding token boundaries, so an escaped label
 * is delimited correctly. Whether the escape itself can be decoded is left to
 * gzc_json_parse_string, exactly as for every other string field in this
 * package.
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
  size_t index = 1;
  for (;;) {
    label_skip_ws(labels, &index);
    while (index < labels.len && labels.data[index] == ',') {
      index++;
      label_skip_ws(labels, &index);
    }
    if (index >= labels.len || labels.data[index] == '}') {
      return GZC_OK;
    }
    gzc_str_t key_raw;
    rc = label_scan_string_token(labels, &index, &key_raw);
    if (rc != GZC_OK) {
      return rc;
    }
    label_skip_ws(labels, &index);
    if (index >= labels.len || labels.data[index] != ':') {
      return GZC_ERR_JSON;
    }
    index++;
    label_skip_ws(labels, &index);
    gzc_str_t value_raw;
    rc = label_scan_string_token(labels, &index, &value_raw);
    if (rc != GZC_OK) {
      return rc;
    }
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
}

int gzc_control_decode_wifi_scan_result(
    gzc_str_t object_json,
    gzc_control_wifi_scan_result_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "ssid", &out->ssid);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "bssid", &out->bssid);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_i64(object_json, "rssi_dbm", &out->rssi_dbm, &out->has_rssi_dbm);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_i64(
        object_json, "frequency_mhz", &out->frequency_mhz, &out->has_frequency_mhz);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "security", &out->security);
  }
  return rc;
}

int gzc_control_decode_wifi_scan_result_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_wifi_scan_result(
      object_json, (gzc_control_wifi_scan_result_t *)out);
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

/* Decodes the optional `info` object of a Friend or Friend Group member. */
static int decode_peer_profile_info(
    gzc_str_t object_json,
    bool *out_has_info,
    gzc_control_peer_profile_info_t *out_info) {
  gzc_str_t nested = gzc_str_from_parts(NULL, 0);
  int rc = gzc_control_opt_raw(object_json, "info", &nested);
  if (rc != GZC_OK || gzc_control_str_empty(nested)) {
    return rc;
  }
  rc = gzc_json_validate_object(nested);
  if (rc == GZC_OK) {
    *out_has_info = true;
    rc = gzc_control_opt_str(nested, "display_name", &out_info->display_name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(nested, "emoji", &out_info->emoji);
  }
  return rc;
}

int gzc_control_decode_invite_token(gzc_str_t object_json, gzc_control_invite_token_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "invite_token", &out->invite_token);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "expires_at", &out->expires_at);
  }
  return rc;
}

int gzc_control_decode_friend(gzc_str_t object_json, gzc_control_friend_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "name", &out->name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "peer_public_key", &out->peer_public_key);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "workspace_name", &out->workspace_name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "created_at", &out->created_at);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "updated_at", &out->updated_at);
  }
  if (rc == GZC_OK) {
    rc = decode_peer_profile_info(object_json, &out->has_info, &out->info);
  }
  return rc;
}

int gzc_control_decode_friend_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_friend(object_json, (gzc_control_friend_t *)out);
}

int gzc_control_decode_friend_group(gzc_str_t object_json, gzc_control_friend_group_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "name", &out->name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "my_role", &out->my_role);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "display_name", &out->display_name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "description", &out->description);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "created_by_peer_public_key", &out->created_by_peer_public_key);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "workspace_name", &out->workspace_name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "created_at", &out->created_at);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "updated_at", &out->updated_at);
  }
  return rc;
}

int gzc_control_decode_friend_group_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_friend_group(object_json, (gzc_control_friend_group_t *)out);
}

int gzc_control_decode_friend_group_member(
    gzc_str_t object_json,
    gzc_control_friend_group_member_t *out) {
  if (out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "name", &out->name);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "peer_public_key", &out->peer_public_key);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_req_str(object_json, "role", &out->role);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "created_at", &out->created_at);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "updated_at", &out->updated_at);
  }
  if (rc == GZC_OK) {
    rc = decode_peer_profile_info(object_json, &out->has_info, &out->info);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_bool(object_json, "online", &out->online, &out->has_online);
  }
  if (rc == GZC_OK) {
    rc = gzc_control_opt_str(object_json, "last_seen_at", &out->last_seen_at);
  }
  return rc;
}

int gzc_control_decode_friend_group_member_item(gzc_str_t object_json, void *out) {
  return gzc_control_decode_friend_group_member(object_json, (gzc_control_friend_group_member_t *)out);
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

int gzc_control_decode_audioplayer_item(gzc_str_t object_json, void *result) {
  gzc_control_audioplayer_item_t *out = result;
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK)
    rc = gzc_control_req_str(object_json, "url", &out->url);
  if (rc == GZC_OK)
    rc = gzc_control_opt_str(object_json, "title", &out->title);
  if (rc == GZC_OK)
    rc = gzc_control_opt_str(object_json, "source_ref", &out->source_ref);
  return rc;
}

int gzc_control_decode_audioplayer_status(gzc_str_t object_json, gzc_control_audioplayer_status_t *out) {
  if (out == NULL)
    return GZC_ERR_INVALID_ARGUMENT;
  memset(out, 0, sizeof(*out));
  int rc = gzc_json_validate_object(object_json);
  if (rc == GZC_OK)
    rc = gzc_control_req_str(object_json, "state", &out->state);
  if (rc == GZC_OK)
    rc = gzc_control_opt_i32(object_json, "current_index", &out->current_index, &out->has_current_index);
  if (rc == GZC_OK)
    rc = gzc_control_req_i64(object_json, "position_ms", &out->position_ms);
  if (rc == GZC_OK)
    rc = gzc_control_opt_i64(object_json, "duration_ms", &out->duration_ms, &out->has_duration_ms);
  if (rc == GZC_OK)
    rc = gzc_control_req_str(object_json, "repeat", &out->repeat);
  if (rc == GZC_OK)
    rc = gzc_control_req_i64(object_json, "playlist_length", &out->playlist_length);
  if (rc == GZC_OK)
    rc = gzc_control_req_i64(object_json, "playlist_revision", &out->playlist_revision);
  if (rc == GZC_OK)
    rc = gzc_control_opt_str(object_json, "error_code", &out->error_code);
  if (rc == GZC_OK)
    rc = gzc_control_opt_str(object_json, "error_message", &out->error_message);
  if (rc == GZC_OK)
    rc = gzc_control_req_i64(object_json, "observed_at_unix_ms", &out->observed_at_unix_ms);
  return rc;
}

int gzc_control_decode_string_item(gzc_str_t raw_json, void *out) {
  return gzc_json_parse_string(raw_json, (gzc_str_t *)out);
}

static int hex_digit(char c) {
  if (c >= '0' && c <= '9') {
    return c - '0';
  }
  if (c >= 'a' && c <= 'f') {
    return c - 'a' + 10;
  }
  if (c >= 'A' && c <= 'F') {
    return c - 'A' + 10;
  }
  return -1;
}

static int read_hex4(const char *p, uint32_t *out) {
  uint32_t value = 0;
  for (int i = 0; i < 4; i++) {
    int digit = hex_digit(p[i]);
    if (digit < 0) {
      return GZC_ERR_JSON;
    }
    value = (value << 4) | (uint32_t)digit;
  }
  *out = value;
  return GZC_OK;
}

/*
 * Decodes the JSON string token raw (quotes included) into dst and reports the
 * unescaped bytes. The unescaped form is never longer than the token minus its
 * quotes, so dst needs at most raw.len - 2 bytes.
 */
int gzc_control_unescape_string(gzc_str_t raw, char *dst, size_t dst_cap, gzc_str_t *out) {
  if (raw.data == NULL || out == NULL || (dst == NULL && dst_cap != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  if (raw.len < 2 || raw.data[0] != '"' || raw.data[raw.len - 1] != '"') {
    return GZC_ERR_JSON;
  }
  size_t w = 0;
  for (size_t r = 1; r + 1 < raw.len; r++) {
    char c = raw.data[r];
    if (c != '\\') {
      if ((unsigned char)c < 0x20 || c == '"') {
        return GZC_ERR_JSON;
      }
      if (w == dst_cap) {
        return GZC_ERR_BUFFER_TOO_SMALL;
      }
      dst[w++] = c;
      continue;
    }
    if (r + 2 >= raw.len) {
      return GZC_ERR_JSON;
    }
    c = raw.data[++r];
    if (w == dst_cap) {
      return GZC_ERR_BUFFER_TOO_SMALL;
    }
    switch (c) {
    case '"':
    case '\\':
    case '/':
      dst[w++] = c;
      break;
    case 'b':
      dst[w++] = '\b';
      break;
    case 'f':
      dst[w++] = '\f';
      break;
    case 'n':
      dst[w++] = '\n';
      break;
    case 'r':
      dst[w++] = '\r';
      break;
    case 't':
      dst[w++] = '\t';
      break;
    case 'u': {
      uint32_t cp = 0;
      if (r + 5 >= raw.len || read_hex4(raw.data + r + 1, &cp) != GZC_OK) {
        return GZC_ERR_JSON;
      }
      r += 4;
      if (cp >= 0xD800 && cp <= 0xDBFF) {
        uint32_t low = 0;
        if (r + 7 >= raw.len || raw.data[r + 1] != '\\' || raw.data[r + 2] != 'u' ||
            read_hex4(raw.data + r + 3, &low) != GZC_OK || low < 0xDC00 || low > 0xDFFF) {
          return GZC_ERR_JSON;
        }
        r += 6;
        cp = 0x10000 + ((cp - 0xD800) << 10) + (low - 0xDC00);
      } else if (cp >= 0xDC00 && cp <= 0xDFFF) {
        return GZC_ERR_JSON;
      }
      size_t width = cp < 0x80 ? 1 : cp < 0x800 ? 2
                                 : cp < 0x10000 ? 3
                                                : 4;
      if (width > dst_cap - w) {
        return GZC_ERR_BUFFER_TOO_SMALL;
      }
      if (cp < 0x80) {
        dst[w++] = (char)cp;
      } else if (cp < 0x800) {
        dst[w++] = (char)(0xC0 | (cp >> 6));
        dst[w++] = (char)(0x80 | (cp & 0x3F));
      } else if (cp < 0x10000) {
        dst[w++] = (char)(0xE0 | (cp >> 12));
        dst[w++] = (char)(0x80 | ((cp >> 6) & 0x3F));
        dst[w++] = (char)(0x80 | (cp & 0x3F));
      } else {
        dst[w++] = (char)(0xF0 | (cp >> 18));
        dst[w++] = (char)(0x80 | ((cp >> 12) & 0x3F));
        dst[w++] = (char)(0x80 | ((cp >> 6) & 0x3F));
        dst[w++] = (char)(0x80 | (cp & 0x3F));
      }
      break;
    }
    default:
      return GZC_ERR_JSON;
    }
  }
  *out = gzc_str_from_parts(dst, w);
  return GZC_OK;
}

/* --- MHS-inspired v0 ---------------------------------------------------- */

bool gzc_control_mhs_v0_storage_valid(const gzc_control_mhs_v0_storage_t *storage) {
  return storage != NULL && storage->used <= storage->cap && (storage->data != NULL || storage->cap == 0);
}

bool gzc_control_mhs_v0_name_valid(gzc_str_t name) {
  if (name.data == NULL || name.len == 0 || name.len > GZC_CONTROL_MHS_V0_MAX_NAME_BYTES ||
      name.data[0] < 'a' || name.data[0] > 'z') {
    return false;
  }
  bool separator = false;
  for (size_t i = 1; i < name.len; i++) {
    char c = name.data[i];
    bool alnum = (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9');
    if (!alnum && ((c != '.' && c != '-') || separator)) {
      return false;
    }
    separator = !alnum;
  }
  return !separator;
}

/* Strict UTF-8, excluding NUL. The bound is applied separately to values. */
static bool mhs_utf8_valid(gzc_str_t text) {
  if (text.data == NULL && text.len != 0) {
    return false;
  }
  for (size_t i = 0; i < text.len;) {
    uint32_t cp = (unsigned char)text.data[i++];
    if (cp == 0) {
      return false;
    }
    if (cp < 0x80) {
      continue;
    }
    size_t extra;
    uint32_t minimum;
    if (cp >= 0xC2 && cp <= 0xDF) {
      extra = 1;
      minimum = 0x80;
      cp &= 0x1F;
    } else if (cp >= 0xE0 && cp <= 0xEF) {
      extra = 2;
      minimum = 0x800;
      cp &= 0x0F;
    } else if (cp >= 0xF0 && cp <= 0xF4) {
      extra = 3;
      minimum = 0x10000;
      cp &= 0x07;
    } else {
      return false;
    }
    if (extra > text.len - i) {
      return false;
    }
    while (extra-- > 0) {
      unsigned char c = (unsigned char)text.data[i++];
      if ((c & 0xC0) != 0x80) {
        return false;
      }
      cp = (cp << 6) | (c & 0x3F);
    }
    if (cp < minimum || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF)) {
      return false;
    }
  }
  return true;
}

bool gzc_control_mhs_v0_string_valid(gzc_str_t value) {
  return value.len <= GZC_CONTROL_MHS_V0_MAX_STRING_BYTES && mhs_utf8_valid(value);
}

static int mhs_string(gzc_str_t raw, gzc_control_mhs_v0_storage_t *storage, gzc_str_t *out) {
  if (raw.data == NULL || raw.len < 2) {
    return GZC_ERR_JSON;
  }
  for (size_t i = 1; i + 1 < raw.len; i++) {
    if ((unsigned char)raw.data[i] < 0x20) {
      return GZC_ERR_JSON;
    }
  }
  int rc = gzc_json_parse_string(raw, out);
  if (rc == GZC_ERR_UNSUPPORTED) {
    if (storage->data == NULL) {
      return GZC_ERR_BUFFER_TOO_SMALL;
    }
    rc = gzc_control_unescape_string(raw, storage->data + storage->used, storage->cap - storage->used, out);
    if (rc == GZC_OK) {
      storage->used += out->len;
    }
  }
  if (rc == GZC_OK && !mhs_utf8_valid(*out)) {
    return GZC_ERR_JSON;
  }
  return rc;
}

static int mhs_field_string(
    gzc_str_t object, const char *name, bool required,
    gzc_control_mhs_v0_storage_t *storage, gzc_str_t *out) {
  gzc_str_t raw;
  int rc = gzc_json_find_field(object, name, &raw);
  if (rc != GZC_OK) {
    return required ? rc : GZC_OK;
  }
  return mhs_string(raw, storage, out);
}

/* Validate exact decimal integrality before converting: strtod can round a
 * fraction next to 2^53 to an integer, so casting its result is not sufficient.
 * raw has already passed JSON validation and the shared f64 codec's size cap. */
static bool mhs_exact_int(gzc_str_t raw, int64_t *out) {
  char digits[128];
  size_t n = 0;
  size_t i = raw.data[0] == '-' ? 1 : 0;
  bool negative = i != 0;
  bool fraction = false;
  int scale = 0;
  for (; i < raw.len && raw.data[i] != 'e' && raw.data[i] != 'E'; i++) {
    if (raw.data[i] == '.') {
      fraction = true;
      continue;
    }
    digits[n++] = raw.data[i];
    if (fraction) {
      scale--;
    }
  }
  if (i < raw.len) {
    i++;
    bool exponent_negative = raw.data[i] == '-';
    if (raw.data[i] == '-' || raw.data[i] == '+') {
      i++;
    }
    int exponent = 0;
    for (; i < raw.len; i++) {
      if (exponent < 1000) {
        exponent = exponent * 10 + raw.data[i] - '0';
      }
    }
    scale += exponent_negative ? -exponent : exponent;
  }
  size_t first = 0;
  while (first < n && digits[first] == '0') {
    first++;
  }
  if (first == n) {
    *out = 0;
    return true;
  }
  while (scale < 0 && n > first && digits[n - 1] == '0') {
    n--;
    scale++;
  }
  if (scale < 0 || scale > 16 || n - first + (size_t)scale > 16) {
    return false;
  }
  int64_t value = 0;
  for (i = first; i < n; i++) {
    value = value * 10 + digits[i] - '0';
  }
  while (scale-- > 0) {
    value *= 10;
  }
  if (value > GZC_CONTROL_MHS_V0_MAX_INT) {
    return false;
  }
  *out = negative ? -value : value;
  return true;
}

static int mhs_value(gzc_str_t raw, gzc_control_mhs_v0_storage_t *storage, gzc_control_mhs_v0_value_t *out) {
  memset(out, 0, sizeof(*out));
  if (raw.data == NULL || raw.len == 0) {
    return GZC_ERR_JSON;
  }
  if (raw.data[0] == '"') {
    out->kind = GZC_CONTROL_MHS_V0_VALUE_STRING;
    int rc = mhs_string(raw, storage, &out->string_value);
    if (rc == GZC_OK && !gzc_control_mhs_v0_string_valid(out->string_value)) {
      rc = GZC_ERR_JSON;
    }
    return rc;
  }
  if (raw.data[0] == 't' || raw.data[0] == 'f') {
    out->kind = GZC_CONTROL_MHS_V0_VALUE_BOOL;
    return gzc_json_parse_bool(raw, &out->bool_value);
  }
  size_t first = raw.data[0] == '-' ? 1 : 0;
  if (first >= raw.len || raw.data[first] < '0' || raw.data[first] > '9' ||
      (raw.data[first] == '0' && first + 1 < raw.len && raw.data[first + 1] >= '0' && raw.data[first + 1] <= '9')) {
    return GZC_ERR_JSON;
  }
  int rc = gzc_json_parse_f64(raw, &out->double_value);
  if (rc != GZC_OK) {
    return rc;
  }
  if (!isfinite(out->double_value)) {
    return GZC_ERR_JSON;
  }
  out->kind = GZC_CONTROL_MHS_V0_VALUE_DOUBLE;
  out->number_json = raw;
  out->has_int_value = mhs_exact_int(raw, &out->int_value);
  out->number_is_integer_token = memchr(raw.data, '.', raw.len) == NULL &&
                                 memchr(raw.data, 'e', raw.len) == NULL && memchr(raw.data, 'E', raw.len) == NULL;
  if (out->has_int_value && out->number_is_integer_token) {
    out->kind = GZC_CONTROL_MHS_V0_VALUE_INT;
  }
  return GZC_OK;
}

typedef int (*mhs_decode_fn)(gzc_str_t raw, gzc_control_mhs_v0_storage_t *storage, void *out);

static int mhs_array(
    gzc_str_t raw, gzc_control_mhs_v0_storage_t *storage, void *out, size_t stride,
    size_t cap, size_t *count, size_t minimum, size_t maximum, mhs_decode_fn decode) {
  if (!gzc_control_mhs_v0_storage_valid(storage) || count == NULL || (out == NULL && cap != 0) || cap > SIZE_MAX / stride) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  *count = 0;
  if (raw.len == 0) {
    return minimum == 0 ? GZC_OK : GZC_ERR_JSON;
  }
  gzc_json_array_iter_t iter;
  int rc = gzc_json_array_iter_init(raw, &iter);
  /* Count before decoding so an invalid batch is not reported as overflow. */
  size_t length = 0;
  while (rc == GZC_OK) {
    gzc_str_t item;
    bool present = false;
    rc = gzc_json_array_iter_next(&iter, &item, &present);
    if (rc != GZC_OK || !present) {
      break;
    }
    if (length == maximum) {
      return GZC_ERR_JSON;
    }
    length++;
  }
  if (rc != GZC_OK || length < minimum) {
    return rc == GZC_OK ? GZC_ERR_JSON : rc;
  }
  rc = gzc_json_array_iter_init(raw, &iter);
  for (size_t i = 0; rc == GZC_OK && i < length; i++) {
    if (i == cap) {
      return GZC_ERR_BUFFER_TOO_SMALL;
    }
    gzc_str_t item;
    bool present = false;
    rc = gzc_json_array_iter_next(&iter, &item, &present);
    if (rc == GZC_OK) {
      rc = decode(item, storage, (uint8_t *)out + i * stride);
    }
    if (rc == GZC_OK) {
      (*count)++;
    }
  }
  return rc;
}

static int mhs_array_field(gzc_str_t object, const char *name, bool required, gzc_str_t *out) {
  int rc = gzc_json_find_field(object, name, out);
  if (rc != GZC_OK) {
    return required ? rc : GZC_OK;
  }
  gzc_json_array_iter_t iter;
  return gzc_json_array_iter_init(*out, &iter);
}

static int mhs_device(gzc_str_t object, gzc_control_mhs_v0_storage_t *storage, void *out) {
  gzc_control_mhs_v0_device_t *device = out;
  memset(device, 0, sizeof(*device));
  int rc = gzc_json_validate_object(object);
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "id", true, storage, &device->id);
  if (rc == GZC_OK && !gzc_control_mhs_v0_name_valid(device->id))
    rc = GZC_ERR_JSON;
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "kind", true, storage, &device->kind);
  if (rc == GZC_OK && device->kind.len == 0)
    rc = GZC_ERR_JSON;
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "description", false, storage, &device->description);
  if (rc == GZC_OK)
    rc = mhs_array_field(object, "tags", false, &device->tags);
  if (rc == GZC_OK)
    rc = mhs_array_field(object, "states", true, &device->states);
  if (rc == GZC_OK) {
    gzc_json_array_iter_t iter;
    gzc_str_t first;
    bool present = false;
    rc = gzc_json_array_iter_init(device->states, &iter);
    if (rc == GZC_OK)
      rc = gzc_json_array_iter_next(&iter, &first, &present);
    if (rc == GZC_OK && !present)
      rc = GZC_ERR_JSON;
  }
  return rc;
}

static int mhs_constraint(gzc_str_t object, const char *name, bool integer, bool *present, double *out) {
  gzc_str_t raw;
  int rc = gzc_json_find_field(object, name, &raw);
  if (rc != GZC_OK)
    return GZC_OK;
  gzc_control_mhs_v0_value_t value;
  gzc_control_mhs_v0_storage_t storage = {0};
  rc = mhs_value(raw, &storage, &value);
  if (rc != GZC_OK)
    return rc;
  if ((value.kind != GZC_CONTROL_MHS_V0_VALUE_INT && value.kind != GZC_CONTROL_MHS_V0_VALUE_DOUBLE) ||
      (integer && !value.has_int_value))
    return GZC_ERR_JSON;
  *present = true;
  *out = value.double_value;
  return GZC_OK;
}

static int mhs_state(gzc_str_t object, gzc_control_mhs_v0_storage_t *storage, void *out) {
  gzc_control_mhs_v0_state_t *state = out;
  memset(state, 0, sizeof(*state));
  int rc = gzc_json_validate_object(object);
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "name", true, storage, &state->name);
  if (rc == GZC_OK && !gzc_control_mhs_v0_name_valid(state->name))
    rc = GZC_ERR_JSON;
  gzc_str_t type = {0};
  gzc_str_t access = {0};
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "type", true, storage, &type);
  if (rc == GZC_OK) {
    if (gzc_control_str_eq_cstr(type, "bool"))
      state->type = GZC_CONTROL_MHS_V0_TYPE_BOOL;
    else if (gzc_control_str_eq_cstr(type, "int"))
      state->type = GZC_CONTROL_MHS_V0_TYPE_INT;
    else if (gzc_control_str_eq_cstr(type, "double"))
      state->type = GZC_CONTROL_MHS_V0_TYPE_DOUBLE;
    else if (gzc_control_str_eq_cstr(type, "string"))
      state->type = GZC_CONTROL_MHS_V0_TYPE_STRING;
    else if (gzc_control_str_eq_cstr(type, "enum"))
      state->type = GZC_CONTROL_MHS_V0_TYPE_ENUM;
    else
      rc = GZC_ERR_JSON;
  }
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "access", true, storage, &access);
  if (rc == GZC_OK) {
    if (gzc_control_str_eq_cstr(access, "read"))
      state->access = GZC_CONTROL_MHS_V0_ACCESS_READ;
    else if (gzc_control_str_eq_cstr(access, "read_write"))
      state->access = GZC_CONTROL_MHS_V0_ACCESS_READ_WRITE;
    else
      rc = GZC_ERR_JSON;
  }
  bool integer = state->type == GZC_CONTROL_MHS_V0_TYPE_INT;
  if (rc == GZC_OK)
    rc = mhs_constraint(object, "min", integer, &state->has_min, &state->min);
  if (rc == GZC_OK)
    rc = mhs_constraint(object, "max", integer, &state->has_max, &state->max);
  if (rc == GZC_OK)
    rc = mhs_constraint(object, "step", integer, &state->has_step, &state->step);
  if (rc == GZC_OK && ((!integer && state->type != GZC_CONTROL_MHS_V0_TYPE_DOUBLE && (state->has_min || state->has_max || state->has_step)) ||
                       (state->has_min && state->has_max && state->min > state->max) || (state->has_step && state->step <= 0)))
    rc = GZC_ERR_JSON;
  if (rc == GZC_OK)
    rc = mhs_array_field(object, "enum_values", state->type == GZC_CONTROL_MHS_V0_TYPE_ENUM, &state->enum_values);
  if (rc == GZC_OK && state->enum_values.len != 0 && state->type != GZC_CONTROL_MHS_V0_TYPE_ENUM)
    rc = GZC_ERR_JSON;
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "unit", false, storage, &state->unit);
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "description", false, storage, &state->description);
  return rc;
}

static int mhs_string_item(gzc_str_t raw, gzc_control_mhs_v0_storage_t *storage, void *out) {
  return mhs_string(raw, storage, out);
}

static int mhs_enum_item(gzc_str_t raw, gzc_control_mhs_v0_storage_t *storage, void *out) {
  int rc = mhs_string(raw, storage, out);
  return rc == GZC_OK && !gzc_control_mhs_v0_string_valid(*(gzc_str_t *)out) ? GZC_ERR_JSON : rc;
}

static int mhs_ref(gzc_str_t object, gzc_control_mhs_v0_storage_t *storage, void *out) {
  gzc_control_mhs_v0_state_ref_t *ref = out;
  memset(ref, 0, sizeof(*ref));
  int rc = gzc_json_validate_object(object);
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "device_id", true, storage, &ref->device_id);
  if (rc == GZC_OK)
    rc = mhs_field_string(object, "state", true, storage, &ref->state);
  if (rc == GZC_OK && (!gzc_control_mhs_v0_name_valid(ref->device_id) || !gzc_control_mhs_v0_name_valid(ref->state)))
    rc = GZC_ERR_JSON;
  return rc;
}

static int mhs_state_value(gzc_str_t object, gzc_control_mhs_v0_storage_t *storage, void *out) {
  gzc_control_mhs_v0_state_value_t *value = out;
  memset(value, 0, sizeof(*value));
  gzc_control_mhs_v0_state_ref_t ref;
  int rc = mhs_ref(object, storage, &ref);
  if (rc != GZC_OK)
    return rc;
  value->device_id = ref.device_id;
  value->state = ref.state;
  gzc_str_t raw;
  rc = gzc_json_find_field(object, "value", &raw);
  return rc == GZC_OK ? mhs_value(raw, storage, &value->value) : rc;
}

int gzc_control_mhs_v0_decode_devices(
    gzc_str_t object, gzc_control_mhs_v0_storage_t *storage,
    gzc_control_mhs_v0_device_t *out, size_t cap, size_t *count) {
  gzc_str_t raw;
  int rc = gzc_json_validate_object(object);
  if (rc == GZC_OK)
    rc = mhs_array_field(object, "devices", true, &raw);
  return rc == GZC_OK ? mhs_array(raw, storage, out, sizeof(*out), cap, count, 0, SIZE_MAX, mhs_device) : rc;
}

int gzc_control_mhs_v0_decode_states(
    gzc_str_t object, gzc_control_mhs_v0_storage_t *storage,
    gzc_control_mhs_v0_state_value_t *out, size_t cap, size_t *count) {
  gzc_str_t raw;
  int rc = gzc_json_validate_object(object);
  if (rc == GZC_OK)
    rc = mhs_array_field(object, "states", true, &raw);
  return rc == GZC_OK ? mhs_array(raw, storage, out, sizeof(*out), cap, count, 1, GZC_CONTROL_MHS_V0_MAX_BATCH, mhs_state_value) : rc;
}

int gzc_control_mhs_v0_decode_refs(
    gzc_str_t object, gzc_control_mhs_v0_storage_t *storage,
    gzc_control_mhs_v0_state_ref_t *out, size_t cap, size_t *count) {
  gzc_str_t raw;
  int rc = gzc_json_validate_object(object);
  if (rc == GZC_OK)
    rc = mhs_array_field(object, "states", true, &raw);
  return rc == GZC_OK ? mhs_array(raw, storage, out, sizeof(*out), cap, count, 1, GZC_CONTROL_MHS_V0_MAX_BATCH, mhs_ref) : rc;
}

int gzc_control_mhs_v0_device_states(
    const gzc_control_mhs_v0_device_t *device, gzc_control_mhs_v0_storage_t *storage,
    gzc_control_mhs_v0_state_t *out, size_t cap, size_t *out_count) {
  if (device == NULL)
    return GZC_ERR_INVALID_ARGUMENT;
  return mhs_array(device->states, storage, out, sizeof(*out), cap, out_count, 1, SIZE_MAX, mhs_state);
}

int gzc_control_mhs_v0_device_tags(
    const gzc_control_mhs_v0_device_t *device, gzc_control_mhs_v0_storage_t *storage,
    gzc_str_t *out, size_t cap, size_t *out_count) {
  if (device == NULL)
    return GZC_ERR_INVALID_ARGUMENT;
  return mhs_array(device->tags, storage, out, sizeof(*out), cap, out_count, 0, SIZE_MAX, mhs_string_item);
}

int gzc_control_mhs_v0_state_enum_values(
    const gzc_control_mhs_v0_state_t *state, gzc_control_mhs_v0_storage_t *storage,
    gzc_str_t *out, size_t cap, size_t *out_count) {
  if (state == NULL)
    return GZC_ERR_INVALID_ARGUMENT;
  return mhs_array(state->enum_values, storage, out, sizeof(*out), cap, out_count,
                   state->type == GZC_CONTROL_MHS_V0_TYPE_ENUM ? 1 : 0, SIZE_MAX, mhs_enum_item);
}
