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

  Stream<List<WorkflowCard>> watchWorkflows(
    String serverId, {
    required String locale,
  }) {
    final query = database.select(database.workflowEntries)
      ..where((row) => row.serverId.equals(serverId))
      ..orderBy([(row) => OrderingTerm.asc(row.name)]);
    return query.watch().map(
      (rows) => rows
          .map((row) => _workflowCardFromRow(row, locale))
          .toList(growable: false),
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

  Future<WorkflowCard?> workflowCard(
    String serverId,
    String name, {
    required String locale,
  }) async {
    final query = database.select(database.workflowEntries)
      ..where((row) => row.serverId.equals(serverId) & row.name.equals(name))
      ..limit(1);
    final row = await query.getSingleOrNull();
    return row == null ? null : _workflowCardFromRow(row, locale);
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
    required bool Function() isCurrent,
    required String locale,
    required String serverId,
    required WorkflowLocale workflowLocale,
  }) async {
    final workflows = await _allWorkflows(client, workflowLocale);
    final workspaces = await _allWorkspaces(client);
    if (!isCurrent()) return const [];
    final workflowIcons = await _workflowIcons(client, workflows, isCurrent);
    if (!isCurrent()) return const [];
    final refreshedAt = DateTime.now().toUtc();

    try {
      await database.transaction(() async {
        _requireCurrent(isCurrent);
        await database
            .into(database.servers)
            .insertOnConflictUpdate(
              ServersCompanion.insert(
                id: serverId,
                endpoint: endpoint,
                lastConnectedAt: Value(refreshedAt),
              ),
            );

        _requireCurrent(isCurrent);
        await database.batch((batch) {
          batch.insertAllOnConflictUpdate(
            database.workflowEntries,
            workflows.map((workflow) {
              final catalog = _workflowCatalog(workflow);
              return WorkflowEntriesCompanion.insert(
                serverId: serverId,
                name: workflow.name,
                locale: Value(locale),
                description: catalog?.description.trim() ?? '',
                driver: workflow.spec.driver.name,
                iconPng: Value(workflowIcons[workflow.name]),
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

        _requireCurrent(isCurrent);
        final workflowNames = workflows.map((item) => item.name).toSet();
        final workspaceNames = workspaces.map((item) => item.name).toSet();
        await (database.delete(database.workflowEntries)..where(
              (row) =>
                  row.serverId.equals(serverId) &
                  row.name.isNotIn(workflowNames),
            ))
            .go();
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
                scope: 'workflow-workspace-snapshot',
                lastSuccessfulRefreshAt: Value(refreshedAt),
              ),
            );
      });
    } on _StaleRefresh {
      return const [];
    }

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

class _StaleRefresh implements Exception {
  const _StaleRefresh();
}

void _requireCurrent(bool Function() isCurrent) {
  if (!isCurrent()) throw const _StaleRefresh();
}

Future<List<Workflow>> _allWorkflows(
  GizClawClient client,
  WorkflowLocale lang,
) async {
  final items = <Workflow>[];
  String? cursor;
  do {
    final response = await client.listWorkflows(
      cursor: cursor,
      limit: 100,
      lang: lang,
    );
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

WorkflowCard _workflowCardFromRow(WorkflowEntry row, String locale) {
  final workflow = Workflow.fromBuffer(row.rawProtobuf);
  final catalog = row.locale == locale ? _workflowCatalog(workflow) : null;
  final localizedName = catalog?.name.trim();
  return WorkflowCard.fromServer(
    name: row.name,
    displayName: localizedName == null || localizedName.isEmpty
        ? row.name
        : localizedName,
    description: catalog?.description.trim() ?? '',
    driver: row.driver,
    iconPng: row.iconPng,
  );
}

Future<Map<String, Uint8List>> _workflowIcons(
  GizClawClient client,
  List<Workflow> workflows,
  bool Function() isCurrent,
) async {
  final icons = <String, Uint8List>{};
  for (final workflow in workflows) {
    _requireCurrent(isCurrent);
    if (!workflow.hasIcon() || !workflow.icon.hasPng()) continue;
    try {
      final result = await client.downloadWorkflowIcon(
        workflow.name,
        IconFormat.ICON_FORMAT_PNG,
      );
      _requireCurrent(isCurrent);
      if (result.bytes.isNotEmpty) icons[workflow.name] = result.bytes;
    } on _StaleRefresh {
      rethrow;
    } catch (_) {
      // Icon loading is best effort. The card keeps its owner-provided text and
      // falls back to the driver placeholder when download or decode fails.
    }
  }
  return icons;
}

WorkflowI18nCatalog? _workflowCatalog(Workflow workflow) =>
    workflow.hasI18n() ? workflow.i18n : null;

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
