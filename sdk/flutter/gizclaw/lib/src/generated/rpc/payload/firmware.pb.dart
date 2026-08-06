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

class FirmwareGetRequest extends $pb.GeneratedMessage {
  factory FirmwareGetRequest({
    $0.FirmwareChannelName? channel,
  }) {
    final result = create();
    if (channel != null) result.channel = channel;
    return result;
  }

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
    ..aE<$0.FirmwareChannelName>(1, _omitFieldNames ? '' : 'channel',
        enumValues: $0.FirmwareChannelName.values)
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

  @$pb.TagNumber(1)
  $0.FirmwareChannelName get channel => $_getN(0);
  @$pb.TagNumber(1)
  set channel($0.FirmwareChannelName value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasChannel() => $_has(0);
  @$pb.TagNumber(1)
  void clearChannel() => $_clearField(1);
}

class FirmwareGetResponse extends $pb.GeneratedMessage {
  factory FirmwareGetResponse({
    $0.FirmwareChannelName? channel,
    $core.String? description,
    $core.String? url,
    $core.String? sha256,
    $fixnum.Int64? size,
  }) {
    final result = create();
    if (channel != null) result.channel = channel;
    if (description != null) result.description = description;
    if (url != null) result.url = url;
    if (sha256 != null) result.sha256 = sha256;
    if (size != null) result.size = size;
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
    ..aE<$0.FirmwareChannelName>(1, _omitFieldNames ? '' : 'channel',
        enumValues: $0.FirmwareChannelName.values)
    ..aOS(2, _omitFieldNames ? '' : 'description')
    ..aOS(3, _omitFieldNames ? '' : 'url')
    ..aOS(4, _omitFieldNames ? '' : 'sha256')
    ..aInt64(5, _omitFieldNames ? '' : 'size')
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
  $0.FirmwareChannelName get channel => $_getN(0);
  @$pb.TagNumber(1)
  set channel($0.FirmwareChannelName value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasChannel() => $_has(0);
  @$pb.TagNumber(1)
  void clearChannel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get description => $_getSZ(1);
  @$pb.TagNumber(2)
  set description($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDescription() => $_has(1);
  @$pb.TagNumber(2)
  void clearDescription() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get url => $_getSZ(2);
  @$pb.TagNumber(3)
  set url($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasUrl() => $_has(2);
  @$pb.TagNumber(3)
  void clearUrl() => $_clearField(3);

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
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
