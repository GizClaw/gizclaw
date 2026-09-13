import 'dart:async';
import 'dart:convert';
import 'dart:ffi';
import 'dart:io';
import 'dart:isolate';
import 'dart:math';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:gizclaw/gizclaw.dart';

import 'lua_vm.dart';

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

/// Native Lua App host. The caller supplies its actual runtime profile ID;
/// VM Core does not provide the GizOS display/audio or Runtime Host modules.
///
/// Native assets compile Lua 5.5 and the VM Core directly from a GizOS checkout.
/// The build hook finds a sibling `gizos` checkout. To override it, run
/// `GIZOS_ROOT=/path/to/gizos ./tool/with_gizos.sh test` from this package;
/// the wrapper passes the path through Flutter's sanitized hook environment.
/// The hook downloads and verifies the Lua tarball pinned in `hook/build.dart`,
/// caching source/build artifacts under `.dart_tool`. No Bazel is invoked.
/// macOS desktop is the validated platform; this host requires native FFI.
///
/// `require("folder.module")` resolves only packaged `folder/module.lua` files.
/// `json.encode`, `json.decode`, `json.null`, and `json.array()` are available;
/// JSON nesting is limited to 64 levels and decode input to 64 KiB. Arguments
/// must be JSON objects; input_schema is metadata, not an argument validator.
/// Empty unmarked Lua tables encode as objects. Jobs have no result RPC;
/// completed jobs are removed and emitted on [jobCompletions], and cancellation
/// waits for native cleanup.
///
/// One host exclusively owns [storageDirectory]. Call [close] before releasing
/// it. Jobs run on isolates, have bounded execution, and cancel cooperatively.
class GizClawLuaAppHost implements GizClawAppHost {
  GizClawLuaAppHost({
    required this.runtime,
    required this.storageDirectory,
    this.maxPackageBytes = 8 * 1024 * 1024,
    this.maxExpandedBytes = 32 * 1024 * 1024,
    this.maxFiles = 256,
    this.memoryLimit = 8 * 1024 * 1024,
    this.sourceLimit = 2 * 1024 * 1024,
    this.outputLimit = 64 * 1024,
    this.executionTimeout = const Duration(seconds: 30),
    this.maxExecutions = 8,
  }) {
    if (!_runtime.hasMatch(runtime) ||
        maxPackageBytes <= 0 ||
        maxExpandedBytes <= 0 ||
        maxFiles <= 0 ||
        memoryLimit < 256 * 1024 ||
        sourceLimit <= 0 ||
        outputLimit <= 0 ||
        maxExecutions <= 0 ||
        executionTimeout.inMilliseconds <= 0 ||
        executionTimeout.inMilliseconds > 2147483647) {
      throw ArgumentError('Invalid App host configuration');
    }
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
  final _jobs = <int, _Run>{};
  final _runs = <_Run>{};
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
    return ClientAppListResponse(runtime: runtime, apps: apps);
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
    final manifest = _manifest(files, appName, runtime);
    // Compile the entry contract before replacing a working installation.
    final source = _chunk(files, manifest, null, null);
    try {
      await _execute(source);
    } on GizClawDeviceControlException catch (error) {
      _invalid('Invalid App entry: ${error.message}');
    }
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
      await File('${directory.path}/digest').writeAsString(sha256, flush: true);
      final index = await _index();
      final old = index[appName];
      index[appName] = generation;
      await _saveIndex(index);
      committed = true;
      if (old != null) {
        await Directory(
          '${storageDirectory.path}/$old',
        ).delete(recursive: true);
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
    await Directory(
      '${storageDirectory.path}/$generation',
    ).delete(recursive: true);
  });

  Future<String> _prepare(
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
    final manifest = _manifest(files, appName, runtime);
    final methods = manifest['methods'] as List<Object?>;
    final matches = methods.whereType<Map<String, Object?>>().where(
      (m) => m['name'] == method,
    );
    if (matches.isEmpty) _missing('Unknown method');
    if (matches.single['mode'] != mode) _invalid('Wrong method mode');
    return _chunk(files, manifest, method, args);
  });

  @override
  Future<String> invoke(String appName, String method, String argsJson) async =>
      _execute(await _prepare(appName, method, argsJson, 'call'));

  @override
  Future<int> startJob(String appName, String method, String argsJson) async {
    final source = await _prepare(appName, method, argsJson, 'job');
    final run = _start(source);
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
    cancelLuaControl(run.control);
    try {
      await run.done;
    } on GizClawDeviceControlException {
      /* Cancellation acknowledged. */
    }
  }

  _Run _start(String source) {
    if (_closed) throw StateError('App host closed');
    if (_runs.length >= maxExecutions) {
      throw const GizClawDeviceControlException(
        StatusCode.STATUS_CODE_RESOURCE_EXHAUSTED,
        'Execution capacity reached',
      );
    }
    final control = createLuaControl(executionTimeout.inMilliseconds);
    if (control == nullptr) throw StateError('Cannot allocate Lua control');
    final address = control.address;
    final memory = memoryLimit, input = sourceLimit, output = outputLimit;
    final run = _Run(control);
    _runs.add(run);
    run.done = _runIsolate(source, address, memory, input, output)
        .catchError(
          (Object error) => throw GizClawDeviceControlException(
            StatusCode.STATUS_CODE_INTERNAL,
            error.toString(),
          ),
        )
        .whenComplete(() {
          run.completed = true;
          _runs.remove(run);
          freeLuaControl(control);
        });
    return run;
  }

  Future<String> _execute(String source) => _start(source).done;

  @override
  Future<void> close() async {
    _closing = true;
    await _tail;
    _closed = true;
    final active = _runs.toList();
    for (final run in active) {
      cancelLuaControl(run.control);
    }
    for (final run in active) {
      try {
        await run.done;
      } on GizClawDeviceControlException {
        /* Drained. */
      }
    }
    await _jobCompletions.close();
  }
}

class _Run {
  _Run(this.control);
  final Pointer<Void> control;
  late Future<String> done;
  bool completed = false;
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
) {
  try {
    final bytes = files['app.json'];
    if (bytes == null || bytes.length > 128 * 1024) {
      _invalid('Missing or oversized app.json');
    }
    final raw = jsonDecode(utf8.decode(bytes));
    if (raw is! Map<String, Object?> ||
        raw.keys.any(
          (k) => !{'app_name', 'runtime', 'entry', 'methods'}.contains(k),
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

String _literal(Object? value, [int depth = 0]) {
  if (depth > 64) _invalid('JSON nesting limit exceeded');
  if (value == null) return 'json_null';
  if (value is String) {
    return '"${utf8.encode(value).map((b) => '\\${b.toString().padLeft(3, '0')}').join()}"';
  }
  if (value is num || value is bool) return '$value';
  if (value is List<Object?>) {
    return 'setmetatable({${value.map((v) => _literal(v, depth + 1)).join(',')}},array_mt)';
  }
  if (value is Map<String, Object?>) {
    return '{${value.entries.map((e) => '[${_literal(e.key)}]=${_literal(e.value, depth + 1)}').join(',')}}';
  }
  _invalid('Unsupported JSON');
}

String _chunk(
  Map<String, List<int>> files,
  Map<String, Object?> manifest,
  String? method,
  Object? args,
) {
  final source = StringBuffer(_encoder)..writeln(_decoder);
  for (final file in files.entries.where((e) => e.key.endsWith('.lua'))) {
    // Each file is a function body, compiled as text by the native VM. No Lua
    // load/loadfile or filesystem searcher is exposed to application code.
    source.writeln(
      'modules[${_literal(file.key)}]=function(...)\n${utf8.decode(file.value)}\nend',
    );
  }
  source.writeln('local entry = require_file(${_literal(manifest['entry'])})');
  source.writeln('assert(type(entry)=="table", "entry must return a table")');
  for (final m
      in (manifest['methods'] as List).whereType<Map<String, Object?>>()) {
    source.writeln(
      'assert(type(entry[${_literal(m['name'])}])=="function", "missing method")',
    );
  }
  source.writeln(
    method == null
        ? 'return "null"'
        : 'return encode(entry[${_literal(method)}](${_literal(args)}),0,{})',
  );
  return source.toString();
}

const _encoder = r'''
local json_null = {}; local array_mt = {}
local function quote(s)
  return '"' .. s:gsub('[%z\1-\31\\"]', function(c)
    return string.format('\\u%04x', string.byte(c))
  end) .. '"'
end
local function encode(v, depth, seen)
  assert(depth <= 64, 'JSON nesting limit')
  local t = type(v)
  if v == json_null or t == 'nil' then return 'null' end
  if t == 'boolean' then return tostring(v) end
  if t == 'string' then return quote(v) end
  if t == 'number' then
    assert(v == v and v ~= math.huge and v ~= -math.huge, 'nonfinite JSON number')
    return tostring(v)
  end
  assert(t == 'table' and not seen[v], 'unsupported or cyclic JSON value')
  seen[v] = true
  local count, array = 0, getmetatable(v) == array_mt
  for k in pairs(v) do count = count + 1 end
  if count > 0 then
    array = true
    for k in pairs(v) do if type(k) ~= 'number' or k < 1 or k > count or k % 1 ~= 0 then array = false end end
  end
  local out = {}
  if array then
    for i=1,count do out[i] = encode(v[i], depth+1, seen) end
  else
    for k,item in pairs(v) do
      assert(type(k)=='string', 'JSON object keys must be strings')
      out[#out+1] = quote(k)..':'..encode(item,depth+1,seen)
    end
  end
  seen[v] = nil
  return (array and '[' or '{') .. table.concat(out, ',') .. (array and ']' or '}')
end
local modules, loaded, loading = {}, {}, {}
local function require_file(path)
  assert(modules[path], 'unknown app module')
  if loaded[path] ~= nil then return loaded[path] end
  assert(not loading[path], 'circular require')
  loading[path] = true
  local value = modules[path](path)
  if value == nil then value = true end
  loaded[path], loading[path] = value, nil
  return value
end
function require(name)
  assert(type(name)=='string' and name:match('^[%a_][%w_.-]*$') and not name:find('..',1,true), 'invalid module name')
  return require_file(name:gsub('%.','/')..'.lua')
end
json = { null = json_null, encode = function(v) return encode(v,0,{}) end }
''';

Future<Map<String, List<int>>> _unpackIsolate(
  List<int> bytes,
  int limit,
  int count,
) => Isolate.run(() => _unpack(bytes, limit, count));
Future<String> _runIsolate(
  String source,
  int address,
  int memory,
  int input,
  int output,
) => Isolate.run(() {
  final vm = GizClawLuaVm(
    memoryLimit: memory,
    sourceLimit: input,
    outputLimit: output,
  );
  try {
    return executeLuaWithControl(vm, source, address);
  } finally {
    vm.close();
  }
});

const _decoder = r'''
local function decode(text)
  assert(type(text) == 'string' and #text <= 65536, 'JSON input limit')
  local pos = 1
  local function ws() local _, last = text:find('^[ \t\r\n]*',pos); pos = (last or pos-1)+1 end
  local function hex()
    local s = text:sub(pos,pos+3)
    assert(#s==4 and s:match('^%x%x%x%x$'), 'invalid unicode escape')
    pos = pos + 4
    return tonumber(s,16)
  end
  local function str()
    assert(text:sub(pos,pos)=='"', 'expected JSON string')
    pos = pos+1
    local out = {}
    while pos <= #text do
      local c = text:sub(pos,pos); pos = pos+1
      if c=='"' then return table.concat(out) end
      if c=='\\' then
        c = text:sub(pos,pos); pos = pos+1
        local escapes = {['"']='"',['\\']='\\',['/']='/',b='\b',f='\f',n='\n',r='\r',t='\t'}
        if c=='u' then
          local code = hex()
          if code >= 0xd800 and code <= 0xdbff then
            assert(text:sub(pos,pos+1)=='\\u', 'missing low surrogate'); pos=pos+2
            local low=hex(); assert(low>=0xdc00 and low<=0xdfff, 'invalid low surrogate')
            code=0x10000+(code-0xd800)*1024+low-0xdc00
          else assert(code<0xdc00 or code>0xdfff, 'unexpected low surrogate') end
          out[#out+1]=utf8.char(code)
        else assert(escapes[c], 'invalid escape'); out[#out+1]=escapes[c] end
      else
        assert(c:byte()>=32, 'unescaped control byte'); out[#out+1]=c
      end
    end
    error('unterminated JSON string')
  end
  local parse
  parse = function(depth)
    assert(depth<=64, 'JSON nesting limit'); ws()
    local c=text:sub(pos,pos)
    if c=='"' then return str() end
    if c=='{' or c=='[' then
      local array=c=='['; local close=array and ']' or '}'
      local value=array and setmetatable({},array_mt) or {}
      pos=pos+1; ws()
      if text:sub(pos,pos)==close then pos=pos+1; return value end
      local index=1
      while true do
        ws(); local key=index
        if not array then key=str(); ws(); assert(text:sub(pos,pos)==':','expected colon'); pos=pos+1 end
        value[key]=parse(depth+1); index=index+1; ws()
        c=text:sub(pos,pos); pos=pos+1
        if c==close then return value end
        assert(c==',','expected comma')
      end
    end
    for token,value in pairs({['true']=true,['false']=false,['null']=json_null}) do
      if text:sub(pos,pos+#token-1)==token then pos=pos+#token; return value end
    end
    local start=pos
    if c=='-' then pos=pos+1 end
    c=text:sub(pos,pos)
    if c=='0' then pos=pos+1
    else
      assert(c:match('^[1-9]$'),'invalid JSON value')
      repeat pos=pos+1 until not text:sub(pos,pos):match('^%d$')
    end
    if text:sub(pos,pos)=='.' then
      pos=pos+1; assert(text:sub(pos,pos):match('^%d$'),'invalid fraction')
      repeat pos=pos+1 until not text:sub(pos,pos):match('^%d$')
    end
    c=text:sub(pos,pos)
    if c=='e' or c=='E' then
      pos=pos+1; c=text:sub(pos,pos)
      if c=='+' or c=='-' then pos=pos+1 end
      assert(text:sub(pos,pos):match('^%d$'),'invalid exponent')
      repeat pos=pos+1 until not text:sub(pos,pos):match('^%d$')
    end
    local number=tonumber(text:sub(start,pos-1))
    assert(number and number~=math.huge and number~=-math.huge,'invalid number')
    return number
  end
  local value=parse(0); ws(); assert(pos>#text,'trailing JSON data'); return value
end
json.decode = decode
json.array = function() return setmetatable({},array_mt) end
''';
