import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/http_sse.dart';
import 'package:giztest/src/assertions.dart';
import 'package:yaml/yaml.dart';

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
  test('SSE evidence retains changed keys without resource payloads', () {
    final projection = decodeHttpEventStream(
      'event: upsert\n'
      'data: {"key":"/gizclaw/v1/device/status","data":{"secret":"private-value"}}\n\n'
      'event: done\ndata: {"timestamp":1791935648850}\n\n',
    );
    final evidence = summarizeHttpEventStream(projection);
    expect(evidence, [
      {'event': 'upsert', 'key': '/gizclaw/v1/device/status'},
      {'event': 'done'},
    ]);
    expect(jsonEncode(evidence), isNot(contains('private-value')));
  });
  test('Peer sync resume permits only an optional live runtime update', () {
    final document = loadYaml(
      File('../../giztest/server.peer.sync.giztest.yaml').readAsStringSync(),
    );
    if (document is! Map || document['steps'] is! List) {
      throw StateError('invalid Peer sync fixture');
    }
    final steps = document['steps'];
    if (steps is! List) throw StateError('missing Peer sync steps');
    final step = steps.whereType<Map>().singleWhere(
      (step) => step['id'] == 'resume_checkpoint',
    );
    final expectations = step['expect'];
    if (expectations is! Map) throw StateError('missing resume assertions');
    final raw = expectations['/raw'];
    if (raw is! Map || raw['pattern'] is! String) {
      throw StateError('missing resume pattern');
    }
    final pattern = raw['pattern'];
    const done = 'event: done\ndata: {"event":"done","timestamp":1}\n\n';
    String upsert(String key, Map<String, Object?> data) =>
        'event: upsert\ndata: ${jsonEncode({'data': data, 'event': 'upsert', 'key': key})}\n\n';
    final runtime = upsert('/gizclaw/v1/device/runtime', {'rx_bytes': 42});
    for (final text in [done, '$runtime$done']) {
      expect(
        () => assertValue(decodeHttpEventStream(text), {
          '/raw': {'pattern': pattern},
        }),
        returnsNormally,
      );
    }
    for (final text in [
      'event: reset\ndata: {"event":"reset"}\n\n$done',
      '$runtime$runtime$done',
      '${upsert('/gizclaw/v1/contacts/changed', {'key': '/gizclaw/v1/device/runtime'})}$done',
      'event: delete\ndata: {"key":"/gizclaw/v1/contacts/changed"}\n\n$done',
    ]) {
      expect(
        () => assertValue(decodeHttpEventStream(text), {
          '/raw': {'pattern': pattern},
        }),
        throwsA(isA<AssertionFailure>()),
      );
    }
  });
}
