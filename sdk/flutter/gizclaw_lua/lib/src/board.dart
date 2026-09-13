import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// Board hardware data. Presentation belongs to [GizClawLuaBoardView].
class GizClawLuaBoard extends ChangeNotifier {
  GizClawLuaBoard({
    required this.displayWidth,
    required this.displayHeight,
    List<GizClawLuaButton> buttons = const [],
    this.touch = false,
    this.audioInput = false,
    this.audioOutput = false,
  }) : buttons = List.unmodifiable(buttons) {
    if (displayWidth < 1 ||
        displayHeight < 1 ||
        displayWidth > 4096 ||
        displayHeight > 4096 ||
        buttons.length > 8 ||
        buttons.map((b) => b.name).toSet().length != buttons.length ||
        buttons.any(
          (b) => !RegExp(r'^[A-Za-z_][A-Za-z0-9_-]{0,63}$').hasMatch(b.name),
        )) {
      throw ArgumentError('Invalid board dimensions or buttons');
    }
    _pixels = Uint16List(displayWidth * displayHeight);
  }
  final int displayWidth, displayHeight;
  final List<GizClawLuaButton> buttons;
  final bool touch, audioInput, audioOutput;
  late Uint16List _pixels;
  Uint16List get framebuffer => Uint16List.fromList(_pixels);
  int pixelAt(int x, int y) {
    RangeError.checkValueInInterval(x, 0, displayWidth - 1, 'x');
    RangeError.checkValueInInterval(y, 0, displayHeight - 1, 'y');
    return _pixels[y * displayWidth + x];
  }

  bool get running => _button != null;
  void Function(int, bool)? _button;
  void Function(int, int, int)? _touch;
  final _pressed = <String, Set<Object>>{};
  void bind(
    void Function(int, bool) button,
    void Function(int, int, int) touch,
  ) {
    if (running) throw StateError('Board already attached to a host');
    _button = button;
    _touch = touch;
    notifyListeners();
  }

  void unbind() {
    _button = null;
    _touch = null;
    _pressed.clear();
    notifyListeners();
  }

  void updateFramebuffer(Uint16List pixels) {
    if (pixels.length != displayWidth * displayHeight) {
      throw ArgumentError('Framebuffer size mismatch');
    }
    _pixels = Uint16List.fromList(pixels);
    notifyListeners();
  }

  /// Each input source owns its press; overlapping keyboard and pointer holds
  /// produce only one down/up pair.
  void pushButton(String name, bool down, {Object source = 'api'}) {
    final index = buttons.indexWhere((b) => b.name == name);
    if (index < 0) {
      throw ArgumentError.value(name, 'name', 'Unknown board button');
    }
    if (!running) return;
    final held = _pressed.putIfAbsent(name, () => {});
    final before = held.isNotEmpty;
    if (down) {
      held.add(source);
    } else {
      held.remove(source);
    }
    if (before != held.isNotEmpty) {
      _button!(index + 1, held.isNotEmpty);
      notifyListeners();
    }
  }

  int componentId(String name) {
    final index = buttons.indexWhere((b) => b.name == name);
    if (index < 0) throw ArgumentError.value(name, 'name', 'Unknown button');
    return index + 1;
  }

  void releaseButtons() {
    for (final entry in _pressed.entries.toList()) {
      for (final source in entry.value.toList()) {
        pushButton(entry.key, false, source: source);
      }
    }
  }

  bool isPressed(String name) => _pressed[name]?.isNotEmpty ?? false;
  void releaseSource(Object source) {
    for (final b in buttons) {
      pushButton(b.name, false, source: source);
    }
  }

  /// kind is 1 down, 2 move, or 3 up in the firmware Touch contract.
  void pushTouch(int kind, int x, int y) {
    if (!touch) throw StateError('Board has no touch input');
    _touch?.call(kind, x, y);
  }
}

class GizClawLuaButton {
  const GizClawLuaButton(this.name, {this.key});
  final String name;
  final LogicalKeyboardKey? key;
}

/// Supply [skin] to place firmware board slots in an arbitrary widget tree.
class GizClawLuaBoardView extends StatefulWidget {
  const GizClawLuaBoardView({
    super.key,
    required this.board,
    this.skin,
    this.controls,
  });
  final GizClawLuaBoard board;
  final Widget? skin;

  /// Host-owned controls, placed by the controls slot.
  final Widget? controls;
  @override
  State<GizClawLuaBoardView> createState() => _BoardViewState();
}

class _BoardViewState extends State<GizClawLuaBoardView>
    with WidgetsBindingObserver {
  final _keyboard = Object();
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didUpdateWidget(GizClawLuaBoardView old) {
    super.didUpdateWidget(old);
    if (old.board != widget.board) old.board.releaseSource(_keyboard);
  }

  @override
  void dispose() {
    widget.board.releaseSource(_keyboard);
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed) {
      widget.board.releaseButtons();
    }
  }

  @override
  Widget build(BuildContext context) => _BoardScope(
    board: widget.board,
    controls: widget.controls,
    child: Focus(
      autofocus: true,
      onFocusChange: (focus) {
        if (!focus) widget.board.releaseButtons();
      },
      onKeyEvent: (_, event) {
        for (final button in widget.board.buttons) {
          if (button.key == event.logicalKey) {
            widget.board.pushButton(
              button.name,
              event is! KeyUpEvent,
              source: _keyboard,
            );
            return KeyEventResult.handled;
          }
        }
        return KeyEventResult.ignored;
      },
      child:
          widget.skin ??
          Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const GizClawLuaBoardSlot.display(),
              Wrap(
                children: [
                  for (final button in widget.board.buttons)
                    GizClawLuaBoardSlot.button(button.name),
                ],
              ),
              const GizClawLuaBoardSlot.controls(),
              const GizClawLuaBoardSlot.status(),
            ],
          ),
    ),
  );
}

class _BoardScope extends InheritedNotifier<GizClawLuaBoard> {
  const _BoardScope({
    required GizClawLuaBoard board,
    required super.child,
    this.controls,
  }) : super(notifier: board);
  final Widget? controls;
  @override
  bool updateShouldNotify(_BoardScope old) =>
      controls != old.controls || super.updateShouldNotify(old);
  static GizClawLuaBoard of(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<_BoardScope>();
    if (scope == null) {
      throw StateError('Board slots require GizClawLuaBoardView');
    }
    return scope.notifier!;
  }
}

class GizClawLuaBoardSlot extends StatelessWidget {
  const GizClawLuaBoardSlot.display({super.key})
    : _kind = 'display',
      _name = null;
  const GizClawLuaBoardSlot.button(String name, {super.key})
    : _kind = 'button',
      _name = name;
  const GizClawLuaBoardSlot.controls({super.key})
    : _kind = 'controls',
      _name = null;
  const GizClawLuaBoardSlot.status({super.key})
    : _kind = 'status',
      _name = null;
  final String _kind;
  final String? _name;
  @override
  Widget build(BuildContext context) {
    final board = _BoardScope.of(context);
    switch (_kind) {
      case 'display':
        return _Display(board: board);
      case 'button':
        if (!board.buttons.any((b) => b.name == _name)) {
          throw ArgumentError('Unknown board button $_name');
        }
        return _Button(board: board, name: _name!);
      case 'controls':
        return KeyedSubtree(
          key: const ValueKey('board-controls'),
          child:
              context
                  .dependOnInheritedWidgetOfExactType<_BoardScope>()!
                  .controls ??
              const SizedBox.shrink(),
        );
      default:
        return Text(
          board.running ? 'Running' : 'Stopped',
          key: const ValueKey('board-status'),
        );
    }
  }
}

class _Button extends StatefulWidget {
  const _Button({required this.board, required this.name});
  final GizClawLuaBoard board;
  final String name;
  @override
  State<_Button> createState() => _ButtonState();
}

class _ButtonState extends State<_Button> {
  final _source = Object();
  final _pointers = <int>{};
  void _release() {
    for (final pointer in _pointers) {
      widget.board.releaseSource((_source, pointer));
    }
    _pointers.clear();
  }

  @override
  void didUpdateWidget(_Button old) {
    super.didUpdateWidget(old);
    if (old.board != widget.board || old.name != widget.name) {
      for (final pointer in _pointers) {
        old.board.releaseSource((_source, pointer));
      }
      _pointers.clear();
    }
  }

  @override
  void dispose() {
    _release();
    super.dispose();
  }

  void _edge(PointerEvent e, bool down) {
    if (down) {
      _pointers.add(e.pointer);
    } else {
      _pointers.remove(e.pointer);
    }
    widget.board.pushButton(widget.name, down, source: (_source, e.pointer));
  }

  @override
  Widget build(BuildContext context) => Listener(
    onPointerDown: (e) => _edge(e, true),
    onPointerUp: (e) => _edge(e, false),
    onPointerCancel: (e) => _edge(e, false),
    child: Semantics(
      button: true,
      label: widget.name,
      child: Container(
        padding: const EdgeInsets.all(12),
        margin: const EdgeInsets.all(4),
        color: widget.board.isPressed(widget.name) ? Colors.blue : Colors.grey,
        child: Text(widget.name),
      ),
    ),
  );
}

class _Display extends StatefulWidget {
  const _Display({required this.board});
  final GizClawLuaBoard board;
  @override
  State<_Display> createState() => _DisplayState();
}

class _DisplayState extends State<_Display> {
  ui.Image? _image;
  int _generation = 0;
  @override
  void initState() {
    super.initState();
    widget.board.addListener(_update);
    _update();
  }

  @override
  void didUpdateWidget(_Display old) {
    super.didUpdateWidget(old);
    if (old.board != widget.board) {
      old.board.removeListener(_update);
      widget.board.addListener(_update);
      _update();
    }
  }

  void _update() {
    final generation = ++_generation, pixels = widget.board.framebuffer;
    final rgba = Uint8List(pixels.length * 4);
    for (var i = 0; i < pixels.length; i++) {
      final p = pixels[i];
      rgba[i * 4] = ((p >> 11) & 31) * 255 ~/ 31;
      rgba[i * 4 + 1] = ((p >> 5) & 63) * 255 ~/ 63;
      rgba[i * 4 + 2] = (p & 31) * 255 ~/ 31;
      rgba[i * 4 + 3] = 255;
    }
    ui.decodeImageFromPixels(
      rgba,
      widget.board.displayWidth,
      widget.board.displayHeight,
      ui.PixelFormat.rgba8888,
      (image) {
        if (!mounted || generation != _generation) {
          image.dispose();
          return;
        }
        setState(() {
          _image?.dispose();
          _image = image;
        });
      },
    );
  }

  @override
  void dispose() {
    widget.board.removeListener(_update);
    _generation++;
    _image?.dispose();
    super.dispose();
  }

  void _point(PointerEvent e, int kind) {
    if (!widget.board.touch) return;
    final render = context.findRenderObject();
    if (render is! RenderBox || render.size.isEmpty) return;
    widget.board.pushTouch(
      kind,
      (e.localPosition.dx * widget.board.displayWidth / render.size.width)
          .floor()
          .clamp(0, widget.board.displayWidth - 1),
      (e.localPosition.dy * widget.board.displayHeight / render.size.height)
          .floor()
          .clamp(0, widget.board.displayHeight - 1),
    );
  }

  @override
  Widget build(BuildContext context) => Listener(
    onPointerDown: (e) => _point(e, 1),
    onPointerMove: (e) => _point(e, 2),
    onPointerUp: (e) => _point(e, 3),
    onPointerCancel: (e) => _point(e, 3),
    child: SizedBox(
      width: widget.board.displayWidth.toDouble(),
      height: widget.board.displayHeight.toDouble(),
      child: RawImage(image: _image, filterQuality: FilterQuality.none),
    ),
  );
}
