// This is a generated file - do not edit.
//
// Generated from payload/workspace.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;
import 'package:protobuf/well_known_types/google/protobuf/struct.pb.dart' as $0;

import 'ai.pb.dart' as $2;
import 'enums.pbenum.dart' as $3;
import 'system.pb.dart' as $1;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

class AgentSelection extends $pb.GeneratedMessage {
  factory AgentSelection({
    $core.String? workspaceName,
  }) {
    final result = create();
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  AgentSelection._();

  factory AgentSelection.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory AgentSelection.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'AgentSelection',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AgentSelection clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AgentSelection copyWith(void Function(AgentSelection) updates) =>
      super.copyWith((message) => updates(message as AgentSelection))
          as AgentSelection;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static AgentSelection create() => AgentSelection._();
  @$core.override
  AgentSelection createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static AgentSelection getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<AgentSelection>(create);
  static AgentSelection? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get workspaceName => $_getSZ(0);
  @$pb.TagNumber(1)
  set workspaceName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasWorkspaceName() => $_has(0);
  @$pb.TagNumber(1)
  void clearWorkspaceName() => $_clearField(1);
}

class PeerRunAgent extends $pb.GeneratedMessage {
  factory PeerRunAgent({
    AgentSelection? active,
    AgentSelection? pending,
  }) {
    final result = create();
    if (active != null) result.active = active;
    if (pending != null) result.pending = pending;
    return result;
  }

  PeerRunAgent._();

  factory PeerRunAgent.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunAgent.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunAgent',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AgentSelection>(1, _omitFieldNames ? '' : 'active',
        subBuilder: AgentSelection.create)
    ..aOM<AgentSelection>(2, _omitFieldNames ? '' : 'pending',
        subBuilder: AgentSelection.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunAgent clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunAgent copyWith(void Function(PeerRunAgent) updates) =>
      super.copyWith((message) => updates(message as PeerRunAgent))
          as PeerRunAgent;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunAgent create() => PeerRunAgent._();
  @$core.override
  PeerRunAgent createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunAgent getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunAgent>(create);
  static PeerRunAgent? _defaultInstance;

  @$pb.TagNumber(1)
  AgentSelection get active => $_getN(0);
  @$pb.TagNumber(1)
  set active(AgentSelection value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasActive() => $_has(0);
  @$pb.TagNumber(1)
  void clearActive() => $_clearField(1);
  @$pb.TagNumber(1)
  AgentSelection ensureActive() => $_ensure(0);

  @$pb.TagNumber(2)
  AgentSelection get pending => $_getN(1);
  @$pb.TagNumber(2)
  set pending(AgentSelection value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasPending() => $_has(1);
  @$pb.TagNumber(2)
  void clearPending() => $_clearField(2);
  @$pb.TagNumber(2)
  AgentSelection ensurePending() => $_ensure(1);
}

class PeerRunHistoryEntry extends $pb.GeneratedMessage {
  factory PeerRunHistoryEntry({
    $core.String? createdAt,
    $core.String? gearId,
    $core.String? id,
    $core.String? name,
    $core.bool? replayAvailable,
    $core.String? text,
    $3.PeerRunHistoryEntryType? type,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (gearId != null) result.gearId = gearId;
    if (id != null) result.id = id;
    if (name != null) result.name = name;
    if (replayAvailable != null) result.replayAvailable = replayAvailable;
    if (text != null) result.text = text;
    if (type != null) result.type = type;
    return result;
  }

  PeerRunHistoryEntry._();

  factory PeerRunHistoryEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunHistoryEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunHistoryEntry',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'gearId')
    ..aOS(3, _omitFieldNames ? '' : 'id')
    ..aOS(4, _omitFieldNames ? '' : 'name')
    ..aOB(5, _omitFieldNames ? '' : 'replayAvailable')
    ..aOS(6, _omitFieldNames ? '' : 'text')
    ..aE<$3.PeerRunHistoryEntryType>(7, _omitFieldNames ? '' : 'type',
        enumValues: $3.PeerRunHistoryEntryType.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryEntry copyWith(void Function(PeerRunHistoryEntry) updates) =>
      super.copyWith((message) => updates(message as PeerRunHistoryEntry))
          as PeerRunHistoryEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryEntry create() => PeerRunHistoryEntry._();
  @$core.override
  PeerRunHistoryEntry createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryEntry getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunHistoryEntry>(create);
  static PeerRunHistoryEntry? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get gearId => $_getSZ(1);
  @$pb.TagNumber(2)
  set gearId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasGearId() => $_has(1);
  @$pb.TagNumber(2)
  void clearGearId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get id => $_getSZ(2);
  @$pb.TagNumber(3)
  set id($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasId() => $_has(2);
  @$pb.TagNumber(3)
  void clearId() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get name => $_getSZ(3);
  @$pb.TagNumber(4)
  set name($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasName() => $_has(3);
  @$pb.TagNumber(4)
  void clearName() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get replayAvailable => $_getBF(4);
  @$pb.TagNumber(5)
  set replayAvailable($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasReplayAvailable() => $_has(4);
  @$pb.TagNumber(5)
  void clearReplayAvailable() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get text => $_getSZ(5);
  @$pb.TagNumber(6)
  set text($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasText() => $_has(5);
  @$pb.TagNumber(6)
  void clearText() => $_clearField(6);

  @$pb.TagNumber(7)
  $3.PeerRunHistoryEntryType get type => $_getN(6);
  @$pb.TagNumber(7)
  set type($3.PeerRunHistoryEntryType value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasType() => $_has(6);
  @$pb.TagNumber(7)
  void clearType() => $_clearField(7);
}

class PeerRunHistoryListRequest extends $pb.GeneratedMessage {
  factory PeerRunHistoryListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
    $3.PeerRunHistoryListRequestOrder? order,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    if (order != null) result.order = order;
    return result;
  }

  PeerRunHistoryListRequest._();

  factory PeerRunHistoryListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunHistoryListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunHistoryListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..aE<$3.PeerRunHistoryListRequestOrder>(3, _omitFieldNames ? '' : 'order',
        enumValues: $3.PeerRunHistoryListRequestOrder.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryListRequest copyWith(
          void Function(PeerRunHistoryListRequest) updates) =>
      super.copyWith((message) => updates(message as PeerRunHistoryListRequest))
          as PeerRunHistoryListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryListRequest create() => PeerRunHistoryListRequest._();
  @$core.override
  PeerRunHistoryListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunHistoryListRequest>(create);
  static PeerRunHistoryListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);

  @$pb.TagNumber(3)
  $3.PeerRunHistoryListRequestOrder get order => $_getN(2);
  @$pb.TagNumber(3)
  set order($3.PeerRunHistoryListRequestOrder value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasOrder() => $_has(2);
  @$pb.TagNumber(3)
  void clearOrder() => $_clearField(3);
}

class PeerRunHistoryListResponse extends $pb.GeneratedMessage {
  factory PeerRunHistoryListResponse({
    $core.bool? available,
    $core.bool? hasNext,
    $core.Iterable<PeerRunHistoryEntry>? items,
    $core.String? message,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (available != null) result.available = available;
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (message != null) result.message = message;
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  PeerRunHistoryListResponse._();

  factory PeerRunHistoryListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunHistoryListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunHistoryListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'available')
    ..aOB(2, _omitFieldNames ? '' : 'hasNext')
    ..pPM<PeerRunHistoryEntry>(3, _omitFieldNames ? '' : 'items',
        subBuilder: PeerRunHistoryEntry.create)
    ..aOS(4, _omitFieldNames ? '' : 'message')
    ..aOS(5, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryListResponse copyWith(
          void Function(PeerRunHistoryListResponse) updates) =>
      super.copyWith(
              (message) => updates(message as PeerRunHistoryListResponse))
          as PeerRunHistoryListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryListResponse create() => PeerRunHistoryListResponse._();
  @$core.override
  PeerRunHistoryListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunHistoryListResponse>(create);
  static PeerRunHistoryListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get available => $_getBF(0);
  @$pb.TagNumber(1)
  set available($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAvailable() => $_has(0);
  @$pb.TagNumber(1)
  void clearAvailable() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get hasNext => $_getBF(1);
  @$pb.TagNumber(2)
  set hasNext($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasHasNext() => $_has(1);
  @$pb.TagNumber(2)
  void clearHasNext() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbList<PeerRunHistoryEntry> get items => $_getList(2);

  @$pb.TagNumber(4)
  $core.String get message => $_getSZ(3);
  @$pb.TagNumber(4)
  set message($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasMessage() => $_has(3);
  @$pb.TagNumber(4)
  void clearMessage() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get nextCursor => $_getSZ(4);
  @$pb.TagNumber(5)
  set nextCursor($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasNextCursor() => $_has(4);
  @$pb.TagNumber(5)
  void clearNextCursor() => $_clearField(5);
}

class PeerRunHistoryPlayRequest extends $pb.GeneratedMessage {
  factory PeerRunHistoryPlayRequest({
    $core.String? historyId,
  }) {
    final result = create();
    if (historyId != null) result.historyId = historyId;
    return result;
  }

  PeerRunHistoryPlayRequest._();

  factory PeerRunHistoryPlayRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunHistoryPlayRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunHistoryPlayRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'historyId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryPlayRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryPlayRequest copyWith(
          void Function(PeerRunHistoryPlayRequest) updates) =>
      super.copyWith((message) => updates(message as PeerRunHistoryPlayRequest))
          as PeerRunHistoryPlayRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryPlayRequest create() => PeerRunHistoryPlayRequest._();
  @$core.override
  PeerRunHistoryPlayRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryPlayRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunHistoryPlayRequest>(create);
  static PeerRunHistoryPlayRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get historyId => $_getSZ(0);
  @$pb.TagNumber(1)
  set historyId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHistoryId() => $_has(0);
  @$pb.TagNumber(1)
  void clearHistoryId() => $_clearField(1);
}

class PeerRunHistoryPlayResponse extends $pb.GeneratedMessage {
  factory PeerRunHistoryPlayResponse({
    $core.bool? accepted,
    $core.String? historyId,
    $core.String? message,
    $core.String? state,
  }) {
    final result = create();
    if (accepted != null) result.accepted = accepted;
    if (historyId != null) result.historyId = historyId;
    if (message != null) result.message = message;
    if (state != null) result.state = state;
    return result;
  }

  PeerRunHistoryPlayResponse._();

  factory PeerRunHistoryPlayResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunHistoryPlayResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunHistoryPlayResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'accepted')
    ..aOS(2, _omitFieldNames ? '' : 'historyId')
    ..aOS(3, _omitFieldNames ? '' : 'message')
    ..aOS(4, _omitFieldNames ? '' : 'state')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryPlayResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunHistoryPlayResponse copyWith(
          void Function(PeerRunHistoryPlayResponse) updates) =>
      super.copyWith(
              (message) => updates(message as PeerRunHistoryPlayResponse))
          as PeerRunHistoryPlayResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryPlayResponse create() => PeerRunHistoryPlayResponse._();
  @$core.override
  PeerRunHistoryPlayResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunHistoryPlayResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunHistoryPlayResponse>(create);
  static PeerRunHistoryPlayResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get accepted => $_getBF(0);
  @$pb.TagNumber(1)
  set accepted($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAccepted() => $_has(0);
  @$pb.TagNumber(1)
  void clearAccepted() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get historyId => $_getSZ(1);
  @$pb.TagNumber(2)
  set historyId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasHistoryId() => $_has(1);
  @$pb.TagNumber(2)
  void clearHistoryId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get message => $_getSZ(2);
  @$pb.TagNumber(3)
  set message($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMessage() => $_has(2);
  @$pb.TagNumber(3)
  void clearMessage() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get state => $_getSZ(3);
  @$pb.TagNumber(4)
  set state($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasState() => $_has(3);
  @$pb.TagNumber(4)
  void clearState() => $_clearField(4);
}

class PeerRunMemoryStatsRequest extends $pb.GeneratedMessage {
  factory PeerRunMemoryStatsRequest() => create();

  PeerRunMemoryStatsRequest._();

  factory PeerRunMemoryStatsRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunMemoryStatsRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunMemoryStatsRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunMemoryStatsRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunMemoryStatsRequest copyWith(
          void Function(PeerRunMemoryStatsRequest) updates) =>
      super.copyWith((message) => updates(message as PeerRunMemoryStatsRequest))
          as PeerRunMemoryStatsRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunMemoryStatsRequest create() => PeerRunMemoryStatsRequest._();
  @$core.override
  PeerRunMemoryStatsRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunMemoryStatsRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunMemoryStatsRequest>(create);
  static PeerRunMemoryStatsRequest? _defaultInstance;
}

class PeerRunMemoryStatsResponse extends $pb.GeneratedMessage {
  factory PeerRunMemoryStatsResponse({
    $core.bool? available,
    $core.String? backend,
    $core.bool? embeddingEnabled,
    $core.String? embeddingStatus,
    $core.bool? enabled,
    $core.String? indexStatus,
    $fixnum.Int64? itemCount,
    $core.String? lastUpdatedAt,
    $core.String? message,
    $0.Struct? metadata,
    $fixnum.Int64? storageBytes,
  }) {
    final result = create();
    if (available != null) result.available = available;
    if (backend != null) result.backend = backend;
    if (embeddingEnabled != null) result.embeddingEnabled = embeddingEnabled;
    if (embeddingStatus != null) result.embeddingStatus = embeddingStatus;
    if (enabled != null) result.enabled = enabled;
    if (indexStatus != null) result.indexStatus = indexStatus;
    if (itemCount != null) result.itemCount = itemCount;
    if (lastUpdatedAt != null) result.lastUpdatedAt = lastUpdatedAt;
    if (message != null) result.message = message;
    if (metadata != null) result.metadata = metadata;
    if (storageBytes != null) result.storageBytes = storageBytes;
    return result;
  }

  PeerRunMemoryStatsResponse._();

  factory PeerRunMemoryStatsResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunMemoryStatsResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunMemoryStatsResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'available')
    ..aOS(2, _omitFieldNames ? '' : 'backend')
    ..aOB(3, _omitFieldNames ? '' : 'embeddingEnabled')
    ..aOS(4, _omitFieldNames ? '' : 'embeddingStatus')
    ..aOB(5, _omitFieldNames ? '' : 'enabled')
    ..aOS(6, _omitFieldNames ? '' : 'indexStatus')
    ..aInt64(7, _omitFieldNames ? '' : 'itemCount')
    ..aOS(8, _omitFieldNames ? '' : 'lastUpdatedAt')
    ..aOS(9, _omitFieldNames ? '' : 'message')
    ..aOM<$0.Struct>(10, _omitFieldNames ? '' : 'metadata',
        subBuilder: $0.Struct.create)
    ..aInt64(11, _omitFieldNames ? '' : 'storageBytes')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunMemoryStatsResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunMemoryStatsResponse copyWith(
          void Function(PeerRunMemoryStatsResponse) updates) =>
      super.copyWith(
              (message) => updates(message as PeerRunMemoryStatsResponse))
          as PeerRunMemoryStatsResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunMemoryStatsResponse create() => PeerRunMemoryStatsResponse._();
  @$core.override
  PeerRunMemoryStatsResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunMemoryStatsResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunMemoryStatsResponse>(create);
  static PeerRunMemoryStatsResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get available => $_getBF(0);
  @$pb.TagNumber(1)
  set available($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAvailable() => $_has(0);
  @$pb.TagNumber(1)
  void clearAvailable() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get backend => $_getSZ(1);
  @$pb.TagNumber(2)
  set backend($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBackend() => $_has(1);
  @$pb.TagNumber(2)
  void clearBackend() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get embeddingEnabled => $_getBF(2);
  @$pb.TagNumber(3)
  set embeddingEnabled($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasEmbeddingEnabled() => $_has(2);
  @$pb.TagNumber(3)
  void clearEmbeddingEnabled() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get embeddingStatus => $_getSZ(3);
  @$pb.TagNumber(4)
  set embeddingStatus($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasEmbeddingStatus() => $_has(3);
  @$pb.TagNumber(4)
  void clearEmbeddingStatus() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get enabled => $_getBF(4);
  @$pb.TagNumber(5)
  set enabled($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasEnabled() => $_has(4);
  @$pb.TagNumber(5)
  void clearEnabled() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get indexStatus => $_getSZ(5);
  @$pb.TagNumber(6)
  set indexStatus($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasIndexStatus() => $_has(5);
  @$pb.TagNumber(6)
  void clearIndexStatus() => $_clearField(6);

  @$pb.TagNumber(7)
  $fixnum.Int64 get itemCount => $_getI64(6);
  @$pb.TagNumber(7)
  set itemCount($fixnum.Int64 value) => $_setInt64(6, value);
  @$pb.TagNumber(7)
  $core.bool hasItemCount() => $_has(6);
  @$pb.TagNumber(7)
  void clearItemCount() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get lastUpdatedAt => $_getSZ(7);
  @$pb.TagNumber(8)
  set lastUpdatedAt($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasLastUpdatedAt() => $_has(7);
  @$pb.TagNumber(8)
  void clearLastUpdatedAt() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get message => $_getSZ(8);
  @$pb.TagNumber(9)
  set message($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasMessage() => $_has(8);
  @$pb.TagNumber(9)
  void clearMessage() => $_clearField(9);

  @$pb.TagNumber(10)
  $0.Struct get metadata => $_getN(9);
  @$pb.TagNumber(10)
  set metadata($0.Struct value) => $_setField(10, value);
  @$pb.TagNumber(10)
  $core.bool hasMetadata() => $_has(9);
  @$pb.TagNumber(10)
  void clearMetadata() => $_clearField(10);
  @$pb.TagNumber(10)
  $0.Struct ensureMetadata() => $_ensure(9);

  @$pb.TagNumber(11)
  $fixnum.Int64 get storageBytes => $_getI64(10);
  @$pb.TagNumber(11)
  set storageBytes($fixnum.Int64 value) => $_setInt64(10, value);
  @$pb.TagNumber(11)
  $core.bool hasStorageBytes() => $_has(10);
  @$pb.TagNumber(11)
  void clearStorageBytes() => $_clearField(11);
}

class PeerRunRecallHit extends $pb.GeneratedMessage {
  factory PeerRunRecallHit({
    $core.String? createdAt,
    $core.String? id,
    $0.Struct? metadata,
    $core.double? score,
    $core.String? snippet,
    $core.String? sourceId,
    $core.String? sourceType,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (id != null) result.id = id;
    if (metadata != null) result.metadata = metadata;
    if (score != null) result.score = score;
    if (snippet != null) result.snippet = snippet;
    if (sourceId != null) result.sourceId = sourceId;
    if (sourceType != null) result.sourceType = sourceType;
    return result;
  }

  PeerRunRecallHit._();

  factory PeerRunRecallHit.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunRecallHit.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunRecallHit',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..aOM<$0.Struct>(3, _omitFieldNames ? '' : 'metadata',
        subBuilder: $0.Struct.create)
    ..aD(4, _omitFieldNames ? '' : 'score')
    ..aOS(5, _omitFieldNames ? '' : 'snippet')
    ..aOS(6, _omitFieldNames ? '' : 'sourceId')
    ..aOS(7, _omitFieldNames ? '' : 'sourceType')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunRecallHit clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunRecallHit copyWith(void Function(PeerRunRecallHit) updates) =>
      super.copyWith((message) => updates(message as PeerRunRecallHit))
          as PeerRunRecallHit;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunRecallHit create() => PeerRunRecallHit._();
  @$core.override
  PeerRunRecallHit createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunRecallHit getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunRecallHit>(create);
  static PeerRunRecallHit? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);

  @$pb.TagNumber(3)
  $0.Struct get metadata => $_getN(2);
  @$pb.TagNumber(3)
  set metadata($0.Struct value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasMetadata() => $_has(2);
  @$pb.TagNumber(3)
  void clearMetadata() => $_clearField(3);
  @$pb.TagNumber(3)
  $0.Struct ensureMetadata() => $_ensure(2);

  @$pb.TagNumber(4)
  $core.double get score => $_getN(3);
  @$pb.TagNumber(4)
  set score($core.double value) => $_setDouble(3, value);
  @$pb.TagNumber(4)
  $core.bool hasScore() => $_has(3);
  @$pb.TagNumber(4)
  void clearScore() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get snippet => $_getSZ(4);
  @$pb.TagNumber(5)
  set snippet($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSnippet() => $_has(4);
  @$pb.TagNumber(5)
  void clearSnippet() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get sourceId => $_getSZ(5);
  @$pb.TagNumber(6)
  set sourceId($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSourceId() => $_has(5);
  @$pb.TagNumber(6)
  void clearSourceId() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get sourceType => $_getSZ(6);
  @$pb.TagNumber(7)
  set sourceType($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasSourceType() => $_has(6);
  @$pb.TagNumber(7)
  void clearSourceType() => $_clearField(7);
}

class PeerRunRecallRequest extends $pb.GeneratedMessage {
  factory PeerRunRecallRequest({
    $0.Struct? filters,
    $fixnum.Int64? limit,
    $core.String? query,
  }) {
    final result = create();
    if (filters != null) result.filters = filters;
    if (limit != null) result.limit = limit;
    if (query != null) result.query = query;
    return result;
  }

  PeerRunRecallRequest._();

  factory PeerRunRecallRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunRecallRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunRecallRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<$0.Struct>(1, _omitFieldNames ? '' : 'filters',
        subBuilder: $0.Struct.create)
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..aOS(3, _omitFieldNames ? '' : 'query')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunRecallRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunRecallRequest copyWith(void Function(PeerRunRecallRequest) updates) =>
      super.copyWith((message) => updates(message as PeerRunRecallRequest))
          as PeerRunRecallRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunRecallRequest create() => PeerRunRecallRequest._();
  @$core.override
  PeerRunRecallRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunRecallRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunRecallRequest>(create);
  static PeerRunRecallRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $0.Struct get filters => $_getN(0);
  @$pb.TagNumber(1)
  set filters($0.Struct value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasFilters() => $_has(0);
  @$pb.TagNumber(1)
  void clearFilters() => $_clearField(1);
  @$pb.TagNumber(1)
  $0.Struct ensureFilters() => $_ensure(0);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get query => $_getSZ(2);
  @$pb.TagNumber(3)
  set query($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasQuery() => $_has(2);
  @$pb.TagNumber(3)
  void clearQuery() => $_clearField(3);
}

class PeerRunRecallResponse extends $pb.GeneratedMessage {
  factory PeerRunRecallResponse({
    $core.bool? available,
    $core.Iterable<PeerRunRecallHit>? hits,
    $core.String? message,
  }) {
    final result = create();
    if (available != null) result.available = available;
    if (hits != null) result.hits.addAll(hits);
    if (message != null) result.message = message;
    return result;
  }

  PeerRunRecallResponse._();

  factory PeerRunRecallResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunRecallResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunRecallResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'available')
    ..pPM<PeerRunRecallHit>(2, _omitFieldNames ? '' : 'hits',
        subBuilder: PeerRunRecallHit.create)
    ..aOS(3, _omitFieldNames ? '' : 'message')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunRecallResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunRecallResponse copyWith(
          void Function(PeerRunRecallResponse) updates) =>
      super.copyWith((message) => updates(message as PeerRunRecallResponse))
          as PeerRunRecallResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunRecallResponse create() => PeerRunRecallResponse._();
  @$core.override
  PeerRunRecallResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunRecallResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunRecallResponse>(create);
  static PeerRunRecallResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get available => $_getBF(0);
  @$pb.TagNumber(1)
  set available($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAvailable() => $_has(0);
  @$pb.TagNumber(1)
  void clearAvailable() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<PeerRunRecallHit> get hits => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get message => $_getSZ(2);
  @$pb.TagNumber(3)
  set message($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMessage() => $_has(2);
  @$pb.TagNumber(3)
  void clearMessage() => $_clearField(3);
}

class PeerRunStatus extends $pb.GeneratedMessage {
  factory PeerRunStatus({
    $core.String? message,
    $core.String? startedAt,
    $3.PeerRunStatusState? state,
    $core.String? updatedAt,
    $core.String? workspaceName,
  }) {
    final result = create();
    if (message != null) result.message = message;
    if (startedAt != null) result.startedAt = startedAt;
    if (state != null) result.state = state;
    if (updatedAt != null) result.updatedAt = updatedAt;
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  PeerRunStatus._();

  factory PeerRunStatus.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunStatus.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunStatus',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'message')
    ..aOS(2, _omitFieldNames ? '' : 'startedAt')
    ..aE<$3.PeerRunStatusState>(3, _omitFieldNames ? '' : 'state',
        enumValues: $3.PeerRunStatusState.values)
    ..aOS(4, _omitFieldNames ? '' : 'updatedAt')
    ..aOS(5, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunStatus clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunStatus copyWith(void Function(PeerRunStatus) updates) =>
      super.copyWith((message) => updates(message as PeerRunStatus))
          as PeerRunStatus;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunStatus create() => PeerRunStatus._();
  @$core.override
  PeerRunStatus createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunStatus getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunStatus>(create);
  static PeerRunStatus? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get message => $_getSZ(0);
  @$pb.TagNumber(1)
  set message($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasMessage() => $_has(0);
  @$pb.TagNumber(1)
  void clearMessage() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get startedAt => $_getSZ(1);
  @$pb.TagNumber(2)
  set startedAt($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasStartedAt() => $_has(1);
  @$pb.TagNumber(2)
  void clearStartedAt() => $_clearField(2);

  @$pb.TagNumber(3)
  $3.PeerRunStatusState get state => $_getN(2);
  @$pb.TagNumber(3)
  set state($3.PeerRunStatusState value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasState() => $_has(2);
  @$pb.TagNumber(3)
  void clearState() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get updatedAt => $_getSZ(3);
  @$pb.TagNumber(4)
  set updatedAt($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasUpdatedAt() => $_has(3);
  @$pb.TagNumber(4)
  void clearUpdatedAt() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get workspaceName => $_getSZ(4);
  @$pb.TagNumber(5)
  set workspaceName($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasWorkspaceName() => $_has(4);
  @$pb.TagNumber(5)
  void clearWorkspaceName() => $_clearField(5);
}

class PeerRunWorkspaceState extends $pb.GeneratedMessage {
  factory PeerRunWorkspaceState({
    $core.String? activeWorkspaceName,
    $core.String? agentType,
    $core.bool? historyAvailable,
    $core.bool? memoryStatsAvailable,
    $core.String? message,
    $core.String? pendingWorkspaceName,
    $core.bool? recallAvailable,
    $3.PeerRunStatusState? runtimeState,
    $core.String? selectedWorkspaceName,
    $core.String? startedAt,
    $core.String? updatedAt,
    $core.String? workflowName,
    $core.String? workspaceName,
  }) {
    final result = create();
    if (activeWorkspaceName != null)
      result.activeWorkspaceName = activeWorkspaceName;
    if (agentType != null) result.agentType = agentType;
    if (historyAvailable != null) result.historyAvailable = historyAvailable;
    if (memoryStatsAvailable != null)
      result.memoryStatsAvailable = memoryStatsAvailable;
    if (message != null) result.message = message;
    if (pendingWorkspaceName != null)
      result.pendingWorkspaceName = pendingWorkspaceName;
    if (recallAvailable != null) result.recallAvailable = recallAvailable;
    if (runtimeState != null) result.runtimeState = runtimeState;
    if (selectedWorkspaceName != null)
      result.selectedWorkspaceName = selectedWorkspaceName;
    if (startedAt != null) result.startedAt = startedAt;
    if (updatedAt != null) result.updatedAt = updatedAt;
    if (workflowName != null) result.workflowName = workflowName;
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  PeerRunWorkspaceState._();

  factory PeerRunWorkspaceState.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerRunWorkspaceState.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerRunWorkspaceState',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'activeWorkspaceName')
    ..aOS(2, _omitFieldNames ? '' : 'agentType')
    ..aOB(3, _omitFieldNames ? '' : 'historyAvailable')
    ..aOB(4, _omitFieldNames ? '' : 'memoryStatsAvailable')
    ..aOS(5, _omitFieldNames ? '' : 'message')
    ..aOS(6, _omitFieldNames ? '' : 'pendingWorkspaceName')
    ..aOB(7, _omitFieldNames ? '' : 'recallAvailable')
    ..aE<$3.PeerRunStatusState>(8, _omitFieldNames ? '' : 'runtimeState',
        enumValues: $3.PeerRunStatusState.values)
    ..aOS(9, _omitFieldNames ? '' : 'selectedWorkspaceName')
    ..aOS(10, _omitFieldNames ? '' : 'startedAt')
    ..aOS(11, _omitFieldNames ? '' : 'updatedAt')
    ..aOS(12, _omitFieldNames ? '' : 'workflowName')
    ..aOS(13, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunWorkspaceState clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerRunWorkspaceState copyWith(
          void Function(PeerRunWorkspaceState) updates) =>
      super.copyWith((message) => updates(message as PeerRunWorkspaceState))
          as PeerRunWorkspaceState;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerRunWorkspaceState create() => PeerRunWorkspaceState._();
  @$core.override
  PeerRunWorkspaceState createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerRunWorkspaceState getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerRunWorkspaceState>(create);
  static PeerRunWorkspaceState? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get activeWorkspaceName => $_getSZ(0);
  @$pb.TagNumber(1)
  set activeWorkspaceName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasActiveWorkspaceName() => $_has(0);
  @$pb.TagNumber(1)
  void clearActiveWorkspaceName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get agentType => $_getSZ(1);
  @$pb.TagNumber(2)
  set agentType($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasAgentType() => $_has(1);
  @$pb.TagNumber(2)
  void clearAgentType() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get historyAvailable => $_getBF(2);
  @$pb.TagNumber(3)
  set historyAvailable($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasHistoryAvailable() => $_has(2);
  @$pb.TagNumber(3)
  void clearHistoryAvailable() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get memoryStatsAvailable => $_getBF(3);
  @$pb.TagNumber(4)
  set memoryStatsAvailable($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasMemoryStatsAvailable() => $_has(3);
  @$pb.TagNumber(4)
  void clearMemoryStatsAvailable() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get message => $_getSZ(4);
  @$pb.TagNumber(5)
  set message($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasMessage() => $_has(4);
  @$pb.TagNumber(5)
  void clearMessage() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get pendingWorkspaceName => $_getSZ(5);
  @$pb.TagNumber(6)
  set pendingWorkspaceName($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasPendingWorkspaceName() => $_has(5);
  @$pb.TagNumber(6)
  void clearPendingWorkspaceName() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get recallAvailable => $_getBF(6);
  @$pb.TagNumber(7)
  set recallAvailable($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasRecallAvailable() => $_has(6);
  @$pb.TagNumber(7)
  void clearRecallAvailable() => $_clearField(7);

  @$pb.TagNumber(8)
  $3.PeerRunStatusState get runtimeState => $_getN(7);
  @$pb.TagNumber(8)
  set runtimeState($3.PeerRunStatusState value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasRuntimeState() => $_has(7);
  @$pb.TagNumber(8)
  void clearRuntimeState() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get selectedWorkspaceName => $_getSZ(8);
  @$pb.TagNumber(9)
  set selectedWorkspaceName($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasSelectedWorkspaceName() => $_has(8);
  @$pb.TagNumber(9)
  void clearSelectedWorkspaceName() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get startedAt => $_getSZ(9);
  @$pb.TagNumber(10)
  set startedAt($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasStartedAt() => $_has(9);
  @$pb.TagNumber(10)
  void clearStartedAt() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.String get updatedAt => $_getSZ(10);
  @$pb.TagNumber(11)
  set updatedAt($core.String value) => $_setString(10, value);
  @$pb.TagNumber(11)
  $core.bool hasUpdatedAt() => $_has(10);
  @$pb.TagNumber(11)
  void clearUpdatedAt() => $_clearField(11);

  @$pb.TagNumber(12)
  $core.String get workflowName => $_getSZ(11);
  @$pb.TagNumber(12)
  set workflowName($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasWorkflowName() => $_has(11);
  @$pb.TagNumber(12)
  void clearWorkflowName() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.String get workspaceName => $_getSZ(12);
  @$pb.TagNumber(13)
  set workspaceName($core.String value) => $_setString(12, value);
  @$pb.TagNumber(13)
  $core.bool hasWorkspaceName() => $_has(12);
  @$pb.TagNumber(13)
  void clearWorkspaceName() => $_clearField(13);
}

class ServerGetRunAgentRequest extends $pb.GeneratedMessage {
  factory ServerGetRunAgentRequest() => create();

  ServerGetRunAgentRequest._();

  factory ServerGetRunAgentRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRunAgentRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRunAgentRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunAgentRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunAgentRequest copyWith(
          void Function(ServerGetRunAgentRequest) updates) =>
      super.copyWith((message) => updates(message as ServerGetRunAgentRequest))
          as ServerGetRunAgentRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRunAgentRequest create() => ServerGetRunAgentRequest._();
  @$core.override
  ServerGetRunAgentRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRunAgentRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetRunAgentRequest>(create);
  static ServerGetRunAgentRequest? _defaultInstance;
}

class ServerGetRunAgentResponse extends $pb.GeneratedMessage {
  factory ServerGetRunAgentResponse({
    PeerRunAgent? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGetRunAgentResponse._();

  factory ServerGetRunAgentResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRunAgentResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRunAgentResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunAgent>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunAgent.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunAgentResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunAgentResponse copyWith(
          void Function(ServerGetRunAgentResponse) updates) =>
      super.copyWith((message) => updates(message as ServerGetRunAgentResponse))
          as ServerGetRunAgentResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRunAgentResponse create() => ServerGetRunAgentResponse._();
  @$core.override
  ServerGetRunAgentResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRunAgentResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetRunAgentResponse>(create);
  static ServerGetRunAgentResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunAgent get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunAgent value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunAgent ensureValue() => $_ensure(0);
}

class ServerGetRunStatusRequest extends $pb.GeneratedMessage {
  factory ServerGetRunStatusRequest() => create();

  ServerGetRunStatusRequest._();

  factory ServerGetRunStatusRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRunStatusRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRunStatusRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunStatusRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunStatusRequest copyWith(
          void Function(ServerGetRunStatusRequest) updates) =>
      super.copyWith((message) => updates(message as ServerGetRunStatusRequest))
          as ServerGetRunStatusRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRunStatusRequest create() => ServerGetRunStatusRequest._();
  @$core.override
  ServerGetRunStatusRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRunStatusRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetRunStatusRequest>(create);
  static ServerGetRunStatusRequest? _defaultInstance;
}

class ServerGetRunStatusResponse extends $pb.GeneratedMessage {
  factory ServerGetRunStatusResponse({
    PeerRunStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGetRunStatusResponse._();

  factory ServerGetRunStatusResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRunStatusResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRunStatusResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunStatusResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunStatusResponse copyWith(
          void Function(ServerGetRunStatusResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGetRunStatusResponse))
          as ServerGetRunStatusResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRunStatusResponse create() => ServerGetRunStatusResponse._();
  @$core.override
  ServerGetRunStatusResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRunStatusResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetRunStatusResponse>(create);
  static ServerGetRunStatusResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunStatus ensureValue() => $_ensure(0);
}

class ServerGetRunWorkspaceMemoryStatsRequest extends $pb.GeneratedMessage {
  factory ServerGetRunWorkspaceMemoryStatsRequest({
    PeerRunMemoryStatsRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGetRunWorkspaceMemoryStatsRequest._();

  factory ServerGetRunWorkspaceMemoryStatsRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRunWorkspaceMemoryStatsRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRunWorkspaceMemoryStatsRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunMemoryStatsRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunMemoryStatsRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunWorkspaceMemoryStatsRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunWorkspaceMemoryStatsRequest copyWith(
          void Function(ServerGetRunWorkspaceMemoryStatsRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ServerGetRunWorkspaceMemoryStatsRequest))
          as ServerGetRunWorkspaceMemoryStatsRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRunWorkspaceMemoryStatsRequest create() =>
      ServerGetRunWorkspaceMemoryStatsRequest._();
  @$core.override
  ServerGetRunWorkspaceMemoryStatsRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRunWorkspaceMemoryStatsRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ServerGetRunWorkspaceMemoryStatsRequest>(create);
  static ServerGetRunWorkspaceMemoryStatsRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunMemoryStatsRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunMemoryStatsRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunMemoryStatsRequest ensureValue() => $_ensure(0);
}

class ServerGetRunWorkspaceMemoryStatsResponse extends $pb.GeneratedMessage {
  factory ServerGetRunWorkspaceMemoryStatsResponse({
    PeerRunMemoryStatsResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGetRunWorkspaceMemoryStatsResponse._();

  factory ServerGetRunWorkspaceMemoryStatsResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRunWorkspaceMemoryStatsResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRunWorkspaceMemoryStatsResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunMemoryStatsResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunMemoryStatsResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunWorkspaceMemoryStatsResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunWorkspaceMemoryStatsResponse copyWith(
          void Function(ServerGetRunWorkspaceMemoryStatsResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ServerGetRunWorkspaceMemoryStatsResponse))
          as ServerGetRunWorkspaceMemoryStatsResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRunWorkspaceMemoryStatsResponse create() =>
      ServerGetRunWorkspaceMemoryStatsResponse._();
  @$core.override
  ServerGetRunWorkspaceMemoryStatsResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRunWorkspaceMemoryStatsResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ServerGetRunWorkspaceMemoryStatsResponse>(create);
  static ServerGetRunWorkspaceMemoryStatsResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunMemoryStatsResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunMemoryStatsResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunMemoryStatsResponse ensureValue() => $_ensure(0);
}

class ServerGetRunWorkspaceRequest extends $pb.GeneratedMessage {
  factory ServerGetRunWorkspaceRequest() => create();

  ServerGetRunWorkspaceRequest._();

  factory ServerGetRunWorkspaceRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRunWorkspaceRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRunWorkspaceRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunWorkspaceRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunWorkspaceRequest copyWith(
          void Function(ServerGetRunWorkspaceRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGetRunWorkspaceRequest))
          as ServerGetRunWorkspaceRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRunWorkspaceRequest create() =>
      ServerGetRunWorkspaceRequest._();
  @$core.override
  ServerGetRunWorkspaceRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRunWorkspaceRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetRunWorkspaceRequest>(create);
  static ServerGetRunWorkspaceRequest? _defaultInstance;
}

class ServerGetRunWorkspaceResponse extends $pb.GeneratedMessage {
  factory ServerGetRunWorkspaceResponse({
    PeerRunWorkspaceState? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGetRunWorkspaceResponse._();

  factory ServerGetRunWorkspaceResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRunWorkspaceResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRunWorkspaceResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunWorkspaceState>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunWorkspaceState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunWorkspaceResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRunWorkspaceResponse copyWith(
          void Function(ServerGetRunWorkspaceResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGetRunWorkspaceResponse))
          as ServerGetRunWorkspaceResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRunWorkspaceResponse create() =>
      ServerGetRunWorkspaceResponse._();
  @$core.override
  ServerGetRunWorkspaceResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRunWorkspaceResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetRunWorkspaceResponse>(create);
  static ServerGetRunWorkspaceResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunWorkspaceState get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunWorkspaceState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunWorkspaceState ensureValue() => $_ensure(0);
}

class ServerGetRuntimeRequest extends $pb.GeneratedMessage {
  factory ServerGetRuntimeRequest() => create();

  ServerGetRuntimeRequest._();

  factory ServerGetRuntimeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRuntimeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRuntimeRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRuntimeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRuntimeRequest copyWith(
          void Function(ServerGetRuntimeRequest) updates) =>
      super.copyWith((message) => updates(message as ServerGetRuntimeRequest))
          as ServerGetRuntimeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRuntimeRequest create() => ServerGetRuntimeRequest._();
  @$core.override
  ServerGetRuntimeRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRuntimeRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetRuntimeRequest>(create);
  static ServerGetRuntimeRequest? _defaultInstance;
}

class ServerGetRuntimeResponse extends $pb.GeneratedMessage {
  factory ServerGetRuntimeResponse({
    $1.Runtime? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGetRuntimeResponse._();

  factory ServerGetRuntimeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetRuntimeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetRuntimeResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<$1.Runtime>(1, _omitFieldNames ? '' : 'value',
        subBuilder: $1.Runtime.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRuntimeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetRuntimeResponse copyWith(
          void Function(ServerGetRuntimeResponse) updates) =>
      super.copyWith((message) => updates(message as ServerGetRuntimeResponse))
          as ServerGetRuntimeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetRuntimeResponse create() => ServerGetRuntimeResponse._();
  @$core.override
  ServerGetRuntimeResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetRuntimeResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetRuntimeResponse>(create);
  static ServerGetRuntimeResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $1.Runtime get value => $_getN(0);
  @$pb.TagNumber(1)
  set value($1.Runtime value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  $1.Runtime ensureValue() => $_ensure(0);
}

class ServerListRunWorkspaceHistoryRequest extends $pb.GeneratedMessage {
  factory ServerListRunWorkspaceHistoryRequest({
    PeerRunHistoryListRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerListRunWorkspaceHistoryRequest._();

  factory ServerListRunWorkspaceHistoryRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerListRunWorkspaceHistoryRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerListRunWorkspaceHistoryRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunHistoryListRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunHistoryListRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerListRunWorkspaceHistoryRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerListRunWorkspaceHistoryRequest copyWith(
          void Function(ServerListRunWorkspaceHistoryRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ServerListRunWorkspaceHistoryRequest))
          as ServerListRunWorkspaceHistoryRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerListRunWorkspaceHistoryRequest create() =>
      ServerListRunWorkspaceHistoryRequest._();
  @$core.override
  ServerListRunWorkspaceHistoryRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerListRunWorkspaceHistoryRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ServerListRunWorkspaceHistoryRequest>(create);
  static ServerListRunWorkspaceHistoryRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunHistoryListRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunHistoryListRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunHistoryListRequest ensureValue() => $_ensure(0);
}

class ServerListRunWorkspaceHistoryResponse extends $pb.GeneratedMessage {
  factory ServerListRunWorkspaceHistoryResponse({
    PeerRunHistoryListResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerListRunWorkspaceHistoryResponse._();

  factory ServerListRunWorkspaceHistoryResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerListRunWorkspaceHistoryResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerListRunWorkspaceHistoryResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunHistoryListResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunHistoryListResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerListRunWorkspaceHistoryResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerListRunWorkspaceHistoryResponse copyWith(
          void Function(ServerListRunWorkspaceHistoryResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ServerListRunWorkspaceHistoryResponse))
          as ServerListRunWorkspaceHistoryResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerListRunWorkspaceHistoryResponse create() =>
      ServerListRunWorkspaceHistoryResponse._();
  @$core.override
  ServerListRunWorkspaceHistoryResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerListRunWorkspaceHistoryResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ServerListRunWorkspaceHistoryResponse>(create);
  static ServerListRunWorkspaceHistoryResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunHistoryListResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunHistoryListResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunHistoryListResponse ensureValue() => $_ensure(0);
}

class ServerPlayRunWorkspaceHistoryRequest extends $pb.GeneratedMessage {
  factory ServerPlayRunWorkspaceHistoryRequest({
    PeerRunHistoryPlayRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPlayRunWorkspaceHistoryRequest._();

  factory ServerPlayRunWorkspaceHistoryRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPlayRunWorkspaceHistoryRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPlayRunWorkspaceHistoryRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunHistoryPlayRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunHistoryPlayRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPlayRunWorkspaceHistoryRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPlayRunWorkspaceHistoryRequest copyWith(
          void Function(ServerPlayRunWorkspaceHistoryRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ServerPlayRunWorkspaceHistoryRequest))
          as ServerPlayRunWorkspaceHistoryRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPlayRunWorkspaceHistoryRequest create() =>
      ServerPlayRunWorkspaceHistoryRequest._();
  @$core.override
  ServerPlayRunWorkspaceHistoryRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPlayRunWorkspaceHistoryRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ServerPlayRunWorkspaceHistoryRequest>(create);
  static ServerPlayRunWorkspaceHistoryRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunHistoryPlayRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunHistoryPlayRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunHistoryPlayRequest ensureValue() => $_ensure(0);
}

class ServerPlayRunWorkspaceHistoryResponse extends $pb.GeneratedMessage {
  factory ServerPlayRunWorkspaceHistoryResponse({
    PeerRunHistoryPlayResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPlayRunWorkspaceHistoryResponse._();

  factory ServerPlayRunWorkspaceHistoryResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPlayRunWorkspaceHistoryResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPlayRunWorkspaceHistoryResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunHistoryPlayResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunHistoryPlayResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPlayRunWorkspaceHistoryResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPlayRunWorkspaceHistoryResponse copyWith(
          void Function(ServerPlayRunWorkspaceHistoryResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ServerPlayRunWorkspaceHistoryResponse))
          as ServerPlayRunWorkspaceHistoryResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPlayRunWorkspaceHistoryResponse create() =>
      ServerPlayRunWorkspaceHistoryResponse._();
  @$core.override
  ServerPlayRunWorkspaceHistoryResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPlayRunWorkspaceHistoryResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ServerPlayRunWorkspaceHistoryResponse>(create);
  static ServerPlayRunWorkspaceHistoryResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunHistoryPlayResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunHistoryPlayResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunHistoryPlayResponse ensureValue() => $_ensure(0);
}

class ServerReloadRunRequest extends $pb.GeneratedMessage {
  factory ServerReloadRunRequest() => create();

  ServerReloadRunRequest._();

  factory ServerReloadRunRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerReloadRunRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerReloadRunRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerReloadRunRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerReloadRunRequest copyWith(
          void Function(ServerReloadRunRequest) updates) =>
      super.copyWith((message) => updates(message as ServerReloadRunRequest))
          as ServerReloadRunRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerReloadRunRequest create() => ServerReloadRunRequest._();
  @$core.override
  ServerReloadRunRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerReloadRunRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerReloadRunRequest>(create);
  static ServerReloadRunRequest? _defaultInstance;
}

class ServerReloadRunResponse extends $pb.GeneratedMessage {
  factory ServerReloadRunResponse({
    PeerRunStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerReloadRunResponse._();

  factory ServerReloadRunResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerReloadRunResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerReloadRunResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerReloadRunResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerReloadRunResponse copyWith(
          void Function(ServerReloadRunResponse) updates) =>
      super.copyWith((message) => updates(message as ServerReloadRunResponse))
          as ServerReloadRunResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerReloadRunResponse create() => ServerReloadRunResponse._();
  @$core.override
  ServerReloadRunResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerReloadRunResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerReloadRunResponse>(create);
  static ServerReloadRunResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunStatus ensureValue() => $_ensure(0);
}

class ServerReloadRunWorkspaceRequest extends $pb.GeneratedMessage {
  factory ServerReloadRunWorkspaceRequest() => create();

  ServerReloadRunWorkspaceRequest._();

  factory ServerReloadRunWorkspaceRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerReloadRunWorkspaceRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerReloadRunWorkspaceRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerReloadRunWorkspaceRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerReloadRunWorkspaceRequest copyWith(
          void Function(ServerReloadRunWorkspaceRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerReloadRunWorkspaceRequest))
          as ServerReloadRunWorkspaceRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerReloadRunWorkspaceRequest create() =>
      ServerReloadRunWorkspaceRequest._();
  @$core.override
  ServerReloadRunWorkspaceRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerReloadRunWorkspaceRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerReloadRunWorkspaceRequest>(
          create);
  static ServerReloadRunWorkspaceRequest? _defaultInstance;
}

class ServerReloadRunWorkspaceResponse extends $pb.GeneratedMessage {
  factory ServerReloadRunWorkspaceResponse({
    PeerRunWorkspaceState? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerReloadRunWorkspaceResponse._();

  factory ServerReloadRunWorkspaceResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerReloadRunWorkspaceResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerReloadRunWorkspaceResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunWorkspaceState>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunWorkspaceState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerReloadRunWorkspaceResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerReloadRunWorkspaceResponse copyWith(
          void Function(ServerReloadRunWorkspaceResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerReloadRunWorkspaceResponse))
          as ServerReloadRunWorkspaceResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerReloadRunWorkspaceResponse create() =>
      ServerReloadRunWorkspaceResponse._();
  @$core.override
  ServerReloadRunWorkspaceResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerReloadRunWorkspaceResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerReloadRunWorkspaceResponse>(
          create);
  static ServerReloadRunWorkspaceResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunWorkspaceState get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunWorkspaceState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunWorkspaceState ensureValue() => $_ensure(0);
}

class ServerRunSayRequest extends $pb.GeneratedMessage {
  factory ServerRunSayRequest({
    $core.String? credentialName,
    $core.String? modelId,
    $core.String? text,
    $core.String? voiceId,
  }) {
    final result = create();
    if (credentialName != null) result.credentialName = credentialName;
    if (modelId != null) result.modelId = modelId;
    if (text != null) result.text = text;
    if (voiceId != null) result.voiceId = voiceId;
    return result;
  }

  ServerRunSayRequest._();

  factory ServerRunSayRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRunSayRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRunSayRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'credentialName')
    ..aOS(2, _omitFieldNames ? '' : 'modelId')
    ..aOS(3, _omitFieldNames ? '' : 'text')
    ..aOS(4, _omitFieldNames ? '' : 'voiceId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRunSayRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRunSayRequest copyWith(void Function(ServerRunSayRequest) updates) =>
      super.copyWith((message) => updates(message as ServerRunSayRequest))
          as ServerRunSayRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRunSayRequest create() => ServerRunSayRequest._();
  @$core.override
  ServerRunSayRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRunSayRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRunSayRequest>(create);
  static ServerRunSayRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get credentialName => $_getSZ(0);
  @$pb.TagNumber(1)
  set credentialName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCredentialName() => $_has(0);
  @$pb.TagNumber(1)
  void clearCredentialName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get modelId => $_getSZ(1);
  @$pb.TagNumber(2)
  set modelId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasModelId() => $_has(1);
  @$pb.TagNumber(2)
  void clearModelId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get text => $_getSZ(2);
  @$pb.TagNumber(3)
  set text($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasText() => $_has(2);
  @$pb.TagNumber(3)
  void clearText() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get voiceId => $_getSZ(3);
  @$pb.TagNumber(4)
  set voiceId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasVoiceId() => $_has(3);
  @$pb.TagNumber(4)
  void clearVoiceId() => $_clearField(4);
}

class ServerRunSayResponse extends $pb.GeneratedMessage {
  factory ServerRunSayResponse({
    $core.bool? accepted,
  }) {
    final result = create();
    if (accepted != null) result.accepted = accepted;
    return result;
  }

  ServerRunSayResponse._();

  factory ServerRunSayResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRunSayResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRunSayResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'accepted')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRunSayResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRunSayResponse copyWith(void Function(ServerRunSayResponse) updates) =>
      super.copyWith((message) => updates(message as ServerRunSayResponse))
          as ServerRunSayResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRunSayResponse create() => ServerRunSayResponse._();
  @$core.override
  ServerRunSayResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRunSayResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRunSayResponse>(create);
  static ServerRunSayResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get accepted => $_getBF(0);
  @$pb.TagNumber(1)
  set accepted($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAccepted() => $_has(0);
  @$pb.TagNumber(1)
  void clearAccepted() => $_clearField(1);
}

class ServerRunWorkspaceRecallRequest extends $pb.GeneratedMessage {
  factory ServerRunWorkspaceRecallRequest({
    PeerRunRecallRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerRunWorkspaceRecallRequest._();

  factory ServerRunWorkspaceRecallRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRunWorkspaceRecallRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRunWorkspaceRecallRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunRecallRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunRecallRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRunWorkspaceRecallRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRunWorkspaceRecallRequest copyWith(
          void Function(ServerRunWorkspaceRecallRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerRunWorkspaceRecallRequest))
          as ServerRunWorkspaceRecallRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRunWorkspaceRecallRequest create() =>
      ServerRunWorkspaceRecallRequest._();
  @$core.override
  ServerRunWorkspaceRecallRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRunWorkspaceRecallRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRunWorkspaceRecallRequest>(
          create);
  static ServerRunWorkspaceRecallRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunRecallRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunRecallRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunRecallRequest ensureValue() => $_ensure(0);
}

class ServerRunWorkspaceRecallResponse extends $pb.GeneratedMessage {
  factory ServerRunWorkspaceRecallResponse({
    PeerRunRecallResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerRunWorkspaceRecallResponse._();

  factory ServerRunWorkspaceRecallResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRunWorkspaceRecallResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRunWorkspaceRecallResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunRecallResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunRecallResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRunWorkspaceRecallResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRunWorkspaceRecallResponse copyWith(
          void Function(ServerRunWorkspaceRecallResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerRunWorkspaceRecallResponse))
          as ServerRunWorkspaceRecallResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRunWorkspaceRecallResponse create() =>
      ServerRunWorkspaceRecallResponse._();
  @$core.override
  ServerRunWorkspaceRecallResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRunWorkspaceRecallResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRunWorkspaceRecallResponse>(
          create);
  static ServerRunWorkspaceRecallResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunRecallResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunRecallResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunRecallResponse ensureValue() => $_ensure(0);
}

class ServerSetRunAgentRequest extends $pb.GeneratedMessage {
  factory ServerSetRunAgentRequest({
    AgentSelection? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerSetRunAgentRequest._();

  factory ServerSetRunAgentRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerSetRunAgentRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerSetRunAgentRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AgentSelection>(1, _omitFieldNames ? '' : 'value',
        subBuilder: AgentSelection.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerSetRunAgentRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerSetRunAgentRequest copyWith(
          void Function(ServerSetRunAgentRequest) updates) =>
      super.copyWith((message) => updates(message as ServerSetRunAgentRequest))
          as ServerSetRunAgentRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerSetRunAgentRequest create() => ServerSetRunAgentRequest._();
  @$core.override
  ServerSetRunAgentRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerSetRunAgentRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerSetRunAgentRequest>(create);
  static ServerSetRunAgentRequest? _defaultInstance;

  @$pb.TagNumber(1)
  AgentSelection get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(AgentSelection value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  AgentSelection ensureValue() => $_ensure(0);
}

class ServerSetRunAgentResponse extends $pb.GeneratedMessage {
  factory ServerSetRunAgentResponse({
    PeerRunAgent? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerSetRunAgentResponse._();

  factory ServerSetRunAgentResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerSetRunAgentResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerSetRunAgentResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunAgent>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunAgent.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerSetRunAgentResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerSetRunAgentResponse copyWith(
          void Function(ServerSetRunAgentResponse) updates) =>
      super.copyWith((message) => updates(message as ServerSetRunAgentResponse))
          as ServerSetRunAgentResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerSetRunAgentResponse create() => ServerSetRunAgentResponse._();
  @$core.override
  ServerSetRunAgentResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerSetRunAgentResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerSetRunAgentResponse>(create);
  static ServerSetRunAgentResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunAgent get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunAgent value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunAgent ensureValue() => $_ensure(0);
}

class ServerSetRunWorkspaceRequest extends $pb.GeneratedMessage {
  factory ServerSetRunWorkspaceRequest({
    AgentSelection? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerSetRunWorkspaceRequest._();

  factory ServerSetRunWorkspaceRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerSetRunWorkspaceRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerSetRunWorkspaceRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AgentSelection>(1, _omitFieldNames ? '' : 'value',
        subBuilder: AgentSelection.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerSetRunWorkspaceRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerSetRunWorkspaceRequest copyWith(
          void Function(ServerSetRunWorkspaceRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerSetRunWorkspaceRequest))
          as ServerSetRunWorkspaceRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerSetRunWorkspaceRequest create() =>
      ServerSetRunWorkspaceRequest._();
  @$core.override
  ServerSetRunWorkspaceRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerSetRunWorkspaceRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerSetRunWorkspaceRequest>(create);
  static ServerSetRunWorkspaceRequest? _defaultInstance;

  @$pb.TagNumber(1)
  AgentSelection get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(AgentSelection value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  AgentSelection ensureValue() => $_ensure(0);
}

class ServerSetRunWorkspaceResponse extends $pb.GeneratedMessage {
  factory ServerSetRunWorkspaceResponse({
    PeerRunWorkspaceState? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerSetRunWorkspaceResponse._();

  factory ServerSetRunWorkspaceResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerSetRunWorkspaceResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerSetRunWorkspaceResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunWorkspaceState>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunWorkspaceState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerSetRunWorkspaceResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerSetRunWorkspaceResponse copyWith(
          void Function(ServerSetRunWorkspaceResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerSetRunWorkspaceResponse))
          as ServerSetRunWorkspaceResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerSetRunWorkspaceResponse create() =>
      ServerSetRunWorkspaceResponse._();
  @$core.override
  ServerSetRunWorkspaceResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerSetRunWorkspaceResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerSetRunWorkspaceResponse>(create);
  static ServerSetRunWorkspaceResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunWorkspaceState get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunWorkspaceState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunWorkspaceState ensureValue() => $_ensure(0);
}

class ServerStopRunRequest extends $pb.GeneratedMessage {
  factory ServerStopRunRequest() => create();

  ServerStopRunRequest._();

  factory ServerStopRunRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerStopRunRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerStopRunRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerStopRunRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerStopRunRequest copyWith(void Function(ServerStopRunRequest) updates) =>
      super.copyWith((message) => updates(message as ServerStopRunRequest))
          as ServerStopRunRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerStopRunRequest create() => ServerStopRunRequest._();
  @$core.override
  ServerStopRunRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerStopRunRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerStopRunRequest>(create);
  static ServerStopRunRequest? _defaultInstance;
}

class ServerStopRunResponse extends $pb.GeneratedMessage {
  factory ServerStopRunResponse({
    PeerRunStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerStopRunResponse._();

  factory ServerStopRunResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerStopRunResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerStopRunResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerStopRunResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerStopRunResponse copyWith(
          void Function(ServerStopRunResponse) updates) =>
      super.copyWith((message) => updates(message as ServerStopRunResponse))
          as ServerStopRunResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerStopRunResponse create() => ServerStopRunResponse._();
  @$core.override
  ServerStopRunResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerStopRunResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerStopRunResponse>(create);
  static ServerStopRunResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunStatus ensureValue() => $_ensure(0);
}

class Workspace extends $pb.GeneratedMessage {
  factory Workspace({
    $core.String? createdAt,
    $core.String? lastActiveAt,
    $core.String? name,
    WorkspaceParameters? parameters,
    $core.String? updatedAt,
    $core.String? workflowName,
    $2.ToolkitPolicy? toolkit,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (lastActiveAt != null) result.lastActiveAt = lastActiveAt;
    if (name != null) result.name = name;
    if (parameters != null) result.parameters = parameters;
    if (updatedAt != null) result.updatedAt = updatedAt;
    if (workflowName != null) result.workflowName = workflowName;
    if (toolkit != null) result.toolkit = toolkit;
    return result;
  }

  Workspace._();

  factory Workspace.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Workspace.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Workspace',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'lastActiveAt')
    ..aOS(3, _omitFieldNames ? '' : 'name')
    ..aOM<WorkspaceParameters>(4, _omitFieldNames ? '' : 'parameters',
        subBuilder: WorkspaceParameters.create)
    ..aOS(5, _omitFieldNames ? '' : 'updatedAt')
    ..aOS(6, _omitFieldNames ? '' : 'workflowName')
    ..aOM<$2.ToolkitPolicy>(7, _omitFieldNames ? '' : 'toolkit',
        subBuilder: $2.ToolkitPolicy.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Workspace clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Workspace copyWith(void Function(Workspace) updates) =>
      super.copyWith((message) => updates(message as Workspace)) as Workspace;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Workspace create() => Workspace._();
  @$core.override
  Workspace createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Workspace getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Workspace>(create);
  static Workspace? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get lastActiveAt => $_getSZ(1);
  @$pb.TagNumber(2)
  set lastActiveAt($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLastActiveAt() => $_has(1);
  @$pb.TagNumber(2)
  void clearLastActiveAt() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get name => $_getSZ(2);
  @$pb.TagNumber(3)
  set name($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasName() => $_has(2);
  @$pb.TagNumber(3)
  void clearName() => $_clearField(3);

  @$pb.TagNumber(4)
  WorkspaceParameters get parameters => $_getN(3);
  @$pb.TagNumber(4)
  set parameters(WorkspaceParameters value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasParameters() => $_has(3);
  @$pb.TagNumber(4)
  void clearParameters() => $_clearField(4);
  @$pb.TagNumber(4)
  WorkspaceParameters ensureParameters() => $_ensure(3);

  @$pb.TagNumber(5)
  $core.String get updatedAt => $_getSZ(4);
  @$pb.TagNumber(5)
  set updatedAt($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasUpdatedAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearUpdatedAt() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get workflowName => $_getSZ(5);
  @$pb.TagNumber(6)
  set workflowName($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasWorkflowName() => $_has(5);
  @$pb.TagNumber(6)
  void clearWorkflowName() => $_clearField(6);

  @$pb.TagNumber(7)
  $2.ToolkitPolicy get toolkit => $_getN(6);
  @$pb.TagNumber(7)
  set toolkit($2.ToolkitPolicy value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasToolkit() => $_has(6);
  @$pb.TagNumber(7)
  void clearToolkit() => $_clearField(7);
  @$pb.TagNumber(7)
  $2.ToolkitPolicy ensureToolkit() => $_ensure(6);
}

class WorkspaceCreateRequest extends $pb.GeneratedMessage {
  factory WorkspaceCreateRequest({
    Workspace? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkspaceCreateRequest._();

  factory WorkspaceCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Workspace>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Workspace.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceCreateRequest copyWith(
          void Function(WorkspaceCreateRequest) updates) =>
      super.copyWith((message) => updates(message as WorkspaceCreateRequest))
          as WorkspaceCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceCreateRequest create() => WorkspaceCreateRequest._();
  @$core.override
  WorkspaceCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceCreateRequest>(create);
  static WorkspaceCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  Workspace get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Workspace value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Workspace ensureValue() => $_ensure(0);
}

class WorkspaceCreateResponse extends $pb.GeneratedMessage {
  factory WorkspaceCreateResponse({
    Workspace? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkspaceCreateResponse._();

  factory WorkspaceCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Workspace>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Workspace.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceCreateResponse copyWith(
          void Function(WorkspaceCreateResponse) updates) =>
      super.copyWith((message) => updates(message as WorkspaceCreateResponse))
          as WorkspaceCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceCreateResponse create() => WorkspaceCreateResponse._();
  @$core.override
  WorkspaceCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceCreateResponse>(create);
  static WorkspaceCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Workspace get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Workspace value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Workspace ensureValue() => $_ensure(0);
}

class WorkspaceDeleteRequest extends $pb.GeneratedMessage {
  factory WorkspaceDeleteRequest({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  WorkspaceDeleteRequest._();

  factory WorkspaceDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceDeleteRequest copyWith(
          void Function(WorkspaceDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as WorkspaceDeleteRequest))
          as WorkspaceDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceDeleteRequest create() => WorkspaceDeleteRequest._();
  @$core.override
  WorkspaceDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceDeleteRequest>(create);
  static WorkspaceDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);
}

class WorkspaceDeleteResponse extends $pb.GeneratedMessage {
  factory WorkspaceDeleteResponse({
    Workspace? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkspaceDeleteResponse._();

  factory WorkspaceDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Workspace>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Workspace.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceDeleteResponse copyWith(
          void Function(WorkspaceDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as WorkspaceDeleteResponse))
          as WorkspaceDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceDeleteResponse create() => WorkspaceDeleteResponse._();
  @$core.override
  WorkspaceDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceDeleteResponse>(create);
  static WorkspaceDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Workspace get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Workspace value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Workspace ensureValue() => $_ensure(0);
}

class WorkspaceGetRequest extends $pb.GeneratedMessage {
  factory WorkspaceGetRequest({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  WorkspaceGetRequest._();

  factory WorkspaceGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceGetRequest copyWith(void Function(WorkspaceGetRequest) updates) =>
      super.copyWith((message) => updates(message as WorkspaceGetRequest))
          as WorkspaceGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceGetRequest create() => WorkspaceGetRequest._();
  @$core.override
  WorkspaceGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceGetRequest>(create);
  static WorkspaceGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);
}

class WorkspaceGetResponse extends $pb.GeneratedMessage {
  factory WorkspaceGetResponse({
    Workspace? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkspaceGetResponse._();

  factory WorkspaceGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Workspace>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Workspace.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceGetResponse copyWith(void Function(WorkspaceGetResponse) updates) =>
      super.copyWith((message) => updates(message as WorkspaceGetResponse))
          as WorkspaceGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceGetResponse create() => WorkspaceGetResponse._();
  @$core.override
  WorkspaceGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceGetResponse>(create);
  static WorkspaceGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Workspace get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Workspace value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Workspace ensureValue() => $_ensure(0);
}

class WorkspaceHistoryAudioGetRequest extends $pb.GeneratedMessage {
  factory WorkspaceHistoryAudioGetRequest({
    $core.String? historyId,
    $core.String? workspaceName,
  }) {
    final result = create();
    if (historyId != null) result.historyId = historyId;
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  WorkspaceHistoryAudioGetRequest._();

  factory WorkspaceHistoryAudioGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceHistoryAudioGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceHistoryAudioGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'historyId')
    ..aOS(2, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryAudioGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryAudioGetRequest copyWith(
          void Function(WorkspaceHistoryAudioGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as WorkspaceHistoryAudioGetRequest))
          as WorkspaceHistoryAudioGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryAudioGetRequest create() =>
      WorkspaceHistoryAudioGetRequest._();
  @$core.override
  WorkspaceHistoryAudioGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryAudioGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceHistoryAudioGetRequest>(
          create);
  static WorkspaceHistoryAudioGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get historyId => $_getSZ(0);
  @$pb.TagNumber(1)
  set historyId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHistoryId() => $_has(0);
  @$pb.TagNumber(1)
  void clearHistoryId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get workspaceName => $_getSZ(1);
  @$pb.TagNumber(2)
  set workspaceName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasWorkspaceName() => $_has(1);
  @$pb.TagNumber(2)
  void clearWorkspaceName() => $_clearField(2);
}

class WorkspaceHistoryAudioGetResponse extends $pb.GeneratedMessage {
  factory WorkspaceHistoryAudioGetResponse({
    $core.String? historyId,
    $core.String? mimeType,
    $fixnum.Int64? sizeBytes,
    $core.String? workspaceName,
  }) {
    final result = create();
    if (historyId != null) result.historyId = historyId;
    if (mimeType != null) result.mimeType = mimeType;
    if (sizeBytes != null) result.sizeBytes = sizeBytes;
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  WorkspaceHistoryAudioGetResponse._();

  factory WorkspaceHistoryAudioGetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceHistoryAudioGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceHistoryAudioGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'historyId')
    ..aOS(2, _omitFieldNames ? '' : 'mimeType')
    ..aInt64(3, _omitFieldNames ? '' : 'sizeBytes')
    ..aOS(4, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryAudioGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryAudioGetResponse copyWith(
          void Function(WorkspaceHistoryAudioGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as WorkspaceHistoryAudioGetResponse))
          as WorkspaceHistoryAudioGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryAudioGetResponse create() =>
      WorkspaceHistoryAudioGetResponse._();
  @$core.override
  WorkspaceHistoryAudioGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryAudioGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceHistoryAudioGetResponse>(
          create);
  static WorkspaceHistoryAudioGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get historyId => $_getSZ(0);
  @$pb.TagNumber(1)
  set historyId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHistoryId() => $_has(0);
  @$pb.TagNumber(1)
  void clearHistoryId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get mimeType => $_getSZ(1);
  @$pb.TagNumber(2)
  set mimeType($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMimeType() => $_has(1);
  @$pb.TagNumber(2)
  void clearMimeType() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get sizeBytes => $_getI64(2);
  @$pb.TagNumber(3)
  set sizeBytes($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSizeBytes() => $_has(2);
  @$pb.TagNumber(3)
  void clearSizeBytes() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get workspaceName => $_getSZ(3);
  @$pb.TagNumber(4)
  set workspaceName($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasWorkspaceName() => $_has(3);
  @$pb.TagNumber(4)
  void clearWorkspaceName() => $_clearField(4);
}

class WorkspaceHistoryGetRequest extends $pb.GeneratedMessage {
  factory WorkspaceHistoryGetRequest({
    $core.String? historyId,
    $core.String? workspaceName,
  }) {
    final result = create();
    if (historyId != null) result.historyId = historyId;
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  WorkspaceHistoryGetRequest._();

  factory WorkspaceHistoryGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceHistoryGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceHistoryGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'historyId')
    ..aOS(2, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryGetRequest copyWith(
          void Function(WorkspaceHistoryGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as WorkspaceHistoryGetRequest))
          as WorkspaceHistoryGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryGetRequest create() => WorkspaceHistoryGetRequest._();
  @$core.override
  WorkspaceHistoryGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceHistoryGetRequest>(create);
  static WorkspaceHistoryGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get historyId => $_getSZ(0);
  @$pb.TagNumber(1)
  set historyId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHistoryId() => $_has(0);
  @$pb.TagNumber(1)
  void clearHistoryId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get workspaceName => $_getSZ(1);
  @$pb.TagNumber(2)
  set workspaceName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasWorkspaceName() => $_has(1);
  @$pb.TagNumber(2)
  void clearWorkspaceName() => $_clearField(2);
}

class WorkspaceHistoryGetResponse extends $pb.GeneratedMessage {
  factory WorkspaceHistoryGetResponse({
    PeerRunHistoryEntry? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkspaceHistoryGetResponse._();

  factory WorkspaceHistoryGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceHistoryGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceHistoryGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunHistoryEntry>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunHistoryEntry.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryGetResponse copyWith(
          void Function(WorkspaceHistoryGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as WorkspaceHistoryGetResponse))
          as WorkspaceHistoryGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryGetResponse create() =>
      WorkspaceHistoryGetResponse._();
  @$core.override
  WorkspaceHistoryGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceHistoryGetResponse>(create);
  static WorkspaceHistoryGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunHistoryEntry get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunHistoryEntry value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunHistoryEntry ensureValue() => $_ensure(0);
}

class WorkspaceHistoryListRequest extends $pb.GeneratedMessage {
  factory WorkspaceHistoryListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
    $3.WorkspaceHistoryListRequestOrder? order,
    $core.String? workspaceName,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    if (order != null) result.order = order;
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  WorkspaceHistoryListRequest._();

  factory WorkspaceHistoryListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceHistoryListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceHistoryListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..aE<$3.WorkspaceHistoryListRequestOrder>(3, _omitFieldNames ? '' : 'order',
        enumValues: $3.WorkspaceHistoryListRequestOrder.values)
    ..aOS(4, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryListRequest copyWith(
          void Function(WorkspaceHistoryListRequest) updates) =>
      super.copyWith(
              (message) => updates(message as WorkspaceHistoryListRequest))
          as WorkspaceHistoryListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryListRequest create() =>
      WorkspaceHistoryListRequest._();
  @$core.override
  WorkspaceHistoryListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceHistoryListRequest>(create);
  static WorkspaceHistoryListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);

  @$pb.TagNumber(3)
  $3.WorkspaceHistoryListRequestOrder get order => $_getN(2);
  @$pb.TagNumber(3)
  set order($3.WorkspaceHistoryListRequestOrder value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasOrder() => $_has(2);
  @$pb.TagNumber(3)
  void clearOrder() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get workspaceName => $_getSZ(3);
  @$pb.TagNumber(4)
  set workspaceName($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasWorkspaceName() => $_has(3);
  @$pb.TagNumber(4)
  void clearWorkspaceName() => $_clearField(4);
}

class WorkspaceHistoryListResponse extends $pb.GeneratedMessage {
  factory WorkspaceHistoryListResponse({
    PeerRunHistoryListResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkspaceHistoryListResponse._();

  factory WorkspaceHistoryListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceHistoryListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceHistoryListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerRunHistoryListResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerRunHistoryListResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryListResponse copyWith(
          void Function(WorkspaceHistoryListResponse) updates) =>
      super.copyWith(
              (message) => updates(message as WorkspaceHistoryListResponse))
          as WorkspaceHistoryListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryListResponse create() =>
      WorkspaceHistoryListResponse._();
  @$core.override
  WorkspaceHistoryListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceHistoryListResponse>(create);
  static WorkspaceHistoryListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerRunHistoryListResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerRunHistoryListResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerRunHistoryListResponse ensureValue() => $_ensure(0);
}

class WorkspaceListRequest extends $pb.GeneratedMessage {
  factory WorkspaceListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
    $core.String? prefix,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    if (prefix != null) result.prefix = prefix;
    return result;
  }

  WorkspaceListRequest._();

  factory WorkspaceListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..aOS(3, _omitFieldNames ? '' : 'prefix')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceListRequest copyWith(void Function(WorkspaceListRequest) updates) =>
      super.copyWith((message) => updates(message as WorkspaceListRequest))
          as WorkspaceListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceListRequest create() => WorkspaceListRequest._();
  @$core.override
  WorkspaceListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceListRequest>(create);
  static WorkspaceListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get prefix => $_getSZ(2);
  @$pb.TagNumber(3)
  set prefix($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPrefix() => $_has(2);
  @$pb.TagNumber(3)
  void clearPrefix() => $_clearField(3);
}

class WorkspaceListResponse extends $pb.GeneratedMessage {
  factory WorkspaceListResponse({
    $core.bool? hasNext,
    $core.Iterable<Workspace>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  WorkspaceListResponse._();

  factory WorkspaceListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<Workspace>(2, _omitFieldNames ? '' : 'items',
        subBuilder: Workspace.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceListResponse copyWith(
          void Function(WorkspaceListResponse) updates) =>
      super.copyWith((message) => updates(message as WorkspaceListResponse))
          as WorkspaceListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceListResponse create() => WorkspaceListResponse._();
  @$core.override
  WorkspaceListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceListResponse>(create);
  static WorkspaceListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<Workspace> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

enum WorkspaceParameters_Value {
  flowcraftWorkspaceParameters,
  doubaoRealtimeWorkspaceParameters,
  asttranslateWorkspaceParameters,
  chatRoomWorkspaceParameters,
  petWorkspaceParameters,
  notSet
}

class WorkspaceParameters extends $pb.GeneratedMessage {
  factory WorkspaceParameters({
    $2.FlowcraftWorkspaceParameters? flowcraftWorkspaceParameters,
    $2.DoubaoRealtimeWorkspaceParameters? doubaoRealtimeWorkspaceParameters,
    $2.ASTTranslateWorkspaceParameters? asttranslateWorkspaceParameters,
    $2.ChatRoomWorkspaceParameters? chatRoomWorkspaceParameters,
    $2.PetWorkspaceParameters? petWorkspaceParameters,
  }) {
    final result = create();
    if (flowcraftWorkspaceParameters != null)
      result.flowcraftWorkspaceParameters = flowcraftWorkspaceParameters;
    if (doubaoRealtimeWorkspaceParameters != null)
      result.doubaoRealtimeWorkspaceParameters =
          doubaoRealtimeWorkspaceParameters;
    if (asttranslateWorkspaceParameters != null)
      result.asttranslateWorkspaceParameters = asttranslateWorkspaceParameters;
    if (chatRoomWorkspaceParameters != null)
      result.chatRoomWorkspaceParameters = chatRoomWorkspaceParameters;
    if (petWorkspaceParameters != null)
      result.petWorkspaceParameters = petWorkspaceParameters;
    return result;
  }

  WorkspaceParameters._();

  factory WorkspaceParameters.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, WorkspaceParameters_Value>
      _WorkspaceParameters_ValueByTag = {
    1: WorkspaceParameters_Value.flowcraftWorkspaceParameters,
    2: WorkspaceParameters_Value.doubaoRealtimeWorkspaceParameters,
    3: WorkspaceParameters_Value.asttranslateWorkspaceParameters,
    4: WorkspaceParameters_Value.chatRoomWorkspaceParameters,
    5: WorkspaceParameters_Value.petWorkspaceParameters,
    0: WorkspaceParameters_Value.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2, 3, 4, 5])
    ..aOM<$2.FlowcraftWorkspaceParameters>(
        1, _omitFieldNames ? '' : 'flowcraftWorkspaceParameters',
        subBuilder: $2.FlowcraftWorkspaceParameters.create)
    ..aOM<$2.DoubaoRealtimeWorkspaceParameters>(
        2, _omitFieldNames ? '' : 'doubaoRealtimeWorkspaceParameters',
        subBuilder: $2.DoubaoRealtimeWorkspaceParameters.create)
    ..aOM<$2.ASTTranslateWorkspaceParameters>(
        3, _omitFieldNames ? '' : 'asttranslateWorkspaceParameters',
        subBuilder: $2.ASTTranslateWorkspaceParameters.create)
    ..aOM<$2.ChatRoomWorkspaceParameters>(
        4, _omitFieldNames ? '' : 'chatRoomWorkspaceParameters',
        subBuilder: $2.ChatRoomWorkspaceParameters.create)
    ..aOM<$2.PetWorkspaceParameters>(
        5, _omitFieldNames ? '' : 'petWorkspaceParameters',
        subBuilder: $2.PetWorkspaceParameters.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceParameters copyWith(void Function(WorkspaceParameters) updates) =>
      super.copyWith((message) => updates(message as WorkspaceParameters))
          as WorkspaceParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceParameters create() => WorkspaceParameters._();
  @$core.override
  WorkspaceParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceParameters getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceParameters>(create);
  static WorkspaceParameters? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(5)
  WorkspaceParameters_Value whichValue() =>
      _WorkspaceParameters_ValueByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(5)
  void clearValue() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  $2.FlowcraftWorkspaceParameters get flowcraftWorkspaceParameters => $_getN(0);
  @$pb.TagNumber(1)
  set flowcraftWorkspaceParameters($2.FlowcraftWorkspaceParameters value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasFlowcraftWorkspaceParameters() => $_has(0);
  @$pb.TagNumber(1)
  void clearFlowcraftWorkspaceParameters() => $_clearField(1);
  @$pb.TagNumber(1)
  $2.FlowcraftWorkspaceParameters ensureFlowcraftWorkspaceParameters() =>
      $_ensure(0);

  @$pb.TagNumber(2)
  $2.DoubaoRealtimeWorkspaceParameters get doubaoRealtimeWorkspaceParameters =>
      $_getN(1);
  @$pb.TagNumber(2)
  set doubaoRealtimeWorkspaceParameters(
          $2.DoubaoRealtimeWorkspaceParameters value) =>
      $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasDoubaoRealtimeWorkspaceParameters() => $_has(1);
  @$pb.TagNumber(2)
  void clearDoubaoRealtimeWorkspaceParameters() => $_clearField(2);
  @$pb.TagNumber(2)
  $2.DoubaoRealtimeWorkspaceParameters
      ensureDoubaoRealtimeWorkspaceParameters() => $_ensure(1);

  @$pb.TagNumber(3)
  $2.ASTTranslateWorkspaceParameters get asttranslateWorkspaceParameters =>
      $_getN(2);
  @$pb.TagNumber(3)
  set asttranslateWorkspaceParameters(
          $2.ASTTranslateWorkspaceParameters value) =>
      $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasAsttranslateWorkspaceParameters() => $_has(2);
  @$pb.TagNumber(3)
  void clearAsttranslateWorkspaceParameters() => $_clearField(3);
  @$pb.TagNumber(3)
  $2.ASTTranslateWorkspaceParameters ensureAsttranslateWorkspaceParameters() =>
      $_ensure(2);

  @$pb.TagNumber(4)
  $2.ChatRoomWorkspaceParameters get chatRoomWorkspaceParameters => $_getN(3);
  @$pb.TagNumber(4)
  set chatRoomWorkspaceParameters($2.ChatRoomWorkspaceParameters value) =>
      $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasChatRoomWorkspaceParameters() => $_has(3);
  @$pb.TagNumber(4)
  void clearChatRoomWorkspaceParameters() => $_clearField(4);
  @$pb.TagNumber(4)
  $2.ChatRoomWorkspaceParameters ensureChatRoomWorkspaceParameters() =>
      $_ensure(3);

  @$pb.TagNumber(5)
  $2.PetWorkspaceParameters get petWorkspaceParameters => $_getN(4);
  @$pb.TagNumber(5)
  set petWorkspaceParameters($2.PetWorkspaceParameters value) =>
      $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasPetWorkspaceParameters() => $_has(4);
  @$pb.TagNumber(5)
  void clearPetWorkspaceParameters() => $_clearField(5);
  @$pb.TagNumber(5)
  $2.PetWorkspaceParameters ensurePetWorkspaceParameters() => $_ensure(4);
}

class WorkspacePutRequest extends $pb.GeneratedMessage {
  factory WorkspacePutRequest({
    Workspace? body,
    $core.String? name,
  }) {
    final result = create();
    if (body != null) result.body = body;
    if (name != null) result.name = name;
    return result;
  }

  WorkspacePutRequest._();

  factory WorkspacePutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspacePutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspacePutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Workspace>(1, _omitFieldNames ? '' : 'body',
        subBuilder: Workspace.create)
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspacePutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspacePutRequest copyWith(void Function(WorkspacePutRequest) updates) =>
      super.copyWith((message) => updates(message as WorkspacePutRequest))
          as WorkspacePutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspacePutRequest create() => WorkspacePutRequest._();
  @$core.override
  WorkspacePutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspacePutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspacePutRequest>(create);
  static WorkspacePutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  Workspace get body => $_getN(0);
  @$pb.TagNumber(1)
  set body(Workspace value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasBody() => $_has(0);
  @$pb.TagNumber(1)
  void clearBody() => $_clearField(1);
  @$pb.TagNumber(1)
  Workspace ensureBody() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);
}

class WorkspacePutResponse extends $pb.GeneratedMessage {
  factory WorkspacePutResponse({
    Workspace? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkspacePutResponse._();

  factory WorkspacePutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspacePutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspacePutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Workspace>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Workspace.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspacePutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspacePutResponse copyWith(void Function(WorkspacePutResponse) updates) =>
      super.copyWith((message) => updates(message as WorkspacePutResponse))
          as WorkspacePutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspacePutResponse create() => WorkspacePutResponse._();
  @$core.override
  WorkspacePutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspacePutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspacePutResponse>(create);
  static WorkspacePutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Workspace get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Workspace value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Workspace ensureValue() => $_ensure(0);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
