import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_app/main.dart';

void main() {
  testWidgets('shows workflow-first mobile shell', (tester) async {
    await tester.pumpWidget(const GizClawApp());

    expect(find.text('GizClaw'), findsOneWidget);
    expect(find.text('Daily Companion'), findsWidgets);
    expect(find.text('All Workflows'), findsOneWidget);
    expect(find.byIcon(Icons.explore), findsOneWidget);
  });

  testWidgets('opens workflow detail from browse', (tester) async {
    await tester.pumpWidget(const GizClawApp());

    await tester.tap(find.text('Daily Companion').first);
    await tester.pumpAndSettle();

    expect(find.text('Workspaces'), findsOneWidget);
    expect(find.text('Morning check-in'), findsOneWidget);
  });
}
