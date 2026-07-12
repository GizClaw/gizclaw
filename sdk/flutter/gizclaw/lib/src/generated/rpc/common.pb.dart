// This is a generated file - do not edit.
//
// Generated from common.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

import 'common.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'common.pbenum.dart';

enum RpcResponse_Body { payload, error, notSet }

class RpcResponse extends $pb.GeneratedMessage {
  factory RpcResponse({
    $core.String? id,
    $core.List<$core.int>? payload,
    RpcError? error,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (payload != null) result.payload = payload;
    if (error != null) result.error = error;
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
    3: RpcResponse_Body.error,
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
    ..aOM<RpcError>(3, _omitFieldNames ? '' : 'error',
        subBuilder: RpcError.create)
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
  RpcError get error => $_getN(2);
  @$pb.TagNumber(3)
  set error(RpcError value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasError() => $_has(2);
  @$pb.TagNumber(3)
  void clearError() => $_clearField(3);
  @$pb.TagNumber(3)
  RpcError ensureError() => $_ensure(2);
}

enum RpcStreamFrame_Body { payload, error, end, notSet }

class RpcStreamFrame extends $pb.GeneratedMessage {
  factory RpcStreamFrame({
    $core.String? id,
    $core.List<$core.int>? payload,
    RpcError? error,
    RpcStreamEnd? end,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (payload != null) result.payload = payload;
    if (error != null) result.error = error;
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
    3: RpcStreamFrame_Body.error,
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
    ..aOM<RpcError>(3, _omitFieldNames ? '' : 'error',
        subBuilder: RpcError.create)
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
  RpcError get error => $_getN(2);
  @$pb.TagNumber(3)
  set error(RpcError value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasError() => $_has(2);
  @$pb.TagNumber(3)
  void clearError() => $_clearField(3);
  @$pb.TagNumber(3)
  RpcError ensureError() => $_ensure(2);

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

class RpcError extends $pb.GeneratedMessage {
  factory RpcError({
    RpcErrorCode? code,
    $core.String? message,
  }) {
    final result = create();
    if (code != null) result.code = code;
    if (message != null) result.message = message;
    return result;
  }

  RpcError._();

  factory RpcError.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RpcError.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RpcError',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<RpcErrorCode>(1, _omitFieldNames ? '' : 'code',
        enumValues: RpcErrorCode.values)
    ..aOS(2, _omitFieldNames ? '' : 'message')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcError clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RpcError copyWith(void Function(RpcError) updates) =>
      super.copyWith((message) => updates(message as RpcError)) as RpcError;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RpcError create() => RpcError._();
  @$core.override
  RpcError createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RpcError getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<RpcError>(create);
  static RpcError? _defaultInstance;

  @$pb.TagNumber(1)
  RpcErrorCode get code => $_getN(0);
  @$pb.TagNumber(1)
  set code(RpcErrorCode value) => $_setField(1, value);
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

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
