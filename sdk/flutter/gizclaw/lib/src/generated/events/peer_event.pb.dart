// This is a generated file - do not edit.
//
// Generated from peer_event.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;

import 'peer_event.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'peer_event.pbenum.dart';

enum PeerEvent_Payload {
  bos,
  eos,
  textDelta,
  textDone,
  workspaceHistoryUpdated,
  friendRelationshipUpdated,
  friendGroupUpdated,
  notSet
}

class PeerEvent extends $pb.GeneratedMessage {
  factory PeerEvent({
    $core.int? version,
    PeerEventType? type,
    StreamBegin? bos,
    StreamEnd? eos,
    TextDelta? textDelta,
    TextDone? textDone,
    WorkspaceHistoryUpdated? workspaceHistoryUpdated,
    FriendRelationshipUpdated? friendRelationshipUpdated,
    FriendGroupUpdated? friendGroupUpdated,
  }) {
    final result = create();
    if (version != null) result.version = version;
    if (type != null) result.type = type;
    if (bos != null) result.bos = bos;
    if (eos != null) result.eos = eos;
    if (textDelta != null) result.textDelta = textDelta;
    if (textDone != null) result.textDone = textDone;
    if (workspaceHistoryUpdated != null)
      result.workspaceHistoryUpdated = workspaceHistoryUpdated;
    if (friendRelationshipUpdated != null)
      result.friendRelationshipUpdated = friendRelationshipUpdated;
    if (friendGroupUpdated != null)
      result.friendGroupUpdated = friendGroupUpdated;
    return result;
  }

  PeerEvent._();

  factory PeerEvent.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerEvent.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, PeerEvent_Payload> _PeerEvent_PayloadByTag =
      {
    10: PeerEvent_Payload.bos,
    11: PeerEvent_Payload.eos,
    12: PeerEvent_Payload.textDelta,
    13: PeerEvent_Payload.textDone,
    14: PeerEvent_Payload.workspaceHistoryUpdated,
    15: PeerEvent_Payload.friendRelationshipUpdated,
    16: PeerEvent_Payload.friendGroupUpdated,
    0: PeerEvent_Payload.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerEvent',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..oo(0, [10, 11, 12, 13, 14, 15, 16])
    ..aI(1, _omitFieldNames ? '' : 'version', fieldType: $pb.PbFieldType.OU3)
    ..aE<PeerEventType>(2, _omitFieldNames ? '' : 'type',
        enumValues: PeerEventType.values)
    ..aOM<StreamBegin>(10, _omitFieldNames ? '' : 'bos',
        subBuilder: StreamBegin.create)
    ..aOM<StreamEnd>(11, _omitFieldNames ? '' : 'eos',
        subBuilder: StreamEnd.create)
    ..aOM<TextDelta>(12, _omitFieldNames ? '' : 'textDelta',
        subBuilder: TextDelta.create)
    ..aOM<TextDone>(13, _omitFieldNames ? '' : 'textDone',
        subBuilder: TextDone.create)
    ..aOM<WorkspaceHistoryUpdated>(
        14, _omitFieldNames ? '' : 'workspaceHistoryUpdated',
        subBuilder: WorkspaceHistoryUpdated.create)
    ..aOM<FriendRelationshipUpdated>(
        15, _omitFieldNames ? '' : 'friendRelationshipUpdated',
        subBuilder: FriendRelationshipUpdated.create)
    ..aOM<FriendGroupUpdated>(16, _omitFieldNames ? '' : 'friendGroupUpdated',
        subBuilder: FriendGroupUpdated.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerEvent clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerEvent copyWith(void Function(PeerEvent) updates) =>
      super.copyWith((message) => updates(message as PeerEvent)) as PeerEvent;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerEvent create() => PeerEvent._();
  @$core.override
  PeerEvent createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerEvent getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PeerEvent>(create);
  static PeerEvent? _defaultInstance;

  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  @$pb.TagNumber(16)
  PeerEvent_Payload whichPayload() => _PeerEvent_PayloadByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  @$pb.TagNumber(16)
  void clearPayload() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  $core.int get version => $_getIZ(0);
  @$pb.TagNumber(1)
  set version($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasVersion() => $_has(0);
  @$pb.TagNumber(1)
  void clearVersion() => $_clearField(1);

  @$pb.TagNumber(2)
  PeerEventType get type => $_getN(1);
  @$pb.TagNumber(2)
  set type(PeerEventType value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasType() => $_has(1);
  @$pb.TagNumber(2)
  void clearType() => $_clearField(2);

  @$pb.TagNumber(10)
  StreamBegin get bos => $_getN(2);
  @$pb.TagNumber(10)
  set bos(StreamBegin value) => $_setField(10, value);
  @$pb.TagNumber(10)
  $core.bool hasBos() => $_has(2);
  @$pb.TagNumber(10)
  void clearBos() => $_clearField(10);
  @$pb.TagNumber(10)
  StreamBegin ensureBos() => $_ensure(2);

  @$pb.TagNumber(11)
  StreamEnd get eos => $_getN(3);
  @$pb.TagNumber(11)
  set eos(StreamEnd value) => $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasEos() => $_has(3);
  @$pb.TagNumber(11)
  void clearEos() => $_clearField(11);
  @$pb.TagNumber(11)
  StreamEnd ensureEos() => $_ensure(3);

  @$pb.TagNumber(12)
  TextDelta get textDelta => $_getN(4);
  @$pb.TagNumber(12)
  set textDelta(TextDelta value) => $_setField(12, value);
  @$pb.TagNumber(12)
  $core.bool hasTextDelta() => $_has(4);
  @$pb.TagNumber(12)
  void clearTextDelta() => $_clearField(12);
  @$pb.TagNumber(12)
  TextDelta ensureTextDelta() => $_ensure(4);

  @$pb.TagNumber(13)
  TextDone get textDone => $_getN(5);
  @$pb.TagNumber(13)
  set textDone(TextDone value) => $_setField(13, value);
  @$pb.TagNumber(13)
  $core.bool hasTextDone() => $_has(5);
  @$pb.TagNumber(13)
  void clearTextDone() => $_clearField(13);
  @$pb.TagNumber(13)
  TextDone ensureTextDone() => $_ensure(5);

  @$pb.TagNumber(14)
  WorkspaceHistoryUpdated get workspaceHistoryUpdated => $_getN(6);
  @$pb.TagNumber(14)
  set workspaceHistoryUpdated(WorkspaceHistoryUpdated value) =>
      $_setField(14, value);
  @$pb.TagNumber(14)
  $core.bool hasWorkspaceHistoryUpdated() => $_has(6);
  @$pb.TagNumber(14)
  void clearWorkspaceHistoryUpdated() => $_clearField(14);
  @$pb.TagNumber(14)
  WorkspaceHistoryUpdated ensureWorkspaceHistoryUpdated() => $_ensure(6);

  @$pb.TagNumber(15)
  FriendRelationshipUpdated get friendRelationshipUpdated => $_getN(7);
  @$pb.TagNumber(15)
  set friendRelationshipUpdated(FriendRelationshipUpdated value) =>
      $_setField(15, value);
  @$pb.TagNumber(15)
  $core.bool hasFriendRelationshipUpdated() => $_has(7);
  @$pb.TagNumber(15)
  void clearFriendRelationshipUpdated() => $_clearField(15);
  @$pb.TagNumber(15)
  FriendRelationshipUpdated ensureFriendRelationshipUpdated() => $_ensure(7);

  @$pb.TagNumber(16)
  FriendGroupUpdated get friendGroupUpdated => $_getN(8);
  @$pb.TagNumber(16)
  set friendGroupUpdated(FriendGroupUpdated value) => $_setField(16, value);
  @$pb.TagNumber(16)
  $core.bool hasFriendGroupUpdated() => $_has(8);
  @$pb.TagNumber(16)
  void clearFriendGroupUpdated() => $_clearField(16);
  @$pb.TagNumber(16)
  FriendGroupUpdated ensureFriendGroupUpdated() => $_ensure(8);
}

class StreamBegin extends $pb.GeneratedMessage {
  factory StreamBegin({
    $core.String? streamId,
    $fixnum.Int64? sequence,
    $fixnum.Int64? timestampUnixMs,
    StreamKind? kind,
    $core.String? label,
    $core.String? mimeType,
  }) {
    final result = create();
    if (streamId != null) result.streamId = streamId;
    if (sequence != null) result.sequence = sequence;
    if (timestampUnixMs != null) result.timestampUnixMs = timestampUnixMs;
    if (kind != null) result.kind = kind;
    if (label != null) result.label = label;
    if (mimeType != null) result.mimeType = mimeType;
    return result;
  }

  StreamBegin._();

  factory StreamBegin.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory StreamBegin.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StreamBegin',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'streamId')
    ..a<$fixnum.Int64>(
        2, _omitFieldNames ? '' : 'sequence', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aInt64(3, _omitFieldNames ? '' : 'timestampUnixMs')
    ..aE<StreamKind>(4, _omitFieldNames ? '' : 'kind',
        enumValues: StreamKind.values)
    ..aOS(5, _omitFieldNames ? '' : 'label')
    ..aOS(6, _omitFieldNames ? '' : 'mimeType')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamBegin clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamBegin copyWith(void Function(StreamBegin) updates) =>
      super.copyWith((message) => updates(message as StreamBegin))
          as StreamBegin;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static StreamBegin create() => StreamBegin._();
  @$core.override
  StreamBegin createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static StreamBegin getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StreamBegin>(create);
  static StreamBegin? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get streamId => $_getSZ(0);
  @$pb.TagNumber(1)
  set streamId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasStreamId() => $_has(0);
  @$pb.TagNumber(1)
  void clearStreamId() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get sequence => $_getI64(1);
  @$pb.TagNumber(2)
  set sequence($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSequence() => $_has(1);
  @$pb.TagNumber(2)
  void clearSequence() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get timestampUnixMs => $_getI64(2);
  @$pb.TagNumber(3)
  set timestampUnixMs($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasTimestampUnixMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearTimestampUnixMs() => $_clearField(3);

  @$pb.TagNumber(4)
  StreamKind get kind => $_getN(3);
  @$pb.TagNumber(4)
  set kind(StreamKind value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasKind() => $_has(3);
  @$pb.TagNumber(4)
  void clearKind() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get label => $_getSZ(4);
  @$pb.TagNumber(5)
  set label($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasLabel() => $_has(4);
  @$pb.TagNumber(5)
  void clearLabel() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get mimeType => $_getSZ(5);
  @$pb.TagNumber(6)
  set mimeType($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasMimeType() => $_has(5);
  @$pb.TagNumber(6)
  void clearMimeType() => $_clearField(6);
}

class StreamEnd extends $pb.GeneratedMessage {
  factory StreamEnd({
    $core.String? streamId,
    $fixnum.Int64? sequence,
    $fixnum.Int64? timestampUnixMs,
    StreamKind? kind,
    $core.String? label,
    $core.String? mimeType,
    EventError? error,
  }) {
    final result = create();
    if (streamId != null) result.streamId = streamId;
    if (sequence != null) result.sequence = sequence;
    if (timestampUnixMs != null) result.timestampUnixMs = timestampUnixMs;
    if (kind != null) result.kind = kind;
    if (label != null) result.label = label;
    if (mimeType != null) result.mimeType = mimeType;
    if (error != null) result.error = error;
    return result;
  }

  StreamEnd._();

  factory StreamEnd.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory StreamEnd.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StreamEnd',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'streamId')
    ..a<$fixnum.Int64>(
        2, _omitFieldNames ? '' : 'sequence', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aInt64(3, _omitFieldNames ? '' : 'timestampUnixMs')
    ..aE<StreamKind>(4, _omitFieldNames ? '' : 'kind',
        enumValues: StreamKind.values)
    ..aOS(5, _omitFieldNames ? '' : 'label')
    ..aOS(6, _omitFieldNames ? '' : 'mimeType')
    ..aOM<EventError>(7, _omitFieldNames ? '' : 'error',
        subBuilder: EventError.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamEnd clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamEnd copyWith(void Function(StreamEnd) updates) =>
      super.copyWith((message) => updates(message as StreamEnd)) as StreamEnd;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static StreamEnd create() => StreamEnd._();
  @$core.override
  StreamEnd createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static StreamEnd getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<StreamEnd>(create);
  static StreamEnd? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get streamId => $_getSZ(0);
  @$pb.TagNumber(1)
  set streamId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasStreamId() => $_has(0);
  @$pb.TagNumber(1)
  void clearStreamId() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get sequence => $_getI64(1);
  @$pb.TagNumber(2)
  set sequence($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSequence() => $_has(1);
  @$pb.TagNumber(2)
  void clearSequence() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get timestampUnixMs => $_getI64(2);
  @$pb.TagNumber(3)
  set timestampUnixMs($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasTimestampUnixMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearTimestampUnixMs() => $_clearField(3);

  @$pb.TagNumber(4)
  StreamKind get kind => $_getN(3);
  @$pb.TagNumber(4)
  set kind(StreamKind value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasKind() => $_has(3);
  @$pb.TagNumber(4)
  void clearKind() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get label => $_getSZ(4);
  @$pb.TagNumber(5)
  set label($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasLabel() => $_has(4);
  @$pb.TagNumber(5)
  void clearLabel() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get mimeType => $_getSZ(5);
  @$pb.TagNumber(6)
  set mimeType($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasMimeType() => $_has(5);
  @$pb.TagNumber(6)
  void clearMimeType() => $_clearField(6);

  @$pb.TagNumber(7)
  EventError get error => $_getN(6);
  @$pb.TagNumber(7)
  set error(EventError value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasError() => $_has(6);
  @$pb.TagNumber(7)
  void clearError() => $_clearField(7);
  @$pb.TagNumber(7)
  EventError ensureError() => $_ensure(6);
}

class EventError extends $pb.GeneratedMessage {
  factory EventError({
    $core.String? code,
    $core.String? message,
    $core.bool? retryable,
  }) {
    final result = create();
    if (code != null) result.code = code;
    if (message != null) result.message = message;
    if (retryable != null) result.retryable = retryable;
    return result;
  }

  EventError._();

  factory EventError.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory EventError.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'EventError',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'code')
    ..aOS(2, _omitFieldNames ? '' : 'message')
    ..aOB(3, _omitFieldNames ? '' : 'retryable')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  EventError clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  EventError copyWith(void Function(EventError) updates) =>
      super.copyWith((message) => updates(message as EventError)) as EventError;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static EventError create() => EventError._();
  @$core.override
  EventError createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static EventError getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<EventError>(create);
  static EventError? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get code => $_getSZ(0);
  @$pb.TagNumber(1)
  set code($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCode() => $_has(0);
  @$pb.TagNumber(1)
  void clearCode() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get message => $_getSZ(1);
  @$pb.TagNumber(2)
  set message($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMessage() => $_has(1);
  @$pb.TagNumber(2)
  void clearMessage() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get retryable => $_getBF(2);
  @$pb.TagNumber(3)
  set retryable($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRetryable() => $_has(2);
  @$pb.TagNumber(3)
  void clearRetryable() => $_clearField(3);
}

class TextDelta extends $pb.GeneratedMessage {
  factory TextDelta({
    $core.String? streamId,
    $fixnum.Int64? sequence,
    $fixnum.Int64? timestampUnixMs,
    $core.String? label,
    $core.String? text,
  }) {
    final result = create();
    if (streamId != null) result.streamId = streamId;
    if (sequence != null) result.sequence = sequence;
    if (timestampUnixMs != null) result.timestampUnixMs = timestampUnixMs;
    if (label != null) result.label = label;
    if (text != null) result.text = text;
    return result;
  }

  TextDelta._();

  factory TextDelta.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory TextDelta.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'TextDelta',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'streamId')
    ..a<$fixnum.Int64>(
        2, _omitFieldNames ? '' : 'sequence', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aInt64(3, _omitFieldNames ? '' : 'timestampUnixMs')
    ..aOS(4, _omitFieldNames ? '' : 'label')
    ..aOS(5, _omitFieldNames ? '' : 'text')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TextDelta clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TextDelta copyWith(void Function(TextDelta) updates) =>
      super.copyWith((message) => updates(message as TextDelta)) as TextDelta;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static TextDelta create() => TextDelta._();
  @$core.override
  TextDelta createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static TextDelta getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<TextDelta>(create);
  static TextDelta? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get streamId => $_getSZ(0);
  @$pb.TagNumber(1)
  set streamId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasStreamId() => $_has(0);
  @$pb.TagNumber(1)
  void clearStreamId() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get sequence => $_getI64(1);
  @$pb.TagNumber(2)
  set sequence($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSequence() => $_has(1);
  @$pb.TagNumber(2)
  void clearSequence() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get timestampUnixMs => $_getI64(2);
  @$pb.TagNumber(3)
  set timestampUnixMs($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasTimestampUnixMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearTimestampUnixMs() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get label => $_getSZ(3);
  @$pb.TagNumber(4)
  set label($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasLabel() => $_has(3);
  @$pb.TagNumber(4)
  void clearLabel() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get text => $_getSZ(4);
  @$pb.TagNumber(5)
  set text($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasText() => $_has(4);
  @$pb.TagNumber(5)
  void clearText() => $_clearField(5);
}

class TextDone extends $pb.GeneratedMessage {
  factory TextDone({
    $core.String? streamId,
    $fixnum.Int64? sequence,
    $fixnum.Int64? timestampUnixMs,
    $core.String? label,
    $core.String? text,
  }) {
    final result = create();
    if (streamId != null) result.streamId = streamId;
    if (sequence != null) result.sequence = sequence;
    if (timestampUnixMs != null) result.timestampUnixMs = timestampUnixMs;
    if (label != null) result.label = label;
    if (text != null) result.text = text;
    return result;
  }

  TextDone._();

  factory TextDone.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory TextDone.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'TextDone',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'streamId')
    ..a<$fixnum.Int64>(
        2, _omitFieldNames ? '' : 'sequence', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aInt64(3, _omitFieldNames ? '' : 'timestampUnixMs')
    ..aOS(4, _omitFieldNames ? '' : 'label')
    ..aOS(5, _omitFieldNames ? '' : 'text')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TextDone clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TextDone copyWith(void Function(TextDone) updates) =>
      super.copyWith((message) => updates(message as TextDone)) as TextDone;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static TextDone create() => TextDone._();
  @$core.override
  TextDone createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static TextDone getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<TextDone>(create);
  static TextDone? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get streamId => $_getSZ(0);
  @$pb.TagNumber(1)
  set streamId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasStreamId() => $_has(0);
  @$pb.TagNumber(1)
  void clearStreamId() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get sequence => $_getI64(1);
  @$pb.TagNumber(2)
  set sequence($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSequence() => $_has(1);
  @$pb.TagNumber(2)
  void clearSequence() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get timestampUnixMs => $_getI64(2);
  @$pb.TagNumber(3)
  set timestampUnixMs($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasTimestampUnixMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearTimestampUnixMs() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get label => $_getSZ(3);
  @$pb.TagNumber(4)
  set label($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasLabel() => $_has(3);
  @$pb.TagNumber(4)
  void clearLabel() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get text => $_getSZ(4);
  @$pb.TagNumber(5)
  set text($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasText() => $_has(4);
  @$pb.TagNumber(5)
  void clearText() => $_clearField(5);
}

class WorkspaceHistoryUpdated extends $pb.GeneratedMessage {
  factory WorkspaceHistoryUpdated({
    $core.String? workspaceName,
    WorkspaceKind? workspaceKind,
    $fixnum.Int64? lastUpdatedAtUnixMs,
  }) {
    final result = create();
    if (workspaceName != null) result.workspaceName = workspaceName;
    if (workspaceKind != null) result.workspaceKind = workspaceKind;
    if (lastUpdatedAtUnixMs != null)
      result.lastUpdatedAtUnixMs = lastUpdatedAtUnixMs;
    return result;
  }

  WorkspaceHistoryUpdated._();

  factory WorkspaceHistoryUpdated.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkspaceHistoryUpdated.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkspaceHistoryUpdated',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'workspaceName')
    ..aE<WorkspaceKind>(2, _omitFieldNames ? '' : 'workspaceKind',
        enumValues: WorkspaceKind.values)
    ..aInt64(3, _omitFieldNames ? '' : 'lastUpdatedAtUnixMs')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryUpdated clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkspaceHistoryUpdated copyWith(
          void Function(WorkspaceHistoryUpdated) updates) =>
      super.copyWith((message) => updates(message as WorkspaceHistoryUpdated))
          as WorkspaceHistoryUpdated;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryUpdated create() => WorkspaceHistoryUpdated._();
  @$core.override
  WorkspaceHistoryUpdated createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkspaceHistoryUpdated getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkspaceHistoryUpdated>(create);
  static WorkspaceHistoryUpdated? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get workspaceName => $_getSZ(0);
  @$pb.TagNumber(1)
  set workspaceName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasWorkspaceName() => $_has(0);
  @$pb.TagNumber(1)
  void clearWorkspaceName() => $_clearField(1);

  @$pb.TagNumber(2)
  WorkspaceKind get workspaceKind => $_getN(1);
  @$pb.TagNumber(2)
  set workspaceKind(WorkspaceKind value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasWorkspaceKind() => $_has(1);
  @$pb.TagNumber(2)
  void clearWorkspaceKind() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get lastUpdatedAtUnixMs => $_getI64(2);
  @$pb.TagNumber(3)
  set lastUpdatedAtUnixMs($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasLastUpdatedAtUnixMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearLastUpdatedAtUnixMs() => $_clearField(3);
}

class FriendRelationshipUpdated extends $pb.GeneratedMessage {
  factory FriendRelationshipUpdated({
    $core.String? peerPublicKey,
    $core.String? workspaceName,
    FriendRelationshipChange? change,
    $fixnum.Int64? revisionUnixMs,
  }) {
    final result = create();
    if (peerPublicKey != null) result.peerPublicKey = peerPublicKey;
    if (workspaceName != null) result.workspaceName = workspaceName;
    if (change != null) result.change = change;
    if (revisionUnixMs != null) result.revisionUnixMs = revisionUnixMs;
    return result;
  }

  FriendRelationshipUpdated._();

  factory FriendRelationshipUpdated.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendRelationshipUpdated.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendRelationshipUpdated',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'peerPublicKey')
    ..aOS(2, _omitFieldNames ? '' : 'workspaceName')
    ..aE<FriendRelationshipChange>(3, _omitFieldNames ? '' : 'change',
        enumValues: FriendRelationshipChange.values)
    ..aInt64(4, _omitFieldNames ? '' : 'revisionUnixMs')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendRelationshipUpdated clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendRelationshipUpdated copyWith(
          void Function(FriendRelationshipUpdated) updates) =>
      super.copyWith((message) => updates(message as FriendRelationshipUpdated))
          as FriendRelationshipUpdated;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendRelationshipUpdated create() => FriendRelationshipUpdated._();
  @$core.override
  FriendRelationshipUpdated createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendRelationshipUpdated getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendRelationshipUpdated>(create);
  static FriendRelationshipUpdated? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get peerPublicKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set peerPublicKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPeerPublicKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearPeerPublicKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get workspaceName => $_getSZ(1);
  @$pb.TagNumber(2)
  set workspaceName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasWorkspaceName() => $_has(1);
  @$pb.TagNumber(2)
  void clearWorkspaceName() => $_clearField(2);

  @$pb.TagNumber(3)
  FriendRelationshipChange get change => $_getN(2);
  @$pb.TagNumber(3)
  set change(FriendRelationshipChange value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasChange() => $_has(2);
  @$pb.TagNumber(3)
  void clearChange() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get revisionUnixMs => $_getI64(3);
  @$pb.TagNumber(4)
  set revisionUnixMs($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRevisionUnixMs() => $_has(3);
  @$pb.TagNumber(4)
  void clearRevisionUnixMs() => $_clearField(4);
}

class FriendGroupUpdated extends $pb.GeneratedMessage {
  factory FriendGroupUpdated({
    $core.String? friendGroupId,
    $core.String? workspaceName,
    FriendGroupChange? change,
    $fixnum.Int64? revisionUnixMs,
    $core.String? affectedPeerPublicKey,
  }) {
    final result = create();
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (workspaceName != null) result.workspaceName = workspaceName;
    if (change != null) result.change = change;
    if (revisionUnixMs != null) result.revisionUnixMs = revisionUnixMs;
    if (affectedPeerPublicKey != null)
      result.affectedPeerPublicKey = affectedPeerPublicKey;
    return result;
  }

  FriendGroupUpdated._();

  factory FriendGroupUpdated.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupUpdated.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupUpdated',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.events.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'friendGroupId')
    ..aOS(2, _omitFieldNames ? '' : 'workspaceName')
    ..aE<FriendGroupChange>(3, _omitFieldNames ? '' : 'change',
        enumValues: FriendGroupChange.values)
    ..aInt64(4, _omitFieldNames ? '' : 'revisionUnixMs')
    ..aOS(5, _omitFieldNames ? '' : 'affectedPeerPublicKey')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupUpdated clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupUpdated copyWith(void Function(FriendGroupUpdated) updates) =>
      super.copyWith((message) => updates(message as FriendGroupUpdated))
          as FriendGroupUpdated;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupUpdated create() => FriendGroupUpdated._();
  @$core.override
  FriendGroupUpdated createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupUpdated getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupUpdated>(create);
  static FriendGroupUpdated? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get friendGroupId => $_getSZ(0);
  @$pb.TagNumber(1)
  set friendGroupId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFriendGroupId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFriendGroupId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get workspaceName => $_getSZ(1);
  @$pb.TagNumber(2)
  set workspaceName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasWorkspaceName() => $_has(1);
  @$pb.TagNumber(2)
  void clearWorkspaceName() => $_clearField(2);

  @$pb.TagNumber(3)
  FriendGroupChange get change => $_getN(2);
  @$pb.TagNumber(3)
  set change(FriendGroupChange value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasChange() => $_has(2);
  @$pb.TagNumber(3)
  void clearChange() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get revisionUnixMs => $_getI64(3);
  @$pb.TagNumber(4)
  set revisionUnixMs($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRevisionUnixMs() => $_has(3);
  @$pb.TagNumber(4)
  void clearRevisionUnixMs() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get affectedPeerPublicKey => $_getSZ(4);
  @$pb.TagNumber(5)
  set affectedPeerPublicKey($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasAffectedPeerPublicKey() => $_has(4);
  @$pb.TagNumber(5)
  void clearAffectedPeerPublicKey() => $_clearField(5);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
