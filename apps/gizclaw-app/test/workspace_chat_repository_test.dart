import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/repositories/workspace_chat_repository.dart';

void main() {
  test('paginates and isolates authoritative workspace history', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = WorkspaceChatRepository(database);
    final client = _HistoryClient([
      [
        _entry(
          id: 'gear-1',
          text: '你好',
          replayAvailable: true,
          type: PeerRunHistoryEntryType.PEER_RUN_HISTORY_ENTRY_TYPE_GEAR,
        ),
      ],
      [
        _entry(
          id: 'agent-1',
          text: '你好，移动端。',
          type: PeerRunHistoryEntryType.PEER_RUN_HISTORY_ENTRY_TYPE_AGENT,
        ),
      ],
    ]);

    await repository.refresh(
      client: client,
      serverId: 'server-a',
      workspaceName: 'workspace-a',
    );

    final messages = await repository
        .watchHistory('server-a', 'workspace-a')
        .first;
    expect(messages, hasLength(2));
    expect(messages.first.incoming, isFalse);
    expect(messages.first.replayAvailable, isTrue);
    expect(messages.last.incoming, isTrue);
    expect(messages.last.text, '你好，移动端。');
    expect(
      await repository.watchHistory('server-a', 'workspace-b').first,
      isEmpty,
    );
    expect(client.cursors, [null, 'page-1']);
  });

  test('complete refresh removes history absent from the server', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = WorkspaceChatRepository(database);
    final client = _HistoryClient([
      [
        _entry(
          id: 'old',
          text: 'old',
          type: PeerRunHistoryEntryType.PEER_RUN_HISTORY_ENTRY_TYPE_AGENT,
        ),
      ],
    ]);
    await repository.refresh(
      client: client,
      serverId: 'server-a',
      workspaceName: 'workspace-a',
    );

    client.pages = const [[]];
    client.cursors.clear();
    await repository.refresh(
      client: client,
      serverId: 'server-a',
      workspaceName: 'workspace-a',
    );

    expect(
      await repository.watchHistory('server-a', 'workspace-a').first,
      isEmpty,
    );
  });

  test('unavailable history preserves the previous cache', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = WorkspaceChatRepository(database);
    final client = _HistoryClient([
      [
        _entry(
          id: 'saved',
          text: 'saved reply',
          type: PeerRunHistoryEntryType.PEER_RUN_HISTORY_ENTRY_TYPE_AGENT,
        ),
      ],
    ]);
    await repository.refresh(
      client: client,
      serverId: 'server-a',
      workspaceName: 'workspace-a',
    );

    client.available = false;
    await expectLater(
      repository.refresh(
        client: client,
        serverId: 'server-a',
        workspaceName: 'workspace-a',
      ),
      throwsStateError,
    );

    final messages = await repository
        .watchHistory('server-a', 'workspace-a')
        .first;
    expect(messages.single.text, 'saved reply');
  });
}

PeerRunHistoryEntry _entry({
  required String id,
  required String text,
  required PeerRunHistoryEntryType type,
  bool replayAvailable = false,
}) {
  return PeerRunHistoryEntry(
    id: id,
    name: type == PeerRunHistoryEntryType.PEER_RUN_HISTORY_ENTRY_TYPE_GEAR
        ? 'transcript'
        : 'assistant',
    text: text,
    replayAvailable: replayAvailable,
    type: type,
    createdAt:
        '2026-07-12T00:00:0${type == PeerRunHistoryEntryType.PEER_RUN_HISTORY_ENTRY_TYPE_GEAR ? '0' : '1'}Z',
  );
}

class _HistoryClient extends GizClawClient {
  _HistoryClient(this.pages) : super(_NeverDataChannelFactory());

  List<List<PeerRunHistoryEntry>> pages;
  final cursors = <String?>[];
  bool available = true;

  @override
  Future<WorkspaceHistoryListResponse> listWorkspaceHistory({
    required String workspaceName,
    String? cursor,
    int? limit,
  }) async {
    cursors.add(cursor);
    final index = cursor == null ? 0 : int.parse(cursor.split('-').last);
    final hasNext = index + 1 < pages.length;
    return WorkspaceHistoryListResponse(
      value: PeerRunHistoryListResponse(
        available: available,
        items: pages[index],
        hasNext: hasNext,
        nextCursor: hasNext ? 'page-${index + 1}' : '',
      ),
    );
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
