import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sqlite3/sqlite3.dart';

import 'package:gizclaw_app/data/database/app_database.dart';

void main() {
  test('migrates a v1 database without adding gear_id twice', () async {
    final database = AppDatabase.forTesting(
      NativeDatabase.memory(setup: _createVersionOneSchema),
    );
    addTearDown(database.close);

    final columns = await database
        .customSelect('PRAGMA table_info(workspace_chat_entries)')
        .get();
    final gearColumns = columns
        .where((row) => row.read<String>('name') == 'gear_id')
        .toList();

    expect(gearColumns, hasLength(1));
  });
}

void _createVersionOneSchema(Database database) {
  database
    ..execute('''
      CREATE TABLE servers (
        id TEXT NOT NULL PRIMARY KEY,
        endpoint TEXT NOT NULL,
        last_connected_at INTEGER
      )
    ''')
    ..execute('''
      CREATE TABLE workspace_entries (
        server_id TEXT NOT NULL,
        name TEXT NOT NULL,
        workflow_name TEXT NOT NULL,
        created_at INTEGER,
        last_active_at INTEGER,
        updated_at INTEGER,
        raw_protobuf BLOB NOT NULL,
        refreshed_at INTEGER NOT NULL,
        PRIMARY KEY (server_id, name)
      )
    ''')
    ..execute('''
      CREATE TABLE sync_states (
        server_id TEXT NOT NULL,
        scope TEXT NOT NULL,
        cursor TEXT,
        last_successful_refresh_at INTEGER,
        PRIMARY KEY (server_id, scope)
      )
    ''')
    ..userVersion = 1;
}
