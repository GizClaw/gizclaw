// This is a generated file - do not edit.
//
// Generated from payload/gnss.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

class ClientGnssReportingGetRequest extends $pb.GeneratedMessage {
  factory ClientGnssReportingGetRequest() => create();

  ClientGnssReportingGetRequest._();

  factory ClientGnssReportingGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientGnssReportingGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientGnssReportingGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGnssReportingGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGnssReportingGetRequest copyWith(
          void Function(ClientGnssReportingGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientGnssReportingGetRequest))
          as ClientGnssReportingGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientGnssReportingGetRequest create() =>
      ClientGnssReportingGetRequest._();
  @$core.override
  ClientGnssReportingGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientGnssReportingGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientGnssReportingGetRequest>(create);
  static ClientGnssReportingGetRequest? _defaultInstance;
}

class ClientGnssReportingGetResponse extends $pb.GeneratedMessage {
  factory ClientGnssReportingGetResponse({
    $core.bool? enabled,
  }) {
    final result = create();
    if (enabled != null) result.enabled = enabled;
    return result;
  }

  ClientGnssReportingGetResponse._();

  factory ClientGnssReportingGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientGnssReportingGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientGnssReportingGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'enabled')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGnssReportingGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGnssReportingGetResponse copyWith(
          void Function(ClientGnssReportingGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientGnssReportingGetResponse))
          as ClientGnssReportingGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientGnssReportingGetResponse create() =>
      ClientGnssReportingGetResponse._();
  @$core.override
  ClientGnssReportingGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientGnssReportingGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientGnssReportingGetResponse>(create);
  static ClientGnssReportingGetResponse? _defaultInstance;

  /// Required presence, including an explicit false. Read from the device.
  @$pb.TagNumber(1)
  $core.bool get enabled => $_getBF(0);
  @$pb.TagNumber(1)
  set enabled($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasEnabled() => $_has(0);
  @$pb.TagNumber(1)
  void clearEnabled() => $_clearField(1);
}

class ClientGnssReportingSetRequest extends $pb.GeneratedMessage {
  factory ClientGnssReportingSetRequest({
    $core.bool? enabled,
  }) {
    final result = create();
    if (enabled != null) result.enabled = enabled;
    return result;
  }

  ClientGnssReportingSetRequest._();

  factory ClientGnssReportingSetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientGnssReportingSetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientGnssReportingSetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'enabled')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGnssReportingSetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGnssReportingSetRequest copyWith(
          void Function(ClientGnssReportingSetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientGnssReportingSetRequest))
          as ClientGnssReportingSetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientGnssReportingSetRequest create() =>
      ClientGnssReportingSetRequest._();
  @$core.override
  ClientGnssReportingSetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientGnssReportingSetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientGnssReportingSetRequest>(create);
  static ClientGnssReportingSetRequest? _defaultInstance;

  /// Required presence. Sets the value explicitly; never toggles it.
  @$pb.TagNumber(1)
  $core.bool get enabled => $_getBF(0);
  @$pb.TagNumber(1)
  set enabled($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasEnabled() => $_has(0);
  @$pb.TagNumber(1)
  void clearEnabled() => $_clearField(1);
}

class ClientGnssReportingSetResponse extends $pb.GeneratedMessage {
  factory ClientGnssReportingSetResponse({
    $core.bool? enabled,
  }) {
    final result = create();
    if (enabled != null) result.enabled = enabled;
    return result;
  }

  ClientGnssReportingSetResponse._();

  factory ClientGnssReportingSetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientGnssReportingSetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientGnssReportingSetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'enabled')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGnssReportingSetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientGnssReportingSetResponse copyWith(
          void Function(ClientGnssReportingSetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientGnssReportingSetResponse))
          as ClientGnssReportingSetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientGnssReportingSetResponse create() =>
      ClientGnssReportingSetResponse._();
  @$core.override
  ClientGnssReportingSetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientGnssReportingSetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientGnssReportingSetResponse>(create);
  static ClientGnssReportingSetResponse? _defaultInstance;

  /// Required presence. The value in effect after the device applied the request.
  @$pb.TagNumber(1)
  $core.bool get enabled => $_getBF(0);
  @$pb.TagNumber(1)
  set enabled($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasEnabled() => $_has(0);
  @$pb.TagNumber(1)
  void clearEnabled() => $_clearField(1);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
