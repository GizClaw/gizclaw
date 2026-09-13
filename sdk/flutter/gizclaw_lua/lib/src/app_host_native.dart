import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:isolate';
import 'dart:math';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:gizclaw/gizclaw.dart';

import 'firmware_host.dart';
import 'board.dart';

final _name = RegExp(r'^[A-Za-z_][A-Za-z0-9_-]{0,63}$');
final _runtime = RegExp(r'^[A-Za-z_][A-Za-z0-9_.-]{0,127}$');
Never _invalid(String message) => throw GizClawDeviceControlException(
  StatusCode.STATUS_CODE_INVALID_ARGUMENT,
  message,
);
Never _missing(String message) => throw GizClawDeviceControlException(
  StatusCode.STATUS_CODE_NOT_FOUND,
  message,
);

bool _safePath(String path) =>
    path.isNotEmpty &&
    !path.contains('\\') &&
    !path.contains('\u0000') &&
    !path.contains(':') &&
    path.split('/').every((p) => p.isNotEmpty && p != '.' && p != '..');

/// Device App execution on the GizOS firmware Lua Host.
/// Supply an explicit runtime profile for the actual board/PAL surface.
class GizClawLuaAppHost implements GizClawAppHost {
  GizClawLuaAppHost({
    required this.runtime,
    required this.storageDirectory,
    GizClawLuaBoard? board,
    this.maxPackageBytes = 8 * 1024 * 1024,
    this.maxExpandedBytes = 32 * 1024 * 1024,
    this.maxFiles = 256,
    this.memoryLimit = 8 * 1024 * 1024,
    this.sourceLimit = 2 * 1024 * 1024,
    this.outputLimit = 64 * 1024,
    this.executionTimeout = const Duration(seconds: 30),
    this.maxExecutions = 8,
  }) : board = board ?? GizClawLuaBoard(displayWidth: 240, displayHeight: 240) {
    if (!_runtime.hasMatch(runtime) ||
        maxPackageBytes <= 0 ||
        maxExpandedBytes <= 0 ||
        maxFiles <= 0 ||
        memoryLimit < 256 * 1024 ||
        sourceLimit <= 0 ||
        outputLimit <= 0 ||
        maxExecutions <= 0 ||
        maxExecutions > 64 ||
        executionTimeout.inMilliseconds <= 0 ||
        executionTimeout.inMilliseconds > 2147483647) {
      throw ArgumentError('Invalid App host configuration');
    }
  }
  final GizClawLuaBoard board;
  final _capabilities = <String, FirmwareCapability>{};
  FirmwareHost? _firmware;
  bool _started = false;
  void registerCapability(
    String name,
    Future<String> Function(String input, String? options) call, {
    void Function()? cancel,
  }) {
    if (_started || _closing || _closed) {
      throw StateError('Capability registry is frozen');
    }
    if (!RegExp(
          r'^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+$',
        ).hasMatch(name) ||
        name.length >= 48 ||
        _capabilities.containsKey(name)) {
      throw ArgumentError('Invalid or duplicate capability name');
    }
    if (_capabilities.length >= 16) {
      throw StateError('Firmware supports at most 16 capabilities');
    }
    _capabilities[name] = FirmwareCapability(name, call, cancel);
  }

  Future<void> start() => _serial(() async {
    _ensureStarted();
  });
  void _ensureStarted() {
    if (_closed) throw StateError('App host closed');
    if (_firmware != null) return;
    if (board.running) throw StateError('Board already attached to a host');
    storageDirectory.createSync(recursive: true);
    _firmware = FirmwareHost(
      root: storageDirectory.absolute.path,
      width: board.displayWidth,
      height: board.displayHeight,
      buttons: board.buttons.length,
      audioInput: board.audioInput,
      audioOutput: board.audioOutput,
      touch: board.touch,
      memory: memoryLimit,
      source: sourceLimit,
      output: outputLimit,
      timeout: executionTimeout.inMilliseconds,
      jobs: maxExecutions,
      capabilities: _capabilities.values.toList(),
      onFrame: board.updateFramebuffer,
    );
    _started = true;
    board.bind(_firmware!.button, _firmware!.touch);
  }

  /// Inject complete 320-sample mono S16LE frames into the bounded test source.
  void pushAudioInput(Uint8List bytes) {
    _ensureStarted();
    _firmware!.pushAudio(bytes);
  }

  /// Drain captured PCM from the bounded test sink; this does not play sound.
  Uint8List readAudioOutput() {
    _ensureStarted();
    return _firmware!.readAudio();
  }

  final String runtime;
  final Directory storageDirectory;
  final int maxPackageBytes, maxExpandedBytes, maxFiles;
  final int memoryLimit, sourceLimit, outputLimit, maxExecutions;
  final Duration executionTimeout;
  final _jobCompletions = StreamController<GizClawAppJobCompletion>.broadcast();

  /// Non-replaying local completion events. Subscribe before starting jobs.
  /// Closing the host drains executions and closes this stream.
  @override
  Stream<GizClawAppJobCompletion> get jobCompletions => _jobCompletions.stream;
  final _garbage = <String>[];
  Future<void>? _closeFuture;
  final _jobs = <int, FirmwareRun>{};
  final _runs = <FirmwareRun>{};
  var _nextJob = 1;
  var _closed = false;
  var _closing = false;
  Future<void> _tail = Future.value();

  Future<T> _serial<T>(Future<T> Function() action) {
    if (_closing || _closed) return Future.error(StateError('App host closed'));
    final next = _tail.then((_) {
      if (_closed) throw StateError('App host closed');
      return action();
    });
    _tail = next.then<void>((_) {}, onError: (Object _, StackTrace _) {});
    return next;
  }

  Future<Map<String, String>> _index() async {
    final file = File('${storageDirectory.path}/installed.json');
    if (!await file.exists()) return {};
    final raw = jsonDecode(await file.readAsString());
    if (raw is! Map<String, Object?>) throw StateError('Invalid install index');
    final result = <String, String>{};
    for (final entry in raw.entries) {
      if (!_name.hasMatch(entry.key) ||
          entry.value is! String ||
          !RegExp(r'^pkg-[a-f0-9]+$').hasMatch(entry.value as String)) {
        throw StateError('Invalid install index');
      }
      result[entry.key] = entry.value as String;
    }
    return result;
  }

  Future<void> _saveIndex(Map<String, String> index) async {
    final temp = File('${storageDirectory.path}/installed.next');
    await temp.writeAsString(jsonEncode(index), flush: true);
    await temp.rename('${storageDirectory.path}/installed.json');
  }

  @override
  Future<ClientAppListResponse> list() => _serial(() async {
    final index = await _index();
    final apps = <InstalledApp>[];
    for (final entry in index.entries) {
      apps.add(
        InstalledApp(
          appName: entry.key,
          sha256: await File(
            '${storageDirectory.path}/${entry.value}/digest',
          ).readAsString(),
        ),
      );
    }
    apps.sort((a, b) => a.appName.compareTo(b.appName));
    return ClientAppListResponse(
      runtime: runtime,
      apps: apps,
      capabilities: _capabilities.keys,
    );
  });

  @override
  Future<void> install({
    required String appName,
    required String url,
    required String sha256,
    required int size,
  }) => _serial(() async {
    if (!_name.hasMatch(appName) ||
        !RegExp(r'^[a-f0-9]{64}$').hasMatch(sha256) ||
        size <= 0 ||
        size > maxPackageBytes) {
      _invalid('Invalid package reference');
    }
    final uri = Uri.tryParse(url);
    if (uri == null ||
        !['http', 'https'].contains(uri.scheme) ||
        uri.host.isEmpty ||
        uri.userInfo.isNotEmpty ||
        uri.hasFragment) {
      _invalid('Invalid package URL');
    }
    final client = HttpClient()
      ..connectionTimeout = const Duration(seconds: 15);
    final bytes = BytesBuilder(copy: false);
    final downloadDeadline = Timer(
      const Duration(seconds: 60),
      () => client.close(force: true),
    );
    try {
      final request = await client
          .getUrl(uri)
          .timeout(const Duration(seconds: 15));
      final response = await request.close().timeout(
        const Duration(seconds: 15),
      );
      if (response.statusCode != 200) _invalid('Package download failed');
      await for (final chunk in response.timeout(const Duration(seconds: 15))) {
        if (bytes.length + chunk.length > size) {
          _invalid('Package size mismatch');
        }
        bytes.add(chunk);
      }
    } finally {
      downloadDeadline.cancel();
      client.close(force: true);
    }
    if (bytes.length != size) _invalid('Package size mismatch');
    final compressed = bytes.takeBytes();
    final hash = await Sha256().hash(compressed);
    if (hash.bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join() !=
        sha256) {
      _invalid('Package sha256 mismatch');
    }
    final files = await _unpackIsolate(compressed, maxExpandedBytes, maxFiles);
    final manifest = _manifest(
      files,
      appName,
      runtime,
      _capabilities.keys.toSet(),
    );
    await storageDirectory.create(recursive: true);
    final generation =
        'pkg-${List.generate(16, (_) => Random.secure().nextInt(256).toRadixString(16).padLeft(2, '0')).join()}';
    final directory = Directory('${storageDirectory.path}/$generation');
    await directory.create();
    var committed = false;
    try {
      final content = Directory('${directory.path}/content');
      await content.create();
      for (final entry in files.entries) {
        final file = File('${content.path}/${entry.key}');
        await file.parent.create(recursive: true);
        await file.writeAsBytes(entry.value, flush: true);
      }
      try {
        await _execute(
          _chunk(files, manifest, null, null),
          name: '@$generation/content/__invoke.lua',
        );
      } on GizClawDeviceControlException catch (error) {
        _invalid('Invalid App entry: ${error.message}');
      }
      await File('${directory.path}/digest').writeAsString(sha256, flush: true);
      final index = await _index();
      final old = index[appName];
      index[appName] = generation;
      await _saveIndex(index);
      committed = true;
      if (old != null) {
        _garbage.add(old);
      }
    } finally {
      if (!committed && await directory.exists()) {
        await directory.delete(recursive: true);
      }
    }
  });

  @override
  Future<void> uninstall(String appName) => _serial(() async {
    if (!_name.hasMatch(appName)) _invalid('Invalid app name');
    final index = await _index();
    final generation = index.remove(appName);
    if (generation == null) _missing('Unknown app');
    await _saveIndex(index);
    _garbage.add(generation);
  });

  Future<(String, String)> _prepare(
    String appName,
    String method,
    String argsJson,
    String mode,
  ) => _serial(() async {
    if (!_name.hasMatch(appName) ||
        !_name.hasMatch(method) ||
        utf8.encode(argsJson).length > sourceLimit) {
      _invalid('Invalid invocation');
    }
    late Object? args;
    try {
      args = jsonDecode(argsJson);
    } on FormatException {
      _invalid('Invalid args_json');
    }
    if (args is! Map<String, Object?>) _invalid('args_json must be an object');
    final generation = (await _index())[appName];
    if (generation == null) _missing('Unknown app');
    final directory = Directory('${storageDirectory.path}/$generation/content');
    final files = <String, List<int>>{};
    await for (final entity in directory.list(
      recursive: true,
      followLinks: false,
    )) {
      if (entity is Link) throw StateError('Symlink in installed app');
      if (entity is File) {
        files[entity.path.substring(directory.path.length + 1)] = await entity
            .readAsBytes();
      }
    }
    final manifest = _manifest(
      files,
      appName,
      runtime,
      _capabilities.keys.toSet(),
    );
    final methods = manifest['methods'] as List<Object?>;
    final matches = methods.whereType<Map<String, Object?>>().where(
      (m) => m['name'] == method,
    );
    if (matches.isEmpty) _missing('Unknown method');
    if (matches.single['mode'] != mode) _invalid('Wrong method mode');
    return (
      _chunk(files, manifest, method, args),
      '@$generation/content/__invoke.lua',
    );
  });

  @override
  Future<String> invoke(String appName, String method, String argsJson) async {
    final prepared = await _prepare(appName, method, argsJson, 'call');
    return _execute(prepared.$1, name: prepared.$2);
  }

  @override
  Future<int> startJob(String appName, String method, String argsJson) async {
    final source = await _prepare(appName, method, argsJson, 'job');
    final run = _start(source.$1, name: source.$2);
    final id = _nextJob++;
    if (_nextJob > 0xffffffff) _nextJob = 1;
    _jobs[id] = run;
    unawaited(
      run.done.then<void>(
        (result) {
          _jobs.remove(id);
          _jobCompletions.add(
            GizClawAppJobCompletion(jobId: id, resultJson: result),
          );
        },
        onError: (Object error, StackTrace stack) {
          _jobs.remove(id);
          _jobCompletions.add(
            GizClawAppJobCompletion(jobId: id, error: error.toString()),
          );
        },
      ),
    );
    return id;
  }

  @override
  Future<void> cancelJob(int jobId) async {
    final run = _jobs[jobId];
    if (run == null || run.completed) _missing('Unknown job');
    _firmware?.cancel(run.id);
    try {
      await run.done;
    } catch (_) {
      /* Cancellation acknowledged. */
    }
  }

  FirmwareRun _start(String source, {required String name}) {
    if (_closing || _closed) throw StateError('App host closed');
    if (_runs.length >= maxExecutions) {
      throw const GizClawDeviceControlException(
        StatusCode.STATUS_CODE_RESOURCE_EXHAUSTED,
        'Execution capacity reached',
      );
    }
    _ensureStarted();
    final run = _firmware!.submit(name, source);
    _runs.add(run);
    unawaited(
      run.done.then<void>(
        (_) {
          _runs.remove(run);
        },
        onError: (Object _, StackTrace _) {
          _runs.remove(run);
        },
      ),
    );
    return run;
  }

  Future<String> _execute(String source, {required String name}) async {
    try {
      return await _start(source, name: name).done;
    } catch (error) {
      throw GizClawDeviceControlException(
        StatusCode.STATUS_CODE_INTERNAL,
        error.toString(),
      );
    }
  }

  @override
  Future<void> close() => _closeFuture ??= _close();
  Future<void> _close() async {
    _closing = true;
    await _tail;
    _closed = true;
    final active = _runs.toList();
    for (final run in active) {
      _firmware?.cancel(run.id);
    }
    for (final run in active) {
      try {
        await run.done;
      } catch (_) {
        /* Drained. */
      }
    }
    await _firmware?.close();
    board.unbind();
    for (final generation in _garbage) {
      final dir = Directory('${storageDirectory.path}/$generation');
      if (await dir.exists()) await dir.delete(recursive: true);
    }
    _garbage.clear();
    await _jobCompletions.close();
  }
}

class _BoundedSink extends ByteConversionSink {
  _BoundedSink(this.bytes, this.limit);
  final BytesBuilder bytes;
  final int limit;
  @override
  void add(List<int> chunk) {
    if (bytes.length + chunk.length > limit) {
      _invalid('Expanded package limit exceeded');
    }
    bytes.add(chunk);
  }

  @override
  void close() {}
}

Future<Map<String, List<int>>> _unpack(
  List<int> compressed,
  int maxBytes,
  int maxFiles,
) async {
  final expanded = BytesBuilder(copy: false);
  try {
    // Reject expansion synchronously, before a stream queue can buffer a bomb.
    final decoder = zlib.decoder.startChunkedConversion(
      _BoundedSink(expanded, maxBytes),
    );
    decoder.add(compressed);
    decoder.close();
    final bytes = expanded.takeBytes();
    final files = <String, List<int>>{};
    final seen = <String>{};
    var offset = 0;
    var ended = false;
    String field(int start, int length) {
      final raw = bytes.sublist(offset + start, offset + start + length);
      final end = raw.indexOf(0);
      return utf8.decode(end < 0 ? raw : raw.sublist(0, end));
    }

    while (offset + 512 <= bytes.length) {
      final header = bytes.sublist(offset, offset + 512);
      if (header.every((b) => b == 0)) {
        ended = true;
        break;
      }
      final checksum = int.tryParse(field(148, 8).trim(), radix: 8);
      final actual = header.asMap().entries.fold<int>(
        0,
        (sum, e) => sum + (e.key >= 148 && e.key < 156 ? 32 : e.value),
      );
      if (checksum != actual) _invalid('Invalid tar checksum');
      final prefix = field(345, 155);
      final name = '${prefix.isEmpty ? '' : '$prefix/'}${field(0, 100)}';
      final type = bytes[offset + 156];
      final directory = type == 53;
      final path = directory && name.endsWith('/')
          ? name.substring(0, name.length - 1)
          : name;
      if (!_safePath(path) ||
          !seen.add(path) ||
          seen.length > maxFiles ||
          !(type == 0 || type == 48 || directory)) {
        _invalid('Unsafe package member');
      }
      final size = int.tryParse(field(124, 12).trim(), radix: 8);
      if (size == null ||
          size < 0 ||
          (directory && size != 0) ||
          offset + 512 + size > bytes.length) {
        _invalid('Invalid tar size');
      }
      if (!directory) {
        files[path] = bytes.sublist(offset + 512, offset + 512 + size);
      }
      offset += 512 + ((size + 511) ~/ 512) * 512;
    }
    if (!ended ||
        bytes.length - offset < 1024 ||
        bytes.sublist(offset).any((b) => b != 0)) {
      _invalid('Invalid tar terminator');
    }
    for (final path in files.keys) {
      final parts = path.split('/');
      for (var i = 1; i < parts.length; i++) {
        if (files.containsKey(parts.take(i).join('/'))) {
          _invalid('Conflicting package paths');
        }
      }
    }
    return files;
  } on GizClawDeviceControlException {
    rethrow;
  } catch (_) {
    _invalid('Invalid tar.zlib package');
  }
}

Map<String, Object?> _manifest(
  Map<String, List<int>> files,
  String appName,
  String runtime,
  Set<String> capabilities,
) {
  try {
    final bytes = files['app.json'];
    if (bytes == null || bytes.length > 128 * 1024) {
      _invalid('Missing or oversized app.json');
    }
    final raw = jsonDecode(utf8.decode(bytes));
    if (raw is! Map<String, Object?> ||
        raw.keys.any(
          (k) => !{
            'app_name',
            'runtime',
            'entry',
            'methods',
            'requires',
          }.contains(k),
        ) ||
        raw['app_name'] != appName ||
        raw['runtime'] != runtime ||
        raw['entry'] is! String ||
        (raw['entry'] as String).runes.length > 1024 ||
        !_safePath(raw['entry'] as String) ||
        !(raw['entry'] as String).endsWith('.lua') ||
        !files.containsKey(raw['entry'])) {
      _invalid('Invalid manifest or runtime mismatch');
    }
    final requires = raw['requires'] ?? <Object?>[];
    if (requires is! List<Object?> ||
        requires.any((c) => c is! String || !capabilities.contains(c))) {
      _invalid('App requires a missing capability');
    }
    final methods = raw['methods'];
    if (methods is! List<Object?> || methods.length > 128) {
      _invalid('Invalid methods');
    }
    final names = <String>{};
    for (final m in methods) {
      if (m is! Map<String, Object?> ||
          m.keys.any(
            (k) => !{'name', 'mode', 'description', 'input_schema'}.contains(k),
          ) ||
          m['name'] is! String ||
          !_name.hasMatch(m['name'] as String) ||
          !names.add(m['name'] as String) ||
          !['call', 'job'].contains(m['mode']) ||
          m['description'] is! String ||
          (m['description'] as String).runes.length > 1024 ||
          m['input_schema'] is! Map<String, Object?>) {
        _invalid('Invalid method definition');
      }
    }
    for (final file in files.entries.where((e) => e.key.endsWith('.lua'))) {
      if (file.value.isNotEmpty && file.value.first == 27 ||
          file.value.contains(0)) {
        _invalid('Lua bytecode or NUL rejected');
      }
      utf8.decode(file.value);
    }
    return raw;
  } on GizClawDeviceControlException {
    rethrow;
  } catch (_) {
    _invalid('Invalid app.json or Lua text');
  }
}

String _quote(String value) =>
    '"${utf8.encode(value).map((b) => '\\${b.toString().padLeft(3, '0')}').join()}"';

String _chunk(
  Map<String, List<int>> files,
  Map<String, Object?> manifest,
  String? method,
  Object? args,
) {
  final source = StringBuffer(
    'local entry = (function()\n${utf8.decode(files[manifest['entry']]!)}\nend)()\n',
  );
  source.writeln('assert(type(entry)=="table", "entry must return a table")');
  for (final m
      in (manifest['methods'] as List).whereType<Map<String, Object?>>()) {
    source.writeln(
      'assert(type(entry[${_quote(m['name'] as String)}])=="function", "missing method")',
    );
  }
  source.writeln(
    method == null
        ? "return 'null'"
        : "local json = require('json'); return json.encode((entry[${_quote(method)}](json.decode(${_quote(jsonEncode(args))}))))",
  );
  return source.toString();
}

Future<Map<String, List<int>>> _unpackIsolate(
  List<int> bytes,
  int maxBytes,
  int maxFiles,
) => Isolate.run(() => _unpack(bytes, maxBytes, maxFiles));
