import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:gizclaw/gizclaw.dart';

import '../audio/pcm_audio_level_source.dart';
import '../connection/gizclaw_connection_controller.dart';
import '../identity/app_identity_store.dart';
import '../l10n/locale_resolution.dart';
import '../prototype/prototype_data.dart';
import '../prototype/prototype_models.dart';
import '../workflows/app_workflow_catalog.dart';
import 'database/app_database.dart';
import 'repositories/mobile_data_repository.dart';
import 'repositories/workspace_chat_repository.dart';
import 'workspace_chat_controller.dart';

enum MobileConnectionState { unconfigured, connecting, connected, offline }

enum MobileConnectionFailureKind { generic, localNetwork }

enum MobileWorkspaceSurface { raid, friend, group, pet }

const _demoServerEndpoint = 'demo.gizclaw.local:9820';

String localizedPeerEventErrorMessage(PeerStreamEvent event, Locale locale) {
  final zh = locale.languageCode == 'zh';
  final serverMessage = event.errorMessage?.trim() ?? '';
  return switch (event.errorCode) {
    'CHATROOM_FRIEND_REMOVED' => zh ? '你们已不是好友。' : 'You are no longer friends.',
    'CHATROOM_MEMBER_REMOVED' =>
      zh ? '你已被移出这个群聊。' : 'You were removed from this group.',
    'CHATROOM_GROUP_DELETED' => zh ? '这个群聊已被删除。' : 'This group was deleted.',
    'CHATROOM_ACCESS_CHECK_FAILED' =>
      zh
          ? '暂时无法确认群聊权限，请重试。'
          : 'Chatroom access could not be verified. Try again.',
    _ =>
      serverMessage.isEmpty
          ? (event.errorCode?.trim().isNotEmpty == true
                ? event.errorCode!
                : (zh ? '工作区请求失败。' : 'Workspace request failed'))
          : serverMessage,
  };
}

Future<T> _workspaceActivationStep<T>(
  String step,
  Future<T> Function() action,
) async {
  try {
    return await action();
  } catch (error, stackTrace) {
    debugPrint('Workspace activation failed at $step: $error');
    Error.throwWithStackTrace(error, stackTrace);
  }
}

class MobileWorkspaceDestination {
  const MobileWorkspaceDestination({
    required this.surface,
    required this.workspaceName,
    this.collection,
    this.resourceId,
  }) : assert(
         surface != MobileWorkspaceSurface.pet || resourceId != null,
         'Pet destinations require a resource ID',
       );

  final String? resourceId;
  final String? collection;
  final MobileWorkspaceSurface surface;
  final String workspaceName;

  String get route => switch (surface) {
    MobileWorkspaceSurface.friend =>
      '/workspaces/${Uri.encodeComponent(workspaceName)}',
    MobileWorkspaceSurface.group =>
      '/groups/${Uri.encodeComponent(workspaceName)}',
    MobileWorkspaceSurface.pet => '/pets/${Uri.encodeComponent(resourceId!)}',
    MobileWorkspaceSurface.raid =>
      '/collections/${collection == null || collection!.isEmpty ? 'assistants' : collection}/${Uri.encodeComponent(workspaceName)}',
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
    this.backgroundReconnectInitialDelay = const Duration(seconds: 1),
    this.backgroundReconnectMaxDelay = const Duration(seconds: 30),
  }) : database = database ?? AppDatabase(),
       assert(!backgroundReconnectInitialDelay.isNegative),
       assert(backgroundReconnectMaxDelay >= backgroundReconnectInitialDelay),
       _backgroundReconnectDelay = backgroundReconnectInitialDelay,
       connection =
           connectionController ??
           GizClawConnectionController(
             profile ?? GizClawConnectionProfile.fromEnvironment(),
             deviceInfo: deviceInfo,
           ),
       _servers = List.unmodifiable(
         _mergeServers(
           servers ?? const [],
           profile?.endpoint ?? connectionController?.profile.endpoint ?? '',
           profile?.registrationToken ??
               connectionController?.profile.registrationToken ??
               '',
         ),
       ) {
    repository = dataRepository ?? MobileDataRepository(this.database);
    _observedMicrophoneStatus = connection.microphoneStatus;
    connection.addListener(_handleConnectionChanged);
    workflows = const [];
  }

  factory MobileDataController.demo({AppDatabase? database}) {
    final controller = _DemoMobileDataController(database: database);
    controller._workflowResources = demoWorkflows;
    controller._rebuildWorkflowCards();
    controller.workspaces = workflowWorkspaces;
    controller.chatroomWorkspaces = chatroomWorkspaceMetadata;
    return controller;
  }

  final AppDatabase database;
  final AppIdentityStore? identityStore;
  final Duration backgroundReconnectInitialDelay;
  final Duration backgroundReconnectMaxDelay;
  final GizClawConnectionController connection;
  List<GizClawServer> _servers;
  late final MobileDataRepository repository;
  late final WorkspaceChatRepository workspaceChatRepository =
      WorkspaceChatRepository(database);

  StreamSubscription<List<WorkspaceCard>>? _workspaceSubscription;
  StreamSubscription<List<ChatroomWorkspaceMetadata>>? _friendChatSubscription;
  StreamSubscription<List<ChatroomWorkspaceMetadata>>?
  _friendGroupChatSubscription;
  StreamSubscription<PeerStreamEvent>? _peerEventSubscription;
  WorkspaceEventSession? _peerEventSession;
  final Map<String, Future<void>> _historyEventRefreshes = {};
  final Set<String> _historyEventRefreshDirty = {};
  List<WorkflowCard> workflows = const [];
  List<Workflow> _workflowResources = const [];
  String runtimeProfileName = '';
  String runtimeProfileRevision = '';
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
  Future<MicrophoneStatus>? _microphoneRecovery;
  Timer? _backgroundReconnectTimer;
  late Duration _backgroundReconnectDelay;
  bool _backgroundReconnectRequired = false;
  bool _recoverMicrophoneAfterResume = false;
  Future<void>? _refreshInFlight;
  bool _refreshAgain = false;
  GizClawClient? _pendingRefreshClient;
  String? _pendingRefreshEndpoint;
  String? _pendingRefreshServerId;
  Locale _effectiveLocale = appEnglishLocale;
  int _serverWatchGeneration = 0;
  int _startGeneration = 0;
  int _peerEventGeneration = 0;
  Future<void>? _workspaceSwitch;
  WorkspaceChatController? _activeWorkspaceChat;
  Future<void>? _closeFuture;
  bool _closing = false;
  bool _updatingConnectionProfile = false;
  bool _disposed = false;
  late MicrophoneStatus _observedMicrophoneStatus;
  final Map<String, ({String title, String workspaceName})> _petRouteContexts =
      {};
  PeerRunWorkspaceState? runWorkspaceState;
  Workspace? activeWorkspaceDocument;
  String peerName = '';
  String peerEmoji = '';

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
  Locale get effectiveLocale => _effectiveLocale;
  MicrophoneStatus get microphoneStatus => connection.microphoneStatus;
  MobileConnectionFailureKind? get connectionFailureKind =>
      classifyMobileConnectionFailure(
        endpoint: connection.profile.endpoint,
        error: lastError,
        platform: defaultTargetPlatform,
        state: connectionState,
      );

  void _handleConnectionChanged() {
    final previous = _observedMicrophoneStatus;
    final current = connection.microphoneStatus;
    _observedMicrophoneStatus = current;
    final recoverySuppressed = _closing || _updatingConnectionProfile;
    if (!recoverySuppressed &&
        connectionState == MobileConnectionState.connected &&
        previous.availability == MicrophoneAvailability.ready &&
        current.availability == MicrophoneAvailability.unavailable) {
      unawaited(_handleEndedMicrophoneTrack(activeWorkspaceChat));
    }
    if (!recoverySuppressed &&
        connectionState != MobileConnectionState.connecting &&
        !connection.isConnected) {
      connectionState = MobileConnectionState.offline;
      _scheduleReconnect();
    } else if (!recoverySuppressed &&
        connection.isConnected &&
        connection.client != null &&
        connectionState == MobileConnectionState.offline) {
      connectionState = MobileConnectionState.connected;
      _backgroundReconnectTimer?.cancel();
      _backgroundReconnectTimer = null;
      _backgroundReconnectDelay = backgroundReconnectInitialDelay;
    }
    notifyListeners();
  }

  Future<void> _handleEndedMicrophoneTrack(
    WorkspaceChatController? chat,
  ) async {
    if (chat != null && (chat.recording || chat.startingInput)) {
      await chat.finishInput(error: 'microphone_track_ended');
    }
    if (_closing ||
        microphoneStatus.availability != MicrophoneAvailability.unavailable) {
      return;
    }
    try {
      await recoverMicrophone();
    } catch (_) {
      // Reconnect already records the transport error for the UI.
    }
  }

  Future<MicrophoneStatus> recoverMicrophone() {
    return _beginMicrophoneRecovery(force: false);
  }

  Future<MicrophoneStatus> _beginMicrophoneRecovery({required bool force}) {
    final active = _microphoneRecovery;
    if (active != null) return active;
    if (connectionState != MobileConnectionState.connected ||
        (!force &&
            microphoneStatus.availability == MicrophoneAvailability.ready)) {
      return Future.value(microphoneStatus);
    }
    late final Future<MicrophoneStatus> recovery;
    recovery = _recoverMicrophone().whenComplete(() {
      if (identical(_microphoneRecovery, recovery)) {
        _microphoneRecovery = null;
      }
    });
    return _microphoneRecovery = recovery;
  }

  Future<MicrophoneStatus> _recoverMicrophone() async {
    await _reconnect();
    return microphoneStatus;
  }

  void handleAppPaused() {
    if (_closing) return;
    _recoverMicrophoneAfterResume = true;
    _backgroundReconnectDelay = backgroundReconnectInitialDelay;
    if (!kReleaseMode) {
      debugPrint(
        'GizClaw lifecycle: background '
        '(connected=${connection.isConnected})',
      );
    }
    if (connectionState != MobileConnectionState.connected ||
        !connection.isConnected) {
      _scheduleReconnect();
    }
  }

  void handleAppResumed() {
    _backgroundReconnectTimer?.cancel();
    _backgroundReconnectTimer = null;
    _backgroundReconnectDelay = backgroundReconnectInitialDelay;
    final forceTransportRecovery = _backgroundReconnectRequired;
    final force = _recoverMicrophoneAfterResume;
    _recoverMicrophoneAfterResume = false;
    if (!kReleaseMode) {
      debugPrint(
        'GizClaw lifecycle: resumed '
        '(connected=${connection.isConnected}, recoverMicrophone=$force)',
      );
    }
    if (connection.profile.isConfigured &&
        (forceTransportRecovery ||
            connectionState != MobileConnectionState.connected ||
            !connection.isConnected)) {
      unawaited(() async {
        try {
          await _reconnect();
        } catch (_) {}
      }());
      return;
    }
    if (connectionState == MobileConnectionState.connected &&
        (force ||
            microphoneStatus.availability ==
                MicrophoneAvailability.unavailable)) {
      unawaited(() async {
        try {
          await _beginMicrophoneRecovery(force: force);
        } catch (_) {}
      }());
    }
    if (connectionState == MobileConnectionState.connected) {
      unawaited(() async {
        await refresh();
        await activeWorkspaceChat?.refreshHistory();
      }());
    }
  }

  void setEffectiveLocale(Locale locale) {
    final normalized = locale.languageCode == 'zh'
        ? appSimplifiedChineseLocale
        : appEnglishLocale;
    if (_effectiveLocale == normalized) return;
    _effectiveLocale = normalized;
    _rebuildWorkflowCards();
    notifyListeners();
  }

  WorkspaceInputMode get activeInputMode =>
      _workspaceInputMode(activeWorkspaceDocument);

  bool get canSetActiveInputMode {
    final workspace = activeWorkspaceDocument;
    return workspace != null && _workspaceExposesInputMode(workspace);
  }

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
      await _openPeerEventSession();
      await refresh(client: client, serverId: serverId);
      _markTransportReady();
    } catch (error) {
      if (!_isCurrentStart(generation, endpoint)) return;
      final discoveredServerId = connection.serverId;
      if (cachedServerId == null && discoveredServerId != null) {
        await _watchServer(discoveredServerId);
      }
      lastError = error;
      if (!kReleaseMode) {
        debugPrint('GizClaw connection failed: $error');
      }
      connectionState = connection.isConnected
          ? MobileConnectionState.connected
          : MobileConnectionState.offline;
      _scheduleReconnect(force: connection.isConnected);
      notifyListeners();
    }
  }

  Future<void> updateServerEndpoint(String endpoint) async {
    final normalized = _normalizeRequiredServerEndpoint(endpoint);
    GizClawServer? selected;
    for (final server in _servers) {
      if (server.accessPoint == normalized) selected = server;
    }
    if (!_servers.any((server) => server.accessPoint == normalized)) {
      selected = GizClawServer(name: normalized, accessPoint: normalized);
      final nextServers = List<GizClawServer>.unmodifiable([
        ..._servers,
        selected,
      ]);
      await identityStore?.saveCustomServers(nextServers);
      _servers = nextServers;
      notifyListeners();
    }
    await _activateServer(selected!);
  }

  Future<void> addServer({
    required String name,
    required String accessPoint,
    String registrationToken = '',
  }) async {
    final normalizedName = name.trim();
    if (normalizedName.isEmpty) {
      throw const FormatException('Enter a server name');
    }
    final normalizedEndpoint = _normalizeRequiredServerEndpoint(accessPoint);
    if (_servers.any((server) => server.accessPoint == normalizedEndpoint)) {
      throw const FormatException('This access point is already in the list');
    }
    final server = GizClawServer(
      name: normalizedName,
      accessPoint: normalizedEndpoint,
      registrationToken: registrationToken.trim(),
    );
    final nextServers = List<GizClawServer>.unmodifiable([..._servers, server]);
    await identityStore?.saveCustomServers(nextServers);
    _servers = nextServers;
    notifyListeners();
    await _activateServer(server);
  }

  Future<void> addOrSelectServer({
    required String name,
    required String accessPoint,
    String registrationToken = '',
  }) async {
    final normalizedEndpoint = _normalizeRequiredServerEndpoint(accessPoint);
    for (var index = 0; index < _servers.length; index += 1) {
      var server = _servers[index];
      if (server.accessPoint == normalizedEndpoint) {
        final token = registrationToken.trim();
        if (token.isNotEmpty && token != server.registrationToken) {
          server = GizClawServer(
            name: server.name,
            accessPoint: server.accessPoint,
            registrationToken: token,
          );
          final nextServers = [..._servers];
          nextServers[index] = server;
          await identityStore?.saveCustomServers(nextServers);
          _servers = List.unmodifiable(nextServers);
          notifyListeners();
        }
        await selectServer(server);
        return;
      }
    }
    await addServer(
      name: name,
      accessPoint: normalizedEndpoint,
      registrationToken: registrationToken,
    );
  }

  Future<void> selectServer(GizClawServer server) async {
    if (!_servers.any(
      (candidate) => candidate.accessPoint == server.accessPoint,
    )) {
      throw ArgumentError.value(server.accessPoint, 'server', 'Unknown server');
    }
    await _activateServer(server);
  }

  Future<void> _activateServer(GizClawServer server) async {
    final normalized = _normalizeRequiredServerEndpoint(server.accessPoint);
    final registrationToken = server.registrationToken.trim();
    if (normalized == connection.profile.endpoint &&
        registrationToken == connection.profile.registrationToken) {
      return;
    }
    _startGeneration += 1;
    await identityStore?.saveEndpoint(normalized);
    _updatingConnectionProfile = true;
    try {
      await _replaceActiveWorkspaceChat(null);
      await _closePeerEventSession();
      await connection.updateProfile(
        connection.profile.copyWith(
          endpoint: normalized,
          registrationToken: registrationToken,
        ),
      );
    } finally {
      _updatingConnectionProfile = false;
    }
    await _stopWatchingServer();
    activeServerId = null;
    peerName = '';
    peerEmoji = '';
    workflows = const [];
    _workflowResources = const [];
    runtimeProfileName = '';
    runtimeProfileRevision = '';
    workspaces = const [];
    chatroomWorkspaces = const [];
    _friendChats = const [];
    _friendGroupChats = const [];
    runWorkspaceState = null;
    activeWorkspaceDocument = null;
    lastError = null;
    unawaited(start());
  }

  Future<void> _watchServer(String serverId) async {
    final generation = ++_serverWatchGeneration;
    if (activeServerId != serverId) {
      peerName = '';
      peerEmoji = '';
    }
    activeServerId = serverId;
    await _workspaceSubscription?.cancel();
    await _friendChatSubscription?.cancel();
    await _friendGroupChatSubscription?.cancel();
    if (generation != _serverWatchGeneration) return;
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
    await _workspaceSubscription?.cancel();
    await _friendChatSubscription?.cancel();
    await _friendGroupChatSubscription?.cancel();
    _workspaceSubscription = null;
    _friendChatSubscription = null;
    _friendGroupChatSubscription = null;
  }

  Future<void> _openPeerEventSession() async {
    await _closePeerEventSession();
    final factory = connection.dataChannelFactory;
    if (factory == null) return;
    final generation = _peerEventGeneration;
    final client = connection.client;
    final session = await WorkspaceEventSession.open(factory);
    if (_closing ||
        generation != _peerEventGeneration ||
        !connection.isConnected ||
        !identical(connection.client, client)) {
      await session.close();
      return;
    }
    _peerEventSession = session;
    _peerEventSubscription = session.events.listen(
      _handlePeerEvent,
      onError: (Object error) {
        if (_closing || !identical(_peerEventSession, session)) return;
        lastError = error;
        notifyListeners();
      },
      onDone: () {
        if (_closing || !identical(_peerEventSession, session)) return;
        _peerEventGeneration += 1;
        _peerEventSession = null;
        _peerEventSubscription = null;
        unawaited(_closeEndedPeerEventSession(session));
      },
    );
  }

  Future<void> _closeEndedPeerEventSession(
    WorkspaceEventSession session,
  ) async {
    try {
      await session.close();
    } catch (error) {
      if (!_closing) {
        lastError = error;
        notifyListeners();
      }
    }
    if (_closing) return;
    try {
      await recoverTransport();
    } catch (error) {
      if (!_closing) {
        lastError = error;
        notifyListeners();
      }
    }
  }

  void _handlePeerEvent(PeerStreamEvent event) {
    if (_closing) return;
    if (event.type == 'friend.relationship.updated' ||
        event.type == 'friend_group.updated') {
      unawaited(refresh());
      return;
    }
    if (event.type != 'workspace.history.updated') return;
    final workspaceName =
        event.workspaceHistoryUpdated?.workspaceName.trim() ?? '';
    if (workspaceName.isEmpty ||
        activeWorkspaceChat?.workspaceName == workspaceName) {
      return;
    }
    if (_historyEventRefreshes.containsKey(workspaceName)) {
      _historyEventRefreshDirty.add(workspaceName);
      return;
    }
    late final Future<void> refreshHistory;
    refreshHistory = _drainWorkspaceHistoryEvents(workspaceName).whenComplete(
      () {
        if (identical(_historyEventRefreshes[workspaceName], refreshHistory)) {
          _historyEventRefreshes.remove(workspaceName);
          _historyEventRefreshDirty.remove(workspaceName);
        }
      },
    );
    _historyEventRefreshes[workspaceName] = refreshHistory;
  }

  Future<void> _drainWorkspaceHistoryEvents(String workspaceName) async {
    do {
      _historyEventRefreshDirty.remove(workspaceName);
      try {
        await _refreshWorkspaceHistoryFromEvent(workspaceName);
      } catch (error) {
        if (!_closing) {
          lastError = error;
          notifyListeners();
        }
      }
    } while (!_closing && _historyEventRefreshDirty.remove(workspaceName));
  }

  @visibleForTesting
  void handlePeerEventForTesting(PeerStreamEvent event) {
    _handlePeerEvent(event);
  }

  @visibleForTesting
  Future<void> waitForPeerEventRefreshesForTesting() async {
    while (_historyEventRefreshes.isNotEmpty) {
      await Future.wait(_historyEventRefreshes.values.toList());
    }
  }

  Future<void> _refreshWorkspaceHistoryFromEvent(String workspaceName) async {
    final client = connection.client;
    final serverId = activeServerId ?? connection.serverId;
    if (_closing || client == null || serverId == null) return;
    await workspaceChatRepository.refresh(
      client: client,
      serverId: serverId,
      workspaceName: workspaceName,
      localPeerPublicKey: clientPublicKey,
    );
  }

  Future<void> _closePeerEventSession() async {
    _peerEventGeneration += 1;
    final subscription = _peerEventSubscription;
    final session = _peerEventSession;
    _peerEventSubscription = null;
    _peerEventSession = null;
    await subscription?.cancel();
    await session?.close();
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
          Object? workflowCatalogError;
          try {
            await _refreshWorkflowCatalog(
              client,
              isCurrent: () =>
                  connection.profile.endpoint == endpoint &&
                  identical(connection.client, client),
            );
          } catch (error) {
            workflowCatalogError = error;
          }
          final warnings = await repository.refresh(
            client: client,
            endpoint: endpoint,
            isCurrent: () =>
                connection.profile.endpoint == endpoint &&
                identical(connection.client, client),
            serverId: serverId,
          );
          if (connection.profile.endpoint != endpoint ||
              !identical(connection.client, client)) {
            continue;
          }
          await _syncRunWorkspace(client);
          await _refreshPeerProfile(client);
          connectionState = MobileConnectionState.connected;
          if (workflowCatalogError != null || warnings.isNotEmpty) {
            lastError = workflowCatalogError ?? warnings.first;
            assert(() {
              if (workflowCatalogError != null) {
                debugPrint(
                  'GizClaw workflow catalog refresh failed: '
                  '$workflowCatalogError',
                );
              }
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

  Future<void> _refreshWorkflowCatalog(
    GizClawClient client, {
    required bool Function() isCurrent,
  }) async {
    final loaded = <Workflow>[];
    final aliases = <String>{};
    String? profileName;
    String? profileRevision;
    for (final collection in appWorkflowCollections) {
      final collectionItems = <Workflow>[];
      String? cursor;
      do {
        final WorkflowListResponse response;
        try {
          response = await client.listWorkflows(
            collection: collection.id,
            cursor: cursor,
            limit: 200,
          );
        } on RpcError catch (error) {
          if (error.code == 404 && cursor == null) break;
          rethrow;
        }
        profileName ??= response.runtimeProfileName;
        profileRevision ??= response.runtimeProfileRevision;
        if (response.runtimeProfileName != profileName ||
            response.runtimeProfileRevision != profileRevision) {
          throw StateError('Runtime profile changed while loading workflows');
        }
        for (final workflow in response.items) {
          if (workflow.collection != collection.id) {
            throw StateError(
              'Workflow ${workflow.alias} belongs to ${workflow.collection}',
            );
          }
          if (!aliases.add(workflow.alias)) {
            throw StateError('Duplicate workflow alias ${workflow.alias}');
          }
          collectionItems.add(workflow.deepCopy());
        }
        cursor = response.hasNext && response.hasNextCursor()
            ? response.nextCursor
            : null;
      } while (cursor != null && cursor.isNotEmpty);
      loaded.addAll(collectionItems);
    }
    if (!isCurrent()) return;
    _workflowResources = List.unmodifiable(loaded);
    runtimeProfileName = profileName ?? '';
    runtimeProfileRevision = profileRevision ?? '';
    _rebuildWorkflowCards();
  }

  void _rebuildWorkflowCards() {
    workflows = List.unmodifiable(
      _workflowResources.map(
        (workflow) => appWorkflowCard(workflow, _effectiveLocale),
      ),
    );
  }

  List<WorkflowCard> workflowsForCollection(String collection) =>
      List.unmodifiable(
        workflows.where((workflow) => workflow.collection == collection),
      );

  Future<void> updatePeerProfile({
    required String name,
    required String emoji,
  }) async {
    final value = await runRpc(
      (client) => client.putServerInfo(
        DeviceProfile(name: name.trim(), emoji: emoji.trim()),
      ),
    );
    peerName = value.value.name;
    peerEmoji = value.value.emoji;
    notifyListeners();
  }

  Future<void> _refreshPeerProfile(GizClawClient client) async {
    try {
      final value = await client.getServerInfo();
      peerName = value.value.name;
      peerEmoji = value.value.emoji;
    } catch (_) {
      // Profile refresh is optional; retain the last text projection.
    }
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
    if (_backgroundReconnectRequired ||
        connectionState != MobileConnectionState.connected ||
        !connection.isConnected) {
      _scheduleReconnect(force: _backgroundReconnectRequired);
    }
  }

  void _scheduleReconnect({bool force = false}) {
    if (force) _backgroundReconnectRequired = true;
    if (_closing ||
        !connection.profile.isConfigured ||
        _backgroundReconnectTimer != null ||
        _reconnecting != null) {
      return;
    }
    final delay = _backgroundReconnectDelay;
    final nextMilliseconds = (delay.inMilliseconds * 2)
        .clamp(
          backgroundReconnectInitialDelay.inMilliseconds,
          backgroundReconnectMaxDelay.inMilliseconds,
        )
        .toInt();
    _backgroundReconnectDelay = Duration(milliseconds: nextMilliseconds);
    if (!kReleaseMode) {
      debugPrint(
        'GizClaw reconnect: scheduled in '
        '${delay.inMilliseconds}ms',
      );
    }
    _backgroundReconnectTimer = Timer(delay, () {
      _backgroundReconnectTimer = null;
      if (_closing) return;
      final required = _backgroundReconnectRequired;
      _backgroundReconnectRequired = false;
      if (!required &&
          connectionState == MobileConnectionState.connected &&
          connection.isConnected) {
        _backgroundReconnectDelay = backgroundReconnectInitialDelay;
        return;
      }
      unawaited(_retryConnection());
    });
  }

  Future<void> _retryConnection() async {
    if (!kReleaseMode) {
      debugPrint('GizClaw reconnect: attempting');
    }
    try {
      await _reconnect();
      _backgroundReconnectDelay = backgroundReconnectInitialDelay;
      if (!kReleaseMode) {
        debugPrint('GizClaw reconnect: connected');
      }
    } catch (error) {
      if (!kReleaseMode) {
        debugPrint('GizClaw reconnect: failed: $error');
      }
      // _clearReconnect schedules the next bounded-backoff attempt.
    }
  }

  Future<GizClawClient> _performReconnect() async {
    await _replaceActiveWorkspaceChat(null);
    await _closePeerEventSession();
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
      }
      await _openPeerEventSession();
      await refresh(client: client, serverId: serverId);
      connectionState = MobileConnectionState.connected;
      _markTransportReady();
      notifyListeners();
      return client;
    } catch (error) {
      connectionState = connection.isConnected
          ? MobileConnectionState.connected
          : MobileConnectionState.offline;
      lastError = error;
      _scheduleReconnect(force: connection.isConnected);
      if (!kReleaseMode) {
        debugPrint('GizClaw reconnect failed: $error');
      }
      notifyListeners();
      rethrow;
    }
  }

  void _markTransportReady() {
    _backgroundReconnectRequired = false;
    _backgroundReconnectTimer?.cancel();
    _backgroundReconnectTimer = null;
    _backgroundReconnectDelay = backgroundReconnectInitialDelay;
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
    bool refreshAfterCreate = true,
  }) async {
    final response = await _friendClient().createFriendGroup(
      name: name.trim(),
      description: description.trim(),
    );
    if (refreshAfterCreate) await refresh();
    return response.value;
  }

  Future<Workspace> createWorkspace({
    required String collection,
    required String workflowAlias,
  }) async {
    final normalizedCollection = collection.trim();
    final normalizedAlias = workflowAlias.trim();
    final selected = workflows.firstWhere(
      (workflow) =>
          workflow.collection == normalizedCollection &&
          workflow.name == normalizedAlias,
      orElse: () => throw StateError(
        'Workflow $normalizedAlias is not available in $normalizedCollection',
      ),
    );
    if (selected.driver == WorkflowDriverKind.unsupported ||
        selected.driver == WorkflowDriverKind.chatroom) {
      throw StateError('Workflow $normalizedAlias is not supported');
    }
    final workspaceName = _newWorkspaceName(normalizedAlias);
    final response = await runRpc(
      (client) => client.createWorkspace(
        WorkspaceCreateBody(
          name: workspaceName,
          collection: normalizedCollection,
          workflowAlias: normalizedAlias,
          parameters: newWorkspaceParametersForDriver(
            selected.driver,
            langPair: selected.workspaceLangPair,
          ),
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

  WorkflowCard workflow(String name, {String collection = ''}) {
    for (final item in workflows) {
      if (item.name == name) return item;
    }
    return WorkflowCard.unknown(name, collection: collection);
  }

  WorkspaceCard workspace(String name) {
    return workspaces.firstWhere(
      (item) => item.name == name,
      orElse: () => WorkspaceCard(
        name: name,
        workflowAlias: '',
        collection: '',
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

  String chatSenderLabel(String workspaceName, String senderPublicKey) {
    final normalizedSender = senderPublicKey.trim();
    for (final metadata in chatroomWorkspaces) {
      if (metadata.kind == ChatroomWorkspaceKind.direct &&
          metadata.peerPublicKey.trim() == normalizedSender &&
          metadata.title.trim().isNotEmpty) {
        return metadata.title.trim();
      }
    }
    if (normalizedSender.length <= 14) return normalizedSender;
    return '${normalizedSender.substring(0, 8)}…'
        '${normalizedSender.substring(normalizedSender.length - 4)}';
  }

  WorkspaceChatController createWorkspaceHistoryViewer({
    required String workspaceName,
    TerminalWorkspaceAccessCallback? onTerminalWorkspaceAccess,
  }) {
    return WorkspaceChatController(
      workspaceName: workspaceName,
      repository: workspaceChatRepository,
      serverId: activeServerId,
      client: connection.client,
      eventSession: _peerEventSession,
      localPeerPublicKey: clientPublicKey,
      senderLabelResolver: (senderPublicKey) =>
          chatSenderLabel(workspaceName, senderPublicKey),
      onWorkspaceAccessError: reconcileWorkspaceFailure,
      onTerminalWorkspaceAccess: onTerminalWorkspaceAccess,
    );
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
    return MobileWorkspaceDestination(
      surface: MobileWorkspaceSurface.raid,
      workspaceName: workspaceName,
      collection: workspace(workspaceName).collection,
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

  Future<void> reconcileWorkspaceFailure(
    String workspaceName,
    Object error,
    GizClawClient sourceClient,
    String sourceServerId,
  ) async {
    if (error is! RpcError || (error.code != 403 && error.code != 404)) return;
    bool isCurrent() =>
        !_closing &&
        activeServerId == sourceServerId &&
        identical(connection.client, sourceClient);
    if (!isCurrent()) return;

    try {
      if (error.code == 404) {
        await repository.deleteWorkspaceProjection(
          sourceServerId,
          workspaceName,
          isCurrent: isCurrent,
        );
        return;
      }

      final endpoint = connection.profile.endpoint;
      final result = await repository.refreshWorkspaceSnapshot(
        client: sourceClient,
        endpoint: endpoint,
        isCurrent: () =>
            isCurrent() &&
            connection.profile.endpoint == endpoint &&
            identical(connection.client, sourceClient),
        serverId: sourceServerId,
      );
      if (!result.applied || result.contains(workspaceName)) return;
      // Atomic snapshot replacement already removed the absent workspace.
    } catch (reconciliationError) {
      assert(() {
        debugPrint(
          'Workspace reconciliation failed for $workspaceName: '
          '$reconciliationError',
        );
        return true;
      }());
    }
  }

  Future<WorkspaceChatController> _activateWorkspaceChatNow(
    String workspaceName,
  ) async {
    final client = connection.client;
    if (client == null) {
      throw StateError('Connect to GizClaw before switching workspace');
    }
    final sourceServerId = activeServerId;
    late final ServerSetRunWorkspaceResponse selected;
    try {
      selected = await _workspaceActivationStep(
        'select workspace',
        () => client.setRunWorkspace(workspaceName),
      );
    } catch (error, stackTrace) {
      if (sourceServerId != null) {
        await reconcileWorkspaceFailure(
          workspaceName,
          error,
          client,
          sourceServerId,
        );
      }
      Error.throwWithStackTrace(error, stackTrace);
    }
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
    try {
      await _loadActiveWorkspaceDocument(client);
    } on RpcError catch (error) {
      if (workspaceName.isEmpty ||
          state.agentType.trim() != 'chatroom' ||
          (error.code != 403 && error.code != 404)) {
        rethrow;
      }
      activeWorkspaceDocument = null;
      final chat = await _installActiveWorkspaceChat(workspaceName);
      await chat.restrictHistory();
      notifyListeners();
      return;
    }
    final repaired = await _ensureActiveWorkspaceParameters(client);
    if (workspaceName.isNotEmpty &&
        (_runWorkspaceNeedsReload(state) || repaired)) {
      state = (await client.reloadRunWorkspace()).value;
      runWorkspaceState = state;
      await _loadActiveWorkspaceDocument(client);
    }
    if (workspaceName.isEmpty) {
      await _replaceActiveWorkspaceChat(null);
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
    await client.putWorkspace(
      resolvedWorkspaceName,
      workspacePutBodyFromWorkspace(updated),
    );
    await _loadActiveWorkspaceDocument(
      client,
      workspaceName: resolvedWorkspaceName,
    );
    return true;
  }

  Future<WorkflowDriverKind> _driverForWorkspace(Workspace workspace) async {
    final cached = workflow(workspace.workflowAlias).driver;
    if (cached != WorkflowDriverKind.unsupported) return cached;
    final client = connection.client;
    if (client == null || workspace.workflowAlias.isEmpty) return cached;
    try {
      final response = await client.getWorkflow(workspace.workflowAlias);
      return appWorkflowCard(response.value, _effectiveLocale).driver;
    } catch (_) {
      return cached;
    }
  }

  Future<WorkspaceChatController> _installActiveWorkspaceChat(
    String workspaceName,
  ) async {
    final current = _activeWorkspaceChat;
    if (current != null && current.workspaceName == workspaceName) {
      notifyListeners();
      return current;
    }
    await _replaceActiveWorkspaceChat(null);
    late final WorkspaceChatController chat;
    chat = WorkspaceChatController(
      workspaceName: workspaceName,
      repository: workspaceChatRepository,
      serverId: activeServerId,
      client: connection.client,
      eventSession: _peerEventSession,
      peerConnection: connection.peerConnection,
      inputTrack: connection.microphoneTrack,
      setInputSending: connection.setMicrophoneSending,
      ownsInputTrack: () => identical(_activeWorkspaceChat, chat),
      onTransportClosed: recoverTransport,
      onWorkspaceAccessError: reconcileWorkspaceFailure,
      peerEventErrorMessageResolver: (event) =>
          localizedPeerEventErrorMessage(event, _effectiveLocale),
      pcmAudioLevels: PcmAudioLevelSource.levels,
      localPeerPublicKey: clientPublicKey,
      senderLabelResolver: (senderPublicKey) =>
          chatSenderLabel(workspaceName, senderPublicKey),
    );
    await _replaceActiveWorkspaceChat(chat);
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
    if (!_workspaceExposesInputMode(workspace)) {
      throw StateError('The active workspace does not expose an input mode');
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
    await client.putWorkspace(
      workspaceName,
      workspacePutBodyFromWorkspace(updated),
    );
    await _loadActiveWorkspaceDocument(client);
    if (_workspaceInputMode(activeWorkspaceDocument) != mode) {
      throw StateError('GizClaw did not persist the requested input mode');
    }
    final reloaded = await client.reloadRunWorkspace();
    runWorkspaceState = reloaded.value;
    await _replaceActiveWorkspaceChat(null);
    await _installActiveWorkspaceChat(workspaceName);
    notifyListeners();
  }

  Future<void> _replaceActiveWorkspaceChat(
    WorkspaceChatController? chat,
  ) async {
    if (identical(chat, _activeWorkspaceChat)) return;
    final previous = _activeWorkspaceChat;
    await previous?.close();
    previous?.dispose();
    _activeWorkspaceChat = chat;
  }

  Future<void> close() {
    final close = _closeFuture;
    if (close != null) return close;
    _closing = true;
    return _closeFuture = _close();
  }

  Future<void> _close() async {
    connection.removeListener(_handleConnectionChanged);
    _startGeneration += 1;
    _serverWatchGeneration += 1;
    _refreshAgain = false;
    _recoverMicrophoneAfterResume = false;
    _backgroundReconnectTimer?.cancel();
    _backgroundReconnectTimer = null;
    _backgroundReconnectRequired = false;

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
    await chat?.releaseInputTrack();
    _activeWorkspaceChat = null;
    final subscriptions = <StreamSubscription<dynamic>?>[
      _workspaceSubscription,
      _friendChatSubscription,
      _friendGroupChatSubscription,
      _peerEventSubscription,
    ];
    _workspaceSubscription = null;
    _friendChatSubscription = null;
    _friendGroupChatSubscription = null;
    _peerEventSubscription = null;
    final peerEventSession = _peerEventSession;
    _peerEventSession = null;

    await Future.wait([
      if (chat != null) attempt(chat.close),
      for (final subscription in subscriptions)
        if (subscription != null) attempt(subscription.cancel),
    ]);
    if (peerEventSession != null) {
      await attempt(peerEventSession.close);
    }
    _historyEventRefreshDirty.clear();
    final historyEventRefreshes = _historyEventRefreshes.values.toList();
    await Future.wait([
      for (final refresh in historyEventRefreshes)
        attempt(() async => await refresh),
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
  WorkflowDriverKind driver, {
  String? langPair,
}) => switch (driver) {
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
      enableSourceLanguageDetect: _workspaceLangPair(langPair) == 'auto',
      input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
      langPair: _workspaceLangPair(langPair),
      mode: ASTTranslateMode.ASTTRANSLATE_MODE_S2S,
    ),
  ),
  _ => throw UnsupportedError(
    'Creating ${driver.label} workspaces is not supported',
  ),
};

String _workspaceLangPair(String? value) {
  final normalized = value?.trim() ?? '';
  return normalized.isEmpty ? 'auto' : normalized;
}

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
  return WorkspaceInputMode.WORKSPACE_INPUT_MODE_UNSPECIFIED;
}

bool _workspaceExposesInputMode(Workspace workspace) {
  if (!workspace.hasParameters()) return false;
  final parameters = workspace.parameters;
  return parameters.hasAsttranslateWorkspaceParameters() ||
      parameters.hasChatRoomWorkspaceParameters() ||
      parameters.hasDoubaoRealtimeWorkspaceParameters() ||
      parameters.hasFlowcraftWorkspaceParameters();
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

@visibleForTesting
MobileConnectionFailureKind? classifyMobileConnectionFailure({
  required String endpoint,
  required Object? error,
  required TargetPlatform platform,
  required MobileConnectionState state,
}) {
  if (state != MobileConnectionState.offline || error == null) return null;
  if (platform == TargetPlatform.iOS && _isLocalNetworkEndpoint(endpoint)) {
    return MobileConnectionFailureKind.localNetwork;
  }
  return MobileConnectionFailureKind.generic;
}

bool _isLocalNetworkEndpoint(String endpoint) {
  final host = Uri.tryParse('gizclaw://${endpoint.trim()}')?.host.toLowerCase();
  if (host == null || host.isEmpty) return false;
  if (host == 'localhost' || host.endsWith('.local')) return true;
  final parts = host.split('.').map(int.tryParse).toList(growable: false);
  if (parts.length == 4 && parts.every((part) => part != null)) {
    final first = parts[0]!;
    final second = parts[1]!;
    return first == 10 ||
        (first == 172 && second >= 16 && second <= 31) ||
        (first == 192 && second == 168) ||
        (first == 169 && second == 254) ||
        first == 127;
  }
  return host == '::1' ||
      host.startsWith('fc') ||
      host.startsWith('fd') ||
      host.startsWith('fe8') ||
      host.startsWith('fe9') ||
      host.startsWith('fea') ||
      host.startsWith('feb');
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
  String activeRegistrationToken,
) {
  final merged = <GizClawServer>[];
  final endpoints = <String>{};
  final trimmedActiveEndpoint = activeEndpoint.trim();
  final normalizedActiveEndpoint = trimmedActiveEndpoint.isEmpty
      ? ''
      : normalizeGizClawEndpoint(trimmedActiveEndpoint);
  for (final server in servers) {
    final endpoint = normalizeGizClawEndpoint(server.accessPoint);
    if (endpoint.isEmpty || !endpoints.add(endpoint)) continue;
    final registrationToken = server.registrationToken.trim().isNotEmpty
        ? server.registrationToken.trim()
        : endpoint == normalizedActiveEndpoint
        ? activeRegistrationToken.trim()
        : '';
    merged.add(
      GizClawServer(
        name: server.name.trim(),
        accessPoint: endpoint,
        registrationToken: registrationToken,
      ),
    );
  }
  if (trimmedActiveEndpoint.isNotEmpty) {
    final endpoint = normalizedActiveEndpoint;
    if (endpoints.add(endpoint)) {
      merged.add(
        GizClawServer(
          name: endpoint,
          accessPoint: endpoint,
          registrationToken: activeRegistrationToken.trim(),
        ),
      );
    }
  }
  return merged;
}

String _normalizeRequiredServerEndpoint(String endpoint) {
  final normalized = normalizeGizClawEndpoint(endpoint);
  if (normalized.isEmpty) {
    throw const FormatException('Enter a server access point');
  }
  return normalized;
}

class _DemoMobileDataController extends MobileDataController {
  _DemoMobileDataController({super.database})
    : super(
        profile: const GizClawConnectionProfile(
          endpoint: _demoServerEndpoint,
          clientPrivateKey: 'demo-private-key',
        ),
        servers: const [
          GizClawServer(name: 'Demo', accessPoint: _demoServerEndpoint),
        ],
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

  static MobileDataController read(BuildContext context) {
    final element = context
        .getElementForInheritedWidgetOfExactType<MobileDataScope>();
    final scope = element?.widget as MobileDataScope?;
    assert(scope != null, 'MobileDataScope is missing');
    return scope!.notifier!;
  }
}
