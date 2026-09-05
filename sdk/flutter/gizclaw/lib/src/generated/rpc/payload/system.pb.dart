// This is a generated file - do not edit.
//
// Generated from payload/system.proto.

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

class ClientGetIdentifiersRequest extends $pb.GeneratedMessage {
  factory ClientGetIdentifiersRequest() => create();

  ClientGetIdentifiersRequest._();

  factory ClientGetIdentifiersRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientGetIdentifiersRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientGetIdentifiersRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGetIdentifiersRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGetIdentifiersRequest copyWith(
          void Function(ClientGetIdentifiersRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientGetIdentifiersRequest))
          as ClientGetIdentifiersRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientGetIdentifiersRequest create() =>
      ClientGetIdentifiersRequest._();
  @$core.override
  ClientGetIdentifiersRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientGetIdentifiersRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientGetIdentifiersRequest>(create);
  static ClientGetIdentifiersRequest? _defaultInstance;
}

class ClientGetIdentifiersResponse extends $pb.GeneratedMessage {
  factory ClientGetIdentifiersResponse({
    DeviceIdentifiers? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientGetIdentifiersResponse._();

  factory ClientGetIdentifiersResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientGetIdentifiersResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientGetIdentifiersResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DeviceIdentifiers>(1, _omitFieldNames ? '' : 'value',
        subBuilder: DeviceIdentifiers.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGetIdentifiersResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGetIdentifiersResponse copyWith(
          void Function(ClientGetIdentifiersResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientGetIdentifiersResponse))
          as ClientGetIdentifiersResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientGetIdentifiersResponse create() =>
      ClientGetIdentifiersResponse._();
  @$core.override
  ClientGetIdentifiersResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientGetIdentifiersResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientGetIdentifiersResponse>(create);
  static ClientGetIdentifiersResponse? _defaultInstance;

  @$pb.TagNumber(1)
  DeviceIdentifiers get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(DeviceIdentifiers value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  DeviceIdentifiers ensureValue() => $_ensure(0);
}

class ClientGetInfoRequest extends $pb.GeneratedMessage {
  factory ClientGetInfoRequest() => create();

  ClientGetInfoRequest._();

  factory ClientGetInfoRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientGetInfoRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientGetInfoRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGetInfoRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGetInfoRequest copyWith(void Function(ClientGetInfoRequest) updates) =>
      super.copyWith((message) => updates(message as ClientGetInfoRequest))
          as ClientGetInfoRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientGetInfoRequest create() => ClientGetInfoRequest._();
  @$core.override
  ClientGetInfoRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientGetInfoRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientGetInfoRequest>(create);
  static ClientGetInfoRequest? _defaultInstance;
}

class ClientGetInfoResponse extends $pb.GeneratedMessage {
  factory ClientGetInfoResponse({
    HardwareInfo? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientGetInfoResponse._();

  factory ClientGetInfoResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientGetInfoResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientGetInfoResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<HardwareInfo>(1, _omitFieldNames ? '' : 'value',
        subBuilder: HardwareInfo.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGetInfoResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGetInfoResponse copyWith(
          void Function(ClientGetInfoResponse) updates) =>
      super.copyWith((message) => updates(message as ClientGetInfoResponse))
          as ClientGetInfoResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientGetInfoResponse create() => ClientGetInfoResponse._();
  @$core.override
  ClientGetInfoResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientGetInfoResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientGetInfoResponse>(create);
  static ClientGetInfoResponse? _defaultInstance;

  @$pb.TagNumber(1)
  HardwareInfo get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(HardwareInfo value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  HardwareInfo ensureValue() => $_ensure(0);
}

class ClientDeviceStatusGetRequest extends $pb.GeneratedMessage {
  factory ClientDeviceStatusGetRequest() => create();

  ClientDeviceStatusGetRequest._();

  factory ClientDeviceStatusGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceStatusGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceStatusGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceStatusGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceStatusGetRequest copyWith(
          void Function(ClientDeviceStatusGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceStatusGetRequest))
          as ClientDeviceStatusGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceStatusGetRequest create() =>
      ClientDeviceStatusGetRequest._();
  @$core.override
  ClientDeviceStatusGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceStatusGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceStatusGetRequest>(create);
  static ClientDeviceStatusGetRequest? _defaultInstance;
}

class ClientDeviceStatusGetResponse extends $pb.GeneratedMessage {
  factory ClientDeviceStatusGetResponse({
    PeerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientDeviceStatusGetResponse._();

  factory ClientDeviceStatusGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceStatusGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceStatusGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceStatusGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceStatusGetResponse copyWith(
          void Function(ClientDeviceStatusGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceStatusGetResponse))
          as ClientDeviceStatusGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceStatusGetResponse create() =>
      ClientDeviceStatusGetResponse._();
  @$core.override
  ClientDeviceStatusGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceStatusGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceStatusGetResponse>(create);
  static ClientDeviceStatusGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerStatus ensureValue() => $_ensure(0);
}

class ClientDeviceVolumeSetRequest extends $pb.GeneratedMessage {
  factory ClientDeviceVolumeSetRequest({
    $fixnum.Int64? level,
    $core.bool? muted,
  }) {
    final result = create();
    if (level != null) result.level = level;
    if (muted != null) result.muted = muted;
    return result;
  }

  ClientDeviceVolumeSetRequest._();

  factory ClientDeviceVolumeSetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceVolumeSetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceVolumeSetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'level')
    ..aOB(2, _omitFieldNames ? '' : 'muted')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceVolumeSetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceVolumeSetRequest copyWith(
          void Function(ClientDeviceVolumeSetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceVolumeSetRequest))
          as ClientDeviceVolumeSetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceVolumeSetRequest create() =>
      ClientDeviceVolumeSetRequest._();
  @$core.override
  ClientDeviceVolumeSetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceVolumeSetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceVolumeSetRequest>(create);
  static ClientDeviceVolumeSetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get level => $_getI64(0);
  @$pb.TagNumber(1)
  set level($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasLevel() => $_has(0);
  @$pb.TagNumber(1)
  void clearLevel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get muted => $_getBF(1);
  @$pb.TagNumber(2)
  set muted($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMuted() => $_has(1);
  @$pb.TagNumber(2)
  void clearMuted() => $_clearField(2);
}

class ClientDeviceVolumeSetResponse extends $pb.GeneratedMessage {
  factory ClientDeviceVolumeSetResponse({
    PeerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientDeviceVolumeSetResponse._();

  factory ClientDeviceVolumeSetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceVolumeSetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceVolumeSetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceVolumeSetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceVolumeSetResponse copyWith(
          void Function(ClientDeviceVolumeSetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceVolumeSetResponse))
          as ClientDeviceVolumeSetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceVolumeSetResponse create() =>
      ClientDeviceVolumeSetResponse._();
  @$core.override
  ClientDeviceVolumeSetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceVolumeSetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceVolumeSetResponse>(create);
  static ClientDeviceVolumeSetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerStatus ensureValue() => $_ensure(0);
}

class ClientDeviceSoundPlayRequest extends $pb.GeneratedMessage {
  factory ClientDeviceSoundPlayRequest({
    $core.String? sound,
    $fixnum.Int64? durationMs,
  }) {
    final result = create();
    if (sound != null) result.sound = sound;
    if (durationMs != null) result.durationMs = durationMs;
    return result;
  }

  ClientDeviceSoundPlayRequest._();

  factory ClientDeviceSoundPlayRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceSoundPlayRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceSoundPlayRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'sound')
    ..aInt64(2, _omitFieldNames ? '' : 'durationMs')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceSoundPlayRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceSoundPlayRequest copyWith(
          void Function(ClientDeviceSoundPlayRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceSoundPlayRequest))
          as ClientDeviceSoundPlayRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceSoundPlayRequest create() =>
      ClientDeviceSoundPlayRequest._();
  @$core.override
  ClientDeviceSoundPlayRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceSoundPlayRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceSoundPlayRequest>(create);
  static ClientDeviceSoundPlayRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sound => $_getSZ(0);
  @$pb.TagNumber(1)
  set sound($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSound() => $_has(0);
  @$pb.TagNumber(1)
  void clearSound() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get durationMs => $_getI64(1);
  @$pb.TagNumber(2)
  set durationMs($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDurationMs() => $_has(1);
  @$pb.TagNumber(2)
  void clearDurationMs() => $_clearField(2);
}

class ClientDeviceSoundPlayResponse extends $pb.GeneratedMessage {
  factory ClientDeviceSoundPlayResponse() => create();

  ClientDeviceSoundPlayResponse._();

  factory ClientDeviceSoundPlayResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceSoundPlayResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceSoundPlayResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceSoundPlayResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceSoundPlayResponse copyWith(
          void Function(ClientDeviceSoundPlayResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceSoundPlayResponse))
          as ClientDeviceSoundPlayResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceSoundPlayResponse create() =>
      ClientDeviceSoundPlayResponse._();
  @$core.override
  ClientDeviceSoundPlayResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceSoundPlayResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceSoundPlayResponse>(create);
  static ClientDeviceSoundPlayResponse? _defaultInstance;
}

class ClientDeviceRebootRequest extends $pb.GeneratedMessage {
  factory ClientDeviceRebootRequest({
    $fixnum.Int64? delayMs,
  }) {
    final result = create();
    if (delayMs != null) result.delayMs = delayMs;
    return result;
  }

  ClientDeviceRebootRequest._();

  factory ClientDeviceRebootRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceRebootRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceRebootRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'delayMs')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceRebootRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceRebootRequest copyWith(
          void Function(ClientDeviceRebootRequest) updates) =>
      super.copyWith((message) => updates(message as ClientDeviceRebootRequest))
          as ClientDeviceRebootRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceRebootRequest create() => ClientDeviceRebootRequest._();
  @$core.override
  ClientDeviceRebootRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceRebootRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceRebootRequest>(create);
  static ClientDeviceRebootRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get delayMs => $_getI64(0);
  @$pb.TagNumber(1)
  set delayMs($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDelayMs() => $_has(0);
  @$pb.TagNumber(1)
  void clearDelayMs() => $_clearField(1);
}

class ClientDeviceRebootResponse extends $pb.GeneratedMessage {
  factory ClientDeviceRebootResponse() => create();

  ClientDeviceRebootResponse._();

  factory ClientDeviceRebootResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceRebootResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceRebootResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceRebootResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceRebootResponse copyWith(
          void Function(ClientDeviceRebootResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceRebootResponse))
          as ClientDeviceRebootResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceRebootResponse create() => ClientDeviceRebootResponse._();
  @$core.override
  ClientDeviceRebootResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceRebootResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceRebootResponse>(create);
  static ClientDeviceRebootResponse? _defaultInstance;
}

class WifiStatus extends $pb.GeneratedMessage {
  factory WifiStatus({
    $core.bool? connected,
    $core.String? ssid,
    $fixnum.Int64? rssiDbm,
    $core.String? ip,
    $core.String? bssid,
  }) {
    final result = create();
    if (connected != null) result.connected = connected;
    if (ssid != null) result.ssid = ssid;
    if (rssiDbm != null) result.rssiDbm = rssiDbm;
    if (ip != null) result.ip = ip;
    if (bssid != null) result.bssid = bssid;
    return result;
  }

  WifiStatus._();

  factory WifiStatus.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WifiStatus.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WifiStatus',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'connected')
    ..aOS(2, _omitFieldNames ? '' : 'ssid')
    ..aInt64(3, _omitFieldNames ? '' : 'rssiDbm')
    ..aOS(4, _omitFieldNames ? '' : 'ip')
    ..aOS(5, _omitFieldNames ? '' : 'bssid')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WifiStatus clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WifiStatus copyWith(void Function(WifiStatus) updates) =>
      super.copyWith((message) => updates(message as WifiStatus)) as WifiStatus;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WifiStatus create() => WifiStatus._();
  @$core.override
  WifiStatus createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WifiStatus getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WifiStatus>(create);
  static WifiStatus? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get connected => $_getBF(0);
  @$pb.TagNumber(1)
  set connected($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasConnected() => $_has(0);
  @$pb.TagNumber(1)
  void clearConnected() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get ssid => $_getSZ(1);
  @$pb.TagNumber(2)
  set ssid($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSsid() => $_has(1);
  @$pb.TagNumber(2)
  void clearSsid() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get rssiDbm => $_getI64(2);
  @$pb.TagNumber(3)
  set rssiDbm($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRssiDbm() => $_has(2);
  @$pb.TagNumber(3)
  void clearRssiDbm() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get ip => $_getSZ(3);
  @$pb.TagNumber(4)
  set ip($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasIp() => $_has(3);
  @$pb.TagNumber(4)
  void clearIp() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get bssid => $_getSZ(4);
  @$pb.TagNumber(5)
  set bssid($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasBssid() => $_has(4);
  @$pb.TagNumber(5)
  void clearBssid() => $_clearField(5);
}

class WifiSavedNetwork extends $pb.GeneratedMessage {
  factory WifiSavedNetwork({
    $core.String? ssid,
  }) {
    final result = create();
    if (ssid != null) result.ssid = ssid;
    return result;
  }

  WifiSavedNetwork._();

  factory WifiSavedNetwork.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WifiSavedNetwork.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WifiSavedNetwork',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'ssid')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WifiSavedNetwork clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WifiSavedNetwork copyWith(void Function(WifiSavedNetwork) updates) =>
      super.copyWith((message) => updates(message as WifiSavedNetwork))
          as WifiSavedNetwork;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WifiSavedNetwork create() => WifiSavedNetwork._();
  @$core.override
  WifiSavedNetwork createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WifiSavedNetwork getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WifiSavedNetwork>(create);
  static WifiSavedNetwork? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get ssid => $_getSZ(0);
  @$pb.TagNumber(1)
  set ssid($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSsid() => $_has(0);
  @$pb.TagNumber(1)
  void clearSsid() => $_clearField(1);
}

class ClientWifiStatusGetRequest extends $pb.GeneratedMessage {
  factory ClientWifiStatusGetRequest() => create();

  ClientWifiStatusGetRequest._();

  factory ClientWifiStatusGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiStatusGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiStatusGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiStatusGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiStatusGetRequest copyWith(
          void Function(ClientWifiStatusGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientWifiStatusGetRequest))
          as ClientWifiStatusGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiStatusGetRequest create() => ClientWifiStatusGetRequest._();
  @$core.override
  ClientWifiStatusGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiStatusGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiStatusGetRequest>(create);
  static ClientWifiStatusGetRequest? _defaultInstance;
}

class ClientWifiStatusGetResponse extends $pb.GeneratedMessage {
  factory ClientWifiStatusGetResponse({
    WifiStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ClientWifiStatusGetResponse._();

  factory ClientWifiStatusGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiStatusGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiStatusGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<WifiStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: WifiStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiStatusGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiStatusGetResponse copyWith(
          void Function(ClientWifiStatusGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientWifiStatusGetResponse))
          as ClientWifiStatusGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiStatusGetResponse create() =>
      ClientWifiStatusGetResponse._();
  @$core.override
  ClientWifiStatusGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiStatusGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiStatusGetResponse>(create);
  static ClientWifiStatusGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  WifiStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(WifiStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  WifiStatus ensureValue() => $_ensure(0);
}

class ClientWifiSavedListRequest extends $pb.GeneratedMessage {
  factory ClientWifiSavedListRequest() => create();

  ClientWifiSavedListRequest._();

  factory ClientWifiSavedListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiSavedListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiSavedListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiSavedListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiSavedListRequest copyWith(
          void Function(ClientWifiSavedListRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientWifiSavedListRequest))
          as ClientWifiSavedListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiSavedListRequest create() => ClientWifiSavedListRequest._();
  @$core.override
  ClientWifiSavedListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiSavedListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiSavedListRequest>(create);
  static ClientWifiSavedListRequest? _defaultInstance;
}

class ClientWifiSavedListResponse extends $pb.GeneratedMessage {
  factory ClientWifiSavedListResponse({
    $core.Iterable<WifiSavedNetwork>? networks,
  }) {
    final result = create();
    if (networks != null) result.networks.addAll(networks);
    return result;
  }

  ClientWifiSavedListResponse._();

  factory ClientWifiSavedListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiSavedListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiSavedListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<WifiSavedNetwork>(1, _omitFieldNames ? '' : 'networks',
        subBuilder: WifiSavedNetwork.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiSavedListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiSavedListResponse copyWith(
          void Function(ClientWifiSavedListResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientWifiSavedListResponse))
          as ClientWifiSavedListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiSavedListResponse create() =>
      ClientWifiSavedListResponse._();
  @$core.override
  ClientWifiSavedListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiSavedListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiSavedListResponse>(create);
  static ClientWifiSavedListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<WifiSavedNetwork> get networks => $_getList(0);
}

class ClientWifiSavedForgetRequest extends $pb.GeneratedMessage {
  factory ClientWifiSavedForgetRequest({
    $core.String? ssid,
  }) {
    final result = create();
    if (ssid != null) result.ssid = ssid;
    return result;
  }

  ClientWifiSavedForgetRequest._();

  factory ClientWifiSavedForgetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiSavedForgetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiSavedForgetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'ssid')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiSavedForgetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiSavedForgetRequest copyWith(
          void Function(ClientWifiSavedForgetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientWifiSavedForgetRequest))
          as ClientWifiSavedForgetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiSavedForgetRequest create() =>
      ClientWifiSavedForgetRequest._();
  @$core.override
  ClientWifiSavedForgetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiSavedForgetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiSavedForgetRequest>(create);
  static ClientWifiSavedForgetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get ssid => $_getSZ(0);
  @$pb.TagNumber(1)
  set ssid($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSsid() => $_has(0);
  @$pb.TagNumber(1)
  void clearSsid() => $_clearField(1);
}

class ClientWifiSavedForgetResponse extends $pb.GeneratedMessage {
  factory ClientWifiSavedForgetResponse() => create();

  ClientWifiSavedForgetResponse._();

  factory ClientWifiSavedForgetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiSavedForgetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiSavedForgetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiSavedForgetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiSavedForgetResponse copyWith(
          void Function(ClientWifiSavedForgetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientWifiSavedForgetResponse))
          as ClientWifiSavedForgetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiSavedForgetResponse create() =>
      ClientWifiSavedForgetResponse._();
  @$core.override
  ClientWifiSavedForgetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiSavedForgetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiSavedForgetResponse>(create);
  static ClientWifiSavedForgetResponse? _defaultInstance;
}

class WifiScanResult extends $pb.GeneratedMessage {
  factory WifiScanResult({
    $core.String? ssid,
    $core.String? bssid,
    $fixnum.Int64? rssiDbm,
    $fixnum.Int64? frequencyMhz,
    $core.String? security,
  }) {
    final result = create();
    if (ssid != null) result.ssid = ssid;
    if (bssid != null) result.bssid = bssid;
    if (rssiDbm != null) result.rssiDbm = rssiDbm;
    if (frequencyMhz != null) result.frequencyMhz = frequencyMhz;
    if (security != null) result.security = security;
    return result;
  }

  WifiScanResult._();

  factory WifiScanResult.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WifiScanResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WifiScanResult',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'ssid')
    ..aOS(2, _omitFieldNames ? '' : 'bssid')
    ..aInt64(3, _omitFieldNames ? '' : 'rssiDbm')
    ..aInt64(4, _omitFieldNames ? '' : 'frequencyMhz')
    ..aOS(5, _omitFieldNames ? '' : 'security')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WifiScanResult clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WifiScanResult copyWith(void Function(WifiScanResult) updates) =>
      super.copyWith((message) => updates(message as WifiScanResult))
          as WifiScanResult;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WifiScanResult create() => WifiScanResult._();
  @$core.override
  WifiScanResult createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WifiScanResult getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WifiScanResult>(create);
  static WifiScanResult? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get ssid => $_getSZ(0);
  @$pb.TagNumber(1)
  set ssid($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSsid() => $_has(0);
  @$pb.TagNumber(1)
  void clearSsid() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get bssid => $_getSZ(1);
  @$pb.TagNumber(2)
  set bssid($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBssid() => $_has(1);
  @$pb.TagNumber(2)
  void clearBssid() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get rssiDbm => $_getI64(2);
  @$pb.TagNumber(3)
  set rssiDbm($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRssiDbm() => $_has(2);
  @$pb.TagNumber(3)
  void clearRssiDbm() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get frequencyMhz => $_getI64(3);
  @$pb.TagNumber(4)
  set frequencyMhz($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasFrequencyMhz() => $_has(3);
  @$pb.TagNumber(4)
  void clearFrequencyMhz() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get security => $_getSZ(4);
  @$pb.TagNumber(5)
  set security($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSecurity() => $_has(4);
  @$pb.TagNumber(5)
  void clearSecurity() => $_clearField(5);
}

class ClientWifiScanRequest extends $pb.GeneratedMessage {
  factory ClientWifiScanRequest({
    $fixnum.Int64? timeoutMs,
  }) {
    final result = create();
    if (timeoutMs != null) result.timeoutMs = timeoutMs;
    return result;
  }

  ClientWifiScanRequest._();

  factory ClientWifiScanRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiScanRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiScanRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'timeoutMs')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiScanRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiScanRequest copyWith(
          void Function(ClientWifiScanRequest) updates) =>
      super.copyWith((message) => updates(message as ClientWifiScanRequest))
          as ClientWifiScanRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiScanRequest create() => ClientWifiScanRequest._();
  @$core.override
  ClientWifiScanRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiScanRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiScanRequest>(create);
  static ClientWifiScanRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get timeoutMs => $_getI64(0);
  @$pb.TagNumber(1)
  set timeoutMs($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasTimeoutMs() => $_has(0);
  @$pb.TagNumber(1)
  void clearTimeoutMs() => $_clearField(1);
}

class ClientWifiScanResponse extends $pb.GeneratedMessage {
  factory ClientWifiScanResponse({
    $core.Iterable<WifiScanResult>? networks,
  }) {
    final result = create();
    if (networks != null) result.networks.addAll(networks);
    return result;
  }

  ClientWifiScanResponse._();

  factory ClientWifiScanResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiScanResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiScanResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<WifiScanResult>(1, _omitFieldNames ? '' : 'networks',
        subBuilder: WifiScanResult.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiScanResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiScanResponse copyWith(
          void Function(ClientWifiScanResponse) updates) =>
      super.copyWith((message) => updates(message as ClientWifiScanResponse))
          as ClientWifiScanResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiScanResponse create() => ClientWifiScanResponse._();
  @$core.override
  ClientWifiScanResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiScanResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiScanResponse>(create);
  static ClientWifiScanResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<WifiScanResult> get networks => $_getList(0);
}

class ClientWifiConnectRequest extends $pb.GeneratedMessage {
  factory ClientWifiConnectRequest({
    $core.String? ssid,
    $core.String? passphrase,
  }) {
    final result = create();
    if (ssid != null) result.ssid = ssid;
    if (passphrase != null) result.passphrase = passphrase;
    return result;
  }

  ClientWifiConnectRequest._();

  factory ClientWifiConnectRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiConnectRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiConnectRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'ssid')
    ..aOS(2, _omitFieldNames ? '' : 'passphrase')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiConnectRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiConnectRequest copyWith(
          void Function(ClientWifiConnectRequest) updates) =>
      super.copyWith((message) => updates(message as ClientWifiConnectRequest))
          as ClientWifiConnectRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiConnectRequest create() => ClientWifiConnectRequest._();
  @$core.override
  ClientWifiConnectRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiConnectRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiConnectRequest>(create);
  static ClientWifiConnectRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get ssid => $_getSZ(0);
  @$pb.TagNumber(1)
  set ssid($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSsid() => $_has(0);
  @$pb.TagNumber(1)
  void clearSsid() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get passphrase => $_getSZ(1);
  @$pb.TagNumber(2)
  set passphrase($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPassphrase() => $_has(1);
  @$pb.TagNumber(2)
  void clearPassphrase() => $_clearField(2);
}

class ClientWifiConnectResponse extends $pb.GeneratedMessage {
  factory ClientWifiConnectResponse() => create();

  ClientWifiConnectResponse._();

  factory ClientWifiConnectResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientWifiConnectResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientWifiConnectResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiConnectResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientWifiConnectResponse copyWith(
          void Function(ClientWifiConnectResponse) updates) =>
      super.copyWith((message) => updates(message as ClientWifiConnectResponse))
          as ClientWifiConnectResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientWifiConnectResponse create() => ClientWifiConnectResponse._();
  @$core.override
  ClientWifiConnectResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientWifiConnectResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientWifiConnectResponse>(create);
  static ClientWifiConnectResponse? _defaultInstance;
}

class DeviceInfo extends $pb.GeneratedMessage {
  factory DeviceInfo({
    HardwareInfo? hardware,
    $core.String? name,
    $core.String? emoji,
    DeviceIdentifiers? identifiers,
    $core.String? debugMode,
  }) {
    final result = create();
    if (hardware != null) result.hardware = hardware;
    if (name != null) result.name = name;
    if (emoji != null) result.emoji = emoji;
    if (identifiers != null) result.identifiers = identifiers;
    if (debugMode != null) result.debugMode = debugMode;
    return result;
  }

  DeviceInfo._();

  factory DeviceInfo.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DeviceInfo.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DeviceInfo',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<HardwareInfo>(1, _omitFieldNames ? '' : 'hardware',
        subBuilder: HardwareInfo.create)
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..aOS(4, _omitFieldNames ? '' : 'emoji')
    ..aOM<DeviceIdentifiers>(5, _omitFieldNames ? '' : 'identifiers',
        subBuilder: DeviceIdentifiers.create)
    ..aOS(6, _omitFieldNames ? '' : 'debugMode')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceInfo clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceInfo copyWith(void Function(DeviceInfo) updates) =>
      super.copyWith((message) => updates(message as DeviceInfo)) as DeviceInfo;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DeviceInfo create() => DeviceInfo._();
  @$core.override
  DeviceInfo createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DeviceInfo getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DeviceInfo>(create);
  static DeviceInfo? _defaultInstance;

  @$pb.TagNumber(1)
  HardwareInfo get hardware => $_getN(0);
  @$pb.TagNumber(1)
  set hardware(HardwareInfo value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasHardware() => $_has(0);
  @$pb.TagNumber(1)
  void clearHardware() => $_clearField(1);
  @$pb.TagNumber(1)
  HardwareInfo ensureHardware() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);

  @$pb.TagNumber(4)
  $core.String get emoji => $_getSZ(2);
  @$pb.TagNumber(4)
  set emoji($core.String value) => $_setString(2, value);
  @$pb.TagNumber(4)
  $core.bool hasEmoji() => $_has(2);
  @$pb.TagNumber(4)
  void clearEmoji() => $_clearField(4);

  @$pb.TagNumber(5)
  DeviceIdentifiers get identifiers => $_getN(3);
  @$pb.TagNumber(5)
  set identifiers(DeviceIdentifiers value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasIdentifiers() => $_has(3);
  @$pb.TagNumber(5)
  void clearIdentifiers() => $_clearField(5);
  @$pb.TagNumber(5)
  DeviceIdentifiers ensureIdentifiers() => $_ensure(3);

  @$pb.TagNumber(6)
  $core.String get debugMode => $_getSZ(4);
  @$pb.TagNumber(6)
  set debugMode($core.String value) => $_setString(4, value);
  @$pb.TagNumber(6)
  $core.bool hasDebugMode() => $_has(4);
  @$pb.TagNumber(6)
  void clearDebugMode() => $_clearField(6);
}

class DeviceProfile extends $pb.GeneratedMessage {
  factory DeviceProfile({
    $core.String? name,
    $core.String? emoji,
    $core.String? debugMode,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (emoji != null) result.emoji = emoji;
    if (debugMode != null) result.debugMode = debugMode;
    return result;
  }

  DeviceProfile._();

  factory DeviceProfile.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DeviceProfile.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DeviceProfile',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'emoji')
    ..aOS(3, _omitFieldNames ? '' : 'debugMode')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceProfile clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceProfile copyWith(void Function(DeviceProfile) updates) =>
      super.copyWith((message) => updates(message as DeviceProfile))
          as DeviceProfile;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DeviceProfile create() => DeviceProfile._();
  @$core.override
  DeviceProfile createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DeviceProfile getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DeviceProfile>(create);
  static DeviceProfile? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get emoji => $_getSZ(1);
  @$pb.TagNumber(2)
  set emoji($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEmoji() => $_has(1);
  @$pb.TagNumber(2)
  void clearEmoji() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get debugMode => $_getSZ(2);
  @$pb.TagNumber(3)
  set debugMode($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDebugMode() => $_has(2);
  @$pb.TagNumber(3)
  void clearDebugMode() => $_clearField(3);
}

class DeviceIdentifiers extends $pb.GeneratedMessage {
  factory DeviceIdentifiers({
    $core.String? sn,
    $core.Iterable<PeerIMEI>? imeis,
    $core.Iterable<PeerLabel>? labels,
  }) {
    final result = create();
    if (sn != null) result.sn = sn;
    if (imeis != null) result.imeis.addAll(imeis);
    if (labels != null) result.labels.addAll(labels);
    return result;
  }

  DeviceIdentifiers._();

  factory DeviceIdentifiers.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DeviceIdentifiers.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DeviceIdentifiers',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'sn')
    ..pPM<PeerIMEI>(2, _omitFieldNames ? '' : 'imeis',
        subBuilder: PeerIMEI.create)
    ..pPM<PeerLabel>(3, _omitFieldNames ? '' : 'labels',
        subBuilder: PeerLabel.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceIdentifiers clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceIdentifiers copyWith(void Function(DeviceIdentifiers) updates) =>
      super.copyWith((message) => updates(message as DeviceIdentifiers))
          as DeviceIdentifiers;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DeviceIdentifiers create() => DeviceIdentifiers._();
  @$core.override
  DeviceIdentifiers createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DeviceIdentifiers getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DeviceIdentifiers>(create);
  static DeviceIdentifiers? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sn => $_getSZ(0);
  @$pb.TagNumber(1)
  set sn($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSn() => $_has(0);
  @$pb.TagNumber(1)
  void clearSn() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<PeerIMEI> get imeis => $_getList(1);

  @$pb.TagNumber(3)
  $pb.PbList<PeerLabel> get labels => $_getList(2);
}

class HardwareInfo extends $pb.GeneratedMessage {
  factory HardwareInfo({
    $core.String? hardwareRevision,
    $core.String? manufacturer,
    $core.String? model,
  }) {
    final result = create();
    if (hardwareRevision != null) result.hardwareRevision = hardwareRevision;
    if (manufacturer != null) result.manufacturer = manufacturer;
    if (model != null) result.model = model;
    return result;
  }

  HardwareInfo._();

  factory HardwareInfo.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory HardwareInfo.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'HardwareInfo',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'hardwareRevision')
    ..aOS(2, _omitFieldNames ? '' : 'manufacturer')
    ..aOS(3, _omitFieldNames ? '' : 'model')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HardwareInfo clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HardwareInfo copyWith(void Function(HardwareInfo) updates) =>
      super.copyWith((message) => updates(message as HardwareInfo))
          as HardwareInfo;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static HardwareInfo create() => HardwareInfo._();
  @$core.override
  HardwareInfo createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static HardwareInfo getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<HardwareInfo>(create);
  static HardwareInfo? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get hardwareRevision => $_getSZ(0);
  @$pb.TagNumber(1)
  set hardwareRevision($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHardwareRevision() => $_has(0);
  @$pb.TagNumber(1)
  void clearHardwareRevision() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get manufacturer => $_getSZ(1);
  @$pb.TagNumber(2)
  set manufacturer($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasManufacturer() => $_has(1);
  @$pb.TagNumber(2)
  void clearManufacturer() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get model => $_getSZ(2);
  @$pb.TagNumber(3)
  set model($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasModel() => $_has(2);
  @$pb.TagNumber(3)
  void clearModel() => $_clearField(3);
}

class PeerIMEI extends $pb.GeneratedMessage {
  factory PeerIMEI({
    $core.String? name,
    $core.String? serial,
    $core.String? tac,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (serial != null) result.serial = serial;
    if (tac != null) result.tac = tac;
    return result;
  }

  PeerIMEI._();

  factory PeerIMEI.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerIMEI.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerIMEI',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'serial')
    ..aOS(3, _omitFieldNames ? '' : 'tac')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerIMEI clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerIMEI copyWith(void Function(PeerIMEI) updates) =>
      super.copyWith((message) => updates(message as PeerIMEI)) as PeerIMEI;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerIMEI create() => PeerIMEI._();
  @$core.override
  PeerIMEI createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerIMEI getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PeerIMEI>(create);
  static PeerIMEI? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get serial => $_getSZ(1);
  @$pb.TagNumber(2)
  set serial($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSerial() => $_has(1);
  @$pb.TagNumber(2)
  void clearSerial() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get tac => $_getSZ(2);
  @$pb.TagNumber(3)
  set tac($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasTac() => $_has(2);
  @$pb.TagNumber(3)
  void clearTac() => $_clearField(3);
}

class PeerLabel extends $pb.GeneratedMessage {
  factory PeerLabel({
    $core.String? key,
    $core.String? value,
  }) {
    final result = create();
    if (key != null) result.key = key;
    if (value != null) result.value = value;
    return result;
  }

  PeerLabel._();

  factory PeerLabel.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerLabel.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerLabel',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'key')
    ..aOS(2, _omitFieldNames ? '' : 'value')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerLabel clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerLabel copyWith(void Function(PeerLabel) updates) =>
      super.copyWith((message) => updates(message as PeerLabel)) as PeerLabel;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerLabel create() => PeerLabel._();
  @$core.override
  PeerLabel createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerLabel getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PeerLabel>(create);
  static PeerLabel? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get key => $_getSZ(0);
  @$pb.TagNumber(1)
  set key($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get value => $_getSZ(1);
  @$pb.TagNumber(2)
  set value($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasValue() => $_has(1);
  @$pb.TagNumber(2)
  void clearValue() => $_clearField(2);
}

class PeerStatus extends $pb.GeneratedMessage {
  factory PeerStatus({
    $fixnum.Int64? batteryPercent,
    $core.bool? charging,
    $0.Struct? details,
    $core.double? gnssAccuracyM,
    $core.double? gnssAltitudeM,
    $core.double? gnssLatitude,
    $core.double? gnssLongitude,
    $core.Iterable<$core.MapEntry<$core.String, $core.String>>? labels,
    $core.bool? muted,
    $core.String? reportedAt,
    $fixnum.Int64? volume,
  }) {
    final result = create();
    if (batteryPercent != null) result.batteryPercent = batteryPercent;
    if (charging != null) result.charging = charging;
    if (details != null) result.details = details;
    if (gnssAccuracyM != null) result.gnssAccuracyM = gnssAccuracyM;
    if (gnssAltitudeM != null) result.gnssAltitudeM = gnssAltitudeM;
    if (gnssLatitude != null) result.gnssLatitude = gnssLatitude;
    if (gnssLongitude != null) result.gnssLongitude = gnssLongitude;
    if (labels != null) result.labels.addEntries(labels);
    if (muted != null) result.muted = muted;
    if (reportedAt != null) result.reportedAt = reportedAt;
    if (volume != null) result.volume = volume;
    return result;
  }

  PeerStatus._();

  factory PeerStatus.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerStatus.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerStatus',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'batteryPercent')
    ..aOB(2, _omitFieldNames ? '' : 'charging')
    ..aOM<$0.Struct>(3, _omitFieldNames ? '' : 'details',
        subBuilder: $0.Struct.create)
    ..aD(4, _omitFieldNames ? '' : 'gnssAccuracyM')
    ..aD(5, _omitFieldNames ? '' : 'gnssAltitudeM')
    ..aD(6, _omitFieldNames ? '' : 'gnssLatitude')
    ..aD(7, _omitFieldNames ? '' : 'gnssLongitude')
    ..m<$core.String, $core.String>(8, _omitFieldNames ? '' : 'labels',
        entryClassName: 'PeerStatus.LabelsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OS,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..aOB(9, _omitFieldNames ? '' : 'muted')
    ..aOS(10, _omitFieldNames ? '' : 'reportedAt')
    ..aInt64(11, _omitFieldNames ? '' : 'volume')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerStatus clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerStatus copyWith(void Function(PeerStatus) updates) =>
      super.copyWith((message) => updates(message as PeerStatus)) as PeerStatus;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerStatus create() => PeerStatus._();
  @$core.override
  PeerStatus createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerStatus getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerStatus>(create);
  static PeerStatus? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get batteryPercent => $_getI64(0);
  @$pb.TagNumber(1)
  set batteryPercent($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasBatteryPercent() => $_has(0);
  @$pb.TagNumber(1)
  void clearBatteryPercent() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get charging => $_getBF(1);
  @$pb.TagNumber(2)
  set charging($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCharging() => $_has(1);
  @$pb.TagNumber(2)
  void clearCharging() => $_clearField(2);

  @$pb.TagNumber(3)
  $0.Struct get details => $_getN(2);
  @$pb.TagNumber(3)
  set details($0.Struct value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasDetails() => $_has(2);
  @$pb.TagNumber(3)
  void clearDetails() => $_clearField(3);
  @$pb.TagNumber(3)
  $0.Struct ensureDetails() => $_ensure(2);

  @$pb.TagNumber(4)
  $core.double get gnssAccuracyM => $_getN(3);
  @$pb.TagNumber(4)
  set gnssAccuracyM($core.double value) => $_setDouble(3, value);
  @$pb.TagNumber(4)
  $core.bool hasGnssAccuracyM() => $_has(3);
  @$pb.TagNumber(4)
  void clearGnssAccuracyM() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.double get gnssAltitudeM => $_getN(4);
  @$pb.TagNumber(5)
  set gnssAltitudeM($core.double value) => $_setDouble(4, value);
  @$pb.TagNumber(5)
  $core.bool hasGnssAltitudeM() => $_has(4);
  @$pb.TagNumber(5)
  void clearGnssAltitudeM() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.double get gnssLatitude => $_getN(5);
  @$pb.TagNumber(6)
  set gnssLatitude($core.double value) => $_setDouble(5, value);
  @$pb.TagNumber(6)
  $core.bool hasGnssLatitude() => $_has(5);
  @$pb.TagNumber(6)
  void clearGnssLatitude() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.double get gnssLongitude => $_getN(6);
  @$pb.TagNumber(7)
  set gnssLongitude($core.double value) => $_setDouble(6, value);
  @$pb.TagNumber(7)
  $core.bool hasGnssLongitude() => $_has(6);
  @$pb.TagNumber(7)
  void clearGnssLongitude() => $_clearField(7);

  @$pb.TagNumber(8)
  $pb.PbMap<$core.String, $core.String> get labels => $_getMap(7);

  @$pb.TagNumber(9)
  $core.bool get muted => $_getBF(8);
  @$pb.TagNumber(9)
  set muted($core.bool value) => $_setBool(8, value);
  @$pb.TagNumber(9)
  $core.bool hasMuted() => $_has(8);
  @$pb.TagNumber(9)
  void clearMuted() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get reportedAt => $_getSZ(9);
  @$pb.TagNumber(10)
  set reportedAt($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasReportedAt() => $_has(9);
  @$pb.TagNumber(10)
  void clearReportedAt() => $_clearField(10);

  @$pb.TagNumber(11)
  $fixnum.Int64 get volume => $_getI64(10);
  @$pb.TagNumber(11)
  set volume($fixnum.Int64 value) => $_setInt64(10, value);
  @$pb.TagNumber(11)
  $core.bool hasVolume() => $_has(10);
  @$pb.TagNumber(11)
  void clearVolume() => $_clearField(11);
}

class PingRequest extends $pb.GeneratedMessage {
  factory PingRequest({
    $fixnum.Int64? clientSendTime,
  }) {
    final result = create();
    if (clientSendTime != null) result.clientSendTime = clientSendTime;
    return result;
  }

  PingRequest._();

  factory PingRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PingRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PingRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'clientSendTime')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PingRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PingRequest copyWith(void Function(PingRequest) updates) =>
      super.copyWith((message) => updates(message as PingRequest))
          as PingRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PingRequest create() => PingRequest._();
  @$core.override
  PingRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PingRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PingRequest>(create);
  static PingRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get clientSendTime => $_getI64(0);
  @$pb.TagNumber(1)
  set clientSendTime($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasClientSendTime() => $_has(0);
  @$pb.TagNumber(1)
  void clearClientSendTime() => $_clearField(1);
}

class PingResponse extends $pb.GeneratedMessage {
  factory PingResponse({
    $fixnum.Int64? serverTime,
  }) {
    final result = create();
    if (serverTime != null) result.serverTime = serverTime;
    return result;
  }

  PingResponse._();

  factory PingResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PingResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PingResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'serverTime')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PingResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PingResponse copyWith(void Function(PingResponse) updates) =>
      super.copyWith((message) => updates(message as PingResponse))
          as PingResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PingResponse create() => PingResponse._();
  @$core.override
  PingResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PingResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PingResponse>(create);
  static PingResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get serverTime => $_getI64(0);
  @$pb.TagNumber(1)
  set serverTime($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasServerTime() => $_has(0);
  @$pb.TagNumber(1)
  void clearServerTime() => $_clearField(1);
}

class ServerRegisterRequest extends $pb.GeneratedMessage {
  factory ServerRegisterRequest({
    $core.String? token,
  }) {
    final result = create();
    if (token != null) result.token = token;
    return result;
  }

  ServerRegisterRequest._();

  factory ServerRegisterRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRegisterRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRegisterRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'token')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRegisterRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRegisterRequest copyWith(
          void Function(ServerRegisterRequest) updates) =>
      super.copyWith((message) => updates(message as ServerRegisterRequest))
          as ServerRegisterRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRegisterRequest create() => ServerRegisterRequest._();
  @$core.override
  ServerRegisterRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRegisterRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRegisterRequest>(create);
  static ServerRegisterRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get token => $_getSZ(0);
  @$pb.TagNumber(1)
  set token($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasToken() => $_has(0);
  @$pb.TagNumber(1)
  void clearToken() => $_clearField(1);
}

class ServerRegisterResponse extends $pb.GeneratedMessage {
  factory ServerRegisterResponse({
    $core.String? runtimeProfileName,
  }) {
    final result = create();
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    return result;
  }

  ServerRegisterResponse._();

  factory ServerRegisterResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRegisterResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRegisterResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'runtimeProfileName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRegisterResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRegisterResponse copyWith(
          void Function(ServerRegisterResponse) updates) =>
      super.copyWith((message) => updates(message as ServerRegisterResponse))
          as ServerRegisterResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRegisterResponse create() => ServerRegisterResponse._();
  @$core.override
  ServerRegisterResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRegisterResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRegisterResponse>(create);
  static ServerRegisterResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get runtimeProfileName => $_getSZ(0);
  @$pb.TagNumber(1)
  set runtimeProfileName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRuntimeProfileName() => $_has(0);
  @$pb.TagNumber(1)
  void clearRuntimeProfileName() => $_clearField(1);
}

class APIKey extends $pb.GeneratedMessage {
  factory APIKey({
    $core.String? name,
    $core.String? displayName,
    $core.String? prefix,
    $core.bool? manageApiKeys,
    $core.String? createdAt,
    $core.String? apiKey,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (displayName != null) result.displayName = displayName;
    if (prefix != null) result.prefix = prefix;
    if (manageApiKeys != null) result.manageApiKeys = manageApiKeys;
    if (createdAt != null) result.createdAt = createdAt;
    if (apiKey != null) result.apiKey = apiKey;
    return result;
  }

  APIKey._();

  factory APIKey.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory APIKey.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'APIKey',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'displayName')
    ..aOS(3, _omitFieldNames ? '' : 'prefix')
    ..aOB(4, _omitFieldNames ? '' : 'manageApiKeys')
    ..aOS(5, _omitFieldNames ? '' : 'createdAt')
    ..aOS(6, _omitFieldNames ? '' : 'apiKey')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKey clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKey copyWith(void Function(APIKey) updates) =>
      super.copyWith((message) => updates(message as APIKey)) as APIKey;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static APIKey create() => APIKey._();
  @$core.override
  APIKey createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static APIKey getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<APIKey>(create);
  static APIKey? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get displayName => $_getSZ(1);
  @$pb.TagNumber(2)
  set displayName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDisplayName() => $_has(1);
  @$pb.TagNumber(2)
  void clearDisplayName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get prefix => $_getSZ(2);
  @$pb.TagNumber(3)
  set prefix($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPrefix() => $_has(2);
  @$pb.TagNumber(3)
  void clearPrefix() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get manageApiKeys => $_getBF(3);
  @$pb.TagNumber(4)
  set manageApiKeys($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasManageApiKeys() => $_has(3);
  @$pb.TagNumber(4)
  void clearManageApiKeys() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get createdAt => $_getSZ(4);
  @$pb.TagNumber(5)
  set createdAt($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasCreatedAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearCreatedAt() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get apiKey => $_getSZ(5);
  @$pb.TagNumber(6)
  set apiKey($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasApiKey() => $_has(5);
  @$pb.TagNumber(6)
  void clearApiKey() => $_clearField(6);
}

class APIKeyCreateRequest extends $pb.GeneratedMessage {
  factory APIKeyCreateRequest({
    $core.String? displayName,
    $core.bool? manageApiKeys,
  }) {
    final result = create();
    if (displayName != null) result.displayName = displayName;
    if (manageApiKeys != null) result.manageApiKeys = manageApiKeys;
    return result;
  }

  APIKeyCreateRequest._();

  factory APIKeyCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory APIKeyCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'APIKeyCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..aOB(2, _omitFieldNames ? '' : 'manageApiKeys')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyCreateRequest copyWith(void Function(APIKeyCreateRequest) updates) =>
      super.copyWith((message) => updates(message as APIKeyCreateRequest))
          as APIKeyCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static APIKeyCreateRequest create() => APIKeyCreateRequest._();
  @$core.override
  APIKeyCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static APIKeyCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<APIKeyCreateRequest>(create);
  static APIKeyCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get manageApiKeys => $_getBF(1);
  @$pb.TagNumber(2)
  set manageApiKeys($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasManageApiKeys() => $_has(1);
  @$pb.TagNumber(2)
  void clearManageApiKeys() => $_clearField(2);
}

class APIKeyCreateResponse extends $pb.GeneratedMessage {
  factory APIKeyCreateResponse({
    APIKey? value,
    $core.String? apiKey,
  }) {
    final result = create();
    if (value != null) result.value = value;
    if (apiKey != null) result.apiKey = apiKey;
    return result;
  }

  APIKeyCreateResponse._();

  factory APIKeyCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory APIKeyCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'APIKeyCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<APIKey>(1, _omitFieldNames ? '' : 'value', subBuilder: APIKey.create)
    ..aOS(2, _omitFieldNames ? '' : 'apiKey')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyCreateResponse copyWith(void Function(APIKeyCreateResponse) updates) =>
      super.copyWith((message) => updates(message as APIKeyCreateResponse))
          as APIKeyCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static APIKeyCreateResponse create() => APIKeyCreateResponse._();
  @$core.override
  APIKeyCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static APIKeyCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<APIKeyCreateResponse>(create);
  static APIKeyCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  APIKey get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(APIKey value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  APIKey ensureValue() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get apiKey => $_getSZ(1);
  @$pb.TagNumber(2)
  set apiKey($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasApiKey() => $_has(1);
  @$pb.TagNumber(2)
  void clearApiKey() => $_clearField(2);
}

class APIKeyListRequest extends $pb.GeneratedMessage {
  factory APIKeyListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  APIKeyListRequest._();

  factory APIKeyListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory APIKeyListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'APIKeyListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyListRequest copyWith(void Function(APIKeyListRequest) updates) =>
      super.copyWith((message) => updates(message as APIKeyListRequest))
          as APIKeyListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static APIKeyListRequest create() => APIKeyListRequest._();
  @$core.override
  APIKeyListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static APIKeyListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<APIKeyListRequest>(create);
  static APIKeyListRequest? _defaultInstance;

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

class APIKeyListResponse extends $pb.GeneratedMessage {
  factory APIKeyListResponse({
    $core.Iterable<APIKey>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  APIKeyListResponse._();

  factory APIKeyListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory APIKeyListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'APIKeyListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<APIKey>(1, _omitFieldNames ? '' : 'items', subBuilder: APIKey.create)
    ..aOS(2, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyListResponse copyWith(void Function(APIKeyListResponse) updates) =>
      super.copyWith((message) => updates(message as APIKeyListResponse))
          as APIKeyListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static APIKeyListResponse create() => APIKeyListResponse._();
  @$core.override
  APIKeyListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static APIKeyListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<APIKeyListResponse>(create);
  static APIKeyListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<APIKey> get items => $_getList(0);

  @$pb.TagNumber(2)
  $core.String get nextCursor => $_getSZ(1);
  @$pb.TagNumber(2)
  set nextCursor($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasNextCursor() => $_has(1);
  @$pb.TagNumber(2)
  void clearNextCursor() => $_clearField(2);
}

class APIKeyRevokeRequest extends $pb.GeneratedMessage {
  factory APIKeyRevokeRequest({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  APIKeyRevokeRequest._();

  factory APIKeyRevokeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory APIKeyRevokeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'APIKeyRevokeRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyRevokeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyRevokeRequest copyWith(void Function(APIKeyRevokeRequest) updates) =>
      super.copyWith((message) => updates(message as APIKeyRevokeRequest))
          as APIKeyRevokeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static APIKeyRevokeRequest create() => APIKeyRevokeRequest._();
  @$core.override
  APIKeyRevokeRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static APIKeyRevokeRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<APIKeyRevokeRequest>(create);
  static APIKeyRevokeRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);
}

class APIKeyRevokeResponse extends $pb.GeneratedMessage {
  factory APIKeyRevokeResponse() => create();

  APIKeyRevokeResponse._();

  factory APIKeyRevokeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory APIKeyRevokeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'APIKeyRevokeResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyRevokeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  APIKeyRevokeResponse copyWith(void Function(APIKeyRevokeResponse) updates) =>
      super.copyWith((message) => updates(message as APIKeyRevokeResponse))
          as APIKeyRevokeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static APIKeyRevokeResponse create() => APIKeyRevokeResponse._();
  @$core.override
  APIKeyRevokeResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static APIKeyRevokeResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<APIKeyRevokeResponse>(create);
  static APIKeyRevokeResponse? _defaultInstance;
}

class ServerPeerDeleteRequest extends $pb.GeneratedMessage {
  factory ServerPeerDeleteRequest() => create();

  ServerPeerDeleteRequest._();

  factory ServerPeerDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPeerDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPeerDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerDeleteRequest copyWith(
          void Function(ServerPeerDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPeerDeleteRequest))
          as ServerPeerDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPeerDeleteRequest create() => ServerPeerDeleteRequest._();
  @$core.override
  ServerPeerDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPeerDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPeerDeleteRequest>(create);
  static ServerPeerDeleteRequest? _defaultInstance;
}

class ServerPeerDeleteResponse extends $pb.GeneratedMessage {
  factory ServerPeerDeleteResponse() => create();

  ServerPeerDeleteResponse._();

  factory ServerPeerDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPeerDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPeerDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerDeleteResponse copyWith(
          void Function(ServerPeerDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPeerDeleteResponse))
          as ServerPeerDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPeerDeleteResponse create() => ServerPeerDeleteResponse._();
  @$core.override
  ServerPeerDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPeerDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPeerDeleteResponse>(create);
  static ServerPeerDeleteResponse? _defaultInstance;
}

class Runtime extends $pb.GeneratedMessage {
  factory Runtime({
    $core.String? lastAddr,
    $core.String? lastSeenAt,
    $core.bool? online,
    $fixnum.Int64? rxBytes,
    $fixnum.Int64? txBytes,
  }) {
    final result = create();
    if (lastAddr != null) result.lastAddr = lastAddr;
    if (lastSeenAt != null) result.lastSeenAt = lastSeenAt;
    if (online != null) result.online = online;
    if (rxBytes != null) result.rxBytes = rxBytes;
    if (txBytes != null) result.txBytes = txBytes;
    return result;
  }

  Runtime._();

  factory Runtime.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Runtime.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Runtime',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'lastAddr')
    ..aOS(2, _omitFieldNames ? '' : 'lastSeenAt')
    ..aOB(3, _omitFieldNames ? '' : 'online')
    ..a<$fixnum.Int64>(4, _omitFieldNames ? '' : 'rxBytes', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(5, _omitFieldNames ? '' : 'txBytes', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Runtime clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Runtime copyWith(void Function(Runtime) updates) =>
      super.copyWith((message) => updates(message as Runtime)) as Runtime;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Runtime create() => Runtime._();
  @$core.override
  Runtime createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Runtime getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Runtime>(create);
  static Runtime? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get lastAddr => $_getSZ(0);
  @$pb.TagNumber(1)
  set lastAddr($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasLastAddr() => $_has(0);
  @$pb.TagNumber(1)
  void clearLastAddr() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get lastSeenAt => $_getSZ(1);
  @$pb.TagNumber(2)
  set lastSeenAt($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLastSeenAt() => $_has(1);
  @$pb.TagNumber(2)
  void clearLastSeenAt() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get online => $_getBF(2);
  @$pb.TagNumber(3)
  set online($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasOnline() => $_has(2);
  @$pb.TagNumber(3)
  void clearOnline() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get rxBytes => $_getI64(3);
  @$pb.TagNumber(4)
  set rxBytes($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRxBytes() => $_has(3);
  @$pb.TagNumber(4)
  void clearRxBytes() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get txBytes => $_getI64(4);
  @$pb.TagNumber(5)
  set txBytes($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasTxBytes() => $_has(4);
  @$pb.TagNumber(5)
  void clearTxBytes() => $_clearField(5);
}

class ServerGetInfoRequest extends $pb.GeneratedMessage {
  factory ServerGetInfoRequest() => create();

  ServerGetInfoRequest._();

  factory ServerGetInfoRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetInfoRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetInfoRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetInfoRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetInfoRequest copyWith(void Function(ServerGetInfoRequest) updates) =>
      super.copyWith((message) => updates(message as ServerGetInfoRequest))
          as ServerGetInfoRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetInfoRequest create() => ServerGetInfoRequest._();
  @$core.override
  ServerGetInfoRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetInfoRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetInfoRequest>(create);
  static ServerGetInfoRequest? _defaultInstance;
}

class ServerGetInfoResponse extends $pb.GeneratedMessage {
  factory ServerGetInfoResponse({
    DeviceInfo? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGetInfoResponse._();

  factory ServerGetInfoResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetInfoResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetInfoResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DeviceInfo>(1, _omitFieldNames ? '' : 'value',
        subBuilder: DeviceInfo.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetInfoResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetInfoResponse copyWith(
          void Function(ServerGetInfoResponse) updates) =>
      super.copyWith((message) => updates(message as ServerGetInfoResponse))
          as ServerGetInfoResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetInfoResponse create() => ServerGetInfoResponse._();
  @$core.override
  ServerGetInfoResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetInfoResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetInfoResponse>(create);
  static ServerGetInfoResponse? _defaultInstance;

  @$pb.TagNumber(1)
  DeviceInfo get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(DeviceInfo value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  DeviceInfo ensureValue() => $_ensure(0);
}

class ServerGetStatusRequest extends $pb.GeneratedMessage {
  factory ServerGetStatusRequest() => create();

  ServerGetStatusRequest._();

  factory ServerGetStatusRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetStatusRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetStatusRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetStatusRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetStatusRequest copyWith(
          void Function(ServerGetStatusRequest) updates) =>
      super.copyWith((message) => updates(message as ServerGetStatusRequest))
          as ServerGetStatusRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetStatusRequest create() => ServerGetStatusRequest._();
  @$core.override
  ServerGetStatusRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetStatusRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetStatusRequest>(create);
  static ServerGetStatusRequest? _defaultInstance;
}

class ServerGetStatusResponse extends $pb.GeneratedMessage {
  factory ServerGetStatusResponse({
    PeerStatus? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerGetStatusResponse._();

  factory ServerGetStatusResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerGetStatusResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerGetStatusResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerStatus>(1, _omitFieldNames ? '' : 'value',
        subBuilder: PeerStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetStatusResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerGetStatusResponse copyWith(
          void Function(ServerGetStatusResponse) updates) =>
      super.copyWith((message) => updates(message as ServerGetStatusResponse))
          as ServerGetStatusResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerGetStatusResponse create() => ServerGetStatusResponse._();
  @$core.override
  ServerGetStatusResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerGetStatusResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerGetStatusResponse>(create);
  static ServerGetStatusResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerStatus get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(PeerStatus value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerStatus ensureValue() => $_ensure(0);
}

class ServerPutInfoRequest extends $pb.GeneratedMessage {
  factory ServerPutInfoRequest({
    DeviceProfile? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPutInfoRequest._();

  factory ServerPutInfoRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPutInfoRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPutInfoRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DeviceProfile>(1, _omitFieldNames ? '' : 'value',
        subBuilder: DeviceProfile.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPutInfoRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPutInfoRequest copyWith(void Function(ServerPutInfoRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPutInfoRequest))
          as ServerPutInfoRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPutInfoRequest create() => ServerPutInfoRequest._();
  @$core.override
  ServerPutInfoRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPutInfoRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPutInfoRequest>(create);
  static ServerPutInfoRequest? _defaultInstance;

  @$pb.TagNumber(1)
  DeviceProfile get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(DeviceProfile value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  DeviceProfile ensureValue() => $_ensure(0);
}

class ServerPutInfoResponse extends $pb.GeneratedMessage {
  factory ServerPutInfoResponse({
    DeviceInfo? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ServerPutInfoResponse._();

  factory ServerPutInfoResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPutInfoResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPutInfoResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DeviceInfo>(1, _omitFieldNames ? '' : 'value',
        subBuilder: DeviceInfo.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPutInfoResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPutInfoResponse copyWith(
          void Function(ServerPutInfoResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPutInfoResponse))
          as ServerPutInfoResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPutInfoResponse create() => ServerPutInfoResponse._();
  @$core.override
  ServerPutInfoResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPutInfoResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPutInfoResponse>(create);
  static ServerPutInfoResponse? _defaultInstance;

  @$pb.TagNumber(1)
  DeviceInfo get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(DeviceInfo value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  DeviceInfo ensureValue() => $_ensure(0);
}

class SpeedTestRequest extends $pb.GeneratedMessage {
  factory SpeedTestRequest({
    $fixnum.Int64? downContentLength,
    $fixnum.Int64? upContentLength,
  }) {
    final result = create();
    if (downContentLength != null) result.downContentLength = downContentLength;
    if (upContentLength != null) result.upContentLength = upContentLength;
    return result;
  }

  SpeedTestRequest._();

  factory SpeedTestRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeedTestRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeedTestRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'downContentLength')
    ..aInt64(2, _omitFieldNames ? '' : 'upContentLength')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeedTestRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeedTestRequest copyWith(void Function(SpeedTestRequest) updates) =>
      super.copyWith((message) => updates(message as SpeedTestRequest))
          as SpeedTestRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeedTestRequest create() => SpeedTestRequest._();
  @$core.override
  SpeedTestRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeedTestRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeedTestRequest>(create);
  static SpeedTestRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get downContentLength => $_getI64(0);
  @$pb.TagNumber(1)
  set downContentLength($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDownContentLength() => $_has(0);
  @$pb.TagNumber(1)
  void clearDownContentLength() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get upContentLength => $_getI64(1);
  @$pb.TagNumber(2)
  set upContentLength($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasUpContentLength() => $_has(1);
  @$pb.TagNumber(2)
  void clearUpContentLength() => $_clearField(2);
}

class SpeedTestResponse extends $pb.GeneratedMessage {
  factory SpeedTestResponse({
    $fixnum.Int64? downContentLength,
    $fixnum.Int64? upContentLength,
  }) {
    final result = create();
    if (downContentLength != null) result.downContentLength = downContentLength;
    if (upContentLength != null) result.upContentLength = upContentLength;
    return result;
  }

  SpeedTestResponse._();

  factory SpeedTestResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeedTestResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeedTestResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'downContentLength')
    ..aInt64(2, _omitFieldNames ? '' : 'upContentLength')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeedTestResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeedTestResponse copyWith(void Function(SpeedTestResponse) updates) =>
      super.copyWith((message) => updates(message as SpeedTestResponse))
          as SpeedTestResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeedTestResponse create() => SpeedTestResponse._();
  @$core.override
  SpeedTestResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeedTestResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeedTestResponse>(create);
  static SpeedTestResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get downContentLength => $_getI64(0);
  @$pb.TagNumber(1)
  set downContentLength($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDownContentLength() => $_has(0);
  @$pb.TagNumber(1)
  void clearDownContentLength() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get upContentLength => $_getI64(1);
  @$pb.TagNumber(2)
  set upContentLength($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasUpContentLength() => $_has(1);
  @$pb.TagNumber(2)
  void clearUpContentLength() => $_clearField(2);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
