import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:gizclaw/gizclaw.dart';

import '../connection/gizclaw_connection_controller.dart';
import '../prototype/prototype_data.dart';
import '../prototype/prototype_models.dart';
import 'database/app_database.dart';
import 'device_workspace_provisioner.dart';
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
    controller.chatroomWorkspaces = chatroomWorkspaceMetadata;
    return controller;
  }

  final AppDatabase database;
  final GizClawConnectionController connection;
  late final MobileDataRepository repository;
  late final WorkspaceChatRepository workspaceChatRepository =
      WorkspaceChatRepository(database);

  StreamSubscription<List<WorkflowCard>>? _workflowSubscription;
  StreamSubscription<List<WorkspaceCard>>? _workspaceSubscription;
  StreamSubscription<List<ChatroomWorkspaceMetadata>>? _friendChatSubscription;
  StreamSubscription<List<ChatroomWorkspaceMetadata>>?
  _friendGroupChatSubscription;
  List<WorkflowCard> workflows = const [];
  List<WorkspaceCard> workspaces = const [];
  List<ChatroomWorkspaceMetadata> chatroomWorkspaces = const [];
  List<ChatroomWorkspaceMetadata> _friendChats = const [];
  List<ChatroomWorkspaceMetadata> _friendGroupChats = const [];
  String? activeServerId;
  MobileConnectionState connectionState = MobileConnectionState.unconfigured;
  Object? lastError;
  bool refreshing = false;
  Future<GizClawClient>? _reconnecting;

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
      if (connectionState == MobileConnectionState.connected) {
        await _ensureDeviceWorkspace(client: client, serverId: serverId);
      }
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

  Future<void> _ensureDeviceWorkspace({
    required GizClawClient client,
    required String serverId,
  }) async {
    final clientPublicKey = connection.clientPublicKey;
    if (clientPublicKey == null ||
        !await repository.hasWorkflow(serverId, mobileAstWorkflowName)) {
      return;
    }
    try {
      final workspaceName = mobileAstWorkspaceName(clientPublicKey);
      final existingWorkspace = await repository.workspaceDocument(
        serverId,
        workspaceName,
      );
      final refreshNeeded = await DeviceWorkspaceProvisioner.forClient(client)
          .ensureMobileAstWorkspace(
            clientPublicKey,
            existingWorkspace: existingWorkspace,
          );
      if (refreshNeeded) {
        await refresh(client: client, serverId: serverId);
      }
    } catch (error) {
      lastError = error;
      assert(() {
        debugPrint('GizClaw device workspace ensure failed: $error');
        return true;
      }());
      notifyListeners();
    }
  }

  Future<void> _watchServer(String serverId) async {
    activeServerId = serverId;
    await _workflowSubscription?.cancel();
    await _workspaceSubscription?.cancel();
    await _friendChatSubscription?.cancel();
    await _friendGroupChatSubscription?.cancel();
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
    _friendChatSubscription = repository.watchFriendChats(serverId).listen((
      value,
    ) {
      _friendChats = value;
      _updateChatroomWorkspaces();
    });
    _friendGroupChatSubscription = repository
        .watchFriendGroupChats(serverId)
        .listen((value) {
          _friendGroupChats = value;
          _updateChatroomWorkspaces();
        });
  }

  void _updateChatroomWorkspaces() {
    chatroomWorkspaces = [..._friendChats, ..._friendGroupChats];
    notifyListeners();
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

  Future<T> runRpc<T>(Future<T> Function(GizClawClient client) request) async {
    final client = connection.client;
    if (connectionState != MobileConnectionState.connected || client == null) {
      throw StateError('Connect to GizClaw before sending an RPC request');
    }
    try {
      return await request(client);
    } catch (error) {
      if (!_isRecoverableTransportError(error)) rethrow;
      final reconnected = await _reconnect();
      return request(reconnected);
    }
  }

  Future<GizClawClient> _reconnect() {
    final active = _reconnecting;
    if (active != null) return active;
    final reconnecting = _performReconnect();
    _reconnecting = reconnecting;
    unawaited(
      reconnecting.then<void>(
        (_) => _clearReconnect(reconnecting),
        onError: (_, _) => _clearReconnect(reconnecting),
      ),
    );
    return reconnecting;
  }

  void _clearReconnect(Future<GizClawClient> reconnecting) {
    if (identical(_reconnecting, reconnecting)) _reconnecting = null;
  }

  Future<GizClawClient> _performReconnect() async {
    connectionState = MobileConnectionState.connecting;
    notifyListeners();
    try {
      final client = await connection.reconnect();
      connectionState = MobileConnectionState.connected;
      lastError = null;
      notifyListeners();
      return client;
    } catch (error) {
      connectionState = MobileConnectionState.offline;
      lastError = error;
      notifyListeners();
      rethrow;
    }
  }

  GizClawClient _friendClient() {
    final client = connection.client;
    if (connectionState != MobileConnectionState.connected || client == null) {
      throw StateError('Connect to GizClaw to manage friends');
    }
    return client;
  }

  Future<FriendInviteTokenGetResponse> getFriendInviteToken() =>
      _friendClient().getFriendInviteToken();

  Future<FriendInviteTokenCreateResponse> createFriendInviteToken() =>
      _friendClient().createFriendInviteToken();

  Future<void> clearFriendInviteToken() async {
    await _friendClient().clearFriendInviteToken();
  }

  Future<FriendObject> addFriend(String inviteToken) async {
    final response = await _friendClient().addFriend(inviteToken.trim());
    await refresh();
    return response.value;
  }

  Future<void> deleteFriend(String id) async {
    await _friendClient().deleteFriend(id.trim());
    await refresh();
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

  ChatroomWorkspaceMetadata? chatroomWorkspace(String workspaceName) {
    for (final metadata in chatroomWorkspaces) {
      if (metadata.workspaceName == workspaceName) return metadata;
    }
    return null;
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
    unawaited(_friendChatSubscription?.cancel());
    unawaited(_friendGroupChatSubscription?.cancel());
    unawaited(connection.close());
    unawaited(database.close());
    super.dispose();
  }
}

bool _isRecoverableTransportError(Object error) {
  if (error is TimeoutException) return true;
  if (error is! StateError) return false;
  final message = error.toString().toLowerCase();
  return message.contains('webrtc') || message.contains('data channel');
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
