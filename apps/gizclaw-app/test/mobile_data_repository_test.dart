import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/repositories/mobile_data_repository.dart';
import 'package:gizclaw_app/prototype/prototype_models.dart';

void main() {
  test(
    'refreshes workspace and social snapshots without listing workflows',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(database.close);
      final repository = MobileDataRepository(database);
      final client = _FakeClient(
        workspaces: [
          Workspace(
            name: 'mobile-plan',
            workflowName: 'build-helper',
            lastActiveAt: '2026-07-12T00:00:00Z',
          ),
          Workspace(
            name: 'social-group-a',
            workflowName: 'chatroom',
            parameters: WorkspaceParameters(
              chatRoomWorkspaceParameters: ChatRoomWorkspaceParameters(
                mode: ChatRoomMode.CHAT_ROOM_MODE_GROUP,
              ),
            ),
          ),
        ],
        friends: [
          FriendObject(
            id: 'friend-a',
            peerPublicKey: 'peer-public-key-a',
            workspaceName: 'social-direct-a',
          ),
        ],
        friendGroups: [
          FriendGroupObject(
            id: 'group-a',
            name: 'Builder Crew',
            description: 'Shipping together',
            workspaceName: 'social-group-a',
          ),
        ],
      );

      await repository.refresh(
        client: client,
        endpoint: '127.0.0.1:23820',
        isCurrent: () => true,
        serverId: 'server-a',
      );

      final workspaces = await repository.watchWorkspaces('server-a').first;
      expect(client.workflowSources, isEmpty);
      final mobileWorkspace = workspaces.firstWhere(
        (workspace) => workspace.name == 'mobile-plan',
      );
      expect(mobileWorkspace.title, 'mobile-plan');
      expect(mobileWorkspace.workflowName, 'build-helper');
      expect(
        workspaces
            .firstWhere((workspace) => workspace.name == 'social-group-a')
            .chatroomKind,
        ChatroomWorkspaceKind.group,
      );
      expect(
        await repository.serverIdForEndpoint('127.0.0.1:23820'),
        'server-a',
      );
      expect(
        (await repository.workspaceDocument(
          'server-a',
          'mobile-plan',
        ))?.workflowName,
        'build-helper',
      );
      expect(await repository.workspaceDocument('server-a', 'missing'), isNull);
      final friendChats = await repository.watchFriendChats('server-a').first;
      expect(friendChats.single.workspaceName, 'social-direct-a');
      expect(friendChats.single.title, 'friend-a');
      expect(friendChats.single.resourceId, 'friend-a');
      final groupChats = await repository
          .watchFriendGroupChats('server-a')
          .first;
      expect(groupChats.single.workspaceName, 'social-group-a');
      expect(groupChats.single.title, 'Builder Crew');
      expect(groupChats.single.description, 'Shipping together');
    },
  );

  test('complete refresh removes rows absent from the snapshot', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(database);
    final client = _FakeClient(
      workspaces: [
        Workspace(name: 'temporary-room', workflowName: 'temporary'),
      ],
    );
    await repository.refresh(
      client: client,
      endpoint: 'local',
      isCurrent: () => true,
      serverId: 'server-a',
    );

    client.workspaces.clear();
    await repository.refresh(
      client: client,
      endpoint: 'local',
      isCurrent: () => true,
      serverId: 'server-a',
    );

    expect(await repository.watchWorkspaces('server-a').first, isEmpty);
  });

  test('workspace failure preserves the previous projection', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(database);
    final client = _FakeClient(
      workspaces: [Workspace(name: 'cached', workflowName: 'flow-a')],
    );
    await repository.refreshWorkspaceSnapshot(
      client: client,
      endpoint: 'local',
      isCurrent: () => true,
      serverId: 'server-a',
    );

    client.workspaces.clear();
    client.failWorkspaces = true;
    final warnings = await repository.refresh(
      client: client,
      endpoint: 'local',
      isCurrent: () => true,
      serverId: 'server-a',
    );

    expect(warnings.map((warning) => warning.scope), contains('Workspaces'));
    expect(
      (await repository.watchWorkspaces('server-a').first).single.name,
      'cached',
    );
  });

  test('workspace snapshot evidence belongs to the committing call', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(database);
    final client = _FakeClient(
      workspaces: [Workspace(name: 'visible', workflowName: 'flow-a')],
    );

    final applied = await repository.refreshWorkspaceSnapshot(
      client: client,
      endpoint: 'local',
      isCurrent: () => true,
      serverId: 'server-a',
    );
    client.workspaces.clear();
    final stale = await repository.refreshWorkspaceSnapshot(
      client: client,
      endpoint: 'local',
      isCurrent: () => false,
      serverId: 'server-a',
    );

    expect(applied.applied, isTrue);
    expect(applied.contains('visible'), isTrue);
    expect(stale.applied, isFalse);
    expect(stale.contains('visible'), isFalse);
    expect(
      (await repository.watchWorkspaces('server-a').first).single.name,
      'visible',
    );
  });

  test(
    'failed workspace replacement preserves the previous projection',
    () async {
      final database = _FailingTransactionDatabase();
      addTearDown(() async {
        database.failTransactions = false;
        await database.close();
      });
      final repository = MobileDataRepository(database);
      final client = _FakeClient(
        workspaces: [Workspace(name: 'cached', workflowName: 'flow-a')],
      );
      await repository.refreshWorkspaceSnapshot(
        client: client,
        endpoint: 'local',
        isCurrent: () => true,
        serverId: 'server-a',
      );

      client.workspaces.clear();
      database.failTransactions = true;
      await expectLater(
        repository.refreshWorkspaceSnapshot(
          client: client,
          endpoint: 'local',
          isCurrent: () => true,
          serverId: 'server-a',
        ),
        throwsStateError,
      );
      database.failTransactions = false;

      expect(
        (await repository.watchWorkspaces('server-a').first).single.name,
        'cached',
      );
    },
  );

  test('targeted eviction stays inside one server partition', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(database);
    final client = _FakeClient(
      workspaces: [Workspace(name: 'shared-name', workflowName: 'flow-a')],
    );
    for (final serverId in ['server-a', 'server-b']) {
      await repository.refreshWorkspaceSnapshot(
        client: client,
        endpoint: '$serverId.local',
        isCurrent: () => true,
        serverId: serverId,
      );
    }

    await repository.deleteWorkspaceProjection(
      'server-a',
      'shared-name',
      isCurrent: () => true,
    );

    expect(
      await repository.workspaceDocument('server-a', 'shared-name'),
      isNull,
    );
    expect(
      await repository.workspaceDocument('server-b', 'shared-name'),
      isNotNull,
    );
  });

  test('targeted eviction rolls back when its source becomes stale', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(database);
    final client = _FakeClient(
      workspaces: [Workspace(name: 'visible', workflowName: 'flow-a')],
    );
    await repository.refreshWorkspaceSnapshot(
      client: client,
      endpoint: 'server-a.local',
      isCurrent: () => true,
      serverId: 'server-a',
    );
    var freshnessChecks = 0;

    await repository.deleteWorkspaceProjection(
      'server-a',
      'visible',
      isCurrent: () => freshnessChecks++ == 0,
    );

    expect(
      await repository.workspaceDocument('server-a', 'visible'),
      isNotNull,
    );
  });

  test(
    'social RPC failure does not leave the workspace catalog stale',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(database.close);
      final repository = MobileDataRepository(database);
      final client = _FakeClient(
        workspaces: [
          Workspace(name: 'old-workspace', workflowName: 'old-workflow'),
        ],
        friends: [
          FriendObject(
            id: 'friend-a',
            peerPublicKey: 'peer-a',
            workspaceName: 'friend-workspace-a',
          ),
        ],
      );
      await repository.refresh(
        client: client,
        endpoint: 'local',
        isCurrent: () => true,
        serverId: 'server-a',
      );

      client.workspaces
        ..clear()
        ..add(Workspace(name: 'new-workspace', workflowName: 'chat'));
      client.failFriends = true;
      client.failFriendGroups = true;

      final warnings = await repository.refresh(
        client: client,
        endpoint: 'local',
        isCurrent: () => true,
        serverId: 'server-a',
      );

      expect(warnings, hasLength(2));
      expect(warnings.map((warning) => warning.scope), ['Friends', 'Groups']);
      expect(
        (await repository.watchWorkspaces('server-a').first).single.name,
        'new-workspace',
      );
      expect(
        (await repository.watchFriendChats('server-a').first).single.resourceId,
        'friend-a',
      );
    },
  );
}

class _FakeClient extends GizClawClient {
  _FakeClient({
    required this.workspaces,
    this.friends = const [],
    this.friendGroups = const [],
  }) : super(_NeverDataChannelFactory());

  final List<FriendGroupObject> friendGroups;
  final List<FriendObject> friends;
  final List<Workspace> workspaces;
  bool failFriends = false;
  bool failFriendGroups = false;
  bool failWorkspaces = false;
  final List<ResourceSource> workflowSources = [];

  @override
  Future<WorkflowListResponse> listWorkflows({
    required ResourceSource source,
    String? cursor,
    int? limit,
  }) async {
    workflowSources.add(source);
    return WorkflowListResponse();
  }

  @override
  Future<WorkspaceListResponse> listWorkspaces({
    String? cursor,
    int? limit,
    String? prefix,
  }) async {
    if (failWorkspaces) throw StateError('workspace catalog unavailable');
    return WorkspaceListResponse(items: workspaces);
  }

  @override
  Future<FriendListResponse> listFriends({String? cursor, int? limit}) async {
    if (failFriends) throw const FormatException('friend payload missing');
    return FriendListResponse(items: friends);
  }

  @override
  Future<FriendGroupListResponse> listFriendGroups({
    String? cursor,
    int? limit,
  }) async {
    if (failFriendGroups) {
      throw const FormatException('friend group payload missing');
    }
    return FriendGroupListResponse(items: friendGroups);
  }
}

class _FailingTransactionDatabase extends AppDatabase {
  _FailingTransactionDatabase() : super.forTesting(NativeDatabase.memory());

  bool failTransactions = false;

  @override
  Future<T> transaction<T>(
    Future<T> Function() action, {
    bool requireNew = false,
  }) {
    if (failTransactions) {
      return Future<T>.error(StateError('transaction failed'));
    }
    return super.transaction(action, requireNew: requireNew);
  }
}

class _NeverDataChannelFactory implements GizClawDataChannelFactory {
  @override
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  }) {
    throw UnsupportedError('No transport is used by this repository test');
  }
}
