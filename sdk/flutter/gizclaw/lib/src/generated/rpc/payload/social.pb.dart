// This is a generated file - do not edit.
//
// Generated from payload/social.proto.

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

class ContactCreateRequest extends $pb.GeneratedMessage {
  factory ContactCreateRequest({
    $core.String? displayName,
    $core.String? phoneNumber,
  }) {
    final result = create();
    if (displayName != null) result.displayName = displayName;
    if (phoneNumber != null) result.phoneNumber = phoneNumber;
    return result;
  }

  ContactCreateRequest._();

  factory ContactCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..aOS(2, _omitFieldNames ? '' : 'phoneNumber')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactCreateRequest copyWith(void Function(ContactCreateRequest) updates) =>
      super.copyWith((message) => updates(message as ContactCreateRequest))
          as ContactCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactCreateRequest create() => ContactCreateRequest._();
  @$core.override
  ContactCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactCreateRequest>(create);
  static ContactCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get phoneNumber => $_getSZ(1);
  @$pb.TagNumber(2)
  set phoneNumber($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPhoneNumber() => $_has(1);
  @$pb.TagNumber(2)
  void clearPhoneNumber() => $_clearField(2);
}

class ContactCreateResponse extends $pb.GeneratedMessage {
  factory ContactCreateResponse({
    ContactObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ContactCreateResponse._();

  factory ContactCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ContactObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: ContactObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactCreateResponse copyWith(
          void Function(ContactCreateResponse) updates) =>
      super.copyWith((message) => updates(message as ContactCreateResponse))
          as ContactCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactCreateResponse create() => ContactCreateResponse._();
  @$core.override
  ContactCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactCreateResponse>(create);
  static ContactCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  ContactObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(ContactObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  ContactObject ensureValue() => $_ensure(0);
}

class ContactDeleteRequest extends $pb.GeneratedMessage {
  factory ContactDeleteRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  ContactDeleteRequest._();

  factory ContactDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactDeleteRequest copyWith(void Function(ContactDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as ContactDeleteRequest))
          as ContactDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactDeleteRequest create() => ContactDeleteRequest._();
  @$core.override
  ContactDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactDeleteRequest>(create);
  static ContactDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class ContactDeleteResponse extends $pb.GeneratedMessage {
  factory ContactDeleteResponse({
    ContactObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ContactDeleteResponse._();

  factory ContactDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ContactObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: ContactObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactDeleteResponse copyWith(
          void Function(ContactDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as ContactDeleteResponse))
          as ContactDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactDeleteResponse create() => ContactDeleteResponse._();
  @$core.override
  ContactDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactDeleteResponse>(create);
  static ContactDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  ContactObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(ContactObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  ContactObject ensureValue() => $_ensure(0);
}

class ContactGetRequest extends $pb.GeneratedMessage {
  factory ContactGetRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  ContactGetRequest._();

  factory ContactGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactGetRequest copyWith(void Function(ContactGetRequest) updates) =>
      super.copyWith((message) => updates(message as ContactGetRequest))
          as ContactGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactGetRequest create() => ContactGetRequest._();
  @$core.override
  ContactGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactGetRequest>(create);
  static ContactGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class ContactGetResponse extends $pb.GeneratedMessage {
  factory ContactGetResponse({
    ContactObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ContactGetResponse._();

  factory ContactGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ContactObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: ContactObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactGetResponse copyWith(void Function(ContactGetResponse) updates) =>
      super.copyWith((message) => updates(message as ContactGetResponse))
          as ContactGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactGetResponse create() => ContactGetResponse._();
  @$core.override
  ContactGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactGetResponse>(create);
  static ContactGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  ContactObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(ContactObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  ContactObject ensureValue() => $_ensure(0);
}

class ContactListRequest extends $pb.GeneratedMessage {
  factory ContactListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  ContactListRequest._();

  factory ContactListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactListRequest copyWith(void Function(ContactListRequest) updates) =>
      super.copyWith((message) => updates(message as ContactListRequest))
          as ContactListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactListRequest create() => ContactListRequest._();
  @$core.override
  ContactListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactListRequest>(create);
  static ContactListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);
}

class ContactListResponse extends $pb.GeneratedMessage {
  factory ContactListResponse({
    $core.bool? hasNext,
    $core.Iterable<ContactObject>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  ContactListResponse._();

  factory ContactListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<ContactObject>(2, _omitFieldNames ? '' : 'items',
        subBuilder: ContactObject.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactListResponse copyWith(void Function(ContactListResponse) updates) =>
      super.copyWith((message) => updates(message as ContactListResponse))
          as ContactListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactListResponse create() => ContactListResponse._();
  @$core.override
  ContactListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactListResponse>(create);
  static ContactListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<ContactObject> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class ContactObject extends $pb.GeneratedMessage {
  factory ContactObject({
    $core.String? createdAt,
    $core.String? displayName,
    $core.String? id,
    $core.String? phoneNumber,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (displayName != null) result.displayName = displayName;
    if (id != null) result.id = id;
    if (phoneNumber != null) result.phoneNumber = phoneNumber;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  ContactObject._();

  factory ContactObject.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactObject.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactObject',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'displayName')
    ..aOS(3, _omitFieldNames ? '' : 'id')
    ..aOS(4, _omitFieldNames ? '' : 'phoneNumber')
    ..aOS(5, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactObject clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactObject copyWith(void Function(ContactObject) updates) =>
      super.copyWith((message) => updates(message as ContactObject))
          as ContactObject;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactObject create() => ContactObject._();
  @$core.override
  ContactObject createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactObject getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactObject>(create);
  static ContactObject? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get displayName => $_getSZ(1);
  @$pb.TagNumber(2)
  set displayName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDisplayName() => $_has(1);
  @$pb.TagNumber(2)
  void clearDisplayName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get id => $_getSZ(2);
  @$pb.TagNumber(3)
  set id($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasId() => $_has(2);
  @$pb.TagNumber(3)
  void clearId() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get phoneNumber => $_getSZ(3);
  @$pb.TagNumber(4)
  set phoneNumber($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPhoneNumber() => $_has(3);
  @$pb.TagNumber(4)
  void clearPhoneNumber() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get updatedAt => $_getSZ(4);
  @$pb.TagNumber(5)
  set updatedAt($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasUpdatedAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearUpdatedAt() => $_clearField(5);
}

class ContactPutRequest extends $pb.GeneratedMessage {
  factory ContactPutRequest({
    $core.String? displayName,
    $core.String? id,
    $core.String? phoneNumber,
  }) {
    final result = create();
    if (displayName != null) result.displayName = displayName;
    if (id != null) result.id = id;
    if (phoneNumber != null) result.phoneNumber = phoneNumber;
    return result;
  }

  ContactPutRequest._();

  factory ContactPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..aOS(3, _omitFieldNames ? '' : 'phoneNumber')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactPutRequest copyWith(void Function(ContactPutRequest) updates) =>
      super.copyWith((message) => updates(message as ContactPutRequest))
          as ContactPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactPutRequest create() => ContactPutRequest._();
  @$core.override
  ContactPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactPutRequest>(create);
  static ContactPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get phoneNumber => $_getSZ(2);
  @$pb.TagNumber(3)
  set phoneNumber($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPhoneNumber() => $_has(2);
  @$pb.TagNumber(3)
  void clearPhoneNumber() => $_clearField(3);
}

class ContactPutResponse extends $pb.GeneratedMessage {
  factory ContactPutResponse({
    ContactObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ContactPutResponse._();

  factory ContactPutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ContactPutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ContactPutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ContactObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: ContactObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactPutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ContactPutResponse copyWith(void Function(ContactPutResponse) updates) =>
      super.copyWith((message) => updates(message as ContactPutResponse))
          as ContactPutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ContactPutResponse create() => ContactPutResponse._();
  @$core.override
  ContactPutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ContactPutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ContactPutResponse>(create);
  static ContactPutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  ContactObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(ContactObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  ContactObject ensureValue() => $_ensure(0);
}

class FriendAddRequest extends $pb.GeneratedMessage {
  factory FriendAddRequest({
    $core.String? inviteToken,
  }) {
    final result = create();
    if (inviteToken != null) result.inviteToken = inviteToken;
    return result;
  }

  FriendAddRequest._();

  factory FriendAddRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendAddRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendAddRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'inviteToken')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendAddRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendAddRequest copyWith(void Function(FriendAddRequest) updates) =>
      super.copyWith((message) => updates(message as FriendAddRequest))
          as FriendAddRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendAddRequest create() => FriendAddRequest._();
  @$core.override
  FriendAddRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendAddRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendAddRequest>(create);
  static FriendAddRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get inviteToken => $_getSZ(0);
  @$pb.TagNumber(1)
  set inviteToken($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasInviteToken() => $_has(0);
  @$pb.TagNumber(1)
  void clearInviteToken() => $_clearField(1);
}

class FriendAddResponse extends $pb.GeneratedMessage {
  factory FriendAddResponse({
    FriendObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendAddResponse._();

  factory FriendAddResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendAddResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendAddResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendAddResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendAddResponse copyWith(void Function(FriendAddResponse) updates) =>
      super.copyWith((message) => updates(message as FriendAddResponse))
          as FriendAddResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendAddResponse create() => FriendAddResponse._();
  @$core.override
  FriendAddResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendAddResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendAddResponse>(create);
  static FriendAddResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendObject ensureValue() => $_ensure(0);
}

class FriendDeleteRequest extends $pb.GeneratedMessage {
  factory FriendDeleteRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  FriendDeleteRequest._();

  factory FriendDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendDeleteRequest copyWith(void Function(FriendDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as FriendDeleteRequest))
          as FriendDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendDeleteRequest create() => FriendDeleteRequest._();
  @$core.override
  FriendDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendDeleteRequest>(create);
  static FriendDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class FriendDeleteResponse extends $pb.GeneratedMessage {
  factory FriendDeleteResponse({
    FriendObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendDeleteResponse._();

  factory FriendDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendDeleteResponse copyWith(void Function(FriendDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as FriendDeleteResponse))
          as FriendDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendDeleteResponse create() => FriendDeleteResponse._();
  @$core.override
  FriendDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendDeleteResponse>(create);
  static FriendDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendObject ensureValue() => $_ensure(0);
}

class FriendInfo extends $pb.GeneratedMessage {
  factory FriendInfo({
    $core.String? name,
    $core.String? emoji,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (emoji != null) result.emoji = emoji;
    return result;
  }

  FriendInfo._();

  factory FriendInfo.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInfo.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInfo',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'emoji')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInfo clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInfo copyWith(void Function(FriendInfo) updates) =>
      super.copyWith((message) => updates(message as FriendInfo)) as FriendInfo;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInfo create() => FriendInfo._();
  @$core.override
  FriendInfo createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInfo getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInfo>(create);
  static FriendInfo? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get emoji => $_getSZ(1);
  @$pb.TagNumber(2)
  set emoji($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEmoji() => $_has(1);
  @$pb.TagNumber(2)
  void clearEmoji() => $_clearField(2);
}

class FriendInfoGetRequest extends $pb.GeneratedMessage {
  factory FriendInfoGetRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  FriendInfoGetRequest._();

  factory FriendInfoGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInfoGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInfoGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInfoGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInfoGetRequest copyWith(void Function(FriendInfoGetRequest) updates) =>
      super.copyWith((message) => updates(message as FriendInfoGetRequest))
          as FriendInfoGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInfoGetRequest create() => FriendInfoGetRequest._();
  @$core.override
  FriendInfoGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInfoGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInfoGetRequest>(create);
  static FriendInfoGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class FriendInfoGetResponse extends $pb.GeneratedMessage {
  factory FriendInfoGetResponse({
    $core.String? id,
    FriendInfo? value,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (value != null) result.value = value;
    return result;
  }

  FriendInfoGetResponse._();

  factory FriendInfoGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInfoGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInfoGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOM<FriendInfo>(2, _omitFieldNames ? '' : 'value',
        subBuilder: FriendInfo.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInfoGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInfoGetResponse copyWith(
          void Function(FriendInfoGetResponse) updates) =>
      super.copyWith((message) => updates(message as FriendInfoGetResponse))
          as FriendInfoGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInfoGetResponse create() => FriendInfoGetResponse._();
  @$core.override
  FriendInfoGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInfoGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInfoGetResponse>(create);
  static FriendInfoGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  FriendInfo get value => $_getN(1);
  @$pb.TagNumber(2)
  set value(FriendInfo value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasValue() => $_has(1);
  @$pb.TagNumber(2)
  void clearValue() => $_clearField(2);
  @$pb.TagNumber(2)
  FriendInfo ensureValue() => $_ensure(1);
}

class FriendGroupCreateRequest extends $pb.GeneratedMessage {
  factory FriendGroupCreateRequest({
    $core.String? description,
    $core.String? name,
  }) {
    final result = create();
    if (description != null) result.description = description;
    if (name != null) result.name = name;
    return result;
  }

  FriendGroupCreateRequest._();

  factory FriendGroupCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'description')
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupCreateRequest copyWith(
          void Function(FriendGroupCreateRequest) updates) =>
      super.copyWith((message) => updates(message as FriendGroupCreateRequest))
          as FriendGroupCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupCreateRequest create() => FriendGroupCreateRequest._();
  @$core.override
  FriendGroupCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupCreateRequest>(create);
  static FriendGroupCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get description => $_getSZ(0);
  @$pb.TagNumber(1)
  set description($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDescription() => $_has(0);
  @$pb.TagNumber(1)
  void clearDescription() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);
}

class FriendGroupCreateResponse extends $pb.GeneratedMessage {
  factory FriendGroupCreateResponse({
    FriendGroupObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupCreateResponse._();

  factory FriendGroupCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupCreateResponse copyWith(
          void Function(FriendGroupCreateResponse) updates) =>
      super.copyWith((message) => updates(message as FriendGroupCreateResponse))
          as FriendGroupCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupCreateResponse create() => FriendGroupCreateResponse._();
  @$core.override
  FriendGroupCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupCreateResponse>(create);
  static FriendGroupCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupObject ensureValue() => $_ensure(0);
}

class FriendGroupDeleteRequest extends $pb.GeneratedMessage {
  factory FriendGroupDeleteRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  FriendGroupDeleteRequest._();

  factory FriendGroupDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupDeleteRequest copyWith(
          void Function(FriendGroupDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as FriendGroupDeleteRequest))
          as FriendGroupDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupDeleteRequest create() => FriendGroupDeleteRequest._();
  @$core.override
  FriendGroupDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupDeleteRequest>(create);
  static FriendGroupDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class FriendGroupDeleteResponse extends $pb.GeneratedMessage {
  factory FriendGroupDeleteResponse({
    FriendGroupObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupDeleteResponse._();

  factory FriendGroupDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupDeleteResponse copyWith(
          void Function(FriendGroupDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as FriendGroupDeleteResponse))
          as FriendGroupDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupDeleteResponse create() => FriendGroupDeleteResponse._();
  @$core.override
  FriendGroupDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupDeleteResponse>(create);
  static FriendGroupDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupObject ensureValue() => $_ensure(0);
}

class FriendGroupGetRequest extends $pb.GeneratedMessage {
  factory FriendGroupGetRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  FriendGroupGetRequest._();

  factory FriendGroupGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupGetRequest copyWith(
          void Function(FriendGroupGetRequest) updates) =>
      super.copyWith((message) => updates(message as FriendGroupGetRequest))
          as FriendGroupGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupGetRequest create() => FriendGroupGetRequest._();
  @$core.override
  FriendGroupGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupGetRequest>(create);
  static FriendGroupGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class FriendGroupGetResponse extends $pb.GeneratedMessage {
  factory FriendGroupGetResponse({
    FriendGroupObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupGetResponse._();

  factory FriendGroupGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupGetResponse copyWith(
          void Function(FriendGroupGetResponse) updates) =>
      super.copyWith((message) => updates(message as FriendGroupGetResponse))
          as FriendGroupGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupGetResponse create() => FriendGroupGetResponse._();
  @$core.override
  FriendGroupGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupGetResponse>(create);
  static FriendGroupGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupObject ensureValue() => $_ensure(0);
}

class FriendGroupInviteTokenClearRequest extends $pb.GeneratedMessage {
  factory FriendGroupInviteTokenClearRequest({
    $core.String? friendGroupId,
  }) {
    final result = create();
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    return result;
  }

  FriendGroupInviteTokenClearRequest._();

  factory FriendGroupInviteTokenClearRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupInviteTokenClearRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupInviteTokenClearRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'friendGroupId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenClearRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenClearRequest copyWith(
          void Function(FriendGroupInviteTokenClearRequest) updates) =>
      super.copyWith((message) =>
              updates(message as FriendGroupInviteTokenClearRequest))
          as FriendGroupInviteTokenClearRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenClearRequest create() =>
      FriendGroupInviteTokenClearRequest._();
  @$core.override
  FriendGroupInviteTokenClearRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenClearRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupInviteTokenClearRequest>(
          create);
  static FriendGroupInviteTokenClearRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get friendGroupId => $_getSZ(0);
  @$pb.TagNumber(1)
  set friendGroupId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFriendGroupId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFriendGroupId() => $_clearField(1);
}

class FriendGroupInviteTokenClearResponse extends $pb.GeneratedMessage {
  factory FriendGroupInviteTokenClearResponse() => create();

  FriendGroupInviteTokenClearResponse._();

  factory FriendGroupInviteTokenClearResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupInviteTokenClearResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupInviteTokenClearResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenClearResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenClearResponse copyWith(
          void Function(FriendGroupInviteTokenClearResponse) updates) =>
      super.copyWith((message) =>
              updates(message as FriendGroupInviteTokenClearResponse))
          as FriendGroupInviteTokenClearResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenClearResponse create() =>
      FriendGroupInviteTokenClearResponse._();
  @$core.override
  FriendGroupInviteTokenClearResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenClearResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          FriendGroupInviteTokenClearResponse>(create);
  static FriendGroupInviteTokenClearResponse? _defaultInstance;
}

class FriendGroupInviteTokenCreateRequest extends $pb.GeneratedMessage {
  factory FriendGroupInviteTokenCreateRequest({
    $core.String? friendGroupId,
  }) {
    final result = create();
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    return result;
  }

  FriendGroupInviteTokenCreateRequest._();

  factory FriendGroupInviteTokenCreateRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupInviteTokenCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupInviteTokenCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'friendGroupId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenCreateRequest copyWith(
          void Function(FriendGroupInviteTokenCreateRequest) updates) =>
      super.copyWith((message) =>
              updates(message as FriendGroupInviteTokenCreateRequest))
          as FriendGroupInviteTokenCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenCreateRequest create() =>
      FriendGroupInviteTokenCreateRequest._();
  @$core.override
  FriendGroupInviteTokenCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenCreateRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          FriendGroupInviteTokenCreateRequest>(create);
  static FriendGroupInviteTokenCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get friendGroupId => $_getSZ(0);
  @$pb.TagNumber(1)
  set friendGroupId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFriendGroupId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFriendGroupId() => $_clearField(1);
}

class FriendGroupInviteTokenCreateResponse extends $pb.GeneratedMessage {
  factory FriendGroupInviteTokenCreateResponse({
    $core.String? expiresAt,
    $core.String? inviteToken,
  }) {
    final result = create();
    if (expiresAt != null) result.expiresAt = expiresAt;
    if (inviteToken != null) result.inviteToken = inviteToken;
    return result;
  }

  FriendGroupInviteTokenCreateResponse._();

  factory FriendGroupInviteTokenCreateResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupInviteTokenCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupInviteTokenCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'expiresAt')
    ..aOS(2, _omitFieldNames ? '' : 'inviteToken')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenCreateResponse copyWith(
          void Function(FriendGroupInviteTokenCreateResponse) updates) =>
      super.copyWith((message) =>
              updates(message as FriendGroupInviteTokenCreateResponse))
          as FriendGroupInviteTokenCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenCreateResponse create() =>
      FriendGroupInviteTokenCreateResponse._();
  @$core.override
  FriendGroupInviteTokenCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenCreateResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          FriendGroupInviteTokenCreateResponse>(create);
  static FriendGroupInviteTokenCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get expiresAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set expiresAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasExpiresAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearExpiresAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get inviteToken => $_getSZ(1);
  @$pb.TagNumber(2)
  set inviteToken($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasInviteToken() => $_has(1);
  @$pb.TagNumber(2)
  void clearInviteToken() => $_clearField(2);
}

class FriendGroupInviteTokenGetRequest extends $pb.GeneratedMessage {
  factory FriendGroupInviteTokenGetRequest({
    $core.String? friendGroupId,
  }) {
    final result = create();
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    return result;
  }

  FriendGroupInviteTokenGetRequest._();

  factory FriendGroupInviteTokenGetRequest.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupInviteTokenGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupInviteTokenGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'friendGroupId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenGetRequest copyWith(
          void Function(FriendGroupInviteTokenGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupInviteTokenGetRequest))
          as FriendGroupInviteTokenGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenGetRequest create() =>
      FriendGroupInviteTokenGetRequest._();
  @$core.override
  FriendGroupInviteTokenGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupInviteTokenGetRequest>(
          create);
  static FriendGroupInviteTokenGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get friendGroupId => $_getSZ(0);
  @$pb.TagNumber(1)
  set friendGroupId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFriendGroupId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFriendGroupId() => $_clearField(1);
}

class FriendGroupInviteTokenGetResponse extends $pb.GeneratedMessage {
  factory FriendGroupInviteTokenGetResponse({
    $core.String? expiresAt,
    $core.String? inviteToken,
  }) {
    final result = create();
    if (expiresAt != null) result.expiresAt = expiresAt;
    if (inviteToken != null) result.inviteToken = inviteToken;
    return result;
  }

  FriendGroupInviteTokenGetResponse._();

  factory FriendGroupInviteTokenGetResponse.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupInviteTokenGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupInviteTokenGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'expiresAt')
    ..aOS(2, _omitFieldNames ? '' : 'inviteToken')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupInviteTokenGetResponse copyWith(
          void Function(FriendGroupInviteTokenGetResponse) updates) =>
      super.copyWith((message) =>
              updates(message as FriendGroupInviteTokenGetResponse))
          as FriendGroupInviteTokenGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenGetResponse create() =>
      FriendGroupInviteTokenGetResponse._();
  @$core.override
  FriendGroupInviteTokenGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupInviteTokenGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupInviteTokenGetResponse>(
          create);
  static FriendGroupInviteTokenGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get expiresAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set expiresAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasExpiresAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearExpiresAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get inviteToken => $_getSZ(1);
  @$pb.TagNumber(2)
  set inviteToken($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasInviteToken() => $_has(1);
  @$pb.TagNumber(2)
  void clearInviteToken() => $_clearField(2);
}

class FriendGroupJoinRequest extends $pb.GeneratedMessage {
  factory FriendGroupJoinRequest({
    $core.String? inviteToken,
  }) {
    final result = create();
    if (inviteToken != null) result.inviteToken = inviteToken;
    return result;
  }

  FriendGroupJoinRequest._();

  factory FriendGroupJoinRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupJoinRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupJoinRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'inviteToken')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupJoinRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupJoinRequest copyWith(
          void Function(FriendGroupJoinRequest) updates) =>
      super.copyWith((message) => updates(message as FriendGroupJoinRequest))
          as FriendGroupJoinRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupJoinRequest create() => FriendGroupJoinRequest._();
  @$core.override
  FriendGroupJoinRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupJoinRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupJoinRequest>(create);
  static FriendGroupJoinRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get inviteToken => $_getSZ(0);
  @$pb.TagNumber(1)
  set inviteToken($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasInviteToken() => $_has(0);
  @$pb.TagNumber(1)
  void clearInviteToken() => $_clearField(1);
}

class FriendGroupJoinResponse extends $pb.GeneratedMessage {
  factory FriendGroupJoinResponse({
    FriendGroupObject? group,
    FriendGroupMemberObject? member,
  }) {
    final result = create();
    if (group != null) result.group = group;
    if (member != null) result.member = member;
    return result;
  }

  FriendGroupJoinResponse._();

  factory FriendGroupJoinResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupJoinResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupJoinResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupObject>(1, _omitFieldNames ? '' : 'group',
        subBuilder: FriendGroupObject.create)
    ..aOM<FriendGroupMemberObject>(2, _omitFieldNames ? '' : 'member',
        subBuilder: FriendGroupMemberObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupJoinResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupJoinResponse copyWith(
          void Function(FriendGroupJoinResponse) updates) =>
      super.copyWith((message) => updates(message as FriendGroupJoinResponse))
          as FriendGroupJoinResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupJoinResponse create() => FriendGroupJoinResponse._();
  @$core.override
  FriendGroupJoinResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupJoinResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupJoinResponse>(create);
  static FriendGroupJoinResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupObject get group => $_getN(0);
  @$pb.TagNumber(1)
  set group(FriendGroupObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasGroup() => $_has(0);
  @$pb.TagNumber(1)
  void clearGroup() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupObject ensureGroup() => $_ensure(0);

  @$pb.TagNumber(2)
  FriendGroupMemberObject get member => $_getN(1);
  @$pb.TagNumber(2)
  set member(FriendGroupMemberObject value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasMember() => $_has(1);
  @$pb.TagNumber(2)
  void clearMember() => $_clearField(2);
  @$pb.TagNumber(2)
  FriendGroupMemberObject ensureMember() => $_ensure(1);
}

class FriendGroupListRequest extends $pb.GeneratedMessage {
  factory FriendGroupListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  FriendGroupListRequest._();

  factory FriendGroupListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupListRequest copyWith(
          void Function(FriendGroupListRequest) updates) =>
      super.copyWith((message) => updates(message as FriendGroupListRequest))
          as FriendGroupListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupListRequest create() => FriendGroupListRequest._();
  @$core.override
  FriendGroupListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupListRequest>(create);
  static FriendGroupListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);
}

class FriendGroupListResponse extends $pb.GeneratedMessage {
  factory FriendGroupListResponse({
    $core.bool? hasNext,
    $core.Iterable<FriendGroupObject>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  FriendGroupListResponse._();

  factory FriendGroupListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<FriendGroupObject>(2, _omitFieldNames ? '' : 'items',
        subBuilder: FriendGroupObject.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupListResponse copyWith(
          void Function(FriendGroupListResponse) updates) =>
      super.copyWith((message) => updates(message as FriendGroupListResponse))
          as FriendGroupListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupListResponse create() => FriendGroupListResponse._();
  @$core.override
  FriendGroupListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupListResponse>(create);
  static FriendGroupListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<FriendGroupObject> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class FriendGroupMemberAddRequest extends $pb.GeneratedMessage {
  factory FriendGroupMemberAddRequest({
    $core.String? friendGroupId,
    $core.String? peerPublicKey,
    $0.FriendGroupMemberMutableRole? role,
  }) {
    final result = create();
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (peerPublicKey != null) result.peerPublicKey = peerPublicKey;
    if (role != null) result.role = role;
    return result;
  }

  FriendGroupMemberAddRequest._();

  factory FriendGroupMemberAddRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberAddRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberAddRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'friendGroupId')
    ..aOS(2, _omitFieldNames ? '' : 'peerPublicKey')
    ..aE<$0.FriendGroupMemberMutableRole>(3, _omitFieldNames ? '' : 'role',
        enumValues: $0.FriendGroupMemberMutableRole.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberAddRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberAddRequest copyWith(
          void Function(FriendGroupMemberAddRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMemberAddRequest))
          as FriendGroupMemberAddRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberAddRequest create() =>
      FriendGroupMemberAddRequest._();
  @$core.override
  FriendGroupMemberAddRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberAddRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberAddRequest>(create);
  static FriendGroupMemberAddRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get friendGroupId => $_getSZ(0);
  @$pb.TagNumber(1)
  set friendGroupId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFriendGroupId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFriendGroupId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get peerPublicKey => $_getSZ(1);
  @$pb.TagNumber(2)
  set peerPublicKey($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPeerPublicKey() => $_has(1);
  @$pb.TagNumber(2)
  void clearPeerPublicKey() => $_clearField(2);

  @$pb.TagNumber(3)
  $0.FriendGroupMemberMutableRole get role => $_getN(2);
  @$pb.TagNumber(3)
  set role($0.FriendGroupMemberMutableRole value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasRole() => $_has(2);
  @$pb.TagNumber(3)
  void clearRole() => $_clearField(3);
}

class FriendGroupMemberAddResponse extends $pb.GeneratedMessage {
  factory FriendGroupMemberAddResponse({
    FriendGroupMemberObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupMemberAddResponse._();

  factory FriendGroupMemberAddResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberAddResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberAddResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupMemberObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupMemberObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberAddResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberAddResponse copyWith(
          void Function(FriendGroupMemberAddResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMemberAddResponse))
          as FriendGroupMemberAddResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberAddResponse create() =>
      FriendGroupMemberAddResponse._();
  @$core.override
  FriendGroupMemberAddResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberAddResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberAddResponse>(create);
  static FriendGroupMemberAddResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupMemberObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupMemberObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupMemberObject ensureValue() => $_ensure(0);
}

class FriendGroupMemberDeleteRequest extends $pb.GeneratedMessage {
  factory FriendGroupMemberDeleteRequest({
    $core.String? friendGroupId,
    $core.String? id,
  }) {
    final result = create();
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (id != null) result.id = id;
    return result;
  }

  FriendGroupMemberDeleteRequest._();

  factory FriendGroupMemberDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'friendGroupId')
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberDeleteRequest copyWith(
          void Function(FriendGroupMemberDeleteRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMemberDeleteRequest))
          as FriendGroupMemberDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberDeleteRequest create() =>
      FriendGroupMemberDeleteRequest._();
  @$core.override
  FriendGroupMemberDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberDeleteRequest>(create);
  static FriendGroupMemberDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get friendGroupId => $_getSZ(0);
  @$pb.TagNumber(1)
  set friendGroupId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFriendGroupId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFriendGroupId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);
}

class FriendGroupMemberDeleteResponse extends $pb.GeneratedMessage {
  factory FriendGroupMemberDeleteResponse({
    FriendGroupMemberObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupMemberDeleteResponse._();

  factory FriendGroupMemberDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupMemberObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupMemberObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberDeleteResponse copyWith(
          void Function(FriendGroupMemberDeleteResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMemberDeleteResponse))
          as FriendGroupMemberDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberDeleteResponse create() =>
      FriendGroupMemberDeleteResponse._();
  @$core.override
  FriendGroupMemberDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberDeleteResponse>(
          create);
  static FriendGroupMemberDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupMemberObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupMemberObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupMemberObject ensureValue() => $_ensure(0);
}

class FriendGroupMemberListRequest extends $pb.GeneratedMessage {
  factory FriendGroupMemberListRequest({
    $core.String? cursor,
    $core.String? friendGroupId,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (limit != null) result.limit = limit;
    return result;
  }

  FriendGroupMemberListRequest._();

  factory FriendGroupMemberListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aOS(2, _omitFieldNames ? '' : 'friendGroupId')
    ..aInt64(3, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberListRequest copyWith(
          void Function(FriendGroupMemberListRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMemberListRequest))
          as FriendGroupMemberListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberListRequest create() =>
      FriendGroupMemberListRequest._();
  @$core.override
  FriendGroupMemberListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberListRequest>(create);
  static FriendGroupMemberListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get friendGroupId => $_getSZ(1);
  @$pb.TagNumber(2)
  set friendGroupId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasFriendGroupId() => $_has(1);
  @$pb.TagNumber(2)
  void clearFriendGroupId() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get limit => $_getI64(2);
  @$pb.TagNumber(3)
  set limit($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasLimit() => $_has(2);
  @$pb.TagNumber(3)
  void clearLimit() => $_clearField(3);
}

class FriendGroupMemberListResponse extends $pb.GeneratedMessage {
  factory FriendGroupMemberListResponse({
    $core.bool? hasNext,
    $core.Iterable<FriendGroupMemberObject>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  FriendGroupMemberListResponse._();

  factory FriendGroupMemberListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<FriendGroupMemberObject>(2, _omitFieldNames ? '' : 'items',
        subBuilder: FriendGroupMemberObject.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberListResponse copyWith(
          void Function(FriendGroupMemberListResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMemberListResponse))
          as FriendGroupMemberListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberListResponse create() =>
      FriendGroupMemberListResponse._();
  @$core.override
  FriendGroupMemberListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberListResponse>(create);
  static FriendGroupMemberListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<FriendGroupMemberObject> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class FriendGroupMemberObject extends $pb.GeneratedMessage {
  factory FriendGroupMemberObject({
    $core.String? createdAt,
    $core.String? friendGroupId,
    $core.String? id,
    $core.String? peerPublicKey,
    $0.FriendGroupMemberRole? role,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (id != null) result.id = id;
    if (peerPublicKey != null) result.peerPublicKey = peerPublicKey;
    if (role != null) result.role = role;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  FriendGroupMemberObject._();

  factory FriendGroupMemberObject.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberObject.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberObject',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'friendGroupId')
    ..aOS(3, _omitFieldNames ? '' : 'id')
    ..aOS(4, _omitFieldNames ? '' : 'peerPublicKey')
    ..aE<$0.FriendGroupMemberRole>(5, _omitFieldNames ? '' : 'role',
        enumValues: $0.FriendGroupMemberRole.values)
    ..aOS(6, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberObject clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberObject copyWith(
          void Function(FriendGroupMemberObject) updates) =>
      super.copyWith((message) => updates(message as FriendGroupMemberObject))
          as FriendGroupMemberObject;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberObject create() => FriendGroupMemberObject._();
  @$core.override
  FriendGroupMemberObject createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberObject getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberObject>(create);
  static FriendGroupMemberObject? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get friendGroupId => $_getSZ(1);
  @$pb.TagNumber(2)
  set friendGroupId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasFriendGroupId() => $_has(1);
  @$pb.TagNumber(2)
  void clearFriendGroupId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get id => $_getSZ(2);
  @$pb.TagNumber(3)
  set id($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasId() => $_has(2);
  @$pb.TagNumber(3)
  void clearId() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get peerPublicKey => $_getSZ(3);
  @$pb.TagNumber(4)
  set peerPublicKey($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPeerPublicKey() => $_has(3);
  @$pb.TagNumber(4)
  void clearPeerPublicKey() => $_clearField(4);

  @$pb.TagNumber(5)
  $0.FriendGroupMemberRole get role => $_getN(4);
  @$pb.TagNumber(5)
  set role($0.FriendGroupMemberRole value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasRole() => $_has(4);
  @$pb.TagNumber(5)
  void clearRole() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get updatedAt => $_getSZ(5);
  @$pb.TagNumber(6)
  set updatedAt($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasUpdatedAt() => $_has(5);
  @$pb.TagNumber(6)
  void clearUpdatedAt() => $_clearField(6);
}

class FriendGroupMemberPutRequest extends $pb.GeneratedMessage {
  factory FriendGroupMemberPutRequest({
    $core.String? friendGroupId,
    $core.String? id,
    $0.FriendGroupMemberMutableRole? role,
  }) {
    final result = create();
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (id != null) result.id = id;
    if (role != null) result.role = role;
    return result;
  }

  FriendGroupMemberPutRequest._();

  factory FriendGroupMemberPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'friendGroupId')
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..aE<$0.FriendGroupMemberMutableRole>(3, _omitFieldNames ? '' : 'role',
        enumValues: $0.FriendGroupMemberMutableRole.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberPutRequest copyWith(
          void Function(FriendGroupMemberPutRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMemberPutRequest))
          as FriendGroupMemberPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberPutRequest create() =>
      FriendGroupMemberPutRequest._();
  @$core.override
  FriendGroupMemberPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberPutRequest>(create);
  static FriendGroupMemberPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get friendGroupId => $_getSZ(0);
  @$pb.TagNumber(1)
  set friendGroupId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFriendGroupId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFriendGroupId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);

  @$pb.TagNumber(3)
  $0.FriendGroupMemberMutableRole get role => $_getN(2);
  @$pb.TagNumber(3)
  set role($0.FriendGroupMemberMutableRole value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasRole() => $_has(2);
  @$pb.TagNumber(3)
  void clearRole() => $_clearField(3);
}

class FriendGroupMemberPutResponse extends $pb.GeneratedMessage {
  factory FriendGroupMemberPutResponse({
    FriendGroupMemberObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupMemberPutResponse._();

  factory FriendGroupMemberPutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMemberPutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMemberPutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupMemberObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupMemberObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberPutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMemberPutResponse copyWith(
          void Function(FriendGroupMemberPutResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMemberPutResponse))
          as FriendGroupMemberPutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberPutResponse create() =>
      FriendGroupMemberPutResponse._();
  @$core.override
  FriendGroupMemberPutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMemberPutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMemberPutResponse>(create);
  static FriendGroupMemberPutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupMemberObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupMemberObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupMemberObject ensureValue() => $_ensure(0);
}

class FriendGroupMessageGetRequest extends $pb.GeneratedMessage {
  factory FriendGroupMessageGetRequest({
    $core.String? friendGroupId,
    $core.String? id,
  }) {
    final result = create();
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (id != null) result.id = id;
    return result;
  }

  FriendGroupMessageGetRequest._();

  factory FriendGroupMessageGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMessageGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMessageGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'friendGroupId')
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageGetRequest copyWith(
          void Function(FriendGroupMessageGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMessageGetRequest))
          as FriendGroupMessageGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageGetRequest create() =>
      FriendGroupMessageGetRequest._();
  @$core.override
  FriendGroupMessageGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMessageGetRequest>(create);
  static FriendGroupMessageGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get friendGroupId => $_getSZ(0);
  @$pb.TagNumber(1)
  set friendGroupId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFriendGroupId() => $_has(0);
  @$pb.TagNumber(1)
  void clearFriendGroupId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);
}

class FriendGroupMessageGetResponse extends $pb.GeneratedMessage {
  factory FriendGroupMessageGetResponse({
    FriendGroupMessageObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupMessageGetResponse._();

  factory FriendGroupMessageGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMessageGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMessageGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupMessageObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupMessageObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageGetResponse copyWith(
          void Function(FriendGroupMessageGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMessageGetResponse))
          as FriendGroupMessageGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageGetResponse create() =>
      FriendGroupMessageGetResponse._();
  @$core.override
  FriendGroupMessageGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMessageGetResponse>(create);
  static FriendGroupMessageGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupMessageObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupMessageObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupMessageObject ensureValue() => $_ensure(0);
}

class FriendGroupMessageListRequest extends $pb.GeneratedMessage {
  factory FriendGroupMessageListRequest({
    $core.String? cursor,
    $core.String? friendGroupId,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (limit != null) result.limit = limit;
    return result;
  }

  FriendGroupMessageListRequest._();

  factory FriendGroupMessageListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMessageListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMessageListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aOS(2, _omitFieldNames ? '' : 'friendGroupId')
    ..aInt64(3, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageListRequest copyWith(
          void Function(FriendGroupMessageListRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMessageListRequest))
          as FriendGroupMessageListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageListRequest create() =>
      FriendGroupMessageListRequest._();
  @$core.override
  FriendGroupMessageListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMessageListRequest>(create);
  static FriendGroupMessageListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get friendGroupId => $_getSZ(1);
  @$pb.TagNumber(2)
  set friendGroupId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasFriendGroupId() => $_has(1);
  @$pb.TagNumber(2)
  void clearFriendGroupId() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get limit => $_getI64(2);
  @$pb.TagNumber(3)
  set limit($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasLimit() => $_has(2);
  @$pb.TagNumber(3)
  void clearLimit() => $_clearField(3);
}

class FriendGroupMessageListResponse extends $pb.GeneratedMessage {
  factory FriendGroupMessageListResponse({
    $core.bool? hasNext,
    $core.Iterable<FriendGroupMessageObject>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  FriendGroupMessageListResponse._();

  factory FriendGroupMessageListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMessageListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMessageListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<FriendGroupMessageObject>(2, _omitFieldNames ? '' : 'items',
        subBuilder: FriendGroupMessageObject.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageListResponse copyWith(
          void Function(FriendGroupMessageListResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMessageListResponse))
          as FriendGroupMessageListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageListResponse create() =>
      FriendGroupMessageListResponse._();
  @$core.override
  FriendGroupMessageListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMessageListResponse>(create);
  static FriendGroupMessageListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<FriendGroupMessageObject> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class FriendGroupMessageObject extends $pb.GeneratedMessage {
  factory FriendGroupMessageObject({
    $core.String? audioContentType,
    $core.String? audioPath,
    $fixnum.Int64? audioSizeBytes,
    $core.String? createdAt,
    $core.String? expiresAt,
    $core.String? friendGroupId,
    $core.String? id,
    $core.String? senderPeerPublicKey,
    $fixnum.Int64? ttlSeconds,
  }) {
    final result = create();
    if (audioContentType != null) result.audioContentType = audioContentType;
    if (audioPath != null) result.audioPath = audioPath;
    if (audioSizeBytes != null) result.audioSizeBytes = audioSizeBytes;
    if (createdAt != null) result.createdAt = createdAt;
    if (expiresAt != null) result.expiresAt = expiresAt;
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (id != null) result.id = id;
    if (senderPeerPublicKey != null)
      result.senderPeerPublicKey = senderPeerPublicKey;
    if (ttlSeconds != null) result.ttlSeconds = ttlSeconds;
    return result;
  }

  FriendGroupMessageObject._();

  factory FriendGroupMessageObject.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMessageObject.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMessageObject',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'audioContentType')
    ..aOS(2, _omitFieldNames ? '' : 'audioPath')
    ..aInt64(3, _omitFieldNames ? '' : 'audioSizeBytes')
    ..aOS(4, _omitFieldNames ? '' : 'createdAt')
    ..aOS(5, _omitFieldNames ? '' : 'expiresAt')
    ..aOS(6, _omitFieldNames ? '' : 'friendGroupId')
    ..aOS(7, _omitFieldNames ? '' : 'id')
    ..aOS(8, _omitFieldNames ? '' : 'senderPeerPublicKey')
    ..aInt64(9, _omitFieldNames ? '' : 'ttlSeconds')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageObject clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageObject copyWith(
          void Function(FriendGroupMessageObject) updates) =>
      super.copyWith((message) => updates(message as FriendGroupMessageObject))
          as FriendGroupMessageObject;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageObject create() => FriendGroupMessageObject._();
  @$core.override
  FriendGroupMessageObject createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageObject getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMessageObject>(create);
  static FriendGroupMessageObject? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get audioContentType => $_getSZ(0);
  @$pb.TagNumber(1)
  set audioContentType($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAudioContentType() => $_has(0);
  @$pb.TagNumber(1)
  void clearAudioContentType() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get audioPath => $_getSZ(1);
  @$pb.TagNumber(2)
  set audioPath($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasAudioPath() => $_has(1);
  @$pb.TagNumber(2)
  void clearAudioPath() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get audioSizeBytes => $_getI64(2);
  @$pb.TagNumber(3)
  set audioSizeBytes($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasAudioSizeBytes() => $_has(2);
  @$pb.TagNumber(3)
  void clearAudioSizeBytes() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get createdAt => $_getSZ(3);
  @$pb.TagNumber(4)
  set createdAt($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasCreatedAt() => $_has(3);
  @$pb.TagNumber(4)
  void clearCreatedAt() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get expiresAt => $_getSZ(4);
  @$pb.TagNumber(5)
  set expiresAt($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasExpiresAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearExpiresAt() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get friendGroupId => $_getSZ(5);
  @$pb.TagNumber(6)
  set friendGroupId($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasFriendGroupId() => $_has(5);
  @$pb.TagNumber(6)
  void clearFriendGroupId() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get id => $_getSZ(6);
  @$pb.TagNumber(7)
  set id($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasId() => $_has(6);
  @$pb.TagNumber(7)
  void clearId() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get senderPeerPublicKey => $_getSZ(7);
  @$pb.TagNumber(8)
  set senderPeerPublicKey($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasSenderPeerPublicKey() => $_has(7);
  @$pb.TagNumber(8)
  void clearSenderPeerPublicKey() => $_clearField(8);

  @$pb.TagNumber(9)
  $fixnum.Int64 get ttlSeconds => $_getI64(8);
  @$pb.TagNumber(9)
  set ttlSeconds($fixnum.Int64 value) => $_setInt64(8, value);
  @$pb.TagNumber(9)
  $core.bool hasTtlSeconds() => $_has(8);
  @$pb.TagNumber(9)
  void clearTtlSeconds() => $_clearField(9);
}

class FriendGroupMessageSendRequest extends $pb.GeneratedMessage {
  factory FriendGroupMessageSendRequest({
    $core.List<$core.int>? audioBase64,
    $core.String? audioContentType,
    $core.String? friendGroupId,
    $fixnum.Int64? ttlSeconds,
  }) {
    final result = create();
    if (audioBase64 != null) result.audioBase64 = audioBase64;
    if (audioContentType != null) result.audioContentType = audioContentType;
    if (friendGroupId != null) result.friendGroupId = friendGroupId;
    if (ttlSeconds != null) result.ttlSeconds = ttlSeconds;
    return result;
  }

  FriendGroupMessageSendRequest._();

  factory FriendGroupMessageSendRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMessageSendRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMessageSendRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..a<$core.List<$core.int>>(
        1, _omitFieldNames ? '' : 'audioBase64', $pb.PbFieldType.OY)
    ..aOS(2, _omitFieldNames ? '' : 'audioContentType')
    ..aOS(3, _omitFieldNames ? '' : 'friendGroupId')
    ..aInt64(4, _omitFieldNames ? '' : 'ttlSeconds')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageSendRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageSendRequest copyWith(
          void Function(FriendGroupMessageSendRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMessageSendRequest))
          as FriendGroupMessageSendRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageSendRequest create() =>
      FriendGroupMessageSendRequest._();
  @$core.override
  FriendGroupMessageSendRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageSendRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMessageSendRequest>(create);
  static FriendGroupMessageSendRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.List<$core.int> get audioBase64 => $_getN(0);
  @$pb.TagNumber(1)
  set audioBase64($core.List<$core.int> value) => $_setBytes(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAudioBase64() => $_has(0);
  @$pb.TagNumber(1)
  void clearAudioBase64() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get audioContentType => $_getSZ(1);
  @$pb.TagNumber(2)
  set audioContentType($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasAudioContentType() => $_has(1);
  @$pb.TagNumber(2)
  void clearAudioContentType() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get friendGroupId => $_getSZ(2);
  @$pb.TagNumber(3)
  set friendGroupId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasFriendGroupId() => $_has(2);
  @$pb.TagNumber(3)
  void clearFriendGroupId() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get ttlSeconds => $_getI64(3);
  @$pb.TagNumber(4)
  set ttlSeconds($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasTtlSeconds() => $_has(3);
  @$pb.TagNumber(4)
  void clearTtlSeconds() => $_clearField(4);
}

class FriendGroupMessageSendResponse extends $pb.GeneratedMessage {
  factory FriendGroupMessageSendResponse({
    FriendGroupMessageObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupMessageSendResponse._();

  factory FriendGroupMessageSendResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupMessageSendResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupMessageSendResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupMessageObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupMessageObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageSendResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupMessageSendResponse copyWith(
          void Function(FriendGroupMessageSendResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendGroupMessageSendResponse))
          as FriendGroupMessageSendResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageSendResponse create() =>
      FriendGroupMessageSendResponse._();
  @$core.override
  FriendGroupMessageSendResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupMessageSendResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupMessageSendResponse>(create);
  static FriendGroupMessageSendResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupMessageObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupMessageObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupMessageObject ensureValue() => $_ensure(0);
}

class FriendGroupObject extends $pb.GeneratedMessage {
  factory FriendGroupObject({
    $core.String? createdAt,
    $core.String? createdByPeerPublicKey,
    $core.String? description,
    $core.String? id,
    $0.FriendGroupMemberRole? myRole,
    $core.String? name,
    $core.String? updatedAt,
    $core.String? workspaceName,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (createdByPeerPublicKey != null)
      result.createdByPeerPublicKey = createdByPeerPublicKey;
    if (description != null) result.description = description;
    if (id != null) result.id = id;
    if (myRole != null) result.myRole = myRole;
    if (name != null) result.name = name;
    if (updatedAt != null) result.updatedAt = updatedAt;
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  FriendGroupObject._();

  factory FriendGroupObject.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupObject.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupObject',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'createdByPeerPublicKey')
    ..aOS(3, _omitFieldNames ? '' : 'description')
    ..aOS(4, _omitFieldNames ? '' : 'id')
    ..aE<$0.FriendGroupMemberRole>(5, _omitFieldNames ? '' : 'myRole',
        enumValues: $0.FriendGroupMemberRole.values)
    ..aOS(6, _omitFieldNames ? '' : 'name')
    ..aOS(7, _omitFieldNames ? '' : 'updatedAt')
    ..aOS(8, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupObject clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupObject copyWith(void Function(FriendGroupObject) updates) =>
      super.copyWith((message) => updates(message as FriendGroupObject))
          as FriendGroupObject;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupObject create() => FriendGroupObject._();
  @$core.override
  FriendGroupObject createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupObject getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupObject>(create);
  static FriendGroupObject? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get createdByPeerPublicKey => $_getSZ(1);
  @$pb.TagNumber(2)
  set createdByPeerPublicKey($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCreatedByPeerPublicKey() => $_has(1);
  @$pb.TagNumber(2)
  void clearCreatedByPeerPublicKey() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get description => $_getSZ(2);
  @$pb.TagNumber(3)
  set description($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDescription() => $_has(2);
  @$pb.TagNumber(3)
  void clearDescription() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get id => $_getSZ(3);
  @$pb.TagNumber(4)
  set id($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasId() => $_has(3);
  @$pb.TagNumber(4)
  void clearId() => $_clearField(4);

  @$pb.TagNumber(5)
  $0.FriendGroupMemberRole get myRole => $_getN(4);
  @$pb.TagNumber(5)
  set myRole($0.FriendGroupMemberRole value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasMyRole() => $_has(4);
  @$pb.TagNumber(5)
  void clearMyRole() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get name => $_getSZ(5);
  @$pb.TagNumber(6)
  set name($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasName() => $_has(5);
  @$pb.TagNumber(6)
  void clearName() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get updatedAt => $_getSZ(6);
  @$pb.TagNumber(7)
  set updatedAt($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasUpdatedAt() => $_has(6);
  @$pb.TagNumber(7)
  void clearUpdatedAt() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get workspaceName => $_getSZ(7);
  @$pb.TagNumber(8)
  set workspaceName($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasWorkspaceName() => $_has(7);
  @$pb.TagNumber(8)
  void clearWorkspaceName() => $_clearField(8);
}

class FriendGroupPutRequest extends $pb.GeneratedMessage {
  factory FriendGroupPutRequest({
    $core.String? description,
    $core.String? id,
    $core.String? name,
  }) {
    final result = create();
    if (description != null) result.description = description;
    if (id != null) result.id = id;
    if (name != null) result.name = name;
    return result;
  }

  FriendGroupPutRequest._();

  factory FriendGroupPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'description')
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..aOS(3, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupPutRequest copyWith(
          void Function(FriendGroupPutRequest) updates) =>
      super.copyWith((message) => updates(message as FriendGroupPutRequest))
          as FriendGroupPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupPutRequest create() => FriendGroupPutRequest._();
  @$core.override
  FriendGroupPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupPutRequest>(create);
  static FriendGroupPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get description => $_getSZ(0);
  @$pb.TagNumber(1)
  set description($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDescription() => $_has(0);
  @$pb.TagNumber(1)
  void clearDescription() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get name => $_getSZ(2);
  @$pb.TagNumber(3)
  set name($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasName() => $_has(2);
  @$pb.TagNumber(3)
  void clearName() => $_clearField(3);
}

class FriendGroupPutResponse extends $pb.GeneratedMessage {
  factory FriendGroupPutResponse({
    FriendGroupObject? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  FriendGroupPutResponse._();

  factory FriendGroupPutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendGroupPutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendGroupPutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<FriendGroupObject>(1, _omitFieldNames ? '' : 'value',
        subBuilder: FriendGroupObject.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupPutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendGroupPutResponse copyWith(
          void Function(FriendGroupPutResponse) updates) =>
      super.copyWith((message) => updates(message as FriendGroupPutResponse))
          as FriendGroupPutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendGroupPutResponse create() => FriendGroupPutResponse._();
  @$core.override
  FriendGroupPutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendGroupPutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendGroupPutResponse>(create);
  static FriendGroupPutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  FriendGroupObject get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(FriendGroupObject value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  FriendGroupObject ensureValue() => $_ensure(0);
}

class FriendInviteTokenClearRequest extends $pb.GeneratedMessage {
  factory FriendInviteTokenClearRequest() => create();

  FriendInviteTokenClearRequest._();

  factory FriendInviteTokenClearRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInviteTokenClearRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInviteTokenClearRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenClearRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenClearRequest copyWith(
          void Function(FriendInviteTokenClearRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendInviteTokenClearRequest))
          as FriendInviteTokenClearRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenClearRequest create() =>
      FriendInviteTokenClearRequest._();
  @$core.override
  FriendInviteTokenClearRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenClearRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInviteTokenClearRequest>(create);
  static FriendInviteTokenClearRequest? _defaultInstance;
}

class FriendInviteTokenClearResponse extends $pb.GeneratedMessage {
  factory FriendInviteTokenClearResponse() => create();

  FriendInviteTokenClearResponse._();

  factory FriendInviteTokenClearResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInviteTokenClearResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInviteTokenClearResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenClearResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenClearResponse copyWith(
          void Function(FriendInviteTokenClearResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendInviteTokenClearResponse))
          as FriendInviteTokenClearResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenClearResponse create() =>
      FriendInviteTokenClearResponse._();
  @$core.override
  FriendInviteTokenClearResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenClearResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInviteTokenClearResponse>(create);
  static FriendInviteTokenClearResponse? _defaultInstance;
}

class FriendInviteTokenCreateRequest extends $pb.GeneratedMessage {
  factory FriendInviteTokenCreateRequest() => create();

  FriendInviteTokenCreateRequest._();

  factory FriendInviteTokenCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInviteTokenCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInviteTokenCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenCreateRequest copyWith(
          void Function(FriendInviteTokenCreateRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendInviteTokenCreateRequest))
          as FriendInviteTokenCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenCreateRequest create() =>
      FriendInviteTokenCreateRequest._();
  @$core.override
  FriendInviteTokenCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInviteTokenCreateRequest>(create);
  static FriendInviteTokenCreateRequest? _defaultInstance;
}

class FriendInviteTokenCreateResponse extends $pb.GeneratedMessage {
  factory FriendInviteTokenCreateResponse({
    $core.String? expiresAt,
    $core.String? inviteToken,
  }) {
    final result = create();
    if (expiresAt != null) result.expiresAt = expiresAt;
    if (inviteToken != null) result.inviteToken = inviteToken;
    return result;
  }

  FriendInviteTokenCreateResponse._();

  factory FriendInviteTokenCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInviteTokenCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInviteTokenCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'expiresAt')
    ..aOS(2, _omitFieldNames ? '' : 'inviteToken')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenCreateResponse copyWith(
          void Function(FriendInviteTokenCreateResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendInviteTokenCreateResponse))
          as FriendInviteTokenCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenCreateResponse create() =>
      FriendInviteTokenCreateResponse._();
  @$core.override
  FriendInviteTokenCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInviteTokenCreateResponse>(
          create);
  static FriendInviteTokenCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get expiresAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set expiresAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasExpiresAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearExpiresAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get inviteToken => $_getSZ(1);
  @$pb.TagNumber(2)
  set inviteToken($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasInviteToken() => $_has(1);
  @$pb.TagNumber(2)
  void clearInviteToken() => $_clearField(2);
}

class FriendInviteTokenGetRequest extends $pb.GeneratedMessage {
  factory FriendInviteTokenGetRequest() => create();

  FriendInviteTokenGetRequest._();

  factory FriendInviteTokenGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInviteTokenGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInviteTokenGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenGetRequest copyWith(
          void Function(FriendInviteTokenGetRequest) updates) =>
      super.copyWith(
              (message) => updates(message as FriendInviteTokenGetRequest))
          as FriendInviteTokenGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenGetRequest create() =>
      FriendInviteTokenGetRequest._();
  @$core.override
  FriendInviteTokenGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInviteTokenGetRequest>(create);
  static FriendInviteTokenGetRequest? _defaultInstance;
}

class FriendInviteTokenGetResponse extends $pb.GeneratedMessage {
  factory FriendInviteTokenGetResponse({
    $core.String? expiresAt,
    $core.String? inviteToken,
  }) {
    final result = create();
    if (expiresAt != null) result.expiresAt = expiresAt;
    if (inviteToken != null) result.inviteToken = inviteToken;
    return result;
  }

  FriendInviteTokenGetResponse._();

  factory FriendInviteTokenGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendInviteTokenGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendInviteTokenGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'expiresAt')
    ..aOS(2, _omitFieldNames ? '' : 'inviteToken')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendInviteTokenGetResponse copyWith(
          void Function(FriendInviteTokenGetResponse) updates) =>
      super.copyWith(
              (message) => updates(message as FriendInviteTokenGetResponse))
          as FriendInviteTokenGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenGetResponse create() =>
      FriendInviteTokenGetResponse._();
  @$core.override
  FriendInviteTokenGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendInviteTokenGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendInviteTokenGetResponse>(create);
  static FriendInviteTokenGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get expiresAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set expiresAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasExpiresAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearExpiresAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get inviteToken => $_getSZ(1);
  @$pb.TagNumber(2)
  set inviteToken($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasInviteToken() => $_has(1);
  @$pb.TagNumber(2)
  void clearInviteToken() => $_clearField(2);
}

class FriendListRequest extends $pb.GeneratedMessage {
  factory FriendListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  FriendListRequest._();

  factory FriendListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendListRequest copyWith(void Function(FriendListRequest) updates) =>
      super.copyWith((message) => updates(message as FriendListRequest))
          as FriendListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendListRequest create() => FriendListRequest._();
  @$core.override
  FriendListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendListRequest>(create);
  static FriendListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);
}

class FriendListResponse extends $pb.GeneratedMessage {
  factory FriendListResponse({
    $core.bool? hasNext,
    $core.Iterable<FriendObject>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  FriendListResponse._();

  factory FriendListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<FriendObject>(2, _omitFieldNames ? '' : 'items',
        subBuilder: FriendObject.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendListResponse copyWith(void Function(FriendListResponse) updates) =>
      super.copyWith((message) => updates(message as FriendListResponse))
          as FriendListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendListResponse create() => FriendListResponse._();
  @$core.override
  FriendListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendListResponse>(create);
  static FriendListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<FriendObject> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class FriendObject extends $pb.GeneratedMessage {
  factory FriendObject({
    $core.String? createdAt,
    $core.String? id,
    $core.String? peerPublicKey,
    $core.String? updatedAt,
    $core.String? workspaceName,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (id != null) result.id = id;
    if (peerPublicKey != null) result.peerPublicKey = peerPublicKey;
    if (updatedAt != null) result.updatedAt = updatedAt;
    if (workspaceName != null) result.workspaceName = workspaceName;
    return result;
  }

  FriendObject._();

  factory FriendObject.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FriendObject.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FriendObject',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..aOS(3, _omitFieldNames ? '' : 'peerPublicKey')
    ..aOS(4, _omitFieldNames ? '' : 'updatedAt')
    ..aOS(5, _omitFieldNames ? '' : 'workspaceName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendObject clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FriendObject copyWith(void Function(FriendObject) updates) =>
      super.copyWith((message) => updates(message as FriendObject))
          as FriendObject;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FriendObject create() => FriendObject._();
  @$core.override
  FriendObject createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FriendObject getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FriendObject>(create);
  static FriendObject? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get peerPublicKey => $_getSZ(2);
  @$pb.TagNumber(3)
  set peerPublicKey($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPeerPublicKey() => $_has(2);
  @$pb.TagNumber(3)
  void clearPeerPublicKey() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get updatedAt => $_getSZ(3);
  @$pb.TagNumber(4)
  set updatedAt($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasUpdatedAt() => $_has(3);
  @$pb.TagNumber(4)
  void clearUpdatedAt() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get workspaceName => $_getSZ(4);
  @$pb.TagNumber(5)
  set workspaceName($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasWorkspaceName() => $_has(4);
  @$pb.TagNumber(5)
  void clearWorkspaceName() => $_clearField(5);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
