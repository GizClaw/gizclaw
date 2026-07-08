#ifndef GZC_TELEMETRY_H
#define GZC_TELEMETRY_H

#include "gzc_buffer.h"
#include "gzc_client.h"
#include "gzc_platform.h"

#ifdef __cplusplus
extern "C" {
#endif

#define GZC_PROTOCOL_TELEMETRY ((uint8_t)0x40)

typedef enum {
  GZC_TELEMETRY_OBSERVATION_BATTERY = 1,
  GZC_TELEMETRY_OBSERVATION_GNSS = 2,
  GZC_TELEMETRY_OBSERVATION_NETWORK = 3,
  GZC_TELEMETRY_OBSERVATION_SYSTEM = 4
} gzc_telemetry_observation_kind_t;

typedef struct {
  bool has_percent;
  double percent;
  bool has_charging;
  bool charging;
  bool has_voltage_mv;
  double voltage_mv;
} gzc_telemetry_battery_t;

typedef struct {
  double latitude;
  double longitude;
  bool has_altitude_m;
  double altitude_m;
  bool has_accuracy_m;
  double accuracy_m;
} gzc_telemetry_gnss_t;

typedef struct {
  bool has_rssi_dbm;
  double rssi_dbm;
  bool has_signal_level;
  double signal_level;
  bool has_rat;
  gzc_str_t rat;
  bool has_operator_name;
  gzc_str_t operator_name;
  bool has_connected;
  bool connected;
} gzc_telemetry_network_t;

typedef struct {
  bool has_uptime_seconds;
  double uptime_seconds;
  bool has_free_memory_bytes;
  double free_memory_bytes;
  bool has_temperature_c;
  double temperature_c;
  bool has_firmware_version;
  gzc_str_t firmware_version;
  bool has_software_version;
  gzc_str_t software_version;
  bool has_hardware_version;
  gzc_str_t hardware_version;
} gzc_telemetry_system_t;

typedef struct {
  int32_t observed_at_delta_ms;
  gzc_telemetry_observation_kind_t kind;
  gzc_telemetry_battery_t battery;
  gzc_telemetry_gnss_t gnss;
  gzc_telemetry_network_t network;
  gzc_telemetry_system_t system;
} gzc_telemetry_observation_t;

typedef struct {
  uint32_t sequence;
  int64_t observed_at_unix_ms;
  const gzc_telemetry_observation_t *observations;
  size_t observation_count;
} gzc_telemetry_frame_t;

int gzc_telemetry_encode_frame(const gzc_telemetry_frame_t *frame, const gzc_platform_t *platform, gzc_buf_t *out_payload);
int gzc_client_send_telemetry(gzc_client_t *client, const gzc_telemetry_frame_t *frame);

#ifdef __cplusplus
}
#endif

#endif
