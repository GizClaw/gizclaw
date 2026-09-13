import 'dart:async';
import 'dart:convert';
import 'dart:ffi';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';

// The asset name remains lua_vm.dart to share the firmware library with the
// separately usable VM Core wrapper.
const _asset = 'package:gizclaw_lua/src/lua_vm.dart';
@Native<
  Pointer<Void> Function(
    Pointer<Utf8>,
    Int,
    Int,
    Int,
    Int,
    Size,
    Size,
    Size,
    Int,
    Int,
  )
>(symbol: 'gcl_create', assetId: _asset)
external Pointer<Void> _create(
  Pointer<Utf8> root,
  int w,
  int h,
  int buttons,
  int touch,
  int memory,
  int source,
  int output,
  int timeout,
  int jobs,
);
@Native<Void Function(Pointer<Void>)>(symbol: 'gcl_destroy', assetId: _asset)
external void _destroy(Pointer<Void> host);
@Native<Int Function(Pointer<Void>, Pointer<Utf8>)>(
  symbol: 'gcl_register',
  assetId: _asset,
)
external int _register(Pointer<Void> host, Pointer<Utf8> name);
@Native<Int Function(Pointer<Void>)>(symbol: 'gcl_start', assetId: _asset)
external int _start(Pointer<Void> host);
@Native<Int Function(Pointer<Void>, Pointer<Utf8>, Pointer<Utf8>)>(
  symbol: 'gcl_submit',
  assetId: _asset,
)
external int _submit(
  Pointer<Void> host,
  Pointer<Utf8> name,
  Pointer<Utf8> source,
);
@Native<Int Function(Pointer<Void>, Int, Pointer<Utf8>, Size)>(
  symbol: 'gcl_status',
  assetId: _asset,
)
external int _status(
  Pointer<Void> host,
  int id,
  Pointer<Utf8> result,
  int size,
);
@Native<Void Function(Pointer<Void>, Int)>(
  symbol: 'gcl_release',
  assetId: _asset,
)
external void _release(Pointer<Void> host, int id);
@Native<Int Function(Pointer<Void>, Int)>(symbol: 'gcl_cancel', assetId: _asset)
external int _cancel(Pointer<Void> host, int id);
@Native<Void Function(Pointer<Void>)>(symbol: 'gcl_pump', assetId: _asset)
external void _pump(Pointer<Void> host);
@Native<Int Function(Pointer<Void>, Int, Int)>(
  symbol: 'gcl_button',
  assetId: _asset,
)
external int _button(Pointer<Void> host, int id, int down);
@Native<Int Function(Pointer<Void>, Int, Int, Int)>(
  symbol: 'gcl_touch',
  assetId: _asset,
)
external int _touch(Pointer<Void> host, int kind, int x, int y);
@Native<Uint64 Function(Pointer<Void>, Pointer<Uint16>)>(
  symbol: 'gcl_frame',
  assetId: _asset,
)
external int _frame(Pointer<Void> host, Pointer<Uint16> pixels);

final class _Event extends Struct {
  @Uint64()
  external int id;
  @Int32()
  external int index;
  @Int32()
  external int cancel;
  @Array(4096)
  external Array<Uint8> input;
  @Array(4096)
  external Array<Uint8> options;
}

@Native<Int Function(Pointer<Void>, Pointer<_Event>)>(
  symbol: 'gcl_poll',
  assetId: _asset,
)
external int _poll(Pointer<Void> host, Pointer<_Event> event);
@Native<Int Function(Pointer<Void>, Uint64, Int, Pointer<Utf8>, Pointer<Utf8>)>(
  symbol: 'gcl_complete',
  assetId: _asset,
)
external int _complete(
  Pointer<Void> host,
  int id,
  int rc,
  Pointer<Utf8> output,
  Pointer<Utf8> error,
);

class FirmwareCapability {
  FirmwareCapability(this.name, this.call, this.cancel);
  final String name;
  final Future<String> Function(String, String?) call;
  final void Function()? cancel;
}

class FirmwareRun {
  FirmwareRun(this.id);
  final int id;
  final completer = Completer<String>();
  Future<String> get done => completer.future;
  bool get completed => completer.isCompleted;
}

class FirmwareHost {
  FirmwareHost({
    required String root,
    required this.width,
    required this.height,
    required int buttons,
    required bool audioInput,
    required bool audioOutput,
    required bool touch,
    required int memory,
    required int source,
    required this.output,
    required int timeout,
    required int jobs,
    required this.capabilities,
    required this.onFrame,
  }) {
    _pointer = using(
      (a) => _create(
        root.toNativeUtf8(allocator: a),
        width,
        height,
        buttons,
        touch ? 1 : 0,
        memory,
        source,
        output,
        timeout,
        jobs,
      ),
    );
    if (_pointer == nullptr) {
      throw StateError('Firmware Host initialization failed');
    }
    _audioConfig(_pointer, audioInput ? 1 : 0, audioOutput ? 1 : 0);
    try {
      for (final c in capabilities) {
        final rc = using(
          (a) => _register(_pointer, c.name.toNativeUtf8(allocator: a)),
        );
        if (rc != 0) throw StateError('Capability ${c.name}: $rc');
      }
      final rc = _start(_pointer);
      if (rc != 0) throw StateError('Firmware Host start: $rc');
    } catch (_) {
      _destroy(_pointer);
      _pointer = nullptr;
      rethrow;
    }
    _timer = Timer.periodic(const Duration(milliseconds: 5), (_) => pump());
  }
  final int width, height, output;
  final List<FirmwareCapability> capabilities;
  final void Function(Uint16List) onFrame;
  Pointer<Void> _pointer = nullptr;
  late Timer _timer;
  final _runs = <int, FirmwareRun>{};
  final _pending = <int, FirmwareCapability>{};
  int _lastFrame = 0;
  FirmwareRun submit(String name, String source) {
    final id = using(
      (a) => _submit(
        _pointer,
        name.toNativeUtf8(allocator: a),
        source.toNativeUtf8(allocator: a),
      ),
    );
    if (id <= 0) throw StateError('Firmware job submission: $id');
    final run = FirmwareRun(id);
    _runs[id] = run;
    return run;
  }

  void cancel(int id) {
    if (_pointer != nullptr) _cancel(_pointer, id);
  }

  void button(int id, bool down) {
    final rc = _button(_pointer, id, down ? 1 : 0);
    if (rc != 0) throw StateError('Button edge: $rc');
  }

  void touch(int kind, int x, int y) {
    final rc = _touch(_pointer, kind, x, y);
    if (rc != 0) throw StateError('Touch point: $rc');
  }

  void pushAudio(Uint8List bytes) {
    using((a) {
      final p = a<Uint8>(bytes.length)
        ..asTypedList(bytes.length).setAll(0, bytes);
      final rc = _audioPush(_pointer, p, bytes.length);
      if (rc != 0) throw StateError('Audio input: $rc');
    });
  }

  Uint8List readAudio() => using((a) {
    final p = a<Uint8>(5120);
    final n = _audioRead(_pointer, p, 5120);
    return Uint8List.fromList(p.asTypedList(n));
  });
  void pump() {
    if (_pointer == nullptr) return;
    _pump(_pointer);
    using((a) {
      final e = a<_Event>();
      while (_poll(_pointer, e) != 0) {
        final id = e.ref.id, c = capabilities[e.ref.index];
        if (e.ref.cancel != 0) {
          if (_pending.remove(id) != null) _cancelHandler(c);
        } else {
          final input = (e.cast<Uint8>() + 16).cast<Utf8>().toDartString();
          final options = (e.cast<Uint8>() + 4112).cast<Utf8>().toDartString();
          _pending[id] = c;
          unawaited(
            Future.sync(
              () => c.call(input, options.isEmpty ? null : options),
            ).then(
              (value) => _finishCapability(id, 0, value, ''),
              onError: (Object error, StackTrace stack) =>
                  _finishCapability(id, -1, '', error.toString()),
            ),
          );
        }
      }
      if (_frame(_pointer, nullptr) != _lastFrame) {
        final pixels = a<Uint16>(width * height);
        _lastFrame = _frame(_pointer, pixels);
        onFrame(Uint16List.fromList(pixels.asTypedList(width * height)));
      }
      final result = a<Uint8>(output + 1).cast<Utf8>();
      for (final run in _runs.values.toList()) {
        final state = _status(_pointer, run.id, result, output + 1);
        if (state >= 3 || state < 0) {
          final text = result.toDartString();
          _release(_pointer, run.id);
          _runs.remove(run.id);
          if (state == 3) {
            run.completer.complete(text);
          } else {
            run.completer.completeError(
              StateError('Lua job state $state: $text'),
            );
          }
        }
      }
    });
  }

  void _finishCapability(int id, int rc, String value, String error) {
    if (_pointer == nullptr || _pending.remove(id) == null) return;
    if (utf8.encode(value).length >= 512 ||
        utf8.encode(error).length >= 192 ||
        value.contains('\u0000') ||
        error.contains('\u0000')) {
      rc = -1;
      value = '';
      error = 'Capability output exceeds firmware limits';
    }
    using(
      (a) => _complete(
        _pointer,
        id,
        rc,
        value.toNativeUtf8(allocator: a),
        error.toNativeUtf8(allocator: a),
      ),
    );
  }

  void _cancelHandler(FirmwareCapability capability) {
    try {
      capability.cancel?.call();
    } catch (error, stack) {
      scheduleMicrotask(() => Zone.current.handleUncaughtError(error, stack));
    }
  }

  Future<void> close() async {
    if (_pointer == nullptr) return;
    final runs = _runs.values.toList();
    for (final run in runs) {
      cancel(run.id);
    }
    for (final run in runs) {
      try {
        await run.done;
      } catch (_) {
        /* drained */
      }
    }
    _timer.cancel();
    for (final c in _pending.values.toList()) {
      _cancelHandler(c);
    }
    _pending.clear();
    _destroy(_pointer);
    _pointer = nullptr;
  }
}

@Native<Void Function(Pointer<Void>, Int, Int)>(
  symbol: 'gcl_audio_config',
  assetId: _asset,
)
external void _audioConfig(Pointer<Void> host, int input, int output);
@Native<Int Function(Pointer<Void>, Pointer<Uint8>, Size)>(
  symbol: 'gcl_audio_push',
  assetId: _asset,
)
external int _audioPush(Pointer<Void> host, Pointer<Uint8> bytes, int size);
@Native<Size Function(Pointer<Void>, Pointer<Uint8>, Size)>(
  symbol: 'gcl_audio_read',
  assetId: _asset,
)
external int _audioRead(Pointer<Void> host, Pointer<Uint8> bytes, int size);
