import 'dart:io';

import 'package:archive/archive.dart';
import 'package:code_assets/code_assets.dart';
import 'package:cryptography/cryptography.dart';
import 'package:hooks/hooks.dart';
import 'package:native_toolchain_c/native_toolchain_c.dart';

// Fetch the exact Lua archive pinned by GizOS MODULE.bazel. Desktop needs no
// embedded stdio overlay. GizOS sources are read in place, never vendored.
void main(List<String> args) async {
  await build(args, (input, output) async {
    if (!input.config.buildCodeAssets) return;
    var ancestor = Directory.fromUri(input.packageRoot);
    while (!Directory(
          '${ancestor.path}/gizos/libs/lua/src/core',
        ).existsSync() &&
        ancestor.parent.path != ancestor.path) {
      ancestor = ancestor.parent;
    }
    final rootFile = File.fromUri(
      input.packageRoot.resolve('.dart_tool/gizos-root'),
    );
    final root = Directory(
      Platform.environment['GIZOS_ROOT'] ??
          input.userDefines.path('gizos_root')?.toFilePath() ??
          (rootFile.existsSync()
              ? rootFile.readAsStringSync().trim()
              : '${ancestor.path}/gizos'),
    );
    if (!rootFile.existsSync()) {
      await rootFile.parent.create(recursive: true);
      await rootFile.writeAsString(root.absolute.path);
    }
    output.dependencies.add(rootFile.uri);

    if (!Directory('${root.path}/libs/lua/src/core').existsSync()) {
      throw StateError(
        'Set GIZOS_ROOT to a GizOS checkout containing libs/lua',
      );
    }
    const revision = 'a5522f06d2679b8f18534fd6a9968f7eb539dc31';
    final vendor = Directory.fromUri(input.outputDirectory.resolve('lua/'));
    await vendor.create(recursive: true);
    final archiveFile = File.fromUri(
      input.outputDirectory.resolve('lua-$revision.tar.gz'),
    );
    late List<int> bytes;
    if (archiveFile.existsSync()) {
      bytes = await archiveFile.readAsBytes();
    } else {
      final client = HttpClient();
      try {
        final response = await (await client.getUrl(
          Uri.parse('https://codeload.github.com/lua/lua/tar.gz/$revision'),
        )).close();
        if (response.statusCode != 200) throw StateError('Lua download failed');
        bytes = await response.fold<List<int>>([], (a, b) => a..addAll(b));
      } finally {
        client.close(force: true);
      }
    }
    final hash = await Sha256().hash(bytes);
    final digest = hash.bytes
        .map((b) => b.toRadixString(16).padLeft(2, '0'))
        .join();
    if (digest !=
        'b85ede9dbc4a4292addcfefebe15168acc7f1d42c674cc297d74f0e8ff2f2dd8') {
      throw StateError('Lua archive digest mismatch');
    }
    if (!archiveFile.existsSync()) await archiveFile.writeAsBytes(bytes);
    for (final file in TarDecoder().decodeBytes(
      GZipDecoder().decodeBytes(bytes),
    )) {
      if (!file.isFile) continue;
      final name = file.name.split('/').last;
      if (!RegExp(r'^[A-Za-z0-9_.-]+$').hasMatch(name)) continue;
      final target = File('${vendor.path}/$name');
      if (!target.existsSync()) await target.writeAsBytes(file.content);
    }
    output.dependencies.add(
      File('${root.path}/third_party/lua.BUILD.bazel').uri,
    );
    final buildFile = await File(
      '${root.path}/third_party/lua.BUILD.bazel',
    ).readAsString();
    final sources = RegExp(
      r'"src/([a-z0-9]+\.c)"',
    ).allMatches(buildFile).map((m) => '${vendor.path}/${m[1]}').toList();
    final core = Directory('${root.path}/libs/lua/src/core');
    sources.addAll(
      core
          .listSync()
          .whereType<File>()
          .where((f) => f.path.endsWith('.c'))
          .map((f) => f.path),
    );
    sources.add('native/control.c');
    await CBuilder.library(
      name: 'gizclaw_lua',
      assetName: 'src/lua_vm.dart',
      sources: sources,
      includes: [vendor.path, '${root.path}/libs/lua/include', core.path],
      flags: ['-std=c11'],
    ).run(input: input, output: output);
  });
}
