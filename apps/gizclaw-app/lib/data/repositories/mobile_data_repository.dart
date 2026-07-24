import 'package:drift/drift.dart';
import 'package:gizclaw/gizclaw.dart';

import '../../prototype/prototype_models.dart';
import '../../workflows/app_workflow_catalog.dart';
import '../database/app_database.dart';

class MobileDataRefreshWarning {
  const MobileDataRefreshWarning({required this.scope, required this.error});

  final Object error;
  final String scope;

  @override
  String toString() => '$scope refresh failed: $error';
}

class WorkspaceSnapshotResult {
  const WorkspaceSnapshotResult({
    required this.applied,
    required this.workspaceNames,
  });

  static const notApplied = WorkspaceSnapshotResult(
    applied: false,
    workspaceNames: <String>{},
  );

  final bool applied;
  final Set<String> workspaceNames;

  bool contains(String workspaceName) => workspaceNames.contains(workspaceName);
}

class MobileDataRepository {
  MobileDataRepository(this.database);

  final AppDatabase database;
  final Map<String, Map<String, FriendInfo>> _friendInfos = {};

  Future<String?> serverIdForEndpoint(String endpoint) async {
    final query = database.select(database.servers)
      ..where((row) => row.endpoint.equals(endpoint))
      ..limit(1);
    return (await query.getSingleOrNull())?.id;
  }

  Stream<List<WorkspaceCard>> watchWorkspaces(
    String serverId, {
    String? collection,
  }) {
    final query = database.select(database.workspaceEntries)
      ..where(
        (row) =>
            row.serverId.equals(serverId) &
            (collection == null
                ? const Constant(true)
                : row.collection.equals(collection)),
      )
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
    return query.watch().map((rows) {
      final infos = _friendInfos[serverId] ?? const <String, FriendInfo>{};
      return rows
          .where((row) => row.workspaceName?.isNotEmpty ?? false)
          .map((row) {
            final info = infos[row.id];
            return ChatroomWorkspaceMetadata(
              workspaceName: row.workspaceName!,
              title: (info?.name.trim().isNotEmpty ?? false)
                  ? info!.name
                  : row.id,
              emoji: info?.emoji ?? '',
              kind: ChatroomWorkspaceKind.direct,
              peerPublicKey: row.peerPublicKey,
              resourceId: row.id,
            );
          })
          .toList(growable: false);
    });
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
          .map((row) {
            final group = FriendGroupObject.fromBuffer(row.rawProtobuf);
            return ChatroomWorkspaceMetadata(
              workspaceName: row.workspaceName!,
              title: row.name.trim().isEmpty ? 'Group chat' : row.name,
              description: row.description,
              kind: ChatroomWorkspaceKind.group,
              resourceId: row.id,
              isGroupOwner:
                  group.myRole ==
                  FriendGroupMemberRole.FRIEND_GROUP_MEMBER_ROLE_OWNER,
            );
          })
          .toList(growable: false),
    );
  }

  Future<Workspace?> workspaceDocument(String serverId, String name) async {
    final query = database.select(database.workspaceEntries)
      ..where((row) => row.serverId.equals(serverId) & row.name.equals(name))
      ..limit(1);
    final row = await query.getSingleOrNull();
    return row == null ? null : Workspace.fromBuffer(row.rawProtobuf);
  }

  Future<void> deleteWorkspaceProjection(
    String serverId,
    String name, {
    required bool Function() isCurrent,
  }) async {
    try {
      await database.transaction(() async {
        _requireCurrent(isCurrent);
        await (database.delete(database.workspaceEntries)..where(
              (row) => row.serverId.equals(serverId) & row.name.equals(name),
            ))
            .go();
        _requireCurrent(isCurrent);
      });
    } on _StaleRefresh {
      return;
    }
  }

  Future<List<MobileDataRefreshWarning>> refresh({
    required GizClawClient client,
    required String endpoint,
    required bool Function() isCurrent,
    required String serverId,
  }) async {
    final warnings = <MobileDataRefreshWarning>[];
    try {
      await refreshWorkspaceSnapshot(
        client: client,
        endpoint: endpoint,
        isCurrent: isCurrent,
        serverId: serverId,
      );
    } catch (error) {
      warnings.add(MobileDataRefreshWarning(scope: 'Workspaces', error: error));
    }
    if (!isCurrent()) return warnings;

    final refreshedAt = DateTime.now().toUtc();
    try {
      final friends = await _allFriends(client);
      final infos = await _allFriendInfos(
        client,
        friends,
        previous: _friendInfos[serverId],
      );
      _requireCurrent(isCurrent);
      _friendInfos[serverId] = infos;
      await _replaceFriends(
        serverId: serverId,
        friends: friends,
        isCurrent: isCurrent,
        refreshedAt: refreshedAt,
      );
    } on _StaleRefresh {
      return warnings;
    } catch (error) {
      warnings.add(MobileDataRefreshWarning(scope: 'Friends', error: error));
    }
    if (!isCurrent()) return warnings;
    try {
      await _replaceFriendGroups(
        serverId: serverId,
        groups: await _allFriendGroups(client),
        isCurrent: isCurrent,
        refreshedAt: refreshedAt,
      );
    } on _StaleRefresh {
      return warnings;
    } catch (error) {
      warnings.add(MobileDataRefreshWarning(scope: 'Groups', error: error));
    }
    return warnings;
  }

  Future<WorkspaceSnapshotResult> refreshWorkspaceSnapshot({
    required GizClawClient client,
    required String endpoint,
    required bool Function() isCurrent,
    required String serverId,
  }) async {
    final workspaces = await _allWorkspaces(client);
    if (!isCurrent()) return WorkspaceSnapshotResult.notApplied;
    final refreshedAt = DateTime.now().toUtc();
    final workspaceNames = workspaces.map((item) => item.value.name).toSet();

    try {
      await database.transaction(() async {
        _requireCurrent(isCurrent);
        await _upsertServer(
          endpoint: endpoint,
          refreshedAt: refreshedAt,
          serverId: serverId,
        );
        _requireCurrent(isCurrent);
        await database.batch((batch) {
          batch.insertAllOnConflictUpdate(
            database.workspaceEntries,
            workspaces.map((item) {
              final workspace = item.value;
              return WorkspaceEntriesCompanion.insert(
                serverId: serverId,
                name: workspace.name,
                workflowAlias: workspace.workflowAlias,
                collection: Value(item.collection),
                createdAt: Value(_dateTimeOrNull(workspace.createdAt)),
                lastActiveAt: Value(_dateTimeOrNull(workspace.lastActiveAt)),
                updatedAt: Value(_dateTimeOrNull(workspace.updatedAt)),
                rawProtobuf: Uint8List.fromList(workspace.writeToBuffer()),
                refreshedAt: refreshedAt,
              );
            }).toList(),
          );
        });
        _requireCurrent(isCurrent);
        await (database.delete(database.workspaceEntries)..where(
              (row) =>
                  row.serverId.equals(serverId) &
                  row.name.isNotIn(workspaceNames),
            ))
            .go();
        _requireCurrent(isCurrent);
        await database
            .into(database.syncStates)
            .insertOnConflictUpdate(
              SyncStatesCompanion.insert(
                serverId: serverId,
                scope: 'workspace-snapshot',
                lastSuccessfulRefreshAt: Value(refreshedAt),
              ),
            );
      });
    } on _StaleRefresh {
      return WorkspaceSnapshotResult.notApplied;
    }

    return WorkspaceSnapshotResult(
      applied: true,
      workspaceNames: Set.unmodifiable(workspaceNames),
    );
  }

  Future<void> _upsertServer({
    required String endpoint,
    required DateTime refreshedAt,
    required String serverId,
  }) => database
      .into(database.servers)
      .insertOnConflictUpdate(
        ServersCompanion.insert(
          id: serverId,
          endpoint: endpoint,
          lastConnectedAt: Value(refreshedAt),
        ),
      );

  Future<void> _replaceFriends({
    required String serverId,
    required List<FriendObject> friends,
    required bool Function() isCurrent,
    required DateTime refreshedAt,
  }) async {
    await database.transaction(() async {
      _requireCurrent(isCurrent);
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
      _requireCurrent(isCurrent);
      final friendIds = friends.map(_friendKey).toSet();
      await (database.delete(database.friendEntries)..where(
            (row) => row.serverId.equals(serverId) & row.id.isNotIn(friendIds),
          ))
          .go();
      _requireCurrent(isCurrent);
    });
  }

  Future<void> _replaceFriendGroups({
    required String serverId,
    required List<FriendGroupObject> groups,
    required bool Function() isCurrent,
    required DateTime refreshedAt,
  }) async {
    await database.transaction(() async {
      _requireCurrent(isCurrent);
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
      _requireCurrent(isCurrent);
      final groupIds = groups.map(_friendGroupKey).toSet();
      await (database.delete(database.friendGroupEntries)..where(
            (row) => row.serverId.equals(serverId) & row.id.isNotIn(groupIds),
          ))
          .go();
      _requireCurrent(isCurrent);
    });
  }
}

class _StaleRefresh implements Exception {
  const _StaleRefresh();
}

void _requireCurrent(bool Function() isCurrent) {
  if (!isCurrent()) throw const _StaleRefresh();
}

Future<List<({String collection, Workspace value})>> _allWorkspaces(
  GizClawClient client,
) async {
  final items = <({String collection, Workspace value})>[];
  String? profileName;
  String? profileRevision;
  for (final collection in appWorkflowCollections) {
    String? cursor;
    do {
      late final WorkspaceListResponse response;
      try {
        response = await client.listWorkspaces(
          collection: collection.id,
          cursor: cursor,
          limit: 100,
        );
      } on RpcError catch (error) {
        if (error.code == 404 && cursor == null) break;
        rethrow;
      }
      profileName ??= response.runtimeProfileName;
      profileRevision ??= response.runtimeProfileRevision;
      if (response.runtimeProfileName != profileName ||
          response.runtimeProfileRevision != profileRevision) {
        throw StateError('Runtime profile changed while loading workspaces');
      }
      items.addAll(
        response.items.map(
          (workspace) => (collection: collection.id, value: workspace),
        ),
      );
      cursor = response.hasNext ? response.nextCursor : null;
    } while (cursor != null && cursor.isNotEmpty);
  }
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

Future<Map<String, FriendInfo>> _allFriendInfos(
  GizClawClient client,
  List<FriendObject> friends, {
  Map<String, FriendInfo>? previous,
}) async {
  final infos = <String, FriendInfo>{...?previous};
  for (final friend in friends) {
    final id = _friendKey(friend);
    try {
      final response = await client.getFriendInfo(id);
      if (response.hasValue()) infos[id] = response.value;
    } catch (_) {
      // Keep the last cached profile when a single friend lookup is transiently unavailable.
    }
  }
  return infos;
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

WorkspaceCard _workspaceCardFromRow(WorkspaceEntry row) {
  final workspace = Workspace.fromBuffer(row.rawProtobuf);
  return WorkspaceCard(
    chatroomKind: _chatroomKind(workspace),
    name: row.name,
    workflowAlias: row.workflowAlias,
    collection: row.collection,
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
