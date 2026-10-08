import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/http_sse.dart';

void main() {
  test('finite SSE projections match shared Go/JavaScript fixtures', () {
    final Object? decoded = jsonDecode(
      File(
        '../../../../api/giztest/testdata/http_sse_vectors.json',
      ).readAsStringSync(),
    );
    expect(decoded, isA<List>());
    if (decoded is! List) {
      throw StateError('invalid fixtures');
    }
    for (final vector in decoded) {
      if (vector is! Map) {
        throw StateError('invalid fixture');
      }
      final text = vector['text'];
      if (text is! String) {
        throw StateError('invalid fixture');
      }
      expect(decodeHttpEventStream(text), vector['expected']);
    }
  });
  test('finite SSE parsing bounds bytes and event counts', () {
    expect(
      () => decodeHttpEventStream('x' * ((4 << 20) + 1)),
      throwsFormatException,
    );
    expect(
      () => decodeHttpEventStream('data:x\n\n' * 16385),
      throwsFormatException,
    );
  });
}
