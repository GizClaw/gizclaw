import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:gizclaw/gizclaw.dart';

import '../connection/gizclaw_connection_controller.dart';
import '../prototype/prototype_data.dart';
import '../prototype/prototype_models.dart';
import 'database/app_database.dart';
import 'repositories/mobile_data_repository.dart';
import 'repositories/workspace_chat_repository.dart';
import 'workspace_chat_controller.dart';

enum MobileConnectionState { unconfigured, connecting, connected, offline }

class MobileDataController extends ChangeNotifier {
  MobileDataController({
    AppDatabase? database,
    GizClawConnectionProfile? profile,
  }) : database = database ?? AppDatabase(),
       connection = GizClawConnectionController(
         profile ?? GizClawConnectionProfile.fromEnvironment(),
       ) {
    repository = MobileDataRepository(this.database);
  }

  factory MobileDataController.demo() {
    final controller = MobileDataController();
    controller.workflows = allWorkflows;
    controller.workspaces = workflowWorkspaces;
    return controller;
  }

  final AppDatabase database;
  final GizClawConnectionController connection;
  late final MobileDataRepository repository;
  late final WorkspaceChatRepository workspaceChatRepository =
      WorkspaceChatRepository(database);

  StreamSubscription<List<WorkflowCard>>? _workflowSubscription;
  StreamSubscription<List<WorkspaceCard>>? _workspaceSubscription;
  List<WorkflowCard> workflows = const [];
  List<WorkspaceCard> workspaces = const [];
  String? activeServerId;
  MobileConnectionState connectionState = MobileConnectionState.unconfigured;
  Object? lastError;
  bool refreshing = false;

  Future<void> start() async {
    if (!connection.profile.isConfigured) {
      connectionState = MobileConnectionState.unconfigured;
      notifyListeners();
      return;
    }
    connectionState = MobileConnectionState.connecting;
    notifyListeners();
    final cachedServerId = await repository.serverIdForEndpoint(
      connection.profile.endpoint,
    );
    if (cachedServerId != null) await _watchServer(cachedServerId);
    try {
      final client = await connection.connect();
      final serverId = connection.serverId!;
      if (serverId != cachedServerId) await _watchServer(serverId);
      connectionState = MobileConnectionState.connected;
      notifyListeners();
      await refresh(client: client, serverId: serverId);
    } catch (error) {
      final discoveredServerId = connection.serverId;
      if (cachedServerId == null && discoveredServerId != null) {
        await _watchServer(discoveredServerId);
      }
      lastError = error;
      assert(() {
        debugPrint('GizClaw connection failed: $error');
        return true;
      }());
      connectionState = MobileConnectionState.offline;
      notifyListeners();
    }
  }

  Future<void> _watchServer(String serverId) async {
    activeServerId = serverId;
    await _workflowSubscription?.cancel();
    await _workspaceSubscription?.cancel();
    _workflowSubscription = repository.watchWorkflows(serverId).listen((value) {
      workflows = value;
      notifyListeners();
    });
    _workspaceSubscription = repository.watchWorkspaces(serverId).listen((
      value,
    ) {
      workspaces = value;
      notifyListeners();
    });
  }

  Future<void> refresh({GizClawClient? client, String? serverId}) async {
    final activeClient = client ?? connection.client;
    final activeServerId = serverId ?? connection.serverId;
    if (activeClient == null || activeServerId == null || refreshing) return;
    refreshing = true;
    lastError = null;
    notifyListeners();
    try {
      await repository.refresh(
        client: activeClient,
        endpoint: connection.profile.endpoint,
        serverId: activeServerId,
      );
      connectionState = MobileConnectionState.connected;
    } catch (error) {
      lastError = error;
      assert(() {
        debugPrint('GizClaw refresh failed: $error');
        return true;
      }());
      connectionState = MobileConnectionState.offline;
    } finally {
      refreshing = false;
      notifyListeners();
    }
  }

  WorkflowCard workflow(String name) {
    return workflows.firstWhere(
      (item) => item.name == name,
      orElse: () => WorkflowCard.unknown(name),
    );
  }

  WorkspaceCard workspace(String name) {
    return workspaces.firstWhere(
      (item) => item.name == name,
      orElse: () => WorkspaceCard(
        name: name,
        workflowName: '',
        lastActive: 'Unavailable',
      ),
    );
  }

  WorkspaceChatController createWorkspaceChat(String workspaceName) {
    return WorkspaceChatController(
      workspaceName: workspaceName,
      repository: workspaceChatRepository,
      serverId: activeServerId,
      client: connection.client,
      dataChannelFactory: connection.dataChannelFactory,
      peerConnection: connection.peerConnection,
    );
  }

  @override
  void dispose() {
    unawaited(_workflowSubscription?.cancel());
    unawaited(_workspaceSubscription?.cancel());
    unawaited(connection.close());
    unawaited(database.close());
    super.dispose();
  }
}

class MobileDataScope extends InheritedNotifier<MobileDataController> {
  const MobileDataScope({
    super.key,
    required MobileDataController controller,
    required super.child,
  }) : super(notifier: controller);

  static MobileDataController watch(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<MobileDataScope>();
    assert(scope != null, 'MobileDataScope is missing');
    return scope!.notifier!;
  }
}
