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

enum MobileWorkspaceSurface { raid, friend, group, pet }

class MobileWorkspaceDestination {
  const MobileWorkspaceDestination({
    required this.surface,
    required this.workspaceName,
    this.resourceId,
    this.driver,
  }) : assert(
         surface != MobileWorkspaceSurface.pet || resourceId != null,
         'Pet destinations require a resource ID',
       ),
       assert(
         surface != MobileWorkspaceSurface.raid || driver != null,
         'Raid destinations require a driver',
       );

  final WorkflowDriverKind? driver;
  final String? resourceId;
  final MobileWorkspaceSurface surface;
  final String workspaceName;

  String get route => switch (surface) {
    MobileWorkspaceSurface.friend =>
      '/raids/drivers/chatroom/${Uri.encodeComponent(workspaceName)}',
    MobileWorkspaceSurface.group =>
      '/groups/${Uri.encodeComponent(workspaceName)}',
    MobileWorkspaceSurface.pet => '/pets/${Uri.encodeComponent(resourceId!)}',
    MobileWorkspaceSurface.raid =>
      '/raids/drivers/${driver!.routeKey}/'
          '${Uri.encodeComponent(workspaceName)}',
  };
}

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
  Future<void> _workspaceSwitch = Future<void>.value();
  WorkspaceChatController? _activeWorkspaceChat;
  final Map<String, ({String title, String workspaceName})> _petRouteContexts =
      {};
  PeerRunWorkspaceState? runWorkspaceState;
  Workspace? activeWorkspaceDocument;

  WorkspaceChatController? get activeWorkspaceChat => _activeWorkspaceChat;
  String? get activeWorkspaceName {
    final name = runWorkspaceState?.activeWorkspaceName.trim() ?? '';
    return name.isEmpty ? null : name;
  }

  WorkspaceInputMode get activeInputMode =>
      _workspaceInputMode(activeWorkspaceDocument);

  ({String title, String workspaceName})? petRouteContext(String petId) =>
      _petRouteContexts[petId];

  void rememberPetRouteContext({
    required String petId,
    required String title,
    required String workspaceName,
  }) {
    final next = (title: title, workspaceName: workspaceName);
    if (_petRouteContexts[petId] == next) return;
    _petRouteContexts[petId] = next;
    notifyListeners();
  }

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
      await _syncRunWorkspace(activeClient);
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

  Future<void> recoverTransport() async {
    await _reconnect();
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
    _replaceActiveWorkspaceChat(null);
    connectionState = MobileConnectionState.connecting;
    notifyListeners();
    try {
      final client = await connection.reconnect();
      lastError = null;
      await _syncRunWorkspace(client);
      connectionState = MobileConnectionState.connected;
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

  Future<FriendGroupObject> createFriendGroup({
    required String name,
    String description = '',
  }) async {
    final response = await _friendClient().createFriendGroup(
      name: name.trim(),
      description: description.trim(),
    );
    await refresh();
    return response.value;
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

  Future<String> routeForWorkspace(String workspaceName) async {
    return (await destinationForWorkspace(workspaceName)).route;
  }

  Future<MobileWorkspaceDestination> destinationForWorkspace(
    String workspaceName,
  ) async {
    final cached = cachedDestinationForWorkspace(workspaceName);
    if (cached != null) return cached;
    final client = connection.client;
    if (client != null) {
      String? cursor;
      do {
        final response = await client.listPets(cursor: cursor, limit: 100);
        for (final pet in response.value.items) {
          if (pet.workspaceName == workspaceName) {
            return MobileWorkspaceDestination(
              surface: MobileWorkspaceSurface.pet,
              workspaceName: workspaceName,
              resourceId: pet.id,
            );
          }
        }
        cursor = response.value.hasNext ? response.value.nextCursor : null;
      } while (cursor != null && cursor.isNotEmpty);
    }
    final workspace = this.workspace(workspaceName);
    return MobileWorkspaceDestination(
      surface: MobileWorkspaceSurface.raid,
      workspaceName: workspaceName,
      driver: workflow(workspace.workflowName).driver,
    );
  }

  MobileWorkspaceDestination? cachedDestinationForWorkspace(
    String workspaceName,
  ) {
    final chatroom = chatroomWorkspace(workspaceName);
    if (chatroom != null) {
      return MobileWorkspaceDestination(
        surface: chatroom.kind == ChatroomWorkspaceKind.group
            ? MobileWorkspaceSurface.group
            : MobileWorkspaceSurface.friend,
        workspaceName: workspaceName,
      );
    }
    for (final entry in _petRouteContexts.entries) {
      if (entry.value.workspaceName == workspaceName) {
        return MobileWorkspaceDestination(
          surface: MobileWorkspaceSurface.pet,
          workspaceName: workspaceName,
          resourceId: entry.key,
        );
      }
    }
    return null;
  }

  Future<WorkspaceChatController> activateWorkspaceChat(String workspaceName) {
    final completer = Completer<WorkspaceChatController>();
    _workspaceSwitch = _workspaceSwitch.then((_) async {
      try {
        final current = _activeWorkspaceChat;
        if (current != null && current.workspaceName == workspaceName) {
          completer.complete(current);
          return;
        }
        completer.complete(await _activateWorkspaceChatNow(workspaceName));
      } catch (error, stackTrace) {
        completer.completeError(error, stackTrace);
      }
    });
    return completer.future;
  }

  Future<WorkspaceChatController> _activateWorkspaceChatNow(
    String workspaceName,
  ) async {
    final client = connection.client;
    if (client == null) {
      throw StateError('Connect to GizClaw before switching workspace');
    }
    final selected = await client.setRunWorkspace(workspaceName);
    runWorkspaceState = selected.value;
    notifyListeners();
    final reloaded = await client.reloadRunWorkspace();
    runWorkspaceState = reloaded.value;
    await _loadActiveWorkspaceDocument(client);
    return _installActiveWorkspaceChat(workspaceName);
  }

  Future<void> _syncRunWorkspace(GizClawClient client) async {
    var state = (await client.getRunWorkspace()).value;
    final workspaceName = state.activeWorkspaceName.trim();
    if (workspaceName.isNotEmpty && _runWorkspaceNeedsReload(state)) {
      state = (await client.reloadRunWorkspace()).value;
    }
    runWorkspaceState = state;
    await _loadActiveWorkspaceDocument(client);
    if (workspaceName.isEmpty) {
      _replaceActiveWorkspaceChat(null);
      notifyListeners();
      return;
    }
    await _installActiveWorkspaceChat(workspaceName);
  }

  Future<void> _loadActiveWorkspaceDocument(GizClawClient client) async {
    final workspaceName = activeWorkspaceName;
    if (workspaceName == null) {
      activeWorkspaceDocument = null;
      return;
    }
    activeWorkspaceDocument = (await client.getWorkspace(workspaceName)).value;
  }

  Future<WorkspaceChatController> _installActiveWorkspaceChat(
    String workspaceName,
  ) async {
    final current = _activeWorkspaceChat;
    if (current != null && current.workspaceName == workspaceName) {
      notifyListeners();
      return current;
    }
    _replaceActiveWorkspaceChat(null);
    final chat = WorkspaceChatController(
      workspaceName: workspaceName,
      repository: workspaceChatRepository,
      serverId: activeServerId,
      client: connection.client,
      dataChannelFactory: connection.dataChannelFactory,
      peerConnection: connection.peerConnection,
      onTransportClosed: recoverTransport,
    );
    _replaceActiveWorkspaceChat(chat);
    await chat.start(activate: false);
    notifyListeners();
    return chat;
  }

  void releaseWorkspaceChat(WorkspaceChatController? chat) {
    // The active conversation belongs to the app, not to an individual page.
  }

  Future<void> setActiveInputMode(WorkspaceInputMode mode) async {
    final client = connection.client;
    final workspace = activeWorkspaceDocument;
    final workspaceName = activeWorkspaceName;
    if (client == null || workspace == null || workspaceName == null) {
      throw StateError('No active workspace is available');
    }
    if (_workspaceInputMode(workspace) == mode) return;
    final updated = workspace.deepCopy();
    _setWorkspaceInputMode(updated, mode);
    activeWorkspaceDocument = (await client.putWorkspace(
      workspaceName,
      updated,
    )).value;
    final reloaded = await client.reloadRunWorkspace();
    runWorkspaceState = reloaded.value;
    _replaceActiveWorkspaceChat(null);
    await _installActiveWorkspaceChat(workspaceName);
    notifyListeners();
  }

  void _replaceActiveWorkspaceChat(WorkspaceChatController? chat) {
    if (identical(chat, _activeWorkspaceChat)) return;
    _activeWorkspaceChat?.dispose();
    _activeWorkspaceChat = chat;
  }

  @override
  void dispose() {
    _replaceActiveWorkspaceChat(null);
    unawaited(_workflowSubscription?.cancel());
    unawaited(_workspaceSubscription?.cancel());
    unawaited(_friendChatSubscription?.cancel());
    unawaited(_friendGroupChatSubscription?.cancel());
    unawaited(connection.close());
    unawaited(database.close());
    super.dispose();
  }
}

bool _runWorkspaceNeedsReload(PeerRunWorkspaceState state) {
  return state.runtimeState ==
          PeerRunStatusState.PEER_RUN_STATUS_STATE_UNSPECIFIED ||
      state.runtimeState == PeerRunStatusState.PEER_RUN_STATUS_STATE_STOPPED ||
      state.runtimeState == PeerRunStatusState.PEER_RUN_STATUS_STATE_STOPPING ||
      state.runtimeState == PeerRunStatusState.PEER_RUN_STATUS_STATE_ERROR;
}

WorkspaceInputMode _workspaceInputMode(Workspace? workspace) {
  if (workspace == null || !workspace.hasParameters()) {
    return WorkspaceInputMode.WORKSPACE_INPUT_MODE_UNSPECIFIED;
  }
  final parameters = workspace.parameters;
  if (parameters.hasAsttranslateWorkspaceParameters()) {
    return parameters.asttranslateWorkspaceParameters.input;
  }
  if (parameters.hasChatRoomWorkspaceParameters()) {
    return parameters.chatRoomWorkspaceParameters.input;
  }
  if (parameters.hasDoubaoRealtimeWorkspaceParameters()) {
    return parameters.doubaoRealtimeWorkspaceParameters.input;
  }
  if (parameters.hasFlowcraftWorkspaceParameters()) {
    return parameters.flowcraftWorkspaceParameters.input;
  }
  return WorkspaceInputMode.WORKSPACE_INPUT_MODE_UNSPECIFIED;
}

void _setWorkspaceInputMode(Workspace workspace, WorkspaceInputMode mode) {
  final parameters = workspace.parameters;
  if (parameters.hasAsttranslateWorkspaceParameters()) {
    parameters.asttranslateWorkspaceParameters.input = mode;
    return;
  }
  if (parameters.hasChatRoomWorkspaceParameters()) {
    parameters.chatRoomWorkspaceParameters.input = mode;
    return;
  }
  if (parameters.hasDoubaoRealtimeWorkspaceParameters()) {
    parameters.doubaoRealtimeWorkspaceParameters.input = mode;
    return;
  }
  if (parameters.hasFlowcraftWorkspaceParameters()) {
    parameters.flowcraftWorkspaceParameters.input = mode;
    return;
  }
  throw StateError('The active workspace does not expose an input mode');
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
