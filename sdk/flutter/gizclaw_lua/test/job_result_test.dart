import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_lua/src/firmware_host.dart';

void main() {
  late Directory root;
  late FirmwareHost host;
  setUp(() async {
    root = await Directory.systemTemp.createTemp('gcl-job-result-');
    host = FirmwareHost(
      root: root.path,
      width: 1,
      height: 1,
      buttons: 0,
      audioInput: false,
      audioOutput: false,
      touch: false,
      memory: 8 * 1024 * 1024,
      source: 4096,
      output: 256,
      timeout: 1000,
      jobs: 2,
      capabilities: [],
      onFrame: (_) {},
    );
  });
  tearDown(() async {
    await host.close();
    await root.delete(recursive: true);
  });
  for (final entry in {
    'return': 'null',
    "return ''": '',
    'return nil': 'nil',
    'return false': 'false',
    'return 42, 99': '42',
    "return 'a' .. string.char(0) .. 'b'": 'a\u0000b',
    "return setmetatable({}, {__tostring=function() return 'value' end})":
        'value',
    "return string.rep('x',256)": 'x' * 256,
  }.entries) {
    test('public job result: ${entry.key}', () async {
      expect(await host.submit('@result.lua', entry.key).done, entry.value);
    });
  }
  test('oversized result fails without truncation', () async {
    await expectLater(
      host.submit('@result.lua', "return string.rep('x',257)").done,
      throwsA(
        isA<StateError>().having(
          (e) => e.message,
          'message',
          contains('H2_LUA_VM_OUTPUT_TOO_LARGE'),
        ),
      ),
    );
  });
  test('filesystem refuses a symlink to a Lua module outside root', () async {
    final outside = await Directory.systemTemp.createTemp('gcl-outside-');
    try {
      await File('${outside.path}/module.lua').writeAsString('return 42');
      await Link(
        '${root.path}/helper.lua',
      ).create('${outside.path}/module.lua');
      await expectLater(
        host.submit('@result.lua', "return require('helper')").done,
        throwsStateError,
      );
    } finally {
      await outside.delete(recursive: true);
    }
  });
  test('private result module is unavailable', () async {
    await expectLater(
      host.submit('@result.lua', "return require('_gizclaw_result')").done,
      throwsStateError,
    );
  });
}
