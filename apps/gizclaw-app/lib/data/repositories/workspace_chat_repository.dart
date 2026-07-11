import 'package:drift/drift.dart';
import 'package:gizclaw/gizclaw.dart';

import '../database/app_database.dart';

class CachedWorkspaceMessage {
  const CachedWorkspaceMessage({
    required this.id,
    required this.incoming,
    required this.text,
    required this.createdAt,
  });

  final DateTime? createdAt;
  final String id;
  final bool incoming;
  final String text;
}

class WorkspaceChatRepository {
  WorkspaceChatRepository(this.database);

  final AppDatabase database;

  Stream<List<CachedWorkspaceMessage>> watchHistory(
    String serverId,
    String workspaceName,
  ) {
    final query = database.select(database.workspaceChatEntries)
      ..where(
        (row) =>
            row.serverId.equals(serverId) &
            row.workspaceName.equals(workspaceName),
      )
      ..orderBy([
        (row) => OrderingTerm.asc(row.createdAt),
        (row) => OrderingTerm.asc(row.historyId),
      ]);
    return query.watch().map(
      (rows) => rows
          .map(
            (row) => CachedWorkspaceMessage(
              id: row.historyId,
              incoming: row.role != 'gear',
              text: row.content,
              createdAt: row.createdAt,
            ),
          )
          .toList(growable: false),
    );
  }

  Future<void> refresh({
    required GizClawClient client,
    required String serverId,
    required String workspaceName,
  }) async {
    final items = <PeerRunHistoryEntry>[];
    String? cursor;
    do {
      final response = await client.listWorkspaceHistory(
        workspaceName: workspaceName,
        cursor: cursor,
        limit: 100,
      );
      if (!response.value.available) {
        final message = response.value.message.trim();
        throw StateError(
          message.isEmpty ? 'Workspace history is unavailable' : message,
        );
      }
      items.addAll(response.value.items);
      cursor = response.value.hasNext ? response.value.nextCursor : null;
    } while (cursor != null && cursor.isNotEmpty);

    final refreshedAt = DateTime.now().toUtc();
    await database.transaction(() async {
      await database.batch((batch) {
        batch.insertAllOnConflictUpdate(
          database.workspaceChatEntries,
          items
              .map(
                (entry) => WorkspaceChatEntriesCompanion.insert(
                  serverId: serverId,
                  workspaceName: workspaceName,
                  historyId: entry.id,
                  role: entry.type.value == 1 ? 'gear' : 'agent',
                  content: entry.text,
                  name: entry.name,
                  createdAt: Value(DateTime.tryParse(entry.createdAt)?.toUtc()),
                  refreshedAt: refreshedAt,
                ),
              )
              .toList(),
        );
      });
      final ids = items.map((entry) => entry.id).toSet();
      await (database.delete(database.workspaceChatEntries)..where(
            (row) =>
                row.serverId.equals(serverId) &
                row.workspaceName.equals(workspaceName) &
                row.historyId.isNotIn(ids),
          ))
          .go();
      await database
          .into(database.syncStates)
          .insertOnConflictUpdate(
            SyncStatesCompanion.insert(
              serverId: serverId,
              scope: 'workspace-chat:$workspaceName',
              lastSuccessfulRefreshAt: Value(refreshedAt),
            ),
          );
    });
  }
}
