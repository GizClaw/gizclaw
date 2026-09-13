import 'dart:convert';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

final class _Config extends Struct {
  external Pointer<Void> allocator;
  external Pointer<Void> user;
  @Size()
  external int memory;
  @Size()
  external int source;
  @Size()
  external int output;
}

final class _Execution extends Struct {
  external Pointer<Uint8> output;
  @Size()
  external int capacity;
  @Size()
  external int size;
  external Pointer<Uint8> error;
  @Size()
  external int errorCapacity;
  @Size()
  external int errorSize;
}

@Native<Int32 Function(Pointer<_Config>, Pointer<Pointer<Void>>)>(
  symbol: 'h2_lua_vm_create',
)
external int _create(Pointer<_Config> config, Pointer<Pointer<Void>> vm);
@Native<Void Function(Pointer<Void>)>(symbol: 'h2_lua_vm_close')
external void _close(Pointer<Void> vm);
@Native<Size Function(Pointer<Void>)>(symbol: 'h2_lua_vm_memory_used')
external int _memory(Pointer<Void> vm);
@Native<
  Int32 Function(
    Pointer<Void>,
    Pointer<Utf8>,
    Pointer<Uint8>,
    Size,
    Pointer<_Execution>,
  )
>(symbol: 'h2_lua_vm_execute_text')
external int _execute(
  Pointer<Void> vm,
  Pointer<Utf8> name,
  Pointer<Uint8> source,
  int size,
  Pointer<_Execution> execution,
);
@Native<Pointer<Void> Function(Int32)>(symbol: 'gizclaw_lua_control_create')
external Pointer<Void> createLuaControl(int timeoutMs);
@Native<Void Function(Pointer<Void>, Pointer<Void>)>(
  symbol: 'gizclaw_lua_control_attach',
)
external void _attach(Pointer<Void> vm, Pointer<Void> control);
@Native<Void Function(Pointer<Void>)>(symbol: 'gizclaw_lua_control_cancel')
external void cancelLuaControl(Pointer<Void> control);
@Native<Void Function(Pointer<Void>)>(symbol: 'gizclaw_lua_control_free')
external void freeLuaControl(Pointer<Void> control);

/// Isolated GizOS Lua VM Core. Close on its owning isolate after execution.
class GizClawLuaVm {
  GizClawLuaVm({
    this.memoryLimit = 8 * 1024 * 1024,
    this.sourceLimit = 2 * 1024 * 1024,
    this.outputLimit = 64 * 1024,
  }) {
    if (memoryLimit < 256 * 1024 || sourceLimit <= 0 || outputLimit <= 0) {
      throw ArgumentError('Invalid Lua VM limits');
    }
    using((arena) {
      final config = arena<_Config>();
      config.ref
        ..memory = memoryLimit
        ..source = sourceLimit
        ..output = outputLimit;
      final result = arena<Pointer<Void>>();
      final code = _create(config, result);
      if (code != 0) throw StateError('Lua VM creation failed: $code');
      _vm = result.value;
    });
  }
  final int memoryLimit, sourceLimit, outputLimit;
  Pointer<Void> _vm = nullptr;
  int get memoryUsed => _memory(_vm);
  String executeText(String source) => _executeText(source);

  String _executeText(String source, {int? controlAddress}) {
    if (_vm == nullptr) throw StateError('VM closed');
    if (controlAddress != null) {
      _attach(_vm, Pointer.fromAddress(controlAddress));
    }
    return using((arena) {
      final bytes = utf8.encode(source);
      if (bytes.length > sourceLimit) {
        throw StateError('Lua source limit exceeded');
      }
      final input = arena<Uint8>(bytes.length + 1)
        ..asTypedList(bytes.length).setAll(0, bytes);
      final execution = arena<_Execution>();
      execution.ref
        ..output = arena<Uint8>(outputLimit + 1)
        ..capacity = outputLimit + 1
        ..error = arena<Uint8>(4096)
        ..errorCapacity = 4096;
      final code = _execute(
        _vm,
        'app'.toNativeUtf8(allocator: arena),
        input,
        bytes.length,
        execution,
      );
      if (code != 0) {
        throw StateError(
          'Lua error $code: ${utf8.decode(execution.ref.error.asTypedList(execution.ref.errorSize), allowMalformed: true)}',
        );
      }
      return utf8.decode(execution.ref.output.asTypedList(execution.ref.size));
    });
  }

  void close() {
    if (_vm != nullptr) {
      _close(_vm);
      _vm = nullptr;
    }
  }
}

// Internal adapter used only by the native App host on the VM-owning isolate.
String executeLuaWithControl(GizClawLuaVm vm, String source, int address) =>
    vm._executeText(source, controlAddress: address);
