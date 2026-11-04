import 'variables.dart';

/// Adds resolved scalar query values while preserving other repeated keys.
String resolveHttpQuery(
  String path,
  Map<Object?, Object?> query,
  Variables variables,
) {
  if (query.isEmpty) {
    return path;
  }
  final parsed = Uri.parse(path);
  final parameters = Map<String, List<String>>.of(parsed.queryParametersAll);
  for (final entry in query.entries) {
    final value = variables.resolve(entry.value);
    if (value is! String && value is! num && value is! bool) {
      throw StateError('HTTP query must be a scalar');
    }
    if (value is num && !value.isFinite) {
      throw StateError('HTTP query must be finite');
    }
    final encoded = value is double && value == value.truncateToDouble()
        ? value.toInt().toString()
        : value.toString();
    parameters[entry.key.toString()] = [encoded];
  }
  return parsed.replace(queryParameters: parameters).toString();
}
