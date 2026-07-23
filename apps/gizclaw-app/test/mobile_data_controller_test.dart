import 'dart:async';

import 'package:drift/native.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/connection/gizclaw_connection_controller.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/mobile_data_controller.dart';
import 'package:gizclaw_app/data/repositories/mobile_data_repository.dart';
import 'package:gizclaw_app/data/repositories/workspace_chat_repository.dart';
import 'package:gizclaw_app/data/workspace_chat_controller.dart';
import 'package:gizclaw_app/identity/app_identity_store.dart';
import 'package:gizclaw_app/l10n/locale_resolution.dart';
import 'package:gizclaw_app/prototype/prototype_models.dart';

void main() {
  test('classifies only iOS LAN failures as local-network recovery', () {
    for (final endpoint in [
      'gizclaw.local:9820',
      '192.168.1.8:9820',
      '10.0.0.8:9820',
      '[fe80::1]:9820',
    ]) {
      expect(
        classifyMobileConnectionFailure(
          endpoint: endpoint,
          error: StateError('No route to host'),
          platform: TargetPlatform.iOS,
          state: MobileConnectionState.offline,
        ),
        MobileConnectionFailureKind.localNetwork,
      );
    }
    expect(
      classifyMobileConnectionFailure(
        endpoint: 'api.example.com:9820',
        error: StateError('No route to host'),
        platform: TargetPlatform.iOS,
        state: MobileConnectionState.offline,
      ),
      MobileConnectionFailureKind.generic,
    );
    expect(
      classifyMobileConnectionFailure(
        endpoint: '192.168.1.8:9820',
        error: StateError('No route to host'),
        platform: TargetPlatform.android,
        state: MobileConnectionState.offline,
      ),
      MobileConnectionFailureKind.generic,
    );
  });

  test('closes demo controller resources idempotently', () async {
    final controller = MobileDataController.demo(
      database: AppDatabase.forTesting(NativeDatabase.memory()),
    );

    await controller.start();
    final firstClose = controller.close();
    expect(controller.close(), same(firstClose));
    await firstClose;
  });

  test('marks an alias absent from the runtime catalog unavailable', () {
    final controller = MobileDataController.demo(
      database: AppDatabase.forTesting(NativeDatabase.memory()),
    );
    addTearDown(controller.close);

    final workflow = controller.workflow('ast-translate-zh-ja');

    expect(workflow.name, 'ast-translate-zh-ja');
    expect(workflow.driver, WorkflowDriverKind.unsupported);
    expect(workflow.title, isNotEmpty);
  });

  test('rejects blank server endpoints before selecting or saving', () async {
    final controller = MobileDataController(
      database: AppDatabase.forTesting(NativeDatabase.memory()),
      profile: _profile(''),
    );
    addTearDown(controller.close);

    await expectLater(
      controller.addServer(name: 'Office', accessPoint: '   '),
      throwsA(
        isA<FormatException>().having(
          (error) => error.message,
          'message',
          'Enter a server access point',
        ),
      ),
    );
    await expectLater(
      controller.updateServerEndpoint(''),
      throwsA(isA<FormatException>()),
    );

    expect(controller.serverEndpoint, isEmpty);
    expect(controller.hasActiveServer, isFalse);
    expect(controller.servers, isEmpty);
  });

  test(
    'rescanning the same server rotates its raw registration token',
    () async {
      final connection = _RefreshTestConnection(
        profile: const GizClawConnectionProfile(
          endpoint: 'office.local:9820',
          clientPrivateKey: 'test-key',
          registrationToken: 'expired-secret',
        ),
        client: _RunWorkspaceClient(),
        serverId: 'server-a',
      );
      final controller = MobileDataController(
        database: AppDatabase.forTesting(NativeDatabase.memory()),
        connectionController: connection,
        servers: const [
          GizClawServer(
            name: 'Office',
            accessPoint: 'office.local:9820',
            registrationToken: 'expired-secret',
          ),
        ],
      );
      addTearDown(controller.close);

      await controller.addOrSelectServer(
        name: 'Ignored duplicate name',
        accessPoint: 'office.local:9820',
        registrationToken: 'replacement-secret',
      );

      expect(controller.servers, hasLength(1));
      expect(controller.servers.single.registrationToken, 'replacement-secret');
      expect(connection.profile.registrationToken, 'replacement-secret');
    },
  );

  test('creates a workspace from a dynamic collection alias', () async {
    final client = _WorkspaceCreateClient();
    final controller = _WorkspaceCreateTestController(client)
      ..connectionState = MobileConnectionState.connected
      ..workflows = const [
        WorkflowCard(
          name: 'journey',
          title: 'Journey',
          subtitle: '',
          driverLabel: 'Flowcraft',
          collection: 'raids',
          bannerColor: Color(0xff000000),
          icon: IconData(0),
          driver: WorkflowDriverKind.flowcraft,
        ),
      ];
    addTearDown(controller.close);

    await controller.createWorkspace(
      collection: 'raids',
      workflowAlias: 'journey',
    );

    expect(client.requests.single.workflowAlias, 'journey');
    expect(client.requests.single.collection, 'raids');
    expect(client.requests.single.hasParameters(), isTrue);
    expect(
      client.requests.single.parameters.flowcraftWorkspaceParameters.input,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    );
  });

  test(
    'creates translation workspaces with the selected language pair',
    () async {
      final client = _WorkspaceCreateClient();
      final aliases = <String, String>{
        'translate-zh-en-auto': 'auto',
        'japanese': 'zh/ja',
        'translate-zh-ko': 'zh/ko',
        'translate-zh-es': 'zh/es',
      };
      final controller = _WorkspaceCreateTestController(client)
        ..connectionState = MobileConnectionState.connected
        ..workflows = aliases.keys
            .map(
              (alias) => WorkflowCard(
                name: alias,
                title: alias,
                subtitle: '',
                driverLabel: 'AST Translate',
                collection: 'translates',
                bannerColor: const Color(0xff000000),
                icon: const IconData(0),
                driver: WorkflowDriverKind.astTranslate,
                workspaceLangPair: aliases[alias],
              ),
            )
            .toList(growable: false);
      addTearDown(controller.close);

      for (final entry in aliases.entries) {
        await controller.createWorkspace(
          collection: 'translates',
          workflowAlias: entry.key,
        );
        final request = client.requests.last;
        expect(request.workflowAlias, entry.key);
        expect(
          request.parameters.asttranslateWorkspaceParameters.langPair,
          entry.value,
        );
        expect(
          request
              .parameters
              .asttranslateWorkspaceParameters
              .enableSourceLanguageDetect,
          entry.value == 'auto',
        );
      }
    },
  );

  test('treats a missing workflow collection as empty', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final client = _PartialWorkflowCatalogClient();
    final connection = _RefreshTestConnection(
      profile: _profile('gizclaw.local:9820'),
      client: client,
      serverId: 'server-a',
    );
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
      dataRepository: _ReconnectRepository(database),
    )..connectionState = MobileConnectionState.connected;
    addTearDown(controller.close);

    await controller.refresh(client: client, serverId: 'server-a');

    expect(client.collections, [
      'assistants',
      'translates',
      'raids',
      'story-teller',
      'role-play',
    ]);
    expect(controller.workflows.map((workflow) => workflow.name), ['chat']);
    expect(controller.lastError, isNull);
  });

  test('waits for an in-flight refresh before closing resources', () async {
    final database = _TrackingDatabase();
    final client = _RunWorkspaceClient();
    final connection = _CloseTrackingConnection(
      profile: _profile('gizclaw.local:9820'),
      client: client,
      serverId: 'server-a',
    );
    final repository = _QueuedRefreshRepository(database);
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
      dataRepository: repository,
    );

    final refresh = controller.refresh(client: client, serverId: 'server-a');
    final close = controller.close();
    await Future<void>.delayed(Duration.zero);

    expect(connection.closeCalled, isFalse);
    expect(database.closeCalled, isFalse);

    repository.firstRefresh.complete(const []);
    await refresh;
    await close;

    expect(connection.closeCalled, isTrue);
    expect(database.closeCalled, isTrue);
  });

  test('continues closing resources after an earlier close fails', () async {
    final database = _TrackingDatabase();
    final connection = _CloseTrackingConnection(
      profile: _profile('gizclaw.local:9820'),
      client: _RunWorkspaceClient(),
      serverId: 'server-a',
      closeError: StateError('connection close failed'),
    );
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
    );

    await expectLater(controller.close(), throwsStateError);

    expect(connection.closeCalled, isTrue);
    expect(database.closeCalled, isTrue);
  });

  test('waits for an in-flight initial connect before closing', () async {
    final database = _TrackingDatabase();
    final client = _RunWorkspaceClient();
    final connection = _BlockingConnectConnection(
      profile: _profile('gizclaw.local:9820'),
      client: client,
      serverId: 'server-a',
    );
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
    );

    final start = controller.start();
    await connection.connectStarted.future;
    final close = controller.close();
    await Future<void>.delayed(Duration.zero);

    expect(connection.closeCalled, isFalse);
    expect(database.closeCalled, isFalse);

    connection.connectResult.complete(client);
    await start;
    await close;

    expect(connection.closeCalled, isTrue);
    expect(database.closeCalled, isTrue);
  });

  test('waits for an in-flight reconnect before closing', () async {
    final database = _TrackingDatabase();
    final client = _RunWorkspaceClient();
    final connection = _BlockingReconnectConnection(
      profile: _profile('gizclaw.local:9820'),
      client: client,
      serverId: 'server-a',
    );
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
    );

    final reconnect = controller.recoverTransport();
    await connection.reconnectStarted.future;
    final close = controller.close();
    await Future<void>.delayed(Duration.zero);

    expect(connection.closeCalled, isFalse);
    expect(database.closeCalled, isFalse);

    connection.reconnectResult.complete(client);
    await reconnect;
    await close;

    expect(connection.closeCalled, isTrue);
    expect(database.closeCalled, isTrue);
  });

  test('coalesces foreground and user microphone recovery', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final client = _RunWorkspaceClient();
    final connection = _BlockingReconnectConnection(
      profile: _profile('gizclaw.local:9820'),
      client: client,
      serverId: 'server-a',
    );
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
    )..connectionState = MobileConnectionState.connected;

    final userRecovery = controller.recoverMicrophone();
    final foregroundRecovery = controller.recoverMicrophone();
    expect(foregroundRecovery, same(userRecovery));
    controller.handleAppResumed();
    await connection.reconnectStarted.future;
    await Future<void>.delayed(Duration.zero);

    connection.reconnectResult.complete(client);
    await userRecovery;
    await controller.close();
  });

  test(
    'forces one microphone recovery after returning from background',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      final client = _RunWorkspaceClient();
      final connection = _BlockingReconnectConnection(
        profile: _profile('gizclaw.local:9820'),
        client: client,
        serverId: 'server-a',
        microphoneStatus: const MicrophoneStatus.ready(),
      );
      final controller = MobileDataController(
        database: database,
        connectionController: connection,
      )..connectionState = MobileConnectionState.connected;

      controller.handleAppResumed();
      await Future<void>.delayed(Duration.zero);
      expect(connection.reconnectCalls, 0);

      controller.handleAppPaused();
      controller.handleAppResumed();
      await connection.reconnectStarted.future;
      expect(connection.reconnectCalls, 1);

      controller.handleAppPaused();
      controller.handleAppResumed();
      await Future<void>.delayed(Duration.zero);
      expect(connection.reconnectCalls, 1);

      connection.reconnectResult.complete(client);
      await Future<void>.delayed(Duration.zero);
      await controller.close();
    },
  );

  test(
    'retries a failed reconnect while the app remains backgrounded',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      final client = _RunWorkspaceClient();
      final connection = _BackgroundRetryConnection(
        profile: _profile('gizclaw.local:9820'),
        client: client,
        serverId: 'server-a',
      );
      final controller = MobileDataController(
        database: database,
        connectionController: connection,
        backgroundReconnectInitialDelay: Duration.zero,
        backgroundReconnectMaxDelay: Duration.zero,
      )..connectionState = MobileConnectionState.connected;

      controller.handleAppPaused();
      await expectLater(controller.recoverTransport(), throwsStateError);
      await connection.reconnected.future;
      for (
        var attempts = 0;
        attempts < 20 &&
            controller.connectionState != MobileConnectionState.connected;
        attempts += 1
      ) {
        await Future<void>.delayed(Duration.zero);
      }

      expect(connection.reconnectCalls, 2);
      expect(controller.connectionState, MobileConnectionState.connected);
      await controller.close();
    },
  );

  test('keeps retrying a failed connection in the foreground', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final client = _RunWorkspaceClient();
    final connection = _BackgroundRetryConnection(
      profile: _profile('gizclaw.local:9820'),
      client: client,
      serverId: 'server-a',
    );
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
      backgroundReconnectInitialDelay: Duration.zero,
      backgroundReconnectMaxDelay: Duration.zero,
    )..connectionState = MobileConnectionState.connected;

    await expectLater(controller.recoverTransport(), throwsStateError);
    await connection.reconnected.future;
    for (
      var attempts = 0;
      attempts < 20 &&
          controller.connectionState != MobileConnectionState.connected;
      attempts += 1
    ) {
      await Future<void>.delayed(Duration.zero);
    }

    expect(connection.reconnectCalls, 2);
    expect(controller.connectionState, MobileConnectionState.connected);
    await controller.close();
  });

  test('reconnects immediately when the app resumes offline', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final client = _RunWorkspaceClient();
    final connection = _BackgroundRetryConnection(
      profile: _profile('gizclaw.local:9820'),
      client: client,
      serverId: 'server-a',
      failuresRemaining: 0,
    )..connected = false;
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
    )..connectionState = MobileConnectionState.offline;

    controller.handleAppPaused();
    controller.handleAppResumed();
    await connection.reconnected.future;
    for (
      var attempts = 0;
      attempts < 20 &&
          controller.connectionState != MobileConnectionState.connected;
      attempts += 1
    ) {
      await Future<void>.delayed(Duration.zero);
    }

    expect(connection.reconnectCalls, 1);
    expect(controller.connectionState, MobileConnectionState.connected);
    await controller.close();
  });

  test(
    'ends active input before recovering an ended microphone track',
    () async {
      final connection = _EndedMicrophoneConnection(
        profile: _profile('gizclaw.local:9820'),
      );
      final controller = _EndedMicrophoneController(connection)
        ..connectionState = MobileConnectionState.connected;
      addTearDown(controller.close);
      controller.chat.recording = true;

      connection.endMicrophoneTrack();
      await Future<void>.delayed(Duration.zero);
      await Future<void>.delayed(Duration.zero);

      expect(controller.chat.finishErrors, ['microphone_track_ended']);
      expect(controller.chat.recording, isFalse);
      expect(controller.recoveryCalls, 1);
      expect(controller.recoveredAfterFinish, isTrue);
    },
  );

  test('suppresses microphone recovery during a server switch', () async {
    final connection = _ProfileSwitchConnection(
      profile: _profile('old.local:9820'),
      client: _RunWorkspaceClient(),
      serverId: 'server-a',
    );
    final controller = _RecoveryCountingMobileDataController(connection)
      ..connectionState = MobileConnectionState.connected;
    addTearDown(controller.close);

    final switchServer = controller.updateServerEndpoint('new.local:9820');
    await connection.updateStarted.future;
    await Future<void>.delayed(Duration.zero);

    expect(controller.recoveryCalls, 0);
    expect(connection.connectCalls, 0);

    connection.allowUpdate.complete();
    await switchServer;
    for (
      var attempts = 0;
      attempts < 20 && connection.connectCalls == 0;
      attempts += 1
    ) {
      await Future<void>.delayed(Duration.zero);
    }

    expect(controller.recoveryCalls, 0);
    expect(connection.connectCalls, 1);
  });

  test('does not retry a mutating RPC after a transport failure', () async {
    var requests = 0;
    var reconnects = 0;

    await expectLater(
      runRpcWithTransportRecovery<void, int>(
        initialTransport: 1,
        request: (_) async {
          requests += 1;
          throw StateError('WebRTC data channel closed');
        },
        reconnect: () async {
          reconnects += 1;
          return 2;
        },
        retryOnTransportError: false,
      ),
      throwsStateError,
    );

    expect(requests, 1);
    expect(reconnects, 0);
  });

  test('retries an idempotent RPC after reconnecting the transport', () async {
    var requests = 0;
    var reconnects = 0;

    final result = await runRpcWithTransportRecovery<String, int>(
      initialTransport: 1,
      request: (transport) async {
        requests += 1;
        if (transport == 1) throw TimeoutException('request timed out');
        return 'ok';
      },
      reconnect: () async {
        reconnects += 1;
        return 2;
      },
      retryOnTransportError: true,
    );

    expect(result, 'ok');
    expect(requests, 2);
    expect(reconnects, 1);
  });

  test('drains a queued refresh after a stale refresh fails', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final oldClient = _RunWorkspaceClient();
    final newClient = _RunWorkspaceClient();
    final connection = _RefreshTestConnection(
      profile: _profile('old.local:9820'),
      client: oldClient,
      serverId: 'old-server',
    );
    final repository = _QueuedRefreshRepository(database);
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
      dataRepository: repository,
    )..connectionState = MobileConnectionState.connected;
    addTearDown(controller.close);

    final oldRefresh = controller.refresh(
      client: oldClient,
      serverId: 'old-server',
    );
    connection
      ..currentProfile = _profile('new.local:9820')
      ..currentClient = newClient
      ..currentServerId = 'new-server';
    final newRefresh = controller.refresh(
      client: newClient,
      serverId: 'new-server',
    );
    while (repository.endpoints.isEmpty) {
      await Future<void>.delayed(Duration.zero);
    }
    repository.firstRefresh.completeError(StateError('old refresh failed'));

    await Future.wait([oldRefresh, newRefresh]);

    expect(repository.endpoints, ['old.local:9820', 'new.local:9820']);
    expect(controller.connectionState, MobileConnectionState.connected);
    expect(controller.lastError, isNull);
  });

  test('switches cached server partitions after reconnect', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final oldClient = _RunWorkspaceClient();
    final newClient = _RunWorkspaceClient();
    final connection = _ReconnectTestConnection(
      profile: _profile('gizclaw.local:9820'),
      client: oldClient,
      serverId: 'old-server',
      reconnectClient: newClient,
      reconnectServerId: 'new-server',
    );
    final repository = _ReconnectRepository(database);
    final controller =
        MobileDataController(
            database: database,
            connectionController: connection,
            dataRepository: repository,
          )
          ..activeServerId = 'old-server'
          ..peerName = 'Old peer'
          ..peerEmoji = '👴'
          ..connectionState = MobileConnectionState.connected;
    addTearDown(controller.close);

    await controller.recoverTransport();

    expect(controller.activeServerId, 'new-server');
    expect(controller.peerName, isEmpty);
    expect(controller.peerEmoji, isEmpty);
    expect(repository.refreshServerIds, ['new-server']);
  });

  test('refreshes runtime workflows after a same-server reconnect', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final oldClient = _RunWorkspaceClient();
    final newClient = _RunWorkspaceClient();
    final connection = _ReconnectTestConnection(
      profile: _profile('gizclaw.local:9820'),
      client: oldClient,
      serverId: 'server-a',
      reconnectClient: newClient,
      reconnectServerId: 'server-a',
      initiallyConnected: false,
    );
    final repository = _ReconnectRepository(database);
    final controller =
        MobileDataController(
            database: database,
            connectionController: connection,
            dataRepository: repository,
          )
          ..activeServerId = 'server-a'
          ..connectionState = MobileConnectionState.offline;
    addTearDown(controller.close);

    controller.setEffectiveLocale(appSimplifiedChineseLocale);
    expect(repository.refreshServerIds, isEmpty);

    await controller.recoverTransport();

    expect(repository.refreshServerIds, ['server-a']);
  });

  test('creates typed defaults for a Doubao workspace', () {
    final parameters = newWorkspaceParametersForDriver(
      WorkflowDriverKind.doubaoRealtime,
    );
    final doubao = parameters.doubaoRealtimeWorkspaceParameters;
    expect(
      doubao.agentType,
      DoubaoRealtimeWorkspaceParametersAgentType
          .DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME,
    );
    expect(doubao.input, WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK);
  });

  test('keeps FlowCraft model aliases in the workflow', () {
    final defaults = newWorkspaceParametersForDriver(
      WorkflowDriverKind.flowcraft,
    ).flowcraftWorkspaceParameters;
    expect(
      defaults.input,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    );
  });

  test('creates the auto S2S profile for a translation workspace', () {
    final parameters = newWorkspaceParametersForDriver(
      WorkflowDriverKind.astTranslate,
    );
    final ast = parameters.asttranslateWorkspaceParameters;
    expect(ast.enableSourceLanguageDetect, isTrue);
    expect(ast.langPair, 'auto');
    expect(ast.mode, ASTTranslateMode.ASTTRANSLATE_MODE_S2S);
    expect(ast.hasTranslationModel(), isFalse);
  });

  test('repairs an empty parameter envelope for mode switching', () {
    final workspace = Workspace(
      name: 'translator',
      workflowAlias: 'volc-ast-translate',
      parameters: WorkspaceParameters(),
    );

    final repaired = workspaceWithDefaultInputParameters(
      workspace,
      WorkflowDriverKind.astTranslate,
    );

    expect(repaired, isNotNull);
    expect(
      repaired!.parameters.asttranslateWorkspaceParameters.input,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    );
    expect(
      repaired.parameters.asttranslateWorkspaceParameters.mode,
      ASTTranslateMode.ASTTRANSLATE_MODE_S2S,
    );
  });

  test('repairs missing translation parameters with the safe auto default', () {
    final workspace = Workspace(
      name: 'japanese-translator',
      workflowAlias: 'translate-zh-ja',
      parameters: WorkspaceParameters(),
    );

    final repaired = workspaceWithDefaultInputParameters(
      workspace,
      WorkflowDriverKind.astTranslate,
    );

    expect(repaired, isNotNull);
    final parameters = repaired!.parameters.asttranslateWorkspaceParameters;
    expect(parameters.enableSourceLanguageDetect, isTrue);
    expect(parameters.langPair, 'auto');
    expect(parameters.mode, ASTTranslateMode.ASTTRANSLATE_MODE_S2S);
  });

  test('preserves existing typed workspace parameters', () {
    final workspace = Workspace(
      parameters: WorkspaceParameters(
        asttranslateWorkspaceParameters: ASTTranslateWorkspaceParameters(
          input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME,
          langPair: 'zh/en',
        ),
      ),
    );

    expect(
      workspaceWithDefaultInputParameters(
        workspace,
        WorkflowDriverKind.astTranslate,
      ),
      isNull,
    );
    expect(
      workspace.parameters.asttranslateWorkspaceParameters.input,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME,
    );
    expect(
      workspace.parameters.asttranslateWorkspaceParameters.langPair,
      'zh/en',
    );
  });

  test(
    'falls back to the workspace catalog when pet discovery fails',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      final client = _FailingPetListClient();
      final controller =
          MobileDataController(
              database: database,
              connectionController: _RefreshTestConnection(
                profile: _profile('gizclaw.local:9820'),
                client: client,
                serverId: 'server-a',
              ),
            )
            ..workspaces = const [
              WorkspaceCard(
                name: 'workspace-a',
                workflowAlias: 'chat',
                collection: 'raids',
                lastActive: '',
              ),
            ];
      addTearDown(controller.close);

      final destination = await controller.destinationForWorkspace(
        'workspace-a',
      );

      expect(destination.surface, MobileWorkspaceSurface.raid);
      expect(destination.workspaceName, 'workspace-a');
    },
  );

  test('repairs the selected workspace before runtime reload', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final client = _WorkspaceActivationClient();
    final controller = MobileDataController(
      database: database,
      connectionController: _RefreshTestConnection(
        profile: _profile('gizclaw.local:9820'),
        client: client,
        serverId: 'server-a',
      ),
    )..connectionState = MobileConnectionState.connected;
    addTearDown(controller.close);

    await controller.activateWorkspaceChat('workspace-new');

    expect(client.putWorkspaceNames, ['workspace-new']);
    expect(
      client
          .workspaces['workspace-new']!
          .parameters
          .flowcraftWorkspaceParameters
          .input,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    );
    expect(controller.activeWorkspaceName, 'workspace-new');
  });

  test('404 selection failure evicts only the targeted projection', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final repository = MobileDataRepository(database);
    final client = _WorkspaceActivationClient();
    final controller = MobileDataController(
      database: database,
      connectionController: _RefreshTestConnection(
        profile: _profile('gizclaw.local:9820'),
        client: client,
        serverId: 'server-a',
      ),
      dataRepository: repository,
    )..activeServerId = 'server-a';
    addTearDown(controller.close);
    await _seedWorkspaceProjection(repository, client, serverId: 'server-a');
    await _seedWorkspaceProjection(repository, client, serverId: 'server-b');
    final error = RpcError(404, 'workspace missing');
    client.selectionError = error;

    await expectLater(
      controller.activateWorkspaceChat('workspace-new'),
      throwsA(same(error)),
    );

    expect(
      await repository.workspaceDocument('server-a', 'workspace-new'),
      isNull,
    );
    expect(
      await repository.workspaceDocument('server-a', 'workspace-old'),
      isNotNull,
    );
    expect(
      await repository.workspaceDocument('server-b', 'workspace-new'),
      isNotNull,
    );
    expect(client.workspaceListCalls, 2);
  });

  test('stale client 404 does not evict a refreshed projection', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final repository = MobileDataRepository(database);
    final staleClient = _WorkspaceActivationClient();
    final currentClient = _WorkspaceActivationClient();
    final connection = _RefreshTestConnection(
      profile: _profile('gizclaw.local:9820'),
      client: staleClient,
      serverId: 'server-a',
    );
    final controller = MobileDataController(
      database: database,
      connectionController: connection,
      dataRepository: repository,
    )..activeServerId = 'server-a';
    addTearDown(controller.close);
    await _seedWorkspaceProjection(
      repository,
      staleClient,
      serverId: 'server-a',
    );
    connection.currentClient = currentClient;
    await _seedWorkspaceProjection(
      repository,
      currentClient,
      serverId: 'server-a',
    );

    await controller.reconcileWorkspaceFailure(
      'workspace-new',
      RpcError(404, 'stale workspace response'),
      staleClient,
      'server-a',
    );

    expect(
      await repository.workspaceDocument('server-a', 'workspace-new'),
      isNotNull,
    );
  });

  test(
    '403 selection failure removes a workspace absent from a fresh list',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      final repository = MobileDataRepository(database);
      final client = _WorkspaceActivationClient();
      final controller = MobileDataController(
        database: database,
        connectionController: _RefreshTestConnection(
          profile: _profile('gizclaw.local:9820'),
          client: client,
          serverId: 'server-a',
        ),
        dataRepository: repository,
      )..activeServerId = 'server-a';
      addTearDown(controller.close);
      await _seedWorkspaceProjection(repository, client, serverId: 'server-a');
      final listCallsBeforeFailure = client.workspaceListCalls;
      client.workspaceSnapshot = [
        client.workspaces['workspace-old']!.deepCopy(),
      ];
      final error = RpcError(403, 'workspace hidden');
      client.selectionError = error;

      await expectLater(
        controller.activateWorkspaceChat('workspace-new'),
        throwsA(same(error)),
      );

      expect(client.workspaceListCalls, listCallsBeforeFailure + 1);
      expect(
        await repository.workspaceDocument('server-a', 'workspace-new'),
        isNull,
      );
      expect(
        await repository.workspaceDocument('server-a', 'workspace-old'),
        isNotNull,
      );
    },
  );

  test(
    '403 selection failure keeps a workspace present in the fresh list',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      final repository = MobileDataRepository(database);
      final client = _WorkspaceActivationClient();
      final controller = MobileDataController(
        database: database,
        connectionController: _RefreshTestConnection(
          profile: _profile('gizclaw.local:9820'),
          client: client,
          serverId: 'server-a',
        ),
        dataRepository: repository,
      )..activeServerId = 'server-a';
      addTearDown(controller.close);
      await _seedWorkspaceProjection(repository, client, serverId: 'server-a');
      final error = RpcError(403, 'workspace forbidden');
      client.selectionError = error;

      await expectLater(
        controller.activateWorkspaceChat('workspace-new'),
        throwsA(same(error)),
      );

      expect(
        await repository.workspaceDocument('server-a', 'workspace-new'),
        isNotNull,
      );
    },
  );

  test(
    'failed 403 reconciliation preserves projection and original error',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      final repository = MobileDataRepository(database);
      final client = _WorkspaceActivationClient();
      final controller = MobileDataController(
        database: database,
        connectionController: _RefreshTestConnection(
          profile: _profile('gizclaw.local:9820'),
          client: client,
          serverId: 'server-a',
        ),
        dataRepository: repository,
      )..activeServerId = 'server-a';
      addTearDown(controller.close);
      await _seedWorkspaceProjection(repository, client, serverId: 'server-a');
      final error = RpcError(403, 'workspace forbidden');
      client
        ..selectionError = error
        ..workspaceListError = StateError('workspace catalog unavailable');

      await expectLater(
        controller.activateWorkspaceChat('workspace-new'),
        throwsA(same(error)),
      );

      expect(
        await repository.workspaceDocument('server-a', 'workspace-new'),
        isNotNull,
      );
    },
  );

  test(
    'non-authoritative selection failure does not reconcile projection',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      final repository = MobileDataRepository(database);
      final client = _WorkspaceActivationClient();
      final controller = MobileDataController(
        database: database,
        connectionController: _RefreshTestConnection(
          profile: _profile('gizclaw.local:9820'),
          client: client,
          serverId: 'server-a',
        ),
        dataRepository: repository,
      )..activeServerId = 'server-a';
      addTearDown(controller.close);
      await _seedWorkspaceProjection(repository, client, serverId: 'server-a');
      final listCallsBeforeFailure = client.workspaceListCalls;
      final error = RpcError(500, 'server unavailable');
      client.selectionError = error;

      await expectLater(
        controller.activateWorkspaceChat('workspace-new'),
        throwsA(same(error)),
      );

      expect(client.workspaceListCalls, listCallsBeforeFailure);
      expect(
        await repository.workspaceDocument('server-a', 'workspace-new'),
        isNotNull,
      );
    },
  );
}

Future<void> _seedWorkspaceProjection(
  MobileDataRepository repository,
  _WorkspaceActivationClient client, {
  required String serverId,
}) async {
  await repository.refreshWorkspaceSnapshot(
    client: client,
    endpoint: '$serverId.local',
    isCurrent: () => true,
    serverId: serverId,
  );
}

GizClawConnectionProfile _profile(String endpoint) =>
    GizClawConnectionProfile(endpoint: endpoint, clientPrivateKey: 'test-key');

class _QueuedRefreshRepository extends MobileDataRepository {
  _QueuedRefreshRepository(super.database);

  final firstRefresh = Completer<List<MobileDataRefreshWarning>>();
  final endpoints = <String>[];

  @override
  Future<List<MobileDataRefreshWarning>> refresh({
    required GizClawClient client,
    required String endpoint,
    required bool Function() isCurrent,
    required String serverId,
  }) {
    endpoints.add(endpoint);
    if (endpoints.length == 1) return firstRefresh.future;
    return Future.value(const []);
  }
}

class _ReconnectRepository extends MobileDataRepository {
  _ReconnectRepository(super.database);

  final refreshServerIds = <String>[];

  @override
  Future<List<MobileDataRefreshWarning>> refresh({
    required GizClawClient client,
    required String endpoint,
    required bool Function() isCurrent,
    required String serverId,
  }) async {
    refreshServerIds.add(serverId);
    return const [];
  }
}

class _RefreshTestConnection extends GizClawConnectionController {
  _RefreshTestConnection({
    required GizClawConnectionProfile profile,
    required GizClawClient client,
    required String serverId,
  }) : currentProfile = profile,
       currentClient = client,
       currentServerId = serverId,
       super(profile);

  GizClawConnectionProfile currentProfile;
  GizClawClient currentClient;
  String currentServerId;

  @override
  GizClawClient get client => currentClient;

  @override
  bool get isConnected => true;

  @override
  GizClawConnectionProfile get profile => currentProfile;

  @override
  String get serverId => currentServerId;

  @override
  Future<GizClawClient> connect() async => currentClient;

  @override
  Future<void> updateProfile(GizClawConnectionProfile profile) async {
    currentProfile = profile;
  }
}

class _ReconnectTestConnection extends _RefreshTestConnection {
  _ReconnectTestConnection({
    required super.profile,
    required super.client,
    required super.serverId,
    required this.reconnectClient,
    required this.reconnectServerId,
    this.initiallyConnected = true,
  });

  final GizClawClient reconnectClient;
  final String reconnectServerId;
  bool initiallyConnected;

  @override
  bool get isConnected => initiallyConnected;

  @override
  Future<GizClawClient> reconnect() async {
    currentClient = reconnectClient;
    currentServerId = reconnectServerId;
    initiallyConnected = true;
    return reconnectClient;
  }
}

class _CloseTrackingConnection extends _RefreshTestConnection {
  _CloseTrackingConnection({
    required super.profile,
    required super.client,
    required super.serverId,
    this.closeError,
  });

  final Object? closeError;
  bool closeCalled = false;

  @override
  Future<void> close() async {
    closeCalled = true;
    final error = closeError;
    if (error != null) throw error;
  }
}

class _BlockingConnectConnection extends _CloseTrackingConnection {
  _BlockingConnectConnection({
    required super.profile,
    required super.client,
    required super.serverId,
  });

  final connectStarted = Completer<void>();
  final connectResult = Completer<GizClawClient>();

  @override
  Future<GizClawClient> connect() {
    connectStarted.complete();
    return connectResult.future;
  }
}

class _BlockingReconnectConnection extends _CloseTrackingConnection {
  _BlockingReconnectConnection({
    required super.profile,
    required super.client,
    required super.serverId,
    this.microphoneStatus = const MicrophoneStatus.unavailable(),
  });

  final reconnectStarted = Completer<void>();
  final reconnectResult = Completer<GizClawClient>();
  @override
  final MicrophoneStatus microphoneStatus;
  int reconnectCalls = 0;

  @override
  Future<GizClawClient> reconnect() {
    reconnectCalls += 1;
    if (!reconnectStarted.isCompleted) reconnectStarted.complete();
    return reconnectResult.future;
  }
}

class _BackgroundRetryConnection extends _RefreshTestConnection {
  _BackgroundRetryConnection({
    required super.profile,
    required super.client,
    required super.serverId,
    this.failuresRemaining = 1,
  });

  final reconnected = Completer<void>();
  int failuresRemaining;
  int reconnectCalls = 0;
  bool connected = true;

  @override
  bool get isConnected => connected;

  @override
  Future<GizClawClient> reconnect() async {
    reconnectCalls += 1;
    if (failuresRemaining > 0) {
      failuresRemaining -= 1;
      connected = false;
      throw StateError('background reconnect failed');
    }
    connected = true;
    if (!reconnected.isCompleted) reconnected.complete();
    return currentClient;
  }
}

class _EndedMicrophoneConnection extends GizClawConnectionController {
  _EndedMicrophoneConnection({required GizClawConnectionProfile profile})
    : super(profile);

  MicrophoneStatus status = const MicrophoneStatus.ready();

  @override
  MicrophoneStatus get microphoneStatus => status;

  void endMicrophoneTrack() {
    status = const MicrophoneStatus.unavailable(
      failureKind: MicrophoneFailureKind.captureUnavailable,
    );
    notifyListeners();
  }

  @override
  Future<void> close() async {}
}

class _ProfileSwitchConnection extends _RefreshTestConnection {
  _ProfileSwitchConnection({
    required super.profile,
    required super.client,
    required super.serverId,
  });

  final updateStarted = Completer<void>();
  final allowUpdate = Completer<void>();
  MicrophoneStatus status = const MicrophoneStatus.ready();
  bool connected = true;
  int connectCalls = 0;

  @override
  bool get isConnected => connected;

  @override
  MicrophoneStatus get microphoneStatus => status;

  @override
  Future<void> updateProfile(GizClawConnectionProfile profile) async {
    currentProfile = profile;
    connected = false;
    status = const MicrophoneStatus.unavailable();
    notifyListeners();
    updateStarted.complete();
    await allowUpdate.future;
  }

  @override
  Future<GizClawClient> connect() async {
    connectCalls += 1;
    connected = true;
    status = const MicrophoneStatus.ready();
    notifyListeners();
    return currentClient;
  }

  @override
  Future<void> close() async {}
}

class _RecoveryCountingMobileDataController extends MobileDataController {
  _RecoveryCountingMobileDataController(_ProfileSwitchConnection connection)
    : super(
        database: AppDatabase.forTesting(NativeDatabase.memory()),
        connectionController: connection,
      );

  int recoveryCalls = 0;

  @override
  Future<MicrophoneStatus> recoverMicrophone() async {
    recoveryCalls += 1;
    return const MicrophoneStatus.ready();
  }
}

class _EndedMicrophoneController extends MobileDataController {
  _EndedMicrophoneController(_EndedMicrophoneConnection connection)
    : super(
        database: AppDatabase.forTesting(NativeDatabase.memory()),
        connectionController: connection,
      ) {
    chat = _EndedTrackChatController(workspaceChatRepository);
  }

  late final _EndedTrackChatController chat;
  int recoveryCalls = 0;
  bool recoveredAfterFinish = false;

  @override
  WorkspaceChatController? get activeWorkspaceChat => chat;

  @override
  Future<MicrophoneStatus> recoverMicrophone() async {
    recoveryCalls += 1;
    recoveredAfterFinish = !chat.recording && chat.finishErrors.isNotEmpty;
    return const MicrophoneStatus.ready();
  }

  @override
  Future<void> close() async {
    await chat.close();
    await super.close();
  }
}

class _EndedTrackChatController extends WorkspaceChatController {
  _EndedTrackChatController(WorkspaceChatRepository repository)
    : super(
        workspaceName: 'translator',
        repository: repository,
        serverId: null,
      );

  final finishErrors = <String?>[];

  @override
  Future<void> finishInput({String? error}) async {
    finishErrors.add(error);
    await Future<void>.delayed(Duration.zero);
    recording = false;
  }
}

class _TrackingDatabase extends AppDatabase {
  _TrackingDatabase() : super.forTesting(NativeDatabase.memory());

  bool closeCalled = false;

  @override
  Future<void> close() async {
    closeCalled = true;
    await super.close();
  }
}

class _RunWorkspaceClient extends GizClawClient {
  _RunWorkspaceClient() : super(_NeverDataChannelFactory());

  @override
  Future<WorkflowListResponse> listWorkflows({
    required String collection,
    String? cursor,
    int? limit,
  }) async => WorkflowListResponse(
    runtimeProfileName: 'test',
    runtimeProfileRevision: 'revision-1',
  );

  @override
  Future<ServerGetRunWorkspaceResponse> getRunWorkspace() async {
    return ServerGetRunWorkspaceResponse(value: PeerRunWorkspaceState());
  }
}

class _PartialWorkflowCatalogClient extends _RunWorkspaceClient {
  final collections = <String>[];

  @override
  Future<WorkflowListResponse> listWorkflows({
    required String collection,
    String? cursor,
    int? limit,
  }) async {
    collections.add(collection);
    if (collection == 'translates') {
      throw RpcError(404, 'workflow collection not found');
    }
    return WorkflowListResponse(
      items: collection == 'assistants'
          ? [
              Workflow(
                alias: 'chat',
                collection: 'assistants',
                driver: WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT,
                i18n: {
                  'en': AliasI18nText(displayName: 'Chat'),
                  'zh-CN': AliasI18nText(displayName: '聊天'),
                }.entries,
              ),
            ]
          : const [],
      runtimeProfileName: 'default',
      runtimeProfileRevision: 'revision-1',
    );
  }
}

class _WorkspaceCreateClient extends _RunWorkspaceClient {
  final requests = <WorkspaceCreateBody>[];

  @override
  Future<WorkspaceCreateResponse> createWorkspace(
    WorkspaceCreateBody workspace,
  ) async {
    requests.add(WorkspaceCreateBody.fromBuffer(workspace.writeToBuffer()));
    return WorkspaceCreateResponse(
      value: Workspace(
        name: workspace.name,
        workflowAlias: workspace.workflowAlias,
      ),
    );
  }
}

class _WorkspaceCreateTestController extends MobileDataController {
  _WorkspaceCreateTestController(_WorkspaceCreateClient client)
    : super(
        database: AppDatabase.forTesting(NativeDatabase.memory()),
        connectionController: _RefreshTestConnection(
          profile: _profile('gizclaw.local:9820'),
          client: client,
          serverId: 'server-a',
        ),
      );

  @override
  Future<void> refresh({GizClawClient? client, String? serverId}) async {}
}

class _FailingPetListClient extends _RunWorkspaceClient {
  @override
  Future<ServerPetListResponse> listPets({String? cursor, int? limit}) async {
    throw StateError('gameplay RPC unavailable');
  }
}

class _WorkspaceActivationClient extends _RunWorkspaceClient {
  final workspaces = <String, Workspace>{
    'workspace-old': Workspace(
      name: 'workspace-old',
      workflowAlias: 'chat',
      parameters: newWorkspaceParametersForDriver(WorkflowDriverKind.flowcraft),
    ),
    'workspace-new': Workspace(
      name: 'workspace-new',
      workflowAlias: 'chat',
      parameters: WorkspaceParameters(),
    ),
  };
  final putWorkspaceNames = <String>[];
  Object? selectionError;
  Object? workspaceListError;
  List<Workspace>? workspaceSnapshot;
  int workspaceListCalls = 0;

  @override
  Future<ServerSetRunWorkspaceResponse> setRunWorkspace(String name) async {
    final error = selectionError;
    if (error != null) throw error;
    return ServerSetRunWorkspaceResponse(
      value: PeerRunWorkspaceState(
        activeWorkspaceName: 'workspace-old',
        selectedWorkspaceName: name,
        pendingWorkspaceName: name,
      ),
    );
  }

  @override
  Future<WorkspaceListResponse> listWorkspaces({
    required String collection,
    String? cursor,
    int? limit,
    String? prefix,
  }) async {
    if (collection != 'raids') return WorkspaceListResponse();
    workspaceListCalls += 1;
    final error = workspaceListError;
    if (error != null) throw error;
    return WorkspaceListResponse(items: workspaceSnapshot ?? workspaces.values);
  }

  @override
  Future<WorkflowGetResponse> getWorkflow(String alias) async {
    return WorkflowGetResponse(
      value: Workflow(
        alias: alias,
        collection: 'raids',
        driver: WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT,
      ),
    );
  }

  @override
  Future<WorkspaceGetResponse> getWorkspace(String name) async {
    return WorkspaceGetResponse(value: workspaces[name]!.deepCopy());
  }

  @override
  Future<WorkspacePutResponse> putWorkspace(
    String name,
    WorkspacePutBody workspace,
  ) async {
    putWorkspaceNames.add(name);
    final stored = workspaces[name]!.deepCopy();
    if (workspace.hasParameters()) {
      stored.parameters = workspace.parameters.deepCopy();
    }
    if (workspace.hasToolkit()) {
      stored.toolkit = workspace.toolkit.deepCopy();
    }
    workspaces[name] = stored;
    return WorkspacePutResponse(value: stored);
  }

  @override
  Future<ServerReloadRunWorkspaceResponse> reloadRunWorkspace() async {
    return ServerReloadRunWorkspaceResponse(
      value: PeerRunWorkspaceState(activeWorkspaceName: 'workspace-new'),
    );
  }
}

class _NeverDataChannelFactory implements GizClawDataChannelFactory {
  @override
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  }) {
    throw UnsupportedError('No data channel is used by this test');
  }
}
