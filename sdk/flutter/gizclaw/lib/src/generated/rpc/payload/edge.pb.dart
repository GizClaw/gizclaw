// This is a generated file - do not edit.
//
// Generated from payload/edge.proto.

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

class PeerAssignment extends $pb.GeneratedMessage {
  factory PeerAssignment({
    $core.String? peerPublicKey,
    $core.String? serverPublicKey,
    $core.String? serverEndpoint,
    $0.PeerRole? role,
    $fixnum.Int64? version,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (peerPublicKey != null) result.peerPublicKey = peerPublicKey;
    if (serverPublicKey != null) result.serverPublicKey = serverPublicKey;
    if (serverEndpoint != null) result.serverEndpoint = serverEndpoint;
    if (role != null) result.role = role;
    if (version != null) result.version = version;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  PeerAssignment._();

  factory PeerAssignment.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PeerAssignment.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PeerAssignment',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'peerPublicKey')
    ..aOS(2, _omitFieldNames ? '' : 'serverPublicKey')
    ..aOS(3, _omitFieldNames ? '' : 'serverEndpoint')
    ..aE<$0.PeerRole>(4, _omitFieldNames ? '' : 'role',
        enumValues: $0.PeerRole.values)
    ..aInt64(5, _omitFieldNames ? '' : 'version')
    ..aOS(6, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerAssignment clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PeerAssignment copyWith(void Function(PeerAssignment) updates) =>
      super.copyWith((message) => updates(message as PeerAssignment))
          as PeerAssignment;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PeerAssignment create() => PeerAssignment._();
  @$core.override
  PeerAssignment createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PeerAssignment getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PeerAssignment>(create);
  static PeerAssignment? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get peerPublicKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set peerPublicKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPeerPublicKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearPeerPublicKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get serverPublicKey => $_getSZ(1);
  @$pb.TagNumber(2)
  set serverPublicKey($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasServerPublicKey() => $_has(1);
  @$pb.TagNumber(2)
  void clearServerPublicKey() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get serverEndpoint => $_getSZ(2);
  @$pb.TagNumber(3)
  set serverEndpoint($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasServerEndpoint() => $_has(2);
  @$pb.TagNumber(3)
  void clearServerEndpoint() => $_clearField(3);

  @$pb.TagNumber(4)
  $0.PeerRole get role => $_getN(3);
  @$pb.TagNumber(4)
  set role($0.PeerRole value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasRole() => $_has(3);
  @$pb.TagNumber(4)
  void clearRole() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get version => $_getI64(4);
  @$pb.TagNumber(5)
  set version($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasVersion() => $_has(4);
  @$pb.TagNumber(5)
  void clearVersion() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get updatedAt => $_getSZ(5);
  @$pb.TagNumber(6)
  set updatedAt($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasUpdatedAt() => $_has(5);
  @$pb.TagNumber(6)
  void clearUpdatedAt() => $_clearField(6);
}

class ServerPeerLookupRequest extends $pb.GeneratedMessage {
  factory ServerPeerLookupRequest({
    $core.String? peerPublicKey,
  }) {
    final result = create();
    if (peerPublicKey != null) result.peerPublicKey = peerPublicKey;
    return result;
  }

  ServerPeerLookupRequest._();

  factory ServerPeerLookupRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPeerLookupRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPeerLookupRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'peerPublicKey')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerLookupRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerLookupRequest copyWith(
          void Function(ServerPeerLookupRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPeerLookupRequest))
          as ServerPeerLookupRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPeerLookupRequest create() => ServerPeerLookupRequest._();
  @$core.override
  ServerPeerLookupRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPeerLookupRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPeerLookupRequest>(create);
  static ServerPeerLookupRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get peerPublicKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set peerPublicKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPeerPublicKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearPeerPublicKey() => $_clearField(1);
}

class ServerPeerLookupResponse extends $pb.GeneratedMessage {
  factory ServerPeerLookupResponse({
    PeerAssignment? assignment,
  }) {
    final result = create();
    if (assignment != null) result.assignment = assignment;
    return result;
  }

  ServerPeerLookupResponse._();

  factory ServerPeerLookupResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPeerLookupResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPeerLookupResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerAssignment>(1, _omitFieldNames ? '' : 'assignment',
        subBuilder: PeerAssignment.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerLookupResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerLookupResponse copyWith(
          void Function(ServerPeerLookupResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPeerLookupResponse))
          as ServerPeerLookupResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPeerLookupResponse create() => ServerPeerLookupResponse._();
  @$core.override
  ServerPeerLookupResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPeerLookupResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPeerLookupResponse>(create);
  static ServerPeerLookupResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerAssignment get assignment => $_getN(0);
  @$pb.TagNumber(1)
  set assignment(PeerAssignment value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAssignment() => $_has(0);
  @$pb.TagNumber(1)
  void clearAssignment() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerAssignment ensureAssignment() => $_ensure(0);
}

class ServerPeerAssignRequest extends $pb.GeneratedMessage {
  factory ServerPeerAssignRequest({
    $core.String? peerPublicKey,
    $fixnum.Int64? expectedVersion,
  }) {
    final result = create();
    if (peerPublicKey != null) result.peerPublicKey = peerPublicKey;
    if (expectedVersion != null) result.expectedVersion = expectedVersion;
    return result;
  }

  ServerPeerAssignRequest._();

  factory ServerPeerAssignRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPeerAssignRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPeerAssignRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'peerPublicKey')
    ..aInt64(2, _omitFieldNames ? '' : 'expectedVersion')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerAssignRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerAssignRequest copyWith(
          void Function(ServerPeerAssignRequest) updates) =>
      super.copyWith((message) => updates(message as ServerPeerAssignRequest))
          as ServerPeerAssignRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPeerAssignRequest create() => ServerPeerAssignRequest._();
  @$core.override
  ServerPeerAssignRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPeerAssignRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPeerAssignRequest>(create);
  static ServerPeerAssignRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get peerPublicKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set peerPublicKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPeerPublicKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearPeerPublicKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get expectedVersion => $_getI64(1);
  @$pb.TagNumber(2)
  set expectedVersion($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasExpectedVersion() => $_has(1);
  @$pb.TagNumber(2)
  void clearExpectedVersion() => $_clearField(2);
}

class ServerPeerAssignResponse extends $pb.GeneratedMessage {
  factory ServerPeerAssignResponse({
    PeerAssignment? assignment,
  }) {
    final result = create();
    if (assignment != null) result.assignment = assignment;
    return result;
  }

  ServerPeerAssignResponse._();

  factory ServerPeerAssignResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerPeerAssignResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerPeerAssignResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerAssignment>(1, _omitFieldNames ? '' : 'assignment',
        subBuilder: PeerAssignment.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerAssignResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerPeerAssignResponse copyWith(
          void Function(ServerPeerAssignResponse) updates) =>
      super.copyWith((message) => updates(message as ServerPeerAssignResponse))
          as ServerPeerAssignResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerPeerAssignResponse create() => ServerPeerAssignResponse._();
  @$core.override
  ServerPeerAssignResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerPeerAssignResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerPeerAssignResponse>(create);
  static ServerPeerAssignResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerAssignment get assignment => $_getN(0);
  @$pb.TagNumber(1)
  set assignment(PeerAssignment value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAssignment() => $_has(0);
  @$pb.TagNumber(1)
  void clearAssignment() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerAssignment ensureAssignment() => $_ensure(0);
}

class ServerRouteResolveRequest extends $pb.GeneratedMessage {
  factory ServerRouteResolveRequest({
    $core.String? targetPeerPublicKey,
  }) {
    final result = create();
    if (targetPeerPublicKey != null)
      result.targetPeerPublicKey = targetPeerPublicKey;
    return result;
  }

  ServerRouteResolveRequest._();

  factory ServerRouteResolveRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRouteResolveRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRouteResolveRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'targetPeerPublicKey')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRouteResolveRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRouteResolveRequest copyWith(
          void Function(ServerRouteResolveRequest) updates) =>
      super.copyWith((message) => updates(message as ServerRouteResolveRequest))
          as ServerRouteResolveRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRouteResolveRequest create() => ServerRouteResolveRequest._();
  @$core.override
  ServerRouteResolveRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRouteResolveRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRouteResolveRequest>(create);
  static ServerRouteResolveRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get targetPeerPublicKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set targetPeerPublicKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasTargetPeerPublicKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearTargetPeerPublicKey() => $_clearField(1);
}

class ServerRouteResolveResponse extends $pb.GeneratedMessage {
  factory ServerRouteResolveResponse({
    PeerAssignment? assignment,
  }) {
    final result = create();
    if (assignment != null) result.assignment = assignment;
    return result;
  }

  ServerRouteResolveResponse._();

  factory ServerRouteResolveResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerRouteResolveResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerRouteResolveResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<PeerAssignment>(1, _omitFieldNames ? '' : 'assignment',
        subBuilder: PeerAssignment.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRouteResolveResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerRouteResolveResponse copyWith(
          void Function(ServerRouteResolveResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ServerRouteResolveResponse))
          as ServerRouteResolveResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerRouteResolveResponse create() => ServerRouteResolveResponse._();
  @$core.override
  ServerRouteResolveResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerRouteResolveResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerRouteResolveResponse>(create);
  static ServerRouteResolveResponse? _defaultInstance;

  @$pb.TagNumber(1)
  PeerAssignment get assignment => $_getN(0);
  @$pb.TagNumber(1)
  set assignment(PeerAssignment value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAssignment() => $_has(0);
  @$pb.TagNumber(1)
  void clearAssignment() => $_clearField(1);
  @$pb.TagNumber(1)
  PeerAssignment ensureAssignment() => $_ensure(0);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
