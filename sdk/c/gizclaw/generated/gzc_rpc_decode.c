/* Code generated for GizClaw C RPC bindings. DO NOT EDIT. */

#include "gzc_rpc_decode.h"

#include <string.h>

int gzc_ping_response_decode_json(gzc_str_t json, gzc_ping_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "server_time", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->server_time);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_speed_test_response_decode_json(gzc_str_t json, gzc_speed_test_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "down_content_length", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->down_content_length);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "up_content_length", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->up_content_length);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_client_get_info_response_decode_json(gzc_str_t json, gzc_client_get_info_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "hardware_revision", &raw);
  if (rc == GZC_OK) {
    out_value->has_hardware_revision = true;
    rc = gzc_json_parse_string(raw, &out_value->hardware_revision);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "manufacturer", &raw);
  if (rc == GZC_OK) {
    out_value->has_manufacturer = true;
    rc = gzc_json_parse_string(raw, &out_value->manufacturer);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "model", &raw);
  if (rc == GZC_OK) {
    out_value->has_model = true;
    rc = gzc_json_parse_string(raw, &out_value->model);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_client_get_identifiers_response_decode_json(gzc_str_t json, gzc_client_get_identifiers_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "imeis", &raw);
  if (rc == GZC_OK) {
    out_value->has_imeis = true;
    rc = (out_value->imeis.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "labels", &raw);
  if (rc == GZC_OK) {
    out_value->has_labels = true;
    rc = (out_value->labels.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "sn", &raw);
  if (rc == GZC_OK) {
    out_value->has_sn = true;
    rc = gzc_json_parse_string(raw, &out_value->sn);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_get_info_response_decode_json(gzc_str_t json, gzc_server_get_info_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "hardware", &raw);
  if (rc == GZC_OK) {
    out_value->has_hardware = true;
    rc = (out_value->hardware.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "sn", &raw);
  if (rc == GZC_OK) {
    out_value->has_sn = true;
    rc = gzc_json_parse_string(raw, &out_value->sn);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_put_info_response_decode_json(gzc_str_t json, gzc_server_put_info_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "hardware", &raw);
  if (rc == GZC_OK) {
    out_value->has_hardware = true;
    rc = (out_value->hardware.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "sn", &raw);
  if (rc == GZC_OK) {
    out_value->has_sn = true;
    rc = gzc_json_parse_string(raw, &out_value->sn);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_get_runtime_response_decode_json(gzc_str_t json, gzc_server_get_runtime_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "last_addr", &raw);
  if (rc == GZC_OK) {
    out_value->has_last_addr = true;
    rc = gzc_json_parse_string(raw, &out_value->last_addr);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "last_seen_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->last_seen_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "online", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->online);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "rx_bytes", &raw);
  if (rc == GZC_OK) {
    out_value->has_rx_bytes = true;
    rc = gzc_json_parse_i32(raw, &out_value->rx_bytes);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "tx_bytes", &raw);
  if (rc == GZC_OK) {
    out_value->has_tx_bytes = true;
    rc = gzc_json_parse_i32(raw, &out_value->tx_bytes);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_get_status_response_decode_json(gzc_str_t json, gzc_server_get_status_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "battery_percent", &raw);
  if (rc == GZC_OK) {
    out_value->has_battery_percent = true;
    rc = gzc_json_parse_i32(raw, &out_value->battery_percent);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "charging", &raw);
  if (rc == GZC_OK) {
    out_value->has_charging = true;
    rc = gzc_json_parse_bool(raw, &out_value->charging);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "details", &raw);
  if (rc == GZC_OK) {
    out_value->has_details = true;
    rc = (out_value->details.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "gnss_accuracy_m", &raw);
  if (rc == GZC_OK) {
    out_value->has_gnss_accuracy_m = true;
    rc = gzc_json_parse_f64(raw, &out_value->gnss_accuracy_m);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "gnss_altitude_m", &raw);
  if (rc == GZC_OK) {
    out_value->has_gnss_altitude_m = true;
    rc = gzc_json_parse_f64(raw, &out_value->gnss_altitude_m);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "gnss_latitude", &raw);
  if (rc == GZC_OK) {
    out_value->has_gnss_latitude = true;
    rc = gzc_json_parse_f64(raw, &out_value->gnss_latitude);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "gnss_longitude", &raw);
  if (rc == GZC_OK) {
    out_value->has_gnss_longitude = true;
    rc = gzc_json_parse_f64(raw, &out_value->gnss_longitude);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "labels", &raw);
  if (rc == GZC_OK) {
    out_value->has_labels = true;
    rc = (out_value->labels.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "muted", &raw);
  if (rc == GZC_OK) {
    out_value->has_muted = true;
    rc = gzc_json_parse_bool(raw, &out_value->muted);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "reported_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_reported_at = true;
    rc = gzc_json_parse_string(raw, &out_value->reported_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "volume", &raw);
  if (rc == GZC_OK) {
    out_value->has_volume = true;
    rc = gzc_json_parse_i32(raw, &out_value->volume);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_get_run_agent_response_decode_json(gzc_str_t json, gzc_server_get_run_agent_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "active", &raw);
  if (rc == GZC_OK) {
    out_value->has_active = true;
    rc = (out_value->active.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "pending", &raw);
  if (rc == GZC_OK) {
    out_value->has_pending = true;
    rc = (out_value->pending.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_set_run_agent_response_decode_json(gzc_str_t json, gzc_server_set_run_agent_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "active", &raw);
  if (rc == GZC_OK) {
    out_value->has_active = true;
    rc = (out_value->active.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "pending", &raw);
  if (rc == GZC_OK) {
    out_value->has_pending = true;
    rc = (out_value->pending.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_get_run_workspace_response_decode_json(gzc_str_t json, gzc_server_get_run_workspace_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "active_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_active_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->active_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "agent_type", &raw);
  if (rc == GZC_OK) {
    out_value->has_agent_type = true;
    rc = gzc_json_parse_string(raw, &out_value->agent_type);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "history_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_history_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->history_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "memory_stats_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_memory_stats_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->memory_stats_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "pending_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_pending_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->pending_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "recall_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_recall_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->recall_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "runtime_state", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->runtime_state.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "selected_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_selected_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->selected_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "started_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_started_at = true;
    rc = gzc_json_parse_string(raw, &out_value->started_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workflow_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workflow_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workspace_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_set_run_workspace_response_decode_json(gzc_str_t json, gzc_server_set_run_workspace_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "active_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_active_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->active_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "agent_type", &raw);
  if (rc == GZC_OK) {
    out_value->has_agent_type = true;
    rc = gzc_json_parse_string(raw, &out_value->agent_type);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "history_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_history_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->history_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "memory_stats_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_memory_stats_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->memory_stats_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "pending_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_pending_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->pending_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "recall_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_recall_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->recall_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "runtime_state", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->runtime_state.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "selected_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_selected_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->selected_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "started_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_started_at = true;
    rc = gzc_json_parse_string(raw, &out_value->started_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workflow_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workflow_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workspace_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_reload_run_workspace_response_decode_json(gzc_str_t json, gzc_server_reload_run_workspace_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "active_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_active_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->active_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "agent_type", &raw);
  if (rc == GZC_OK) {
    out_value->has_agent_type = true;
    rc = gzc_json_parse_string(raw, &out_value->agent_type);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "history_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_history_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->history_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "memory_stats_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_memory_stats_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->memory_stats_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "pending_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_pending_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->pending_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "recall_available", &raw);
  if (rc == GZC_OK) {
    out_value->has_recall_available = true;
    rc = gzc_json_parse_bool(raw, &out_value->recall_available);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "runtime_state", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->runtime_state.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "selected_workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_selected_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->selected_workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "started_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_started_at = true;
    rc = gzc_json_parse_string(raw, &out_value->started_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workflow_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workflow_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workspace_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_list_run_workspace_history_response_decode_json(gzc_str_t json, gzc_server_list_run_workspace_history_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "available", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->available);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_play_run_workspace_history_response_decode_json(gzc_str_t json, gzc_server_play_run_workspace_history_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "accepted", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->accepted);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "history_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->history_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "state", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->state);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_get_run_workspace_memory_stats_response_decode_json(gzc_str_t json, gzc_server_get_run_workspace_memory_stats_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "available", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->available);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "backend", &raw);
  if (rc == GZC_OK) {
    out_value->has_backend = true;
    rc = gzc_json_parse_string(raw, &out_value->backend);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "embedding_enabled", &raw);
  if (rc == GZC_OK) {
    out_value->has_embedding_enabled = true;
    rc = gzc_json_parse_bool(raw, &out_value->embedding_enabled);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "embedding_status", &raw);
  if (rc == GZC_OK) {
    out_value->has_embedding_status = true;
    rc = gzc_json_parse_string(raw, &out_value->embedding_status);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "enabled", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->enabled);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "index_status", &raw);
  if (rc == GZC_OK) {
    out_value->has_index_status = true;
    rc = gzc_json_parse_string(raw, &out_value->index_status);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "item_count", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->item_count);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "last_updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_last_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->last_updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "metadata", &raw);
  if (rc == GZC_OK) {
    out_value->has_metadata = true;
    rc = (out_value->metadata.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "storage_bytes", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->storage_bytes);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_run_workspace_recall_response_decode_json(gzc_str_t json, gzc_server_run_workspace_recall_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "available", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->available);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "hits", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->hits.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_reload_run_response_decode_json(gzc_str_t json, gzc_server_reload_run_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "started_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_started_at = true;
    rc = gzc_json_parse_string(raw, &out_value->started_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "state", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->state.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_get_run_status_response_decode_json(gzc_str_t json, gzc_server_get_run_status_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "started_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_started_at = true;
    rc = gzc_json_parse_string(raw, &out_value->started_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "state", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->state.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_stop_run_response_decode_json(gzc_str_t json, gzc_server_stop_run_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "started_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_started_at = true;
    rc = gzc_json_parse_string(raw, &out_value->started_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "state", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->state.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_run_say_response_decode_json(gzc_str_t json, gzc_server_run_say_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "accepted", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->accepted);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_firmware_list_response_decode_json(gzc_str_t json, gzc_firmware_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_firmware_get_response_decode_json(gzc_str_t json, gzc_firmware_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "slots", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->slots.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_firmware_files_download_response_decode_json(gzc_str_t json, gzc_firmware_files_download_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "artifact", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->artifact.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "channel", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->channel.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "file", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->file.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "firmware_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->firmware_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "path", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->path);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workspace_list_response_decode_json(gzc_str_t json, gzc_workspace_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_workspace_get_response_decode_json(gzc_str_t json, gzc_workspace_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "last_active_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->last_active_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "parameters", &raw);
  if (rc == GZC_OK) {
    out_value->has_parameters = true;
    rc = (out_value->parameters.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workflow_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workspace_create_response_decode_json(gzc_str_t json, gzc_workspace_create_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "last_active_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->last_active_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "parameters", &raw);
  if (rc == GZC_OK) {
    out_value->has_parameters = true;
    rc = (out_value->parameters.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workflow_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workspace_put_response_decode_json(gzc_str_t json, gzc_workspace_put_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "last_active_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->last_active_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "parameters", &raw);
  if (rc == GZC_OK) {
    out_value->has_parameters = true;
    rc = (out_value->parameters.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workflow_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workspace_delete_response_decode_json(gzc_str_t json, gzc_workspace_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "last_active_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->last_active_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "parameters", &raw);
  if (rc == GZC_OK) {
    out_value->has_parameters = true;
    rc = (out_value->parameters.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workflow_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workspace_history_list_response_decode_json(gzc_str_t json, gzc_workspace_history_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "available", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->available);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "message", &raw);
  if (rc == GZC_OK) {
    out_value->has_message = true;
    rc = gzc_json_parse_string(raw, &out_value->message);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_workspace_history_get_response_decode_json(gzc_str_t json, gzc_workspace_history_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "gear_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_gear_id = true;
    rc = gzc_json_parse_string(raw, &out_value->gear_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "replay_available", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->replay_available);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "text", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->text);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "type", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->type);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workspace_history_audio_get_response_decode_json(gzc_str_t json, gzc_workspace_history_audio_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "history_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->history_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "mime_type", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->mime_type);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "size_bytes", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->size_bytes);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workspace_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workflow_list_response_decode_json(gzc_str_t json, gzc_workflow_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_workflow_get_response_decode_json(gzc_str_t json, gzc_workflow_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "metadata", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->metadata.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "spec", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->spec.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workflow_create_response_decode_json(gzc_str_t json, gzc_workflow_create_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "metadata", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->metadata.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "spec", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->spec.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workflow_put_response_decode_json(gzc_str_t json, gzc_workflow_put_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "metadata", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->metadata.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "spec", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->spec.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_workflow_delete_response_decode_json(gzc_str_t json, gzc_workflow_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "metadata", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->metadata.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "spec", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->spec.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_model_list_response_decode_json(gzc_str_t json, gzc_model_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_model_get_response_decode_json(gzc_str_t json, gzc_model_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "capabilities", &raw);
  if (rc == GZC_OK) {
    out_value->has_capabilities = true;
    rc = (out_value->capabilities.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "kind", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->kind.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->provider.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider_data", &raw);
  if (rc == GZC_OK) {
    out_value->has_provider_data = true;
    rc = (out_value->provider_data.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "source", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->source.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "synced_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_synced_at = true;
    rc = gzc_json_parse_string(raw, &out_value->synced_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_model_create_response_decode_json(gzc_str_t json, gzc_model_create_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "capabilities", &raw);
  if (rc == GZC_OK) {
    out_value->has_capabilities = true;
    rc = (out_value->capabilities.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "kind", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->kind.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->provider.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider_data", &raw);
  if (rc == GZC_OK) {
    out_value->has_provider_data = true;
    rc = (out_value->provider_data.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "source", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->source.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "synced_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_synced_at = true;
    rc = gzc_json_parse_string(raw, &out_value->synced_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_model_put_response_decode_json(gzc_str_t json, gzc_model_put_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "capabilities", &raw);
  if (rc == GZC_OK) {
    out_value->has_capabilities = true;
    rc = (out_value->capabilities.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "kind", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->kind.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->provider.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider_data", &raw);
  if (rc == GZC_OK) {
    out_value->has_provider_data = true;
    rc = (out_value->provider_data.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "source", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->source.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "synced_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_synced_at = true;
    rc = gzc_json_parse_string(raw, &out_value->synced_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_model_delete_response_decode_json(gzc_str_t json, gzc_model_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "capabilities", &raw);
  if (rc == GZC_OK) {
    out_value->has_capabilities = true;
    rc = (out_value->capabilities.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "kind", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->kind.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->provider.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider_data", &raw);
  if (rc == GZC_OK) {
    out_value->has_provider_data = true;
    rc = (out_value->provider_data.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "source", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->source.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "synced_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_synced_at = true;
    rc = gzc_json_parse_string(raw, &out_value->synced_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_voice_list_response_decode_json(gzc_str_t json, gzc_voice_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_voice_get_response_decode_json(gzc_str_t json, gzc_voice_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->provider.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider_data", &raw);
  if (rc == GZC_OK) {
    out_value->has_provider_data = true;
    rc = (out_value->provider_data.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "source", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->source.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "synced_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_synced_at = true;
    rc = gzc_json_parse_string(raw, &out_value->synced_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_credential_list_response_decode_json(gzc_str_t json, gzc_credential_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_credential_get_response_decode_json(gzc_str_t json, gzc_credential_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "body", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->body.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->provider);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_credential_create_response_decode_json(gzc_str_t json, gzc_credential_create_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "body", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->body.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->provider);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_credential_put_response_decode_json(gzc_str_t json, gzc_credential_put_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "body", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->body.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->provider);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_credential_delete_response_decode_json(gzc_str_t json, gzc_credential_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "body", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->body.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "provider", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->provider);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_contact_list_response_decode_json(gzc_str_t json, gzc_contact_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_contact_get_response_decode_json(gzc_str_t json, gzc_contact_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "display_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_display_name = true;
    rc = gzc_json_parse_string(raw, &out_value->display_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "phone_number", &raw);
  if (rc == GZC_OK) {
    out_value->has_phone_number = true;
    rc = gzc_json_parse_string(raw, &out_value->phone_number);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_contact_create_response_decode_json(gzc_str_t json, gzc_contact_create_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "display_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_display_name = true;
    rc = gzc_json_parse_string(raw, &out_value->display_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "phone_number", &raw);
  if (rc == GZC_OK) {
    out_value->has_phone_number = true;
    rc = gzc_json_parse_string(raw, &out_value->phone_number);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_contact_put_response_decode_json(gzc_str_t json, gzc_contact_put_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "display_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_display_name = true;
    rc = gzc_json_parse_string(raw, &out_value->display_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "phone_number", &raw);
  if (rc == GZC_OK) {
    out_value->has_phone_number = true;
    rc = gzc_json_parse_string(raw, &out_value->phone_number);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_contact_delete_response_decode_json(gzc_str_t json, gzc_contact_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "display_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_display_name = true;
    rc = gzc_json_parse_string(raw, &out_value->display_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "phone_number", &raw);
  if (rc == GZC_OK) {
    out_value->has_phone_number = true;
    rc = gzc_json_parse_string(raw, &out_value->phone_number);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_invite_token_get_response_decode_json(gzc_str_t json, gzc_friend_invite_token_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "expires_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_expires_at = true;
    rc = gzc_json_parse_string(raw, &out_value->expires_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "invite_token", &raw);
  if (rc == GZC_OK) {
    out_value->has_invite_token = true;
    rc = gzc_json_parse_string(raw, &out_value->invite_token);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_invite_token_create_response_decode_json(gzc_str_t json, gzc_friend_invite_token_create_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "expires_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->expires_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "invite_token", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->invite_token);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_friend_invite_token_clear_response_decode_json(gzc_str_t json, gzc_friend_invite_token_clear_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  (void)json;
  return GZC_OK;
}

int gzc_friend_add_response_decode_json(gzc_str_t json, gzc_friend_add_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_list_response_decode_json(gzc_str_t json, gzc_friend_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_delete_response_decode_json(gzc_str_t json, gzc_friend_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_list_response_decode_json(gzc_str_t json, gzc_friend_group_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_get_response_decode_json(gzc_str_t json, gzc_friend_group_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_by_peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_by_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->created_by_peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "my_role", &raw);
  if (rc == GZC_OK) {
    out_value->has_my_role = true;
    rc = (out_value->my_role.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_create_response_decode_json(gzc_str_t json, gzc_friend_group_create_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_by_peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_by_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->created_by_peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "my_role", &raw);
  if (rc == GZC_OK) {
    out_value->has_my_role = true;
    rc = (out_value->my_role.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_put_response_decode_json(gzc_str_t json, gzc_friend_group_put_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_by_peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_by_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->created_by_peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "my_role", &raw);
  if (rc == GZC_OK) {
    out_value->has_my_role = true;
    rc = (out_value->my_role.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_delete_response_decode_json(gzc_str_t json, gzc_friend_group_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_by_peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_by_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->created_by_peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "description", &raw);
  if (rc == GZC_OK) {
    out_value->has_description = true;
    rc = gzc_json_parse_string(raw, &out_value->description);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "my_role", &raw);
  if (rc == GZC_OK) {
    out_value->has_my_role = true;
    rc = (out_value->my_role.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc == GZC_OK) {
    out_value->has_name = true;
    rc = gzc_json_parse_string(raw, &out_value->name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workspace_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workspace_name);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_invite_token_get_response_decode_json(gzc_str_t json, gzc_friend_group_invite_token_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "expires_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_expires_at = true;
    rc = gzc_json_parse_string(raw, &out_value->expires_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "invite_token", &raw);
  if (rc == GZC_OK) {
    out_value->has_invite_token = true;
    rc = gzc_json_parse_string(raw, &out_value->invite_token);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_invite_token_create_response_decode_json(gzc_str_t json, gzc_friend_group_invite_token_create_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "expires_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->expires_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "invite_token", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->invite_token);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_friend_group_invite_token_clear_response_decode_json(gzc_str_t json, gzc_friend_group_invite_token_clear_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  (void)json;
  return GZC_OK;
}

int gzc_friend_group_join_response_decode_json(gzc_str_t json, gzc_friend_group_join_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "group", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->group.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "member", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->member.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_friend_group_member_list_response_decode_json(gzc_str_t json, gzc_friend_group_member_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_member_add_response_decode_json(gzc_str_t json, gzc_friend_group_member_add_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "friend_group_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_friend_group_id = true;
    rc = gzc_json_parse_string(raw, &out_value->friend_group_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "role", &raw);
  if (rc == GZC_OK) {
    out_value->has_role = true;
    rc = (out_value->role.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_member_put_response_decode_json(gzc_str_t json, gzc_friend_group_member_put_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "friend_group_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_friend_group_id = true;
    rc = gzc_json_parse_string(raw, &out_value->friend_group_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "role", &raw);
  if (rc == GZC_OK) {
    out_value->has_role = true;
    rc = (out_value->role.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_member_delete_response_decode_json(gzc_str_t json, gzc_friend_group_member_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "friend_group_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_friend_group_id = true;
    rc = gzc_json_parse_string(raw, &out_value->friend_group_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "role", &raw);
  if (rc == GZC_OK) {
    out_value->has_role = true;
    rc = (out_value->role.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_updated_at = true;
    rc = gzc_json_parse_string(raw, &out_value->updated_at);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_message_list_response_decode_json(gzc_str_t json, gzc_friend_group_message_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_message_get_response_decode_json(gzc_str_t json, gzc_friend_group_message_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "audio_content_type", &raw);
  if (rc == GZC_OK) {
    out_value->has_audio_content_type = true;
    rc = gzc_json_parse_string(raw, &out_value->audio_content_type);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "audio_path", &raw);
  if (rc == GZC_OK) {
    out_value->has_audio_path = true;
    rc = gzc_json_parse_string(raw, &out_value->audio_path);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "audio_size_bytes", &raw);
  if (rc == GZC_OK) {
    out_value->has_audio_size_bytes = true;
    rc = gzc_json_parse_i64(raw, &out_value->audio_size_bytes);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "expires_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_expires_at = true;
    rc = gzc_json_parse_string(raw, &out_value->expires_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "friend_group_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_friend_group_id = true;
    rc = gzc_json_parse_string(raw, &out_value->friend_group_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "sender_peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_sender_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->sender_peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "ttl_seconds", &raw);
  if (rc == GZC_OK) {
    out_value->has_ttl_seconds = true;
    rc = gzc_json_parse_i32(raw, &out_value->ttl_seconds);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_friend_group_message_send_response_decode_json(gzc_str_t json, gzc_friend_group_message_send_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "audio_content_type", &raw);
  if (rc == GZC_OK) {
    out_value->has_audio_content_type = true;
    rc = gzc_json_parse_string(raw, &out_value->audio_content_type);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "audio_path", &raw);
  if (rc == GZC_OK) {
    out_value->has_audio_path = true;
    rc = gzc_json_parse_string(raw, &out_value->audio_path);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "audio_size_bytes", &raw);
  if (rc == GZC_OK) {
    out_value->has_audio_size_bytes = true;
    rc = gzc_json_parse_i64(raw, &out_value->audio_size_bytes);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_created_at = true;
    rc = gzc_json_parse_string(raw, &out_value->created_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "expires_at", &raw);
  if (rc == GZC_OK) {
    out_value->has_expires_at = true;
    rc = gzc_json_parse_string(raw, &out_value->expires_at);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "friend_group_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_friend_group_id = true;
    rc = gzc_json_parse_string(raw, &out_value->friend_group_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc == GZC_OK) {
    out_value->has_id = true;
    rc = gzc_json_parse_string(raw, &out_value->id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "sender_peer_public_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_sender_peer_public_key = true;
    rc = gzc_json_parse_string(raw, &out_value->sender_peer_public_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "ttl_seconds", &raw);
  if (rc == GZC_OK) {
    out_value->has_ttl_seconds = true;
    rc = gzc_json_parse_i32(raw, &out_value->ttl_seconds);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_game_ruleset_get_response_decode_json(gzc_str_t json, gzc_server_game_ruleset_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "spec", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->spec.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_pet_def_pixa_download_response_decode_json(gzc_str_t json, gzc_pet_def_pixa_download_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "pixa_path", &raw);
  if (rc == GZC_OK) {
    out_value->has_pixa_path = true;
    rc = gzc_json_parse_string(raw, &out_value->pixa_path);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "size_bytes", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->size_bytes);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_badge_def_pixa_download_response_decode_json(gzc_str_t json, gzc_badge_def_pixa_download_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "pixa_path", &raw);
  if (rc == GZC_OK) {
    out_value->has_pixa_path = true;
    rc = gzc_json_parse_string(raw, &out_value->pixa_path);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "size_bytes", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->size_bytes);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_pet_list_response_decode_json(gzc_str_t json, gzc_server_pet_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_pet_get_response_decode_json(gzc_str_t json, gzc_server_pet_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "ability", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->ability.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "display_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->display_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "exp", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->exp);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "last_active_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->last_active_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "level", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->level);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "life", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->life.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "owner_public_key", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->owner_public_key);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "petdef_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->petdef_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "ruleset_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->ruleset_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workflow_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workflow_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workspace_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_pet_adopt_response_decode_json(gzc_str_t json, gzc_server_pet_adopt_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "pet", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->pet.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "points", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->points.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "transaction", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->transaction.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_pet_put_response_decode_json(gzc_str_t json, gzc_server_pet_put_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "ability", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->ability.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "display_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->display_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "exp", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->exp);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "last_active_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->last_active_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "level", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->level);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "life", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->life.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "owner_public_key", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->owner_public_key);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "petdef_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->petdef_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "ruleset_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->ruleset_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workflow_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workflow_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workspace_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_pet_delete_response_decode_json(gzc_str_t json, gzc_server_pet_delete_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "ability", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->ability.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "display_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->display_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "exp", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->exp);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "last_active_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->last_active_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "level", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->level);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "life", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->life.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "owner_public_key", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->owner_public_key);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "petdef_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->petdef_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "ruleset_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->ruleset_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "workflow_name", &raw);
  if (rc == GZC_OK) {
    out_value->has_workflow_name = true;
    rc = gzc_json_parse_string(raw, &out_value->workflow_name);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "workspace_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->workspace_name);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_pet_drive_response_decode_json(gzc_str_t json, gzc_server_pet_drive_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "badges", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->badges.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "game_result", &raw);
  if (rc == GZC_OK) {
    out_value->has_game_result = true;
    rc = (out_value->game_result.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "pet", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->pet.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "points", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->points.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "reward_grants", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->reward_grants.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "transactions", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->transactions.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_points_get_response_decode_json(gzc_str_t json, gzc_server_points_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "balance", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->balance);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "owner_public_key", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->owner_public_key);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "ruleset_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->ruleset_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_points_transaction_list_response_decode_json(gzc_str_t json, gzc_server_points_transaction_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_points_transaction_get_response_decode_json(gzc_str_t json, gzc_server_points_transaction_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "balance_after", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->balance_after);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "delta", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->delta);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "game_result_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_game_result_id = true;
    rc = gzc_json_parse_string(raw, &out_value->game_result_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "owner_public_key", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->owner_public_key);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "pet_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_pet_id = true;
    rc = gzc_json_parse_string(raw, &out_value->pet_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "reason", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->reason);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "reward_grant_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_reward_grant_id = true;
    rc = gzc_json_parse_string(raw, &out_value->reward_grant_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "ruleset_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->ruleset_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "source_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->source_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "source_type", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->source_type);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_badge_list_response_decode_json(gzc_str_t json, gzc_server_badge_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_badge_get_response_decode_json(gzc_str_t json, gzc_server_badge_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "active", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->active);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "badge_def_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->badge_def_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "exp", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->exp);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "level", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->level);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "owner_public_key", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->owner_public_key);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "progress", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->progress);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "updated_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->updated_at);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}

int gzc_server_game_result_list_response_decode_json(gzc_str_t json, gzc_server_game_result_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_game_result_get_response_decode_json(gzc_str_t json, gzc_server_game_result_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "difficulty", &raw);
  if (rc == GZC_OK) {
    out_value->has_difficulty = true;
    rc = gzc_json_parse_string(raw, &out_value->difficulty);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "duration_ms", &raw);
  if (rc == GZC_OK) {
    out_value->has_duration_ms = true;
    rc = gzc_json_parse_i64(raw, &out_value->duration_ms);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "game_def_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->game_def_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "idempotency_key", &raw);
  if (rc == GZC_OK) {
    out_value->has_idempotency_key = true;
    rc = gzc_json_parse_string(raw, &out_value->idempotency_key);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "max_score", &raw);
  if (rc == GZC_OK) {
    out_value->has_max_score = true;
    rc = gzc_json_parse_i64(raw, &out_value->max_score);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "occurred_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->occurred_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "outcome", &raw);
  if (rc == GZC_OK) {
    out_value->has_outcome = true;
    rc = gzc_json_parse_string(raw, &out_value->outcome);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "owner_public_key", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->owner_public_key);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "payload", &raw);
  if (rc == GZC_OK) {
    out_value->has_payload = true;
    rc = (out_value->payload.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "pet_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->pet_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "ruleset_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->ruleset_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "score", &raw);
  if (rc == GZC_OK) {
    out_value->has_score = true;
    rc = gzc_json_parse_i64(raw, &out_value->score);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_reward_grant_list_response_decode_json(gzc_str_t json, gzc_server_reward_grant_list_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "has_next", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_bool(raw, &out_value->has_next);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "items", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->items.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "next_cursor", &raw);
  if (rc == GZC_OK) {
    out_value->has_next_cursor = true;
    rc = gzc_json_parse_string(raw, &out_value->next_cursor);
    if (rc != GZC_OK) { return rc; }
  }
  return GZC_OK;
}

int gzc_server_reward_grant_get_response_decode_json(gzc_str_t json, gzc_server_reward_grant_get_response_t *out_value) {
  if (out_value == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  memset(out_value, 0, sizeof(*out_value));
  gzc_str_t raw;
  int rc;
  rc = gzc_json_find_field(json, "ability_delta", &raw);
  if (rc == GZC_OK) {
    out_value->has_ability_delta = true;
    rc = (out_value->ability_delta.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "badge_exp_delta", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = (out_value->badge_exp_delta.raw = raw, GZC_OK);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "created_at", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->created_at);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "game_result_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_game_result_id = true;
    rc = gzc_json_parse_string(raw, &out_value->game_result_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "life_delta", &raw);
  if (rc == GZC_OK) {
    out_value->has_life_delta = true;
    rc = (out_value->life_delta.raw = raw, GZC_OK);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "owner_public_key", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->owner_public_key);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "pet_exp_delta", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->pet_exp_delta);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "pet_id", &raw);
  if (rc == GZC_OK) {
    out_value->has_pet_id = true;
    rc = gzc_json_parse_string(raw, &out_value->pet_id);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "points_delta", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_i64(raw, &out_value->points_delta);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "reason", &raw);
  if (rc == GZC_OK) {
    out_value->has_reason = true;
    rc = gzc_json_parse_string(raw, &out_value->reason);
    if (rc != GZC_OK) { return rc; }
  }
  rc = gzc_json_find_field(json, "ruleset_name", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->ruleset_name);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "source_id", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->source_id);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_find_field(json, "source_type", &raw);
  if (rc != GZC_OK) { return rc; }
  rc = gzc_json_parse_string(raw, &out_value->source_type);
  if (rc != GZC_OK) { return rc; }
  return GZC_OK;
}
