import 'package:flutter/cupertino.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_app/main.dart';
import 'package:gizclaw_app/data/mobile_data_controller.dart';

void main() {
  Future<void> pumpApp(WidgetTester tester) async {
    await tester.pumpWidget(
      GizClawApp(dataController: MobileDataController.demo()),
    );
    await tester.pump(const Duration(milliseconds: 700));
  }

  testWidgets('shows the Cupertino workflow-first shell', (tester) async {
    await pumpApp(tester);

    expect(find.text('Play your\nworkflows'), findsOneWidget);
    expect(find.text('Everyday companions'), findsOneWidget);
    expect(find.text('Jump back in'), findsOneWidget);
    expect(find.byIcon(CupertinoIcons.compass_fill), findsOneWidget);
    expect(find.byType(CupertinoTabBar), findsOneWidget);
  });

  testWidgets('opens workflow detail from browse', (tester) async {
    await pumpApp(tester);

    await tester.drag(
      find.byType(CustomScrollView).first,
      const Offset(0, -560),
    );
    await tester.pump(const Duration(milliseconds: 400));
    await tester.tap(find.byType(WorkflowListTile).first);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));

    expect(find.byType(WorkflowDetailPage), findsOneWidget);
    expect(find.byType(WorkflowArtworkHero), findsOneWidget);
  });

  testWidgets('opens collections and the full workflow list', (tester) async {
    await pumpApp(tester);

    await tester.tap(find.byType(FeaturedCollectionCard).first);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    expect(find.byType(CollectionPage), findsOneWidget);
    expect(find.byType(CollectionArtworkHero), findsOneWidget);
    expect(find.byType(WorkflowArtworkHero), findsNothing);

    await tester.pageBack();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    await tester.drag(
      find.byType(CustomScrollView).first,
      const Offset(0, -440),
    );
    await tester.pump(const Duration(milliseconds: 400));
    await tester.tap(find.text('View all'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    expect(find.byType(AllWorkflowsPage), findsOneWidget);
  });

  testWidgets('opens chat types before their conversations', (tester) async {
    await pumpApp(tester);

    await tester.tap(find.text('Chats'));
    await tester.pump(const Duration(milliseconds: 700));

    expect(find.text('Raid'), findsOneWidget);
    expect(find.text('Group Chat'), findsOneWidget);
    expect(find.byType(CupertinoSlidingSegmentedControl), findsNothing);
    expect(find.text('Morning check-in'), findsNothing);

    await tester.tap(find.text('Raid'));
    await tester.pumpAndSettle();

    expect(find.byType(RaidWorkspacesPage), findsOneWidget);
    expect(find.text('Mobile app plan'), findsOneWidget);
    expect(find.text('Morning check-in'), findsNothing);

    await tester.pageBack();
    await tester.pumpAndSettle();
    await tester.tap(find.text('Group Chat'));
    await tester.pumpAndSettle();
    expect(find.text('Home Room'), findsOneWidget);
  });

  testWidgets('keeps each primary tab navigation stack', (tester) async {
    await pumpApp(tester);

    await tester.tap(find.text('Chats'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Raid'));
    await tester.pumpAndSettle();
    expect(find.byType(RaidWorkspacesPage), findsOneWidget);
    await tester.tap(find.text('Mobile app plan'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.byType(WorkspaceChatPage), findsOneWidget);

    await tester.tap(find.text('Browse'));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Play your\nworkflows'), findsOneWidget);

    await tester.tap(find.text('Chats'));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.byType(WorkspaceChatPage), findsOneWidget);
  });

  testWidgets('shows five primary destinations', (tester) async {
    await pumpApp(tester);

    for (final label in ['Browse', 'Chats', 'Friends', 'Pet', 'Me']) {
      expect(find.text(label), findsOneWidget);
    }
  });

  testWidgets('shows friends, pet, and profile surfaces', (tester) async {
    await pumpApp(tester);

    await tester.tap(find.text('Friends'));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('YOUR CIRCLE'), findsOneWidget);
    expect(find.text('Avery'), findsOneWidget);

    await tester.tap(find.text('Pet'));
    await tester.pump(const Duration(milliseconds: 400));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Miso'), findsOneWidget);
    expect(find.text('Level 7  |  620 friendship XP'), findsOneWidget);

    await tester.tap(find.text('Me'));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Local client'), findsOneWidget);
    expect(find.text('Connected over WebRTC'), findsOneWidget);
  });

  testWidgets('fits the compact iPhone viewport', (tester) async {
    tester.view.physicalSize = const Size(375, 667);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await pumpApp(tester);
    expect(find.text('Play your\nworkflows'), findsOneWidget);

    await tester.tap(find.text('Pet'));
    await tester.pump(const Duration(milliseconds: 400));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Miso'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
