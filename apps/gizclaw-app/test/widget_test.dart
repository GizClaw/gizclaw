import 'package:flutter/cupertino.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/main.dart';
import 'package:gizclaw_app/app/global_conversation_control.dart';
import 'package:gizclaw_app/data/mobile_data_controller.dart';
import 'package:gizclaw_app/giz_ui/giz_ui.dart';

void main() {
  Finder primaryNav(String label) =>
      find.byKey(ValueKey('primary-nav-${label.toLowerCase()}'));

  Future<void> tapPrimaryNav(WidgetTester tester, String label) async {
    final destination = primaryNav(label);
    final dock = find.byKey(const ValueKey('primary-nav-scroll'));
    await tester.drag(dock, const Offset(1000, 0));
    await tester.pumpAndSettle();
    for (
      var attempt = 0;
      attempt < 6 && destination.evaluate().isEmpty;
      attempt++
    ) {
      await tester.drag(dock, const Offset(-120, 0));
      await tester.pumpAndSettle();
    }
    await tester.ensureVisible(destination);
    await tester.pumpAndSettle();
    await tester.tap(destination);
  }

  Future<void> pumpApp(
    WidgetTester tester, {
    MobileDataController? controller,
  }) async {
    await tester.pumpWidget(
      GizClawApp(dataController: controller ?? MobileDataController.demo()),
    );
    await tester.pump(const Duration(milliseconds: 700));
  }

  testWidgets('opens on the active conversation destination', (tester) async {
    await pumpApp(tester);

    expect(find.byType(ActiveWorkspacePage), findsOneWidget);
    expect(find.text('No active conversation'), findsOneWidget);
    expect(primaryNav('Active'), findsOneWidget);
    expect(find.byKey(const ValueKey('voice-mode-thumb')), findsOneWidget);
    expect(find.text('LIVE'), findsNothing);
    expect(find.byType(CupertinoTabBar), findsNothing);
    for (final destination in [
      'Flowcraft',
      'Doubao',
      'Translate',
      'Friends',
      'Groups',
      'Pets',
    ]) {
      expect(primaryNav(destination), findsOneWidget);
    }
    expect(primaryNav('Raids'), findsNothing);
  });

  testWidgets('shows the current active workspace conversation', (
    tester,
  ) async {
    final controller = MobileDataController.demo()
      ..runWorkspaceState = PeerRunWorkspaceState(
        activeWorkspaceName: 'Parser pass',
      );
    await pumpApp(tester, controller: controller);

    expect(find.byType(ActiveWorkspacePage), findsOneWidget);
    expect(find.byType(WorkspaceChatPage), findsOneWidget);
    expect(find.text('No active conversation'), findsNothing);
    expect(find.text('OFFLINE'), findsOneWidget);
  });

  testWidgets('shows the pet scene for an active pet workspace', (
    tester,
  ) async {
    final controller = _ActiveDestinationController(
      const MobileWorkspaceDestination(
        surface: MobileWorkspaceSurface.pet,
        workspaceName: 'pet-workspace',
        resourceId: 'pet-1',
      ),
    );
    await pumpApp(tester, controller: controller);

    expect(find.byType(ActiveWorkspacePage), findsOneWidget);
    expect(find.byKey(const ValueKey('active-pet-pet-1')), findsOneWidget);
    expect(find.byType(WorkspaceChatPage), findsNothing);
  });

  testWidgets('shows the chatroom scene for an active group workspace', (
    tester,
  ) async {
    final controller = _ActiveDestinationController(
      const MobileWorkspaceDestination(
        surface: MobileWorkspaceSurface.group,
        workspaceName: 'group-workspace',
      ),
    );
    await pumpApp(tester, controller: controller);

    expect(find.byType(ActiveWorkspacePage), findsOneWidget);
    expect(
      find.byKey(const ValueKey('active-chatroom-group-workspace')),
      findsOneWidget,
    );
    expect(find.byType(ChatroomWorkspacePage), findsOneWidget);
  });

  testWidgets('opens workflow drivers directly from the dock', (tester) async {
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Flowcraft');
    await tester.pumpAndSettle();

    expect(find.byType(DriverWorkspacesPage), findsOneWidget);
    expect(find.text('Mobile app plan'), findsOneWidget);
    expect(find.text('Morning check-in'), findsNothing);

    await tapPrimaryNav(tester, 'Groups');
    await tester.pumpAndSettle();
    expect(find.text('Builder Crew'), findsOneWidget);
    expect(find.text('Avery'), findsNothing);
    expect(find.text('Mobile app plan'), findsNothing);

    await tester.tap(find.text('Builder Crew'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    expect(find.byType(ChatroomWorkspacePage), findsOneWidget);
    expect(find.byType(WorkspaceChatPage), findsOneWidget);
    expect(find.text('Builder Crew'), findsOneWidget);
    expect(find.textContaining('Group chat'), findsOneWidget);
    expect(find.byType(CupertinoTextField), findsNothing);
  });

  testWidgets('keeps driver destinations visible without workspaces', (
    tester,
  ) async {
    final controller = MobileDataController.demo();
    controller.workspaces = controller.workspaces
        .where(
          (workspace) =>
              workspace.workflowName != 'realtime-lab' &&
              workspace.workflowName != 'ast-translate',
        )
        .toList(growable: false);
    await pumpApp(tester, controller: controller);

    expect(primaryNav('Doubao'), findsOneWidget);
    expect(primaryNav('Translate'), findsOneWidget);

    await tapPrimaryNav(tester, 'Doubao');
    await tester.pumpAndSettle();
    expect(find.text('No Doubao Realtime workspaces yet.'), findsOneWidget);

    await tapPrimaryNav(tester, 'Translate');
    await tester.pumpAndSettle();
    expect(find.text('No AST Translate workspaces yet.'), findsOneWidget);
  });

  testWidgets('hides tabs in chat and restores the driver destination', (
    tester,
  ) async {
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Flowcraft');
    await tester.pumpAndSettle();
    expect(find.byType(DriverWorkspacesPage), findsOneWidget);
    await tester.tap(find.text('Mobile app plan'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.byType(WorkspaceChatPage), findsOneWidget);
    expect(find.byType(CupertinoTabBar).hitTestable(), findsNothing);
    expect(
      find.byType(GlobalConversationControl).hitTestable(),
      findsOneWidget,
    );

    await tester.tap(find.byIcon(CupertinoIcons.chevron_left).hitTestable());
    await tester.pumpAndSettle();
    expect(find.byType(DriverWorkspacesPage), findsOneWidget);
    expect(find.byType(CupertinoTabBar), findsNothing);
    expect(primaryNav('Flowcraft'), findsOneWidget);
    await tapPrimaryNav(tester, 'Active');
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.byType(ActiveWorkspacePage), findsOneWidget);

    await tapPrimaryNav(tester, 'Flowcraft');
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.byType(DriverWorkspacesPage), findsOneWidget);
  });

  testWidgets('renders the workspace signal room', (tester) async {
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Translate');
    await tester.pumpAndSettle();
    await tester.tap(find.text('Parser pass'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));

    expect(find.byType(WorkspaceChatPage), findsOneWidget);
    expect(find.text('AGENT SIGNAL ONLINE'), findsNothing);
    expect(find.text('OFFLINE'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('workspace-activation-button')),
      findsOneWidget,
    );
    expect(
      tester.getSize(find.byKey(const ValueKey('workspace-activation-button'))),
      const Size.square(58),
    );
    expect(find.text('ACTIVATE'), findsNothing);
    expect(
      find.image(const AssetImage('assets/drivers/ast-translate.png')),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets('follows system brightness in the workspace signal room', (
    tester,
  ) async {
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.dark;
    addTearDown(tester.platformDispatcher.clearPlatformBrightnessTestValue);
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Translate');
    await tester.pumpAndSettle();
    await tester.tap(find.text('Parser pass'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));

    expect(
      find.byWidgetPredicate(
        (widget) =>
            widget is CupertinoPageScaffold &&
            widget.backgroundColor == const Color(0xFF0A100D),
      ),
      findsOneWidget,
    );
    expect(find.byType(CupertinoTabBar).hitTestable(), findsNothing);

    tester.platformDispatcher.platformBrightnessTestValue = Brightness.light;
    await tester.pump();

    expect(
      find.byWidgetPredicate(
        (widget) =>
            widget is CupertinoPageScaffold &&
            widget.backgroundColor == GizColors.canvas,
      ),
      findsOneWidget,
    );
    expect(find.byType(CupertinoTabBar).hitTestable(), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('shows expanded primary destinations', (tester) async {
    await pumpApp(tester);

    for (final label in [
      'Active',
      'Flowcraft',
      'Doubao',
      'Translate',
      'Friends',
      'Groups',
      'Pets',
      'Identity',
    ]) {
      expect(primaryNav(label), findsOneWidget);
    }
    expect(find.byIcon(CupertinoIcons.game_controller), findsOneWidget);
    expect(find.byIcon(CupertinoIcons.wand_stars), findsOneWidget);
    expect(find.byIcon(CupertinoIcons.paw), findsOneWidget);
    expect(
      find.byKey(const ValueKey('primary-nav-translate-glyph')),
      findsOneWidget,
    );
    expect(find.byKey(const ValueKey('primary-nav-scroll')), findsOneWidget);
    expect(find.byKey(const ValueKey('primary-nav-edge-fade')), findsOneWidget);
  });

  testWidgets('shows the global voice mode toggle and audio field', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await pumpApp(tester);

    expect(find.byKey(const ValueKey('voice-mode-toggle')), findsOneWidget);
    expect(find.byKey(const ValueKey('voice-mode-ptt')), findsOneWidget);
    expect(find.byKey(const ValueKey('voice-mode-realtime')), findsOneWidget);
    expect(find.byKey(const ValueKey('voice-mode-thumb')), findsOneWidget);
    expect(find.byKey(const ValueKey('global-audio-field')), findsOneWidget);
  });

  testWidgets('slides the voice thumb between PTT and realtime', (
    tester,
  ) async {
    final controller = _ModeSwitchController();
    await pumpApp(tester, controller: controller);

    final thumb = find.byKey(const ValueKey('voice-mode-thumb'));
    final pttPosition = tester.getTopLeft(thumb);
    await tester.drag(thumb, const Offset(64, 0));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 320));

    expect(controller.mode, WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME);
    expect(tester.getTopLeft(thumb).dx, greaterThan(pttPosition.dx + 50));

    await tester.drag(thumb, const Offset(-64, 0));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 320));
    expect(
      controller.mode,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    );
  });

  testWidgets('opens group creation controls', (tester) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Groups');
    await tester.pumpAndSettle();
    expect(find.text('Builder Crew'), findsOneWidget);
    expect(find.text('Avery'), findsNothing);

    await tester.tap(find.bySemanticsLabel('Create group'));
    await tester.pumpAndSettle();
    expect(find.text('Create Group'), findsNWidgets(2));
    expect(find.byType(CupertinoTextField), findsNWidgets(2));
    expect(
      tester
          .getBottomRight(find.byKey(const ValueKey('create-group-sheet')))
          .dy,
      844,
    );
  });

  testWidgets('shows friends, pet, and profile surfaces', (tester) async {
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Friends');
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('YOUR CIRCLE'), findsOneWidget);
    expect(find.text('Avery'), findsOneWidget);

    await tapPrimaryNav(tester, 'Pets');
    await tester.pump(const Duration(milliseconds: 400));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Connect to GizClaw to meet your pets.'), findsOneWidget);

    await tapPrimaryNav(tester, 'Identity');
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Local client'), findsOneWidget);
    expect(find.text('Connected over WebRTC'), findsOneWidget);
  });

  testWidgets('opens real friend connection controls', (tester) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Friends');
    await tester.pumpAndSettle();
    await tester.tap(find.bySemanticsLabel('Add friend'));
    await tester.pumpAndSettle();

    expect(find.text('Connect'), findsOneWidget);
    expect(find.text('My Invite'), findsOneWidget);
    expect(find.byType(CupertinoTextField), findsOneWidget);
    expect(
      tester
          .getBottomRight(find.byKey(const ValueKey('friend-connect-sheet')))
          .dy,
      844,
    );

    await tester.ensureVisible(find.text('My Invite'));
    await tester.tap(find.text('My Invite'));
    await tester.pumpAndSettle();
    expect(find.text('Connect to GizClaw to manage friends'), findsOneWidget);
  });

  testWidgets('opens a friend chatroom workspace', (tester) async {
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Friends');
    await tester.pumpAndSettle();
    await tester.tap(find.text('Avery'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));

    expect(find.byType(ChatroomWorkspacePage), findsOneWidget);
    expect(find.byType(WorkspaceChatPage), findsOneWidget);
    expect(find.text('Avery'), findsOneWidget);
    expect(find.textContaining('Direct chat'), findsOneWidget);
    expect(find.textContaining('Unavailable'), findsNothing);
    expect(
      find.byKey(const ValueKey('workspace-activation-button')),
      findsOneWidget,
    );
    expect(find.byType(CupertinoTextField), findsNothing);
    expect(find.byType(CupertinoTabBar).hitTestable(), findsNothing);
  });

  testWidgets('fits the compact iPhone viewport', (tester) async {
    tester.view.physicalSize = const Size(375, 667);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await pumpApp(tester);
    expect(find.byType(ActiveWorkspacePage), findsOneWidget);

    await tapPrimaryNav(tester, 'Pets');
    await tester.pump(const Duration(milliseconds: 400));
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Connect to GizClaw to meet your pets.'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('fits workspace controls in the compact iPhone viewport', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(375, 667);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await pumpApp(tester);
    await tapPrimaryNav(tester, 'Translate');
    await tester.pumpAndSettle();
    await tester.tap(find.text('Parser pass'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));

    expect(
      find.byKey(const ValueKey('workspace-activation-button')),
      findsOneWidget,
    );
    expect(find.text('Parser pass'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}

class _ModeSwitchController extends MobileDataController {
  _ModeSwitchController();

  WorkspaceInputMode mode =
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK;

  @override
  String? get activeWorkspaceName => 'Parser pass';

  @override
  WorkspaceInputMode get activeInputMode => mode;

  @override
  Future<void> setActiveInputMode(WorkspaceInputMode mode) async {
    this.mode = mode;
    notifyListeners();
  }
}

class _ActiveDestinationController extends MobileDataController {
  _ActiveDestinationController(this.destination);

  final MobileWorkspaceDestination destination;

  @override
  String? get activeWorkspaceName => destination.workspaceName;

  @override
  Future<MobileWorkspaceDestination> destinationForWorkspace(
    String workspaceName,
  ) async => destination;
}
