import 'dart:io';

import 'package:code_assets/code_assets.dart';
import 'package:hooks/hooks.dart';
import 'package:native_toolchain_c/native_toolchain_c.dart';

import 'runtime_package.dart';

void main(List<String> args) async {
  await build(args, (input, output) async {
    if (!input.config.buildCodeAssets) return;
    final os = input.config.code.targetOS;
    if ((os != OS.macOS && os != OS.linux) ||
        (os == OS.macOS && !Platform.isMacOS) ||
        (os == OS.linux && !Platform.isLinux)) {
      throw UnsupportedError(
        'gizclaw_lua supports native macOS/Linux builds only; iOS/Android/Windows PAL integration is not provided',
      );
    }
    final setting = File.fromUri(
      input.packageRoot.resolve('.dart_tool/gizos-lua-runtime-src'),
    );
    final local =
        Platform.environment['GIZOS_LUA_RUNTIME_SRC'] ??
        input.userDefines.path('GIZOS_LUA_RUNTIME_SRC')?.toFilePath() ??
        (setting.existsSync() ? setting.readAsStringSync().trim() : null);
    if (setting.existsSync()) output.dependencies.add(setting.uri);
    final root = await resolveRuntimePackage(
      localPath: local,
      cache: Directory.fromUri(
        input.outputDirectory.resolve('runtime-sources/'),
      ),
    );
    if (local != null && File(local).existsSync()) {
      output.dependencies.add(File(local).uri);
    }
    final manifest = await validatePackageDirectory(root);
    output.dependencies.add(File('${root.path}/manifest.json').uri);
    final includes = manifest.includes.map((p) => '${root.path}/$p').toList();
    final archives = <String>[];
    for (var index = 0; index < manifest.units.length; index++) {
      final unit = manifest.units[index];
      final name = 'gizos_runtime_$index';
      await CBuilder.library(
        name: name,
        sources: unit.sources.map((p) => '${root.path}/$p').toList(),
        includes: includes,
        flags: [
          // CBuilder adds -Wl,-encryptable even during Apple static compilation.
          if (os == OS.macOS) '-Wno-unused-command-line-argument',
          ...manifest.flags,
          ...unit.flags,
          ...manifest.defines.map((d) => '-D$d'),
          ...unit.defines.map((d) => '-D$d'),
        ],
        buildModeDefine: false,
        ndebugDefine: false,
        linkModePreference: LinkModePreference.static,
      ).run(input: input, output: output);
      archives.add(
        input.outputDirectory
            .resolve(os.libraryFileName(name, StaticLinking()))
            .toFilePath(),
      );
    }
    final osName = os == OS.macOS ? 'darwin' : 'linux';
    final links = manifest.linkFlags[osName];
    if (links == null) invalidPackage('missing per_os.$osName.link_flags');
    await CBuilder.library(
      name: 'gizclaw_lua',
      assetName: 'src/lua_vm.dart',
      sources: ['native/host.c', 'native/os_posix.c'],
      includes: includes,
      flags: [
        '-std=c11',
        '-D_POSIX_C_SOURCE=200809L',
        if (os == OS.macOS) '-D_DARWIN_C_SOURCE',
        '-lpthread',
        ...links,
        if (os == OS.macOS)
          ...archives.expand((a) => ['-Xlinker', '-force_load', '-Xlinker', a])
        else ...[
          '-Wl,--whole-archive',
          ...archives,
          '-Wl,--no-whole-archive',
        ],
      ],
    ).run(input: input, output: output);
  });
}
