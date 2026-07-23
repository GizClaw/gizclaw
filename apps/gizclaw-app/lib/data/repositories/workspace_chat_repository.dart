import 'package:drift/drift.dart';
import 'package:gizclaw/gizclaw.dart';

import '../database/app_database.dart';

class CachedWorkspaceMessage {
  const CachedWorkspaceMessage({
    required this.id,
    required this.incoming,
    required this.text,
    required this.createdAt,
    required this.replayAvailable,
    this.senderPublicKey,
  });

  final DateTime? createdAt;
  final String id;
  final bool incoming;
  final bool replayAvailable;
  final String? senderPublicKey;
  final String text;
}

class WorkspaceChatRepository {
  WorkspaceChatRepository(this.database);

  final AppDatabase database;
  final Map<String, bool> _replayAvailability = {};

  Stream<List<CachedWorkspaceMessage>> watchHistory(
    String serverId,
    String workspaceName, [
    String? localPeerPublicKey,
  ]) {
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
              incoming:
                  row.role != 'gear' ||
                  (localPeerPublicKey != null &&
                      row.gearId?.trim().isNotEmpty == true &&
                      row.gearId != localPeerPublicKey),
              replayAvailable:
                  _replayAvailability[_historyKey(
                    serverId,
                    workspaceName,
                    row.historyId,
                  )] ??
                  false,
              senderPublicKey: row.role == 'gear'
                  ? _nonEmpty(row.gearId)
                  : null,
              text: row.content,
              createdAt: row.createdAt,
            ),
          )
          .toList(growable: false),
    );
  }

  Future<List<CachedWorkspaceMessage>> refresh({
    required GizClawClient client,
    required String serverId,
    required String workspaceName,
    String? localPeerPublicKey,
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

    for (final item in items) {
      _replayAvailability[_historyKey(serverId, workspaceName, item.id)] =
          item.replayAvailable;
    }

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
                  gearId: Value(
                    entry.hasGearId() && entry.gearId.trim().isNotEmpty
                        ? entry.gearId.trim()
                        : null,
                  ),
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
    return items
        .map(
          (entry) => CachedWorkspaceMessage(
            id: entry.id,
            incoming:
                entry.type.value != 1 ||
                (localPeerPublicKey != null &&
                    entry.hasGearId() &&
                    entry.gearId.trim().isNotEmpty &&
                    entry.gearId.trim() != localPeerPublicKey),
            text: entry.text,
            createdAt: DateTime.tryParse(entry.createdAt)?.toUtc(),
            replayAvailable: entry.replayAvailable,
            senderPublicKey:
                entry.type.value == 1 &&
                    entry.hasGearId() &&
                    entry.gearId.trim().isNotEmpty
                ? entry.gearId.trim()
                : null,
          ),
        )
        .toList(growable: false);
  }
}

String _historyKey(String serverId, String workspaceName, String historyId) =>
    '$serverId\u0000$workspaceName\u0000$historyId';

String? _nonEmpty(String? value) {
  final normalized = value?.trim() ?? '';
  return normalized.isEmpty ? null : normalized;
}
