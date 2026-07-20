import 'package:drift/native.dart';
import 'package:flutter/cupertino.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/main.dart';
import 'package:gizclaw_app/app/app_locale_controller.dart';
import 'package:gizclaw_app/app/global_conversation_control.dart';
import 'package:gizclaw_app/connection/gizclaw_connection_controller.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/mobile_data_controller.dart';
import 'package:gizclaw_app/data/repositories/workspace_chat_repository.dart';
import 'package:gizclaw_app/data/workspace_chat_controller.dart';
import 'package:gizclaw_app/features/identity/server_pages.dart';
import 'package:gizclaw_app/features/onboarding/server_onboarding_page.dart';
import 'package:gizclaw_app/giz_ui/giz_ui.dart';
import 'package:gizclaw_app/identity/app_identity_store.dart';
import 'package:gizclaw_app/l10n/generated/app_localizations.dart';
import 'package:gizclaw_app/prototype/prototype_data.dart';
import 'package:gizclaw_app/prototype/prototype_models.dart';
import 'package:gizclaw_app/workflows/app_workflow_catalog.dart';

AppDatabase _testDatabase() => AppDatabase.forTesting(NativeDatabase.memory());

const _testServerEndpoint = 'test.gizclaw.local:9820';

void main() {
  final dataControllers = <MobileDataController>[];

  void appTestWidgets(
    String description,
    Future<void> Function(WidgetTester tester) body,
  ) {
    testWidgets(description, (tester) async {
      try {
        await body(tester);
      } finally {
        final controllers = dataControllers.reversed.toList();
        dataControllers.clear();
        await tester.runAsync(() async {
          for (final controller in controllers) {
            await controller.close();
          }
        });
      }
    });
  }

  test('keeps system actions separate from the lime accent', () {
    expect(gizCupertinoTheme.primaryColor, GizColors.primary);
    expect(gizCupertinoTheme.primaryContrastingColor, GizColors.onPrimary);
    expect(GizColors.primary, isNot(GizColors.accent));
    expect(GizColors.primary, const Color(0xFF2F8FFF));
    expect(GizColors.onAccent, GizColors.ink);
    expect(GizColors.messageBlue, const Color(0xFF007AFF));
  });

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
    AppLocaleController? localeController,
  }) async {
    final dataController =
        controller ?? MobileDataController.demo(database: _testDatabase());
    dataControllers.add(dataController);
    await tester.pumpWidget(
      GizClawApp(
        dataController: dataController,
        localeController: localeController,
      ),
    );
    await tester.pump(const Duration(milliseconds: 700));
  }

  appTestWidgets('opens on the active conversation destination', (
    tester,
  ) async {
    await pumpApp(tester);

    expect(find.byType(ActiveWorkspacePage), findsOneWidget);
    expect(find.text('No active conversation'), findsOneWidget);
    expect(primaryNav('Home'), findsOneWidget);
    expect(find.byKey(const ValueKey('voice-mode-thumb')), findsOneWidget);
    expect(find.text('LIVE'), findsNothing);
    expect(find.byType(CupertinoTabBar), findsNothing);
    for (final destination in [
      'Assistants',
      'Translates',
      'Raids',
      'Story Teller',
      'Role Play',
      'Friends',
      'Groups',
      'Pets',
    ]) {
      expect(primaryNav(destination), findsOneWidget);
    }
  });

  appTestWidgets('forwards background and resume lifecycle changes', (
    tester,
  ) async {
    final controller = _LifecycleController();
    await pumpApp(tester, controller: controller);

    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await tester.pump();
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pump();

    expect(controller.pauseCalls, 1);
    expect(controller.resumeCalls, 1);
  });

  appTestWidgets('opens an unconfigured app on server onboarding', (
    tester,
  ) async {
    await pumpApp(tester, controller: _OnboardingServerController());

    expect(find.byType(ServerOnboardingPage), findsOneWidget);
    expect(find.text('Your agents, everywhere.'), findsOneWidget);
    expect(find.text('Agents that feel close'), findsOneWidget);
    expect(find.byKey(const ValueKey('server-onboarding-cta')), findsOneWidget);
    expect(find.byKey(const ValueKey('primary-nav-scroll')), findsNothing);
    expect(find.byKey(const ValueKey('global-audio-field')), findsNothing);
  });

  appTestWidgets('switches language before server setup', (tester) async {
    final localeController = AppLocaleController(
      platformLocales: const [Locale('en')],
    );
    await pumpApp(
      tester,
      controller: _OnboardingServerController(),
      localeController: localeController,
    );

    await tester.tap(find.text('System (Default)'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('简体中文'));
    await tester.pumpAndSettle();

    expect(find.text('你的智能体，随处相伴。'), findsOneWidget);
    expect(
      localeController.preference,
      AppLanguagePreference.simplifiedChinese,
    );
  });

  appTestWidgets('opens server choices from onboarding', (tester) async {
    await pumpApp(tester, controller: _OnboardingServerController());

    await tester.tap(find.byKey(const ValueKey('server-onboarding-cta')));
    await tester.pumpAndSettle();

    expect(find.byType(ServerListPage), findsOneWidget);
    expect(find.text('Development'), findsNothing);
    expect(find.text('Production'), findsNothing);
    expect(
      find.text('No servers added yet. Use Add server to continue.'),
      findsOneWidget,
    );
    expect(find.bySemanticsLabel('Add server'), findsOneWidget);
  });

  appTestWidgets('opens a capability story from onboarding', (tester) async {
    await pumpApp(tester, controller: _OnboardingServerController());

    expect(find.text('READ STORY'), findsWidgets);
    expect(
      find.bySemanticsLabel('Read Agents that feel close'),
      findsOneWidget,
    );

    await tester.tap(
      find.byKey(const ValueKey('onboarding-story-daily-companion')),
    );
    await tester.pumpAndSettle();

    expect(
      find.byKey(const ValueKey('onboarding-article-daily-companion')),
      findsOneWidget,
    );
    await tester.drag(
      find.byKey(const ValueKey('onboarding-article-daily-companion')),
      const Offset(0, -420),
    );
    await tester.pumpAndSettle();

    expect(find.text('Built around your day'), findsOneWidget);
    expect(
      find.text(
        'Your conversations stay connected through the GizClaw server you choose.',
      ),
      findsOneWidget,
    );
    expect(
      find.byWidgetPredicate(
        (widget) =>
            widget is Hero &&
            widget.tag == 'onboarding-feature-daily-companion',
      ),
      findsWidgets,
    );
  });

  appTestWidgets('leaves onboarding after adding a server', (tester) async {
    final controller = _OnboardingServerController();
    await pumpApp(tester, controller: controller);

    await tester.tap(find.byKey(const ValueKey('server-onboarding-cta')));
    await tester.pumpAndSettle();
    await tester.tap(find.bySemanticsLabel('Add server'));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byKey(const ValueKey('server-name-field')),
      'Office',
    );
    await tester.enterText(
      find.byKey(const ValueKey('server-access-point-field')),
      'office.local:9820',
    );
    await tester.tap(find.byKey(const ValueKey('add-server')));
    await tester.pumpAndSettle();

    expect(controller.activeServer?.name, 'Office');
    expect(find.byType(MePage), findsOneWidget);
    expect(find.byType(ServerOnboardingPage), findsNothing);
  });

  appTestWidgets('opens the server scanner from the Identity action', (
    tester,
  ) async {
    await pumpApp(tester);
    await tapPrimaryNav(tester, 'Identity');
    await tester.pumpAndSettle();

    tester
        .widget<GizPageActionButton>(
          find.byKey(const ValueKey('identity-scan-server-qr')),
        )
        .onPressed!();
    await tester.pumpAndSettle();

    expect(find.byType(ScanServerQrPage), findsOneWidget);
  });

  appTestWidgets('shows the current active workspace conversation', (
    tester,
  ) async {
    final controller = MobileDataController.demo(database: _testDatabase())
      ..runWorkspaceState = PeerRunWorkspaceState(
        activeWorkspaceName: 'Parser pass',
      );
    await pumpApp(tester, controller: controller);

    expect(find.byType(ActiveWorkspacePage), findsOneWidget);
    expect(find.byType(WorkspaceChatPage), findsOneWidget);
    expect(find.text('No active conversation'), findsNothing);
    expect(find.text('OFFLINE'), findsOneWidget);
  });

  appTestWidgets('shows the pet scene for an active pet workspace', (
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

  appTestWidgets('shows the chatroom scene for an active group workspace', (
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

  appTestWidgets('opens collection menus and a full workflow picker', (
    tester,
  ) async {
    await pumpApp(tester);

    for (final label in [
      'Assistants',
      'Translates',
      'Raids',
      'Story Teller',
      'Role Play',
    ]) {
      expect(primaryNav(label), findsOneWidget);
    }

    await tapPrimaryNav(tester, 'Raids');
    await tester.pumpAndSettle();

    expect(find.byType(CollectionWorkspacesPage), findsOneWidget);
    expect(find.text('Raids'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('create-workspace-raids')),
      findsOneWidget,
    );
    expect(find.text('Mobile app plan'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('create-workspace-raids')));
    await tester.pumpAndSettle();

    expect(find.byType(WorkflowPickerPage), findsOneWidget);
    expect(find.text('Journey'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('workspace-generate-model')),
      findsNothing,
    );
    expect(find.byKey(const ValueKey('workspace-extract-model')), findsNothing);
    expect(find.byType(CupertinoTextField), findsNothing);
  });

  appTestWidgets('keeps the fixed workspace catalog visible', (tester) async {
    final controller = MobileDataController.demo(database: _testDatabase());
    controller.workspaces = const [];
    await pumpApp(tester, controller: controller);

    expect(primaryNav('Assistants'), findsOneWidget);
    expect(primaryNav('Translates'), findsOneWidget);
    expect(primaryNav('Raids'), findsOneWidget);
    expect(primaryNav('Story Teller'), findsOneWidget);
    expect(primaryNav('Role Play'), findsOneWidget);

    await tapPrimaryNav(tester, 'Raids');
    await tester.pumpAndSettle();
    expect(find.text('Raids'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('create-workspace-raids')),
      findsOneWidget,
    );
    expect(find.text('No workspaces yet.'), findsOneWidget);
  });

  appTestWidgets('marks a legacy workspace with an unknown alias unavailable', (
    tester,
  ) async {
    final controller = MobileDataController.demo(database: _testDatabase());
    controller.workspaces = const [
      WorkspaceCard(
        name: 'legacy-workspace',
        workflowAlias: 'legacy-dynamic-workflow',
        collection: 'raids',
        lastActive: 'Previously created',
      ),
    ];
    await pumpApp(tester, controller: controller);

    await tapPrimaryNav(tester, 'Raids');
    await tester.pumpAndSettle();

    expect(find.text('legacy-workspace'), findsOneWidget);
    expect(find.textContaining('Unavailable'), findsOneWidget);
  });

  appTestWidgets('hides tabs in chat and restores the driver destination', (
    tester,
  ) async {
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Raids');
    await tester.pumpAndSettle();
    expect(find.byType(CollectionWorkspacesPage), findsOneWidget);
    await tester.tap(find.text('Mobile app plan'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.byType(WorkspaceChatPage), findsOneWidget);
    expect(find.byType(CupertinoTabBar).hitTestable(), findsNothing);
    expect(
      find.byType(GlobalConversationControl).hitTestable(),
      findsOneWidget,
    );

    await tester.tap(find.byIcon(GizIcons.chevron_left).hitTestable());
    await tester.pumpAndSettle();
    expect(find.byType(CollectionWorkspacesPage), findsOneWidget);
    expect(find.byType(CupertinoTabBar), findsNothing);
    expect(primaryNav('Raids'), findsOneWidget);
    await tapPrimaryNav(tester, 'Home');
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.byType(ActiveWorkspacePage), findsOneWidget);

    await tapPrimaryNav(tester, 'Raids');
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.byType(CollectionWorkspacesPage), findsOneWidget);
  });

  appTestWidgets('renders the workspace signal room', (tester) async {
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Translates');
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
      find.byKey(const ValueKey('resource-initial-Parser pass')),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });

  appTestWidgets('does not auto-scroll realtime messages during a user drag', (
    tester,
  ) async {
    final controller = _ScrollableMessagesController();
    controller.mode = WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME;
    controller.scrollChat.recording = true;
    dataControllers.add(controller);
    await tester.pumpWidget(
      MobileDataScope(
        controller: controller,
        child: const CupertinoApp(
          theme: gizCupertinoTheme,
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: WorkspaceChatPage(workspaceName: 'Parser pass'),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 300));

    final messageList = find.byKey(const ValueKey('workspace-message-list'));
    expect(messageList, findsOneWidget);
    final scrollable = tester.state<ScrollableState>(
      find.descendant(of: messageList, matching: find.byType(Scrollable)),
    );
    expect(scrollable.position.pixels, 0);

    final gesture = await tester.startGesture(tester.getCenter(messageList));
    await gesture.moveBy(const Offset(0, 260));
    await tester.pump();
    expect(scrollable.position.pixels, greaterThan(48));

    controller.scrollChat.appendMessage('Message received while dragging');
    await tester.pump();
    expect(scrollable.position.pixels, greaterThan(48));
    expect(find.byKey(const ValueKey('new-messages-below')), findsOneWidget);

    await gesture.up();
    await tester.pump(const Duration(milliseconds: 500));
    expect(scrollable.position.pixels, greaterThan(48));
  });

  appTestWidgets(
    'preserves a settled history position until new messages are requested',
    (tester) async {
      final controller = _ScrollableMessagesController();
      dataControllers.add(controller);
      await tester.pumpWidget(
        MobileDataScope(
          controller: controller,
          child: const CupertinoApp(
            theme: gizCupertinoTheme,
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            home: WorkspaceChatPage(workspaceName: 'Parser pass'),
          ),
        ),
      );
      await tester.pump(const Duration(milliseconds: 300));

      final messageList = find.byKey(const ValueKey('workspace-message-list'));
      final scrollable = tester.state<ScrollableState>(
        find.descendant(of: messageList, matching: find.byType(Scrollable)),
      );
      final gesture = await tester.startGesture(tester.getCenter(messageList));
      await gesture.moveBy(const Offset(0, 260));
      await tester.pump();
      await gesture.up();
      await tester.pump(const Duration(seconds: 1));
      final previousPixels = scrollable.position.pixels;
      final previousExtent = scrollable.position.maxScrollExtent;
      expect(previousPixels, greaterThan(48));

      controller.scrollChat.appendMessage('Message received while reading');
      await tester.pump();
      await tester.pump();

      final extentGrowth = scrollable.position.maxScrollExtent - previousExtent;
      expect(extentGrowth, greaterThan(0));
      expect(
        scrollable.position.pixels,
        closeTo(previousPixels + extentGrowth, 3),
      );
      expect(find.byKey(const ValueKey('new-messages-below')), findsOneWidget);

      await tester.tap(find.byKey(const ValueKey('new-messages-below')));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));

      expect(scrollable.position.pixels, closeTo(0, 0.5));
      expect(find.byKey(const ValueKey('new-messages-below')), findsNothing);

      controller.scrollChat.appendMessage('Message followed at the bottom');
      await tester.pump();
      await tester.pump();
      expect(scrollable.position.pixels, closeTo(0, 0.5));
    },
  );

  appTestWidgets('follows system brightness in the workspace signal room', (
    tester,
  ) async {
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.dark;
    addTearDown(tester.platformDispatcher.clearPlatformBrightnessTestValue);
    await pumpApp(tester);

    await tapPrimaryNav(tester, 'Translates');
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

  appTestWidgets('shows expanded primary destinations', (tester) async {
    await pumpApp(tester);

    for (final label in [
      'Home',
      'Assistants',
      'Translates',
      'Raids',
      'Story Teller',
      'Role Play',
      'Friends',
      'Groups',
      'Pets',
      'Identity',
    ]) {
      expect(primaryNav(label), findsOneWidget);
    }
    expect(find.byIcon(GizIcons.house_fill), findsOneWidget);
    expect(find.byIcon(GizIcons.paw), findsOneWidget);
    expect(find.byKey(const ValueKey('primary-nav-scroll')), findsOneWidget);
    expect(find.byKey(const ValueKey('primary-nav-edge-fade')), findsOneWidget);
  });

  appTestWidgets('shows the global voice mode toggle and audio field', (
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
    final audioField = find.byKey(const ValueKey('global-audio-field'));
    expect(audioField, findsOneWidget);
    expect(
      find.descendant(of: audioField, matching: find.byType(CustomPaint)),
      findsNothing,
    );
    expect(
      find.byKey(const ValueKey('global-audio-field-scale')),
      findsOneWidget,
    );
    final radialGlow = find.descendant(
      of: audioField,
      matching: find.byWidgetPredicate(
        (widget) =>
            widget is DecoratedBox &&
            widget.decoration is BoxDecoration &&
            (widget.decoration as BoxDecoration).gradient is RadialGradient,
      ),
    );
    expect(radialGlow, findsOneWidget);
    final glow = tester.widget<DecoratedBox>(radialGlow);
    final decoration = glow.decoration as BoxDecoration;
    expect(decoration.shape, BoxShape.circle);
    expect(decoration.borderRadius, isNull);
    expect(decoration.gradient, isA<RadialGradient>());
    expect(decoration.border, isNull);
    expect(decoration.boxShadow, isNotEmpty);
    expect(find.byKey(const ValueKey('global-audio-field-edge')), findsNothing);
  });

  appTestWidgets('selects voice modes only from the labelled targets', (
    tester,
  ) async {
    final controller = _ModeSwitchController();
    await pumpApp(tester, controller: controller);

    await tester.tap(find.byKey(const ValueKey('voice-mode-realtime')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 320));

    expect(controller.mode, WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME);
    expect(controller.modeSelectionCalls, 1);
    expect(controller.chat.startInputCalls, 1);
    expect(controller.chat.recording, isTrue);

    await tester.tap(find.byKey(const ValueKey('voice-mode-ptt')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 320));
    expect(
      controller.mode,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    );
    expect(controller.modeSelectionCalls, 2);
  });

  appTestWidgets('holds and releases one PTT turn without switching mode', (
    tester,
  ) async {
    final controller = _ModeSwitchController();
    await pumpApp(tester, controller: controller);

    final thumb = find.byKey(const ValueKey('voice-mode-thumb'));
    final gesture = await tester.startGesture(tester.getCenter(thumb));
    await tester.pump(const Duration(milliseconds: 100));

    expect(controller.chat.startInputCalls, 1);
    expect(controller.chat.recording, isTrue);
    expect(controller.modeSelectionCalls, 0);

    await gesture.moveBy(const Offset(-24, 0));
    await tester.pump();

    expect(controller.chat.finishInputCalls, 0);
    expect(controller.chat.recording, isTrue);
    expect(controller.modeSelectionCalls, 0);

    await gesture.up();
    await tester.pump();

    expect(controller.chat.finishInputCalls, 1);
    expect(controller.chat.recording, isFalse);
    expect(controller.modeSelectionCalls, 0);
  });

  appTestWidgets('cancels one active PTT turn without switching mode', (
    tester,
  ) async {
    final controller = _ModeSwitchController();
    await pumpApp(tester, controller: controller);

    final thumb = find.byKey(const ValueKey('voice-mode-thumb'));
    final gesture = await tester.startGesture(tester.getCenter(thumb));
    await tester.pump(const Duration(milliseconds: 100));
    await gesture.cancel();
    await tester.pump();

    expect(controller.chat.startInputCalls, 1);
    expect(controller.chat.finishInputCalls, 1);
    expect(controller.chat.recording, isFalse);
    expect(controller.modeSelectionCalls, 0);
  });

  appTestWidgets('releases a short PTT press without starting a turn', (
    tester,
  ) async {
    final controller = _ModeSwitchController();
    await pumpApp(tester, controller: controller);

    final thumb = find.byKey(const ValueKey('voice-mode-thumb'));
    final gesture = await tester.startGesture(tester.getCenter(thumb));
    await tester.pump(const Duration(milliseconds: 50));
    await gesture.up();
    await tester.pump(const Duration(milliseconds: 100));

    expect(controller.chat.startInputCalls, 0);
    expect(controller.chat.finishInputCalls, 0);
    expect(controller.modeSelectionCalls, 0);
  });

  appTestWidgets('keeps a moved PTT pointer in the same turn and mode', (
    tester,
  ) async {
    final controller = _ModeSwitchController();
    await pumpApp(tester, controller: controller);

    final thumb = find.byKey(const ValueKey('voice-mode-thumb'));
    final gesture = await tester.startGesture(tester.getCenter(thumb));
    await gesture.moveBy(const Offset(64, 0));
    await tester.pump(const Duration(milliseconds: 100));

    expect(controller.chat.startInputCalls, 1);
    expect(controller.chat.recording, isTrue);
    expect(controller.modeSelectionCalls, 0);

    await gesture.up();
    await tester.pump();

    expect(controller.chat.finishInputCalls, 1);
    expect(controller.chat.recording, isFalse);
    expect(controller.modeSelectionCalls, 0);
    expect(find.text('Unable to switch mode'), findsNothing);
  });

  appTestWidgets('ignores thumb movement in realtime mode', (tester) async {
    final controller = _ModeSwitchController();
    await pumpApp(tester, controller: controller);

    await tester.tap(find.byKey(const ValueKey('voice-mode-realtime')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 320));

    final thumb = find.byKey(const ValueKey('voice-mode-thumb'));
    final gesture = await tester.startGesture(tester.getCenter(thumb));
    await gesture.moveBy(const Offset(-64, 0));
    await gesture.up();
    await tester.pump();

    expect(controller.mode, WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME);
    expect(controller.modeSelectionCalls, 1);
    expect(controller.chat.startInputCalls, 1);
    expect(controller.chat.finishInputCalls, 0);
    expect(controller.chat.recording, isTrue);
  });

  appTestWidgets('shows a red unavailable microphone and retries on tap', (
    tester,
  ) async {
    final controller = _UnavailableMicrophoneController();
    await pumpApp(tester, controller: controller);

    final thumb = find.byKey(const ValueKey('voice-mode-thumb'));
    final container = tester.widget<AnimatedContainer>(
      find.descendant(of: thumb, matching: find.byType(AnimatedContainer)),
    );
    final decoration = container.decoration! as BoxDecoration;
    expect((decoration.gradient! as LinearGradient).colors, const [
      CupertinoColors.systemRed,
      CupertinoColors.systemRed,
    ]);
    expect(
      tester
          .widgetList<Semantics>(
            find.descendant(of: thumb, matching: find.byType(Semantics)),
          )
          .any(
            (widget) =>
                widget.properties.label ==
                'Microphone unavailable: permission required. '
                    'Double tap to retry',
          ),
      isTrue,
    );
    await tester.tap(thumb);
    await tester.pump();

    expect(controller.recoveryCalls, 1);
    final recovered = tester.widget<AnimatedContainer>(
      find.descendant(of: thumb, matching: find.byType(AnimatedContainer)),
    );
    expect(
      (recovered.decoration! as BoxDecoration).gradient,
      isNot(
        isA<LinearGradient>().having(
          (gradient) => gradient.colors,
          'colors',
          const [CupertinoColors.systemRed, CupertinoColors.systemRed],
        ),
      ),
    );
  });

  appTestWidgets('releases active PTT after microphone becomes unavailable', (
    tester,
  ) async {
    final controller = _InterruptedMicrophoneController();
    await pumpApp(tester, controller: controller);

    final thumb = find.byKey(const ValueKey('voice-mode-thumb'));
    final gesture = await tester.startGesture(tester.getCenter(thumb));
    await tester.pump(const Duration(milliseconds: 100));
    expect(controller.chat.recording, isTrue);

    controller.interruptMicrophone();
    await tester.pump();
    await gesture.up();
    await tester.pump();

    expect(controller.chat.finishInputCalls, 1);
    expect(controller.chat.recording, isFalse);
  });

  appTestWidgets('opens group creation controls', (tester) async {
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

  appTestWidgets('shows friends, pet, and profile surfaces', (tester) async {
    final controller = _ServerListTestController();
    await pumpApp(tester, controller: controller);

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
    expect(find.text('This device'), findsOneWidget);
    expect(find.text('Device identity ready'), findsOneWidget);
    expect(find.text('Public identity'), findsOneWidget);
    expect(find.text('Private key'), findsOneWidget);
    expect(find.text('Server'), findsOneWidget);
    expect(find.text('Office · $_testServerEndpoint'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('server-settings-row')));
    await tester.pumpAndSettle();
    expect(find.text('Servers'), findsOneWidget);
    expect(find.text('Office'), findsOneWidget);
    expect(find.text(_testServerEndpoint), findsOneWidget);
    expect(find.byKey(const ValueKey('selected-server')), findsOneWidget);

    await tester.tap(find.bySemanticsLabel('Add server'));
    await tester.pumpAndSettle();
    expect(find.text('Add Server'), findsNWidgets(2));
    expect(find.byKey(const ValueKey('scan-server-qr')), findsOneWidget);
    expect(find.byKey(const ValueKey('server-name-field')), findsOneWidget);
    await tester.enterText(
      find.byKey(const ValueKey('server-name-field')),
      'Office',
    );
    await tester.enterText(
      find.byKey(const ValueKey('server-access-point-field')),
      'gizclaw.local',
    );
    await tester.tap(find.byKey(const ValueKey('add-server')));
    await tester.pump();
    expect(find.byKey(const ValueKey('add-server-error')), findsOneWidget);
    expect(
      find.text(
        'Use a domain or IP address with a port, for example gizclaw.local:9820.',
      ),
      findsOneWidget,
    );

    await tester.enterText(
      find.byKey(const ValueKey('server-access-point-field')),
      _testServerEndpoint,
    );
    await tester.tap(find.byKey(const ValueKey('add-server')));
    await tester.pump();
    expect(
      find.text('This access point is already in the list.'),
      findsOneWidget,
    );

    Navigator.of(
      tester.element(find.byKey(const ValueKey('server-name-field'))),
    ).pop();
    await tester.pumpAndSettle();
    await tester.runAsync(
      () => controller.addServer(
        name: 'Branch',
        accessPoint: 'office.local:9820',
      ),
    );
    await tester.pumpAndSettle();
    expect(controller.servers.last.name, 'Branch');
    expect(controller.serverEndpoint, 'office.local:9820');
    expect(find.byType(ServerListPage), findsOneWidget);
    expect(find.text('Branch'), findsOneWidget);
    expect(find.text('office.local:9820'), findsOneWidget);
    expect(find.byKey(const ValueKey('selected-server')), findsOneWidget);
  });

  appTestWidgets('shows actionable iOS local-network recovery guidance', (
    tester,
  ) async {
    debugDefaultTargetPlatformOverride = TargetPlatform.iOS;
    try {
      final controller = _LocalNetworkFailureController();
      await pumpApp(tester, controller: controller);

      await tapPrimaryNav(tester, 'Identity');
      await tester.pumpAndSettle();
      expect(find.text('Local network connection unavailable'), findsOneWidget);
      expect(find.textContaining('same reachable Wi-Fi'), findsOneWidget);
      final retry = find.byKey(const ValueKey('local-network-retry'));
      await tester.ensureVisible(retry);
      await tester.pumpAndSettle();
      await tester.tap(retry);
      await tester.pump();
      expect(controller.retryCalls, 1);
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
  });

  appTestWidgets('adds a server from the pushed page', (tester) async {
    final controller = _ImmediateAddServerController();
    await pumpApp(tester, controller: controller);
    await tapPrimaryNav(tester, 'Identity');
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('server-settings-row')));
    await tester.pumpAndSettle();
    await tester.tap(find.bySemanticsLabel('Add server'));
    await tester.pumpAndSettle();

    expect(
      find.byKey(const ValueKey('server-registration-token-field')),
      findsNothing,
    );
    await tester.enterText(
      find.byKey(const ValueKey('server-name-field')),
      'Office',
    );
    await tester.enterText(
      find.byKey(const ValueKey('server-access-point-field')),
      'office.local:9820',
    );
    tester
        .widget<CupertinoButton>(find.byKey(const ValueKey('add-server')))
        .onPressed!();
    await tester.pumpAndSettle();

    expect(find.byType(ServerListPage), findsOneWidget);
    expect(controller.addedName, 'Office');
    expect(controller.addedAccessPoint, 'office.local:9820');
  });

  appTestWidgets('opens real friend connection controls', (tester) async {
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

  appTestWidgets('opens a friend chatroom workspace', (tester) async {
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

  appTestWidgets('shows the specific workspace activation failure', (
    tester,
  ) async {
    final controller = _ActivationFailureController();
    await pumpApp(tester, controller: controller);

    await tapPrimaryNav(tester, 'Friends');
    await tester.pumpAndSettle();
    await tester.tap(find.text('Avery'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    await tester.tap(find.byKey(const ValueKey('workspace-activation-button')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));

    expect(find.text('Unable to activate'), findsOneWidget);
    expect(
      find.textContaining('flowcraft parameter "generate_model" requires'),
      findsOneWidget,
    );
  });

  appTestWidgets('fits the compact iPhone viewport', (tester) async {
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

  appTestWidgets('fits workspace controls in the compact iPhone viewport', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(375, 667);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await pumpApp(tester);
    await tapPrimaryNav(tester, 'Translates');
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
  _ModeSwitchController()
    : super(
        database: _testDatabase(),
        profile: const GizClawConnectionProfile(
          endpoint: _testServerEndpoint,
          clientPrivateKey: 'test-key',
        ),
      ) {
    chat = _ModeSwitchChatController(workspaceChatRepository);
  }

  late final _ModeSwitchChatController chat;

  WorkspaceInputMode mode =
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK;
  int modeSelectionCalls = 0;

  @override
  String? get activeWorkspaceName => 'Parser pass';

  @override
  WorkspaceInputMode get activeInputMode => mode;

  @override
  WorkspaceChatController? get activeWorkspaceChat => chat;

  @override
  Future<void> start() async {}

  @override
  Future<void> setActiveInputMode(WorkspaceInputMode mode) async {
    modeSelectionCalls += 1;
    this.mode = mode;
    notifyListeners();
  }

  @override
  Future<void> close() async {
    await chat.close();
    await super.close();
  }
}

class _ScrollableMessagesController extends _ModeSwitchController {
  _ScrollableMessagesController() {
    scrollChat = _ScrollableMessagesChatController(workspaceChatRepository);
  }

  late final _ScrollableMessagesChatController scrollChat;

  @override
  WorkspaceChatController? get activeWorkspaceChat => scrollChat;

  @override
  Future<void> close() async {
    await scrollChat.close();
    await super.close();
  }
}

class _ScrollableMessagesChatController extends _ModeSwitchChatController {
  _ScrollableMessagesChatController(super.repository);

  final List<WorkspaceChatMessage> _visibleMessages = List.generate(
    30,
    (index) => WorkspaceChatMessage(
      id: 'message-$index',
      incoming: index.isEven,
      text: 'Conversation message $index with enough text to fill the list.',
      state: WorkspaceMessageState.complete,
    ),
  );

  @override
  List<WorkspaceChatMessage> get messages =>
      List.unmodifiable(_visibleMessages);

  void appendMessage(String text) {
    _visibleMessages.add(
      WorkspaceChatMessage(
        id: 'message-${_visibleMessages.length}',
        incoming: true,
        text: text,
        state: WorkspaceMessageState.complete,
      ),
    );
    notifyListeners();
  }
}

class _UnavailableMicrophoneController extends _ModeSwitchController {
  _UnavailableMicrophoneController() {
    connectionState = MobileConnectionState.connected;
  }

  MicrophoneStatus status = const MicrophoneStatus.unavailable(
    failureKind: MicrophoneFailureKind.permissionDenied,
  );
  int recoveryCalls = 0;

  @override
  MicrophoneStatus get microphoneStatus => status;

  @override
  Future<MicrophoneStatus> recoverMicrophone() async {
    recoveryCalls += 1;
    status = const MicrophoneStatus.ready();
    notifyListeners();
    return status;
  }
}

class _InterruptedMicrophoneController extends _ModeSwitchController {
  _InterruptedMicrophoneController() {
    connectionState = MobileConnectionState.connected;
  }

  MicrophoneStatus status = const MicrophoneStatus.ready();

  @override
  MicrophoneStatus get microphoneStatus => status;

  void interruptMicrophone() {
    status = const MicrophoneStatus.unavailable(
      failureKind: MicrophoneFailureKind.captureUnavailable,
    );
    notifyListeners();
  }
}

class _LifecycleController extends _ModeSwitchController {
  int pauseCalls = 0;
  int resumeCalls = 0;

  @override
  void handleAppPaused() {
    pauseCalls += 1;
  }

  @override
  void handleAppResumed() {
    resumeCalls += 1;
  }
}

class _ServerListTestController extends MobileDataController {
  _ServerListTestController()
    : super(
        database: AppDatabase.forTesting(NativeDatabase.memory()),
        profile: const GizClawConnectionProfile(
          endpoint: _testServerEndpoint,
          clientPrivateKey: 'test-key',
        ),
        servers: const [
          GizClawServer(name: 'Office', accessPoint: _testServerEndpoint),
        ],
      ) {
    workflows = demoWorkflows
        .map((workflow) => appWorkflowCard(workflow, const Locale('en')))
        .toList();
    workspaces = workflowWorkspaces;
    chatroomWorkspaces = chatroomWorkspaceMetadata;
  }

  @override
  Future<void> start() async {
    connectionState = MobileConnectionState.offline;
    notifyListeners();
  }
}

class _LocalNetworkFailureController extends _ServerListTestController {
  _LocalNetworkFailureController() {
    lastError = StateError('No route to host');
  }

  int retryCalls = 0;

  @override
  Future<void> recoverTransport() async {
    retryCalls += 1;
  }
}

class _ActivationFailureController extends _ServerListTestController {
  @override
  Future<WorkspaceChatController> activateWorkspaceChat(String workspaceName) {
    return Future.error(
      StateError(
        'flowcraft parameter "generate_model" requires a concrete Model',
      ),
    );
  }
}

class _OnboardingServerController extends MobileDataController {
  _OnboardingServerController()
    : super(
        database: _testDatabase(),
        profile: const GizClawConnectionProfile(
          endpoint: '',
          clientPrivateKey: 'test-key',
        ),
      );

  GizClawServer? _selectedServer;

  @override
  GizClawServer? get activeServer => _selectedServer;

  @override
  Future<void> start() async {
    connectionState = MobileConnectionState.unconfigured;
    notifyListeners();
  }

  @override
  Future<void> addServer({
    required String name,
    required String accessPoint,
    String registrationToken = '',
  }) async {
    _selectedServer = GizClawServer(
      name: name,
      accessPoint: accessPoint,
      registrationToken: registrationToken,
    );
    notifyListeners();
  }
}

class _ImmediateAddServerController extends _ServerListTestController {
  String? addedName;
  String? addedAccessPoint;

  @override
  Future<void> addServer({
    required String name,
    required String accessPoint,
    String registrationToken = '',
  }) async {
    addedName = name;
    addedAccessPoint = accessPoint;
  }
}

class _ModeSwitchChatController extends WorkspaceChatController {
  _ModeSwitchChatController(WorkspaceChatRepository repository)
    : super(
        workspaceName: 'Parser pass',
        repository: repository,
        serverId: null,
      );

  int startInputCalls = 0;
  int finishInputCalls = 0;

  @override
  bool get canRecord => true;

  @override
  Future<void> startInput() async {
    startInputCalls += 1;
    recording = true;
    notifyListeners();
  }

  @override
  Future<void> finishInput({String? error}) async {
    finishInputCalls += 1;
    recording = false;
    notifyListeners();
  }
}

class _ActiveDestinationController extends MobileDataController {
  _ActiveDestinationController(this.destination)
    : super(
        database: _testDatabase(),
        profile: const GizClawConnectionProfile(
          endpoint: _testServerEndpoint,
          clientPrivateKey: 'test-key',
        ),
      );

  final MobileWorkspaceDestination destination;

  @override
  String? get activeWorkspaceName => destination.workspaceName;

  @override
  Future<void> start() async {}

  @override
  Future<MobileWorkspaceDestination> destinationForWorkspace(
    String workspaceName,
  ) async => destination;
}
