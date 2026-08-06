import 'package:drift/drift.dart';
import 'package:gizclaw/gizclaw.dart';

import '../database/app_database.dart';

class CachedWorkspaceMessage {
  const CachedWorkspaceMessage({
    required this.name,
    required this.incoming,
    required this.text,
    required this.createdAt,
    required this.replayAvailable,
    this.senderPublicKey,
  });

  final DateTime? createdAt;
  final String name;
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
        (row) => OrderingTerm.asc(row.historyName),
      ]);
    return query.watch().map(
      (rows) => rows
          .map(
            (row) => CachedWorkspaceMessage(
              name: row.historyName,
              incoming:
                  row.role != 'gear' ||
                  (localPeerPublicKey != null &&
                      row.gearId?.trim().isNotEmpty == true &&
                      row.gearId != localPeerPublicKey),
              replayAvailable:
                  _replayAvailability[_historyKey(
                    serverId,
                    workspaceName,
                    row.historyName,
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
      _replayAvailability[_historyKey(serverId, workspaceName, item.name)] =
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
                  historyName: entry.name,
                  role: entry.type.value == 1 ? 'gear' : 'agent',
                  gearId: Value(
                    entry.hasGearId() && entry.gearId.trim().isNotEmpty
                        ? entry.gearId.trim()
                        : null,
                  ),
                  content: entry.text,
                  name: entry.actorName,
                  createdAt: Value(DateTime.tryParse(entry.createdAt)?.toUtc()),
                  refreshedAt: refreshedAt,
                ),
              )
              .toList(),
        );
      });
      final names = items.map((entry) => entry.name).toSet();
      await (database.delete(database.workspaceChatEntries)..where(
            (row) =>
                row.serverId.equals(serverId) &
                row.workspaceName.equals(workspaceName) &
                row.historyName.isNotIn(names),
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
            name: entry.name,
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

String _historyKey(String serverId, String workspaceName, String historyName) =>
    '$serverId\u0000$workspaceName\u0000$historyName';

String? _nonEmpty(String? value) {
  final normalized = value?.trim() ?? '';
  return normalized.isEmpty ? null : normalized;
}
