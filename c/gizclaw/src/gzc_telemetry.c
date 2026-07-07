#include "gzc_telemetry.h"

#include <string.h>

enum {
  gzc_pb_wire_varint = 0,
  gzc_pb_wire_fixed64 = 1,
  gzc_pb_wire_len = 2
};

static int telemetry_append_varint(gzc_buf_t *buf, const gzc_platform_t *platform, uint64_t value) {
  uint8_t data[10];
  size_t len = 0;
  do {
    uint8_t byte = (uint8_t)(value & 0x7f);
    value >>= 7;
    if (value != 0) {
      byte |= 0x80;
    }
    data[len++] = byte;
  } while (value != 0);
  return gzc_buf_append(buf, platform, data, len);
}

static int telemetry_append_tag(gzc_buf_t *buf, const gzc_platform_t *platform, uint32_t field_number, uint8_t wire_type) {
  return telemetry_append_varint(buf, platform, ((uint64_t)field_number << 3) | wire_type);
}

static int telemetry_append_uint32(gzc_buf_t *buf, const gzc_platform_t *platform, uint32_t field_number, uint32_t value) {
  int rc = telemetry_append_tag(buf, platform, field_number, gzc_pb_wire_varint);
  if (rc != GZC_OK) {
    return rc;
  }
  return telemetry_append_varint(buf, platform, value);
}

static int telemetry_append_int64(gzc_buf_t *buf, const gzc_platform_t *platform, uint32_t field_number, int64_t value) {
  int rc = telemetry_append_tag(buf, platform, field_number, gzc_pb_wire_varint);
  if (rc != GZC_OK) {
    return rc;
  }
  return telemetry_append_varint(buf, platform, (uint64_t)value);
}

static int telemetry_append_int32(gzc_buf_t *buf, const gzc_platform_t *platform, uint32_t field_number, int32_t value) {
  return telemetry_append_int64(buf, platform, field_number, (int64_t)value);
}

static int telemetry_append_bool(gzc_buf_t *buf, const gzc_platform_t *platform, uint32_t field_number, bool value) {
  int rc = telemetry_append_tag(buf, platform, field_number, gzc_pb_wire_varint);
  if (rc != GZC_OK) {
    return rc;
  }
  return telemetry_append_varint(buf, platform, value ? 1 : 0);
}

static int telemetry_append_double(gzc_buf_t *buf, const gzc_platform_t *platform, uint32_t field_number, double value) {
  int rc = telemetry_append_tag(buf, platform, field_number, gzc_pb_wire_fixed64);
  if (rc != GZC_OK) {
    return rc;
  }
  uint64_t bits = 0;
  uint8_t data[8];
  memcpy(&bits, &value, sizeof(bits));
  for (size_t i = 0; i < sizeof(data); i++) {
    data[i] = (uint8_t)((bits >> (i * 8)) & 0xff);
  }
  return gzc_buf_append(buf, platform, data, sizeof(data));
}

static int telemetry_append_string(gzc_buf_t *buf, const gzc_platform_t *platform, uint32_t field_number, gzc_str_t value) {
  if (value.data == NULL && value.len != 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  int rc = telemetry_append_tag(buf, platform, field_number, gzc_pb_wire_len);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = telemetry_append_varint(buf, platform, value.len);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append(buf, platform, value.data, value.len);
}

static int telemetry_append_message(gzc_buf_t *buf, const gzc_platform_t *platform, uint32_t field_number, const gzc_buf_t *message) {
  int rc = telemetry_append_tag(buf, platform, field_number, gzc_pb_wire_len);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = telemetry_append_varint(buf, platform, message->len);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append(buf, platform, message->data, message->len);
}

static int telemetry_encode_battery(const gzc_telemetry_battery_t *battery, const gzc_platform_t *platform, gzc_buf_t *out) {
  if (battery->has_percent) {
    int rc = telemetry_append_double(out, platform, 1, battery->percent);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (battery->has_charging) {
    int rc = telemetry_append_bool(out, platform, 2, battery->charging);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (battery->has_voltage_mv) {
    int rc = telemetry_append_double(out, platform, 3, battery->voltage_mv);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_OK;
}

static int telemetry_encode_gnss(const gzc_telemetry_gnss_t *gnss, const gzc_platform_t *platform, gzc_buf_t *out) {
  int rc = telemetry_append_double(out, platform, 1, gnss->latitude);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = telemetry_append_double(out, platform, 2, gnss->longitude);
  if (rc != GZC_OK) {
    return rc;
  }
  if (gnss->has_altitude_m) {
    rc = telemetry_append_double(out, platform, 3, gnss->altitude_m);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (gnss->has_accuracy_m) {
    rc = telemetry_append_double(out, platform, 4, gnss->accuracy_m);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_OK;
}

static int telemetry_encode_network(const gzc_telemetry_network_t *network, const gzc_platform_t *platform, gzc_buf_t *out) {
  if (network->has_rssi_dbm) {
    int rc = telemetry_append_double(out, platform, 1, network->rssi_dbm);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (network->has_signal_level) {
    int rc = telemetry_append_double(out, platform, 2, network->signal_level);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (network->has_rat) {
    int rc = telemetry_append_string(out, platform, 3, network->rat);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (network->has_operator_name) {
    int rc = telemetry_append_string(out, platform, 4, network->operator_name);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (network->has_connected) {
    int rc = telemetry_append_bool(out, platform, 5, network->connected);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_OK;
}

static int telemetry_encode_system(const gzc_telemetry_system_t *system, const gzc_platform_t *platform, gzc_buf_t *out) {
  if (system->has_uptime_seconds) {
    int rc = telemetry_append_double(out, platform, 1, system->uptime_seconds);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (system->has_free_memory_bytes) {
    int rc = telemetry_append_double(out, platform, 2, system->free_memory_bytes);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (system->has_temperature_c) {
    int rc = telemetry_append_double(out, platform, 3, system->temperature_c);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (system->has_firmware_version) {
    int rc = telemetry_append_string(out, platform, 4, system->firmware_version);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (system->has_software_version) {
    int rc = telemetry_append_string(out, platform, 5, system->software_version);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (system->has_hardware_version) {
    int rc = telemetry_append_string(out, platform, 6, system->hardware_version);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_OK;
}

static int telemetry_encode_observation(const gzc_telemetry_observation_t *observation, const gzc_platform_t *platform, gzc_buf_t *out) {
  if (observation->observed_at_delta_ms != 0) {
    int rc = telemetry_append_int32(out, platform, 1, observation->observed_at_delta_ms);
    if (rc != GZC_OK) {
      return rc;
    }
  }

  gzc_buf_t body;
  gzc_buf_init(&body);
  uint32_t body_field = 0;
  int rc = GZC_OK;
  switch (observation->kind) {
  case GZC_TELEMETRY_OBSERVATION_BATTERY:
    body_field = 10;
    rc = telemetry_encode_battery(&observation->battery, platform, &body);
    break;
  case GZC_TELEMETRY_OBSERVATION_GNSS:
    body_field = 11;
    rc = telemetry_encode_gnss(&observation->gnss, platform, &body);
    break;
  case GZC_TELEMETRY_OBSERVATION_NETWORK:
    body_field = 12;
    rc = telemetry_encode_network(&observation->network, platform, &body);
    break;
  case GZC_TELEMETRY_OBSERVATION_SYSTEM:
    body_field = 13;
    rc = telemetry_encode_system(&observation->system, platform, &body);
    break;
  default:
    rc = GZC_ERR_INVALID_ARGUMENT;
    break;
  }
  if (rc == GZC_OK) {
    rc = telemetry_append_message(out, platform, body_field, &body);
  }
  gzc_buf_free(&body, platform);
  return rc;
}

int gzc_telemetry_encode_frame(const gzc_telemetry_frame_t *frame, const gzc_platform_t *platform, gzc_buf_t *out_payload) {
  if (frame == NULL || out_payload == NULL || frame->observation_count == 0 ||
      (frame->observations == NULL && frame->observation_count != 0)) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *alloc = platform == NULL ? gzc_default_platform() : platform;
  gzc_buf_reset(out_payload);
  if (frame->sequence != 0) {
    int rc = telemetry_append_uint32(out_payload, alloc, 1, frame->sequence);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  if (frame->observed_at_unix_ms != 0) {
    int rc = telemetry_append_int64(out_payload, alloc, 2, frame->observed_at_unix_ms);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  for (size_t i = 0; i < frame->observation_count; i++) {
    gzc_buf_t observation;
    gzc_buf_init(&observation);
    int rc = telemetry_encode_observation(&frame->observations[i], alloc, &observation);
    if (rc == GZC_OK) {
      rc = telemetry_append_message(out_payload, alloc, 3, &observation);
    }
    gzc_buf_free(&observation, alloc);
    if (rc != GZC_OK) {
      return rc;
    }
  }
  return GZC_OK;
}

int gzc_client_send_telemetry(gzc_client_t *client, const gzc_telemetry_frame_t *frame) {
  if (client == NULL || frame == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  const gzc_platform_t *platform = gzc_client_platform(client);
  gzc_telemetry_frame_t stamped_frame = *frame;
  if (stamped_frame.observed_at_unix_ms == 0 && platform != NULL && platform->time_unix_ms != NULL) {
    stamped_frame.observed_at_unix_ms = platform->time_unix_ms(platform->userdata);
  }
  gzc_buf_t payload;
  gzc_buf_init(&payload);
  int rc = gzc_telemetry_encode_frame(&stamped_frame, platform, &payload);
  if (rc == GZC_OK) {
    rc = gzc_client_send_packet(client, GZC_PROTOCOL_TELEMETRY, payload.data, payload.len);
  }
  gzc_buf_free(&payload, platform);
  return rc;
}
