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

  test('resets id-shaped Peer caches when upgrading a v9 database', () async {
    final database = AppDatabase.forTesting(
      NativeDatabase.memory(setup: _createVersionNineSchema),
    );
    addTearDown(database.close);

    final historyColumns = await _columnNames(
      database,
      'workspace_chat_entries',
    );
    final friendColumns = await _columnNames(database, 'friend_entries');
    final groupColumns = await _columnNames(database, 'friend_group_entries');

    expect(historyColumns, contains('history_name'));
    expect(historyColumns, isNot(contains('history_id')));
    expect(friendColumns, contains('name'));
    expect(friendColumns, isNot(contains('id')));
    expect(groupColumns, containsAll(<String>['name', 'display_name']));
    expect(groupColumns, isNot(contains('id')));
    expect(await database.select(database.workspaceChatEntries).get(), isEmpty);
    expect(await database.select(database.friendEntries).get(), isEmpty);
    expect(await database.select(database.friendGroupEntries).get(), isEmpty);
  });
}

Future<Set<String>> _columnNames(AppDatabase database, String table) async {
  final columns = await database
      .customSelect('PRAGMA table_info($table)')
      .get();
  return columns.map((row) => row.read<String>('name')).toSet();
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

void _createVersionNineSchema(Database database) {
  database
    ..execute('''
      CREATE TABLE workspace_chat_entries (
        server_id TEXT NOT NULL,
        workspace_name TEXT NOT NULL,
        history_id TEXT NOT NULL,
        role TEXT NOT NULL,
        gear_id TEXT,
        content TEXT NOT NULL,
        name TEXT NOT NULL,
        created_at INTEGER,
        refreshed_at INTEGER NOT NULL,
        PRIMARY KEY (server_id, workspace_name, history_id)
      )
    ''')
    ..execute('''
      CREATE TABLE friend_entries (
        server_id TEXT NOT NULL,
        id TEXT NOT NULL,
        peer_public_key TEXT NOT NULL,
        workspace_name TEXT,
        raw_protobuf BLOB NOT NULL,
        refreshed_at INTEGER NOT NULL,
        PRIMARY KEY (server_id, id)
      )
    ''')
    ..execute('''
      CREATE TABLE friend_group_entries (
        server_id TEXT NOT NULL,
        id TEXT NOT NULL,
        name TEXT NOT NULL,
        description TEXT NOT NULL,
        workspace_name TEXT,
        raw_protobuf BLOB NOT NULL,
        refreshed_at INTEGER NOT NULL,
        PRIMARY KEY (server_id, id)
      )
    ''')
    ..execute('''
      INSERT INTO workspace_chat_entries (
        server_id, workspace_name, history_id, role, content, name, refreshed_at
      ) VALUES ('server-a', 'workspace-a', 'history-a', 'assistant', 'hello', 'actor-a', 1)
    ''')
    ..execute('''
      INSERT INTO friend_entries (
        server_id, id, peer_public_key, raw_protobuf, refreshed_at
      ) VALUES ('server-a', 'friend-id-a', 'peer-a', X'00', 1)
    ''')
    ..execute('''
      INSERT INTO friend_group_entries (
        server_id, id, name, description, raw_protobuf, refreshed_at
      ) VALUES ('server-a', 'group-id-a', 'Group A', '', X'00', 1)
    ''')
    ..userVersion = 9;
}
