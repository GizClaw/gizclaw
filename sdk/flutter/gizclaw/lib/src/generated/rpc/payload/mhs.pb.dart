// This is a generated file - do not edit.
//
// Generated from payload/mhs.proto.

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

enum MhsValue_Value { boolValue, intValue, doubleValue, stringValue, notSet }

/// Exactly one value must be present, including false, zero and empty string.
/// Enum states use string_value. Strings are UTF-8, without NUL, at most 256
/// bytes. Integers are restricted to the JSON safe range +/-9007199254740991.
class MhsValue extends $pb.GeneratedMessage {
  factory MhsValue({
    $core.bool? boolValue,
    $fixnum.Int64? intValue,
    $core.double? doubleValue,
    $core.String? stringValue,
  }) {
    final result = create();
    if (boolValue != null) result.boolValue = boolValue;
    if (intValue != null) result.intValue = intValue;
    if (doubleValue != null) result.doubleValue = doubleValue;
    if (stringValue != null) result.stringValue = stringValue;
    return result;
  }

  MhsValue._();

  factory MhsValue.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MhsValue.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, MhsValue_Value> _MhsValue_ValueByTag = {
    1: MhsValue_Value.boolValue,
    2: MhsValue_Value.intValue,
    3: MhsValue_Value.doubleValue,
    4: MhsValue_Value.stringValue,
    0: MhsValue_Value.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MhsValue',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2, 3, 4])
    ..aOB(1, _omitFieldNames ? '' : 'boolValue')
    ..aInt64(2, _omitFieldNames ? '' : 'intValue')
    ..aD(3, _omitFieldNames ? '' : 'doubleValue')
    ..aOS(4, _omitFieldNames ? '' : 'stringValue')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MhsValue clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MhsValue copyWith(void Function(MhsValue) updates) =>
      super.copyWith((message) => updates(message as MhsValue)) as MhsValue;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MhsValue create() => MhsValue._();
  @$core.override
  MhsValue createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MhsValue getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<MhsValue>(create);
  static MhsValue? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  MhsValue_Value whichValue() => _MhsValue_ValueByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  void clearValue() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  $core.bool get boolValue => $_getBF(0);
  @$pb.TagNumber(1)
  set boolValue($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasBoolValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearBoolValue() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get intValue => $_getI64(1);
  @$pb.TagNumber(2)
  set intValue($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasIntValue() => $_has(1);
  @$pb.TagNumber(2)
  void clearIntValue() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.double get doubleValue => $_getN(2);
  @$pb.TagNumber(3)
  set doubleValue($core.double value) => $_setDouble(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDoubleValue() => $_has(2);
  @$pb.TagNumber(3)
  void clearDoubleValue() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get stringValue => $_getSZ(3);
  @$pb.TagNumber(4)
  set stringValue($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasStringValue() => $_has(3);
  @$pb.TagNumber(4)
  void clearStringValue() => $_clearField(4);
}

/// Keys are case-sensitive and at most 64 ASCII bytes each.
class MhsStateRef extends $pb.GeneratedMessage {
  factory MhsStateRef({
    $core.String? deviceId,
    $core.String? state,
  }) {
    final result = create();
    if (deviceId != null) result.deviceId = deviceId;
    if (state != null) result.state = state;
    return result;
  }

  MhsStateRef._();

  factory MhsStateRef.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MhsStateRef.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MhsStateRef',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..aOS(2, _omitFieldNames ? '' : 'state')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MhsStateRef clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MhsStateRef copyWith(void Function(MhsStateRef) updates) =>
      super.copyWith((message) => updates(message as MhsStateRef))
          as MhsStateRef;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MhsStateRef create() => MhsStateRef._();
  @$core.override
  MhsStateRef createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MhsStateRef getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<MhsStateRef>(create);
  static MhsStateRef? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get state => $_getSZ(1);
  @$pb.TagNumber(2)
  set state($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasState() => $_has(1);
  @$pb.TagNumber(2)
  void clearState() => $_clearField(2);
}

class MhsStateValue extends $pb.GeneratedMessage {
  factory MhsStateValue({
    $core.String? deviceId,
    $core.String? state,
    MhsValue? value,
  }) {
    final result = create();
    if (deviceId != null) result.deviceId = deviceId;
    if (state != null) result.state = state;
    if (value != null) result.value = value;
    return result;
  }

  MhsStateValue._();

  factory MhsStateValue.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MhsStateValue.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MhsStateValue',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..aOS(2, _omitFieldNames ? '' : 'state')
    ..aOM<MhsValue>(3, _omitFieldNames ? '' : 'value',
        subBuilder: MhsValue.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MhsStateValue clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MhsStateValue copyWith(void Function(MhsStateValue) updates) =>
      super.copyWith((message) => updates(message as MhsStateValue))
          as MhsStateValue;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MhsStateValue create() => MhsStateValue._();
  @$core.override
  MhsStateValue createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MhsStateValue getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<MhsStateValue>(create);
  static MhsStateValue? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get state => $_getSZ(1);
  @$pb.TagNumber(2)
  set state($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasState() => $_has(1);
  @$pb.TagNumber(2)
  void clearState() => $_clearField(2);

  @$pb.TagNumber(3)
  MhsValue get value => $_getN(2);
  @$pb.TagNumber(3)
  set value(MhsValue value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasValue() => $_has(2);
  @$pb.TagNumber(3)
  void clearValue() => $_clearField(3);
  @$pb.TagNumber(3)
  MhsValue ensureValue() => $_ensure(2);
}

/// Requests contain 1-32 unique keys. An unimplemented key returns NOT_FOUND,
/// even if the shared RuntimeProfile advertises it. Invalid requests return
/// INVALID_ARGUMENT. Responses contain exactly the requested keys, once each.
class ClientMhsV0ReadRequest extends $pb.GeneratedMessage {
  factory ClientMhsV0ReadRequest({
    $core.Iterable<MhsStateRef>? states,
  }) {
    final result = create();
    if (states != null) result.states.addAll(states);
    return result;
  }

  ClientMhsV0ReadRequest._();

  factory ClientMhsV0ReadRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientMhsV0ReadRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientMhsV0ReadRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<MhsStateRef>(1, _omitFieldNames ? '' : 'states',
        subBuilder: MhsStateRef.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMhsV0ReadRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMhsV0ReadRequest copyWith(
          void Function(ClientMhsV0ReadRequest) updates) =>
      super.copyWith((message) => updates(message as ClientMhsV0ReadRequest))
          as ClientMhsV0ReadRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientMhsV0ReadRequest create() => ClientMhsV0ReadRequest._();
  @$core.override
  ClientMhsV0ReadRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientMhsV0ReadRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientMhsV0ReadRequest>(create);
  static ClientMhsV0ReadRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<MhsStateRef> get states => $_getList(0);
}

class ClientMhsV0ReadResponse extends $pb.GeneratedMessage {
  factory ClientMhsV0ReadResponse({
    $core.Iterable<MhsStateValue>? states,
  }) {
    final result = create();
    if (states != null) result.states.addAll(states);
    return result;
  }

  ClientMhsV0ReadResponse._();

  factory ClientMhsV0ReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientMhsV0ReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientMhsV0ReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<MhsStateValue>(1, _omitFieldNames ? '' : 'states',
        subBuilder: MhsStateValue.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMhsV0ReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMhsV0ReadResponse copyWith(
          void Function(ClientMhsV0ReadResponse) updates) =>
      super.copyWith((message) => updates(message as ClientMhsV0ReadResponse))
          as ClientMhsV0ReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientMhsV0ReadResponse create() => ClientMhsV0ReadResponse._();
  @$core.override
  ClientMhsV0ReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientMhsV0ReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientMhsV0ReadResponse>(create);
  static ClientMhsV0ReadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<MhsStateValue> get states => $_getList(0);
}

/// A write is all-or-nothing from the caller's view: validate every entry and
/// precondition before applying any. Reject the whole batch with INVALID_ARGUMENT,
/// NOT_FOUND or FAILED_PRECONDITION rather than applying a partial batch. Drivers
/// enforce their own safety limits and may clamp/round within the manifest bounds.
/// No procedures, notifications, slots or streams are part of this protocol.
class ClientMhsV0WriteRequest extends $pb.GeneratedMessage {
  factory ClientMhsV0WriteRequest({
    $core.Iterable<MhsStateValue>? states,
  }) {
    final result = create();
    if (states != null) result.states.addAll(states);
    return result;
  }

  ClientMhsV0WriteRequest._();

  factory ClientMhsV0WriteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientMhsV0WriteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientMhsV0WriteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<MhsStateValue>(1, _omitFieldNames ? '' : 'states',
        subBuilder: MhsStateValue.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMhsV0WriteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMhsV0WriteRequest copyWith(
          void Function(ClientMhsV0WriteRequest) updates) =>
      super.copyWith((message) => updates(message as ClientMhsV0WriteRequest))
          as ClientMhsV0WriteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientMhsV0WriteRequest create() => ClientMhsV0WriteRequest._();
  @$core.override
  ClientMhsV0WriteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientMhsV0WriteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientMhsV0WriteRequest>(create);
  static ClientMhsV0WriteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<MhsStateValue> get states => $_getList(0);
}

/// Return exactly the requested keys and the values actually in effect after
/// the write, once each. A successful response never omits a rejected entry.
class ClientMhsV0WriteResponse extends $pb.GeneratedMessage {
  factory ClientMhsV0WriteResponse({
    $core.Iterable<MhsStateValue>? states,
  }) {
    final result = create();
    if (states != null) result.states.addAll(states);
    return result;
  }

  ClientMhsV0WriteResponse._();

  factory ClientMhsV0WriteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientMhsV0WriteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientMhsV0WriteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<MhsStateValue>(1, _omitFieldNames ? '' : 'states',
        subBuilder: MhsStateValue.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMhsV0WriteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMhsV0WriteResponse copyWith(
          void Function(ClientMhsV0WriteResponse) updates) =>
      super.copyWith((message) => updates(message as ClientMhsV0WriteResponse))
          as ClientMhsV0WriteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientMhsV0WriteResponse create() => ClientMhsV0WriteResponse._();
  @$core.override
  ClientMhsV0WriteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientMhsV0WriteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientMhsV0WriteResponse>(create);
  static ClientMhsV0WriteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<MhsStateValue> get states => $_getList(0);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
