import 'dart:convert';
import 'dart:io';

import 'package:archive/archive.dart';
import 'package:cryptography/cryptography.dart';

// Populate all three fields together when GizOS publishes its source package.
const runtimePackagePin = (url: '', sha256: '', size: 0);
typedef PackagePin = ({String url, String sha256, int size});

Never invalidPackage(String message) =>
    throw StateError('GizOS Lua runtime source package: $message');

String safePackagePath(String path) {
  if (path.isEmpty ||
      path.startsWith('/') ||
      path.contains('\\') ||
      path.contains(':') ||
      path.contains('\u0000') ||
      path.split('/').any((p) => p.isEmpty || p == '.' || p == '..')) {
    invalidPackage('unsafe path: $path');
  }
  return path;
}

List<String> strings(Object? value, String field) {
  if (value is! List ||
      value.any((v) => v is! String || v.contains('\u0000'))) {
    invalidPackage('$field must be an array of strings');
  }
  return value.cast<String>();
}

class RuntimeManifest {
  RuntimeManifest(Map<String, Object?> json) {
    if (json['schema_version'] != 1 || json['schema_version'] is! int) {
      invalidPackage(
        'unsupported schema_version ${json['schema_version']}; expected 1',
      );
    }
    if (json['runtime_profile_id'] != 'runtime.lua.gizos') {
      invalidPackage(
        'unexpected runtime_profile_id ${json['runtime_profile_id']}',
      );
    }
    sources = strings(json['sources'], 'sources').map(safePackagePath).toList();
    includes = strings(
      json['include_dirs'],
      'include_dirs',
    ).map(safePackagePath).toList();
    defines = strings(json['defines'], 'defines');
    flags = strings(json['cflags'], 'cflags');
    final rawUnits = json['compilation_units'];
    if (rawUnits is! List) invalidPackage('missing compilation_units');
    units = rawUnits.map((u) {
      if (u is! Map<String, Object?>) {
        invalidPackage('invalid compilation unit');
      }
      return (
        sources: strings(
          u['sources'],
          'unit.sources',
        ).map(safePackagePath).toList(),
        flags: strings(u['cflags'], 'unit.cflags'),
        defines: strings(u['defines'], 'unit.defines'),
      );
    }).toList();
    final selected = units.expand((u) => u.sources).toList();
    if (sources.isEmpty ||
        sources.any((s) => !s.endsWith('.c')) ||
        sources.toSet().length != sources.length ||
        selected.length != sources.length ||
        selected.toSet().length != selected.length ||
        !selected.toSet().containsAll(sources)) {
      invalidPackage(
        'compilation_units must cover every C source exactly once',
      );
    }
    final perOS = json['per_os'];
    if (perOS is! Map<String, Object?>) invalidPackage('missing per_os');
    linkFlags = perOS.map((os, value) {
      if (value is! Map<String, Object?>) invalidPackage('invalid per_os.$os');
      return MapEntry(
        os,
        strings(value['link_flags'], 'per_os.$os.link_flags'),
      );
    });
  }
  late final List<String> sources, includes, defines, flags;
  late final List<
    ({List<String> sources, List<String> flags, List<String> defines})
  >
  units;
  late final Map<String, List<String>> linkFlags;
}

Future<String> verifyPackageBytes(List<int> bytes, PackagePin pin) async {
  final digest = (await Sha256().hash(
    bytes,
  )).bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
  if (pin.sha256.isNotEmpty || pin.size != 0) {
    if (!RegExp(r'^[a-f0-9]{64}$').hasMatch(pin.sha256) || pin.size <= 0) {
      invalidPackage('incomplete sha256/size pin');
    }
    if (digest != pin.sha256) {
      invalidPackage('sha256 mismatch: expected ${pin.sha256}, got $digest');
    }
    if (bytes.length != pin.size) {
      invalidPackage(
        'size mismatch: expected ${pin.size}, got ${bytes.length}',
      );
    }
  }
  return digest;
}

Future<Directory> resolveRuntimePackage({
  required String? localPath,
  required Directory cache,
  PackagePin pin = runtimePackagePin,
}) async {
  if (localPath != null && Directory(localPath).existsSync()) {
    final root = Directory(localPath).absolute;
    await validatePackageDirectory(root);
    return root;
  }
  List<int> bytes;
  if (localPath != null) {
    if (!localPath.endsWith('.tar.gz') || !File(localPath).existsSync()) {
      invalidPackage(
        'GIZOS_LUA_RUNTIME_SRC must name an existing .tar.gz or extracted directory',
      );
    }
    bytes = await File(localPath).readAsBytes();
  } else {
    if (pin.url.isEmpty || pin.sha256.isEmpty || pin.size <= 0) {
      invalidPackage(
        'no published package is pinned; set GIZOS_LUA_RUNTIME_SRC to a .tar.gz or extracted directory',
      );
    }
    final archive = File('${cache.path}/${pin.sha256}.tar.gz');
    if (archive.existsSync()) {
      bytes = await archive.readAsBytes();
    } else {
      final client = HttpClient();
      try {
        final response = await (await client.getUrl(
          Uri.parse(pin.url),
        )).close();
        if (response.statusCode != 200) {
          invalidPackage('download HTTP ${response.statusCode}');
        }
        bytes = <int>[];
        await for (final chunk in response) {
          bytes.addAll(chunk);
          if (bytes.length > pin.size) {
            invalidPackage('download exceeds pinned size');
          }
        }
      } finally {
        client.close(force: true);
      }
    }
  }
  final digest = await verifyPackageBytes(bytes, pin);
  await cache.create(recursive: true);
  final root = Directory('${cache.path}/$digest');
  // Extract into a fresh directory; never follow existing cache entries/links.
  final staging = await cache.createTemp('extract-');
  try {
    final decoder = TarDecoder();
    final archive = decoder.decodeBytes(GZipDecoder().decodeBytes(bytes));
    final rawNames = <String>{};
    for (final entry in decoder.files) {
      if (!['', '0', '5'].contains(entry.typeFlag) ||
          !rawNames.add(entry.filename)) {
        invalidPackage(
          'link, special file or duplicate entry: ${entry.filename}',
        );
      }
    }
    final seen = <String>{};
    for (final entry in archive) {
      final name = safePackagePath(
        entry.name.endsWith('/')
            ? entry.name.substring(0, entry.name.length - 1)
            : entry.name,
      );
      if (entry.isSymbolicLink || !seen.add(name)) {
        invalidPackage('link or duplicate entry: $name');
      }
      if (entry.isDirectory) {
        await Directory('${staging.path}/$name').create(recursive: true);
      } else {
        final file = File('${staging.path}/$name');
        await file.parent.create(recursive: true);
        await file.writeAsBytes(entry.content);
      }
    }
    await validatePackageDirectory(staging);
    if (await FileSystemEntity.type(root.path, followLinks: false) !=
        FileSystemEntityType.notFound) {
      if (await FileSystemEntity.type(root.path, followLinks: false) ==
          FileSystemEntityType.link) {
        invalidPackage('cache root is a symlink');
      }
      await root.delete(recursive: true);
    }
    await staging.rename(root.path);
    if (localPath == null) {
      await File('${cache.path}/$digest.tar.gz').writeAsBytes(bytes);
    }
  } finally {
    if (staging.existsSync()) await staging.delete(recursive: true);
  }
  return root;
}

Future<RuntimeManifest> validatePackageDirectory(Directory root) async {
  if (await FileSystemEntity.type(root.path, followLinks: false) ==
      FileSystemEntityType.link) {
    invalidPackage('root is a symlink');
  }
  await for (final entity in root.list(recursive: true, followLinks: false)) {
    if (entity is Link) invalidPackage('symlink: ${entity.path}');
  }
  final file = File('${root.path}/manifest.json');
  if (!file.existsSync()) invalidPackage('missing manifest.json');
  final json = jsonDecode(await file.readAsString());
  if (json is! Map<String, Object?>) {
    invalidPackage('manifest must be an object');
  }
  final manifest = RuntimeManifest(json);
  for (final source in manifest.sources) {
    if (!File('${root.path}/$source').existsSync()) {
      invalidPackage('missing source $source');
    }
  }
  for (final include in manifest.includes) {
    if (!Directory('${root.path}/$include').existsSync()) {
      invalidPackage('missing include directory $include');
    }
  }
  return manifest;
}
