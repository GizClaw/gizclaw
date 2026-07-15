import 'package:drift/drift.dart';
import 'package:drift_flutter/drift_flutter.dart';

part 'app_database.g.dart';

class Servers extends Table {
  TextColumn get id => text()();
  TextColumn get endpoint => text()();
  DateTimeColumn get lastConnectedAt => dateTime().nullable()();

  @override
  Set<Column<Object>> get primaryKey => {id};
}

class WorkflowEntries extends Table {
  TextColumn get serverId => text()();
  TextColumn get name => text()();
  TextColumn get locale => text().nullable()();
  TextColumn get description => text()();
  TextColumn get driver => text()();
  BlobColumn get rawProtobuf => blob()();
  DateTimeColumn get refreshedAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {serverId, name};
}

class WorkspaceEntries extends Table {
  TextColumn get serverId => text()();
  TextColumn get name => text()();
  TextColumn get workflowName => text()();
  DateTimeColumn get createdAt => dateTime().nullable()();
  DateTimeColumn get lastActiveAt => dateTime().nullable()();
  DateTimeColumn get updatedAt => dateTime().nullable()();
  BlobColumn get rawProtobuf => blob()();
  DateTimeColumn get refreshedAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {serverId, name};
}

class SyncStates extends Table {
  TextColumn get serverId => text()();
  TextColumn get scope => text()();
  TextColumn get cursor => text().nullable()();
  DateTimeColumn get lastSuccessfulRefreshAt => dateTime().nullable()();

  @override
  Set<Column<Object>> get primaryKey => {serverId, scope};
}

class WorkspaceChatEntries extends Table {
  TextColumn get serverId => text()();
  TextColumn get workspaceName => text()();
  TextColumn get historyId => text()();
  TextColumn get role => text()();
  TextColumn get content => text()();
  TextColumn get name => text()();
  DateTimeColumn get createdAt => dateTime().nullable()();
  DateTimeColumn get refreshedAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {serverId, workspaceName, historyId};
}

class FriendEntries extends Table {
  TextColumn get serverId => text()();
  TextColumn get id => text()();
  TextColumn get peerPublicKey => text()();
  TextColumn get workspaceName => text().nullable()();
  BlobColumn get rawProtobuf => blob()();
  DateTimeColumn get refreshedAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {serverId, id};
}

class FriendGroupEntries extends Table {
  TextColumn get serverId => text()();
  TextColumn get id => text()();
  TextColumn get name => text()();
  TextColumn get description => text()();
  TextColumn get workspaceName => text().nullable()();
  BlobColumn get rawProtobuf => blob()();
  DateTimeColumn get refreshedAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {serverId, id};
}

@DriftDatabase(
  tables: [
    Servers,
    WorkflowEntries,
    WorkspaceEntries,
    SyncStates,
    WorkspaceChatEntries,
    FriendEntries,
    FriendGroupEntries,
  ],
)
class AppDatabase extends _$AppDatabase {
  AppDatabase() : super(driftDatabase(name: 'gizclaw_mobile_cache'));

  AppDatabase.forTesting(super.executor);

  @override
  int get schemaVersion => 4;

  @override
  MigrationStrategy get migration => MigrationStrategy(
    onCreate: (migrator) => migrator.createAll(),
    onUpgrade: (migrator, from, to) async {
      if (from < 2) {
        await migrator.createTable(workspaceChatEntries);
      }
      if (from < 3) {
        await migrator.createTable(friendEntries);
        await migrator.createTable(friendGroupEntries);
      }
      if (from < 4) {
        await migrator.addColumn(workflowEntries, workflowEntries.locale);
      }
    },
  );
}
