/// Hand-written models for the `/gizclaw/v1/*` contract in `api/http/peer.json`
/// and the `api/http/shared.json` schemas it references.
///
/// Every model is immutable and decodes with `fromJson`. Unknown JSON keys are
/// ignored. Open-ended objects (`PeerStatus`, `DeviceInfo`) keep a typed core
/// and expose the complete decoded object as `raw` so callers can read fields
/// the SDK does not model.
library;

import 'json.dart';

/// One API key owned by the bound device (`APIKey`).
class ApiKey {
  const ApiKey({
    required this.name,
    required this.displayName,
    required this.prefix,
    required this.apiKey,
    required this.manageApiKeys,
    required this.createdAt,
  });

  factory ApiKey.fromJson(Object? json) {
    final object = asJsonObject(json, 'APIKey');
    return ApiKey(
      name: readString(object, 'name'),
      displayName: readString(object, 'display_name'),
      prefix: readString(object, 'prefix'),
      apiKey: readString(object, 'api_key'),
      manageApiKeys: readBool(object, 'manage_api_keys'),
      createdAt: readDateTime(object, 'created_at'),
    );
  }

  /// Opaque key identity (`key_...`).
  final String name;
  final String displayName;
  final String prefix;

  /// Complete recoverable credential (`gizclaw_sk_v1_...`).
  final String apiKey;
  final bool manageApiKeys;
  final DateTime createdAt;

  JsonObject toJson() => {
    'name': name,
    'display_name': displayName,
    'prefix': prefix,
    'api_key': apiKey,
    'manage_api_keys': manageApiKeys,
    'created_at': encodeDateTime(createdAt),
  };
}

/// Body of `POST /gizclaw/v1/api-keys` (`APIKeyCreateRequest`).
class ApiKeyCreateRequest {
  const ApiKeyCreateRequest({
    required this.displayName,
    required this.manageApiKeys,
  });

  final String displayName;
  final bool manageApiKeys;

  JsonObject toJson() => {
    'display_name': displayName,
    'manage_api_keys': manageApiKeys,
  };
}

/// Result of `POST /gizclaw/v1/api-keys` (`APIKeyCreateResult`).
class ApiKeyCreateResult {
  const ApiKeyCreateResult({required this.value, required this.apiKey});

  factory ApiKeyCreateResult.fromJson(Object? json) {
    final object = asJsonObject(json, 'APIKeyCreateResult');
    return ApiKeyCreateResult(
      value: ApiKey.fromJson(object['value']),
      apiKey: readString(object, 'api_key'),
    );
  }

  final ApiKey value;
  final String apiKey;

  JsonObject toJson() => {'value': value.toJson(), 'api_key': apiKey};
}

/// One page of API keys (`APIKeyList`).
class ApiKeyList {
  const ApiKeyList({required this.items, this.nextCursor});

  factory ApiKeyList.fromJson(Object? json) {
    final object = asJsonObject(json, 'APIKeyList');
    return ApiKeyList(
      items: readList(object, 'items', ApiKey.fromJson),
      nextCursor: readOptionalString(object, 'next_cursor'),
    );
  }

  final List<ApiKey> items;
  final String? nextCursor;

  JsonObject toJson() => withoutNulls({
    'items': items.map((item) => item.toJson()).toList(growable: false),
    'next_cursor': nextCursor,
  });
}

/// Hardware description of the bound device (`HardwareInfo`).
class HardwareInfo {
  const HardwareInfo({this.manufacturer, this.model, this.hardwareRevision});

  factory HardwareInfo.fromJson(Object? json) {
    final object = asJsonObject(json, 'HardwareInfo');
    return HardwareInfo(
      manufacturer: readOptionalString(object, 'manufacturer'),
      model: readOptionalString(object, 'model'),
      hardwareRevision: readOptionalString(object, 'hardware_revision'),
    );
  }

  final String? manufacturer;
  final String? model;
  final String? hardwareRevision;

  JsonObject toJson() => withoutNulls({
    'manufacturer': manufacturer,
    'model': model,
    'hardware_revision': hardwareRevision,
  });
}

/// One IMEI declared by the device (`PeerIMEI`).
class PeerImei {
  const PeerImei({required this.tac, required this.serial, this.name});

  factory PeerImei.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerIMEI');
    return PeerImei(
      tac: readString(object, 'tac'),
      serial: readString(object, 'serial'),
      name: readOptionalString(object, 'name'),
    );
  }

  final String tac;
  final String serial;
  final String? name;

  JsonObject toJson() =>
      withoutNulls({'name': name, 'tac': tac, 'serial': serial});
}

/// One key/value label declared by the device (`PeerLabel`).
class PeerLabel {
  const PeerLabel({required this.key, required this.value});

  factory PeerLabel.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerLabel');
    return PeerLabel(
      key: readString(object, 'key'),
      value: readString(object, 'value'),
    );
  }

  final String key;
  final String value;

  JsonObject toJson() => {'key': key, 'value': value};
}

/// Identifiers declared by the device (`DeviceIdentifiers`).
class DeviceIdentifiers {
  const DeviceIdentifiers({
    this.sn,
    this.imeis = const [],
    this.labels = const [],
  });

  factory DeviceIdentifiers.fromJson(Object? json) {
    final object = asJsonObject(json, 'DeviceIdentifiers');
    return DeviceIdentifiers(
      sn: readOptionalString(object, 'sn'),
      imeis: readList(object, 'imeis', PeerImei.fromJson),
      labels: readList(object, 'labels', PeerLabel.fromJson),
    );
  }

  final String? sn;
  final List<PeerImei> imeis;
  final List<PeerLabel> labels;

  JsonObject toJson() => withoutNulls({
    'sn': sn,
    'imeis': imeis.map((item) => item.toJson()).toList(growable: false),
    'labels': labels.map((item) => item.toJson()).toList(growable: false),
  });
}

/// Identity, hardware, and identifiers of the bound device (`DeviceInfo`).
///
/// The contract leaves `DeviceInfo` open; [raw] holds the complete decoded
/// object for keys this SDK does not model.
class DeviceInfo {
  const DeviceInfo({
    this.name,
    this.emoji,
    this.hardware,
    this.identifiers,
    this.raw = const {},
  });

  factory DeviceInfo.fromJson(Object? json) {
    final object = asJsonObject(json, 'DeviceInfo');
    final hardware = readOptionalObject(object, 'hardware');
    final identifiers = readOptionalObject(object, 'identifiers');
    return DeviceInfo(
      name: readOptionalString(object, 'name'),
      emoji: readOptionalString(object, 'emoji'),
      hardware: hardware == null ? null : HardwareInfo.fromJson(hardware),
      identifiers: identifiers == null
          ? null
          : DeviceIdentifiers.fromJson(identifiers),
      raw: Map.unmodifiable(object),
    );
  }

  final String? name;
  final String? emoji;
  final HardwareInfo? hardware;
  final DeviceIdentifiers? identifiers;

  /// Complete decoded response object, including unmodeled keys.
  final Map<String, Object?> raw;

  JsonObject toJson() => withoutNulls({
    ...raw,
    'name': name,
    'emoji': emoji,
    'hardware': hardware?.toJson(),
    'identifiers': identifiers?.toJson(),
  });
}

/// Online runtime of the bound device (shared `Runtime`).
class DeviceRuntime {
  const DeviceRuntime({
    required this.online,
    required this.lastSeenAt,
    this.lastAddr,
    this.rxBytes,
    this.txBytes,
  });

  factory DeviceRuntime.fromJson(Object? json) {
    final object = asJsonObject(json, 'Runtime');
    return DeviceRuntime(
      online: readBool(object, 'online'),
      lastSeenAt: readDateTime(object, 'last_seen_at'),
      lastAddr: readOptionalString(object, 'last_addr'),
      rxBytes: readOptionalInt(object, 'rx_bytes'),
      txBytes: readOptionalInt(object, 'tx_bytes'),
    );
  }

  final bool online;
  final DateTime lastSeenAt;
  final String? lastAddr;
  final int? rxBytes;
  final int? txBytes;

  JsonObject toJson() => withoutNulls({
    'online': online,
    'last_seen_at': encodeDateTime(lastSeenAt),
    'last_addr': lastAddr,
    'rx_bytes': rxBytes,
    'tx_bytes': txBytes,
  });
}

/// Latest status reported by the device (shared `PeerStatus`).
///
/// Every field is optional in the contract. [labels] and [details] default to
/// empty maps; [raw] holds the complete decoded object.
class PeerStatus {
  const PeerStatus({
    this.audioplayer,
    this.reportedAt,
    this.volume,
    this.muted,
    this.batteryPercent,
    this.charging,
    this.gnssLatitude,
    this.gnssLongitude,
    this.gnssAltitudeM,
    this.gnssAccuracyM,
    this.firmwareSha256,
    this.labels = const {},
    this.details = const {},
    this.raw = const {},
  });

  factory PeerStatus.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerStatus');
    return PeerStatus(
      audioplayer: object['audioplayer'] == null
          ? null
          : AudioPlayerStatus.fromJson(object['audioplayer']),
      reportedAt: readOptionalDateTime(object, 'reported_at'),
      volume: readOptionalInt(object, 'volume'),
      muted: readOptionalBool(object, 'muted'),
      batteryPercent: readOptionalInt(object, 'battery_percent'),
      charging: readOptionalBool(object, 'charging'),
      gnssLatitude: readOptionalDouble(object, 'gnss_latitude'),
      gnssLongitude: readOptionalDouble(object, 'gnss_longitude'),
      gnssAltitudeM: readOptionalDouble(object, 'gnss_altitude_m'),
      gnssAccuracyM: readOptionalDouble(object, 'gnss_accuracy_m'),
      firmwareSha256: readOptionalString(object, 'firmware_sha256'),
      labels: Map.unmodifiable(readStringMap(object, 'labels')),
      details: Map.unmodifiable(readOptionalObject(object, 'details') ?? {}),
      raw: Map.unmodifiable(object),
    );
  }

  final AudioPlayerStatus? audioplayer;
  final DateTime? reportedAt;
  final int? volume;
  final bool? muted;
  final int? batteryPercent;
  final bool? charging;
  final double? gnssLatitude;
  final double? gnssLongitude;
  final double? gnssAltitudeM;
  final double? gnssAccuracyM;

  /// Lowercase SHA-256 digest of the package the device reported running.
  ///
  /// Compare it with `package.sha256` of the channel from [DeviceFirmware] to
  /// tell whether the device already runs that package. The device reports it
  /// on a control response, so it is absent until the device has answered one.
  final String? firmwareSha256;
  final Map<String, String> labels;
  final Map<String, Object?> details;

  /// Complete decoded response object, including unmodeled keys.
  final Map<String, Object?> raw;

  JsonObject toJson() => withoutNulls({
    ...raw,
    'audioplayer': audioplayer?.toJson(),
    'reported_at': reportedAt == null ? null : encodeDateTime(reportedAt!),
    'volume': volume,
    'muted': muted,
    'battery_percent': batteryPercent,
    'charging': charging,
    'gnss_latitude': gnssLatitude,
    'gnss_longitude': gnssLongitude,
    'gnss_altitude_m': gnssAltitudeM,
    'gnss_accuracy_m': gnssAccuracyM,
    'firmware_sha256': firmwareSha256,
    'labels': labels.isEmpty ? null : labels,
    'details': details.isEmpty ? null : details,
  });
}

/// Queryable telemetry field names (`PeerTelemetryField`).
///
/// Responses carry the field as a plain `String` so a server that adds a
/// field keeps decoding; requests accept any string and these constants cover
/// the contract's enumeration.
abstract final class PeerTelemetryField {
  static const batteryPercent = 'battery.percent';
  static const batteryCharging = 'battery.charging';
  static const batteryVoltageMv = 'battery.voltage_mv';
  static const gnssLatitude = 'gnss.latitude';
  static const gnssLongitude = 'gnss.longitude';
  static const gnssAltitudeM = 'gnss.altitude_m';
  static const gnssAccuracyM = 'gnss.accuracy_m';
  static const networkRssiDbm = 'network.rssi_dbm';
  static const networkSignalLevel = 'network.signal_level';
  static const networkConnected = 'network.connected';
  static const systemUptimeSeconds = 'system.uptime_seconds';
  static const systemFreeMemoryBytes = 'system.free_memory_bytes';
  static const systemTemperatureC = 'system.temperature_c';

  static const values = [
    batteryPercent,
    batteryCharging,
    batteryVoltageMv,
    gnssLatitude,
    gnssLongitude,
    gnssAltitudeM,
    gnssAccuracyM,
    networkRssiDbm,
    networkSignalLevel,
    networkConnected,
    systemUptimeSeconds,
    systemFreeMemoryBytes,
    systemTemperatureC,
  ];
}

/// Bucket aggregate mode (`PeerTelemetryAggregate`).
enum PeerTelemetryAggregate {
  avg('avg'),
  min('min'),
  max('max'),
  sum('sum'),
  count('count'),
  last('last');

  const PeerTelemetryAggregate(this.wireValue);

  /// Value sent on the wire.
  final String wireValue;
}

/// Telemetry point ordering (`PeerTelemetryOrder`).
enum PeerTelemetryOrder {
  asc('asc'),
  desc('desc');

  const PeerTelemetryOrder(this.wireValue);

  /// Value sent on the wire.
  final String wireValue;
}

/// Latest value of one telemetry field (`PeerTelemetryValue`).
class PeerTelemetryValue {
  const PeerTelemetryValue({
    required this.field,
    required this.value,
    required this.observedAtUnixMs,
  });

  factory PeerTelemetryValue.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerTelemetryValue');
    return PeerTelemetryValue(
      field: readString(object, 'field'),
      value: readDouble(object, 'value'),
      observedAtUnixMs: readInt(object, 'observed_at_unix_ms'),
    );
  }

  final String field;
  final double value;
  final int observedAtUnixMs;

  JsonObject toJson() => {
    'field': field,
    'value': value,
    'observed_at_unix_ms': observedAtUnixMs,
  };
}

/// One sampled telemetry point (`PeerTelemetryPoint`).
class PeerTelemetryPoint {
  const PeerTelemetryPoint({
    required this.observedAtUnixMs,
    required this.value,
  });

  factory PeerTelemetryPoint.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerTelemetryPoint');
    return PeerTelemetryPoint(
      observedAtUnixMs: readInt(object, 'observed_at_unix_ms'),
      value: readDouble(object, 'value'),
    );
  }

  final int observedAtUnixMs;
  final double value;

  JsonObject toJson() => {
    'observed_at_unix_ms': observedAtUnixMs,
    'value': value,
  };
}

/// One aggregated telemetry bucket (`PeerTelemetryAggregatePoint`).
class PeerTelemetryAggregatePoint {
  const PeerTelemetryAggregatePoint({
    required this.bucketStartTimeMs,
    required this.value,
  });

  factory PeerTelemetryAggregatePoint.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerTelemetryAggregatePoint');
    return PeerTelemetryAggregatePoint(
      bucketStartTimeMs: readInt(object, 'bucket_start_time_ms'),
      value: readDouble(object, 'value'),
    );
  }

  final int bucketStartTimeMs;
  final double value;

  JsonObject toJson() => {
    'bucket_start_time_ms': bucketStartTimeMs,
    'value': value,
  };
}

/// Result of `GET /gizclaw/v1/device/telemetry/{field}/latest`.
class PeerTelemetryLatestResponse {
  const PeerTelemetryLatestResponse({
    required this.peerPublicKey,
    required this.values,
  });

  factory PeerTelemetryLatestResponse.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerTelemetryLatestResponse');
    return PeerTelemetryLatestResponse(
      peerPublicKey: readString(object, 'peer_public_key'),
      values: readList(object, 'values', PeerTelemetryValue.fromJson),
    );
  }

  final String peerPublicKey;
  final List<PeerTelemetryValue> values;

  JsonObject toJson() => {
    'peer_public_key': peerPublicKey,
    'values': values.map((item) => item.toJson()).toList(growable: false),
  };
}

/// Result of `GET /gizclaw/v1/device/telemetry`.
class PeerTelemetryRangeResponse {
  const PeerTelemetryRangeResponse({
    required this.peerPublicKey,
    required this.field,
    required this.startTimeMs,
    required this.endTimeMs,
    required this.stepMs,
    required this.points,
  });

  factory PeerTelemetryRangeResponse.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerTelemetryRangeResponse');
    return PeerTelemetryRangeResponse(
      peerPublicKey: readString(object, 'peer_public_key'),
      field: readString(object, 'field'),
      startTimeMs: readInt(object, 'start_time_ms'),
      endTimeMs: readInt(object, 'end_time_ms'),
      stepMs: readInt(object, 'step_ms'),
      points: readList(object, 'points', PeerTelemetryPoint.fromJson),
    );
  }

  final String peerPublicKey;
  final String field;
  final int startTimeMs;
  final int endTimeMs;
  final int stepMs;
  final List<PeerTelemetryPoint> points;

  JsonObject toJson() => {
    'peer_public_key': peerPublicKey,
    'field': field,
    'start_time_ms': startTimeMs,
    'end_time_ms': endTimeMs,
    'step_ms': stepMs,
    'points': points.map((item) => item.toJson()).toList(growable: false),
  };
}

/// Result of `GET /gizclaw/v1/device/telemetry/aggregate`.
class PeerTelemetryAggregateResponse {
  const PeerTelemetryAggregateResponse({
    required this.peerPublicKey,
    required this.field,
    required this.aggregate,
    required this.bucketMs,
    required this.points,
  });

  factory PeerTelemetryAggregateResponse.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerTelemetryAggregateResponse');
    return PeerTelemetryAggregateResponse(
      peerPublicKey: readString(object, 'peer_public_key'),
      field: readString(object, 'field'),
      aggregate: readString(object, 'aggregate'),
      bucketMs: readInt(object, 'bucket_ms'),
      points: readList(object, 'points', PeerTelemetryAggregatePoint.fromJson),
    );
  }

  final String peerPublicKey;
  final String field;

  /// Aggregate mode as sent on the wire; see [PeerTelemetryAggregate].
  final String aggregate;
  final int bucketMs;
  final List<PeerTelemetryAggregatePoint> points;

  JsonObject toJson() => {
    'peer_public_key': peerPublicKey,
    'field': field,
    'aggregate': aggregate,
    'bucket_ms': bucketMs,
    'points': points.map((item) => item.toJson()).toList(growable: false),
  };
}

/// Body of `PUT /gizclaw/v1/device/volume` (`DeviceVolumeSetRequest`).
class DeviceVolumeSetRequest {
  const DeviceVolumeSetRequest({required this.level, required this.muted});

  /// Absolute volume level from 0 to 100.
  final int level;
  final bool muted;

  JsonObject toJson() => {'level': level, 'muted': muted};
}

/// Status reported by the device after a control command
/// (`DeviceControlStatus`).
class DeviceControlStatus {
  const DeviceControlStatus({required this.status});

  factory DeviceControlStatus.fromJson(Object? json) {
    final object = asJsonObject(json, 'DeviceControlStatus');
    return DeviceControlStatus(status: PeerStatus.fromJson(object['status']));
  }

  final PeerStatus status;

  JsonObject toJson() => {'status': status.toJson()};
}

/// Body of `POST /gizclaw/v1/device/actions/play-sound`
/// (`DevicePlaySoundRequest`).
class DevicePlaySoundRequest {
  const DevicePlaySoundRequest({required this.sound, this.durationMs});

  /// Device-defined sound identifier; at most 32 UTF-8 bytes.
  final String sound;
  final int? durationMs;

  JsonObject toJson() =>
      withoutNulls({'sound': sound, 'duration_ms': durationMs});
}

/// Body of `POST /gizclaw/v1/device/actions/reboot` (`DeviceRebootRequest`).
class DeviceRebootRequest {
  const DeviceRebootRequest({this.delayMs});

  final int? delayMs;

  JsonObject toJson() => withoutNulls({'delay_ms': delayMs});
}

/// Firmware channel name (`FirmwareChannelName`).
enum FirmwareChannelName {
  stable('stable'),
  beta('beta'),
  develop('develop');

  const FirmwareChannelName(this.wireValue);

  /// Value sent on the wire.
  final String wireValue;
}

/// External firmware package configuration (`FirmwarePackage`).
class FirmwarePackage {
  const FirmwarePackage({
    required this.url,
    required this.sha256,
    required this.size,
  });

  factory FirmwarePackage.fromJson(Object? json) {
    final object = asJsonObject(json, 'FirmwarePackage');
    return FirmwarePackage(
      url: readString(object, 'url'),
      sha256: readString(object, 'sha256'),
      size: readInt(object, 'size'),
    );
  }

  /// HTTPS URL of the exact `.tar.zlib` archive bytes.
  final String url;

  /// Lowercase SHA-256 digest of the complete archive bytes.
  final String sha256;

  /// Exact archive size in bytes.
  final int size;

  JsonObject toJson() => {'url': url, 'sha256': sha256, 'size': size};
}

/// One firmware channel of a Firmware configuration (`FirmwareSlot`).
class FirmwareSlot {
  const FirmwareSlot({this.description, this.package});

  factory FirmwareSlot.fromJson(Object? json) {
    final object = asJsonObject(json, 'FirmwareSlot');
    final package = readOptionalObject(object, 'package');
    return FirmwareSlot(
      description: readOptionalString(object, 'description'),
      package: package == null ? null : FirmwarePackage.fromJson(package),
    );
  }

  final String? description;

  /// Package configured for this channel, absent when the channel has none.
  final FirmwarePackage? package;

  JsonObject toJson() =>
      withoutNulls({'description': description, 'package': package?.toJson()});
}

/// Firmware channels configured for the device (`DeviceFirmware`).
///
/// Read with [GizClawControlClient.getDeviceFirmware]. Channel selection
/// belongs to the caller: pick a channel here and name it when calling
/// [GizClawControlClient.updateDeviceFirmware].
class DeviceFirmware {
  const DeviceFirmware({
    required this.stable,
    required this.beta,
    required this.develop,
    this.description,
  });

  factory DeviceFirmware.fromJson(Object? json) {
    final object = asJsonObject(json, 'DeviceFirmware');
    final slots = asJsonObject(object['slots'], 'FirmwareSlots');
    return DeviceFirmware(
      description: readOptionalString(object, 'description'),
      stable: FirmwareSlot.fromJson(slots['stable']),
      beta: FirmwareSlot.fromJson(slots['beta']),
      develop: FirmwareSlot.fromJson(slots['develop']),
    );
  }

  /// Description of the Firmware configuration bound to the device.
  final String? description;

  final FirmwareSlot stable;
  final FirmwareSlot beta;
  final FirmwareSlot develop;

  /// Slot configured for [channel].
  FirmwareSlot slot(FirmwareChannelName channel) => switch (channel) {
    FirmwareChannelName.stable => stable,
    FirmwareChannelName.beta => beta,
    FirmwareChannelName.develop => develop,
  };

  JsonObject toJson() => withoutNulls({
    'description': description,
    'slots': {
      'stable': stable.toJson(),
      'beta': beta.toJson(),
      'develop': develop.toJson(),
    },
  });
}

/// Body of `POST /gizclaw/v1/device/actions/firmware-update`.
class DeviceFirmwareUpdateRequest {
  const DeviceFirmwareUpdateRequest({this.channel, this.sha256});

  /// Channel to install, or null to let the device keep its own channel.
  final FirmwareChannelName? channel;

  /// Package digest the caller saw, or null to skip the device-side check.
  final String? sha256;

  JsonObject toJson() =>
      withoutNulls({'channel': channel?.wireValue, 'sha256': sha256});
}

/// Current Wi-Fi status of the device (`DeviceWifiStatus`).
class DeviceWifiStatus {
  const DeviceWifiStatus({
    required this.connected,
    this.ssid,
    this.rssiDbm,
    this.ip,
    this.bssid,
  });

  factory DeviceWifiStatus.fromJson(Object? json) {
    final object = asJsonObject(json, 'DeviceWifiStatus');
    return DeviceWifiStatus(
      connected: readBool(object, 'connected'),
      ssid: readOptionalString(object, 'ssid'),
      rssiDbm: readOptionalInt(object, 'rssi_dbm'),
      ip: readOptionalString(object, 'ip'),
      bssid: readOptionalString(object, 'bssid'),
    );
  }

  final bool connected;
  final String? ssid;
  final int? rssiDbm;
  final String? ip;
  final String? bssid;

  JsonObject toJson() => withoutNulls({
    'connected': connected,
    'ssid': ssid,
    'rssi_dbm': rssiDbm,
    'ip': ip,
    'bssid': bssid,
  });
}

/// One Wi-Fi network saved on the device (`DeviceWifiSavedNetwork`).
class DeviceWifiSavedNetwork {
  const DeviceWifiSavedNetwork({required this.ssid});

  factory DeviceWifiSavedNetwork.fromJson(Object? json) {
    final object = asJsonObject(json, 'DeviceWifiSavedNetwork');
    return DeviceWifiSavedNetwork(ssid: readString(object, 'ssid'));
  }

  final String ssid;

  JsonObject toJson() => {'ssid': ssid};
}

/// Wi-Fi networks saved on the device (`DeviceWifiSavedList`).
class DeviceWifiSavedList {
  const DeviceWifiSavedList({required this.networks});

  factory DeviceWifiSavedList.fromJson(Object? json) {
    final object = asJsonObject(json, 'DeviceWifiSavedList');
    return DeviceWifiSavedList(
      networks: readList(object, 'networks', DeviceWifiSavedNetwork.fromJson),
    );
  }

  final List<DeviceWifiSavedNetwork> networks;

  JsonObject toJson() => {
    'networks': networks.map((item) => item.toJson()).toList(growable: false),
  };
}

/// Body of `POST /gizclaw/v1/device/wifi/scan`.
class DeviceWifiScanRequest {
  const DeviceWifiScanRequest({this.timeoutMs});

  final int? timeoutMs;

  JsonObject toJson() => withoutNulls({'timeout_ms': timeoutMs});
}

/// One nearby Wi-Fi network reported by the device.
class WifiScanResult {
  const WifiScanResult({
    required this.ssid,
    this.bssid,
    this.rssiDbm,
    this.frequencyMhz,
    this.security,
  });

  factory WifiScanResult.fromJson(Object? json) {
    final object = asJsonObject(json, 'WifiScanResult');
    return WifiScanResult(
      ssid: readString(object, 'ssid'),
      bssid: readOptionalString(object, 'bssid'),
      rssiDbm: readOptionalInt(object, 'rssi_dbm'),
      frequencyMhz: readOptionalInt(object, 'frequency_mhz'),
      security: readOptionalString(object, 'security'),
    );
  }

  final String ssid;
  final String? bssid;
  final int? rssiDbm;
  final int? frequencyMhz;
  final String? security;

  JsonObject toJson() => withoutNulls({
    'ssid': ssid,
    'bssid': bssid,
    'rssi_dbm': rssiDbm,
    'frequency_mhz': frequencyMhz,
    'security': security,
  });
}

/// Nearby Wi-Fi networks returned by a device scan.
class DeviceWifiScanResponse {
  const DeviceWifiScanResponse({required this.networks});

  factory DeviceWifiScanResponse.fromJson(Object? json) {
    final object = asJsonObject(json, 'DeviceWifiScanResponse');
    return DeviceWifiScanResponse(
      networks: readList(object, 'networks', WifiScanResult.fromJson),
    );
  }

  final List<WifiScanResult> networks;

  JsonObject toJson() => {
    'networks': networks.map((item) => item.toJson()).toList(growable: false),
  };
}

/// Body of `PUT /gizclaw/v1/device/wifi`.
class DeviceWifiConnectRequest {
  const DeviceWifiConnectRequest({required this.ssid, this.passphrase});

  final String ssid;
  final String? passphrase;

  JsonObject toJson() => withoutNulls({'ssid': ssid, 'passphrase': passphrase});
}

/// One contact owned by the bound device (`Contact`).
class Contact {
  const Contact({
    required this.name,
    this.displayName,
    this.phoneNumber,
    this.createdAt,
    this.updatedAt,
  });

  factory Contact.fromJson(Object? json) {
    final object = asJsonObject(json, 'Contact');
    return Contact(
      name: readString(object, 'name'),
      displayName: readOptionalString(object, 'display_name'),
      phoneNumber: readOptionalString(object, 'phone_number'),
      createdAt: readOptionalDateTime(object, 'created_at'),
      updatedAt: readOptionalDateTime(object, 'updated_at'),
    );
  }

  /// Owner-scoped immutable contact name.
  final String name;
  final String? displayName;
  final String? phoneNumber;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  JsonObject toJson() => withoutNulls({
    'name': name,
    'display_name': displayName,
    'phone_number': phoneNumber,
    'created_at': createdAt == null ? null : encodeDateTime(createdAt!),
    'updated_at': updatedAt == null ? null : encodeDateTime(updatedAt!),
  });
}

/// One page of contacts (`ContactList`).
class ContactList {
  const ContactList({
    required this.items,
    required this.hasNext,
    this.nextCursor,
  });

  factory ContactList.fromJson(Object? json) {
    final object = asJsonObject(json, 'ContactList');
    return ContactList(
      items: readList(object, 'items', Contact.fromJson),
      hasNext: readBool(object, 'has_next'),
      nextCursor: readOptionalString(object, 'next_cursor'),
    );
  }

  final List<Contact> items;
  final bool hasNext;
  final String? nextCursor;

  JsonObject toJson() => withoutNulls({
    'items': items.map((item) => item.toJson()).toList(growable: false),
    'has_next': hasNext,
    'next_cursor': nextCursor,
  });
}

/// Body of `POST /gizclaw/v1/contacts` (`ContactCreateRequest`).
class ContactCreateRequest {
  const ContactCreateRequest({
    required this.name,
    this.displayName,
    this.phoneNumber,
  });

  final String name;
  final String? displayName;
  final String? phoneNumber;

  JsonObject toJson() => withoutNulls({
    'name': name,
    'display_name': displayName,
    'phone_number': phoneNumber,
  });
}

/// Body of `PUT /gizclaw/v1/contacts/{contactName}` (`ContactPutRequest`).
class ContactPutRequest {
  const ContactPutRequest({this.displayName, this.phoneNumber});

  final String? displayName;
  final String? phoneNumber;

  JsonObject toJson() =>
      withoutNulls({'display_name': displayName, 'phone_number': phoneNumber});
}

/// Error payload carried by every non-2xx response (`ErrorPayload`).
class ErrorPayload {
  const ErrorPayload({
    required this.code,
    required this.message,
    this.details = const {},
  });

  factory ErrorPayload.fromJson(Object? json) {
    final object = asJsonObject(json, 'ErrorPayload');
    return ErrorPayload(
      code: readString(object, 'code'),
      message: readString(object, 'message'),
      details: Map.unmodifiable(readOptionalObject(object, 'details') ?? {}),
    );
  }

  final String code;
  final String message;
  final Map<String, Object?> details;

  JsonObject toJson() => withoutNulls({
    'code': code,
    'message': message,
    'details': details.isEmpty ? null : details,
  });
}

/// Envelope of every non-2xx response (`ErrorResponse`).
class ErrorResponse {
  const ErrorResponse({required this.error});

  factory ErrorResponse.fromJson(Object? json) {
    final object = asJsonObject(json, 'ErrorResponse');
    return ErrorResponse(error: ErrorPayload.fromJson(object['error']));
  }

  final ErrorPayload error;

  JsonObject toJson() => {'error': error.toJson()};
}

/// Device audio player item.
class AudioPlayerItem {
  const AudioPlayerItem({required this.url, this.title, this.sourceRef});
  factory AudioPlayerItem.fromJson(Object? json) {
    final object = asJsonObject(json, 'AudioPlayerItem');
    return AudioPlayerItem(
      url: readString(object, 'url'),
      title: readOptionalString(object, 'title'),
      sourceRef: readOptionalString(object, 'source_ref'),
    );
  }
  final String url;
  final String? title;
  final String? sourceRef;
  JsonObject toJson() =>
      withoutNulls({'url': url, 'title': title, 'source_ref': sourceRef});
}

/// Device audio player snapshot.
class AudioPlayerStatus {
  const AudioPlayerStatus({
    required this.state,
    this.currentIndex,
    required this.positionMs,
    this.durationMs,
    required this.repeat,
    required this.playlistLength,
    required this.playlistRevision,
    this.errorCode,
    this.errorMessage,
    required this.observedAtUnixMs,
  });
  factory AudioPlayerStatus.fromJson(Object? json) {
    final object = asJsonObject(json, 'AudioPlayerStatus');
    return AudioPlayerStatus(
      state: readString(object, 'state'),
      currentIndex: readOptionalInt(object, 'current_index'),
      positionMs: readInt(object, 'position_ms'),
      durationMs: readOptionalInt(object, 'duration_ms'),
      repeat: readString(object, 'repeat'),
      playlistLength: readInt(object, 'playlist_length'),
      playlistRevision: readInt(object, 'playlist_revision'),
      errorCode: readOptionalString(object, 'error_code'),
      errorMessage: readOptionalString(object, 'error_message'),
      observedAtUnixMs: readInt(object, 'observed_at_unix_ms'),
    );
  }
  final String state;
  final int? currentIndex;
  final int positionMs;
  final int? durationMs;
  final String repeat;
  final int playlistLength;
  final int playlistRevision;
  final String? errorCode;
  final String? errorMessage;
  final int observedAtUnixMs;
  JsonObject toJson() => withoutNulls({
    'state': state,
    'current_index': currentIndex,
    'position_ms': positionMs,
    'duration_ms': durationMs,
    'repeat': repeat,
    'playlist_length': playlistLength,
    'playlist_revision': playlistRevision,
    'error_code': errorCode,
    'error_message': errorMessage,
    'observed_at_unix_ms': observedAtUnixMs,
  });
}

/// Acknowledged player status. Playback begins asynchronously.
class AudioPlayerResponse {
  const AudioPlayerResponse({required this.status});
  final AudioPlayerStatus status;
  factory AudioPlayerResponse.fromJson(Object? json) => AudioPlayerResponse(
    status: AudioPlayerStatus.fromJson(
      asJsonObject(json, 'AudioPlayerResponse')['status'],
    ),
  );
}

/// The device's current playlist, in playback order.
class AudioPlayerPlaylist {
  const AudioPlayerPlaylist({
    required this.items,
    required this.playlistRevision,
  });
  final List<AudioPlayerItem> items;
  final int playlistRevision;
  factory AudioPlayerPlaylist.fromJson(Object? json) {
    final object = asJsonObject(json, 'AudioPlayerPlaylist');
    return AudioPlayerPlaylist(
      items: List.unmodifiable(
        asJsonList(object['items'], 'items').map(AudioPlayerItem.fromJson),
      ),
      playlistRevision: readInt(object, 'playlist_revision'),
    );
  }
}
