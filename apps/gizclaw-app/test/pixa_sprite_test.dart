import 'dart:typed_data';

import 'package:flutter/cupertino.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/pixa_sprite.dart';

import 'pixa_fixture_test_data.dart';

void main() {
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
      find.byIcon(CupertinoIcons.exclamationmark_triangle),
      findsOneWidget,
    );
  });
}
