// This is a generated file - do not edit.
//
// Generated from payload/app.proto.

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

/// InstalledApp identifies one App package installed on the Client. sha256 is
/// the lowercase digest of the exact installed package archive.
class InstalledApp extends $pb.GeneratedMessage {
  factory InstalledApp({
    $core.String? appName,
    $core.String? sha256,
  }) {
    final result = InstalledApp._();
    if (appName != null) result.appName = appName;
    if (sha256 != null) result.sha256 = sha256;
    return result;
  }

  InstalledApp._();

  factory InstalledApp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InstalledApp()..mergeFromBuffer(data, registry);
  factory InstalledApp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InstalledApp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'InstalledApp',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: InstalledApp.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'appName')
    ..aOS(2, _omitFieldNames ? '' : 'sha256')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InstalledApp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InstalledApp copyWith(void Function(InstalledApp) updates) =>
      super.copyWith((message) => updates(message as InstalledApp))
          as InstalledApp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use InstalledApp() / InstalledApp.new instead')
  static InstalledApp create() => InstalledApp._();
  static $pb.GeneratedMessage $_createMessage() => InstalledApp._();
  @$core.override
  InstalledApp createEmptyInstance() => InstalledApp._();
  @$core.pragma('dart2js:noInline')
  static InstalledApp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<InstalledApp>(
          InstalledApp.$_createMessage);
  static InstalledApp? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get appName => $_getSZ(0);
  @$pb.TagNumber(1)
  set appName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAppName() => $_has(0);
  @$pb.TagNumber(1)
  void clearAppName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get sha256 => $_getSZ(1);
  @$pb.TagNumber(2)
  set sha256($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSha256() => $_has(1);
  @$pb.TagNumber(2)
  void clearSha256() => $_clearField(2);
}

class ClientAppListRequest extends $pb.GeneratedMessage {
  factory ClientAppListRequest() => ClientAppListRequest._();

  ClientAppListRequest._();

  factory ClientAppListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppListRequest()..mergeFromBuffer(data, registry);
  factory ClientAppListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppListRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppListRequest.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppListRequest copyWith(void Function(ClientAppListRequest) updates) =>
      super.copyWith((message) => updates(message as ClientAppListRequest))
          as ClientAppListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppListRequest() / ClientAppListRequest.new instead')
  static ClientAppListRequest create() => ClientAppListRequest._();
  static $pb.GeneratedMessage $_createMessage() => ClientAppListRequest._();
  @$core.override
  ClientAppListRequest createEmptyInstance() => ClientAppListRequest._();
  @$core.pragma('dart2js:noInline')
  static ClientAppListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppListRequest>(
          ClientAppListRequest.$_createMessage);
  static ClientAppListRequest? _defaultInstance;
}

class ClientAppListResponse extends $pb.GeneratedMessage {
  factory ClientAppListResponse({
    $core.String? runtime,
    $core.Iterable<InstalledApp>? apps,
    $core.Iterable<$core.String>? capabilities,
  }) {
    final result = ClientAppListResponse._();
    if (runtime != null) result.runtime = runtime;
    if (apps != null) result.apps.addAll(apps);
    if (capabilities != null) result.capabilities.addAll(capabilities);
    return result;
  }

  ClientAppListResponse._();

  factory ClientAppListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppListResponse()..mergeFromBuffer(data, registry);
  factory ClientAppListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppListResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppListResponse.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'runtime')
    ..pPM<InstalledApp>(2, _omitFieldNames ? '' : 'apps',
        subBuilder: InstalledApp.$_createMessage)
    ..pPS(3, _omitFieldNames ? '' : 'capabilities')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppListResponse copyWith(
          void Function(ClientAppListResponse) updates) =>
      super.copyWith((message) => updates(message as ClientAppListResponse))
          as ClientAppListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppListResponse() / ClientAppListResponse.new instead')
  static ClientAppListResponse create() => ClientAppListResponse._();
  static $pb.GeneratedMessage $_createMessage() => ClientAppListResponse._();
  @$core.override
  ClientAppListResponse createEmptyInstance() => ClientAppListResponse._();
  @$core.pragma('dart2js:noInline')
  static ClientAppListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppListResponse>(
          ClientAppListResponse.$_createMessage);
  static ClientAppListResponse? _defaultInstance;

  /// runtime is the single opaque App runtime profile ID this Client provides,
  /// for example "runtime.lua.gizos".
  @$pb.TagNumber(1)
  $core.String get runtime => $_getSZ(0);
  @$pb.TagNumber(1)
  set runtime($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRuntime() => $_has(0);
  @$pb.TagNumber(1)
  void clearRuntime() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<InstalledApp> get apps => $_getList(1);

  /// capabilities lists every capability name registered in the Client's Lua
  /// host, frozen when the host starts. Apps whose manifest requires a missing
  /// capability are neither installed nor exposed.
  @$pb.TagNumber(3)
  $pb.PbList<$core.String> get capabilities => $_getList(2);
}

/// ClientAppInstallRequest asks the Client to download, verify, and install
/// one App package archive. Installing an app_name that is already installed
/// replaces it.
class ClientAppInstallRequest extends $pb.GeneratedMessage {
  factory ClientAppInstallRequest({
    $core.String? appName,
    $core.String? url,
    $core.String? sha256,
    $fixnum.Int64? size,
  }) {
    final result = ClientAppInstallRequest._();
    if (appName != null) result.appName = appName;
    if (url != null) result.url = url;
    if (sha256 != null) result.sha256 = sha256;
    if (size != null) result.size = size;
    return result;
  }

  ClientAppInstallRequest._();

  factory ClientAppInstallRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppInstallRequest()..mergeFromBuffer(data, registry);
  factory ClientAppInstallRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppInstallRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppInstallRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppInstallRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'appName')
    ..aOS(2, _omitFieldNames ? '' : 'url')
    ..aOS(3, _omitFieldNames ? '' : 'sha256')
    ..aInt64(4, _omitFieldNames ? '' : 'size')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppInstallRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppInstallRequest copyWith(
          void Function(ClientAppInstallRequest) updates) =>
      super.copyWith((message) => updates(message as ClientAppInstallRequest))
          as ClientAppInstallRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppInstallRequest() / ClientAppInstallRequest.new instead')
  static ClientAppInstallRequest create() => ClientAppInstallRequest._();
  static $pb.GeneratedMessage $_createMessage() => ClientAppInstallRequest._();
  @$core.override
  ClientAppInstallRequest createEmptyInstance() => ClientAppInstallRequest._();
  @$core.pragma('dart2js:noInline')
  static ClientAppInstallRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppInstallRequest>(
          ClientAppInstallRequest.$_createMessage);
  static ClientAppInstallRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get appName => $_getSZ(0);
  @$pb.TagNumber(1)
  set appName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAppName() => $_has(0);
  @$pb.TagNumber(1)
  void clearAppName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get url => $_getSZ(1);
  @$pb.TagNumber(2)
  set url($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasUrl() => $_has(1);
  @$pb.TagNumber(2)
  void clearUrl() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get sha256 => $_getSZ(2);
  @$pb.TagNumber(3)
  set sha256($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSha256() => $_has(2);
  @$pb.TagNumber(3)
  void clearSha256() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get size => $_getI64(3);
  @$pb.TagNumber(4)
  set size($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSize() => $_has(3);
  @$pb.TagNumber(4)
  void clearSize() => $_clearField(4);
}

class ClientAppInstallResponse extends $pb.GeneratedMessage {
  factory ClientAppInstallResponse() => ClientAppInstallResponse._();

  ClientAppInstallResponse._();

  factory ClientAppInstallResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppInstallResponse()..mergeFromBuffer(data, registry);
  factory ClientAppInstallResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppInstallResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppInstallResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppInstallResponse.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppInstallResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppInstallResponse copyWith(
          void Function(ClientAppInstallResponse) updates) =>
      super.copyWith((message) => updates(message as ClientAppInstallResponse))
          as ClientAppInstallResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppInstallResponse() / ClientAppInstallResponse.new instead')
  static ClientAppInstallResponse create() => ClientAppInstallResponse._();
  static $pb.GeneratedMessage $_createMessage() => ClientAppInstallResponse._();
  @$core.override
  ClientAppInstallResponse createEmptyInstance() =>
      ClientAppInstallResponse._();
  @$core.pragma('dart2js:noInline')
  static ClientAppInstallResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppInstallResponse>(
          ClientAppInstallResponse.$_createMessage);
  static ClientAppInstallResponse? _defaultInstance;
}

class ClientAppUninstallRequest extends $pb.GeneratedMessage {
  factory ClientAppUninstallRequest({
    $core.String? appName,
  }) {
    final result = ClientAppUninstallRequest._();
    if (appName != null) result.appName = appName;
    return result;
  }

  ClientAppUninstallRequest._();

  factory ClientAppUninstallRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppUninstallRequest()..mergeFromBuffer(data, registry);
  factory ClientAppUninstallRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppUninstallRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppUninstallRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppUninstallRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'appName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppUninstallRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppUninstallRequest copyWith(
          void Function(ClientAppUninstallRequest) updates) =>
      super.copyWith((message) => updates(message as ClientAppUninstallRequest))
          as ClientAppUninstallRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppUninstallRequest() / ClientAppUninstallRequest.new instead')
  static ClientAppUninstallRequest create() => ClientAppUninstallRequest._();
  static $pb.GeneratedMessage $_createMessage() =>
      ClientAppUninstallRequest._();
  @$core.override
  ClientAppUninstallRequest createEmptyInstance() =>
      ClientAppUninstallRequest._();
  @$core.pragma('dart2js:noInline')
  static ClientAppUninstallRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppUninstallRequest>(
          ClientAppUninstallRequest.$_createMessage);
  static ClientAppUninstallRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get appName => $_getSZ(0);
  @$pb.TagNumber(1)
  set appName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAppName() => $_has(0);
  @$pb.TagNumber(1)
  void clearAppName() => $_clearField(1);
}

class ClientAppUninstallResponse extends $pb.GeneratedMessage {
  factory ClientAppUninstallResponse() => ClientAppUninstallResponse._();

  ClientAppUninstallResponse._();

  factory ClientAppUninstallResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppUninstallResponse()..mergeFromBuffer(data, registry);
  factory ClientAppUninstallResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppUninstallResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppUninstallResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppUninstallResponse.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppUninstallResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppUninstallResponse copyWith(
          void Function(ClientAppUninstallResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientAppUninstallResponse))
          as ClientAppUninstallResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppUninstallResponse() / ClientAppUninstallResponse.new instead')
  static ClientAppUninstallResponse create() => ClientAppUninstallResponse._();
  static $pb.GeneratedMessage $_createMessage() =>
      ClientAppUninstallResponse._();
  @$core.override
  ClientAppUninstallResponse createEmptyInstance() =>
      ClientAppUninstallResponse._();
  @$core.pragma('dart2js:noInline')
  static ClientAppUninstallResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppUninstallResponse>(
          ClientAppUninstallResponse.$_createMessage);
  static ClientAppUninstallResponse? _defaultInstance;
}

/// ClientAppInvokeRequest runs one mode "call" App method to completion.
/// args_json is a JSON object; result_json is the JSON-encoded method result.
class ClientAppInvokeRequest extends $pb.GeneratedMessage {
  factory ClientAppInvokeRequest({
    $core.String? appName,
    $core.String? method,
    $core.String? argsJson,
  }) {
    final result = ClientAppInvokeRequest._();
    if (appName != null) result.appName = appName;
    if (method != null) result.method = method;
    if (argsJson != null) result.argsJson = argsJson;
    return result;
  }

  ClientAppInvokeRequest._();

  factory ClientAppInvokeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppInvokeRequest()..mergeFromBuffer(data, registry);
  factory ClientAppInvokeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppInvokeRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppInvokeRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppInvokeRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'appName')
    ..aOS(2, _omitFieldNames ? '' : 'method')
    ..aOS(3, _omitFieldNames ? '' : 'argsJson')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppInvokeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppInvokeRequest copyWith(
          void Function(ClientAppInvokeRequest) updates) =>
      super.copyWith((message) => updates(message as ClientAppInvokeRequest))
          as ClientAppInvokeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppInvokeRequest() / ClientAppInvokeRequest.new instead')
  static ClientAppInvokeRequest create() => ClientAppInvokeRequest._();
  static $pb.GeneratedMessage $_createMessage() => ClientAppInvokeRequest._();
  @$core.override
  ClientAppInvokeRequest createEmptyInstance() => ClientAppInvokeRequest._();
  @$core.pragma('dart2js:noInline')
  static ClientAppInvokeRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppInvokeRequest>(
          ClientAppInvokeRequest.$_createMessage);
  static ClientAppInvokeRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get appName => $_getSZ(0);
  @$pb.TagNumber(1)
  set appName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAppName() => $_has(0);
  @$pb.TagNumber(1)
  void clearAppName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get method => $_getSZ(1);
  @$pb.TagNumber(2)
  set method($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMethod() => $_has(1);
  @$pb.TagNumber(2)
  void clearMethod() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get argsJson => $_getSZ(2);
  @$pb.TagNumber(3)
  set argsJson($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasArgsJson() => $_has(2);
  @$pb.TagNumber(3)
  void clearArgsJson() => $_clearField(3);
}

class ClientAppInvokeResponse extends $pb.GeneratedMessage {
  factory ClientAppInvokeResponse({
    $core.String? resultJson,
  }) {
    final result = ClientAppInvokeResponse._();
    if (resultJson != null) result.resultJson = resultJson;
    return result;
  }

  ClientAppInvokeResponse._();

  factory ClientAppInvokeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppInvokeResponse()..mergeFromBuffer(data, registry);
  factory ClientAppInvokeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppInvokeResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppInvokeResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppInvokeResponse.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'resultJson')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppInvokeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppInvokeResponse copyWith(
          void Function(ClientAppInvokeResponse) updates) =>
      super.copyWith((message) => updates(message as ClientAppInvokeResponse))
          as ClientAppInvokeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppInvokeResponse() / ClientAppInvokeResponse.new instead')
  static ClientAppInvokeResponse create() => ClientAppInvokeResponse._();
  static $pb.GeneratedMessage $_createMessage() => ClientAppInvokeResponse._();
  @$core.override
  ClientAppInvokeResponse createEmptyInstance() => ClientAppInvokeResponse._();
  @$core.pragma('dart2js:noInline')
  static ClientAppInvokeResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppInvokeResponse>(
          ClientAppInvokeResponse.$_createMessage);
  static ClientAppInvokeResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get resultJson => $_getSZ(0);
  @$pb.TagNumber(1)
  set resultJson($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasResultJson() => $_has(0);
  @$pb.TagNumber(1)
  void clearResultJson() => $_clearField(1);
}

/// ClientAppJobStartRequest starts one mode "job" App method and returns
/// without waiting for it to finish.
class ClientAppJobStartRequest extends $pb.GeneratedMessage {
  factory ClientAppJobStartRequest({
    $core.String? appName,
    $core.String? method,
    $core.String? argsJson,
  }) {
    final result = ClientAppJobStartRequest._();
    if (appName != null) result.appName = appName;
    if (method != null) result.method = method;
    if (argsJson != null) result.argsJson = argsJson;
    return result;
  }

  ClientAppJobStartRequest._();

  factory ClientAppJobStartRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppJobStartRequest()..mergeFromBuffer(data, registry);
  factory ClientAppJobStartRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppJobStartRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppJobStartRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppJobStartRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'appName')
    ..aOS(2, _omitFieldNames ? '' : 'method')
    ..aOS(3, _omitFieldNames ? '' : 'argsJson')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppJobStartRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppJobStartRequest copyWith(
          void Function(ClientAppJobStartRequest) updates) =>
      super.copyWith((message) => updates(message as ClientAppJobStartRequest))
          as ClientAppJobStartRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppJobStartRequest() / ClientAppJobStartRequest.new instead')
  static ClientAppJobStartRequest create() => ClientAppJobStartRequest._();
  static $pb.GeneratedMessage $_createMessage() => ClientAppJobStartRequest._();
  @$core.override
  ClientAppJobStartRequest createEmptyInstance() =>
      ClientAppJobStartRequest._();
  @$core.pragma('dart2js:noInline')
  static ClientAppJobStartRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppJobStartRequest>(
          ClientAppJobStartRequest.$_createMessage);
  static ClientAppJobStartRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get appName => $_getSZ(0);
  @$pb.TagNumber(1)
  set appName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAppName() => $_has(0);
  @$pb.TagNumber(1)
  void clearAppName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get method => $_getSZ(1);
  @$pb.TagNumber(2)
  set method($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMethod() => $_has(1);
  @$pb.TagNumber(2)
  void clearMethod() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get argsJson => $_getSZ(2);
  @$pb.TagNumber(3)
  set argsJson($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasArgsJson() => $_has(2);
  @$pb.TagNumber(3)
  void clearArgsJson() => $_clearField(3);
}

class ClientAppJobStartResponse extends $pb.GeneratedMessage {
  factory ClientAppJobStartResponse({
    $core.int? jobId,
  }) {
    final result = ClientAppJobStartResponse._();
    if (jobId != null) result.jobId = jobId;
    return result;
  }

  ClientAppJobStartResponse._();

  factory ClientAppJobStartResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppJobStartResponse()..mergeFromBuffer(data, registry);
  factory ClientAppJobStartResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppJobStartResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppJobStartResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppJobStartResponse.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'jobId', fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppJobStartResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppJobStartResponse copyWith(
          void Function(ClientAppJobStartResponse) updates) =>
      super.copyWith((message) => updates(message as ClientAppJobStartResponse))
          as ClientAppJobStartResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppJobStartResponse() / ClientAppJobStartResponse.new instead')
  static ClientAppJobStartResponse create() => ClientAppJobStartResponse._();
  static $pb.GeneratedMessage $_createMessage() =>
      ClientAppJobStartResponse._();
  @$core.override
  ClientAppJobStartResponse createEmptyInstance() =>
      ClientAppJobStartResponse._();
  @$core.pragma('dart2js:noInline')
  static ClientAppJobStartResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppJobStartResponse>(
          ClientAppJobStartResponse.$_createMessage);
  static ClientAppJobStartResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get jobId => $_getIZ(0);
  @$pb.TagNumber(1)
  set jobId($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasJobId() => $_has(0);
  @$pb.TagNumber(1)
  void clearJobId() => $_clearField(1);
}

class ClientAppJobCancelRequest extends $pb.GeneratedMessage {
  factory ClientAppJobCancelRequest({
    $core.int? jobId,
  }) {
    final result = ClientAppJobCancelRequest._();
    if (jobId != null) result.jobId = jobId;
    return result;
  }

  ClientAppJobCancelRequest._();

  factory ClientAppJobCancelRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppJobCancelRequest()..mergeFromBuffer(data, registry);
  factory ClientAppJobCancelRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppJobCancelRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppJobCancelRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppJobCancelRequest.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'jobId', fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppJobCancelRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppJobCancelRequest copyWith(
          void Function(ClientAppJobCancelRequest) updates) =>
      super.copyWith((message) => updates(message as ClientAppJobCancelRequest))
          as ClientAppJobCancelRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppJobCancelRequest() / ClientAppJobCancelRequest.new instead')
  static ClientAppJobCancelRequest create() => ClientAppJobCancelRequest._();
  static $pb.GeneratedMessage $_createMessage() =>
      ClientAppJobCancelRequest._();
  @$core.override
  ClientAppJobCancelRequest createEmptyInstance() =>
      ClientAppJobCancelRequest._();
  @$core.pragma('dart2js:noInline')
  static ClientAppJobCancelRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppJobCancelRequest>(
          ClientAppJobCancelRequest.$_createMessage);
  static ClientAppJobCancelRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get jobId => $_getIZ(0);
  @$pb.TagNumber(1)
  set jobId($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasJobId() => $_has(0);
  @$pb.TagNumber(1)
  void clearJobId() => $_clearField(1);
}

class ClientAppJobCancelResponse extends $pb.GeneratedMessage {
  factory ClientAppJobCancelResponse() => ClientAppJobCancelResponse._();

  ClientAppJobCancelResponse._();

  factory ClientAppJobCancelResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppJobCancelResponse()..mergeFromBuffer(data, registry);
  factory ClientAppJobCancelResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientAppJobCancelResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAppJobCancelResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: ClientAppJobCancelResponse.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppJobCancelResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAppJobCancelResponse copyWith(
          void Function(ClientAppJobCancelResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientAppJobCancelResponse))
          as ClientAppJobCancelResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ClientAppJobCancelResponse() / ClientAppJobCancelResponse.new instead')
  static ClientAppJobCancelResponse create() => ClientAppJobCancelResponse._();
  static $pb.GeneratedMessage $_createMessage() =>
      ClientAppJobCancelResponse._();
  @$core.override
  ClientAppJobCancelResponse createEmptyInstance() =>
      ClientAppJobCancelResponse._();
  @$core.pragma('dart2js:noInline')
  static ClientAppJobCancelResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAppJobCancelResponse>(
          ClientAppJobCancelResponse.$_createMessage);
  static ClientAppJobCancelResponse? _defaultInstance;
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
