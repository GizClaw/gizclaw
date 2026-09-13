import 'dart:convert';
import 'dart:io';

import 'package:archive/archive.dart';
import 'package:cryptography/cryptography.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_lua/gizclaw_lua.dart';

void main() {
  late Directory directory;
  late GizClawLuaAppHost host;
  late HttpServer server;
  var package = <int>[];
  setUp(() async {
    directory = await Directory.systemTemp.createTemp('gizclaw-app-test-');
    host = GizClawLuaAppHost(
      runtime: 'runtime.lua.test',
      storageDirectory: directory,
    );
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      request.response.add(package);
      await request.response.close();
    });
  });
  tearDown(() async {
    await host.close();
    await server.close(force: true);
    await directory.delete(recursive: true);
  });
  Future<void> install({String? digest}) async {
    final hash = await Sha256().hash(package);
    await host.install(
      appName: 'sample',
      url: 'http://127.0.0.1:${server.port}/app',
      sha256:
          digest ??
          hash.bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join(),
      size: package.length,
    );
  }

  test('native VM executes text, reports memory, and rejects bytecode', () {
    final vm = GizClawLuaVm();
    try {
      expect(vm.executeText('return 6*7'), '42');
      expect(vm.memoryUsed, greaterThan(0));
      expect(() => vm.executeText('\u001bLua'), throwsStateError);
    } finally {
      vm.close();
    }
  });
  test(
    'install, confined require, JSON result, persistent list and uninstall',
    () async {
      package = samplePackage();
      await install();
      final list = await host.list();
      expect(list.runtime, 'runtime.lua.test');
      expect(list.apps.single.appName, 'sample');
      expect(
        jsonDecode(
          await host.invoke(
            'sample',
            'echo',
            '{"value":"hi\\n世界","array":[],"nil":null}',
          ),
        ),
        {'value': 'hi\n世界', 'array': [], 'nil': null, 'answer': 42},
      );
      await host.close();
      host = GizClawLuaAppHost(
        runtime: 'runtime.lua.test',
        storageDirectory: directory,
      );
      expect((await host.list()).apps.single.sha256, list.apps.single.sha256);
      await host.uninstall('sample');
      expect((await host.list()).apps, isEmpty);
    },
  );
  test(
    'Lua JSON decoder handles surrogate pairs and rejects malformed input',
    () async {
      package = samplePackage();
      await install();
      final encoded =
          '{"text":"\\ud83d\\ude00","values":[true,false,null,-1.25e2]}';
      expect(
        jsonDecode(
          await host.invoke('sample', 'echo', jsonEncode({'encoded': encoded})),
        ),
        jsonDecode(encoded),
      );
      for (final invalid in ['01', '[1,]', '{"a":}', '"\\ud800"']) {
        await expectLater(
          host.invoke('sample', 'echo', jsonEncode({'encoded': invalid})),
          throwsA(isA<GizClawDeviceControlException>()),
        );
      }
    },
  );
  test('package rejects Lua bytecode', () async {
    package = samplePackage(mainSource: '\u001bLua');
    await expectLater(install(), throwsA(isA<GizClawDeviceControlException>()));
  });
  test('expanded size and file count are bounded', () async {
    await host.close();
    host = GizClawLuaAppHost(
      runtime: 'runtime.lua.test',
      storageDirectory: directory,
      maxExpandedBytes: 1024,
    );
    package = samplePackage();
    await expectLater(install(), throwsA(isA<GizClawDeviceControlException>()));
    await host.close();
    host = GizClawLuaAppHost(
      runtime: 'runtime.lua.test',
      storageDirectory: directory,
      maxFiles: 2,
    );
    await expectLater(install(), throwsA(isA<GizClawDeviceControlException>()));
  });
  test('timeouts and protected loops release native execution', () async {
    await host.close();
    host = GizClawLuaAppHost(
      runtime: 'runtime.lua.test',
      storageDirectory: directory,
      executionTimeout: const Duration(milliseconds: 150),
    );
    package = samplePackage(
      mainSource:
          'return {echo=function(args) while true do pcall(function() while true do end end) end end, loop=function(args) end}',
    );
    await install();
    await expectLater(
      host.invoke('sample', 'echo', '{}').timeout(const Duration(seconds: 5)),
      throwsA(isA<GizClawDeviceControlException>()),
    );
  });
  test(
    'job completion reports results locally and removes finished jobs',
    () async {
      package = samplePackage(
        mainSource:
            'return {echo=function(args) return args end, loop=function(args) return 42 end}',
      );
      await install();
      final completion = host.jobCompletions.first;
      final id = await host.startJob('sample', 'loop', '{}');
      final event = await completion;
      expect(event.jobId, id);
      expect(event.resultJson, '42');
      expect(event.error, isNull);
      await expectLater(
        host.cancelJob(id),
        throwsA(isA<GizClawDeviceControlException>()),
      );
    },
  );
  test('rejects SHA mismatch without installing', () async {
    package = samplePackage();
    await expectLater(
      install(digest: '0' * 64),
      throwsA(isA<GizClawDeviceControlException>()),
    );
    expect((await host.list()).apps, isEmpty);
  });
  for (final path in ['../escape', '/absolute', 'a//b', 'a/./b', 'a\\b']) {
    test('rejects unsafe path $path', () async {
      package = samplePackage(extraPath: path);
      await expectLater(
        install(),
        throwsA(isA<GizClawDeviceControlException>()),
      );
    });
  }
  test('runtime mismatch preserves previous installation', () async {
    package = samplePackage();
    await install();
    final digest = (await host.list()).apps.single.sha256;
    package = samplePackage(runtime: 'runtime.lua.other');
    await expectLater(install(), throwsA(isA<GizClawDeviceControlException>()));
    expect((await host.list()).apps.single.sha256, digest);
  });
  test('job cancellation interrupts infinite native Lua loop', () async {
    package = samplePackage();
    await install();
    final id = await host.startJob('sample', 'loop', '{}');
    await host.cancelJob(id).timeout(const Duration(seconds: 5));
    await expectLater(
      host.invoke('sample', 'loop', '{}'),
      throwsA(isA<GizClawDeviceControlException>()),
    );
    await expectLater(
      host.invoke('sample', 'unknown', '{}'),
      throwsA(
        isA<GizClawDeviceControlException>().having(
          (e) => e.code,
          'code',
          StatusCode.STATUS_CODE_NOT_FOUND,
        ),
      ),
    );
    await expectLater(
      host.invoke('sample', 'echo', '[]'),
      throwsA(
        isA<GizClawDeviceControlException>().having(
          (e) => e.code,
          'code',
          StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        ),
      ),
    );
  });
}

List<int> samplePackage({
  String runtime = 'runtime.lua.test',
  String? extraPath,
  String? mainSource,
}) {
  final files = <String, String>{
    'app.json': jsonEncode({
      'app_name': 'sample',
      'runtime': runtime,
      'entry': 'main.lua',
      'methods': [
        {
          'name': 'echo',
          'mode': 'call',
          'description': 'Echo',
          'input_schema': <String, Object?>{},
        },
        {
          'name': 'loop',
          'mode': 'job',
          'description': 'Loop',
          'input_schema': <String, Object?>{},
        },
      ],
    }),
    'main.lua':
        mainSource ??
        'local helper = require("helper"); return {echo=function(args) args.answer=helper.answer; if args.encoded then return json.decode(args.encoded) end; return json.decode(json.encode(args)) end, loop=function(args) while true do end end}',
    'helper.lua': 'return {answer=42}',
    ?extraPath: 'bad',
  };
  final archive = Archive();
  for (final entry in files.entries) {
    final bytes = utf8.encode(entry.value);
    archive.addFile(ArchiveFile(entry.key, bytes.length, bytes));
  }
  return zlib.encode(TarEncoder().encode(archive));
}
