/// Strict readers for untrusted JSON objects.
///
/// Every reader throws [FormatException] when a required key is missing or a
/// value has the wrong JSON type. Keys that a model does not know are ignored
/// by construction: readers only look up the keys they are asked for.
library;

typedef JsonObject = Map<String, Object?>;

JsonObject asJsonObject(Object? value, String context) {
  if (value is Map<String, Object?>) {
    return value;
  }
  if (value is Map) {
    return value.map((key, item) => MapEntry(key.toString(), item));
  }
  throw FormatException('$context: expected a JSON object');
}

List<Object?> asJsonList(Object? value, String context) {
  if (value is List<Object?>) {
    return value;
  }
  throw FormatException('$context: expected a JSON array');
}

String readString(JsonObject json, String key) {
  final value = json[key];
  if (value is String) {
    return value;
  }
  throw FormatException('$key: expected a string');
}

String? readOptionalString(JsonObject json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  if (value is String) {
    return value;
  }
  throw FormatException('$key: expected a string');
}

bool readBool(JsonObject json, String key) {
  final value = json[key];
  if (value is bool) {
    return value;
  }
  throw FormatException('$key: expected a boolean');
}

bool? readOptionalBool(JsonObject json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  if (value is bool) {
    return value;
  }
  throw FormatException('$key: expected a boolean');
}

int readInt(JsonObject json, String key) {
  final value = readOptionalInt(json, key);
  if (value == null) {
    throw FormatException('$key: expected an integer');
  }
  return value;
}

int? readOptionalInt(JsonObject json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  if (value is int) {
    return value;
  }
  if (value is double && value == value.truncateToDouble()) {
    return value.toInt();
  }
  throw FormatException('$key: expected an integer');
}

double readDouble(JsonObject json, String key) {
  final value = readOptionalDouble(json, key);
  if (value == null) {
    throw FormatException('$key: expected a number');
  }
  return value;
}

double? readOptionalDouble(JsonObject json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  if (value is num) {
    return value.toDouble();
  }
  throw FormatException('$key: expected a number');
}

DateTime readDateTime(JsonObject json, String key) {
  final value = readOptionalDateTime(json, key);
  if (value == null) {
    throw FormatException('$key: expected an RFC 3339 timestamp');
  }
  return value;
}

DateTime? readOptionalDateTime(JsonObject json, String key) {
  final value = readOptionalString(json, key);
  if (value == null) {
    return null;
  }
  final parsed = DateTime.tryParse(value);
  if (parsed == null) {
    throw FormatException('$key: expected an RFC 3339 timestamp');
  }
  return parsed;
}

List<T> readList<T>(
  JsonObject json,
  String key,
  T Function(Object? item) decode,
) {
  final value = json[key];
  if (value == null) {
    return const [];
  }
  return asJsonList(value, key).map(decode).toList(growable: false);
}

JsonObject? readOptionalObject(JsonObject json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  return asJsonObject(value, key);
}

Map<String, String> readStringMap(JsonObject json, String key) {
  final value = readOptionalObject(json, key);
  if (value == null) {
    return const {};
  }
  return value.map((entryKey, entryValue) {
    if (entryValue is! String) {
      throw FormatException('$key.$entryKey: expected a string');
    }
    return MapEntry(entryKey, entryValue);
  });
}

/// Returns a JSON object without the entries whose value is `null`.
JsonObject withoutNulls(JsonObject json) {
  return {
    for (final entry in json.entries)
      if (entry.value != null) entry.key: entry.value,
  };
}

/// Encodes [value] as an RFC 3339 UTC timestamp, without fractional seconds
/// when they are zero.
String encodeDateTime(DateTime value) {
  final utc = value.toUtc();
  if (utc.millisecond == 0 && utc.microsecond == 0) {
    return '${utc.toIso8601String().substring(0, 19)}Z';
  }
  return utc.toIso8601String();
}
