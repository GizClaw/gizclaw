// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'app_database.dart';

// ignore_for_file: type=lint
class $ServersTable extends Servers with TableInfo<$ServersTable, Server> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $ServersTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _endpointMeta = const VerificationMeta(
    'endpoint',
  );
  @override
  late final GeneratedColumn<String> endpoint = GeneratedColumn<String>(
    'endpoint',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _lastConnectedAtMeta = const VerificationMeta(
    'lastConnectedAt',
  );
  @override
  late final GeneratedColumn<DateTime> lastConnectedAt =
      GeneratedColumn<DateTime>(
        'last_connected_at',
        aliasedName,
        true,
        type: DriftSqlType.dateTime,
        requiredDuringInsert: false,
      );
  @override
  List<GeneratedColumn> get $columns => [id, endpoint, lastConnectedAt];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'servers';
  @override
  VerificationContext validateIntegrity(
    Insertable<Server> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('endpoint')) {
      context.handle(
        _endpointMeta,
        endpoint.isAcceptableOrUnknown(data['endpoint']!, _endpointMeta),
      );
    } else if (isInserting) {
      context.missing(_endpointMeta);
    }
    if (data.containsKey('last_connected_at')) {
      context.handle(
        _lastConnectedAtMeta,
        lastConnectedAt.isAcceptableOrUnknown(
          data['last_connected_at']!,
          _lastConnectedAtMeta,
        ),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  Server map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return Server(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      endpoint: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}endpoint'],
      )!,
      lastConnectedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}last_connected_at'],
      ),
    );
  }

  @override
  $ServersTable createAlias(String alias) {
    return $ServersTable(attachedDatabase, alias);
  }
}

class Server extends DataClass implements Insertable<Server> {
  final String id;
  final String endpoint;
  final DateTime? lastConnectedAt;
  const Server({
    required this.id,
    required this.endpoint,
    this.lastConnectedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    map['endpoint'] = Variable<String>(endpoint);
    if (!nullToAbsent || lastConnectedAt != null) {
      map['last_connected_at'] = Variable<DateTime>(lastConnectedAt);
    }
    return map;
  }

  ServersCompanion toCompanion(bool nullToAbsent) {
    return ServersCompanion(
      id: Value(id),
      endpoint: Value(endpoint),
      lastConnectedAt: lastConnectedAt == null && nullToAbsent
          ? const Value.absent()
          : Value(lastConnectedAt),
    );
  }

  factory Server.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return Server(
      id: serializer.fromJson<String>(json['id']),
      endpoint: serializer.fromJson<String>(json['endpoint']),
      lastConnectedAt: serializer.fromJson<DateTime?>(json['lastConnectedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'endpoint': serializer.toJson<String>(endpoint),
      'lastConnectedAt': serializer.toJson<DateTime?>(lastConnectedAt),
    };
  }

  Server copyWith({
    String? id,
    String? endpoint,
    Value<DateTime?> lastConnectedAt = const Value.absent(),
  }) => Server(
    id: id ?? this.id,
    endpoint: endpoint ?? this.endpoint,
    lastConnectedAt: lastConnectedAt.present
        ? lastConnectedAt.value
        : this.lastConnectedAt,
  );
  Server copyWithCompanion(ServersCompanion data) {
    return Server(
      id: data.id.present ? data.id.value : this.id,
      endpoint: data.endpoint.present ? data.endpoint.value : this.endpoint,
      lastConnectedAt: data.lastConnectedAt.present
          ? data.lastConnectedAt.value
          : this.lastConnectedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('Server(')
          ..write('id: $id, ')
          ..write('endpoint: $endpoint, ')
          ..write('lastConnectedAt: $lastConnectedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(id, endpoint, lastConnectedAt);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is Server &&
          other.id == this.id &&
          other.endpoint == this.endpoint &&
          other.lastConnectedAt == this.lastConnectedAt);
}

class ServersCompanion extends UpdateCompanion<Server> {
  final Value<String> id;
  final Value<String> endpoint;
  final Value<DateTime?> lastConnectedAt;
  final Value<int> rowid;
  const ServersCompanion({
    this.id = const Value.absent(),
    this.endpoint = const Value.absent(),
    this.lastConnectedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  ServersCompanion.insert({
    required String id,
    required String endpoint,
    this.lastConnectedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       endpoint = Value(endpoint);
  static Insertable<Server> custom({
    Expression<String>? id,
    Expression<String>? endpoint,
    Expression<DateTime>? lastConnectedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (endpoint != null) 'endpoint': endpoint,
      if (lastConnectedAt != null) 'last_connected_at': lastConnectedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  ServersCompanion copyWith({
    Value<String>? id,
    Value<String>? endpoint,
    Value<DateTime?>? lastConnectedAt,
    Value<int>? rowid,
  }) {
    return ServersCompanion(
      id: id ?? this.id,
      endpoint: endpoint ?? this.endpoint,
      lastConnectedAt: lastConnectedAt ?? this.lastConnectedAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (endpoint.present) {
      map['endpoint'] = Variable<String>(endpoint.value);
    }
    if (lastConnectedAt.present) {
      map['last_connected_at'] = Variable<DateTime>(lastConnectedAt.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('ServersCompanion(')
          ..write('id: $id, ')
          ..write('endpoint: $endpoint, ')
          ..write('lastConnectedAt: $lastConnectedAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $WorkflowEntriesTable extends WorkflowEntries
    with TableInfo<$WorkflowEntriesTable, WorkflowEntry> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $WorkflowEntriesTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _serverIdMeta = const VerificationMeta(
    'serverId',
  );
  @override
  late final GeneratedColumn<String> serverId = GeneratedColumn<String>(
    'server_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _nameMeta = const VerificationMeta('name');
  @override
  late final GeneratedColumn<String> name = GeneratedColumn<String>(
    'name',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _descriptionMeta = const VerificationMeta(
    'description',
  );
  @override
  late final GeneratedColumn<String> description = GeneratedColumn<String>(
    'description',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _driverMeta = const VerificationMeta('driver');
  @override
  late final GeneratedColumn<String> driver = GeneratedColumn<String>(
    'driver',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _rawProtobufMeta = const VerificationMeta(
    'rawProtobuf',
  );
  @override
  late final GeneratedColumn<Uint8List> rawProtobuf =
      GeneratedColumn<Uint8List>(
        'raw_protobuf',
        aliasedName,
        false,
        type: DriftSqlType.blob,
        requiredDuringInsert: true,
      );
  static const VerificationMeta _refreshedAtMeta = const VerificationMeta(
    'refreshedAt',
  );
  @override
  late final GeneratedColumn<DateTime> refreshedAt = GeneratedColumn<DateTime>(
    'refreshed_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [
    serverId,
    name,
    description,
    driver,
    rawProtobuf,
    refreshedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'workflow_entries';
  @override
  VerificationContext validateIntegrity(
    Insertable<WorkflowEntry> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('server_id')) {
      context.handle(
        _serverIdMeta,
        serverId.isAcceptableOrUnknown(data['server_id']!, _serverIdMeta),
      );
    } else if (isInserting) {
      context.missing(_serverIdMeta);
    }
    if (data.containsKey('name')) {
      context.handle(
        _nameMeta,
        name.isAcceptableOrUnknown(data['name']!, _nameMeta),
      );
    } else if (isInserting) {
      context.missing(_nameMeta);
    }
    if (data.containsKey('description')) {
      context.handle(
        _descriptionMeta,
        description.isAcceptableOrUnknown(
          data['description']!,
          _descriptionMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_descriptionMeta);
    }
    if (data.containsKey('driver')) {
      context.handle(
        _driverMeta,
        driver.isAcceptableOrUnknown(data['driver']!, _driverMeta),
      );
    } else if (isInserting) {
      context.missing(_driverMeta);
    }
    if (data.containsKey('raw_protobuf')) {
      context.handle(
        _rawProtobufMeta,
        rawProtobuf.isAcceptableOrUnknown(
          data['raw_protobuf']!,
          _rawProtobufMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_rawProtobufMeta);
    }
    if (data.containsKey('refreshed_at')) {
      context.handle(
        _refreshedAtMeta,
        refreshedAt.isAcceptableOrUnknown(
          data['refreshed_at']!,
          _refreshedAtMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_refreshedAtMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {serverId, name};
  @override
  WorkflowEntry map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return WorkflowEntry(
      serverId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}server_id'],
      )!,
      name: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}name'],
      )!,
      description: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}description'],
      )!,
      driver: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}driver'],
      )!,
      rawProtobuf: attachedDatabase.typeMapping.read(
        DriftSqlType.blob,
        data['${effectivePrefix}raw_protobuf'],
      )!,
      refreshedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}refreshed_at'],
      )!,
    );
  }

  @override
  $WorkflowEntriesTable createAlias(String alias) {
    return $WorkflowEntriesTable(attachedDatabase, alias);
  }
}

class WorkflowEntry extends DataClass implements Insertable<WorkflowEntry> {
  final String serverId;
  final String name;
  final String description;
  final String driver;
  final Uint8List rawProtobuf;
  final DateTime refreshedAt;
  const WorkflowEntry({
    required this.serverId,
    required this.name,
    required this.description,
    required this.driver,
    required this.rawProtobuf,
    required this.refreshedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['server_id'] = Variable<String>(serverId);
    map['name'] = Variable<String>(name);
    map['description'] = Variable<String>(description);
    map['driver'] = Variable<String>(driver);
    map['raw_protobuf'] = Variable<Uint8List>(rawProtobuf);
    map['refreshed_at'] = Variable<DateTime>(refreshedAt);
    return map;
  }

  WorkflowEntriesCompanion toCompanion(bool nullToAbsent) {
    return WorkflowEntriesCompanion(
      serverId: Value(serverId),
      name: Value(name),
      description: Value(description),
      driver: Value(driver),
      rawProtobuf: Value(rawProtobuf),
      refreshedAt: Value(refreshedAt),
    );
  }

  factory WorkflowEntry.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return WorkflowEntry(
      serverId: serializer.fromJson<String>(json['serverId']),
      name: serializer.fromJson<String>(json['name']),
      description: serializer.fromJson<String>(json['description']),
      driver: serializer.fromJson<String>(json['driver']),
      rawProtobuf: serializer.fromJson<Uint8List>(json['rawProtobuf']),
      refreshedAt: serializer.fromJson<DateTime>(json['refreshedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'serverId': serializer.toJson<String>(serverId),
      'name': serializer.toJson<String>(name),
      'description': serializer.toJson<String>(description),
      'driver': serializer.toJson<String>(driver),
      'rawProtobuf': serializer.toJson<Uint8List>(rawProtobuf),
      'refreshedAt': serializer.toJson<DateTime>(refreshedAt),
    };
  }

  WorkflowEntry copyWith({
    String? serverId,
    String? name,
    String? description,
    String? driver,
    Uint8List? rawProtobuf,
    DateTime? refreshedAt,
  }) => WorkflowEntry(
    serverId: serverId ?? this.serverId,
    name: name ?? this.name,
    description: description ?? this.description,
    driver: driver ?? this.driver,
    rawProtobuf: rawProtobuf ?? this.rawProtobuf,
    refreshedAt: refreshedAt ?? this.refreshedAt,
  );
  WorkflowEntry copyWithCompanion(WorkflowEntriesCompanion data) {
    return WorkflowEntry(
      serverId: data.serverId.present ? data.serverId.value : this.serverId,
      name: data.name.present ? data.name.value : this.name,
      description: data.description.present
          ? data.description.value
          : this.description,
      driver: data.driver.present ? data.driver.value : this.driver,
      rawProtobuf: data.rawProtobuf.present
          ? data.rawProtobuf.value
          : this.rawProtobuf,
      refreshedAt: data.refreshedAt.present
          ? data.refreshedAt.value
          : this.refreshedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('WorkflowEntry(')
          ..write('serverId: $serverId, ')
          ..write('name: $name, ')
          ..write('description: $description, ')
          ..write('driver: $driver, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    serverId,
    name,
    description,
    driver,
    $driftBlobEquality.hash(rawProtobuf),
    refreshedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is WorkflowEntry &&
          other.serverId == this.serverId &&
          other.name == this.name &&
          other.description == this.description &&
          other.driver == this.driver &&
          $driftBlobEquality.equals(other.rawProtobuf, this.rawProtobuf) &&
          other.refreshedAt == this.refreshedAt);
}

class WorkflowEntriesCompanion extends UpdateCompanion<WorkflowEntry> {
  final Value<String> serverId;
  final Value<String> name;
  final Value<String> description;
  final Value<String> driver;
  final Value<Uint8List> rawProtobuf;
  final Value<DateTime> refreshedAt;
  final Value<int> rowid;
  const WorkflowEntriesCompanion({
    this.serverId = const Value.absent(),
    this.name = const Value.absent(),
    this.description = const Value.absent(),
    this.driver = const Value.absent(),
    this.rawProtobuf = const Value.absent(),
    this.refreshedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  WorkflowEntriesCompanion.insert({
    required String serverId,
    required String name,
    required String description,
    required String driver,
    required Uint8List rawProtobuf,
    required DateTime refreshedAt,
    this.rowid = const Value.absent(),
  }) : serverId = Value(serverId),
       name = Value(name),
       description = Value(description),
       driver = Value(driver),
       rawProtobuf = Value(rawProtobuf),
       refreshedAt = Value(refreshedAt);
  static Insertable<WorkflowEntry> custom({
    Expression<String>? serverId,
    Expression<String>? name,
    Expression<String>? description,
    Expression<String>? driver,
    Expression<Uint8List>? rawProtobuf,
    Expression<DateTime>? refreshedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (serverId != null) 'server_id': serverId,
      if (name != null) 'name': name,
      if (description != null) 'description': description,
      if (driver != null) 'driver': driver,
      if (rawProtobuf != null) 'raw_protobuf': rawProtobuf,
      if (refreshedAt != null) 'refreshed_at': refreshedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  WorkflowEntriesCompanion copyWith({
    Value<String>? serverId,
    Value<String>? name,
    Value<String>? description,
    Value<String>? driver,
    Value<Uint8List>? rawProtobuf,
    Value<DateTime>? refreshedAt,
    Value<int>? rowid,
  }) {
    return WorkflowEntriesCompanion(
      serverId: serverId ?? this.serverId,
      name: name ?? this.name,
      description: description ?? this.description,
      driver: driver ?? this.driver,
      rawProtobuf: rawProtobuf ?? this.rawProtobuf,
      refreshedAt: refreshedAt ?? this.refreshedAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (serverId.present) {
      map['server_id'] = Variable<String>(serverId.value);
    }
    if (name.present) {
      map['name'] = Variable<String>(name.value);
    }
    if (description.present) {
      map['description'] = Variable<String>(description.value);
    }
    if (driver.present) {
      map['driver'] = Variable<String>(driver.value);
    }
    if (rawProtobuf.present) {
      map['raw_protobuf'] = Variable<Uint8List>(rawProtobuf.value);
    }
    if (refreshedAt.present) {
      map['refreshed_at'] = Variable<DateTime>(refreshedAt.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('WorkflowEntriesCompanion(')
          ..write('serverId: $serverId, ')
          ..write('name: $name, ')
          ..write('description: $description, ')
          ..write('driver: $driver, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $WorkspaceEntriesTable extends WorkspaceEntries
    with TableInfo<$WorkspaceEntriesTable, WorkspaceEntry> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $WorkspaceEntriesTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _serverIdMeta = const VerificationMeta(
    'serverId',
  );
  @override
  late final GeneratedColumn<String> serverId = GeneratedColumn<String>(
    'server_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _nameMeta = const VerificationMeta('name');
  @override
  late final GeneratedColumn<String> name = GeneratedColumn<String>(
    'name',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _workflowNameMeta = const VerificationMeta(
    'workflowName',
  );
  @override
  late final GeneratedColumn<String> workflowName = GeneratedColumn<String>(
    'workflow_name',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _createdAtMeta = const VerificationMeta(
    'createdAt',
  );
  @override
  late final GeneratedColumn<DateTime> createdAt = GeneratedColumn<DateTime>(
    'created_at',
    aliasedName,
    true,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _lastActiveAtMeta = const VerificationMeta(
    'lastActiveAt',
  );
  @override
  late final GeneratedColumn<DateTime> lastActiveAt = GeneratedColumn<DateTime>(
    'last_active_at',
    aliasedName,
    true,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _updatedAtMeta = const VerificationMeta(
    'updatedAt',
  );
  @override
  late final GeneratedColumn<DateTime> updatedAt = GeneratedColumn<DateTime>(
    'updated_at',
    aliasedName,
    true,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _rawProtobufMeta = const VerificationMeta(
    'rawProtobuf',
  );
  @override
  late final GeneratedColumn<Uint8List> rawProtobuf =
      GeneratedColumn<Uint8List>(
        'raw_protobuf',
        aliasedName,
        false,
        type: DriftSqlType.blob,
        requiredDuringInsert: true,
      );
  static const VerificationMeta _refreshedAtMeta = const VerificationMeta(
    'refreshedAt',
  );
  @override
  late final GeneratedColumn<DateTime> refreshedAt = GeneratedColumn<DateTime>(
    'refreshed_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [
    serverId,
    name,
    workflowName,
    createdAt,
    lastActiveAt,
    updatedAt,
    rawProtobuf,
    refreshedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'workspace_entries';
  @override
  VerificationContext validateIntegrity(
    Insertable<WorkspaceEntry> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('server_id')) {
      context.handle(
        _serverIdMeta,
        serverId.isAcceptableOrUnknown(data['server_id']!, _serverIdMeta),
      );
    } else if (isInserting) {
      context.missing(_serverIdMeta);
    }
    if (data.containsKey('name')) {
      context.handle(
        _nameMeta,
        name.isAcceptableOrUnknown(data['name']!, _nameMeta),
      );
    } else if (isInserting) {
      context.missing(_nameMeta);
    }
    if (data.containsKey('workflow_name')) {
      context.handle(
        _workflowNameMeta,
        workflowName.isAcceptableOrUnknown(
          data['workflow_name']!,
          _workflowNameMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_workflowNameMeta);
    }
    if (data.containsKey('created_at')) {
      context.handle(
        _createdAtMeta,
        createdAt.isAcceptableOrUnknown(data['created_at']!, _createdAtMeta),
      );
    }
    if (data.containsKey('last_active_at')) {
      context.handle(
        _lastActiveAtMeta,
        lastActiveAt.isAcceptableOrUnknown(
          data['last_active_at']!,
          _lastActiveAtMeta,
        ),
      );
    }
    if (data.containsKey('updated_at')) {
      context.handle(
        _updatedAtMeta,
        updatedAt.isAcceptableOrUnknown(data['updated_at']!, _updatedAtMeta),
      );
    }
    if (data.containsKey('raw_protobuf')) {
      context.handle(
        _rawProtobufMeta,
        rawProtobuf.isAcceptableOrUnknown(
          data['raw_protobuf']!,
          _rawProtobufMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_rawProtobufMeta);
    }
    if (data.containsKey('refreshed_at')) {
      context.handle(
        _refreshedAtMeta,
        refreshedAt.isAcceptableOrUnknown(
          data['refreshed_at']!,
          _refreshedAtMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_refreshedAtMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {serverId, name};
  @override
  WorkspaceEntry map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return WorkspaceEntry(
      serverId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}server_id'],
      )!,
      name: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}name'],
      )!,
      workflowName: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}workflow_name'],
      )!,
      createdAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}created_at'],
      ),
      lastActiveAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}last_active_at'],
      ),
      updatedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}updated_at'],
      ),
      rawProtobuf: attachedDatabase.typeMapping.read(
        DriftSqlType.blob,
        data['${effectivePrefix}raw_protobuf'],
      )!,
      refreshedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}refreshed_at'],
      )!,
    );
  }

  @override
  $WorkspaceEntriesTable createAlias(String alias) {
    return $WorkspaceEntriesTable(attachedDatabase, alias);
  }
}

class WorkspaceEntry extends DataClass implements Insertable<WorkspaceEntry> {
  final String serverId;
  final String name;
  final String workflowName;
  final DateTime? createdAt;
  final DateTime? lastActiveAt;
  final DateTime? updatedAt;
  final Uint8List rawProtobuf;
  final DateTime refreshedAt;
  const WorkspaceEntry({
    required this.serverId,
    required this.name,
    required this.workflowName,
    this.createdAt,
    this.lastActiveAt,
    this.updatedAt,
    required this.rawProtobuf,
    required this.refreshedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['server_id'] = Variable<String>(serverId);
    map['name'] = Variable<String>(name);
    map['workflow_name'] = Variable<String>(workflowName);
    if (!nullToAbsent || createdAt != null) {
      map['created_at'] = Variable<DateTime>(createdAt);
    }
    if (!nullToAbsent || lastActiveAt != null) {
      map['last_active_at'] = Variable<DateTime>(lastActiveAt);
    }
    if (!nullToAbsent || updatedAt != null) {
      map['updated_at'] = Variable<DateTime>(updatedAt);
    }
    map['raw_protobuf'] = Variable<Uint8List>(rawProtobuf);
    map['refreshed_at'] = Variable<DateTime>(refreshedAt);
    return map;
  }

  WorkspaceEntriesCompanion toCompanion(bool nullToAbsent) {
    return WorkspaceEntriesCompanion(
      serverId: Value(serverId),
      name: Value(name),
      workflowName: Value(workflowName),
      createdAt: createdAt == null && nullToAbsent
          ? const Value.absent()
          : Value(createdAt),
      lastActiveAt: lastActiveAt == null && nullToAbsent
          ? const Value.absent()
          : Value(lastActiveAt),
      updatedAt: updatedAt == null && nullToAbsent
          ? const Value.absent()
          : Value(updatedAt),
      rawProtobuf: Value(rawProtobuf),
      refreshedAt: Value(refreshedAt),
    );
  }

  factory WorkspaceEntry.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return WorkspaceEntry(
      serverId: serializer.fromJson<String>(json['serverId']),
      name: serializer.fromJson<String>(json['name']),
      workflowName: serializer.fromJson<String>(json['workflowName']),
      createdAt: serializer.fromJson<DateTime?>(json['createdAt']),
      lastActiveAt: serializer.fromJson<DateTime?>(json['lastActiveAt']),
      updatedAt: serializer.fromJson<DateTime?>(json['updatedAt']),
      rawProtobuf: serializer.fromJson<Uint8List>(json['rawProtobuf']),
      refreshedAt: serializer.fromJson<DateTime>(json['refreshedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'serverId': serializer.toJson<String>(serverId),
      'name': serializer.toJson<String>(name),
      'workflowName': serializer.toJson<String>(workflowName),
      'createdAt': serializer.toJson<DateTime?>(createdAt),
      'lastActiveAt': serializer.toJson<DateTime?>(lastActiveAt),
      'updatedAt': serializer.toJson<DateTime?>(updatedAt),
      'rawProtobuf': serializer.toJson<Uint8List>(rawProtobuf),
      'refreshedAt': serializer.toJson<DateTime>(refreshedAt),
    };
  }

  WorkspaceEntry copyWith({
    String? serverId,
    String? name,
    String? workflowName,
    Value<DateTime?> createdAt = const Value.absent(),
    Value<DateTime?> lastActiveAt = const Value.absent(),
    Value<DateTime?> updatedAt = const Value.absent(),
    Uint8List? rawProtobuf,
    DateTime? refreshedAt,
  }) => WorkspaceEntry(
    serverId: serverId ?? this.serverId,
    name: name ?? this.name,
    workflowName: workflowName ?? this.workflowName,
    createdAt: createdAt.present ? createdAt.value : this.createdAt,
    lastActiveAt: lastActiveAt.present ? lastActiveAt.value : this.lastActiveAt,
    updatedAt: updatedAt.present ? updatedAt.value : this.updatedAt,
    rawProtobuf: rawProtobuf ?? this.rawProtobuf,
    refreshedAt: refreshedAt ?? this.refreshedAt,
  );
  WorkspaceEntry copyWithCompanion(WorkspaceEntriesCompanion data) {
    return WorkspaceEntry(
      serverId: data.serverId.present ? data.serverId.value : this.serverId,
      name: data.name.present ? data.name.value : this.name,
      workflowName: data.workflowName.present
          ? data.workflowName.value
          : this.workflowName,
      createdAt: data.createdAt.present ? data.createdAt.value : this.createdAt,
      lastActiveAt: data.lastActiveAt.present
          ? data.lastActiveAt.value
          : this.lastActiveAt,
      updatedAt: data.updatedAt.present ? data.updatedAt.value : this.updatedAt,
      rawProtobuf: data.rawProtobuf.present
          ? data.rawProtobuf.value
          : this.rawProtobuf,
      refreshedAt: data.refreshedAt.present
          ? data.refreshedAt.value
          : this.refreshedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('WorkspaceEntry(')
          ..write('serverId: $serverId, ')
          ..write('name: $name, ')
          ..write('workflowName: $workflowName, ')
          ..write('createdAt: $createdAt, ')
          ..write('lastActiveAt: $lastActiveAt, ')
          ..write('updatedAt: $updatedAt, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    serverId,
    name,
    workflowName,
    createdAt,
    lastActiveAt,
    updatedAt,
    $driftBlobEquality.hash(rawProtobuf),
    refreshedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is WorkspaceEntry &&
          other.serverId == this.serverId &&
          other.name == this.name &&
          other.workflowName == this.workflowName &&
          other.createdAt == this.createdAt &&
          other.lastActiveAt == this.lastActiveAt &&
          other.updatedAt == this.updatedAt &&
          $driftBlobEquality.equals(other.rawProtobuf, this.rawProtobuf) &&
          other.refreshedAt == this.refreshedAt);
}

class WorkspaceEntriesCompanion extends UpdateCompanion<WorkspaceEntry> {
  final Value<String> serverId;
  final Value<String> name;
  final Value<String> workflowName;
  final Value<DateTime?> createdAt;
  final Value<DateTime?> lastActiveAt;
  final Value<DateTime?> updatedAt;
  final Value<Uint8List> rawProtobuf;
  final Value<DateTime> refreshedAt;
  final Value<int> rowid;
  const WorkspaceEntriesCompanion({
    this.serverId = const Value.absent(),
    this.name = const Value.absent(),
    this.workflowName = const Value.absent(),
    this.createdAt = const Value.absent(),
    this.lastActiveAt = const Value.absent(),
    this.updatedAt = const Value.absent(),
    this.rawProtobuf = const Value.absent(),
    this.refreshedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  WorkspaceEntriesCompanion.insert({
    required String serverId,
    required String name,
    required String workflowName,
    this.createdAt = const Value.absent(),
    this.lastActiveAt = const Value.absent(),
    this.updatedAt = const Value.absent(),
    required Uint8List rawProtobuf,
    required DateTime refreshedAt,
    this.rowid = const Value.absent(),
  }) : serverId = Value(serverId),
       name = Value(name),
       workflowName = Value(workflowName),
       rawProtobuf = Value(rawProtobuf),
       refreshedAt = Value(refreshedAt);
  static Insertable<WorkspaceEntry> custom({
    Expression<String>? serverId,
    Expression<String>? name,
    Expression<String>? workflowName,
    Expression<DateTime>? createdAt,
    Expression<DateTime>? lastActiveAt,
    Expression<DateTime>? updatedAt,
    Expression<Uint8List>? rawProtobuf,
    Expression<DateTime>? refreshedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (serverId != null) 'server_id': serverId,
      if (name != null) 'name': name,
      if (workflowName != null) 'workflow_name': workflowName,
      if (createdAt != null) 'created_at': createdAt,
      if (lastActiveAt != null) 'last_active_at': lastActiveAt,
      if (updatedAt != null) 'updated_at': updatedAt,
      if (rawProtobuf != null) 'raw_protobuf': rawProtobuf,
      if (refreshedAt != null) 'refreshed_at': refreshedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  WorkspaceEntriesCompanion copyWith({
    Value<String>? serverId,
    Value<String>? name,
    Value<String>? workflowName,
    Value<DateTime?>? createdAt,
    Value<DateTime?>? lastActiveAt,
    Value<DateTime?>? updatedAt,
    Value<Uint8List>? rawProtobuf,
    Value<DateTime>? refreshedAt,
    Value<int>? rowid,
  }) {
    return WorkspaceEntriesCompanion(
      serverId: serverId ?? this.serverId,
      name: name ?? this.name,
      workflowName: workflowName ?? this.workflowName,
      createdAt: createdAt ?? this.createdAt,
      lastActiveAt: lastActiveAt ?? this.lastActiveAt,
      updatedAt: updatedAt ?? this.updatedAt,
      rawProtobuf: rawProtobuf ?? this.rawProtobuf,
      refreshedAt: refreshedAt ?? this.refreshedAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (serverId.present) {
      map['server_id'] = Variable<String>(serverId.value);
    }
    if (name.present) {
      map['name'] = Variable<String>(name.value);
    }
    if (workflowName.present) {
      map['workflow_name'] = Variable<String>(workflowName.value);
    }
    if (createdAt.present) {
      map['created_at'] = Variable<DateTime>(createdAt.value);
    }
    if (lastActiveAt.present) {
      map['last_active_at'] = Variable<DateTime>(lastActiveAt.value);
    }
    if (updatedAt.present) {
      map['updated_at'] = Variable<DateTime>(updatedAt.value);
    }
    if (rawProtobuf.present) {
      map['raw_protobuf'] = Variable<Uint8List>(rawProtobuf.value);
    }
    if (refreshedAt.present) {
      map['refreshed_at'] = Variable<DateTime>(refreshedAt.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('WorkspaceEntriesCompanion(')
          ..write('serverId: $serverId, ')
          ..write('name: $name, ')
          ..write('workflowName: $workflowName, ')
          ..write('createdAt: $createdAt, ')
          ..write('lastActiveAt: $lastActiveAt, ')
          ..write('updatedAt: $updatedAt, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $SyncStatesTable extends SyncStates
    with TableInfo<$SyncStatesTable, SyncState> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $SyncStatesTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _serverIdMeta = const VerificationMeta(
    'serverId',
  );
  @override
  late final GeneratedColumn<String> serverId = GeneratedColumn<String>(
    'server_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _scopeMeta = const VerificationMeta('scope');
  @override
  late final GeneratedColumn<String> scope = GeneratedColumn<String>(
    'scope',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _cursorMeta = const VerificationMeta('cursor');
  @override
  late final GeneratedColumn<String> cursor = GeneratedColumn<String>(
    'cursor',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _lastSuccessfulRefreshAtMeta =
      const VerificationMeta('lastSuccessfulRefreshAt');
  @override
  late final GeneratedColumn<DateTime> lastSuccessfulRefreshAt =
      GeneratedColumn<DateTime>(
        'last_successful_refresh_at',
        aliasedName,
        true,
        type: DriftSqlType.dateTime,
        requiredDuringInsert: false,
      );
  @override
  List<GeneratedColumn> get $columns => [
    serverId,
    scope,
    cursor,
    lastSuccessfulRefreshAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'sync_states';
  @override
  VerificationContext validateIntegrity(
    Insertable<SyncState> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('server_id')) {
      context.handle(
        _serverIdMeta,
        serverId.isAcceptableOrUnknown(data['server_id']!, _serverIdMeta),
      );
    } else if (isInserting) {
      context.missing(_serverIdMeta);
    }
    if (data.containsKey('scope')) {
      context.handle(
        _scopeMeta,
        scope.isAcceptableOrUnknown(data['scope']!, _scopeMeta),
      );
    } else if (isInserting) {
      context.missing(_scopeMeta);
    }
    if (data.containsKey('cursor')) {
      context.handle(
        _cursorMeta,
        cursor.isAcceptableOrUnknown(data['cursor']!, _cursorMeta),
      );
    }
    if (data.containsKey('last_successful_refresh_at')) {
      context.handle(
        _lastSuccessfulRefreshAtMeta,
        lastSuccessfulRefreshAt.isAcceptableOrUnknown(
          data['last_successful_refresh_at']!,
          _lastSuccessfulRefreshAtMeta,
        ),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {serverId, scope};
  @override
  SyncState map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return SyncState(
      serverId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}server_id'],
      )!,
      scope: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}scope'],
      )!,
      cursor: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}cursor'],
      ),
      lastSuccessfulRefreshAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}last_successful_refresh_at'],
      ),
    );
  }

  @override
  $SyncStatesTable createAlias(String alias) {
    return $SyncStatesTable(attachedDatabase, alias);
  }
}

class SyncState extends DataClass implements Insertable<SyncState> {
  final String serverId;
  final String scope;
  final String? cursor;
  final DateTime? lastSuccessfulRefreshAt;
  const SyncState({
    required this.serverId,
    required this.scope,
    this.cursor,
    this.lastSuccessfulRefreshAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['server_id'] = Variable<String>(serverId);
    map['scope'] = Variable<String>(scope);
    if (!nullToAbsent || cursor != null) {
      map['cursor'] = Variable<String>(cursor);
    }
    if (!nullToAbsent || lastSuccessfulRefreshAt != null) {
      map['last_successful_refresh_at'] = Variable<DateTime>(
        lastSuccessfulRefreshAt,
      );
    }
    return map;
  }

  SyncStatesCompanion toCompanion(bool nullToAbsent) {
    return SyncStatesCompanion(
      serverId: Value(serverId),
      scope: Value(scope),
      cursor: cursor == null && nullToAbsent
          ? const Value.absent()
          : Value(cursor),
      lastSuccessfulRefreshAt: lastSuccessfulRefreshAt == null && nullToAbsent
          ? const Value.absent()
          : Value(lastSuccessfulRefreshAt),
    );
  }

  factory SyncState.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return SyncState(
      serverId: serializer.fromJson<String>(json['serverId']),
      scope: serializer.fromJson<String>(json['scope']),
      cursor: serializer.fromJson<String?>(json['cursor']),
      lastSuccessfulRefreshAt: serializer.fromJson<DateTime?>(
        json['lastSuccessfulRefreshAt'],
      ),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'serverId': serializer.toJson<String>(serverId),
      'scope': serializer.toJson<String>(scope),
      'cursor': serializer.toJson<String?>(cursor),
      'lastSuccessfulRefreshAt': serializer.toJson<DateTime?>(
        lastSuccessfulRefreshAt,
      ),
    };
  }

  SyncState copyWith({
    String? serverId,
    String? scope,
    Value<String?> cursor = const Value.absent(),
    Value<DateTime?> lastSuccessfulRefreshAt = const Value.absent(),
  }) => SyncState(
    serverId: serverId ?? this.serverId,
    scope: scope ?? this.scope,
    cursor: cursor.present ? cursor.value : this.cursor,
    lastSuccessfulRefreshAt: lastSuccessfulRefreshAt.present
        ? lastSuccessfulRefreshAt.value
        : this.lastSuccessfulRefreshAt,
  );
  SyncState copyWithCompanion(SyncStatesCompanion data) {
    return SyncState(
      serverId: data.serverId.present ? data.serverId.value : this.serverId,
      scope: data.scope.present ? data.scope.value : this.scope,
      cursor: data.cursor.present ? data.cursor.value : this.cursor,
      lastSuccessfulRefreshAt: data.lastSuccessfulRefreshAt.present
          ? data.lastSuccessfulRefreshAt.value
          : this.lastSuccessfulRefreshAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('SyncState(')
          ..write('serverId: $serverId, ')
          ..write('scope: $scope, ')
          ..write('cursor: $cursor, ')
          ..write('lastSuccessfulRefreshAt: $lastSuccessfulRefreshAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode =>
      Object.hash(serverId, scope, cursor, lastSuccessfulRefreshAt);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is SyncState &&
          other.serverId == this.serverId &&
          other.scope == this.scope &&
          other.cursor == this.cursor &&
          other.lastSuccessfulRefreshAt == this.lastSuccessfulRefreshAt);
}

class SyncStatesCompanion extends UpdateCompanion<SyncState> {
  final Value<String> serverId;
  final Value<String> scope;
  final Value<String?> cursor;
  final Value<DateTime?> lastSuccessfulRefreshAt;
  final Value<int> rowid;
  const SyncStatesCompanion({
    this.serverId = const Value.absent(),
    this.scope = const Value.absent(),
    this.cursor = const Value.absent(),
    this.lastSuccessfulRefreshAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  SyncStatesCompanion.insert({
    required String serverId,
    required String scope,
    this.cursor = const Value.absent(),
    this.lastSuccessfulRefreshAt = const Value.absent(),
    this.rowid = const Value.absent(),
  }) : serverId = Value(serverId),
       scope = Value(scope);
  static Insertable<SyncState> custom({
    Expression<String>? serverId,
    Expression<String>? scope,
    Expression<String>? cursor,
    Expression<DateTime>? lastSuccessfulRefreshAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (serverId != null) 'server_id': serverId,
      if (scope != null) 'scope': scope,
      if (cursor != null) 'cursor': cursor,
      if (lastSuccessfulRefreshAt != null)
        'last_successful_refresh_at': lastSuccessfulRefreshAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  SyncStatesCompanion copyWith({
    Value<String>? serverId,
    Value<String>? scope,
    Value<String?>? cursor,
    Value<DateTime?>? lastSuccessfulRefreshAt,
    Value<int>? rowid,
  }) {
    return SyncStatesCompanion(
      serverId: serverId ?? this.serverId,
      scope: scope ?? this.scope,
      cursor: cursor ?? this.cursor,
      lastSuccessfulRefreshAt:
          lastSuccessfulRefreshAt ?? this.lastSuccessfulRefreshAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (serverId.present) {
      map['server_id'] = Variable<String>(serverId.value);
    }
    if (scope.present) {
      map['scope'] = Variable<String>(scope.value);
    }
    if (cursor.present) {
      map['cursor'] = Variable<String>(cursor.value);
    }
    if (lastSuccessfulRefreshAt.present) {
      map['last_successful_refresh_at'] = Variable<DateTime>(
        lastSuccessfulRefreshAt.value,
      );
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('SyncStatesCompanion(')
          ..write('serverId: $serverId, ')
          ..write('scope: $scope, ')
          ..write('cursor: $cursor, ')
          ..write('lastSuccessfulRefreshAt: $lastSuccessfulRefreshAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

abstract class _$AppDatabase extends GeneratedDatabase {
  _$AppDatabase(QueryExecutor e) : super(e);
  $AppDatabaseManager get managers => $AppDatabaseManager(this);
  late final $ServersTable servers = $ServersTable(this);
  late final $WorkflowEntriesTable workflowEntries = $WorkflowEntriesTable(
    this,
  );
  late final $WorkspaceEntriesTable workspaceEntries = $WorkspaceEntriesTable(
    this,
  );
  late final $SyncStatesTable syncStates = $SyncStatesTable(this);
  @override
  Iterable<TableInfo<Table, Object?>> get allTables =>
      allSchemaEntities.whereType<TableInfo<Table, Object?>>();
  @override
  List<DatabaseSchemaEntity> get allSchemaEntities => [
    servers,
    workflowEntries,
    workspaceEntries,
    syncStates,
  ];
}

typedef $$ServersTableCreateCompanionBuilder =
    ServersCompanion Function({
      required String id,
      required String endpoint,
      Value<DateTime?> lastConnectedAt,
      Value<int> rowid,
    });
typedef $$ServersTableUpdateCompanionBuilder =
    ServersCompanion Function({
      Value<String> id,
      Value<String> endpoint,
      Value<DateTime?> lastConnectedAt,
      Value<int> rowid,
    });

class $$ServersTableFilterComposer
    extends Composer<_$AppDatabase, $ServersTable> {
  $$ServersTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get endpoint => $composableBuilder(
    column: $table.endpoint,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get lastConnectedAt => $composableBuilder(
    column: $table.lastConnectedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$ServersTableOrderingComposer
    extends Composer<_$AppDatabase, $ServersTable> {
  $$ServersTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get endpoint => $composableBuilder(
    column: $table.endpoint,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get lastConnectedAt => $composableBuilder(
    column: $table.lastConnectedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$ServersTableAnnotationComposer
    extends Composer<_$AppDatabase, $ServersTable> {
  $$ServersTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get endpoint =>
      $composableBuilder(column: $table.endpoint, builder: (column) => column);

  GeneratedColumn<DateTime> get lastConnectedAt => $composableBuilder(
    column: $table.lastConnectedAt,
    builder: (column) => column,
  );
}

class $$ServersTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $ServersTable,
          Server,
          $$ServersTableFilterComposer,
          $$ServersTableOrderingComposer,
          $$ServersTableAnnotationComposer,
          $$ServersTableCreateCompanionBuilder,
          $$ServersTableUpdateCompanionBuilder,
          (Server, BaseReferences<_$AppDatabase, $ServersTable, Server>),
          Server,
          PrefetchHooks Function()
        > {
  $$ServersTableTableManager(_$AppDatabase db, $ServersTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$ServersTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$ServersTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$ServersTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String> endpoint = const Value.absent(),
                Value<DateTime?> lastConnectedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => ServersCompanion(
                id: id,
                endpoint: endpoint,
                lastConnectedAt: lastConnectedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                required String endpoint,
                Value<DateTime?> lastConnectedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => ServersCompanion.insert(
                id: id,
                endpoint: endpoint,
                lastConnectedAt: lastConnectedAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$ServersTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $ServersTable,
      Server,
      $$ServersTableFilterComposer,
      $$ServersTableOrderingComposer,
      $$ServersTableAnnotationComposer,
      $$ServersTableCreateCompanionBuilder,
      $$ServersTableUpdateCompanionBuilder,
      (Server, BaseReferences<_$AppDatabase, $ServersTable, Server>),
      Server,
      PrefetchHooks Function()
    >;
typedef $$WorkflowEntriesTableCreateCompanionBuilder =
    WorkflowEntriesCompanion Function({
      required String serverId,
      required String name,
      required String description,
      required String driver,
      required Uint8List rawProtobuf,
      required DateTime refreshedAt,
      Value<int> rowid,
    });
typedef $$WorkflowEntriesTableUpdateCompanionBuilder =
    WorkflowEntriesCompanion Function({
      Value<String> serverId,
      Value<String> name,
      Value<String> description,
      Value<String> driver,
      Value<Uint8List> rawProtobuf,
      Value<DateTime> refreshedAt,
      Value<int> rowid,
    });

class $$WorkflowEntriesTableFilterComposer
    extends Composer<_$AppDatabase, $WorkflowEntriesTable> {
  $$WorkflowEntriesTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get serverId => $composableBuilder(
    column: $table.serverId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get name => $composableBuilder(
    column: $table.name,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get description => $composableBuilder(
    column: $table.description,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get driver => $composableBuilder(
    column: $table.driver,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<Uint8List> get rawProtobuf => $composableBuilder(
    column: $table.rawProtobuf,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$WorkflowEntriesTableOrderingComposer
    extends Composer<_$AppDatabase, $WorkflowEntriesTable> {
  $$WorkflowEntriesTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get serverId => $composableBuilder(
    column: $table.serverId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get name => $composableBuilder(
    column: $table.name,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get description => $composableBuilder(
    column: $table.description,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get driver => $composableBuilder(
    column: $table.driver,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<Uint8List> get rawProtobuf => $composableBuilder(
    column: $table.rawProtobuf,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$WorkflowEntriesTableAnnotationComposer
    extends Composer<_$AppDatabase, $WorkflowEntriesTable> {
  $$WorkflowEntriesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get serverId =>
      $composableBuilder(column: $table.serverId, builder: (column) => column);

  GeneratedColumn<String> get name =>
      $composableBuilder(column: $table.name, builder: (column) => column);

  GeneratedColumn<String> get description => $composableBuilder(
    column: $table.description,
    builder: (column) => column,
  );

  GeneratedColumn<String> get driver =>
      $composableBuilder(column: $table.driver, builder: (column) => column);

  GeneratedColumn<Uint8List> get rawProtobuf => $composableBuilder(
    column: $table.rawProtobuf,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => column,
  );
}

class $$WorkflowEntriesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $WorkflowEntriesTable,
          WorkflowEntry,
          $$WorkflowEntriesTableFilterComposer,
          $$WorkflowEntriesTableOrderingComposer,
          $$WorkflowEntriesTableAnnotationComposer,
          $$WorkflowEntriesTableCreateCompanionBuilder,
          $$WorkflowEntriesTableUpdateCompanionBuilder,
          (
            WorkflowEntry,
            BaseReferences<_$AppDatabase, $WorkflowEntriesTable, WorkflowEntry>,
          ),
          WorkflowEntry,
          PrefetchHooks Function()
        > {
  $$WorkflowEntriesTableTableManager(
    _$AppDatabase db,
    $WorkflowEntriesTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$WorkflowEntriesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$WorkflowEntriesTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$WorkflowEntriesTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> serverId = const Value.absent(),
                Value<String> name = const Value.absent(),
                Value<String> description = const Value.absent(),
                Value<String> driver = const Value.absent(),
                Value<Uint8List> rawProtobuf = const Value.absent(),
                Value<DateTime> refreshedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => WorkflowEntriesCompanion(
                serverId: serverId,
                name: name,
                description: description,
                driver: driver,
                rawProtobuf: rawProtobuf,
                refreshedAt: refreshedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String serverId,
                required String name,
                required String description,
                required String driver,
                required Uint8List rawProtobuf,
                required DateTime refreshedAt,
                Value<int> rowid = const Value.absent(),
              }) => WorkflowEntriesCompanion.insert(
                serverId: serverId,
                name: name,
                description: description,
                driver: driver,
                rawProtobuf: rawProtobuf,
                refreshedAt: refreshedAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$WorkflowEntriesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $WorkflowEntriesTable,
      WorkflowEntry,
      $$WorkflowEntriesTableFilterComposer,
      $$WorkflowEntriesTableOrderingComposer,
      $$WorkflowEntriesTableAnnotationComposer,
      $$WorkflowEntriesTableCreateCompanionBuilder,
      $$WorkflowEntriesTableUpdateCompanionBuilder,
      (
        WorkflowEntry,
        BaseReferences<_$AppDatabase, $WorkflowEntriesTable, WorkflowEntry>,
      ),
      WorkflowEntry,
      PrefetchHooks Function()
    >;
typedef $$WorkspaceEntriesTableCreateCompanionBuilder =
    WorkspaceEntriesCompanion Function({
      required String serverId,
      required String name,
      required String workflowName,
      Value<DateTime?> createdAt,
      Value<DateTime?> lastActiveAt,
      Value<DateTime?> updatedAt,
      required Uint8List rawProtobuf,
      required DateTime refreshedAt,
      Value<int> rowid,
    });
typedef $$WorkspaceEntriesTableUpdateCompanionBuilder =
    WorkspaceEntriesCompanion Function({
      Value<String> serverId,
      Value<String> name,
      Value<String> workflowName,
      Value<DateTime?> createdAt,
      Value<DateTime?> lastActiveAt,
      Value<DateTime?> updatedAt,
      Value<Uint8List> rawProtobuf,
      Value<DateTime> refreshedAt,
      Value<int> rowid,
    });

class $$WorkspaceEntriesTableFilterComposer
    extends Composer<_$AppDatabase, $WorkspaceEntriesTable> {
  $$WorkspaceEntriesTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get serverId => $composableBuilder(
    column: $table.serverId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get name => $composableBuilder(
    column: $table.name,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get workflowName => $composableBuilder(
    column: $table.workflowName,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get lastActiveAt => $composableBuilder(
    column: $table.lastActiveAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<Uint8List> get rawProtobuf => $composableBuilder(
    column: $table.rawProtobuf,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$WorkspaceEntriesTableOrderingComposer
    extends Composer<_$AppDatabase, $WorkspaceEntriesTable> {
  $$WorkspaceEntriesTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get serverId => $composableBuilder(
    column: $table.serverId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get name => $composableBuilder(
    column: $table.name,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get workflowName => $composableBuilder(
    column: $table.workflowName,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get lastActiveAt => $composableBuilder(
    column: $table.lastActiveAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<Uint8List> get rawProtobuf => $composableBuilder(
    column: $table.rawProtobuf,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$WorkspaceEntriesTableAnnotationComposer
    extends Composer<_$AppDatabase, $WorkspaceEntriesTable> {
  $$WorkspaceEntriesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get serverId =>
      $composableBuilder(column: $table.serverId, builder: (column) => column);

  GeneratedColumn<String> get name =>
      $composableBuilder(column: $table.name, builder: (column) => column);

  GeneratedColumn<String> get workflowName => $composableBuilder(
    column: $table.workflowName,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get createdAt =>
      $composableBuilder(column: $table.createdAt, builder: (column) => column);

  GeneratedColumn<DateTime> get lastActiveAt => $composableBuilder(
    column: $table.lastActiveAt,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get updatedAt =>
      $composableBuilder(column: $table.updatedAt, builder: (column) => column);

  GeneratedColumn<Uint8List> get rawProtobuf => $composableBuilder(
    column: $table.rawProtobuf,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => column,
  );
}

class $$WorkspaceEntriesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $WorkspaceEntriesTable,
          WorkspaceEntry,
          $$WorkspaceEntriesTableFilterComposer,
          $$WorkspaceEntriesTableOrderingComposer,
          $$WorkspaceEntriesTableAnnotationComposer,
          $$WorkspaceEntriesTableCreateCompanionBuilder,
          $$WorkspaceEntriesTableUpdateCompanionBuilder,
          (
            WorkspaceEntry,
            BaseReferences<
              _$AppDatabase,
              $WorkspaceEntriesTable,
              WorkspaceEntry
            >,
          ),
          WorkspaceEntry,
          PrefetchHooks Function()
        > {
  $$WorkspaceEntriesTableTableManager(
    _$AppDatabase db,
    $WorkspaceEntriesTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$WorkspaceEntriesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$WorkspaceEntriesTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$WorkspaceEntriesTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> serverId = const Value.absent(),
                Value<String> name = const Value.absent(),
                Value<String> workflowName = const Value.absent(),
                Value<DateTime?> createdAt = const Value.absent(),
                Value<DateTime?> lastActiveAt = const Value.absent(),
                Value<DateTime?> updatedAt = const Value.absent(),
                Value<Uint8List> rawProtobuf = const Value.absent(),
                Value<DateTime> refreshedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => WorkspaceEntriesCompanion(
                serverId: serverId,
                name: name,
                workflowName: workflowName,
                createdAt: createdAt,
                lastActiveAt: lastActiveAt,
                updatedAt: updatedAt,
                rawProtobuf: rawProtobuf,
                refreshedAt: refreshedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String serverId,
                required String name,
                required String workflowName,
                Value<DateTime?> createdAt = const Value.absent(),
                Value<DateTime?> lastActiveAt = const Value.absent(),
                Value<DateTime?> updatedAt = const Value.absent(),
                required Uint8List rawProtobuf,
                required DateTime refreshedAt,
                Value<int> rowid = const Value.absent(),
              }) => WorkspaceEntriesCompanion.insert(
                serverId: serverId,
                name: name,
                workflowName: workflowName,
                createdAt: createdAt,
                lastActiveAt: lastActiveAt,
                updatedAt: updatedAt,
                rawProtobuf: rawProtobuf,
                refreshedAt: refreshedAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$WorkspaceEntriesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $WorkspaceEntriesTable,
      WorkspaceEntry,
      $$WorkspaceEntriesTableFilterComposer,
      $$WorkspaceEntriesTableOrderingComposer,
      $$WorkspaceEntriesTableAnnotationComposer,
      $$WorkspaceEntriesTableCreateCompanionBuilder,
      $$WorkspaceEntriesTableUpdateCompanionBuilder,
      (
        WorkspaceEntry,
        BaseReferences<_$AppDatabase, $WorkspaceEntriesTable, WorkspaceEntry>,
      ),
      WorkspaceEntry,
      PrefetchHooks Function()
    >;
typedef $$SyncStatesTableCreateCompanionBuilder =
    SyncStatesCompanion Function({
      required String serverId,
      required String scope,
      Value<String?> cursor,
      Value<DateTime?> lastSuccessfulRefreshAt,
      Value<int> rowid,
    });
typedef $$SyncStatesTableUpdateCompanionBuilder =
    SyncStatesCompanion Function({
      Value<String> serverId,
      Value<String> scope,
      Value<String?> cursor,
      Value<DateTime?> lastSuccessfulRefreshAt,
      Value<int> rowid,
    });

class $$SyncStatesTableFilterComposer
    extends Composer<_$AppDatabase, $SyncStatesTable> {
  $$SyncStatesTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get serverId => $composableBuilder(
    column: $table.serverId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get scope => $composableBuilder(
    column: $table.scope,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get cursor => $composableBuilder(
    column: $table.cursor,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get lastSuccessfulRefreshAt => $composableBuilder(
    column: $table.lastSuccessfulRefreshAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$SyncStatesTableOrderingComposer
    extends Composer<_$AppDatabase, $SyncStatesTable> {
  $$SyncStatesTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get serverId => $composableBuilder(
    column: $table.serverId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get scope => $composableBuilder(
    column: $table.scope,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get cursor => $composableBuilder(
    column: $table.cursor,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get lastSuccessfulRefreshAt => $composableBuilder(
    column: $table.lastSuccessfulRefreshAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$SyncStatesTableAnnotationComposer
    extends Composer<_$AppDatabase, $SyncStatesTable> {
  $$SyncStatesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get serverId =>
      $composableBuilder(column: $table.serverId, builder: (column) => column);

  GeneratedColumn<String> get scope =>
      $composableBuilder(column: $table.scope, builder: (column) => column);

  GeneratedColumn<String> get cursor =>
      $composableBuilder(column: $table.cursor, builder: (column) => column);

  GeneratedColumn<DateTime> get lastSuccessfulRefreshAt => $composableBuilder(
    column: $table.lastSuccessfulRefreshAt,
    builder: (column) => column,
  );
}

class $$SyncStatesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $SyncStatesTable,
          SyncState,
          $$SyncStatesTableFilterComposer,
          $$SyncStatesTableOrderingComposer,
          $$SyncStatesTableAnnotationComposer,
          $$SyncStatesTableCreateCompanionBuilder,
          $$SyncStatesTableUpdateCompanionBuilder,
          (
            SyncState,
            BaseReferences<_$AppDatabase, $SyncStatesTable, SyncState>,
          ),
          SyncState,
          PrefetchHooks Function()
        > {
  $$SyncStatesTableTableManager(_$AppDatabase db, $SyncStatesTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$SyncStatesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$SyncStatesTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$SyncStatesTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> serverId = const Value.absent(),
                Value<String> scope = const Value.absent(),
                Value<String?> cursor = const Value.absent(),
                Value<DateTime?> lastSuccessfulRefreshAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => SyncStatesCompanion(
                serverId: serverId,
                scope: scope,
                cursor: cursor,
                lastSuccessfulRefreshAt: lastSuccessfulRefreshAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String serverId,
                required String scope,
                Value<String?> cursor = const Value.absent(),
                Value<DateTime?> lastSuccessfulRefreshAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => SyncStatesCompanion.insert(
                serverId: serverId,
                scope: scope,
                cursor: cursor,
                lastSuccessfulRefreshAt: lastSuccessfulRefreshAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$SyncStatesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $SyncStatesTable,
      SyncState,
      $$SyncStatesTableFilterComposer,
      $$SyncStatesTableOrderingComposer,
      $$SyncStatesTableAnnotationComposer,
      $$SyncStatesTableCreateCompanionBuilder,
      $$SyncStatesTableUpdateCompanionBuilder,
      (SyncState, BaseReferences<_$AppDatabase, $SyncStatesTable, SyncState>),
      SyncState,
      PrefetchHooks Function()
    >;

class $AppDatabaseManager {
  final _$AppDatabase _db;
  $AppDatabaseManager(this._db);
  $$ServersTableTableManager get servers =>
      $$ServersTableTableManager(_db, _db.servers);
  $$WorkflowEntriesTableTableManager get workflowEntries =>
      $$WorkflowEntriesTableTableManager(_db, _db.workflowEntries);
  $$WorkspaceEntriesTableTableManager get workspaceEntries =>
      $$WorkspaceEntriesTableTableManager(_db, _db.workspaceEntries);
  $$SyncStatesTableTableManager get syncStates =>
      $$SyncStatesTableTableManager(_db, _db.syncStates);
}
