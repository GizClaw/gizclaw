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

class _NeverDataChannelFactory implements GizClawDataChannelFactory {
  @override
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  }) {
    throw UnsupportedError('No data channel is used by this test');
  }
}
