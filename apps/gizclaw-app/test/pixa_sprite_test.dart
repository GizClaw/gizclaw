import 'dart:typed_data';

import 'package:flutter/cupertino.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/giz_ui/giz_ui.dart';
import 'package:gizclaw_app/pixa_sprite.dart';

import 'pixa_fixture_test_data.dart';

void main() {
  test('removes only background pixels connected to the frame edge', () {
    final data = Uint8ClampedList(5 * 5 * 4);
    for (var pixel = 0; pixel < 25; pixel += 1) {
      data.setRange(pixel * 4, pixel * 4 + 4, [10, 20, 30, 255]);
    }
    for (final pixel in [6, 7, 8, 11, 13, 16, 17, 18]) {
      data.setRange(pixel * 4, pixel * 4 + 4, [220, 40, 20, 255]);
    }
    final frame = PixaFrameRgba(width: 5, height: 5, data: data);

    final result = removePixaEdgeBackground(frame);

    expect(result.data.sublist(0, 4), [0, 0, 0, 0]);
    expect(result.data[3], 0);
    expect(result.data[12 * 4 + 3], 255);
    expect(result.data[13 * 4 + 3], 255);
  });

  testWidgets('renders a pixa-backed pet sprite fixture', (tester) async {
    final asset = validatePixa(
      makePetPixaFixture(),
      mode: PixaValidationMode.petdef,
    );

    await tester.pumpWidget(
      CupertinoApp(
        home: Center(child: PixaSprite(asset: asset, width: 32, height: 16)),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byType(CustomPaint), findsOneWidget);
  });

  testWidgets('shows a compact error state for unsupported pixa frames', (
    tester,
  ) async {
    final bytes = makePetPixaFixture();
    final payloadOffset = ByteData.sublistView(
      bytes,
    ).getUint32(32, Endian.little);
    final frameOffset = payloadOffset - 16;
    bytes[frameOffset + 2] = 1;
    final asset = validatePixa(bytes, mode: PixaValidationMode.petdef);

    await tester.pumpWidget(
      CupertinoApp(
        home: Center(child: PixaSprite(asset: asset, width: 32, height: 16)),
      ),
    );
    await tester.pumpAndSettle();

    expect(
      find.byIcon(GizIcons.exclamationmark_triangle),
      findsOneWidget,
    );
  });
}
