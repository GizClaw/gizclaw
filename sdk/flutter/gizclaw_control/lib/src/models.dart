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
    this.reportedAt,
    this.volume,
    this.muted,
    this.batteryPercent,
    this.charging,
    this.gnssLatitude,
    this.gnssLongitude,
    this.gnssAltitudeM,
    this.gnssAccuracyM,
    this.labels = const {},
    this.details = const {},
    this.raw = const {},
  });

  factory PeerStatus.fromJson(Object? json) {
    final object = asJsonObject(json, 'PeerStatus');
    return PeerStatus(
      reportedAt: readOptionalDateTime(object, 'reported_at'),
      volume: readOptionalInt(object, 'volume'),
      muted: readOptionalBool(object, 'muted'),
      batteryPercent: readOptionalInt(object, 'battery_percent'),
      charging: readOptionalBool(object, 'charging'),
      gnssLatitude: readOptionalDouble(object, 'gnss_latitude'),
      gnssLongitude: readOptionalDouble(object, 'gnss_longitude'),
      gnssAltitudeM: readOptionalDouble(object, 'gnss_altitude_m'),
      gnssAccuracyM: readOptionalDouble(object, 'gnss_accuracy_m'),
      labels: Map.unmodifiable(readStringMap(object, 'labels')),
      details: Map.unmodifiable(readOptionalObject(object, 'details') ?? {}),
      raw: Map.unmodifiable(object),
    );
  }

  final DateTime? reportedAt;
  final int? volume;
  final bool? muted;
  final int? batteryPercent;
  final bool? charging;
  final double? gnssLatitude;
  final double? gnssLongitude;
  final double? gnssAltitudeM;
  final double? gnssAccuracyM;
  final Map<String, String> labels;
  final Map<String, Object?> details;

  /// Complete decoded response object, including unmodeled keys.
  final Map<String, Object?> raw;

  JsonObject toJson() => withoutNulls({
    ...raw,
    'reported_at': reportedAt == null ? null : encodeDateTime(reportedAt!),
    'volume': volume,
    'muted': muted,
    'battery_percent': batteryPercent,
    'charging': charging,
    'gnss_latitude': gnssLatitude,
    'gnss_longitude': gnssLongitude,
    'gnss_altitude_m': gnssAltitudeM,
    'gnss_accuracy_m': gnssAccuracyM,
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

/// Result of `GET /gizclaw/v1/device/telemetry/latest`.
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
