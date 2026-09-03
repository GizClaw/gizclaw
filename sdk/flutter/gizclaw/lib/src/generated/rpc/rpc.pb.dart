// This is a generated file - do not edit.
//
// Generated from rpc.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

import 'rpc.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'rpc.pbenum.dart';

enum RpcResponse_Body { payload, status, notSet }

class RpcResponse extends $pb.GeneratedMessage {
  factory RpcResponse({
    $core.String? id,
    $core.List<$core.int>? payload,
    RpcStatus? status,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (payload != null) result.payload = payload;
    if (status != null) result.status = status;
    return result;
  }

  RpcResponse._();

  factory RpcResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RpcResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, RpcResponse_Body> _RpcResponse_BodyByTag = {
    2: RpcResponse_Body.payload,
    3: RpcResponse_Body.status,
    0: RpcResponse_Body.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RpcResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [2, 3])
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..a<$core.List<$core.int>>(
        2, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..aOM<RpcStatus>(3, _omitFieldNames ? '' : 'status',
        subBuilder: RpcStatus.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcResponse copyWith(void Function(RpcResponse) updates) =>
      super.copyWith((message) => updates(message as RpcResponse))
          as RpcResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RpcResponse create() => RpcResponse._();
  @$core.override
  RpcResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RpcResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RpcResponse>(create);
  static RpcResponse? _defaultInstance;

  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  RpcResponse_Body whichBody() => _RpcResponse_BodyByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  void clearBody() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.List<$core.int> get payload => $_getN(1);
  @$pb.TagNumber(2)
  set payload($core.List<$core.int> value) => $_setBytes(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPayload() => $_has(1);
  @$pb.TagNumber(2)
  void clearPayload() => $_clearField(2);

  @$pb.TagNumber(3)
  RpcStatus get status => $_getN(2);
  @$pb.TagNumber(3)
  set status(RpcStatus value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasStatus() => $_has(2);
  @$pb.TagNumber(3)
  void clearStatus() => $_clearField(3);
  @$pb.TagNumber(3)
  RpcStatus ensureStatus() => $_ensure(2);
}

enum RpcStreamFrame_Body { payload, status, end, notSet }

class RpcStreamFrame extends $pb.GeneratedMessage {
  factory RpcStreamFrame({
    $core.String? id,
    $core.List<$core.int>? payload,
    RpcStatus? status,
    RpcStreamEnd? end,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (payload != null) result.payload = payload;
    if (status != null) result.status = status;
    if (end != null) result.end = end;
    return result;
  }

  RpcStreamFrame._();

  factory RpcStreamFrame.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RpcStreamFrame.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, RpcStreamFrame_Body>
      _RpcStreamFrame_BodyByTag = {
    2: RpcStreamFrame_Body.payload,
    3: RpcStreamFrame_Body.status,
    4: RpcStreamFrame_Body.end,
    0: RpcStreamFrame_Body.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RpcStreamFrame',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [2, 3, 4])
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..a<$core.List<$core.int>>(
        2, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..aOM<RpcStatus>(3, _omitFieldNames ? '' : 'status',
        subBuilder: RpcStatus.create)
    ..aOM<RpcStreamEnd>(4, _omitFieldNames ? '' : 'end',
        subBuilder: RpcStreamEnd.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcStreamFrame clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcStreamFrame copyWith(void Function(RpcStreamFrame) updates) =>
      super.copyWith((message) => updates(message as RpcStreamFrame))
          as RpcStreamFrame;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RpcStreamFrame create() => RpcStreamFrame._();
  @$core.override
  RpcStreamFrame createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RpcStreamFrame getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RpcStreamFrame>(create);
  static RpcStreamFrame? _defaultInstance;

  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  RpcStreamFrame_Body whichBody() =>
      _RpcStreamFrame_BodyByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  void clearBody() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.List<$core.int> get payload => $_getN(1);
  @$pb.TagNumber(2)
  set payload($core.List<$core.int> value) => $_setBytes(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPayload() => $_has(1);
  @$pb.TagNumber(2)
  void clearPayload() => $_clearField(2);

  @$pb.TagNumber(3)
  RpcStatus get status => $_getN(2);
  @$pb.TagNumber(3)
  set status(RpcStatus value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasStatus() => $_has(2);
  @$pb.TagNumber(3)
  void clearStatus() => $_clearField(3);
  @$pb.TagNumber(3)
  RpcStatus ensureStatus() => $_ensure(2);

  @$pb.TagNumber(4)
  RpcStreamEnd get end => $_getN(3);
  @$pb.TagNumber(4)
  set end(RpcStreamEnd value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasEnd() => $_has(3);
  @$pb.TagNumber(4)
  void clearEnd() => $_clearField(4);
  @$pb.TagNumber(4)
  RpcStreamEnd ensureEnd() => $_ensure(3);
}

/// RpcStatus is the terminal status of one RPC. It follows google.rpc.Status
/// with one deviation: details is a typed ErrorInfo rather than
/// repeated google.protobuf.Any, because nanopb cannot resolve type URLs at
/// runtime and the C SDK allocates nothing.
class RpcStatus extends $pb.GeneratedMessage {
  factory RpcStatus({
    StatusCode? code,
    $core.String? message,
    ErrorInfo? info,
  }) {
    final result = create();
    if (code != null) result.code = code;
    if (message != null) result.message = message;
    if (info != null) result.info = info;
    return result;
  }

  RpcStatus._();

  factory RpcStatus.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RpcStatus.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RpcStatus',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<StatusCode>(1, _omitFieldNames ? '' : 'code',
        enumValues: StatusCode.values)
    ..aOS(2, _omitFieldNames ? '' : 'message')
    ..aOM<ErrorInfo>(3, _omitFieldNames ? '' : 'info',
        subBuilder: ErrorInfo.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcStatus clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcStatus copyWith(void Function(RpcStatus) updates) =>
      super.copyWith((message) => updates(message as RpcStatus)) as RpcStatus;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RpcStatus create() => RpcStatus._();
  @$core.override
  RpcStatus createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RpcStatus getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<RpcStatus>(create);
  static RpcStatus? _defaultInstance;

  @$pb.TagNumber(1)
  StatusCode get code => $_getN(0);
  @$pb.TagNumber(1)
  set code(StatusCode value) => $_setField(1, value);
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
  ErrorInfo get info => $_getN(2);
  @$pb.TagNumber(3)
  set info(ErrorInfo value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasInfo() => $_has(2);
  @$pb.TagNumber(3)
  void clearInfo() => $_clearField(3);
  @$pb.TagNumber(3)
  ErrorInfo ensureInfo() => $_ensure(2);
}

/// ErrorInfo names the specific failure behind a StatusCode. code is the class
/// a client branches on; reason is the reason a human or a UI reads. It follows
/// google.rpc.ErrorInfo without the metadata map, which no caller needs and
/// which nanopb can only express as a bounded repeated entry message.
class ErrorInfo extends $pb.GeneratedMessage {
  factory ErrorInfo({
    $core.String? reason,
    $core.String? domain,
  }) {
    final result = create();
    if (reason != null) result.reason = reason;
    if (domain != null) result.domain = domain;
    return result;
  }

  ErrorInfo._();

  factory ErrorInfo.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ErrorInfo.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ErrorInfo',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'reason')
    ..aOS(2, _omitFieldNames ? '' : 'domain')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ErrorInfo clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ErrorInfo copyWith(void Function(ErrorInfo) updates) =>
      super.copyWith((message) => updates(message as ErrorInfo)) as ErrorInfo;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ErrorInfo create() => ErrorInfo._();
  @$core.override
  ErrorInfo createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ErrorInfo getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ErrorInfo>(create);
  static ErrorInfo? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get reason => $_getSZ(0);
  @$pb.TagNumber(1)
  set reason($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasReason() => $_has(0);
  @$pb.TagNumber(1)
  void clearReason() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get domain => $_getSZ(1);
  @$pb.TagNumber(2)
  set domain($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDomain() => $_has(1);
  @$pb.TagNumber(2)
  void clearDomain() => $_clearField(2);
}

class RpcStreamEnd extends $pb.GeneratedMessage {
  factory RpcStreamEnd() => create();

  RpcStreamEnd._();

  factory RpcStreamEnd.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RpcStreamEnd.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RpcStreamEnd',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcStreamEnd clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcStreamEnd copyWith(void Function(RpcStreamEnd) updates) =>
      super.copyWith((message) => updates(message as RpcStreamEnd))
          as RpcStreamEnd;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RpcStreamEnd create() => RpcStreamEnd._();
  @$core.override
  RpcStreamEnd createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RpcStreamEnd getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RpcStreamEnd>(create);
  static RpcStreamEnd? _defaultInstance;
}

class RpcMethodOptions extends $pb.GeneratedMessage {
  factory RpcMethodOptions({
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

  RpcMethodOptions._();

  factory RpcMethodOptions.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RpcMethodOptions.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RpcMethodOptions',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'request')
    ..aOS(3, _omitFieldNames ? '' : 'response')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcMethodOptions clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcMethodOptions copyWith(void Function(RpcMethodOptions) updates) =>
      super.copyWith((message) => updates(message as RpcMethodOptions))
          as RpcMethodOptions;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RpcMethodOptions create() => RpcMethodOptions._();
  @$core.override
  RpcMethodOptions createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RpcMethodOptions getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RpcMethodOptions>(create);
  static RpcMethodOptions? _defaultInstance;

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

class RpcRequest extends $pb.GeneratedMessage {
  factory RpcRequest({
    $core.String? id,
    RpcMethod? method,
    $core.List<$core.int>? payload,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (method != null) result.method = method;
    if (payload != null) result.payload = payload;
    return result;
  }

  RpcRequest._();

  factory RpcRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RpcRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RpcRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aE<RpcMethod>(2, _omitFieldNames ? '' : 'method',
        enumValues: RpcMethod.values)
    ..a<$core.List<$core.int>>(
        3, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcRequest copyWith(void Function(RpcRequest) updates) =>
      super.copyWith((message) => updates(message as RpcRequest)) as RpcRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RpcRequest create() => RpcRequest._();
  @$core.override
  RpcRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RpcRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RpcRequest>(create);
  static RpcRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  RpcMethod get method => $_getN(1);
  @$pb.TagNumber(2)
  set method(RpcMethod value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasMethod() => $_has(1);
  @$pb.TagNumber(2)
  void clearMethod() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.List<$core.int> get payload => $_getN(2);
  @$pb.TagNumber(3)
  set payload($core.List<$core.int> value) => $_setBytes(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPayload() => $_has(2);
  @$pb.TagNumber(3)
  void clearPayload() => $_clearField(3);
}

class Rpc {
  static final rpcMethod = $pb.Extension<RpcMethodOptions>(
      _omitMessageNames ? '' : 'google.protobuf.EnumValueOptions',
      _omitFieldNames ? '' : 'rpcMethod',
      51000,
      $pb.PbFieldType.OM,
      defaultOrMaker: RpcMethodOptions.getDefault,
      subBuilder: RpcMethodOptions.create);
  static void registerAllExtensions($pb.ExtensionRegistry registry) {
    registry.add(rpcMethod);
  }
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
