import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/document.dart';
import 'package:giztest/src/http_query.dart';
import 'package:giztest/src/variables.dart';

void main() {
  test('query overrides one key and preserves unrelated repeated values', () {
    final variables = Variables({
      'checkpoint': const VariableSpec(
        direction: 'input',
        type: 'number',
        value: 1700000000000,
      ),
    });
    final path = resolveHttpQuery('/sync?tag=a&tag=b&timestamp=1&timestamp=2', {
      'timestamp': r'${checkpoint}',
      'search': '设备 + room',
      'online': true,
      'limit': 20.0,
    }, variables);
    expect(Uri.parse(path).queryParametersAll, {
      'tag': ['a', 'b'],
      'timestamp': ['1700000000000'],
      'search': ['设备 + room'],
      'online': ['true'],
      'limit': ['20'],
    });
  });
  test('empty query preserves the original path', () {
    expect(
      resolveHttpQuery('/sync?tag=a&tag=b', {}, Variables({})),
      '/sync?tag=a&tag=b',
    );
  });
  test('query rejects non-scalar and non-finite values', () {
    for (final value in [null, [], {}, double.nan, double.infinity]) {
      expect(
        () => resolveHttpQuery('/sync', {'timestamp': value}, Variables({})),
        throwsStateError,
      );
    }
  });
}
