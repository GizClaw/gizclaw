/// Scenario execution: one task per document repeat, steps in order, then the
/// finalizers, producing the same report shape as the Go runner.
library;

import 'dart:async';
import 'dart:io';

import 'assertions.dart';
import 'client.dart';
import 'document.dart';
import 'variables.dart';

const _cleanupBudget = Duration(seconds: 30);
const _clientRpcPoll = Duration(milliseconds: 10);

class StepReport {
  StepReport({
    required this.id,
    required this.operation,
    required this.stage,
    this.client,
  });

  final String id;
  final String operation;
  final String stage;
  final String? client;
  String status = 'failed';
  int durationMs = 0;
  String? error;
  Map<String, Object?>? evidence;

  Map<String, Object?> toJson() => {
    'id': id,
    'operation': operation,
    if (client != null) 'client': client,
    'status': status,
    'stage': stage,
    'duration_ms': durationMs,
    if (error != null && error!.isNotEmpty) 'error': error,
    if (evidence != null && evidence!.isNotEmpty) 'evidence': evidence,
  };
}

class TaskReport {
  TaskReport({
    required this.path,
    required this.name,
    required this.taskId,
    required this.repeatIndex,
  });

  final String path;
  final String name;
  final String taskId;
  final int repeatIndex;
  String status = 'failed';
  int durationMs = 0;
  Map<String, String> clients = {};
  List<StepReport> steps = [];
  List<StepReport> cleanup = [];
  String? error;

  Map<String, Object?> toJson() => {
    'path': path,
    'name': name,
    'task_id': taskId,
    'status': status,
    'repeat_index': repeatIndex,
    'duration_ms': durationMs,
    if (clients.isNotEmpty) 'clients': clients,
    'steps': steps.map((step) => step.toJson()).toList(),
    if (cleanup.isNotEmpty)
      'cleanup': cleanup.map((step) => step.toJson()).toList(),
    if (error != null && error!.isNotEmpty) 'error': error,
  };
}

class Report {
  Report(this.startedAt);

  final DateTime startedAt;
  String status = 'passed';
  int durationMs = 0;
  List<TaskReport> tasks = [];

  Map<String, Object?> toJson() => {
    'version': 'v1',
    'status': status,
    'started_at': startedAt.toIso8601String(),
    'duration_ms': durationMs,
    'tasks': tasks.map((task) => task.toJson()).toList(),
  };
}

class _StepOutcome {
  const _StepOutcome({this.value, this.evidence});

  final Object? value;
  final Map<String, Object?>? evidence;
}

/// Runs one step and returns its result value and evidence. The value is what
/// capture and expect resolve pointers against.
Future<_StepOutcome> _runStep(
  Step step,
  Map<String, ScenarioClient> clients,
  Variables variables,
  IOSink out,
) async {
  final client = step.client == null ? null : clients[step.client];

  final rpc = step.rpc;
  if (rpc != null) {
    if (client == null) {
      throw StateError('step ${step.id} has no connected client');
    }
    final params = variables.resolve(
      rpc['request'] ?? const <String, Object?>{},
    );
    return _StepOutcome(
      value: await client.callRpc(rpc['method'] as String, params),
    );
  }

  final http = step.http;
  if (http != null) {
    if (client == null) {
      throw StateError('step ${step.id} has no connected client');
    }
    final path = variables.resolveString(http['path'], 'http path');
    if (!path.startsWith('/')) {
      throw StateError('http path must resolve to an absolute path');
    }
    final headers = <String, String>{};
    for (final entry
        in (http['headers'] as Map<String, Object?>? ?? const {}).entries) {
      headers[entry.key] = variables.resolveString(
        entry.value,
        'header ${entry.key}',
      );
    }
    final body = http.containsKey('body')
        ? variables.resolve(http['body'])
        : null;
    final method = http['method'] as String;
    final result = await client.callHttp(method, path, headers, body);
    final evidence = <String, Object?>{
      'method': method,
      'path': path,
      'status': result.status,
    };
    final declared = http['status'] as int?;
    if (declared != null && result.status != declared) {
      throw AssertionFailure('http status = ${result.status}, want $declared');
    }
    if (declared == null && result.status >= 400) {
      throw AssertionFailure('http status = ${result.status}');
    }
    return _StepOutcome(value: result.body, evidence: evidence);
  }

  final clientRpc = step.clientRpc;
  if (clientRpc != null) {
    if (client == null) {
      throw StateError('step ${step.id} has no connected client');
    }
    final method = clientRpc['method'] as String;
    if (!client.inbound.containsKey(method)) {
      throw StateError('client RPC $method was not installed');
    }
    final expected = clientRpc['expect_calls'] as int? ?? 1;
    // The counter is cumulative from connect time and the wait is "at least
    // N", matching the Go runner.
    while ((client.inbound[method] ?? 0) < expected) {
      await Future<void>.delayed(_clientRpcPoll);
    }
    final evidence = <String, Object?>{
      'calls': client.inbound[method] ?? 0,
      'method': method,
    };
    return _StepOutcome(value: evidence, evidence: evidence);
  }

  final output = step.output;
  if (output != null) {
    final name = output['variable'] as String;
    final entry = variables[name];
    if (entry == null || entry.data == null) {
      throw StateError('output variable $name unavailable');
    }
    if (entry.spec.secret) {
      throw StateError('secret variable $name cannot be emitted');
    }
    if (entry.spec.type == 'object') {
      throw StateError('variable $name type object cannot be emitted');
    }
    final text = '${entry.data}';
    out.writeln('$name=$text');
    return _StepOutcome(
      evidence: {
        'bytes': text.length,
        'truncated': false,
        'type': entry.spec.type,
        'variable': name,
      },
    );
  }

  throw StateError('step ${step.id} declares no supported operation');
}

class _StepResult {
  const _StepResult(this.report, this.failure);

  final StepReport report;
  final Object? failure;
}

Future<_StepResult> _runStepReport(
  Step step,
  Map<String, ScenarioClient> clients,
  Variables variables,
  List<String> redactions,
  String stage,
  IOSink out, {
  Duration? budget,
}) async {
  final started = DateTime.now();
  final report = StepReport(
    client: step.client,
    id: step.id,
    operation: step.operation,
    stage: stage,
  );
  Object? failure;
  var outcome = const _StepOutcome();
  final stepTimeout = step.timeout == null
      ? budget
      : parseDuration(step.timeout!);
  try {
    final future = _runStep(step, clients, variables, out);
    outcome = stepTimeout == null
        ? await future
        : await future.timeout(stepTimeout);
  } catch (error) {
    failure = error;
  }

  final expectError = step.expectError;
  if (expectError != null) {
    final code = failure is ScenarioRpcError ? failure.code : null;
    if (code == null) {
      failure ??= AssertionFailure(
        'expected RPC error code ${expectError['code']}, got success',
      );
    } else if (code != expectError['code']) {
      failure = AssertionFailure(
        'RPC error code = $code, want ${expectError['code']}',
      );
    } else {
      failure = null;
      outcome = _StepOutcome(evidence: {'rpc_error_code': code});
    }
  }

  if (failure == null && outcome.value != null) {
    try {
      if (step.saveAs != null) {
        variables.assign(step.saveAs!, outcome.value);
      }
      for (final entry in step.capture.entries) {
        final result = jsonPointer(outcome.value, entry.value);
        if (!result.found) {
          throw StateError('capture pointer ${entry.value} not found');
        }
        variables.assign(entry.key, result.value);
      }
      assertValue(outcome.value, step.expect);
    } catch (error) {
      failure = error;
    }
  }

  report.durationMs = DateTime.now().difference(started).inMilliseconds;
  report.status = failure == null ? 'passed' : 'failed';
  if (failure != null) {
    report.error = safeError(failure, redactions);
  }
  report.evidence =
      outcome.evidence ??
      (failure == null && outcome.value != null
          ? const {'result': 'captured'}
          : null);
  return _StepResult(report, failure);
}

Future<TaskReport> _runTask(
  GiztestDocument document,
  int repeatIndex,
  IOSink out,
) async {
  final started = DateTime.now();
  final report = TaskReport(
    name: document.name,
    path: document.path,
    repeatIndex: repeatIndex,
    taskId: '${document.name}-${repeatIndex.toString().padLeft(4, '0')}',
  );

  Variables variables;
  try {
    variables = Variables(
      document.variables,
      environment: Platform.environment,
    );
  } catch (error) {
    report.error = safeError(error);
    report.durationMs = DateTime.now().difference(started).inMilliseconds;
    return report;
  }
  final redactions = variables.redactions(document.redact);
  final clients = <String, ScenarioClient>{};
  final deadline = started.add(document.taskTimeout);
  Duration remaining() {
    final left = deadline.difference(DateTime.now());
    return left.isNegative ? Duration.zero : left;
  }

  try {
    for (final name in document.clients.keys.toList()..sort()) {
      final client = await ScenarioClient.connect(
        name,
        document.clients[name]!,
        document.steps,
        variables,
      ).timeout(remaining());
      clients[name] = client;
      report.clients[name] = client.fingerprint;
    }
    for (final step in document.steps) {
      final result = await _runStepReport(
        step,
        clients,
        variables,
        redactions,
        step.operation,
        out,
        budget: remaining(),
      );
      report.steps.add(result.report);
      if (result.failure != null) {
        report.error = safeError(result.failure, redactions);
        break;
      }
    }
    report.status = report.error == null ? 'passed' : 'failed';
  } catch (error) {
    report.error = safeError(error, redactions);
    report.status = 'failed';
  }

  // Finalizers always run, on a fresh budget detached from the task timeout.
  for (final step in document.finalizers) {
    final result = await _runStepReport(
      step,
      clients,
      variables,
      redactions,
      'cleanup',
      out,
      budget: _cleanupBudget,
    );
    report.cleanup.add(result.report);
    if (result.failure != null) {
      report.status = 'failed';
      report.error ??= safeError(result.failure, redactions);
    }
  }

  for (final client in clients.values) {
    await client.close();
  }
  report.durationMs = DateTime.now().difference(started).inMilliseconds;
  return report;
}

Future<Report> runDocuments(
  List<GiztestDocument> documents,
  int parallel,
  IOSink out,
) async {
  final report = Report(DateTime.now());
  final queue = <({GiztestDocument document, int index})>[];
  for (final document in documents) {
    for (var index = 0; index < document.repeat; index++) {
      queue.add((document: document, index: index));
    }
  }

  var next = 0;
  Future<void> worker() async {
    while (true) {
      final current = next++;
      if (current >= queue.length) {
        return;
      }
      final item = queue[current];
      report.tasks.add(await _runTask(item.document, item.index, out));
    }
  }

  await Future.wait([
    for (var index = 0; index < parallel.clamp(1, queue.length | 1); index++)
      worker(),
  ]);

  report.tasks.sort((left, right) {
    final byPath = left.path.compareTo(right.path);
    return byPath != 0 ? byPath : left.repeatIndex.compareTo(right.repeatIndex);
  });
  report.durationMs = DateTime.now()
      .difference(report.startedAt)
      .inMilliseconds;
  report.status = report.tasks.every((task) => task.status == 'passed')
      ? 'passed'
      : 'failed';
  return report;
}
