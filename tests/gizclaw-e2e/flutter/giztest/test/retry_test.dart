import 'dart:async';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/assertions.dart';
import 'package:giztest/src/document.dart';
import 'package:giztest/src/runner.dart';
import 'package:giztest/src/variables.dart';

Step presenceStep({RetrySpec? retry, String? timeout}) => Step(
  id: 'presence',
  raw: const {
    'rpc': {'method': 'server.friend.list'},
  },
  retry: retry,
  timeout: timeout,
);

StepResult resultFor(Object? failure) {
  final report = StepReport(id: 'presence', operation: 'rpc', stage: 'rpc')
    ..status = failure == null ? 'passed' : 'failed'
    ..error = failure?.toString()
    ..evidence = {'online': failure == null};
  return StepResult(report, failure);
}

Future<GiztestDocument> loadPolicy(
  String policy, {
  bool finalizer = false,
  String operation = 'rpc: {method: all.ping, request: {}}',
}) async {
  final directory = await Directory.systemTemp.createTemp('giztest-retry-');
  try {
    final path = File('${directory.path}/retry.giztest.yaml');
    await path.writeAsString('''
# User Story:
# As a scenario author,
# I want bounded RPC polling,
# So that shared state can reach its historical view.
version: gizclaw.test/v1alpha1
name: retry
clients:
  peer: {identity: ephemeral, connection: webrtc, access_point: '127.0.0.1:9821'}
steps:
- id: initial
  client: peer
  rpc: {method: all.ping, request: {}}
${finalizer ? 'finally:' : ''}
- id: poll
  client: peer
  $operation
  retry: $policy
''');
    return await loadDocument(path.path);
  } finally {
    await directory.delete(recursive: true);
  }
}

void main() {
  test(
    'shared presence documents retain their assertion retry policy',
    () async {
      for (final name in [
        'server.friend.list',
        'server.friend_group.members.list',
        'server.friend_groups.http',
      ]) {
        final document = await loadDocument('../../giztest/$name.giztest.yaml');
        final retry = document.steps
            .singleWhere((step) => step.retry != null)
            .retry!;
        expect(retry.attempts, 10);
        expect(retry.on, ['assertion']);
        expect(retry.delay, const Duration(seconds: 10));
      }
    },
  );

  test('valid retry defaults and maximum delay match Go', () async {
    final defaults = await loadPolicy('{attempts: 2}');
    expect(defaults.steps.last.retry!.on, ['timeout']);
    expect(defaults.steps.last.retry!.delay, Duration.zero);
    final maximum = await loadPolicy(
      '{attempts: 10, on: [timeout, assertion], delay: 5m}',
    );
    expect(maximum.steps.last.retry!.delay, const Duration(minutes: 5));
  });

  for (final policy in [
    'true',
    '[]',
    '{attempts: 1}',
    '{attempts: 11}',
    '{attempts: 2.5}',
    '{attempts: "2"}',
    '{attempts: 2, on: []}',
    '{attempts: 2, on: assertion}',
    '{attempts: 2, on: [operation]}',
    '{attempts: 2, on: [assertion, assertion]}',
    '{attempts: 2, delay: 0s}',
    '{attempts: 2, delay: -1s}',
    '{attempts: 2, delay: 5m1s}',
    '{attempts: 2, delay: 10}',
    '{attempts: 2, delay: invalid}',
    '{attempts: 2, unknown: true}',
  ]) {
    test('invalid policy is rejected: $policy', () async {
      await expectLater(loadPolicy(policy), throwsA(isA<DocumentException>()));
    });
  }

  test('finalizers and HTTP steps cannot declare RPC retries', () async {
    await expectLater(
      loadPolicy('{attempts: 2}', finalizer: true),
      throwsA(isA<DocumentException>()),
    );
    await expectLater(
      loadPolicy(
        '{attempts: 2}',
        operation: 'http: {method: GET, path: /info}',
      ),
      throwsA(isA<DocumentException>()),
    );
  });

  test(
    'delayed presence retries and commits only successful captures',
    () async {
      final variables = Variables({
        'capture': const VariableSpec(direction: 'output', type: 'string'),
      });
      var calls = 0;
      final result = await runStepWithRetry(
        presenceStep(retry: const RetrySpec(attempts: 3, on: ['assertion'])),
        variables,
        (_) async {
          calls++;
          variables.assign('capture', 'attempt-$calls');
          return resultFor(
            calls < 3 ? const AssertionFailure('offline') : null,
          );
        },
      );
      expect(calls, 3);
      expect(result.failure, isNull);
      expect(variables['capture']!.data, 'attempt-3');
      expect(result.report.attempts!.map((attempt) => attempt['status']), [
        'failed',
        'failed',
        'passed',
      ]);
      expect(result.report.toJson()['attempts'], hasLength(3));
    },
  );

  test(
    'exhausted assertions fail without leaking captured variables',
    () async {
      final variables = Variables({
        'capture': const VariableSpec(direction: 'output', type: 'string'),
      });
      var calls = 0;
      final result = await runStepWithRetry(
        presenceStep(retry: const RetrySpec(attempts: 2, on: ['assertion'])),
        variables,
        (_) async {
          calls++;
          variables.assign('capture', 'failed-$calls');
          return resultFor(const AssertionFailure('offline'));
        },
      );
      expect(calls, 2);
      expect(result.failure, isA<AssertionFailure>());
      expect(variables['capture']!.data, isNull);
    },
  );

  for (final failure in [
    StateError('transport failed'),
    const AssertionFailure('offline'),
  ]) {
    test('default timeout policy stops on ${failure.runtimeType}', () async {
      var calls = 0;
      final result = await runStepWithRetry(
        presenceStep(retry: const RetrySpec(attempts: 3)),
        Variables({}),
        (_) async {
          calls++;
          return resultFor(failure);
        },
      );
      expect(calls, 1);
      expect(result.failure, same(failure));
    });
  }

  test('completed RPC timeouts are retried under the default policy', () async {
    var calls = 0;
    final result = await runStepWithRetry(
      presenceStep(retry: const RetrySpec(attempts: 3)),
      Variables({}),
      (_) async {
        calls++;
        return resultFor(
          calls == 1 ? TimeoutException('request timed out') : null,
        );
      },
    );
    expect(calls, 2);
    expect(result.failure, isNull);
    expect(result.report.attempts!.first['failure_kind'], 'timeout');
  });

  test(
    'the task deadline bounds retry delays and prevents another RPC',
    () async {
      var calls = 0;
      final result = await runStepWithRetry(
        presenceStep(
          retry: const RetrySpec(
            attempts: 3,
            on: ['assertion'],
            delay: Duration(seconds: 5),
          ),
        ),
        Variables({}),
        (_) async {
          calls++;
          return resultFor(const AssertionFailure('offline'));
        },
        budget: const Duration(milliseconds: 30),
      ).timeout(const Duration(seconds: 1));
      expect(calls, 1);
      expect(result.failure, isA<TimeoutException>());
      expect(result.report.status, 'failed');
    },
  );

  test(
    'each attempt receives the shorter step and remaining task budget',
    () async {
      for (final stepTimeout in ['5ms', '5s']) {
        final budgets = <Duration>[];
        final result = await runStepWithRetry(
          presenceStep(
            timeout: stepTimeout,
            retry: const RetrySpec(
              attempts: 2,
              on: ['assertion'],
              delay: Duration(milliseconds: 10),
            ),
          ),
          Variables({}),
          (remaining) async {
            budgets.add(remaining!);
            return resultFor(
              budgets.length == 1 ? const AssertionFailure('offline') : null,
            );
          },
          budget: const Duration(milliseconds: 200),
        );
        expect(result.failure, isNull);
        expect(budgets, hasLength(2));
        for (final budget in budgets) {
          expect(budget, lessThanOrEqualTo(parseDuration(stepTimeout)));
          expect(budget, lessThanOrEqualTo(const Duration(milliseconds: 200)));
        }
        if (stepTimeout == '5s') expect(budgets.last, lessThan(budgets.first));
      }
    },
  );
}
