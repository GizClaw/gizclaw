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
    List<String> targetSources(String package, String target) {
      final file = File('${root.path}/$package/BUILD.bazel');
      output.dependencies.add(file.uri);
      final text = file.readAsStringSync();
      final block = RegExp(
        'name = "$target",([\\s\\S]*?)(?=\\n\\))',
      ).firstMatch(text);
      if (block == null) {
        throw StateError('Unsupported GizOS layout: $package:$target');
      }
      final src = RegExp(
        r'srcs = (\[[\s\S]*?\]|[A-Z_]+)',
      ).firstMatch(block[1]!);
      if (src == null) throw StateError('No srcs in $package:$target');
      var list = src[1]!;
      if (!list.startsWith('[')) {
        list = RegExp('$list = (\\[[\\s\\S]*?\\])').firstMatch(text)![1]!;
      }
      final paths = RegExp(
        r'"([^" ]+\.(?:c|cpp))"',
      ).allMatches(list).map((m) => '${root.path}/$package/${m[1]}').toList();
      if (paths.isEmpty || paths.any((p) => !File(p).existsSync())) {
        throw StateError('Invalid source list: $package:$target');
      }
      return paths;
    }

    sources.addAll(targetSources('libs/lua', 'lua_core'));
    sources.addAll(targetSources('libs/lua', 'lua_runtime'));
    sources.addAll(targetSources('libs/runtime', 'runtime'));
    sources.addAll(targetSources('libs/pal', 'unsupported'));
    sources.addAll(
      targetSources('libs/pal/providers/posix/pal_core', 'host_fs'),
    );
    final desktop = targetSources(
      'libs/pal/providers/desktop/pal_core',
      'core',
    );
    // CBuilder applies one language to all inputs. Compile the desktop C++
    // provider separately, then link its object into the C firmware library.
    final object = File.fromUri(input.outputDirectory.resolve('desktop.o'));
    final includePaths = [
      vendor.path,
      '${root.path}/libs/lua/include',
      '${root.path}/libs/runtime/include',
      '${root.path}/libs/pal/include',
      '${root.path}/libs/pal/providers/desktop/pal_core/include',
      '${root.path}/libs/pal/providers/posix/pal_core/include',
      '${root.path}/libs/lua/src',
    ];
    if ((input.config.code.targetOS != OS.macOS &&
            input.config.code.targetOS != OS.linux) ||
        (input.config.code.targetOS == OS.macOS) != Platform.isMacOS) {
      throw UnsupportedError(
        'gizclaw_lua currently builds on native macOS/Linux hosts only',
      );
    }
    final desktopInputs = <File>[...desktop.map(File.new)];
    for (final dir in [
      '${root.path}/libs/pal/include',
      '${root.path}/libs/pal/providers/desktop/pal_core/include',
    ]) {
      desktopInputs.addAll(
        Directory(dir)
            .listSync(recursive: true)
            .whereType<File>()
            .where((f) => f.path.endsWith('.h')),
      );
    }
    final fingerprintBytes = <int>[];
    for (final file in desktopInputs) {
      fingerprintBytes.addAll(file.readAsBytesSync());
    }
    final fingerprint =
        '${await Sha256().hash(fingerprintBytes)}:${input.config.code.targetArchitecture}';
    final fingerprintFile = File.fromUri(
      input.outputDirectory.resolve('desktop.sha256'),
    );
    if (!object.existsSync() ||
        !fingerprintFile.existsSync() ||
        fingerprintFile.readAsStringSync() != fingerprint) {
      final compiled = await Process.run('c++', [
        '-std=c++17',
        if (Platform.isMacOS) ...[
          '-mmacosx-version-min=${input.config.code.macOS.targetVersion}.0',
          '-arch',
          input.config.code.targetArchitecture == Architecture.x64
              ? 'x86_64'
              : '${input.config.code.targetArchitecture}',
        ],
        '-fPIC',
        '-c',
        ...desktop,
        ...includePaths.expand((p) => ['-I', p]),
        '-o',
        object.path,
      ]);
      if (compiled.exitCode != 0) {
        throw StateError('Desktop PAL: ${compiled.stderr}');
      }
      fingerprintFile.writeAsStringSync(fingerprint);
    }
    output.dependencies.addAll(desktop.map((p) => File(p).uri));
    sources.add(object.path);
    final module = File('${root.path}/MODULE.bazel');
    output.dependencies.add(module.uri);
    final pin = RegExp(
      r'name = "h2_vendor_yyjson",([\s\S]*?)\n\)',
    ).firstMatch(module.readAsStringSync());
    if (pin == null) throw StateError('Missing GizOS yyjson pin');
    final url = RegExp(r'urls = \["([^" ]+)"\]').firstMatch(pin[1]!)![1]!;
    final sha = RegExp(r'sha256 = "([a-f0-9]+)"').firstMatch(pin[1]!)![1]!;
    final yy = Directory.fromUri(input.outputDirectory.resolve('yyjson/'));
    await yy.create(recursive: true);
    final cached = File('${yy.path}/archive.tar.gz');
    List<int> yyBytes;
    if (cached.existsSync()) {
      yyBytes = cached.readAsBytesSync();
    } else {
      final client = HttpClient();
      try {
        final response = await (await client.getUrl(Uri.parse(url))).close();
        if (response.statusCode != 200) {
          throw StateError('yyjson download failed');
        }
        yyBytes = await response.fold<List<int>>([], (a, b) => a..addAll(b));
      } finally {
        client.close(force: true);
      }
    }
    final yyDigest = (await Sha256().hash(
      yyBytes,
    )).bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
    if (yyDigest != sha) throw StateError('yyjson digest mismatch');
    if (!cached.existsSync()) await cached.writeAsBytes(yyBytes);
    for (final f in TarDecoder().decodeBytes(
      GZipDecoder().decodeBytes(yyBytes),
    )) {
      if (f.isFile &&
          (f.name.endsWith('/src/yyjson.c') ||
              f.name.endsWith('/src/yyjson.h'))) {
        final target = File('${yy.path}/${f.name.split('/').last}');
        if (!target.existsSync()) await target.writeAsBytes(f.content);
      }
    }
    sources.add('${yy.path}/yyjson.c');
    sources.add('native/host.c');
    await CBuilder.library(
      name: 'gizclaw_lua',
      assetName: 'src/lua_vm.dart',
      sources: sources,
      includes: [...includePaths, '${root.path}/libs/lua/src/core', yy.path],
      flags: [
        '-std=c11',
        '-D_DEFAULT_SOURCE',
        '-D_XOPEN_SOURCE=700',
        if (Platform.isMacOS) '-lc++' else '-lstdc++',
        '-lpthread',
      ],
    ).run(input: input, output: output);
  });
}
