import 'dart:convert';
import 'dart:io';

import 'package:archive/archive.dart';
import 'package:flutter_test/flutter_test.dart';

import '../hook/runtime_package.dart';

Map<String, Object?> manifest() => {
  'schema_version': 1,
  'runtime_profile_id': 'runtime.lua.gizos',
  'sources': ['src/runtime.c'],
  'include_dirs': ['include'],
  'defines': ['COMMON=1'],
  'cflags': ['-std=c11'],
  'compilation_units': [
    {
      'sources': ['src/runtime.c'],
      'defines': ['UNIT=2'],
      'cflags': ['-Wall'],
    },
  ],
  'per_os': {
    'darwin': {
      'link_flags': ['-lm'],
    },
  },
};

void main() {
  late Directory temp;
  setUp(() async {
    temp = await Directory.systemTemp.createTemp('gcl-package-test-');
  });
  tearDown(() async {
    await temp.delete(recursive: true);
  });
  Matcher fails(String text) => throwsA(
    isA<StateError>().having((e) => e.message, 'message', contains(text)),
  );
  Future<File> tarball({Map<String, Object?>? json, ArchiveFile? extra}) async {
    final archive = Archive()
      ..addFile(
        ArchiveFile.string('manifest.json', jsonEncode(json ?? manifest())),
      )
      ..addFile(
        ArchiveFile.string('src/runtime.c', 'int runtime(void) { return 1; }'),
      )
      ..addFile(ArchiveFile.string('include/runtime.h', 'int runtime(void);'));
    if (extra != null) archive.addFile(extra);
    return File(
      '${temp.path}/runtime.tar.gz',
    ).writeAsBytes(GZipEncoder().encode(TarEncoder().encode(archive)));
  }

  test(
    'unpublished pin directs developer to the source package setting',
    () async {
      await expectLater(
        resolveRuntimePackage(localPath: null, cache: temp),
        fails('set GIZOS_LUA_RUNTIME_SRC'),
      );
    },
  );
  test('rejects unknown schema and profile', () {
    expect(
      () => RuntimeManifest(manifest()..['schema_version'] = 2),
      fails('schema_version 2'),
    );
    expect(
      () => RuntimeManifest(manifest()..['runtime_profile_id'] = 'other'),
      fails('runtime_profile_id other'),
    );
  });
  test('rejects compilation units that omit or duplicate a source', () {
    expect(
      () => RuntimeManifest(manifest()..['compilation_units'] = []),
      fails('exactly once'),
    );
    final json = manifest();
    (json['compilation_units'] as List).add(
      (json['compilation_units'] as List).first,
    );
    expect(() => RuntimeManifest(json), fails('exactly once'));
  });
  test('local tarball is verified with a pin before extraction', () async {
    final file = await tarball();
    final bytes = await file.readAsBytes();
    final digest = await verifyPackageBytes(bytes, runtimePackagePin);
    final pin = (url: '', sha256: digest, size: bytes.length);
    final root = await resolveRuntimePackage(
      localPath: file.path,
      cache: Directory('${temp.path}/cache'),
      pin: pin,
    );
    expect(root.path, endsWith(digest));
    final parsed = await validatePackageDirectory(root);
    expect(parsed.units.single.defines, ['UNIT=2']);
    expect(parsed.linkFlags['darwin'], ['-lm']);
    await file.writeAsBytes([...bytes.take(bytes.length - 1), bytes.last ^ 1]);
    await expectLater(
      resolveRuntimePackage(
        localPath: file.path,
        cache: Directory('${temp.path}/cache'),
        pin: pin,
      ),
      fails('sha256 mismatch'),
    );
    await expectLater(
      verifyPackageBytes(bytes, (
        url: '',
        sha256: digest,
        size: bytes.length + 1,
      )),
      fails('size mismatch'),
    );
  });
  test(
    'wrong schema fails during resolution and leaves no extracted cache',
    () async {
      final file = await tarball(json: manifest()..['schema_version'] = 9);
      final cache = Directory('${temp.path}/cache');
      await expectLater(
        resolveRuntimePackage(localPath: file.path, cache: cache),
        fails('schema_version 9'),
      );
      expect(cache.listSync(), isEmpty);
    },
  );
  for (final path in [
    '/absolute.c',
    '../escape.c',
    'src/../../escape.c',
    'C:/escape.c',
    'a\\escape.c',
  ]) {
    test('rejects archive path $path', () async {
      final file = await tarball(extra: ArchiveFile.string(path, 'bad'));
      await expectLater(
        resolveRuntimePackage(
          localPath: file.path,
          cache: Directory('${temp.path}/cache'),
        ),
        fails('unsafe path'),
      );
    });
  }
  test('rejects symlink archive entries', () async {
    final file = await tarball(extra: ArchiveFile.symlink('link', '/tmp'));
    await expectLater(
      resolveRuntimePackage(
        localPath: file.path,
        cache: Directory('${temp.path}/cache'),
      ),
      fails('link'),
    );
  });
  test('extracted directory accepted; symlink rejected', () async {
    final file = await tarball();
    final root = await resolveRuntimePackage(
      localPath: file.path,
      cache: Directory('${temp.path}/cache'),
    );
    expect(
      (await resolveRuntimePackage(localPath: root.path, cache: temp)).path,
      root.path,
    );
    await Link('${root.path}/unsafe').create('/tmp');
    await expectLater(
      resolveRuntimePackage(localPath: root.path, cache: temp),
      fails('symlink'),
    );
  });
  test(
    'downloads verify pin and reuse cached archive without server',
    () async {
      final file = await tarball();
      final bytes = await file.readAsBytes();
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      server.listen((request) async {
        request.response.add(bytes);
        await request.response.close();
      });
      final pin = (
        url: 'http://127.0.0.1:${server.port}/runtime.tar.gz',
        sha256: await verifyPackageBytes(bytes, runtimePackagePin),
        size: bytes.length,
      );
      final cache = Directory('${temp.path}/cache');
      try {
        await resolveRuntimePackage(localPath: null, cache: cache, pin: pin);
      } finally {
        await server.close(force: true);
      }
      final root = await resolveRuntimePackage(
        localPath: null,
        cache: cache,
        pin: pin,
      );
      expect(File('${root.path}/manifest.json').existsSync(), isTrue);
    },
  );
}
