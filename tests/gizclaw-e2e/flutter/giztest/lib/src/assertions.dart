/// JSON-pointer resolution and expectation matching for scenario steps.
///
/// Assertion messages never echo the asserted content, matching the Go runner:
/// a failure names the pointer and the operator only.
library;

import 'dart:convert';

/// Marks an expectation or declared-status failure, which the report
/// distinguishes from an operation failure.
class AssertionFailure implements Exception {
  const AssertionFailure(this.message);

  final String message;

  @override
  String toString() => message;
}

class PointerResult {
  const PointerResult(this.found, this.value);

  final bool found;
  final Object? value;
}

PointerResult jsonPointer(Object? input, String pointer) {
  if (pointer.isEmpty) {
    return PointerResult(true, input);
  }
  var current = input;
  final body = pointer.startsWith('/') ? pointer.substring(1) : pointer;
  for (final raw in body.split('/')) {
    final part = raw.replaceAll('~1', '/').replaceAll('~0', '~');
    if (current is List) {
      final index = int.tryParse(part);
      if (index == null || index < 0 || index >= current.length) {
        return const PointerResult(false, null);
      }
      current = current[index];
      continue;
    }
    if (current is Map) {
      if (!current.containsKey(part)) {
        return const PointerResult(false, null);
      }
      current = current[part];
      continue;
    }
    return const PointerResult(false, null);
  }
  return PointerResult(true, current);
}

/// Compares two values by their canonical JSON text, so numbers and strings
/// never compare equal and object key order does not matter.
bool jsonEqual(Object? left, Object? right) =>
    _stableJson(left) == _stableJson(right);

String _stableJson(Object? value) {
  if (value is List) {
    return '[${value.map(_stableJson).join(',')}]';
  }
  if (value is Map) {
    final keys = value.keys.map((key) => key.toString()).toList()..sort();
    return '{${keys.map((key) => '${jsonEncode(key)}:${_stableJson(value[key])}').join(',')}}';
  }
  if (value is num && value == value.roundToDouble() && value.abs() < 1e15) {
    return value.toInt().toString();
  }
  return jsonEncode(value);
}

/// Coerces a value to matchable text: a string, or a list of strings joined
/// without a separator.
String? _stringTarget(Object? value) {
  if (value is String) {
    return value;
  }
  if (value is List) {
    final buffer = StringBuffer();
    for (final item in value) {
      if (item is! String) {
        return null;
      }
      buffer.write(item);
    }
    return buffer.toString();
  }
  return null;
}

/// Accepts numbers and the decimal strings protojson emits for 64-bit integer
/// fields.
double? _numericTarget(Object? value) {
  if (value is num) {
    return value.isFinite ? value.toDouble() : null;
  }
  if (value is String) {
    final parsed = double.tryParse(value.trim());
    return parsed != null && parsed.isFinite ? parsed : null;
  }
  return null;
}

bool _isEmpty(Object? value) {
  if (value == null) return true;
  if (value is String) return value.isEmpty;
  if (value is List) return value.isEmpty;
  if (value is Map) return value.isEmpty;
  return false;
}

List<String> _needles(Object? value) {
  if (value is String) return [value];
  if (value is List) return value.map((item) => item as String).toList();
  return const [];
}

void _assertStringMatchers(
  String path,
  Object? value,
  Map<String, Object?> expectation,
) {
  const keys = [
    'contains',
    'contains_all',
    'contains_any',
    'not_contains',
    'pattern',
    'min_length',
    'max_length',
  ];
  if (!keys.any(expectation.containsKey)) {
    return;
  }
  final text = _stringTarget(value);
  if (text == null) {
    throw AssertionFailure(
      'assert $path requires a string or text-fragment target',
    );
  }
  final contains = expectation['contains'];
  if (contains != null && !text.contains(contains as String)) {
    throw AssertionFailure('assert $path contains failed');
  }
  final all = expectation['contains_all'];
  if (all != null && !_needles(all).every(text.contains)) {
    throw AssertionFailure('assert $path contains_all failed');
  }
  final any = expectation['contains_any'];
  if (any != null && !_needles(any).any(text.contains)) {
    throw AssertionFailure('assert $path contains_any failed');
  }
  final forbidden = expectation['not_contains'];
  if (forbidden != null && _needles(forbidden).any(text.contains)) {
    throw AssertionFailure('assert $path not_contains failed');
  }
  final pattern = expectation['pattern'];
  if (pattern != null && !RegExp(pattern as String).hasMatch(text)) {
    throw AssertionFailure('assert $path pattern failed');
  }
  final length = text.runes.length;
  final minLength = expectation['min_length'];
  if (minLength != null && length < (minLength as int)) {
    throw AssertionFailure('assert $path min_length failed');
  }
  final maxLength = expectation['max_length'];
  if (maxLength != null && length > (maxLength as int)) {
    throw AssertionFailure('assert $path max_length failed');
  }
}

void _assertNumericBounds(
  String path,
  Object? value,
  Map<String, Object?> expectation,
) {
  final minimum = expectation['minimum'];
  final maximum = expectation['maximum'];
  if (minimum == null && maximum == null) {
    return;
  }
  final number = _numericTarget(value);
  if (number == null) {
    throw AssertionFailure('assert $path requires a numeric target');
  }
  if (minimum != null && number < (minimum as num)) {
    throw AssertionFailure('assert $path minimum failed');
  }
  if (maximum != null && number > (maximum as num)) {
    throw AssertionFailure('assert $path maximum failed');
  }
}

void assertValue(
  Object? input,
  Map<String, Map<String, Object?>> expectations,
) {
  for (final entry in expectations.entries) {
    final path = entry.key;
    final expectation = entry.value;
    final result = jsonPointer(input, path);
    final present = expectation['present'] as bool?;
    if (present != null && result.found != present) {
      // The Go runner reports the observed presence, not the expected one.
      throw AssertionFailure('assert $path presence = ${result.found}');
    }
    if (!result.found) {
      if (present == false) {
        continue;
      }
      throw AssertionFailure('assert path $path not found');
    }
    if (expectation.containsKey('equals') &&
        !jsonEqual(result.value, expectation['equals'])) {
      throw AssertionFailure('assert $path equals failed');
    }
    final count = expectation['count'] as int?;
    if (count != null) {
      final value = result.value;
      if (value is! List || value.length != count) {
        throw AssertionFailure('assert $path count failed');
      }
    }
    final nonEmpty = expectation['non_empty'] as bool?;
    if (nonEmpty != null && _isEmpty(result.value) == nonEmpty) {
      throw AssertionFailure('assert $path non_empty failed');
    }
    _assertStringMatchers(path, result.value, expectation);
    _assertNumericBounds(path, result.value, expectation);
  }
}

/// Redacts secrets, blanks any message naming a credential concept, and caps
/// the length, matching the Go runner's report sanitation.
String safeError(Object? error, [List<String> redactions = const []]) {
  if (error == null) {
    return '';
  }
  var text = error is AssertionFailure ? error.message : error.toString();
  for (final secret in redactions) {
    if (secret.isNotEmpty) {
      text = text.replaceAll(secret, '[REDACTED]');
    }
  }
  final lowered = text.toLowerCase();
  for (final word in const [
    'token',
    'credential',
    'authorization',
    'private_key',
  ]) {
    if (lowered.contains(word)) {
      return 'redacted execution error';
    }
  }
  return text.length > 512 ? text.substring(0, 512) : text;
}
