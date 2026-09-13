import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_lua/gizclaw_lua.dart';

void main() {
  GizClawLuaBoard board() => GizClawLuaBoard(
    displayWidth: 240,
    displayHeight: 240,
    buttons: const [
      GizClawLuaButton('ok', key: LogicalKeyboardKey.enter),
      GizClawLuaButton('back'),
    ],
  );
  testWidgets(
    'default skin derives slots from board and merges input sources',
    (tester) async {
      final b = board();
      final edges = <String>[];
      b.bind((id, down) => edges.add('$id:$down'), (_, _, _) {});
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(body: GizClawLuaBoardView(board: b)),
        ),
      );
      await tester.pump();
      expect(find.text('ok'), findsOneWidget);
      expect(find.text('back'), findsOneWidget);
      expect(find.byType(RawImage), findsOneWidget);
      expect(find.text('Running'), findsOneWidget);
      await tester.sendKeyDownEvent(LogicalKeyboardKey.enter);
      final gesture = await tester.startGesture(
        tester.getCenter(find.text('ok')),
      );
      await gesture.up();
      expect(edges, ['1:true']);
      await tester.sendKeyUpEvent(LogicalKeyboardKey.enter);
      expect(edges, ['1:true', '1:false']);
      await tester.pumpWidget(const SizedBox());
      b.dispose();
    },
  );
  testWidgets('two pointers hold a button until both release', (tester) async {
    final b = board();
    final edges = <bool>[];
    b.bind((_, down) => edges.add(down), (_, _, _) {});
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: GizClawLuaBoardView(board: b)),
      ),
    );
    final point = tester.getCenter(find.text('ok'));
    final first = await tester.startGesture(point, pointer: 1);
    final second = await tester.startGesture(point, pointer: 2);
    await first.up();
    expect(edges, [true]);
    await second.up();
    expect(edges, [true, false]);
    await tester.pumpWidget(const SizedBox());
    b.dispose();
  });
  testWidgets('scaled display maps pointer coordinates to framebuffer', (
    tester,
  ) async {
    final b = GizClawLuaBoard(
      displayWidth: 240,
      displayHeight: 240,
      touch: true,
    );
    final points = <(int, int, int)>[];
    b.bind((_, _) {}, (kind, x, y) => points.add((kind, x, y)));
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: GizClawLuaBoardView(
            board: b,
            skin: const Align(
              alignment: Alignment.topLeft,
              child: SizedBox(
                width: 120,
                height: 120,
                child: GizClawLuaBoardSlot.display(),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tapAt(const Offset(60, 60));
    expect(points, [(1, 120, 120), (3, 120, 120)]);
    await tester.pumpWidget(const SizedBox());
    b.dispose();
  });
  testWidgets('custom skin binds display button controls and status slots', (
    tester,
  ) async {
    final b = board();
    final edges = <bool>[];
    b.bind((_, down) => edges.add(down), (_, _, _) {});
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: GizClawLuaBoardView(
            board: b,
            controls: const Text('Host actions'),
            skin: const Column(
              children: [
                Text('Custom shell'),
                GizClawLuaBoardSlot.status(),
                GizClawLuaBoardSlot.display(),
                Row(
                  children: [
                    GizClawLuaBoardSlot.button('back'),
                    GizClawLuaBoardSlot.controls(),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
    await tester.pump();
    expect(find.text('Custom shell'), findsOneWidget);
    expect(find.text('Host actions'), findsOneWidget);
    expect(find.text('ok'), findsNothing);
    await tester.tap(find.text('back'));
    expect(edges, [true, false]);
    expect(find.byKey(const ValueKey('board-controls')), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    b.dispose();
  });
}
