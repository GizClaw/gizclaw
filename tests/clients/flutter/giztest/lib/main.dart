/// giztest: runs GizClaw scenario documents with the Flutter device SDK and
/// the Dart control SDK.
///
/// The device peer comes from `gizclaw` and every Public HTTP step goes
/// through `gizclaw_control`, so one scenario suite exercises both sides of
/// the Dart surface against a live server.
///
/// This is a Flutter desktop binary rather than a plain Dart CLI because the
/// device side needs the `flutter_webrtc` platform implementation. Build it
/// once, then run the binary with the same arguments as `gizclaw test`:
///
///     giztest validate -f <file-or-directory>...
///     giztest run <file-or-directory>... [--parallel N] [--output report.json]
library;

import 'dart:convert';
import 'dart:io';

import 'package:args/args.dart';
import 'package:flutter/widgets.dart';

import 'src/document.dart';
import 'src/runner.dart';

const _exitValidation = 2;
const _exitExecution = 4;

Future<void> main(List<String> arguments) async {
  WidgetsFlutterBinding.ensureInitialized();
  final code = await _run(arguments);
  await stdout.flush();
  exit(code);
}

Future<int> _run(List<String> arguments) async {
  if (arguments.isEmpty) {
    stderr.writeln('usage: giztest <validate|run> ...');
    return _exitValidation;
  }
  final command = arguments.first;
  final rest = arguments.skip(1).toList();
  try {
    if (command == 'validate') {
      return await _validate(rest);
    }
    if (command == 'run') {
      return await _runScenarios(rest);
    }
    stderr.writeln('unknown command: $command');
    return _exitValidation;
  } on DocumentException catch (error) {
    stderr.writeln(error);
    return _exitValidation;
  } on FormatException catch (error) {
    stderr.writeln(error.message);
    return _exitValidation;
  }
}

Future<int> _validate(List<String> arguments) async {
  final parser = ArgParser()..addMultiOption('file', abbr: 'f');
  final parsed = parser.parse(arguments);
  if (parsed.rest.isNotEmpty) {
    stderr.writeln('validate accepts no positional arguments');
    return _exitValidation;
  }
  final inputs = parsed.multiOption('file');
  if (inputs.isEmpty) {
    stderr.writeln('validate requires --file');
    return _exitValidation;
  }
  final result = await loadDocuments(await discover(inputs));
  _reportSkipped(result);
  stdout.writeln('validated ${result.documents.length} Giztest documents');
  return 0;
}

Future<int> _runScenarios(List<String> arguments) async {
  final parser = ArgParser()
    ..addOption('parallel', defaultsTo: '1')
    ..addOption('output');
  final parsed = parser.parse(arguments);
  if (parsed.rest.isEmpty) {
    stderr.writeln('run requires at least one file or directory');
    return _exitValidation;
  }
  final parallel = int.tryParse(parsed.option('parallel') ?? '1') ?? 0;
  if (parallel < 1) {
    stderr.writeln('--parallel must be at least 1');
    return _exitValidation;
  }
  final result = await loadDocuments(await discover(parsed.rest));
  _reportSkipped(result);
  final report = await runDocuments(result.documents, parallel, stdout);
  final output = parsed.option('output');
  if (output != null) {
    await _writeReport(output, report);
  }
  stdout.writeln(
    'Giztest ${report.status}: ${report.tasks.length} tasks '
    'in ${report.durationMs}ms',
  );
  return report.status == 'passed' ? 0 : _exitExecution;
}

void _reportSkipped(LoadResult result) {
  for (final entry in result.skipped.entries) {
    stderr.writeln('skipped ${entry.key}: ${entry.value}');
  }
}

/// Replaces the target atomically so a partial file is never left behind for
/// the caller to read.
Future<void> _writeReport(String target, Report report) async {
  final directory = File(target).parent.path;
  final temporary = File('$directory/.giztest-report-$pid.json');
  await temporary.writeAsString(
    '${const JsonEncoder.withIndent('  ').convert(report.toJson())}\n',
  );
  await temporary.rename(target);
}
