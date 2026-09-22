import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/document.dart';
import 'package:giztest/src/runner.dart';

const document = '''# User Story:
# As a Giztest SDK tester,
# I want to accept shared load timing fields,
# So that SDK checks can validate shared documents without simulating load.
version: gizclaw.test/v1alpha1
name: timing
clients:
  peer: {identity: ephemeral, connection: webrtc, access_point: localhost:9820}
variables: {}
steps:
  - id: ping
    client: peer
    rpc: {method: all.ping, request: {}}
''';

void main() {
  test('timing fields are accepted and validated before being ignored', () {
    for (final fields in [
      'start_jitter: 30s\nstagger: 2s\nstep_jitter: 3s\nseed: 0\n',
      "start_jitter: '0'\nstagger: 1h2m3.5s\nseed: 9007199254740991\n",
      'start_jitter: 9223372036854775807ns\n',
    ]) {
      expect(
        parseDocument('timing.giztest.yaml', document + fields).name,
        'timing',
      );
    }
    for (final fields in [
      'start_jitter: -1s\n',
      'stagger: nonsense\n',
      'step_jitter: 0..3s\n',
      "start_jitter: ''\n",
      'start_jitter: 0\n',
      'start_jitter: null\n',
      'step_jitter: {min: 0s, max: 3s}\n',
      'seed: -1\n',
      'seed: 1.5\n',
      "seed: '1'\n",
      'seed: null\n',
      'seed: 9007199254740992\n',
      'start_jitter: 9223372036854775808ns\n',
      'repeat: 3\nstagger: 4611686018427387904ns\n',
      'repeat: 2\nstagger: 9223372036854775807ns\nstart_jitter: 2ns\n',
    ]) {
      expect(
        () => parseDocument('timing.giztest.yaml', document + fields),
        throwsFormatException,
        reason: fields,
      );
    }
    final barrier = document.replaceFirst(
      '    rpc: {method: all.ping, request: {}}',
      '    barrier: {}',
    );
    expect(
      () => parseDocument('timing.giztest.yaml', '${barrier}step_jitter: 3s\n'),
      throwsFormatException,
    );
  });
  test('SDK reports identify ignored scheduling', () {
    expect(Report(DateTime.now()).toJson()['timing_mode'], 'ignored');
  });
}
