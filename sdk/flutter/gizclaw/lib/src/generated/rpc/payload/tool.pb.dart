// This is a generated file - do not edit.
//
// Generated from payload/tool.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

import 'tool.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'tool.pbenum.dart';

/// ClientToolOptions binds a ClientTool to its stable name and to the payload
/// messages carried by invoke. It mirrors RpcMethodOptions so the same
/// generators read both registries.
class ClientToolOptions extends $pb.GeneratedMessage {
  factory ClientToolOptions({
    $core.String? name,
    $core.String? request,
    $core.String? response,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (request != null) result.request = request;
    if (response != null) result.response = response;
    return result;
  }

  ClientToolOptions._();

  factory ClientToolOptions.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientToolOptions.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientToolOptions',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'request')
    ..aOS(3, _omitFieldNames ? '' : 'response')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolOptions clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolOptions copyWith(void Function(ClientToolOptions) updates) =>
      super.copyWith((message) => updates(message as ClientToolOptions))
          as ClientToolOptions;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientToolOptions create() => ClientToolOptions._();
  @$core.override
  ClientToolOptions createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientToolOptions getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientToolOptions>(create);
  static ClientToolOptions? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get request => $_getSZ(1);
  @$pb.TagNumber(2)
  set request($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRequest() => $_has(1);
  @$pb.TagNumber(2)
  void clearRequest() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get response => $_getSZ(2);
  @$pb.TagNumber(3)
  set response($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasResponse() => $_has(2);
  @$pb.TagNumber(3)
  void clearResponse() => $_clearField(3);
}

/// ClientToolV0InvokeRequest carries one tool call. payload is the encoded
/// request message the tool declares; it is absent when that message has no
/// fields. A tool the device does not implement answers UNIMPLEMENTED, matching
/// the method-not-found answer the removed per-tool methods used to give.
class ClientToolV0InvokeRequest extends $pb.GeneratedMessage {
  factory ClientToolV0InvokeRequest({
    ClientTool? tool,
    $core.List<$core.int>? payload,
  }) {
    final result = create();
    if (tool != null) result.tool = tool;
    if (payload != null) result.payload = payload;
    return result;
  }

  ClientToolV0InvokeRequest._();

  factory ClientToolV0InvokeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientToolV0InvokeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientToolV0InvokeRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<ClientTool>(1, _omitFieldNames ? '' : 'tool',
        enumValues: ClientTool.values)
    ..a<$core.List<$core.int>>(
        2, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolV0InvokeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolV0InvokeRequest copyWith(
          void Function(ClientToolV0InvokeRequest) updates) =>
      super.copyWith((message) => updates(message as ClientToolV0InvokeRequest))
          as ClientToolV0InvokeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientToolV0InvokeRequest create() => ClientToolV0InvokeRequest._();
  @$core.override
  ClientToolV0InvokeRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientToolV0InvokeRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientToolV0InvokeRequest>(create);
  static ClientToolV0InvokeRequest? _defaultInstance;

  @$pb.TagNumber(1)
  ClientTool get tool => $_getN(0);
  @$pb.TagNumber(1)
  set tool(ClientTool value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasTool() => $_has(0);
  @$pb.TagNumber(1)
  void clearTool() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.List<$core.int> get payload => $_getN(1);
  @$pb.TagNumber(2)
  set payload($core.List<$core.int> value) => $_setBytes(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPayload() => $_has(1);
  @$pb.TagNumber(2)
  void clearPayload() => $_clearField(2);
}

/// ClientToolV0InvokeResponse carries the encoded response message the invoked
/// tool declares, absent when that message has no fields. Failures travel as an
/// RpcStatus on the envelope, not as a payload.
class ClientToolV0InvokeResponse extends $pb.GeneratedMessage {
  factory ClientToolV0InvokeResponse({
    $core.List<$core.int>? payload,
  }) {
    final result = create();
    if (payload != null) result.payload = payload;
    return result;
  }

  ClientToolV0InvokeResponse._();

  factory ClientToolV0InvokeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientToolV0InvokeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientToolV0InvokeResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..a<$core.List<$core.int>>(
        1, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolV0InvokeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolV0InvokeResponse copyWith(
          void Function(ClientToolV0InvokeResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientToolV0InvokeResponse))
          as ClientToolV0InvokeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientToolV0InvokeResponse create() => ClientToolV0InvokeResponse._();
  @$core.override
  ClientToolV0InvokeResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientToolV0InvokeResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientToolV0InvokeResponse>(create);
  static ClientToolV0InvokeResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.List<$core.int> get payload => $_getN(0);
  @$pb.TagNumber(1)
  set payload($core.List<$core.int> value) => $_setBytes(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPayload() => $_has(0);
  @$pb.TagNumber(1)
  void clearPayload() => $_clearField(1);
}

class ClientToolV0ListRequest extends $pb.GeneratedMessage {
  factory ClientToolV0ListRequest() => create();

  ClientToolV0ListRequest._();

  factory ClientToolV0ListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientToolV0ListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientToolV0ListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolV0ListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolV0ListRequest copyWith(
          void Function(ClientToolV0ListRequest) updates) =>
      super.copyWith((message) => updates(message as ClientToolV0ListRequest))
          as ClientToolV0ListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientToolV0ListRequest create() => ClientToolV0ListRequest._();
  @$core.override
  ClientToolV0ListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientToolV0ListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientToolV0ListRequest>(create);
  static ClientToolV0ListRequest? _defaultInstance;
}

/// ClientToolV0ListResponse lists the tools the device actually implements, so
/// a caller hides or skips a control it would only fail. Unknown values must be
/// ignored rather than rejected, and the list never includes UNSPECIFIED.
class ClientToolV0ListResponse extends $pb.GeneratedMessage {
  factory ClientToolV0ListResponse({
    $core.Iterable<ClientTool>? tools,
  }) {
    final result = create();
    if (tools != null) result.tools.addAll(tools);
    return result;
  }

  ClientToolV0ListResponse._();

  factory ClientToolV0ListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientToolV0ListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientToolV0ListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pc<ClientTool>(1, _omitFieldNames ? '' : 'tools', $pb.PbFieldType.KE,
        valueOf: ClientTool.valueOf,
        enumValues: ClientTool.values,
        defaultEnumValue: ClientTool.CLIENT_TOOL_UNSPECIFIED)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolV0ListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientToolV0ListResponse copyWith(
          void Function(ClientToolV0ListResponse) updates) =>
      super.copyWith((message) => updates(message as ClientToolV0ListResponse))
          as ClientToolV0ListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientToolV0ListResponse create() => ClientToolV0ListResponse._();
  @$core.override
  ClientToolV0ListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientToolV0ListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientToolV0ListResponse>(create);
  static ClientToolV0ListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<ClientTool> get tools => $_getList(0);
}

class Tool {
  static final clientTool = $pb.Extension<ClientToolOptions>(
      _omitMessageNames ? '' : 'google.protobuf.EnumValueOptions',
      _omitFieldNames ? '' : 'clientTool',
      51001,
      $pb.PbFieldType.OM,
      defaultOrMaker: ClientToolOptions.getDefault,
      subBuilder: ClientToolOptions.create);
  static void registerAllExtensions($pb.ExtensionRegistry registry) {
    registry.add(clientTool);
  }
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
