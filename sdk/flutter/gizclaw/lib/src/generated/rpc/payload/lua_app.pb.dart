// This is a generated file - do not edit.
//
// Generated from payload/lua_app.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

/// app_id is the package's stable identity, not a filesystem path or Peer ID.
class LuaAppInfo extends $pb.GeneratedMessage {
  factory LuaAppInfo({
    $core.String? appId,
    $core.String? version,
    $core.String? displayName,
    $core.String? description,
  }) {
    final result = create();
    if (appId != null) result.appId = appId;
    if (version != null) result.version = version;
    if (displayName != null) result.displayName = displayName;
    if (description != null) result.description = description;
    return result;
  }

  LuaAppInfo._();

  factory LuaAppInfo.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory LuaAppInfo.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'LuaAppInfo',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'appId')
    ..aOS(2, _omitFieldNames ? '' : 'version')
    ..aOS(3, _omitFieldNames ? '' : 'displayName')
    ..aOS(4, _omitFieldNames ? '' : 'description')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LuaAppInfo clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LuaAppInfo copyWith(void Function(LuaAppInfo) updates) =>
      super.copyWith((message) => updates(message as LuaAppInfo)) as LuaAppInfo;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static LuaAppInfo create() => LuaAppInfo._();
  @$core.override
  LuaAppInfo createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static LuaAppInfo getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<LuaAppInfo>(create);
  static LuaAppInfo? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get appId => $_getSZ(0);
  @$pb.TagNumber(1)
  set appId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAppId() => $_has(0);
  @$pb.TagNumber(1)
  void clearAppId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get version => $_getSZ(1);
  @$pb.TagNumber(2)
  set version($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasVersion() => $_has(1);
  @$pb.TagNumber(2)
  void clearVersion() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get displayName => $_getSZ(2);
  @$pb.TagNumber(3)
  set displayName($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDisplayName() => $_has(2);
  @$pb.TagNumber(3)
  void clearDisplayName() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get description => $_getSZ(3);
  @$pb.TagNumber(4)
  set description($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasDescription() => $_has(3);
  @$pb.TagNumber(4)
  void clearDescription() => $_clearField(4);
}

class ClientLuaAppListRequest extends $pb.GeneratedMessage {
  factory ClientLuaAppListRequest() => create();

  ClientLuaAppListRequest._();

  factory ClientLuaAppListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientLuaAppListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientLuaAppListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppListRequest copyWith(
          void Function(ClientLuaAppListRequest) updates) =>
      super.copyWith((message) => updates(message as ClientLuaAppListRequest))
          as ClientLuaAppListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientLuaAppListRequest create() => ClientLuaAppListRequest._();
  @$core.override
  ClientLuaAppListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientLuaAppListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientLuaAppListRequest>(create);
  static ClientLuaAppListRequest? _defaultInstance;
}

class ClientLuaAppListResponse extends $pb.GeneratedMessage {
  factory ClientLuaAppListResponse({
    $core.Iterable<LuaAppInfo>? apps,
  }) {
    final result = create();
    if (apps != null) result.apps.addAll(apps);
    return result;
  }

  ClientLuaAppListResponse._();

  factory ClientLuaAppListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientLuaAppListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientLuaAppListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<LuaAppInfo>(1, _omitFieldNames ? '' : 'apps',
        subBuilder: LuaAppInfo.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppListResponse copyWith(
          void Function(ClientLuaAppListResponse) updates) =>
      super.copyWith((message) => updates(message as ClientLuaAppListResponse))
          as ClientLuaAppListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientLuaAppListResponse create() => ClientLuaAppListResponse._();
  @$core.override
  ClientLuaAppListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientLuaAppListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientLuaAppListResponse>(create);
  static ClientLuaAppListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<LuaAppInfo> get apps => $_getList(0);
}

/// The device downloads and validates the complete manifest and every payload
/// before publishing the installation. Success means installed, not queued.
/// Failure preserves the previous app and its user data. Insufficient storage
/// or an unavailable installer returns UNIMPLEMENTED (HTTP DEVICE_UNSUPPORTED).
/// No automatic retry. The Server does not download or unpack the URL.
class ClientLuaAppInstallRequest extends $pb.GeneratedMessage {
  factory ClientLuaAppInstallRequest({
    $core.String? url,
    $core.String? sha256,
  }) {
    final result = create();
    if (url != null) result.url = url;
    if (sha256 != null) result.sha256 = sha256;
    return result;
  }

  ClientLuaAppInstallRequest._();

  factory ClientLuaAppInstallRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientLuaAppInstallRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientLuaAppInstallRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'url')
    ..aOS(2, _omitFieldNames ? '' : 'sha256')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppInstallRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppInstallRequest copyWith(
          void Function(ClientLuaAppInstallRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientLuaAppInstallRequest))
          as ClientLuaAppInstallRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientLuaAppInstallRequest create() => ClientLuaAppInstallRequest._();
  @$core.override
  ClientLuaAppInstallRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientLuaAppInstallRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientLuaAppInstallRequest>(create);
  static ClientLuaAppInstallRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get url => $_getSZ(0);
  @$pb.TagNumber(1)
  set url($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasUrl() => $_has(0);
  @$pb.TagNumber(1)
  void clearUrl() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get sha256 => $_getSZ(1);
  @$pb.TagNumber(2)
  set sha256($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSha256() => $_has(1);
  @$pb.TagNumber(2)
  void clearSha256() => $_clearField(2);
}

class ClientLuaAppInstallResponse extends $pb.GeneratedMessage {
  factory ClientLuaAppInstallResponse({
    LuaAppInfo? app,
  }) {
    final result = create();
    if (app != null) result.app = app;
    return result;
  }

  ClientLuaAppInstallResponse._();

  factory ClientLuaAppInstallResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientLuaAppInstallResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientLuaAppInstallResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<LuaAppInfo>(1, _omitFieldNames ? '' : 'app',
        subBuilder: LuaAppInfo.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppInstallResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppInstallResponse copyWith(
          void Function(ClientLuaAppInstallResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientLuaAppInstallResponse))
          as ClientLuaAppInstallResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientLuaAppInstallResponse create() =>
      ClientLuaAppInstallResponse._();
  @$core.override
  ClientLuaAppInstallResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientLuaAppInstallResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientLuaAppInstallResponse>(create);
  static ClientLuaAppInstallResponse? _defaultInstance;

  @$pb.TagNumber(1)
  LuaAppInfo get app => $_getN(0);
  @$pb.TagNumber(1)
  set app(LuaAppInfo value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApp() => $_has(0);
  @$pb.TagNumber(1)
  void clearApp() => $_clearField(1);
  @$pb.TagNumber(1)
  LuaAppInfo ensureApp() => $_ensure(0);
}

/// Select an app_id returned by lua.app.list. Parameters map directly to GizOS
/// h2_lua_arg_t and the Lua args table: both keys and values are strings.
/// At most 16 parameters; keys 1..64 UTF-8 bytes, values 0..1024 bytes;
/// total key+value bytes at most 4096. NUL is forbidden. Omitted params is empty.
/// NOT_FOUND means the app is not installed. Respond before handing over the UI
/// or disconnecting. Success acknowledges launch acceptance, not game completion.
class ClientLuaAppRunRequest extends $pb.GeneratedMessage {
  factory ClientLuaAppRunRequest({
    $core.String? appId,
    $core.Iterable<$core.MapEntry<$core.String, $core.String>>? params,
  }) {
    final result = create();
    if (appId != null) result.appId = appId;
    if (params != null) result.params.addEntries(params);
    return result;
  }

  ClientLuaAppRunRequest._();

  factory ClientLuaAppRunRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientLuaAppRunRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientLuaAppRunRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'appId')
    ..m<$core.String, $core.String>(2, _omitFieldNames ? '' : 'params',
        entryClassName: 'ClientLuaAppRunRequest.ParamsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OS,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppRunRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppRunRequest copyWith(
          void Function(ClientLuaAppRunRequest) updates) =>
      super.copyWith((message) => updates(message as ClientLuaAppRunRequest))
          as ClientLuaAppRunRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientLuaAppRunRequest create() => ClientLuaAppRunRequest._();
  @$core.override
  ClientLuaAppRunRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientLuaAppRunRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientLuaAppRunRequest>(create);
  static ClientLuaAppRunRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get appId => $_getSZ(0);
  @$pb.TagNumber(1)
  set appId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAppId() => $_has(0);
  @$pb.TagNumber(1)
  void clearAppId() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, $core.String> get params => $_getMap(1);
}

class ClientLuaAppRunResponse extends $pb.GeneratedMessage {
  factory ClientLuaAppRunResponse() => create();

  ClientLuaAppRunResponse._();

  factory ClientLuaAppRunResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientLuaAppRunResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientLuaAppRunResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppRunResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientLuaAppRunResponse copyWith(
          void Function(ClientLuaAppRunResponse) updates) =>
      super.copyWith((message) => updates(message as ClientLuaAppRunResponse))
          as ClientLuaAppRunResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientLuaAppRunResponse create() => ClientLuaAppRunResponse._();
  @$core.override
  ClientLuaAppRunResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientLuaAppRunResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientLuaAppRunResponse>(create);
  static ClientLuaAppRunResponse? _defaultInstance;
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
