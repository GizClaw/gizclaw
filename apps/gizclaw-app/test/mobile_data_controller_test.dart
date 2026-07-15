import 'dart:async';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/connection/gizclaw_connection_controller.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/mobile_data_controller.dart';
import 'package:gizclaw_app/data/repositories/mobile_data_repository.dart';
import 'package:gizclaw_app/prototype/prototype_models.dart';

void main() {
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
    addTearDown(database.close);
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
    repository.firstRefresh.completeError(StateError('old refresh failed'));

    await Future.wait([oldRefresh, newRefresh]);

    expect(repository.endpoints, ['old.local:9820', 'new.local:9820']);
    expect(controller.connectionState, MobileConnectionState.connected);
    expect(controller.lastError, isNull);
  });

  test('switches cached server partitions after reconnect', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
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
          ..connectionState = MobileConnectionState.connected;
    addTearDown(controller.dispose);

    await controller.recoverTransport();

    expect(controller.activeServerId, 'new-server');
    expect(repository.workflowWatchServerIds, ['new-server']);
    expect(repository.refreshServerIds, ['new-server']);
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
      workflowName: 'volc-ast-translate',
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
      addTearDown(database.close);
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
            ..workflows = [
              WorkflowCard.fromServer(
                name: 'flow-a',
                description: '',
                driver: 'flowcraft',
              ),
            ]
            ..workspaces = const [
              WorkspaceCard(
                name: 'workspace-a',
                workflowName: 'flow-a',
                lastActive: '',
              ),
            ];

      final destination = await controller.destinationForWorkspace(
        'workspace-a',
      );

      expect(destination.surface, MobileWorkspaceSurface.raid);
      expect(destination.driver, WorkflowDriverKind.flowcraft);
    },
  );

  test('repairs the selected workspace before runtime reload', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final client = _WorkspaceActivationClient();
    final repository = _RecordingWorkspaceRepository(database)
      ..workspace = client.workspaces['workspace-new'];
    final controller =
        MobileDataController(
            database: database,
            dataRepository: repository,
            connectionController: _RefreshTestConnection(
              profile: _profile('gizclaw.local:9820'),
              client: client,
              serverId: 'server-a',
            ),
          )
          ..activeServerId = 'server-a'
          ..connectionState = MobileConnectionState.connected
          ..workflows = [
            WorkflowCard.fromServer(
              name: 'flow-a',
              description: '',
              driver: 'flowcraft',
            ),
          ];
    addTearDown(controller.dispose);

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

  test(
    'evicts a cached workspace when server rejects activation as missing',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(database.close);
      final repository = _RecordingWorkspaceRepository(database)
        ..workspace = Workspace(
          name: 'missing-workspace',
          workflowName: 'flow-a',
        );
      final client = _MissingWorkspaceActivationClient();
      final controller =
          MobileDataController(
              database: database,
              dataRepository: repository,
              connectionController: _RefreshTestConnection(
                profile: _profile('gizclaw.local:9820'),
                client: client,
                serverId: 'server-a',
              ),
            )
            ..activeServerId = 'server-a'
            ..connectionState = MobileConnectionState.connected;
      addTearDown(controller.dispose);

      await expectLater(
        controller.activateWorkspaceChat('missing-workspace'),
        throwsStateError,
      );

      expect(client.setWorkspaceNames, ['missing-workspace']);
      expect(repository.deletedWorkspaces, [
        (serverId: 'server-a', name: 'missing-workspace'),
      ]);
    },
  );

  test(
    'refreshes and evicts a cached workspace denied after server deletion',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(database.close);
      final repository = _RecordingWorkspaceRepository(database)
        ..workspace = Workspace(
          name: 'deleted-workspace',
          workflowName: 'flow-a',
        )
        ..clearWorkspaceOnRefresh = true;
      final client = _DeniedWorkspaceActivationClient();
      final controller =
          MobileDataController(
              database: database,
              dataRepository: repository,
              connectionController: _RefreshTestConnection(
                profile: _profile('gizclaw.local:9820'),
                client: client,
                serverId: 'server-a',
              ),
            )
            ..activeServerId = 'server-a'
            ..connectionState = MobileConnectionState.connected;
      addTearDown(controller.dispose);

      await expectLater(
        controller.activateWorkspaceChat('deleted-workspace'),
        throwsA(
          isA<StateError>().having(
            (error) => error.message,
            'message',
            'This workspace was deleted or you no longer have access to it.',
          ),
        ),
      );

      expect(client.setWorkspaceNames, ['deleted-workspace']);
      expect(repository.refreshCount, 1);
      expect(repository.deletedWorkspaces, [
        (serverId: 'server-a', name: 'deleted-workspace'),
      ]);
    },
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
    required String serverId,
  }) {
    endpoints.add(endpoint);
    if (endpoints.length == 1) return firstRefresh.future;
    return Future.value(const []);
  }
}

class _ReconnectRepository extends MobileDataRepository {
  _ReconnectRepository(super.database);

  final workflowWatchServerIds = <String>[];
  final refreshServerIds = <String>[];

  @override
  Stream<List<WorkflowCard>> watchWorkflows(String serverId) {
    workflowWatchServerIds.add(serverId);
    return const Stream.empty();
  }

  @override
  Future<List<MobileDataRefreshWarning>> refresh({
    required GizClawClient client,
    required String endpoint,
    required String serverId,
  }) async {
    refreshServerIds.add(serverId);
    return const [];
  }
}

class _RecordingWorkspaceRepository extends MobileDataRepository {
  _RecordingWorkspaceRepository(super.database);

  bool clearWorkspaceOnRefresh = false;
  final deletedWorkspaces = <({String serverId, String name})>[];
  int refreshCount = 0;
  Workspace? workspace;

  @override
  Future<List<MobileDataRefreshWarning>> refresh({
    required GizClawClient client,
    required String endpoint,
    required String serverId,
  }) async {
    refreshCount++;
    if (clearWorkspaceOnRefresh) workspace = null;
    return const [];
  }

  @override
  Future<Workspace?> workspaceDocument(String serverId, String name) async {
    return workspace?.deepCopy();
  }

  @override
  Future<void> deleteWorkspace(String serverId, String name) async {
    deletedWorkspaces.add((serverId: serverId, name: name));
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
}

class _ReconnectTestConnection extends _RefreshTestConnection {
  _ReconnectTestConnection({
    required super.profile,
    required super.client,
    required super.serverId,
    required this.reconnectClient,
    required this.reconnectServerId,
  });

  final GizClawClient reconnectClient;
  final String reconnectServerId;

  @override
  Future<GizClawClient> reconnect() async {
    currentClient = reconnectClient;
    currentServerId = reconnectServerId;
    return reconnectClient;
  }
}

class _RunWorkspaceClient extends GizClawClient {
  _RunWorkspaceClient() : super(_NeverDataChannelFactory());

  @override
  Future<ServerGetRunWorkspaceResponse> getRunWorkspace() async {
    return ServerGetRunWorkspaceResponse(value: PeerRunWorkspaceState());
  }
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
      workflowName: 'flow-a',
      parameters: newWorkspaceParametersForDriver(WorkflowDriverKind.flowcraft),
    ),
    'workspace-new': Workspace(
      name: 'workspace-new',
      workflowName: 'flow-a',
      parameters: WorkspaceParameters(),
    ),
  };
  final putWorkspaceNames = <String>[];

  @override
  Future<ServerSetRunWorkspaceResponse> setRunWorkspace(String name) async {
    return ServerSetRunWorkspaceResponse(
      value: PeerRunWorkspaceState(
        activeWorkspaceName: 'workspace-old',
        selectedWorkspaceName: name,
        pendingWorkspaceName: name,
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
    Workspace workspace,
  ) async {
    putWorkspaceNames.add(name);
    workspaces[name] = workspace.deepCopy();
    return WorkspacePutResponse(value: workspace);
  }

  @override
  Future<ServerReloadRunWorkspaceResponse> reloadRunWorkspace() async {
    return ServerReloadRunWorkspaceResponse(
      value: PeerRunWorkspaceState(activeWorkspaceName: 'workspace-new'),
    );
  }
}

class _MissingWorkspaceActivationClient extends _RunWorkspaceClient {
  final setWorkspaceNames = <String>[];

  @override
  Future<ServerSetRunWorkspaceResponse> setRunWorkspace(String name) async {
    setWorkspaceNames.add(name);
    throw RpcError(404, 'workspace "$name" not found');
  }
}

class _DeniedWorkspaceActivationClient extends _RunWorkspaceClient {
  final setWorkspaceNames = <String>[];

  @override
  Future<ServerSetRunWorkspaceResponse> setRunWorkspace(String name) async {
    setWorkspaceNames.add(name);
    throw RpcError(400, 'acl: denied');
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
