import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/client.dart';

void main() {
  test('scenario requests wrap single-value messages exactly once', () {
    const value = <String, Object?>{'name': 'example'};
    final implicit = scenarioRequest('server.workspace.create', value);
    final explicit = scenarioRequest('server.workspace.create', {
      'value': value,
    });
    expect(implicit.writeToBuffer(), explicit.writeToBuffer());
    expect(implicit.toProto3Json(), {'value': value});
  });

  test('scenario requests keep ordinary messages unwrapped', () {
    final request = scenarioRequest('server.workspace.get', {
      'name': 'example',
    });
    expect(request.toProto3Json(), {'name': 'example'});
  });

  test('wrapped scenario requests still reject unknown fields', () {
    expect(
      () => scenarioRequest('server.workspace.create', {'unknown': true}),
      throwsFormatException,
    );
  });
}
