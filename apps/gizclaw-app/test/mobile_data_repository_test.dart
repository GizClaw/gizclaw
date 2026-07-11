import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/repositories/mobile_data_repository.dart';

void main() {
  test('refreshes workflow and workspace snapshots into Drift', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(database);
    final client = _FakeClient(
      workflows: [
        WorkflowDocument(
          metadata: WorkflowMetadata(
            name: 'build-helper',
            description: 'Build something useful.',
          ),
          spec: WorkflowSpec(driver: WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT),
        ),
      ],
      workspaces: [
        Workspace(
          displayName: 'My Mobile Plan',
          name: 'mobile-plan',
          workflowName: 'build-helper',
          lastActiveAt: '2026-07-12T00:00:00Z',
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
    expect(workflows.single.title, 'Build Helper');
    expect(workflows.single.driverLabel, 'Flowcraft');
    expect(workspaces.single.name, 'mobile-plan');
    expect(workspaces.single.title, 'My Mobile Plan');
    expect(workspaces.single.workflowName, 'build-helper');
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
  });

  test('complete refresh removes rows absent from the snapshot', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = MobileDataRepository(database);
    final client = _FakeClient(
      workflows: [
        WorkflowDocument(
          metadata: WorkflowMetadata(name: 'temporary'),
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
}

class _FakeClient extends GizClawClient {
  _FakeClient({required this.workflows, required this.workspaces})
    : super(_NeverDataChannelFactory());

  final List<WorkflowDocument> workflows;
  final List<Workspace> workspaces;

  @override
  Future<WorkflowListResponse> listWorkflows({
    String? cursor,
    int? limit,
  }) async {
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
