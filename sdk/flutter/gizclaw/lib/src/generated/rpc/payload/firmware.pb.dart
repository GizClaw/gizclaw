// This is a generated file - do not edit.
//
// Generated from payload/firmware.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;

import 'enums.pbenum.dart' as $0;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

class Firmware extends $pb.GeneratedMessage {
  factory Firmware({
    $core.String? createdAt,
    $core.String? description,
    $core.String? name,
    FirmwareSlots? slots,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (description != null) result.description = description;
    if (name != null) result.name = name;
    if (slots != null) result.slots = slots;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  Firmware._();

  factory Firmware.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Firmware.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Firmware',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'description')
    ..aOS(3, _omitFieldNames ? '' : 'name')
    ..aOM<FirmwareSlots>(4, _omitFieldNames ? '' : 'slots',
        subBuilder: FirmwareSlots.create)
    ..aOS(5, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Firmware clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Firmware copyWith(void Function(Firmware) updates) =>
      super.copyWith((message) => updates(message as Firmware)) as Firmware;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Firmware create() => Firmware._();
  @$core.override
  Firmware createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Firmware getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Firmware>(create);
  static Firmware? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get description => $_getSZ(1);
  @$pb.TagNumber(2)
  set description($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDescription() => $_has(1);
  @$pb.TagNumber(2)
  void clearDescription() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get name => $_getSZ(2);
  @$pb.TagNumber(3)
  set name($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasName() => $_has(2);
  @$pb.TagNumber(3)
  void clearName() => $_clearField(3);

  @$pb.TagNumber(4)
  FirmwareSlots get slots => $_getN(3);
  @$pb.TagNumber(4)
  set slots(FirmwareSlots value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasSlots() => $_has(3);
  @$pb.TagNumber(4)
  void clearSlots() => $_clearField(4);
  @$pb.TagNumber(4)
  FirmwareSlots ensureSlots() => $_ensure(3);

  @$pb.TagNumber(5)
  $core.String get updatedAt => $_getSZ(4);
  @$pb.TagNumber(5)
  set updatedAt($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasUpdatedAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearUpdatedAt() => $_clearField(5);
}

class FirmwareArtifact extends $pb.GeneratedMessage {
  factory FirmwareArtifact({
    $core.String? contentType,
    $core.String? filesPath,
    $core.String? manifestPath,
    $core.String? sha256,
    $fixnum.Int64? size,
    $core.String? tarPath,
    $core.String? uploadedAt,
  }) {
    final result = create();
    if (contentType != null) result.contentType = contentType;
    if (filesPath != null) result.filesPath = filesPath;
    if (manifestPath != null) result.manifestPath = manifestPath;
    if (sha256 != null) result.sha256 = sha256;
    if (size != null) result.size = size;
    if (tarPath != null) result.tarPath = tarPath;
    if (uploadedAt != null) result.uploadedAt = uploadedAt;
    return result;
  }

  FirmwareArtifact._();

  factory FirmwareArtifact.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FirmwareArtifact.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FirmwareArtifact',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'contentType')
    ..aOS(2, _omitFieldNames ? '' : 'filesPath')
    ..aOS(3, _omitFieldNames ? '' : 'manifestPath')
    ..aOS(4, _omitFieldNames ? '' : 'sha256')
    ..aInt64(5, _omitFieldNames ? '' : 'size')
    ..aOS(6, _omitFieldNames ? '' : 'tarPath')
    ..aOS(7, _omitFieldNames ? '' : 'uploadedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareArtifact clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareArtifact copyWith(void Function(FirmwareArtifact) updates) =>
      super.copyWith((message) => updates(message as FirmwareArtifact))
          as FirmwareArtifact;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FirmwareArtifact create() => FirmwareArtifact._();
  @$core.override
  FirmwareArtifact createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FirmwareArtifact getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FirmwareArtifact>(create);
  static FirmwareArtifact? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get contentType => $_getSZ(0);
  @$pb.TagNumber(1)
  set contentType($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasContentType() => $_has(0);
  @$pb.TagNumber(1)
  void clearContentType() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get filesPath => $_getSZ(1);
  @$pb.TagNumber(2)
  set filesPath($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasFilesPath() => $_has(1);
  @$pb.TagNumber(2)
  void clearFilesPath() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get manifestPath => $_getSZ(2);
  @$pb.TagNumber(3)
  set manifestPath($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasManifestPath() => $_has(2);
  @$pb.TagNumber(3)
  void clearManifestPath() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get sha256 => $_getSZ(3);
  @$pb.TagNumber(4)
  set sha256($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSha256() => $_has(3);
  @$pb.TagNumber(4)
  void clearSha256() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get size => $_getI64(4);
  @$pb.TagNumber(5)
  set size($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSize() => $_has(4);
  @$pb.TagNumber(5)
  void clearSize() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get tarPath => $_getSZ(5);
  @$pb.TagNumber(6)
  set tarPath($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasTarPath() => $_has(5);
  @$pb.TagNumber(6)
  void clearTarPath() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get uploadedAt => $_getSZ(6);
  @$pb.TagNumber(7)
  set uploadedAt($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasUploadedAt() => $_has(6);
  @$pb.TagNumber(7)
  void clearUploadedAt() => $_clearField(7);
}

class FirmwareArtifactEntry extends $pb.GeneratedMessage {
  factory FirmwareArtifactEntry({
    $core.String? contentType,
    $core.String? modTime,
    $core.int? mode,
    $core.String? path,
    $fixnum.Int64? size,
    $0.FirmwareArtifactEntryType? type,
  }) {
    final result = create();
    if (contentType != null) result.contentType = contentType;
    if (modTime != null) result.modTime = modTime;
    if (mode != null) result.mode = mode;
    if (path != null) result.path = path;
    if (size != null) result.size = size;
    if (type != null) result.type = type;
    return result;
  }

  FirmwareArtifactEntry._();

  factory FirmwareArtifactEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FirmwareArtifactEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FirmwareArtifactEntry',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'contentType')
    ..aOS(2, _omitFieldNames ? '' : 'modTime')
    ..aI(3, _omitFieldNames ? '' : 'mode')
    ..aOS(4, _omitFieldNames ? '' : 'path')
    ..aInt64(5, _omitFieldNames ? '' : 'size')
    ..aE<$0.FirmwareArtifactEntryType>(6, _omitFieldNames ? '' : 'type',
        enumValues: $0.FirmwareArtifactEntryType.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareArtifactEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareArtifactEntry copyWith(
          void Function(FirmwareArtifactEntry) updates) =>
      super.copyWith((message) => updates(message as FirmwareArtifactEntry))
          as FirmwareArtifactEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FirmwareArtifactEntry create() => FirmwareArtifactEntry._();
  @$core.override
  FirmwareArtifactEntry createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FirmwareArtifactEntry getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FirmwareArtifactEntry>(create);
  static FirmwareArtifactEntry? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get contentType => $_getSZ(0);
  @$pb.TagNumber(1)
  set contentType($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasContentType() => $_has(0);
  @$pb.TagNumber(1)
  void clearContentType() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get modTime => $_getSZ(1);
  @$pb.TagNumber(2)
  set modTime($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasModTime() => $_has(1);
  @$pb.TagNumber(2)
  void clearModTime() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.int get mode => $_getIZ(2);
  @$pb.TagNumber(3)
  set mode($core.int value) => $_setSignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMode() => $_has(2);
  @$pb.TagNumber(3)
  void clearMode() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get path => $_getSZ(3);
  @$pb.TagNumber(4)
  set path($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPath() => $_has(3);
  @$pb.TagNumber(4)
  void clearPath() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get size => $_getI64(4);
  @$pb.TagNumber(5)
  set size($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSize() => $_has(4);
  @$pb.TagNumber(5)
  void clearSize() => $_clearField(5);

  @$pb.TagNumber(6)
  $0.FirmwareArtifactEntryType get type => $_getN(5);
  @$pb.TagNumber(6)
  set type($0.FirmwareArtifactEntryType value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasType() => $_has(5);
  @$pb.TagNumber(6)
  void clearType() => $_clearField(6);
}

class FirmwareFilesDownloadRequest extends $pb.GeneratedMessage {
  factory FirmwareFilesDownloadRequest({
    $0.FirmwareChannelName? channel,
    $core.String? path,
  }) {
    final result = create();
    if (channel != null) result.channel = channel;
    if (path != null) result.path = path;
    return result;
  }

  FirmwareFilesDownloadRequest._();

  factory FirmwareFilesDownloadRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FirmwareFilesDownloadRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FirmwareFilesDownloadRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$0.FirmwareChannelName>(1, _omitFieldNames ? '' : 'channel',
        enumValues: $0.FirmwareChannelName.values)
    ..aOS(2, _omitFieldNames ? '' : 'path')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareFilesDownloadRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareFilesDownloadRequest copyWith(
          void Function(FirmwareFilesDownloadRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FirmwareFilesDownloadRequest))
          as FirmwareFilesDownloadRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FirmwareFilesDownloadRequest create() =>
      FirmwareFilesDownloadRequest._();
  @$core.override
  FirmwareFilesDownloadRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FirmwareFilesDownloadRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FirmwareFilesDownloadRequest>(create);
  static FirmwareFilesDownloadRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $0.FirmwareChannelName get channel => $_getN(0);
  @$pb.TagNumber(1)
  set channel($0.FirmwareChannelName value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasChannel() => $_has(0);
  @$pb.TagNumber(1)
  void clearChannel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get path => $_getSZ(1);
  @$pb.TagNumber(2)
  set path($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPath() => $_has(1);
  @$pb.TagNumber(2)
  void clearPath() => $_clearField(2);
}

class FirmwareFilesDownloadResponse extends $pb.GeneratedMessage {
  factory FirmwareFilesDownloadResponse({
    FirmwareArtifact? artifact,
    $0.FirmwareChannelName? channel,
    FirmwareArtifactEntry? file,
    $core.String? firmwareId,
    $core.String? path,
  }) {
    final result = create();
    if (artifact != null) result.artifact = artifact;
    if (channel != null) result.channel = channel;
    if (file != null) result.file = file;
    if (firmwareId != null) result.firmwareId = firmwareId;
    if (path != null) result.path = path;
    return result;
  }

  FirmwareFilesDownloadResponse._();

  factory FirmwareFilesDownloadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FirmwareFilesDownloadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FirmwareFilesDownloadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FirmwareArtifact>(1, _omitFieldNames ? '' : 'artifact',
        subBuilder: FirmwareArtifact.create)
    ..aE<$0.FirmwareChannelName>(2, _omitFieldNames ? '' : 'channel',
        enumValues: $0.FirmwareChannelName.values)
    ..aOM<FirmwareArtifactEntry>(3, _omitFieldNames ? '' : 'file',
        subBuilder: FirmwareArtifactEntry.create)
    ..aOS(4, _omitFieldNames ? '' : 'firmwareId')
    ..aOS(5, _omitFieldNames ? '' : 'path')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareFilesDownloadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareFilesDownloadResponse copyWith(
          void Function(FirmwareFilesDownloadResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FirmwareFilesDownloadResponse))
          as FirmwareFilesDownloadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FirmwareFilesDownloadResponse create() =>
      FirmwareFilesDownloadResponse._();
  @$core.override
  FirmwareFilesDownloadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FirmwareFilesDownloadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FirmwareFilesDownloadResponse>(create);
  static FirmwareFilesDownloadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FirmwareArtifact get artifact => $_getN(0);
  @$pb.TagNumber(1)
  set artifact(FirmwareArtifact value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasArtifact() => $_has(0);
  @$pb.TagNumber(1)
  void clearArtifact() => $_clearField(1);
  @$pb.TagNumber(1)
  FirmwareArtifact ensureArtifact() => $_ensure(0);

  @$pb.TagNumber(2)
  $0.FirmwareChannelName get channel => $_getN(1);
  @$pb.TagNumber(2)
  set channel($0.FirmwareChannelName value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasChannel() => $_has(1);
  @$pb.TagNumber(2)
  void clearChannel() => $_clearField(2);

  @$pb.TagNumber(3)
  FirmwareArtifactEntry get file => $_getN(2);
  @$pb.TagNumber(3)
  set file(FirmwareArtifactEntry value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasFile() => $_has(2);
  @$pb.TagNumber(3)
  void clearFile() => $_clearField(3);
  @$pb.TagNumber(3)
  FirmwareArtifactEntry ensureFile() => $_ensure(2);

  @$pb.TagNumber(4)
  $core.String get firmwareId => $_getSZ(3);
  @$pb.TagNumber(4)
  set firmwareId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasFirmwareId() => $_has(3);
  @$pb.TagNumber(4)
  void clearFirmwareId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get path => $_getSZ(4);
  @$pb.TagNumber(5)
  set path($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasPath() => $_has(4);
  @$pb.TagNumber(5)
  void clearPath() => $_clearField(5);
}

class FirmwareGetRequest extends $pb.GeneratedMessage {
  factory FirmwareGetRequest() => create();

  FirmwareGetRequest._();

  factory FirmwareGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FirmwareGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FirmwareGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareGetRequest copyWith(void Function(FirmwareGetRequest) updates) =>
      super.copyWith((message) => updates(message as FirmwareGetRequest))
          as FirmwareGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FirmwareGetRequest create() => FirmwareGetRequest._();
  @$core.override
  FirmwareGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FirmwareGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FirmwareGetRequest>(create);
  static FirmwareGetRequest? _defaultInstance;
}

class FirmwareGetResponse extends $pb.GeneratedMessage {
  factory FirmwareGetResponse({
    Firmware? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FirmwareGetResponse._();

  factory FirmwareGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FirmwareGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FirmwareGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Firmware>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Firmware.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareGetResponse copyWith(void Function(FirmwareGetResponse) updates) =>
      super.copyWith((message) => updates(message as FirmwareGetResponse))
          as FirmwareGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FirmwareGetResponse create() => FirmwareGetResponse._();
  @$core.override
  FirmwareGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FirmwareGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FirmwareGetResponse>(create);
  static FirmwareGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Firmware get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Firmware value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Firmware ensureValue() => $_ensure(0);
}

class FirmwareSlot extends $pb.GeneratedMessage {
  factory FirmwareSlot({
    FirmwareArtifact? artifact,
    $core.String? description,
  }) {
    final result = create();
    if (artifact != null) result.artifact = artifact;
    if (description != null) result.description = description;
    return result;
  }

  FirmwareSlot._();

  factory FirmwareSlot.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FirmwareSlot.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FirmwareSlot',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FirmwareArtifact>(1, _omitFieldNames ? '' : 'artifact',
        subBuilder: FirmwareArtifact.create)
    ..aOS(2, _omitFieldNames ? '' : 'description')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareSlot clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareSlot copyWith(void Function(FirmwareSlot) updates) =>
      super.copyWith((message) => updates(message as FirmwareSlot))
          as FirmwareSlot;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FirmwareSlot create() => FirmwareSlot._();
  @$core.override
  FirmwareSlot createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FirmwareSlot getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FirmwareSlot>(create);
  static FirmwareSlot? _defaultInstance;

  @$pb.TagNumber(1)
  FirmwareArtifact get artifact => $_getN(0);
  @$pb.TagNumber(1)
  set artifact(FirmwareArtifact value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasArtifact() => $_has(0);
  @$pb.TagNumber(1)
  void clearArtifact() => $_clearField(1);
  @$pb.TagNumber(1)
  FirmwareArtifact ensureArtifact() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get description => $_getSZ(1);
  @$pb.TagNumber(2)
  set description($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDescription() => $_has(1);
  @$pb.TagNumber(2)
  void clearDescription() => $_clearField(2);
}

class FirmwareSlots extends $pb.GeneratedMessage {
  factory FirmwareSlots({
    FirmwareSlot? beta,
    FirmwareSlot? develop,
    FirmwareSlot? pending,
    FirmwareSlot? stable,
  }) {
    final result = create();
    if (beta != null) result.beta = beta;
    if (develop != null) result.develop = develop;
    if (pending != null) result.pending = pending;
    if (stable != null) result.stable = stable;
    return result;
  }

  FirmwareSlots._();

  factory FirmwareSlots.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FirmwareSlots.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FirmwareSlots',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FirmwareSlot>(1, _omitFieldNames ? '' : 'beta',
        subBuilder: FirmwareSlot.create)
    ..aOM<FirmwareSlot>(2, _omitFieldNames ? '' : 'develop',
        subBuilder: FirmwareSlot.create)
    ..aOM<FirmwareSlot>(3, _omitFieldNames ? '' : 'pending',
        subBuilder: FirmwareSlot.create)
    ..aOM<FirmwareSlot>(4, _omitFieldNames ? '' : 'stable',
        subBuilder: FirmwareSlot.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareSlots clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FirmwareSlots copyWith(void Function(FirmwareSlots) updates) =>
      super.copyWith((message) => updates(message as FirmwareSlots))
          as FirmwareSlots;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FirmwareSlots create() => FirmwareSlots._();
  @$core.override
  FirmwareSlots createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FirmwareSlots getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FirmwareSlots>(create);
  static FirmwareSlots? _defaultInstance;

  @$pb.TagNumber(1)
  FirmwareSlot get beta => $_getN(0);
  @$pb.TagNumber(1)
  set beta(FirmwareSlot value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasBeta() => $_has(0);
  @$pb.TagNumber(1)
  void clearBeta() => $_clearField(1);
  @$pb.TagNumber(1)
  FirmwareSlot ensureBeta() => $_ensure(0);

  @$pb.TagNumber(2)
  FirmwareSlot get develop => $_getN(1);
  @$pb.TagNumber(2)
  set develop(FirmwareSlot value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasDevelop() => $_has(1);
  @$pb.TagNumber(2)
  void clearDevelop() => $_clearField(2);
  @$pb.TagNumber(2)
  FirmwareSlot ensureDevelop() => $_ensure(1);

  @$pb.TagNumber(3)
  FirmwareSlot get pending => $_getN(2);
  @$pb.TagNumber(3)
  set pending(FirmwareSlot value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasPending() => $_has(2);
  @$pb.TagNumber(3)
  void clearPending() => $_clearField(3);
  @$pb.TagNumber(3)
  FirmwareSlot ensurePending() => $_ensure(2);

  @$pb.TagNumber(4)
  FirmwareSlot get stable => $_getN(3);
  @$pb.TagNumber(4)
  set stable(FirmwareSlot value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasStable() => $_has(3);
  @$pb.TagNumber(4)
  void clearStable() => $_clearField(4);
  @$pb.TagNumber(4)
  FirmwareSlot ensureStable() => $_ensure(3);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
