/// The Go/C load runner executes scheduling. This SDK contract runner validates
/// and ignores these fields and records timing_mode: ignored in its report.
library;

final _pattern = RegExp(r'^(0|([0-9]+(\.[0-9]+)?(ns|us|µs|μs|ms|s|m|h))+)$');
final _part = RegExp(r'([0-9]+)(?:\.([0-9]+))?(ns|us|µs|μs|ms|s|m|h)');
final _maximum = BigInt.parse('9223372036854775807');
const _units = {
  'h': 3600000000000,
  'm': 60000000000,
  's': 1000000000,
  'ms': 1000000,
  'us': 1000,
  'µs': 1000,
  'μs': 1000,
  'ns': 1,
};

BigInt _duration(Object? value, String field) {
  if (value is! String || !_pattern.hasMatch(value)) {
    throw FormatException('$field must be a non-negative duration');
  }
  var total = BigInt.zero;
  for (final match in _part.allMatches(value)) {
    final fraction = match.group(2) ?? '';
    final scale = BigInt.from(10).pow(fraction.length);
    total +=
        BigInt.parse('${match.group(1)}$fraction') *
        BigInt.from(_units[match.group(3)]!) ~/
        scale;
  }
  if (total > _maximum) {
    throw FormatException('$field exceeds the duration range');
  }
  return total;
}

void validateTiming(Map<String, Object?> fields) {
  BigInt duration(String name) =>
      fields.containsKey(name) ? _duration(fields[name], name) : BigInt.zero;
  final start = duration('start_jitter');
  final stagger = duration('stagger');
  final step = duration('step_jitter');
  final seed = fields['seed'];
  if (fields.containsKey('seed') &&
      (seed is! int || seed < 0 || seed > 9007199254740991)) {
    throw const FormatException(
      'seed must be an integer in 0..9007199254740991',
    );
  }
  final repeat = fields['repeat'] ?? 1;
  if (repeat is int &&
      repeat > 0 &&
      BigInt.from(repeat - 1) * stagger +
              (start > BigInt.zero ? start - BigInt.one : BigInt.zero) >
          _maximum) {
    throw const FormatException(
      '(repeat - 1) * stagger + start_jitter exceeds the duration range',
    );
  }
  final steps = fields['steps'];
  if (steps is List &&
      steps.any((step) => step is Map && step['barrier'] != null) &&
      (start > BigInt.zero || stagger > BigInt.zero || step > BigInt.zero)) {
    throw const FormatException(
      'barrier cannot be combined with non-zero start_jitter, stagger or step_jitter',
    );
  }
}
