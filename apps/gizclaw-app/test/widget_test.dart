import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_app/main.dart';

void main() {
  Future<void> pumpApp(WidgetTester tester) async {
    await tester.pumpWidget(const GizClawApp());
    await tester.pump(const Duration(milliseconds: 700));
  }

  testWidgets('shows workflow-first mobile shell', (tester) async {
    await pumpApp(tester);

    expect(find.text('PLAY YOUR\nWORKFLOWS'), findsOneWidget);
    expect(find.text('Daily Companion'), findsWidgets);
    expect(find.text('All Workflows'), findsOneWidget);
    expect(find.byIcon(Icons.explore), findsOneWidget);
  });

  testWidgets('opens workflow detail from browse', (tester) async {
    await pumpApp(tester);

    final cardTap = tester.widget<InkWell>(
      find
          .descendant(
            of: find.byType(FeaturedWorkflowCard).first,
            matching: find.byType(InkWell),
          )
          .first,
    );
    cardTap.onTap!();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));

    expect(find.byType(WorkflowDetailPage), findsOneWidget);
  });

  testWidgets('shows workspace and group chat tabs', (tester) async {
    await pumpApp(tester);

    await tester.tap(find.text('Chats'));
    await tester.pump(const Duration(milliseconds: 700));

    expect(find.text('Workspace'), findsOneWidget);
    expect(find.text('Group Chat'), findsOneWidget);
    expect(find.text('Morning check-in'), findsWidgets);

    await tester.tap(find.text('Group Chat'));
    await tester.pump(const Duration(milliseconds: 700));

    expect(find.text('Home Room'), findsOneWidget);
  });

  testWidgets('shows five primary destinations', (tester) async {
    await pumpApp(tester);

    expect(find.text('Browse'), findsOneWidget);
    expect(find.text('Chats'), findsOneWidget);
    expect(find.text('Friends'), findsOneWidget);
    expect(find.text('Pet'), findsOneWidget);
    expect(find.text('Me'), findsOneWidget);
  });
}
