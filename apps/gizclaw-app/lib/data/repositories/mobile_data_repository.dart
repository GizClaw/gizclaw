import 'package:drift/drift.dart';
import 'package:gizclaw/gizclaw.dart';

import '../../prototype/prototype_models.dart';
import '../database/app_database.dart';

class MobileDataRefreshWarning {
  const MobileDataRefreshWarning({required this.scope, required this.error});

  final Object error;
  final String scope;

  @override
  String toString() => '$scope refresh failed: $error';
}

class MobileDataRepository {
  MobileDataRepository(this.database);

  final AppDatabase database;

  Future<String?> serverIdForEndpoint(String endpoint) async {
    final query = database.select(database.servers)
      ..where((row) => row.endpoint.equals(endpoint))
      ..limit(1);
    return (await query.getSingleOrNull())?.id;
  }

  Stream<List<WorkflowCard>> watchWorkflows(String serverId) {
    final query = database.select(database.workflowEntries)
      ..where((row) => row.serverId.equals(serverId))
      ..orderBy([(row) => OrderingTerm.asc(row.name)]);
    return query.watch().map(
      (rows) => rows.map(_workflowCardFromRow).toList(growable: false),
    );
  }

  Stream<List<WorkspaceCard>> watchWorkspaces(String serverId) {
    final query = database.select(database.workspaceEntries)
      ..where((row) => row.serverId.equals(serverId))
      ..orderBy([
        (row) => OrderingTerm.desc(row.lastActiveAt),
        (row) => OrderingTerm.asc(row.name),
      ]);
    return query.watch().map(
      (rows) => rows.map(_workspaceCardFromRow).toList(growable: false),
    );
  }

  Stream<List<ChatroomWorkspaceMetadata>> watchFriendChats(String serverId) {
    final query = database.select(database.friendEntries)
      ..where((row) => row.serverId.equals(serverId))
      ..orderBy([(row) => OrderingTerm.asc(row.peerPublicKey)]);
    return query.watch().map(
      (rows) => rows
          .where((row) => row.workspaceName?.isNotEmpty ?? false)
          .map(
            (row) => ChatroomWorkspaceMetadata(
              workspaceName: row.workspaceName!,
              title: row.id,
              kind: ChatroomWorkspaceKind.direct,
              resourceId: row.id,
            ),
          )
          .toList(growable: false),
    );
  }

  Stream<List<ChatroomWorkspaceMetadata>> watchFriendGroupChats(
    String serverId,
  ) {
    final query = database.select(database.friendGroupEntries)
      ..where((row) => row.serverId.equals(serverId))
      ..orderBy([(row) => OrderingTerm.asc(row.name)]);
    return query.watch().map(
      (rows) => rows
          .where((row) => row.workspaceName?.isNotEmpty ?? false)
          .map(
            (row) => ChatroomWorkspaceMetadata(
              workspaceName: row.workspaceName!,
              title: row.name.trim().isEmpty ? 'Group chat' : row.name,
              description: row.description,
              kind: ChatroomWorkspaceKind.group,
              resourceId: row.id,
            ),
          )
          .toList(growable: false),
    );
  }

  Future<bool> hasWorkflow(String serverId, String name) async {
    final query = database.select(database.workflowEntries)
      ..where((row) => row.serverId.equals(serverId) & row.name.equals(name))
      ..limit(1);
    return await query.getSingleOrNull() != null;
  }

  Future<WorkflowCard?> workflowCard(String serverId, String name) async {
    final query = database.select(database.workflowEntries)
      ..where((row) => row.serverId.equals(serverId) & row.name.equals(name))
      ..limit(1);
    final row = await query.getSingleOrNull();
    return row == null ? null : _workflowCardFromRow(row);
  }

  Future<Workspace?> workspaceDocument(String serverId, String name) async {
    final query = database.select(database.workspaceEntries)
      ..where((row) => row.serverId.equals(serverId) & row.name.equals(name))
      ..limit(1);
    final row = await query.getSingleOrNull();
    return row == null ? null : Workspace.fromBuffer(row.rawProtobuf);
  }

  Future<List<MobileDataRefreshWarning>> refresh({
    required GizClawClient client,
    required String endpoint,
    required String serverId,
  }) async {
    final workflows = await _allWorkflows(client);
    final workspaces = await _allWorkspaces(client);
    final refreshedAt = DateTime.now().toUtc();

    await database.transaction(() async {
      await database
          .into(database.servers)
          .insertOnConflictUpdate(
            ServersCompanion.insert(
              id: serverId,
              endpoint: endpoint,
              lastConnectedAt: Value(refreshedAt),
            ),
          );

      await database.batch((batch) {
        batch.insertAllOnConflictUpdate(
          database.workflowEntries,
          workflows.map((workflow) {
            return WorkflowEntriesCompanion.insert(
              serverId: serverId,
              name: workflow.metadata.name,
              description: workflow.metadata.description,
              driver: workflow.spec.driver.name,
              rawProtobuf: Uint8List.fromList(workflow.writeToBuffer()),
              refreshedAt: refreshedAt,
            );
          }).toList(),
        );
        batch.insertAllOnConflictUpdate(
          database.workspaceEntries,
          workspaces.map((workspace) {
            return WorkspaceEntriesCompanion.insert(
              serverId: serverId,
              name: workspace.name,
              workflowName: workspace.workflowName,
              createdAt: Value(_dateTimeOrNull(workspace.createdAt)),
              lastActiveAt: Value(_dateTimeOrNull(workspace.lastActiveAt)),
              updatedAt: Value(_dateTimeOrNull(workspace.updatedAt)),
              rawProtobuf: Uint8List.fromList(workspace.writeToBuffer()),
              refreshedAt: refreshedAt,
            );
          }).toList(),
        );
      });

      final workflowNames = workflows.map((item) => item.metadata.name).toSet();
      final workspaceNames = workspaces.map((item) => item.name).toSet();
      await (database.delete(database.workflowEntries)..where(
            (row) =>
                row.serverId.equals(serverId) & row.name.isNotIn(workflowNames),
          ))
          .go();
      await (database.delete(database.workspaceEntries)..where(
            (row) =>
                row.serverId.equals(serverId) &
                row.name.isNotIn(workspaceNames),
          ))
          .go();
      await database
          .into(database.syncStates)
          .insertOnConflictUpdate(
            SyncStatesCompanion.insert(
              serverId: serverId,
              scope: 'workflow-workspace-snapshot',
              lastSuccessfulRefreshAt: Value(refreshedAt),
            ),
          );
    });

    final warnings = <MobileDataRefreshWarning>[];
    try {
      await _replaceFriends(
        serverId: serverId,
        friends: await _allFriends(client),
        refreshedAt: refreshedAt,
      );
    } catch (error) {
      warnings.add(MobileDataRefreshWarning(scope: 'Friends', error: error));
    }
    try {
      await _replaceFriendGroups(
        serverId: serverId,
        groups: await _allFriendGroups(client),
        refreshedAt: refreshedAt,
      );
    } catch (error) {
      warnings.add(MobileDataRefreshWarning(scope: 'Groups', error: error));
    }
    return warnings;
  }

  Future<void> _replaceFriends({
    required String serverId,
    required List<FriendObject> friends,
    required DateTime refreshedAt,
  }) async {
    await database.transaction(() async {
      await database.batch((batch) {
        batch.insertAllOnConflictUpdate(
          database.friendEntries,
          friends.map((friend) {
            return FriendEntriesCompanion.insert(
              serverId: serverId,
              id: _friendKey(friend),
              peerPublicKey: friend.peerPublicKey,
              workspaceName: Value(
                friend.hasWorkspaceName() ? friend.workspaceName : null,
              ),
              rawProtobuf: Uint8List.fromList(friend.writeToBuffer()),
              refreshedAt: refreshedAt,
            );
          }).toList(),
        );
      });
      final friendIds = friends.map(_friendKey).toSet();
      await (database.delete(database.friendEntries)..where(
            (row) => row.serverId.equals(serverId) & row.id.isNotIn(friendIds),
          ))
          .go();
    });
  }

  Future<void> _replaceFriendGroups({
    required String serverId,
    required List<FriendGroupObject> groups,
    required DateTime refreshedAt,
  }) async {
    await database.transaction(() async {
      await database.batch((batch) {
        batch.insertAllOnConflictUpdate(
          database.friendGroupEntries,
          groups.map((group) {
            return FriendGroupEntriesCompanion.insert(
              serverId: serverId,
              id: _friendGroupKey(group),
              name: group.name,
              description: group.description,
              workspaceName: Value(
                group.hasWorkspaceName() ? group.workspaceName : null,
              ),
              rawProtobuf: Uint8List.fromList(group.writeToBuffer()),
              refreshedAt: refreshedAt,
            );
          }).toList(),
        );
      });
      final groupIds = groups.map(_friendGroupKey).toSet();
      await (database.delete(database.friendGroupEntries)..where(
            (row) => row.serverId.equals(serverId) & row.id.isNotIn(groupIds),
          ))
          .go();
    });
  }
}

Future<List<WorkflowDocument>> _allWorkflows(GizClawClient client) async {
  final items = <WorkflowDocument>[];
  String? cursor;
  do {
    final response = await client.listWorkflows(cursor: cursor, limit: 100);
    items.addAll(response.items);
    cursor = response.hasNext ? response.nextCursor : null;
  } while (cursor != null && cursor.isNotEmpty);
  return items;
}

Future<List<Workspace>> _allWorkspaces(GizClawClient client) async {
  final items = <Workspace>[];
  String? cursor;
  do {
    final response = await client.listWorkspaces(cursor: cursor, limit: 100);
    items.addAll(response.items);
    cursor = response.hasNext ? response.nextCursor : null;
  } while (cursor != null && cursor.isNotEmpty);
  return items;
}

Future<List<FriendObject>> _allFriends(GizClawClient client) async {
  final items = <FriendObject>[];
  String? cursor;
  do {
    final response = await client.listFriends(cursor: cursor, limit: 100);
    items.addAll(response.items);
    cursor = response.hasNext ? response.nextCursor : null;
  } while (cursor != null && cursor.isNotEmpty);
  return items.where((item) => _friendKey(item).isNotEmpty).toList();
}

Future<List<FriendGroupObject>> _allFriendGroups(GizClawClient client) async {
  final items = <FriendGroupObject>[];
  String? cursor;
  do {
    final response = await client.listFriendGroups(cursor: cursor, limit: 100);
    items.addAll(response.items);
    cursor = response.hasNext ? response.nextCursor : null;
  } while (cursor != null && cursor.isNotEmpty);
  return items.where((item) => _friendGroupKey(item).isNotEmpty).toList();
}

String _friendKey(FriendObject friend) {
  if (friend.id.trim().isNotEmpty) return friend.id.trim();
  if (friend.peerPublicKey.trim().isNotEmpty) {
    return friend.peerPublicKey.trim();
  }
  return friend.workspaceName.trim();
}

String _friendGroupKey(FriendGroupObject group) {
  if (group.id.trim().isNotEmpty) return group.id.trim();
  if (group.workspaceName.trim().isNotEmpty) return group.workspaceName.trim();
  return group.name.trim();
}

WorkflowCard _workflowCardFromRow(WorkflowEntry row) {
  return WorkflowCard.fromServer(
    name: row.name,
    description: row.description,
    driver: row.driver,
  );
}

WorkspaceCard _workspaceCardFromRow(WorkspaceEntry row) {
  final workspace = Workspace.fromBuffer(row.rawProtobuf);
  return WorkspaceCard(
    chatroomKind: _chatroomKind(workspace),
    name: row.name,
    workflowName: row.workflowName,
    lastActive: _relativeTime(
      row.lastActiveAt ?? row.updatedAt ?? row.createdAt,
    ),
  );
}

ChatroomWorkspaceKind? _chatroomKind(Workspace workspace) {
  if (!workspace.hasParameters() ||
      !workspace.parameters.hasChatRoomWorkspaceParameters()) {
    return null;
  }
  return switch (workspace.parameters.chatRoomWorkspaceParameters.mode) {
    ChatRoomMode.CHAT_ROOM_MODE_DIRECT => ChatroomWorkspaceKind.direct,
    ChatRoomMode.CHAT_ROOM_MODE_GROUP => ChatroomWorkspaceKind.group,
    _ => null,
  };
}

DateTime? _dateTimeOrNull(String value) {
  if (value.isEmpty) return null;
  return DateTime.tryParse(value)?.toUtc();
}

String _relativeTime(DateTime? value) {
  if (value == null) return 'Never opened';
  final elapsed = DateTime.now().toUtc().difference(value.toUtc());
  if (elapsed.isNegative || elapsed.inMinutes < 1) return 'Just now';
  if (elapsed.inHours < 1) return '${elapsed.inMinutes} min ago';
  if (elapsed.inDays < 1) return '${elapsed.inHours} hr ago';
  if (elapsed.inDays == 1) return 'Yesterday';
  return '${elapsed.inDays} days ago';
}
