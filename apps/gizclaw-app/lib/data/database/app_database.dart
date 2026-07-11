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

@DriftDatabase(tables: [Servers, WorkflowEntries, WorkspaceEntries, SyncStates])
class AppDatabase extends _$AppDatabase {
  AppDatabase() : super(driftDatabase(name: 'gizclaw_mobile_cache'));

  AppDatabase.forTesting(super.executor);

  @override
  int get schemaVersion => 1;
}
