import 'dart:convert';

/// Projects a finite SSE response into the shared Giztest HTTP step value.
Map<String, Object?> decodeHttpEventStream(String text) {
  if (utf8.encode(text).length > 4 << 20) {
    throw const FormatException('giztest: SSE response exceeds 4 MiB');
  }
  final events = <Map<String, Object?>>[];
  final result = <String, Object?>{'events': events, 'raw': text};
  var pending = text
      .replaceFirst(RegExp(r'^\uFEFF'), '')
      .replaceAll(RegExp(r'\r\n?'), '\n');
  var name = 'message';
  var data = <String>[];
  while (true) {
    final end = pending.indexOf('\n');
    if (end < 0) break;
    final line = pending.substring(0, end);
    pending = pending.substring(end + 1);
    if (line.isEmpty) {
      if (data.isNotEmpty) {
        if (events.length >= 16384) {
          throw const FormatException(
            'giztest: SSE response exceeds 16384 events',
          );
        }
        final text = data.join('\n');
        Object? value;
        try {
          value = jsonDecode(text);
        } on FormatException {
          value = text;
        }
        final event = <String, Object?>{'event': name, 'data': value};
        events.add(event);
        result['last_event'] = event;
      }
      name = 'message';
      data = <String>[];
      continue;
    }
    final separator = line.indexOf(':');
    final field = separator < 0 ? line : line.substring(0, separator);
    final value = separator < 0
        ? ''
        : line.substring(separator + 1).replaceFirst(RegExp(r'^ '), '');
    if (field == 'event') name = value.isEmpty ? 'message' : value;
    if (field == 'data') data.add(value);
  }
  return result;
}
