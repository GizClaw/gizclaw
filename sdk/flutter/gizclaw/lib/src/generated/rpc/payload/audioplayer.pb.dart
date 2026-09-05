// This is a generated file - do not edit.
//
// Generated from payload/audioplayer.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

/// One playable item. URL is HTTPS, without embedded credentials (1..1024
/// UTF-8 bytes). Display title and opaque catalog ref are optional (128 bytes).
class AudioPlayerItem extends $pb.GeneratedMessage {
  factory AudioPlayerItem({
    $core.String? url,
    $core.String? title,
    $core.String? sourceRef,
  }) {
    final result = create();
    if (url != null) result.url = url;
    if (title != null) result.title = title;
    if (sourceRef != null) result.sourceRef = sourceRef;
    return result;
  }

  AudioPlayerItem._();

  factory AudioPlayerItem.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory AudioPlayerItem.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'AudioPlayerItem',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'url')
    ..aOS(2, _omitFieldNames ? '' : 'title')
    ..aOS(3, _omitFieldNames ? '' : 'sourceRef')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AudioPlayerItem clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AudioPlayerItem copyWith(void Function(AudioPlayerItem) updates) =>
      super.copyWith((message) => updates(message as AudioPlayerItem))
          as AudioPlayerItem;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static AudioPlayerItem create() => AudioPlayerItem._();
  @$core.override
  AudioPlayerItem createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static AudioPlayerItem getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<AudioPlayerItem>(create);
  static AudioPlayerItem? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get url => $_getSZ(0);
  @$pb.TagNumber(1)
  set url($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasUrl() => $_has(0);
  @$pb.TagNumber(1)
  void clearUrl() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get title => $_getSZ(1);
  @$pb.TagNumber(2)
  set title($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasTitle() => $_has(1);
  @$pb.TagNumber(2)
  void clearTitle() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get sourceRef => $_getSZ(2);
  @$pb.TagNumber(3)
  set sourceRef($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSourceRef() => $_has(2);
  @$pb.TagNumber(3)
  void clearSourceRef() => $_clearField(3);
}

/// One complete device snapshot. State is stopped/buffering/playing/ended/error;
/// repeat is off/one/all. Index is zero-based and absent when no track is selected.
/// Unknown duration is absent, not zero. Progress is actual playout time.
class AudioPlayerStatus extends $pb.GeneratedMessage {
  factory AudioPlayerStatus({
    $core.String? state,
    $core.int? currentIndex,
    $fixnum.Int64? positionMs,
    $fixnum.Int64? durationMs,
    $core.String? repeat,
    $core.int? playlistLength,
    $core.int? playlistRevision,
    $core.String? errorCode,
    $core.String? errorMessage,
    $fixnum.Int64? observedAtUnixMs,
  }) {
    final result = create();
    if (state != null) result.state = state;
    if (currentIndex != null) result.currentIndex = currentIndex;
    if (positionMs != null) result.positionMs = positionMs;
    if (durationMs != null) result.durationMs = durationMs;
    if (repeat != null) result.repeat = repeat;
    if (playlistLength != null) result.playlistLength = playlistLength;
    if (playlistRevision != null) result.playlistRevision = playlistRevision;
    if (errorCode != null) result.errorCode = errorCode;
    if (errorMessage != null) result.errorMessage = errorMessage;
    if (observedAtUnixMs != null) result.observedAtUnixMs = observedAtUnixMs;
    return result;
  }

  AudioPlayerStatus._();

  factory AudioPlayerStatus.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory AudioPlayerStatus.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'AudioPlayerStatus',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'state')
    ..aI(2, _omitFieldNames ? '' : 'currentIndex',
        fieldType: $pb.PbFieldType.OU3)
    ..a<$fixnum.Int64>(
        3, _omitFieldNames ? '' : 'positionMs', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(
        4, _omitFieldNames ? '' : 'durationMs', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOS(5, _omitFieldNames ? '' : 'repeat')
    ..aI(6, _omitFieldNames ? '' : 'playlistLength',
        fieldType: $pb.PbFieldType.OU3)
    ..aI(7, _omitFieldNames ? '' : 'playlistRevision',
        fieldType: $pb.PbFieldType.OU3)
    ..aOS(8, _omitFieldNames ? '' : 'errorCode')
    ..aOS(9, _omitFieldNames ? '' : 'errorMessage')
    ..aInt64(10, _omitFieldNames ? '' : 'observedAtUnixMs')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AudioPlayerStatus clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AudioPlayerStatus copyWith(void Function(AudioPlayerStatus) updates) =>
      super.copyWith((message) => updates(message as AudioPlayerStatus))
          as AudioPlayerStatus;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static AudioPlayerStatus create() => AudioPlayerStatus._();
  @$core.override
  AudioPlayerStatus createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static AudioPlayerStatus getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<AudioPlayerStatus>(create);
  static AudioPlayerStatus? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get state => $_getSZ(0);
  @$pb.TagNumber(1)
  set state($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasState() => $_has(0);
  @$pb.TagNumber(1)
  void clearState() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get currentIndex => $_getIZ(1);
  @$pb.TagNumber(2)
  set currentIndex($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCurrentIndex() => $_has(1);
  @$pb.TagNumber(2)
  void clearCurrentIndex() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get positionMs => $_getI64(2);
  @$pb.TagNumber(3)
  set positionMs($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPositionMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearPositionMs() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get durationMs => $_getI64(3);
  @$pb.TagNumber(4)
  set durationMs($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasDurationMs() => $_has(3);
  @$pb.TagNumber(4)
  void clearDurationMs() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get repeat => $_getSZ(4);
  @$pb.TagNumber(5)
  set repeat($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasRepeat() => $_has(4);
  @$pb.TagNumber(5)
  void clearRepeat() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.int get playlistLength => $_getIZ(5);
  @$pb.TagNumber(6)
  set playlistLength($core.int value) => $_setUnsignedInt32(5, value);
  @$pb.TagNumber(6)
  $core.bool hasPlaylistLength() => $_has(5);
  @$pb.TagNumber(6)
  void clearPlaylistLength() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.int get playlistRevision => $_getIZ(6);
  @$pb.TagNumber(7)
  set playlistRevision($core.int value) => $_setUnsignedInt32(6, value);
  @$pb.TagNumber(7)
  $core.bool hasPlaylistRevision() => $_has(6);
  @$pb.TagNumber(7)
  void clearPlaylistRevision() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get errorCode => $_getSZ(7);
  @$pb.TagNumber(8)
  set errorCode($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasErrorCode() => $_has(7);
  @$pb.TagNumber(8)
  void clearErrorCode() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get errorMessage => $_getSZ(8);
  @$pb.TagNumber(9)
  set errorMessage($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasErrorMessage() => $_has(8);
  @$pb.TagNumber(9)
  void clearErrorMessage() => $_clearField(9);

  @$pb.TagNumber(10)
  $fixnum.Int64 get observedAtUnixMs => $_getI64(9);
  @$pb.TagNumber(10)
  set observedAtUnixMs($fixnum.Int64 value) => $_setInt64(9, value);
  @$pb.TagNumber(10)
  $core.bool hasObservedAtUnixMs() => $_has(9);
  @$pb.TagNumber(10)
  void clearObservedAtUnixMs() => $_clearField(10);
}

class ClientDeviceAudioPlayerGetRequest extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerGetRequest() => create();

  ClientDeviceAudioPlayerGetRequest._();

  factory ClientDeviceAudioPlayerGetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerGetRequest copyWith(
          void Function(ClientDeviceAudioPlayerGetRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerGetRequest))
          as ClientDeviceAudioPlayerGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerGetRequest create() =>
      ClientDeviceAudioPlayerGetRequest._();
  @$core.override
  ClientDeviceAudioPlayerGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceAudioPlayerGetRequest>(
          create);
  static ClientDeviceAudioPlayerGetRequest? _defaultInstance;
}

class ClientDeviceAudioPlayerGetResponse extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerGetResponse({
    AudioPlayerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientDeviceAudioPlayerGetResponse._();

  factory ClientDeviceAudioPlayerGetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AudioPlayerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: AudioPlayerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerGetResponse copyWith(
          void Function(ClientDeviceAudioPlayerGetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerGetResponse))
          as ClientDeviceAudioPlayerGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerGetResponse create() =>
      ClientDeviceAudioPlayerGetResponse._();
  @$core.override
  ClientDeviceAudioPlayerGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceAudioPlayerGetResponse>(
          create);
  static ClientDeviceAudioPlayerGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  AudioPlayerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(AudioPlayerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  AudioPlayerStatus ensureValue() => $_ensure(0);
}

class ClientDeviceAudioPlayerPlaylistGetRequest extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerPlaylistGetRequest() => create();

  ClientDeviceAudioPlayerPlaylistGetRequest._();

  factory ClientDeviceAudioPlayerPlaylistGetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerPlaylistGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerPlaylistGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistGetRequest copyWith(
          void Function(ClientDeviceAudioPlayerPlaylistGetRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerPlaylistGetRequest))
          as ClientDeviceAudioPlayerPlaylistGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistGetRequest create() =>
      ClientDeviceAudioPlayerPlaylistGetRequest._();
  @$core.override
  ClientDeviceAudioPlayerPlaylistGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistGetRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerPlaylistGetRequest>(create);
  static ClientDeviceAudioPlayerPlaylistGetRequest? _defaultInstance;
}

class ClientDeviceAudioPlayerPlaylistGetResponse extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerPlaylistGetResponse({
    $core.Iterable<AudioPlayerItem>? items,
    $core.int? playlistRevision,
  }) {
    final result = create();
    if (items != null) result.items.addAll(items);
    if (playlistRevision != null) result.playlistRevision = playlistRevision;
    return result;
  }

  ClientDeviceAudioPlayerPlaylistGetResponse._();

  factory ClientDeviceAudioPlayerPlaylistGetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerPlaylistGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerPlaylistGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<AudioPlayerItem>(1, _omitFieldNames ? '' : 'items',
        subBuilder: AudioPlayerItem.create)
    ..aI(2, _omitFieldNames ? '' : 'playlistRevision',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistGetResponse copyWith(
          void Function(ClientDeviceAudioPlayerPlaylistGetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerPlaylistGetResponse))
          as ClientDeviceAudioPlayerPlaylistGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistGetResponse create() =>
      ClientDeviceAudioPlayerPlaylistGetResponse._();
  @$core.override
  ClientDeviceAudioPlayerPlaylistGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistGetResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerPlaylistGetResponse>(create);
  static ClientDeviceAudioPlayerPlaylistGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<AudioPlayerItem> get items => $_getList(0);

  @$pb.TagNumber(2)
  $core.int get playlistRevision => $_getIZ(1);
  @$pb.TagNumber(2)
  set playlistRevision($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPlaylistRevision() => $_has(1);
  @$pb.TagNumber(2)
  void clearPlaylistRevision() => $_clearField(2);
}

/// Validate and reserve capacity before stopping/replacing. Failure preserves
/// both the previous playlist and playback. Empty items clears the playlist.
class ClientDeviceAudioPlayerPlaylistSetRequest extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerPlaylistSetRequest({
    $core.Iterable<AudioPlayerItem>? items,
  }) {
    final result = create();
    if (items != null) result.items.addAll(items);
    return result;
  }

  ClientDeviceAudioPlayerPlaylistSetRequest._();

  factory ClientDeviceAudioPlayerPlaylistSetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerPlaylistSetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerPlaylistSetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<AudioPlayerItem>(1, _omitFieldNames ? '' : 'items',
        subBuilder: AudioPlayerItem.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistSetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistSetRequest copyWith(
          void Function(ClientDeviceAudioPlayerPlaylistSetRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerPlaylistSetRequest))
          as ClientDeviceAudioPlayerPlaylistSetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistSetRequest create() =>
      ClientDeviceAudioPlayerPlaylistSetRequest._();
  @$core.override
  ClientDeviceAudioPlayerPlaylistSetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistSetRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerPlaylistSetRequest>(create);
  static ClientDeviceAudioPlayerPlaylistSetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<AudioPlayerItem> get items => $_getList(0);
}

class ClientDeviceAudioPlayerPlaylistSetResponse extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerPlaylistSetResponse({
    AudioPlayerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientDeviceAudioPlayerPlaylistSetResponse._();

  factory ClientDeviceAudioPlayerPlaylistSetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerPlaylistSetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerPlaylistSetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AudioPlayerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: AudioPlayerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistSetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistSetResponse copyWith(
          void Function(ClientDeviceAudioPlayerPlaylistSetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerPlaylistSetResponse))
          as ClientDeviceAudioPlayerPlaylistSetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistSetResponse create() =>
      ClientDeviceAudioPlayerPlaylistSetResponse._();
  @$core.override
  ClientDeviceAudioPlayerPlaylistSetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistSetResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerPlaylistSetResponse>(create);
  static ClientDeviceAudioPlayerPlaylistSetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  AudioPlayerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(AudioPlayerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  AudioPlayerStatus ensureValue() => $_ensure(0);
}

/// Atomically append 1..32 items, respecting the total 32-item capacity. Does not
/// interrupt playback or start a stopped/ended player. Never automatically retry.
class ClientDeviceAudioPlayerPlaylistAppendRequest
    extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerPlaylistAppendRequest({
    $core.Iterable<AudioPlayerItem>? items,
  }) {
    final result = create();
    if (items != null) result.items.addAll(items);
    return result;
  }

  ClientDeviceAudioPlayerPlaylistAppendRequest._();

  factory ClientDeviceAudioPlayerPlaylistAppendRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerPlaylistAppendRequest.fromJson(
          $core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerPlaylistAppendRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<AudioPlayerItem>(1, _omitFieldNames ? '' : 'items',
        subBuilder: AudioPlayerItem.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistAppendRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistAppendRequest copyWith(
          void Function(ClientDeviceAudioPlayerPlaylistAppendRequest)
              updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerPlaylistAppendRequest))
          as ClientDeviceAudioPlayerPlaylistAppendRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistAppendRequest create() =>
      ClientDeviceAudioPlayerPlaylistAppendRequest._();
  @$core.override
  ClientDeviceAudioPlayerPlaylistAppendRequest createEmptyInstance() =>
      create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistAppendRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerPlaylistAppendRequest>(create);
  static ClientDeviceAudioPlayerPlaylistAppendRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<AudioPlayerItem> get items => $_getList(0);
}

class ClientDeviceAudioPlayerPlaylistAppendResponse
    extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerPlaylistAppendResponse({
    AudioPlayerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientDeviceAudioPlayerPlaylistAppendResponse._();

  factory ClientDeviceAudioPlayerPlaylistAppendResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerPlaylistAppendResponse.fromJson(
          $core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerPlaylistAppendResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AudioPlayerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: AudioPlayerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistAppendResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlaylistAppendResponse copyWith(
          void Function(ClientDeviceAudioPlayerPlaylistAppendResponse)
              updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerPlaylistAppendResponse))
          as ClientDeviceAudioPlayerPlaylistAppendResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistAppendResponse create() =>
      ClientDeviceAudioPlayerPlaylistAppendResponse._();
  @$core.override
  ClientDeviceAudioPlayerPlaylistAppendResponse createEmptyInstance() =>
      create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlaylistAppendResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerPlaylistAppendResponse>(create);
  static ClientDeviceAudioPlayerPlaylistAppendResponse? _defaultInstance;

  @$pb.TagNumber(1)
  AudioPlayerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(AudioPlayerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  AudioPlayerStatus ensureValue() => $_ensure(0);
}

/// Accept playback from the beginning of the selected track, replacing current
/// playback. Response acknowledges acceptance; telemetry reports actual start.
class ClientDeviceAudioPlayerPlayRequest extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerPlayRequest({
    $core.int? index,
  }) {
    final result = create();
    if (index != null) result.index = index;
    return result;
  }

  ClientDeviceAudioPlayerPlayRequest._();

  factory ClientDeviceAudioPlayerPlayRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerPlayRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerPlayRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'index', fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlayRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlayRequest copyWith(
          void Function(ClientDeviceAudioPlayerPlayRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerPlayRequest))
          as ClientDeviceAudioPlayerPlayRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlayRequest create() =>
      ClientDeviceAudioPlayerPlayRequest._();
  @$core.override
  ClientDeviceAudioPlayerPlayRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlayRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceAudioPlayerPlayRequest>(
          create);
  static ClientDeviceAudioPlayerPlayRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get index => $_getIZ(0);
  @$pb.TagNumber(1)
  set index($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasIndex() => $_has(0);
  @$pb.TagNumber(1)
  void clearIndex() => $_clearField(1);
}

class ClientDeviceAudioPlayerPlayResponse extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerPlayResponse({
    AudioPlayerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientDeviceAudioPlayerPlayResponse._();

  factory ClientDeviceAudioPlayerPlayResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerPlayResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerPlayResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AudioPlayerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: AudioPlayerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlayResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerPlayResponse copyWith(
          void Function(ClientDeviceAudioPlayerPlayResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerPlayResponse))
          as ClientDeviceAudioPlayerPlayResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlayResponse create() =>
      ClientDeviceAudioPlayerPlayResponse._();
  @$core.override
  ClientDeviceAudioPlayerPlayResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerPlayResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerPlayResponse>(create);
  static ClientDeviceAudioPlayerPlayResponse? _defaultInstance;

  @$pb.TagNumber(1)
  AudioPlayerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(AudioPlayerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  AudioPlayerStatus ensureValue() => $_ensure(0);
}

/// Idempotently stop, retaining the playlist and repeat mode.
class ClientDeviceAudioPlayerStopRequest extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerStopRequest() => create();

  ClientDeviceAudioPlayerStopRequest._();

  factory ClientDeviceAudioPlayerStopRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerStopRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerStopRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerStopRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerStopRequest copyWith(
          void Function(ClientDeviceAudioPlayerStopRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerStopRequest))
          as ClientDeviceAudioPlayerStopRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerStopRequest create() =>
      ClientDeviceAudioPlayerStopRequest._();
  @$core.override
  ClientDeviceAudioPlayerStopRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerStopRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceAudioPlayerStopRequest>(
          create);
  static ClientDeviceAudioPlayerStopRequest? _defaultInstance;
}

class ClientDeviceAudioPlayerStopResponse extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerStopResponse({
    AudioPlayerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientDeviceAudioPlayerStopResponse._();

  factory ClientDeviceAudioPlayerStopResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerStopResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerStopResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AudioPlayerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: AudioPlayerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerStopResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerStopResponse copyWith(
          void Function(ClientDeviceAudioPlayerStopResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerStopResponse))
          as ClientDeviceAudioPlayerStopResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerStopResponse create() =>
      ClientDeviceAudioPlayerStopResponse._();
  @$core.override
  ClientDeviceAudioPlayerStopResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerStopResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerStopResponse>(create);
  static ClientDeviceAudioPlayerStopResponse? _defaultInstance;

  @$pb.TagNumber(1)
  AudioPlayerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(AudioPlayerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  AudioPlayerStatus ensureValue() => $_ensure(0);
}

/// off: stop after the list; one: repeat current track; all: repeat the list.
/// Changing the mode does not interrupt the current track.
class ClientDeviceAudioPlayerModeSetRequest extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerModeSetRequest({
    $core.String? repeat,
  }) {
    final result = create();
    if (repeat != null) result.repeat = repeat;
    return result;
  }

  ClientDeviceAudioPlayerModeSetRequest._();

  factory ClientDeviceAudioPlayerModeSetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerModeSetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerModeSetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'repeat')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerModeSetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerModeSetRequest copyWith(
          void Function(ClientDeviceAudioPlayerModeSetRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerModeSetRequest))
          as ClientDeviceAudioPlayerModeSetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerModeSetRequest create() =>
      ClientDeviceAudioPlayerModeSetRequest._();
  @$core.override
  ClientDeviceAudioPlayerModeSetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerModeSetRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerModeSetRequest>(create);
  static ClientDeviceAudioPlayerModeSetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get repeat => $_getSZ(0);
  @$pb.TagNumber(1)
  set repeat($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRepeat() => $_has(0);
  @$pb.TagNumber(1)
  void clearRepeat() => $_clearField(1);
}

class ClientDeviceAudioPlayerModeSetResponse extends $pb.GeneratedMessage {
  factory ClientDeviceAudioPlayerModeSetResponse({
    AudioPlayerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientDeviceAudioPlayerModeSetResponse._();

  factory ClientDeviceAudioPlayerModeSetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceAudioPlayerModeSetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceAudioPlayerModeSetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AudioPlayerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: AudioPlayerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerModeSetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceAudioPlayerModeSetResponse copyWith(
          void Function(ClientDeviceAudioPlayerModeSetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientDeviceAudioPlayerModeSetResponse))
          as ClientDeviceAudioPlayerModeSetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerModeSetResponse create() =>
      ClientDeviceAudioPlayerModeSetResponse._();
  @$core.override
  ClientDeviceAudioPlayerModeSetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceAudioPlayerModeSetResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientDeviceAudioPlayerModeSetResponse>(create);
  static ClientDeviceAudioPlayerModeSetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  AudioPlayerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(AudioPlayerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  AudioPlayerStatus ensureValue() => $_ensure(0);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
