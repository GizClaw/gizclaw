import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:gizclaw/gizclaw.dart';

import '../connection/gizclaw_connection_controller.dart';
import '../identity/app_identity_store.dart';
import '../prototype/prototype_data.dart';
import '../prototype/prototype_models.dart';
import 'database/app_database.dart';
import 'device_workspace_provisioner.dart';
import 'repositories/mobile_data_repository.dart';
import 'repositories/workspace_chat_repository.dart';
import 'workspace_chat_controller.dart';

enum MobileConnectionState { unconfigured, connecting, connected, offline }

enum MobileWorkspaceSurface { raid, friend, group, pet }

Future<T> _workspaceActivationStep<T>(
  String step,
  Future<T> Function() action,
) async {
  try {
    return await action();
  } catch (error) {
    debugPrint('Workspace activation failed at $step: $error');
    throw StateError('$step: $error');
  }
}

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
    GizClawConnectionController? connectionController,
    MobileDataRepository? dataRepository,
    DeviceInfo? deviceInfo,
    List<GizClawServer>? servers,
    this.identityStore,
  }) : database = database ?? AppDatabase(),
       connection =
           connectionController ??
           GizClawConnectionController(
             profile ?? GizClawConnectionProfile.fromEnvironment(),
             deviceInfo: deviceInfo,
           ),
       _servers = List.unmodifiable(
         _mergeServers(
           servers ?? gizClawPresetServers,
           profile?.endpoint ?? connectionController?.profile.endpoint ?? '',
         ),
       ) {
    repository = dataRepository ?? MobileDataRepository(this.database);
  }

  factory MobileDataController.demo({AppDatabase? database}) {
    final controller = _DemoMobileDataController(database: database);
    controller.workflows = allWorkflows;
    controller.workspaces = workflowWorkspaces;
    controller.chatroomWorkspaces = chatroomWorkspaceMetadata;
    return controller;
  }

  final AppDatabase database;
  final AppIdentityStore? identityStore;
  final GizClawConnectionController connection;
  List<GizClawServer> _servers;
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
  final Set<Future<void>> _startsInFlight = {};
  Future<GizClawClient>? _reconnecting;
  Future<void>? _refreshInFlight;
  bool _refreshAgain = false;
  GizClawClient? _pendingRefreshClient;
  String? _pendingRefreshEndpoint;
  String? _pendingRefreshServerId;
  int _serverWatchGeneration = 0;
  int _startGeneration = 0;
  Future<void>? _workspaceSwitch;
  WorkspaceChatController? _activeWorkspaceChat;
  Future<void>? _closeFuture;
  bool _closing = false;
  bool _disposed = false;
  final Map<String, ({String title, String workspaceName})> _petRouteContexts =
      {};
  PeerRunWorkspaceState? runWorkspaceState;
  Workspace? activeWorkspaceDocument;

  WorkspaceChatController? get activeWorkspaceChat => _activeWorkspaceChat;
  String? get activeWorkspaceName {
    final name = runWorkspaceState?.activeWorkspaceName.trim() ?? '';
    return name.isEmpty ? null : name;
  }

  String get serverEndpoint => connection.profile.endpoint;
  List<GizClawServer> get servers => _servers;
  GizClawServer? get activeServer {
    final endpoint = serverEndpoint;
    for (final server in _servers) {
      if (server.accessPoint == endpoint) return server;
    }
    return null;
  }

  bool get hasActiveServer => activeServer != null;
  String? get clientPublicKey => connection.clientPublicKey;

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

  Future<void> start() {
    if (_closing) return Future<void>.value();
    late final Future<void> trackedStart;
    trackedStart = _start().whenComplete(() {
      _startsInFlight.remove(trackedStart);
    });
    _startsInFlight.add(trackedStart);
    return trackedStart;
  }

  Future<void> _start() async {
    final generation = ++_startGeneration;
    final endpoint = connection.profile.endpoint;
    if (!connection.profile.isConfigured) {
      connectionState = MobileConnectionState.unconfigured;
      notifyListeners();
      return;
    }
    connectionState = MobileConnectionState.connecting;
    notifyListeners();
    final cachedServerId = await repository.serverIdForEndpoint(endpoint);
    if (!_isCurrentStart(generation, endpoint)) return;
    if (cachedServerId != null) await _watchServer(cachedServerId);
    if (!_isCurrentStart(generation, endpoint)) return;
    try {
      final client = await connection.connect();
      if (!_isCurrentStart(generation, endpoint)) return;
      final serverId = connection.serverId!;
      if (serverId != cachedServerId) await _watchServer(serverId);
      if (!_isCurrentStart(generation, endpoint)) return;
      connectionState = MobileConnectionState.connected;
      notifyListeners();
      await refresh(client: client, serverId: serverId);
      if (_isCurrentStart(generation, endpoint) &&
          connectionState == MobileConnectionState.connected) {
        await _ensureDeviceWorkspace(client: client, serverId: serverId);
      }
    } catch (error) {
      if (!_isCurrentStart(generation, endpoint)) return;
      final discoveredServerId = connection.serverId;
      if (cachedServerId == null && discoveredServerId != null) {
        await _watchServer(discoveredServerId);
      }
      lastError = error;
      assert(() {
        debugPrint('GizClaw connection failed: $error');
        return true;
      }());
      connectionState = connection.isConnected
          ? MobileConnectionState.connected
          : MobileConnectionState.offline;
      notifyListeners();
    }
  }

  Future<void> updateServerEndpoint(String endpoint) async {
    final normalized = normalizeGizClawEndpoint(endpoint);
    if (!_servers.any((server) => server.accessPoint == normalized)) {
      final nextServers = List<GizClawServer>.unmodifiable([
        ..._servers,
        GizClawServer(name: normalized, accessPoint: normalized),
      ]);
      await identityStore?.saveCustomServers(_customServers(nextServers));
      _servers = nextServers;
      notifyListeners();
    }
    await _activateServerEndpoint(normalized);
  }

  Future<void> addServer({
    required String name,
    required String accessPoint,
  }) async {
    final normalizedName = name.trim();
    if (normalizedName.isEmpty) {
      throw const FormatException('Enter a server name');
    }
    final normalizedEndpoint = normalizeGizClawEndpoint(accessPoint);
    if (_servers.any((server) => server.accessPoint == normalizedEndpoint)) {
      throw const FormatException('This access point is already in the list');
    }
    final server = GizClawServer(
      name: normalizedName,
      accessPoint: normalizedEndpoint,
    );
    final nextServers = List<GizClawServer>.unmodifiable([..._servers, server]);
    await identityStore?.saveCustomServers(_customServers(nextServers));
    _servers = nextServers;
    notifyListeners();
    await _activateServerEndpoint(normalizedEndpoint);
  }

  Future<void> selectServer(GizClawServer server) async {
    if (!_servers.any(
      (candidate) => candidate.accessPoint == server.accessPoint,
    )) {
      throw ArgumentError.value(server.accessPoint, 'server', 'Unknown server');
    }
    await _activateServerEndpoint(server.accessPoint);
  }

  Future<void> _activateServerEndpoint(String endpoint) async {
    final normalized = normalizeGizClawEndpoint(endpoint);
    if (normalized == connection.profile.endpoint) return;
    _startGeneration += 1;
    await identityStore?.saveEndpoint(normalized);
    _replaceActiveWorkspaceChat(null);
    await connection.updateProfile(
      connection.profile.copyWith(endpoint: normalized),
    );
    await _stopWatchingServer();
    activeServerId = null;
    workflows = const [];
    workspaces = const [];
    chatroomWorkspaces = const [];
    _friendChats = const [];
    _friendGroupChats = const [];
    runWorkspaceState = null;
    activeWorkspaceDocument = null;
    lastError = null;
    unawaited(start());
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
    final generation = ++_serverWatchGeneration;
    activeServerId = serverId;
    await _workflowSubscription?.cancel();
    await _workspaceSubscription?.cancel();
    await _friendChatSubscription?.cancel();
    await _friendGroupChatSubscription?.cancel();
    if (generation != _serverWatchGeneration) return;
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

  Future<void> _stopWatchingServer() async {
    _serverWatchGeneration += 1;
    await _workflowSubscription?.cancel();
    await _workspaceSubscription?.cancel();
    await _friendChatSubscription?.cancel();
    await _friendGroupChatSubscription?.cancel();
    _workflowSubscription = null;
    _workspaceSubscription = null;
    _friendChatSubscription = null;
    _friendGroupChatSubscription = null;
  }

  void _updateChatroomWorkspaces() {
    chatroomWorkspaces = [..._friendChats, ..._friendGroupChats];
    notifyListeners();
  }

  Future<void> refresh({GizClawClient? client, String? serverId}) {
    if (_closing) return Future<void>.value();
    final activeClient = client ?? connection.client;
    final resolvedServerId = serverId ?? connection.serverId;
    if (activeClient == null || resolvedServerId == null) {
      return Future<void>.value();
    }
    _pendingRefreshClient = activeClient;
    _pendingRefreshEndpoint = connection.profile.endpoint;
    _pendingRefreshServerId = resolvedServerId;
    _refreshAgain = true;
    final inFlight = _refreshInFlight;
    if (inFlight != null) return inFlight;
    final refresh = _drainRefreshes();
    _refreshInFlight = refresh;
    return refresh;
  }

  Future<void> _drainRefreshes() async {
    refreshing = true;
    lastError = null;
    notifyListeners();
    try {
      do {
        _refreshAgain = false;
        final client = _pendingRefreshClient!;
        final endpoint = _pendingRefreshEndpoint!;
        final serverId = _pendingRefreshServerId!;
        lastError = null;
        try {
          final warnings = await repository.refresh(
            client: client,
            endpoint: endpoint,
            serverId: serverId,
          );
          if (connection.profile.endpoint != endpoint ||
              !identical(connection.client, client)) {
            continue;
          }
          await _syncRunWorkspace(client);
          connectionState = MobileConnectionState.connected;
          if (warnings.isNotEmpty) {
            lastError = warnings.first;
            assert(() {
              for (final warning in warnings) {
                debugPrint('GizClaw partial refresh: $warning');
              }
              return true;
            }());
          }
        } catch (error) {
          if (connection.profile.endpoint != endpoint ||
              !identical(connection.client, client)) {
            continue;
          }
          lastError = error;
          assert(() {
            debugPrint('GizClaw refresh failed: $error');
            return true;
          }());
          connectionState = connection.isConnected
              ? MobileConnectionState.connected
              : MobileConnectionState.offline;
        }
      } while (_refreshAgain);
    } finally {
      refreshing = false;
      _refreshInFlight = null;
      notifyListeners();
    }
  }

  Future<T> runRpc<T>(
    Future<T> Function(GizClawClient client) request, {
    bool retryOnTransportError = false,
  }) async {
    final client = connection.client;
    if (connectionState != MobileConnectionState.connected || client == null) {
      throw StateError('Connect to GizClaw before sending an RPC request');
    }
    return runRpcWithTransportRecovery(
      initialTransport: client,
      request: request,
      reconnect: _reconnect,
      retryOnTransportError: retryOnTransportError,
    );
  }

  Future<void> recoverTransport() async {
    await _reconnect();
  }

  Future<GizClawClient> _reconnect() {
    if (_closing) {
      return Future<GizClawClient>.error(
        StateError('Mobile data controller is closed'),
      );
    }
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
      final serverId = connection.serverId;
      if (serverId == null) {
        throw StateError('GizClaw reconnect did not return a server identity');
      }
      lastError = null;
      if (serverId != activeServerId) {
        await _watchServer(serverId);
        await refresh(client: client, serverId: serverId);
      } else {
        await _syncRunWorkspace(client);
      }
      connectionState = MobileConnectionState.connected;
      notifyListeners();
      return client;
    } catch (error) {
      connectionState = connection.isConnected
          ? MobileConnectionState.connected
          : MobileConnectionState.offline;
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

  Future<Workspace> createWorkspace({
    required WorkflowDriverKind driver,
    required String workflowName,
    required String name,
  }) async {
    final normalizedWorkflow = workflowName.trim();
    final normalizedName = name.trim();
    if (normalizedWorkflow.isEmpty) {
      throw ArgumentError.value(workflowName, 'workflowName', 'is required');
    }
    if (normalizedName.isEmpty) {
      throw ArgumentError.value(name, 'name', 'is required');
    }
    if (normalizedName.length > 80) {
      throw ArgumentError.value(name, 'name', 'must be at most 80 characters');
    }
    final workspaceName = _newWorkspaceName(normalizedName);
    final response = await runRpc(
      (client) => client.createWorkspace(
        Workspace(
          name: workspaceName,
          workflowName: normalizedWorkflow,
          parameters: newWorkspaceParametersForDriver(driver),
        ),
      ),
    );
    await refresh();
    return response.value;
  }

  bool _isCurrentStart(int generation, String endpoint) {
    return !_closing &&
        generation == _startGeneration &&
        endpoint == connection.profile.endpoint;
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
      try {
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
      } catch (_) {
        // Pet discovery is optional when routing an ordinary workspace.
      }
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
    if (_closing) {
      return Future<WorkspaceChatController>.error(
        StateError('Mobile data controller is closed'),
      );
    }
    final completer = Completer<WorkspaceChatController>();
    final previousSwitch = _workspaceSwitch;
    _workspaceSwitch = (previousSwitch ?? Future<void>.value()).then((_) async {
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
    final selected = await _workspaceActivationStep(
      'select workspace',
      () => client.setRunWorkspace(workspaceName),
    );
    runWorkspaceState = selected.value;
    notifyListeners();
    await _workspaceActivationStep(
      'load workspace',
      () => _loadActiveWorkspaceDocument(client, workspaceName: workspaceName),
    );
    await _workspaceActivationStep(
      'update workspace parameters',
      () => _ensureActiveWorkspaceParameters(
        client,
        workspaceName: workspaceName,
      ),
    );
    final reloaded = await _workspaceActivationStep(
      'reload workspace runtime',
      client.reloadRunWorkspace,
    );
    runWorkspaceState = reloaded.value;
    await _workspaceActivationStep(
      'load active workspace',
      () => _loadActiveWorkspaceDocument(client),
    );
    return _installActiveWorkspaceChat(workspaceName);
  }

  Future<void> _syncRunWorkspace(GizClawClient client) async {
    var state = (await client.getRunWorkspace()).value;
    final workspaceName = state.activeWorkspaceName.trim();
    runWorkspaceState = state;
    await _loadActiveWorkspaceDocument(client);
    final repaired = await _ensureActiveWorkspaceParameters(client);
    if (workspaceName.isNotEmpty &&
        (_runWorkspaceNeedsReload(state) || repaired)) {
      state = (await client.reloadRunWorkspace()).value;
      runWorkspaceState = state;
      await _loadActiveWorkspaceDocument(client);
    }
    if (workspaceName.isEmpty) {
      _replaceActiveWorkspaceChat(null);
      notifyListeners();
      return;
    }
    await _installActiveWorkspaceChat(workspaceName);
  }

  Future<void> _loadActiveWorkspaceDocument(
    GizClawClient client, {
    String? workspaceName,
  }) async {
    final resolvedWorkspaceName = workspaceName ?? activeWorkspaceName;
    if (resolvedWorkspaceName == null) {
      activeWorkspaceDocument = null;
      return;
    }
    activeWorkspaceDocument = (await client.getWorkspace(
      resolvedWorkspaceName,
    )).value;
  }

  Future<bool> _ensureActiveWorkspaceParameters(
    GizClawClient client, {
    String? workspaceName,
  }) async {
    final workspace = activeWorkspaceDocument;
    final resolvedWorkspaceName = workspaceName ?? activeWorkspaceName;
    if (workspace == null || resolvedWorkspaceName == null) {
      return false;
    }
    final driver = await _driverForWorkspace(workspace);
    final updated = workspaceWithDefaultInputParameters(workspace, driver);
    if (updated == null) return false;
    await client.putWorkspace(resolvedWorkspaceName, updated);
    await _loadActiveWorkspaceDocument(
      client,
      workspaceName: resolvedWorkspaceName,
    );
    return true;
  }

  Future<WorkflowDriverKind> _driverForWorkspace(Workspace workspace) async {
    final cached = workflow(workspace.workflowName).driver;
    if (cached != WorkflowDriverKind.unsupported) return cached;
    final serverId = activeServerId;
    if (serverId == null) return cached;
    return (await repository.workflowCard(
          serverId,
          workspace.workflowName,
        ))?.driver ??
        cached;
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
    final currentChat = _activeWorkspaceChat;
    if (currentChat != null &&
        (currentChat.recording || currentChat.startingInput)) {
      await currentChat.finishInput();
    }
    final updated =
        workspaceWithDefaultInputParameters(
          workspace,
          await _driverForWorkspace(workspace),
        ) ??
        workspace.deepCopy();
    _setWorkspaceInputMode(updated, mode);
    await client.putWorkspace(workspaceName, updated);
    await _loadActiveWorkspaceDocument(client);
    if (_workspaceInputMode(activeWorkspaceDocument) != mode) {
      throw StateError('GizClaw did not persist the requested input mode');
    }
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

  Future<void> close() {
    final close = _closeFuture;
    if (close != null) return close;
    _closing = true;
    return _closeFuture = _close();
  }

  Future<void> _close() async {
    _startGeneration += 1;
    _serverWatchGeneration += 1;
    _refreshAgain = false;

    Object? firstError;
    StackTrace? firstStackTrace;
    Future<void> attempt(Future<void> Function() action) async {
      try {
        await action();
      } catch (error, stackTrace) {
        firstError ??= error;
        firstStackTrace ??= stackTrace;
      }
    }

    final starts = _startsInFlight.toList();
    final reconnect = _reconnecting;
    final refresh = _refreshInFlight;
    final workspaceSwitch = _workspaceSwitch;
    await Future.wait([
      for (final start in starts) attempt(() => start),
      if (reconnect != null) attempt(() async => await reconnect),
      if (refresh != null) attempt(() => refresh),
      if (workspaceSwitch != null) attempt(() => workspaceSwitch),
    ]);

    final chat = _activeWorkspaceChat;
    _activeWorkspaceChat = null;
    final subscriptions = <StreamSubscription<dynamic>?>[
      _workflowSubscription,
      _workspaceSubscription,
      _friendChatSubscription,
      _friendGroupChatSubscription,
    ];
    _workflowSubscription = null;
    _workspaceSubscription = null;
    _friendChatSubscription = null;
    _friendGroupChatSubscription = null;

    await Future.wait([
      if (chat != null) attempt(chat.close),
      for (final subscription in subscriptions)
        if (subscription != null) attempt(subscription.cancel),
    ]);
    await attempt(connection.close);
    await attempt(database.close);
    if (firstError case final error?) {
      Error.throwWithStackTrace(error, firstStackTrace!);
    }
  }

  @override
  void notifyListeners() {
    if (_disposed) return;
    super.notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    unawaited(close());
    super.dispose();
  }
}

String _newWorkspaceName(String workflowName) {
  final normalized = workflowName
      .toLowerCase()
      .replaceAll(RegExp(r'[^a-z0-9]+'), '-')
      .replaceAll(RegExp(r'^-+|-+$'), '');
  final validPrefix = normalized.isEmpty
      ? 'workspace'
      : RegExp(r'^[a-z]').hasMatch(normalized)
      ? normalized
      : 'workspace-$normalized';
  final prefix = validPrefix.length > 32
      ? validPrefix.substring(0, 32)
      : validPrefix;
  final suffix = DateTime.now().toUtc().microsecondsSinceEpoch.toRadixString(
    36,
  );
  return '$prefix-$suffix';
}

@visibleForTesting
WorkspaceParameters newWorkspaceParametersForDriver(
  WorkflowDriverKind driver,
) => switch (driver) {
  WorkflowDriverKind.flowcraft => WorkspaceParameters(
    flowcraftWorkspaceParameters: FlowcraftWorkspaceParameters(
      agentType: FlowcraftWorkspaceParametersAgentType
          .FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_FLOWCRAFT,
      input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    ),
  ),
  WorkflowDriverKind.doubaoRealtime => WorkspaceParameters(
    doubaoRealtimeWorkspaceParameters: DoubaoRealtimeWorkspaceParameters(
      agentType: DoubaoRealtimeWorkspaceParametersAgentType
          .DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME,
      input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    ),
  ),
  WorkflowDriverKind.astTranslate => WorkspaceParameters(
    asttranslateWorkspaceParameters: ASTTranslateWorkspaceParameters(
      agentType: ASTTranslateWorkspaceParametersAgentType
          .ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE,
      enableSourceLanguageDetect: true,
      input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
      langPair: mobileAstLanguagePair,
      mode: ASTTranslateMode.ASTTRANSLATE_MODE_S2S,
    ),
  ),
  _ => throw UnsupportedError(
    'Creating ${driver.label} workspaces is not supported',
  ),
};

@visibleForTesting
Workspace? workspaceWithDefaultInputParameters(
  Workspace workspace,
  WorkflowDriverKind driver,
) {
  if (_workspaceExposesInputMode(workspace)) return null;
  if (driver != WorkflowDriverKind.flowcraft &&
      driver != WorkflowDriverKind.doubaoRealtime &&
      driver != WorkflowDriverKind.astTranslate) {
    return null;
  }
  return workspace.deepCopy()
    ..parameters = newWorkspaceParametersForDriver(driver);
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
  if (parameters.hasPetWorkspaceParameters()) {
    return parameters.petWorkspaceParameters.input;
  }
  return WorkspaceInputMode.WORKSPACE_INPUT_MODE_UNSPECIFIED;
}

bool _workspaceExposesInputMode(Workspace workspace) {
  if (!workspace.hasParameters()) return false;
  final parameters = workspace.parameters;
  return parameters.hasAsttranslateWorkspaceParameters() ||
      parameters.hasChatRoomWorkspaceParameters() ||
      parameters.hasDoubaoRealtimeWorkspaceParameters() ||
      parameters.hasFlowcraftWorkspaceParameters() ||
      parameters.hasPetWorkspaceParameters();
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
  if (parameters.hasPetWorkspaceParameters()) {
    parameters.petWorkspaceParameters.input = mode;
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

@visibleForTesting
Future<T> runRpcWithTransportRecovery<T, Transport>({
  required Transport initialTransport,
  required Future<T> Function(Transport transport) request,
  required Future<Transport> Function() reconnect,
  required bool retryOnTransportError,
}) async {
  try {
    return await request(initialTransport);
  } catch (error) {
    if (!retryOnTransportError || !_isRecoverableTransportError(error)) {
      rethrow;
    }
    return request(await reconnect());
  }
}

List<GizClawServer> _mergeServers(
  List<GizClawServer> servers,
  String activeEndpoint,
) {
  final merged = <GizClawServer>[];
  final endpoints = <String>{};
  for (final server in [...gizClawPresetServers, ...servers]) {
    final endpoint = normalizeGizClawEndpoint(server.accessPoint);
    if (!endpoints.add(endpoint)) continue;
    merged.add(GizClawServer(name: server.name.trim(), accessPoint: endpoint));
  }
  final trimmedActiveEndpoint = activeEndpoint.trim();
  if (trimmedActiveEndpoint.isNotEmpty) {
    final endpoint = normalizeGizClawEndpoint(trimmedActiveEndpoint);
    if (endpoints.add(endpoint)) {
      merged.add(GizClawServer(name: endpoint, accessPoint: endpoint));
    }
  }
  return merged;
}

List<GizClawServer> _customServers(List<GizClawServer> servers) {
  final presetEndpoints = {
    for (final server in gizClawPresetServers) server.accessPoint,
  };
  return [
    for (final server in servers)
      if (!presetEndpoints.contains(server.accessPoint)) server,
  ];
}

class _DemoMobileDataController extends MobileDataController {
  _DemoMobileDataController({AppDatabase? database})
    : super(
        database: database,
        profile: const GizClawConnectionProfile(
          endpoint: gizClawDevelopmentServerEndpoint,
          clientPrivateKey: 'demo-private-key',
        ),
        servers: gizClawPresetServers,
      );

  @override
  Future<void> start() async {
    connectionState = MobileConnectionState.offline;
    notifyListeners();
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
