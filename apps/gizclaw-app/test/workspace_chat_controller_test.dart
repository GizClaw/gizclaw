import 'dart:async';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/repositories/workspace_chat_repository.dart';
import 'package:gizclaw_app/data/workspace_chat_controller.dart';

void main() {
  test('derives an inbound PCM level from cumulative WebRTC energy', () {
    expect(
      audioLevelFromEnergyDelta(
        previousEnergy: 1,
        previousDuration: 10,
        energy: 1.04,
        duration: 11,
      ),
      closeTo(0.2, 0.0001),
    );
  });

  test('ignores reset and stalled WebRTC energy counters', () {
    expect(
      audioLevelFromEnergyDelta(
        previousEnergy: 2,
        previousDuration: 10,
        energy: 1,
        duration: 11,
      ),
      0,
    );
    expect(
      audioLevelFromEnergyDelta(
        previousEnergy: 1,
        previousDuration: 10,
        energy: 1.1,
        duration: 10,
      ),
      0,
    );
  });

  test('normalizes standard and legacy WebRTC audio levels', () {
    expect(normalizedAudioLevel(0.25), 0.25);
    expect(normalizedAudioLevel('0.5'), 0.5);
    expect(normalizedAudioLevel(16384), closeTo(0.5, 0.001));
    expect(normalizedAudioLevel(null), 0);
  });

  test('appends a final text.done chunk to streamed deltas', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: WorkspaceChatRepository(database),
      serverId: null,
    );
    addTearDown(controller.dispose);

    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.delta',
        streamId: 'answer-1',
        label: 'assistant',
        text: 'Hello ',
      ),
    );
    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.delta',
        streamId: 'answer-1',
        label: 'assistant',
        text: 'world',
      ),
    );
    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.done',
        streamId: 'answer-1',
        label: 'assistant',
        text: '!',
      ),
    );

    expect(controller.messages.single.text, 'Hello world!');
    expect(controller.messages.single.state, WorkspaceMessageState.complete);
  });

  test('accepts text.done containing the complete streamed text', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: WorkspaceChatRepository(database),
      serverId: null,
    );
    addTearDown(controller.dispose);

    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.delta',
        streamId: 'answer-2',
        label: 'assistant',
        text: 'Complete ',
      ),
    );
    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.done',
        streamId: 'answer-2',
        label: 'assistant',
        text: 'Complete response',
      ),
    );

    expect(controller.messages.single.text, 'Complete response');
  });

  test('keeps a history-only viewer in error when refresh fails', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: _FailingHistoryRepository(database),
      serverId: 'server-a',
      client: GizClawClient(_NeverDataChannelFactory()),
    );
    addTearDown(controller.dispose);

    await controller.start(conversation: false);

    expect(controller.state, WorkspaceChatState.error);
    expect(controller.lastError, isA<StateError>());
  });

  test(
    'explains an inaccessible history viewer removed by reconciliation',
    () async {
      final database = AppDatabase.forTesting(NativeDatabase.memory());
      addTearDown(database.close);
      var reconciliations = 0;
      final controller = WorkspaceChatController(
        workspaceName: 'deleted-workspace',
        repository: _DeniedHistoryRepository(database),
        serverId: 'server-a',
        client: GizClawClient(_NeverDataChannelFactory()),
        onAccessDenied: () async {
          reconciliations++;
          return true;
        },
      );
      addTearDown(controller.dispose);

      await controller.start(conversation: false);

      expect(reconciliations, 1);
      expect(controller.state, WorkspaceChatState.error);
      expect(
        controller.lastError,
        isA<StateError>().having(
          (error) => error.message,
          'message',
          'This workspace was deleted or you no longer have access to it.',
        ),
      );
    },
  );

  test('keeps repeated live text until a new history row arrives', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = _ControlledHistoryRepository(database)
      ..history = const [
        CachedWorkspaceMessage(
          id: 'history-old',
          incoming: true,
          text: 'OK',
          createdAt: null,
          replayAvailable: false,
        ),
      ];
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: repository,
      serverId: 'server-a',
      client: GizClawClient(_NeverDataChannelFactory()),
    );
    addTearDown(controller.dispose);
    addTearDown(repository.close);
    await controller.start(conversation: false);

    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.done',
        streamId: 'answer-new',
        label: 'assistant',
        text: 'OK',
      ),
    );

    expect(controller.messages, hasLength(2));
    repository.emit([
      ...repository.history,
      const CachedWorkspaceMessage(
        id: 'history-new',
        incoming: true,
        text: 'OK',
        createdAt: null,
        replayAvailable: false,
      ),
    ]);
    await Future<void>.delayed(Duration.zero);

    expect(controller.messages, hasLength(2));
    expect(controller.messages.map((message) => message.id), [
      'history-old',
      'history-new',
    ]);
  });
}

class _FailingHistoryRepository extends WorkspaceChatRepository {
  _FailingHistoryRepository(super.database);

  @override
  Future<List<CachedWorkspaceMessage>> refresh({
    required GizClawClient client,
    required String serverId,
    required String workspaceName,
  }) async {
    throw StateError('history unavailable');
  }
}

class _DeniedHistoryRepository extends WorkspaceChatRepository {
  _DeniedHistoryRepository(super.database);

  @override
  Future<List<CachedWorkspaceMessage>> refresh({
    required GizClawClient client,
    required String serverId,
    required String workspaceName,
  }) async {
    throw RpcError(400, 'acl: denied');
  }
}

class _ControlledHistoryRepository extends WorkspaceChatRepository {
  _ControlledHistoryRepository(super.database);

  final _controller = StreamController<List<CachedWorkspaceMessage>>();
  List<CachedWorkspaceMessage> history = const [];

  @override
  Stream<List<CachedWorkspaceMessage>> watchHistory(
    String serverId,
    String workspaceName,
  ) => _controller.stream;

  @override
  Future<List<CachedWorkspaceMessage>> refresh({
    required GizClawClient client,
    required String serverId,
    required String workspaceName,
  }) async => history;

  void emit(List<CachedWorkspaceMessage> value) {
    history = value;
    _controller.add(value);
  }

  Future<void> close() => _controller.close();
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
