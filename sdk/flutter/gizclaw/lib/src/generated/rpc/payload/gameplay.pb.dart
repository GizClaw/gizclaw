// This is a generated file - do not edit.
//
// Generated from payload/gameplay.proto.

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

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

class Badge extends $pb.GeneratedMessage {
  factory Badge({
    $core.bool? active,
    $core.String? badgeDefId,
    $core.String? createdAt,
    $fixnum.Int64? exp,
    $core.String? id,
    $fixnum.Int64? level,
    $core.String? ownerPublicKey,
    $fixnum.Int64? progress,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (active != null) result.active = active;
    if (badgeDefId != null) result.badgeDefId = badgeDefId;
    if (createdAt != null) result.createdAt = createdAt;
    if (exp != null) result.exp = exp;
    if (id != null) result.id = id;
    if (level != null) result.level = level;
    if (ownerPublicKey != null) result.ownerPublicKey = ownerPublicKey;
    if (progress != null) result.progress = progress;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  Badge._();

  factory Badge.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Badge.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Badge',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'active')
    ..aOS(2, _omitFieldNames ? '' : 'badgeDefId')
    ..aOS(3, _omitFieldNames ? '' : 'createdAt')
    ..aInt64(4, _omitFieldNames ? '' : 'exp')
    ..aOS(5, _omitFieldNames ? '' : 'id')
    ..aInt64(6, _omitFieldNames ? '' : 'level')
    ..aOS(7, _omitFieldNames ? '' : 'ownerPublicKey')
    ..aInt64(8, _omitFieldNames ? '' : 'progress')
    ..aOS(9, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Badge clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Badge copyWith(void Function(Badge) updates) =>
      super.copyWith((message) => updates(message as Badge)) as Badge;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Badge create() => Badge._();
  @$core.override
  Badge createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Badge getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Badge>(create);
  static Badge? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get active => $_getBF(0);
  @$pb.TagNumber(1)
  set active($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasActive() => $_has(0);
  @$pb.TagNumber(1)
  void clearActive() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get badgeDefId => $_getSZ(1);
  @$pb.TagNumber(2)
  set badgeDefId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBadgeDefId() => $_has(1);
  @$pb.TagNumber(2)
  void clearBadgeDefId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get createdAt => $_getSZ(2);
  @$pb.TagNumber(3)
  set createdAt($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasCreatedAt() => $_has(2);
  @$pb.TagNumber(3)
  void clearCreatedAt() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get exp => $_getI64(3);
  @$pb.TagNumber(4)
  set exp($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasExp() => $_has(3);
  @$pb.TagNumber(4)
  void clearExp() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get id => $_getSZ(4);
  @$pb.TagNumber(5)
  set id($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasId() => $_has(4);
  @$pb.TagNumber(5)
  void clearId() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get level => $_getI64(5);
  @$pb.TagNumber(6)
  set level($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasLevel() => $_has(5);
  @$pb.TagNumber(6)
  void clearLevel() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get ownerPublicKey => $_getSZ(6);
  @$pb.TagNumber(7)
  set ownerPublicKey($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasOwnerPublicKey() => $_has(6);
  @$pb.TagNumber(7)
  void clearOwnerPublicKey() => $_clearField(7);

  @$pb.TagNumber(8)
  $fixnum.Int64 get progress => $_getI64(7);
  @$pb.TagNumber(8)
  set progress($fixnum.Int64 value) => $_setInt64(7, value);
  @$pb.TagNumber(8)
  $core.bool hasProgress() => $_has(7);
  @$pb.TagNumber(8)
  void clearProgress() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get updatedAt => $_getSZ(8);
  @$pb.TagNumber(9)
  set updatedAt($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasUpdatedAt() => $_has(8);
  @$pb.TagNumber(9)
  void clearUpdatedAt() => $_clearField(9);
}

class BadgeDefPixaDownloadRequest extends $pb.GeneratedMessage {
  factory BadgeDefPixaDownloadRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  BadgeDefPixaDownloadRequest._();

  factory BadgeDefPixaDownloadRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory BadgeDefPixaDownloadRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'BadgeDefPixaDownloadRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BadgeDefPixaDownloadRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BadgeDefPixaDownloadRequest copyWith(
          void Function(BadgeDefPixaDownloadRequest) updates) =>
      super.copyWith(
              (message) => updates(message as BadgeDefPixaDownloadRequest))
          as BadgeDefPixaDownloadRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static BadgeDefPixaDownloadRequest create() =>
      BadgeDefPixaDownloadRequest._();
  @$core.override
  BadgeDefPixaDownloadRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static BadgeDefPixaDownloadRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<BadgeDefPixaDownloadRequest>(create);
  static BadgeDefPixaDownloadRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class BadgeDefPixaDownloadResponse extends $pb.GeneratedMessage {
  factory BadgeDefPixaDownloadResponse({
    $core.String? id,
    $core.String? pixaPath,
    $fixnum.Int64? sizeBytes,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (pixaPath != null) result.pixaPath = pixaPath;
    if (sizeBytes != null) result.sizeBytes = sizeBytes;
    return result;
  }

  BadgeDefPixaDownloadResponse._();

  factory BadgeDefPixaDownloadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory BadgeDefPixaDownloadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'BadgeDefPixaDownloadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'pixaPath')
    ..aInt64(3, _omitFieldNames ? '' : 'sizeBytes')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BadgeDefPixaDownloadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BadgeDefPixaDownloadResponse copyWith(
          void Function(BadgeDefPixaDownloadResponse) updates) =>
      super.copyWith(
              (message) => updates(message as BadgeDefPixaDownloadResponse))
          as BadgeDefPixaDownloadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static BadgeDefPixaDownloadResponse create() =>
      BadgeDefPixaDownloadResponse._();
  @$core.override
  BadgeDefPixaDownloadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static BadgeDefPixaDownloadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<BadgeDefPixaDownloadResponse>(create);
  static BadgeDefPixaDownloadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get pixaPath => $_getSZ(1);
  @$pb.TagNumber(2)
  set pixaPath($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPixaPath() => $_has(1);
  @$pb.TagNumber(2)
  void clearPixaPath() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get sizeBytes => $_getI64(2);
  @$pb.TagNumber(3)
  set sizeBytes($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSizeBytes() => $_has(2);
  @$pb.TagNumber(3)
  void clearSizeBytes() => $_clearField(3);
}

class BadgeListResponse extends $pb.GeneratedMessage {
  factory BadgeListResponse({
    $core.bool? hasNext,
    $core.Iterable<Badge>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  BadgeListResponse._();

  factory BadgeListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory BadgeListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'BadgeListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<Badge>(2, _omitFieldNames ? '' : 'items', subBuilder: Badge.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BadgeListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BadgeListResponse copyWith(void Function(BadgeListResponse) updates) =>
      super.copyWith((message) => updates(message as BadgeListResponse))
          as BadgeListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static BadgeListResponse create() => BadgeListResponse._();
  @$core.override
  BadgeListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static BadgeListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<BadgeListResponse>(create);
  static BadgeListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<Badge> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class GameResult extends $pb.GeneratedMessage {
  factory GameResult({
    $core.String? createdAt,
    $core.String? difficulty,
    $fixnum.Int64? durationMs,
    $core.String? gameDefId,
    $core.String? id,
    $core.String? idempotencyKey,
    $fixnum.Int64? maxScore,
    $core.String? occurredAt,
    $core.String? outcome,
    $core.String? ownerPublicKey,
    GameplayMetadata? payload,
    $core.String? petId,
    $core.String? rulesetName,
    $fixnum.Int64? score,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (difficulty != null) result.difficulty = difficulty;
    if (durationMs != null) result.durationMs = durationMs;
    if (gameDefId != null) result.gameDefId = gameDefId;
    if (id != null) result.id = id;
    if (idempotencyKey != null) result.idempotencyKey = idempotencyKey;
    if (maxScore != null) result.maxScore = maxScore;
    if (occurredAt != null) result.occurredAt = occurredAt;
    if (outcome != null) result.outcome = outcome;
    if (ownerPublicKey != null) result.ownerPublicKey = ownerPublicKey;
    if (payload != null) result.payload = payload;
    if (petId != null) result.petId = petId;
    if (rulesetName != null) result.rulesetName = rulesetName;
    if (score != null) result.score = score;
    return result;
  }

  GameResult._();

  factory GameResult.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameResult',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'difficulty')
    ..aInt64(3, _omitFieldNames ? '' : 'durationMs')
    ..aOS(4, _omitFieldNames ? '' : 'gameDefId')
    ..aOS(5, _omitFieldNames ? '' : 'id')
    ..aOS(6, _omitFieldNames ? '' : 'idempotencyKey')
    ..aInt64(7, _omitFieldNames ? '' : 'maxScore')
    ..aOS(8, _omitFieldNames ? '' : 'occurredAt')
    ..aOS(9, _omitFieldNames ? '' : 'outcome')
    ..aOS(10, _omitFieldNames ? '' : 'ownerPublicKey')
    ..aOM<GameplayMetadata>(11, _omitFieldNames ? '' : 'payload',
        subBuilder: GameplayMetadata.create)
    ..aOS(12, _omitFieldNames ? '' : 'petId')
    ..aOS(13, _omitFieldNames ? '' : 'rulesetName')
    ..aInt64(14, _omitFieldNames ? '' : 'score')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameResult clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameResult copyWith(void Function(GameResult) updates) =>
      super.copyWith((message) => updates(message as GameResult)) as GameResult;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameResult create() => GameResult._();
  @$core.override
  GameResult createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameResult getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameResult>(create);
  static GameResult? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get difficulty => $_getSZ(1);
  @$pb.TagNumber(2)
  set difficulty($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDifficulty() => $_has(1);
  @$pb.TagNumber(2)
  void clearDifficulty() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get durationMs => $_getI64(2);
  @$pb.TagNumber(3)
  set durationMs($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDurationMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearDurationMs() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get gameDefId => $_getSZ(3);
  @$pb.TagNumber(4)
  set gameDefId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasGameDefId() => $_has(3);
  @$pb.TagNumber(4)
  void clearGameDefId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get id => $_getSZ(4);
  @$pb.TagNumber(5)
  set id($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasId() => $_has(4);
  @$pb.TagNumber(5)
  void clearId() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get idempotencyKey => $_getSZ(5);
  @$pb.TagNumber(6)
  set idempotencyKey($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasIdempotencyKey() => $_has(5);
  @$pb.TagNumber(6)
  void clearIdempotencyKey() => $_clearField(6);

  @$pb.TagNumber(7)
  $fixnum.Int64 get maxScore => $_getI64(6);
  @$pb.TagNumber(7)
  set maxScore($fixnum.Int64 value) => $_setInt64(6, value);
  @$pb.TagNumber(7)
  $core.bool hasMaxScore() => $_has(6);
  @$pb.TagNumber(7)
  void clearMaxScore() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get occurredAt => $_getSZ(7);
  @$pb.TagNumber(8)
  set occurredAt($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasOccurredAt() => $_has(7);
  @$pb.TagNumber(8)
  void clearOccurredAt() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get outcome => $_getSZ(8);
  @$pb.TagNumber(9)
  set outcome($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasOutcome() => $_has(8);
  @$pb.TagNumber(9)
  void clearOutcome() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get ownerPublicKey => $_getSZ(9);
  @$pb.TagNumber(10)
  set ownerPublicKey($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasOwnerPublicKey() => $_has(9);
  @$pb.TagNumber(10)
  void clearOwnerPublicKey() => $_clearField(10);

  @$pb.TagNumber(11)
  GameplayMetadata get payload => $_getN(10);
  @$pb.TagNumber(11)
  set payload(GameplayMetadata value) => $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasPayload() => $_has(10);
  @$pb.TagNumber(11)
  void clearPayload() => $_clearField(11);
  @$pb.TagNumber(11)
  GameplayMetadata ensurePayload() => $_ensure(10);

  @$pb.TagNumber(12)
  $core.String get petId => $_getSZ(11);
  @$pb.TagNumber(12)
  set petId($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasPetId() => $_has(11);
  @$pb.TagNumber(12)
  void clearPetId() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.String get rulesetName => $_getSZ(12);
  @$pb.TagNumber(13)
  set rulesetName($core.String value) => $_setString(12, value);
  @$pb.TagNumber(13)
  $core.bool hasRulesetName() => $_has(12);
  @$pb.TagNumber(13)
  void clearRulesetName() => $_clearField(13);

  @$pb.TagNumber(14)
  $fixnum.Int64 get score => $_getI64(13);
  @$pb.TagNumber(14)
  set score($fixnum.Int64 value) => $_setInt64(13, value);
  @$pb.TagNumber(14)
  $core.bool hasScore() => $_has(13);
  @$pb.TagNumber(14)
  void clearScore() => $_clearField(14);
}

class GameResultListResponse extends $pb.GeneratedMessage {
  factory GameResultListResponse({
    $core.bool? hasNext,
    $core.Iterable<GameResult>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  GameResultListResponse._();

  factory GameResultListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameResultListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameResultListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<GameResult>(2, _omitFieldNames ? '' : 'items',
        subBuilder: GameResult.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameResultListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameResultListResponse copyWith(
          void Function(GameResultListResponse) updates) =>
      super.copyWith((message) => updates(message as GameResultListResponse))
          as GameResultListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameResultListResponse create() => GameResultListResponse._();
  @$core.override
  GameResultListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameResultListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameResultListResponse>(create);
  static GameResultListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<GameResult> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class GameRewardSpec extends $pb.GeneratedMessage {
  factory GameRewardSpec({
    $core.Iterable<$core.MapEntry<$core.String, $fixnum.Int64>>? badgeExpDelta,
    $fixnum.Int64? petExpDelta,
    $fixnum.Int64? pointsDelta,
  }) {
    final result = create();
    if (badgeExpDelta != null) result.badgeExpDelta.addEntries(badgeExpDelta);
    if (petExpDelta != null) result.petExpDelta = petExpDelta;
    if (pointsDelta != null) result.pointsDelta = pointsDelta;
    return result;
  }

  GameRewardSpec._();

  factory GameRewardSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameRewardSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameRewardSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, $fixnum.Int64>(2, _omitFieldNames ? '' : 'badgeExpDelta',
        entryClassName: 'GameRewardSpec.BadgeExpDeltaEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.O6,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..aInt64(4, _omitFieldNames ? '' : 'petExpDelta')
    ..aInt64(5, _omitFieldNames ? '' : 'pointsDelta')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRewardSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRewardSpec copyWith(void Function(GameRewardSpec) updates) =>
      super.copyWith((message) => updates(message as GameRewardSpec))
          as GameRewardSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameRewardSpec create() => GameRewardSpec._();
  @$core.override
  GameRewardSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameRewardSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameRewardSpec>(create);
  static GameRewardSpec? _defaultInstance;

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, $fixnum.Int64> get badgeExpDelta => $_getMap(0);

  @$pb.TagNumber(4)
  $fixnum.Int64 get petExpDelta => $_getI64(1);
  @$pb.TagNumber(4)
  set petExpDelta($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(4)
  $core.bool hasPetExpDelta() => $_has(1);
  @$pb.TagNumber(4)
  void clearPetExpDelta() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get pointsDelta => $_getI64(2);
  @$pb.TagNumber(5)
  set pointsDelta($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(5)
  $core.bool hasPointsDelta() => $_has(2);
  @$pb.TagNumber(5)
  void clearPointsDelta() => $_clearField(5);
}

class GameRuleset extends $pb.GeneratedMessage {
  factory GameRuleset({
    $core.String? createdAt,
    $core.String? name,
    GameRulesetSpec? spec,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (name != null) result.name = name;
    if (spec != null) result.spec = spec;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  GameRuleset._();

  factory GameRuleset.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameRuleset.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameRuleset',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..aOM<GameRulesetSpec>(3, _omitFieldNames ? '' : 'spec',
        subBuilder: GameRulesetSpec.create)
    ..aOS(4, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRuleset clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRuleset copyWith(void Function(GameRuleset) updates) =>
      super.copyWith((message) => updates(message as GameRuleset))
          as GameRuleset;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameRuleset create() => GameRuleset._();
  @$core.override
  GameRuleset createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameRuleset getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameRuleset>(create);
  static GameRuleset? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);

  @$pb.TagNumber(3)
  GameRulesetSpec get spec => $_getN(2);
  @$pb.TagNumber(3)
  set spec(GameRulesetSpec value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasSpec() => $_has(2);
  @$pb.TagNumber(3)
  void clearSpec() => $_clearField(3);
  @$pb.TagNumber(3)
  GameRulesetSpec ensureSpec() => $_ensure(2);

  @$pb.TagNumber(4)
  $core.String get updatedAt => $_getSZ(3);
  @$pb.TagNumber(4)
  set updatedAt($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasUpdatedAt() => $_has(3);
  @$pb.TagNumber(4)
  void clearUpdatedAt() => $_clearField(4);
}

class GameRulesetDriveSpec extends $pb.GeneratedMessage {
  factory GameRulesetDriveSpec({
    GameRewardSpec? defaultReward,
    $core.Iterable<$core.MapEntry<$core.String, GameRewardSpec>>? gameRewards,
  }) {
    final result = create();
    if (defaultReward != null) result.defaultReward = defaultReward;
    if (gameRewards != null) result.gameRewards.addEntries(gameRewards);
    return result;
  }

  GameRulesetDriveSpec._();

  factory GameRulesetDriveSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameRulesetDriveSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameRulesetDriveSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameRewardSpec>(3, _omitFieldNames ? '' : 'defaultReward',
        subBuilder: GameRewardSpec.create)
    ..m<$core.String, GameRewardSpec>(4, _omitFieldNames ? '' : 'gameRewards',
        entryClassName: 'GameRulesetDriveSpec.GameRewardsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: GameRewardSpec.create,
        valueDefaultOrMaker: GameRewardSpec.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRulesetDriveSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRulesetDriveSpec copyWith(void Function(GameRulesetDriveSpec) updates) =>
      super.copyWith((message) => updates(message as GameRulesetDriveSpec))
          as GameRulesetDriveSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameRulesetDriveSpec create() => GameRulesetDriveSpec._();
  @$core.override
  GameRulesetDriveSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameRulesetDriveSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameRulesetDriveSpec>(create);
  static GameRulesetDriveSpec? _defaultInstance;

  @$pb.TagNumber(3)
  GameRewardSpec get defaultReward => $_getN(0);
  @$pb.TagNumber(3)
  set defaultReward(GameRewardSpec value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasDefaultReward() => $_has(0);
  @$pb.TagNumber(3)
  void clearDefaultReward() => $_clearField(3);
  @$pb.TagNumber(3)
  GameRewardSpec ensureDefaultReward() => $_ensure(0);

  @$pb.TagNumber(4)
  $pb.PbMap<$core.String, GameRewardSpec> get gameRewards => $_getMap(1);
}

class GameRulesetPetPoolEntry extends $pb.GeneratedMessage {
  factory GameRulesetPetPoolEntry({
    $fixnum.Int64? adoptionCost,
    $core.String? petdefId,
    $core.String? rarity,
    $fixnum.Int64? weight,
    $core.String? workflowName,
  }) {
    final result = create();
    if (adoptionCost != null) result.adoptionCost = adoptionCost;
    if (petdefId != null) result.petdefId = petdefId;
    if (rarity != null) result.rarity = rarity;
    if (weight != null) result.weight = weight;
    if (workflowName != null) result.workflowName = workflowName;
    return result;
  }

  GameRulesetPetPoolEntry._();

  factory GameRulesetPetPoolEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameRulesetPetPoolEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameRulesetPetPoolEntry',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'adoptionCost')
    ..aOS(2, _omitFieldNames ? '' : 'petdefId')
    ..aOS(3, _omitFieldNames ? '' : 'rarity')
    ..aInt64(4, _omitFieldNames ? '' : 'weight')
    ..aOS(5, _omitFieldNames ? '' : 'workflowName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRulesetPetPoolEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRulesetPetPoolEntry copyWith(
          void Function(GameRulesetPetPoolEntry) updates) =>
      super.copyWith((message) => updates(message as GameRulesetPetPoolEntry))
          as GameRulesetPetPoolEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameRulesetPetPoolEntry create() => GameRulesetPetPoolEntry._();
  @$core.override
  GameRulesetPetPoolEntry createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameRulesetPetPoolEntry getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameRulesetPetPoolEntry>(create);
  static GameRulesetPetPoolEntry? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get adoptionCost => $_getI64(0);
  @$pb.TagNumber(1)
  set adoptionCost($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAdoptionCost() => $_has(0);
  @$pb.TagNumber(1)
  void clearAdoptionCost() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get petdefId => $_getSZ(1);
  @$pb.TagNumber(2)
  set petdefId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPetdefId() => $_has(1);
  @$pb.TagNumber(2)
  void clearPetdefId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get rarity => $_getSZ(2);
  @$pb.TagNumber(3)
  set rarity($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRarity() => $_has(2);
  @$pb.TagNumber(3)
  void clearRarity() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get weight => $_getI64(3);
  @$pb.TagNumber(4)
  set weight($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasWeight() => $_has(3);
  @$pb.TagNumber(4)
  void clearWeight() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get workflowName => $_getSZ(4);
  @$pb.TagNumber(5)
  set workflowName($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasWorkflowName() => $_has(4);
  @$pb.TagNumber(5)
  void clearWorkflowName() => $_clearField(5);
}

class GameRulesetPointsSpec extends $pb.GeneratedMessage {
  factory GameRulesetPointsSpec({
    $fixnum.Int64? initialBalance,
  }) {
    final result = create();
    if (initialBalance != null) result.initialBalance = initialBalance;
    return result;
  }

  GameRulesetPointsSpec._();

  factory GameRulesetPointsSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameRulesetPointsSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameRulesetPointsSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'initialBalance')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRulesetPointsSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRulesetPointsSpec copyWith(
          void Function(GameRulesetPointsSpec) updates) =>
      super.copyWith((message) => updates(message as GameRulesetPointsSpec))
          as GameRulesetPointsSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameRulesetPointsSpec create() => GameRulesetPointsSpec._();
  @$core.override
  GameRulesetPointsSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameRulesetPointsSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameRulesetPointsSpec>(create);
  static GameRulesetPointsSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get initialBalance => $_getI64(0);
  @$pb.TagNumber(1)
  set initialBalance($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasInitialBalance() => $_has(0);
  @$pb.TagNumber(1)
  void clearInitialBalance() => $_clearField(1);
}

class GameRulesetSpec extends $pb.GeneratedMessage {
  factory GameRulesetSpec({
    $core.Iterable<$core.String>? badgeDefIds,
    $core.String? defaultWorkflowName,
    $core.String? description,
    GameRulesetDriveSpec? drive,
    $core.bool? enabled,
    $core.Iterable<$core.String>? gameDefIds,
    GameplayMetadata? metadata,
    $core.Iterable<GameRulesetPetPoolEntry>? petPool,
    GameRulesetPointsSpec? points,
  }) {
    final result = create();
    if (badgeDefIds != null) result.badgeDefIds.addAll(badgeDefIds);
    if (defaultWorkflowName != null)
      result.defaultWorkflowName = defaultWorkflowName;
    if (description != null) result.description = description;
    if (drive != null) result.drive = drive;
    if (enabled != null) result.enabled = enabled;
    if (gameDefIds != null) result.gameDefIds.addAll(gameDefIds);
    if (metadata != null) result.metadata = metadata;
    if (petPool != null) result.petPool.addAll(petPool);
    if (points != null) result.points = points;
    return result;
  }

  GameRulesetSpec._();

  factory GameRulesetSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameRulesetSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameRulesetSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPS(1, _omitFieldNames ? '' : 'badgeDefIds')
    ..aOS(2, _omitFieldNames ? '' : 'defaultWorkflowName')
    ..aOS(3, _omitFieldNames ? '' : 'description')
    ..aOM<GameRulesetDriveSpec>(4, _omitFieldNames ? '' : 'drive',
        subBuilder: GameRulesetDriveSpec.create)
    ..aOB(5, _omitFieldNames ? '' : 'enabled')
    ..pPS(6, _omitFieldNames ? '' : 'gameDefIds')
    ..aOM<GameplayMetadata>(7, _omitFieldNames ? '' : 'metadata',
        subBuilder: GameplayMetadata.create)
    ..pPM<GameRulesetPetPoolEntry>(8, _omitFieldNames ? '' : 'petPool',
        subBuilder: GameRulesetPetPoolEntry.create)
    ..aOM<GameRulesetPointsSpec>(9, _omitFieldNames ? '' : 'points',
        subBuilder: GameRulesetPointsSpec.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRulesetSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameRulesetSpec copyWith(void Function(GameRulesetSpec) updates) =>
      super.copyWith((message) => updates(message as GameRulesetSpec))
          as GameRulesetSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameRulesetSpec create() => GameRulesetSpec._();
  @$core.override
  GameRulesetSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameRulesetSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameRulesetSpec>(create);
  static GameRulesetSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<$core.String> get badgeDefIds => $_getList(0);

  @$pb.TagNumber(2)
  $core.String get defaultWorkflowName => $_getSZ(1);
  @$pb.TagNumber(2)
  set defaultWorkflowName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDefaultWorkflowName() => $_has(1);
  @$pb.TagNumber(2)
  void clearDefaultWorkflowName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get description => $_getSZ(2);
  @$pb.TagNumber(3)
  set description($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDescription() => $_has(2);
  @$pb.TagNumber(3)
  void clearDescription() => $_clearField(3);

  @$pb.TagNumber(4)
  GameRulesetDriveSpec get drive => $_getN(3);
  @$pb.TagNumber(4)
  set drive(GameRulesetDriveSpec value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasDrive() => $_has(3);
  @$pb.TagNumber(4)
  void clearDrive() => $_clearField(4);
  @$pb.TagNumber(4)
  GameRulesetDriveSpec ensureDrive() => $_ensure(3);

  @$pb.TagNumber(5)
  $core.bool get enabled => $_getBF(4);
  @$pb.TagNumber(5)
  set enabled($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasEnabled() => $_has(4);
  @$pb.TagNumber(5)
  void clearEnabled() => $_clearField(5);

  @$pb.TagNumber(6)
  $pb.PbList<$core.String> get gameDefIds => $_getList(5);

  @$pb.TagNumber(7)
  GameplayMetadata get metadata => $_getN(6);
  @$pb.TagNumber(7)
  set metadata(GameplayMetadata value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasMetadata() => $_has(6);
  @$pb.TagNumber(7)
  void clearMetadata() => $_clearField(7);
  @$pb.TagNumber(7)
  GameplayMetadata ensureMetadata() => $_ensure(6);

  @$pb.TagNumber(8)
  $pb.PbList<GameRulesetPetPoolEntry> get petPool => $_getList(7);

  @$pb.TagNumber(9)
  GameRulesetPointsSpec get points => $_getN(8);
  @$pb.TagNumber(9)
  set points(GameRulesetPointsSpec value) => $_setField(9, value);
  @$pb.TagNumber(9)
  $core.bool hasPoints() => $_has(8);
  @$pb.TagNumber(9)
  void clearPoints() => $_clearField(9);
  @$pb.TagNumber(9)
  GameRulesetPointsSpec ensurePoints() => $_ensure(8);
}

class GameplayGetRequest extends $pb.GeneratedMessage {
  factory GameplayGetRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  GameplayGetRequest._();

  factory GameplayGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameplayGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameplayGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameplayGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameplayGetRequest copyWith(void Function(GameplayGetRequest) updates) =>
      super.copyWith((message) => updates(message as GameplayGetRequest))
          as GameplayGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameplayGetRequest create() => GameplayGetRequest._();
  @$core.override
  GameplayGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameplayGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameplayGetRequest>(create);
  static GameplayGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class GameplayListRequest extends $pb.GeneratedMessage {
  factory GameplayListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  GameplayListRequest._();

  factory GameplayListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameplayListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameplayListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameplayListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameplayListRequest copyWith(void Function(GameplayListRequest) updates) =>
      super.copyWith((message) => updates(message as GameplayListRequest))
          as GameplayListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameplayListRequest create() => GameplayListRequest._();
  @$core.override
  GameplayListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameplayListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameplayListRequest>(create);
  static GameplayListRequest? _defaultInstance;

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
}

class GameplayMetadata extends $pb.GeneratedMessage {
  factory GameplayMetadata({
    $0.Struct? fields,
  }) {
    final result = create();
    if (fields != null) result.fields = fields;
    return result;
  }

  GameplayMetadata._();

  factory GameplayMetadata.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GameplayMetadata.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GameplayMetadata',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<$0.Struct>(1, _omitFieldNames ? '' : 'fields',
        subBuilder: $0.Struct.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameplayMetadata clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GameplayMetadata copyWith(void Function(GameplayMetadata) updates) =>
      super.copyWith((message) => updates(message as GameplayMetadata))
          as GameplayMetadata;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GameplayMetadata create() => GameplayMetadata._();
  @$core.override
  GameplayMetadata createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GameplayMetadata getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GameplayMetadata>(create);
  static GameplayMetadata? _defaultInstance;

  @$pb.TagNumber(1)
  $0.Struct get fields => $_getN(0);
  @$pb.TagNumber(1)
  set fields($0.Struct value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasFields() => $_has(0);
  @$pb.TagNumber(1)
  void clearFields() => $_clearField(1);
  @$pb.TagNumber(1)
  $0.Struct ensureFields() => $_ensure(0);
}

class Pet extends $pb.GeneratedMessage {
  factory Pet({
    $core.String? createdAt,
    $core.String? displayName,
    $core.String? id,
    $core.String? lastActiveAt,
    PetLife? life,
    $core.String? ownerPublicKey,
    $core.String? petdefId,
    $core.String? rulesetName,
    $core.String? updatedAt,
    $core.String? workflowName,
    $core.String? workspaceName,
    PetProgression? progression,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (displayName != null) result.displayName = displayName;
    if (id != null) result.id = id;
    if (lastActiveAt != null) result.lastActiveAt = lastActiveAt;
    if (life != null) result.life = life;
    if (ownerPublicKey != null) result.ownerPublicKey = ownerPublicKey;
    if (petdefId != null) result.petdefId = petdefId;
    if (rulesetName != null) result.rulesetName = rulesetName;
    if (updatedAt != null) result.updatedAt = updatedAt;
    if (workflowName != null) result.workflowName = workflowName;
    if (workspaceName != null) result.workspaceName = workspaceName;
    if (progression != null) result.progression = progression;
    return result;
  }

  Pet._();

  factory Pet.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Pet.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Pet',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(2, _omitFieldNames ? '' : 'createdAt')
    ..aOS(3, _omitFieldNames ? '' : 'displayName')
    ..aOS(5, _omitFieldNames ? '' : 'id')
    ..aOS(6, _omitFieldNames ? '' : 'lastActiveAt')
    ..aOM<PetLife>(8, _omitFieldNames ? '' : 'life', subBuilder: PetLife.create)
    ..aOS(9, _omitFieldNames ? '' : 'ownerPublicKey')
    ..aOS(10, _omitFieldNames ? '' : 'petdefId')
    ..aOS(11, _omitFieldNames ? '' : 'rulesetName')
    ..aOS(12, _omitFieldNames ? '' : 'updatedAt')
    ..aOS(13, _omitFieldNames ? '' : 'workflowName')
    ..aOS(14, _omitFieldNames ? '' : 'workspaceName')
    ..aOM<PetProgression>(15, _omitFieldNames ? '' : 'progression',
        subBuilder: PetProgression.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Pet clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Pet copyWith(void Function(Pet) updates) =>
      super.copyWith((message) => updates(message as Pet)) as Pet;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Pet create() => Pet._();
  @$core.override
  Pet createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Pet getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Pet>(create);
  static Pet? _defaultInstance;

  @$pb.TagNumber(2)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(2)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(2)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(2)
  void clearCreatedAt() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get displayName => $_getSZ(1);
  @$pb.TagNumber(3)
  set displayName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(3)
  $core.bool hasDisplayName() => $_has(1);
  @$pb.TagNumber(3)
  void clearDisplayName() => $_clearField(3);

  @$pb.TagNumber(5)
  $core.String get id => $_getSZ(2);
  @$pb.TagNumber(5)
  set id($core.String value) => $_setString(2, value);
  @$pb.TagNumber(5)
  $core.bool hasId() => $_has(2);
  @$pb.TagNumber(5)
  void clearId() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get lastActiveAt => $_getSZ(3);
  @$pb.TagNumber(6)
  set lastActiveAt($core.String value) => $_setString(3, value);
  @$pb.TagNumber(6)
  $core.bool hasLastActiveAt() => $_has(3);
  @$pb.TagNumber(6)
  void clearLastActiveAt() => $_clearField(6);

  @$pb.TagNumber(8)
  PetLife get life => $_getN(4);
  @$pb.TagNumber(8)
  set life(PetLife value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasLife() => $_has(4);
  @$pb.TagNumber(8)
  void clearLife() => $_clearField(8);
  @$pb.TagNumber(8)
  PetLife ensureLife() => $_ensure(4);

  @$pb.TagNumber(9)
  $core.String get ownerPublicKey => $_getSZ(5);
  @$pb.TagNumber(9)
  set ownerPublicKey($core.String value) => $_setString(5, value);
  @$pb.TagNumber(9)
  $core.bool hasOwnerPublicKey() => $_has(5);
  @$pb.TagNumber(9)
  void clearOwnerPublicKey() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get petdefId => $_getSZ(6);
  @$pb.TagNumber(10)
  set petdefId($core.String value) => $_setString(6, value);
  @$pb.TagNumber(10)
  $core.bool hasPetdefId() => $_has(6);
  @$pb.TagNumber(10)
  void clearPetdefId() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.String get rulesetName => $_getSZ(7);
  @$pb.TagNumber(11)
  set rulesetName($core.String value) => $_setString(7, value);
  @$pb.TagNumber(11)
  $core.bool hasRulesetName() => $_has(7);
  @$pb.TagNumber(11)
  void clearRulesetName() => $_clearField(11);

  @$pb.TagNumber(12)
  $core.String get updatedAt => $_getSZ(8);
  @$pb.TagNumber(12)
  set updatedAt($core.String value) => $_setString(8, value);
  @$pb.TagNumber(12)
  $core.bool hasUpdatedAt() => $_has(8);
  @$pb.TagNumber(12)
  void clearUpdatedAt() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.String get workflowName => $_getSZ(9);
  @$pb.TagNumber(13)
  set workflowName($core.String value) => $_setString(9, value);
  @$pb.TagNumber(13)
  $core.bool hasWorkflowName() => $_has(9);
  @$pb.TagNumber(13)
  void clearWorkflowName() => $_clearField(13);

  @$pb.TagNumber(14)
  $core.String get workspaceName => $_getSZ(10);
  @$pb.TagNumber(14)
  set workspaceName($core.String value) => $_setString(10, value);
  @$pb.TagNumber(14)
  $core.bool hasWorkspaceName() => $_has(10);
  @$pb.TagNumber(14)
  void clearWorkspaceName() => $_clearField(14);

  @$pb.TagNumber(15)
  PetProgression get progression => $_getN(11);
  @$pb.TagNumber(15)
  set progression(PetProgression value) => $_setField(15, value);
  @$pb.TagNumber(15)
  $core.bool hasProgression() => $_has(11);
  @$pb.TagNumber(15)
  void clearProgression() => $_clearField(15);
  @$pb.TagNumber(15)
  PetProgression ensureProgression() => $_ensure(11);
}

class PetAdoptRequest extends $pb.GeneratedMessage {
  factory PetAdoptRequest({
    $core.String? displayName,
    $core.String? rulesetName,
  }) {
    final result = create();
    if (displayName != null) result.displayName = displayName;
    if (rulesetName != null) result.rulesetName = rulesetName;
    return result;
  }

  PetAdoptRequest._();

  factory PetAdoptRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetAdoptRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetAdoptRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..aOS(2, _omitFieldNames ? '' : 'rulesetName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetAdoptRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetAdoptRequest copyWith(void Function(PetAdoptRequest) updates) =>
      super.copyWith((message) => updates(message as PetAdoptRequest))
          as PetAdoptRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetAdoptRequest create() => PetAdoptRequest._();
  @$core.override
  PetAdoptRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetAdoptRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetAdoptRequest>(create);
  static PetAdoptRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get rulesetName => $_getSZ(1);
  @$pb.TagNumber(2)
  set rulesetName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRulesetName() => $_has(1);
  @$pb.TagNumber(2)
  void clearRulesetName() => $_clearField(2);
}

class PetAdoptResponse extends $pb.GeneratedMessage {
  factory PetAdoptResponse({
    Pet? pet,
    PointsAccount? points,
    PointsTransaction? transaction,
  }) {
    final result = create();
    if (pet != null) result.pet = pet;
    if (points != null) result.points = points;
    if (transaction != null) result.transaction = transaction;
    return result;
  }

  PetAdoptResponse._();

  factory PetAdoptResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetAdoptResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetAdoptResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Pet>(1, _omitFieldNames ? '' : 'pet', subBuilder: Pet.create)
    ..aOM<PointsAccount>(2, _omitFieldNames ? '' : 'points',
        subBuilder: PointsAccount.create)
    ..aOM<PointsTransaction>(3, _omitFieldNames ? '' : 'transaction',
        subBuilder: PointsTransaction.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetAdoptResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetAdoptResponse copyWith(void Function(PetAdoptResponse) updates) =>
      super.copyWith((message) => updates(message as PetAdoptResponse))
          as PetAdoptResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetAdoptResponse create() => PetAdoptResponse._();
  @$core.override
  PetAdoptResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetAdoptResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetAdoptResponse>(create);
  static PetAdoptResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Pet get pet => $_getN(0);
  @$pb.TagNumber(1)
  set pet(Pet value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasPet() => $_has(0);
  @$pb.TagNumber(1)
  void clearPet() => $_clearField(1);
  @$pb.TagNumber(1)
  Pet ensurePet() => $_ensure(0);

  @$pb.TagNumber(2)
  PointsAccount get points => $_getN(1);
  @$pb.TagNumber(2)
  set points(PointsAccount value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasPoints() => $_has(1);
  @$pb.TagNumber(2)
  void clearPoints() => $_clearField(2);
  @$pb.TagNumber(2)
  PointsAccount ensurePoints() => $_ensure(1);

  @$pb.TagNumber(3)
  PointsTransaction get transaction => $_getN(2);
  @$pb.TagNumber(3)
  set transaction(PointsTransaction value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasTransaction() => $_has(2);
  @$pb.TagNumber(3)
  void clearTransaction() => $_clearField(3);
  @$pb.TagNumber(3)
  PointsTransaction ensureTransaction() => $_ensure(2);
}

class PetDefPixaDownloadRequest extends $pb.GeneratedMessage {
  factory PetDefPixaDownloadRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  PetDefPixaDownloadRequest._();

  factory PetDefPixaDownloadRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetDefPixaDownloadRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetDefPixaDownloadRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDefPixaDownloadRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDefPixaDownloadRequest copyWith(
          void Function(PetDefPixaDownloadRequest) updates) =>
      super.copyWith((message) => updates(message as PetDefPixaDownloadRequest))
          as PetDefPixaDownloadRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetDefPixaDownloadRequest create() => PetDefPixaDownloadRequest._();
  @$core.override
  PetDefPixaDownloadRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetDefPixaDownloadRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetDefPixaDownloadRequest>(create);
  static PetDefPixaDownloadRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class PetDefPixaDownloadResponse extends $pb.GeneratedMessage {
  factory PetDefPixaDownloadResponse({
    $core.String? id,
    $core.String? pixaPath,
    $fixnum.Int64? sizeBytes,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (pixaPath != null) result.pixaPath = pixaPath;
    if (sizeBytes != null) result.sizeBytes = sizeBytes;
    return result;
  }

  PetDefPixaDownloadResponse._();

  factory PetDefPixaDownloadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetDefPixaDownloadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetDefPixaDownloadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'pixaPath')
    ..aInt64(3, _omitFieldNames ? '' : 'sizeBytes')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDefPixaDownloadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDefPixaDownloadResponse copyWith(
          void Function(PetDefPixaDownloadResponse) updates) =>
      super.copyWith(
              (message) => updates(message as PetDefPixaDownloadResponse))
          as PetDefPixaDownloadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetDefPixaDownloadResponse create() => PetDefPixaDownloadResponse._();
  @$core.override
  PetDefPixaDownloadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetDefPixaDownloadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetDefPixaDownloadResponse>(create);
  static PetDefPixaDownloadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get pixaPath => $_getSZ(1);
  @$pb.TagNumber(2)
  set pixaPath($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPixaPath() => $_has(1);
  @$pb.TagNumber(2)
  void clearPixaPath() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get sizeBytes => $_getI64(2);
  @$pb.TagNumber(3)
  set sizeBytes($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSizeBytes() => $_has(2);
  @$pb.TagNumber(3)
  void clearSizeBytes() => $_clearField(3);
}

class PetPixaDownloadRequest extends $pb.GeneratedMessage {
  factory PetPixaDownloadRequest({
    $core.String? petId,
  }) {
    final result = create();
    if (petId != null) result.petId = petId;
    return result;
  }

  PetPixaDownloadRequest._();

  factory PetPixaDownloadRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPixaDownloadRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPixaDownloadRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'petId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPixaDownloadRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPixaDownloadRequest copyWith(
          void Function(PetPixaDownloadRequest) updates) =>
      super.copyWith((message) => updates(message as PetPixaDownloadRequest))
          as PetPixaDownloadRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPixaDownloadRequest create() => PetPixaDownloadRequest._();
  @$core.override
  PetPixaDownloadRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPixaDownloadRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPixaDownloadRequest>(create);
  static PetPixaDownloadRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get petId => $_getSZ(0);
  @$pb.TagNumber(1)
  set petId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPetId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPetId() => $_clearField(1);
}

class PetPixaDownloadResponse extends $pb.GeneratedMessage {
  factory PetPixaDownloadResponse({
    $core.String? petId,
    $core.String? petdefId,
    $core.String? pixaPath,
    $fixnum.Int64? sizeBytes,
  }) {
    final result = create();
    if (petId != null) result.petId = petId;
    if (petdefId != null) result.petdefId = petdefId;
    if (pixaPath != null) result.pixaPath = pixaPath;
    if (sizeBytes != null) result.sizeBytes = sizeBytes;
    return result;
  }

  PetPixaDownloadResponse._();

  factory PetPixaDownloadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPixaDownloadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPixaDownloadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'petId')
    ..aOS(2, _omitFieldNames ? '' : 'petdefId')
    ..aOS(3, _omitFieldNames ? '' : 'pixaPath')
    ..aInt64(4, _omitFieldNames ? '' : 'sizeBytes')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPixaDownloadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPixaDownloadResponse copyWith(
          void Function(PetPixaDownloadResponse) updates) =>
      super.copyWith((message) => updates(message as PetPixaDownloadResponse))
          as PetPixaDownloadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPixaDownloadResponse create() => PetPixaDownloadResponse._();
  @$core.override
  PetPixaDownloadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPixaDownloadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPixaDownloadResponse>(create);
  static PetPixaDownloadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get petId => $_getSZ(0);
  @$pb.TagNumber(1)
  set petId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPetId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPetId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get petdefId => $_getSZ(1);
  @$pb.TagNumber(2)
  set petdefId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPetdefId() => $_has(1);
  @$pb.TagNumber(2)
  void clearPetdefId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get pixaPath => $_getSZ(2);
  @$pb.TagNumber(3)
  set pixaPath($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPixaPath() => $_has(2);
  @$pb.TagNumber(3)
  void clearPixaPath() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get sizeBytes => $_getI64(3);
  @$pb.TagNumber(4)
  set sizeBytes($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSizeBytes() => $_has(3);
  @$pb.TagNumber(4)
  void clearSizeBytes() => $_clearField(4);
}

class PetPresentation extends $pb.GeneratedMessage {
  factory PetPresentation({
    $core.String? petId,
    $core.String? petdefId,
    $core.String? defaultLocale,
    PetPresentationAttrSpec? attr,
    PetPresentationDriveSpec? drive,
    PetPresentationPixaMetadata? pixaMetadata,
    PetPresentationI18nSpec? i18n,
    $core.String? pixaPath,
    $core.String? petdefUpdatedAt,
  }) {
    final result = create();
    if (petId != null) result.petId = petId;
    if (petdefId != null) result.petdefId = petdefId;
    if (defaultLocale != null) result.defaultLocale = defaultLocale;
    if (attr != null) result.attr = attr;
    if (drive != null) result.drive = drive;
    if (pixaMetadata != null) result.pixaMetadata = pixaMetadata;
    if (i18n != null) result.i18n = i18n;
    if (pixaPath != null) result.pixaPath = pixaPath;
    if (petdefUpdatedAt != null) result.petdefUpdatedAt = petdefUpdatedAt;
    return result;
  }

  PetPresentation._();

  factory PetPresentation.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentation.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentation',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'petId')
    ..aOS(2, _omitFieldNames ? '' : 'petdefId')
    ..aOS(3, _omitFieldNames ? '' : 'defaultLocale')
    ..aOM<PetPresentationAttrSpec>(4, _omitFieldNames ? '' : 'attr',
        subBuilder: PetPresentationAttrSpec.create)
    ..aOM<PetPresentationDriveSpec>(5, _omitFieldNames ? '' : 'drive',
        subBuilder: PetPresentationDriveSpec.create)
    ..aOM<PetPresentationPixaMetadata>(6, _omitFieldNames ? '' : 'pixaMetadata',
        subBuilder: PetPresentationPixaMetadata.create)
    ..aOM<PetPresentationI18nSpec>(7, _omitFieldNames ? '' : 'i18n',
        subBuilder: PetPresentationI18nSpec.create)
    ..aOS(8, _omitFieldNames ? '' : 'pixaPath')
    ..aOS(9, _omitFieldNames ? '' : 'petdefUpdatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentation clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentation copyWith(void Function(PetPresentation) updates) =>
      super.copyWith((message) => updates(message as PetPresentation))
          as PetPresentation;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentation create() => PetPresentation._();
  @$core.override
  PetPresentation createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentation getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentation>(create);
  static PetPresentation? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get petId => $_getSZ(0);
  @$pb.TagNumber(1)
  set petId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPetId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPetId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get petdefId => $_getSZ(1);
  @$pb.TagNumber(2)
  set petdefId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPetdefId() => $_has(1);
  @$pb.TagNumber(2)
  void clearPetdefId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get defaultLocale => $_getSZ(2);
  @$pb.TagNumber(3)
  set defaultLocale($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDefaultLocale() => $_has(2);
  @$pb.TagNumber(3)
  void clearDefaultLocale() => $_clearField(3);

  @$pb.TagNumber(4)
  PetPresentationAttrSpec get attr => $_getN(3);
  @$pb.TagNumber(4)
  set attr(PetPresentationAttrSpec value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasAttr() => $_has(3);
  @$pb.TagNumber(4)
  void clearAttr() => $_clearField(4);
  @$pb.TagNumber(4)
  PetPresentationAttrSpec ensureAttr() => $_ensure(3);

  @$pb.TagNumber(5)
  PetPresentationDriveSpec get drive => $_getN(4);
  @$pb.TagNumber(5)
  set drive(PetPresentationDriveSpec value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasDrive() => $_has(4);
  @$pb.TagNumber(5)
  void clearDrive() => $_clearField(5);
  @$pb.TagNumber(5)
  PetPresentationDriveSpec ensureDrive() => $_ensure(4);

  @$pb.TagNumber(6)
  PetPresentationPixaMetadata get pixaMetadata => $_getN(5);
  @$pb.TagNumber(6)
  set pixaMetadata(PetPresentationPixaMetadata value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasPixaMetadata() => $_has(5);
  @$pb.TagNumber(6)
  void clearPixaMetadata() => $_clearField(6);
  @$pb.TagNumber(6)
  PetPresentationPixaMetadata ensurePixaMetadata() => $_ensure(5);

  @$pb.TagNumber(7)
  PetPresentationI18nSpec get i18n => $_getN(6);
  @$pb.TagNumber(7)
  set i18n(PetPresentationI18nSpec value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasI18n() => $_has(6);
  @$pb.TagNumber(7)
  void clearI18n() => $_clearField(7);
  @$pb.TagNumber(7)
  PetPresentationI18nSpec ensureI18n() => $_ensure(6);

  @$pb.TagNumber(8)
  $core.String get pixaPath => $_getSZ(7);
  @$pb.TagNumber(8)
  set pixaPath($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasPixaPath() => $_has(7);
  @$pb.TagNumber(8)
  void clearPixaPath() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get petdefUpdatedAt => $_getSZ(8);
  @$pb.TagNumber(9)
  set petdefUpdatedAt($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasPetdefUpdatedAt() => $_has(8);
  @$pb.TagNumber(9)
  void clearPetdefUpdatedAt() => $_clearField(9);
}

class PetPresentationActionEffectSpec extends $pb.GeneratedMessage {
  factory PetPresentationActionEffectSpec({
    PetPresentationAttrDelta? attrDelta,
    $fixnum.Int64? petExpDelta,
  }) {
    final result = create();
    if (attrDelta != null) result.attrDelta = attrDelta;
    if (petExpDelta != null) result.petExpDelta = petExpDelta;
    return result;
  }

  PetPresentationActionEffectSpec._();

  factory PetPresentationActionEffectSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationActionEffectSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationActionEffectSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetPresentationAttrDelta>(1, _omitFieldNames ? '' : 'attrDelta',
        subBuilder: PetPresentationAttrDelta.create)
    ..aInt64(2, _omitFieldNames ? '' : 'petExpDelta')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationActionEffectSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationActionEffectSpec copyWith(
          void Function(PetPresentationActionEffectSpec) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationActionEffectSpec))
          as PetPresentationActionEffectSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationActionEffectSpec create() =>
      PetPresentationActionEffectSpec._();
  @$core.override
  PetPresentationActionEffectSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationActionEffectSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationActionEffectSpec>(
          create);
  static PetPresentationActionEffectSpec? _defaultInstance;

  @$pb.TagNumber(1)
  PetPresentationAttrDelta get attrDelta => $_getN(0);
  @$pb.TagNumber(1)
  set attrDelta(PetPresentationAttrDelta value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAttrDelta() => $_has(0);
  @$pb.TagNumber(1)
  void clearAttrDelta() => $_clearField(1);
  @$pb.TagNumber(1)
  PetPresentationAttrDelta ensureAttrDelta() => $_ensure(0);

  @$pb.TagNumber(2)
  $fixnum.Int64 get petExpDelta => $_getI64(1);
  @$pb.TagNumber(2)
  set petExpDelta($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPetExpDelta() => $_has(1);
  @$pb.TagNumber(2)
  void clearPetExpDelta() => $_clearField(2);
}

class PetPresentationActionSpec extends $pb.GeneratedMessage {
  factory PetPresentationActionSpec({
    $core.String? id,
    $fixnum.Int64? cost,
    PetPresentationActionEffectSpec? effect,
    $core.String? visualClipId,
    $core.String? icon,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (cost != null) result.cost = cost;
    if (effect != null) result.effect = effect;
    if (visualClipId != null) result.visualClipId = visualClipId;
    if (icon != null) result.icon = icon;
    return result;
  }

  PetPresentationActionSpec._();

  factory PetPresentationActionSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationActionSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationActionSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aInt64(2, _omitFieldNames ? '' : 'cost')
    ..aOM<PetPresentationActionEffectSpec>(3, _omitFieldNames ? '' : 'effect',
        subBuilder: PetPresentationActionEffectSpec.create)
    ..aOS(4, _omitFieldNames ? '' : 'visualClipId')
    ..aOS(5, _omitFieldNames ? '' : 'icon')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationActionSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationActionSpec copyWith(
          void Function(PetPresentationActionSpec) updates) =>
      super.copyWith((message) => updates(message as PetPresentationActionSpec))
          as PetPresentationActionSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationActionSpec create() => PetPresentationActionSpec._();
  @$core.override
  PetPresentationActionSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationActionSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationActionSpec>(create);
  static PetPresentationActionSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get cost => $_getI64(1);
  @$pb.TagNumber(2)
  set cost($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCost() => $_has(1);
  @$pb.TagNumber(2)
  void clearCost() => $_clearField(2);

  @$pb.TagNumber(3)
  PetPresentationActionEffectSpec get effect => $_getN(2);
  @$pb.TagNumber(3)
  set effect(PetPresentationActionEffectSpec value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasEffect() => $_has(2);
  @$pb.TagNumber(3)
  void clearEffect() => $_clearField(3);
  @$pb.TagNumber(3)
  PetPresentationActionEffectSpec ensureEffect() => $_ensure(2);

  @$pb.TagNumber(4)
  $core.String get visualClipId => $_getSZ(3);
  @$pb.TagNumber(4)
  set visualClipId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasVisualClipId() => $_has(3);
  @$pb.TagNumber(4)
  void clearVisualClipId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get icon => $_getSZ(4);
  @$pb.TagNumber(5)
  set icon($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasIcon() => $_has(4);
  @$pb.TagNumber(5)
  void clearIcon() => $_clearField(5);
}

class PetPresentationAttrDelta extends $pb.GeneratedMessage {
  factory PetPresentationAttrDelta({
    PetLife? life,
  }) {
    final result = create();
    if (life != null) result.life = life;
    return result;
  }

  PetPresentationAttrDelta._();

  factory PetPresentationAttrDelta.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationAttrDelta.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationAttrDelta',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetLife>(1, _omitFieldNames ? '' : 'life', subBuilder: PetLife.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationAttrDelta clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationAttrDelta copyWith(
          void Function(PetPresentationAttrDelta) updates) =>
      super.copyWith((message) => updates(message as PetPresentationAttrDelta))
          as PetPresentationAttrDelta;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationAttrDelta create() => PetPresentationAttrDelta._();
  @$core.override
  PetPresentationAttrDelta createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationAttrDelta getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationAttrDelta>(create);
  static PetPresentationAttrDelta? _defaultInstance;

  @$pb.TagNumber(1)
  PetLife get life => $_getN(0);
  @$pb.TagNumber(1)
  set life(PetLife value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasLife() => $_has(0);
  @$pb.TagNumber(1)
  void clearLife() => $_clearField(1);
  @$pb.TagNumber(1)
  PetLife ensureLife() => $_ensure(0);
}

class PetPresentationAttrGroupSpec extends $pb.GeneratedMessage {
  factory PetPresentationAttrGroupSpec({
    $core.Iterable<$core.MapEntry<$core.String, PetPresentationAttrValueSpec>>?
        value,
  }) {
    final result = create();
    if (value != null) result.value.addEntries(value);
    return result;
  }

  PetPresentationAttrGroupSpec._();

  factory PetPresentationAttrGroupSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationAttrGroupSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationAttrGroupSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, PetPresentationAttrValueSpec>(
        1, _omitFieldNames ? '' : 'value',
        entryClassName: 'PetPresentationAttrGroupSpec.ValueEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: PetPresentationAttrValueSpec.create,
        valueDefaultOrMaker: PetPresentationAttrValueSpec.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationAttrGroupSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationAttrGroupSpec copyWith(
          void Function(PetPresentationAttrGroupSpec) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationAttrGroupSpec))
          as PetPresentationAttrGroupSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationAttrGroupSpec create() =>
      PetPresentationAttrGroupSpec._();
  @$core.override
  PetPresentationAttrGroupSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationAttrGroupSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationAttrGroupSpec>(create);
  static PetPresentationAttrGroupSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbMap<$core.String, PetPresentationAttrValueSpec> get value =>
      $_getMap(0);
}

class PetPresentationAttrSpec extends $pb.GeneratedMessage {
  factory PetPresentationAttrSpec({
    PetPresentationAttrGroupSpec? life,
    PetPresentationAttrGroupSpec? progression,
  }) {
    final result = create();
    if (life != null) result.life = life;
    if (progression != null) result.progression = progression;
    return result;
  }

  PetPresentationAttrSpec._();

  factory PetPresentationAttrSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationAttrSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationAttrSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetPresentationAttrGroupSpec>(1, _omitFieldNames ? '' : 'life',
        subBuilder: PetPresentationAttrGroupSpec.create)
    ..aOM<PetPresentationAttrGroupSpec>(2, _omitFieldNames ? '' : 'progression',
        subBuilder: PetPresentationAttrGroupSpec.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationAttrSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationAttrSpec copyWith(
          void Function(PetPresentationAttrSpec) updates) =>
      super.copyWith((message) => updates(message as PetPresentationAttrSpec))
          as PetPresentationAttrSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationAttrSpec create() => PetPresentationAttrSpec._();
  @$core.override
  PetPresentationAttrSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationAttrSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationAttrSpec>(create);
  static PetPresentationAttrSpec? _defaultInstance;

  @$pb.TagNumber(1)
  PetPresentationAttrGroupSpec get life => $_getN(0);
  @$pb.TagNumber(1)
  set life(PetPresentationAttrGroupSpec value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasLife() => $_has(0);
  @$pb.TagNumber(1)
  void clearLife() => $_clearField(1);
  @$pb.TagNumber(1)
  PetPresentationAttrGroupSpec ensureLife() => $_ensure(0);

  @$pb.TagNumber(2)
  PetPresentationAttrGroupSpec get progression => $_getN(1);
  @$pb.TagNumber(2)
  set progression(PetPresentationAttrGroupSpec value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasProgression() => $_has(1);
  @$pb.TagNumber(2)
  void clearProgression() => $_clearField(2);
  @$pb.TagNumber(2)
  PetPresentationAttrGroupSpec ensureProgression() => $_ensure(1);
}

class PetPresentationAttrValueSpec extends $pb.GeneratedMessage {
  factory PetPresentationAttrValueSpec({
    $fixnum.Int64? initial,
  }) {
    final result = create();
    if (initial != null) result.initial = initial;
    return result;
  }

  PetPresentationAttrValueSpec._();

  factory PetPresentationAttrValueSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationAttrValueSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationAttrValueSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'initial')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationAttrValueSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationAttrValueSpec copyWith(
          void Function(PetPresentationAttrValueSpec) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationAttrValueSpec))
          as PetPresentationAttrValueSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationAttrValueSpec create() =>
      PetPresentationAttrValueSpec._();
  @$core.override
  PetPresentationAttrValueSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationAttrValueSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationAttrValueSpec>(create);
  static PetPresentationAttrValueSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get initial => $_getI64(0);
  @$pb.TagNumber(1)
  set initial($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasInitial() => $_has(0);
  @$pb.TagNumber(1)
  void clearInitial() => $_clearField(1);
}

class PetPresentationDriveSpec extends $pb.GeneratedMessage {
  factory PetPresentationDriveSpec({
    $core.Iterable<PetPresentationActionSpec>? actions,
  }) {
    final result = create();
    if (actions != null) result.actions.addAll(actions);
    return result;
  }

  PetPresentationDriveSpec._();

  factory PetPresentationDriveSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationDriveSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationDriveSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<PetPresentationActionSpec>(1, _omitFieldNames ? '' : 'actions',
        subBuilder: PetPresentationActionSpec.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationDriveSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationDriveSpec copyWith(
          void Function(PetPresentationDriveSpec) updates) =>
      super.copyWith((message) => updates(message as PetPresentationDriveSpec))
          as PetPresentationDriveSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationDriveSpec create() => PetPresentationDriveSpec._();
  @$core.override
  PetPresentationDriveSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationDriveSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationDriveSpec>(create);
  static PetPresentationDriveSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<PetPresentationActionSpec> get actions => $_getList(0);
}

class PetPresentationI18nAttrGroup extends $pb.GeneratedMessage {
  factory PetPresentationI18nAttrGroup({
    $core
        .Iterable<$core.MapEntry<$core.String, PetPresentationI18nDisplayText>>?
        value,
  }) {
    final result = create();
    if (value != null) result.value.addEntries(value);
    return result;
  }

  PetPresentationI18nAttrGroup._();

  factory PetPresentationI18nAttrGroup.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationI18nAttrGroup.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationI18nAttrGroup',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, PetPresentationI18nDisplayText>(
        1, _omitFieldNames ? '' : 'value',
        entryClassName: 'PetPresentationI18nAttrGroup.ValueEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: PetPresentationI18nDisplayText.create,
        valueDefaultOrMaker: PetPresentationI18nDisplayText.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nAttrGroup clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nAttrGroup copyWith(
          void Function(PetPresentationI18nAttrGroup) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationI18nAttrGroup))
          as PetPresentationI18nAttrGroup;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nAttrGroup create() =>
      PetPresentationI18nAttrGroup._();
  @$core.override
  PetPresentationI18nAttrGroup createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nAttrGroup getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationI18nAttrGroup>(create);
  static PetPresentationI18nAttrGroup? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbMap<$core.String, PetPresentationI18nDisplayText> get value =>
      $_getMap(0);
}

class PetPresentationI18nAttrSpec extends $pb.GeneratedMessage {
  factory PetPresentationI18nAttrSpec({
    PetPresentationI18nAttrGroup? life,
    PetPresentationI18nAttrGroup? progression,
  }) {
    final result = create();
    if (life != null) result.life = life;
    if (progression != null) result.progression = progression;
    return result;
  }

  PetPresentationI18nAttrSpec._();

  factory PetPresentationI18nAttrSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationI18nAttrSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationI18nAttrSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetPresentationI18nAttrGroup>(1, _omitFieldNames ? '' : 'life',
        subBuilder: PetPresentationI18nAttrGroup.create)
    ..aOM<PetPresentationI18nAttrGroup>(2, _omitFieldNames ? '' : 'progression',
        subBuilder: PetPresentationI18nAttrGroup.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nAttrSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nAttrSpec copyWith(
          void Function(PetPresentationI18nAttrSpec) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationI18nAttrSpec))
          as PetPresentationI18nAttrSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nAttrSpec create() =>
      PetPresentationI18nAttrSpec._();
  @$core.override
  PetPresentationI18nAttrSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nAttrSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationI18nAttrSpec>(create);
  static PetPresentationI18nAttrSpec? _defaultInstance;

  @$pb.TagNumber(1)
  PetPresentationI18nAttrGroup get life => $_getN(0);
  @$pb.TagNumber(1)
  set life(PetPresentationI18nAttrGroup value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasLife() => $_has(0);
  @$pb.TagNumber(1)
  void clearLife() => $_clearField(1);
  @$pb.TagNumber(1)
  PetPresentationI18nAttrGroup ensureLife() => $_ensure(0);

  @$pb.TagNumber(2)
  PetPresentationI18nAttrGroup get progression => $_getN(1);
  @$pb.TagNumber(2)
  set progression(PetPresentationI18nAttrGroup value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasProgression() => $_has(1);
  @$pb.TagNumber(2)
  void clearProgression() => $_clearField(2);
  @$pb.TagNumber(2)
  PetPresentationI18nAttrGroup ensureProgression() => $_ensure(1);
}

class PetPresentationI18nCatalog extends $pb.GeneratedMessage {
  factory PetPresentationI18nCatalog({
    $core.String? displayName,
    $core.String? description,
    PetPresentationI18nAttrSpec? attr,
    PetPresentationI18nDriveSpec? drive,
  }) {
    final result = create();
    if (displayName != null) result.displayName = displayName;
    if (description != null) result.description = description;
    if (attr != null) result.attr = attr;
    if (drive != null) result.drive = drive;
    return result;
  }

  PetPresentationI18nCatalog._();

  factory PetPresentationI18nCatalog.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationI18nCatalog.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationI18nCatalog',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..aOS(2, _omitFieldNames ? '' : 'description')
    ..aOM<PetPresentationI18nAttrSpec>(3, _omitFieldNames ? '' : 'attr',
        subBuilder: PetPresentationI18nAttrSpec.create)
    ..aOM<PetPresentationI18nDriveSpec>(4, _omitFieldNames ? '' : 'drive',
        subBuilder: PetPresentationI18nDriveSpec.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nCatalog clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nCatalog copyWith(
          void Function(PetPresentationI18nCatalog) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationI18nCatalog))
          as PetPresentationI18nCatalog;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nCatalog create() => PetPresentationI18nCatalog._();
  @$core.override
  PetPresentationI18nCatalog createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nCatalog getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationI18nCatalog>(create);
  static PetPresentationI18nCatalog? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get description => $_getSZ(1);
  @$pb.TagNumber(2)
  set description($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDescription() => $_has(1);
  @$pb.TagNumber(2)
  void clearDescription() => $_clearField(2);

  @$pb.TagNumber(3)
  PetPresentationI18nAttrSpec get attr => $_getN(2);
  @$pb.TagNumber(3)
  set attr(PetPresentationI18nAttrSpec value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasAttr() => $_has(2);
  @$pb.TagNumber(3)
  void clearAttr() => $_clearField(3);
  @$pb.TagNumber(3)
  PetPresentationI18nAttrSpec ensureAttr() => $_ensure(2);

  @$pb.TagNumber(4)
  PetPresentationI18nDriveSpec get drive => $_getN(3);
  @$pb.TagNumber(4)
  set drive(PetPresentationI18nDriveSpec value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasDrive() => $_has(3);
  @$pb.TagNumber(4)
  void clearDrive() => $_clearField(4);
  @$pb.TagNumber(4)
  PetPresentationI18nDriveSpec ensureDrive() => $_ensure(3);
}

class PetPresentationI18nDisplayText extends $pb.GeneratedMessage {
  factory PetPresentationI18nDisplayText({
    $core.String? displayName,
  }) {
    final result = create();
    if (displayName != null) result.displayName = displayName;
    return result;
  }

  PetPresentationI18nDisplayText._();

  factory PetPresentationI18nDisplayText.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationI18nDisplayText.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationI18nDisplayText',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nDisplayText clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nDisplayText copyWith(
          void Function(PetPresentationI18nDisplayText) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationI18nDisplayText))
          as PetPresentationI18nDisplayText;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nDisplayText create() =>
      PetPresentationI18nDisplayText._();
  @$core.override
  PetPresentationI18nDisplayText createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nDisplayText getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationI18nDisplayText>(create);
  static PetPresentationI18nDisplayText? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);
}

class PetPresentationI18nDriveSpec extends $pb.GeneratedMessage {
  factory PetPresentationI18nDriveSpec({
    $core
        .Iterable<$core.MapEntry<$core.String, PetPresentationI18nDisplayText>>?
        actions,
  }) {
    final result = create();
    if (actions != null) result.actions.addEntries(actions);
    return result;
  }

  PetPresentationI18nDriveSpec._();

  factory PetPresentationI18nDriveSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationI18nDriveSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationI18nDriveSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, PetPresentationI18nDisplayText>(
        1, _omitFieldNames ? '' : 'actions',
        entryClassName: 'PetPresentationI18nDriveSpec.ActionsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: PetPresentationI18nDisplayText.create,
        valueDefaultOrMaker: PetPresentationI18nDisplayText.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nDriveSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nDriveSpec copyWith(
          void Function(PetPresentationI18nDriveSpec) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationI18nDriveSpec))
          as PetPresentationI18nDriveSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nDriveSpec create() =>
      PetPresentationI18nDriveSpec._();
  @$core.override
  PetPresentationI18nDriveSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nDriveSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationI18nDriveSpec>(create);
  static PetPresentationI18nDriveSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbMap<$core.String, PetPresentationI18nDisplayText> get actions =>
      $_getMap(0);
}

class PetPresentationI18nSpec extends $pb.GeneratedMessage {
  factory PetPresentationI18nSpec({
    $core.Iterable<$core.MapEntry<$core.String, PetPresentationI18nCatalog>>?
        value,
  }) {
    final result = create();
    if (value != null) result.value.addEntries(value);
    return result;
  }

  PetPresentationI18nSpec._();

  factory PetPresentationI18nSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationI18nSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationI18nSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, PetPresentationI18nCatalog>(
        1, _omitFieldNames ? '' : 'value',
        entryClassName: 'PetPresentationI18nSpec.ValueEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: PetPresentationI18nCatalog.create,
        valueDefaultOrMaker: PetPresentationI18nCatalog.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationI18nSpec copyWith(
          void Function(PetPresentationI18nSpec) updates) =>
      super.copyWith((message) => updates(message as PetPresentationI18nSpec))
          as PetPresentationI18nSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nSpec create() => PetPresentationI18nSpec._();
  @$core.override
  PetPresentationI18nSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationI18nSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationI18nSpec>(create);
  static PetPresentationI18nSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbMap<$core.String, PetPresentationI18nCatalog> get value => $_getMap(0);
}

class PetPresentationPixaCanvasMetadata extends $pb.GeneratedMessage {
  factory PetPresentationPixaCanvasMetadata({
    $fixnum.Int64? width,
    $fixnum.Int64? height,
  }) {
    final result = create();
    if (width != null) result.width = width;
    if (height != null) result.height = height;
    return result;
  }

  PetPresentationPixaCanvasMetadata._();

  factory PetPresentationPixaCanvasMetadata.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationPixaCanvasMetadata.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationPixaCanvasMetadata',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'width')
    ..aInt64(2, _omitFieldNames ? '' : 'height')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationPixaCanvasMetadata clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationPixaCanvasMetadata copyWith(
          void Function(PetPresentationPixaCanvasMetadata) updates) =>
      super.copyWith((message) =>
              updates(message as PetPresentationPixaCanvasMetadata))
          as PetPresentationPixaCanvasMetadata;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationPixaCanvasMetadata create() =>
      PetPresentationPixaCanvasMetadata._();
  @$core.override
  PetPresentationPixaCanvasMetadata createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationPixaCanvasMetadata getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationPixaCanvasMetadata>(
          create);
  static PetPresentationPixaCanvasMetadata? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get width => $_getI64(0);
  @$pb.TagNumber(1)
  set width($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasWidth() => $_has(0);
  @$pb.TagNumber(1)
  void clearWidth() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get height => $_getI64(1);
  @$pb.TagNumber(2)
  set height($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasHeight() => $_has(1);
  @$pb.TagNumber(2)
  void clearHeight() => $_clearField(2);
}

class PetPresentationPixaClipMetadata extends $pb.GeneratedMessage {
  factory PetPresentationPixaClipMetadata({
    $core.String? id,
    $core.String? actionId,
    $core.String? pixaClipName,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (actionId != null) result.actionId = actionId;
    if (pixaClipName != null) result.pixaClipName = pixaClipName;
    return result;
  }

  PetPresentationPixaClipMetadata._();

  factory PetPresentationPixaClipMetadata.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationPixaClipMetadata.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationPixaClipMetadata',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'actionId')
    ..aOS(3, _omitFieldNames ? '' : 'pixaClipName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationPixaClipMetadata clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationPixaClipMetadata copyWith(
          void Function(PetPresentationPixaClipMetadata) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationPixaClipMetadata))
          as PetPresentationPixaClipMetadata;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationPixaClipMetadata create() =>
      PetPresentationPixaClipMetadata._();
  @$core.override
  PetPresentationPixaClipMetadata createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationPixaClipMetadata getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationPixaClipMetadata>(
          create);
  static PetPresentationPixaClipMetadata? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get actionId => $_getSZ(1);
  @$pb.TagNumber(2)
  set actionId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasActionId() => $_has(1);
  @$pb.TagNumber(2)
  void clearActionId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get pixaClipName => $_getSZ(2);
  @$pb.TagNumber(3)
  set pixaClipName($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPixaClipName() => $_has(2);
  @$pb.TagNumber(3)
  void clearPixaClipName() => $_clearField(3);
}

class PetPresentationPixaMetadata extends $pb.GeneratedMessage {
  factory PetPresentationPixaMetadata({
    $core.String? version,
    PetPresentationPixaCanvasMetadata? canvas,
    $core.Iterable<PetPresentationPixaClipMetadata>? clips,
  }) {
    final result = create();
    if (version != null) result.version = version;
    if (canvas != null) result.canvas = canvas;
    if (clips != null) result.clips.addAll(clips);
    return result;
  }

  PetPresentationPixaMetadata._();

  factory PetPresentationPixaMetadata.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPresentationPixaMetadata.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPresentationPixaMetadata',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'version')
    ..aOM<PetPresentationPixaCanvasMetadata>(2, _omitFieldNames ? '' : 'canvas',
        subBuilder: PetPresentationPixaCanvasMetadata.create)
    ..pPM<PetPresentationPixaClipMetadata>(3, _omitFieldNames ? '' : 'clips',
        subBuilder: PetPresentationPixaClipMetadata.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationPixaMetadata clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPresentationPixaMetadata copyWith(
          void Function(PetPresentationPixaMetadata) updates) =>
      super.copyWith(
              (message) => updates(message as PetPresentationPixaMetadata))
          as PetPresentationPixaMetadata;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPresentationPixaMetadata create() =>
      PetPresentationPixaMetadata._();
  @$core.override
  PetPresentationPixaMetadata createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPresentationPixaMetadata getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPresentationPixaMetadata>(create);
  static PetPresentationPixaMetadata? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get version => $_getSZ(0);
  @$pb.TagNumber(1)
  set version($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasVersion() => $_has(0);
  @$pb.TagNumber(1)
  void clearVersion() => $_clearField(1);

  @$pb.TagNumber(2)
  PetPresentationPixaCanvasMetadata get canvas => $_getN(1);
  @$pb.TagNumber(2)
  set canvas(PetPresentationPixaCanvasMetadata value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasCanvas() => $_has(1);
  @$pb.TagNumber(2)
  void clearCanvas() => $_clearField(2);
  @$pb.TagNumber(2)
  PetPresentationPixaCanvasMetadata ensureCanvas() => $_ensure(1);

  @$pb.TagNumber(3)
  $pb.PbList<PetPresentationPixaClipMetadata> get clips => $_getList(2);
}

class PetDeleteRequest extends $pb.GeneratedMessage {
  factory PetDeleteRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  PetDeleteRequest._();

  factory PetDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDeleteRequest copyWith(void Function(PetDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as PetDeleteRequest))
          as PetDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetDeleteRequest create() => PetDeleteRequest._();
  @$core.override
  PetDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetDeleteRequest>(create);
  static PetDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class PetDriveGameResultInput extends $pb.GeneratedMessage {
  factory PetDriveGameResultInput({
    $core.String? difficulty,
    $fixnum.Int64? durationMs,
    $core.String? gameDefId,
    $core.String? idempotencyKey,
    $fixnum.Int64? maxScore,
    $core.String? occurredAt,
    $core.String? outcome,
    GameplayMetadata? payload,
    $fixnum.Int64? score,
  }) {
    final result = create();
    if (difficulty != null) result.difficulty = difficulty;
    if (durationMs != null) result.durationMs = durationMs;
    if (gameDefId != null) result.gameDefId = gameDefId;
    if (idempotencyKey != null) result.idempotencyKey = idempotencyKey;
    if (maxScore != null) result.maxScore = maxScore;
    if (occurredAt != null) result.occurredAt = occurredAt;
    if (outcome != null) result.outcome = outcome;
    if (payload != null) result.payload = payload;
    if (score != null) result.score = score;
    return result;
  }

  PetDriveGameResultInput._();

  factory PetDriveGameResultInput.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetDriveGameResultInput.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetDriveGameResultInput',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'difficulty')
    ..aInt64(2, _omitFieldNames ? '' : 'durationMs')
    ..aOS(3, _omitFieldNames ? '' : 'gameDefId')
    ..aOS(4, _omitFieldNames ? '' : 'idempotencyKey')
    ..aInt64(5, _omitFieldNames ? '' : 'maxScore')
    ..aOS(6, _omitFieldNames ? '' : 'occurredAt')
    ..aOS(7, _omitFieldNames ? '' : 'outcome')
    ..aOM<GameplayMetadata>(8, _omitFieldNames ? '' : 'payload',
        subBuilder: GameplayMetadata.create)
    ..aInt64(9, _omitFieldNames ? '' : 'score')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDriveGameResultInput clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDriveGameResultInput copyWith(
          void Function(PetDriveGameResultInput) updates) =>
      super.copyWith((message) => updates(message as PetDriveGameResultInput))
          as PetDriveGameResultInput;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetDriveGameResultInput create() => PetDriveGameResultInput._();
  @$core.override
  PetDriveGameResultInput createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetDriveGameResultInput getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetDriveGameResultInput>(create);
  static PetDriveGameResultInput? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get difficulty => $_getSZ(0);
  @$pb.TagNumber(1)
  set difficulty($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDifficulty() => $_has(0);
  @$pb.TagNumber(1)
  void clearDifficulty() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get durationMs => $_getI64(1);
  @$pb.TagNumber(2)
  set durationMs($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDurationMs() => $_has(1);
  @$pb.TagNumber(2)
  void clearDurationMs() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get gameDefId => $_getSZ(2);
  @$pb.TagNumber(3)
  set gameDefId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasGameDefId() => $_has(2);
  @$pb.TagNumber(3)
  void clearGameDefId() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get idempotencyKey => $_getSZ(3);
  @$pb.TagNumber(4)
  set idempotencyKey($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasIdempotencyKey() => $_has(3);
  @$pb.TagNumber(4)
  void clearIdempotencyKey() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get maxScore => $_getI64(4);
  @$pb.TagNumber(5)
  set maxScore($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasMaxScore() => $_has(4);
  @$pb.TagNumber(5)
  void clearMaxScore() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get occurredAt => $_getSZ(5);
  @$pb.TagNumber(6)
  set occurredAt($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasOccurredAt() => $_has(5);
  @$pb.TagNumber(6)
  void clearOccurredAt() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get outcome => $_getSZ(6);
  @$pb.TagNumber(7)
  set outcome($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasOutcome() => $_has(6);
  @$pb.TagNumber(7)
  void clearOutcome() => $_clearField(7);

  @$pb.TagNumber(8)
  GameplayMetadata get payload => $_getN(7);
  @$pb.TagNumber(8)
  set payload(GameplayMetadata value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasPayload() => $_has(7);
  @$pb.TagNumber(8)
  void clearPayload() => $_clearField(8);
  @$pb.TagNumber(8)
  GameplayMetadata ensurePayload() => $_ensure(7);

  @$pb.TagNumber(9)
  $fixnum.Int64 get score => $_getI64(8);
  @$pb.TagNumber(9)
  set score($fixnum.Int64 value) => $_setInt64(8, value);
  @$pb.TagNumber(9)
  $core.bool hasScore() => $_has(8);
  @$pb.TagNumber(9)
  void clearScore() => $_clearField(9);
}

class PetDriveRequest extends $pb.GeneratedMessage {
  factory PetDriveRequest({
    $core.String? action,
    PetDriveGameResultInput? gameResult,
    $core.String? petId,
  }) {
    final result = create();
    if (action != null) result.action = action;
    if (gameResult != null) result.gameResult = gameResult;
    if (petId != null) result.petId = petId;
    return result;
  }

  PetDriveRequest._();

  factory PetDriveRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetDriveRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetDriveRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'action')
    ..aOM<PetDriveGameResultInput>(2, _omitFieldNames ? '' : 'gameResult',
        subBuilder: PetDriveGameResultInput.create)
    ..aOS(3, _omitFieldNames ? '' : 'petId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDriveRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDriveRequest copyWith(void Function(PetDriveRequest) updates) =>
      super.copyWith((message) => updates(message as PetDriveRequest))
          as PetDriveRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetDriveRequest create() => PetDriveRequest._();
  @$core.override
  PetDriveRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetDriveRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetDriveRequest>(create);
  static PetDriveRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get action => $_getSZ(0);
  @$pb.TagNumber(1)
  set action($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAction() => $_has(0);
  @$pb.TagNumber(1)
  void clearAction() => $_clearField(1);

  @$pb.TagNumber(2)
  PetDriveGameResultInput get gameResult => $_getN(1);
  @$pb.TagNumber(2)
  set gameResult(PetDriveGameResultInput value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasGameResult() => $_has(1);
  @$pb.TagNumber(2)
  void clearGameResult() => $_clearField(2);
  @$pb.TagNumber(2)
  PetDriveGameResultInput ensureGameResult() => $_ensure(1);

  @$pb.TagNumber(3)
  $core.String get petId => $_getSZ(2);
  @$pb.TagNumber(3)
  set petId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPetId() => $_has(2);
  @$pb.TagNumber(3)
  void clearPetId() => $_clearField(3);
}

class PetDriveResponse extends $pb.GeneratedMessage {
  factory PetDriveResponse({
    $core.Iterable<Badge>? badges,
    GameResult? gameResult,
    Pet? pet,
    PointsAccount? points,
    $core.Iterable<RewardGrant>? rewardGrants,
    $core.Iterable<PointsTransaction>? transactions,
  }) {
    final result = create();
    if (badges != null) result.badges.addAll(badges);
    if (gameResult != null) result.gameResult = gameResult;
    if (pet != null) result.pet = pet;
    if (points != null) result.points = points;
    if (rewardGrants != null) result.rewardGrants.addAll(rewardGrants);
    if (transactions != null) result.transactions.addAll(transactions);
    return result;
  }

  PetDriveResponse._();

  factory PetDriveResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetDriveResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetDriveResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<Badge>(1, _omitFieldNames ? '' : 'badges', subBuilder: Badge.create)
    ..aOM<GameResult>(2, _omitFieldNames ? '' : 'gameResult',
        subBuilder: GameResult.create)
    ..aOM<Pet>(3, _omitFieldNames ? '' : 'pet', subBuilder: Pet.create)
    ..aOM<PointsAccount>(4, _omitFieldNames ? '' : 'points',
        subBuilder: PointsAccount.create)
    ..pPM<RewardGrant>(5, _omitFieldNames ? '' : 'rewardGrants',
        subBuilder: RewardGrant.create)
    ..pPM<PointsTransaction>(6, _omitFieldNames ? '' : 'transactions',
        subBuilder: PointsTransaction.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDriveResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetDriveResponse copyWith(void Function(PetDriveResponse) updates) =>
      super.copyWith((message) => updates(message as PetDriveResponse))
          as PetDriveResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetDriveResponse create() => PetDriveResponse._();
  @$core.override
  PetDriveResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetDriveResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetDriveResponse>(create);
  static PetDriveResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<Badge> get badges => $_getList(0);

  @$pb.TagNumber(2)
  GameResult get gameResult => $_getN(1);
  @$pb.TagNumber(2)
  set gameResult(GameResult value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasGameResult() => $_has(1);
  @$pb.TagNumber(2)
  void clearGameResult() => $_clearField(2);
  @$pb.TagNumber(2)
  GameResult ensureGameResult() => $_ensure(1);

  @$pb.TagNumber(3)
  Pet get pet => $_getN(2);
  @$pb.TagNumber(3)
  set pet(Pet value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasPet() => $_has(2);
  @$pb.TagNumber(3)
  void clearPet() => $_clearField(3);
  @$pb.TagNumber(3)
  Pet ensurePet() => $_ensure(2);

  @$pb.TagNumber(4)
  PointsAccount get points => $_getN(3);
  @$pb.TagNumber(4)
  set points(PointsAccount value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasPoints() => $_has(3);
  @$pb.TagNumber(4)
  void clearPoints() => $_clearField(4);
  @$pb.TagNumber(4)
  PointsAccount ensurePoints() => $_ensure(3);

  @$pb.TagNumber(5)
  $pb.PbList<RewardGrant> get rewardGrants => $_getList(4);

  @$pb.TagNumber(6)
  $pb.PbList<PointsTransaction> get transactions => $_getList(5);
}

class PetGetRequest extends $pb.GeneratedMessage {
  factory PetGetRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  PetGetRequest._();

  factory PetGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetGetRequest copyWith(void Function(PetGetRequest) updates) =>
      super.copyWith((message) => updates(message as PetGetRequest))
          as PetGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetGetRequest create() => PetGetRequest._();
  @$core.override
  PetGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetGetRequest>(create);
  static PetGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class PetListResponse extends $pb.GeneratedMessage {
  factory PetListResponse({
    $core.bool? hasNext,
    $core.Iterable<Pet>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  PetListResponse._();

  factory PetListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<Pet>(2, _omitFieldNames ? '' : 'items', subBuilder: Pet.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetListResponse copyWith(void Function(PetListResponse) updates) =>
      super.copyWith((message) => updates(message as PetListResponse))
          as PetListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetListResponse create() => PetListResponse._();
  @$core.override
  PetListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetListResponse>(create);
  static PetListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<Pet> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class PetPutRequest extends $pb.GeneratedMessage {
  factory PetPutRequest({
    $core.String? displayName,
    $core.String? id,
  }) {
    final result = create();
    if (displayName != null) result.displayName = displayName;
    if (id != null) result.id = id;
    return result;
  }

  PetPutRequest._();

  factory PetPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetPutRequest copyWith(void Function(PetPutRequest) updates) =>
      super.copyWith((message) => updates(message as PetPutRequest))
          as PetPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetPutRequest create() => PetPutRequest._();
  @$core.override
  PetPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetPutRequest>(create);
  static PetPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);
}

class PointsAccount extends $pb.GeneratedMessage {
  factory PointsAccount({
    $fixnum.Int64? balance,
    $core.String? createdAt,
    $core.String? ownerPublicKey,
    $core.String? rulesetName,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (balance != null) result.balance = balance;
    if (createdAt != null) result.createdAt = createdAt;
    if (ownerPublicKey != null) result.ownerPublicKey = ownerPublicKey;
    if (rulesetName != null) result.rulesetName = rulesetName;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  PointsAccount._();

  factory PointsAccount.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PointsAccount.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PointsAccount',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'balance')
    ..aOS(2, _omitFieldNames ? '' : 'createdAt')
    ..aOS(3, _omitFieldNames ? '' : 'ownerPublicKey')
    ..aOS(4, _omitFieldNames ? '' : 'rulesetName')
    ..aOS(5, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PointsAccount clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PointsAccount copyWith(void Function(PointsAccount) updates) =>
      super.copyWith((message) => updates(message as PointsAccount))
          as PointsAccount;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PointsAccount create() => PointsAccount._();
  @$core.override
  PointsAccount createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PointsAccount getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PointsAccount>(create);
  static PointsAccount? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get balance => $_getI64(0);
  @$pb.TagNumber(1)
  set balance($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasBalance() => $_has(0);
  @$pb.TagNumber(1)
  void clearBalance() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get createdAt => $_getSZ(1);
  @$pb.TagNumber(2)
  set createdAt($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCreatedAt() => $_has(1);
  @$pb.TagNumber(2)
  void clearCreatedAt() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get ownerPublicKey => $_getSZ(2);
  @$pb.TagNumber(3)
  set ownerPublicKey($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasOwnerPublicKey() => $_has(2);
  @$pb.TagNumber(3)
  void clearOwnerPublicKey() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get rulesetName => $_getSZ(3);
  @$pb.TagNumber(4)
  set rulesetName($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRulesetName() => $_has(3);
  @$pb.TagNumber(4)
  void clearRulesetName() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get updatedAt => $_getSZ(4);
  @$pb.TagNumber(5)
  set updatedAt($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasUpdatedAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearUpdatedAt() => $_clearField(5);
}

class PointsTransaction extends $pb.GeneratedMessage {
  factory PointsTransaction({
    $fixnum.Int64? balanceAfter,
    $core.String? createdAt,
    $fixnum.Int64? delta,
    $core.String? gameResultId,
    $core.String? id,
    $core.String? ownerPublicKey,
    $core.String? petId,
    $core.String? reason,
    $core.String? rewardGrantId,
    $core.String? rulesetName,
    $core.String? sourceId,
    $core.String? sourceType,
  }) {
    final result = create();
    if (balanceAfter != null) result.balanceAfter = balanceAfter;
    if (createdAt != null) result.createdAt = createdAt;
    if (delta != null) result.delta = delta;
    if (gameResultId != null) result.gameResultId = gameResultId;
    if (id != null) result.id = id;
    if (ownerPublicKey != null) result.ownerPublicKey = ownerPublicKey;
    if (petId != null) result.petId = petId;
    if (reason != null) result.reason = reason;
    if (rewardGrantId != null) result.rewardGrantId = rewardGrantId;
    if (rulesetName != null) result.rulesetName = rulesetName;
    if (sourceId != null) result.sourceId = sourceId;
    if (sourceType != null) result.sourceType = sourceType;
    return result;
  }

  PointsTransaction._();

  factory PointsTransaction.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PointsTransaction.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PointsTransaction',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'balanceAfter')
    ..aOS(2, _omitFieldNames ? '' : 'createdAt')
    ..aInt64(3, _omitFieldNames ? '' : 'delta')
    ..aOS(4, _omitFieldNames ? '' : 'gameResultId')
    ..aOS(5, _omitFieldNames ? '' : 'id')
    ..aOS(6, _omitFieldNames ? '' : 'ownerPublicKey')
    ..aOS(7, _omitFieldNames ? '' : 'petId')
    ..aOS(8, _omitFieldNames ? '' : 'reason')
    ..aOS(9, _omitFieldNames ? '' : 'rewardGrantId')
    ..aOS(10, _omitFieldNames ? '' : 'rulesetName')
    ..aOS(11, _omitFieldNames ? '' : 'sourceId')
    ..aOS(12, _omitFieldNames ? '' : 'sourceType')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PointsTransaction clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PointsTransaction copyWith(void Function(PointsTransaction) updates) =>
      super.copyWith((message) => updates(message as PointsTransaction))
          as PointsTransaction;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PointsTransaction create() => PointsTransaction._();
  @$core.override
  PointsTransaction createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PointsTransaction getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PointsTransaction>(create);
  static PointsTransaction? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get balanceAfter => $_getI64(0);
  @$pb.TagNumber(1)
  set balanceAfter($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasBalanceAfter() => $_has(0);
  @$pb.TagNumber(1)
  void clearBalanceAfter() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get createdAt => $_getSZ(1);
  @$pb.TagNumber(2)
  set createdAt($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCreatedAt() => $_has(1);
  @$pb.TagNumber(2)
  void clearCreatedAt() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get delta => $_getI64(2);
  @$pb.TagNumber(3)
  set delta($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDelta() => $_has(2);
  @$pb.TagNumber(3)
  void clearDelta() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get gameResultId => $_getSZ(3);
  @$pb.TagNumber(4)
  set gameResultId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasGameResultId() => $_has(3);
  @$pb.TagNumber(4)
  void clearGameResultId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get id => $_getSZ(4);
  @$pb.TagNumber(5)
  set id($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasId() => $_has(4);
  @$pb.TagNumber(5)
  void clearId() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get ownerPublicKey => $_getSZ(5);
  @$pb.TagNumber(6)
  set ownerPublicKey($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasOwnerPublicKey() => $_has(5);
  @$pb.TagNumber(6)
  void clearOwnerPublicKey() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get petId => $_getSZ(6);
  @$pb.TagNumber(7)
  set petId($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasPetId() => $_has(6);
  @$pb.TagNumber(7)
  void clearPetId() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get reason => $_getSZ(7);
  @$pb.TagNumber(8)
  set reason($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasReason() => $_has(7);
  @$pb.TagNumber(8)
  void clearReason() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get rewardGrantId => $_getSZ(8);
  @$pb.TagNumber(9)
  set rewardGrantId($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasRewardGrantId() => $_has(8);
  @$pb.TagNumber(9)
  void clearRewardGrantId() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get rulesetName => $_getSZ(9);
  @$pb.TagNumber(10)
  set rulesetName($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasRulesetName() => $_has(9);
  @$pb.TagNumber(10)
  void clearRulesetName() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.String get sourceId => $_getSZ(10);
  @$pb.TagNumber(11)
  set sourceId($core.String value) => $_setString(10, value);
  @$pb.TagNumber(11)
  $core.bool hasSourceId() => $_has(10);
  @$pb.TagNumber(11)
  void clearSourceId() => $_clearField(11);

  @$pb.TagNumber(12)
  $core.String get sourceType => $_getSZ(11);
  @$pb.TagNumber(12)
  set sourceType($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasSourceType() => $_has(11);
  @$pb.TagNumber(12)
  void clearSourceType() => $_clearField(12);
}

class PointsTransactionListResponse extends $pb.GeneratedMessage {
  factory PointsTransactionListResponse({
    $core.bool? hasNext,
    $core.Iterable<PointsTransaction>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  PointsTransactionListResponse._();

  factory PointsTransactionListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PointsTransactionListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PointsTransactionListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<PointsTransaction>(2, _omitFieldNames ? '' : 'items',
        subBuilder: PointsTransaction.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PointsTransactionListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PointsTransactionListResponse copyWith(
          void Function(PointsTransactionListResponse) updates) =>
      super.copyWith(
              (message) => updates(message as PointsTransactionListResponse))
          as PointsTransactionListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PointsTransactionListResponse create() =>
      PointsTransactionListResponse._();
  @$core.override
  PointsTransactionListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PointsTransactionListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PointsTransactionListResponse>(create);
  static PointsTransactionListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<PointsTransaction> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class RewardGrant extends $pb.GeneratedMessage {
  factory RewardGrant({
    $core.Iterable<$core.MapEntry<$core.String, $fixnum.Int64>>? badgeExpDelta,
    $core.String? createdAt,
    $core.String? gameResultId,
    $core.String? id,
    $core.String? ownerPublicKey,
    $fixnum.Int64? petExpDelta,
    $core.String? petId,
    $fixnum.Int64? pointsDelta,
    $core.String? reason,
    $core.String? rulesetName,
    $core.String? sourceId,
    $core.String? sourceType,
  }) {
    final result = create();
    if (badgeExpDelta != null) result.badgeExpDelta.addEntries(badgeExpDelta);
    if (createdAt != null) result.createdAt = createdAt;
    if (gameResultId != null) result.gameResultId = gameResultId;
    if (id != null) result.id = id;
    if (ownerPublicKey != null) result.ownerPublicKey = ownerPublicKey;
    if (petExpDelta != null) result.petExpDelta = petExpDelta;
    if (petId != null) result.petId = petId;
    if (pointsDelta != null) result.pointsDelta = pointsDelta;
    if (reason != null) result.reason = reason;
    if (rulesetName != null) result.rulesetName = rulesetName;
    if (sourceId != null) result.sourceId = sourceId;
    if (sourceType != null) result.sourceType = sourceType;
    return result;
  }

  RewardGrant._();

  factory RewardGrant.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RewardGrant.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RewardGrant',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, $fixnum.Int64>(2, _omitFieldNames ? '' : 'badgeExpDelta',
        entryClassName: 'RewardGrant.BadgeExpDeltaEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.O6,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..aOS(3, _omitFieldNames ? '' : 'createdAt')
    ..aOS(4, _omitFieldNames ? '' : 'gameResultId')
    ..aOS(5, _omitFieldNames ? '' : 'id')
    ..aOS(7, _omitFieldNames ? '' : 'ownerPublicKey')
    ..aInt64(8, _omitFieldNames ? '' : 'petExpDelta')
    ..aOS(9, _omitFieldNames ? '' : 'petId')
    ..aInt64(10, _omitFieldNames ? '' : 'pointsDelta')
    ..aOS(11, _omitFieldNames ? '' : 'reason')
    ..aOS(12, _omitFieldNames ? '' : 'rulesetName')
    ..aOS(13, _omitFieldNames ? '' : 'sourceId')
    ..aOS(14, _omitFieldNames ? '' : 'sourceType')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RewardGrant clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RewardGrant copyWith(void Function(RewardGrant) updates) =>
      super.copyWith((message) => updates(message as RewardGrant))
          as RewardGrant;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RewardGrant create() => RewardGrant._();
  @$core.override
  RewardGrant createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RewardGrant getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RewardGrant>(create);
  static RewardGrant? _defaultInstance;

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, $fixnum.Int64> get badgeExpDelta => $_getMap(0);

  @$pb.TagNumber(3)
  $core.String get createdAt => $_getSZ(1);
  @$pb.TagNumber(3)
  set createdAt($core.String value) => $_setString(1, value);
  @$pb.TagNumber(3)
  $core.bool hasCreatedAt() => $_has(1);
  @$pb.TagNumber(3)
  void clearCreatedAt() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get gameResultId => $_getSZ(2);
  @$pb.TagNumber(4)
  set gameResultId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(4)
  $core.bool hasGameResultId() => $_has(2);
  @$pb.TagNumber(4)
  void clearGameResultId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get id => $_getSZ(3);
  @$pb.TagNumber(5)
  set id($core.String value) => $_setString(3, value);
  @$pb.TagNumber(5)
  $core.bool hasId() => $_has(3);
  @$pb.TagNumber(5)
  void clearId() => $_clearField(5);

  @$pb.TagNumber(7)
  $core.String get ownerPublicKey => $_getSZ(4);
  @$pb.TagNumber(7)
  set ownerPublicKey($core.String value) => $_setString(4, value);
  @$pb.TagNumber(7)
  $core.bool hasOwnerPublicKey() => $_has(4);
  @$pb.TagNumber(7)
  void clearOwnerPublicKey() => $_clearField(7);

  @$pb.TagNumber(8)
  $fixnum.Int64 get petExpDelta => $_getI64(5);
  @$pb.TagNumber(8)
  set petExpDelta($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(8)
  $core.bool hasPetExpDelta() => $_has(5);
  @$pb.TagNumber(8)
  void clearPetExpDelta() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get petId => $_getSZ(6);
  @$pb.TagNumber(9)
  set petId($core.String value) => $_setString(6, value);
  @$pb.TagNumber(9)
  $core.bool hasPetId() => $_has(6);
  @$pb.TagNumber(9)
  void clearPetId() => $_clearField(9);

  @$pb.TagNumber(10)
  $fixnum.Int64 get pointsDelta => $_getI64(7);
  @$pb.TagNumber(10)
  set pointsDelta($fixnum.Int64 value) => $_setInt64(7, value);
  @$pb.TagNumber(10)
  $core.bool hasPointsDelta() => $_has(7);
  @$pb.TagNumber(10)
  void clearPointsDelta() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.String get reason => $_getSZ(8);
  @$pb.TagNumber(11)
  set reason($core.String value) => $_setString(8, value);
  @$pb.TagNumber(11)
  $core.bool hasReason() => $_has(8);
  @$pb.TagNumber(11)
  void clearReason() => $_clearField(11);

  @$pb.TagNumber(12)
  $core.String get rulesetName => $_getSZ(9);
  @$pb.TagNumber(12)
  set rulesetName($core.String value) => $_setString(9, value);
  @$pb.TagNumber(12)
  $core.bool hasRulesetName() => $_has(9);
  @$pb.TagNumber(12)
  void clearRulesetName() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.String get sourceId => $_getSZ(10);
  @$pb.TagNumber(13)
  set sourceId($core.String value) => $_setString(10, value);
  @$pb.TagNumber(13)
  $core.bool hasSourceId() => $_has(10);
  @$pb.TagNumber(13)
  void clearSourceId() => $_clearField(13);

  @$pb.TagNumber(14)
  $core.String get sourceType => $_getSZ(11);
  @$pb.TagNumber(14)
  set sourceType($core.String value) => $_setString(11, value);
  @$pb.TagNumber(14)
  $core.bool hasSourceType() => $_has(11);
  @$pb.TagNumber(14)
  void clearSourceType() => $_clearField(14);
}

class RewardGrantListResponse extends $pb.GeneratedMessage {
  factory RewardGrantListResponse({
    $core.bool? hasNext,
    $core.Iterable<RewardGrant>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  RewardGrantListResponse._();

  factory RewardGrantListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RewardGrantListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RewardGrantListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<RewardGrant>(2, _omitFieldNames ? '' : 'items',
        subBuilder: RewardGrant.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RewardGrantListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RewardGrantListResponse copyWith(
          void Function(RewardGrantListResponse) updates) =>
      super.copyWith((message) => updates(message as RewardGrantListResponse))
          as RewardGrantListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RewardGrantListResponse create() => RewardGrantListResponse._();
  @$core.override
  RewardGrantListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RewardGrantListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RewardGrantListResponse>(create);
  static RewardGrantListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<RewardGrant> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class ServerBadgeGetRequest extends $pb.GeneratedMessage {
  factory ServerBadgeGetRequest({
    GameplayGetRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerBadgeGetRequest._();

  factory ServerBadgeGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerBadgeGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerBadgeGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayGetRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayGetRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerBadgeGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerBadgeGetRequest copyWith(
          void Function(ServerBadgeGetRequest) updates) =>
      super.copyWith((message) => updates(message as ServerBadgeGetRequest))
          as ServerBadgeGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerBadgeGetRequest create() => ServerBadgeGetRequest._();
  @$core.override
  ServerBadgeGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerBadgeGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerBadgeGetRequest>(create);
  static ServerBadgeGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayGetRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayGetRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayGetRequest ensureValue() => $_ensure(0);
}

class ServerBadgeGetResponse extends $pb.GeneratedMessage {
  factory ServerBadgeGetResponse({
    Badge? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerBadgeGetResponse._();

  factory ServerBadgeGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerBadgeGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerBadgeGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Badge>(1, _omitFieldNames ? '' : 'value', subBuilder: Badge.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerBadgeGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerBadgeGetResponse copyWith(
          void Function(ServerBadgeGetResponse) updates) =>
      super.copyWith((message) => updates(message as ServerBadgeGetResponse))
          as ServerBadgeGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerBadgeGetResponse create() => ServerBadgeGetResponse._();
  @$core.override
  ServerBadgeGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerBadgeGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerBadgeGetResponse>(create);
  static ServerBadgeGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Badge get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Badge value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Badge ensureValue() => $_ensure(0);
}

class ServerBadgeListRequest extends $pb.GeneratedMessage {
  factory ServerBadgeListRequest({
    GameplayListRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerBadgeListRequest._();

  factory ServerBadgeListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerBadgeListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerBadgeListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayListRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayListRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerBadgeListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerBadgeListRequest copyWith(
          void Function(ServerBadgeListRequest) updates) =>
      super.copyWith((message) => updates(message as ServerBadgeListRequest))
          as ServerBadgeListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerBadgeListRequest create() => ServerBadgeListRequest._();
  @$core.override
  ServerBadgeListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerBadgeListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerBadgeListRequest>(create);
  static ServerBadgeListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayListRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayListRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayListRequest ensureValue() => $_ensure(0);
}

class ServerBadgeListResponse extends $pb.GeneratedMessage {
  factory ServerBadgeListResponse({
    BadgeListResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerBadgeListResponse._();

  factory ServerBadgeListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerBadgeListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerBadgeListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<BadgeListResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: BadgeListResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerBadgeListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerBadgeListResponse copyWith(
          void Function(ServerBadgeListResponse) updates) =>
      super.copyWith((message) => updates(message as ServerBadgeListResponse))
          as ServerBadgeListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerBadgeListResponse create() => ServerBadgeListResponse._();
  @$core.override
  ServerBadgeListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerBadgeListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerBadgeListResponse>(create);
  static ServerBadgeListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  BadgeListResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(BadgeListResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  BadgeListResponse ensureValue() => $_ensure(0);
}

class ServerGameResultGetRequest extends $pb.GeneratedMessage {
  factory ServerGameResultGetRequest({
    GameplayGetRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGameResultGetRequest._();

  factory ServerGameResultGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGameResultGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGameResultGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayGetRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayGetRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameResultGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameResultGetRequest copyWith(
          void Function(ServerGameResultGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGameResultGetRequest))
          as ServerGameResultGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGameResultGetRequest create() => ServerGameResultGetRequest._();
  @$core.override
  ServerGameResultGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGameResultGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGameResultGetRequest>(create);
  static ServerGameResultGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayGetRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayGetRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayGetRequest ensureValue() => $_ensure(0);
}

class ServerGameResultGetResponse extends $pb.GeneratedMessage {
  factory ServerGameResultGetResponse({
    GameResult? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGameResultGetResponse._();

  factory ServerGameResultGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGameResultGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGameResultGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameResult>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameResult.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameResultGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameResultGetResponse copyWith(
          void Function(ServerGameResultGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGameResultGetResponse))
          as ServerGameResultGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGameResultGetResponse create() =>
      ServerGameResultGetResponse._();
  @$core.override
  ServerGameResultGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGameResultGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGameResultGetResponse>(create);
  static ServerGameResultGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  GameResult get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameResult value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameResult ensureValue() => $_ensure(0);
}

class ServerGameResultListRequest extends $pb.GeneratedMessage {
  factory ServerGameResultListRequest({
    GameplayListRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGameResultListRequest._();

  factory ServerGameResultListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGameResultListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGameResultListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayListRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayListRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameResultListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameResultListRequest copyWith(
          void Function(ServerGameResultListRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGameResultListRequest))
          as ServerGameResultListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGameResultListRequest create() =>
      ServerGameResultListRequest._();
  @$core.override
  ServerGameResultListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGameResultListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGameResultListRequest>(create);
  static ServerGameResultListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayListRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayListRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayListRequest ensureValue() => $_ensure(0);
}

class ServerGameResultListResponse extends $pb.GeneratedMessage {
  factory ServerGameResultListResponse({
    GameResultListResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGameResultListResponse._();

  factory ServerGameResultListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGameResultListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGameResultListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameResultListResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameResultListResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameResultListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameResultListResponse copyWith(
          void Function(ServerGameResultListResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGameResultListResponse))
          as ServerGameResultListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGameResultListResponse create() =>
      ServerGameResultListResponse._();
  @$core.override
  ServerGameResultListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGameResultListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGameResultListResponse>(create);
  static ServerGameResultListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  GameResultListResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameResultListResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameResultListResponse ensureValue() => $_ensure(0);
}

class ServerGameRulesetGetRequest extends $pb.GeneratedMessage {
  factory ServerGameRulesetGetRequest({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  ServerGameRulesetGetRequest._();

  factory ServerGameRulesetGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGameRulesetGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGameRulesetGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameRulesetGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameRulesetGetRequest copyWith(
          void Function(ServerGameRulesetGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGameRulesetGetRequest))
          as ServerGameRulesetGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGameRulesetGetRequest create() =>
      ServerGameRulesetGetRequest._();
  @$core.override
  ServerGameRulesetGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGameRulesetGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGameRulesetGetRequest>(create);
  static ServerGameRulesetGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);
}

class ServerGameRulesetGetResponse extends $pb.GeneratedMessage {
  factory ServerGameRulesetGetResponse({
    GameRuleset? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGameRulesetGetResponse._();

  factory ServerGameRulesetGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGameRulesetGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGameRulesetGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameRuleset>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameRuleset.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameRulesetGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGameRulesetGetResponse copyWith(
          void Function(ServerGameRulesetGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerGameRulesetGetResponse))
          as ServerGameRulesetGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGameRulesetGetResponse create() =>
      ServerGameRulesetGetResponse._();
  @$core.override
  ServerGameRulesetGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGameRulesetGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGameRulesetGetResponse>(create);
  static ServerGameRulesetGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  GameRuleset get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameRuleset value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameRuleset ensureValue() => $_ensure(0);
}

class ServerPetAdoptRequest extends $pb.GeneratedMessage {
  factory ServerPetAdoptRequest({
    PetAdoptRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetAdoptRequest._();

  factory ServerPetAdoptRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetAdoptRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetAdoptRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetAdoptRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetAdoptRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetAdoptRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetAdoptRequest copyWith(
          void Function(ServerPetAdoptRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPetAdoptRequest))
          as ServerPetAdoptRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetAdoptRequest create() => ServerPetAdoptRequest._();
  @$core.override
  ServerPetAdoptRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetAdoptRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetAdoptRequest>(create);
  static ServerPetAdoptRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PetAdoptRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetAdoptRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetAdoptRequest ensureValue() => $_ensure(0);
}

class ServerPetAdoptResponse extends $pb.GeneratedMessage {
  factory ServerPetAdoptResponse({
    PetAdoptResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetAdoptResponse._();

  factory ServerPetAdoptResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetAdoptResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetAdoptResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetAdoptResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetAdoptResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetAdoptResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetAdoptResponse copyWith(
          void Function(ServerPetAdoptResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPetAdoptResponse))
          as ServerPetAdoptResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetAdoptResponse create() => ServerPetAdoptResponse._();
  @$core.override
  ServerPetAdoptResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetAdoptResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetAdoptResponse>(create);
  static ServerPetAdoptResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PetAdoptResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetAdoptResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetAdoptResponse ensureValue() => $_ensure(0);
}

class ServerPetDeleteRequest extends $pb.GeneratedMessage {
  factory ServerPetDeleteRequest({
    PetDeleteRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetDeleteRequest._();

  factory ServerPetDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetDeleteRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetDeleteRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetDeleteRequest copyWith(
          void Function(ServerPetDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPetDeleteRequest))
          as ServerPetDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetDeleteRequest create() => ServerPetDeleteRequest._();
  @$core.override
  ServerPetDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetDeleteRequest>(create);
  static ServerPetDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PetDeleteRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetDeleteRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetDeleteRequest ensureValue() => $_ensure(0);
}

class ServerPetDeleteResponse extends $pb.GeneratedMessage {
  factory ServerPetDeleteResponse({
    Pet? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetDeleteResponse._();

  factory ServerPetDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Pet>(1, _omitFieldNames ? '' : 'value', subBuilder: Pet.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetDeleteResponse copyWith(
          void Function(ServerPetDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPetDeleteResponse))
          as ServerPetDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetDeleteResponse create() => ServerPetDeleteResponse._();
  @$core.override
  ServerPetDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetDeleteResponse>(create);
  static ServerPetDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Pet get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Pet value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Pet ensureValue() => $_ensure(0);
}

class ServerPetDriveRequest extends $pb.GeneratedMessage {
  factory ServerPetDriveRequest({
    PetDriveRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetDriveRequest._();

  factory ServerPetDriveRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetDriveRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetDriveRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetDriveRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetDriveRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetDriveRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetDriveRequest copyWith(
          void Function(ServerPetDriveRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPetDriveRequest))
          as ServerPetDriveRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetDriveRequest create() => ServerPetDriveRequest._();
  @$core.override
  ServerPetDriveRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetDriveRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetDriveRequest>(create);
  static ServerPetDriveRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PetDriveRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetDriveRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetDriveRequest ensureValue() => $_ensure(0);
}

class ServerPetDriveResponse extends $pb.GeneratedMessage {
  factory ServerPetDriveResponse({
    PetDriveResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetDriveResponse._();

  factory ServerPetDriveResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetDriveResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetDriveResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetDriveResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetDriveResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetDriveResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetDriveResponse copyWith(
          void Function(ServerPetDriveResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPetDriveResponse))
          as ServerPetDriveResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetDriveResponse create() => ServerPetDriveResponse._();
  @$core.override
  ServerPetDriveResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetDriveResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetDriveResponse>(create);
  static ServerPetDriveResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PetDriveResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetDriveResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetDriveResponse ensureValue() => $_ensure(0);
}

class ServerPetGetRequest extends $pb.GeneratedMessage {
  factory ServerPetGetRequest({
    PetGetRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetGetRequest._();

  factory ServerPetGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetGetRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetGetRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetGetRequest copyWith(void Function(ServerPetGetRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPetGetRequest))
          as ServerPetGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetGetRequest create() => ServerPetGetRequest._();
  @$core.override
  ServerPetGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetGetRequest>(create);
  static ServerPetGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PetGetRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetGetRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetGetRequest ensureValue() => $_ensure(0);
}

class ServerPetGetResponse extends $pb.GeneratedMessage {
  factory ServerPetGetResponse({
    Pet? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetGetResponse._();

  factory ServerPetGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Pet>(1, _omitFieldNames ? '' : 'value', subBuilder: Pet.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetGetResponse copyWith(void Function(ServerPetGetResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPetGetResponse))
          as ServerPetGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetGetResponse create() => ServerPetGetResponse._();
  @$core.override
  ServerPetGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetGetResponse>(create);
  static ServerPetGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Pet get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Pet value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Pet ensureValue() => $_ensure(0);
}

class ServerPetPixaDownloadRequest extends $pb.GeneratedMessage {
  factory ServerPetPixaDownloadRequest({
    PetPixaDownloadRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetPixaDownloadRequest._();

  factory ServerPetPixaDownloadRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetPixaDownloadRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetPixaDownloadRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetPixaDownloadRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetPixaDownloadRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPixaDownloadRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPixaDownloadRequest copyWith(
          void Function(ServerPetPixaDownloadRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerPetPixaDownloadRequest))
          as ServerPetPixaDownloadRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetPixaDownloadRequest create() =>
      ServerPetPixaDownloadRequest._();
  @$core.override
  ServerPetPixaDownloadRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetPixaDownloadRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetPixaDownloadRequest>(create);
  static ServerPetPixaDownloadRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PetPixaDownloadRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetPixaDownloadRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetPixaDownloadRequest ensureValue() => $_ensure(0);
}

class ServerPetPixaDownloadResponse extends $pb.GeneratedMessage {
  factory ServerPetPixaDownloadResponse({
    PetPixaDownloadResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetPixaDownloadResponse._();

  factory ServerPetPixaDownloadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetPixaDownloadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetPixaDownloadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetPixaDownloadResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetPixaDownloadResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPixaDownloadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPixaDownloadResponse copyWith(
          void Function(ServerPetPixaDownloadResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerPetPixaDownloadResponse))
          as ServerPetPixaDownloadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetPixaDownloadResponse create() =>
      ServerPetPixaDownloadResponse._();
  @$core.override
  ServerPetPixaDownloadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetPixaDownloadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetPixaDownloadResponse>(create);
  static ServerPetPixaDownloadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PetPixaDownloadResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetPixaDownloadResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetPixaDownloadResponse ensureValue() => $_ensure(0);
}

class ServerPetPresentationGetRequest extends $pb.GeneratedMessage {
  factory ServerPetPresentationGetRequest({
    PetGetRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetPresentationGetRequest._();

  factory ServerPetPresentationGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetPresentationGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetPresentationGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetGetRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetGetRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPresentationGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPresentationGetRequest copyWith(
          void Function(ServerPetPresentationGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerPetPresentationGetRequest))
          as ServerPetPresentationGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetPresentationGetRequest create() =>
      ServerPetPresentationGetRequest._();
  @$core.override
  ServerPetPresentationGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetPresentationGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetPresentationGetRequest>(
          create);
  static ServerPetPresentationGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PetGetRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetGetRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetGetRequest ensureValue() => $_ensure(0);
}

class ServerPetPresentationGetResponse extends $pb.GeneratedMessage {
  factory ServerPetPresentationGetResponse({
    PetPresentation? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetPresentationGetResponse._();

  factory ServerPetPresentationGetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetPresentationGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetPresentationGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetPresentation>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetPresentation.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPresentationGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPresentationGetResponse copyWith(
          void Function(ServerPetPresentationGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerPetPresentationGetResponse))
          as ServerPetPresentationGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetPresentationGetResponse create() =>
      ServerPetPresentationGetResponse._();
  @$core.override
  ServerPetPresentationGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetPresentationGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetPresentationGetResponse>(
          create);
  static ServerPetPresentationGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PetPresentation get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetPresentation value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetPresentation ensureValue() => $_ensure(0);
}

class ServerPetListRequest extends $pb.GeneratedMessage {
  factory ServerPetListRequest({
    GameplayListRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetListRequest._();

  factory ServerPetListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayListRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayListRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetListRequest copyWith(void Function(ServerPetListRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPetListRequest))
          as ServerPetListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetListRequest create() => ServerPetListRequest._();
  @$core.override
  ServerPetListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetListRequest>(create);
  static ServerPetListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayListRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayListRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayListRequest ensureValue() => $_ensure(0);
}

class ServerPetListResponse extends $pb.GeneratedMessage {
  factory ServerPetListResponse({
    PetListResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetListResponse._();

  factory ServerPetListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetListResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetListResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetListResponse copyWith(
          void Function(ServerPetListResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPetListResponse))
          as ServerPetListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetListResponse create() => ServerPetListResponse._();
  @$core.override
  ServerPetListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetListResponse>(create);
  static ServerPetListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PetListResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetListResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetListResponse ensureValue() => $_ensure(0);
}

class ServerPetPutRequest extends $pb.GeneratedMessage {
  factory ServerPetPutRequest({
    PetPutRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetPutRequest._();

  factory ServerPetPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PetPutRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PetPutRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPutRequest copyWith(void Function(ServerPetPutRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPetPutRequest))
          as ServerPetPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetPutRequest create() => ServerPetPutRequest._();
  @$core.override
  ServerPetPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetPutRequest>(create);
  static ServerPetPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  PetPutRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PetPutRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PetPutRequest ensureValue() => $_ensure(0);
}

class ServerPetPutResponse extends $pb.GeneratedMessage {
  factory ServerPetPutResponse({
    Pet? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPetPutResponse._();

  factory ServerPetPutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPetPutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPetPutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Pet>(1, _omitFieldNames ? '' : 'value', subBuilder: Pet.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPetPutResponse copyWith(void Function(ServerPetPutResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPetPutResponse))
          as ServerPetPutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPetPutResponse create() => ServerPetPutResponse._();
  @$core.override
  ServerPetPutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPetPutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPetPutResponse>(create);
  static ServerPetPutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Pet get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Pet value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Pet ensureValue() => $_ensure(0);
}

class ServerPointsGetRequest extends $pb.GeneratedMessage {
  factory ServerPointsGetRequest({
    $core.String? rulesetName,
  }) {
    final result = create();
    if (rulesetName != null) result.rulesetName = rulesetName;
    return result;
  }

  ServerPointsGetRequest._();

  factory ServerPointsGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPointsGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPointsGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'rulesetName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsGetRequest copyWith(
          void Function(ServerPointsGetRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPointsGetRequest))
          as ServerPointsGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPointsGetRequest create() => ServerPointsGetRequest._();
  @$core.override
  ServerPointsGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPointsGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPointsGetRequest>(create);
  static ServerPointsGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get rulesetName => $_getSZ(0);
  @$pb.TagNumber(1)
  set rulesetName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRulesetName() => $_has(0);
  @$pb.TagNumber(1)
  void clearRulesetName() => $_clearField(1);
}

class ServerPointsGetResponse extends $pb.GeneratedMessage {
  factory ServerPointsGetResponse({
    PointsAccount? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPointsGetResponse._();

  factory ServerPointsGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPointsGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPointsGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PointsAccount>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PointsAccount.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsGetResponse copyWith(
          void Function(ServerPointsGetResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPointsGetResponse))
          as ServerPointsGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPointsGetResponse create() => ServerPointsGetResponse._();
  @$core.override
  ServerPointsGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPointsGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPointsGetResponse>(create);
  static ServerPointsGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PointsAccount get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PointsAccount value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PointsAccount ensureValue() => $_ensure(0);
}

class ServerPointsTransactionGetRequest extends $pb.GeneratedMessage {
  factory ServerPointsTransactionGetRequest({
    GameplayGetRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPointsTransactionGetRequest._();

  factory ServerPointsTransactionGetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPointsTransactionGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPointsTransactionGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayGetRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayGetRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsTransactionGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsTransactionGetRequest copyWith(
          void Function(ServerPointsTransactionGetRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ServerPointsTransactionGetRequest))
          as ServerPointsTransactionGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPointsTransactionGetRequest create() =>
      ServerPointsTransactionGetRequest._();
  @$core.override
  ServerPointsTransactionGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPointsTransactionGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPointsTransactionGetRequest>(
          create);
  static ServerPointsTransactionGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayGetRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayGetRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayGetRequest ensureValue() => $_ensure(0);
}

class ServerPointsTransactionGetResponse extends $pb.GeneratedMessage {
  factory ServerPointsTransactionGetResponse({
    PointsTransaction? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPointsTransactionGetResponse._();

  factory ServerPointsTransactionGetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPointsTransactionGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPointsTransactionGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PointsTransaction>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PointsTransaction.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsTransactionGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsTransactionGetResponse copyWith(
          void Function(ServerPointsTransactionGetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ServerPointsTransactionGetResponse))
          as ServerPointsTransactionGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPointsTransactionGetResponse create() =>
      ServerPointsTransactionGetResponse._();
  @$core.override
  ServerPointsTransactionGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPointsTransactionGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPointsTransactionGetResponse>(
          create);
  static ServerPointsTransactionGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PointsTransaction get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PointsTransaction value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PointsTransaction ensureValue() => $_ensure(0);
}

class ServerPointsTransactionListRequest extends $pb.GeneratedMessage {
  factory ServerPointsTransactionListRequest({
    GameplayListRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPointsTransactionListRequest._();

  factory ServerPointsTransactionListRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPointsTransactionListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPointsTransactionListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayListRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayListRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsTransactionListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsTransactionListRequest copyWith(
          void Function(ServerPointsTransactionListRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ServerPointsTransactionListRequest))
          as ServerPointsTransactionListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPointsTransactionListRequest create() =>
      ServerPointsTransactionListRequest._();
  @$core.override
  ServerPointsTransactionListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPointsTransactionListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPointsTransactionListRequest>(
          create);
  static ServerPointsTransactionListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayListRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayListRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayListRequest ensureValue() => $_ensure(0);
}

class ServerPointsTransactionListResponse extends $pb.GeneratedMessage {
  factory ServerPointsTransactionListResponse({
    PointsTransactionListResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPointsTransactionListResponse._();

  factory ServerPointsTransactionListResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPointsTransactionListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPointsTransactionListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PointsTransactionListResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PointsTransactionListResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsTransactionListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPointsTransactionListResponse copyWith(
          void Function(ServerPointsTransactionListResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ServerPointsTransactionListResponse))
          as ServerPointsTransactionListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPointsTransactionListResponse create() =>
      ServerPointsTransactionListResponse._();
  @$core.override
  ServerPointsTransactionListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPointsTransactionListResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ServerPointsTransactionListResponse>(create);
  static ServerPointsTransactionListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PointsTransactionListResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PointsTransactionListResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PointsTransactionListResponse ensureValue() => $_ensure(0);
}

class ServerRewardGrantGetRequest extends $pb.GeneratedMessage {
  factory ServerRewardGrantGetRequest({
    GameplayGetRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerRewardGrantGetRequest._();

  factory ServerRewardGrantGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRewardGrantGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRewardGrantGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayGetRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayGetRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRewardGrantGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRewardGrantGetRequest copyWith(
          void Function(ServerRewardGrantGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerRewardGrantGetRequest))
          as ServerRewardGrantGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRewardGrantGetRequest create() =>
      ServerRewardGrantGetRequest._();
  @$core.override
  ServerRewardGrantGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRewardGrantGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRewardGrantGetRequest>(create);
  static ServerRewardGrantGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayGetRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayGetRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayGetRequest ensureValue() => $_ensure(0);
}

class ServerRewardGrantGetResponse extends $pb.GeneratedMessage {
  factory ServerRewardGrantGetResponse({
    RewardGrant? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerRewardGrantGetResponse._();

  factory ServerRewardGrantGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRewardGrantGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRewardGrantGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<RewardGrant>(1, _omitFieldNames ? '' : 'value',
        subBuilder: RewardGrant.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRewardGrantGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRewardGrantGetResponse copyWith(
          void Function(ServerRewardGrantGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerRewardGrantGetResponse))
          as ServerRewardGrantGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRewardGrantGetResponse create() =>
      ServerRewardGrantGetResponse._();
  @$core.override
  ServerRewardGrantGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRewardGrantGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRewardGrantGetResponse>(create);
  static ServerRewardGrantGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  RewardGrant get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(RewardGrant value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  RewardGrant ensureValue() => $_ensure(0);
}

class ServerRewardGrantListRequest extends $pb.GeneratedMessage {
  factory ServerRewardGrantListRequest({
    GameplayListRequest? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerRewardGrantListRequest._();

  factory ServerRewardGrantListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRewardGrantListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRewardGrantListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<GameplayListRequest>(1, _omitFieldNames ? '' : 'value',
        subBuilder: GameplayListRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRewardGrantListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRewardGrantListRequest copyWith(
          void Function(ServerRewardGrantListRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ServerRewardGrantListRequest))
          as ServerRewardGrantListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRewardGrantListRequest create() =>
      ServerRewardGrantListRequest._();
  @$core.override
  ServerRewardGrantListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRewardGrantListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRewardGrantListRequest>(create);
  static ServerRewardGrantListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  GameplayListRequest get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(GameplayListRequest value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  GameplayListRequest ensureValue() => $_ensure(0);
}

class ServerRewardGrantListResponse extends $pb.GeneratedMessage {
  factory ServerRewardGrantListResponse({
    RewardGrantListResponse? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerRewardGrantListResponse._();

  factory ServerRewardGrantListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRewardGrantListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRewardGrantListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<RewardGrantListResponse>(1, _omitFieldNames ? '' : 'value',
        subBuilder: RewardGrantListResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRewardGrantListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRewardGrantListResponse copyWith(
          void Function(ServerRewardGrantListResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerRewardGrantListResponse))
          as ServerRewardGrantListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRewardGrantListResponse create() =>
      ServerRewardGrantListResponse._();
  @$core.override
  ServerRewardGrantListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRewardGrantListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRewardGrantListResponse>(create);
  static ServerRewardGrantListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  RewardGrantListResponse get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(RewardGrantListResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  RewardGrantListResponse ensureValue() => $_ensure(0);
}

class PetLife extends $pb.GeneratedMessage {
  factory PetLife({
    $core.Iterable<$core.MapEntry<$core.String, $fixnum.Int64>>? value,
  }) {
    final result = create();
    if (value != null) result.value.addEntries(value);
    return result;
  }

  PetLife._();

  factory PetLife.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetLife.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetLife',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, $fixnum.Int64>(1, _omitFieldNames ? '' : 'value',
        entryClassName: 'PetLife.ValueEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.O6,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetLife clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetLife copyWith(void Function(PetLife) updates) =>
      super.copyWith((message) => updates(message as PetLife)) as PetLife;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetLife create() => PetLife._();
  @$core.override
  PetLife createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetLife getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PetLife>(create);
  static PetLife? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbMap<$core.String, $fixnum.Int64> get value => $_getMap(0);
}

class PetProgression extends $pb.GeneratedMessage {
  factory PetProgression({
    $core.Iterable<$core.MapEntry<$core.String, $fixnum.Int64>>? value,
  }) {
    final result = create();
    if (value != null) result.value.addEntries(value);
    return result;
  }

  PetProgression._();

  factory PetProgression.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetProgression.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetProgression',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, $fixnum.Int64>(1, _omitFieldNames ? '' : 'value',
        entryClassName: 'PetProgression.ValueEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.O6,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetProgression clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetProgression copyWith(void Function(PetProgression) updates) =>
      super.copyWith((message) => updates(message as PetProgression))
          as PetProgression;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetProgression create() => PetProgression._();
  @$core.override
  PetProgression createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetProgression getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetProgression>(create);
  static PetProgression? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbMap<$core.String, $fixnum.Int64> get value => $_getMap(0);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
