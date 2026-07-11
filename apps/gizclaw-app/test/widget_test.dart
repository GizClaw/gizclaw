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

    expect(find.text('Play your\nworkflows'), findsOneWidget);
    expect(find.text('Everyday companions'), findsOneWidget);
    expect(find.text('All Workflows'), findsOneWidget);
    expect(find.byIcon(Icons.explore), findsOneWidget);
  });

  testWidgets('opens workflow detail from browse', (tester) async {
    await pumpApp(tester);

    await tester.drag(find.byType(CustomScrollView), const Offset(0, -560));
    await tester.pump(const Duration(milliseconds: 500));

    final workflowTap = tester.widget<InkWell>(
      find
          .descendant(
            of: find.byType(WorkflowListTile).first,
            matching: find.byType(InkWell),
          )
          .first,
    );
    workflowTap.onTap!();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));

    expect(find.byType(WorkflowDetailPage), findsOneWidget);
  });

  testWidgets('opens collections and the full workflow list', (tester) async {
    await pumpApp(tester);

    final collectionTap = tester.widget<InkWell>(
      find
          .descendant(
            of: find.byType(FeaturedCollectionCard).first,
            matching: find.byType(InkWell),
          )
          .first,
    );
    collectionTap.onTap!();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    expect(find.byType(CollectionPage), findsOneWidget);

    await tester.pageBack();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    await tester.tap(find.text('View all'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    expect(find.byType(AllWorkflowsPage), findsOneWidget);
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

  testWidgets('shows redesigned friends, pet, and profile surfaces', (
    tester,
  ) async {
    await pumpApp(tester);

    await tester.tap(find.text('Friends'));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('YOUR CIRCLE'), findsOneWidget);
    expect(find.text('Avery'), findsOneWidget);

    await tester.tap(find.text('Pet'));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Miso'), findsOneWidget);
    expect(find.text('Level 7  ·  620 friendship XP'), findsOneWidget);

    await tester.tap(find.text('Me'));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Local client'), findsOneWidget);
    expect(find.text('Connected over WebRTC'), findsOneWidget);
  });
}
