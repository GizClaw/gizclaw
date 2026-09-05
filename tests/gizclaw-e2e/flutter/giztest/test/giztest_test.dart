import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/assertions.dart';
import 'package:giztest/src/document.dart';
import 'package:giztest/src/variables.dart';

final scenarioRoot = Directory('../../giztest').absolute.path;

void main() {
  group('jsonPointer', () {
    test('resolves objects, arrays and escapes', () {
      final input = {
        'a/b': 1,
        'n~m': 2,
        'items': [
          {'value': 'first'},
          {'value': 'second'},
        ],
        'nested': {'empty': ''},
      };
      expect(jsonPointer(input, '').value, same(input));
      expect(jsonPointer(input, '/items/1/value').value, 'second');
      expect(jsonPointer(input, '/a~1b').value, 1);
      expect(jsonPointer(input, '/n~0m').value, 2);
      expect(jsonPointer(input, '/items/2').found, isFalse);
      expect(jsonPointer(input, '/items/-1').found, isFalse);
      // A token must parse whole, so a numeric prefix does not index the list.
      expect(jsonPointer(input, '/items/1junk').found, isFalse);
      expect(jsonPointer(input, '/missing').found, isFalse);
      expect(jsonPointer(input, '/nested/empty/deeper').found, isFalse);
      final nullValue = jsonPointer({'value': null}, '/value');
      expect(nullValue.found, isTrue);
      expect(nullValue.value, isNull);
    });
  });

  group('jsonEqual', () {
    test('compares by JSON text, never across types', () {
      expect(jsonEqual(35, 35), isTrue);
      expect(jsonEqual(35, '35'), isFalse);
      expect(jsonEqual(true, 'true'), isFalse);
      expect(jsonEqual(1.0, 1), isTrue);
      expect(jsonEqual({'a': 1, 'b': 2}, {'b': 2, 'a': 1}), isTrue);
      expect(jsonEqual({'a': 1}, {'a': 1, 'b': 2}), isFalse);
      expect(jsonEqual([1, 2], [2, 1]), isFalse);
    });
  });

  group('assertValue', () {
    final input = <String, Object?>{
      'count': 0,
      'error': {'code': 'INVALID_REQUEST'},
      'items': ['a', 'b'],
      'name': 'kitchen speaker',
      'reading': '1700000000',
      'volume': 35,
    };

    test('enforces each supported operator', () {
      assertValue(input, {
        '/count': {'non_empty': true},
        '/error/code': {'equals': 'INVALID_REQUEST'},
        '/items': {'count': 2},
        '/items/0': {'contains': 'a'},
        '/missing': {'present': false},
        '/name': {'max_length': 32, 'min_length': 3, 'pattern': '^kitchen'},
        '/reading': {'minimum': 1000000000},
        '/volume': {'equals': 35, 'maximum': 100, 'minimum': 0},
      });
    });

    test('rejects a mismatched value', () {
      expect(
        () => assertValue(input, {
          '/volume': {'equals': '35'},
        }),
        throwsA(isA<AssertionFailure>()),
      );
      expect(
        () => assertValue(input, {
          '/items': {'count': 3},
        }),
        throwsA(isA<AssertionFailure>()),
      );
      expect(
        () => assertValue(input, {
          '/missing': {'equals': 1},
        }),
        throwsA(isA<AssertionFailure>()),
      );
      expect(
        () => assertValue(input, {
          '/volume': {'contains': '35'},
        }),
        throwsA(isA<AssertionFailure>()),
      );
      expect(
        () => assertValue(input, {
          '/name': {'minimum': 1},
        }),
        throwsA(isA<AssertionFailure>()),
      );
    });

    test('treats only null, empty text and empty containers as empty', () {
      // Matching the Go runner, a zero number is not empty.
      expect(
        () => assertValue(input, {
          '/count': {'non_empty': false},
        }),
        throwsA(isA<AssertionFailure>()),
      );
      assertValue(
        {'list': <Object?>[]},
        {
          '/list': {'non_empty': false},
        },
      );
    });

    test('joins text-fragment arrays', () {
      assertValue(
        {
          'parts': ['hello ', 'world'],
        },
        {
          '/parts': {'contains': 'lo wo'},
        },
      );
      expect(
        () => assertValue(
          {
            'parts': ['a', 1],
          },
          {
            '/parts': {'contains': 'a'},
          },
        ),
        throwsA(isA<AssertionFailure>()),
      );
    });
  });

  group('generateValue', () {
    test('matches the Go runner shapes', () {
      expect(
        generateValue('uuid'),
        matches(
          r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-'
          r'[0-9a-f]{12}$',
        ),
      );
      for (final kind in ['token', 'string']) {
        final value = generateValue(kind);
        expect(value.length, 33);
        expect(value, matches(r'^g[0-9a-f]{32}$'));
      }
    });
  });

  group('variables', () {
    test('resolve inputs from value, env and generate', () {
      final variables = Variables(
        {
          'endpoint': const VariableSpec(
            direction: 'input',
            env: 'GIZTEST_UNIT_ENDPOINT',
            type: 'string',
          ),
          'level': const VariableSpec(
            direction: 'input',
            type: 'integer',
            value: 35,
          ),
          'name': const VariableSpec(
            direction: 'input',
            generate: 'token',
            type: 'string',
          ),
        },
        environment: {'GIZTEST_UNIT_ENDPOINT': '127.0.0.1:9821'},
      );
      expect(variables.resolve('\${endpoint}'), '127.0.0.1:9821');
      expect(variables.resolve('\${level}'), 35);
      expect(variables.resolve('\${name}'), matches(r'^g[0-9a-f]{32}$'));
      expect(
        variables.resolve('Bearer \${endpoint}/x'),
        'Bearer 127.0.0.1:9821/x',
      );
      expect(
        variables.resolve({
          'body': {'level': '\${level}'},
        }),
        {
          'body': {'level': 35},
        },
      );
      expect(variables.resolve(['\${level}', 1]), [35, 1]);
    });

    test('reject a missing environment input', () {
      expect(
        () => Variables({
          'endpoint': const VariableSpec(
            direction: 'input',
            env: 'GIZTEST_UNIT_ABSENT',
            type: 'string',
          ),
        }, environment: const {}),
        throwsStateError,
      );
    });

    test('assign outputs once and gate references until captured', () {
      final variables = Variables({
        'api_key': const VariableSpec(
          direction: 'output',
          secret: true,
          type: 'string',
        ),
      });
      expect(() => variables.resolve('\${api_key}'), throwsStateError);
      variables.assign('api_key', 'gizclaw_sk_v1_secret');
      expect(variables.resolve('\${api_key}'), 'gizclaw_sk_v1_secret');
      expect(() => variables.assign('api_key', 'other'), throwsStateError);
      expect(() => variables.assign('unknown', 'x'), throwsStateError);
      expect(variables.redactions(), ['gizclaw_sk_v1_secret']);
    });

    test('reject an output value of the wrong declared type', () {
      final variables = Variables({
        'count': const VariableSpec(direction: 'output', type: 'integer'),
      });
      expect(() => variables.assign('count', '12'), throwsStateError);
      variables.assign('count', 12);
    });

    test('sort redactions longest first', () {
      final variables = Variables({
        'long': const VariableSpec(
          direction: 'input',
          secret: true,
          type: 'string',
          value: 'abcdef',
        ),
        'short': const VariableSpec(
          direction: 'input',
          secret: true,
          type: 'string',
          value: 'abc',
        ),
      });
      expect(variables.redactions(), ['abcdef', 'abc']);
    });
  });

  group('safeError', () {
    test('redacts secrets and blanks credential wording', () {
      expect(
        safeError(StateError('saw abcdef here'), ['abcdef']),
        contains('[REDACTED]'),
      );
      expect(
        safeError(StateError('bad Authorization header')),
        'redacted execution error',
      );
      expect(safeError(StateError('x' * 600)).length, 512);
      expect(safeError(null), '');
    });
  });

  group('parseDuration', () {
    test('accepts Go duration strings', () {
      expect(parseDuration('30s'), const Duration(seconds: 30));
      expect(parseDuration('2m30s'), const Duration(seconds: 150));
      expect(parseDuration('1.5h'), const Duration(minutes: 90));
      expect(parseDuration('250ms'), const Duration(milliseconds: 250));
      expect(() => parseDuration('soon'), throwsFormatException);
    });
  });

  group('documents', () {
    test('parse a real device control scenario', () async {
      final document = await loadDocument(
        '$scenarioRoot/server.device.volume.set.giztest.yaml',
      );
      expect(document.name, 'server.device.volume.set');
      expect(document.repeat, 1);
      expect(document.clients.keys, ['peer']);
      // The client_rpc step asserts the call count, so it follows the HTTP
      // step that makes the Server call the device.
      expect(document.steps.map((step) => step.operation).toList(), [
        'rpc',
        'rpc',
        'http',
        'http',
        'rpc',
        'http',
        'client_rpc',
      ]);
      expect(document.finalizers.single.id, 'cleanup_peer');
      expect(collectReferences(document.steps[3]), ['api_key']);
    });

    test('load every device, contact and API key scenario', () async {
      final paths = await discover([scenarioRoot]);
      final selected = paths
          .where(
            (file) => RegExp(
              r'server\.(device|contact|contacts|api_key)\.',
            ).hasMatch(file.split('/').last),
          )
          .toList();
      expect(selected.length, greaterThanOrEqualTo(20));
      final result = await loadDocuments(selected);
      expect(result.skipped.length, 1);
      expect(
        result.skipped.keys.single,
        endsWith('server.device.audioplayer.telemetry.giztest.yaml'),
      );
      expect(result.skipped.values.single, contains('telemetry'));
      expect(result.documents.length, selected.length - 1);
    });

    test('skip scenarios that use unsupported step kinds', () async {
      final result = await loadDocuments(await discover([scenarioRoot]));
      expect(result.skipped, isNotEmpty);
      for (final reason in result.skipped.values) {
        expect(reason, contains('unsupported step kind'));
      }
    });

    test('reject a non-scenario file', () async {
      expect(
        () => discover(['$scenarioRoot/../run_tests.sh']),
        throwsFormatException,
      );
    });
  });
}
