import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_app/pixa_sprite.dart';

import 'package:gizclaw_app/main.dart';

void main() {
  testWidgets('renders the Pixa smoke surface', (WidgetTester tester) async {
    await tester.pumpWidget(const MyApp());
    await tester.pumpAndSettle();

    expect(find.text('GizClaw Pixa'), findsOneWidget);
    expect(find.text('Miso Preview'), findsOneWidget);
    expect(find.textContaining('clip idle'), findsOneWidget);
    expect(find.byType(PixaSprite), findsOneWidget);
  });
}
