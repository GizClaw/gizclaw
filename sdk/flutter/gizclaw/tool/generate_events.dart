import 'dart:convert';
import 'dart:io';

void main() {
  final repo = _repoRoot();
  final package = '$repo/sdk/flutter/gizclaw';
  final outDir = '$package/lib/src/generated/events';
  final output = Directory(outDir);
  if (output.existsSync()) {
    output.deleteSync(recursive: true);
  }
  output.createSync(recursive: true);

  final result = Process.runSync('protoc', [
    '--proto_path=$repo/api/proto/events',
    '--plugin=protoc-gen-dart=${_protocGenDart(package)}',
    '--dart_out=$outDir',
    '$repo/api/proto/events/peer_event.proto',
  ]);
  if (result.exitCode != 0) {
    stderr.write(result.stderr);
    exit(result.exitCode);
  }

  final format = Process.runSync('dart', ['format', outDir]);
  if (format.exitCode != 0) {
    stderr.write(format.stderr);
    exit(format.exitCode);
  }
}

String _repoRoot() {
  final result = Process.runSync('git', ['rev-parse', '--show-toplevel']);
  if (result.exitCode != 0) {
    stderr.write(result.stderr);
    exit(result.exitCode);
  }
  return (result.stdout as String).trim();
}

String _protocGenDart(String package) {
  final configFile = File('$package/.dart_tool/package_config.json');
  if (!configFile.existsSync()) {
    throw StateError('run flutter pub get before generating Event sources');
  }
  final config =
      jsonDecode(configFile.readAsStringSync()) as Map<String, Object?>;
  final packages = config['packages']! as List<Object?>;
  final protocPlugin = packages.cast<Map<String, Object?>>().singleWhere(
    (entry) => entry['name'] == 'protoc_plugin',
  );
  final rootUri = configFile.uri.resolve(protocPlugin['rootUri']! as String);
  final executable = File.fromUri(
    Directory.fromUri(rootUri).uri.resolve('bin/protoc_plugin.dart'),
  );
  if (!executable.existsSync()) {
    throw StateError('protoc_plugin does not provide bin/protoc_plugin.dart');
  }
  final wrapper = File('$package/.dart_tool/protoc-gen-dart-events');
  wrapper.writeAsStringSync(
    '#!/bin/sh\n'
    'exec ${_shellQuote(Platform.resolvedExecutable)} '
    '--packages=${_shellQuote(configFile.path)} '
    '${_shellQuote(executable.path)} "\$@"\n',
  );
  final chmod = Process.runSync('chmod', ['+x', wrapper.path]);
  if (chmod.exitCode != 0) {
    stderr.write(chmod.stderr);
    throw StateError('could not make protoc-gen-dart wrapper executable');
  }
  return wrapper.path;
}

String _shellQuote(String value) {
  return "'${value.replaceAll("'", "'\"'\"'")}'";
}
