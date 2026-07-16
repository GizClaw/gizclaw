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
  static const VerificationMeta _localeMeta = const VerificationMeta('locale');
  @override
  late final GeneratedColumn<String> locale = GeneratedColumn<String>(
    'locale',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
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
  static const VerificationMeta _iconPngMeta = const VerificationMeta(
    'iconPng',
  );
  @override
  late final GeneratedColumn<Uint8List> iconPng = GeneratedColumn<Uint8List>(
    'icon_png',
    aliasedName,
    true,
    type: DriftSqlType.blob,
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
    locale,
    description,
    driver,
    iconPng,
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
    if (data.containsKey('locale')) {
      context.handle(
        _localeMeta,
        locale.isAcceptableOrUnknown(data['locale']!, _localeMeta),
      );
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
    if (data.containsKey('icon_png')) {
      context.handle(
        _iconPngMeta,
        iconPng.isAcceptableOrUnknown(data['icon_png']!, _iconPngMeta),
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
      locale: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}locale'],
      ),
      description: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}description'],
      )!,
      driver: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}driver'],
      )!,
      iconPng: attachedDatabase.typeMapping.read(
        DriftSqlType.blob,
        data['${effectivePrefix}icon_png'],
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
  $WorkflowEntriesTable createAlias(String alias) {
    return $WorkflowEntriesTable(attachedDatabase, alias);
  }
}

class WorkflowEntry extends DataClass implements Insertable<WorkflowEntry> {
  final String serverId;
  final String name;
  final String? locale;
  final String description;
  final String driver;
  final Uint8List? iconPng;
  final Uint8List rawProtobuf;
  final DateTime refreshedAt;
  const WorkflowEntry({
    required this.serverId,
    required this.name,
    this.locale,
    required this.description,
    required this.driver,
    this.iconPng,
    required this.rawProtobuf,
    required this.refreshedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['server_id'] = Variable<String>(serverId);
    map['name'] = Variable<String>(name);
    if (!nullToAbsent || locale != null) {
      map['locale'] = Variable<String>(locale);
    }
    map['description'] = Variable<String>(description);
    map['driver'] = Variable<String>(driver);
    if (!nullToAbsent || iconPng != null) {
      map['icon_png'] = Variable<Uint8List>(iconPng);
    }
    map['raw_protobuf'] = Variable<Uint8List>(rawProtobuf);
    map['refreshed_at'] = Variable<DateTime>(refreshedAt);
    return map;
  }

  WorkflowEntriesCompanion toCompanion(bool nullToAbsent) {
    return WorkflowEntriesCompanion(
      serverId: Value(serverId),
      name: Value(name),
      locale: locale == null && nullToAbsent
          ? const Value.absent()
          : Value(locale),
      description: Value(description),
      driver: Value(driver),
      iconPng: iconPng == null && nullToAbsent
          ? const Value.absent()
          : Value(iconPng),
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
      locale: serializer.fromJson<String?>(json['locale']),
      description: serializer.fromJson<String>(json['description']),
      driver: serializer.fromJson<String>(json['driver']),
      iconPng: serializer.fromJson<Uint8List?>(json['iconPng']),
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
      'locale': serializer.toJson<String?>(locale),
      'description': serializer.toJson<String>(description),
      'driver': serializer.toJson<String>(driver),
      'iconPng': serializer.toJson<Uint8List?>(iconPng),
      'rawProtobuf': serializer.toJson<Uint8List>(rawProtobuf),
      'refreshedAt': serializer.toJson<DateTime>(refreshedAt),
    };
  }

  WorkflowEntry copyWith({
    String? serverId,
    String? name,
    Value<String?> locale = const Value.absent(),
    String? description,
    String? driver,
    Value<Uint8List?> iconPng = const Value.absent(),
    Uint8List? rawProtobuf,
    DateTime? refreshedAt,
  }) => WorkflowEntry(
    serverId: serverId ?? this.serverId,
    name: name ?? this.name,
    locale: locale.present ? locale.value : this.locale,
    description: description ?? this.description,
    driver: driver ?? this.driver,
    iconPng: iconPng.present ? iconPng.value : this.iconPng,
    rawProtobuf: rawProtobuf ?? this.rawProtobuf,
    refreshedAt: refreshedAt ?? this.refreshedAt,
  );
  WorkflowEntry copyWithCompanion(WorkflowEntriesCompanion data) {
    return WorkflowEntry(
      serverId: data.serverId.present ? data.serverId.value : this.serverId,
      name: data.name.present ? data.name.value : this.name,
      locale: data.locale.present ? data.locale.value : this.locale,
      description: data.description.present
          ? data.description.value
          : this.description,
      driver: data.driver.present ? data.driver.value : this.driver,
      iconPng: data.iconPng.present ? data.iconPng.value : this.iconPng,
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
          ..write('locale: $locale, ')
          ..write('description: $description, ')
          ..write('driver: $driver, ')
          ..write('iconPng: $iconPng, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    serverId,
    name,
    locale,
    description,
    driver,
    $driftBlobEquality.hash(iconPng),
    $driftBlobEquality.hash(rawProtobuf),
    refreshedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is WorkflowEntry &&
          other.serverId == this.serverId &&
          other.name == this.name &&
          other.locale == this.locale &&
          other.description == this.description &&
          other.driver == this.driver &&
          $driftBlobEquality.equals(other.iconPng, this.iconPng) &&
          $driftBlobEquality.equals(other.rawProtobuf, this.rawProtobuf) &&
          other.refreshedAt == this.refreshedAt);
}

class WorkflowEntriesCompanion extends UpdateCompanion<WorkflowEntry> {
  final Value<String> serverId;
  final Value<String> name;
  final Value<String?> locale;
  final Value<String> description;
  final Value<String> driver;
  final Value<Uint8List?> iconPng;
  final Value<Uint8List> rawProtobuf;
  final Value<DateTime> refreshedAt;
  final Value<int> rowid;
  const WorkflowEntriesCompanion({
    this.serverId = const Value.absent(),
    this.name = const Value.absent(),
    this.locale = const Value.absent(),
    this.description = const Value.absent(),
    this.driver = const Value.absent(),
    this.iconPng = const Value.absent(),
    this.rawProtobuf = const Value.absent(),
    this.refreshedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  WorkflowEntriesCompanion.insert({
    required String serverId,
    required String name,
    this.locale = const Value.absent(),
    required String description,
    required String driver,
    this.iconPng = const Value.absent(),
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
    Expression<String>? locale,
    Expression<String>? description,
    Expression<String>? driver,
    Expression<Uint8List>? iconPng,
    Expression<Uint8List>? rawProtobuf,
    Expression<DateTime>? refreshedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (serverId != null) 'server_id': serverId,
      if (name != null) 'name': name,
      if (locale != null) 'locale': locale,
      if (description != null) 'description': description,
      if (driver != null) 'driver': driver,
      if (iconPng != null) 'icon_png': iconPng,
      if (rawProtobuf != null) 'raw_protobuf': rawProtobuf,
      if (refreshedAt != null) 'refreshed_at': refreshedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  WorkflowEntriesCompanion copyWith({
    Value<String>? serverId,
    Value<String>? name,
    Value<String?>? locale,
    Value<String>? description,
    Value<String>? driver,
    Value<Uint8List?>? iconPng,
    Value<Uint8List>? rawProtobuf,
    Value<DateTime>? refreshedAt,
    Value<int>? rowid,
  }) {
    return WorkflowEntriesCompanion(
      serverId: serverId ?? this.serverId,
      name: name ?? this.name,
      locale: locale ?? this.locale,
      description: description ?? this.description,
      driver: driver ?? this.driver,
      iconPng: iconPng ?? this.iconPng,
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
    if (locale.present) {
      map['locale'] = Variable<String>(locale.value);
    }
    if (description.present) {
      map['description'] = Variable<String>(description.value);
    }
    if (driver.present) {
      map['driver'] = Variable<String>(driver.value);
    }
    if (iconPng.present) {
      map['icon_png'] = Variable<Uint8List>(iconPng.value);
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
          ..write('locale: $locale, ')
          ..write('description: $description, ')
          ..write('driver: $driver, ')
          ..write('iconPng: $iconPng, ')
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

class $WorkspaceChatEntriesTable extends WorkspaceChatEntries
    with TableInfo<$WorkspaceChatEntriesTable, WorkspaceChatEntry> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $WorkspaceChatEntriesTable(this.attachedDatabase, [this._alias]);
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
  static const VerificationMeta _workspaceNameMeta = const VerificationMeta(
    'workspaceName',
  );
  @override
  late final GeneratedColumn<String> workspaceName = GeneratedColumn<String>(
    'workspace_name',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _historyIdMeta = const VerificationMeta(
    'historyId',
  );
  @override
  late final GeneratedColumn<String> historyId = GeneratedColumn<String>(
    'history_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _roleMeta = const VerificationMeta('role');
  @override
  late final GeneratedColumn<String> role = GeneratedColumn<String>(
    'role',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _contentMeta = const VerificationMeta(
    'content',
  );
  @override
  late final GeneratedColumn<String> content = GeneratedColumn<String>(
    'content',
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
    workspaceName,
    historyId,
    role,
    content,
    name,
    createdAt,
    refreshedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'workspace_chat_entries';
  @override
  VerificationContext validateIntegrity(
    Insertable<WorkspaceChatEntry> instance, {
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
    if (data.containsKey('workspace_name')) {
      context.handle(
        _workspaceNameMeta,
        workspaceName.isAcceptableOrUnknown(
          data['workspace_name']!,
          _workspaceNameMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_workspaceNameMeta);
    }
    if (data.containsKey('history_id')) {
      context.handle(
        _historyIdMeta,
        historyId.isAcceptableOrUnknown(data['history_id']!, _historyIdMeta),
      );
    } else if (isInserting) {
      context.missing(_historyIdMeta);
    }
    if (data.containsKey('role')) {
      context.handle(
        _roleMeta,
        role.isAcceptableOrUnknown(data['role']!, _roleMeta),
      );
    } else if (isInserting) {
      context.missing(_roleMeta);
    }
    if (data.containsKey('content')) {
      context.handle(
        _contentMeta,
        content.isAcceptableOrUnknown(data['content']!, _contentMeta),
      );
    } else if (isInserting) {
      context.missing(_contentMeta);
    }
    if (data.containsKey('name')) {
      context.handle(
        _nameMeta,
        name.isAcceptableOrUnknown(data['name']!, _nameMeta),
      );
    } else if (isInserting) {
      context.missing(_nameMeta);
    }
    if (data.containsKey('created_at')) {
      context.handle(
        _createdAtMeta,
        createdAt.isAcceptableOrUnknown(data['created_at']!, _createdAtMeta),
      );
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
  Set<GeneratedColumn> get $primaryKey => {serverId, workspaceName, historyId};
  @override
  WorkspaceChatEntry map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return WorkspaceChatEntry(
      serverId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}server_id'],
      )!,
      workspaceName: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}workspace_name'],
      )!,
      historyId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}history_id'],
      )!,
      role: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}role'],
      )!,
      content: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}content'],
      )!,
      name: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}name'],
      )!,
      createdAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}created_at'],
      ),
      refreshedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}refreshed_at'],
      )!,
    );
  }

  @override
  $WorkspaceChatEntriesTable createAlias(String alias) {
    return $WorkspaceChatEntriesTable(attachedDatabase, alias);
  }
}

class WorkspaceChatEntry extends DataClass
    implements Insertable<WorkspaceChatEntry> {
  final String serverId;
  final String workspaceName;
  final String historyId;
  final String role;
  final String content;
  final String name;
  final DateTime? createdAt;
  final DateTime refreshedAt;
  const WorkspaceChatEntry({
    required this.serverId,
    required this.workspaceName,
    required this.historyId,
    required this.role,
    required this.content,
    required this.name,
    this.createdAt,
    required this.refreshedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['server_id'] = Variable<String>(serverId);
    map['workspace_name'] = Variable<String>(workspaceName);
    map['history_id'] = Variable<String>(historyId);
    map['role'] = Variable<String>(role);
    map['content'] = Variable<String>(content);
    map['name'] = Variable<String>(name);
    if (!nullToAbsent || createdAt != null) {
      map['created_at'] = Variable<DateTime>(createdAt);
    }
    map['refreshed_at'] = Variable<DateTime>(refreshedAt);
    return map;
  }

  WorkspaceChatEntriesCompanion toCompanion(bool nullToAbsent) {
    return WorkspaceChatEntriesCompanion(
      serverId: Value(serverId),
      workspaceName: Value(workspaceName),
      historyId: Value(historyId),
      role: Value(role),
      content: Value(content),
      name: Value(name),
      createdAt: createdAt == null && nullToAbsent
          ? const Value.absent()
          : Value(createdAt),
      refreshedAt: Value(refreshedAt),
    );
  }

  factory WorkspaceChatEntry.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return WorkspaceChatEntry(
      serverId: serializer.fromJson<String>(json['serverId']),
      workspaceName: serializer.fromJson<String>(json['workspaceName']),
      historyId: serializer.fromJson<String>(json['historyId']),
      role: serializer.fromJson<String>(json['role']),
      content: serializer.fromJson<String>(json['content']),
      name: serializer.fromJson<String>(json['name']),
      createdAt: serializer.fromJson<DateTime?>(json['createdAt']),
      refreshedAt: serializer.fromJson<DateTime>(json['refreshedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'serverId': serializer.toJson<String>(serverId),
      'workspaceName': serializer.toJson<String>(workspaceName),
      'historyId': serializer.toJson<String>(historyId),
      'role': serializer.toJson<String>(role),
      'content': serializer.toJson<String>(content),
      'name': serializer.toJson<String>(name),
      'createdAt': serializer.toJson<DateTime?>(createdAt),
      'refreshedAt': serializer.toJson<DateTime>(refreshedAt),
    };
  }

  WorkspaceChatEntry copyWith({
    String? serverId,
    String? workspaceName,
    String? historyId,
    String? role,
    String? content,
    String? name,
    Value<DateTime?> createdAt = const Value.absent(),
    DateTime? refreshedAt,
  }) => WorkspaceChatEntry(
    serverId: serverId ?? this.serverId,
    workspaceName: workspaceName ?? this.workspaceName,
    historyId: historyId ?? this.historyId,
    role: role ?? this.role,
    content: content ?? this.content,
    name: name ?? this.name,
    createdAt: createdAt.present ? createdAt.value : this.createdAt,
    refreshedAt: refreshedAt ?? this.refreshedAt,
  );
  WorkspaceChatEntry copyWithCompanion(WorkspaceChatEntriesCompanion data) {
    return WorkspaceChatEntry(
      serverId: data.serverId.present ? data.serverId.value : this.serverId,
      workspaceName: data.workspaceName.present
          ? data.workspaceName.value
          : this.workspaceName,
      historyId: data.historyId.present ? data.historyId.value : this.historyId,
      role: data.role.present ? data.role.value : this.role,
      content: data.content.present ? data.content.value : this.content,
      name: data.name.present ? data.name.value : this.name,
      createdAt: data.createdAt.present ? data.createdAt.value : this.createdAt,
      refreshedAt: data.refreshedAt.present
          ? data.refreshedAt.value
          : this.refreshedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('WorkspaceChatEntry(')
          ..write('serverId: $serverId, ')
          ..write('workspaceName: $workspaceName, ')
          ..write('historyId: $historyId, ')
          ..write('role: $role, ')
          ..write('content: $content, ')
          ..write('name: $name, ')
          ..write('createdAt: $createdAt, ')
          ..write('refreshedAt: $refreshedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    serverId,
    workspaceName,
    historyId,
    role,
    content,
    name,
    createdAt,
    refreshedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is WorkspaceChatEntry &&
          other.serverId == this.serverId &&
          other.workspaceName == this.workspaceName &&
          other.historyId == this.historyId &&
          other.role == this.role &&
          other.content == this.content &&
          other.name == this.name &&
          other.createdAt == this.createdAt &&
          other.refreshedAt == this.refreshedAt);
}

class WorkspaceChatEntriesCompanion
    extends UpdateCompanion<WorkspaceChatEntry> {
  final Value<String> serverId;
  final Value<String> workspaceName;
  final Value<String> historyId;
  final Value<String> role;
  final Value<String> content;
  final Value<String> name;
  final Value<DateTime?> createdAt;
  final Value<DateTime> refreshedAt;
  final Value<int> rowid;
  const WorkspaceChatEntriesCompanion({
    this.serverId = const Value.absent(),
    this.workspaceName = const Value.absent(),
    this.historyId = const Value.absent(),
    this.role = const Value.absent(),
    this.content = const Value.absent(),
    this.name = const Value.absent(),
    this.createdAt = const Value.absent(),
    this.refreshedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  WorkspaceChatEntriesCompanion.insert({
    required String serverId,
    required String workspaceName,
    required String historyId,
    required String role,
    required String content,
    required String name,
    this.createdAt = const Value.absent(),
    required DateTime refreshedAt,
    this.rowid = const Value.absent(),
  }) : serverId = Value(serverId),
       workspaceName = Value(workspaceName),
       historyId = Value(historyId),
       role = Value(role),
       content = Value(content),
       name = Value(name),
       refreshedAt = Value(refreshedAt);
  static Insertable<WorkspaceChatEntry> custom({
    Expression<String>? serverId,
    Expression<String>? workspaceName,
    Expression<String>? historyId,
    Expression<String>? role,
    Expression<String>? content,
    Expression<String>? name,
    Expression<DateTime>? createdAt,
    Expression<DateTime>? refreshedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (serverId != null) 'server_id': serverId,
      if (workspaceName != null) 'workspace_name': workspaceName,
      if (historyId != null) 'history_id': historyId,
      if (role != null) 'role': role,
      if (content != null) 'content': content,
      if (name != null) 'name': name,
      if (createdAt != null) 'created_at': createdAt,
      if (refreshedAt != null) 'refreshed_at': refreshedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  WorkspaceChatEntriesCompanion copyWith({
    Value<String>? serverId,
    Value<String>? workspaceName,
    Value<String>? historyId,
    Value<String>? role,
    Value<String>? content,
    Value<String>? name,
    Value<DateTime?>? createdAt,
    Value<DateTime>? refreshedAt,
    Value<int>? rowid,
  }) {
    return WorkspaceChatEntriesCompanion(
      serverId: serverId ?? this.serverId,
      workspaceName: workspaceName ?? this.workspaceName,
      historyId: historyId ?? this.historyId,
      role: role ?? this.role,
      content: content ?? this.content,
      name: name ?? this.name,
      createdAt: createdAt ?? this.createdAt,
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
    if (workspaceName.present) {
      map['workspace_name'] = Variable<String>(workspaceName.value);
    }
    if (historyId.present) {
      map['history_id'] = Variable<String>(historyId.value);
    }
    if (role.present) {
      map['role'] = Variable<String>(role.value);
    }
    if (content.present) {
      map['content'] = Variable<String>(content.value);
    }
    if (name.present) {
      map['name'] = Variable<String>(name.value);
    }
    if (createdAt.present) {
      map['created_at'] = Variable<DateTime>(createdAt.value);
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
    return (StringBuffer('WorkspaceChatEntriesCompanion(')
          ..write('serverId: $serverId, ')
          ..write('workspaceName: $workspaceName, ')
          ..write('historyId: $historyId, ')
          ..write('role: $role, ')
          ..write('content: $content, ')
          ..write('name: $name, ')
          ..write('createdAt: $createdAt, ')
          ..write('refreshedAt: $refreshedAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $FriendEntriesTable extends FriendEntries
    with TableInfo<$FriendEntriesTable, FriendEntry> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $FriendEntriesTable(this.attachedDatabase, [this._alias]);
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
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _peerPublicKeyMeta = const VerificationMeta(
    'peerPublicKey',
  );
  @override
  late final GeneratedColumn<String> peerPublicKey = GeneratedColumn<String>(
    'peer_public_key',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _workspaceNameMeta = const VerificationMeta(
    'workspaceName',
  );
  @override
  late final GeneratedColumn<String> workspaceName = GeneratedColumn<String>(
    'workspace_name',
    aliasedName,
    true,
    type: DriftSqlType.string,
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
    id,
    peerPublicKey,
    workspaceName,
    rawProtobuf,
    refreshedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'friend_entries';
  @override
  VerificationContext validateIntegrity(
    Insertable<FriendEntry> instance, {
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
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('peer_public_key')) {
      context.handle(
        _peerPublicKeyMeta,
        peerPublicKey.isAcceptableOrUnknown(
          data['peer_public_key']!,
          _peerPublicKeyMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_peerPublicKeyMeta);
    }
    if (data.containsKey('workspace_name')) {
      context.handle(
        _workspaceNameMeta,
        workspaceName.isAcceptableOrUnknown(
          data['workspace_name']!,
          _workspaceNameMeta,
        ),
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
  Set<GeneratedColumn> get $primaryKey => {serverId, id};
  @override
  FriendEntry map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return FriendEntry(
      serverId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}server_id'],
      )!,
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      peerPublicKey: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}peer_public_key'],
      )!,
      workspaceName: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}workspace_name'],
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
  $FriendEntriesTable createAlias(String alias) {
    return $FriendEntriesTable(attachedDatabase, alias);
  }
}

class FriendEntry extends DataClass implements Insertable<FriendEntry> {
  final String serverId;
  final String id;
  final String peerPublicKey;
  final String? workspaceName;
  final Uint8List rawProtobuf;
  final DateTime refreshedAt;
  const FriendEntry({
    required this.serverId,
    required this.id,
    required this.peerPublicKey,
    this.workspaceName,
    required this.rawProtobuf,
    required this.refreshedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['server_id'] = Variable<String>(serverId);
    map['id'] = Variable<String>(id);
    map['peer_public_key'] = Variable<String>(peerPublicKey);
    if (!nullToAbsent || workspaceName != null) {
      map['workspace_name'] = Variable<String>(workspaceName);
    }
    map['raw_protobuf'] = Variable<Uint8List>(rawProtobuf);
    map['refreshed_at'] = Variable<DateTime>(refreshedAt);
    return map;
  }

  FriendEntriesCompanion toCompanion(bool nullToAbsent) {
    return FriendEntriesCompanion(
      serverId: Value(serverId),
      id: Value(id),
      peerPublicKey: Value(peerPublicKey),
      workspaceName: workspaceName == null && nullToAbsent
          ? const Value.absent()
          : Value(workspaceName),
      rawProtobuf: Value(rawProtobuf),
      refreshedAt: Value(refreshedAt),
    );
  }

  factory FriendEntry.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return FriendEntry(
      serverId: serializer.fromJson<String>(json['serverId']),
      id: serializer.fromJson<String>(json['id']),
      peerPublicKey: serializer.fromJson<String>(json['peerPublicKey']),
      workspaceName: serializer.fromJson<String?>(json['workspaceName']),
      rawProtobuf: serializer.fromJson<Uint8List>(json['rawProtobuf']),
      refreshedAt: serializer.fromJson<DateTime>(json['refreshedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'serverId': serializer.toJson<String>(serverId),
      'id': serializer.toJson<String>(id),
      'peerPublicKey': serializer.toJson<String>(peerPublicKey),
      'workspaceName': serializer.toJson<String?>(workspaceName),
      'rawProtobuf': serializer.toJson<Uint8List>(rawProtobuf),
      'refreshedAt': serializer.toJson<DateTime>(refreshedAt),
    };
  }

  FriendEntry copyWith({
    String? serverId,
    String? id,
    String? peerPublicKey,
    Value<String?> workspaceName = const Value.absent(),
    Uint8List? rawProtobuf,
    DateTime? refreshedAt,
  }) => FriendEntry(
    serverId: serverId ?? this.serverId,
    id: id ?? this.id,
    peerPublicKey: peerPublicKey ?? this.peerPublicKey,
    workspaceName: workspaceName.present
        ? workspaceName.value
        : this.workspaceName,
    rawProtobuf: rawProtobuf ?? this.rawProtobuf,
    refreshedAt: refreshedAt ?? this.refreshedAt,
  );
  FriendEntry copyWithCompanion(FriendEntriesCompanion data) {
    return FriendEntry(
      serverId: data.serverId.present ? data.serverId.value : this.serverId,
      id: data.id.present ? data.id.value : this.id,
      peerPublicKey: data.peerPublicKey.present
          ? data.peerPublicKey.value
          : this.peerPublicKey,
      workspaceName: data.workspaceName.present
          ? data.workspaceName.value
          : this.workspaceName,
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
    return (StringBuffer('FriendEntry(')
          ..write('serverId: $serverId, ')
          ..write('id: $id, ')
          ..write('peerPublicKey: $peerPublicKey, ')
          ..write('workspaceName: $workspaceName, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    serverId,
    id,
    peerPublicKey,
    workspaceName,
    $driftBlobEquality.hash(rawProtobuf),
    refreshedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is FriendEntry &&
          other.serverId == this.serverId &&
          other.id == this.id &&
          other.peerPublicKey == this.peerPublicKey &&
          other.workspaceName == this.workspaceName &&
          $driftBlobEquality.equals(other.rawProtobuf, this.rawProtobuf) &&
          other.refreshedAt == this.refreshedAt);
}

class FriendEntriesCompanion extends UpdateCompanion<FriendEntry> {
  final Value<String> serverId;
  final Value<String> id;
  final Value<String> peerPublicKey;
  final Value<String?> workspaceName;
  final Value<Uint8List> rawProtobuf;
  final Value<DateTime> refreshedAt;
  final Value<int> rowid;
  const FriendEntriesCompanion({
    this.serverId = const Value.absent(),
    this.id = const Value.absent(),
    this.peerPublicKey = const Value.absent(),
    this.workspaceName = const Value.absent(),
    this.rawProtobuf = const Value.absent(),
    this.refreshedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  FriendEntriesCompanion.insert({
    required String serverId,
    required String id,
    required String peerPublicKey,
    this.workspaceName = const Value.absent(),
    required Uint8List rawProtobuf,
    required DateTime refreshedAt,
    this.rowid = const Value.absent(),
  }) : serverId = Value(serverId),
       id = Value(id),
       peerPublicKey = Value(peerPublicKey),
       rawProtobuf = Value(rawProtobuf),
       refreshedAt = Value(refreshedAt);
  static Insertable<FriendEntry> custom({
    Expression<String>? serverId,
    Expression<String>? id,
    Expression<String>? peerPublicKey,
    Expression<String>? workspaceName,
    Expression<Uint8List>? rawProtobuf,
    Expression<DateTime>? refreshedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (serverId != null) 'server_id': serverId,
      if (id != null) 'id': id,
      if (peerPublicKey != null) 'peer_public_key': peerPublicKey,
      if (workspaceName != null) 'workspace_name': workspaceName,
      if (rawProtobuf != null) 'raw_protobuf': rawProtobuf,
      if (refreshedAt != null) 'refreshed_at': refreshedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  FriendEntriesCompanion copyWith({
    Value<String>? serverId,
    Value<String>? id,
    Value<String>? peerPublicKey,
    Value<String?>? workspaceName,
    Value<Uint8List>? rawProtobuf,
    Value<DateTime>? refreshedAt,
    Value<int>? rowid,
  }) {
    return FriendEntriesCompanion(
      serverId: serverId ?? this.serverId,
      id: id ?? this.id,
      peerPublicKey: peerPublicKey ?? this.peerPublicKey,
      workspaceName: workspaceName ?? this.workspaceName,
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
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (peerPublicKey.present) {
      map['peer_public_key'] = Variable<String>(peerPublicKey.value);
    }
    if (workspaceName.present) {
      map['workspace_name'] = Variable<String>(workspaceName.value);
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
    return (StringBuffer('FriendEntriesCompanion(')
          ..write('serverId: $serverId, ')
          ..write('id: $id, ')
          ..write('peerPublicKey: $peerPublicKey, ')
          ..write('workspaceName: $workspaceName, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $FriendGroupEntriesTable extends FriendGroupEntries
    with TableInfo<$FriendGroupEntriesTable, FriendGroupEntry> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $FriendGroupEntriesTable(this.attachedDatabase, [this._alias]);
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
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
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
  static const VerificationMeta _workspaceNameMeta = const VerificationMeta(
    'workspaceName',
  );
  @override
  late final GeneratedColumn<String> workspaceName = GeneratedColumn<String>(
    'workspace_name',
    aliasedName,
    true,
    type: DriftSqlType.string,
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
    id,
    name,
    description,
    workspaceName,
    rawProtobuf,
    refreshedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'friend_group_entries';
  @override
  VerificationContext validateIntegrity(
    Insertable<FriendGroupEntry> instance, {
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
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
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
    if (data.containsKey('workspace_name')) {
      context.handle(
        _workspaceNameMeta,
        workspaceName.isAcceptableOrUnknown(
          data['workspace_name']!,
          _workspaceNameMeta,
        ),
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
  Set<GeneratedColumn> get $primaryKey => {serverId, id};
  @override
  FriendGroupEntry map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return FriendGroupEntry(
      serverId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}server_id'],
      )!,
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      name: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}name'],
      )!,
      description: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}description'],
      )!,
      workspaceName: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}workspace_name'],
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
  $FriendGroupEntriesTable createAlias(String alias) {
    return $FriendGroupEntriesTable(attachedDatabase, alias);
  }
}

class FriendGroupEntry extends DataClass
    implements Insertable<FriendGroupEntry> {
  final String serverId;
  final String id;
  final String name;
  final String description;
  final String? workspaceName;
  final Uint8List rawProtobuf;
  final DateTime refreshedAt;
  const FriendGroupEntry({
    required this.serverId,
    required this.id,
    required this.name,
    required this.description,
    this.workspaceName,
    required this.rawProtobuf,
    required this.refreshedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['server_id'] = Variable<String>(serverId);
    map['id'] = Variable<String>(id);
    map['name'] = Variable<String>(name);
    map['description'] = Variable<String>(description);
    if (!nullToAbsent || workspaceName != null) {
      map['workspace_name'] = Variable<String>(workspaceName);
    }
    map['raw_protobuf'] = Variable<Uint8List>(rawProtobuf);
    map['refreshed_at'] = Variable<DateTime>(refreshedAt);
    return map;
  }

  FriendGroupEntriesCompanion toCompanion(bool nullToAbsent) {
    return FriendGroupEntriesCompanion(
      serverId: Value(serverId),
      id: Value(id),
      name: Value(name),
      description: Value(description),
      workspaceName: workspaceName == null && nullToAbsent
          ? const Value.absent()
          : Value(workspaceName),
      rawProtobuf: Value(rawProtobuf),
      refreshedAt: Value(refreshedAt),
    );
  }

  factory FriendGroupEntry.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return FriendGroupEntry(
      serverId: serializer.fromJson<String>(json['serverId']),
      id: serializer.fromJson<String>(json['id']),
      name: serializer.fromJson<String>(json['name']),
      description: serializer.fromJson<String>(json['description']),
      workspaceName: serializer.fromJson<String?>(json['workspaceName']),
      rawProtobuf: serializer.fromJson<Uint8List>(json['rawProtobuf']),
      refreshedAt: serializer.fromJson<DateTime>(json['refreshedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'serverId': serializer.toJson<String>(serverId),
      'id': serializer.toJson<String>(id),
      'name': serializer.toJson<String>(name),
      'description': serializer.toJson<String>(description),
      'workspaceName': serializer.toJson<String?>(workspaceName),
      'rawProtobuf': serializer.toJson<Uint8List>(rawProtobuf),
      'refreshedAt': serializer.toJson<DateTime>(refreshedAt),
    };
  }

  FriendGroupEntry copyWith({
    String? serverId,
    String? id,
    String? name,
    String? description,
    Value<String?> workspaceName = const Value.absent(),
    Uint8List? rawProtobuf,
    DateTime? refreshedAt,
  }) => FriendGroupEntry(
    serverId: serverId ?? this.serverId,
    id: id ?? this.id,
    name: name ?? this.name,
    description: description ?? this.description,
    workspaceName: workspaceName.present
        ? workspaceName.value
        : this.workspaceName,
    rawProtobuf: rawProtobuf ?? this.rawProtobuf,
    refreshedAt: refreshedAt ?? this.refreshedAt,
  );
  FriendGroupEntry copyWithCompanion(FriendGroupEntriesCompanion data) {
    return FriendGroupEntry(
      serverId: data.serverId.present ? data.serverId.value : this.serverId,
      id: data.id.present ? data.id.value : this.id,
      name: data.name.present ? data.name.value : this.name,
      description: data.description.present
          ? data.description.value
          : this.description,
      workspaceName: data.workspaceName.present
          ? data.workspaceName.value
          : this.workspaceName,
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
    return (StringBuffer('FriendGroupEntry(')
          ..write('serverId: $serverId, ')
          ..write('id: $id, ')
          ..write('name: $name, ')
          ..write('description: $description, ')
          ..write('workspaceName: $workspaceName, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    serverId,
    id,
    name,
    description,
    workspaceName,
    $driftBlobEquality.hash(rawProtobuf),
    refreshedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is FriendGroupEntry &&
          other.serverId == this.serverId &&
          other.id == this.id &&
          other.name == this.name &&
          other.description == this.description &&
          other.workspaceName == this.workspaceName &&
          $driftBlobEquality.equals(other.rawProtobuf, this.rawProtobuf) &&
          other.refreshedAt == this.refreshedAt);
}

class FriendGroupEntriesCompanion extends UpdateCompanion<FriendGroupEntry> {
  final Value<String> serverId;
  final Value<String> id;
  final Value<String> name;
  final Value<String> description;
  final Value<String?> workspaceName;
  final Value<Uint8List> rawProtobuf;
  final Value<DateTime> refreshedAt;
  final Value<int> rowid;
  const FriendGroupEntriesCompanion({
    this.serverId = const Value.absent(),
    this.id = const Value.absent(),
    this.name = const Value.absent(),
    this.description = const Value.absent(),
    this.workspaceName = const Value.absent(),
    this.rawProtobuf = const Value.absent(),
    this.refreshedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  FriendGroupEntriesCompanion.insert({
    required String serverId,
    required String id,
    required String name,
    required String description,
    this.workspaceName = const Value.absent(),
    required Uint8List rawProtobuf,
    required DateTime refreshedAt,
    this.rowid = const Value.absent(),
  }) : serverId = Value(serverId),
       id = Value(id),
       name = Value(name),
       description = Value(description),
       rawProtobuf = Value(rawProtobuf),
       refreshedAt = Value(refreshedAt);
  static Insertable<FriendGroupEntry> custom({
    Expression<String>? serverId,
    Expression<String>? id,
    Expression<String>? name,
    Expression<String>? description,
    Expression<String>? workspaceName,
    Expression<Uint8List>? rawProtobuf,
    Expression<DateTime>? refreshedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (serverId != null) 'server_id': serverId,
      if (id != null) 'id': id,
      if (name != null) 'name': name,
      if (description != null) 'description': description,
      if (workspaceName != null) 'workspace_name': workspaceName,
      if (rawProtobuf != null) 'raw_protobuf': rawProtobuf,
      if (refreshedAt != null) 'refreshed_at': refreshedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  FriendGroupEntriesCompanion copyWith({
    Value<String>? serverId,
    Value<String>? id,
    Value<String>? name,
    Value<String>? description,
    Value<String?>? workspaceName,
    Value<Uint8List>? rawProtobuf,
    Value<DateTime>? refreshedAt,
    Value<int>? rowid,
  }) {
    return FriendGroupEntriesCompanion(
      serverId: serverId ?? this.serverId,
      id: id ?? this.id,
      name: name ?? this.name,
      description: description ?? this.description,
      workspaceName: workspaceName ?? this.workspaceName,
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
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (name.present) {
      map['name'] = Variable<String>(name.value);
    }
    if (description.present) {
      map['description'] = Variable<String>(description.value);
    }
    if (workspaceName.present) {
      map['workspace_name'] = Variable<String>(workspaceName.value);
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
    return (StringBuffer('FriendGroupEntriesCompanion(')
          ..write('serverId: $serverId, ')
          ..write('id: $id, ')
          ..write('name: $name, ')
          ..write('description: $description, ')
          ..write('workspaceName: $workspaceName, ')
          ..write('rawProtobuf: $rawProtobuf, ')
          ..write('refreshedAt: $refreshedAt, ')
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
  late final $WorkspaceChatEntriesTable workspaceChatEntries =
      $WorkspaceChatEntriesTable(this);
  late final $FriendEntriesTable friendEntries = $FriendEntriesTable(this);
  late final $FriendGroupEntriesTable friendGroupEntries =
      $FriendGroupEntriesTable(this);
  @override
  Iterable<TableInfo<Table, Object?>> get allTables =>
      allSchemaEntities.whereType<TableInfo<Table, Object?>>();
  @override
  List<DatabaseSchemaEntity> get allSchemaEntities => [
    servers,
    workflowEntries,
    workspaceEntries,
    syncStates,
    workspaceChatEntries,
    friendEntries,
    friendGroupEntries,
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
      Value<String?> locale,
      required String description,
      required String driver,
      Value<Uint8List?> iconPng,
      required Uint8List rawProtobuf,
      required DateTime refreshedAt,
      Value<int> rowid,
    });
typedef $$WorkflowEntriesTableUpdateCompanionBuilder =
    WorkflowEntriesCompanion Function({
      Value<String> serverId,
      Value<String> name,
      Value<String?> locale,
      Value<String> description,
      Value<String> driver,
      Value<Uint8List?> iconPng,
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

  ColumnFilters<String> get locale => $composableBuilder(
    column: $table.locale,
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

  ColumnFilters<Uint8List> get iconPng => $composableBuilder(
    column: $table.iconPng,
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

  ColumnOrderings<String> get locale => $composableBuilder(
    column: $table.locale,
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

  ColumnOrderings<Uint8List> get iconPng => $composableBuilder(
    column: $table.iconPng,
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

  GeneratedColumn<String> get locale =>
      $composableBuilder(column: $table.locale, builder: (column) => column);

  GeneratedColumn<String> get description => $composableBuilder(
    column: $table.description,
    builder: (column) => column,
  );

  GeneratedColumn<String> get driver =>
      $composableBuilder(column: $table.driver, builder: (column) => column);

  GeneratedColumn<Uint8List> get iconPng =>
      $composableBuilder(column: $table.iconPng, builder: (column) => column);

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
                Value<String?> locale = const Value.absent(),
                Value<String> description = const Value.absent(),
                Value<String> driver = const Value.absent(),
                Value<Uint8List?> iconPng = const Value.absent(),
                Value<Uint8List> rawProtobuf = const Value.absent(),
                Value<DateTime> refreshedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => WorkflowEntriesCompanion(
                serverId: serverId,
                name: name,
                locale: locale,
                description: description,
                driver: driver,
                iconPng: iconPng,
                rawProtobuf: rawProtobuf,
                refreshedAt: refreshedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String serverId,
                required String name,
                Value<String?> locale = const Value.absent(),
                required String description,
                required String driver,
                Value<Uint8List?> iconPng = const Value.absent(),
                required Uint8List rawProtobuf,
                required DateTime refreshedAt,
                Value<int> rowid = const Value.absent(),
              }) => WorkflowEntriesCompanion.insert(
                serverId: serverId,
                name: name,
                locale: locale,
                description: description,
                driver: driver,
                iconPng: iconPng,
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
typedef $$WorkspaceChatEntriesTableCreateCompanionBuilder =
    WorkspaceChatEntriesCompanion Function({
      required String serverId,
      required String workspaceName,
      required String historyId,
      required String role,
      required String content,
      required String name,
      Value<DateTime?> createdAt,
      required DateTime refreshedAt,
      Value<int> rowid,
    });
typedef $$WorkspaceChatEntriesTableUpdateCompanionBuilder =
    WorkspaceChatEntriesCompanion Function({
      Value<String> serverId,
      Value<String> workspaceName,
      Value<String> historyId,
      Value<String> role,
      Value<String> content,
      Value<String> name,
      Value<DateTime?> createdAt,
      Value<DateTime> refreshedAt,
      Value<int> rowid,
    });

class $$WorkspaceChatEntriesTableFilterComposer
    extends Composer<_$AppDatabase, $WorkspaceChatEntriesTable> {
  $$WorkspaceChatEntriesTableFilterComposer({
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

  ColumnFilters<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get historyId => $composableBuilder(
    column: $table.historyId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get role => $composableBuilder(
    column: $table.role,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get content => $composableBuilder(
    column: $table.content,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get name => $composableBuilder(
    column: $table.name,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$WorkspaceChatEntriesTableOrderingComposer
    extends Composer<_$AppDatabase, $WorkspaceChatEntriesTable> {
  $$WorkspaceChatEntriesTableOrderingComposer({
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

  ColumnOrderings<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get historyId => $composableBuilder(
    column: $table.historyId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get role => $composableBuilder(
    column: $table.role,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get content => $composableBuilder(
    column: $table.content,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get name => $composableBuilder(
    column: $table.name,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$WorkspaceChatEntriesTableAnnotationComposer
    extends Composer<_$AppDatabase, $WorkspaceChatEntriesTable> {
  $$WorkspaceChatEntriesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get serverId =>
      $composableBuilder(column: $table.serverId, builder: (column) => column);

  GeneratedColumn<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
    builder: (column) => column,
  );

  GeneratedColumn<String> get historyId =>
      $composableBuilder(column: $table.historyId, builder: (column) => column);

  GeneratedColumn<String> get role =>
      $composableBuilder(column: $table.role, builder: (column) => column);

  GeneratedColumn<String> get content =>
      $composableBuilder(column: $table.content, builder: (column) => column);

  GeneratedColumn<String> get name =>
      $composableBuilder(column: $table.name, builder: (column) => column);

  GeneratedColumn<DateTime> get createdAt =>
      $composableBuilder(column: $table.createdAt, builder: (column) => column);

  GeneratedColumn<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => column,
  );
}

class $$WorkspaceChatEntriesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $WorkspaceChatEntriesTable,
          WorkspaceChatEntry,
          $$WorkspaceChatEntriesTableFilterComposer,
          $$WorkspaceChatEntriesTableOrderingComposer,
          $$WorkspaceChatEntriesTableAnnotationComposer,
          $$WorkspaceChatEntriesTableCreateCompanionBuilder,
          $$WorkspaceChatEntriesTableUpdateCompanionBuilder,
          (
            WorkspaceChatEntry,
            BaseReferences<
              _$AppDatabase,
              $WorkspaceChatEntriesTable,
              WorkspaceChatEntry
            >,
          ),
          WorkspaceChatEntry,
          PrefetchHooks Function()
        > {
  $$WorkspaceChatEntriesTableTableManager(
    _$AppDatabase db,
    $WorkspaceChatEntriesTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$WorkspaceChatEntriesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$WorkspaceChatEntriesTableOrderingComposer(
                $db: db,
                $table: table,
              ),
          createComputedFieldComposer: () =>
              $$WorkspaceChatEntriesTableAnnotationComposer(
                $db: db,
                $table: table,
              ),
          updateCompanionCallback:
              ({
                Value<String> serverId = const Value.absent(),
                Value<String> workspaceName = const Value.absent(),
                Value<String> historyId = const Value.absent(),
                Value<String> role = const Value.absent(),
                Value<String> content = const Value.absent(),
                Value<String> name = const Value.absent(),
                Value<DateTime?> createdAt = const Value.absent(),
                Value<DateTime> refreshedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => WorkspaceChatEntriesCompanion(
                serverId: serverId,
                workspaceName: workspaceName,
                historyId: historyId,
                role: role,
                content: content,
                name: name,
                createdAt: createdAt,
                refreshedAt: refreshedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String serverId,
                required String workspaceName,
                required String historyId,
                required String role,
                required String content,
                required String name,
                Value<DateTime?> createdAt = const Value.absent(),
                required DateTime refreshedAt,
                Value<int> rowid = const Value.absent(),
              }) => WorkspaceChatEntriesCompanion.insert(
                serverId: serverId,
                workspaceName: workspaceName,
                historyId: historyId,
                role: role,
                content: content,
                name: name,
                createdAt: createdAt,
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

typedef $$WorkspaceChatEntriesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $WorkspaceChatEntriesTable,
      WorkspaceChatEntry,
      $$WorkspaceChatEntriesTableFilterComposer,
      $$WorkspaceChatEntriesTableOrderingComposer,
      $$WorkspaceChatEntriesTableAnnotationComposer,
      $$WorkspaceChatEntriesTableCreateCompanionBuilder,
      $$WorkspaceChatEntriesTableUpdateCompanionBuilder,
      (
        WorkspaceChatEntry,
        BaseReferences<
          _$AppDatabase,
          $WorkspaceChatEntriesTable,
          WorkspaceChatEntry
        >,
      ),
      WorkspaceChatEntry,
      PrefetchHooks Function()
    >;
typedef $$FriendEntriesTableCreateCompanionBuilder =
    FriendEntriesCompanion Function({
      required String serverId,
      required String id,
      required String peerPublicKey,
      Value<String?> workspaceName,
      required Uint8List rawProtobuf,
      required DateTime refreshedAt,
      Value<int> rowid,
    });
typedef $$FriendEntriesTableUpdateCompanionBuilder =
    FriendEntriesCompanion Function({
      Value<String> serverId,
      Value<String> id,
      Value<String> peerPublicKey,
      Value<String?> workspaceName,
      Value<Uint8List> rawProtobuf,
      Value<DateTime> refreshedAt,
      Value<int> rowid,
    });

class $$FriendEntriesTableFilterComposer
    extends Composer<_$AppDatabase, $FriendEntriesTable> {
  $$FriendEntriesTableFilterComposer({
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

  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get peerPublicKey => $composableBuilder(
    column: $table.peerPublicKey,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
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

class $$FriendEntriesTableOrderingComposer
    extends Composer<_$AppDatabase, $FriendEntriesTable> {
  $$FriendEntriesTableOrderingComposer({
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

  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get peerPublicKey => $composableBuilder(
    column: $table.peerPublicKey,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
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

class $$FriendEntriesTableAnnotationComposer
    extends Composer<_$AppDatabase, $FriendEntriesTable> {
  $$FriendEntriesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get serverId =>
      $composableBuilder(column: $table.serverId, builder: (column) => column);

  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get peerPublicKey => $composableBuilder(
    column: $table.peerPublicKey,
    builder: (column) => column,
  );

  GeneratedColumn<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
    builder: (column) => column,
  );

  GeneratedColumn<Uint8List> get rawProtobuf => $composableBuilder(
    column: $table.rawProtobuf,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => column,
  );
}

class $$FriendEntriesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $FriendEntriesTable,
          FriendEntry,
          $$FriendEntriesTableFilterComposer,
          $$FriendEntriesTableOrderingComposer,
          $$FriendEntriesTableAnnotationComposer,
          $$FriendEntriesTableCreateCompanionBuilder,
          $$FriendEntriesTableUpdateCompanionBuilder,
          (
            FriendEntry,
            BaseReferences<_$AppDatabase, $FriendEntriesTable, FriendEntry>,
          ),
          FriendEntry,
          PrefetchHooks Function()
        > {
  $$FriendEntriesTableTableManager(_$AppDatabase db, $FriendEntriesTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$FriendEntriesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$FriendEntriesTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$FriendEntriesTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> serverId = const Value.absent(),
                Value<String> id = const Value.absent(),
                Value<String> peerPublicKey = const Value.absent(),
                Value<String?> workspaceName = const Value.absent(),
                Value<Uint8List> rawProtobuf = const Value.absent(),
                Value<DateTime> refreshedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => FriendEntriesCompanion(
                serverId: serverId,
                id: id,
                peerPublicKey: peerPublicKey,
                workspaceName: workspaceName,
                rawProtobuf: rawProtobuf,
                refreshedAt: refreshedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String serverId,
                required String id,
                required String peerPublicKey,
                Value<String?> workspaceName = const Value.absent(),
                required Uint8List rawProtobuf,
                required DateTime refreshedAt,
                Value<int> rowid = const Value.absent(),
              }) => FriendEntriesCompanion.insert(
                serverId: serverId,
                id: id,
                peerPublicKey: peerPublicKey,
                workspaceName: workspaceName,
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

typedef $$FriendEntriesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $FriendEntriesTable,
      FriendEntry,
      $$FriendEntriesTableFilterComposer,
      $$FriendEntriesTableOrderingComposer,
      $$FriendEntriesTableAnnotationComposer,
      $$FriendEntriesTableCreateCompanionBuilder,
      $$FriendEntriesTableUpdateCompanionBuilder,
      (
        FriendEntry,
        BaseReferences<_$AppDatabase, $FriendEntriesTable, FriendEntry>,
      ),
      FriendEntry,
      PrefetchHooks Function()
    >;
typedef $$FriendGroupEntriesTableCreateCompanionBuilder =
    FriendGroupEntriesCompanion Function({
      required String serverId,
      required String id,
      required String name,
      required String description,
      Value<String?> workspaceName,
      required Uint8List rawProtobuf,
      required DateTime refreshedAt,
      Value<int> rowid,
    });
typedef $$FriendGroupEntriesTableUpdateCompanionBuilder =
    FriendGroupEntriesCompanion Function({
      Value<String> serverId,
      Value<String> id,
      Value<String> name,
      Value<String> description,
      Value<String?> workspaceName,
      Value<Uint8List> rawProtobuf,
      Value<DateTime> refreshedAt,
      Value<int> rowid,
    });

class $$FriendGroupEntriesTableFilterComposer
    extends Composer<_$AppDatabase, $FriendGroupEntriesTable> {
  $$FriendGroupEntriesTableFilterComposer({
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

  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
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

  ColumnFilters<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
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

class $$FriendGroupEntriesTableOrderingComposer
    extends Composer<_$AppDatabase, $FriendGroupEntriesTable> {
  $$FriendGroupEntriesTableOrderingComposer({
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

  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
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

  ColumnOrderings<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
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

class $$FriendGroupEntriesTableAnnotationComposer
    extends Composer<_$AppDatabase, $FriendGroupEntriesTable> {
  $$FriendGroupEntriesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get serverId =>
      $composableBuilder(column: $table.serverId, builder: (column) => column);

  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get name =>
      $composableBuilder(column: $table.name, builder: (column) => column);

  GeneratedColumn<String> get description => $composableBuilder(
    column: $table.description,
    builder: (column) => column,
  );

  GeneratedColumn<String> get workspaceName => $composableBuilder(
    column: $table.workspaceName,
    builder: (column) => column,
  );

  GeneratedColumn<Uint8List> get rawProtobuf => $composableBuilder(
    column: $table.rawProtobuf,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get refreshedAt => $composableBuilder(
    column: $table.refreshedAt,
    builder: (column) => column,
  );
}

class $$FriendGroupEntriesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $FriendGroupEntriesTable,
          FriendGroupEntry,
          $$FriendGroupEntriesTableFilterComposer,
          $$FriendGroupEntriesTableOrderingComposer,
          $$FriendGroupEntriesTableAnnotationComposer,
          $$FriendGroupEntriesTableCreateCompanionBuilder,
          $$FriendGroupEntriesTableUpdateCompanionBuilder,
          (
            FriendGroupEntry,
            BaseReferences<
              _$AppDatabase,
              $FriendGroupEntriesTable,
              FriendGroupEntry
            >,
          ),
          FriendGroupEntry,
          PrefetchHooks Function()
        > {
  $$FriendGroupEntriesTableTableManager(
    _$AppDatabase db,
    $FriendGroupEntriesTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$FriendGroupEntriesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$FriendGroupEntriesTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$FriendGroupEntriesTableAnnotationComposer(
                $db: db,
                $table: table,
              ),
          updateCompanionCallback:
              ({
                Value<String> serverId = const Value.absent(),
                Value<String> id = const Value.absent(),
                Value<String> name = const Value.absent(),
                Value<String> description = const Value.absent(),
                Value<String?> workspaceName = const Value.absent(),
                Value<Uint8List> rawProtobuf = const Value.absent(),
                Value<DateTime> refreshedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => FriendGroupEntriesCompanion(
                serverId: serverId,
                id: id,
                name: name,
                description: description,
                workspaceName: workspaceName,
                rawProtobuf: rawProtobuf,
                refreshedAt: refreshedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String serverId,
                required String id,
                required String name,
                required String description,
                Value<String?> workspaceName = const Value.absent(),
                required Uint8List rawProtobuf,
                required DateTime refreshedAt,
                Value<int> rowid = const Value.absent(),
              }) => FriendGroupEntriesCompanion.insert(
                serverId: serverId,
                id: id,
                name: name,
                description: description,
                workspaceName: workspaceName,
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

typedef $$FriendGroupEntriesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $FriendGroupEntriesTable,
      FriendGroupEntry,
      $$FriendGroupEntriesTableFilterComposer,
      $$FriendGroupEntriesTableOrderingComposer,
      $$FriendGroupEntriesTableAnnotationComposer,
      $$FriendGroupEntriesTableCreateCompanionBuilder,
      $$FriendGroupEntriesTableUpdateCompanionBuilder,
      (
        FriendGroupEntry,
        BaseReferences<
          _$AppDatabase,
          $FriendGroupEntriesTable,
          FriendGroupEntry
        >,
      ),
      FriendGroupEntry,
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
  $$WorkspaceChatEntriesTableTableManager get workspaceChatEntries =>
      $$WorkspaceChatEntriesTableTableManager(_db, _db.workspaceChatEntries);
  $$FriendEntriesTableTableManager get friendEntries =>
      $$FriendEntriesTableTableManager(_db, _db.friendEntries);
  $$FriendGroupEntriesTableTableManager get friendGroupEntries =>
      $$FriendGroupEntriesTableTableManager(_db, _db.friendGroupEntries);
}
