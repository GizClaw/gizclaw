import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/repositories/mobile_data_repository.dart';
import 'package:gizclaw_app/prototype/prototype_models.dart';

void main() {
  test('refreshes workflow and workspace snapshots into Drift', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(
      database,
      deviceLocaleTag: () => 'zh-CN',
    );
    final client = _FakeClient(
      workflows: [
        Workflow(
          name: 'build-helper',
          i18n: WorkflowI18nCatalog(name: '构建助手', description: '构建有用的东西。'),
          spec: WorkflowSpec(driver: WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT),
        ),
      ],
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
      serverId: 'server-a',
    );

    final workflows = await repository.watchWorkflows('server-a').first;
    final workspaces = await repository.watchWorkspaces('server-a').first;
    expect(workflows.single.name, 'build-helper');
    expect(workflows.single.title, '构建助手');
    expect(workflows.single.subtitle, '构建有用的东西。');
    expect(workflows.single.driverLabel, 'Flowcraft');
    expect(client.lastWorkflowLang, WorkflowLocale.WORKFLOW_LOCALE_ZH_CN);
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
    expect(await repository.serverIdForEndpoint('127.0.0.1:23820'), 'server-a');
    expect(await repository.hasWorkflow('server-a', 'build-helper'), isTrue);
    expect(await repository.hasWorkflow('server-a', 'missing'), isFalse);
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
    final groupChats = await repository.watchFriendGroupChats('server-a').first;
    expect(groupChats.single.workspaceName, 'social-group-a');
    expect(groupChats.single.title, 'Builder Crew');
    expect(groupChats.single.description, 'Shipping together');
  });

  test('uses the default catalog for an unsupported device locale', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(
      database,
      deviceLocaleTag: () => 'zh-TW',
    );
    final client = _FakeClient(
      workflows: [
        Workflow(
          name: 'stable-name',
          i18n: WorkflowI18nCatalog(name: '简体中文', description: '简体中文说明'),
          spec: WorkflowSpec(driver: WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT),
        ),
      ],
      workspaces: const [],
    );

    await repository.refresh(
      client: client,
      endpoint: 'local',
      serverId: 'server-a',
    );

    final card = (await repository.watchWorkflows('server-a').first).single;
    expect(card.title, '简体中文');
    expect(card.subtitle, '简体中文说明');
    expect(
      client.lastWorkflowLang,
      WorkflowLocale.WORKFLOW_LOCALE_UNSPECIFIED,
    );
  });

  test('complete refresh removes rows absent from the snapshot', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(database);
    final client = _FakeClient(
      workflows: [
        Workflow(
          name: 'temporary',
          spec: WorkflowSpec(driver: WorkflowDriver.WORKFLOW_DRIVER_CHATROOM),
        ),
      ],
      workspaces: [
        Workspace(name: 'temporary-room', workflowName: 'temporary'),
      ],
    );
    await repository.refresh(
      client: client,
      endpoint: 'local',
      serverId: 'server-a',
    );

    client.workflows.clear();
    client.workspaces.clear();
    await repository.refresh(
      client: client,
      endpoint: 'local',
      serverId: 'server-a',
    );

    expect(await repository.watchWorkflows('server-a').first, isEmpty);
    expect(await repository.watchWorkspaces('server-a').first, isEmpty);
  });

  test(
    'social RPC failure does not leave the workspace catalog stale',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(database.close);
      final repository = MobileDataRepository(database);
      final client = _FakeClient(
        workflows: [
          Workflow(
            name: 'old-workflow',
            spec: WorkflowSpec(
              driver: WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT,
            ),
          ),
        ],
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
        serverId: 'server-a',
      );

      client.workflows
        ..clear()
        ..add(
          Workflow(
            name: 'new-workflow',
            spec: WorkflowSpec(
              driver: WorkflowDriver.WORKFLOW_DRIVER_AST_TRANSLATE,
            ),
          ),
        );
      client.workspaces
        ..clear()
        ..add(Workspace(name: 'new-workspace', workflowName: 'new-workflow'));
      client.failFriends = true;

      final warnings = await repository.refresh(
        client: client,
        endpoint: 'local',
        serverId: 'server-a',
      );

      expect(warnings, hasLength(1));
      expect(warnings.single.scope, 'Friends');
      expect(
        (await repository.watchWorkflows('server-a').first).single.name,
        'new-workflow',
      );
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
    required this.workflows,
    required this.workspaces,
    this.friends = const [],
    this.friendGroups = const [],
  }) : super(_NeverDataChannelFactory());

  final List<FriendGroupObject> friendGroups;
  final List<FriendObject> friends;
  final List<Workflow> workflows;
  final List<Workspace> workspaces;
  bool failFriends = false;
  WorkflowLocale? lastWorkflowLang;

  @override
  Future<WorkflowListResponse> listWorkflows({
    String? cursor,
    int? limit,
    WorkflowLocale? lang,
  }) async {
    lastWorkflowLang = lang;
    return WorkflowListResponse(items: workflows);
  }

  @override
  Future<WorkspaceListResponse> listWorkspaces({
    String? cursor,
    int? limit,
    String? prefix,
  }) async {
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
    return FriendGroupListResponse(items: friendGroups);
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
