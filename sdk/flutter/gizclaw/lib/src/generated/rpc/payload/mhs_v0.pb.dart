// This is a generated file - do not edit.
//
// Generated from payload/mhs_v0.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

import 'mhs_v0.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'mhs_v0.pbenum.dart';

/// An HWD identifies the shape of one hardware instance, not its instance ID.
/// A RuntimeProfile manifest lists the instances present on a product. Read-only
/// HWDs have no write messages. The message names are the source for SDK codecs.
class ClientHwdOptions extends $pb.GeneratedMessage {
  factory ClientHwdOptions({
    $core.String? name,
    $core.String? readResponse,
    $core.String? writeRequest,
    $core.String? writeResponse,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (readResponse != null) result.readResponse = readResponse;
    if (writeRequest != null) result.writeRequest = writeRequest;
    if (writeResponse != null) result.writeResponse = writeResponse;
    return result;
  }

  ClientHwdOptions._();

  factory ClientHwdOptions.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientHwdOptions.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientHwdOptions',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'readResponse')
    ..aOS(3, _omitFieldNames ? '' : 'writeRequest')
    ..aOS(4, _omitFieldNames ? '' : 'writeResponse')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientHwdOptions clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientHwdOptions copyWith(void Function(ClientHwdOptions) updates) =>
      super.copyWith((message) => updates(message as ClientHwdOptions))
          as ClientHwdOptions;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientHwdOptions create() => ClientHwdOptions._();
  @$core.override
  ClientHwdOptions createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientHwdOptions getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientHwdOptions>(create);
  static ClientHwdOptions? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get readResponse => $_getSZ(1);
  @$pb.TagNumber(2)
  set readResponse($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasReadResponse() => $_has(1);
  @$pb.TagNumber(2)
  void clearReadResponse() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get writeRequest => $_getSZ(2);
  @$pb.TagNumber(3)
  set writeRequest($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasWriteRequest() => $_has(2);
  @$pb.TagNumber(3)
  void clearWriteRequest() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get writeResponse => $_getSZ(3);
  @$pb.TagNumber(4)
  set writeResponse($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasWriteResponse() => $_has(3);
  @$pb.TagNumber(4)
  void clearWriteResponse() => $_clearField(4);
}

/// The Server checks the ID against the bound manifest and verifies the HWD
/// before forwarding. A device returns NOT_FOUND when the physical instance is
/// absent, even if the RuntimeProfile advertises it. Failures use RpcStatus.
class ClientMhsV0ReadRequest extends $pb.GeneratedMessage {
  factory ClientMhsV0ReadRequest({
    $core.String? id,
    ClientHwd? hwd,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (hwd != null) result.hwd = hwd;
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
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aE<ClientHwd>(2, _omitFieldNames ? '' : 'hwd',
        enumValues: ClientHwd.values)
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
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  ClientHwd get hwd => $_getN(1);
  @$pb.TagNumber(2)
  set hwd(ClientHwd value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasHwd() => $_has(1);
  @$pb.TagNumber(2)
  void clearHwd() => $_clearField(2);
}

/// Present capabilities report the fields this concrete instance can write.
/// Omitted capabilities mean unknown; fields in a generic HWD schema do not
/// imply write support on a particular device.
class MhsV0WriteCapabilities extends $pb.GeneratedMessage {
  factory MhsV0WriteCapabilities({
    $core.Iterable<$core.String>? fields,
  }) {
    final result = create();
    if (fields != null) result.fields.addAll(fields);
    return result;
  }

  MhsV0WriteCapabilities._();

  factory MhsV0WriteCapabilities.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MhsV0WriteCapabilities.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MhsV0WriteCapabilities',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPS(1, _omitFieldNames ? '' : 'fields')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MhsV0WriteCapabilities clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MhsV0WriteCapabilities copyWith(
          void Function(MhsV0WriteCapabilities) updates) =>
      super.copyWith((message) => updates(message as MhsV0WriteCapabilities))
          as MhsV0WriteCapabilities;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MhsV0WriteCapabilities create() => MhsV0WriteCapabilities._();
  @$core.override
  MhsV0WriteCapabilities createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MhsV0WriteCapabilities getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<MhsV0WriteCapabilities>(create);
  static MhsV0WriteCapabilities? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<$core.String> get fields => $_getList(0);
}

class ClientMhsV0ReadResponse extends $pb.GeneratedMessage {
  factory ClientMhsV0ReadResponse({
    $core.List<$core.int>? payload,
    MhsV0WriteCapabilities? writeCapabilities,
  }) {
    final result = create();
    if (payload != null) result.payload = payload;
    if (writeCapabilities != null) result.writeCapabilities = writeCapabilities;
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
    ..a<$core.List<$core.int>>(
        1, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..aOM<MhsV0WriteCapabilities>(2, _omitFieldNames ? '' : 'writeCapabilities',
        subBuilder: MhsV0WriteCapabilities.create)
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
  $core.List<$core.int> get payload => $_getN(0);
  @$pb.TagNumber(1)
  set payload($core.List<$core.int> value) => $_setBytes(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPayload() => $_has(0);
  @$pb.TagNumber(1)
  void clearPayload() => $_clearField(1);

  @$pb.TagNumber(2)
  MhsV0WriteCapabilities get writeCapabilities => $_getN(1);
  @$pb.TagNumber(2)
  set writeCapabilities(MhsV0WriteCapabilities value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasWriteCapabilities() => $_has(1);
  @$pb.TagNumber(2)
  void clearWriteCapabilities() => $_clearField(2);
  @$pb.TagNumber(2)
  MhsV0WriteCapabilities ensureWriteCapabilities() => $_ensure(1);
}

class ClientMhsV0WriteRequest extends $pb.GeneratedMessage {
  factory ClientMhsV0WriteRequest({
    $core.String? id,
    ClientHwd? hwd,
    $core.List<$core.int>? payload,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (hwd != null) result.hwd = hwd;
    if (payload != null) result.payload = payload;
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
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aE<ClientHwd>(2, _omitFieldNames ? '' : 'hwd',
        enumValues: ClientHwd.values)
    ..a<$core.List<$core.int>>(
        3, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
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
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  ClientHwd get hwd => $_getN(1);
  @$pb.TagNumber(2)
  set hwd(ClientHwd value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasHwd() => $_has(1);
  @$pb.TagNumber(2)
  void clearHwd() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.List<$core.int> get payload => $_getN(2);
  @$pb.TagNumber(3)
  set payload($core.List<$core.int> value) => $_setBytes(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPayload() => $_has(2);
  @$pb.TagNumber(3)
  void clearPayload() => $_clearField(3);
}

class ClientMhsV0WriteResponse extends $pb.GeneratedMessage {
  factory ClientMhsV0WriteResponse({
    $core.List<$core.int>? payload,
  }) {
    final result = create();
    if (payload != null) result.payload = payload;
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
    ..a<$core.List<$core.int>>(
        1, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
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
  $core.List<$core.int> get payload => $_getN(0);
  @$pb.TagNumber(1)
  set payload($core.List<$core.int> value) => $_setBytes(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPayload() => $_has(0);
  @$pb.TagNumber(1)
  void clearPayload() => $_clearField(1);
}

/// Read-only HWDs. Optional fields distinguish a measured zero or false from
/// an observation unavailable on this hardware revision.
class WifiHwdReadResponse extends $pb.GeneratedMessage {
  factory WifiHwdReadResponse({
    $core.bool? connected,
    $core.String? ssid,
    $core.String? bssid,
    $core.int? rssiDbm,
    $core.String? ip,
  }) {
    final result = create();
    if (connected != null) result.connected = connected;
    if (ssid != null) result.ssid = ssid;
    if (bssid != null) result.bssid = bssid;
    if (rssiDbm != null) result.rssiDbm = rssiDbm;
    if (ip != null) result.ip = ip;
    return result;
  }

  WifiHwdReadResponse._();

  factory WifiHwdReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WifiHwdReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WifiHwdReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'connected')
    ..aOS(2, _omitFieldNames ? '' : 'ssid')
    ..aOS(3, _omitFieldNames ? '' : 'bssid')
    ..aI(4, _omitFieldNames ? '' : 'rssiDbm')
    ..aOS(5, _omitFieldNames ? '' : 'ip')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WifiHwdReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WifiHwdReadResponse copyWith(void Function(WifiHwdReadResponse) updates) =>
      super.copyWith((message) => updates(message as WifiHwdReadResponse))
          as WifiHwdReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WifiHwdReadResponse create() => WifiHwdReadResponse._();
  @$core.override
  WifiHwdReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WifiHwdReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WifiHwdReadResponse>(create);
  static WifiHwdReadResponse? _defaultInstance;

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
  $core.String get bssid => $_getSZ(2);
  @$pb.TagNumber(3)
  set bssid($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasBssid() => $_has(2);
  @$pb.TagNumber(3)
  void clearBssid() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.int get rssiDbm => $_getIZ(3);
  @$pb.TagNumber(4)
  set rssiDbm($core.int value) => $_setSignedInt32(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRssiDbm() => $_has(3);
  @$pb.TagNumber(4)
  void clearRssiDbm() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get ip => $_getSZ(4);
  @$pb.TagNumber(5)
  set ip($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasIp() => $_has(4);
  @$pb.TagNumber(5)
  void clearIp() => $_clearField(5);
}

class BleHwdReadResponse extends $pb.GeneratedMessage {
  factory BleHwdReadResponse({
    $core.bool? powered,
    $core.bool? advertising,
    $core.bool? scanning,
    $core.int? connectionCount,
  }) {
    final result = create();
    if (powered != null) result.powered = powered;
    if (advertising != null) result.advertising = advertising;
    if (scanning != null) result.scanning = scanning;
    if (connectionCount != null) result.connectionCount = connectionCount;
    return result;
  }

  BleHwdReadResponse._();

  factory BleHwdReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory BleHwdReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'BleHwdReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'powered')
    ..aOB(2, _omitFieldNames ? '' : 'advertising')
    ..aOB(3, _omitFieldNames ? '' : 'scanning')
    ..aI(4, _omitFieldNames ? '' : 'connectionCount',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BleHwdReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BleHwdReadResponse copyWith(void Function(BleHwdReadResponse) updates) =>
      super.copyWith((message) => updates(message as BleHwdReadResponse))
          as BleHwdReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static BleHwdReadResponse create() => BleHwdReadResponse._();
  @$core.override
  BleHwdReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static BleHwdReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<BleHwdReadResponse>(create);
  static BleHwdReadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get powered => $_getBF(0);
  @$pb.TagNumber(1)
  set powered($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPowered() => $_has(0);
  @$pb.TagNumber(1)
  void clearPowered() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get advertising => $_getBF(1);
  @$pb.TagNumber(2)
  set advertising($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasAdvertising() => $_has(1);
  @$pb.TagNumber(2)
  void clearAdvertising() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get scanning => $_getBF(2);
  @$pb.TagNumber(3)
  set scanning($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasScanning() => $_has(2);
  @$pb.TagNumber(3)
  void clearScanning() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.int get connectionCount => $_getIZ(3);
  @$pb.TagNumber(4)
  set connectionCount($core.int value) => $_setUnsignedInt32(3, value);
  @$pb.TagNumber(4)
  $core.bool hasConnectionCount() => $_has(3);
  @$pb.TagNumber(4)
  void clearConnectionCount() => $_clearField(4);
}

class ModemHwdReadResponse extends $pb.GeneratedMessage {
  factory ModemHwdReadResponse({
    $core.bool? simPresent,
    $core.bool? registered,
    $core.String? rat,
    $core.int? rssiDbm,
    $core.int? signalLevel,
  }) {
    final result = create();
    if (simPresent != null) result.simPresent = simPresent;
    if (registered != null) result.registered = registered;
    if (rat != null) result.rat = rat;
    if (rssiDbm != null) result.rssiDbm = rssiDbm;
    if (signalLevel != null) result.signalLevel = signalLevel;
    return result;
  }

  ModemHwdReadResponse._();

  factory ModemHwdReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModemHwdReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModemHwdReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'simPresent')
    ..aOB(2, _omitFieldNames ? '' : 'registered')
    ..aOS(3, _omitFieldNames ? '' : 'rat')
    ..aI(4, _omitFieldNames ? '' : 'rssiDbm')
    ..aI(5, _omitFieldNames ? '' : 'signalLevel',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModemHwdReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModemHwdReadResponse copyWith(void Function(ModemHwdReadResponse) updates) =>
      super.copyWith((message) => updates(message as ModemHwdReadResponse))
          as ModemHwdReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModemHwdReadResponse create() => ModemHwdReadResponse._();
  @$core.override
  ModemHwdReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModemHwdReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModemHwdReadResponse>(create);
  static ModemHwdReadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get simPresent => $_getBF(0);
  @$pb.TagNumber(1)
  set simPresent($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSimPresent() => $_has(0);
  @$pb.TagNumber(1)
  void clearSimPresent() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get registered => $_getBF(1);
  @$pb.TagNumber(2)
  set registered($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRegistered() => $_has(1);
  @$pb.TagNumber(2)
  void clearRegistered() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get rat => $_getSZ(2);
  @$pb.TagNumber(3)
  set rat($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRat() => $_has(2);
  @$pb.TagNumber(3)
  void clearRat() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.int get rssiDbm => $_getIZ(3);
  @$pb.TagNumber(4)
  set rssiDbm($core.int value) => $_setSignedInt32(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRssiDbm() => $_has(3);
  @$pb.TagNumber(4)
  void clearRssiDbm() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.int get signalLevel => $_getIZ(4);
  @$pb.TagNumber(5)
  set signalLevel($core.int value) => $_setUnsignedInt32(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSignalLevel() => $_has(4);
  @$pb.TagNumber(5)
  void clearSignalLevel() => $_clearField(5);
}

class BatteryHwdReadResponse extends $pb.GeneratedMessage {
  factory BatteryHwdReadResponse({
    $core.double? percent,
    $core.bool? charging,
    $core.double? voltageMv,
  }) {
    final result = create();
    if (percent != null) result.percent = percent;
    if (charging != null) result.charging = charging;
    if (voltageMv != null) result.voltageMv = voltageMv;
    return result;
  }

  BatteryHwdReadResponse._();

  factory BatteryHwdReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory BatteryHwdReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'BatteryHwdReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aD(1, _omitFieldNames ? '' : 'percent')
    ..aOB(2, _omitFieldNames ? '' : 'charging')
    ..aD(3, _omitFieldNames ? '' : 'voltageMv')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BatteryHwdReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BatteryHwdReadResponse copyWith(
          void Function(BatteryHwdReadResponse) updates) =>
      super.copyWith((message) => updates(message as BatteryHwdReadResponse))
          as BatteryHwdReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static BatteryHwdReadResponse create() => BatteryHwdReadResponse._();
  @$core.override
  BatteryHwdReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static BatteryHwdReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<BatteryHwdReadResponse>(create);
  static BatteryHwdReadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.double get percent => $_getN(0);
  @$pb.TagNumber(1)
  set percent($core.double value) => $_setDouble(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPercent() => $_has(0);
  @$pb.TagNumber(1)
  void clearPercent() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get charging => $_getBF(1);
  @$pb.TagNumber(2)
  set charging($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCharging() => $_has(1);
  @$pb.TagNumber(2)
  void clearCharging() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.double get voltageMv => $_getN(2);
  @$pb.TagNumber(3)
  set voltageMv($core.double value) => $_setDouble(2, value);
  @$pb.TagNumber(3)
  $core.bool hasVoltageMv() => $_has(2);
  @$pb.TagNumber(3)
  void clearVoltageMv() => $_clearField(3);
}

class MicHwdReadResponse extends $pb.GeneratedMessage {
  factory MicHwdReadResponse({
    $core.bool? available,
    $core.bool? capturing,
  }) {
    final result = create();
    if (available != null) result.available = available;
    if (capturing != null) result.capturing = capturing;
    return result;
  }

  MicHwdReadResponse._();

  factory MicHwdReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MicHwdReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MicHwdReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'available')
    ..aOB(2, _omitFieldNames ? '' : 'capturing')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MicHwdReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MicHwdReadResponse copyWith(void Function(MicHwdReadResponse) updates) =>
      super.copyWith((message) => updates(message as MicHwdReadResponse))
          as MicHwdReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MicHwdReadResponse create() => MicHwdReadResponse._();
  @$core.override
  MicHwdReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MicHwdReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<MicHwdReadResponse>(create);
  static MicHwdReadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get available => $_getBF(0);
  @$pb.TagNumber(1)
  set available($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAvailable() => $_has(0);
  @$pb.TagNumber(1)
  void clearAvailable() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get capturing => $_getBF(1);
  @$pb.TagNumber(2)
  set capturing($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCapturing() => $_has(1);
  @$pb.TagNumber(2)
  void clearCapturing() => $_clearField(2);
}

/// Write requests update the present fields only. An empty request is invalid.
/// Drivers enforce physical safety limits and return the values actually in
/// effect. A timeout does not prove a write was not applied; re-read the HWD.
class DisplayHwdReadResponse extends $pb.GeneratedMessage {
  factory DisplayHwdReadResponse({
    $core.int? brightnessPercent,
    $core.bool? enabled,
    $core.int? offTimeoutMs,
  }) {
    final result = create();
    if (brightnessPercent != null) result.brightnessPercent = brightnessPercent;
    if (enabled != null) result.enabled = enabled;
    if (offTimeoutMs != null) result.offTimeoutMs = offTimeoutMs;
    return result;
  }

  DisplayHwdReadResponse._();

  factory DisplayHwdReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DisplayHwdReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DisplayHwdReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'brightnessPercent',
        fieldType: $pb.PbFieldType.OU3)
    ..aOB(2, _omitFieldNames ? '' : 'enabled')
    ..aI(3, _omitFieldNames ? '' : 'offTimeoutMs',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DisplayHwdReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DisplayHwdReadResponse copyWith(
          void Function(DisplayHwdReadResponse) updates) =>
      super.copyWith((message) => updates(message as DisplayHwdReadResponse))
          as DisplayHwdReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DisplayHwdReadResponse create() => DisplayHwdReadResponse._();
  @$core.override
  DisplayHwdReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DisplayHwdReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DisplayHwdReadResponse>(create);
  static DisplayHwdReadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get brightnessPercent => $_getIZ(0);
  @$pb.TagNumber(1)
  set brightnessPercent($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasBrightnessPercent() => $_has(0);
  @$pb.TagNumber(1)
  void clearBrightnessPercent() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get enabled => $_getBF(1);
  @$pb.TagNumber(2)
  set enabled($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEnabled() => $_has(1);
  @$pb.TagNumber(2)
  void clearEnabled() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.int get offTimeoutMs => $_getIZ(2);
  @$pb.TagNumber(3)
  set offTimeoutMs($core.int value) => $_setUnsignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasOffTimeoutMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearOffTimeoutMs() => $_clearField(3);
}

class DisplayHwdWriteRequest extends $pb.GeneratedMessage {
  factory DisplayHwdWriteRequest({
    $core.int? brightnessPercent,
    $core.bool? enabled,
    $core.int? offTimeoutMs,
  }) {
    final result = create();
    if (brightnessPercent != null) result.brightnessPercent = brightnessPercent;
    if (enabled != null) result.enabled = enabled;
    if (offTimeoutMs != null) result.offTimeoutMs = offTimeoutMs;
    return result;
  }

  DisplayHwdWriteRequest._();

  factory DisplayHwdWriteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DisplayHwdWriteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DisplayHwdWriteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'brightnessPercent',
        fieldType: $pb.PbFieldType.OU3)
    ..aOB(2, _omitFieldNames ? '' : 'enabled')
    ..aI(3, _omitFieldNames ? '' : 'offTimeoutMs',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DisplayHwdWriteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DisplayHwdWriteRequest copyWith(
          void Function(DisplayHwdWriteRequest) updates) =>
      super.copyWith((message) => updates(message as DisplayHwdWriteRequest))
          as DisplayHwdWriteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DisplayHwdWriteRequest create() => DisplayHwdWriteRequest._();
  @$core.override
  DisplayHwdWriteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DisplayHwdWriteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DisplayHwdWriteRequest>(create);
  static DisplayHwdWriteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get brightnessPercent => $_getIZ(0);
  @$pb.TagNumber(1)
  set brightnessPercent($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasBrightnessPercent() => $_has(0);
  @$pb.TagNumber(1)
  void clearBrightnessPercent() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get enabled => $_getBF(1);
  @$pb.TagNumber(2)
  set enabled($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEnabled() => $_has(1);
  @$pb.TagNumber(2)
  void clearEnabled() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.int get offTimeoutMs => $_getIZ(2);
  @$pb.TagNumber(3)
  set offTimeoutMs($core.int value) => $_setUnsignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasOffTimeoutMs() => $_has(2);
  @$pb.TagNumber(3)
  void clearOffTimeoutMs() => $_clearField(3);
}

class DisplayHwdWriteResponse extends $pb.GeneratedMessage {
  factory DisplayHwdWriteResponse({
    DisplayHwdReadResponse? applied,
  }) {
    final result = create();
    if (applied != null) result.applied = applied;
    return result;
  }

  DisplayHwdWriteResponse._();

  factory DisplayHwdWriteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DisplayHwdWriteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DisplayHwdWriteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DisplayHwdReadResponse>(1, _omitFieldNames ? '' : 'applied',
        subBuilder: DisplayHwdReadResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DisplayHwdWriteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DisplayHwdWriteResponse copyWith(
          void Function(DisplayHwdWriteResponse) updates) =>
      super.copyWith((message) => updates(message as DisplayHwdWriteResponse))
          as DisplayHwdWriteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DisplayHwdWriteResponse create() => DisplayHwdWriteResponse._();
  @$core.override
  DisplayHwdWriteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DisplayHwdWriteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DisplayHwdWriteResponse>(create);
  static DisplayHwdWriteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  DisplayHwdReadResponse get applied => $_getN(0);
  @$pb.TagNumber(1)
  set applied(DisplayHwdReadResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApplied() => $_has(0);
  @$pb.TagNumber(1)
  void clearApplied() => $_clearField(1);
  @$pb.TagNumber(1)
  DisplayHwdReadResponse ensureApplied() => $_ensure(0);
}

class LedHwdReadResponse extends $pb.GeneratedMessage {
  factory LedHwdReadResponse({
    $core.bool? enabled,
    $core.int? brightnessPercent,
  }) {
    final result = create();
    if (enabled != null) result.enabled = enabled;
    if (brightnessPercent != null) result.brightnessPercent = brightnessPercent;
    return result;
  }

  LedHwdReadResponse._();

  factory LedHwdReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory LedHwdReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'LedHwdReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'enabled')
    ..aI(2, _omitFieldNames ? '' : 'brightnessPercent',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LedHwdReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LedHwdReadResponse copyWith(void Function(LedHwdReadResponse) updates) =>
      super.copyWith((message) => updates(message as LedHwdReadResponse))
          as LedHwdReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static LedHwdReadResponse create() => LedHwdReadResponse._();
  @$core.override
  LedHwdReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static LedHwdReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<LedHwdReadResponse>(create);
  static LedHwdReadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get enabled => $_getBF(0);
  @$pb.TagNumber(1)
  set enabled($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasEnabled() => $_has(0);
  @$pb.TagNumber(1)
  void clearEnabled() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get brightnessPercent => $_getIZ(1);
  @$pb.TagNumber(2)
  set brightnessPercent($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBrightnessPercent() => $_has(1);
  @$pb.TagNumber(2)
  void clearBrightnessPercent() => $_clearField(2);
}

class LedHwdWriteRequest extends $pb.GeneratedMessage {
  factory LedHwdWriteRequest({
    $core.bool? enabled,
    $core.int? brightnessPercent,
  }) {
    final result = create();
    if (enabled != null) result.enabled = enabled;
    if (brightnessPercent != null) result.brightnessPercent = brightnessPercent;
    return result;
  }

  LedHwdWriteRequest._();

  factory LedHwdWriteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory LedHwdWriteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'LedHwdWriteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'enabled')
    ..aI(2, _omitFieldNames ? '' : 'brightnessPercent',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LedHwdWriteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LedHwdWriteRequest copyWith(void Function(LedHwdWriteRequest) updates) =>
      super.copyWith((message) => updates(message as LedHwdWriteRequest))
          as LedHwdWriteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static LedHwdWriteRequest create() => LedHwdWriteRequest._();
  @$core.override
  LedHwdWriteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static LedHwdWriteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<LedHwdWriteRequest>(create);
  static LedHwdWriteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get enabled => $_getBF(0);
  @$pb.TagNumber(1)
  set enabled($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasEnabled() => $_has(0);
  @$pb.TagNumber(1)
  void clearEnabled() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get brightnessPercent => $_getIZ(1);
  @$pb.TagNumber(2)
  set brightnessPercent($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBrightnessPercent() => $_has(1);
  @$pb.TagNumber(2)
  void clearBrightnessPercent() => $_clearField(2);
}

class LedHwdWriteResponse extends $pb.GeneratedMessage {
  factory LedHwdWriteResponse({
    LedHwdReadResponse? applied,
  }) {
    final result = create();
    if (applied != null) result.applied = applied;
    return result;
  }

  LedHwdWriteResponse._();

  factory LedHwdWriteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory LedHwdWriteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'LedHwdWriteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<LedHwdReadResponse>(1, _omitFieldNames ? '' : 'applied',
        subBuilder: LedHwdReadResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LedHwdWriteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LedHwdWriteResponse copyWith(void Function(LedHwdWriteResponse) updates) =>
      super.copyWith((message) => updates(message as LedHwdWriteResponse))
          as LedHwdWriteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static LedHwdWriteResponse create() => LedHwdWriteResponse._();
  @$core.override
  LedHwdWriteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static LedHwdWriteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<LedHwdWriteResponse>(create);
  static LedHwdWriteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  LedHwdReadResponse get applied => $_getN(0);
  @$pb.TagNumber(1)
  set applied(LedHwdReadResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApplied() => $_has(0);
  @$pb.TagNumber(1)
  void clearApplied() => $_clearField(1);
  @$pb.TagNumber(1)
  LedHwdReadResponse ensureApplied() => $_ensure(0);
}

class SpeakerHwdReadResponse extends $pb.GeneratedMessage {
  factory SpeakerHwdReadResponse({
    $core.int? volumePercent,
    $core.bool? muted,
  }) {
    final result = create();
    if (volumePercent != null) result.volumePercent = volumePercent;
    if (muted != null) result.muted = muted;
    return result;
  }

  SpeakerHwdReadResponse._();

  factory SpeakerHwdReadResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeakerHwdReadResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeakerHwdReadResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'volumePercent',
        fieldType: $pb.PbFieldType.OU3)
    ..aOB(2, _omitFieldNames ? '' : 'muted')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeakerHwdReadResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeakerHwdReadResponse copyWith(
          void Function(SpeakerHwdReadResponse) updates) =>
      super.copyWith((message) => updates(message as SpeakerHwdReadResponse))
          as SpeakerHwdReadResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeakerHwdReadResponse create() => SpeakerHwdReadResponse._();
  @$core.override
  SpeakerHwdReadResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeakerHwdReadResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeakerHwdReadResponse>(create);
  static SpeakerHwdReadResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get volumePercent => $_getIZ(0);
  @$pb.TagNumber(1)
  set volumePercent($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasVolumePercent() => $_has(0);
  @$pb.TagNumber(1)
  void clearVolumePercent() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get muted => $_getBF(1);
  @$pb.TagNumber(2)
  set muted($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMuted() => $_has(1);
  @$pb.TagNumber(2)
  void clearMuted() => $_clearField(2);
}

class SpeakerHwdWriteRequest extends $pb.GeneratedMessage {
  factory SpeakerHwdWriteRequest({
    $core.int? volumePercent,
    $core.bool? muted,
  }) {
    final result = create();
    if (volumePercent != null) result.volumePercent = volumePercent;
    if (muted != null) result.muted = muted;
    return result;
  }

  SpeakerHwdWriteRequest._();

  factory SpeakerHwdWriteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeakerHwdWriteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeakerHwdWriteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'volumePercent',
        fieldType: $pb.PbFieldType.OU3)
    ..aOB(2, _omitFieldNames ? '' : 'muted')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeakerHwdWriteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeakerHwdWriteRequest copyWith(
          void Function(SpeakerHwdWriteRequest) updates) =>
      super.copyWith((message) => updates(message as SpeakerHwdWriteRequest))
          as SpeakerHwdWriteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeakerHwdWriteRequest create() => SpeakerHwdWriteRequest._();
  @$core.override
  SpeakerHwdWriteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeakerHwdWriteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeakerHwdWriteRequest>(create);
  static SpeakerHwdWriteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get volumePercent => $_getIZ(0);
  @$pb.TagNumber(1)
  set volumePercent($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasVolumePercent() => $_has(0);
  @$pb.TagNumber(1)
  void clearVolumePercent() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get muted => $_getBF(1);
  @$pb.TagNumber(2)
  set muted($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMuted() => $_has(1);
  @$pb.TagNumber(2)
  void clearMuted() => $_clearField(2);
}

class SpeakerHwdWriteResponse extends $pb.GeneratedMessage {
  factory SpeakerHwdWriteResponse({
    SpeakerHwdReadResponse? applied,
  }) {
    final result = create();
    if (applied != null) result.applied = applied;
    return result;
  }

  SpeakerHwdWriteResponse._();

  factory SpeakerHwdWriteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeakerHwdWriteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeakerHwdWriteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<SpeakerHwdReadResponse>(1, _omitFieldNames ? '' : 'applied',
        subBuilder: SpeakerHwdReadResponse.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeakerHwdWriteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeakerHwdWriteResponse copyWith(
          void Function(SpeakerHwdWriteResponse) updates) =>
      super.copyWith((message) => updates(message as SpeakerHwdWriteResponse))
          as SpeakerHwdWriteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeakerHwdWriteResponse create() => SpeakerHwdWriteResponse._();
  @$core.override
  SpeakerHwdWriteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeakerHwdWriteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeakerHwdWriteResponse>(create);
  static SpeakerHwdWriteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  SpeakerHwdReadResponse get applied => $_getN(0);
  @$pb.TagNumber(1)
  set applied(SpeakerHwdReadResponse value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApplied() => $_has(0);
  @$pb.TagNumber(1)
  void clearApplied() => $_clearField(1);
  @$pb.TagNumber(1)
  SpeakerHwdReadResponse ensureApplied() => $_ensure(0);
}

class Mhs_v0 {
  static final clientHwd = $pb.Extension<ClientHwdOptions>(
      _omitMessageNames ? '' : 'google.protobuf.EnumValueOptions',
      _omitFieldNames ? '' : 'clientHwd',
      51002,
      $pb.PbFieldType.OM,
      defaultOrMaker: ClientHwdOptions.getDefault,
      subBuilder: ClientHwdOptions.create);
  static void registerAllExtensions($pb.ExtensionRegistry registry) {
    registry.add(clientHwd);
  }
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
