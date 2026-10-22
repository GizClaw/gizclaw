// This is a generated file - do not edit.
//
// Generated from payload/management.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

class ContentAgeOption extends $pb.GeneratedMessage {
  factory ContentAgeOption({
    $core.String? id,
    $core.String? displayName,
    $core.int? minAgeYears,
    $core.int? maxAgeYears,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (displayName != null) result.displayName = displayName;
    if (minAgeYears != null) result.minAgeYears = minAgeYears;
    if (maxAgeYears != null) result.maxAgeYears = maxAgeYears;
    return result;
  }

  ContentAgeOption._();

  factory ContentAgeOption.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContentAgeOption.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContentAgeOption',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'displayName')
    ..aI(3, _omitFieldNames ? '' : 'minAgeYears',
        fieldType: $pb.PbFieldType.OU3)
    ..aI(4, _omitFieldNames ? '' : 'maxAgeYears',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContentAgeOption clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContentAgeOption copyWith(void Function(ContentAgeOption) updates) =>
      super.copyWith((message) => updates(message as ContentAgeOption))
          as ContentAgeOption;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContentAgeOption create() => ContentAgeOption._();
  @$core.override
  ContentAgeOption createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContentAgeOption getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContentAgeOption>(create);
  static ContentAgeOption? _defaultInstance;

  /// Device-declared stable ID, 1..64 UTF-8 bytes.
  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  /// Display text, 1..128 UTF-8 bytes.
  @$pb.TagNumber(2)
  $core.String get displayName => $_getSZ(1);
  @$pb.TagNumber(2)
  set displayName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDisplayName() => $_has(1);
  @$pb.TagNumber(2)
  void clearDisplayName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.int get minAgeYears => $_getIZ(2);
  @$pb.TagNumber(3)
  set minAgeYears($core.int value) => $_setUnsignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMinAgeYears() => $_has(2);
  @$pb.TagNumber(3)
  void clearMinAgeYears() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.int get maxAgeYears => $_getIZ(3);
  @$pb.TagNumber(4)
  set maxAgeYears($core.int value) => $_setUnsignedInt32(3, value);
  @$pb.TagNumber(4)
  $core.bool hasMaxAgeYears() => $_has(3);
  @$pb.TagNumber(4)
  void clearMaxAgeYears() => $_clearField(4);
}

class ContentAgeState extends $pb.GeneratedMessage {
  factory ContentAgeState({
    $core.String? optionId,
    $core.Iterable<ContentAgeOption>? options,
  }) {
    final result = create();
    if (optionId != null) result.optionId = optionId;
    if (options != null) result.options.addAll(options);
    return result;
  }

  ContentAgeState._();

  factory ContentAgeState.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContentAgeState.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContentAgeState',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'optionId')
    ..pPM<ContentAgeOption>(2, _omitFieldNames ? '' : 'options',
        subBuilder: ContentAgeOption.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContentAgeState clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContentAgeState copyWith(void Function(ContentAgeState) updates) =>
      super.copyWith((message) => updates(message as ContentAgeState))
          as ContentAgeState;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContentAgeState create() => ContentAgeState._();
  @$core.override
  ContentAgeState createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContentAgeState getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContentAgeState>(create);
  static ContentAgeState? _defaultInstance;

  /// Effective selection. Must occur in options.
  @$pb.TagNumber(1)
  $core.String get optionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set optionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasOptionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearOptionId() => $_clearField(1);

  /// At most 16 supported choices, with unique IDs.
  @$pb.TagNumber(2)
  $pb.PbList<ContentAgeOption> get options => $_getList(1);
}

class ClientContentAgeGetRequest extends $pb.GeneratedMessage {
  factory ClientContentAgeGetRequest() => create();

  ClientContentAgeGetRequest._();

  factory ClientContentAgeGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientContentAgeGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientContentAgeGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientContentAgeGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientContentAgeGetRequest copyWith(
          void Function(ClientContentAgeGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientContentAgeGetRequest))
          as ClientContentAgeGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientContentAgeGetRequest create() => ClientContentAgeGetRequest._();
  @$core.override
  ClientContentAgeGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientContentAgeGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientContentAgeGetRequest>(create);
  static ClientContentAgeGetRequest? _defaultInstance;
}

class ClientContentAgeGetResponse extends $pb.GeneratedMessage {
  factory ClientContentAgeGetResponse({
    ContentAgeState? state,
  }) {
    final result = create();
    if (state != null) result.state = state;
    return result;
  }

  ClientContentAgeGetResponse._();

  factory ClientContentAgeGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientContentAgeGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientContentAgeGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ContentAgeState>(1, _omitFieldNames ? '' : 'state',
        subBuilder: ContentAgeState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientContentAgeGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientContentAgeGetResponse copyWith(
          void Function(ClientContentAgeGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientContentAgeGetResponse))
          as ClientContentAgeGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientContentAgeGetResponse create() =>
      ClientContentAgeGetResponse._();
  @$core.override
  ClientContentAgeGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientContentAgeGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientContentAgeGetResponse>(create);
  static ClientContentAgeGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  ContentAgeState get state => $_getN(0);
  @$pb.TagNumber(1)
  set state(ContentAgeState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasState() => $_has(0);
  @$pb.TagNumber(1)
  void clearState() => $_clearField(1);
  @$pb.TagNumber(1)
  ContentAgeState ensureState() => $_ensure(0);
}

class ClientContentAgeSetRequest extends $pb.GeneratedMessage {
  factory ClientContentAgeSetRequest({
    $core.String? optionId,
  }) {
    final result = create();
    if (optionId != null) result.optionId = optionId;
    return result;
  }

  ClientContentAgeSetRequest._();

  factory ClientContentAgeSetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientContentAgeSetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientContentAgeSetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'optionId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientContentAgeSetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientContentAgeSetRequest copyWith(
          void Function(ClientContentAgeSetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientContentAgeSetRequest))
          as ClientContentAgeSetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientContentAgeSetRequest create() => ClientContentAgeSetRequest._();
  @$core.override
  ClientContentAgeSetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientContentAgeSetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientContentAgeSetRequest>(create);
  static ClientContentAgeSetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get optionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set optionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasOptionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearOptionId() => $_clearField(1);
}

class ClientContentAgeSetResponse extends $pb.GeneratedMessage {
  factory ClientContentAgeSetResponse({
    ContentAgeState? applied,
  }) {
    final result = create();
    if (applied != null) result.applied = applied;
    return result;
  }

  ClientContentAgeSetResponse._();

  factory ClientContentAgeSetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientContentAgeSetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientContentAgeSetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ContentAgeState>(1, _omitFieldNames ? '' : 'applied',
        subBuilder: ContentAgeState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientContentAgeSetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientContentAgeSetResponse copyWith(
          void Function(ClientContentAgeSetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientContentAgeSetResponse))
          as ClientContentAgeSetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientContentAgeSetResponse create() =>
      ClientContentAgeSetResponse._();
  @$core.override
  ClientContentAgeSetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientContentAgeSetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientContentAgeSetResponse>(create);
  static ClientContentAgeSetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  ContentAgeState get applied => $_getN(0);
  @$pb.TagNumber(1)
  set applied(ContentAgeState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApplied() => $_has(0);
  @$pb.TagNumber(1)
  void clearApplied() => $_clearField(1);
  @$pb.TagNumber(1)
  ContentAgeState ensureApplied() => $_ensure(0);
}

class DeviceFeature extends $pb.GeneratedMessage {
  factory DeviceFeature({
    $core.String? id,
    $core.String? displayName,
    $core.bool? enabled,
    $core.bool? writable,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (displayName != null) result.displayName = displayName;
    if (enabled != null) result.enabled = enabled;
    if (writable != null) result.writable = writable;
    return result;
  }

  DeviceFeature._();

  factory DeviceFeature.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DeviceFeature.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DeviceFeature',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'displayName')
    ..aOB(3, _omitFieldNames ? '' : 'enabled')
    ..aOB(4, _omitFieldNames ? '' : 'writable')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceFeature clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceFeature copyWith(void Function(DeviceFeature) updates) =>
      super.copyWith((message) => updates(message as DeviceFeature))
          as DeviceFeature;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DeviceFeature create() => DeviceFeature._();
  @$core.override
  DeviceFeature createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DeviceFeature getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DeviceFeature>(create);
  static DeviceFeature? _defaultInstance;

  /// Device-declared stable ID, 1..64 UTF-8 bytes. Never a preference key.
  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  /// Display text, 1..128 UTF-8 bytes.
  @$pb.TagNumber(2)
  $core.String get displayName => $_getSZ(1);
  @$pb.TagNumber(2)
  set displayName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDisplayName() => $_has(1);
  @$pb.TagNumber(2)
  void clearDisplayName() => $_clearField(2);

  /// Required presence, including false.
  @$pb.TagNumber(3)
  $core.bool get enabled => $_getBF(2);
  @$pb.TagNumber(3)
  set enabled($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasEnabled() => $_has(2);
  @$pb.TagNumber(3)
  void clearEnabled() => $_clearField(3);

  /// Required presence. False means read-only.
  @$pb.TagNumber(4)
  $core.bool get writable => $_getBF(3);
  @$pb.TagNumber(4)
  set writable($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasWritable() => $_has(3);
  @$pb.TagNumber(4)
  void clearWritable() => $_clearField(4);
}

class ClientDeviceFeaturesGetRequest extends $pb.GeneratedMessage {
  factory ClientDeviceFeaturesGetRequest() => create();

  ClientDeviceFeaturesGetRequest._();

  factory ClientDeviceFeaturesGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceFeaturesGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceFeaturesGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceFeaturesGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceFeaturesGetRequest copyWith(
          void Function(ClientDeviceFeaturesGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceFeaturesGetRequest))
          as ClientDeviceFeaturesGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceFeaturesGetRequest create() =>
      ClientDeviceFeaturesGetRequest._();
  @$core.override
  ClientDeviceFeaturesGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceFeaturesGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceFeaturesGetRequest>(create);
  static ClientDeviceFeaturesGetRequest? _defaultInstance;
}

class ClientDeviceFeaturesGetResponse extends $pb.GeneratedMessage {
  factory ClientDeviceFeaturesGetResponse({
    $core.Iterable<DeviceFeature>? features,
  }) {
    final result = create();
    if (features != null) result.features.addAll(features);
    return result;
  }

  ClientDeviceFeaturesGetResponse._();

  factory ClientDeviceFeaturesGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceFeaturesGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceFeaturesGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<DeviceFeature>(1, _omitFieldNames ? '' : 'features',
        subBuilder: DeviceFeature.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceFeaturesGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceFeaturesGetResponse copyWith(
          void Function(ClientDeviceFeaturesGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceFeaturesGetResponse))
          as ClientDeviceFeaturesGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceFeaturesGetResponse create() =>
      ClientDeviceFeaturesGetResponse._();
  @$core.override
  ClientDeviceFeaturesGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceFeaturesGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceFeaturesGetResponse>(
          create);
  static ClientDeviceFeaturesGetResponse? _defaultInstance;

  /// At most 32 features, with unique IDs.
  @$pb.TagNumber(1)
  $pb.PbList<DeviceFeature> get features => $_getList(0);
}

class ClientDeviceFeatureSetRequest extends $pb.GeneratedMessage {
  factory ClientDeviceFeatureSetRequest({
    $core.String? featureId,
    $core.bool? enabled,
  }) {
    final result = create();
    if (featureId != null) result.featureId = featureId;
    if (enabled != null) result.enabled = enabled;
    return result;
  }

  ClientDeviceFeatureSetRequest._();

  factory ClientDeviceFeatureSetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceFeatureSetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceFeatureSetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'featureId')
    ..aOB(2, _omitFieldNames ? '' : 'enabled')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceFeatureSetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceFeatureSetRequest copyWith(
          void Function(ClientDeviceFeatureSetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceFeatureSetRequest))
          as ClientDeviceFeatureSetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceFeatureSetRequest create() =>
      ClientDeviceFeatureSetRequest._();
  @$core.override
  ClientDeviceFeatureSetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceFeatureSetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceFeatureSetRequest>(create);
  static ClientDeviceFeatureSetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get featureId => $_getSZ(0);
  @$pb.TagNumber(1)
  set featureId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFeatureId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFeatureId() => $_clearField(1);

  /// Required. Unknown ID: NOT_FOUND; immutable: FAILED_PRECONDITION.
  @$pb.TagNumber(2)
  $core.bool get enabled => $_getBF(1);
  @$pb.TagNumber(2)
  set enabled($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEnabled() => $_has(1);
  @$pb.TagNumber(2)
  void clearEnabled() => $_clearField(2);
}

class ClientDeviceFeatureSetResponse extends $pb.GeneratedMessage {
  factory ClientDeviceFeatureSetResponse({
    DeviceFeature? applied,
  }) {
    final result = create();
    if (applied != null) result.applied = applied;
    return result;
  }

  ClientDeviceFeatureSetResponse._();

  factory ClientDeviceFeatureSetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientDeviceFeatureSetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientDeviceFeatureSetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DeviceFeature>(1, _omitFieldNames ? '' : 'applied',
        subBuilder: DeviceFeature.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceFeatureSetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientDeviceFeatureSetResponse copyWith(
          void Function(ClientDeviceFeatureSetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientDeviceFeatureSetResponse))
          as ClientDeviceFeatureSetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientDeviceFeatureSetResponse create() =>
      ClientDeviceFeatureSetResponse._();
  @$core.override
  ClientDeviceFeatureSetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientDeviceFeatureSetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientDeviceFeatureSetResponse>(create);
  static ClientDeviceFeatureSetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  DeviceFeature get applied => $_getN(0);
  @$pb.TagNumber(1)
  set applied(DeviceFeature value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApplied() => $_has(0);
  @$pb.TagNumber(1)
  void clearApplied() => $_clearField(1);
  @$pb.TagNumber(1)
  DeviceFeature ensureApplied() => $_ensure(0);
}

/// The device owns accounting and enforces this limit in its current usage
/// window. Zero limit means unlimited; used_seconds still reports actual use.
class UsageTimeState extends $pb.GeneratedMessage {
  factory UsageTimeState({
    $core.int? limitSeconds,
    $core.int? usedSeconds,
    $core.bool? exhausted,
  }) {
    final result = create();
    if (limitSeconds != null) result.limitSeconds = limitSeconds;
    if (usedSeconds != null) result.usedSeconds = usedSeconds;
    if (exhausted != null) result.exhausted = exhausted;
    return result;
  }

  UsageTimeState._();

  factory UsageTimeState.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory UsageTimeState.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UsageTimeState',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'limitSeconds',
        fieldType: $pb.PbFieldType.OU3)
    ..aI(2, _omitFieldNames ? '' : 'usedSeconds',
        fieldType: $pb.PbFieldType.OU3)
    ..aOB(3, _omitFieldNames ? '' : 'exhausted')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UsageTimeState clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UsageTimeState copyWith(void Function(UsageTimeState) updates) =>
      super.copyWith((message) => updates(message as UsageTimeState))
          as UsageTimeState;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static UsageTimeState create() => UsageTimeState._();
  @$core.override
  UsageTimeState createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static UsageTimeState getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<UsageTimeState>(create);
  static UsageTimeState? _defaultInstance;

  /// Required presence, including zero.
  @$pb.TagNumber(1)
  $core.int get limitSeconds => $_getIZ(0);
  @$pb.TagNumber(1)
  set limitSeconds($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasLimitSeconds() => $_has(0);
  @$pb.TagNumber(1)
  void clearLimitSeconds() => $_clearField(1);

  /// Required presence, including zero.
  @$pb.TagNumber(2)
  $core.int get usedSeconds => $_getIZ(1);
  @$pb.TagNumber(2)
  set usedSeconds($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasUsedSeconds() => $_has(1);
  @$pb.TagNumber(2)
  void clearUsedSeconds() => $_clearField(2);

  /// Required presence, including false.
  @$pb.TagNumber(3)
  $core.bool get exhausted => $_getBF(2);
  @$pb.TagNumber(3)
  set exhausted($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasExhausted() => $_has(2);
  @$pb.TagNumber(3)
  void clearExhausted() => $_clearField(3);
}

class ClientUsageTimeGetRequest extends $pb.GeneratedMessage {
  factory ClientUsageTimeGetRequest() => create();

  ClientUsageTimeGetRequest._();

  factory ClientUsageTimeGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientUsageTimeGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientUsageTimeGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientUsageTimeGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientUsageTimeGetRequest copyWith(
          void Function(ClientUsageTimeGetRequest) updates) =>
      super.copyWith((message) => updates(message as ClientUsageTimeGetRequest))
          as ClientUsageTimeGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientUsageTimeGetRequest create() => ClientUsageTimeGetRequest._();
  @$core.override
  ClientUsageTimeGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientUsageTimeGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientUsageTimeGetRequest>(create);
  static ClientUsageTimeGetRequest? _defaultInstance;
}

class ClientUsageTimeGetResponse extends $pb.GeneratedMessage {
  factory ClientUsageTimeGetResponse({
    UsageTimeState? state,
  }) {
    final result = create();
    if (state != null) result.state = state;
    return result;
  }

  ClientUsageTimeGetResponse._();

  factory ClientUsageTimeGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientUsageTimeGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientUsageTimeGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<UsageTimeState>(1, _omitFieldNames ? '' : 'state',
        subBuilder: UsageTimeState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientUsageTimeGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientUsageTimeGetResponse copyWith(
          void Function(ClientUsageTimeGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientUsageTimeGetResponse))
          as ClientUsageTimeGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientUsageTimeGetResponse create() => ClientUsageTimeGetResponse._();
  @$core.override
  ClientUsageTimeGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientUsageTimeGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientUsageTimeGetResponse>(create);
  static ClientUsageTimeGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  UsageTimeState get state => $_getN(0);
  @$pb.TagNumber(1)
  set state(UsageTimeState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasState() => $_has(0);
  @$pb.TagNumber(1)
  void clearState() => $_clearField(1);
  @$pb.TagNumber(1)
  UsageTimeState ensureState() => $_ensure(0);
}

class ClientUsageTimeSetRequest extends $pb.GeneratedMessage {
  factory ClientUsageTimeSetRequest({
    $core.int? limitSeconds,
  }) {
    final result = create();
    if (limitSeconds != null) result.limitSeconds = limitSeconds;
    return result;
  }

  ClientUsageTimeSetRequest._();

  factory ClientUsageTimeSetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientUsageTimeSetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientUsageTimeSetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'limitSeconds',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientUsageTimeSetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientUsageTimeSetRequest copyWith(
          void Function(ClientUsageTimeSetRequest) updates) =>
      super.copyWith((message) => updates(message as ClientUsageTimeSetRequest))
          as ClientUsageTimeSetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientUsageTimeSetRequest create() => ClientUsageTimeSetRequest._();
  @$core.override
  ClientUsageTimeSetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientUsageTimeSetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientUsageTimeSetRequest>(create);
  static ClientUsageTimeSetRequest? _defaultInstance;

  /// Required presence, including zero.
  @$pb.TagNumber(1)
  $core.int get limitSeconds => $_getIZ(0);
  @$pb.TagNumber(1)
  set limitSeconds($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasLimitSeconds() => $_has(0);
  @$pb.TagNumber(1)
  void clearLimitSeconds() => $_clearField(1);
}

class ClientUsageTimeSetResponse extends $pb.GeneratedMessage {
  factory ClientUsageTimeSetResponse({
    UsageTimeState? applied,
  }) {
    final result = create();
    if (applied != null) result.applied = applied;
    return result;
  }

  ClientUsageTimeSetResponse._();

  factory ClientUsageTimeSetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientUsageTimeSetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientUsageTimeSetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<UsageTimeState>(1, _omitFieldNames ? '' : 'applied',
        subBuilder: UsageTimeState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientUsageTimeSetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientUsageTimeSetResponse copyWith(
          void Function(ClientUsageTimeSetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ClientUsageTimeSetResponse))
          as ClientUsageTimeSetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientUsageTimeSetResponse create() => ClientUsageTimeSetResponse._();
  @$core.override
  ClientUsageTimeSetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientUsageTimeSetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientUsageTimeSetResponse>(create);
  static ClientUsageTimeSetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  UsageTimeState get applied => $_getN(0);
  @$pb.TagNumber(1)
  set applied(UsageTimeState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApplied() => $_has(0);
  @$pb.TagNumber(1)
  void clearApplied() => $_clearField(1);
  @$pb.TagNumber(1)
  UsageTimeState ensureApplied() => $_ensure(0);
}

class AISafetyOption extends $pb.GeneratedMessage {
  factory AISafetyOption({
    $core.String? id,
    $core.String? displayName,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (displayName != null) result.displayName = displayName;
    return result;
  }

  AISafetyOption._();

  factory AISafetyOption.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory AISafetyOption.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'AISafetyOption',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'displayName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AISafetyOption clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AISafetyOption copyWith(void Function(AISafetyOption) updates) =>
      super.copyWith((message) => updates(message as AISafetyOption))
          as AISafetyOption;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static AISafetyOption create() => AISafetyOption._();
  @$core.override
  AISafetyOption createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static AISafetyOption getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<AISafetyOption>(create);
  static AISafetyOption? _defaultInstance;

  /// Device-declared stable level, 1..64 UTF-8 bytes.
  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  /// Display text, 1..128 UTF-8 bytes.
  @$pb.TagNumber(2)
  $core.String get displayName => $_getSZ(1);
  @$pb.TagNumber(2)
  set displayName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDisplayName() => $_has(1);
  @$pb.TagNumber(2)
  void clearDisplayName() => $_clearField(2);
}

class AISafetyState extends $pb.GeneratedMessage {
  factory AISafetyState({
    $core.String? level,
    $core.Iterable<AISafetyOption>? options,
  }) {
    final result = create();
    if (level != null) result.level = level;
    if (options != null) result.options.addAll(options);
    return result;
  }

  AISafetyState._();

  factory AISafetyState.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory AISafetyState.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'AISafetyState',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'level')
    ..pPM<AISafetyOption>(2, _omitFieldNames ? '' : 'options',
        subBuilder: AISafetyOption.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AISafetyState clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AISafetyState copyWith(void Function(AISafetyState) updates) =>
      super.copyWith((message) => updates(message as AISafetyState))
          as AISafetyState;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static AISafetyState create() => AISafetyState._();
  @$core.override
  AISafetyState createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static AISafetyState getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<AISafetyState>(create);
  static AISafetyState? _defaultInstance;

  /// Effective level. Must occur in options.
  @$pb.TagNumber(1)
  $core.String get level => $_getSZ(0);
  @$pb.TagNumber(1)
  set level($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasLevel() => $_has(0);
  @$pb.TagNumber(1)
  void clearLevel() => $_clearField(1);

  /// At most 16 supported choices, with unique IDs.
  @$pb.TagNumber(2)
  $pb.PbList<AISafetyOption> get options => $_getList(1);
}

class ClientAISafetyGetRequest extends $pb.GeneratedMessage {
  factory ClientAISafetyGetRequest() => create();

  ClientAISafetyGetRequest._();

  factory ClientAISafetyGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientAISafetyGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAISafetyGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAISafetyGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAISafetyGetRequest copyWith(
          void Function(ClientAISafetyGetRequest) updates) =>
      super.copyWith((message) => updates(message as ClientAISafetyGetRequest))
          as ClientAISafetyGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientAISafetyGetRequest create() => ClientAISafetyGetRequest._();
  @$core.override
  ClientAISafetyGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientAISafetyGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAISafetyGetRequest>(create);
  static ClientAISafetyGetRequest? _defaultInstance;
}

class ClientAISafetyGetResponse extends $pb.GeneratedMessage {
  factory ClientAISafetyGetResponse({
    AISafetyState? state,
  }) {
    final result = create();
    if (state != null) result.state = state;
    return result;
  }

  ClientAISafetyGetResponse._();

  factory ClientAISafetyGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientAISafetyGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAISafetyGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AISafetyState>(1, _omitFieldNames ? '' : 'state',
        subBuilder: AISafetyState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAISafetyGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAISafetyGetResponse copyWith(
          void Function(ClientAISafetyGetResponse) updates) =>
      super.copyWith((message) => updates(message as ClientAISafetyGetResponse))
          as ClientAISafetyGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientAISafetyGetResponse create() => ClientAISafetyGetResponse._();
  @$core.override
  ClientAISafetyGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientAISafetyGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAISafetyGetResponse>(create);
  static ClientAISafetyGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  AISafetyState get state => $_getN(0);
  @$pb.TagNumber(1)
  set state(AISafetyState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasState() => $_has(0);
  @$pb.TagNumber(1)
  void clearState() => $_clearField(1);
  @$pb.TagNumber(1)
  AISafetyState ensureState() => $_ensure(0);
}

class ClientAISafetySetRequest extends $pb.GeneratedMessage {
  factory ClientAISafetySetRequest({
    $core.String? level,
  }) {
    final result = create();
    if (level != null) result.level = level;
    return result;
  }

  ClientAISafetySetRequest._();

  factory ClientAISafetySetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientAISafetySetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAISafetySetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'level')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAISafetySetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAISafetySetRequest copyWith(
          void Function(ClientAISafetySetRequest) updates) =>
      super.copyWith((message) => updates(message as ClientAISafetySetRequest))
          as ClientAISafetySetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientAISafetySetRequest create() => ClientAISafetySetRequest._();
  @$core.override
  ClientAISafetySetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientAISafetySetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAISafetySetRequest>(create);
  static ClientAISafetySetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get level => $_getSZ(0);
  @$pb.TagNumber(1)
  set level($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasLevel() => $_has(0);
  @$pb.TagNumber(1)
  void clearLevel() => $_clearField(1);
}

class ClientAISafetySetResponse extends $pb.GeneratedMessage {
  factory ClientAISafetySetResponse({
    AISafetyState? applied,
  }) {
    final result = create();
    if (applied != null) result.applied = applied;
    return result;
  }

  ClientAISafetySetResponse._();

  factory ClientAISafetySetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientAISafetySetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientAISafetySetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<AISafetyState>(1, _omitFieldNames ? '' : 'applied',
        subBuilder: AISafetyState.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAISafetySetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientAISafetySetResponse copyWith(
          void Function(ClientAISafetySetResponse) updates) =>
      super.copyWith((message) => updates(message as ClientAISafetySetResponse))
          as ClientAISafetySetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientAISafetySetResponse create() => ClientAISafetySetResponse._();
  @$core.override
  ClientAISafetySetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientAISafetySetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientAISafetySetResponse>(create);
  static ClientAISafetySetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  AISafetyState get applied => $_getN(0);
  @$pb.TagNumber(1)
  set applied(AISafetyState value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApplied() => $_has(0);
  @$pb.TagNumber(1)
  void clearApplied() => $_clearField(1);
  @$pb.TagNumber(1)
  AISafetyState ensureApplied() => $_ensure(0);
}

class ClientManagementPasswordGetRequest extends $pb.GeneratedMessage {
  factory ClientManagementPasswordGetRequest() => create();

  ClientManagementPasswordGetRequest._();

  factory ClientManagementPasswordGetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientManagementPasswordGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientManagementPasswordGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordGetRequest copyWith(
          void Function(ClientManagementPasswordGetRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientManagementPasswordGetRequest))
          as ClientManagementPasswordGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordGetRequest create() =>
      ClientManagementPasswordGetRequest._();
  @$core.override
  ClientManagementPasswordGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientManagementPasswordGetRequest>(
          create);
  static ClientManagementPasswordGetRequest? _defaultInstance;
}

class ClientManagementPasswordGetResponse extends $pb.GeneratedMessage {
  factory ClientManagementPasswordGetResponse({
    $core.bool? isSet,
  }) {
    final result = create();
    if (isSet != null) result.isSet = isSet;
    return result;
  }

  ClientManagementPasswordGetResponse._();

  factory ClientManagementPasswordGetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientManagementPasswordGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientManagementPasswordGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'isSet')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordGetResponse copyWith(
          void Function(ClientManagementPasswordGetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientManagementPasswordGetResponse))
          as ClientManagementPasswordGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordGetResponse create() =>
      ClientManagementPasswordGetResponse._();
  @$core.override
  ClientManagementPasswordGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordGetResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientManagementPasswordGetResponse>(create);
  static ClientManagementPasswordGetResponse? _defaultInstance;

  /// Required presence, including false.
  @$pb.TagNumber(1)
  $core.bool get isSet => $_getBF(0);
  @$pb.TagNumber(1)
  set isSet($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasIsSet() => $_has(0);
  @$pb.TagNumber(1)
  void clearIsSet() => $_clearField(1);
}

class ClientManagementPasswordSetRequest extends $pb.GeneratedMessage {
  factory ClientManagementPasswordSetRequest({
    $core.String? password,
  }) {
    final result = create();
    if (password != null) result.password = password;
    return result;
  }

  ClientManagementPasswordSetRequest._();

  factory ClientManagementPasswordSetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientManagementPasswordSetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientManagementPasswordSetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'password')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordSetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordSetRequest copyWith(
          void Function(ClientManagementPasswordSetRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientManagementPasswordSetRequest))
          as ClientManagementPasswordSetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordSetRequest create() =>
      ClientManagementPasswordSetRequest._();
  @$core.override
  ClientManagementPasswordSetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordSetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientManagementPasswordSetRequest>(
          create);
  static ClientManagementPasswordSetRequest? _defaultInstance;

  /// 1..128 UTF-8 bytes. Sensitive: never log, trace or copy into telemetry.
  @$pb.TagNumber(1)
  $core.String get password => $_getSZ(0);
  @$pb.TagNumber(1)
  set password($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPassword() => $_has(0);
  @$pb.TagNumber(1)
  void clearPassword() => $_clearField(1);
}

class ClientManagementPasswordSetResponse extends $pb.GeneratedMessage {
  factory ClientManagementPasswordSetResponse({
    $core.bool? isSet,
  }) {
    final result = create();
    if (isSet != null) result.isSet = isSet;
    return result;
  }

  ClientManagementPasswordSetResponse._();

  factory ClientManagementPasswordSetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientManagementPasswordSetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientManagementPasswordSetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'isSet')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordSetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordSetResponse copyWith(
          void Function(ClientManagementPasswordSetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientManagementPasswordSetResponse))
          as ClientManagementPasswordSetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordSetResponse create() =>
      ClientManagementPasswordSetResponse._();
  @$core.override
  ClientManagementPasswordSetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordSetResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientManagementPasswordSetResponse>(create);
  static ClientManagementPasswordSetResponse? _defaultInstance;

  /// Required, true only after the password is persistently set.
  @$pb.TagNumber(1)
  $core.bool get isSet => $_getBF(0);
  @$pb.TagNumber(1)
  set isSet($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasIsSet() => $_has(0);
  @$pb.TagNumber(1)
  void clearIsSet() => $_clearField(1);
}

/// Clears only the management password. Does not factory-reset the device.
class ClientManagementPasswordResetRequest extends $pb.GeneratedMessage {
  factory ClientManagementPasswordResetRequest() => create();

  ClientManagementPasswordResetRequest._();

  factory ClientManagementPasswordResetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientManagementPasswordResetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientManagementPasswordResetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordResetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordResetRequest copyWith(
          void Function(ClientManagementPasswordResetRequest) updates) =>
      super.copyWith((message) =>
              updates(message as ClientManagementPasswordResetRequest))
          as ClientManagementPasswordResetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordResetRequest create() =>
      ClientManagementPasswordResetRequest._();
  @$core.override
  ClientManagementPasswordResetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordResetRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientManagementPasswordResetRequest>(create);
  static ClientManagementPasswordResetRequest? _defaultInstance;
}

class ClientManagementPasswordResetResponse extends $pb.GeneratedMessage {
  factory ClientManagementPasswordResetResponse({
    $core.bool? isSet,
  }) {
    final result = create();
    if (isSet != null) result.isSet = isSet;
    return result;
  }

  ClientManagementPasswordResetResponse._();

  factory ClientManagementPasswordResetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientManagementPasswordResetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientManagementPasswordResetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'isSet')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordResetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientManagementPasswordResetResponse copyWith(
          void Function(ClientManagementPasswordResetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as ClientManagementPasswordResetResponse))
          as ClientManagementPasswordResetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordResetResponse create() =>
      ClientManagementPasswordResetResponse._();
  @$core.override
  ClientManagementPasswordResetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientManagementPasswordResetResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ClientManagementPasswordResetResponse>(create);
  static ClientManagementPasswordResetResponse? _defaultInstance;

  /// Required, false only after the password is cleared.
  @$pb.TagNumber(1)
  $core.bool get isSet => $_getBF(0);
  @$pb.TagNumber(1)
  set isSet($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasIsSet() => $_has(0);
  @$pb.TagNumber(1)
  void clearIsSet() => $_clearField(1);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
